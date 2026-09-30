package resolver

import (
	"fmt"
	"math"
	"reflect"
	"sort"

	"github.com/yuyosy/structivedit/document"
	"github.com/yuyosy/structivedit/schema"
)

// CompiledSchema is an immutable, validated copy of a public Schema tree.
type CompiledSchema struct {
	root                *schema.Node
	hasCustomValidators bool
}

// ValidationIssue is the internal form of a public validation result.
type ValidationIssue struct {
	NodeID   document.NodeID
	Path     document.Path
	Code     string
	Message  string
	Severity schema.Severity
}

// CompileSchema validates root and deep-copies its slices and range pointers.
func CompileSchema(root schema.Node) (CompiledSchema, error) {
	active := make(map[uintptr]bool)
	cloned, err := cloneSchemaNode(root, active)
	if err != nil {
		return CompiledSchema{}, schema.ErrInvalidSchema
	}
	return CompiledSchema{root: &cloned, hasCustomValidators: schemaHasCustomValidators(cloned)}, nil
}

func schemaHasCustomValidators(node schema.Node) bool {
	switch node.Kind {
	case schema.StringKind, schema.BoolKind, schema.IntegerKind, schema.FloatKind, schema.NullKind:
		return len(node.Scalar.Validators) > 0
	case schema.ObjectKind:
		if len(node.Object.Validators) > 0 {
			return true
		}
		for _, field := range node.Object.Fields {
			if schemaHasCustomValidators(field.Schema) {
				return true
			}
		}
	case schema.ArrayKind:
		return node.Array.Item != nil && schemaHasCustomValidators(*node.Array.Item)
	}
	return false
}

