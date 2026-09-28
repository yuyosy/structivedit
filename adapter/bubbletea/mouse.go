package bubbletea

import tea "charm.land/bubbletea/v2"

func (model *Model) handleMouseClick(message tea.MouseClickMsg) {
	if message.Button != tea.MouseLeft {
		return
	}
	if id, ok := model.hitTest(message.X, message.Y); ok {
		model.focus(id)
	}
}

func (model *Model) handleMouseWheel(message tea.MouseWheelMsg) {
	switch message.Button {
	case tea.MouseWheelUp:
		model.scroll(-3)
	case tea.MouseWheelDown:
		model.scroll(3)
	}
}
