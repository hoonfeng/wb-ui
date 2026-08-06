package rendering

import (
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
	if g.BorderBoxHeight() < 2*14 {
		t.Errorf("title height %.1f: CJK did not wrap to 2 lines (flex-shrunk box)", g.BorderBoxHeight())
	}
	if g.BorderBoxHeight() > 3*14 {
		t.Errorf("title height %.1f: CJK wrapped to >2 lines (1 char per line?)", g.BorderBoxHeight())
	}
}
