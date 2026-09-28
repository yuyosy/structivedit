package structivedit

import (
	"fmt"
	"math"
	"reflect"
	"strconv"

	"github.com/yuyosy/structivedit/document"
	"github.com/yuyosy/structivedit/internal/operation"
	"github.com/yuyosy/structivedit/internal/resolver"
	"github.com/yuyosy/structivedit/schema"
)

type addCandidate struct {
	kind          schema.Kind
	scalar        any
	hasScalar     bool
	inputID       InputID
	objectFields  []candidateField
	arrayItems    []*addCandidate
}

type candidateField struct {
	name  string
	value *addCandidate
}

type candidateBuilder struct {
	inputs []InputRequest
}

// PrepareAdd returns a plan for one schema-defined sequence item or object
// field. It does not allocate NodeIDs or change Document, Revision, or History.
func (e *Editor) PrepareAdd(parent document.NodeID, target AddTarget) (AddPlan, error) {
	if e == nil || e.doc == nil {
		return AddPlan{}, ErrInvalidInput
	}
	if !resolver.HasSchema(e.schema) {
		return AddPlan{}, ErrSchemaRequired
	}
	if _, ok := e.doc.Node(parent); !ok {
		return AddPlan{}, document.ErrNodeNotFound
	}
	candidate, inputs, err := e.prepareCandidate(parent, target)
	if err != nil {
		return AddPlan{}, err
	}
	_ = candidate // Prepare intentionally keeps only the requests and plan seal.
	publicInputs := cloneInputRequests(inputs)
	plan := AddPlan{
		Parent:      parent,
		Target:      target,
		Revision:    e.doc.Revision(),
		Inputs:      publicInputs,
		editorToken: e.identity,
	}
	plan.seal = addPlanSeal{
		parent:   parent,
		target:   target,
		revision: e.doc.Revision(),
		inputs:   cloneInputRequests(inputs),
	}
	return plan, nil
}

// CommitAdd revalidates plan ownership, current Revision, Schema target,
// requested inputs, and Capabilities before changing the Document.
func (e *Editor) CommitAdd(plan AddPlan, values map[InputID]any) (ApplyResult, error) {
	if e == nil || e.doc == nil {
		return ApplyResult{}, ErrInvalidInput
	}
	if !resolver.HasSchema(e.schema) {
		return ApplyResult{}, ErrSchemaRequired
	}
	if plan.editorToken != e.identity || plan.seal.parent == 0 || plan.Parent != plan.seal.parent || plan.Target != plan.seal.target || plan.Revision != plan.seal.revision || plan.Revision != e.doc.Revision() || !equalInputRequests(plan.Inputs, plan.seal.inputs) {
		return ApplyResult{}, ErrStaleAddPlan
	}
	candidate, inputs, err := e.prepareCandidate(plan.seal.parent, plan.seal.target)
	if err != nil || !equalInputRequests(inputs, plan.seal.inputs) {
		return ApplyResult{}, ErrStaleAddPlan
	}
	if err := validateInputSet(inputs, values); err != nil {
		return ApplyResult{}, err
	}
	if err := fillCandidate(candidate, values); err != nil {
		return ApplyResult{}, err
	}

	builder, err := document.NewBuilderFrom(e.doc)
	if err != nil {
		return ApplyResult{}, err
	}
	parentNode, _ := e.doc.Node(plan.Parent)
	var rootID document.NodeID
	created := make([]document.NodeID, 0)
	addOperation := operation.AddNode{ParentID: plan.Parent, ParentKind: parentNode.Kind()}
	switch plan.Target.Kind {
	case AddSequenceItem:
		items, _ := e.doc.SequenceItems(plan.Parent)
		rootID, err = materializeCandidate(builder, candidate, &created)
		if err != nil {
			return ApplyResult{}, err
		}
		addOperation.SequenceItems = append(items, rootID)
		addOperation.ParentKind = document.NodeSequence
		addOperation.ParentID = plan.Parent
	case AddObjectField:
		entries, _ := e.doc.MappingEntries(plan.Parent)
		keyID, keyErr := builder.NewScalar(plan.Target.Field)
		if keyErr != nil {
			return ApplyResult{}, keyErr
		}
		created = append(created, keyID)
		rootID, err = materializeCandidate(builder, candidate, &created)
		if err != nil {
			return ApplyResult{}, err
		}
		addOperation.MappingEntries = append(entries, document.MappingEntry{Key: keyID, Value: rootID})
		addOperation.ParentKind = document.NodeMapping
		addOperation.ParentID = plan.Parent
	default:
		return ApplyResult{}, ErrInvalidInput
	}
	if err := addOperation.Apply(builder); err != nil {
		return ApplyResult{}, err
	}
	updated, err := builder.Build()
	if err != nil {
		return ApplyResult{}, err
	}
	updatedIssues := publicIssues(resolver.ValidateSchema(updated, e.schema))
	previousIssues := e.issues
	parentIndex := 0
	if plan.Target.Kind == AddSequenceItem {
		items, _ := e.doc.SequenceItems(plan.Parent)
		parentIndex = len(items)
	} else {
		entries, _ := e.doc.MappingEntries(plan.Parent)
		parentIndex = len(entries)
	}
	effect := operation.Effect{Kind: operation.EffectNodeAdded, NodeID: rootID, ParentID: plan.Parent, ToIndex: parentIndex}
	if err := e.history.Record(e.doc, updated, created, []operation.Effect{effect}); err != nil {
		return ApplyResult{}, err
	}
	e.doc = updated
	e.issues = cloneValidationIssues(updatedIssues)
	result := ApplyResult{DocumentChanged: true, Revision: updated.Revision()}
	result.Events = append(result.Events, eventsForEffects([]operation.Effect{effect}, updated.Revision())...)
	if !equalIssues(previousIssues, updatedIssues) {
		result.Events = append(result.Events, Event{Kind: EventValidationChanged, Issues: cloneValidationIssues(updatedIssues), Revision: updated.Revision()})
	}
	result.Events = append(result.Events, Event{Kind: EventHistoryChanged, Revision: updated.Revision()})
	result.Events = cloneEvents(result.Events)
	return result, nil
}

