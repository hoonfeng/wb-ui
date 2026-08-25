package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// TestScrollContainerContentMovesOnPaint: a scroll container's content must
// move up by -scrollTop when painted. Reproduces the "thinking text scrolls
// but painted content stays put" symptom.
func TestScrollContainerContentMovesOnPaint(t *testing.T) {
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}
	body := `<div id="sc" style="position:absolute;top:50px;left:50px;width:200px;height:100px;overflow:auto;background:#fff">`
	for i := 0; i < 20; i++ {
		body += `<div>LINE` + string(rune('0'+i%10)) + `-1234567890abcdef</div>`
	}
	body += `</div>`
	htmlStr := `<!DOCTYPE html><html><head><style>html,body{margin:0;padding:0}</style></head><body>` + body + `</body></html>`
	doc, err := html.Parse(htmlStr)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rv := NewRenderTreeBuilder(style.NewResolver()).Build(doc)
	rv.SetViewportSize(600, 400)
	rv.Layout(layout.NewLayoutState(600, 400))
	sc := rv.FindRenderBoxForNode(findID(rv, "sc"))
	if sc == nil {
		t.Fatalf("sc not found")
	}
	pb := sc.PaddingBoxRect()
	t.Logf("sc pb=(%.0f,%.0f %.0fx%.0f)", pb.X, pb.Y, pb.Width, pb.Height)

	paint := func() *graphics.Canvas {
		c := graphics.NewCanvas(300, 200)
		c.Clear(graphics.Color{R: 0, G: 0, B: 0, A: 255})
		Paint(rv, c, Rect{X: 0, Y: 0, Width: 300, Height: 200})
		return c
	}

	// scrollTop=0: first line should paint near the container top (pb.Y+padding).
	c0 := paint()
	textY0 := findDarkRow(c0, int(pb.X+8), int(pb.X+150), int(pb.Y)+6, int(pb.Y)+60)
	t.Logf("scrollTop=0: first text row y=%d", textY0)

	// scrollTop=50: content moves UP by 50. Verify the SAME physical line
	// scrolled: the 4th line (textY0 + 3×lineGap) at scrollTop=0 must be at
	// (that y − 50) at scrollTop=50.
	rv.SetBoxScrollOffset(sc, 0, 50)
	c50 := paint()
	textY50 := findDarkRow(c50, int(pb.X+8), int(pb.X+150), int(pb.Y)+6, int(pb.Y)+120)
	t.Logf("scrollTop=50: first text row y=%d", textY50)

	hasInk := func(c *graphics.Canvas, y int) bool {
		for x := int(pb.X + 8); x < int(pb.X+150); x++ {
			col := c.PixelAt(x, y)
			if col.R < 200 || col.G < 200 || col.B < 200 {
				return true
			}
		}
		return false
	}
	// 行间距 = 行盒高度：默认 line-height 现为 normal（字体度量），
	// 显式 line-height:1.2 时 16px → 19.2。动态取字体度量，避免字体/行高变化漂移。
	a, d, g := layout.FontMetricsFunc("serif", 16, 400, "normal")
	lineGap := a + d + g
	if lineGap <= 0 {
		lineGap = 19.0 // 兜底（历史 1.2×16 值）
	}
	from := textY0 + int(3*lineGap)
	to := from - 50
	t.Logf("4th line: scrollTop=0 at y=%d, scrollTop=50 at y=%d (want -50), lineGap=%.1f", from, to, lineGap)
	// ±3px 容差：行盒高取整/字体度量亚像素差异
	hasInkNear := func(c *graphics.Canvas, y int) bool {
		for dy := -3; dy <= 3; dy++ {
			if hasInk(c, y+dy) {
				return true
			}
		}
		return false
	}
	if !hasInkNear(c0, from) {
		t.Errorf("scrollTop=0: no ink near y=%d (expected 4th text line)", from)
	}
	if !hasInkNear(c50, to) {
		t.Errorf("scrollTop=50: no ink near y=%d (4th line should have scrolled here)", to)
	}
	if textY50 < 0 {
		t.Fatalf("no visible text after scroll")
	}
}

func findID(rv *RenderView, id string) *dom.Element {
	var out *dom.Element
	var walk func(o RenderObject)
	walk = func(o RenderObject) {
		if out != nil {
			return
		}
		if el, ok := o.Node().(*dom.Element); ok && el.GetAttribute("id") == id {
			out = el
			return
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(RenderObject(rv))
	return out
}

// findDarkRow scans x∈[x0,x1], y∈[y0,y1] for a row with any pixel that is
// neither the container background (#fff → all 255) nor the black canvas
// (0,0,0). Returns the first such row, or -1.
func findDarkRow(c *graphics.Canvas, x0, x1, y0, y1 int) int {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			col := c.PixelAt(x, y)
			if col.R < 200 || col.G < 200 || col.B < 200 {
				return y
			}
		}
	}
	return -1
}
