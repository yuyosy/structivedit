package bubbletea

// ModelOption configures a Bubble Tea Model.
type ModelOption func(*modelOptions)

type modelOptions struct {
	expandAliases bool
}

// WithAliasExpansion displays the read-only contents of aliases that point to
// mappings or sequences. The default is false.
func WithAliasExpansion(enabled bool) ModelOption {
	return func(options *modelOptions) {
		options.expandAliases = enabled
	}
}
