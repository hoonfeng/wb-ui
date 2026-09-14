package rendering

import (
	"testing"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// TestReproClockIcon: 复现配置窗口时钟图标渲染。
// 验证 .ic-clock 的 ring（16x16 border-radius:50% 圆环）是否空心正圆，
// 以及 h/m 指针是否水平居中。
func TestReproClockIcon(t *testing.T) {
	doc := dom.NewDocument()
	htmlEl := dom.NewElement(doc, "html")
	doc.AppendChild(htmlEl)
	bodyEl := dom.NewElement(doc, "body")
	htmlEl.AppendChild(bodyEl)

	styleEl := dom.NewElement(doc, "style")
	styleEl.SetTextContent(`
* { margin:0; padding:0; box-sizing:border-box; }
body { background:#1c2438; }
.picon { width:26px; height:26px; margin:0 auto 4px; position:relative; background:#223050; }
.ic { position:relative; display:inline-block; width:24px; height:24px; }
.ic-clock { width:20px; height:20px; margin-top:2px; }
.ic-clock .h { position:absolute; left:5px; top:5px; width:2px; height:6px; background:#5d8df0; }
.ic-clock .m { position:absolute; left:5px; top:9px; width:5px; height:2px; background:#5d8df0; }
.ic-clock .ring { position:absolute; left:0; top:0; width:16px; height:16px; border:2px solid #5d8df0; border-radius:50%; }
`)
	htmlEl.AppendChild(styleEl)

	picon := dom.NewElement(doc, "div")
	picon.SetClassName("picon")
	bodyEl.AppendChild(picon)

	ic := dom.NewElement(doc, "div")
	ic.SetClassName("ic ic-clock")
	picon.AppendChild(ic)
	ring := dom.NewElement(doc, "div")
	ring.SetClassName("ring")
	ic.AppendChild(ring)
	h := dom.NewElement(doc, "div")
	h.SetClassName("h")
	ic.AppendChild(h)
	m := dom.NewElement(doc, "div")
	m.SetClassName("m")
	ic.AppendChild(m)

	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(styleEl.TextContent()).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)

	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("RenderView is nil")
	}
	rv.SetViewportSize(60, 60)
	rv.Layout(nil)

	canvas := graphics.NewCanvas(60, 60)
	defer canvas.Release()
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 60, Height: 60})

	savePNG(canvas, "clock_icon_repro.png")
	t.Log("saved clock_icon_repro.png")
}
