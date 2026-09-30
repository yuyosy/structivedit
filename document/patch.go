package document

import "sort"

// SnapshotPatch stores only the nodes that differ between two snapshots from
// one Builder lineage. It is intended for compact undo and redo history.
type SnapshotPatch struct {
	lineage     lineageID
	beforeRoot  NodeID
	afterRoot   NodeID
	beforeNext  NodeID
	afterNext   NodeID
	changed     bool
	nodeChanges []snapshotNodeChange
}

type snapshotNodeChange struct {
	id     NodeID
	before *docNode
	after  *docNode
}

// NewSnapshotPatch captures the content difference between before and after.
// The patch copies changed nodes, so it remains immutable when either
// Builder continues to change.
func NewSnapshotPatch(before, after *Document) (*SnapshotPatch, error) {
	if before == nil || after == nil || !before.SameLineage(after) {
		return nil, ErrInvalidDocument
	}
	changedIDs := make([]NodeID, 0, len(before.nodes)+len(after.nodes))
	for id := range before.nodes {
		changedIDs = append(changedIDs, id)
	}
	for id := range after.nodes {
		if before.nodes[id] == nil {
			changedIDs = append(changedIDs, id)
		}
	}
	return NewSnapshotPatchForNodes(before, after, changedIDs)
}

// NewSnapshotPatchForNodes captures differences among the listed node IDs.
// Callers that already know an operation's changed nodes can avoid scanning
// the complete snapshots. The caller must include every changed node.
func NewSnapshotPatchForNodes(before, after *Document, changedIDs []NodeID) (*SnapshotPatch, error) {
	if before == nil || after == nil || !before.SameLineage(after) {
		return nil, ErrInvalidDocument
	}
	patch := &SnapshotPatch{
		lineage:    before.lineage,
		beforeRoot: before.root,
		afterRoot:  after.root,
		beforeNext: before.nextID,
		afterNext:  after.nextID,
		changed:    before.root != after.root,
	}
	seen := make(map[NodeID]struct{}, len(changedIDs))
	for _, id := range changedIDs {
		if id == 0 {
			return nil, ErrInvalidDocument
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		beforeNode := before.nodes[id]
		afterNode := after.nodes[id]
		if equalDocNode(beforeNode, afterNode) {
			continue
		}
		patch.changed = true
		patch.nodeChanges = append(patch.nodeChanges, snapshotNodeChange{
			id:     id,
			before: cloneDocNode(beforeNode),
			after:  cloneDocNode(afterNode),
		})
	}
	sort.Slice(patch.nodeChanges, func(i, j int) bool {
		return patch.nodeChanges[i].id < patch.nodeChanges[j].id
	})
	return patch, nil
}

// Apply restores one side of the patch into builder. reverse selects before;
// otherwise after is restored. Node content is checked before mutation so a
// patch cannot silently overwrite a different edit to the same nodes.
func (patch *SnapshotPatch) Apply(builder *Builder, reverse bool) error {
	if patch == nil || builder == nil || !builder.ready() || builder.draft.lineage != patch.lineage {
		return ErrInvalidDocument
	}
	sourceRoot, targetRoot := patch.beforeRoot, patch.afterRoot
	targetNext := patch.afterNext
	if reverse {
		sourceRoot, targetRoot = patch.afterRoot, patch.beforeRoot
		targetNext = patch.beforeNext
	}
	if builder.draft.root != sourceRoot {
		return ErrInvalidDocument
	}
	for _, change := range patch.nodeChanges {
		expected := change.before
		if reverse {
			expected = change.after
		}
		if !equalDocNode(builder.draft.nodes[change.id], expected) {
			return ErrInvalidDocument
		}
	}

	nodes := make(map[NodeID]*docNode, len(builder.draft.nodes)+len(patch.nodeChanges))
	for id, node := range builder.draft.nodes {
		nodes[id] = node
	}
	for _, change := range patch.nodeChanges {
		replacement := change.after
		if reverse {
			replacement = change.before
		}
		if replacement == nil {
			delete(nodes, change.id)
		} else {
			nodes[change.id] = cloneDocNode(replacement)
		}
	}
	nextID := maxNextID(builder.draft.nextID, targetNext)
	reserved := cloneReserved(builder.draft.reserved)
	parents, refs, err := rebuildIndexes(targetRoot, nodes, reserved, nextID)
	if err != nil {
		return err
	}
	if err := validateRootedTree(targetRoot, nodes, parents); err != nil {
		return err
	}
	return builder.commitPrepared(targetRoot, nodes, reserved, nextID, parents, refs, patch.changed)
}

func equalDocNode(left, right *docNode) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.id != right.id || left.kind != right.kind || left.restrictions != right.restrictions {
		return false
	}
	switch left.kind {
	case NodeScalar:
		return equalScalar(left.scalar, right.scalar)
	case NodeSequence:
		return left.sequence != nil && right.sequence != nil && equalNodeIDs(left.sequence.items, right.sequence.items)
	case NodeMapping:
		if left.mapping == nil || right.mapping == nil || len(left.mapping.entries) != len(right.mapping.entries) {
			return false
		}
		for index, entry := range left.mapping.entries {
			if entry != right.mapping.entries[index] {
				return false
			}
		}
		return true
	case NodeReference:
		return left.reference != nil && right.reference != nil && left.reference.target == right.reference.target
	default:
		return false
	}
}
