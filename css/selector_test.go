// Translation of: Source/WebCore/css/CSSSelector.cpp (test portion)
//                  Source/WebCore/css/SelectorChecker.cpp (test portion)
// Tests for CSSSelector data structures, String() rendering, pseudo-class/element
// lookup, and the SelectorChecker right-to-left match algorithm.

package css

import (
	"testing"

	"wb-ui/dom"
)

// --- SimpleSelector / ComplexSelector String() -------------------------------

func TestSelector_SimpleTagString(t *testing.T) {
	s := SimpleSelector{Match: MatchTag, Value: "div"}
	if got := s.String(); got != "div" {
		t.Fatalf("got %q want div", got)
	}
}

func TestSelector_SimpleIDString(t *testing.T) {
	s := SimpleSelector{Match: MatchID, Value: "main"}
	if got := s.String(); got != "#main" {
		t.Fatalf("got %q want #main", got)
	}
}

func TestSelector_SimpleClassString(t *testing.T) {
	s := SimpleSelector{Match: MatchClass, Value: "button"}
	if got := s.String(); got != ".button" {
		t.Fatalf("got %q want .button", got)
	}
}

func TestSelector_AttributeSetString(t *testing.T) {
	s := SimpleSelector{Match: MatchSet, Attribute: "disabled"}
	if got := s.String(); got != "[disabled]" {
		t.Fatalf("got %q want [disabled]", got)
	}
}

func TestSelector_AttributeExactString(t *testing.T) {
	s := SimpleSelector{Match: MatchExact, Attribute: "type", Value: "text"}
	if got := s.String(); got != `[type="text"]` {
		t.Fatalf("got %q want [type=\"text\"]", got)
	}
}

func TestSelector_PseudoClassString(t *testing.T) {
	s := SimpleSelector{Match: MatchPseudoClass, PseudoClass: PseudoClassHover}
	if got := s.String(); got != ":hover" {
		t.Fatalf("got %q want :hover", got)
	}
}

func TestSelector_PseudoElementString(t *testing.T) {
	s := SimpleSelector{Match: MatchPseudoElement, PseudoElem: PseudoElementBefore}
	if got := s.String(); got != "::before" {
		t.Fatalf("got %q want ::before", got)
	}
}

func TestSelector_NthChildString(t *testing.T) {
	s := SimpleSelector{
		Match:       MatchPseudoClass,
		PseudoClass: PseudoClassNthChild,
		NthA:        2,
		NthB:        1,
	}
	if got := s.String(); got != ":nth-child(2n+1)" {
		t.Fatalf("got %q want :nth-child(2n+1)", got)
	}
}

func TestSelector_ComplexSelectorString(t *testing.T) {
	// div.foo > p:first-child
	cs := ComplexSelector{
		Compounds: []CompoundSelector{
			{
				Selectors: []SimpleSelector{
					{Match: MatchTag, Value: "div"},
					{Match: MatchClass, Value: "foo"},
				},
			},
			{
				Relation: RelationChild,
				Selectors: []SimpleSelector{
					{Match: MatchTag, Value: "p"},
					{Match: MatchPseudoClass, PseudoClass: PseudoClassFirstChild},
				},
			},
		},
	}
	if got := cs.String(); got != "div.foo > p:first-child" {
		t.Fatalf("got %q want div.foo > p:first-child", got)
	}
}

func TestSelector_ListString(t *testing.T) {
	list := &SelectorList{
		Selectors: []ComplexSelector{
			{Compounds: []CompoundSelector{{Selectors: []SimpleSelector{{Match: MatchTag, Value: "h1"}}}}},
			{Compounds: []CompoundSelector{{Selectors: []SimpleSelector{{Match: MatchTag, Value: "h2"}}}}},
		},
	}
	if got := list.String(); got != "h1, h2" {
		t.Fatalf("got %q want \"h1, h2\"", got)
	}
}

// --- Lookup helpers ----------------------------------------------------------

func TestSelector_LookupPseudoClass(t *testing.T) {
	cases := map[string]PseudoClass{
		"hover":         PseudoClassHover,
		"focus":         PseudoClassFocus,
		"first-child":   PseudoClassFirstChild,
		"nth-child":     PseudoClassNthChild,
		"not":           PseudoClassNot,
		"placeholder-shown": PseudoClassPlaceholderShown,
		// HTML/Fullscreen 状态类（本轮新增，`:modal` 由 dom 的模态状态消费）
		"fullscreen":    PseudoClassFullscreen,
		"open":          PseudoClassOpen,
		"closed":        PseudoClassClosed,
		"modal":         PseudoClassModal,
	}
	for name, want := range cases {
		if got := LookupPseudoClass(name); got != want {
			t.Errorf("%q: got %v want %v", name, got, want)
		}
	}
	// 名称大小写不敏感（解析路径先 ToLower，查询路径同样要容错）。
	if got := LookupPseudoClass("MODAL"); got != PseudoClassModal {
		t.Errorf("LookupPseudoClass(\"MODAL\") = %v want PseudoClassModal", got)
	}
	if got := LookupPseudoClass("nonexistent"); got != PseudoClassUnknown {
		t.Fatalf("unknown pseudo-class got %v want PseudoClassUnknown", got)
	}
}

