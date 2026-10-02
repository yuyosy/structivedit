package structivedit_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	se "github.com/yuyosy/structivedit"
	yamlcodec "github.com/yuyosy/structivedit/codec/yaml"
	"github.com/yuyosy/structivedit/schema"
)

func TestYAMLDeleteOptionalFieldAndUndo(t *testing.T) {
	for _, input := range []string{"name: hello\n", "name: !custom hello\n"} {
		session, err := yamlcodec.Decode(strings.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		shape := schema.Node{Kind: schema.ObjectKind, Object: schema.ObjectSchema{Fields: []schema.Field{{Name: "name", Schema: schema.Node{Kind: schema.StringKind}}}}}
		editor, err := se.New(session.Document(), se.WithSchema(shape), se.WithPolicy(se.Policy{Default: se.ScopePolicy{Editable: se.Allow, Deletable: se.Allow}}))
		if err != nil {
			t.Fatal(err)
		}
		entries, _ := editor.Document().MappingEntries(editor.Document().Root())
		if _, err := editor.Apply(se.SetValue{NodeID: entries[0].Key, Value: "renamed"}); !errors.Is(err, se.ErrNotEditable) {
			t.Fatalf("key edit: %v", err)
		}
		_, err = editor.Delete(entries[0].Value)
		if strings.Contains(input, "!custom") {
			if !errors.Is(err, se.ErrNotDeletable) {
				t.Fatalf("tagged value delete: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := editor.Apply(se.Undo{}); err != nil {
			t.Fatal(err)
		}
		var encoded bytes.Buffer
		if err := session.Encode(&encoded, editor.Document()); err != nil {
			t.Fatal(err)
		}
		if encoded.String() != input {
			t.Fatalf("undo output = %q", encoded.String())
		}
	}
}

func TestYAMLMovePreservesAnchorOrder(t *testing.T) {
	session, err := yamlcodec.Decode(strings.NewReader("- &a hello\n- ordinary\n- *a\n"))
	if err != nil {
		t.Fatal(err)
	}
	editor, err := se.New(session.Document(), se.WithPolicy(se.Policy{Default: se.ScopePolicy{Reorderable: se.Allow}}))
	if err != nil {
		t.Fatal(err)
	}
	items, _ := editor.Document().SequenceItems(editor.Document().Root())
	before := editor.Document()
	if _, err := editor.Move(items[0], 2); !errors.Is(err, se.ErrNotReorderable) {
		t.Fatalf("forward anchor move: %v", err)
	}
	if editor.Document() != before || editor.IsDirty() {
		t.Fatal("rejected move changed editor")
	}
	if _, err := editor.Move(items[0], 1); err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := session.Encode(&encoded, editor.Document()); err != nil {
		t.Fatal(err)
	}
	if _, err := yamlcodec.Decode(&encoded); err != nil {
		t.Fatal(err)
	}
}

func TestYAMLNaNRangeValidation(t *testing.T) {
	session, err := yamlcodec.Decode(strings.NewReader(".nan\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, ranged := range []bool{false, true} {
		shape := schema.Node{Kind: schema.FloatKind}
		if ranged {
			shape.Scalar.Min = float64(0)
			shape.Scalar.Max = float64(1)
		}
		editor, err := se.New(session.Document(), se.WithSchema(shape))
		if err != nil {
			t.Fatal(err)
		}
		if editor.HasErrors() != ranged {
			t.Fatalf("ranged=%v issues=%v", ranged, editor.Issues())
		}
	}
}
