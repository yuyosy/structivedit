package schema

import "github.com/yuyosy/structivedit/document"

// Validator checks one present scalar value. Validators run synchronously
// against an immutable Document snapshot.
type Validator func(ValueContext) []Issue

// ObjectValidator checks relationships among an object's fields. Validators
// run synchronously against an immutable Document snapshot.
type ObjectValidator func(ObjectContext) []Issue

// ValueContext describes the scalar value being checked.
type ValueContext struct {
	Document *document.Document
	NodeID   document.NodeID
	Path     document.Path
	Value    any
}

// FieldValue describes one declared field. Duplicate entries produce one
// FieldValue per value in document order; an absent field has Present=false.
type FieldValue struct {
	Name    string
	NodeID  document.NodeID
	Present bool
	Kind    document.NodeKind
	Value   any
}

// ObjectContext describes an object and its declared fields in schema order.
type ObjectContext struct {
	Document *document.Document
	NodeID   document.NodeID
	Path     document.Path
	Fields   []FieldValue
}

// Issue is returned by a custom schema validator.
type Issue struct {
	Code     string
	Message  string
	Severity Severity
	Field    string
}
