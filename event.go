package structivedit

import "github.com/yuyosy/structivedit/document"

// ApplyResult describes the committed effects of one Action.
type ApplyResult struct {
	DocumentChanged bool
	Revision        document.Revision
	Events          []Event
}

// EventKind identifies a synchronous event emitted by an Editor action.
type EventKind uint8

const (
	EventValueChanged EventKind = iota
	EventNodeAdded
	EventNodeDeleted
	EventNodeMoved
	EventFocusChanged
	EventValidationChanged
	EventHistoryChanged
)

// Event contains the values associated with one successful Editor action.
// Fields that do not apply to Kind retain their zero value.
type Event struct {
	Kind          EventKind
	NodeID        document.NodeID
	ParentID      document.NodeID
	FromIndex     int
	ToIndex       int
	BeforeValue   any
	AfterValue    any
	PreviousFocus document.NodeID
	Focus         document.NodeID
	Issues        []ValidationIssue
	Revision      document.Revision
}

func cloneEvents(events []Event) []Event {
	cloned := make([]Event, len(events))
	for index, event := range events {
		cloned[index] = event
		cloned[index].Issues = cloneValidationIssues(event.Issues)
	}
	return cloned
}
