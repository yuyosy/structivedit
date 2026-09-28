package resolver

import (
	"errors"
	"sort"

	"github.com/yuyosy/structivedit/document"
	"github.com/yuyosy/structivedit/internal/selector"
)

type Decision uint8

const (
	Inherit Decision = iota
	Allow
	Deny
)

type ScopePolicy struct {
	Editable    Decision
	Addable     Decision
	Deletable   Decision
	Reorderable Decision
}

type TypePolicy struct {
	String  Decision
	Bool    Decision
	Integer Decision
	Float   Decision
	Null    Decision
}

type Rule struct {
	Selector selector.Pattern
	Policy   ScopePolicy
	order    int
}

type CompiledPolicy struct {
	defaultPolicy ScopePolicy
	types         TypePolicy
	rules         []Rule
}

// ResolvedPolicy is the effective Policy permission set for one Node. Hard
// constraints from Schema, References, and NodeRestrictions are evaluated
// separately.
type ResolvedPolicy struct {
	Editable    bool
	Addable     bool
	Deletable   bool
	Reorderable bool
}

var errInvalidDecision = errors.New("resolver: invalid policy decision")

// Compile copies and orders policy rules for immutable use by an Editor.
func Compile(defaultPolicy ScopePolicy, types TypePolicy, rules []Rule) (CompiledPolicy, error) {
	defaultPolicy.Editable = resolvedDefault(defaultPolicy.Editable)
	defaultPolicy.Addable = resolvedDefault(defaultPolicy.Addable)
	defaultPolicy.Deletable = resolvedDefault(defaultPolicy.Deletable)
	defaultPolicy.Reorderable = resolvedDefault(defaultPolicy.Reorderable)
	if !validDecision(defaultPolicy.Editable) || !validDecision(defaultPolicy.Addable) || !validDecision(defaultPolicy.Deletable) || !validDecision(defaultPolicy.Reorderable) {
		return CompiledPolicy{}, errInvalidDecision
	}
	if !validDecision(types.String) || !validDecision(types.Bool) || !validDecision(types.Integer) || !validDecision(types.Float) || !validDecision(types.Null) {
		return CompiledPolicy{}, errInvalidDecision
	}
	clonedRules := make([]Rule, len(rules))
	for index, rule := range rules {
		if !validDecision(rule.Policy.Editable) || !validDecision(rule.Policy.Addable) || !validDecision(rule.Policy.Deletable) || !validDecision(rule.Policy.Reorderable) {
			return CompiledPolicy{}, errInvalidDecision
		}
		rule.order = index
		clonedRules[index] = rule
	}
	sort.SliceStable(clonedRules, func(i, j int) bool {
		comparison := selector.CompareSpecificity(clonedRules[i].Selector, clonedRules[j].Selector)
		if comparison != 0 {
			return comparison < 0
		}
		return clonedRules[i].order < clonedRules[j].order
	})
	return CompiledPolicy{defaultPolicy: defaultPolicy, types: types, rules: clonedRules}, nil
}

// Resolve applies Default, scalar TypePolicy, then each matching Scope from
// the root to id. The bool is false if doc or id is invalid.
func Resolve(policy CompiledPolicy, doc *document.Document, id document.NodeID) (ResolvedPolicy, bool) {
	if doc == nil || id == 0 {
		return ResolvedPolicy{}, false
	}
	node, ok := doc.Node(id)
	if !ok {
		return ResolvedPolicy{}, false
	}
	resolved := ResolvedPolicy{
		Editable:    policy.defaultPolicy.Editable == Allow,
		Addable:     policy.defaultPolicy.Addable == Allow,
		Deletable:   policy.defaultPolicy.Deletable == Allow,
		Reorderable: policy.defaultPolicy.Reorderable == Allow,
	}
	if node.Kind() == document.NodeScalar {
		if typeDecision := scalarDecision(policy.types, node); typeDecision != Inherit {
			resolved.Editable = typeDecision == Allow
		}
	}
	ancestors, ok := pathFromRoot(doc, id)
	if !ok {
		return ResolvedPolicy{}, false
	}
	for _, scopeID := range ancestors {
		for _, rule := range policy.rules {
			if !selector.MatchesNode(doc, rule.Selector, scopeID) {
				continue
			}
			applyDecision(&resolved.Editable, rule.Policy.Editable)
			applyDecision(&resolved.Addable, rule.Policy.Addable)
			applyDecision(&resolved.Deletable, rule.Policy.Deletable)
			applyDecision(&resolved.Reorderable, rule.Policy.Reorderable)
		}
	}
	return resolved, true
}

func scalarDecision(types TypePolicy, node *document.Node) Decision {
	kind, ok := node.ScalarKind()
	if !ok {
		return Inherit
	}
	switch kind {
	case document.ScalarString:
		return types.String
	case document.ScalarBool:
		return types.Bool
	case document.ScalarInteger:
		return types.Integer
	case document.ScalarFloat:
		return types.Float
	case document.ScalarNull:
		return types.Null
	default:
		return Inherit
	}
}

func pathFromRoot(doc *document.Document, id document.NodeID) ([]document.NodeID, bool) {
	if _, err := doc.Path(id); err != nil {
		return nil, false
	}
	path := []document.NodeID{id}
	for current := id; current != doc.Root(); {
		parent, ok := doc.Parent(current)
		if !ok {
			return nil, false
		}
		current = parent.Parent
		path = append(path, current)
	}
	for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
		path[left], path[right] = path[right], path[left]
	}
	return path, true
}

func applyDecision(current *bool, decision Decision) {
	switch decision {
	case Allow:
		*current = true
	case Deny:
		*current = false
	}
}

func validDecision(decision Decision) bool {
	return decision == Inherit || decision == Allow || decision == Deny
}

func resolvedDefault(decision Decision) Decision {
	if decision == Inherit {
		return Deny
	}
	return decision
}
