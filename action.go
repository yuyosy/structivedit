package structivedit

import "github.com/yuyosy/structivedit/document"

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

func (SetValue) action() {}
func (Toggle) action()   {}
func (Focus) action()    {}
func (Undo) action()     {}
func (Redo) action()     {}
