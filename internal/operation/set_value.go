package operation

import "github.com/yuyosy/structivedit/document"

// SetValue applies one scalar update to a Document draft.
type SetValue struct {
	NodeID document.NodeID
	Value  any
}

func (operation SetValue) Apply(builder *document.Builder) error {
	if builder == nil {
		return document.ErrInvalidDocument
	}
	return builder.SetScalar(operation.NodeID, operation.Value)
}
