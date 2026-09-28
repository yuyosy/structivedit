# Basic embedding example

Decode a YAML document, create an Editor, and attach the Bubble Tea adapter:

```go
source := strings.NewReader("name: router01\nenabled: true\n")
session, err := yamlcodec.Decode(source)
if err != nil {
	return err
}

editor, err := structivedit.New(session.Document(), structivedit.WithPolicy(
	structivedit.Policy{
		Default: structivedit.ScopePolicy{Editable: structivedit.Allow},
	},
))
if err != nil {
	return err
}

model := bubbletea.NewModel(editor)
program := tea.NewProgram(model)
_, err = program.Run()
return err
```

The embedding application owns file loading and saving. Register a save handler
with `model.SetSaveHandler` when Ctrl+S should persist the current snapshot.
