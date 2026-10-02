package bubbletea

// ModelOption configures a Bubble Tea Model.
type ModelOption func(*modelOptions)

type modelOptions struct {
	expandAliases    bool
	aliasRowLimit    int
	colors           bool
	inlineEditing    bool
	mouseDoubleClick bool
}

// WithAliasRowLimit bounds projected alias rows per view. The default is
// 10,000. A nonpositive limit hides projected contents. Ownership rows remain
// accessible regardless of this limit.
func WithAliasRowLimit(limit int) ModelOption {
	return func(options *modelOptions) { options.aliasRowLimit = max(0, limit) }
}

// WithAliasExpansion displays the read-only contents of aliases that point to
// mappings or sequences. The default is false.
func WithAliasExpansion(enabled bool) ModelOption {
	return func(options *modelOptions) {
		options.expandAliases = enabled
	}
}

// WithColors enables or disables terminal colors. Colors are also disabled
// when the NO_COLOR environment variable is set.
func WithColors(enabled bool) ModelOption {
	return func(options *modelOptions) {
		options.colors = enabled
	}
}

// WithInlineEditing shows the active scalar editor in its tree row instead of
// the dedicated input area. The default is false.
func WithInlineEditing(enabled bool) ModelOption {
	return func(options *modelOptions) {
		options.inlineEditing = enabled
	}
}

// WithMouseDoubleClick enables editing scalar values by double-clicking their
// rows. Bool values are toggled instead. The default is true.
func WithMouseDoubleClick(enabled bool) ModelOption {
	return func(options *modelOptions) {
		options.mouseDoubleClick = enabled
	}
}
