package resolver

import (
	"testing"

	"github.com/yuyosy/structivedit/document"
	"github.com/yuyosy/structivedit/schema"
)

func TestValidateUnknownFieldPolicies(t *testing.T) {
	doc, keyID, _ := makeAdditionalPropertyDocument(t, "extra", "value")
	tests := []struct {
		name       string
		policy     schema.UnknownFieldPolicy
		wantIssues int
		severity   schema.Severity
	}{
		{name: "allow"},
		{name: "warn", policy: schema.UnknownFieldWarn, wantIssues: 1, severity: schema.SeverityWarning},
		{name: "deny", policy: schema.UnknownFieldDeny, wantIssues: 1, severity: schema.SeverityError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compiled, err := CompileSchema(schema.Node{
				Kind: schema.ObjectKind,
				Object: schema.ObjectSchema{
					Fields:        []schema.Field{{Name: "known", Schema: schema.Node{Kind: schema.IntegerKind}}},
					UnknownFields: test.policy,
				},
			})
			if err != nil {
				t.Fatalf("CompileSchema: %v", err)
			}
			issues := ValidateSchema(doc, compiled)
			if len(issues) != test.wantIssues {
				t.Fatalf("ValidateSchema returned %d issues, want %d: %#v", len(issues), test.wantIssues, issues)
			}
			if test.wantIssues == 0 {
				return
			}
			if issues[0].Code != "schema.unknown_field" || issues[0].NodeID != keyID || issues[0].Severity != test.severity {
				t.Fatalf("unknown-field issue = %#v", issues[0])
			}
		})
	}
}

func TestAdditionalPropertySchemaValidatesUnknownValues(t *testing.T) {
	doc, _, valueID := makeAdditionalPropertyDocument(t, "extra", int64(7))
	additional := schema.Node{Kind: schema.StringKind}
	compiled, err := CompileSchema(schema.Node{
		Kind: schema.ObjectKind,
		Object: schema.ObjectSchema{
			Fields:               []schema.Field{{Name: "known", Schema: schema.Node{Kind: schema.IntegerKind}}},
			AdditionalProperties: &additional,
		},
	})
	if err != nil {
		t.Fatalf("CompileSchema: %v", err)
	}

	issues := ValidateSchema(doc, compiled)
	if len(issues) != 1 || issues[0].Code != "schema.type_mismatch" || issues[0].NodeID != valueID {
		t.Fatalf("additional property validation issues = %#v", issues)
	}
}

func TestAdditionalPropertyCustomValidatorsUseFullValidation(t *testing.T) {
	doc, _, dynamicValueID := makeAdditionalPropertyDocument(t, "extra", "ok")
	entries, _ := doc.MappingEntries(doc.Root())
	knownValueID := entries[0].Value
	additional := schema.Node{
		Kind: schema.StringKind,
		Scalar: schema.ScalarSchema{Validators: []schema.Validator{func(context schema.ValueContext) []schema.Issue {
			_, knownValue, ok := context.Document.Scalar(knownValueID)
			if ok && knownValue.(int64) > 0 {
				return []schema.Issue{{Code: "schema.dependent", Message: "extra value depends on known", Severity: schema.SeverityError}}
			}
			return nil
		}}},
	}
	compiled, err := CompileSchema(schema.Node{
		Kind: schema.ObjectKind,
		Object: schema.ObjectSchema{
			Fields:               []schema.Field{{Name: "known", Schema: schema.Node{Kind: schema.IntegerKind}}},
			AdditionalProperties: &additional,
		},
	})
	if err != nil {
		t.Fatalf("CompileSchema: %v", err)
	}
	previous := ValidateSchema(doc, compiled)

	edit, err := document.NewBuilderFrom(doc)
	if err != nil {
		t.Fatalf("NewBuilderFrom: %v", err)
	}
	if err := edit.SetScalar(knownValueID, int64(1)); err != nil {
		t.Fatalf("SetScalar: %v", err)
	}
	updated, err := edit.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	incremental := ValidateValueChange(updated, compiled, previous, knownValueID)
	complete := ValidateSchema(updated, compiled)
	if !sameValidationIssues(incremental, complete) {
		t.Fatalf("incremental issues %#v differ from full issues %#v", incremental, complete)
	}
	if len(complete) != 1 || complete[0].NodeID != dynamicValueID || complete[0].Code != "schema.dependent" {
		t.Fatalf("full validation issues = %#v", complete)
	}
}

func TestCompileSchemaRejectsInvalidAdditionalPropertySchema(t *testing.T) {
	root := &schema.Node{Kind: schema.ObjectKind}
	root.Object.AdditionalProperties = root
	if _, err := CompileSchema(*root); err != schema.ErrInvalidSchema {
		t.Fatalf("CompileSchema recursive additional property error = %v, want ErrInvalidSchema", err)
	}
	if _, err := CompileSchema(schema.Node{Kind: schema.ObjectKind, Object: schema.ObjectSchema{UnknownFields: schema.UnknownFieldPolicy(255)}}); err != schema.ErrInvalidSchema {
		t.Fatalf("CompileSchema invalid unknown-field policy error = %v, want ErrInvalidSchema", err)
	}
}

func makeAdditionalPropertyDocument(t *testing.T, key string, value any) (*document.Document, document.NodeID, document.NodeID) {
	t.Helper()
	builder := document.NewBuilder()
	knownKey, err := builder.NewScalar("known")
	if err != nil {
		t.Fatalf("NewScalar known key: %v", err)
	}
	knownValue, err := builder.NewScalar(int64(0))
	if err != nil {
		t.Fatalf("NewScalar known value: %v", err)
	}
	additionalKey, err := builder.NewScalar(key)
	if err != nil {
		t.Fatalf("NewScalar additional key: %v", err)
	}
	additionalValue, err := builder.NewScalar(value)
	if err != nil {
		t.Fatalf("NewScalar additional value: %v", err)
	}
	root, err := builder.NewMapping([]document.MappingEntry{
		{Key: knownKey, Value: knownValue},
		{Key: additionalKey, Value: additionalValue},
	})
	if err != nil {
		t.Fatalf("NewMapping: %v", err)
	}
	if err := builder.SetRoot(root); err != nil {
		t.Fatalf("SetRoot: %v", err)
	}
	doc, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return doc, additionalKey, additionalValue
}
