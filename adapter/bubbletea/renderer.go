package bubbletea

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/yuyosy/structivedit"
	"github.com/yuyosy/structivedit/document"
	"github.com/yuyosy/structivedit/schema"
)

func (model *Model) visibleRows() []treeRow {
	if model == nil || model.editor == nil || model.editor.Document() == nil {
		return nil
	}
	views := model.editor.Views()
	byID := make(map[document.NodeID]structivedit.NodeView, len(views))
	for _, view := range views {
		byID[view.ID] = view
	}
	rows := make([]treeRow, 0, len(views))
	var appendNode func(document.NodeID, int, string)
	appendNode = func(id document.NodeID, depth int, label string) {
		view, ok := byID[id]
		if !ok {
			return
		}
		children := visibleChildren(view)
		row := treeRow{
			nodeID:    id,
			depth:     depth,
			label:     label,
			hasChild:  len(children) > 0,
			expanded:  model.expanded[id],
			valueText: rowValue(model.editor.Document(), view),
			issueText: rowIssue(view.Issues),
		}
		rows = append(rows, row)
		if !row.hasChild || !row.expanded {
			return
		}
		if view.Kind == document.NodeSequence {
			for index, child := range children {
				appendNode(child, depth+1, fmt.Sprintf("[%d]", index))
			}
			return
		}
		for _, entry := range view.MappingEntries {
			key, keyExists := byID[entry.Key]
			label := "<key>"
			if keyExists {
				label = mappingKeyLabel(key)
			}
			appendNode(entry.Value, depth+1, label)
		}
	}
	root := model.editor.Document().Root()
	appendNode(root, 0, "$")
	return rows
}

func rowValue(doc *document.Document, view structivedit.NodeView) string {
	switch view.Kind {
	case document.NodeScalar:
		kind, _, _ := doc.Scalar(view.ID)
		return "[" + scalarKindName(kind) + "] " + formatScalarValue(view.ScalarValue)
	case document.NodeSequence:
		return fmt.Sprintf("[seq] %d items", len(view.SequenceItems))
	case document.NodeMapping:
		return fmt.Sprintf("[map] %d entries", len(view.MappingEntries))
	case document.NodeReference:
		if !view.HasReferenceTarget {
			return "-> <missing target>"
		}
		path, err := doc.Path(view.ReferenceTarget)
		if err != nil {
			return "-> <invalid target>"
		}
		value := "[ref] -> " + path.String()
		if referenceCycle(doc, view.ID, view.ReferenceTarget) {
			value += " (cycle)"
		}
		return value
	default:
		return ""
	}
}

func formatScalarValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case string:
		return strconv.Quote(typed)
	case bool:
		return strconv.FormatBool(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		if math.IsNaN(typed) {
			if math.Signbit(typed) {
				return "-.nan"
			}
			return ".nan"
		}
		if math.IsInf(typed, 1) {
			return ".inf"
		}
		if math.IsInf(typed, -1) {
			return "-.inf"
		}
		return strconv.FormatFloat(typed, 'g', -1, 64)
	default:
		return fmt.Sprint(value)
	}
}

func scalarKindName(kind document.ScalarKind) string {
	switch kind {
	case document.ScalarString:
		return "str"
	case document.ScalarBool:
		return "bool"
	case document.ScalarInteger:
		return "int"
	case document.ScalarFloat:
		return "float"
	case document.ScalarNull:
		return "null"
	default:
		return "?"
	}
}

func mappingKeyLabel(view structivedit.NodeView) string {
	if view.Kind == document.NodeScalar {
		if text, ok := view.ScalarValue.(string); ok {
			if text == "" || strings.ContainsAny(text, "\r\n\t") {
				return strconv.Quote(text)
			}
			return text
		}
		return "[" + formatScalarValue(view.ScalarValue) + "]"
	}
	if view.Kind == document.NodeSequence {
		return fmt.Sprintf("[sequence key: %d items]", len(view.SequenceItems))
	}
	if view.Kind == document.NodeMapping {
		return fmt.Sprintf("[mapping key: %d entries]", len(view.MappingEntries))
	}
	return "[reference key]"
}

func referenceCycle(doc *document.Document, referenceID, targetID document.NodeID) bool {
	for current := referenceID; current != 0; {
		if current == targetID {
			return true
		}
		parent, ok := doc.Parent(current)
		if !ok {
			return false
		}
		current = parent.Parent
	}
	return false
}

