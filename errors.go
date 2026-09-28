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
	// ErrInvalidInput reports malformed public API input.
	ErrInvalidInput = errors.New("structivedit: invalid input")
	// ErrNotEditable reports that a node cannot be edited under its policy or constraints.
	ErrNotEditable = errors.New("structivedit: node is not editable")
	// ErrReferenceReadOnly reports an attempt to edit through a Reference node.
	ErrReferenceReadOnly = errors.New("structivedit: reference is read only")
	// ErrTypeMismatch reports an input whose scalar kind differs from the existing node.
	ErrTypeMismatch = errors.New("structivedit: scalar type mismatch")
	// ErrNoUndo reports that no retained Document operation can be undone.
	ErrNoUndo = errors.New("structivedit: no operation to undo")
	// ErrNoRedo reports that no retained Document operation can be redone.
	ErrNoRedo = errors.New("structivedit: no operation to redo")
)
