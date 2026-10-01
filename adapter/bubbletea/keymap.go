package bubbletea

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/yuyosy/structivedit"
	"github.com/yuyosy/structivedit/document"
)

func (model *Model) handleKey(key tea.Key) (tea.Model, tea.Cmd) {
	model.lastClick = mouseClickState{}
	model.clearMessage()
	switch model.mode {
	case editMode:
		model.handleEditKey(key)
	case addFieldMode:
		model.handleAddFieldKey(key)
	case addValueMode:
		model.handleAddValueKey(key)
	case deleteConfirmMode:
		model.handleDeleteKey(key)
	case saveConflictMode:
		model.handleSaveConflictKey(key)
	default:
		return model.handleBrowseKey(key)
	}
	return model, nil
}

func (model *Model) handleBrowseKey(key tea.Key) (tea.Model, tea.Cmd) {
	if (key.Text == "?" || key.Code == '?') && !key.Mod.Contains(tea.ModCtrl) && !key.Mod.Contains(tea.ModAlt) {
		model.showAllShortcuts = !model.showAllShortcuts
		return model, nil
	}
	if key.Mod.Contains(tea.ModCtrl) {
		switch key.Code {
		case 'c', 'q':
			return model, tea.Quit
		case 's':
			model.save()
			return model, nil
		case 'z':
			model.apply(structivedit.Undo{})
			return model, nil
		case 'y':
			model.apply(structivedit.Redo{})
			return model, nil
		}
	}
	if key.Mod.Contains(tea.ModAlt) {
		switch key.Code {
		case tea.KeyUp:
			model.moveFocused(-1)
			return model, nil
		case tea.KeyDown:
			model.moveFocused(1)
			return model, nil
		}
	}
	switch key.Code {
	case tea.KeyUp:
		model.focusAdjacent(-1)
	case tea.KeyDown:
		model.focusAdjacent(1)
	case tea.KeyLeft:
		model.collapseOrFocusParent()
	case tea.KeyRight:
		model.expandOrFocusChild()
	case tea.KeyEnter:
		model.enterFocused()
	case tea.KeyPgUp:
		model.scroll(-model.visibleRowCount())
	case tea.KeyPgDown:
		model.scroll(model.visibleRowCount())
	case tea.KeyTab:
		model.toggleCursorMode()
	default:
		if key.Mod != 0 {
			return model, nil
		}
		switch key.Text {
		case " ":
			model.toggleFocused()
		case "a":
			model.startAdd()
		case "d":
			model.startDelete()
		case "q":
			return model, tea.Quit
		}
	}
	return model, nil
}

func (model *Model) toggleCursorMode() {
	if model.cursorMode == rowHeadCursor {
		model.cursorMode = valueCellCursor
		return
	}
	model.cursorMode = rowHeadCursor
}

func (model *Model) handleEditKey(key tea.Key) {
	if key.Code == tea.KeyEscape {
		model.mode = browseMode
		model.setMessage("Edit cancelled")
		return
	}
	if key.Code == tea.KeyEnter {
		if model.edit.multiline && key.Mod.Contains(tea.ModShift) {
			model.edit.value.insertNewline()
			model.keepMultilineCursorVisible()
			return
		}
		model.commitEdit()
		return
	}
	if model.edit.multiline && key.Code == tea.KeyTab {
		model.edit.value.insert([]rune{'\t'})
		model.keepMultilineCursorVisible()
		return
	}
	model.edit.value.handleKey(key, model.edit.multiline)
	model.keepMultilineCursorVisible()
}

func (model *Model) keepMultilineCursorVisible() {
	if model == nil || !model.edit.multiline {
		return
	}
	lines := model.edit.value.lines()
	if len(lines) == 0 {
		return
	}
	cursorLine, _ := model.edit.value.cursorLineColumn(lines)
	visible := min(multilineEditorMaxVisibleLines, len(lines))
	start := model.edit.viewportLine
	if cursorLine < start {
		start = cursorLine
	} else if cursorLine >= start+visible {
		start = cursorLine - visible + 1
	}
	maximum := len(lines) - visible
	if start > maximum {
		start = maximum
	}
	if start < 0 {
		start = 0
	}
	model.edit.viewportLine = start
}

