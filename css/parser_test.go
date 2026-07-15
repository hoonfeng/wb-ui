// Translation of: Source/WebCore/css/parser/CSSParser.cpp (test portion)
// Tests for the CSS parser covering rule kinds and declaration structure.

package css

import (
	"testing"
)

func TestParser_SimpleStyleRule(t *testing.T) {
	p := NewParser("p { color: red; }")
	rules := p.ParseStyleSheet()
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	sr, ok := rules[0].(*StyleRule)
	if !ok {
		t.Fatalf("got %T, want *StyleRule", rules[0])
	}
	if sr.Selectors == nil || len(sr.Selectors.Selectors) != 1 {
		t.Fatalf("expected 1 selector, got %+v", sr.Selectors)
	}
	if got := sr.Selectors.Selectors[0].String(); got != "p" {
		t.Fatalf("selector=%q want p", got)
	}
	if len(sr.Declarations) != 1 {
		t.Fatalf("got %d declarations, want 1", len(sr.Declarations))
	}
	if sr.Declarations[0].Name != "color" {
		t.Fatalf("decl name=%q want color", sr.Declarations[0].Name)
	}
	if sr.Declarations[0].ValueString() != "red" {
		t.Fatalf("decl value=%q want red", sr.Declarations[0].ValueString())
	}
}

func TestParser_MultipleDeclarations(t *testing.T) {
	p := NewParser("p { color: red; background: blue; font-size: 12px; }")
	rules := p.ParseStyleSheet()
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	sr := rules[0].(*StyleRule)
	if len(sr.Declarations) != 3 {
		t.Fatalf("got %d decls, want 3", len(sr.Declarations))
	}
	want := []string{"color", "background", "font-size"}
	for i, w := range want {
		if sr.Declarations[i].Name != w {
			t.Fatalf("decl[%d]=%q want %q", i, sr.Declarations[i].Name, w)
		}
	}
}

func TestParser_Important(t *testing.T) {
	p := NewParser("p { color: red !important; }")
	rules := p.ParseStyleSheet()
	sr := rules[0].(*StyleRule)
	if !sr.Declarations[0].Important {
		t.Fatalf("decl not marked important")
	}
}

func TestParser_ComplexSelector(t *testing.T) {
	p := NewParser("div.foo > p:first-child { color: red; }")
	rules := p.ParseStyleSheet()
	sr := rules[0].(*StyleRule)
	got := sr.Selectors.Selectors[0].String()
	want := "div.foo > p:first-child"
	if got != want {
		t.Fatalf("selector=%q want %q", got, want)
	}
}

func TestParser_SelectorList(t *testing.T) {
	p := NewParser("h1, h2, h3 { color: red; }")
	rules := p.ParseStyleSheet()
	sr := rules[0].(*StyleRule)
	if len(sr.Selectors.Selectors) != 3 {
		t.Fatalf("got %d selectors, want 3", len(sr.Selectors.Selectors))
	}
}

func TestParser_MediaRule(t *testing.T) {
	p := NewParser("@media screen and (min-width: 600px) { p { color: red; } }")
	rules := p.ParseStyleSheet()
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	mr, ok := rules[0].(*MediaRule)
	if !ok {
		t.Fatalf("got %T, want *MediaRule", rules[0])
	}
	if mr.Condition == "" {
		t.Fatalf("empty condition")
	}
	if len(mr.Rules) != 1 {
		t.Fatalf("got %d nested rules, want 1", len(mr.Rules))
	}
}

func TestParser_ImportRule(t *testing.T) {
	p := NewParser(`@import "style.css";`)
	rules := p.ParseStyleSheet()
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	ir, ok := rules[0].(*ImportRule)
	if !ok {
		t.Fatalf("got %T, want *ImportRule", rules[0])
	}
	if ir.Href != "style.css" {
		t.Fatalf("href=%q want style.css", ir.Href)
	}
}

func TestParser_ImportRuleWithURL(t *testing.T) {
	p := NewParser(`@import url(http://example.com/style.css) screen;`)
	rules := p.ParseStyleSheet()
	ir := rules[0].(*ImportRule)
	if ir.Href != "http://example.com/style.css" {
		t.Fatalf("href=%q want http://example.com/style.css", ir.Href)
	}
	if ir.Media == "" {
		t.Fatalf("empty media")
	}
}

func TestParser_FontFaceRule(t *testing.T) {
	p := NewParser(`@font-face { font-family: "MyFont"; src: url("myfont.woff"); }`)
	rules := p.ParseStyleSheet()
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	ff, ok := rules[0].(*FontFaceRule)
	if !ok {
		t.Fatalf("got %T, want *FontFaceRule", rules[0])
	}
	if len(ff.Declarations) != 2 {
		t.Fatalf("got %d decls, want 2", len(ff.Declarations))
	}
}

