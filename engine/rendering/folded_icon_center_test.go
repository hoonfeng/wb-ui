package rendering

import (
	"testing"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/html"
	"wb-ui/engine/style"
)

// TestFoldedSummaryIconLineCentered is a regression test for align-items:
// center in an auto-height flex line: the icon left of "完成摘要" must be
// centered against the REAL line height (tallest item after layout), not the
// container's initial single-line estimate.
//
// Scenario: .folded-summary (flex row, align-items:center, auto height) with
// two short svg icons + "完成摘要" + a very long desc. The long desc shrinks
// the title to ~27px so its 4 CJK chars wrap 2+2 (28.8px line, like Edge) —
// the line grows only AFTER child layout. Before the fix the icon was
// centered against the initial 19.2px single-line estimate and sat ~10px
// high once the title wrapped — "inline-centered, not line-centered".
func TestFoldedSummaryIconLineCentered(t *testing.T) {
	htmlSrc := `<html><head><style>
body { margin: 0; font-family: sans-serif; font-size: 12px; }
.folded-summary { display: flex; align-items: center; gap: 5px; padding: 5px 10px; width: 300px; }
</style></head><body>
<div class="folded-summary">
	<svg class="folded-chevron" viewBox="0 0 8 8" width="9" height="9"><path d="M2.6 1.2 L6.8 4 L2.6 6.8 Z"/></svg>
	<svg class="svg-icon" width="11" height="11" viewBox="0 0 24 24"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>
	<span class="folded-title">完成摘要</span>
	<span class="folded-desc">这是一个非常长的摘要文本用于验证文本溢出省略号裁剪功能是否正常工作超出容器宽度</span>
</div>
</body></html>`

	doc, err := html.ParseDocument(htmlSrc)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	cssText := `
body { margin: 0; font-family: sans-serif; font-size: 12px; }
.folded-summary { display: flex; align-items: center; gap: 5px; padding: 5px 10px; width: 300px; }
`
	css.NewParser(cssText).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("nil rv")
	}
	rv.SetViewportSize(1280, 800)
	rv.Layout(nil)

	// Collect geometry of .folded-summary, the two svg icons and .folded-title.
	type geo struct {
		top, h float64
	}
	geom := map[string]geo{}
	var walk func(RenderObject)
	walk = func(ro RenderObject) {
		el, ok := ro.Node().(*dom.Element)
		if ok {
			cn := el.GetClassName()
			if cn == "folded-summary" || cn == "folded-chevron" || cn == "svg-icon" || cn == "folded-title" {
				if lb := ro.LayoutBox(); lb != nil && rv.LayoutState() != nil {
					g := rv.LayoutState().GeometryForBox(lb)
					geom[cn] = geo{top: g.Top(), h: g.BorderBoxHeight()}
				}
			}
		}
		for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(rv)

	sum, ok := geom["folded-summary"]
	if !ok {
		t.Fatal("no .folded-summary geometry")
	}
	title, ok := geom["folded-title"]
	if !ok {
		t.Fatal("no .folded-title geometry")
	}
	// Sanity: the title must actually wrap (2 lines) for this regression test
	// to exercise the post-layout line-height growth.
	// 行高区间按字体度量留出裕量，不再硬编码 14px：`sans-serif` 现在按平台
	// 映射到 Arial（graphics.firstConcreteFamily），12px 文本 normal 行高从
	// 14.4 变为 13.8，2 行高度 27.6 会被 `2*14` 误判为「场景无效」。
	if title.h < 2*13 || title.h > 3*15 {
		t.Fatalf("title height %.1f: expected 2 wrapped lines (~27.6px), test scenario invalid", title.h)
	}
	sumCenter := sum.top + sum.h/2
	t.Logf("summary y=%.1f h=%.1f center=%.1f | title h=%.1f", sum.top, sum.h, sumCenter, title.h)

	for _, cn := range []string{"folded-chevron", "svg-icon", "folded-title"} {
		g, ok := geom[cn]
		if !ok {
			t.Errorf("no geometry for .%s", cn)
			continue
		}
		center := g.top + g.h/2
		t.Logf("%s y=%.1f h=%.1f center=%.1f", cn, g.top, g.h, center)
		if diff := center - sumCenter; diff > 1.0 || diff < -1.0 {
			t.Errorf(".%s center %.1f must match container center %.1f (line-centered)", cn, center, sumCenter)
		}
	}
}