func (e *Editor) prepareCandidate(parent document.NodeID, target AddTarget) (*addCandidate, []InputRequest, error) {
	if target.Kind == AddSequenceItem && target.Field != "" {
		return nil, nil, ErrInvalidInput
	}
	if target.Kind != AddSequenceItem && target.Kind != AddObjectField {
		return nil, nil, ErrInvalidInput
	}
	capabilities, valid := resolver.ResolveCapabilities(e.doc, parent, e.schema, e.policy)
	if !valid || !capabilities.Addable {
		return nil, nil, ErrNotAddable
	}
	fieldDefault := false
	var defaultValue any
	switch target.Kind {
	case AddSequenceItem:
		item, ok := resolver.SequenceItemSchema(e.doc, parent, e.schema)
		if !ok {
			return nil, nil, ErrNotAddable
		}
		items, _ := e.doc.SequenceItems(parent)
		path := fmt.Sprintf("[%d]", len(items))
		builder := &candidateBuilder{}
		candidate := builder.build(item, path, false, nil)
		return candidate, builder.inputs, nil
	case AddObjectField:
		fields, ok := resolver.ObjectFieldsAt(e.doc, parent, e.schema)
		if !ok {
			return nil, nil, ErrNotAddable
		}
		field, exists := schemaField(fields, target.Field)
		if !exists || objectFieldExists(e.doc, parent, target.Field) {
			return nil, nil, ErrNotAddable
		}
		fieldDefault = field.HasDefault
		defaultValue = field.Default
		builder := &candidateBuilder{}
		candidate := builder.build(field.Schema, target.Field, fieldDefault, defaultValue)
		return candidate, builder.inputs, nil
	default:
		return nil, nil, ErrInvalidInput
	}
}

func (builder *candidateBuilder) build(node schema.Node, fieldPath string, fieldHasDefault bool, fieldDefault any) *addCandidate {
	candidate := &addCandidate{kind: node.Kind}
	switch node.Kind {
	case schema.StringKind, schema.BoolKind, schema.IntegerKind, schema.FloatKind, schema.NullKind:
		hasDefault := fieldHasDefault || node.Scalar.HasDefault
		defaultValue := node.Scalar.Default
		if fieldHasDefault {
			defaultValue = fieldDefault
		}
		id := InputID("input-" + strconv.Itoa(len(builder.inputs)))
		label := node.Label
		if label == "" {
			label = fieldPath
		}
		request := InputRequest{
			ID:            id,
			FieldPath:     fieldPath,
			Label:         label,
			ExpectedKind:  node.Kind,
			ValueRequired: !hasDefault,
			HasDefault:    hasDefault,
			Default:       defaultValue,
		}
		builder.inputs = append(builder.inputs, request)
		candidate.inputID = id
		candidate.hasScalar = hasDefault
		candidate.scalar = defaultValue
	case schema.ObjectKind:
		for _, field := range node.Object.Fields {
			include := field.Required
			if !include && isScalarSchema(field.Schema.Kind) {
				include = field.HasDefault || field.Schema.Scalar.HasDefault
			}
			if !include {
				continue
			}
			path := joinFieldPath(fieldPath, field.Name)
			child := builder.build(field.Schema, path, field.HasDefault, field.Default)
			candidate.objectFields = append(candidate.objectFields, candidateField{name: field.Name, value: child})
		}
	case schema.ArrayKind:
		count := 0
		if node.Array.MinItems != nil {
			count = *node.Array.MinItems
		}
		for index := 0; index < count; index++ {
			path := fieldPath + "[" + strconv.Itoa(index) + "]"
			candidate.arrayItems = append(candidate.arrayItems, builder.build(node.Array.Item, path, false, nil))
		}
	}
	return candidate
}