func (model *Model) commitEdit() {
	view, err := model.editor.View(model.edit.nodeID)
	if err != nil {
		model.fail(err)
		model.mode = browseMode
		return
	}
	current, _, ok := model.editor.Document().Scalar(view.ID)
	if !ok {
		model.fail(document.ErrInvalidDocument)
		model.mode = browseMode
		return
	}
	value, err := parseScalarInput(current, model.edit.value.String())
	if err != nil {
		model.setErrorMessage(err.Error())
		return
	}
	model.mode = browseMode
	model.apply(structivedit.SetValue{NodeID: model.edit.nodeID, Value: value})
}

func (model *Model) handleAddFieldKey(key tea.Key) {
	if key.Code == tea.KeyEscape {
		model.cancelAdd()
		return
	}
	if key.Code == tea.KeyEnter {
		field := model.add.field.String()
		if field == "" {
			model.setErrorMessage("Enter a field name")
			return
		}
		plan, err := model.editor.PrepareAdd(model.add.parent, structivedit.AddTarget{
			Kind:  structivedit.AddObjectField,
			Field: field,
		})
		if err != nil {
			model.setErrorMessage(err.Error())
			return
		}
		model.beginAddPlan(plan)
		return
	}
	model.add.field.handleKey(key, false)
}

func (model *Model) handleAddValueKey(key tea.Key) {
	if key.Code == tea.KeyEscape {
		model.cancelAdd()
		return
	}
	if key.Code == tea.KeyTab {
		if err := model.storeAddInput(); err != nil {
			model.setErrorMessage(err.Error())
			return
		}
		delta := 1
		if key.Mod.Contains(tea.ModShift) {
			delta = -1
		}
		model.add.inputAt = (model.add.inputAt + delta + len(model.add.plan.Inputs)) % len(model.add.plan.Inputs)
		model.loadAddInput()
		return
	}
	if key.Code == tea.KeyEnter {
		if err := model.storeAddInput(); err != nil {
			model.setErrorMessage(err.Error())
			return
		}
		if model.add.inputAt+1 < len(model.add.plan.Inputs) {
			model.add.inputAt++
			model.loadAddInput()
			return
		}
		model.commitAdd()
		return
	}
	model.add.input.handleKey(key, false)
}

func (model *Model) handleDeleteKey(key tea.Key) {
	switch key.Code {
	case tea.KeyEscape:
		model.mode = browseMode
		model.setMessage("Delete cancelled")
	case tea.KeyTab:
		model.delete.confirm = !model.delete.confirm
	case tea.KeyEnter:
		if !model.delete.confirm {
			model.mode = browseMode
			model.setMessage("Delete cancelled")
			return
		}
		id := model.delete.nodeID
		model.mode = browseMode
		result, err := model.editor.Delete(id)
		if err != nil {
			model.fail(err)
			return
		}
		model.setMessage("Node deleted")
		model.afterApply(result)
	}
}

func (model *Model) focusAdjacent(delta int) {
	rows := model.visibleRows()
	if len(rows) == 0 {
		return
	}
	index := -1
	if id, ok := model.editor.Focused(); ok {
		for position, row := range rows {
			if row.focusable && row.nodeID == id {
				index = position
				break
			}
		}
	}
	if index < 0 {
		if delta > 0 {
			index = 0
		} else {
			index = len(rows) - 1
		}
	} else {
		index += delta
	}
	for index >= 0 && index < len(rows) && !rows[index].focusable {
		index += delta
	}
	if index >= 0 && index < len(rows) {
		model.focus(rows[index].nodeID)
	}
}

func (model *Model) hasVisibleChildren(view structivedit.NodeView) bool {
	return len(visibleChildren(view)) > 0 || len(model.referenceChildren(view)) > 0
}

func (model *Model) collapseOrFocusParent() {
	id, ok := model.editor.Focused()
	if !ok {
		return
	}
	view, err := model.editor.View(id)
	if err != nil {
		return
	}
	if model.hasVisibleChildren(view) && model.expanded[id] {
		model.expanded[id] = false
		model.keepFocusVisible()
		return
	}
	if view.HasParent {
		model.focus(view.Parent.Parent)
	}
}

func (model *Model) expandOrFocusChild() {
	id, ok := model.editor.Focused()
	if !ok {
		return
	}
	view, err := model.editor.View(id)
	if err != nil {
		return
	}
	if len(model.referenceChildren(view)) > 0 {
		model.expanded[id] = true
		return
	}
	children := visibleChildren(view)
	if len(children) == 0 {
		return
	}
	if !model.expanded[id] {
		model.expanded[id] = true
		return
	}
	model.focus(children[0])
}

