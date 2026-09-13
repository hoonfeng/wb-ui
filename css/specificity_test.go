// Translation of: Source/WebCore/css/CSSSelector.cpp (specificity test portion)
// Tests for the (a,b,c) specificity tuple: Add, Compare, and the per-selector
// SpecificityOfComplex / SpecificityOfList calculators.

package css

import "testing"

func TestSpecificity_Add(t *testing.T) {
	a := Specificity{A: 1, B: 2, C: 3}
	b := Specificity{A: 4, B: 5, C: 6}
	got := a.Add(b)
	want := Specificity{A: 5, B: 7, C: 9}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestSpecificity_Compare(t *testing.T) {
	cases := []struct {
		name string
		a, b Specificity
		want int // -1, 0, +1
	}{
		{"less by A", Specificity{0, 5, 5}, Specificity{1, 0, 0}, -1},
		{"less by B when A equal", Specificity{1, 0, 5}, Specificity{1, 1, 0}, -1},
		{"less by C when A,B equal", Specificity{1, 1, 0}, Specificity{1, 1, 1}, -1},
		{"equal", Specificity{1, 2, 3}, Specificity{1, 2, 3}, 0},
		{"greater by A", Specificity{2, 0, 0}, Specificity{1, 99, 99}, 1},
		{"greater by B", Specificity{1, 2, 0}, Specificity{1, 1, 99}, 1},
		{"greater by C", Specificity{1, 1, 2}, Specificity{1, 1, 1}, 1},
	}
	for _, c := range cases {
		got := c.a.Compare(c.b)
		if got != c.want {
			t.Errorf("%s: %v.Compare(%v) = %d, want %d", c.name, c.a, c.b, got, c.want)
		}
	}
}

func TestSpecificity_String(t *testing.T) {
	s := Specificity{A: 1, B: 2, C: 3}
	if got := s.String(); got != "(1,2,3)" {
		t.Fatalf("got %q want (1,2,3)", got)
	}
}

func TestSpecificity_TagSelector(t *testing.T) {
	// "div" → (0,0,1)
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{{Match: MatchTag, Value: "div"}}},
		},
	}
	got := SpecificityOfComplex(sel)
	want := Specificity{A: 0, B: 0, C: 1}
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSpecificity_ClassSelector(t *testing.T) {
	// ".foo" → (0,1,0)
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{{Match: MatchClass, Value: "foo"}}},
		},
	}
	got := SpecificityOfComplex(sel)
	want := Specificity{A: 0, B: 1, C: 0}
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSpecificity_IDSelector(t *testing.T) {
	// "#main" → (1,0,0)
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{{Match: MatchID, Value: "main"}}},
		},
	}
	got := SpecificityOfComplex(sel)
	want := Specificity{A: 1, B: 0, C: 0}
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSpecificity_CompoundSelector(t *testing.T) {
	// "div.foo#main" → (1,1,1)
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{
				{Match: MatchTag, Value: "div"},
				{Match: MatchClass, Value: "foo"},
				{Match: MatchID, Value: "main"},
			}},
		},
	}
	got := SpecificityOfComplex(sel)
	want := Specificity{A: 1, B: 1, C: 1}
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSpecificity_AttributeAndPseudoClass(t *testing.T) {
	// "a[href][disabled]:hover" → (0,3,1)
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{
				{Match: MatchTag, Value: "a"},
				{Match: MatchSet, Attribute: "href"},
				{Match: MatchSet, Attribute: "disabled"},
				{Match: MatchPseudoClass, PseudoClass: PseudoClassHover},
			}},
		},
	}
	got := SpecificityOfComplex(sel)
	// a=0 (no ID), b=3 (2 attributes + 1 pseudo-class), c=1 (1 type)
	want := Specificity{A: 0, B: 3, C: 1}
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSpecificity_PseudoElement(t *testing.T) {
	// "p::before" → (0,0,2)
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{
				{Match: MatchTag, Value: "p"},
				{Match: MatchPseudoElement, PseudoElem: PseudoElementBefore},
			}},
		},
	}
	got := SpecificityOfComplex(sel)
	want := Specificity{A: 0, B: 0, C: 2}
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSpecificity_DescendantCombinator(t *testing.T) {
	// "div#main ul li.foo" → (1,1,3)
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{
				{Match: MatchTag, Value: "div"},
				{Match: MatchID, Value: "main"},
			}},
			{
				Relation: RelationDescendant,
				Selectors: []SimpleSelector{{Match: MatchTag, Value: "ul"}},
			},
			{
				Relation: RelationDescendant,
				Selectors: []SimpleSelector{
					{Match: MatchTag, Value: "li"},
					{Match: MatchClass, Value: "foo"},
				},
			},
		},
	}
	got := SpecificityOfComplex(sel)
	want := Specificity{A: 1, B: 1, C: 3}
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSpecificity_WherePseudoClass(t *testing.T) {
	// ":where(div, span) p" → (0,0,1) — :where contributes zero
	inner := &SelectorList{
		Selectors: []ComplexSelector{
			{Compounds: []CompoundSelector{{Selectors: []SimpleSelector{{Match: MatchTag, Value: "div"}}}}},
			{Compounds: []CompoundSelector{{Selectors: []SimpleSelector{{Match: MatchTag, Value: "span"}}}}},
		},
	}
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{{
				Match:       MatchPseudoClass,
				PseudoClass: PseudoClassWhere,
				SelectorList: inner,
			}}},
			{
				Relation: RelationDescendant,
				Selectors: []SimpleSelector{{Match: MatchTag, Value: "p"}},
			},
		},
	}
	got := SpecificityOfComplex(sel)
	want := Specificity{A: 0, B: 0, C: 1}
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSpecificity_NotPseudoClass(t *testing.T) {
	// ":not(.foo.bar) p" → (0,2,1) — :not takes max of its argument
	inner := &SelectorList{
		Selectors: []ComplexSelector{
			{Compounds: []CompoundSelector{{Selectors: []SimpleSelector{
				{Match: MatchClass, Value: "foo"},
				{Match: MatchClass, Value: "bar"},
			}}}},
		},
	}
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{{
				Match:       MatchPseudoClass,
				PseudoClass: PseudoClassNot,
				SelectorList: inner,
			}}},
			{
				Relation: RelationDescendant,
				Selectors: []SimpleSelector{{Match: MatchTag, Value: "p"}},
			},
		},
	}
	got := SpecificityOfComplex(sel)
	want := Specificity{A: 0, B: 2, C: 1}
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSpecificity_ListTakesMax(t *testing.T) {
	// Selector list "div, #main, .foo" — list specificity should be max = (1,0,0)
	list := &SelectorList{
		Selectors: []ComplexSelector{
			{Compounds: []CompoundSelector{{Selectors: []SimpleSelector{{Match: MatchTag, Value: "div"}}}}},
			{Compounds: []CompoundSelector{{Selectors: []SimpleSelector{{Match: MatchID, Value: "main"}}}}},
			{Compounds: []CompoundSelector{{Selectors: []SimpleSelector{{Match: MatchClass, Value: "foo"}}}}},
		},
	}
	got := SpecificityOfList(list)
	want := Specificity{A: 1, B: 0, C: 0}
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSpecificity_NthChildWithOfClause(t *testing.T) {
	// :nth-child(2n of .item) → base (0,1,0) + max of (.item) = (0,2,0)
	inner := &SelectorList{
		Selectors: []ComplexSelector{
			{Compounds: []CompoundSelector{{Selectors: []SimpleSelector{{Match: MatchClass, Value: "item"}}}}},
		},
	}
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{{
				Match:       MatchPseudoClass,
				PseudoClass: PseudoClassNthChild,
				NthA:        2,
				NthB:        0,
				SelectorList: inner,
			}}},
		},
	}
	got := SpecificityOfComplex(sel)
	want := Specificity{A: 0, B: 2, C: 0}
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSpecificity_UniversalSelectorContributesZero(t *testing.T) {
	// Universal selector "*" is modeled as MatchTag with value "*"; per Selectors
	// Level 3 it contributes (0,0,0). It used to count as a type selector
	// (0,0,1), which let `*{...}` tie with and (via source order) override
	// type-selector rules such as `html{box-sizing:border-box}`.
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{{Match: MatchTag, Value: "*"}}},
		},
	}
	got := SpecificityOfComplex(sel)
	want := Specificity{A: 0, B: 0, C: 0}
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}

	// A compound of "*" plus a class must still contribute the class (0,1,0).
	sel2 := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{
				{Match: MatchTag, Value: "*"},
				{Match: MatchClass, Value: "box"},
			}},
		},
	}
	if got := SpecificityOfComplex(sel2); got != (Specificity{B: 1}) {
		t.Fatalf("* .box: got %v want (0,1,0)", got)
	}
}
