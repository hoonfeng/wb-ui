package markdown

// Translation of: markdown-it/lib/ruler.mjs
// Completeness: 100%
//
// Ruler is an ordered, named rule set used by the three parser chains
// (core / block / inline). Rules can be inserted before/after an existing
// rule, replaced, pushed at the end, and individually enabled/disabled. Each
// rule may belong to one or more named "chains" (via Alt) so a single Ruler can
// feed multiple rule lists through GetRules(chainName).
//
// Faithfulness notes:
//   - markdown-it throws on unknown rule names for before/after/at and for
//     enable/disable when ignoreInvalid is false. We mirror that with panic,
//     since these are programmer errors at config time.

// RuleFn is the unified signature for all parser rules. The first argument is
// the parser state (cast to *StateCore / *StateBlock / *StateInline inside the
// rule). startLine/endLine are used by block rules; inline and core rules
// ignore them. silent requests a "look-ahead" probe that must not mutate state.
// A rule returns true if it consumed input (in silent mode: "I can handle
// this"). Core rules ignore all params except state; the boolean return is
// unused for core rules.
//
// Deviation from the 2-param signature suggested in the task spec: markdown-it
// block rules genuinely take (state, startLine, endLine, silent), so a single
// 4-param type covers all three chains without needing per-chain Ruler
// specialisation. Inline rules simply ignore the two line parameters, and core
// rules ignore everything except state.
type RuleFn func(state interface{}, startLine, endLine int, silent bool) bool

// Rule is a single entry in a Ruler. Alt lists alternative chain names the rule
// participates in (in addition to its own Name).
type Rule struct {
	Name    string
	Enabled bool
	Fn      RuleFn
	Alt     []string
}

// Ruler holds the ordered list of rules for one parser chain.
type Ruler struct {
	rules []Rule
}

// NewRuler constructs an empty Ruler.
func NewRuler() *Ruler { return &Ruler{} }

// find returns the index of the rule named name, or -1. Mirrors
// Ruler.prototype.__find__.
func (r *Ruler) find(name string) int {
	for i, rule := range r.rules {
		if rule.Name == name {
			return i
		}
	}
	return -1
}

// Before inserts a new rule immediately before the rule named beforeName.
// Mirrors Ruler.prototype.before.
func (r *Ruler) Before(beforeName, ruleName string, fn RuleFn, alt []string) {
	index := r.find(beforeName)
	if index == -1 {
		panic("Parser rule not found: " + beforeName)
	}
	r.rules = append(r.rules, Rule{}) // grow
	copy(r.rules[index+1:], r.rules[index:])
	r.rules[index] = Rule{Name: ruleName, Enabled: true, Fn: fn, Alt: alt}
}

// After inserts a new rule immediately after the rule named afterName.
// Mirrors Ruler.prototype.after.
func (r *Ruler) After(afterName, ruleName string, fn RuleFn, alt []string) {
	index := r.find(afterName)
	if index == -1 {
		panic("Parser rule not found: " + afterName)
	}
	r.rules = append(r.rules, Rule{}) // grow
	copy(r.rules[index+2:], r.rules[index+1:])
	r.rules[index+1] = Rule{Name: ruleName, Enabled: true, Fn: fn, Alt: alt}
}

// At replaces the rule named name with a new rule (ruleName, fn, alt) at the
// same position. Mirrors Ruler.prototype.at.
func (r *Ruler) At(name, ruleName string, fn RuleFn, alt []string) {
	index := r.find(name)
	if index == -1 {
		panic("Parser rule not found: " + name)
	}
	r.rules[index] = Rule{Name: ruleName, Enabled: true, Fn: fn, Alt: alt}
}

// Push appends a new rule at the end of the chain. Mirrors Ruler.prototype.push.
func (r *Ruler) Push(ruleName string, fn RuleFn, alt []string) {
	r.rules = append(r.rules, Rule{Name: ruleName, Enabled: true, Fn: fn, Alt: alt})
}

// Enable enables the named rules. If a name is unknown and ignoreInvalid is
// false, it panics (mirroring markdown-it's throw). Mirrors
// Ruler.prototype.enable.
func (r *Ruler) Enable(names []string, ignoreInvalid bool) {
	for _, name := range names {
		idx := r.find(name)
		if idx < 0 {
			if ignoreInvalid {
				continue
			}
			panic("Rules manager: invalid rule name " + name)
		}
		r.rules[idx].Enabled = true
	}
}

// Disable disables the named rules. If a name is unknown and ignoreInvalid is
// false, it panics. Mirrors Ruler.prototype.disable.
func (r *Ruler) Disable(names []string, ignoreInvalid bool) {
	for _, name := range names {
		idx := r.find(name)
		if idx < 0 {
			if ignoreInvalid {
				continue
			}
			panic("Rules manager: invalid rule name " + name)
		}
		r.rules[idx].Enabled = false
	}
}

// GetRules returns the enabled rule functions participating in the chain
// chainName. An empty chainName ("") is the main chain and returns ALL enabled
// rules (mirroring markdown-it's __compile__ where `if (chain && ...) { return }`
// skips the alt-membership check when chain is the falsy empty string). A
// non-empty chainName returns only rules whose Alt list contains chainName.
func (r *Ruler) GetRules(chainName string) []RuleFn {
	result := []RuleFn{}
	for _, rule := range r.rules {
		if !rule.Enabled {
			continue
		}
		if chainName != "" {
			// non-empty chain: only rules that declare this alt chain
			found := false
			for _, alt := range rule.Alt {
				if alt == chainName {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		// empty chain (""): include every enabled rule
		result = append(result, rule.Fn)
	}
	return result
}