func TestSelector_LookupPseudoElement(t *testing.T) {
	cases := map[string]PseudoElement{
		"before":       PseudoElementBefore,
		"after":        PseudoElementAfter,
		"first-line":   PseudoElementFirstLine,
		"first-letter": PseudoElementFirstLetter,
		"selection":    PseudoElementSelection,
		// 模态 dialog 的遮罩层（rendering 依赖它生成伪元素盒）
		"backdrop":     PseudoElementBackdrop,
		"marker":       PseudoElementMarker,
		"placeholder":  PseudoElementPlaceholder,
		"cue":          PseudoElementCue,
		"slotted":      PseudoElementSlotted,
		"part":         PseudoElementPart,
		"highlight":    PseudoElementHighlight,
		"-webkit-scrollbar":       PseudoElementWebkitScrollbar,
		"-webkit-scrollbar-thumb": PseudoElementWebkitScrollbarThumb,
		"-webkit-scrollbar-track": PseudoElementWebkitScrollbarTrack,
	}
	for name, want := range cases {
		if got := LookupPseudoElement(name); got != want {
			t.Errorf("%q: got %v want %v", name, got, want)
		}
	}
	if got := LookupPseudoElement("nonexistent-pseudo"); got != PseudoElementUnknown {
		t.Fatalf("unknown pseudo-element got %v want PseudoElementUnknown", got)
	}
}

// TestSelector_PseudoNameRoundTrip 锁死三张表的一致性：枚举 ↔ PseudoClassName /
// PseudoElementName ↔ LookupPseudoClass / LookupPseudoElement。
//
// 这个测试来自一次真实缺口：PseudoElementWebkitScrollbar{,-Thumb,-Track} 在
// LookupPseudoElement 里有条目、但 PseudoElementName 漏了三项，于是
// `::-webkit-scrollbar { ... }` 经 SimpleSelector.String() 序列化后变成裸 "::"
// （调试输出、规则键、去重判断都会读到错的名字）。任何一侧新增枚举而漏另一侧，
// 这里都会立刻失败。
func TestSelector_PseudoNameRoundTrip(t *testing.T) {
	pseudoClasses := []PseudoClass{
		PseudoClassHover, PseudoClassFocus, PseudoClassFocusVisible, PseudoClassFocusWithin,
		PseudoClassActive, PseudoClassVisited, PseudoClassLink, PseudoClassAnyLink,
		PseudoClassTarget, PseudoClassEmpty, PseudoClassRoot, PseudoClassScope,
		PseudoClassFirstChild, PseudoClassLastChild, PseudoClassOnlyChild,
		PseudoClassFirstOfType, PseudoClassLastOfType, PseudoClassOnlyOfType,
		PseudoClassChecked, PseudoClassDisabled, PseudoClassEnabled, PseudoClassPlaceholderShown,
		PseudoClassReadOnly, PseudoClassReadWrite, PseudoClassRequired, PseudoClassOptional,
		PseudoClassValid, PseudoClassInvalid, PseudoClassInRange, PseudoClassOutOfRange,
		PseudoClassDefault, PseudoClassIndeterminate,
		PseudoClassNthChild, PseudoClassNthLastChild, PseudoClassNthOfType, PseudoClassNthLastOfType,
		PseudoClassNot, PseudoClassIs, PseudoClassWhere, PseudoClassHas,
		PseudoClassLang, PseudoClassDir, PseudoClassDefined,
		PseudoClassHost, PseudoClassHostContext,
		PseudoClassFullscreen, PseudoClassOpen, PseudoClassClosed, PseudoClassModal,
	}
	for _, p := range pseudoClasses {
		name := PseudoClassName(p)
		if name == "" {
			t.Errorf("PseudoClassName(%d) 为空：枚举新增后忘了补名称表", p)
			continue
		}
		if got := LookupPseudoClass(name); got != p {
			t.Errorf("往返不一致：%d → %q → %d", p, name, got)
		}
		// 序列化（含参数形式）不得丢掉名字。
		sel := SimpleSelector{Match: MatchPseudoClass, PseudoClass: p}
		want := ":" + name
		if p == PseudoClassNthChild {
			// :nth-child 必定输出参数括号；空参数（零值 AnB）序列化为 (0)
			// （永不匹配，与浏览器把无效 :nth-child() 当无效选择器的效果一致）。
			want += "(0)"
		}
		if got := sel.String(); got != want {
			t.Errorf("序列化 %d = %q want %q", p, got, want)
		}
	}
	if got := PseudoClassName(PseudoClassUnknown); got != "" {
		t.Errorf("PseudoClassName(Unknown) = %q want 空串", got)
	}

	pseudoElements := []PseudoElement{
		PseudoElementBefore, PseudoElementAfter, PseudoElementFirstLine, PseudoElementFirstLetter,
		PseudoElementMarker, PseudoElementPlaceholder, PseudoElementSelection, PseudoElementBackdrop,
		PseudoElementCue, PseudoElementSlotted, PseudoElementPart, PseudoElementHighlight,
		PseudoElementViewTransition, PseudoElementViewTransitionGroup,
		PseudoElementViewTransitionImagePair, PseudoElementViewTransitionOld,
		PseudoElementViewTransitionNew,
		PseudoElementWebkitScrollbar, PseudoElementWebkitScrollbarThumb, PseudoElementWebkitScrollbarTrack,
	}
	for _, p := range pseudoElements {
		name := PseudoElementName(p)
		if name == "" {
			t.Errorf("PseudoElementName(%d) 为空：枚举新增后忘了补名称表", p)
			continue
		}
		if got := LookupPseudoElement(name); got != p {
			t.Errorf("往返不一致：%d → %q → %d", p, name, got)
		}
		sel := SimpleSelector{Match: MatchPseudoElement, PseudoElem: p}
		if got, want := sel.String(), "::"+name; got != want {
			t.Errorf("序列化 %d = %q want %q", p, got, want)
		}
	}
	if got := PseudoElementName(PseudoElementUnknown); got != "" {
		t.Errorf("PseudoElementName(Unknown) = %q want 空串", got)
	}
}

