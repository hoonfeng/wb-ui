//go:build ignore

// Command xseg_dump renders dev/xseg_probe/xseg.html and prints the geometry
// of the xseg flex container, each flex:1 span, and its inline text segments.
// Purpose: diagnose text-align:center inside flex items shifting text right.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/html5"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/rendering"
	"wb-ui/style"
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

	data, _ := os.ReadFile(filepath.Join("dev", "xseg_probe", "xseg.html"))
	doc, _ := html.Parse(string(data))
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	collect(doc, resolver)

	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	rv.SetResolver(resolver)
	rv.SetViewportSize(500, 320)
	state := layout.NewLayoutState(500, 320)
	rv.Layout(state)

	fmt.Println("=== XSEG DUMP ===")
	var walkLayout func(lb *layout.ElementBox, depth int)
	walkLayout = func(lb *layout.ElementBox, depth int) {
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
		fmt.Printf("%s[box] %-24s rect=(%.1f,%.1f w=%.1f h=%.1f) contentBox=(%.1f,%.1f w=%.1f h=%.1f) disp=%s textAlign=%v\n",
			indent, name+"."+cls, g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight(),
			g.ContentBoxLeft(), g.ContentBoxTop(), g.ContentWidth(), g.ContentHeight(),
			st.Display.String(), st.TextAlign)
		for _, c := range lb.Children() {
			if itb, ok := c.(*layout.InlineTextBox); ok {
				for _, seg := range itb.TextSegments {
					fmt.Printf("%s  [text %q] seg x=%.1f y=%.1f w=%.1f h=%.1f\n", indent, itb.Text(), seg.X, seg.Y, seg.Width, seg.Height)
				}
			}
			if eb, ok := c.(*layout.ElementBox); ok {
				walkLayout(eb, depth+1)
			}
		}
	}
	walkLayout(rv.LayoutBox(), 0)
}
