package style

import (
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
)

// TestWebkitScrollbarRules: ::-webkit-scrollbar / ::-webkit-scrollbar-thumb rules
// must map onto the element's scrollbar palette (raw properties read by the
// painter) WITHOUT polluting the element's own computed style.
func TestWebkitScrollbarRules(t *testing.T) {
	sheet := css.NewCSSStyleSheet()
	p := css.NewParser(`
		:root { --scrollbar-thumb: #ff6600; }
		.scroll::-webkit-scrollbar { width: 4px; }
		.scroll::-webkit-scrollbar-thumb { background: var(--scrollbar-thumb); border-radius: 2px; }
	`)
	p.ParseStyleSheetInto(sheet)

	r := NewResolver()
	r.AddStyleSheet(sheet)

	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	el.SetAttribute("class", "scroll")
	cs := r.ResolveElement(el)

	if got := cs.GetProperty("-webkit-scrollbar-width"); got != "4px" {
		t.Fatalf("-webkit-scrollbar-width=%q, want 4px", got)
	}
	// var() 解析后应为实际颜色
	if got := cs.GetProperty("-webkit-scrollbar-thumb-color"); got != "#ff6600" {
		t.Fatalf("-webkit-scrollbar-thumb-color=%q, want #ff6600", got)
	}
	if got := cs.GetProperty("-webkit-scrollbar-thumb-radius"); got != "2px" {
		t.Fatalf("-webkit-scrollbar-thumb-radius=%q, want 2px", got)
	}
	// 关键：scrollbar 规则不得污染元素自身的 width/background
	if cs.Width.Unit != "" && cs.Width.Unit != "auto" {
		t.Fatalf("element width polluted by scrollbar rule: %+v", cs.Width)
	}
	if cs.BackgroundColor.A != 0 {
		t.Fatalf("element background polluted by scrollbar rule: %+v", cs.BackgroundColor)
	}
}