// TestSelector_ParseModalAndBackdrop 解析路径：`:modal` 与 `::backdrop` 必须按
// 冒号数量分流（单冒号 → 伪类，双冒号 → 伪元素），不能互相串。
func TestSelector_ParseModalAndBackdrop(t *testing.T) {
	m, pc, pe := ParsePseudoElementOrClass(1, "modal")
	if m != MatchPseudoClass || pc != PseudoClassModal || pe != PseudoElementUnknown {
		t.Fatalf(":modal → m=%v pc=%v pe=%v", m, pc, pe)
	}
	m, pc, pe = ParsePseudoElementOrClass(2, "backdrop")
	if m != MatchPseudoElement || pc != PseudoClassUnknown || pe != PseudoElementBackdrop {
		t.Fatalf("::backdrop → m=%v pc=%v pe=%v", m, pc, pe)
	}
	// 单冒号写 ::backdrop 的旧式写法（或误写）→ 伪元素表里没有同名伪类，
	// 落到 PseudoClassUnknown（浏览器同样不承认 :-backdrop）。
	m, pc, pe = ParsePseudoElementOrClass(1, "backdrop")
	if m != MatchPseudoClass || pc != PseudoClassUnknown || pe != PseudoElementUnknown {
		t.Fatalf(":backdrop（单冒号）→ m=%v pc=%v pe=%v", m, pc, pe)
	}
	// view-transition 系列已解析但永不匹配（无 view-transition 机制），
	// 解析层仍要给出正确名字（供 UA 表/调试使用）。
	sel := SimpleSelector{Match: MatchPseudoElement, PseudoElem: PseudoElementViewTransitionGroup}
	if got := sel.String(); got != "::view-transition-group" {
		t.Fatalf("::view-transition-group 序列化 = %q", got)
	}
}

func TestSelector_ParsePseudoElementOrClass(t *testing.T) {
	// Single colon + legacy pseudo-element name → pseudo-element.
	m, pc, pe := ParsePseudoElementOrClass(1, "before")
	if m != MatchPseudoElement || pc != PseudoClassUnknown || pe != PseudoElementBefore {
		t.Fatalf("legacy single-colon ::before: m=%v pc=%v pe=%v", m, pc, pe)
	}
	// Double colon + pseudo-element name → pseudo-element.
	m, pc, pe = ParsePseudoElementOrClass(2, "after")
	if m != MatchPseudoElement || pe != PseudoElementAfter {
		t.Fatalf("double-colon ::after: m=%v pc=%v pe=%v", m, pc, pe)
	}
	// Single colon + pseudo-class name → pseudo-class.
	m, pc, pe = ParsePseudoElementOrClass(1, "hover")
	if m != MatchPseudoClass || pc != PseudoClassHover || pe != PseudoElementUnknown {
		t.Fatalf("single-colon :hover: m=%v pc=%v pe=%v", m, pc, pe)
	}
}

// --- SelectorChecker ---------------------------------------------------------

func newTestDoc(t *testing.T) *dom.Document {
	t.Helper()
	return dom.NewDocument()
}

func TestSelector_MatchTag(t *testing.T) {
	doc := newTestDoc(t)
	el := dom.NewElement(doc, "div")
	c := NewSelectorChecker()
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{{Match: MatchTag, Value: "div"}}},
		},
	}
	if !c.Match(sel, el) {
		t.Fatalf("div should match div")
	}
	notDiv := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{{Match: MatchTag, Value: "span"}}},
		},
	}
	if c.Match(notDiv, el) {
		t.Fatalf("span should not match div")
	}
}

