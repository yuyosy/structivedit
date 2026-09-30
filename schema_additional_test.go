package structivedit

import (
	"testing"

	"github.com/yuyosy/structivedit/document"
	"github.com/yuyosy/structivedit/schema"
)

func TestPrepareAddSupportsAdditionalObjectProperties(t *testing.T) {
	doc, rootID, _, _ := buildAdditionalPropertyTestDocument(t, false)
	additional := schema.Node{Kind: schema.StringKind}
	editor, err := New(doc,
		WithSchema(schema.Node{Kind: schema.ObjectKind, Object: schema.ObjectSchema{AdditionalProperties: &additional}}),
		WithPolicy(Policy{Default: ScopePolicy{Editable: Allow, Addable: Allow, Deletable: Allow}}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if view, err := editor.View(rootID); err != nil || !view.Capabilities.Addable {
		t.Fatalf("root Addable = %v, error = %v", view.Capabilities.Addable, err)
	}

	plan, err := editor.PrepareAdd(rootID, AddTarget{Kind: AddObjectField, Field: "custom"})
	if err != nil {
		t.Fatalf("PrepareAdd: %v", err)
	}
	if len(plan.Inputs) != 1 || plan.Inputs[0].ExpectedKind != schema.StringKind {
		t.Fatalf("additional property input requests = %#v", plan.Inputs)
	}
	if _, err := editor.CommitAdd(plan, map[InputID]any{plan.Inputs[0].ID: "value"}); err != nil {
		t.Fatalf("CommitAdd: %v", err)
	}
	entries, ok := editor.Document().MappingEntries(rootID)
	if !ok || len(entries) != 1 {
		t.Fatalf("mapping entries after add = %#v", entries)
	}
	keyKind, keyValue, keyOK := editor.Document().Scalar(entries[0].Key)
	valueKind, value, valueOK := editor.Document().Scalar(entries[0].Value)
	if !keyOK || keyKind != document.ScalarString || keyValue != "custom" || !valueOK || valueKind != document.ScalarString || value != "value" {
		t.Fatalf("added mapping entry = (%v, %v), want (custom, value)", keyValue, value)
	}
	if issues := editor.Validate(); len(issues) != 0 {
		t.Fatalf("validation after add = %#v, want no issues", issues)
	}
}

func TestUnknownFieldDenyDisallowsAddingAdditionalProperties(t *testing.T) {
	doc, rootID, _, _ := buildAdditionalPropertyTestDocument(t, false)
	additional := schema.Node{Kind: schema.StringKind}
	editor, err := New(doc,
		WithSchema(schema.Node{
			Kind: schema.ObjectKind,
			Object: schema.ObjectSchema{
				AdditionalProperties: &additional,
				UnknownFields:        schema.UnknownFieldDeny,
			},
		}),
		WithPolicy(Policy{Default: ScopePolicy{Addable: Allow}}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	view, err := editor.View(rootID)
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if view.Capabilities.Addable {
		t.Fatal("root reports Addable with UnknownFieldDeny and no declared fields")
	}
	if _, err := editor.PrepareAdd(rootID, AddTarget{Kind: AddObjectField, Field: "custom"}); err != ErrNotAddable {
		t.Fatalf("PrepareAdd error = %v, want ErrNotAddable", err)
	}
}

func TestUnknownAdditionalPropertyCanBeDeleted(t *testing.T) {
	doc, rootID, keyID, valueID := buildAdditionalPropertyTestDocument(t, true)
	editor, err := New(doc,
		WithSchema(schema.Node{Kind: schema.ObjectKind, Object: schema.ObjectSchema{UnknownFields: schema.UnknownFieldWarn}}),
		WithPolicy(Policy{Default: ScopePolicy{Deletable: Allow}}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	view, err := editor.View(valueID)
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if !view.Capabilities.Deletable {
		t.Fatal("unknown property is not deletable")
	}
	issues := editor.Issues()
	if len(issues) != 1 || issues[0].Code != "schema.unknown_field" || issues[0].NodeID != keyID {
		t.Fatalf("initial unknown-field issue = %#v", issues)
	}
	if _, err := editor.Delete(valueID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	entries, ok := editor.Document().MappingEntries(rootID)
	if !ok || len(entries) != 0 {
		t.Fatalf("mapping entries after delete = %#v, want empty", entries)
	}
}

func buildAdditionalPropertyTestDocument(t *testing.T, includeExtra bool) (*document.Document, document.NodeID, document.NodeID, document.NodeID) {
	t.Helper()
	builder := document.NewBuilder()
	entries := make([]document.MappingEntry, 0)
	var keyID, valueID document.NodeID
	var err error
	if includeExtra {
		keyID, err = builder.NewScalar("extra")
		if err != nil {
			t.Fatalf("NewScalar key: %v", err)
		}
		valueID, err = builder.NewScalar("value")
		if err != nil {
			t.Fatalf("NewScalar value: %v", err)
		}
		entries = append(entries, document.MappingEntry{Key: keyID, Value: valueID})
	}
	rootID, err := builder.NewMapping(entries)
	if err != nil {
		t.Fatalf("NewMapping: %v", err)
	}
	if err := builder.SetRoot(rootID); err != nil {
		t.Fatalf("SetRoot: %v", err)
	}
	doc, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return doc, rootID, keyID, valueID
}
