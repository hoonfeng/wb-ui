package rendering

import (
	"fmt"
	"testing"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/html"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// TestEllipsisReproFoldedDesc reproduces the folded-summary ellipsis failure:
// .folded-desc (flex item, overflow:hidden + text-overflow:ellipsis) text is
// NOT truncated in wb-ui although the browser truncates it. It dumps the render
// object type of the desc element, whether findTextOverflowAncestor finds a
// truncation container, and the text segment geometry vs the content box.
func TestEllipsisReproFoldedDesc(t *testing.T) {
	htmlSrc := `<html><head><style>
body { margin: 0; font-family: sans-serif; font-size: 12px; }
.folded-summary { display: flex; align-items: center; gap: 6px; padding: 6px 10px; width: 300px; background: #eeeeee; }
.folded-title { font-weight: 600; }
.folded-desc[data-v-cdb19c8e] { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: #888888; }
</style></head><body>
<div class="folded-summary">
	<span class="folded-title">完成摘要</span>
	<span class="folded-desc" data-v-cdb19c8e>这是一个非常长的摘要文本用于验证文本溢出省略号裁剪功能是否正常工作超出容器宽度</span>
</div>
</body></html>`

	doc, err := html.ParseDocument(htmlSrc)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	cssText := `
.folded-summary { display: flex; align-items: center; gap: 6px; padding: 6px 10px; width: 300px; background: #eeeeee; }
.folded-title { font-weight: 600; }
.folded-desc[data-v-cdb19c8e] { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: #888888; }
`
	css.NewParser(cssText).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("nil rv")
	}
	rv.SetViewportSize(1280, 800)
	rv.Layout(nil)

	// Find the render object for .folded-desc
	var descRO RenderObject
	var findDesc func(RenderObject)
	findDesc = func(ro RenderObject) {
		if descRO != nil {
			return
		}
		if el, ok := ro.Node().(*dom.Element); ok && el.GetClassName() == "folded-desc" {
			descRO = ro
			return
		}
		for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
			findDesc(c)
		}
	}
	findDesc(rv)
	if descRO == nil {
		t.Fatal("no .folded-desc render object")
	}
	t.Logf("desc render object type: %T", descRO)

	// Find the RenderText child of desc
	var rt *RenderText
	var findText func(RenderObject)
	findText = func(ro RenderObject) {
		if rt != nil {
			return
		}
		if o, ok := ro.(*RenderText); ok {
			rt = o
			return
		}
		for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
			findText(c)
		}
	}
	findText(descRO)
	if rt == nil {
		t.Fatal("no RenderText under .folded-desc")
	}
	toCB := findTextOverflowAncestor(rt)
	if toCB == nil {
		t.Logf("findTextOverflowAncestor = NIL (no truncation container!)")
	} else {
		t.Logf("findTextOverflowAncestor = %+v", toCB)
	}

	// Text segments geometry
	st := rt.Style()
	font := toGraphicsFont(st)
	segs := rt.Segments()
	t.Logf("segments=%d text=%q", len(segs), rt.OriginalText())
	for _, s := range segs {
		t.Logf("  seg start=%d len=%d X=%.1f Y=%.1f W=%.1f H=%.1f sub=%q", s.Start, s.Len, s.X, s.Y, s.Width, s.Height,
			string([]rune(rt.OriginalText())[s.Start:s.Start+s.Len]))
	}

	// Content box geometry of the desc LayoutBox
	lb := descRO.LayoutBox()
	if lb != nil && rv.LayoutState() != nil {
		g := rv.LayoutState().GeometryForBox(lb)
		t.Logf("desc LayoutBox: left=%.1f top=%.1f w=%.1f h=%.1f", g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight())
	}
	// Measure full text width
	fullW := graphics.MeasureText(font, rt.OriginalText())
	t.Logf("full text width=%.1f", fullW)

	// Dump all layout boxes for the summary subtree
	fmt.Println("")
	t.Log("=== ALL BOXES ===")
	walkAllLayoutBoxes(findByClass(rv, "folded-summary"), t)

	// Paint and save PNG for pixel verification
	canvas := graphics.NewCanvas(1280, 800)
	defer canvas.Release()
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 1280, Height: 800})
	savePNG(canvas, "wbui_folded_desc_repro.png")
}
