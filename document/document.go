package document

import (
	"sort"
	"sync/atomic"
)

// Document is an immutable snapshot of a single-root ownership tree and its
// reference graph. Its fields are private so callers cannot mutate a snapshot.
type Document struct {
	root     NodeID
	nodes    map[NodeID]*docNode
	parents  map[NodeID]ParentRef
	refs     map[NodeID]map[NodeID]struct{}
	nextID   NodeID
	revision Revision
	lineage  lineageID
}

var lineageSequence atomic.Uint64

func newLineageID() lineageID {
	id := lineageSequence.Add(1)
	if id == 0 {
		// A process cannot safely issue a duplicate lineage identity after the
		// uint64 space has been exhausted.
		panic("document: lineage ID space exhausted")
	}
	return lineageID(id)
}

// Root returns the root node ID, or zero for a nil Document.
func (d *Document) Root() NodeID {
	if d == nil {
		return 0
	}
	return d.root
}

// Node returns a read-only handle to id when it exists in the snapshot.
func (d *Document) Node(id NodeID) (*Node, bool) {
	if d == nil || id == 0 || d.nodes[id] == nil {
		return nil, false
	}
	return &Node{doc: d, id: id}, true
}

// Parent returns the ownership parent of id. The root has no parent.
func (d *Document) Parent(id NodeID) (ParentRef, bool) {
	if d == nil {
		return ParentRef{}, false
	}
	parent, ok := d.parents[id]
	return parent, ok
}

// Revision returns the snapshot's revision, or zero for a nil Document.
func (d *Document) Revision() Revision {
	if d == nil {
		return 0
	}
	return d.revision
}

// SameLineage reports whether other is a snapshot from the same Builder
// lineage. Unrelated builders have distinct lineage identities.
func (d *Document) SameLineage(other *Document) bool {
	return d != nil && other != nil && d.lineage != 0 && d.lineage == other.lineage
}

// Children returns the ownership children of id in document order. Mapping
// children are returned as Key, Value pairs for each ordered entry. Reference
// targets are not ownership children.
func (d *Document) Children(id NodeID) []NodeID {
	if d == nil {
		return nil
	}
	entry := d.nodes[id]
	if entry == nil {
		return nil
	}
	switch entry.kind {
	case NodeSequence:
		return append([]NodeID(nil), entry.sequence.items...)
	case NodeMapping:
		children := make([]NodeID, 0, len(entry.mapping.entries)*2)
		for _, mappingEntry := range entry.mapping.entries {
			children = append(children, mappingEntry.key, mappingEntry.value)
		}
		return children
	default:
		return nil
	}
}

// ReferenceTarget returns the target of id when id is a reference node.
func (d *Document) ReferenceTarget(id NodeID) (NodeID, bool) {
	if d == nil {
		return 0, false
	}
	entry := d.nodes[id]
	if entry == nil || entry.kind != NodeReference || entry.reference == nil {
		return 0, false
	}
	return entry.reference.target, true
}

// Scalar returns the scalar kind and normalized value when id is a scalar.
func (d *Document) Scalar(id NodeID) (ScalarKind, any, bool) {
	node, ok := d.Node(id)
	if !ok {
		return 0, nil, false
	}
	return node.Scalar()
}

// SequenceItems returns a copy of id's items when id is a sequence.
func (d *Document) SequenceItems(id NodeID) ([]NodeID, bool) {
	node, ok := d.Node(id)
	if !ok || node.Kind() != NodeSequence {
		return nil, false
	}
	return node.SequenceItems(), true
}

// MappingEntries returns a copy of id's ordered entries when id is a mapping.
func (d *Document) MappingEntries(id NodeID) ([]MappingEntry, bool) {
	node, ok := d.Node(id)
	if !ok || node.Kind() != NodeMapping {
		return nil, false
	}
	return node.MappingEntries(), true
}

// SequenceLen returns the number of items without copying the sequence.
func (d *Document) SequenceLen(id NodeID) (int, bool) {
	if d == nil {
		return 0, false
	}
	node := d.nodes[id]
	if node == nil || node.kind != NodeSequence {
		return 0, false
	}
	return len(node.sequence.items), true
}

// SequenceItem returns one item without copying the sequence.
func (d *Document) SequenceItem(id NodeID, index int) (NodeID, bool) {
	count, ok := d.SequenceLen(id)
	if !ok || index < 0 || index >= count {
		return 0, false
	}
	return d.nodes[id].sequence.items[index], true
}

// MappingEntryAt returns one entry without copying the mapping.
func (d *Document) MappingEntryAt(id NodeID, index int) (MappingEntry, bool) {
	if d == nil || index < 0 {
		return MappingEntry{}, false
	}
	node := d.nodes[id]
	if node == nil || node.kind != NodeMapping || index >= len(node.mapping.entries) {
		return MappingEntry{}, false
	}
	entry := node.mapping.entries[index]
	return MappingEntry{Key: entry.key, Value: entry.value}, true
}

// ReferenceSources returns reference nodes pointing at target, ordered by ID.
func (d *Document) ReferenceSources(target NodeID) []NodeID {
	if d == nil {
		return nil
	}
	sources := make([]NodeID, 0, len(d.refs[target]))
	for source := range d.refs[target] {
		sources = append(sources, source)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i] < sources[j] })
	return sources
}
