# StructiveEdit

StructiveEdit is a Go library and reference terminal app for editing one YAML
document as an immutable, ordered node tree. The Core owns document identity,
schema validation, permissions, undo/redo, and dirty tracking. The YAML codec
and Bubble Tea adapter stay in separate packages.

## Requirements

- Go 1.27 or later.
- The Core and each Editor instance are not thread-safe. Use one Editor from a
  single goroutine.
- YAML is the only bundled format. JSON support is not included.

## Reference CLI

Run the editor from a checkout:

```sh
go run ./cmd/structivedit config.yaml
```

The CLI opens one existing file. It writes only after Ctrl+S. Press `q` or
Escape to leave the editor; if the document is dirty, choose discard or return
to editing. Ctrl+S errors stay visible in the editor and do not clear dirty
state.

Keyboard controls:

| Key | Action |
|---|---|
| Up / Down | Move focus through visible nodes |
| Left / Right | Collapse or expand a container; move to its parent or first child |
| Enter | Edit a scalar or expand/collapse a container |
| Space | Toggle a boolean |
| Alt+Up / Alt+Down | Reorder a sequence item |
| Ctrl+Z / Ctrl+Y | Undo / redo |
| Ctrl+S | Save through the calling application |
| `a` / `d` | Start schema-driven add / delete when available |
| Escape / `q` | Cancel a prompt or leave the editor |

The reference CLI has no schema file option, so Add and Delete are unavailable
there. Embedding applications can enable them by supplying a Schema and Policy.

## Embed the editor

The caller owns file I/O and chooses when a save succeeds. Call `MarkClean`
only after both encoding and writing finish successfully. The caller also
decides whether to keep editing or discard changes when the TUI exits dirty.

```go
func edit(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	session, err := yamlcodec.Decode(file)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}

	editor, err := structivedit.New(
		session.Document(),
		structivedit.WithPolicy(structivedit.Policy{
			Default: structivedit.ScopePolicy{
				Editable:    structivedit.Allow,
				Reorderable: structivedit.Allow,
			},
		}),
	)
	if err != nil {
		return err
	}
	editor.MarkClean()

	model := bubbletea.NewModel(editor)
	model.SetSaveHandler(func() error {
		var output bytes.Buffer
		if err := session.Encode(&output, editor.Document()); err != nil {
			return err
		}
		if err := os.WriteFile(path, output.Bytes(), 0o600); err != nil {
			return err
		}
		editor.MarkClean()
		return nil
	})
	_, err = tea.NewProgram(model).Run()
	return err
}
```

The snippet uses these imports:

```go
import (
	"bytes"
	"os"

	structivedit "github.com/yuyosy/structivedit"
	"github.com/yuyosy/structivedit/adapter/bubbletea"
	yamlcodec "github.com/yuyosy/structivedit/codec/yaml"
	"github.com/yuyosy/structivedit/schema"
	tea "charm.land/bubbletea/v2"
)
```

Add operations are prepared and committed through `Editor.PrepareAdd` and
`Editor.CommitAdd`. Array item schemas use a pointer to `schema.Node` so the Go
type remains finite in size:

```go
arraySchema := schema.Node{
	Kind: schema.ArrayKind,
	Array: schema.ArraySchema{
		Item: &schema.Node{Kind: schema.StringKind},
	},
}
```

Schemas are copied and validated when an Editor is created. Schema cycles are
rejected.

## YAML preservation

The codec reads one YAML 1.2 Core Schema document and preserves the semantic
node structure. It supports ordered sequences and mappings, duplicate keys,
non-string keys, comments, scalar and collection styles, anchors, aliases, and
merge entries. Aliases and merge entries are read-only. Custom tags retain their
tag and representable payload and make the tagged root read-only. Integers
outside the signed 64-bit range are rejected.

Encoding preserves metadata by NodeID across edits, reordering, undo, and redo.
Output is UTF-8 with LF line endings, two-space indentation, and a final newline;
byte-for-byte source reproduction is not promised. Unrepresentable node or tag
payloads and multiple documents return errors instead of being silently
discarded.

## Development

Build all packages with:

```sh
go build ./...
```

GitHub Actions builds with Go 1.27 and the current stable Go release.
