package document

// Node is a read-only handle to a node in an immutable Document snapshot.
// A Node is meaningful only together with the Document that created it.
type Node struct {
	doc *Document
	id  NodeID
}

type docNode struct {
	id           NodeID
	kind         NodeKind
	scalar       *scalarNode
	mapping      *mappingNode
	sequence     *sequenceNode
	reference    *referenceNode
	restrictions NodeRestrictions
}

type mappingNode struct {
	entries []mappingEntry
}

type sequenceNode struct {
	items []NodeID
}

type referenceNode struct {
	target NodeID
}

type mappingEntry struct {
	key   NodeID
	value NodeID
}

// MappingEntry identifies the key and value nodes of one ordered mapping entry.
type MappingEntry struct {
	Key   NodeID
	Value NodeID
}

// ParentRef identifies a node's ownership parent and its position in that
// parent's child list.
type ParentRef struct {
	Parent NodeID
	Role   ParentRole
	Index  int
}

type NodeKind uint8

const (
	NodeScalar NodeKind = iota
	NodeMapping
	NodeSequence
	NodeReference
)

type ParentRole uint8

const (
	ParentSequenceItem ParentRole = iota
	ParentMappingKey
	ParentMappingValue
)

// NodeRestrictions contains hard constraints attached directly to a node.
// Restrictions are inherited through ownership when read from a Node.
type NodeRestrictions uint8

const (
	RestrictionReadOnly NodeRestrictions = 1 << iota
	// RestrictionKeyReadOnly prevents editing inside a mapping key, while
	// permitting deletion of the containing entry.
	RestrictionKeyReadOnly
	// RestrictionReferenceOrder requires a reference's target to precede it
	// in ownership preorder, as required by formats such as YAML.
	RestrictionReferenceOrder
)

// ID returns this node's stable identity within its Document.
func (n *Node) ID() NodeID {
	if n == nil {
		return 0
	}
	return n.id
}

// Kind returns the node's kind. A nil or invalid handle returns NodeScalar,
// the zero value of NodeKind.
func (n *Node) Kind() NodeKind {
	entry := n.entry()
	if entry == nil {
		return NodeScalar
	}
	return entry.kind
}

// Restrictions returns the restrictions directly set on this node and all of
// its ownership ancestors.
func (n *Node) Restrictions() NodeRestrictions {
	if n == nil || n.doc == nil {
		return 0
	}
	var result NodeRestrictions
	seen := make(map[NodeID]struct{})
	for id := n.id; id != 0; {
		if _, exists := seen[id]; exists {
			break
		}
		seen[id] = struct{}{}
		entry := n.doc.nodes[id]
		if entry == nil {
			break
		}
		result |= entry.restrictions
		parent, ok := n.doc.parents[id]
		if !ok {
			break
		}
		id = parent.Parent
	}
	return result
}

// Scalar returns the scalar kind and value when this is a scalar node.
func (n *Node) Scalar() (ScalarKind, any, bool) {
	entry := n.entry()
	if entry == nil || entry.kind != NodeScalar || entry.scalar == nil {
		return 0, nil, false
	}
	return entry.scalar.kind, entry.scalar.value, true
}

// ScalarKind returns the scalar kind when this is a scalar node.
func (n *Node) ScalarKind() (ScalarKind, bool) {
	kind, _, ok := n.Scalar()
	return kind, ok
}

// ScalarValue returns the normalized scalar value when this is a scalar node.
func (n *Node) ScalarValue() (any, bool) {
	_, value, ok := n.Scalar()
	return value, ok
}

// SequenceItems returns a copy of the sequence's child IDs.
func (n *Node) SequenceItems() []NodeID {
	entry := n.entry()
	if entry == nil || entry.kind != NodeSequence || entry.sequence == nil {
		return nil
	}
	return append([]NodeID(nil), entry.sequence.items...)
}

// MappingEntries returns a copy of the mapping's ordered entries.
func (n *Node) MappingEntries() []MappingEntry {
	entry := n.entry()
	if entry == nil || entry.kind != NodeMapping || entry.mapping == nil {
		return nil
	}
	entries := make([]MappingEntry, len(entry.mapping.entries))
	for i, item := range entry.mapping.entries {
		entries[i] = MappingEntry{Key: item.key, Value: item.value}
	}
	return entries
}

// ReferenceTarget returns the target ID when this is a reference node.
func (n *Node) ReferenceTarget() (NodeID, bool) {
	entry := n.entry()
	if entry == nil || entry.kind != NodeReference || entry.reference == nil {
		return 0, false
	}
	return entry.reference.target, true
}

func (n *Node) entry() *docNode {
	if n == nil || n.doc == nil || n.id == 0 {
		return nil
	}
	return n.doc.nodes[n.id]
}
