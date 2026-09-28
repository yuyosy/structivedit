package structivedit

import (
	"github.com/yuyosy/structivedit/document"
	"github.com/yuyosy/structivedit/schema"
)

// ValidationIssue reports a schema or custom-validator finding at a node.
// Missing required fields are attached to their containing Object node.
type ValidationIssue struct {
	NodeID   document.NodeID
	Path     document.Path
	Code     string
	Message  string
	Severity schema.Severity
}