func cloneSchemaNode(node schema.Node, active map[uintptr]bool) (schema.Node, error) {
	if node.Kind > schema.ArrayKind ||
		!validSchemaDecision(node.Capabilities.Add) ||
		!validSchemaDecision(node.Capabilities.Delete) ||
		!validSchemaDecision(node.Capabilities.Reorder) {
		return schema.Node{}, schema.ErrInvalidSchema
	}

	scalarSet := schemaScalarHasData(node.Scalar)
	objectSet := schemaObjectHasData(node.Object)
	arraySet := schemaArrayHasData(node.Array)
	switch node.Kind {
	case schema.StringKind, schema.BoolKind, schema.IntegerKind, schema.FloatKind, schema.NullKind:
		if objectSet || arraySet {
			return schema.Node{}, schema.ErrInvalidSchema
		}
	case schema.ObjectKind:
		if scalarSet || arraySet {
			return schema.Node{}, schema.ErrInvalidSchema
		}
	case schema.ArrayKind:
		if scalarSet || objectSet {
			return schema.Node{}, schema.ErrInvalidSchema
		}
	}

	cloned := node
	switch node.Kind {
	case schema.StringKind, schema.BoolKind, schema.IntegerKind, schema.FloatKind, schema.NullKind:
		if node.Scalar.HasDefault {
			if !validCanonicalValue(node.Kind, node.Scalar.Default) {
				return schema.Node{}, schema.ErrInvalidSchema
			}
		} else if node.Scalar.Default != nil {
			return schema.Node{}, schema.ErrInvalidSchema
		}
		cloned.Scalar.Enum = append([]any(nil), node.Scalar.Enum...)
		if node.Scalar.Enum != nil && len(node.Scalar.Enum) == 0 {
			cloned.Scalar.Enum = make([]any, 0)
		}
		for _, value := range node.Scalar.Enum {
			if !validCanonicalValue(node.Kind, value) {
				return schema.Node{}, schema.ErrInvalidSchema
			}
		}
		if node.Scalar.Min != nil || node.Scalar.Max != nil {
			if node.Kind != schema.IntegerKind && node.Kind != schema.FloatKind {
				return schema.Node{}, schema.ErrInvalidSchema
			}
			if node.Scalar.Min != nil && !validRangeValue(node.Kind, node.Scalar.Min) {
				return schema.Node{}, schema.ErrInvalidSchema
			}
			if node.Scalar.Max != nil && !validRangeValue(node.Kind, node.Scalar.Max) {
				return schema.Node{}, schema.ErrInvalidSchema
			}
			if node.Scalar.Min != nil && node.Scalar.Max != nil && rangeGreater(node.Kind, node.Scalar.Min, node.Scalar.Max) {
				return schema.Node{}, schema.ErrInvalidSchema
			}
		}
		cloned.Scalar.Validators = append([]schema.Validator(nil), node.Scalar.Validators...)
	case schema.ObjectKind:
		if node.Object.Fields != nil {
			identity := reflect.ValueOf(node.Object.Fields).Pointer()
			if identity != 0 && active[identity] {
				return schema.Node{}, schema.ErrInvalidSchema
			}
			if identity != 0 {
				active[identity] = true
				defer delete(active, identity)
			}
		}
		cloned.Object.Fields = make([]schema.Field, len(node.Object.Fields))
		seenFields := make(map[string]struct{}, len(node.Object.Fields))
		for index, field := range node.Object.Fields {
			if _, exists := seenFields[field.Name]; exists {
				return schema.Node{}, schema.ErrInvalidSchema
			}
			seenFields[field.Name] = struct{}{}
			child, err := cloneSchemaNode(field.Schema, active)
			if err != nil {
				return schema.Node{}, schema.ErrInvalidSchema
			}
			if field.HasDefault {
				if !isScalarSchemaKind(child.Kind) || !validCanonicalValue(child.Kind, field.Default) {
					return schema.Node{}, schema.ErrInvalidSchema
				}
			} else if field.Default != nil {
				return schema.Node{}, schema.ErrInvalidSchema
			}
			cloned.Object.Fields[index] = schema.Field{
				Name:       field.Name,
				Schema:     child,
				Required:   field.Required,
				HasDefault: field.HasDefault,
				Default:    field.Default,
			}
		}
		cloned.Object.Validators = append([]schema.ObjectValidator(nil), node.Object.Validators...)
	case schema.ArrayKind:
		if node.Array.Item == nil {
			return schema.Node{}, schema.ErrInvalidSchema
		}
		if node.Array.MinItems != nil && *node.Array.MinItems < 0 {
			return schema.Node{}, schema.ErrInvalidSchema
		}
		if node.Array.MaxItems != nil && *node.Array.MaxItems < 0 {
			return schema.Node{}, schema.ErrInvalidSchema
		}
		if node.Array.MinItems != nil && node.Array.MaxItems != nil && *node.Array.MinItems > *node.Array.MaxItems {
			return schema.Node{}, schema.ErrInvalidSchema
		}
		identity := reflect.ValueOf(node.Array.Item).Pointer()
		if identity == 0 || active[identity] {
			return schema.Node{}, schema.ErrInvalidSchema
		}
		active[identity] = true
		item, err := cloneSchemaNode(*node.Array.Item, active)
		delete(active, identity)
		if err != nil {
			return schema.Node{}, schema.ErrInvalidSchema
		}
		cloned.Array.Item = &item
		cloned.Array.MinItems = cloneIntPointer(node.Array.MinItems)
		cloned.Array.MaxItems = cloneIntPointer(node.Array.MaxItems)
	}
	return cloned, nil
}

func schemaScalarHasData(value schema.ScalarSchema) bool {
	return value.HasDefault || value.Default != nil || value.Enum != nil || value.Min != nil || value.Max != nil || value.Validators != nil
}

func schemaObjectHasData(value schema.ObjectSchema) bool {
	return value.Fields != nil || value.Validators != nil
}

func schemaArrayHasData(value schema.ArraySchema) bool {
	return value.MinItems != nil || value.MaxItems != nil || (value.Item != nil && schemaNodeHasData(*value.Item))
}

func schemaNodeHasData(value schema.Node) bool {
	return value.Kind != schema.StringKind || schemaScalarHasData(value.Scalar) || schemaObjectHasData(value.Object) || value.Array.MinItems != nil || value.Array.MaxItems != nil ||
		value.Capabilities != (schema.NodeCapabilities{}) || value.Label != "" || value.Description != "" || value.UIHint != ""
}

func validSchemaDecision(value schema.Decision) bool {
	return value <= schema.DecisionDeny
}

func isScalarSchemaKind(kind schema.Kind) bool {
	return kind <= schema.NullKind
}

func validCanonicalValue(kind schema.Kind, value any) bool {
	switch kind {
	case schema.StringKind:
		_, ok := value.(string)
		return ok
	case schema.BoolKind:
		_, ok := value.(bool)
		return ok
	case schema.IntegerKind:
		_, ok := value.(int64)
		return ok
	case schema.FloatKind:
		_, ok := value.(float64)
		return ok
	case schema.NullKind:
		return value == nil
	default:
		return false
	}
}