func TestSelector_MatchIDAndClass(t *testing.T) {
	doc := newTestDoc(t)
	el := dom.NewElement(doc, "p")
	el.SetId("intro")
	el.SetAttribute("class", "lead highlight")
	c := NewSelectorChecker()
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{
				{Match: MatchTag, Value: "p"},
				{Match: MatchID, Value: "intro"},
				{Match: MatchClass, Value: "highlight"},
			}},
		},
	}
	if !c.Match(sel, el) {
		t.Fatalf("p#intro.highlight should match")
	}
	missingClass := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{
				{Match: MatchClass, Value: "nope"},
			}},
		},
	}
	if c.Match(missingClass, el) {
		t.Fatalf("class=nope should not match")
	}
}

func TestSelector_MatchDescendant(t *testing.T) {
	doc := newTestDoc(t)
	root := dom.NewElement(doc, "div")
	middle := dom.NewElement(doc, "section")
	leaf := dom.NewElement(doc, "p")
	_ = root.AppendChild(middle)
	_ = middle.AppendChild(leaf)
	c := NewSelectorChecker()
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{{Match: MatchTag, Value: "div"}}},
			{
				Relation: RelationDescendant,
				Selectors: []SimpleSelector{{Match: MatchTag, Value: "p"}},
			},
		},
	}
	if !c.Match(sel, leaf) {
		t.Fatalf("div p should match nested p")
	}
}

func TestSelector_MatchChild(t *testing.T) {
	doc := newTestDoc(t)
	root := dom.NewElement(doc, "div")
	middle := dom.NewElement(doc, "section")
	leaf := dom.NewElement(doc, "p")
	_ = root.AppendChild(middle)
	_ = middle.AppendChild(leaf)
	c := NewSelectorChecker()
	// div > p — should NOT match (leaf's parent is section, not div).
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{{Match: MatchTag, Value: "div"}}},
			{
				Relation: RelationChild,
				Selectors: []SimpleSelector{{Match: MatchTag, Value: "p"}},
			},
		},
	}
	if c.Match(sel, leaf) {
		t.Fatalf("div > p should not match (parent is section)")
	}
	// section > p — should match.
	sel2 := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{{Match: MatchTag, Value: "section"}}},
			{
				Relation: RelationChild,
				Selectors: []SimpleSelector{{Match: MatchTag, Value: "p"}},
			},
		},
	}
	if !c.Match(sel2, leaf) {
		t.Fatalf("section > p should match")
	}
}

func TestSelector_MatchAdjacentSibling(t *testing.T) {
	doc := newTestDoc(t)
	parent := dom.NewElement(doc, "ul")
	first := dom.NewElement(doc, "li")
	second := dom.NewElement(doc, "li")
	_ = parent.AppendChild(first)
	_ = parent.AppendChild(second)
	c := NewSelectorChecker()
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{{Match: MatchTag, Value: "li"}}},
			{
				Relation: RelationDirectAdjacent,
				Selectors: []SimpleSelector{{Match: MatchTag, Value: "li"}},
			},
		},
	}
	if !c.Match(sel, second) {
		t.Fatalf("li + li should match the second li")
	}
	if c.Match(sel, first) {
		t.Fatalf("li + li should not match the first li")
	}
}

func TestSelector_MatchGeneralSibling(t *testing.T) {
	doc := newTestDoc(t)
	parent := dom.NewElement(doc, "div")
	a := dom.NewElement(doc, "h1")
	b := dom.NewElement(doc, "p")
	cc := dom.NewElement(doc, "p")
	_ = parent.AppendChild(a)
	_ = parent.AppendChild(b)
	_ = parent.AppendChild(cc)
	c := NewSelectorChecker()
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{{Match: MatchTag, Value: "h1"}}},
			{
				Relation: RelationIndirectAdjacent,
				Selectors: []SimpleSelector{{Match: MatchTag, Value: "p"}},
			},
		},
	}
	// h1 ~ p should match both p elements (both are siblings following h1).
	if !c.Match(sel, b) {
		t.Fatalf("h1 ~ p should match first p")
	}
	if !c.Match(sel, cc) {
		t.Fatalf("h1 ~ p should match second p")
	}
	// Should not match h1 itself.
	if c.Match(sel, a) {
		t.Fatalf("h1 ~ p should not match h1")
	}
}

