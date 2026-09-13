package rendering

import (
	"math"
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/style"
)

// TestFoldedTitleCJKWrap is a regression test for flex-shrunk CJK line
// breaking: a blockified flex item (span.folded-title, "完成摘要") compressed
// to ~27px by flex-shrink must wrap its 4 ideographs into 2 lines (2+2 like
// Edge), not stay on one 41px line overflowing the shrunken box.
//
// Root cause: the anonymous inline wrapper holding the span's text ran
// auto-width expansion (max-content) inside the flex item, so its line width
// was 41px and no break happened. Fixed by skipping auto-width expansion
// inside flex items and constraining inline children to the parent line width.
func TestFoldedTitleCJKWrap(t *testing.T) {
	htmlSrc := `<html><head><style>
body { margin: 0; font-family: sans-serif; font-size: 12px; }
.folded-summary { display: flex; align-items: center; gap: 5px; padding: 5px 10px; width: 300px; }
.folded-title { font-weight: 500; }
.folded-desc { color: #888; }
</style></head><body>
<div class="folded-summary">
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
.folded-title { font-weight: 500; }
.folded-desc { color: #888; }
`
	css.NewParser(cssText).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("nil rv")
	}
	rv.SetViewportSize(1280, 800)
	rv.Layout(nil)

	var titleRO RenderObject
	var findTitle func(RenderObject)
	findTitle = func(ro RenderObject) {
		if titleRO != nil {
			return
		}
		if el, ok := ro.Node().(*dom.Element); ok && el.GetClassName() == "folded-title" {
			titleRO = ro
			return
		}
		for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
			findTitle(c)
		}
	}
	findTitle(rv)
	if titleRO == nil {
		t.Fatal("no .folded-title render object")
	}

	// Count distinct line Ys among the title's text segments: 4 CJK chars in
	// a ~27px box must produce 2 lines (2+2), not 1 (no break) nor 4 (1/line).
	lb := titleRO.LayoutBox()
	if lb == nil || rv.LayoutState() == nil {
		t.Fatal("no layout box for title")
	}
	g := rv.LayoutState().GeometryForBox(lb)
	t.Logf("title box: w=%.1f h=%.1f", g.BorderBoxWidth(), g.BorderBoxHeight())
	// 直接数文本段的不同 Y 得到真实行数（注释里的本意），不再用硬编码
	// 14px 行高做高度阈值：`font-family: sans-serif` 现在按平台映射到 Arial
	// （graphics.firstConcreteFamily），12px 文本的 normal 行高从 14.4 变成
	// 13.8，2 行高度 27.6 < 2*14 会被误判成「没有换行」。
	lines := map[float64]bool{}
	for _, seg := range lb.TextSegments {
		lines[math.Round(seg.Y)] = true
	}
	if len(lines) == 0 {
		// 无文本段（文本可能挂在匿名盒上）：退回按行高区间判断。
		lineH := g.BorderBoxHeight() / 2
		if lineH < 12 || lineH > 16 {
			t.Errorf("title height %.1f: unexpected line height", g.BorderBoxHeight())
		}
		return
	}
	if len(lines) < 2 {
		t.Errorf("title lines %d (h=%.1f): CJK did not wrap to 2 lines (flex-shrunk box)", len(lines), g.BorderBoxHeight())
	}
	if len(lines) > 2 {
		t.Errorf("title lines %d (h=%.1f): CJK wrapped to >2 lines (1 char per line?)", len(lines), g.BorderBoxHeight())
	}
}
