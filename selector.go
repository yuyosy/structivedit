package structivedit

import (
	"fmt"

	selectorimpl "github.com/yuyosy/structivedit/internal/selector"
)

// Selector is an opaque, immutable pattern used to match Document locations.
// Construct one with ParseSelector or MustSelector.
type Selector struct {
	pattern selectorimpl.Pattern
}

// SelectorParseError describes a selector syntax error. Offset is a 0-based
// UTF-8 byte offset into the input.
type SelectorParseError struct {
	Offset  int
	Message string
}

func (e *SelectorParseError) Error() string {
	if e == nil {
		return "selector syntax error"
	}
	return fmt.Sprintf("selector syntax error at byte %d: %s", e.Offset, e.Message)
}

// Unwrap classifies parse failures with ErrSelectorSyntax.
func (e *SelectorParseError) Unwrap() error { return ErrSelectorSyntax }

// ParseSelector parses a StructiveEdit selector.
func ParseSelector(input string) (Selector, error) {
	pattern, err := selectorimpl.Parse(input)
	if err != nil {
		if parseErr, ok := err.(*selectorimpl.ParseError); ok {
			return Selector{}, &SelectorParseError{Offset: parseErr.Offset, Message: parseErr.Message}
		}
		return Selector{}, &SelectorParseError{Offset: 0, Message: err.Error()}
	}
	return Selector{pattern: pattern}, nil
}

// MustSelector parses input and panics if it is invalid.
func MustSelector(input string) Selector {
	selector, err := ParseSelector(input)
	if err != nil {
		panic(err)
	}
	return selector
}