func validateInputSet(requests []InputRequest, values map[InputID]any) error {
	known := make(map[InputID]InputRequest, len(requests))
	for _, request := range requests {
		known[request.ID] = request
	}
	for id := range values {
		if _, ok := known[id]; !ok {
			return ErrInvalidInput
		}
	}
	for _, request := range requests {
		value, supplied := values[request.ID]
		if !supplied {
			if request.ValueRequired {
				return ErrMissingInput
			}
			if !request.HasDefault {
				return ErrStaleAddPlan
			}
			continue
		}
		kind, _, err := scalarInput(value)
		if err != nil {
			return ErrTypeMismatch
		}
		expected, valid := documentScalarKind(request.ExpectedKind)
		if !valid || kind != expected {
			return ErrTypeMismatch
		}
	}
	return nil
}

func fillCandidate(candidate *addCandidate, values map[InputID]any) error {
	if candidate == nil {
		return ErrStaleAddPlan
	}
	switch candidate.kind {
	case schema.StringKind, schema.BoolKind, schema.IntegerKind, schema.FloatKind, schema.NullKind:
		value, supplied := values[candidate.inputID]
		if supplied {
			candidate.scalar = value
			candidate.hasScalar = true
		}
		if !candidate.hasScalar {
			return ErrMissingInput
		}
		return nil
	case schema.ObjectKind:
		for _, field := range candidate.objectFields {
			if err := fillCandidate(field.value, values); err != nil {
				return err
			}
		}
	case schema.ArrayKind:
		for _, item := range candidate.arrayItems {
			if err := fillCandidate(item, values); err != nil {
				return err
			}
		}
	default:
		return ErrInvalidSchema
	}
	return nil
}

func materializeCandidate(builder *document.Builder, candidate *addCandidate, created *[]document.NodeID) (document.NodeID, error) {
	if builder == nil || candidate == nil {
		return 0, document.ErrInvalidDocument
	}
	switch candidate.kind {
	case schema.StringKind, schema.BoolKind, schema.IntegerKind, schema.FloatKind, schema.NullKind:
		if !candidate.hasScalar {
			return 0, ErrMissingInput
		}
		id, err := builder.NewScalar(candidate.scalar)
		if err == nil {
			*created = append(*created, id)
		}
		return id, err
	case schema.ObjectKind:
		entries := make([]document.MappingEntry, 0, len(candidate.objectFields))
		for _, field := range candidate.objectFields {
			keyID, err := builder.NewScalar(field.name)
			if err != nil {
				return 0, err
			}
			*created = append(*created, keyID)
			valueID, err := materializeCandidate(builder, field.value, created)
			if err != nil {
				return 0, err
			}
			entries = append(entries, document.MappingEntry{Key: keyID, Value: valueID})
		}
		id, err := builder.NewMapping(entries)
		if err == nil {
			*created = append(*created, id)
		}
		return id, err
	case schema.ArrayKind:
		items := make([]document.NodeID, 0, len(candidate.arrayItems))
		for _, item := range candidate.arrayItems {
			itemID, err := materializeCandidate(builder, item, created)
			if err != nil {
				return 0, err
			}
			items = append(items, itemID)
		}
		id, err := builder.NewSequence(items)
		if err == nil {
			*created = append(*created, id)
		}
		return id, err
	default:
		return 0, ErrInvalidSchema
	}
}

func objectFieldExists(doc *document.Document, parent document.NodeID, name string) bool {
	entries, ok := doc.MappingEntries(parent)
	if !ok {
		return false
	}
	for _, entry := range entries {
		kind, value, scalar := doc.Scalar(entry.Key)
		if scalar && kind == document.ScalarString && value.(string) == name {
			return true
		}
	}
	return false
}

func schemaField(fields []schema.Field, name string) (schema.Field, bool) {
	for _, field := range fields {
		if field.Name == name {
			return field, true
		}
	}
	return schema.Field{}, false
}

func isScalarSchema(kind schema.Kind) bool { return kind <= schema.NullKind }

func joinFieldPath(parent, field string) string {
	if parent == "" {
		return field
	}
	return parent + "." + field
}

func documentScalarKind(kind schema.Kind) (document.ScalarKind, bool) {
	switch kind {
	case schema.StringKind:
		return document.ScalarString, true
	case schema.BoolKind:
		return document.ScalarBool, true
	case schema.IntegerKind:
		return document.ScalarInteger, true
	case schema.FloatKind:
		return document.ScalarFloat, true
	case schema.NullKind:
		return document.ScalarNull, true
	default:
		return 0, false
	}
}

func cloneInputRequests(inputs []InputRequest) []InputRequest {
	return append([]InputRequest(nil), inputs...)
}

func equalInputRequests(left, right []InputRequest) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].ID != right[index].ID || left[index].FieldPath != right[index].FieldPath || left[index].Label != right[index].Label || left[index].ExpectedKind != right[index].ExpectedKind || left[index].ValueRequired != right[index].ValueRequired || left[index].HasDefault != right[index].HasDefault || !equalInputDefault(left[index].ExpectedKind, left[index].Default, right[index].Default) {
			return false
		}
	}
	return true
}

func equalInputDefault(kind schema.Kind, left, right any) bool {
	if kind == schema.FloatKind {
		leftFloat, leftOK := left.(float64)
		rightFloat, rightOK := right.(float64)
		return leftOK && rightOK && math.Float64bits(leftFloat) == math.Float64bits(rightFloat)
	}
	return reflect.DeepEqual(left, right)
}