func TestParser_KeyframesRule(t *testing.T) {
	p := NewParser(`@keyframes spin { from { transform: rotate(0); } to { transform: rotate(360deg); } }`)
	rules := p.ParseStyleSheet()
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	kf, ok := rules[0].(*KeyframesRule)
	if !ok {
		t.Fatalf("got %T, want *KeyframesRule", rules[0])
	}
	if kf.Name != "spin" {
		t.Fatalf("name=%q want spin", kf.Name)
	}
	if len(kf.Keyframes) != 2 {
		t.Fatalf("got %d keyframes, want 2", len(kf.Keyframes))
	}
	if kf.Keyframes[0].Keys[0] != "from" || kf.Keyframes[1].Keys[0] != "to" {
		t.Fatalf("keys=%v want [from to]", []string{kf.Keyframes[0].Keys[0], kf.Keyframes[1].Keys[0]})
	}
}

func TestParser_PageRule(t *testing.T) {
	p := NewParser(`@page { margin: 1in; }`)
	rules := p.ParseStyleSheet()
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	pg, ok := rules[0].(*PageRule)
	if !ok {
		t.Fatalf("got %T, want *PageRule", rules[0])
	}
	if len(pg.Declarations) != 1 {
		t.Fatalf("got %d decls, want 1", len(pg.Declarations))
	}
}

func TestParser_NamespaceRule(t *testing.T) {
	p := NewParser(`@namespace svg url(http://www.w3.org/2000/svg);`)
	rules := p.ParseStyleSheet()
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	ns, ok := rules[0].(*NamespaceRule)
	if !ok {
		t.Fatalf("got %T, want *NamespaceRule", rules[0])
	}
	if ns.Prefix != "svg" {
		t.Fatalf("prefix=%q want svg", ns.Prefix)
	}
	if ns.URI != "http://www.w3.org/2000/svg" {
		t.Fatalf("uri=%q want http://www.w3.org/2000/svg", ns.URI)
	}
}

func TestParser_SupportsRule(t *testing.T) {
	p := NewParser(`@supports (display: grid) { .grid { display: grid; } }`)
	rules := p.ParseStyleSheet()
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	sup, ok := rules[0].(*SupportsRule)
	if !ok {
		t.Fatalf("got %T, want *SupportsRule", rules[0])
	}
	if len(sup.Rules) != 1 {
		t.Fatalf("got %d nested rules, want 1", len(sup.Rules))
	}
}

func TestParser_MultipleRules(t *testing.T) {
	p := NewParser("p { color: red; } a { color: blue; }")
	rules := p.ParseStyleSheet()
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(rules))
	}
	if rules[0].(*StyleRule).Selectors.String() != "p" {
		t.Fatalf("first rule=%q want p", rules[0].(*StyleRule).Selectors.String())
	}
	if rules[1].(*StyleRule).Selectors.String() != "a" {
		t.Fatalf("second rule=%q want a", rules[1].(*StyleRule).Selectors.String())
	}
}

func TestParser_NestedAtRuleInsideMedia(t *testing.T) {
	p := NewParser(`@media screen { @media (max-width: 600px) { p { color: red; } } }`)
	rules := p.ParseStyleSheet()
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	mr := rules[0].(*MediaRule)
	if len(mr.Rules) != 1 {
		t.Fatalf("expected 1 nested rule")
	}
	if _, ok := mr.Rules[0].(*MediaRule); !ok {
		t.Fatalf("nested rule=%T want *MediaRule", mr.Rules[0])
	}
}

func TestParser_DeclarationList(t *testing.T) {
	p := NewParser("color: red; background: blue")
	decls := p.ParseDeclarationList()
	if len(decls) != 2 {
		t.Fatalf("got %d decls, want 2", len(decls))
	}
	if decls[0].Name != "color" || decls[1].Name != "background" {
		t.Fatalf("decl order wrong: %v %v", decls[0].Name, decls[1].Name)
	}
}

func TestParser_CommentRecovery(t *testing.T) {
	p := NewParser("/* comment */ p { color: red; /* inline */ }")
	rules := p.ParseStyleSheet()
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
}

func TestParser_UnknownAtRuleSkipped(t *testing.T) {
	p := NewParser("@unknown-thing { color: red; } p { color: blue; }")
	rules := p.ParseStyleSheet()
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	if _, ok := rules[0].(*StyleRule); !ok {
		t.Fatalf("got %T, want *StyleRule", rules[0])
	}
}

func TestParser_CommaSelector(t *testing.T) {
	// Test the four combinator types in one selector list.
	cases := []string{
		"div p",        // descendant
		"div > p",      // child
		"div + p",      // adjacent sibling
		"div ~ p",      // general sibling
		"div, p, span", // selector list
	}
	for _, c := range cases {
		p := NewParser(c + " { color: red; }")
		rules := p.ParseStyleSheet()
		if len(rules) != 1 {
			t.Fatalf("%q: got %d rules, want 1", c, len(rules))
		}
		sr := rules[0].(*StyleRule)
		if len(sr.Selectors.Selectors) == 0 {
			t.Fatalf("%q: empty selector list", c)
		}
	}
}