func (model *Model) enterFocused() {
	id, ok := model.editor.Focused()
	if !ok {
		return
	}
	view, err := model.editor.View(id)
	if err != nil {
		model.fail(err)
		return
	}
	if view.Kind == document.NodeMapping || view.Kind == document.NodeSequence || len(model.referenceChildren(view)) > 0 {
		if model.hasVisibleChildren(view) {
			model.expanded[id] = !model.expanded[id]
		}
		return
	}
	if view.Kind == document.NodeScalar && view.Capabilities.Editable {
		kind, value, scalar := model.editor.Document().Scalar(id)
		if scalar {
			text := scalarText(kind, value)
			model.edit = editBuffer{
				nodeID:    id,
				value:     newTextInput(text),
				multiline: kind == document.ScalarString && strings.ContainsAny(text, "\r\n"),
			}
			model.showAllShortcuts = false
			model.mode = editMode
			model.keepMultilineCursorVisible()
			model.clearMessage()
		}
	}
}

func (model *Model) toggleFocused() {
	id, ok := model.editor.Focused()
	if !ok {
		return
	}
	_, value, scalar := model.editor.Document().Scalar(id)
	if !scalar {
		return
	}
	if _, isBool := value.(bool); !isBool {
		return
	}
	model.clearMessage()
	model.apply(structivedit.Toggle{NodeID: id})
}

func (model *Model) startAdd() {
	parent, ok := model.editor.Focused()
	if !ok {
		return
	}
	view, err := model.editor.View(parent)
	if err != nil || !view.Capabilities.Addable {
		model.setErrorMessage("This node cannot accept an item")
		return
	}
	model.add = addPrompt{parent: parent, values: make(map[structivedit.InputID]any)}
	switch view.Kind {
	case document.NodeSequence:
		plan, err := model.editor.PrepareAdd(parent, structivedit.AddTarget{Kind: structivedit.AddSequenceItem})
		if err != nil {
			model.fail(err)
			return
		}
		model.beginAddPlan(plan)
	case document.NodeMapping:
		model.showAllShortcuts = false
		model.mode = addFieldMode
	default:
		model.setErrorMessage("Only sequences and mappings accept items")
	}
}

func (model *Model) beginAddPlan(plan structivedit.AddPlan) {
	model.showAllShortcuts = false
	model.add.plan = plan
	model.add.values = make(map[structivedit.InputID]any)
	if len(plan.Inputs) == 0 {
		model.mode = browseMode
		model.commitAdd()
		return
	}
	model.mode = addValueMode
	model.add.inputAt = 0
	model.loadAddInput()
}

func (model *Model) loadAddInput() {
	if model.add.inputAt < 0 || model.add.inputAt >= len(model.add.plan.Inputs) {
		return
	}
	request := model.add.plan.Inputs[model.add.inputAt]
	value := ""
	touched := false
	if previous, ok := model.add.values[request.ID]; ok {
		value = defaultInputText(previous)
		touched = true
	} else if request.HasDefault {
		value = defaultInputText(request.Default)
	}
	model.add.input = newTextInput(value)
	model.add.input.touched = touched
}

func (model *Model) storeAddInput() error {
	if model.add.inputAt < 0 || model.add.inputAt >= len(model.add.plan.Inputs) {
		return document.ErrInvalidDocument
	}
	request := model.add.plan.Inputs[model.add.inputAt]
	if !model.add.input.touched && request.HasDefault {
		model.add.values[request.ID] = request.Default
		return nil
	}
	value, err := parseSchemaInput(request.ExpectedKind, model.add.input.String())
	if err != nil {
		return fmt.Errorf("%s: %w", request.Label, err)
	}
	model.add.values[request.ID] = value
	return nil
}

func (model *Model) commitAdd() {
	result, err := model.editor.CommitAdd(model.add.plan, model.add.values)
	model.mode = browseMode
	if err != nil {
		model.fail(err)
		return
	}
	model.setMessage("Node added")
	model.afterApply(result)
	for _, event := range result.Events {
		if event.Kind == structivedit.EventNodeAdded {
			_ = model.editor.Focus(event.NodeID)
			break
		}
	}
}

func (model *Model) cancelAdd() {
	model.mode = browseMode
	model.add = addPrompt{}
	model.setMessage("Add cancelled")
}

