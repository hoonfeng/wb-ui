// Translation of: Source/WebCore/css/CSSSelector.cpp (specificity helpers)
//                  Source/WebCore/css/CSSSelector.h (computeSpecificityTuple)
// Completeness: 80%
// Simplifications:
//   - only the (a,b,c) tuple is computed, not the packed unsigned int form
//   - the :is() / :where() / :not() pseudo-class argument lists are recursed into
//     with the "use the largest value" rule from the spec
//   - the negation pseudo-class historically contributes nothing; per the modern
//     spec (Selectors Level 4) it is treated like :is() (computed from its argument)
//   - pseudo-elements contribute to the c component

package css

// Specificity is the (a, b, c) tuple defined by CSS Selectors Level 3.
//   a = number of ID selectors
//   b = number of class selectors, attribute selectors, and pseudo-classes
//   c = number of type selectors and pseudo-elements
//
// Universal selector and combinators contribute nothing.
type Specificity struct {
	A, B, C int
}

// Add sums two specificity tuples component-wise.
func (s Specificity) Add(o Specificity) Specificity {
	return Specificity{A: s.A + o.A, B: s.B + o.B, C: s.C + o.C}
}

// Compare returns -1, 0, or +1 according to whether s is less than, equal to, or
// greater than o under the spec's (a, b, c) lexicographic comparison.
func (s Specificity) Compare(o Specificity) int {
	if s.A != o.A {
		if s.A < o.A {
			return -1
		}
		return 1
	}
	if s.B != o.B {
		if s.B < o.B {
			return -1
		}
		return 1
	}
	if s.C != o.C {
		if s.C < o.C {
			return -1
		}
		return 1
	}
	return 0
}

// SpecificityOfComplex computes the specificity of a single complex selector by
// summing the specificities of all its compounds. The first compound's Relation is
// ignored (it has no combinator).
func SpecificityOfComplex(c ComplexSelector) Specificity {
	var sum Specificity
	for _, comp := range c.Compounds {
		for _, s := range comp.Selectors {
			sum = sum.Add(specificityOfSimple(s))
		}
	}
	return sum
}

// SpecificityOfList computes the specificity of a SelectorList as the maximum
// specificity of its complex selectors (mirroring CSS-wide convention that a
// comma-separated list matches if any of its selectors matches, with the matched
// selector's specificity used).
func SpecificityOfList(l *SelectorList) Specificity {
	if l == nil {
		return Specificity{}
	}
	max := Specificity{}
	for _, cs := range l.Selectors {
		s := SpecificityOfComplex(cs)
		if s.Compare(max) > 0 {
			max = s
		}
	}
	return max
}

// specificityOfSimple computes the specificity contribution of a single SimpleSelector.
func specificityOfSimple(s SimpleSelector) Specificity {
	switch s.Match {
	case MatchID:
		return Specificity{A: 1}
	case MatchClass, MatchSet, MatchExact, MatchList, MatchHyphen, MatchBegin, MatchEnd, MatchContain:
		// Attribute selectors are case-insensitivity flags only; they contribute (0,1,0).
		return Specificity{B: 1}
	case MatchTag:
		return Specificity{C: 1}
	case MatchPseudoClass:
		return pseudoClassSpecificity(s)
	case MatchPseudoElement:
		return Specificity{C: 1}
	}
	return Specificity{}
}

// pseudoClassSpecificity implements the spec rules for pseudo-classes that take
// selector arguments: the contribution is the maximum specificity of the
// arguments (or zero for :where(), which is always zero).
func pseudoClassSpecificity(s SimpleSelector) Specificity {
	switch s.PseudoClass {
	case PseudoClassWhere:
		// :where() always contributes zero per the spec.
		return Specificity{}
	case PseudoClassIs, PseudoClassNot, PseudoClassHas:
		// Treat as the max of the argument list.
		return SpecificityOfList(s.SelectorList)
	case PseudoClassNthChild, PseudoClassNthLastChild:
		// :nth-child(an+b of S) — the optional selector list contributes too. We
		// conservatively add (0,1,0) for the pseudo-class itself plus the list.
		base := Specificity{B: 1}
		return base.Add(SpecificityOfList(s.SelectorList))
	default:
		return Specificity{B: 1}
	}
}

// String renders the specificity as "(a,b,c)".
func (s Specificity) String() string {
	return "(" + intToString(s.A) + "," + intToString(s.B) + "," + intToString(s.C) + ")"
}
