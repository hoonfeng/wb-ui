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

func main() {
	data, err := os.ReadFile("dev/suites/pixel_probe/test_html/step17_form.html")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	htmlSrc := string(data)

	mgr := graphics.InitFontManager("")
	if mgr != nil {
		mgr.LoadSystemFonts()
	}
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}

	// ═══════════════════════════════════════
	// FULL CSS TEST: using the step17_form.html
	// ═══════════════════════════════════════
	doc, err := html.Parse(htmlSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	r := style.NewResolver()
	r.AddStyleSheet(html5.NewUAStyleSheet())
	// Extract <style> elements
	extractStyles(doc.DocumentElement(), r)

	b := rendering.NewRenderTreeBuilder(r)
	rv := b.Build(doc)
	if rv == nil {
		fmt.Fprintln(os.Stderr, "ERROR: no RV")
		os.Exit(1)
	}
	rv.SetViewportSize(600, 1000)
	rv.Layout(nil)

	fmt.Println("=== FULL CSS TEST ===")
	fmt.Println("--- Render tree (password region) ---")
	printRegion(rendering.RenderObject(rv), 120, 180)

	fmt.Println("\n--- HitTest near password input ---")
	for y := 155; y <= 195; y += 3 {
		hit := rendering.HitTest(rv, 44, float64(y), "")
		if hit != nil {
			fmt.Printf("  (44,%d) -> %s", y, hit.LocalName())
			if t := hit.GetAttribute("type"); t != "" {
				fmt.Printf(" type=%q", t)
			}
			fmt.Println()
		} else {
			fmt.Printf("  (44,%d) -> nil\n", y)
		}
	}
	_ = layout.MeasureTextFunc
}

func printRegion(o rendering.RenderObject, minY, maxY float64) {
	if o == nil {
		return
	}
	var x, y, w, h float64
	hasBounds := false
	if lb := o.LayoutBox(); lb != nil {
		if ls := o.View().LayoutState(); ls != nil {
			g := ls.GeometryForBox(lb)
			x, y, w, h = g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight()
			hasBounds = true
		}
	}
	// Only print if within region
	if hasBounds && y >= minY && y <= maxY {
		name := o.RenderName()
		var nfo string
		if n := o.Node(); n != nil {
			if el, ok := n.(*dom.Element); ok {
				nfo = fmt.Sprintf(" <%s", el.LocalName())
				if t := el.GetAttribute("type"); t != "" {
					nfo += fmt.Sprintf(" type=%q", t)
				}
				nfo += ">"
			}
		} else {
			nfo = " [anon]"
		}
		fmt.Printf("  %s (%.0f,%.0f %.0fx%.0f)%s\n", name, x, y, w, h, nfo)
	} else if !hasBounds {
		name := o.RenderName()
		var nfo string
		if n := o.Node(); n != nil {
			if el, ok := n.(*dom.Element); ok {
				nfo = fmt.Sprintf(" <%s>", el.LocalName())
			}
		}
		if nfo != "" {
			fmt.Printf("  %s%s (no layout box)\n", name, nfo)
		}
	}
	// Also print if has bounds and overlaps region
	if !hasBounds || (y+h >= minY && y <= maxY) {
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			printRegion(c, minY, maxY)
		}
	}
}

func extractStyles(el dom.Node, resolver *style.Resolver) {
	var root dom.Node = el
	if doc, ok := el.(*dom.Document); ok {
		root = doc.DocumentElement()
	}
	if root == nil { return }
	var walk func(dom.Node)
	walk = func(n dom.Node) {
		if n == nil { return }
		if child, ok := n.(*dom.Element); ok && child.LocalName() == "style" {
			cssText := child.TextContent()
			if len(cssText) > 0 {
				sheet := css.NewCSSStyleSheet()
				// Use simple whitespace check
				hasNonSpace := false
				for _, r := range cssText {
					if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
						hasNonSpace = true
						break
					}
				}
				if hasNonSpace {
					css.NewParser(cssText).ParseStyleSheetInto(sheet)
					resolver.AddStyleSheet(sheet)
				}
			}
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(root)
}
