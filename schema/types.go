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
