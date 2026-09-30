package structivedit

import (
	"math"
	"reflect"

	"github.com/yuyosy/structivedit/document"
	"github.com/yuyosy/structivedit/internal/operation"
	"github.com/yuyosy/structivedit/internal/resolver"
	"github.com/yuyosy/structivedit/schema"
)

// Editor coordinates an immutable Document, its Schema and Policy, logical
// focus, and the current validation result.
type Editor struct {
	doc               *document.Document
	schema            resolver.CompiledSchema
	policy            resolver.CompiledPolicy
	focused           document.NodeID
	history           *operation.History
	issues            []ValidationIssue
	identity          *editorIdentity
	viewCache         []NodeView
	viewCacheByID     map[document.NodeID]int
	viewCacheRevision document.Revision
	viewCacheValid    bool
}

type editorIdentity struct {
	marker byte
}

// New creates an Editor for a valid immutable Document snapshot.
func New(doc *document.Document, options ...Option) (*Editor, error) {
	if doc == nil || doc.Root() == 0 {
		return nil, document.ErrInvalidDocument
	}
	settings := editorOptions{historyLimit: 100}
	for _, option := range options {
		if option == nil {
			return nil, ErrInvalidInput
		}
		if err := option(&settings); err != nil {
			return nil, err
		}
	}

	var compiledSchema resolver.CompiledSchema
	if settings.schema != nil {
		var err error
		compiledSchema, err = resolver.CompileSchema(*settings.schema)
		if err != nil {
			return nil, ErrInvalidSchema
		}
	}
	compiledPolicy, err := settings.policy.compile()
	if err != nil {
		return nil, ErrInvalidPolicy
	}
	e := &Editor{
		doc:      doc,
		schema:   compiledSchema,
		policy:   compiledPolicy,
		history:  operation.NewHistory(settings.historyLimit),
		identity: &editorIdentity{marker: 1},
	}
	e.issues = publicIssues(resolver.ValidateSchema(doc, compiledSchema))
	return e, nil
}

// Document returns the current immutable Document snapshot.
func (e *Editor) Document() *document.Document {
	if e == nil {
		return nil
	}
	return e.doc
}

// Apply validates and applies one built-in Action transactionally.
func (e *Editor) Apply(action Action) (ApplyResult, error) {
	if e == nil {
		return ApplyResult{}, ErrInvalidInput
	}
	switch typed := action.(type) {
	case SetValue:
		return e.applySetValue(typed.NodeID, typed.Value)
	case *SetValue:
		if typed == nil {
			return ApplyResult{}, ErrInvalidInput
		}
		return e.applySetValue(typed.NodeID, typed.Value)
	case Toggle:
		return e.applyToggle(typed.NodeID)
	case *Toggle:
		if typed == nil {
			return ApplyResult{}, ErrInvalidInput
		}
		return e.applyToggle(typed.NodeID)
	case Focus:
		return e.applyFocus(typed.NodeID)
	case *Focus:
		if typed == nil {
			return ApplyResult{}, ErrInvalidInput
		}
		return e.applyFocus(typed.NodeID)
	case Undo, *Undo:
		return e.applyUndo()
	case Redo, *Redo:
		return e.applyRedo()
	case Add:
		return e.CommitAdd(typed.Plan, typed.Values)
	case *Add:
		if typed == nil {
			return ApplyResult{}, ErrInvalidInput
		}
		return e.CommitAdd(typed.Plan, typed.Values)
	case Delete:
		return e.applyDelete(typed.NodeID)
	case *Delete:
		if typed == nil {
			return ApplyResult{}, ErrInvalidInput
		}
		return e.applyDelete(typed.NodeID)
	case Move:
		return e.applyMove(typed.NodeID, typed.ToIndex)
	case *Move:
		if typed == nil {
			return ApplyResult{}, ErrInvalidInput
		}
		return e.applyMove(typed.NodeID, typed.ToIndex)
	default:
		return ApplyResult{}, ErrInvalidInput
	}
}