func TestSelector_MatchAttributeSelectors(t *testing.T) {
	doc := newTestDoc(t)
	el := dom.NewElement(doc, "input")
	el.SetAttribute("type", "text")
	el.SetAttribute("data-id", "user-42")
	c := NewSelectorChecker()

	// [type]
	setSel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{{Match: MatchSet, Attribute: "type"}}},
		},
	}
	if !c.Match(setSel, el) {
		t.Fatalf("[type] should match")
	}

	// [type="text"]
	exactSel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{
				{Match: MatchExact, Attribute: "type", Value: "text"},
			}},
		},
	}
	if !c.Match(exactSel, el) {
		t.Fatalf(`[type="text"] should match`)
	}

	// [data-id^="user-"]
	beginSel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{
				{Match: MatchBegin, Attribute: "data-id", Value: "user-"},
			}},
		},
	}
	if !c.Match(beginSel, el) {
		t.Fatalf(`[data-id^="user-"] should match`)
	}

	// [data-id$="-42"]
	endSel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{
				{Match: MatchEnd, Attribute: "data-id", Value: "-42"},
			}},
		},
	}
	if !c.Match(endSel, el) {
		t.Fatalf(`[data-id$="-42"] should match`)
	}

	// [type~="text"] — type has no whitespace-separated tokens so "text" is a single
	// token, this should match.
	listSel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{
				{Match: MatchList, Attribute: "type", Value: "text"},
			}},
		},
	}
	if !c.Match(listSel, el) {
		t.Fatalf(`[type~="text"] should match single-token value`)
	}
}

func TestSelector_MatchFirstChild(t *testing.T) {
	doc := newTestDoc(t)
	parent := dom.NewElement(doc, "ul")
	a := dom.NewElement(doc, "li")
	b := dom.NewElement(doc, "li")
	_ = parent.AppendChild(a)
	_ = parent.AppendChild(b)
	c := NewSelectorChecker()
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{{Match: MatchPseudoClass, PseudoClass: PseudoClassFirstChild}}},
		},
	}
	if !c.Match(sel, a) {
		t.Fatalf("first li should match :first-child")
	}
	if c.Match(sel, b) {
		t.Fatalf("second li should not match :first-child")
	}
}

func TestSelector_MatchNthChild(t *testing.T) {
	doc := newTestDoc(t)
	parent := dom.NewElement(doc, "ol")
	var items []*dom.Element
	for i := 0; i < 5; i++ {
		li := dom.NewElement(doc, "li")
		_ = parent.AppendChild(li)
		items = append(items, li)
	}
	c := NewSelectorChecker()
	// :nth-child(2n+1) — odd positions: items[0], items[2], items[4]
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{{
				Match:       MatchPseudoClass,
				PseudoClass: PseudoClassNthChild,
				NthA:        2,
				NthB:        1,
			}}},
		},
	}
	for i, li := range items {
		wantMatch := i%2 == 0 // 1-indexed positions 1, 3, 5 → 0-indexed 0, 2, 4
		got := c.Match(sel, li)
		if got != wantMatch {
			t.Errorf("item %d (1-indexed pos %d): got match=%v want %v", i, i+1, got, wantMatch)
		}
	}
}

func TestSelector_MatchNotPseudoClass(t *testing.T) {
	doc := newTestDoc(t)
	el := dom.NewElement(doc, "p")
	el.SetAttribute("class", "active")
	c := NewSelectorChecker()
	// p:not(.active) should NOT match (el has .active)
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{
				{Match: MatchTag, Value: "p"},
				{
					Match:       MatchPseudoClass,
					PseudoClass: PseudoClassNot,
					SelectorList: &SelectorList{
						Selectors: []ComplexSelector{
							{Compounds: []CompoundSelector{{
								Selectors: []SimpleSelector{{Match: MatchClass, Value: "active"}},
							}}},
						},
					},
				},
			}},
		},
	}
	if c.Match(sel, el) {
		t.Fatalf("p:not(.active) should not match element with class=active")
	}
	// p:not(.missing) should match.
	sel2 := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{
				{Match: MatchTag, Value: "p"},
				{
					Match:       MatchPseudoClass,
					PseudoClass: PseudoClassNot,
					SelectorList: &SelectorList{
						Selectors: []ComplexSelector{
							{Compounds: []CompoundSelector{{
								Selectors: []SimpleSelector{{Match: MatchClass, Value: "missing"}},
							}}},
						},
					},
				},
			}},
		},
	}
	if !c.Match(sel2, el) {
		t.Fatalf("p:not(.missing) should match element without .missing")
	}
}

func TestSelector_MatchList(t *testing.T) {
	doc := newTestDoc(t)
	el := dom.NewElement(doc, "p")
	c := NewSelectorChecker()
	list := &SelectorList{
		Selectors: []ComplexSelector{
			{Compounds: []CompoundSelector{{Selectors: []SimpleSelector{{Match: MatchTag, Value: "div"}}}}},
			{Compounds: []CompoundSelector{{Selectors: []SimpleSelector{{Match: MatchTag, Value: "p"}}}}},
		},
	}
	if !c.MatchList(list, el) {
		t.Fatalf("MatchList should match when any selector matches")
	}
	noneList := &SelectorList{
		Selectors: []ComplexSelector{
			{Compounds: []CompoundSelector{{Selectors: []SimpleSelector{{Match: MatchTag, Value: "div"}}}}},
		},
	}
	if c.MatchList(noneList, el) {
		t.Fatalf("MatchList should not match when no selector matches")
	}
}

