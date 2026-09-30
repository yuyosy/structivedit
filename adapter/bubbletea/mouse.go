package bubbletea

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/yuyosy/structivedit/document"
)

const doubleClickInterval = 500 * time.Millisecond

type mouseClickState struct {
	nodeID document.NodeID
	at     time.Time
}

func (model *Model) handleMouseClick(message tea.MouseClickMsg) {
	if message.Button != tea.MouseLeft {
		model.lastClick = mouseClickState{}
		return
	}
	region, ok := model.hitTest(message.X, message.Y)
	if !ok {
		model.lastClick = mouseClickState{}
		return
	}
	model.focus(region.nodeID)
	if model.mode != browseMode {
		model.lastClick = mouseClickState{}
		return
	}
	foldArea := region.foldable && message.X >= region.foldStart && message.X < region.foldEnd
	if foldArea {
		model.lastClick = mouseClickState{}
		model.enterFocused()
		return
	}
	if !model.mouseDoubleClick {
		model.lastClick = mouseClickState{}
		return
	}

	now := time.Now()
	elapsed := now.Sub(model.lastClick.at)
	if model.lastClick.nodeID == region.nodeID && !model.lastClick.at.IsZero() && elapsed >= 0 && elapsed <= doubleClickInterval {
		model.lastClick = mouseClickState{}
		model.handleMouseDoubleClick(region.nodeID)
		return
	}
	model.lastClick = mouseClickState{nodeID: region.nodeID, at: now}
}

func (model *Model) handleMouseDoubleClick(id document.NodeID) {
	if model == nil || model.editor == nil || model.mode != browseMode {
		return
	}
	view, err := model.editor.View(id)
	if err != nil {
		model.fail(err)
		return
	}
	if view.Kind != document.NodeScalar || !view.Capabilities.Editable {
		return
	}
	_, value, scalar := model.editor.Document().Scalar(id)
	if !scalar {
		return
	}
	if _, isBool := value.(bool); isBool {
		model.toggleFocused()
		return
	}
	model.enterFocused()
}

func (model *Model) handleMouseWheel(message tea.MouseWheelMsg) {
	model.lastClick = mouseClickState{}
	switch message.Button {
	case tea.MouseWheelUp:
		model.scroll(-3)
	case tea.MouseWheelDown:
		model.scroll(3)
	}
}
