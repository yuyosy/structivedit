package structivedit

import (
	"github.com/yuyosy/structivedit/document"
	"github.com/yuyosy/structivedit/internal/operation"
	"github.com/yuyosy/structivedit/internal/resolver"
)

// CanAdd reports whether parent currently accepts at least one schema-defined
// child.
func (e *Editor) CanAdd(parent document.NodeID) bool {
	if e == nil || e.doc == nil {
		return false
	}
	capabilities, valid := resolver.ResolveCapabilities(e.doc, parent, e.schema, e.policy)
	return valid && capabilities.Addable
}

// CanDelete reports whether id can be removed under the current schema,
// policy, restrictions, and reference graph.
func (e *Editor) CanDelete(id document.NodeID) bool {
	if e == nil || e.doc == nil {
		return false
	}
	capabilities, valid := resolver.ResolveCapabilities(e.doc, id, e.schema, e.policy)
	return valid && capabilities.Deletable
}

// CanReorder reports whether id can move within its current sequence.
func (e *Editor) CanReorder(id document.NodeID) bool {
	if e == nil || e.doc == nil {
		return false
	}
	capabilities, valid := resolver.ResolveCapabilities(e.doc, id, e.schema, e.policy)
	return valid && capabilities.Reorderable
}

// Delete removes one optional mapping field or sequence item.
func (e *Editor) Delete(id document.NodeID) (ApplyResult, error) {
	return e.Apply(Delete{NodeID: id})
}

// Move places id at finalIndex in its parent sequence.
func (e *Editor) Move(id document.NodeID, finalIndex int) (ApplyResult, error) {
	return e.Apply(Move{NodeID: id, ToIndex: finalIndex})
}

func (e *Editor) applyDelete(id document.NodeID) (ApplyResult, error) {
	if e == nil || e.doc == nil {
		return ApplyResult{}, ErrInvalidInput
	}
	if !resolver.HasSchema(e.schema) {
		return ApplyResult{}, ErrSchemaRequired
	}
	if _, ok := e.doc.Node(id); !ok {
		return ApplyResult{}, document.ErrNodeNotFound
	}
	capabilities, valid := resolver.ResolveCapabilities(e.doc, id, e.schema, e.policy)
	if !valid || !capabilities.Deletable {
		return ApplyResult{}, ErrNotDeletable
	}
	parent, hasParent := e.doc.Parent(id)
	if !hasParent {
		return ApplyResult{}, ErrNotDeletable
	}

	deleteOperation := operation.DeleteNode{ParentID: parent.Parent}
	roots := []document.NodeID{id}
	switch parent.Role {
	case document.ParentSequenceItem:
		items, ok := e.doc.SequenceItems(parent.Parent)
		if !ok || parent.Index < 0 || parent.Index >= len(items) || items[parent.Index] != id {
			return ApplyResult{}, document.ErrInvalidDocument
		}
		deleteOperation.ParentKind = document.NodeSequence
		deleteOperation.SequenceItems = append([]document.NodeID(nil), items[:parent.Index]...)
		deleteOperation.SequenceItems = append(deleteOperation.SequenceItems, items[parent.Index+1:]...)
	case document.ParentMappingValue:
		entries, ok := e.doc.MappingEntries(parent.Parent)
		if !ok || parent.Index < 0 || parent.Index >= len(entries) || entries[parent.Index].Value != id {
			return ApplyResult{}, document.ErrInvalidDocument
		}
		deleteOperation.ParentKind = document.NodeMapping
		roots = append(roots, entries[parent.Index].Key)
		deleteOperation.MappingEntries = append([]document.MappingEntry(nil), entries[:parent.Index]...)
		deleteOperation.MappingEntries = append(deleteOperation.MappingEntries, entries[parent.Index+1:]...)
	default:
		return ApplyResult{}, ErrNotDeletable
	}

	removed := make(map[document.NodeID]struct{})
	for _, root := range roots {
		if err := collectOwnershipSubtree(e.doc, root, removed); err != nil {
			return ApplyResult{}, err
		}
	}
	builder, err := document.NewBuilderFrom(e.doc)
	if err != nil {
		return ApplyResult{}, err
	}
	if err := deleteOperation.Apply(builder); err != nil {
		return ApplyResult{}, err
	}
	updated, err := builder.Build()
	if err != nil {
		return ApplyResult{}, err
	}
	updatedIssues := publicIssues(resolver.ValidateSchema(updated, e.schema))
	previousIssues := e.issues
	effect := operation.Effect{
		Kind:      operation.EffectNodeDeleted,
		NodeID:    id,
		ParentID:  parent.Parent,
		FromIndex: parent.Index,
	}
	if err := e.history.Record(e.doc, updated, flattenNodeSet(removed), []operation.Effect{effect}); err != nil {
		return ApplyResult{}, err
	}
	previousFocus := e.focused
	focusChanged := false
	if _, deleted := removed[e.focused]; e.focused != 0 && deleted {
		e.focused = parent.Parent
		focusChanged = previousFocus != e.focused
	}
	e.doc = updated
	e.issues = cloneValidationIssues(updatedIssues)
	result := ApplyResult{DocumentChanged: true, Revision: updated.Revision()}
	result.Events = append(result.Events, eventsForEffects([]operation.Effect{effect}, updated.Revision())...)
	if focusChanged {
		result.Events = append(result.Events, Event{
			Kind:          EventFocusChanged,
			PreviousFocus: previousFocus,
			Focus:         e.focused,
			Revision:      updated.Revision(),
		})
	}
	if !equalIssues(previousIssues, updatedIssues) {
		result.Events = append(result.Events, Event{Kind: EventValidationChanged, Issues: cloneValidationIssues(updatedIssues), Revision: updated.Revision()})
	}
	result.Events = append(result.Events, Event{Kind: EventHistoryChanged, Revision: updated.Revision()})
	result.Events = cloneEvents(result.Events)
	return result, nil
}

