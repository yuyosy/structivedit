package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	structivedit "github.com/yuyosy/structivedit"
	"github.com/yuyosy/structivedit/adapter/bubbletea"
	yamlcodec "github.com/yuyosy/structivedit/codec/yaml"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) != 1 || args[0] == "" {
		return fmt.Errorf("usage: structivedit <file.yaml>")
	}
	path := args[0]
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	fileInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("stat %s: %w", path, err)
	}
	session, decodeErr := yamlcodec.Decode(file)
	closeErr := file.Close()
	if decodeErr != nil {
		return fmt.Errorf("decode %s: %w", path, decodeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", path, closeErr)
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
	model := bubbletea.NewModel(editor)
	model.SetSaveHandler(func() error {
		var encoded bytes.Buffer
		if err := session.Encode(&encoded, editor.Document()); err != nil {
			return fmt.Errorf("encode %s: %w", path, err)
		}
		if err := os.WriteFile(path, encoded.Bytes(), fileInfo.Mode().Perm()); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		editor.MarkClean()
		return nil
	})

	reader := bufio.NewReader(stdin)
	for {
		program := tea.NewProgram(model, tea.WithInput(reader), tea.WithOutput(stdout))
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
