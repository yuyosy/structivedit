package schema

// Kind identifies the data shape described by a schema node.
type Kind uint8

const (
	StringKind Kind = iota
	BoolKind
	IntegerKind
	FloatKind
	NullKind
	ObjectKind
	ArrayKind
)

// Decision controls whether a schema-level operation is permitted.
type Decision uint8

const (
	DecisionInherit Decision = iota
	DecisionAllow
	DecisionDeny
)

// Severity is the importance assigned to a validation issue.
type Severity uint8

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
)

// UnknownFieldPolicy controls reports for object keys not declared in
// ObjectSchema.Fields and whether AdditionalProperties can be used to add
// new keys.
type UnknownFieldPolicy uint8

const (
	// UnknownFieldAllow accepts additional keys without reporting an issue.
	UnknownFieldAllow UnknownFieldPolicy = iota
	// UnknownFieldWarn accepts additional keys and reports a warning.
	UnknownFieldWarn
	// UnknownFieldDeny reports existing additional keys as errors and disallows
	// adding new ones.
	UnknownFieldDeny
)
