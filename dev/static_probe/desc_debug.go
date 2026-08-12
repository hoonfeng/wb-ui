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

	path := filepath.Join("dev", "static_probe", "desc_test.html")
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

	// Find the button element.
	var btn *dom.Element
	var find func(n dom.Node)
	find = func(n dom.Node) {
		if el, ok := n.(*dom.Element); ok && el.LocalName() == "button" && btn == nil {
			btn = el
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			find(c)
		}
	}
	find(doc)
	if btn == nil {
		fmt.Println("no button")
		return
	}
	cs := resolver.ResolveElement(btn)
	fmt.Printf("button bg=#%02x%02x%02x parent=%s\n", cs.BackgroundColor.R, cs.BackgroundColor.G, cs.BackgroundColor.B, btn.ParentNode().NodeValue())
	if p, ok := btn.ParentNode().(*dom.Element); ok {
		fmt.Printf("parent class=%q\n", p.GetAttribute("class"))
	}
	// Directly test selector matching with the same DOM.
	checker := css.NewSelectorChecker()
	// Manual: parse ".activity-bar button" and match.
	sheet := css.NewCSSStyleSheet()
	pp := css.NewParser(".activity-bar button { color: red; }")
	pp.ParseStyleSheetInto(sheet)
	for _, r := range sheet.Rules() {
		if sr, ok := r.(*css.StyleRule); ok {
			for _, sel := range sr.Selectors.Selectors {
				m := checker.Match(sel, btn)
				fmt.Printf("match(.activity-bar button, button) = %v\n", m)
			}
		}
	}

	// ── Render tree check: what style did the render object get? ──
	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	rv.SetResolver(resolver)
	rv.SetViewportSize(1280, 800)
	state := layout.NewLayoutState(1280, 800)
	rv.Layout(state)
	var walk func(o rendering.RenderObject)
	count := 0
	walk = func(o rendering.RenderObject) {
		if o.Node() == btn {
			count++
			st := o.Style()
			rb, _ := o.(*rendering.RenderBox)
			if st != nil && rb != nil {
				fmt.Printf("render-tree button[%d] type=%T bg=#%02x%02x%02x xy=(%.0f,%.0f) wh=(%.0f,%.0f)\n",
					count, o, st.BackgroundColor.R, st.BackgroundColor.G, st.BackgroundColor.B,
					rb.X(), rb.Y(), rb.Width(), rb.Height())
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(rv)
	fmt.Printf("total boxes with Node()==btn: %d\n", count)

	// ── Paint and read the button pixel ──
	canvas := graphics.NewCanvas(1280, 800)
	defer canvas.Release()
	rendering.Paint(rv, canvas, rendering.Rect{X: 0, Y: 0, Width: 1280, Height: 800})
	p := canvas.PixelAt(10, 10)
	fmt.Printf("pixel(10,10) = #%02x%02x%02x a=%d\n", p.R, p.G, p.B, p.A)
}
