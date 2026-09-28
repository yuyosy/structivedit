package structivedit

import "github.com/yuyosy/structivedit/document"

// NodeView is a read-only semantic view of one Document node.
type NodeView struct {
	ID                 document.NodeID
	Parent             document.ParentRef
	HasParent          bool
	Path               document.Path
	Kind               document.NodeKind
	ScalarValue        any
	SequenceItems      []document.NodeID
	MappingEntries     []document.MappingEntry
	ReferenceTarget    document.NodeID
	HasReferenceTarget bool
	Restrictions       document.NodeRestrictions
	Capabilities       Capabilities
	Issues             []ValidationIssue
}

func cloneValidationIssues(issues []ValidationIssue) []ValidationIssue {
	return append([]ValidationIssue(nil), issues...)
}
