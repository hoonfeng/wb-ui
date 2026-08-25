package rendering

import (
	"testing"

	"wb-ui/html"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

func TestAbsChildOverflowClipProbe(t *testing.T) {
	htmlStr := `<html><body style="margin:0">
<div id="box" style="position:relative;left:10px;top:10px;width:100px;height:60px;overflow:hidden;background:#333">
<div id="abs" style="position:absolute;left:80px;top:20px;width:80px;height:40px;background:#f00"></div>
</div></body></html>`
	doc, err := html.Parse(htmlStr)
	if err != nil { t.Fatal(err) }
	resolver := style.NewResolver()
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	rv.SetViewportSize(300, 300)
	state := layout.NewLayoutState(300, 300)
	rv.Layout(state)
	boxEl := doc.GetElementById("box")
	absEl := doc.GetElementById("abs")
	if boxEl == nil || absEl == nil { t.Fatal("nil el") }
	b := rv.FindRenderBoxForNode(boxEl)
	a := rv.FindRenderBoxForNode(absEl)
	if b == nil || a == nil { t.Fatal("nil box") }
	t.Logf("box abs=(%.0f,%.0f) size=(%.0f,%.0f) | abs abs=(%.0f,%.0f) size=(%.0f,%.0f)",
		b.AbsoluteX(), b.AbsoluteY(), b.Width(), b.Height(),
		a.AbsoluteX(), a.AbsoluteY(), a.Width(), a.Height())
	canvas := graphics.NewCanvas(300, 300)
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 300, Height: 300})
	defer canvas.Release()
	ax := int(a.AbsoluteX()); ay := int(a.AbsoluteY())
	t.Logf("abs内box内(%d,%d)=%+v", ax+5, ay+5, canvas.PixelAt(ax+5, ay+5))
	t.Logf("abs内box外(%d,%d)=%+v", ax+70, ay+5, canvas.PixelAt(ax+70, ay+5))
}
