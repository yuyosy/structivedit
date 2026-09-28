package structivedit

import (
	"github.com/yuyosy/structivedit/internal/resolver"
)

// Decision is a Policy permission decision. Inherit keeps the current value.
type Decision uint8

const (
	Inherit Decision = iota
	Allow
	Deny
)

// ScopePolicy controls operations for a Selector scope.
type ScopePolicy struct {
	Editable    Decision
	Addable     Decision
	Deletable   Decision
	Reorderable Decision
}

// Rule applies a ScopePolicy at every location matching Selector.
type Rule struct {
	Selector Selector
	Policy   ScopePolicy
}

// TypePolicy controls scalar editing by ScalarKind. Structural permissions
// are controlled by ScopePolicy only.
type TypePolicy struct {
	String  Decision
	Bool    Decision
	Integer Decision
	Float   Decision
	Null    Decision
}

// Policy is the immutable-at-use configuration for operation permissions.
// Decisions omitted from Default resolve to Deny.
type Policy struct {
	Default ScopePolicy
	Rules   []Rule
	Types   TypePolicy
}

func (p Policy) compile() (resolver.CompiledPolicy, error) {
	rules := make([]resolver.Rule, len(p.Rules))
	for index, rule := range p.Rules {
		rules[index] = resolver.Rule{
			Selector: rule.Selector.pattern,
			Policy:   toResolverScopePolicy(rule.Policy),
		}
	}
	compiled, err := resolver.Compile(
		toResolverScopePolicy(p.Default),
		resolver.TypePolicy{
			String:  resolver.Decision(p.Types.String),
			Bool:    resolver.Decision(p.Types.Bool),
			Integer: resolver.Decision(p.Types.Integer),
			Float:   resolver.Decision(p.Types.Float),
			Null:    resolver.Decision(p.Types.Null),
		},
		rules,
	)
	if err != nil {
		return resolver.CompiledPolicy{}, ErrInvalidPolicy
	}
	return compiled, nil
}

func toResolverScopePolicy(policy ScopePolicy) resolver.ScopePolicy {
	return resolver.ScopePolicy{
		Editable:    resolver.Decision(policy.Editable),
		Addable:     resolver.Decision(policy.Addable),
		Deletable:   resolver.Decision(policy.Deletable),
		Reorderable: resolver.Decision(policy.Reorderable),
	}
}
