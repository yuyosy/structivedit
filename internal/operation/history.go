package operation

import "github.com/yuyosy/structivedit/document"

// History retains reversible Document mutations and tracks the saved position.
// base and cursor are logical positions independent of Document Revision.
type History struct {
	entries []Operation
	base    int
	cursor  int
	cleanAt int
	limit   int
}

// NewHistory creates an empty history. A zero limit tracks Dirty state without
// retaining Undo / Redo entries.
func NewHistory(limit int) *History {
	if limit < 0 {
		limit = 0
	}
	return &History{cleanAt: 0, limit: limit}
}

// Record adds one successfully built Document mutation as a single entry.
func (history *History) Record(before, after *document.Document, affected []document.NodeID, effects []Effect) error {
	if history == nil {
		return ErrInvalidOperation
	}
	entry, err := NewSnapshotOperation(before, after, affected, effects)
	if err != nil {
		return err
	}
	end := history.base + len(history.entries)
	if history.cursor < history.base || history.cursor > end {
		return ErrInvalidOperation
	}
	if history.cursor < end {
		if history.cleanAt > history.cursor {
			history.cleanAt = -1
		}
		kept := append([]Operation(nil), history.entries[:history.cursor-history.base]...)
		history.entries = kept
	}
	if history.limit == 0 {
		history.cursor++
		history.base = history.cursor
		history.entries = nil
		if history.cleanAt >= 0 && history.cleanAt < history.base {
			history.cleanAt = -1
		}
		return nil
	}
	history.entries = append(history.entries, entry)
	history.cursor++
	if excess := len(history.entries) - history.limit; excess > 0 {
		history.entries = append([]Operation(nil), history.entries[excess:]...)
		history.base += excess
		if history.cleanAt >= 0 && history.cleanAt < history.base {
			history.cleanAt = -1
		}
	}
	return nil
}

// UndoCandidate returns the next operation without moving the logical cursor.
func (history *History) UndoCandidate() Operation {
	if history == nil || history.cursor <= history.base {
		return nil
	}
	index := history.cursor - history.base - 1
	if index < 0 || index >= len(history.entries) {
		return nil
	}
	return history.entries[index]
}

// RedoCandidate returns the next operation without moving the logical cursor.
func (history *History) RedoCandidate() Operation {
	if history == nil {
		return nil
	}
	index := history.cursor - history.base
	if index < 0 || index >= len(history.entries) {
		return nil
	}
	return history.entries[index]
}

// CommitUndo moves the cursor after an Undo has successfully built a snapshot.
func (history *History) CommitUndo() bool {
	if history == nil || history.UndoCandidate() == nil {
		return false
	}
	history.cursor--
	return true
}

// CommitRedo moves the cursor after a Redo has successfully built a snapshot.
func (history *History) CommitRedo() bool {
	if history == nil || history.RedoCandidate() == nil {
		return false
	}
	history.cursor++
	return true
}

// MarkClean records the current logical position as the saved position.
func (history *History) MarkClean() {
	if history != nil {
		history.cleanAt = history.cursor
	}
}

// Dirty reports whether the cursor differs from the retained saved position.
func (history *History) Dirty() bool {
	return history != nil && (history.cleanAt < 0 || history.cursor != history.cleanAt)
}
