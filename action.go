package structivedit

import (
	"github.com/yuyosy/structivedit/document"
	"github.com/yuyosy/structivedit/schema"
)

// Action is a closed set of semantic operations accepted by Editor.Apply.
type Action interface {
	action()
}

// SetValue changes a scalar while preserving its ScalarKind.
type SetValue struct {
	NodeID document.NodeID
	Value  any
}

// Toggle switches a Bool scalar between true and false.
type Toggle struct {
	NodeID document.NodeID
}

// Focus changes the Editor's logical focus. NodeID zero clears focus.
type Focus struct {
	NodeID document.NodeID
}

// Undo restores the previous successful Document mutation.
type Undo struct{}

// Redo reapplies the next undone Document mutation.
type Redo struct{}

// Add commits an AddPlan produced by Editor.PrepareAdd.
type Add struct {
	Plan   AddPlan
	Values map[InputID]any
}

// Delete removes an optional object field or sequence item.
type Delete struct {
	NodeID document.NodeID
}

// Move reorders a sequence item to ToIndex after removing its old position.
type Move struct {
	NodeID  document.NodeID
	ToIndex int
}

// InputID identifies one scalar value requested by an AddPlan.
type InputID string

// AddTargetKind selects a schema-defined structure to add.
type AddTargetKind uint8

const (
	AddSequenceItem AddTargetKind = iota
	AddObjectField
)

// AddTarget identifies either an array item or an object field.
type AddTarget struct {
	Kind  AddTargetKind
	Field string
}

// InputRequest describes a scalar input required or optionally accepted while
// committing an AddPlan.
type InputRequest struct {
	ID            InputID
	FieldPath     string
	Label         string
	ExpectedKind  schema.Kind
	ValueRequired bool
	HasDefault    bool
	Default       any
}

// AddPlan is an immutable-at-use request to add schema-defined content.
// Inputs is returned as a copy; CommitAdd checks it against its private seal.
type AddPlan struct {
	Parent      document.NodeID
	Target      AddTarget
	Revision    document.Revision
	Inputs      []InputRequest
	editorToken *editorIdentity
	seal        addPlanSeal
}

type addPlanSeal struct {
	parent   document.NodeID
	target   AddTarget
	revision document.Revision
	inputs   []InputRequest
}

func (SetValue) action() {}
func (Toggle) action()   {}
func (Focus) action()    {}
func (Undo) action()     {}
func (Redo) action()     {}
func (Add) action()      {}
func (Delete) action()   {}
func (Move) action()     {}
