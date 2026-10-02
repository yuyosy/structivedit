package document

// OrderedReferencesValid reports whether references carrying
// RestrictionReferenceOrder follow their targets in ownership preorder.
// Unrestricted references may point forward or form cycles.
func (d *Document) OrderedReferencesValid() bool {
	if d == nil || d.Root() == 0 {
		return false
	}
	seen := make(map[NodeID]bool, len(d.nodes))
	stack := []NodeID{d.root}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		node := d.nodes[id]
		if node == nil {
			return false
		}
		seen[id] = true
		if node.kind == NodeReference && node.restrictions&RestrictionReferenceOrder != 0 && !seen[node.reference.target] {
			return false
		}
		children := d.Children(id)
		for i := len(children) - 1; i >= 0; i-- {
			stack = append(stack, children[i])
		}
	}
	return true
}
