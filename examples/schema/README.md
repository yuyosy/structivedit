# Schema example

Schemas describe scalar constraints and the fields or items that Add may create.
The adapter uses those declarations to request the values needed for a new node.

```go
shape := schema.Node{
	Kind: schema.ObjectKind,
	Object: schema.ObjectSchema{Fields: []schema.Field{
		{
			Name:     "name",
			Schema:   schema.Node{Kind: schema.StringKind},
			Required: true,
		},
		{
			Name: "enabled",
			Schema: schema.Node{
				Kind:   schema.BoolKind,
				Scalar: schema.ScalarSchema{HasDefault: true, Default: true},
			},
			HasDefault: true,
			Default:    true,
		},
	}},
}

editor, err := structivedit.New(
	session.Document(),
	structivedit.WithSchema(shape),
	structivedit.WithPolicy(structivedit.Policy{
		Default: structivedit.ScopePolicy{
			Editable: structivedit.Allow,
			Addable:  structivedit.Allow,
			Deletable: structivedit.Allow,
		},
	}),
)
```

`ArraySchema.Item` is a pointer so recursive schema values have finite Go size.
Callers should pass a tree without schema cycles; `structivedit.New` validates
and copies it before the Editor uses it.
