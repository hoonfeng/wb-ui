//go:build ignore

// 诊断 wb-ui linear-gradient 渲染能力：同页面渲染多种渐变写法，
// 输出每种写法的像素色彩分布（判断「调色板彩虹按钮不显示」根因）。
package main

import (
	"fmt"
	"os"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/html"
	"wb-ui/engine/html5"
	"wb-ui/engine/layout"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/rendering"
	"wb-ui/engine/style"
)

func collect(doc *dom.Document, resolver *style.Resolver) {
	var walk func(n dom.Node)
	walk = func(n dom.Node) {
		if el, ok := n.(*dom.Element); ok && el.LocalName() == "style" {
			if c := el.FirstChild(); c != nil {
				if t, ok := c.(*dom.Text); ok {
					sheet := css.NewCSSStyleSheet()
					sheet.SetOrigin(css.OriginAuthor)
					p := css.NewParser(t.Data())
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
}

func main() {
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

	htmlStr := `<!DOCTYPE html><html><head><style>
  .a { background:linear-gradient(90deg,#ff0000,#ffff00,#00ff00); width:90px; height:30px; }
  .b { background:linear-gradient(45deg,#ff0000,#ffff00); width:90px; height:30px; }
  .c { background:linear-gradient(135deg,#ff0000,#ffff00); width:90px; height:30px; }
  .d { background:linear-gradient(90deg,#ff0000,#ffff00,#00ff00,#00ffff,#0000ff,#ff00ff,#ff0000); width:90px; height:30px; }
  .e { background:linear-gradient(to bottom right,#ff0000,#ffff00); width:90px; height:30px; }
  .f { background:linear-gradient(0deg,#ff0000,#ffff00); width:90px; height:30px; }
</style></head><body>
<div class="a"></div><div class="b" style="margin-top:6px"></div>
<div class="c" style="margin-top:6px"></div><div class="d" style="margin-top:6px"></div>
<div class="e" style="margin-top:6px"></div><div class="f" style="margin-top:6px"></div>
</body></html>`

	doc, _ := html.Parse(htmlStr)
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	collect(doc, resolver)

	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	rv.SetResolver(resolver)
	rv.SetViewportSize(200, 256)
	state := layout.NewLayoutState(200, 256)
	rv.Layout(state)

	canvas := graphics.NewCanvas(200, 256)
	defer canvas.Release()
	rendering.Paint(rv, canvas, rendering.Rect{X: 0, Y: 0, Width: 200, Height: 256})

	names := []string{"a:90deg 红黄绿", "b:45deg 红黄", "c:135deg 红黄", "d:90deg 七色(同c但90)", "e:to bottom right 红黄", "f:0deg 红黄"}
	for i, name := range names {
		y := 15 + i*36
		var row []string
		for x := 5; x < 95; x += 10 {
			p := canvas.PixelAt(x, y)
			row = append(row, fmt.Sprintf("#%02x%02x%02x", p.R, p.G, p.B))
		}
		fmt.Printf("%s\n  %v\n", name, row)
	}
	os.Exit(0)
}
