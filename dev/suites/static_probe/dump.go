//go:build ignore

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

	path := filepath.Join("dev", "suites", "static_test", "ide_static.html")
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}
	doc, err := html.Parse(string(data))
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse:", err)
		os.Exit(1)
	}
	// Count DOM elements
	domCount := 0
	var count func(n dom.Node)
	count = func(n dom.Node) {
		if _, ok := n.(*dom.Element); ok {
			domCount++
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			count(c)
		}
	}
	count(doc)
	fmt.Printf("DOM elements: %d\n", domCount)

	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	mergeCSSFromDOM(doc, resolver)

	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	rv.SetResolver(resolver)
	if rv == nil {
		fmt.Println("RenderView nil")
		os.Exit(1)
	}
	// Count render objects
	roCount := 0
	var walk func(o rendering.RenderObject)
	walk = func(o rendering.RenderObject) {
		roCount++
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(rv)
	fmt.Printf("Render objects: %d\n", roCount)

	rv.SetViewportSize(1280, 800)
	state := layout.NewLayoutState(1280, 800)
	rv.Layout(state)

	// Dump geometry of the first N elements with size
	shown := 0
	var dump func(o rendering.RenderObject, depth int)
	dump = func(o rendering.RenderObject, depth int) {
		if shown >= 40 {
			return
		}
		if rb, ok := o.(*rendering.RenderBox); ok {
			w, h := rb.Width(), rb.Height()
			if w > 0 || h > 0 {
				display := "?"
				if st := rb.Style(); st != nil {
					display = st.Display.String()
				}
				nm := rb.RenderName()
				if nm == "" {
					nm = "?"
				}
				fmt.Printf("  %s(%d) %-16s xy=(%.0f,%.0f) wh=(%.0f,%.0f) disp=%s\n",
					pad(depth), shown, nm, rb.X(), rb.Y(), w, h, display)
				shown++
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			dump(c, depth+1)
		}
	}
	dump(rv, 0)
}

func pad(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		s += "  "
	}
	return s
}
