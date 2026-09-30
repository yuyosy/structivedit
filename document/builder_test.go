package document

import "testing"

func TestBuilderIncrementalIndexesAndSnapshotIsolation(t *testing.T) {
	builder := NewBuilder()
	target, err := builder.Reserve()
	if err != nil {
		t.Fatal(err)
	}
	alias, err := builder.Reserve()
	if err != nil {
		t.Fatal(err)
	}
	root, err := builder.Reserve()
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.DefineReference(alias, target); err != nil {
		t.Fatal(err)
	}
	if err := builder.DefineScalar(target, "before"); err != nil {
		t.Fatal(err)
	}
	if err := builder.DefineSequence(root, []NodeID{target, alias}); err != nil {
		t.Fatal(err)
	}
	if err := builder.SetRoot(root); err != nil {
		t.Fatal(err)
	}
	before, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}

	parent, ok := before.Parent(alias)
	if !ok || parent != (ParentRef{Parent: root, Role: ParentSequenceItem, Index: 1}) {
		t.Fatalf("alias parent = %#v, %v", parent, ok)
	}
	if got, ok := before.ReferenceTarget(alias); !ok || got != target {
		t.Fatalf("reference target = %d, %v; want %d, true", got, ok, target)
	}

	updatedBuilder, err := NewBuilderFrom(before)
	if err != nil {
		t.Fatal(err)
	}
	if err := updatedBuilder.SetScalar(target, "after"); err != nil {
		t.Fatal(err)
	}
	if err := updatedBuilder.SetSequenceItems(root, []NodeID{alias}); err != ErrInvalidDocument {
		t.Fatalf("removing a referenced target returned %v, want ErrInvalidDocument", err)
	}
	after, err := updatedBuilder.Build()
	if err != nil {
		t.Fatal(err)
	}
	if _, value, ok := before.Scalar(target); !ok || value != "before" {
		t.Fatalf("original snapshot value = %v, %v; want before, true", value, ok)
	}
	if _, value, ok := after.Scalar(target); !ok || value != "after" {
		t.Fatalf("updated snapshot value = %v, %v; want after, true", value, ok)
	}
}

func BenchmarkBuilderReserveAndDefine(b *testing.B) {
	for iteration := 0; iteration < b.N; iteration++ {
		builder := NewBuilder()
		ids := make([]NodeID, 2048)
		for index := range ids {
			id, err := builder.Reserve()
			if err != nil {
				b.Fatal(err)
			}
			ids[index] = id
		}
		for _, id := range ids {
			if err := builder.DefineScalar(id, "value"); err != nil {
				b.Fatal(err)
			}
		}
		root, err := builder.NewSequence(ids)
		if err != nil {
			b.Fatal(err)
		}
		if err := builder.SetRoot(root); err != nil {
			b.Fatal(err)
		}
		if _, err := builder.Build(); err != nil {
			b.Fatal(err)
		}
	}
}
