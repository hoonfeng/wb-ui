//go:build ignore

// Command vcenter_dump renders dev/fixtures/xseg/xseg_real.html and prints the
// geometry of .txt (flex align-items:center) box, its span flex item, and
// text segments — to diagnose vertical centering of CJK text in flex.
package main

import (
	"fmt"
	"os"
	"path/filepath"

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

	data, _ := os.ReadFile(filepath.Join("dev", "fixtures", "xseg", "xseg_real.html"))
	doc, _ := html.Parse(string(data))
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	collect(doc, resolver)

	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	rv.SetResolver(resolver)
	rv.SetViewportSize(360, 200)
	state := layout.NewLayoutState(360, 200)
	rv.Layout(state)

	fmt.Println("=== VCENTER DUMP ===")
	// 顶级绝对坐标换算：打印每个 box 的 rect（绝对），seg 相对于其父 box 内容盒。
	var walk func(lb *layout.ElementBox, depth int)
	walk = func(lb *layout.ElementBox, depth int) {
		g := state.GeometryForBox(lb)
		indent := ""
		for i := 0; i < depth; i++ {
			indent += "  "
		}
		name := "(anon)"
		cls := ""
		if lb.Element() != nil {
			name = lb.Element().LocalName()
			cls = lb.Element().GetAttribute("class")
		}
		st := lb.Style()
		fmt.Printf("%s[box] %-16s absRect=(%.1f,%.1f w=%.1f h=%.1f) disp=%s lh=%.2f fs=%.1f\n",
			indent, name+"."+cls, g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight(),
			st.Display.String(), st.LineHeight.Value, st.FontSize)
		for _, c := range lb.Children() {
			if itb, ok := c.(*layout.InlineTextBox); ok {
				for _, seg := range itb.TextSegments {
					fmt.Printf("%s  [text %q] seg(rel content) x=%.1f y=%.1f w=%.1f h=%.1f LineY=%.1f LineH=%.2f\n",
						indent, itb.Text(), seg.X, seg.Y, seg.Width, seg.Height, seg.LineY, seg.LineHeight)
				}
			}
			if eb, ok := c.(*layout.ElementBox); ok {
				walk(eb, depth+1)
			}
		}
	}
	walk(rv.LayoutBox(), 0)
}