// View returns a value-copy view of id.
func (e *Editor) View(id document.NodeID) (NodeView, error) {
	if e == nil || e.doc == nil {
		return NodeView{}, document.ErrNodeNotFound
	}
	if e.viewCacheValid && e.viewCacheRevision == e.doc.Revision() {
		if index, ok := e.viewCacheByID[id]; ok {
			return cloneNodeView(e.viewCache[index]), nil
		}
	}
	return e.view(id)
}

// Views returns all ownership nodes in preorder. Mapping entries are visited
// as key then value, and Reference targets are not traversed.
func (e *Editor) Views() []NodeView {
	if e == nil || e.doc == nil || e.doc.Root() == 0 {
		return nil
	}
	if e.viewCacheValid && e.viewCacheRevision == e.doc.Revision() {
		return cloneNodeViews(e.viewCache)
	}
	issuesByNode := make(map[document.NodeID][]ValidationIssue, len(e.issues))
	for _, issue := range e.issues {
		issuesByNode[issue.NodeID] = append(issuesByNode[issue.NodeID], issue)
	}
	views := make([]NodeView, 0)
	stack := []document.NodeID{e.doc.Root()}
	for len(stack) > 0 {
		last := len(stack) - 1
		id := stack[last]
		stack = stack[:last]
		view, err := e.viewWithIssues(id, issuesByNode[id])
		if err != nil {
			continue
		}
		views = append(views, view)
		children := e.doc.Children(id)
		for index := len(children) - 1; index >= 0; index-- {
			stack = append(stack, children[index])
		}
	}
	e.viewCache = cloneNodeViews(views)
	e.viewCacheByID = make(map[document.NodeID]int, len(e.viewCache))
	for index, view := range e.viewCache {
		e.viewCacheByID[view.ID] = index
	}
	e.viewCacheRevision = e.doc.Revision()
	e.viewCacheValid = true
	return views
}

// Focus sets the logical focus. NodeID zero clears focus.
func (e *Editor) Focus(id document.NodeID) error {
	_, err := e.Apply(Focus{NodeID: id})
	return err
}

// Focused returns the current logical focus, if one is set.
func (e *Editor) Focused() (document.NodeID, bool) {
	if e == nil || e.focused == 0 {
		return 0, false
	}
	return e.focused, true
}

// IsDirty reports whether the current Document differs from the last position
// marked clean.
func (e *Editor) IsDirty() bool {
	return e != nil && e.history != nil && e.history.Dirty()
}

// MarkClean records the current Document position as successfully saved.
func (e *Editor) MarkClean() {
	if e != nil && e.history != nil {
		e.history.MarkClean()
	}
}

// Issues returns a copy of the most recent complete validation result.
func (e *Editor) Issues() []ValidationIssue {
	if e == nil {
		return nil
	}
	return cloneValidationIssues(e.issues)
}

// Validate recomputes the current complete validation result and returns a copy.
func (e *Editor) Validate() []ValidationIssue {
	if e == nil || e.doc == nil {
		return nil
	}
	issues := publicIssues(resolver.ValidateSchema(e.doc, e.schema))
	e.issues = cloneValidationIssues(issues)
	e.invalidateViews()
	return cloneValidationIssues(issues)
}

// HasErrors reports whether the current validation result contains an Error.
func (e *Editor) HasErrors() bool {
	if e == nil {
		return false
	}
	for _, issue := range e.issues {
		if issue.Severity == schema.SeverityError {
			return true
		}
	}
	return false
}

func (e *Editor) view(id document.NodeID) (NodeView, error) {
	issues := make([]ValidationIssue, 0)
	for _, issue := range e.issues {
		if issue.NodeID == id {
			issues = append(issues, issue)
		}
	}
	return e.viewWithIssues(id, issues)
}

