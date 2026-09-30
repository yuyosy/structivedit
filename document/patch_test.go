package document

import "testing"

func TestSnapshotPatchUndoRedoStoresOnlyChangedNodes(t *testing.T) {
	initialBuilder := NewBuilder()
	items := make([]NodeID, 512)
	for index := range items {
		id, err := initialBuilder.NewScalar(index)
		if err != nil {
			t.Fatal(err)
		}
		items[index] = id
	}
	root, err := initialBuilder.NewSequence(items)
	if err != nil {
		t.Fatal(err)
	}
	if err := initialBuilder.SetRoot(root); err != nil {
		t.Fatal(err)
	}
	before, err := initialBuilder.Build()
	if err != nil {
		t.Fatal(err)
	}

	editBuilder, err := NewBuilderFrom(before)
	if err != nil {
		t.Fatal(err)
	}
	if err := editBuilder.SetScalar(items[0], 999); err != nil {
		t.Fatal(err)
	}
	after, err := editBuilder.Build()
	if err != nil {
		t.Fatal(err)
	}
	patch, err := NewSnapshotPatch(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(patch.nodeChanges); got != 1 {
		t.Fatalf("patch stores %d nodes, want 1", got)
	}

	undoBuilder, err := NewBuilderFrom(after)
	if err != nil {
		t.Fatal(err)
	}
	if err := patch.Apply(undoBuilder, true); err != nil {
		t.Fatal(err)
	}
	undone, err := undoBuilder.Build()
	if err != nil {
		t.Fatal(err)
	}
	if _, value, ok := undone.Scalar(items[0]); !ok || value != int64(0) {
		t.Fatalf("undo value = %v, %v; want 0, true", value, ok)
	}

	redoBuilder, err := NewBuilderFrom(undone)
	if err != nil {
		t.Fatal(err)
	}
	if err := patch.Apply(redoBuilder, false); err != nil {
		t.Fatal(err)
	}
	redone, err := redoBuilder.Build()
	if err != nil {
		t.Fatal(err)
	}
	if _, value, ok := redone.Scalar(items[0]); !ok || value != int64(999) {
		t.Fatalf("redo value = %v, %v; want 999, true", value, ok)
	}
}
