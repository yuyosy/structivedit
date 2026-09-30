package resolver

import (
	"github.com/yuyosy/structivedit/document"
	"github.com/yuyosy/structivedit/schema"
)

// ResolveCapabilities combines Policy, Schema structure, and Document hard
// constraints for one node.
func ResolveCapabilities(doc *document.Document, id document.NodeID, compiledSchema CompiledSchema, policy CompiledPolicy) (ResolvedPolicy, bool) {
	if doc == nil {
		return ResolvedPolicy{}, false
	}
	node, ok := doc.Node(id)
	if !ok {
		return ResolvedPolicy{}, false
	}
	permissions, ok := Resolve(policy, doc, id)
	if !ok {
		return ResolvedPolicy{}, false
	}
	result := ResolvedPolicy{}
	parent, hasParent := doc.Parent(id)
	isMappingKey := hasParent && parent.Role == document.ParentMappingKey
	isReadOnly := node.Restrictions()&document.RestrictionReadOnly != 0
	isReference := node.Kind() == document.NodeReference
	if node.Kind() == document.NodeScalar && !isMappingKey && !isReadOnly && !isReference && permissions.Editable {
		result.Editable = true
	}
	if !isMappingKey && !isReadOnly && !isReference {
		result.Addable = canAddTo(doc, id, compiledSchema, permissions.Addable)
		result.Deletable = canDelete(doc, id, compiledSchema, permissions.Deletable)
		result.Reorderable = canReorder(doc, id, compiledSchema, permissions.Reorderable)
	}
	return result, true
}

func canAddTo(doc *document.Document, id document.NodeID, compiled CompiledSchema, permitted bool) bool {
	if !permitted {
		return false
	}
	node, ok := doc.Node(id)
	if !ok || node.Restrictions()&document.RestrictionReadOnly != 0 {
		return false
	}
	shape := schemaNodeAt(doc, id, compiled)
	if shape == nil {
		return false
	}
	eligible := false
	switch node.Kind() {
	case document.NodeSequence:
		if shape.Kind != schema.ArrayKind {
			return false
		}
		items, _ := doc.SequenceItems(id)
		eligible = shape.Array.MaxItems == nil || len(items) < *shape.Array.MaxItems
	case document.NodeMapping:
		if shape.Kind != schema.ObjectKind {
			return false
		}
		entries, _ := doc.MappingEntries(id)
		present := make(map[string]struct{}, len(entries))
		for _, entry := range entries {
			kind, value, scalar := doc.Scalar(entry.Key)
			if scalar && kind == document.ScalarString {
				present[value.(string)] = struct{}{}
			}
		}
		for _, field := range shape.Object.Fields {
			if _, exists := present[field.Name]; !exists {
				eligible = true
				break
			}
		}
		if !eligible && shape.Object.AdditionalProperties != nil && shape.Object.UnknownFields != schema.UnknownFieldDeny {
			eligible = true
		}
		if !eligible && shape.Object.UnknownFields == schema.UnknownFieldDeny {
			return false
		}
	default:
		return false
	}
	return applySchemaDecision(shape.Capabilities.Add, eligible)
}

func canDelete(doc *document.Document, id document.NodeID, compiled CompiledSchema, permitted bool) bool {
	if !permitted {
		return false
	}
	node, ok := doc.Node(id)
	if !ok || node.Kind() == document.NodeReference || node.Restrictions()&document.RestrictionReadOnly != 0 {
		return false
	}
	parent, hasParent := doc.Parent(id)
	if !hasParent {
		return false
	}
	shape := schemaNodeAt(doc, id, compiled)
	eligible := false
	deletionRoots := []document.NodeID{id}
	switch parent.Role {
	case document.ParentSequenceItem:
		containerShape := schemaNodeAt(doc, parent.Parent, compiled)
		if shape == nil || containerShape == nil || containerShape.Kind != schema.ArrayKind {
			return false
		}
		items, ok := doc.SequenceItems(parent.Parent)
		if !ok {
			return false
		}
		eligible = containerShape.Array.MinItems == nil || len(items)-1 >= *containerShape.Array.MinItems
	case document.ParentMappingValue:
		containerShape := schemaNodeAt(doc, parent.Parent, compiled)
		if containerShape == nil || containerShape.Kind != schema.ObjectKind {
			return false
		}
		field, exists := schemaFieldForValue(doc, parent.Parent, parent.Index, containerShape)
		if !exists {
			return false
		}
		entries, ok := doc.MappingEntries(parent.Parent)
		if !ok || parent.Index < 0 || parent.Index >= len(entries) || entries[parent.Index].Value != id {
			return false
		}
		deletionRoots = append(deletionRoots, entries[parent.Index].Key)
		eligible = !field.Required
	default:
		return false
	}
	if !eligible || (shape != nil && !applySchemaDecision(shape.Capabilities.Delete, true)) {
		return false
	}
	return deletionSafe(doc, deletionRoots)
}

