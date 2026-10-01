# StructiveEdit

StructiveEdit is a Go library and reference terminal app for editing YAML as an
ordered tree. The core editor, document model, codecs, and terminal UI are
separate packages so applications can own persistence and choose which parts to
embed.

## Packages

| Package | Responsibility |
|---|---|
| `document` | Immutable document snapshots, node identities, paths, and builders |
| `structivedit` | Editing, validation, permissions, focus, and undo/redo |
| `codec` | Format-independent decode and encode session contract |
| `codec/yaml` | YAML decoding, metadata preservation, and encoding |
| `adapter/bubbletea` | Bubble Tea terminal editor and input handling |
| `cmd/structivedit` | Reference CLI for editing an existing YAML file |

## Requirements and concurrency

- Go 1.27 or later.
- YAML is the only bundled format. The `codec` interface can be implemented
  for other formats.
- An `Editor` instance is not safe for concurrent use. Keep all calls on one
  Editor on a single goroutine, including read methods.
- A `document.Document` is an immutable snapshot. After capturing a snapshot,
  its read-only `Document` and `Node` methods can be called concurrently. Do
  not call `Editor.Document` concurrently with other use of that Editor.

## Reference CLI

Run the editor from a checkout:

```sh
go run ./cmd/structivedit [--expand-aliases] [--no-color] [--inline-edit] config.yaml
```

The CLI opens one existing YAML file. It does not create a file when starting.
The file is written after Ctrl+S or after choosing overwrite in the external
change prompt.

| Option | Behavior |
|---|---|
| `--expand-aliases` | Show alias targets as read-only rows |
| `--inline-edit` | Edit scalar values in their tree rows |
| `--no-color` | Disable terminal colors; `NO_COLOR` has the same effect |

| Key | Behavior |
|---|---|
| Up / Down | Move focus through visible nodes |
| Left / Right | Collapse or expand a container; move to its parent or first child |
| Enter | Edit a scalar or expand/collapse a container |
| Tab | Switch between row-head and Value-cell cursor modes |
| Space | Toggle a boolean |
| Alt+Up / Alt+Down | Reorder a sequence item when allowed |
| Ctrl+Z / Ctrl+Y | Undo / redo |
| Ctrl+S | Save through the CLI's file handler |
| `?` | Show or hide the full shortcut list |
| `a` / `d` | Start schema-driven add / delete when available |
| Escape | Cancel the current prompt |
| `q` / Ctrl+Q / Ctrl+C | Leave the editor from the browse view |

Double-clicking an editable scalar edits it; double-clicking an editable
boolean toggles it. The reference CLI has no schema file option, so Add and
Delete are unavailable there.

### Save and external changes

The CLI compares the file's SHA-256 fingerprint with the version last loaded or
saved whenever Ctrl+S is pressed. If the content changed or the file was
removed, choose:

| Key | Behavior |
|---|---|
| `o` | Overwrite the external version with the current document |
| `r` | Reload the file and discard local edits |
| Escape | Cancel saving and keep local edits |

If reload fails because the file is missing or invalid YAML, the error remains
visible and the prompt stays open so the user can retry, overwrite, or cancel.
Save errors remain visible and do not clear the dirty state. There is no
background file watcher or file lock: changes are checked when saving, and a
separate process can still write between the check and the save. When leaving
with unsaved edits, the CLI asks whether to discard them or continue editing.

## Embed the editor

The embedding application owns file I/O and decides when a save succeeds. A
typical integration decodes a document, creates an Editor, attaches a save
handler, then runs the Bubble Tea program:

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

	editor, err := structivedit.New(session.Document(), structivedit.WithPolicy(
		structivedit.Policy{
			Default: structivedit.ScopePolicy{Editable: structivedit.Allow},
		},
	))
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

The example uses these imports:

```go
import (
	"bytes"
	"os"

	structivedit "github.com/yuyosy/structivedit"
	"github.com/yuyosy/structivedit/adapter/bubbletea"
	yamlcodec "github.com/yuyosy/structivedit/codec/yaml"
	tea "charm.land/bubbletea/v2"
)
```

`bubbletea.NewModel` also accepts these options:

| Option | Default / behavior |
|---|---|
| `WithAliasExpansion(true)` | Disabled by default; show alias targets as read-only rows |
| `WithInlineEditing(true)` | Disabled by default; edit scalar values in their rows |
| `WithColors(false)` | Enabled by default; `NO_COLOR` disables colors |
| `WithMouseDoubleClick(false)` | Enabled by default for editable scalars and booleans |