func (model *Model) startDelete() {
	id, ok := model.editor.Focused()
	if !ok || !model.editor.CanDelete(id) {
		model.setErrorMessage("This node cannot be deleted")
		return
	}
	model.delete = deletePrompt{nodeID: id}
	model.showAllShortcuts = false
	model.mode = deleteConfirmMode
}

func (model *Model) moveFocused(delta int) {
	id, ok := model.editor.Focused()
	if !ok || !model.editor.CanReorder(id) {
		return
	}
	view, err := model.editor.View(id)
	if err != nil || !view.HasParent || view.Parent.Role != document.ParentSequenceItem {
		return
	}
	items, ok := model.editor.Document().SequenceItems(view.Parent.Parent)
	if !ok {
		return
	}
	index := view.Parent.Index + delta
	if index < 0 || index >= len(items) {
		return
	}
	model.apply(structivedit.Move{NodeID: id, ToIndex: index})
}

func (model *Model) focus(id document.NodeID) {
	if err := model.editor.Focus(id); err != nil {
		model.fail(err)
		return
	}
	model.keepFocusVisible()
}

func (model *Model) apply(action structivedit.Action) {
	result, err := model.editor.Apply(action)
	if err != nil {
		model.fail(err)
		return
	}
	model.afterApply(result)
}

func (model *Model) afterApply(result structivedit.ApplyResult) {
	model.hitRegions = nil
	model.keepFocusVisible()
	if result.DocumentChanged {
		model.expanded[model.editor.Document().Root()] = true
	}
}

func (model *Model) save() {
	if model.saveHandler == nil {
		model.setMessage("Save is handled by the calling application")
		return
	}
	if err := model.saveHandler(); err != nil {
		if errors.Is(err, ErrSaveConflict) && model.saveConflictHandler != nil {
			model.mode = saveConflictMode
			model.setErrorMessage("The file changed outside the editor")
			return
		}
		model.fail(err)
		return
	}
	model.setMessage("Saved")
}

func (model *Model) handleSaveConflictKey(key tea.Key) {
	if key.Code == tea.KeyEscape {
		model.cancelSaveConflict()
		return
	}
	if key.Mod != 0 {
		return
	}
	switch strings.ToLower(key.Text) {
	case "o":
		model.resolveSaveConflict(SaveConflictOverwrite)
	case "r":
		model.resolveSaveConflict(SaveConflictReload)
	}
}

func (model *Model) fail(err error) {
	if err == nil {
		return
	}
	model.setErrorMessage(err.Error())
}

func (model *Model) visibleRowCount() int {
	count := model.height - 3 - model.inputAreaLineCount() - model.keyboardHelpLineCount()
	if count < 1 {
		return 1
	}
	return count
}

func (model *Model) inputAreaLineCount() int {
	switch model.mode {
	case editMode:
		if model.inlineEditing && !model.edit.multiline {
			return 1
		}
		if model.edit.multiline {
			return 1 + min(multilineEditorMaxVisibleLines, len(model.edit.value.lines()))
		}
		return 2
	case addFieldMode, addValueMode:
		return 2
	case deleteConfirmMode, saveConflictMode:
		return 1
	default:
		return 0
	}
}

func (model *Model) scroll(delta int) {
	rows := model.visibleRows()
	maximum := len(rows) - model.visibleRowCount()
	if maximum < 0 {
		maximum = 0
	}
	model.viewport.offset += delta
	if model.viewport.offset < 0 {
		model.viewport.offset = 0
	}
	if model.viewport.offset > maximum {
		model.viewport.offset = maximum
	}
}

func (model *Model) keepFocusVisible() {
	if model == nil || model.editor == nil {
		return
	}
	rows := model.visibleRows()
	focused, ok := model.editor.Focused()
	if !ok {
		return
	}
	index := -1
	for position, row := range rows {
		if row.nodeID == focused {
			index = position
			break
		}
	}
	if index < 0 {
		return
	}
	count := model.visibleRowCount()
	if index < model.viewport.offset {
		model.viewport.offset = index
	} else if index >= model.viewport.offset+count {
		model.viewport.offset = index - count + 1
	}
	maximum := len(rows) - count
	if maximum < 0 {
		maximum = 0
	}
	if model.viewport.offset > maximum {
		model.viewport.offset = maximum
	}
}

func (model *Model) currentDeletePath() string {
	view, err := model.editor.View(model.delete.nodeID)
	if err != nil {
		return fmt.Sprintf("node %d", model.delete.nodeID)
	}
	return readablePath(model.editor.Document(), view.Path)
}
