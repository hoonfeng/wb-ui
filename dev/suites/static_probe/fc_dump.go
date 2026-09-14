// Command fc_dump renders flexcenter_test.html and prints the geometry of the
// three flex-column children, revealing the justify-content:center bug.
//go:build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/html"
	"wb-ui/engine/html5"
	"wb-ui/engine/layout"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/rendering"
	"wb-ui/engine/style"
)

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

	path := filepath.Join("dev", "suites", "static_probe", "flexcenter_test.html")
	data, _ := os.ReadFile(path)
	doc, _ := html.Parse(string(data))
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	var collect func(n dom.Node)
	collect = func(n dom.Node) {
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
			collect(c)
		}
	}
	collect(doc)

	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	rv.SetResolver(resolver)
	rv.SetViewportSize(1280, 800)
	state := layout.NewLayoutState(1280, 800)
	rv.Layout(state)

	fmt.Println("=== FLEX CENTER TEST ===")
	var walk func(o rendering.RenderObject)
	walk = func(o rendering.RenderObject) {
		if el, ok := o.Node().(*dom.Element); ok {
			cls := el.GetAttribute("class")
			if strings.Contains(cls, "welcome") {
				lb := o.LayoutBox()
				g := state.GeometryForBox(lb)
				st := lb.Style()
				fmt.Printf("  .%s xy=(%.0f,%.0f) wh=(%.0f,%.0f) disp=%s justify=%s gap=%v font=%v\n",
					cls, g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight(),
					st.Display.String(), st.JustifyContent, st.Gap, st.FontSize)
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(rv)
}
