package document

import "errors"

var (
	ErrInvalidDocument    = errors.New("document: invalid document")
	ErrNodeNotFound       = errors.New("document: node not found")
	ErrNodeAlreadyOwned   = errors.New("document: node already has an owner")
	ErrNodeAlreadyDefined = errors.New("document: node ID is already defined")
	ErrInvalidNodeKind    = errors.New("document: invalid node kind")
	ErrInvalidScalar      = errors.New("document: invalid scalar")
	ErrInvalidPath        = errors.New("document: invalid path")
	ErrPathNotFound       = errors.New("document: path not found")
)
