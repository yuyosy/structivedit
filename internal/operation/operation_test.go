package operation

import (
	"testing"

	"github.com/yuyosy/structivedit/document"
)

func TestSnapshotOperationTracksStructuralParentChanges(t *testing.T) {
	initial := document.NewBuilder()
	first, err := initial.NewScalar("first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := initial.NewScalar("second")
	if err != nil {
		t.Fatal(err)
	}
	root, err := initial.NewSequence([]document.NodeID{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if err := initial.SetRoot(root); err != nil {
		t.Fatal(err)
	}
	before, err := initial.Build()
	if err != nil {
		t.Fatal(err)
	}

	edit, err := document.NewBuilderFrom(before)
	if err != nil {
		t.Fatal(err)
	}
	if err := edit.SetSequenceItems(root, []document.NodeID{second, first}); err != nil {
		t.Fatal(err)
	}
	after, err := edit.Build()
	if err != nil {
		t.Fatal(err)
	}
	op, err := NewSnapshotOperation(before, after, []document.NodeID{first}, []Effect{{
		Kind:     EffectNodeMoved,
		NodeID:   first,
		ParentID: root,
	}})
	if err != nil {
		t.Fatal(err)
	}

	undoBuilder, err := document.NewBuilderFrom(after)
	if err != nil {
		t.Fatal(err)
	}
	if err := op.Undo(undoBuilder); err != nil {
		t.Fatal(err)
	}
	undone, err := undoBuilder.Build()
	if err != nil {
		t.Fatal(err)
	}
	items, _ := undone.SequenceItems(root)
	if len(items) != 2 || items[0] != first || items[1] != second {
		t.Fatalf("undo order = %v, want [%d %d]", items, first, second)
	}

	redoBuilder, err := document.NewBuilderFrom(undone)
	if err != nil {
		t.Fatal(err)
	}
	if err := op.Apply(redoBuilder); err != nil {
		t.Fatal(err)
	}
	redone, err := redoBuilder.Build()
	if err != nil {
		t.Fatal(err)
	}
	items, _ = redone.SequenceItems(root)
	if len(items) != 2 || items[0] != second || items[1] != first {
		t.Fatalf("redo order = %v, want [%d %d]", items, second, first)
	}
}

func TestSnapshotOperationRestoresAddedAndDeletedNodes(t *testing.T) {
	initial := document.NewBuilder()
	first, err := initial.NewScalar("first")
	if err != nil {
		t.Fatal(err)
	}
	root, err := initial.NewSequence([]document.NodeID{first})
	if err != nil {
		t.Fatal(err)
	}
	if err := initial.SetRoot(root); err != nil {
		t.Fatal(err)
	}
	before, err := initial.Build()
	if err != nil {
		t.Fatal(err)
	}

	addBuilder, err := document.NewBuilderFrom(before)
	if err != nil {
		t.Fatal(err)
	}
	second, err := addBuilder.NewScalar("second")
	if err != nil {
		t.Fatal(err)
	}
	if err := addBuilder.SetSequenceItems(root, []document.NodeID{first, second}); err != nil {
		t.Fatal(err)
	}
	added, err := addBuilder.Build()
	if err != nil {
		t.Fatal(err)
	}
	add, err := NewSnapshotOperation(before, added, []document.NodeID{second}, []Effect{{
		Kind:     EffectNodeAdded,
		NodeID:   second,
		ParentID: root,
	}})
	if err != nil {
		t.Fatal(err)
	}
	undoAdd, err := document.NewBuilderFrom(added)
	if err != nil {
		t.Fatal(err)
	}
	if err := add.Undo(undoAdd); err != nil {
		t.Fatal(err)
	}
	undoneAdd, err := undoAdd.Build()
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := undoneAdd.Node(second); exists {
		t.Fatal("undo add retained the newly added node")
	}

	deleteBuilder, err := document.NewBuilderFrom(added)
	if err != nil {
		t.Fatal(err)
	}
	if err := deleteBuilder.SetSequenceItems(root, []document.NodeID{first}); err != nil {
		t.Fatal(err)
	}
	deleted, err := deleteBuilder.Build()
	if err != nil {
		t.Fatal(err)
	}
	del, err := NewSnapshotOperation(added, deleted, []document.NodeID{second}, []Effect{{
		Kind:     EffectNodeDeleted,
		NodeID:   second,
		ParentID: root,
	}})
	if err != nil {
		t.Fatal(err)
	}
	undoDelete, err := document.NewBuilderFrom(deleted)
	if err != nil {
		t.Fatal(err)
	}
	if err := del.Undo(undoDelete); err != nil {
		t.Fatal(err)
	}
	undoneDelete, err := undoDelete.Build()
	if err != nil {
		t.Fatal(err)
	}
	if _, value, exists := undoneDelete.Scalar(second); !exists || value != "second" {
		t.Fatalf("undo delete value = %v, %v; want second, true", value, exists)
	}
}