func (e *Editor) viewWithIssues(id document.NodeID, issues []ValidationIssue) (NodeView, error) {
	node, ok := e.doc.Node(id)
	if !ok {
		return NodeView{}, document.ErrNodeNotFound
	}
	path, err := e.doc.Path(id)
	if err != nil {
		return NodeView{}, err
	}
	view := NodeView{
		ID:           id,
		Path:         path,
		Kind:         node.Kind(),
		Restrictions: node.Restrictions(),
	}
	view.Parent, view.HasParent = e.doc.Parent(id)
	if _, value, scalar := node.Scalar(); scalar {
		view.ScalarValue = value
	}
	if node.Kind() == document.NodeSequence {
		view.SequenceItems, _ = e.doc.SequenceItems(id)
	}
	if node.Kind() == document.NodeMapping {
		view.MappingEntries, _ = e.doc.MappingEntries(id)
	}
	if target, reference := node.ReferenceTarget(); reference {
		view.ReferenceTarget = target
		view.HasReferenceTarget = true
	}
	if capabilities, valid := resolver.ResolveCapabilities(e.doc, id, e.schema, e.policy); valid {
		view.Capabilities = publicCapabilities(capabilities)
	}
	view.Issues = cloneValidationIssues(issues)
	return view, nil
}

func (e *Editor) invalidateViews() {
	if e != nil {
		e.viewCache = nil
		e.viewCacheByID = nil
		e.viewCacheValid = false
	}
}

func cloneNodeViews(views []NodeView) []NodeView {
	cloned := make([]NodeView, len(views))
	for index, view := range views {
		cloned[index] = cloneNodeView(view)
	}
	return cloned
}

func cloneNodeView(view NodeView) NodeView {
	view.SequenceItems = append([]document.NodeID(nil), view.SequenceItems...)
	view.MappingEntries = append([]document.MappingEntry(nil), view.MappingEntries...)
	view.Issues = cloneValidationIssues(view.Issues)
	return view
}

func (e *Editor) applyFocus(id document.NodeID) (ApplyResult, error) {
	if id != 0 {
		if _, ok := e.doc.Node(id); !ok {
			return ApplyResult{}, document.ErrNodeNotFound
		}
	}
	previous := e.focused
	if previous == id {
		return ApplyResult{Revision: e.doc.Revision()}, nil
	}
	e.focused = id
	result := ApplyResult{
		Revision: e.doc.Revision(),
		Events: []Event{{
			Kind:          EventFocusChanged,
			PreviousFocus: previous,
			Focus:         id,
			Revision:      e.doc.Revision(),
		}},
	}
	result.Events = cloneEvents(result.Events)
	return result, nil
}

func (e *Editor) applyToggle(id document.NodeID) (ApplyResult, error) {
	node, ok := e.doc.Node(id)
	if !ok {
		return ApplyResult{}, document.ErrNodeNotFound
	}
	if node.Kind() == document.NodeReference {
		return ApplyResult{}, ErrReferenceReadOnly
	}
	kind, value, scalar := node.Scalar()
	if !scalar || kind != document.ScalarBool {
		return ApplyResult{}, ErrTypeMismatch
	}
	return e.applySetValue(id, !value.(bool))
}

func (e *Editor) applySetValue(id document.NodeID, value any) (ApplyResult, error) {
	node, ok := e.doc.Node(id)
	if !ok {
		return ApplyResult{}, document.ErrNodeNotFound
	}
	if node.Kind() == document.NodeReference {
		return ApplyResult{}, ErrReferenceReadOnly
	}
	if parent, hasParent := e.doc.Parent(id); hasParent && parent.Role == document.ParentMappingKey {
		return ApplyResult{}, ErrNotEditable
	}
	if node.Restrictions()&document.RestrictionReadOnly != 0 {
		return ApplyResult{}, ErrNotEditable
	}
	currentKind, currentValue, scalar := node.Scalar()
	if !scalar {
		return ApplyResult{}, ErrTypeMismatch
	}
	inputKind, normalized, err := scalarInput(value)
	if err != nil {
		return ApplyResult{}, err
	}
	if inputKind != currentKind {
		return ApplyResult{}, ErrTypeMismatch
	}
	capabilities, valid := resolver.ResolveCapabilities(e.doc, id, e.schema, e.policy)
	if !valid || !capabilities.Editable {
		return ApplyResult{}, ErrNotEditable
	}
	if equalScalarValue(currentKind, currentValue, normalized) {
		return ApplyResult{Revision: e.doc.Revision()}, nil
	}

	builder, err := document.NewBuilderFrom(e.doc)
	if err != nil {
		return ApplyResult{}, err
	}
	op := operation.SetValue{NodeID: id, Value: normalized}
	if err := op.Apply(builder); err != nil {
		return ApplyResult{}, err
	}
	updated, err := builder.Build()
	if err != nil {
		return ApplyResult{}, err
	}
	updatedIssues := publicIssues(resolver.ValidateValueChange(updated, e.schema, internalIssues(e.issues), id))
	previousIssues := e.issues
	effect := operation.Effect{
		Kind:        operation.EffectValueChanged,
		NodeID:      id,
		BeforeValue: currentValue,
		AfterValue:  normalized,
	}
	if err := e.history.Record(e.doc, updated, []document.NodeID{id}, []operation.Effect{effect}); err != nil {
		return ApplyResult{}, err
	}
	e.doc = updated
	e.issues = cloneValidationIssues(updatedIssues)
	e.invalidateViews()
	result := ApplyResult{DocumentChanged: true, Revision: updated.Revision()}
	result.Events = append(result.Events, eventsForEffects([]operation.Effect{effect}, updated.Revision())...)
	if !equalIssues(previousIssues, updatedIssues) {
		result.Events = append(result.Events, Event{
			Kind:     EventValidationChanged,
			Issues:   cloneValidationIssues(updatedIssues),
			Revision: updated.Revision(),
		})
	}
	result.Events = append(result.Events, Event{Kind: EventHistoryChanged, Revision: updated.Revision()})
	result.Events = cloneEvents(result.Events)
	return result, nil
}

