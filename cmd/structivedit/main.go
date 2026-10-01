package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	structivedit "github.com/yuyosy/structivedit"
	"github.com/yuyosy/structivedit/adapter/bubbletea"
	"github.com/yuyosy/structivedit/codec"
	yamlcodec "github.com/yuyosy/structivedit/codec/yaml"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("structivedit", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var expandAliases bool
	var noColor bool
	var inlineEdit bool
	flags.BoolVar(&expandAliases, "expand-aliases", false, "show alias contents as read-only rows")
	flags.BoolVar(&noColor, "no-color", false, "disable terminal colors")
	flags.BoolVar(&inlineEdit, "inline-edit", false, "edit scalar values in their tree rows")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w\nusage: structivedit [--expand-aliases] [--no-color] [--inline-edit] <file.yaml>", err)
	}
	files := flags.Args()
	if len(files) != 1 || files[0] == "" {
		return fmt.Errorf("usage: structivedit [--expand-aliases] [--no-color] [--inline-edit] <file.yaml>")
	}
	path := files[0]
	session, fileMode, sourceHash, err := loadFile(path)
	if err != nil {
		return err
	}
	policy := structivedit.Policy{
		Default: structivedit.ScopePolicy{
			Editable:    structivedit.Allow,
			Reorderable: structivedit.Allow,
		},
	}
	editor, err := structivedit.New(session.Document(), structivedit.WithPolicy(policy))
	if err != nil {
		return fmt.Errorf("create editor: %w", err)
	}
	editor.MarkClean()
	model := bubbletea.NewModel(
		editor,
		bubbletea.WithAliasExpansion(expandAliases),
		bubbletea.WithColors(!noColor),
		bubbletea.WithInlineEditing(inlineEdit),
	)
	encodeCurrent := func() ([]byte, error) {
		var encoded bytes.Buffer
		if err := session.Encode(&encoded, editor.Document()); err != nil {
			return nil, fmt.Errorf("encode %s: %w", path, err)
		}
		return encoded.Bytes(), nil
	}
	writeEncoded := func(encoded []byte) error {
		if err := os.WriteFile(path, encoded, fileMode); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		sourceHash = sha256.Sum256(encoded)
		editor.MarkClean()
		return nil
	}
	writeCurrent := func() error {
		encoded, err := encodeCurrent()
		if err != nil {
			return err
		}
		return writeEncoded(encoded)
	}
	model.SetSaveHandler(func() error {
		encoded, err := encodeCurrent()
		if err != nil {
			return err
		}
		changed, err := fileChanged(path, sourceHash)
		if err != nil {
			return fmt.Errorf("check %s before save: %w", path, err)
		}
		if changed {
			return bubbletea.ErrSaveConflict
		}
		return writeEncoded(encoded)
	})
	model.SetSaveConflictHandler(func(action bubbletea.SaveConflictAction) (*structivedit.Editor, error) {
		switch action {
		case bubbletea.SaveConflictOverwrite:
			if err := writeCurrent(); err != nil {
				return nil, err
			}
			return nil, nil
		case bubbletea.SaveConflictReload:
			reloadedSession, reloadedMode, reloadedHash, err := loadFile(path)
			if err != nil {
				return nil, err
			}
			reloadedEditor, err := structivedit.New(reloadedSession.Document(), structivedit.WithPolicy(policy))
			if err != nil {
				return nil, fmt.Errorf("create editor from %s: %w", path, err)
			}
			reloadedEditor.MarkClean()
			session = reloadedSession
			fileMode = reloadedMode
			sourceHash = reloadedHash
			editor = reloadedEditor
			return reloadedEditor, nil
		default:
			return nil, fmt.Errorf("unknown save conflict action: %d", action)
		}
	})

	reader := bufio.NewReader(stdin)
	for {
		// Pass the original reader so Bubble Tea can detect the terminal and
		// enable raw mode. The buffered reader is reserved for the post-TUI
		// unsaved-changes prompt.
		program := tea.NewProgram(model, tea.WithInput(stdin), tea.WithOutput(stdout))
		_, err := program.Run()
		if err != nil && !errors.Is(err, tea.ErrInterrupted) {
			return fmt.Errorf("run editor: %w", err)
		}
		if !editor.IsDirty() {
			return nil
		}
		discard, err := confirmExit(reader, stderr)
		if err != nil {
			return fmt.Errorf("confirm exit: %w", err)
		}
		if discard {
			return nil
		}
	}
}

func loadFile(path string) (codec.Session, os.FileMode, [sha256.Size]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, [sha256.Size]byte{}, fmt.Errorf("open %s: %w", path, err)
	}
	fileInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, [sha256.Size]byte{}, fmt.Errorf("stat %s: %w", path, err)
	}
	digest := sha256.New()
	session, decodeErr := yamlcodec.Decode(io.TeeReader(file, digest))
	closeErr := file.Close()
	if decodeErr != nil {
		return nil, 0, [sha256.Size]byte{}, fmt.Errorf("decode %s: %w", path, decodeErr)
	}
	if closeErr != nil {
		return nil, 0, [sha256.Size]byte{}, fmt.Errorf("close %s: %w", path, closeErr)
	}
	var sourceHash [sha256.Size]byte
	copy(sourceHash[:], digest.Sum(nil))
	return session, fileInfo.Mode().Perm(), sourceHash, nil
}

func fileChanged(path string, expected [sha256.Size]byte) (bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	digest := sha256.New()
	_, readErr := io.Copy(digest, file)
	closeErr := file.Close()
	if readErr != nil {
		return false, readErr
	}
	if closeErr != nil {
		return false, closeErr
	}
	var actual [sha256.Size]byte
	copy(actual[:], digest.Sum(nil))
	return actual != expected, nil
}

func confirmExit(input *bufio.Reader, output io.Writer) (bool, error) {
	for {
		if _, err := fmt.Fprint(output, "Unsaved changes: [d] discard / [c] continue editing (default c): "); err != nil {
			return false, err
		}
		line, err := input.ReadString('\n')
		if err != nil && len(line) == 0 {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "d", "discard", "y", "yes":
			return true, nil
		case "", "c", "continue", "n", "no":
			return false, nil
		default:
			if _, writeErr := fmt.Fprintln(output, "Enter d to discard or c to continue editing."); writeErr != nil {
				return false, writeErr
			}
		}
		if err != nil {
			return false, err
		}
	}
}