func TestSelector_MatchIsWhere(t *testing.T) {
	doc := newTestDoc(t)
	el := dom.NewElement(doc, "span")
	c := NewSelectorChecker()
	// :is(span, div) should match span.
	isSel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{{
				Match:       MatchPseudoClass,
				PseudoClass: PseudoClassIs,
				SelectorList: &SelectorList{
					Selectors: []ComplexSelector{
						{Compounds: []CompoundSelector{{Selectors: []SimpleSelector{{Match: MatchTag, Value: "span"}}}}},
						{Compounds: []CompoundSelector{{Selectors: []SimpleSelector{{Match: MatchTag, Value: "div"}}}}},
					},
				},
			}}},
		},
	}
	if !c.Match(isSel, el) {
		t.Fatalf(":is(span, div) should match span")
	}
	// :where(p, div) should NOT match span.
	whereSel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{{
				Match:       MatchPseudoClass,
				PseudoClass: PseudoClassWhere,
				SelectorList: &SelectorList{
					Selectors: []ComplexSelector{
						{Compounds: []CompoundSelector{{Selectors: []SimpleSelector{{Match: MatchTag, Value: "p"}}}}},
						{Compounds: []CompoundSelector{{Selectors: []SimpleSelector{{Match: MatchTag, Value: "div"}}}}},
					},
				},
			}}},
		},
	}
	if c.Match(whereSel, el) {
		t.Fatalf(":where(p, div) should not match span")
	}
}

func TestSelector_MatchHas(t *testing.T) {
	doc := newTestDoc(t)
	parent := dom.NewElement(doc, "div")
	child := dom.NewElement(doc, "p")
	_ = parent.AppendChild(child)
	c := NewSelectorChecker()
	// div:has(p) should match parent because parent has a <p> descendant.
	sel := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{
				{Match: MatchTag, Value: "div"},
				{
					Match:       MatchPseudoClass,
					PseudoClass: PseudoClassHas,
					SelectorList: &SelectorList{
						Selectors: []ComplexSelector{
							{Compounds: []CompoundSelector{{Selectors: []SimpleSelector{{Match: MatchTag, Value: "p"}}}}},
						},
					},
				},
			}},
		},
	}
	if !c.Match(sel, parent) {
		t.Fatalf("div:has(p) should match div containing a p")
	}
	// div:has(span) should NOT match parent (no span descendant).
	spanHas := ComplexSelector{
		Compounds: []CompoundSelector{
			{Selectors: []SimpleSelector{
				{Match: MatchTag, Value: "div"},
				{
					Match:       MatchPseudoClass,
					PseudoClass: PseudoClassHas,
					SelectorList: &SelectorList{
						Selectors: []ComplexSelector{
							{Compounds: []CompoundSelector{{Selectors: []SimpleSelector{{Match: MatchTag, Value: "span"}}}}},
						},
					},
				},
			}},
		},
	}
	if c.Match(spanHas, parent) {
		t.Fatalf("div:has(span) should not match div with no span descendant")
	}
}

// ─── CSS Scoping selectors (:host / :host-context / ::slotted / ::part) ───

func parseComplex(t *testing.T, cssText string) ComplexSelector {
	t.Helper()
	list := NewParser(cssText).ParseSelectorList()
	if list == nil || len(list.Selectors) == 0 {
		t.Fatalf("parse %q failed", cssText)
	}
	return list.Selectors[0]
}

func TestSelector_MatchHost(t *testing.T) {
	doc := newTestDoc(t)
	host := dom.NewElement(doc, "div")
	_, _ = host.AttachShadow("open")
	c := NewSelectorChecker()

	if !c.Match(parseComplex(t, ":host"), host) {
		t.Fatalf(":host should match a shadow host")
	}
	plain := dom.NewElement(doc, "div")
	if c.Match(parseComplex(t, ":host"), plain) {
		t.Fatalf(":host should not match a plain element")
	}
}

func TestSelector_MatchHostWithSelector(t *testing.T) {
	doc := newTestDoc(t)
	host := dom.NewElement(doc, "div")
	host.SetAttribute("class", "foo")
	_, _ = host.AttachShadow("open")
	c := NewSelectorChecker()

	if !c.Match(parseComplex(t, ":host(.foo)"), host) {
		t.Fatalf(":host(.foo) should match host with class foo")
	}
	if c.Match(parseComplex(t, ":host(.bar)"), host) {
		t.Fatalf(":host(.bar) should not match host with class foo")
	}
}

func TestSelector_MatchHostContext(t *testing.T) {
	doc := newTestDoc(t)
	wrapper := dom.NewElement(doc, "div")
	wrapper.SetAttribute("class", "dark")
	host := dom.NewElement(doc, "div")
	_ = wrapper.AppendChild(host)
	_, _ = host.AttachShadow("open")
	c := NewSelectorChecker()

	if !c.Match(parseComplex(t, ":host-context(.dark)"), host) {
		t.Fatalf(":host-context(.dark) should match host inside .dark ancestor")
	}
	if c.Match(parseComplex(t, ":host-context(.light)"), host) {
		t.Fatalf(":host-context(.light) should not match host inside .dark ancestor")
	}
}

