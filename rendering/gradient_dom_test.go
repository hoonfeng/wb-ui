package rendering

import (
	"fmt"
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/html5"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// TestGradientDomParse 走完整 CSS 链：打印各角度渐变的
// BackgroundImage 串与 parseGradient 解析结果，定位非 90deg 失败点。
func TestGradientDomParse(t *testing.T) {
	if graphics.GetFontManager() == nil {
		mgr := graphics.InitFontManager("")
		mgr.LoadSystemFonts()
	}
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}
	htmlStr := `<html><head><style>
  .a { background:linear-gradient(90deg,#ff0000,#ffff00); width:60px; height:30px; }
  .b { background:linear-gradient(45deg,#ff0000,#ffff00); width:60px; height:30px; }
  .c { background:linear-gradient(135deg,#ff0000,#ffff00); width:60px; height:30px; }
  .d { background:linear-gradient(0deg,#ff0000,#ffff00); width:60px; height:30px; }
</style></head><body>
<div class="a" id="a"></div><div class="b" id="b" style="margin-top:4px"></div>
<div class="c" id="c" style="margin-top:4px"></div><div class="d" id="d" style="margin-top:4px"></div>
</body></html>`
	doc, _ := html.Parse(htmlStr)
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	var walk func(n dom.Node)
	walk = func(n dom.Node) {
		if el, ok := n.(*dom.Element); ok && el.LocalName() == "style" {
			if c := el.FirstChild(); c != nil {
				if tx, ok := c.(*dom.Text); ok {
					sheet := css.NewCSSStyleSheet()
					sheet.SetOrigin(css.OriginAuthor)
					p := css.NewParser(tx.Data())
					p.SetOrigin(css.OriginAuthor)
					for _, r := range p.ParseStyleSheet() {
						sheet.AppendRule(r)
					}
					resolver.AddStyleSheet(sheet)
				}
			}
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(doc)

	builder := NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	rv.SetResolver(resolver)
	rv.SetViewportSize(100, 140)
	state := layout.NewLayoutState(100, 140)
	rv.Layout(state)

	// 找 div 元素，打印 computed BackgroundImage + parseGradient
	for _, id := range []string{"a", "b", "c", "d"} {
		var ro RenderObject
		var findRO func(n RenderObject)
		findRO = func(n RenderObject) {
			if ro != nil {
				return
			}
			if el, ok := n.Node().(*dom.Element); ok && el.GetAttribute("id") == id {
				ro = n
				return
			}
			for c := n.FirstChild(); c != nil; c = c.NextSibling() {
				findRO(c)
			}
		}
		findRO(rv)
		if ro == nil {
			t.Fatalf("no render object %s", id)
		}
		rb := asRenderBox(ro)
		if rb == nil {
			t.Fatalf("%s: no RenderBox", id)
		}
		st := rb.Style()
		bi := st.BackgroundImage
		layers := splitBackgroundLayers(bi)
		lg := parseGradient(bi)
		dir := ""
		if lg != nil {
			dir = fmt.Sprintf("Angle=%.0f IsAngle=%v stops=%d", lg.Direction.Angle, lg.Direction.IsAngle, len(lg.Stops))
		}
		t.Logf("%s: BackgroundImage=%q layers=%d lg=%v dir=%s", id, bi, len(layers), lg != nil, dir)
	}
	// DOM 级渲染像素采样（含 computeGradientDest）
	canvas := graphics.NewCanvas(100, 140)
	defer canvas.Release()
	// 对照1：直接画纯色验证 canvas 通道
	canvas.FillRect(60, 100, 30, 20, graphics.Color{R: 0x00, G: 0xff, B: 0x00, A: 0xff})
	p := canvas.PixelAt(75, 110)
	t.Logf("对照1 纯绿: #%02x%02x%02x", p.R, p.G, p.B)

	var dump func(n RenderObject, depth int)
	dump = func(n RenderObject, depth int) {
		if x, y, w, h, ok := BoxGeometry(n); ok && w > 0 {
			t.Logf("box[%d] (%.0f,%.0f %.0fx%.0f)", depth, x, y, w, h)
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			dump(c, depth+1)
		}
	}
	dump(RenderObject(rv), 0)

	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 100, Height: 140})
	for _, y := range []int{20, 55, 90, 125} {
		var row []string
		for _, x := range []int{12, 36, 60} {
			p := canvas.PixelAt(x, y)
			row = append(row, fmt.Sprintf("#%02x%02x%02x", p.R, p.G, p.B))
		}
		t.Logf("y=%d: %v", y, row)
	}
	_ = builder
}
