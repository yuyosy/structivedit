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
	var appendNode func(document.NodeID, int, string, bool, map[document.NodeID]bool)
	appendNode = func(id document.NodeID, depth int, label string, readOnly bool, ancestors map[document.NodeID]bool) {
		if id == 0 || ancestors[id] {
			return
		}
		view, ok := byID[id]
		if !ok {
			return
		}
		children := visibleChildren(view)
		var aliasTarget structivedit.NodeView
		projected := false
		if len(children) == 0 && model.expandAliases && view.Kind == document.NodeReference && view.HasReferenceTarget && !ancestors[view.ReferenceTarget] {
			if target, exists := byID[view.ReferenceTarget]; exists {
				aliasTarget = target
				children = visibleChildren(target)
				projected = len(children) > 0
			}
		}
		rowID := id
		if readOnly {
			rowID = 0
			label += " (read-only)"
		}
		row := treeRow{
			nodeID:    rowID,
			depth:     depth,
			label:     label,
			focusable: !readOnly,
			hasChild:  len(children) > 0,
			expanded:  readOnly || model.expanded[id],
			valueText: rowValue(model.editor.Document(), view),
			issueText: rowIssue(view.Issues),
		}
		rows = append(rows, row)
		if !row.hasChild || !row.expanded {
			return
		}
		ancestors[id] = true
		defer delete(ancestors, id)
		if view.Kind == document.NodeSequence {
			for index, child := range children {
				appendNode(child, depth+1, fmt.Sprintf("[%d]", index), readOnly, ancestors)
			}
			return
		}
		entries := view.MappingEntries
		if projected {
			if ancestors[aliasTarget.ID] {
				return
			}
			ancestors[aliasTarget.ID] = true
			defer delete(ancestors, aliasTarget.ID)
			if aliasTarget.Kind == document.NodeSequence {
				for index, child := range aliasTarget.SequenceItems {
					appendNode(child, depth+1, fmt.Sprintf("[%d]", index), true, ancestors)
				}
				return
			}
			entries = aliasTarget.MappingEntries
		}
		for _, entry := range entries {
			key, keyExists := byID[entry.Key]
			label := "<key>"
			if keyExists {
				label = mappingKeyLabel(key)
			}
			appendNode(entry.Value, depth+1, label, readOnly || projected, ancestors)
		}
	}
	root := model.editor.Document().Root()
	appendNode(root, 0, "$", false, make(map[document.NodeID]bool))
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
	stateStyle := model.styles.saved
	if model.editor.IsDirty() {
		state = "Modified"
		stateStyle = model.styles.modified
	}
	issueCountText := fmt.Sprintf("%d issues", issueCount)
	issueCountStyle := model.styles.muted
	if issueCount > 0 {
		issueCountStyle = model.styles.error
	}
	lines = append(lines, model.renderStyledLine(width,
		lineSegment{text: "StructiveEdit", style: model.styles.header},
		lineSegment{text: " | ", style: model.styles.plain},
		lineSegment{text: state, style: stateStyle},
		lineSegment{text: fmt.Sprintf(" | %d visible rows | ", len(rows)), style: model.styles.muted},
		lineSegment{text: issueCountText, style: issueCountStyle},
	))
	model.hitRegions = model.hitRegions[:0]
	focused, hasFocus := model.editor.Focused()
	for index := start; index < end; index++ {
		row := rows[index]
		cursor := " "
		cursorStyle := model.styles.plain
		labelStyle := model.styles.key
		valueStyle := model.valueStyle(row.valueText)
		if row.focusable && hasFocus && row.nodeID == focused {
			cursor = ">"
			cursorStyle = model.styles.cursor
			labelStyle = model.styles.focus
		}
		if !row.focusable {
			labelStyle = model.styles.readOnly
			valueStyle = model.styles.readOnly
		}
		treeMark := "  "
		if row.hasChild {
			if row.focusable {
				if row.expanded {
					treeMark = "- "
				} else {
					treeMark = "+ "
				}
			}
		}
		segments := []lineSegment{
			{text: cursor + strings.Repeat("  ", row.depth), style: cursorStyle},
			{text: treeMark, style: model.styles.tree},
			{text: row.label, style: labelStyle},
			{text: ": ", style: model.styles.plain},
			{text: row.valueText, style: valueStyle},
		}
		if row.issueText != "" {
			segments = append(segments,
				lineSegment{text: "  ", style: model.styles.plain},
				lineSegment{text: row.issueText, style: model.issueStyle(row.issueText)},
			)
		}
		lines = append(lines, model.renderStyledLine(width, segments...))
		if row.focusable {
			model.hitRegions = append(model.hitRegions, hitRegion{line: len(lines) - 1, nodeID: row.nodeID})
		}
	}
	for len(lines)-1 < count {
		lines = append(lines, "")
	}
	lines = append(lines, model.renderStyledLine(width,
		lineSegment{text: model.promptLine(), style: model.styles.prompt},
	))
	footer := model.message
	if footer == "" {
		footer = "↑/↓ move · ←/→ fold · Enter edit · Space toggle · a add · d delete · Ctrl+S save · q quit"
	}
	lines = append(lines, model.renderStyledLine(width,
		lineSegment{text: footer, style: model.styles.muted},
	))
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