func rowIssue(issues []structivedit.ValidationIssue) string {
	if len(issues) == 0 {
		return ""
	}
	marker := "[i]"
	switch issues[0].Severity {
	case schema.SeverityError:
		marker = "[E]"
	case schema.SeverityWarning:
		marker = "[W]"
	}
	message := marker + " " + issues[0].Message
	if len(issues) > 1 {
		message += fmt.Sprintf(" (+%d)", len(issues)-1)
	}
	return message
}

func (model *Model) render() tea.View {
	if model == nil || model.editor == nil || model.editor.Document() == nil {
		view := tea.NewView("Editor unavailable\nPress q to quit")
		view.AltScreen = true
		return view
	}
	rows := model.visibleRows()
	model.keepFocusVisible()
	count := model.visibleRowCount()
	start := model.viewport.offset
	if start < 0 {
		start = 0
	}
	if start > len(rows) {
		start = len(rows)
	}
	end := start + count
	if end > len(rows) {
		end = len(rows)
	}
	width := model.width
	if width < 20 {
		width = 20
	}
	lines := make([]string, 0, end-start+3)
	issueCount := len(model.editor.Issues())
	state := "Saved"
	if model.editor.IsDirty() {
		state = "Modified"
	}
	lines = append(lines, clipLine(fmt.Sprintf("StructiveEdit | %s | %d visible nodes | %d issues", state, len(rows), issueCount), width))
	model.hitRegions = model.hitRegions[:0]
	focused, hasFocus := model.editor.Focused()
	for index := start; index < end; index++ {
		row := rows[index]
		cursor := " "
		if hasFocus && row.nodeID == focused {
			cursor = ">"
		}
		treeMark := "  "
		if row.hasChild {
			if row.expanded {
				treeMark = "- "
			} else {
				treeMark = "+ "
			}
		}
		line := cursor + strings.Repeat("  ", row.depth) + treeMark + row.label + ": " + row.valueText
		if row.issueText != "" {
			line += "  " + row.issueText
		}
		lines = append(lines, clipLine(line, width))
		model.hitRegions = append(model.hitRegions, hitRegion{line: len(lines) - 1, nodeID: row.nodeID})
	}
	for len(lines)-1 < count {
		lines = append(lines, "")
	}
	lines = append(lines, clipLine(model.promptLine(), width))
	footer := model.message
	if footer == "" {
		footer = "↑/↓ move · ←/→ fold · Enter edit · Space toggle · a add · d delete · Ctrl+S save · q quit"
	}
	lines = append(lines, clipLine(footer, width))
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (model *Model) promptLine() string {
	switch model.mode {
	case editMode:
		return fmt.Sprintf("Edit %s > %s", model.editPath(), inputWithCursor(model.edit.value))
	case addFieldMode:
		return "Add mapping field > " + inputWithCursor(model.add.field)
	case addValueMode:
		if model.add.inputAt < 0 || model.add.inputAt >= len(model.add.plan.Inputs) {
			return "Add values"
		}
		request := model.add.plan.Inputs[model.add.inputAt]
		optional := "required"
		if request.HasDefault {
			optional = "default " + defaultInputText(request.Default)
		}
		return fmt.Sprintf("Add %s (%s, %d/%d, %s) > %s", request.Label, schemaKindName(request.ExpectedKind), model.add.inputAt+1, len(model.add.plan.Inputs), optional, inputWithCursor(model.add.input))
	case deleteConfirmMode:
		choice := "[Cancel]  Confirm"
		if model.delete.confirm {
			choice = "Cancel  [Confirm]"
		}
		return fmt.Sprintf("Delete %s?  %s  (Tab changes, Enter selects, Esc cancels)", model.currentDeletePath(), choice)
	default:
		return ""
	}
}

func schemaKindName(kind schema.Kind) string {
	switch kind {
	case schema.StringKind:
		return "string"
	case schema.BoolKind:
		return "bool"
	case schema.IntegerKind:
		return "integer"
	case schema.FloatKind:
		return "float"
	case schema.NullKind:
		return "null"
	default:
		return "unsupported"
	}
}

func (model *Model) editPath() string {
	if view, err := model.editor.View(model.edit.nodeID); err == nil {
		return view.Path.String()
	}
	return fmt.Sprintf("node %d", model.edit.nodeID)
}

func inputWithCursor(input textInput) string {
	value := input.String()
	runes := []rune(value)
	position := input.cursor
	if position < 0 {
		position = 0
	}
	if position > len(runes) {
		position = len(runes)
	}
	return string(runes[:position]) + "|" + string(runes[position:])
}

func clipLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(line)
	if len(runes) <= width {
		return line
	}
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}
