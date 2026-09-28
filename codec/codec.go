// Package codec defines format-independent document sessions.
package codec

import (
	"errors"
	"io"

	"github.com/yuyosy/structivedit/document"
)

// ErrForeignDocument is returned when a Session encodes a snapshot from a
// different Document lineage.
var ErrForeignDocument = errors.New("codec: foreign document")

// Codec decodes one input stream into a Document-bound Session.
type Codec interface {
	Decode(io.Reader) (Session, error)
}

// Session binds an immutable Document snapshot to format-specific metadata.
type Session interface {
	Document() *document.Document
	Encode(io.Writer, *document.Document) error
}
