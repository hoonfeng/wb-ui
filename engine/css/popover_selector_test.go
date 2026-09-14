// Tests for the :popover-open pseudo-class (HTML §6.12): it matches exactly the
// elements whose popover visibility state is "showing", and nothing else.
//
// The state lives on the DOM node (dom.PopoverState.Showing) because both the
// CSS selector engine and the popover algorithms need it — see
// engine/dom/popoverstate.go and the wb-ui/engine/popover package.

package css

import (
	"testing"

	"wb-ui/engine/dom"
)

func TestSelector_PopoverOpenPseudoClass(t *testing.T) {
	doc := dom.NewDocument()
	html := dom.NewElement(doc, "html")
	doc.AppendChild(html)
	body := dom.NewElement(doc, "body")
	html.AppendChild(body)

	popover := dom.NewElement(doc, "div")
	popover.SetAttribute("popover", "")
	body.AppendChild(popover)

	plain := dom.NewElement(doc, "div")
	body.AppendChild(plain)

	c := NewSelectorChecker()
	sel := mustParseOneSelector(t, ":popover-open")
	if got := sel.String(); got != ":popover-open" {
		t.Fatalf("ComplexSelector.String() = %q，want :popover-open", got)
	}
	// UA 样式表使用的形式：`[popover]:not(:popover-open)` —— 属性选择器与伪类
	// 的组合必须能解析并匹配。
	ua := mustParseOneSelector(t, "[popover]:not(:popover-open)")

	// 未显示：带 popover 属性 → 匹配 UA 的「未显示」规则、不匹配 :popover-open。
	if c.Match(sel, popover) {
		t.Fatal("hidden 状态的 popover 不应匹配 :popover-open")
	}
	if !c.Match(ua, popover) {
		t.Fatal("[popover]:not(:popover-open) 应匹配隐藏中的 popover")
	}

	// 显示：只有 popover visibility state 变化，属性不变。
	popover.Popover().Showing = true
	if !c.Match(sel, popover) {
		t.Fatal("showing 状态的 popover 应匹配 :popover-open")
	}
	if c.Match(ua, popover) {
		t.Fatal("[popover]:not(:popover-open) 不应匹配显示中的 popover")
	}
	// 无 popover 属性的元素无论如何都不匹配（未显示）。
	if c.Match(sel, plain) {
		t.Fatal("无 popover 属性的元素不应匹配 :popover-open")
	}

	popover.Popover().Showing = false
	if c.Match(sel, popover) {
		t.Fatal("关闭后不应再匹配 :popover-open")
	}
}

// TestSelector_PopoverOpenNeedsShowingState 锁死「:popover-open 只看可见状态，
// 不看 popover 属性的状态」这条规范语义：manual popover 显示时同样匹配。
func TestSelector_PopoverOpenNeedsShowingState(t *testing.T) {
	doc := dom.NewDocument()
	html := dom.NewElement(doc, "html")
	doc.AppendChild(html)
	body := dom.NewElement(doc, "body")
	html.AppendChild(body)

	manual := dom.NewElement(doc, "div")
	manual.SetAttribute("popover", "manual")
	body.AppendChild(manual)

	c := NewSelectorChecker()
	sel := mustParseOneSelector(t, ":popover-open")

	manual.Popover().Showing = true
	if !c.Match(sel, manual) {
		t.Fatal("manual popover 显示时也应匹配 :popover-open（可见状态是唯一判据）")
	}
}
