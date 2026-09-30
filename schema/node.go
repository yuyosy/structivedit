package schema

// Node describes one scalar, object, or array value.
//
// Exactly one of Scalar, Object, or Array may contain data, and it must match
// Kind. Schema validation is performed when an Editor accepts the schema.
type Node struct {
	Kind         Kind
	Scalar       ScalarSchema
	Object       ObjectSchema
	Array        ArraySchema
	Capabilities NodeCapabilities
	Label        string
	Description  string
	UIHint       string
}

// ScalarSchema describes constraints and defaults for a scalar value.
type ScalarSchema struct {
	HasDefault bool
	Default    any
	Enum       []any
	Min        any
	Max        any
	Validators []Validator
}

// ObjectSchema describes an object's declared and additional fields.
type ObjectSchema struct {
	Fields []Field
	// AdditionalProperties validates values for undeclared keys and supplies
	// the schema used to add a new key when UnknownFields permits it.
	AdditionalProperties *Node
	UnknownFields        UnknownFieldPolicy
	Validators           []ObjectValidator
}

// Field describes one named object field.
type Field struct {
	Name       string
	Schema     Node
	Required   bool
	HasDefault bool
	Default    any
}

// ArraySchema describes the item shape and permitted array length.
type ArraySchema struct {
	// Item is a pointer to keep the recursive Go type finite in size.
	Item     *Node
	MinItems *int
	MaxItems *int
}

// NodeCapabilities contains schema-level operation decisions.
type NodeCapabilities struct {
	Add     Decision
	Delete  Decision
	Reorder Decision
}
