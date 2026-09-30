package operation

import (
	"errors"

	"github.com/yuyosy/structivedit/document"
)

// Operation is a validated Document mutation applied to a private Builder.
type Operation interface {
	Apply(*document.Builder) error
	Undo(*document.Builder) error
	AffectedNodeIDs() []document.NodeID
	Effects(reverse bool) []Effect
}

// EffectKind identifies an operation-level change used to emit public Events.
type EffectKind uint8

const (
	EffectValueChanged EffectKind = iota
	EffectNodeAdded
	EffectNodeDeleted
	EffectNodeMoved
)

// Effect is a framework-independent description of one Document operation.
type Effect struct {
	Kind        EffectKind
	NodeID      document.NodeID
	ParentID    document.NodeID
	FromIndex   int
	ToIndex     int
	BeforeValue any
	AfterValue  any
}

var ErrInvalidOperation = errors.New("operation: invalid snapshot operation")

func NewSnapshotOperation(before, after *document.Document, affected []document.NodeID, effects []Effect) (Operation, error) {
	if before == nil || after == nil || !before.SameLineage(after) {
		return nil, ErrInvalidOperation
	}
	changedIDs := append([]document.NodeID(nil), affected...)
	for _, effect := range effects {
		if effect.NodeID != 0 {
			changedIDs = append(changedIDs, effect.NodeID)
		}
		if effect.ParentID != 0 {
			changedIDs = append(changedIDs, effect.ParentID)
		}
	}
	patch, err := document.NewSnapshotPatchForNodes(before, after, changedIDs)
	if err != nil {
		return nil, ErrInvalidOperation
	}
	return &snapshotOperation{
		patch:    patch,
		affected: append([]document.NodeID(nil), affected...),
		effects:  append([]Effect(nil), effects...),
	}, nil
}

type snapshotOperation struct {
	patch    *document.SnapshotPatch
	affected []document.NodeID
	effects  []Effect
}

func (operation *snapshotOperation) Apply(builder *document.Builder) error {
	if operation == nil || builder == nil {
		return ErrInvalidOperation
	}
	return operation.patch.Apply(builder, false)
}

func (operation *snapshotOperation) Undo(builder *document.Builder) error {
	if operation == nil || builder == nil {
		return ErrInvalidOperation
	}
	return operation.patch.Apply(builder, true)
}

func (operation *snapshotOperation) AffectedNodeIDs() []document.NodeID {
	if operation == nil {
		return nil
	}
	return append([]document.NodeID(nil), operation.affected...)
}

func (operation *snapshotOperation) Effects(reverse bool) []Effect {
	if operation == nil {
		return nil
	}
	if !reverse {
		return append([]Effect(nil), operation.effects...)
	}
	effects := make([]Effect, len(operation.effects))
	for index := range operation.effects {
		effects[len(operation.effects)-index-1] = reverseEffect(operation.effects[index])
	}
	return effects
}

func reverseEffect(effect Effect) Effect {
	switch effect.Kind {
	case EffectValueChanged:
		effect.BeforeValue, effect.AfterValue = effect.AfterValue, effect.BeforeValue
	case EffectNodeAdded:
		effect.Kind = EffectNodeDeleted
		effect.FromIndex = effect.ToIndex
	case EffectNodeDeleted:
		effect.Kind = EffectNodeAdded
		effect.ToIndex = effect.FromIndex
	case EffectNodeMoved:
		effect.FromIndex, effect.ToIndex = effect.ToIndex, effect.FromIndex
	}
	return effect
}
