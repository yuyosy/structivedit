package resolver

import (
	"fmt"
	"testing"

	"github.com/yuyosy/structivedit/document"
	"github.com/yuyosy/structivedit/schema"
)

func TestValidateValueChangeMatchesFullValidation(t *testing.T) {
	builder := document.NewBuilder()
	firstKey, err := builder.NewScalar("item")
	if err != nil {
		t.Fatal(err)
	}
	firstValue, err := builder.NewScalar(1)
	if err != nil {
		t.Fatal(err)
	}
	secondKey, err := builder.NewScalar("item")
	if err != nil {
		t.Fatal(err)
	}
	secondValue, err := builder.NewScalar(2)
	if err != nil {
		t.Fatal(err)
	}
	root, err := builder.NewMapping([]document.MappingEntry{
		{Key: firstKey, Value: firstValue},
		{Key: secondKey, Value: secondValue},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SetRoot(root); err != nil {
		t.Fatal(err)
	}
	before, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompileSchema(schema.Node{
		Kind: schema.ObjectKind,
		Object: schema.ObjectSchema{Fields: []schema.Field{{
			Name: "item",
			Schema: schema.Node{
				Kind:   schema.IntegerKind,
				Scalar: schema.ScalarSchema{Min: int64(5)},
			},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	previous := ValidateSchema(before, compiled)

	edit, err := document.NewBuilderFrom(before)
	if err != nil {
		t.Fatal(err)
	}
	if err := edit.SetScalar(secondValue, 5); err != nil {
		t.Fatal(err)
	}
	after, err := edit.Build()
	if err != nil {
		t.Fatal(err)
	}
	incremental := ValidateValueChange(after, compiled, previous, secondValue)
	complete := ValidateSchema(after, compiled)
	if !sameValidationIssues(incremental, complete) {
		t.Fatalf("incremental issues %#v differ from full issues %#v", incremental, complete)
	}
}

func TestValidateValueChangeFallsBackForCustomValidators(t *testing.T) {
	builder := document.NewBuilder()
	keyA, err := builder.NewScalar("a")
	if err != nil {
		t.Fatal(err)
	}
	valueA, err := builder.NewScalar(0)
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := builder.NewScalar("b")
	if err != nil {
		t.Fatal(err)
	}
	valueB, err := builder.NewScalar("ok")
	if err != nil {
		t.Fatal(err)
	}
	root, err := builder.NewMapping([]document.MappingEntry{
		{Key: keyA, Value: valueA},
		{Key: keyB, Value: valueB},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.SetRoot(root); err != nil {
		t.Fatal(err)
	}
	before, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompileSchema(schema.Node{
		Kind: schema.ObjectKind,
		Object: schema.ObjectSchema{Fields: []schema.Field{
			{Name: "a", Schema: schema.Node{Kind: schema.IntegerKind}},
			{Name: "b", Schema: schema.Node{
				Kind: schema.StringKind,
				Scalar: schema.ScalarSchema{Validators: []schema.Validator{func(context schema.ValueContext) []schema.Issue {
					_, dependency, ok := context.Document.Scalar(valueA)
					if ok && dependency.(int64) > 0 {
						return []schema.Issue{{Code: "dependent", Message: "a must be zero", Severity: schema.SeverityError}}
					}
					return nil
				}}},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	previous := ValidateSchema(before, compiled)

	edit, err := document.NewBuilderFrom(before)
	if err != nil {
		t.Fatal(err)
	}
	if err := edit.SetScalar(valueA, 1); err != nil {
		t.Fatal(err)
	}
	after, err := edit.Build()
	if err != nil {
		t.Fatal(err)
	}
	incremental := ValidateValueChange(after, compiled, previous, valueA)
	complete := ValidateSchema(after, compiled)
	if !sameValidationIssues(incremental, complete) {
		t.Fatalf("custom validator fallback issues %#v differ from full issues %#v", incremental, complete)
	}
}

func sameValidationIssues(left, right []ValidationIssue) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].NodeID != right[index].NodeID || left[index].Code != right[index].Code || left[index].Message != right[index].Message || left[index].Severity != right[index].Severity || !left[index].Path.Equal(right[index].Path) {
			return false
		}
	}
	return true
}

func BenchmarkValidateDuplicateKeysLargeMapping(b *testing.B) {
	builder := document.NewBuilder()
	entries := make([]document.MappingEntry, 2048)
	for index := range entries {
		key, err := builder.NewScalar(fmt.Sprintf("key-%d", index))
		if err != nil {
			b.Fatal(err)
		}
		value, err := builder.NewScalar(index)
		if err != nil {
			b.Fatal(err)
		}
		entries[index] = document.MappingEntry{Key: key, Value: value}
	}
	root, err := builder.NewMapping(entries)
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
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		ValidateSchema(doc, CompiledSchema{})
	}
}