func TestSelector_MatchSlotted(t *testing.T) {
	doc := newTestDoc(t)
	host := dom.NewElement(doc, "div")
	sr, _ := host.AttachShadow("open")
	slot := dom.NewElement(doc, "slot")
	_ = sr.AppendChild(slot)
	span := dom.NewElement(doc, "span")
	_ = host.AppendChild(span) // light-DOM child assigned to the default slot
	c := NewSelectorChecker()

	if !c.Match(parseComplex(t, "::slotted(span)"), span) {
		t.Fatalf("::slotted(span) should match an assigned light-DOM span")
	}
	// A non-assigned element must not match ::slotted.
	other := dom.NewElement(doc, "span")
	if c.Match(parseComplex(t, "::slotted(span)"), other) {
		t.Fatalf("::slotted(span) should not match an unassigned span")
	}
}

func TestSelector_MatchPart(t *testing.T) {
	doc := newTestDoc(t)
	host := dom.NewElement(doc, "div")
	sr, _ := host.AttachShadow("open")
	btn := dom.NewElement(doc, "button")
	btn.SetAttribute("part", "btn primary")
	_ = sr.AppendChild(btn)
	c := NewSelectorChecker()

	if !c.Match(parseComplex(t, "::part(btn)"), btn) {
		t.Fatalf("::part(btn) should match an element with part=btn")
	}
	if c.Match(parseComplex(t, "::part(icon)"), btn) {
		t.Fatalf("::part(icon) should not match an element without part=icon")
	}
}

// TestSelector_MatchPartExportparts verifies ::part matching through the exportparts
// re-export chain (CSS Scoping Level 1 §4.5): a shadow host's exportparts attribute
// exposes an inner part name under an outer name to the next tree up.
func TestSelector_MatchPartExportparts(t *testing.T) {
	doc := newTestDoc(t)
	innerHost := dom.NewElement(doc, "xinner")
	innerSR, _ := innerHost.AttachShadow("open")
	btn := dom.NewElement(doc, "button")
	btn.SetAttribute("part", "inner")
	_ = innerSR.AppendChild(btn)
	innerHost.SetAttribute("exportparts", "inner: outer")
	c := NewSelectorChecker()

	if !c.Match(parseComplex(t, "::part(inner)"), btn) {
		t.Fatalf("::part(inner) should match the element's own part name")
	}
	if !c.Match(parseComplex(t, "::part(outer)"), btn) {
		t.Fatalf("::part(outer) should match via exportparts re-export")
	}
	if c.Match(parseComplex(t, "::part(nope)"), btn) {
		t.Fatalf("::part(nope) should not match")
	}
}

// TestSelector_MatchPartExportpartsMultiLayer verifies the exportparts re-export
// chain across two nested shadow layers: inner -> mid -> outer.
func TestSelector_MatchPartExportpartsMultiLayer(t *testing.T) {
	doc := newTestDoc(t)
	outerHost := dom.NewElement(doc, "xouter")
	outerSR, _ := outerHost.AttachShadow("open")
	midHost := dom.NewElement(doc, "xmid")
	_ = outerSR.AppendChild(midHost)
	midSR, _ := midHost.AttachShadow("open")
	btn := dom.NewElement(doc, "button")
	btn.SetAttribute("part", "inner")
	_ = midSR.AppendChild(btn)
	midHost.SetAttribute("exportparts", "inner: mid")
	outerHost.SetAttribute("exportparts", "mid: outer")
	c := NewSelectorChecker()

	if !c.Match(parseComplex(t, "::part(outer)"), btn) {
		t.Fatalf("::part(outer) should match across two exportparts layers")
	}
	if !c.Match(parseComplex(t, "::part(mid)"), btn) {
		t.Fatalf("::part(mid) should match at the middle layer")
	}
	if c.Match(parseComplex(t, "::part(nope)"), btn) {
		t.Fatalf("::part(nope) should not match")
	}
}

// TestSelector_MatchPartExportpartsOneToMany verifies a single inner part name can be
// exported to multiple outer names (`exportparts="inner: a, inner: b"`), per CSS Scoping
// Level 1 §4.5 part-mapping-list. Before one-to-many support, map[string]string dropped
// the first outer name and only ::part(b) matched.
func TestSelector_MatchPartExportpartsOneToMany(t *testing.T) {
	doc := newTestDoc(t)
	innerHost := dom.NewElement(doc, "xinner")
	innerSR, _ := innerHost.AttachShadow("open")
	btn := dom.NewElement(doc, "button")
	btn.SetAttribute("part", "inner")
	_ = innerSR.AppendChild(btn)
	innerHost.SetAttribute("exportparts", "inner: a, inner: b")
	c := NewSelectorChecker()

	if !c.Match(parseComplex(t, "::part(a)"), btn) {
		t.Fatalf("::part(a) should match the first exported outer name")
	}
	if !c.Match(parseComplex(t, "::part(b)"), btn) {
		t.Fatalf("::part(b) should match the second exported outer name")
	}
	if c.Match(parseComplex(t, "::part(nope)"), btn) {
		t.Fatalf("::part(nope) should not match")
	}
}

