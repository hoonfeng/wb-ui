//go:build ignore

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

func mergeCSSFromDOM(doc *dom.Document, resolver *style.Resolver) {
	var collect func(n dom.Node, styles *[]string)
	collect = func(n dom.Node, styles *[]string) {
		if el, ok := n.(*dom.Element); ok && el.LocalName() == "style" {
			if c := el.FirstChild(); c != nil {
				if t, ok := c.(*dom.Text); ok {
					*styles = append(*styles, t.Data())
				}
			}
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			collect(c, styles)
		}
	}
	var styles []string
	collect(doc, &styles)
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

	path := filepath.Join("dev", "static_probe", "grid_app.html")
	data, _ := os.ReadFile(path)
	doc, _ := html.Parse(string(data))
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	mergeCSSFromDOM(doc, resolver)

	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	rv.SetResolver(resolver)
	rv.SetViewportSize(1280, 800)
	state := layout.NewLayoutState(1280, 800)
	rv.Layout(state)

	// Dump layout tree geometry for root + key nodes.
	lbRoot := rv.LayoutBox()
	if lbRoot == nil {
		fmt.Println("layout root nil")
		return
	}
	dumpLB(lbRoot, state, 0)
}

func dumpLB(lb *layout.ElementBox, state *layout.LayoutState, depth int) {
	if lb == nil || depth > 8 {
		return
	}
	g := state.GeometryForBox(lb)
	var disp string
	if st := lb.Style(); st != nil {
		disp = st.Display.String()
	}
	elName := ""
	if el := lb.Element(); el != nil {
		elName = el.LocalName()
		if c := el.GetAttribute("class"); c != "" {
			elName += "." + c[:min(24, len(c))]
		}
	}
	fmt.Printf("  LB[%d] %-24s xy=(%.0f,%.0f) wh=(%.0f,%.0f) disp=%s\n",
		depth, elName, g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight(), disp)
	for _, ch := range lb.Children() {
		if eb, ok := ch.(*layout.ElementBox); ok {
			dumpLB(eb, state, depth+1)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