func validRangeValue(kind schema.Kind, value any) bool {
	if kind == schema.IntegerKind {
		_, ok := value.(int64)
		return ok
	}
	floatValue, ok := value.(float64)
	return ok && !math.IsNaN(floatValue)
}

func rangeGreater(kind schema.Kind, left, right any) bool {
	if kind == schema.IntegerKind {
		return left.(int64) > right.(int64)
	}
	return left.(float64) > right.(float64)
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

// ValidateSchema checks the Document against the compiled schema and reports
// duplicate mapping keys throughout the Document, even without a schema.
func ValidateSchema(doc *document.Document, compiled CompiledSchema) []ValidationIssue {
	if doc == nil || doc.Root() == 0 {
		return nil
	}
	issues := make([]ValidationIssue, 0)
	validateDuplicateKeys(doc, &issues)
	if compiled.root != nil {
		validateSchemaNode(doc, *compiled.root, doc.Root(), &issues)
	}
	sortValidationIssues(issues)
	return append([]ValidationIssue(nil), issues...)
}

// ValidateValueChange revalidates one scalar and preserves unaffected issues.
// Custom validators may inspect any Document node, so schemas that contain
// them conservatively fall back to a complete validation pass.
func ValidateValueChange(doc *document.Document, compiled CompiledSchema, previous []ValidationIssue, id document.NodeID) []ValidationIssue {
	if doc == nil || doc.Root() == 0 {
		return nil
	}
	if compiled.hasCustomValidators {
		return ValidateSchema(doc, compiled)
	}
	issues := make([]ValidationIssue, 0, len(previous)+2)
	for _, issue := range previous {
		if issue.NodeID == id && issue.Code != "schema.duplicate_key" {
			continue
		}
		issues = append(issues, issue)
	}
	if shape := schemaNodeAt(doc, id, compiled); shape != nil {
		validateSchemaNode(doc, *shape, id, &issues)
	}
	sortValidationIssues(issues)
	return append([]ValidationIssue(nil), issues...)
}

func sortValidationIssues(issues []ValidationIssue) {
	sort.SliceStable(issues, func(i, j int) bool {
		leftPath, rightPath := issues[i].Path.String(), issues[j].Path.String()
		if leftPath != rightPath {
			return leftPath < rightPath
		}
		if issues[i].Code != issues[j].Code {
			return issues[i].Code < issues[j].Code
		}
		return issues[i].Message < issues[j].Message
	})
}

func validateSchemaNode(doc *document.Document, expected schema.Node, id document.NodeID, issues *[]ValidationIssue) {
	node, ok := doc.Node(id)
	if !ok {
		return
	}
	path, err := doc.Path(id)
	if err != nil {
		return
	}
	if !matchesSchemaKind(node, expected.Kind) {
		addIssue(issues, id, path, "schema.type_mismatch", fmt.Sprintf("expected %s, got %s", schemaKindName(expected.Kind), documentKindName(node)), schema.SeverityError)
		return
	}

	switch expected.Kind {
	case schema.StringKind, schema.BoolKind, schema.IntegerKind, schema.FloatKind, schema.NullKind:
		validateScalar(doc, expected.Scalar, id, path, issues)
	case schema.ObjectKind:
		validateObject(doc, expected.Object, id, path, issues)
	case schema.ArrayKind:
		items, _ := doc.SequenceItems(id)
		if expected.Array.MinItems != nil && len(items) < *expected.Array.MinItems {
			addIssue(issues, id, path, "schema.min_items", fmt.Sprintf("must contain at least %d items", *expected.Array.MinItems), schema.SeverityError)
		}
		if expected.Array.MaxItems != nil && len(items) > *expected.Array.MaxItems {
			addIssue(issues, id, path, "schema.max_items", fmt.Sprintf("must contain at most %d items", *expected.Array.MaxItems), schema.SeverityError)
		}
		for _, itemID := range items {
			validateSchemaNode(doc, *expected.Array.Item, itemID, issues)
		}
	}
}

func validateScalar(doc *document.Document, rules schema.ScalarSchema, id document.NodeID, path document.Path, issues *[]ValidationIssue) {
	kind, value, ok := doc.Scalar(id)
	if !ok {
		return
	}
	if rules.Enum != nil && !enumContains(kind, value, rules.Enum) {
		addIssue(issues, id, path, "schema.enum", fmt.Sprintf("value %s is not in the allowed values", scalarDisplay(value)), schema.SeverityError)
	}
	if kind == document.ScalarInteger {
		integer := value.(int64)
		if minimum, ok := rules.Min.(int64); ok && integer < minimum {
			addIssue(issues, id, path, "schema.min", fmt.Sprintf("must be at least %d", minimum), schema.SeverityError)
		}
		if maximum, ok := rules.Max.(int64); ok && integer > maximum {
			addIssue(issues, id, path, "schema.max", fmt.Sprintf("must be at most %d", maximum), schema.SeverityError)
		}
	}
	if kind == document.ScalarFloat {
		floating := value.(float64)
		if minimum, ok := rules.Min.(float64); ok && floating < minimum {
			addIssue(issues, id, path, "schema.min", fmt.Sprintf("must be at least %g", minimum), schema.SeverityError)
		}
		if maximum, ok := rules.Max.(float64); ok && floating > maximum {
			addIssue(issues, id, path, "schema.max", fmt.Sprintf("must be at most %g", maximum), schema.SeverityError)
		}
	}
	context := schema.ValueContext{Document: doc, NodeID: id, Path: path, Value: value}
	for _, validator := range rules.Validators {
		if validator == nil {
			continue
		}
		for _, issue := range validator(context) {
			addCustomIssue(issues, id, path, issue)
		}
	}
}

func validateObject(doc *document.Document, rules schema.ObjectSchema, id document.NodeID, path document.Path, issues *[]ValidationIssue) {
	entries, _ := doc.MappingEntries(id)
	valuesByField := make(map[string][]document.NodeID, len(rules.Fields))
	declaredFields := make(map[string]struct{}, len(rules.Fields))
	for _, field := range rules.Fields {
		declaredFields[field.Name] = struct{}{}
	}
	for _, entry := range entries {
		kind, value, isScalar := doc.Scalar(entry.Key)
		if !isScalar || kind != document.ScalarString {
			continue
		}
		fieldName := value.(string)
		if _, declared := declaredFields[fieldName]; declared {
			valuesByField[fieldName] = append(valuesByField[fieldName], entry.Value)
		}
	}

	fieldValues := make([]schema.FieldValue, 0, len(rules.Fields))
	for _, field := range rules.Fields {
		values := valuesByField[field.Name]
		if len(values) == 0 {
			fieldValues = append(fieldValues, schema.FieldValue{Name: field.Name})
			if field.Required {
				addIssue(issues, id, path, "schema.required", fmt.Sprintf("required field %q is missing", field.Name), schema.SeverityError)
			}
			continue
		}
		for _, valueID := range values {
			valueNode, exists := doc.Node(valueID)
			if !exists {
				continue
			}
			fieldValue := schema.FieldValue{Name: field.Name, NodeID: valueID, Present: true, Kind: valueNode.Kind()}
			if _, value, scalar := doc.Scalar(valueID); scalar {
				fieldValue.Value = value
			}
			fieldValues = append(fieldValues, fieldValue)
			validateSchemaNode(doc, field.Schema, valueID, issues)
		}
	}

	context := schema.ObjectContext{Document: doc, NodeID: id, Path: path, Fields: append([]schema.FieldValue(nil), fieldValues...)}
	for _, validator := range rules.Validators {
		if validator == nil {
			continue
		}
		for _, issue := range validator(context) {
			bound := false
			if issue.Field != "" {
				for _, fieldID := range valuesByField[issue.Field] {
					fieldPath, err := doc.Path(fieldID)
					if err == nil {
						addCustomIssue(issues, fieldID, fieldPath, issue)
						bound = true
					}
				}
			}
			if !bound {
				addCustomIssue(issues, id, path, issue)
			}
		}
	}
}

func matchesSchemaKind(node *document.Node, expected schema.Kind) bool {
	if node == nil {
		return false
	}
	switch expected {
	case schema.ObjectKind:
		return node.Kind() == document.NodeMapping
	case schema.ArrayKind:
		return node.Kind() == document.NodeSequence
	case schema.StringKind, schema.BoolKind, schema.IntegerKind, schema.FloatKind, schema.NullKind:
		kind, ok := node.ScalarKind()
		if !ok {
			return false
		}
		switch expected {
		case schema.StringKind:
			return kind == document.ScalarString
		case schema.BoolKind:
			return kind == document.ScalarBool
		case schema.IntegerKind:
			return kind == document.ScalarInteger
		case schema.FloatKind:
			return kind == document.ScalarFloat
		case schema.NullKind:
			return kind == document.ScalarNull
		}
	}
	return false
}

func documentKindName(node *document.Node) string {
	if node == nil {
		return "unknown"
	}
	switch node.Kind() {
	case document.NodeMapping:
		return "object"
	case document.NodeSequence:
		return "array"
	case document.NodeReference:
		return "reference"
	case document.NodeScalar:
		kind, _, ok := node.Scalar()
		if !ok {
			return "scalar"
		}
		switch kind {
		case document.ScalarString:
			return "string"
		case document.ScalarBool:
			return "bool"
		case document.ScalarInteger:
			return "integer"
		case document.ScalarFloat:
			return "float"
		case document.ScalarNull:
			return "null"
		}
	}
	return "unknown"
}

func schemaKindName(kind schema.Kind) string {
	switch kind {
	case schema.StringKind:
		return "string"
	case schema.BoolKind:
		return "bool"
	case schema.IntegerKind:
		return "integer"
	case schema.FloatKind:
		return "float"
	case schema.NullKind:
		return "null"
	case schema.ObjectKind:
		return "object"
	case schema.ArrayKind:
		return "array"
	default:
		return "unknown"
	}
}

func enumContains(actualKind document.ScalarKind, actual any, allowed []any) bool {
	for _, candidate := range allowed {
		switch actualKind {
		case document.ScalarString:
			if value, ok := candidate.(string); ok && value == actual.(string) {
				return true
			}
		case document.ScalarBool:
			if value, ok := candidate.(bool); ok && value == actual.(bool) {
				return true
			}
		case document.ScalarInteger:
			if value, ok := candidate.(int64); ok && value == actual.(int64) {
				return true
			}
		case document.ScalarFloat:
			if value, ok := candidate.(float64); ok && math.Float64bits(value) == math.Float64bits(actual.(float64)) {
				return true
			}
		case document.ScalarNull:
			if candidate == nil {
				return true
			}
		}
	}
	return false
}

func scalarDisplay(value any) string {
	if value == nil {
		return "null"
	}
	return fmt.Sprintf("%v", value)
}

func addIssue(issues *[]ValidationIssue, id document.NodeID, path document.Path, code, message string, severity schema.Severity) {
	*issues = append(*issues, ValidationIssue{NodeID: id, Path: path, Code: code, Message: message, Severity: severity})
}

func addCustomIssue(issues *[]ValidationIssue, id document.NodeID, path document.Path, issue schema.Issue) {
	severity := issue.Severity
	if severity > schema.SeverityError {
		severity = schema.SeverityError
	}
	addIssue(issues, id, path, issue.Code, issue.Message, severity)
}

func validateDuplicateKeys(doc *document.Document, issues *[]ValidationIssue) {
	visited := make(map[document.NodeID]struct{})
	keyHashes := make(map[document.NodeID]uint64)
	var walk func(document.NodeID)
	walk = func(id document.NodeID) {
		if _, seen := visited[id]; seen {
			return
		}
		visited[id] = struct{}{}
		node, ok := doc.Node(id)
		if !ok {
			return
		}
		if node.Kind() == document.NodeMapping {
			entries, _ := doc.MappingEntries(id)
			byHash := make(map[uint64][]document.NodeID, len(entries))
			for _, entry := range entries {
				fingerprint := mappingKeyFingerprint(doc, entry.Key, keyHashes)
				for _, previous := range byHash[fingerprint] {
					if equalMappingKey(doc, previous, entry.Key, make(map[nodePair]bool)) {
						if path, err := doc.Path(entry.Value); err == nil {
							addIssue(issues, entry.Value, path, "schema.duplicate_key", "duplicate mapping key", schema.SeverityWarning)
						}
						break
					}
				}
				byHash[fingerprint] = append(byHash[fingerprint], entry.Key)
				walk(entry.Key)
				walk(entry.Value)
			}
			return
		}
		if node.Kind() == document.NodeSequence {
			items, _ := doc.SequenceItems(id)
			for _, item := range items {
				walk(item)
			}
		}
	}
	walk(doc.Root())
}

func mappingKeyFingerprint(doc *document.Document, id document.NodeID, memo map[document.NodeID]uint64) uint64 {
	if fingerprint, ok := memo[id]; ok {
		return fingerprint
	}
	node, ok := doc.Node(id)
	if !ok {
		return 0
	}
	fingerprint := mixFingerprint(0x9e3779b97f4a7c15, uint64(node.Kind()))
	switch node.Kind() {
	case document.NodeScalar:
		kind, value, scalar := node.Scalar()
		if !scalar {
			break
		}
		fingerprint = mixFingerprint(fingerprint, uint64(kind))
		switch kind {
		case document.ScalarString:
			text := value.(string)
			for index := 0; index < len(text); index++ {
				fingerprint = mixFingerprint(fingerprint, uint64(text[index]))
			}
			fingerprint = mixFingerprint(fingerprint, uint64(len(text)))
		case document.ScalarBool:
			if value.(bool) {
				fingerprint = mixFingerprint(fingerprint, 1)
			}
		case document.ScalarInteger:
			fingerprint = mixFingerprint(fingerprint, uint64(value.(int64)))
		case document.ScalarFloat:
			fingerprint = mixFingerprint(fingerprint, math.Float64bits(value.(float64)))
		}
	case document.NodeSequence:
		items, _ := doc.SequenceItems(id)
		fingerprint = mixFingerprint(fingerprint, uint64(len(items)))
		for _, item := range items {
			fingerprint = mixFingerprint(fingerprint, mappingKeyFingerprint(doc, item, memo))
		}
	case document.NodeMapping:
		entries, _ := doc.MappingEntries(id)
		fingerprint = mixFingerprint(fingerprint, uint64(len(entries)))
		for _, entry := range entries {
			fingerprint = mixFingerprint(fingerprint, mappingKeyFingerprint(doc, entry.Key, memo))
			fingerprint = mixFingerprint(fingerprint, mappingKeyFingerprint(doc, entry.Value, memo))
		}
	case document.NodeReference:
		target, _ := doc.ReferenceTarget(id)
		fingerprint = mixFingerprint(fingerprint, uint64(target))
	}
	memo[id] = fingerprint
	return fingerprint
}

func mixFingerprint(current, value uint64) uint64 {
	current ^= value + 0x9e3779b97f4a7c15 + (current << 6) + (current >> 2)
	return current
}

type nodePair struct {
	left  document.NodeID
	right document.NodeID
}

func equalMappingKey(doc *document.Document, leftID, rightID document.NodeID, active map[nodePair]bool) bool {
	if leftID == rightID {
		return true
	}
	pair := nodePair{left: leftID, right: rightID}
	if active[pair] {
		return true
	}
	active[pair] = true
	defer delete(active, pair)
	left, leftOK := doc.Node(leftID)
	right, rightOK := doc.Node(rightID)
	if !leftOK || !rightOK || left.Kind() != right.Kind() {
		return false
	}
	switch left.Kind() {
	case document.NodeScalar:
		leftKind, leftValue, leftOK := left.Scalar()
		rightKind, rightValue, rightOK := right.Scalar()
		return leftOK && rightOK && leftKind == rightKind && equalScalarValues(leftKind, leftValue, rightValue)
	case document.NodeSequence:
		leftItems, _ := doc.SequenceItems(leftID)
		rightItems, _ := doc.SequenceItems(rightID)
		if len(leftItems) != len(rightItems) {
			return false
		}
		for index := range leftItems {
			if !equalMappingKey(doc, leftItems[index], rightItems[index], active) {
				return false
			}
		}
		return true
	case document.NodeMapping:
		leftEntries, _ := doc.MappingEntries(leftID)
		rightEntries, _ := doc.MappingEntries(rightID)
		if len(leftEntries) != len(rightEntries) {
			return false
		}
		for index := range leftEntries {
			if !equalMappingKey(doc, leftEntries[index].Key, rightEntries[index].Key, active) || !equalMappingKey(doc, leftEntries[index].Value, rightEntries[index].Value, active) {
				return false
			}
		}
		return true
	case document.NodeReference:
		leftTarget, leftOK := doc.ReferenceTarget(leftID)
		rightTarget, rightOK := doc.ReferenceTarget(rightID)
		return leftOK && rightOK && leftTarget == rightTarget
	default:
		return false
	}
}

func equalScalarValues(kind document.ScalarKind, left, right any) bool {
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
