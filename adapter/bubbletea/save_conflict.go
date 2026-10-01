package bubbletea

import (
	"errors"

	"github.com/yuyosy/structivedit"
)

// ErrSaveConflict tells Model that the caller detected an external change
// after the current document was loaded and before it was saved.
var ErrSaveConflict = errors.New("bubbletea: file changed outside the editor")

// SaveConflictAction selects how to resolve a detected external change.
type SaveConflictAction uint8

const (
	// SaveConflictOverwrite replaces the externally changed file with the
	// current Editor document.
	SaveConflictOverwrite SaveConflictAction = iota
	// SaveConflictReload discards the current Editor document and loads the
	// external version.
	SaveConflictReload
)

// SaveConflictHandler resolves an external change. For SaveConflictReload,
// return a newly loaded, clean Editor; for SaveConflictOverwrite, return nil.
type SaveConflictHandler func(SaveConflictAction) (*structivedit.Editor, error)

// SetSaveConflictHandler registers the resolver used when SaveHandler returns
// ErrSaveConflict. The UI offers overwrite, reload, or cancel. Cancel is
// handled by the Model and leaves the current Editor unchanged.
func (model *Model) SetSaveConflictHandler(handler SaveConflictHandler) {
	if model != nil {
		model.saveConflictHandler = handler
	}
}

func (model *Model) resolveSaveConflict(action SaveConflictAction) {
	if model.saveConflictHandler == nil {
		model.mode = browseMode
		model.setErrorMessage(ErrSaveConflict.Error())
		return
	}
	editor, err := model.saveConflictHandler(action)
	if err != nil {
		model.fail(err)
		return
	}
	if action == SaveConflictReload {
		if editor == nil || editor.Document() == nil || editor.Document().Root() == 0 {
			model.setErrorMessage("Reload returned an empty editor")
			return
		}
		model.replaceEditor(editor)
		model.setMessage("External file loaded; local changes discarded")
		return
	}
	model.mode = browseMode
	model.setMessage("Saved")
}

func (model *Model) cancelSaveConflict() {
	model.mode = browseMode
	model.setMessage("Save cancelled; local changes preserved")
}
