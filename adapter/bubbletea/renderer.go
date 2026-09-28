package bubbletea

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
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
	lines := make([]string, 0, end-start+5)
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
	cursorModeText := "Row head"
	if model.cursorMode == valueCellCursor {
		cursorModeText = "Value cell"
	}
	lines = append(lines, model.renderStyledLine(width,
		lineSegment{text: "StructiveEdit", style: model.styles.header},
		lineSegment{text: " | ", style: model.styles.plain},
		lineSegment{text: state, style: stateStyle},
		lineSegment{text: fmt.Sprintf(" | Cursor: %s | %d visible rows | ", cursorModeText, len(rows)), style: model.styles.muted},
		lineSegment{text: issueCountText, style: issueCountStyle},
	))
	lines = append(lines, model.renderStyledLine(width,
		lineSegment{text: strings.Repeat("─", width), style: model.styles.muted},
	))
	model.hitRegions = model.hitRegions[:0]
	focused, hasFocus := model.editor.Focused()
	for index := start; index < end; index++ {
		row := rows[index]
		cursor := " "
		cursorStyle := model.styles.plain
		valueCursor := ""
		labelStyle := model.styles.key
		valueStyle := model.valueStyle(row.valueText)
		if row.focusable && hasFocus && row.nodeID == focused {
			labelStyle = model.styles.focus
			if model.cursorMode == valueCellCursor {
				valueCursor = "> "
				valueStyle = valueStyle.Underline(true).Bold(true)
			} else {
				cursor = ">"
				cursorStyle = model.styles.cursor
			}
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
			{text: valueCursor, style: model.styles.cursor},
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
	for len(lines)-2 < count {
		lines = append(lines, "")
	}
	for _, inputLine := range model.inputAreaLines(width) {
		lines = append(lines, model.renderStyledLine(width, inputLine...))
	}
	lines = append(lines, model.renderStyledLine(width, model.keyboardHelpSegments(0)...))
	lines = append(lines, model.renderStyledLine(width, model.keyboardHelpSegments(1)...))
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (model *Model) inputAreaLines(width int) [][]lineSegment {
	switch model.mode {
	case editMode:
		context := []lineSegment{
			{text: "", style: model.styles.inputLabel},
			{text: model.editPath(), style: model.styles.inputPath},
		}
		return model.inputPromptLines(width, context, model.edit.value)
	case addFieldMode:
		context := []lineSegment{{text: " Add mapping field", style: model.styles.inputLabel}}
		return model.inputPromptLines(width, context, model.add.field)
	case addValueMode:
		if model.add.inputAt < 0 || model.add.inputAt >= len(model.add.plan.Inputs) {
			return [][]lineSegment{nil, model.fullWidthLine(width, nil, model.styles.inputArea)}
		}
		request := model.add.plan.Inputs[model.add.inputAt]
		optional := "required"
		if request.HasDefault {
			optional = "default " + defaultInputText(request.Default)
		}
		context := []lineSegment{{
			text:  fmt.Sprintf(" Add %s (%s, %d/%d, %s)", request.Label, schemaKindName(request.ExpectedKind), model.add.inputAt+1, len(model.add.plan.Inputs), optional),
			style: model.styles.inputLabel,
		}}
		return model.inputPromptLines(width, context, model.add.input)
	case deleteConfirmMode:
		choice := "[Cancel]  Confirm"
		if model.delete.confirm {
			choice = "Cancel  [Confirm]"
		}
		segments := []lineSegment{
			{text: " Delete ", style: model.styles.inputLabel},
			{text: model.currentDeletePath(), style: model.styles.inputPath},
			{text: "?  " + choice + "  (Tab changes, Enter selects, Esc cancels)", style: model.styles.inputLabel},
		}
		if model.message != "" {
			segments = append(segments, lineSegment{text: "  " + model.message, style: model.styles.inputError})
		}
		return [][]lineSegment{model.fullWidthLine(width, segments, model.styles.contextArea)}
	default:
		if model.message == "" {
			return [][]lineSegment{nil}
		}
		return [][]lineSegment{model.fullWidthLine(width,
			[]lineSegment{{text: " " + model.message, style: model.styles.inputLabel}},
			model.styles.contextArea,
		)}
	}
}

func (model *Model) inputPromptLines(width int, context []lineSegment, input textInput) [][]lineSegment {
	if model.message != "" {
		context = append(context, lineSegment{text: "  " + model.message, style: model.styles.inputError})
	}
	context = append([]lineSegment{{text: " ", style: model.styles.contextArea}}, context...)
	contextLine := model.fullWidthLine(width, context, model.styles.contextArea)
	inputLine := model.inputValueLine(width, input)
	return [][]lineSegment{contextLine, inputLine}
}

func (model *Model) inputValueLine(width int, input textInput) []lineSegment {
	before, after := inputWindow(input, width-2)
	segments := []lineSegment{{text: " ", style: model.styles.inputArea}}
	if before != "" {
		segments = append(segments, lineSegment{text: before, style: model.styles.inputText})
	}
	segments = append(segments, lineSegment{text: "|", style: model.styles.inputCursor})
	if after != "" {
		segments = append(segments, lineSegment{text: after, style: model.styles.inputText})
	}
	return model.fullWidthLine(width, segments, model.styles.inputArea)
}

func (model *Model) fullWidthLine(width int, segments []lineSegment, background lipgloss.Style) []lineSegment {
	if width <= 0 {
		return nil
	}
	length := 0
	for _, segment := range segments {
		length += len([]rune(segment.text))
	}
	limit := width
	truncated := length > width
	if truncated {
		limit--
	}
	result := make([]lineSegment, 0, len(segments)+1)
	remaining := limit
	for _, segment := range segments {
		if remaining == 0 {
			break
		}
		runes := []rune(segment.text)
		if len(runes) > remaining {
			runes = runes[:remaining]
		}
		if len(runes) > 0 {
			result = append(result, lineSegment{text: string(runes), style: segment.style})
			remaining -= len(runes)
		}
	}
	if truncated {
		result = append(result, lineSegment{text: "…", style: background})
	} else if remaining > 0 {
		result = append(result, lineSegment{text: strings.Repeat(" ", remaining), style: background})
	}
	return result
}

func inputWindow(input textInput, width int) (string, string) {
	runes := input.runes
	position := input.cursor
	if position < 0 {
		position = 0
	}
	if position > len(runes) {
		position = len(runes)
	}
	if width <= 0 || len(runes) <= width {
		return string(runes[:position]), string(runes[position:])
	}
	start := position - width/2
	if start < 0 {
		start = 0
	}
	end := start + width
	if end > len(runes) {
		end = len(runes)
		start = end - width
	}
	for end-start+(boolInt(start > 0))+boolInt(end < len(runes)) > width {
		if end > position {
			end--
		} else if start < position {
			start++
		} else {
			break
		}
	}
	before := string(runes[start:position])
	after := string(runes[position:end])
	if start > 0 {
		before = "…" + before
	}
	if end < len(runes) {
		after += "…"
	}
	return before, after
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
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
		return readablePath(model.editor.Document(), view.Path)
	}
	return fmt.Sprintf("node %d", model.edit.nodeID)
}

func readablePath(doc *document.Document, path document.Path) string {
	if doc == nil {
		return path.String()
	}
	current := doc.Root()
	result := "$"
	for _, segment := range path.Segments() {
		switch typed := segment.(type) {
		case document.SequenceIndexSegment:
			items, ok := doc.SequenceItems(current)
			if !ok || typed.Index() >= len(items) {
				return path.String()
			}
			result += fmt.Sprintf("[%d]", typed.Index())
			current = items[typed.Index()]
		case document.MappingEntrySegment:
			entries, ok := doc.MappingEntries(current)
			if !ok || typed.EntryIndex() >= len(entries) {
				return path.String()
			}
			entry := entries[typed.EntryIndex()]
			switch typed.Role() {
			case document.MappingValueRole:
				_, suffix := mappingKeyPathLabel(doc, entry.Key, typed.EntryIndex())
				result += suffix
				current = entry.Value
			case document.MappingKeyRole:
				key, _ := mappingKeyPathLabel(doc, entry.Key, typed.EntryIndex())
				result += "[mapping key " + key + "]"
				current = entry.Key
			default:
				return path.String()
			}
		default:
			return path.String()
		}
	}
	return result
}

func mappingKeyPathLabel(doc *document.Document, keyID document.NodeID, index int) (string, string) {
	node, ok := doc.Node(keyID)
	if !ok {
		fallback := fmt.Sprintf("key #%d", index)
		return fallback, "[" + fallback + "]"
	}
	value, scalar := node.ScalarValue()
	if !scalar {
		fallback := fmt.Sprintf("key #%d", index)
		return fallback, "[" + fallback + "]"
	}
	text, isString := value.(string)
	if isString && isPathIdentifier(text) {
		return text, "." + text
	}
	label := formatScalarValue(value)
	return label, "[" + label + "]"
}

func isPathIdentifier(value string) bool {
	runes := []rune(value)
	if len(runes) == 0 || !(runes[0] == '_' || runes[0] >= 'A' && runes[0] <= 'Z' || runes[0] >= 'a' && runes[0] <= 'z') {
		return false
	}
	for _, char := range runes[1:] {
		if char == '_' || char == '-' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			continue
		}
		return false
	}
	return true
}

func (model *Model) keyboardHelpSegments(line int) []lineSegment {
	shortcuts := [][][2]string{
		{
			{"↑/↓", " move"},
			{"←/→", " fold"},
			{"Enter", " edit"},
			{"Tab", " cursor"},
			{"Space", " toggle"},
			{"Alt+↑/↓", " reorder"},
		},
		{
			{"Ctrl+Z", " undo"},
			{"Ctrl+Y", " redo"},
			{"Ctrl+S", " save"},
			{"a", " add"},
			{"d", " delete"},
			{"Esc/q", " cancel/quit"},
		},
	}
	if line < 0 || line >= len(shortcuts) {
		return nil
	}
	segments := make([]lineSegment, 0, len(shortcuts[line])*3)
	for index, shortcut := range shortcuts[line] {
		if index > 0 {
			segments = append(segments, lineSegment{text: "  ", style: model.styles.muted})
		}
		segments = append(segments,
			lineSegment{text: shortcut[0], style: model.styles.shortcutKey},
			lineSegment{text: shortcut[1], style: model.styles.muted},
		)
	}
	return segments
}