func (e *Editor) applyMove(id document.NodeID, finalIndex int) (ApplyResult, error) {
	if e == nil || e.doc == nil {
		return ApplyResult{}, ErrInvalidInput
	}
	node, ok := e.doc.Node(id)
	if !ok {
		return ApplyResult{}, document.ErrNodeNotFound
	}
	parent, hasParent := e.doc.Parent(id)
	if !hasParent || parent.Role != document.ParentSequenceItem || node.Kind() == document.NodeReference {
		return ApplyResult{}, ErrNotReorderable
	}
	items, ok := e.doc.SequenceItems(parent.Parent)
	if !ok || parent.Index < 0 || parent.Index >= len(items) || items[parent.Index] != id {
		return ApplyResult{}, ErrNotReorderable
	}
	if finalIndex < 0 || finalIndex >= len(items) {
		return ApplyResult{}, ErrInvalidInput
	}
	if finalIndex == parent.Index {
		return ApplyResult{Revision: e.doc.Revision()}, nil
	}
	capabilities, valid := resolver.ResolveCapabilities(e.doc, id, e.schema, e.policy)
	if !valid || !capabilities.Reorderable {
		return ApplyResult{}, ErrNotReorderable
	}

	reordered := make([]document.NodeID, 0, len(items))
	reordered = append(reordered, items[:parent.Index]...)
	reordered = append(reordered, items[parent.Index+1:]...)
	reordered = append(reordered, 0)
	copy(reordered[finalIndex+1:], reordered[finalIndex:])
	reordered[finalIndex] = id
	builder, err := document.NewBuilderFrom(e.doc)
	if err != nil {
		return ApplyResult{}, err
	}
	moveOperation := operation.MoveNode{ParentID: parent.Parent, Items: reordered}
	if err := moveOperation.Apply(builder); err != nil {
		return ApplyResult{}, err
	}
	updated, err := builder.Build()
	if err != nil {
		return ApplyResult{}, err
	}
	if !updated.OrderedReferencesValid() {
		return ApplyResult{}, ErrNotReorderable
	}
	updatedIssues := publicIssues(resolver.ValidateSchema(updated, e.schema))
	previousIssues := e.issues
	effect := operation.Effect{Kind: operation.EffectNodeMoved, NodeID: id, ParentID: parent.Parent, FromIndex: parent.Index, ToIndex: finalIndex}
	if err := e.history.Record(e.doc, updated, []document.NodeID{id}, []operation.Effect{effect}); err != nil {
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

func collectOwnershipSubtree(doc *document.Document, root document.NodeID, collected map[document.NodeID]struct{}) error {
	if doc == nil || collected == nil {
		return document.ErrInvalidDocument
	}
	if _, seen := collected[root]; seen {
		return nil
	}
	if _, ok := doc.Node(root); !ok {
		return document.ErrNodeNotFound
	}
	collected[root] = struct{}{}
	for _, child := range doc.Children(root) {
		if err := collectOwnershipSubtree(doc, child, collected); err != nil {
			return err
		}
	}
	return nil
}

func flattenNodeSet(nodes map[document.NodeID]struct{}) []document.NodeID {
	result := make([]document.NodeID, 0, len(nodes))
	for id := range nodes {
		result = append(result, id)
	}
	return result
}
