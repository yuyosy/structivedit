// Package bubbletea adapts an Editor to a keyboard-first terminal tree editor.
package bubbletea

import (
	tea "charm.land/bubbletea/v2"
	"github.com/yuyosy/structivedit"
	"github.com/yuyosy/structivedit/document"
)

type mode uint8

const (
	browseMode mode = iota
	editMode
	addFieldMode
	addValueMode
	deleteConfirmMode
)

type editBuffer struct {
	nodeID document.NodeID
	value  textInput
}

type addPrompt struct {
	parent  document.NodeID
	field   textInput
	plan    structivedit.AddPlan
	input   textInput
	inputAt int
	values  map[structivedit.InputID]any
}

type deletePrompt struct {
	nodeID  document.NodeID
	confirm bool
}

type viewport struct {
	offset int
}

type treeRow struct {
	nodeID    document.NodeID
	depth     int
	label     string
	focusable bool
	hasChild  bool
	expanded  bool
	valueText string
	issueText string
}

type hitRegion struct {
	line   int
	nodeID document.NodeID
}

// Model is a Bubble Tea model that displays and edits an Editor's Document.
// File I/O stays with the caller; SetSaveHandler connects an explicit save key
// to the caller's persistence logic.
type Model struct {
	editor        *structivedit.Editor
	mode          mode
	expanded      map[document.NodeID]bool
	edit          editBuffer
	add           addPrompt
	delete        deletePrompt
	viewport      viewport
	width         int
	height        int
	message       string
	saveHandler   func() error
	hitRegions    []hitRegion
	expandAliases bool
}

// NewModel creates a terminal model for editor. Containers at the root and
// one level below it start expanded; deeper content can be opened as needed.
func NewModel(editor *structivedit.Editor, options ...ModelOption) *Model {
	settings := modelOptions{}
	for _, option := range options {
		if option != nil {
			option(&settings)
		}
	}
	model := &Model{
		editor:        editor,
		expanded:      make(map[document.NodeID]bool),
		width:         100,
		height:        24,
		expandAliases: settings.expandAliases,
	}
	if editor != nil && editor.Document() != nil {
		root := editor.Document().Root()
		_ = editor.Focus(root)
		model.expandInitial(root, 0)
		model.expandInitialAliases()
	}
	return model
}

// Editor returns the Editor controlled by this model.
func (model *Model) Editor() *structivedit.Editor {
	if model == nil {
		return nil
	}
	return model.editor
}

// SetSaveHandler registers the caller's persistence function for Ctrl+S.
// The handler should call Editor.MarkClean only after encoding and writing
// have both succeeded.
func (model *Model) SetSaveHandler(handler func() error) {
	if model != nil {
		model.saveHandler = handler
	}
}

// Init implements tea.Model.
func (model *Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (model *Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if model == nil {
		return model, nil
	}
	if model.editor == nil {
		if pressed, ok := message.(tea.KeyPressMsg); ok {
			key := pressed.Key()
			if key.Code == tea.KeyEscape || key.Text == "q" || key.Mod.Contains(tea.ModCtrl) && key.Code == 'c' {
				return model, tea.Quit
			}
		}
		model.message = "Editor unavailable"
		return model, nil
	}
	switch typed := message.(type) {
	case tea.WindowSizeMsg:
		model.width = typed.Width
		model.height = typed.Height
		model.keepFocusVisible()
	case tea.KeyPressMsg:
		return model.handleKey(typed.Key())
	case tea.MouseClickMsg:
		model.handleMouseClick(typed)
	case tea.MouseWheelMsg:
		model.handleMouseWheel(typed)
	}
	return model, nil
}

// View implements tea.Model.
func (model *Model) View() tea.View { return model.render() }

func (model *Model) expandInitial(id document.NodeID, depth int) {
	if model.editor == nil || depth > 1 {
		return
	}
	view, err := model.editor.View(id)
	if err != nil {
		return
	}
	if view.Kind != document.NodeMapping && view.Kind != document.NodeSequence {
		return
	}
	model.expanded[id] = true
	for _, child := range visibleChildren(view) {
		model.expandInitial(child, depth+1)
	}
}

func visibleChildren(view structivedit.NodeView) []document.NodeID {
	if view.Kind == document.NodeSequence {
		return append([]document.NodeID(nil), view.SequenceItems...)
	}
	if view.Kind == document.NodeMapping {
		children := make([]document.NodeID, len(view.MappingEntries))
		for index, entry := range view.MappingEntries {
			children[index] = entry.Value
		}
		return children
	}
	return nil
}

func (model *Model) referenceChildren(view structivedit.NodeView) []document.NodeID {
	if model == nil || !model.expandAliases || view.Kind != document.NodeReference || !view.HasReferenceTarget {
		return nil
	}
	target, err := model.editor.View(view.ReferenceTarget)
	if err != nil {
		return nil
	}
	return visibleChildren(target)
}

func (model *Model) expandInitialAliases() {
	if model == nil || model.editor == nil || !model.expandAliases {
		return
	}
	for _, view := range model.editor.Views() {
		if len(model.referenceChildren(view)) > 0 {
			model.expanded[view.ID] = true
		}
	}
}
