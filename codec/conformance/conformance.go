// Package conformance provides reusable tests for codec.Codec implementations.
package conformance

import (
	"bytes"
	"errors"
	"io"
	"math"
	"reflect"
	"testing"

	"github.com/yuyosy/structivedit/codec"
	"github.com/yuyosy/structivedit/document"
)

// Sample pairs a format-specific input with its expected semantic Document.
// It must include at least one string mapping value or sequence item so the
// suite can verify encoding a later snapshot from the same Document lineage.
// Use features representable by the codec under test; format-specific
// capabilities can be tested separately by that codec's own tests.
type Sample struct {
	Input    []byte
	Expected *document.Document
}

// Factory creates a Codec for each conformance check.
type Factory func() codec.Codec

// Run executes the common Codec contract checks using a representable sample.
// Call it from a _test.go file, passing a factory and an expected Document.
func Run(t *testing.T, factory Factory, sample Sample) {
	t.Helper()
	if factory == nil {
		t.Fatal("codec factory is nil")
	}
	if sample.Expected == nil || sample.Expected.Root() == 0 {
		t.Fatal("sample expected Document is empty")
	}

	t.Run("decode", func(t *testing.T) {
		session := decodeSample(t, factory, sample.Input)
		if session.Document() == nil || session.Document().Root() == 0 {
			t.Fatal("Decode returned an empty Document")
		}
		if !equalDocuments(session.Document(), sample.Expected) {
			t.Fatal("decoded Document differs from sample expectation")
		}
	})

	t.Run("encode_round_trip", func(t *testing.T) {
		session := decodeSample(t, factory, sample.Input)
		var output bytes.Buffer
		if err := session.Encode(&output, session.Document()); err != nil {
			t.Fatalf("Encode decoded Document: %v", err)
		}
		roundTrip := decodeSample(t, factory, output.Bytes())
		if !equalDocuments(roundTrip.Document(), sample.Expected) {
			t.Fatal("decode/encode round trip changed the sample Document")
		}
	})

	t.Run("encode_edited_snapshot", func(t *testing.T) {
		session := decodeSample(t, factory, sample.Input)
		id, ok := firstStringValue(session.Document(), session.Document().Root())
		if !ok {
			t.Fatal("sample must contain a string mapping value or sequence item")
		}
		path, err := session.Document().Path(id)
		if err != nil {
			t.Fatalf("get edited value path: %v", err)
		}
		_, oldValue, ok := session.Document().Scalar(id)
		if !ok {
			t.Fatal("selected sample node is not a scalar")
		}
		newValue := oldValue.(string) + " edited"

		edited, err := document.NewBuilderFrom(session.Document())
		if err != nil {
			t.Fatalf("copy decoded Document: %v", err)
		}
		if err := edited.SetScalar(id, newValue); err != nil {
			t.Fatalf("edit decoded Document: %v", err)
		}
		editedDocument, err := edited.Build()
		if err != nil {
			t.Fatalf("build edited Document: %v", err)
		}

		expectedID, err := sample.Expected.LookupPath(path)
		if err != nil {
			t.Fatalf("resolve expected edited value: %v", err)
		}
		expectedEdit, err := document.NewBuilderFrom(sample.Expected)
		if err != nil {
			t.Fatalf("copy expected Document: %v", err)
		}
		if err := expectedEdit.SetScalar(expectedID, newValue); err != nil {
			t.Fatalf("edit expected Document: %v", err)
		}
		expectedDocument, err := expectedEdit.Build()
		if err != nil {
			t.Fatalf("build expected edited Document: %v", err)
		}

		var output bytes.Buffer
		if err := session.Encode(&output, editedDocument); err != nil {
			t.Fatalf("Encode edited snapshot: %v", err)
		}
		roundTrip := decodeSample(t, factory, output.Bytes())
		if !equalDocuments(roundTrip.Document(), expectedDocument) {
			t.Fatal("encoding an edited snapshot changed its semantic Document")
		}
	})

	t.Run("reject_foreign_document", func(t *testing.T) {
		session := decodeSample(t, factory, sample.Input)
		foreign := newScalarDocument(t, "foreign")
		if err := session.Encode(io.Discard, foreign); !errors.Is(err, codec.ErrForeignDocument) {
			t.Fatalf("Encode foreign Document error = %v, want %v", err, codec.ErrForeignDocument)
		}
	})

	t.Run("propagate_reader_error", func(t *testing.T) {
		readerErr := errors.New("conformance: reader failure")
		current := factory()
		if nilInterface(current) {
			t.Fatal("codec factory returned nil")
		}
		_, err := current.Decode(failingReader{err: readerErr})
		if !errors.Is(err, readerErr) {
			t.Fatalf("Decode reader error = %v, want %v", err, readerErr)
		}
	})

	t.Run("propagate_writer_error", func(t *testing.T) {
		session := decodeSample(t, factory, sample.Input)
		writerErr := errors.New("conformance: writer failure")
		if err := session.Encode(failingWriter{err: writerErr}, session.Document()); !errors.Is(err, writerErr) {
			t.Fatalf("Encode writer error = %v, want %v", err, writerErr)
		}
	})
}