func canReorder(doc *document.Document, id document.NodeID, compiled CompiledSchema, permitted bool) bool {
	if !permitted {
		return false
	}
	node, ok := doc.Node(id)
	if !ok || node.Kind() == document.NodeReference || node.Restrictions()&document.RestrictionReadOnly != 0 {
		return false
	}
	parent, hasParent := doc.Parent(id)
	if !hasParent || parent.Role != document.ParentSequenceItem {
		return false
	}
	items, ok := doc.SequenceItems(parent.Parent)
	if !ok || len(items) < 2 {
		return false
	}
	shape := schemaNodeAt(doc, id, compiled)
	if shape == nil {
		// Reordering an existing sequence item does not require a Schema.
		return true
	}
	return applySchemaDecision(shape.Capabilities.Reorder, true)
}

func schemaNodeAt(doc *document.Document, id document.NodeID, compiled CompiledSchema) *schema.Node {
	if doc == nil || compiled.root == nil || id == 0 {
		return nil
	}
	path, err := doc.Path(id)
	if err != nil {
		return nil
	}
	currentSchema := compiled.root
	currentID := doc.Root()
	for _, segment := range path.Segments() {
		switch typed := segment.(type) {
		case document.SequenceIndexSegment:
			if currentSchema.Kind != schema.ArrayKind || currentSchema.Array.Item == nil {
				return nil
			}
			items, ok := doc.SequenceItems(currentID)
			if !ok || typed.Index() < 0 || typed.Index() >= len(items) {
				return nil
			}
			currentSchema = currentSchema.Array.Item
			currentID = items[typed.Index()]
		case document.MappingEntrySegment:
			if typed.Role() != document.MappingValueRole || currentSchema.Kind != schema.ObjectKind {
				return nil
			}
			entries, ok := doc.MappingEntries(currentID)
			if !ok || typed.EntryIndex() < 0 || typed.EntryIndex() >= len(entries) {
				return nil
			}
			entry := entries[typed.EntryIndex()]
			kind, keyValue, scalar := doc.Scalar(entry.Key)
			if scalar && kind == document.ScalarString {
				if field, exists := schemaField(currentSchema.Object.Fields, keyValue.(string)); exists {
					currentSchema = &field.Schema
				} else if currentSchema.Object.AdditionalProperties != nil {
					currentSchema = currentSchema.Object.AdditionalProperties
				} else {
					return nil
				}
			} else if currentSchema.Object.AdditionalProperties != nil {
				currentSchema = currentSchema.Object.AdditionalProperties
			} else {
				return nil
			}
			currentID = entry.Value
		default:
			return nil
		}
	}
	if currentID != id {
		return nil
	}
	return currentSchema
}

// HasSchema reports whether compiled contains a root Schema node.
func HasSchema(compiled CompiledSchema) bool { return compiled.root != nil }

// SequenceItemSchema returns the item schema for a documented sequence.
func SequenceItemSchema(doc *document.Document, id document.NodeID, compiled CompiledSchema) (schema.Node, bool) {
	node, ok := doc.Node(id)
	shape := schemaNodeAt(doc, id, compiled)
	if !ok || shape == nil || node.Kind() != document.NodeSequence || shape.Kind != schema.ArrayKind {
		return schema.Node{}, false
	}
	return *shape.Array.Item, true
}