func (e *Editor) applyUndo() (ApplyResult, error) {
	entry := e.history.UndoCandidate()
	if entry == nil {
		return ApplyResult{}, ErrNoUndo
	}
	builder, err := document.NewBuilderFrom(e.doc)
	if err != nil {
		return ApplyResult{}, err
	}
	if err := entry.Undo(builder); err != nil {
		return ApplyResult{}, err
	}
	updated, err := builder.Build()
	if err != nil {
		return ApplyResult{}, err
	}
	effects := entry.Effects(true)
	updatedIssues := publicIssues(validateAfterEffects(updated, e.schema, e.issues, effects))
	if !e.history.CommitUndo() {
		return ApplyResult{}, ErrNoUndo
	}
	previousIssues := e.issues
	e.doc = updated
	e.issues = cloneValidationIssues(updatedIssues)
	e.invalidateViews()
	result := ApplyResult{DocumentChanged: true, Revision: updated.Revision()}
	result.Events = append(result.Events, eventsForEffects(effects, updated.Revision())...)
	if !equalIssues(previousIssues, updatedIssues) {
		result.Events = append(result.Events, Event{Kind: EventValidationChanged, Issues: cloneValidationIssues(updatedIssues), Revision: updated.Revision()})
	}
	result.Events = append(result.Events, Event{Kind: EventHistoryChanged, Revision: updated.Revision()})
	result.Events = cloneEvents(result.Events)
	return result, nil
}

func (e *Editor) applyRedo() (ApplyResult, error) {
	entry := e.history.RedoCandidate()
	if entry == nil {
		return ApplyResult{}, ErrNoRedo
	}
	builder, err := document.NewBuilderFrom(e.doc)
	if err != nil {
		return ApplyResult{}, err
	}
	if err := entry.Apply(builder); err != nil {
		return ApplyResult{}, err
	}
	updated, err := builder.Build()
	if err != nil {
		return ApplyResult{}, err
	}
	effects := entry.Effects(false)
	updatedIssues := publicIssues(validateAfterEffects(updated, e.schema, e.issues, effects))
	if !e.history.CommitRedo() {
		return ApplyResult{}, ErrNoRedo
	}
	previousIssues := e.issues
	e.doc = updated
	e.issues = cloneValidationIssues(updatedIssues)
	e.invalidateViews()
	result := ApplyResult{DocumentChanged: true, Revision: updated.Revision()}
	result.Events = append(result.Events, eventsForEffects(effects, updated.Revision())...)
	if !equalIssues(previousIssues, updatedIssues) {
		result.Events = append(result.Events, Event{Kind: EventValidationChanged, Issues: cloneValidationIssues(updatedIssues), Revision: updated.Revision()})
	}
	result.Events = append(result.Events, Event{Kind: EventHistoryChanged, Revision: updated.Revision()})
	result.Events = cloneEvents(result.Events)
	return result, nil
}

