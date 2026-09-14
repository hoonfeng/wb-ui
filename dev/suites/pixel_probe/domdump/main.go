// Command comprehenv renders HTML through wb-ui and dumps the full element
// data (computed styles, box model, render tree) for comparison against a
// real browser's Elements panel data.
package main

import (
	"fmt"
	"os"

	"wb-ui/dom"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/rendering"
	"wb-ui/webkit"
)

func main() {
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

	data, err := os.ReadFile("dev/suites/pixel_probe/test_html/step17_form.html")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	wv := webkit.NewWebView()
	err = wv.LoadHTML(string(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: LoadHTML: %v\n", err)
		os.Exit(1)
	}
	wv.Resize(600, 1000)
	wv.EnsureLayout()

	rv := wv.RenderView()
	if rv == nil {
		fmt.Println("ERROR: no RenderView")
		os.Exit(1)
	}

	doc := wv.MainFrame().Document()
	if doc == nil {
		fmt.Println("ERROR: no Document")
		os.Exit(1)
	}

	fmt.Println("=== wb-ui Full Element Data (computed + box model) ===")
	
	// Walk all DOM elements and print their wb-ui computed "styles" + render tree info
	allElements := doc.GetElementsByTagName("*")
	for _, el := range allElements {
		tag := el.LocalName()
		cls := el.ClassName()
		ro := findRenderObjectForNode(rendering.RenderObject(rv), el)
		
		// Get render object info
		var rx, ry, rw, rh float64
		var renderType string
		var roType string
		var hasRO bool
		if ro != nil {
			hasRO = true
			roType = ro.RenderName()
			if lb := ro.LayoutBox(); lb != nil {
				if ls := rv.LayoutState(); ls != nil {
					g := ls.GeometryForBox(lb)
					rx, ry, rw, rh = g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight()
				}
			}
			// Get computed style from the element's RenderObject
			if st := ro.Style(); st != nil {
				renderType = fmt.Sprintf(" display=%s overflow=%s bg=#%02x%02x%02x",
					st.Display.String(), st.OverflowX.String(), st.BackgroundColor.R, st.BackgroundColor.G, st.BackgroundColor.B)
			}
		}
		
		attrStr := ""
		for _, attr := range []string{"type", "value", "name", "placeholder", "checked", "selected"} {
			v := el.GetAttribute(attr)
			if v != "" {
				attrStr += fmt.Sprintf(" %s=%q", attr, v)
			}
		}
		if el.IsFocused() {
			attrStr += " focused"
		}

		id := ""
		if el.GetId() != "" {
			id = "#" + el.GetId()
		}
		dot := ""
		if cls != "" {
			dot = "." + cls
		}

		fmt.Printf("\n<%s%s%s>%s\n", tag, id, dot, attrStr)
		if hasRO {
			fmt.Printf("  RenderObject : %s\n", roType)
			fmt.Printf("  Box          : (%.0f, %.0f) %.0f x %.0f\n", rx, ry, rw, rh)
			fmt.Printf("  RenderExtra  : %s\n", renderType)
			
			// Hit-test this element with the specific edge coordinate (44,159) for password
			hitX := rx + rw/2
			hitY := ry + rh/2
			// For password input, also test the edge coordinate
			if tag == "input" && el.GetAttribute("type") == "password" {
				fmt.Printf("  HitTest-edge  : ")
				hitEdge := rendering.HitTest(rv, 44, 159, "")
				if hitEdge == el {
					fmt.Printf("OK at (44,159)\n")
				} else if hitEdge != nil {
					fmt.Printf("WRONG! got %s type=%q at (44,159)\n", hitEdge.LocalName(), hitEdge.GetAttribute("type"))
				} else {
					fmt.Printf("MISS at (44,159)\n")
				}
			}
			hitEl := rendering.HitTest(rv, hitX, hitY, "")
			if hitEl == el {
				fmt.Printf("  HitTest      : OK (center at %.0f,%.0f)\n", hitX, hitY)
			} else if hitEl != nil {
				fmt.Printf("  HitTest      : WRONG! got %s at (%.0f,%.0f)\n", hitEl.LocalName(), hitX, hitY)
			} else {
				fmt.Printf("  HitTest      : MISS at (%.0f,%.0f)\n", hitX, hitY)
			}
		} else {
			fmt.Printf("  RenderObject : NONE\n")
		}
	}

	// Also dump the complete render tree
	fmt.Println("\n=== RENDER TREE (full) ===")
	dumpTree(rendering.RenderObject(rv), 0)
}

func findRenderObjectForNode(ro rendering.RenderObject, target *dom.Element) rendering.RenderObject {
	if ro == nil || target == nil {
		return nil
	}
	if n := ro.Node(); n != nil {
		if e, ok := n.(*dom.Element); ok && e == target {
			return ro
		}
	}
	for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
		if found := findRenderObjectForNode(c, target); found != nil {
			return found
		}
	}
	return nil
}

func dumpTree(o rendering.RenderObject, depth int) {
	if o == nil {
		return
	}
	pad := ""
	for i := 0; i < depth; i++ {
		pad += "  "
	}
	name := o.RenderName()
	var nfo string
	if n := o.Node(); n != nil {
		if el, ok := n.(*dom.Element); ok {
			cn := el.ClassName()
			tn := el.LocalName()
			typ := el.GetAttribute("type")
			nfo = fmt.Sprintf(" <%s", tn)
			if cn != "" {
				nfo += "." + cn
			}
			if typ != "" {
				nfo += fmt.Sprintf(" type=%q", typ)
			}
			nfo += ">"
		}
	}
	if lb := o.LayoutBox(); lb != nil {
		if ls := o.View().LayoutState(); ls != nil {
			g := ls.GeometryForBox(lb)
			x, y, w, h := g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight()
			fmt.Printf("%s%s (%.0f,%.0f %.0fx%.0f)%s\n", pad, name, x, y, w, h, nfo)
		} else {
			fmt.Printf("%s%s%s\n", pad, name, nfo)
		}
	} else {
		fmt.Printf("%s%s%s\n", pad, name, nfo)
	}
	for c := o.FirstChild(); c != nil; c = c.NextSibling() {
		dumpTree(c, depth+1)
	}
}
