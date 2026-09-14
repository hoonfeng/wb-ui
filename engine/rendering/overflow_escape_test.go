package rendering

import (
	"fmt"
	"image"
	"testing"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/html"
	"wb-ui/engine/html5"
	"wb-ui/engine/layout"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// buildDocHTML 解析 HTML → 渲染树 → 布局（consistency 同路径）。
func buildDocHTML(t *testing.T, w, h int, docHTML string) *RenderView {
	t.Helper()
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}
	doc, err := html.Parse(docHTML)
	if err != nil {
		t.Fatal(err)
	}
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	// CSS 注入：遍历 DOM <style>
	injectStyles(doc, resolver)
	builder := NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	rv.SetResolver(resolver)
	rv.SetViewportSize(float64(w), float64(h))
	state := layout.NewLayoutState(float64(w), float64(h))
	rv.Layout(state)
	return rv
}

// injectStyles 读取文档中的 <style> 文本加入 resolver（wbui.go 的 mergeCSSFromDOM 简化版）。
func injectStyles(doc *dom.Document, resolver *style.Resolver) {
	var styles []string
	var collect func(n dom.Node)
	collect = func(n dom.Node) {
		if el, ok := n.(*dom.Element); ok && el.LocalName() == "style" {
			if c := el.FirstChild(); c != nil {
				if t, ok := c.(*dom.Text); ok {
					styles = append(styles, t.Data())
				}
			}
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			collect(c)
		}
	}
	collect(doc)
	for _, s := range styles {
		sheet := css.NewCSSStyleSheet()
		sheet.SetOrigin(css.OriginAuthor)
		p := css.NewParser(s)
		p.SetOrigin(css.OriginAuthor)
		for _, r := range p.ParseStyleSheet() {
			sheet.AppendRule(r)
		}
		resolver.AddStyleSheet(sheet)
	}
}

func dumpLayers(t *testing.T, rv *RenderView) {
	var walk func(l *RenderLayer, d int)
	walk = func(l *RenderLayer, d int) {
		if l == nil {
			return
		}
		name := "?"
		if o := l.Owner(); o != nil {
			name = o.RenderName()
			if st := o.Style(); st != nil {
				pos := "static"
				switch st.Position {
				case style.PositionRelative:
					pos = "rel"
				case style.PositionAbsolute:
					pos = "abs"
				case style.PositionFixed:
					pos = "fixed"
				}
				name += fmt.Sprintf(" pos=%s ovfX=%d ovfY=%d", pos, st.OverflowX, st.OverflowY)
			}
		}
		lr, cr, clip := l.CalculateRects()
		t.Logf("%s%s lr=(%.0f,%.0f %.0fx%.0f) cr=(%.0f,%.0f %.0fx%.0f) clip=%v",
			indent(d), name, lr.X, lr.Y, lr.Width, lr.Height, cr.X, cr.Y, cr.Width, cr.Height, clip)
		for c := l.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c, d+1)
		}
	}
	walk(rv.RootLayer(), 0)
}

func indent(d int) string {
	s := ""
	for i := 0; i < d; i++ {
		s += "  "
	}
	return s
}

func TestLayerClipEscapeViewportCB(t *testing.T) {
	docHTML := `<!DOCTYPE html><html><head><meta charset="utf-8"><style>
		body{margin:0;background:transparent;overflow:hidden}
	</style></head><body>
		<div class="frame" style="position:absolute;inset:0;background:#00ff88"></div>
	</body></html>`
	rv := buildDocHTML(t, 400, 300, docHTML)
	dumpLayers(t, rv)

	// 直接检查逃逸判定
	var bodyLayer, frameLayer *RenderLayer
	var walkL func(l *RenderLayer)
	walkL = func(l *RenderLayer) {
		if l == nil {
			return
		}
		if o := l.Owner(); o != nil {
			if rb := asRenderBox(o); rb != nil {
				if el, ok := rb.Node().(*dom.Element); ok {
					if el.LocalName() == "body" {
						bodyLayer = l
					}
					if el.GetAttribute("class") == "frame" {
						frameLayer = l
					}
				}
			}
		}
		for c := l.FirstChild(); c != nil; c = c.NextSibling() {
			walkL(c)
		}
	}
	walkL(rv.RootLayer())
	t.Logf("bodyLayer=%v frameLayer=%v escapes=%v", bodyLayer != nil, frameLayer != nil, layerEscapesClip(frameLayer, bodyLayer))

	canvas := graphics.NewCanvas(400, 300)
	defer canvas.Release()
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 400, Height: 300})
	if got := canvas.PixelAt(200, 150); got.G < 200 || got.R > 100 || got.B > 200 {
		t.Fatalf("center pixel = %+v, want green (escape body clip)", got)
	}
	_ = image.Rect
}