func TestSelector_MatchPartWithHostPrefix(t *testing.T) {
	doc := newTestDoc(t)
	host := dom.NewElement(doc, "xwidget")
	sr, _ := host.AttachShadow("open")
	btn := dom.NewElement(doc, "button")
	btn.SetAttribute("part", "btn")
	_ = sr.AppendChild(btn)
	c := NewSelectorChecker()

	if !c.Match(parseComplex(t, "xwidget::part(btn)"), btn) {
		t.Fatalf("xwidget::part(btn) should match (host tag prefix + part)")
	}
	if c.Match(parseComplex(t, "ywidget::part(btn)"), btn) {
		t.Fatalf("ywidget::part(btn) should not match host tagged xwidget")
	}
	// The host-selector prefix may be a class / attribute, not just a tag.
	host.SetAttribute("class", "card")
	if !c.Match(parseComplex(t, ".card::part(btn)"), btn) {
		t.Fatalf(".card::part(btn) should match (host class prefix + part)")
	}
}

func TestSelector_MatchSlottedWithHostPrefix(t *testing.T) {
	doc := newTestDoc(t)
	host := dom.NewElement(doc, "xwidget")
	sr, _ := host.AttachShadow("open")
	slot := dom.NewElement(doc, "slot")
	_ = sr.AppendChild(slot)
	span := dom.NewElement(doc, "span")
	_ = host.AppendChild(span) // light-DOM child assigned to the default slot
	c := NewSelectorChecker()

	if !c.Match(parseComplex(t, "xwidget::slotted(span)"), span) {
		t.Fatalf("xwidget::slotted(span) should match (host tag prefix + slotted)")
	}
	if c.Match(parseComplex(t, "ywidget::slotted(span)"), span) {
		t.Fatalf("ywidget::slotted(span) should not match host tagged xwidget")
	}
}

// TestSelector_MatchPartAcrossCombinator covers the cross-shadow-boundary forward
// matching across a combinator: `.outer x-widget::part(btn)` must match the part
// element inside x-widget's shadow tree when `.outer` is an ancestor of the host
// (in the light-DOM tree), NOT an ancestor of the part element (which is impossible,
// since the part element lives in the shadow tree).
func TestSelector_MatchPartAcrossCombinator(t *testing.T) {
	doc := newTestDoc(t)
	outer := dom.NewElement(doc, "div")
	outer.SetAttribute("class", "outer")
	host := dom.NewElement(doc, "xwidget")
	_ = outer.AppendChild(host)
	sr, _ := host.AttachShadow("open")
	btn := dom.NewElement(doc, "button")
	btn.SetAttribute("part", "btn")
	_ = sr.AppendChild(btn)
	c := NewSelectorChecker()

	// descendant combinator: .outer is an ancestor of the host.
	if !c.Match(parseComplex(t, ".outer xwidget::part(btn)"), btn) {
		t.Fatalf(".outer xwidget::part(btn) should match (ancestor .outer + host + part)")
	}
	// child combinator: host's direct parent is .outer.
	if !c.Match(parseComplex(t, ".outer > xwidget::part(btn)"), btn) {
		t.Fatalf(".outer > xwidget::part(btn) should match (direct parent .outer)")
	}
	// negative: no matching ancestor.
	if c.Match(parseComplex(t, ".nope xwidget::part(btn)"), btn) {
		t.Fatalf(".nope xwidget::part(btn) should not match (no .nope ancestor)")
	}
}

// TestSelector_MatchSlottedAcrossCombinator mirrors the ::slotted variant: the host
// prefix crosses the boundary, then the combinator walks the host's light-DOM
// ancestors.
func TestSelector_MatchSlottedAcrossCombinator(t *testing.T) {
	doc := newTestDoc(t)
	outer := dom.NewElement(doc, "div")
	outer.SetAttribute("class", "outer")
	host := dom.NewElement(doc, "xwidget")
	_ = outer.AppendChild(host)
	sr, _ := host.AttachShadow("open")
	slot := dom.NewElement(doc, "slot")
	_ = sr.AppendChild(slot)
	span := dom.NewElement(doc, "span")
	_ = host.AppendChild(span) // light-DOM child assigned to the default slot
	c := NewSelectorChecker()

	if !c.Match(parseComplex(t, ".outer xwidget::slotted(span)"), span) {
		t.Fatalf(".outer xwidget::slotted(span) should match (ancestor .outer)")
	}
	if c.Match(parseComplex(t, ".nope xwidget::slotted(span)"), span) {
		t.Fatalf(".nope xwidget::slotted(span) should not match (no .nope ancestor)")
	}
}