func TestParser_AnB(t *testing.T) {
	cases := []struct {
		input string
		a, b  int
	}{
		{"odd", 2, 1},
		{"even", 2, 0},
		{"3", 0, 3},
		{"n", 1, 0},
		{"2n", 2, 0},
		{"2n+1", 2, 1},
		{"-n+3", -1, 3},
		{"n+2", 1, 2},
	}
	for _, c := range cases {
		a, b := parseAnB([]Token{{Type: TokenIdent, Value: c.input}})
		if a != c.a || b != c.b {
			t.Errorf("%q: got (%d,%d) want (%d,%d)", c.input, a, b, c.a, c.b)
		}
	}
}

func TestParser_CSSNthChild(t *testing.T) {
	p := NewParser("li:nth-child(2n+1) { color: red; }")
	rules := p.ParseStyleSheet()
	sr := rules[0].(*StyleRule)
	sel := sr.Selectors.Selectors[0]
	if len(sel.Compounds) != 1 || len(sel.Compounds[0].Selectors) != 2 {
		t.Fatalf("selector=%+v", sel)
	}
	ss := sel.Compounds[0].Selectors[1]
	if ss.PseudoClass != PseudoClassNthChild {
		t.Fatalf("pseudo-class=%v want nth-child", ss.PseudoClass)
	}
	if ss.NthA != 2 || ss.NthB != 1 {
		t.Fatalf("nth=(%d,%d) want (2,1)", ss.NthA, ss.NthB)
	}
}

func TestParser_NotPseudoClass(t *testing.T) {
	p := NewParser("p:not(.active) { color: red; }")
	rules := p.ParseStyleSheet()
	sr := rules[0].(*StyleRule)
	sel := sr.Selectors.Selectors[0]
	ss := sel.Compounds[0].Selectors[1]
	if ss.PseudoClass != PseudoClassNot {
		t.Fatalf("pseudo=%v want not", ss.PseudoClass)
	}
	if ss.SelectorList == nil || len(ss.SelectorList.Selectors) != 1 {
		t.Fatalf("expected 1 sub-selector")
	}
}

func TestParser_AttributeSelector(t *testing.T) {
	cases := []struct {
		input string
		match Match
	}{
		{`[attr]`, MatchSet},
		{`[attr="val"]`, MatchExact},
		{`[attr~="val"]`, MatchList},
		{`[attr|="val"]`, MatchHyphen},
		{`[attr^="val"]`, MatchBegin},
		{`[attr$="val"]`, MatchEnd},
		{`[attr*="val"]`, MatchContain},
	}
	for _, c := range cases {
		p := NewParser("p" + c.input + " { color: red; }")
		rules := p.ParseStyleSheet()
		if len(rules) != 1 {
			t.Fatalf("%q: got %d rules, want 1", c.input, len(rules))
		}
		sr := rules[0].(*StyleRule)
		sel := sr.Selectors.Selectors[0]
		var ss SimpleSelector
		for _, s := range sel.Compounds[0].Selectors {
			if s.Match == c.match {
				ss = s
				break
			}
		}
		if ss.Attribute != "attr" {
			t.Fatalf("%q: attribute=%q want attr", c.input, ss.Attribute)
		}
	}
}

func TestParser_PseudoElement(t *testing.T) {
	cases := []string{"before", "after", "first-line", "first-letter", "placeholder", "selection"}
	for _, c := range cases {
		p := NewParser("p::" + c + " { color: red; }")
		rules := p.ParseStyleSheet()
		if len(rules) != 1 {
			t.Fatalf("%q: got %d rules, want 1", c, len(rules))
		}
		sr := rules[0].(*StyleRule)
		sel := sr.Selectors.Selectors[0]
		found := false
		for _, s := range sel.Compounds[0].Selectors {
			if s.Match == MatchPseudoElement {
				found = true
				if s.PseudoElem == PseudoElementUnknown {
					t.Fatalf("%q: unknown pseudo-element", c)
				}
			}
		}
		if !found {
			t.Fatalf("%q: no pseudo-element in selector", c)
		}
	}
}

func TestParser_VarFunction(t *testing.T) {
	p := NewParser("p { color: var(--main-color); }")
	rules := p.ParseStyleSheet()
	sr := rules[0].(*StyleRule)
	if len(sr.Declarations) != 1 {
		t.Fatalf("got %d decls, want 1", len(sr.Declarations))
	}
	// The value should contain a function token for var().
	found := false
	for _, tok := range sr.Declarations[0].Value {
		if tok.Type == TokenFunction && tok.Value == "var" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("var() not found in value: %v", sr.Declarations[0].Value)
	}
}
