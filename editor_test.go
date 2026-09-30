package structivedit

import (
	"testing"

	"github.com/yuyosy/structivedit/document"
)

func TestNewRejectsZeroValueDocument(t *testing.T) {
	if _, err := New(&document.Document{}); err != document.ErrInvalidDocument {
		t.Fatalf("New zero-value Document error = %v, want ErrInvalidDocument", err)
	}
}

func TestViewsCacheReturnsCopiesAndInvalidatesAfterEdits(t *testing.T) {
	builder := document.NewBuilder()
	valueID, err := builder.NewScalar("before")
	if err != nil {
		t.Fatal(err)
	}
	rootID, err := builder.NewSequence([]document.NodeID{valueID})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SetRoot(rootID); err != nil {
		t.Fatal(err)
	}
	doc, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	editor, err := New(doc, WithPolicy(Policy{Default: ScopePolicy{Editable: Allow}}))
	if err != nil {
		t.Fatal(err)
	}

	first := editor.Views()
	first[0].SequenceItems[0] = 0
	first[1].ScalarValue = "mutated view"
	second := editor.Views()
	if second[0].SequenceItems[0] != valueID {
		t.Fatalf("cached sequence item = %d, want %d", second[0].SequenceItems[0], valueID)
	}
	if second[1].ScalarValue != "before" {
		t.Fatalf("cached scalar = %v, want before", second[1].ScalarValue)
	}

	if _, err := editor.Apply(SetValue{NodeID: valueID, Value: "after"}); err != nil {
		t.Fatal(err)
	}
	third := editor.Views()
	if third[1].ScalarValue != "after" {
		t.Fatalf("view after edit = %v, want after", third[1].ScalarValue)
	}
	if _, err := editor.Apply(Undo{}); err != nil {
		t.Fatal(err)
	}
	fourth := editor.Views()
	if fourth[1].ScalarValue != "before" {
		t.Fatalf("view after undo = %v, want before", fourth[1].ScalarValue)
	}
}

func BenchmarkEditorViewsCached(b *testing.B) {
	doc := makeBenchmarkDocument(b, 2048)
	editor, err := New(doc)
	if err != nil {
		b.Fatal(err)
	}
	editor.Views()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		editor.Views()
	}
}

func BenchmarkEditorViewsCold(b *testing.B) {
	doc := makeBenchmarkDocument(b, 2048)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		editor, err := New(doc)
		if err != nil {
			b.Fatal(err)
		}
		editor.Views()
	}
}

func makeBenchmarkDocument(b testing.TB, count int) *document.Document {
	b.Helper()
	builder := document.NewBuilder()
	items := make([]document.NodeID, count)
	for index := range items {
		id, err := builder.NewScalar(index)
		if err != nil {
			b.Fatal(err)
		}
		items[index] = id
	}
	root, err := builder.NewSequence(items)
	if err != nil {
		b.Fatal(err)
	}
	if err := builder.SetRoot(root); err != nil {
		b.Fatal(err)
	}
	doc, err := builder.Build()
	if err != nil {
		b.Fatal(err)
	}
	return doc
}
