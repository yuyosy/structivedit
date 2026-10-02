package bubbletea

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	se "github.com/yuyosy/structivedit"
	yamlcodec "github.com/yuyosy/structivedit/codec/yaml"
	"github.com/yuyosy/structivedit/schema"
)

func testEditor(t testing.TB, source string, options ...se.Option) *se.Editor {
	t.Helper()
	session, err := yamlcodec.Decode(strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	editor, err := se.New(session.Document(), options...)
	if err != nil {
		t.Fatal(err)
	}
	return editor
}

func TestViewEscapesUntrustedControlsAndFitsCells(t *testing.T) {
	m := NewModel(testEditor(t, "\"bad\\x1b[31mkey\": 日本語\n"), WithColors(false))
	m.width = 30
	m.setErrorMessage("error\x1b]52;c;payload\a\nspoofed")
	content := m.View().Content
	if strings.ContainsAny(content, "\x1b\a") {
		t.Fatalf("raw terminal controls: %q", content)
	}
	if !strings.Contains(content, `\x1b`) {
		t.Fatal("control not made visible")
	}
	for _, line := range strings.Split(content, "\n") {
		if ansi.StringWidth(line) > m.width {
			t.Fatalf("line too wide: %q", line)
		}
	}
}

func TestClippingPreservesGraphemes(t *testing.T) {
	m := NewModel(nil, WithColors(false))
	for _, value := range []string{"日本語", "e\u0301abcd", "👩‍💻abcd", "🇯🇵abcd"} {
		for width := 1; width <= 8; width++ {
			line := m.renderStyledLine(width, lineSegment{text: value})
			if ansi.StringWidth(line) > width {
				t.Fatalf("width=%d value=%q line=%q", width, value, line)
			}
			if strings.HasSuffix(line, "\u200d…") {
				t.Fatalf("split emoji: %q", line)
			}
			input := newTextInput(value)
			left, right := inputWindow(input, width)
			if ansi.StringWidth(left+right) > width {
				t.Fatalf("input too wide: %q %q", left, right)
			}
		}
	}
}

func TestInputMovesAndDeletesWholeGraphemes(t *testing.T) {
	for _, cluster := range []string{"e\u0301", "👩‍💻", "🇯🇵", "\r\n"} {
		input := newTextInput("a" + cluster + "b")
		input.handleKey(tea.Key{Code: tea.KeyLeft}, true)
		input.handleKey(tea.Key{Code: tea.KeyBackspace}, true)
		if input.String() != "ab" || input.cursor != 1 {
			t.Fatalf("cluster=%q input=%q cursor=%d", cluster, input.String(), input.cursor)
		}
		input = newTextInput(cluster + "b")
		input.cursor = 0
		input.handleKey(tea.Key{Code: tea.KeyDelete}, true)
		if input.String() != "b" {
			t.Fatalf("delete cluster: %q", input.String())
		}
	}
}

func TestRowsCacheTracksExpansionEditsAndValidation(t *testing.T) {
	invalid := false
	item := schema.Node{Kind: schema.StringKind, Scalar: schema.ScalarSchema{Validators: []schema.Validator{func(schema.ValueContext) []schema.Issue {
		if invalid {
			return []schema.Issue{{Message: "changed", Severity: schema.SeverityError}}
		}
		return nil
	}}}}
	editor := testEditor(t, "- value\n", se.WithSchema(schema.Node{Kind: schema.ArrayKind, Array: schema.ArraySchema{Item: &item}}), se.WithPolicy(se.Policy{Default: se.ScopePolicy{Editable: se.Allow}}))
	m := NewModel(editor, WithColors(false))
	first := m.visibleRows()
	if second := m.visibleRows(); &first[0] != &second[0] {
		t.Fatal("rows not cached")
	}
	m.expanded[editor.Document().Root()] = false
	if len(m.visibleRows()) != 1 {
		t.Fatal("fold not reflected")
	}
	m.expanded[editor.Document().Root()] = true
	items, _ := editor.Document().SequenceItems(editor.Document().Root())
	if _, err := editor.Apply(se.SetValue{NodeID: items[0], Value: "edited"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.visibleRows()[1].valueText, "edited") {
		t.Fatal("stale edited value")
	}
	invalid = true
	editor.Validate()
	if !strings.Contains(m.visibleRows()[1].issueText, "changed") {
		t.Fatal("stale validation")
	}
}

func TestAliasProjectionIsBounded(t *testing.T) {
	editor := testEditor(t, "a: &a [1, 2]\nb: &b [*a, *a]\nc: [*b, *b]\n")
	m := NewModel(editor, WithAliasExpansion(true), WithAliasRowLimit(4))
	rows := m.visibleRows()
	projected, limited := 0, false
	for _, row := range rows {
		if strings.Contains(row.label, "read-only") {
			projected++
		}
		if strings.Contains(row.label, "alias expansion limited") {
			limited = true
		}
	}
	if projected > 4 || !limited {
		t.Fatalf("projected=%d limited=%v", projected, limited)
	}
}

func TestSaveConflictReloadFailureAndCancelPreserveEdits(t *testing.T) {
	editor := testEditor(t, "before\n", se.WithPolicy(se.Policy{Default: se.ScopePolicy{Editable: se.Allow}}))
	if _, err := editor.Apply(se.SetValue{NodeID: editor.Document().Root(), Value: "edited"}); err != nil {
		t.Fatal(err)
	}
	m := NewModel(editor)
	m.SetSaveHandler(func() error { return ErrSaveConflict })
	m.SetSaveConflictHandler(func(SaveConflictAction) (*se.Editor, error) { return nil, errors.New("reload failed") })
	m.save()
	if m.mode != saveConflictMode {
		t.Fatal("conflict prompt not opened")
	}
	m.resolveSaveConflict(SaveConflictReload)
	if m.mode != saveConflictMode || m.Editor() != editor || !editor.IsDirty() {
		t.Fatal("failed reload lost state")
	}
	m.cancelSaveConflict()
	if m.mode != browseMode || !editor.IsDirty() {
		t.Fatal("cancel lost edits")
	}
	m.save()
	fresh := testEditor(t, "external\n")
	m.SetSaveConflictHandler(func(SaveConflictAction) (*se.Editor, error) { return fresh, nil })
	m.resolveSaveConflict(SaveConflictReload)
	if m.Editor() != fresh || m.mode != browseMode || fresh.IsDirty() {
		t.Fatal("reload did not replace editor")
	}
}

func BenchmarkViewCached(b *testing.B) {
	var source strings.Builder
	for range 2048 {
		source.WriteString("- 日本語\n")
	}
	m := NewModel(testEditor(b, source.String()), WithColors(false))
	m.View()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		m.View()
	}
}
