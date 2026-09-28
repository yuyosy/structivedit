package operation

import "github.com/yuyosy/structivedit/document"

// MoveNode replaces one sequence's item order.
type MoveNode struct {
	ParentID document.NodeID
	Items    []document.NodeID
}

func (operation MoveNode) Apply(builder *document.Builder) error {
	if builder == nil {
		return document.ErrInvalidDocument
	}
	return builder.SetSequenceItems(operation.ParentID, operation.Items)
}
