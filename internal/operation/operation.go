package operation

import "github.com/yuyosy/structivedit/document"

// Operation is a validated Document mutation applied to a private Builder.
type Operation interface {
	Apply(*document.Builder) error
}
