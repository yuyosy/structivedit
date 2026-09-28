package operation

import "github.com/yuyosy/structivedit/document"

// AddNode attaches newly created ownership nodes to a sequence or mapping.
type AddNode struct {
	ParentID      document.NodeID
	ParentKind    document.NodeKind
	SequenceItems []document.NodeID
	MappingEntries []document.MappingEntry
}

func (operation AddNode) Apply(builder *document.Builder) error {
	if builder == nil {
		return document.ErrInvalidDocument
	}
	switch operation.ParentKind {
	case document.NodeSequence:
		return builder.SetSequenceItems(operation.ParentID, operation.SequenceItems)
	case document.NodeMapping:
		return builder.SetMappingEntries(operation.ParentID, operation.MappingEntries)
	default:
		return document.ErrInvalidNodeKind
	}
}
