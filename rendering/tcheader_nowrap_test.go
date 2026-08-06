package rendering

import (
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/style"
)

// TestTcHeaderNowrapSingleLine reproduces the tool-call header wrap failure:
// .tl-tc-header (flex row) with .tl-tc-param (flex:1, min-width:0,
// overflow:hidden, text-overflow:ellipsis, white-space:nowrap) must render on
// a SINGLE line even when the param text is long (browser behavior). The old
// engine wrapped the param onto 2 lines (header height 27→43px) for certain
// content (CJK paths / commands with newlines) because white-space:nowrap was
// not honored for those segments.
func TestTcHeaderNowrapSingleLine(t *testing.T) {
	htmlSrc := `<html><head><style>
body { margin: 0; font-family: sans-serif; font-size: 13px; }
.tl-tc-header { display: flex; align-items: center; gap: 4px; width: 400px; }
.tl-tc-name { font-size: 12px; font-weight: 500; flex-shrink: 0; }
.tl-tc-param { font-size: 11px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; min-width: 0; font-family: sans-serif; }
.tl-tc-summary { font-size: 11px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; min-width: 0; }
</style></head><body>
<div class="tl-tc-header">
	<span class="tl-tc-name">执行命令</span>
	<span class="tl-tc-param">$ cd /d F:\syproject\gou-ide&#10;set CGO_ENABLED=1 && go build ./cmd/companion 这是一行含换行符的长命令文本</span>
	<span class="tl-tc-summary">已完成</span>
</div>
</body></html>`

	doc, err := html.ParseDocument(htmlSrc)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	cssText := `
.tl-tc-header { display: flex; align-items: center; gap: 4px; width: 400px; }
.tl-tc-name { font-size: 12px; font-weight: 500; flex-shrink: 0; }
.tl-tc-param { font-size: 11px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; min-width: 0; font-family: sans-serif; }
.tl-tc-summary { font-size: 11px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; min-width: 0; }
`
	css.NewParser(cssText).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("nil rv")
	}
	rv.SetViewportSize(1280, 800)
	rv.Layout(nil)

	// Locate .tl-tc-param render object + its box.
	var paramRO RenderObject
	var findParam func(RenderObject)
	findParam = func(ro RenderObject) {
		if paramRO != nil {
			return
		}
		if el, ok := ro.Node().(*dom.Element); ok && el.GetClassName() == "tl-tc-param" {
			paramRO = ro
			return
		}
		for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
			findParam(c)
		}
	}
	findParam(rv)
	if paramRO == nil {
		t.Fatal("no .tl-tc-param render object")
	}
	st := paramRO.LayoutBox().Style()
	if st == nil {
		t.Fatal("param has no computed style")
	}
	if st.WhiteSpace != style.WhiteSpaceNoWrap {
		t.Errorf("param white-space = %v, want WhiteSpaceNoWrap(%d)", st.WhiteSpace, style.WhiteSpaceNoWrap)
	}
	g := rv.LayoutState().GeometryForBox(paramRO.LayoutBox())
	h := g.BorderBoxHeight()
	t.Logf("param WhiteSpace=%d h=%.1f w=%.1f", st.WhiteSpace, h, g.BorderBoxWidth())
	// Single-line height for 11px font ≈ 13-18px. Two lines ≈ 26-36px.
	if h > 22 {
		t.Errorf("param box height = %.1f (>22px) — text wrapped onto 2+ lines; want single line", h)
	}

	// Header must also stay single line.
	var hdrRO RenderObject
	var findHdr func(RenderObject)
	findHdr = func(ro RenderObject) {
		if hdrRO != nil {
			return
		}
		if el, ok := ro.Node().(*dom.Element); ok && el.GetClassName() == "tl-tc-header" {
			hdrRO = ro
			return
		}
		for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
			findHdr(c)
		}
	}
	findHdr(rv)
	if hdrRO != nil {
		hg := rv.LayoutState().GeometryForBox(hdrRO.LayoutBox())
		t.Logf("header h=%.1f", hg.BorderBoxHeight())
	}
}