Call `Editor.MarkClean` only after encoding and writing both succeed. The
caller is responsible for the dirty-exit decision and for selecting file
permissions and other persistence behavior. `Editor.Apply` accepts typed
operations such as `SetValue`, `Toggle`, `Move`, `Add`, `Delete`, `Focus`,
`Undo`, and `Redo`.

### External change handling

The Bubble Tea adapter does not read, watch, or lock files. An application that
wants conflict handling can compare a saved fingerprint or version in its
`SetSaveHandler` and return `bubbletea.ErrSaveConflict` when the source changed.
Register `SetSaveConflictHandler` to resolve that error:

- `SaveConflictOverwrite`: write the current document and return a nil Editor.
- `SaveConflictReload`: decode the external version, create a clean Editor
  from its new session, and return that Editor. The model switches to it.
- Escape cancels the save without calling the conflict handler; the current
  Editor and its local changes stay in place.

After reload, use `model.Editor()` to get the Editor now controlled by the
model. If application callbacks retain their own Editor or codec Session
references, update those references in the reload handler as well. The
reference CLI implements this flow with SHA-256 fingerprints.

## Schemas and permissions

Schemas describe scalar constraints, declared object fields, array items, and
the shapes that Add can create. They can include defaults, enums, numeric
ranges, required fields, array length limits, and custom validators. Schema
validators run synchronously against immutable document snapshots.

Permissions are configured separately with `Policy`. Unspecified operations
are denied. Scope rules can allow or deny editing, adding, deleting, or
reordering at selected locations; `TypePolicy` controls scalar editing by
kind. Selectors use paths beginning with `$`, with key and index segments,
wildcards (`.*`, `[*]`), and the recursive wildcard (`.**`). For example,
`$.services[0].port` selects an array item's `port` value and
`$["service name"].**` uses a quoted key and recursive wildcard. This is a
small selector grammar, not full JSONPath.

Object schemas accept undeclared keys by default. Set `UnknownFields` to
`schema.UnknownFieldWarn` or `schema.UnknownFieldDeny` to report them as
warnings or errors. `AdditionalProperties` validates undeclared values and
provides the schema for adding new keys when policy permits it. Array item
schemas use a pointer so recursive Go schema types have finite size. Schemas
are copied and validated when an Editor is created; schema cycles are rejected.

```go
additionalValue := schema.Node{Kind: schema.StringKind}
shape := schema.Node{
	Kind: schema.ObjectKind,
	Object: schema.ObjectSchema{
		Fields: []schema.Field{{
			Name:     "name",
			Schema:   schema.Node{Kind: schema.StringKind},
			Required: true,
		}},
		AdditionalProperties: &additionalValue,
		UnknownFields:        schema.UnknownFieldWarn,
	},
}
```

This schema fragment uses `github.com/yuyosy/structivedit/schema`. Pass
`shape` to `structivedit.WithSchema` when creating the Editor; allow `Addable`
in the applicable Policy scope to enable adding entries.

See the [basic embedding example](examples/basic/README.md) and the
[schema example](examples/schema/README.md) for focused integrations.

## YAML codec

The bundled codec reads one YAML 1.2 Core Schema document. It supports ordered
sequences and mappings, duplicate keys, non-string keys, comments, scalar and
collection styles, anchors, aliases, and merge entries. Alias references and
merge entries are read-only; alias target rows can be shown with
`bubbletea.WithAliasExpansion(true)`. Custom tags retain their tag and
representable payload and make the tagged node read-only. Integers outside the
signed 64-bit range and multiple YAML documents are rejected.

Encoding preserves YAML metadata by NodeID across edits, reordering, undo, and
redo. Output is UTF-8 with LF line endings, two-space indentation, and a final
newline. Byte-for-byte source reproduction is not promised; unrepresentable
nodes or tag payloads return errors rather than being silently discarded.

For untrusted input, use `yamlcodec.DecodeWithOptions` to limit input bytes,
node count, or nesting depth. A zero limit is unlimited. `yamlcodec.Decode`
keeps unlimited behavior for compatibility.

## Implementing another codec

Implement `codec.Codec` and `codec.Session` for the format, then encode later
snapshots from the same Document lineage without silently discarding content.
`codec/conformance.Run` can be called from the codec's `_test.go` file to check
decode, round-trip, edited-snapshot, lineage, and reader/writer error behavior.
Format-specific preservation behavior belongs in that codec's own tests.

## Development

Build all packages:

```sh
go build ./...
```

Run all package tests:

```sh
go test ./...
```

GitHub Actions tests and builds with Go 1.27 and the current stable Go release.
