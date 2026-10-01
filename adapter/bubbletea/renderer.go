package bubbletea

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/yuyosy/structivedit"
	"github.com/yuyosy/structivedit/document"
	"github.com/yuyosy/structivedit/schema"
)

const multilineEditorMaxVisibleLines = 5

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
	model.inputCursorX = -1
	model.inputCursorY = -1
	model.inputCursorInTree = false
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
	width := model.layoutWidth()
	helpLines := model.keyboardHelpLines(width)
	lines := make([]string, 0, end-start+3+model.inputAreaLineCount()+len(helpLines))
	issueCount := len(model.editor.Issues())
	state := "Saved"
	stateStyle := model.styles.saved
	if model.editor.IsDirty() {
		state = "Modified"
		stateStyle = model.styles.modified
	}
	issueLabel := "issues"
	if issueCount == 1 {
		issueLabel = "issue"
	}
	issueCountText := fmt.Sprintf("%d %s", issueCount, issueLabel)
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
		lineSegment{text: fmt.Sprintf("Cursor: %s | %d visible rows", cursorModeText, len(rows)), style: model.styles.muted},
	))
	context := model.statusContext()
	contextStyle := model.styles.muted
	if model.message != "" && model.message != "Saved" {
		contextStyle = model.styles.info
	}
	if model.messageError {
		contextStyle = model.styles.error
	}
	statusSegments := []lineSegment{
		{text: "[" + state + "]", style: stateStyle},
		{text: " | ", style: model.styles.muted},
		{text: issueCountText, style: issueCountStyle},
		{text: " | ", style: model.styles.muted},
		{text: context, style: contextStyle},
	}
	lines = append(lines, model.renderStyledLine(width, statusSegments...))
	lines = append(lines, model.renderStyledLine(width,
		lineSegment{text: strings.Repeat("─", width), style: model.styles.muted},
	))
	model.hitRegions = model.hitRegions[:0]
	focused, hasFocus := model.editor.Focused()
	for index := start; index < end; index++ {
		row := rows[index]
		inlineEditing := model.inlineEditing && model.mode == editMode && !model.edit.multiline && row.nodeID == model.edit.nodeID
		cursor := " "
		cursorStyle := model.styles.plain
		valueCursor := ""
		labelStyle := model.styles.key
		valueStyle := model.valueStyle(row.valueText)
		if row.focusable && hasFocus && row.nodeID == focused {
			labelStyle = model.styles.focus
			if !inlineEditing {
				if model.cursorMode == valueCellCursor {
					valueCursor = "> "
					valueStyle = valueStyle.Underline(true).Bold(true)
				} else {
					cursor = ">"
					cursorStyle = model.styles.cursor
				}
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
		}
		if inlineEditing {
			prefixWidth := lineSegmentWidth(segments)
			maxPrefixWidth := width - 8
			if prefixWidth > maxPrefixWidth {
				segments = clipLineSegments(segments, maxPrefixWidth, model.styles.muted)
				prefixWidth = lineSegmentWidth(segments)
			}
			inputSegments, cursorOffset := model.inlineInputSegments(model.edit.value, width-prefixWidth)
			model.inputCursorX = prefixWidth + cursorOffset
			model.inputCursorY = len(lines)
			model.inputCursorInTree = true
			segments = append(segments, inputSegments...)
		} else {
			segments = append(segments,
				lineSegment{text: valueCursor, style: model.styles.cursor},
				lineSegment{text: row.valueText, style: valueStyle},
			)
		}
		if row.issueText != "" && !inlineEditing {
			segments = append(segments,
				lineSegment{text: "  ", style: model.styles.plain},
				lineSegment{text: row.issueText, style: model.issueStyle(row.issueText)},
			)
		}
		lines = append(lines, model.renderStyledLine(width, segments...))
		if row.focusable {
			region := hitRegion{line: len(lines) - 1, nodeID: row.nodeID, foldable: row.hasChild}
			if row.hasChild {
				region.foldStart = 1 + 2*row.depth
				region.foldEnd = region.foldStart + 2
			}
			model.hitRegions = append(model.hitRegions, region)
		}
	}
	for len(lines)-3 < count {
		lines = append(lines, "")
	}
	inputAreaTop := len(lines)
	for _, inputLine := range model.inputAreaLines(width) {
		lines = append(lines, model.renderStyledLine(width, inputLine...))
	}
	if model.inputCursorX >= 0 && model.inputCursorY >= 0 && !model.inputCursorInTree {
		model.inputCursorY += inputAreaTop
	}
	for _, helpLine := range helpLines {
		lines = append(lines, model.renderStyledLine(width, helpLine...))
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	if model.inputCursorX >= 0 && model.inputCursorY >= 0 {
		cursor := tea.NewCursor(model.inputCursorX, model.inputCursorY)
		cursor.Shape = tea.CursorBar
		cursor.Blink = false
		if model.colorsActive() {
			cursor.Color = color.RGBA{R: 255, G: 215, B: 0, A: 255}
		}
		view.Cursor = cursor
	}
	return view
}

func (model *Model) statusContext() string {
	context := "Ready"
	switch model.mode {
	case editMode:
		context = "Editing"
		if model.edit.multiline {
			context = "Editing multiline"
		}
		if model.inlineEditing {
			context = "Editing inline"
		}
	case addFieldMode:
		context = "Adding field"
	case addValueMode:
		context = "Adding value"
	case deleteConfirmMode:
		context = "Confirm delete"
	case saveConflictMode:
		context = "Resolve file change"
	}
	if model.message == "" || model.message == "Saved" {
		return context
	}
	if model.mode == browseMode {
		return model.message
	}
	return context + " · " + model.message
}

func (model *Model) inputAreaLines(width int) [][]lineSegment {
	switch model.mode {
	case editMode:
		label := " Edit "
		if model.inlineEditing {
			label = " Editing inline "
		}
		if model.edit.multiline {
			label = " Edit multiline "
		}
		context := []lineSegment{
			{text: label, style: model.styles.inputLabel},
			{text: model.editPath(), style: model.styles.inputPath},
		}
		if model.edit.multiline {
			lines := model.edit.value.lines()
			start := model.edit.viewportLine
			if start < 0 {
				start = 0
			}
			if start >= len(lines) {
				start = len(lines) - 1
			}
			end := start + min(multilineEditorMaxVisibleLines, len(lines)-start)
			context = append(context, lineSegment{
				text:  fmt.Sprintf("  lines %d-%d/%d", start+1, end, len(lines)),
				style: model.styles.inputLabel,
			})
		} else if model.inlineEditing {
			return [][]lineSegment{model.fullWidthLine(width, append([]lineSegment{{text: " ", style: model.styles.contextArea}}, context...), model.styles.contextArea)}
		}
		return model.inputPromptLines(width, context, model.edit.value, model.edit.multiline)
	case addFieldMode:
		context := []lineSegment{{text: " Add mapping field", style: model.styles.inputLabel}}
		return model.inputPromptLines(width, context, model.add.field, false)
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
		return model.inputPromptLines(width, context, model.add.input, false)
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
		return [][]lineSegment{model.fullWidthLine(width, segments, model.styles.contextArea)}
	case saveConflictMode:
		segments := []lineSegment{
			{text: " File changed outside the editor. ", style: model.styles.inputLabel},
			{text: "[o] Overwrite  [r] Reload and discard  [Esc] Cancel", style: model.styles.inputPath},
		}
		return [][]lineSegment{model.fullWidthLine(width, segments, model.styles.contextArea)}
	default:
		return nil
	}
}

func (model *Model) inputPromptLines(width int, context []lineSegment, input textInput, multiline bool) [][]lineSegment {
	context = append([]lineSegment{{text: " ", style: model.styles.contextArea}}, context...)
	contextLine := model.fullWidthLine(width, context, model.styles.contextArea)
	if multiline {
		lines := input.lines()
		cursorLine, cursorColumn := input.cursorLineColumn(lines)
		start := model.edit.viewportLine
		if start < 0 {
			start = 0
		}
		if start >= len(lines) {
			start = len(lines) - 1
		}
		end := start + min(multilineEditorMaxVisibleLines, len(lines)-start)
		result := [][]lineSegment{contextLine}
		for lineIndex := start; lineIndex < end; lineIndex++ {
			line := lines[lineIndex]
			lineRunes := input.runes[line.start:line.end]
			column := 0
			active := lineIndex == cursorLine
			if active {
				column = cursorColumn
			}
			lineSegments, cursorX := model.multilineInputLine(width, lineRunes, column, active)
			result = append(result, lineSegments)
			if active {
				model.inputCursorX = cursorX
				model.inputCursorY = len(result) - 1
			}
		}
		return result
	}
	inputLine, cursorX := model.inputValueLine(width, input)
	model.inputCursorX = cursorX
	model.inputCursorY = 1
	return [][]lineSegment{contextLine, inputLine}
}

func (model *Model) multilineInputLine(width int, runes []rune, cursor int, active bool) ([]lineSegment, int) {
	segments := []lineSegment{{text: " ", style: model.styles.inputArea}}
	if width <= 1 {
		return segments, 0
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	if !active {
		text := inputPrefixWindow(displayInputText(runes), width-1)
		segments = append(segments, lineSegment{text: text, style: model.styles.inputText})
		return padInputLine(segments, width, model.styles.inputArea), -1
	}
	before := displayInputText(runes[:cursor])
	after := displayInputText(runes[cursor:])
	textWidth := width - 2 // one leading cell and the cursor marker
	leftBudget := (textWidth + 1) / 2
	rightBudget := textWidth - leftBudget
	left := inputSuffixWindow(before, leftBudget)
	right := inputPrefixWindow(after, rightBudget)
	remaining := textWidth - lipgloss.Width(left) - lipgloss.Width(right)
	if remaining > 0 {
		right = inputPrefixWindow(after, rightBudget+remaining)
		remaining = textWidth - lipgloss.Width(left) - lipgloss.Width(right)
	}
	if remaining > 0 {
		left = inputSuffixWindow(before, leftBudget+remaining)
	}
	if left != "" {
		segments = append(segments, lineSegment{text: left, style: model.styles.inputText})
	}
	if right != "" {
		segments = append(segments, lineSegment{text: right, style: model.styles.inputText})
	}
	return padInputLine(segments, width, model.styles.inputArea), 1 + lipgloss.Width(left)
}

func inputSuffixWindow(value string, width int) string {
	if width <= 0 || value == "" {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	runes := []rune(value)
	start := len(runes)
	for start > 0 {
		candidate := string(runes[start-1:])
		if lipgloss.Width("…"+candidate) > width {
			break
		}
		start--
	}
	return "…" + string(runes[start:])
}

func inputPrefixWindow(value string, width int) string {
	if width <= 0 || value == "" {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	runes := []rune(value)
	end := 0
	for end < len(runes) {
		candidate := string(runes[:end+1])
		if lipgloss.Width(candidate+"…") > width {
			break
		}
		end++
	}
	return string(runes[:end]) + "…"
}

func padInputLine(segments []lineSegment, width int, background lipgloss.Style) []lineSegment {
	used := 0
	for _, segment := range segments {
		used += lipgloss.Width(segment.text)
	}
	if used < width {
		segments = append(segments, lineSegment{text: strings.Repeat(" ", width-used), style: background})
	}
	return segments
}

func (model *Model) inputValueLine(width int, input textInput) ([]lineSegment, int) {
	before, after := inputWindow(input, width-2)
	segments := []lineSegment{{text: " ", style: model.styles.inputArea}}
	if before != "" {
		segments = append(segments, lineSegment{text: before, style: model.styles.inputText})
	}
	if after != "" {
		segments = append(segments, lineSegment{text: after, style: model.styles.inputText})
	}
	return model.fullWidthLine(width, segments, model.styles.inputArea), 1 + lipgloss.Width(before)
}

func (model *Model) inlineInputSegments(input textInput, width int) ([]lineSegment, int) {
	if width <= 0 {
		return nil, 0
	}
	before, after := inputWindow(input, width-1)
	segments := make([]lineSegment, 0, 3)
	if before != "" {
		segments = append(segments, lineSegment{text: before, style: model.styles.inputText})
	}
	if after != "" {
		segments = append(segments, lineSegment{text: after, style: model.styles.inputText})
	}
	return model.fullWidthLine(width, segments, model.styles.inputArea), lipgloss.Width(before)
}

func lineSegmentWidth(segments []lineSegment) int {
	width := 0
	for _, segment := range segments {
		width += len([]rune(segment.text))
	}
	return width
}

func clipLineSegments(segments []lineSegment, width int, ellipsisStyle lipgloss.Style) []lineSegment {
	if width <= 0 {
		return nil
	}
	if lineSegmentWidth(segments) <= width {
		return segments
	}
	remaining := width - 1
	result := make([]lineSegment, 0, len(segments)+1)
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
	return append(result, lineSegment{text: "…", style: ellipsisStyle})
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

func (model *Model) layoutWidth() int {
	if model.width < 6 {
		return 6
	}
	return model.width
}

func (model *Model) keyboardHelpLineCount() int {
	return len(model.keyboardHelpLines(model.layoutWidth()))
}

func (model *Model) keyboardHelpLines(width int) [][]lineSegment {
	if model.mode == saveConflictMode {
		return [][]lineSegment{model.compactShortcutLine(width, [][2]string{
			{"o", "overwrite"},
			{"r", "reload"},
			{"Esc", "cancel"},
		}, false)}
	}
	if model.mode == editMode {
		shortcuts := [][2]string{
			{"←/→", " cursor"},
			{"Home/End", " start/end"},
			{"Enter", " save"},
			{"Esc", " cancel"},
		}
		if model.edit.multiline {
			shortcuts = [][2]string{
				{"←/→", " cursor"},
				{"↑/↓", " cursor"},
				{"Home/End", " line edge"},
				{"Tab", " tab"},
				{"Enter", " save"},
				{"Shift+Enter", " newline"},
				{"Esc", " cancel"},
			}
		}
		return [][]lineSegment{model.compactShortcutLine(width, shortcuts, false)}
	}
	groups := [][][2]string{
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
			{"q", " quit"},
		},
	}
	if model.mode != browseMode {
		groups[1][5] = [2]string{"Esc", " cancel"}
	}
	shortcuts := make([][2]string, 0, 12)
	for _, group := range groups {
		shortcuts = append(shortcuts, group...)
	}
	if model.mode == browseMode && model.showAllShortcuts {
		return model.expandedShortcutLines(width, shortcuts)
	}
	return [][]lineSegment{model.compactShortcutLine(width, shortcuts, model.mode == browseMode)}
}

func (model *Model) compactShortcutLine(width int, shortcuts [][2]string, showMore bool) []lineSegment {
	if width <= 0 {
		return nil
	}
	shown := 0
	used := 0
	for shown < len(shortcuts) {
		separator := 0
		if shown > 0 {
			separator = 2
		}
		candidate := used + separator + shortcutItemWidth(shortcuts[shown])
		if candidate <= width {
			candidateWithMore := candidate
			if showMore {
				candidateWithMore += 7
				if shown+1 < len(shortcuts) {
					candidateWithMore++
				}
			}
			if candidateWithMore > width {
				break
			}
			used = candidate
			shown++
			continue
		}
		break
	}
	truncated := shown < len(shortcuts)
	ellipsisWidth := func() int {
		width := 1
		if showMore {
			width += 7
		}
		return width
	}
	if truncated {
		for shown > 0 {
			if used+ellipsisWidth() <= width {
				break
			}
			shown--
			used = shortcutListWidth(shortcuts, shown)
		}
	}

	segments := make([]lineSegment, 0, shown*3+4)
	for index := 0; index < shown; index++ {
		if index > 0 {
			segments = append(segments, lineSegment{text: "  ", style: model.styles.muted})
		}
		segments = appendShortcutSegments(segments, shortcuts[index], model.styles.shortcutKey, model.styles.muted)
	}
	if truncated && used+ellipsisWidth() <= width {
		segments = append(segments, lineSegment{text: "…", style: model.styles.muted})
	}
	if showMore {
		if len(segments) > 0 {
			segments = append(segments, lineSegment{text: " ", style: model.styles.muted})
		}
		segments = appendShortcutSegments(segments, [2]string{"?", " more"}, model.styles.shortcutKey, model.styles.muted)
	}
	return segments
}

func (model *Model) expandedShortcutLines(width int, shortcuts [][2]string) [][]lineSegment {
	lines := make([][]lineSegment, 0, len(shortcuts))
	current := make([]lineSegment, 0, 6)
	currentWidth := 0
	for _, shortcut := range shortcuts {
		itemWidth := shortcutItemWidth(shortcut)
		itemSegments := []lineSegment{
			{text: shortcut[0], style: model.styles.shortcutKey},
			{text: shortcut[1], style: model.styles.muted},
		}
		if itemWidth > width {
			if len(current) > 0 {
				lines = append(lines, current)
				current = nil
				currentWidth = 0
			}
			lines = append(lines, wrapShortcutSegments(itemSegments, width)...)
			continue
		}
		separator := 0
		if currentWidth > 0 {
			separator = 2
		}
		if currentWidth > 0 && currentWidth+separator+itemWidth > width {
			lines = append(lines, current)
			current = make([]lineSegment, 0, 6)
			currentWidth = 0
			separator = 0
		}
		if separator > 0 {
			current = append(current, lineSegment{text: "  ", style: model.styles.muted})
		}
		current = appendShortcutSegments(current, shortcut, model.styles.shortcutKey, model.styles.muted)
		currentWidth += separator + itemWidth
	}
	if len(current) > 0 {
		lines = append(lines, current)
	}
	if len(lines) == 0 {
		lines = append(lines, nil)
	}
	less := [2]string{"?", " less"}
	last := len(lines) - 1
	if lineSegmentWidth(lines[last])+8 <= width {
		lines[last] = append(lines[last],
			lineSegment{text: "  ", style: model.styles.muted},
			lineSegment{text: less[0], style: model.styles.shortcutKey},
			lineSegment{text: less[1], style: model.styles.muted},
		)
	} else {
		lines = append(lines, []lineSegment{
			{text: less[0], style: model.styles.shortcutKey},
			{text: less[1], style: model.styles.muted},
		})
	}
	return lines
}

func wrapShortcutSegments(segments []lineSegment, width int) [][]lineSegment {
	lines := make([][]lineSegment, 0, len(segments))
	current := make([]lineSegment, 0, len(segments))
	currentWidth := 0
	for _, segment := range segments {
		runes := []rune(segment.text)
		for len(runes) > 0 {
			if currentWidth == width {
				lines = append(lines, current)
				current = make([]lineSegment, 0, len(segments))
				currentWidth = 0
			}
			remaining := width - currentWidth
			count := len(runes)
			if count > remaining {
				count = remaining
			}
			current = append(current, lineSegment{text: string(runes[:count]), style: segment.style})
			currentWidth += count
			runes = runes[count:]
		}
	}
	if len(current) > 0 {
		lines = append(lines, current)
	}
	return lines
}

func appendShortcutSegments(segments []lineSegment, shortcut [2]string, keyStyle, labelStyle lipgloss.Style) []lineSegment {
	return append(segments,
		lineSegment{text: shortcut[0], style: keyStyle},
		lineSegment{text: shortcut[1], style: labelStyle},
	)
}

func shortcutItemWidth(shortcut [2]string) int {
	return len([]rune(shortcut[0])) + len([]rune(shortcut[1]))
}

func shortcutListWidth(shortcuts [][2]string, count int) int {
	width := 0
	for index := 0; index < count; index++ {
		if index > 0 {
			width += 2
		}
		width += shortcutItemWidth(shortcuts[index])
	}
	return width
}
