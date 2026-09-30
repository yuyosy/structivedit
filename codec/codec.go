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
//
// A successful Decode returns a non-nil Session with a non-nil Document.
// The Session retains any format-specific information needed to encode later
// snapshots from that Document's lineage. Implementations should return
// codec.ErrForeignDocument when asked to encode an unrelated snapshot, and
// should propagate reader and writer errors to their callers.
type Codec interface {
	Decode(io.Reader) (Session, error)
}

// Session binds an immutable Document snapshot to format-specific metadata.
// Document returns the snapshot produced by Decode. Encode accepts snapshots
// from the same Document lineage, including later edited snapshots, and must
// not silently discard content it cannot represent.
type Session interface {
	Document() *document.Document
	Encode(io.Writer, *document.Document) error
}