func scalarInput(value any) (document.ScalarKind, any, error) {
	if value == nil {
		return document.ScalarNull, nil, nil
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.String:
		return document.ScalarString, reflected.String(), nil
	case reflect.Bool:
		return document.ScalarBool, reflected.Bool(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return document.ScalarInteger, reflected.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		unsigned := reflected.Uint()
		if unsigned > uint64(^uint64(0)>>1) {
			return 0, nil, document.ErrInvalidScalar
		}
		return document.ScalarInteger, int64(unsigned), nil
	case reflect.Float32, reflect.Float64:
		return document.ScalarFloat, reflected.Float(), nil
	default:
		return 0, nil, document.ErrInvalidScalar
	}
}

func equalScalarValue(kind document.ScalarKind, left, right any) bool {
	switch kind {
	case document.ScalarString:
		return left.(string) == right.(string)
	case document.ScalarBool:
		return left.(bool) == right.(bool)
	case document.ScalarInteger:
		return left.(int64) == right.(int64)
	case document.ScalarFloat:
		return math.Float64bits(left.(float64)) == math.Float64bits(right.(float64))
	case document.ScalarNull:
		return left == nil && right == nil
	default:
		return false
	}
}

func publicIssues(issues []resolver.ValidationIssue) []ValidationIssue {
	converted := make([]ValidationIssue, len(issues))
	for index, issue := range issues {
		converted[index] = ValidationIssue{
			NodeID:   issue.NodeID,
			Path:     issue.Path,
			Code:     issue.Code,
			Message:  issue.Message,
			Severity: issue.Severity,
		}
	}
	return converted
}

func internalIssues(issues []ValidationIssue) []resolver.ValidationIssue {
	converted := make([]resolver.ValidationIssue, len(issues))
	for index, issue := range issues {
		converted[index] = resolver.ValidationIssue{
			NodeID:   issue.NodeID,
			Path:     issue.Path,
			Code:     issue.Code,
			Message:  issue.Message,
			Severity: issue.Severity,
		}
	}
	return converted
}

func validateAfterEffects(doc *document.Document, compiled resolver.CompiledSchema, previous []ValidationIssue, effects []operation.Effect) []resolver.ValidationIssue {
	if len(effects) == 1 && effects[0].Kind == operation.EffectValueChanged {
		return resolver.ValidateValueChange(doc, compiled, internalIssues(previous), effects[0].NodeID)
	}
	return resolver.ValidateSchema(doc, compiled)
}

func publicCapabilities(capabilities resolver.ResolvedPolicy) Capabilities {
	return Capabilities{
		Editable:    capabilities.Editable,
		Addable:     capabilities.Addable,
		Deletable:   capabilities.Deletable,
		Reorderable: capabilities.Reorderable,
	}
}

func equalIssues(left, right []ValidationIssue) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].NodeID != right[index].NodeID || !left[index].Path.Equal(right[index].Path) || left[index].Code != right[index].Code || left[index].Message != right[index].Message || left[index].Severity != right[index].Severity {
			return false
		}
	}
	return true
}

func eventsForEffects(effects []operation.Effect, revision document.Revision) []Event {
	events := make([]Event, 0, len(effects))
	for _, effect := range effects {
		event := Event{
			NodeID:      effect.NodeID,
			ParentID:    effect.ParentID,
			FromIndex:   effect.FromIndex,
			ToIndex:     effect.ToIndex,
			BeforeValue: effect.BeforeValue,
			AfterValue:  effect.AfterValue,
			Revision:    revision,
		}
		switch effect.Kind {
		case operation.EffectValueChanged:
			event.Kind = EventValueChanged
		case operation.EffectNodeAdded:
			event.Kind = EventNodeAdded
		case operation.EffectNodeDeleted:
			event.Kind = EventNodeDeleted
		case operation.EffectNodeMoved:
			event.Kind = EventNodeMoved
		default:
			continue
		}
		events = append(events, event)
	}
	return events
}
