package operation

import "github.com/yuyosy/structivedit/document"

// DeleteNode removes an optional mapping entry value or sequence item by
// replacing the parent's ordered child list.
type DeleteNode struct {
	ParentID       document.NodeID
	ParentKind     document.NodeKind
	SequenceItems  []document.NodeID
	MappingEntries []document.MappingEntry
}

func (operation DeleteNode) Apply(builder *document.Builder) error {
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
