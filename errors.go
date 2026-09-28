package structivedit

import (
	"errors"

	"github.com/yuyosy/structivedit/schema"
)

var (
	// ErrSelectorSyntax classifies errors returned by ParseSelector.
	ErrSelectorSyntax = errors.New("structivedit: selector syntax error")
	// ErrInvalidPolicy reports invalid Decision values in a Policy.
	ErrInvalidPolicy = errors.New("structivedit: invalid policy")
	// ErrInvalidSchema classifies malformed schema trees.
	ErrInvalidSchema = schema.ErrInvalidSchema
)
