package schema

import "errors"

// ErrInvalidSchema classifies a malformed schema tree.
var ErrInvalidSchema = errors.New("structivedit: invalid schema")
