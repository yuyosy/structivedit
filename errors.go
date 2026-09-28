package structivedit

import "errors"

var (
	// ErrSelectorSyntax classifies errors returned by ParseSelector.
	ErrSelectorSyntax = errors.New("structivedit: selector syntax error")
)