func decodeSample(t *testing.T, factory Factory, input []byte) codec.Session {
	t.Helper()
	current := factory()
	if nilInterface(current) {
		t.Fatal("codec factory returned nil")
	}
	session, err := current.Decode(bytes.NewReader(input))
	if err != nil {
		t.Fatalf("Decode sample: %v", err)
	}
	if nilInterface(session) {
		t.Fatal("Decode returned nil Session")
	}
	return session
}

func nilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func newScalarDocument(t *testing.T, value string) *document.Document {
	t.Helper()
	builder := document.NewBuilder()
	root, err := builder.NewScalar(value)
	if err != nil {
		t.Fatalf("create foreign Document scalar: %v", err)
	}
	if err := builder.SetRoot(root); err != nil {
		t.Fatalf("set foreign Document root: %v", err)
	}
	result, err := builder.Build()
	if err != nil {
		t.Fatalf("build foreign Document: %v", err)
	}
	return result
}

func equalDocuments(left, right *document.Document) bool {
	if left == nil || right == nil || left.Root() == 0 || right.Root() == 0 {
		return false
	}
	return equalNodes(left, left.Root(), right, right.Root())
}

func equalNodes(leftDoc *document.Document, leftID document.NodeID, rightDoc *document.Document, rightID document.NodeID) bool {
	left, leftOK := leftDoc.Node(leftID)
	right, rightOK := rightDoc.Node(rightID)
	if !leftOK || !rightOK || left.Kind() != right.Kind() {
		return false
	}
	switch left.Kind() {
	case document.NodeScalar:
		leftKind, leftValue, leftOK := left.Scalar()
		rightKind, rightValue, rightOK := right.Scalar()
		return leftOK && rightOK && leftKind == rightKind && equalScalar(leftKind, leftValue, rightValue)
	case document.NodeSequence:
		leftItems := left.SequenceItems()
		rightItems := right.SequenceItems()
		if len(leftItems) != len(rightItems) {
			return false
		}
		for index := range leftItems {
			if !equalNodes(leftDoc, leftItems[index], rightDoc, rightItems[index]) {
				return false
			}
		}
		return true
	case document.NodeMapping:
		leftEntries := left.MappingEntries()
		rightEntries := right.MappingEntries()
		if len(leftEntries) != len(rightEntries) {
			return false
		}
		for index := range leftEntries {
			if !equalNodes(leftDoc, leftEntries[index].Key, rightDoc, rightEntries[index].Key) ||
				!equalNodes(leftDoc, leftEntries[index].Value, rightDoc, rightEntries[index].Value) {
				return false
			}
		}
		return true
	case document.NodeReference:
		leftTarget, leftOK := left.ReferenceTarget()
		rightTarget, rightOK := right.ReferenceTarget()
		if !leftOK || !rightOK {
			return false
		}
		leftPath, leftErr := leftDoc.Path(leftTarget)
		rightPath, rightErr := rightDoc.Path(rightTarget)
		return leftErr == nil && rightErr == nil && leftPath.String() == rightPath.String()
	default:
		return false
	}
}

func equalScalar(kind document.ScalarKind, left, right any) bool {
	switch kind {
	case document.ScalarString:
		return left.(string) == right.(string)
	case document.ScalarBool:
		return left.(bool) == right.(bool)
	case document.ScalarInteger:
		return left.(int64) == right.(int64)
	case document.ScalarFloat:
		return math.Float64bits(left.(float64)) == math.Float64bits(right.(float64))
	case document.ScalarNull:
		return left == nil && right == nil
	default:
		return false
	}
}

func firstStringValue(doc *document.Document, id document.NodeID) (document.NodeID, bool) {
	node, ok := doc.Node(id)
	if !ok {
		return 0, false
	}
	switch node.Kind() {
	case document.NodeScalar:
		kind, _, scalar := node.Scalar()
		return id, scalar && kind == document.ScalarString
	case document.NodeSequence:
		for _, item := range node.SequenceItems() {
			if found, ok := firstStringValue(doc, item); ok {
				return found, true
			}
		}
	case document.NodeMapping:
		for _, entry := range node.MappingEntries() {
			if found, ok := firstStringValue(doc, entry.Value); ok {
				return found, true
			}
		}
	}
	return 0, false
}

type failingReader struct{ err error }

func (reader failingReader) Read([]byte) (int, error) { return 0, reader.err }

type failingWriter struct{ err error }

func (writer failingWriter) Write([]byte) (int, error) { return 0, writer.err }