// ObjectFieldsAt returns the declared field schemas for a documented mapping.
func ObjectFieldsAt(doc *document.Document, id document.NodeID, compiled CompiledSchema) ([]schema.Field, bool) {
	node, ok := doc.Node(id)
	shape := schemaNodeAt(doc, id, compiled)
	if !ok || shape == nil || node.Kind() != document.NodeMapping || shape.Kind != schema.ObjectKind {
		return nil, false
	}
	return append([]schema.Field(nil), shape.Object.Fields...), true
}

// ObjectFieldSchema returns one declared field from a documented mapping.
func ObjectFieldSchema(doc *document.Document, id document.NodeID, name string, compiled CompiledSchema) (schema.Field, bool) {
	fields, ok := ObjectFieldsAt(doc, id, compiled)
	if !ok {
		return schema.Field{}, false
	}
	for _, field := range fields {
		if field.Name == name {
			return field, true
		}
	}
	return schema.Field{}, false
}

// AdditionalPropertySchemaAt returns the value schema for a new, undeclared
// string-keyed property when the object's policy permits adding one.
func AdditionalPropertySchemaAt(doc *document.Document, id document.NodeID, compiled CompiledSchema) (schema.Node, bool) {
	node, ok := doc.Node(id)
	shape := schemaNodeAt(doc, id, compiled)
	if !ok || shape == nil || node.Kind() != document.NodeMapping || shape.Kind != schema.ObjectKind ||
		shape.Object.AdditionalProperties == nil || shape.Object.UnknownFields == schema.UnknownFieldDeny {
		return schema.Node{}, false
	}
	return *shape.Object.AdditionalProperties, true
}

func schemaField(fields []schema.Field, name string) (*schema.Field, bool) {
	for index := range fields {
		if fields[index].Name == name {
			return &fields[index], true
		}
	}
	return nil, false
}

func schemaFieldForValue(doc *document.Document, parent document.NodeID, entryIndex int, shape *schema.Node) (*schema.Field, bool) {
	entries, ok := doc.MappingEntries(parent)
	if !ok || entryIndex < 0 || entryIndex >= len(entries) {
		return nil, false
	}
	kind, value, scalar := doc.Scalar(entries[entryIndex].Key)
	if scalar && kind == document.ScalarString {
		if field, exists := schemaField(shape.Object.Fields, value.(string)); exists {
			return field, true
		}
	}
	// Additional properties are optional by definition and can be deleted even
	// when no value schema is configured or the validation policy reports them.
	return &schema.Field{}, true
}

func applySchemaDecision(decision schema.Decision, inherited bool) bool {
	switch decision {
	case schema.DecisionAllow:
		return true
	case schema.DecisionDeny:
		return false
	default:
		return inherited
	}
}

func deletionSafe(doc *document.Document, roots []document.NodeID) bool {
	subtree := make(map[document.NodeID]struct{})
	var collect func(document.NodeID) bool
	collect = func(id document.NodeID) bool {
		if _, seen := subtree[id]; seen {
			return true
		}
		subtree[id] = struct{}{}
		node, ok := doc.Node(id)
		if !ok {
			return false
		}
		if node.Restrictions()&document.RestrictionReadOnly != 0 {
			return false
		}
		for _, child := range doc.Children(id) {
			if !collect(child) {
				return false
			}
		}
		return true
	}
	for _, root := range roots {
		if !collect(root) {
			return false
		}
	}

	visited := make(map[document.NodeID]struct{})
	var inspect func(document.NodeID) bool
	inspect = func(id document.NodeID) bool {
		if _, seen := visited[id]; seen {
			return true
		}
		visited[id] = struct{}{}
		_, ok := doc.Node(id)
		if !ok {
			return true
		}
		if target, reference := doc.ReferenceTarget(id); reference {
			if _, removed := subtree[target]; removed {
				if _, sourceRemoved := subtree[id]; !sourceRemoved {
					return false
				}
			}
		}
		for _, child := range doc.Children(id) {
			if !inspect(child) {
				return false
			}
		}
		return true
	}
	return inspect(doc.Root())
}
