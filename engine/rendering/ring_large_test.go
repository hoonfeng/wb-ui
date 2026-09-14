package rendering

import (
	"testing"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// TestReproRoundRingLarge: 用 40x40 border-radius:50% 大圆环验证
// StrokeRoundRect 是否渲染出对称正圆（时钟图标 16px 太小，偏差难辨）。
func TestReproRoundRingLarge(t *testing.T) {
	doc := dom.NewDocument()
	htmlEl := dom.NewElement(doc, "html")
	doc.AppendChild(htmlEl)
	bodyEl := dom.NewElement(doc, "body")
	htmlEl.AppendChild(bodyEl)

	styleEl := dom.NewElement(doc, "style")
	styleEl.SetTextContent(`
* { margin:0; padding:0; box-sizing:border-box; }
body { background:#1c2438; }
.wrap { width:80px; height:80px; background:#223050; }
.ring { position:absolute; left:10px; top:10px; width:40px; height:40px; border:4px solid #5d8df0; border-radius:50%; }
`)
	htmlEl.AppendChild(styleEl)

	wrap := dom.NewElement(doc, "div")
	wrap.SetClassName("wrap")
	bodyEl.AppendChild(wrap)
	ring := dom.NewElement(doc, "div")
	ring.SetClassName("ring")
	wrap.AppendChild(ring)

	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(styleEl.TextContent()).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)

	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("RenderView is nil")
	}
	rv.SetViewportSize(80, 80)
	rv.Layout(nil)

	canvas := graphics.NewCanvas(80, 80)
	defer canvas.Release()
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 80, Height: 80})

	// 直接验证 StrokePath 画闭合矩形：左/上/下/右四条边。
	blue := graphics.Color{R: 0xFF, G: 0x00, B: 0x00, A: 0xFF}
	canvas.StrokePath([]graphics.Point{
		{X: 10, Y: 55}, {X: 70, Y: 55}, {X: 70, Y: 75}, {X: 10, Y: 75}, {X: 10, Y: 55},
	}, 2, blue, "butt", "round")

	savePNG(canvas, "ring_large.png")
	t.Log("saved ring_large.png")
}
