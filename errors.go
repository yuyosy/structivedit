package structivedit

import "errors"

var (
	// ErrSelectorSyntax classifies errors returned by ParseSelector.
	ErrSelectorSyntax = errors.New("structivedit: selector syntax error")
	// ErrInvalidPolicy reports invalid Decision values in a Policy.
	ErrInvalidPolicy = errors.New("structivedit: invalid policy")
)
