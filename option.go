package structivedit

import "github.com/yuyosy/structivedit/schema"

// Option configures an Editor during construction.
type Option func(*editorOptions) error

type editorOptions struct {
	schema       *schema.Node
	policy       Policy
	historyLimit int
}

// WithSchema configures the schema used for validation and structural actions.
func WithSchema(value schema.Node) Option {
	return func(options *editorOptions) error {
		cloned := value
		options.schema = &cloned
		return nil
	}
}

// WithPolicy configures the Editor's permission policy.
func WithPolicy(value Policy) Option {
	return func(options *editorOptions) error {
		value.Rules = append([]Rule(nil), value.Rules...)
		options.policy = value
		return nil
	}
}

// WithHistoryLimit sets the maximum retained number of Document operations.
// Zero disables Undo / Redo history. The default is 100.
func WithHistoryLimit(limit int) Option {
	return func(options *editorOptions) error {
		if limit < 0 {
			return ErrInvalidInput
		}
		options.historyLimit = limit
		return nil
	}
}
