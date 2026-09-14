// Command test_click tests the Host's click handling without opening a window.
// It creates a WebView, loads HTML, builds render tree, and simulates clicks
// by directly calling the event processor.
package main

import (
	"fmt"
	"os"

	"wb-ui/engine/dom"
	"wb-ui/engine/html5"
	"wb-ui/engine/layout"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/rendering"
	"wb-ui/webkit"
)

func main() {
	// Init font system
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

	// Get the DOM document
	doc := wv.MainFrame().Document()
	if doc == nil {
		fmt.Println("ERROR: no document")
		os.Exit(1)
	}

	// Get RenderView
	rv := wv.RenderView()
	if rv == nil {
		fmt.Println("ERROR: no RenderView")
		os.Exit(1)
	}

	// ══════════════════════════════════════════
	// TEST 1: HitTest all form controls
	// ══════════════════════════════════════════
	fmt.Println("=== TEST 1: HitTest Form Controls ===")
	type hitPoint struct{ x, y float64; name string }
	points := []hitPoint{
		{44, 102, "Username input"},
		{44, 159, "Password input"},
		{44, 217, "Email input"},
		{44, 369, "Textarea"},
		{60, 497, "Primary button"},
		{44, 609, "Checkbox A"},
		{44, 772, "Radio 1"},
		{60, 935, "Select"},
		{44, 1037, "Progress bar"},
	}
	for _, p := range points {
		el := rendering.HitTest(rv, p.x, p.y, "")
		if el != nil {
			fmt.Printf("  %-20s -> %s type=%q class=%q\n", p.name, el.LocalName(), el.GetAttribute("type"), el.ClassName())
		} else {
			fmt.Printf("  %-20s -> nil!\n", p.name)
		}
	}

	// ══════════════════════════════════════════
	// TEST 2: Checkbox toggle via HitTest + ToInputElement
	// ══════════════════════════════════════════
	fmt.Println("\n=== TEST 2: Checkbox Toggle ===")
	cbEl := rendering.HitTest(rv, 44, 609, "")
	if cbEl != nil {
		cbIn, cbOk := html5.ToInputElement(cbEl)
		if cbOk {
			fmt.Printf("  Initial: checked=%v attr=%q\n", cbIn.Checked(), cbEl.GetAttribute("checked"))
			cbIn.SetChecked(!cbIn.Checked())
			fmt.Printf("  After toggle: checked=%v attr=%q\n", cbIn.Checked(), cbEl.GetAttribute("checked"))
			wv.RebuildRenderTree()
			wv.EnsureLayout()
			// Re-check after rebuild
			doc2 := wv.MainFrame().Document()
			if doc2 != nil {
				allCbs := doc2.GetElementsByTagName("input")
				for _, inp := range allCbs {
					if inp.GetAttribute("type") == "checkbox" {
						if in2, ok2 := html5.ToInputElement(inp); ok2 {
							fmt.Printf("  After rebuild: checkbox checked=%v\n", in2.Checked())
						}
					}
				}
			}
		}
	} else {
		fmt.Println("  ERROR: cannot find checkbox at (44,609)")
	}

	// ══════════════════════════════════════════
	// TEST 3: Full click simulation (Host-style)
	// ══════════════════════════════════════════
	fmt.Println("\n=== TEST 3: Full Click Chain (Focus + Toggle) ===")
	
	// Create a mock Host using PostEvent for testing.
	// Since NewHost creates a real window (which we don't want for testing),
	// we test the individual components directly.
	
	// Simulate what Host.processEvents does:
	// 1. HitTest at checkbox position
	clickX, clickY := 44.0, 609.0
	hitEl := rendering.HitTest(rv, clickX, clickY, "")
	if hitEl != nil {
		isInput := hitEl.LocalName() == "input"
		inputType := hitEl.GetAttribute("type")
		isCBorRadio := isInput && (inputType == "checkbox" || inputType == "radio")
		isTextFC := isInput && inputType != "checkbox" && inputType != "radio" && inputType != "submit" && inputType != "reset"
		
		fmt.Printf("  HitEl: %s type=%q\n", hitEl.LocalName(), inputType)
		fmt.Printf("  isInput=%v isCBorRadio=%v isTextFormControl=%v\n", isInput, isCBorRadio, isTextFC)
		
		if isCBorRadio {
			fmt.Println("  → Should toggle checkbox/radio")
		} else if hitEl.LocalName() == "textarea" || isTextFC {
			fmt.Println("  → Should focus text form control")
		}
		
		// Test FocusElement
		if hitEl.LocalName() == "textarea" {
			hitEl.SetFocused(true)
			fmt.Printf("  Focused set on textarea: %v\n", hitEl.IsFocused())
		}
	}

	// ══════════════════════════════════════════
	// TEST 4: Render tree structure verification
	// ══════════════════════════════════════════
	fmt.Println("\n=== TEST 4: Render Tree Children (Password region) ===")
	// Find the password input's render object and check its parent
	var walkRO func(rendering.RenderObject, int)
	walkRO = func(o rendering.RenderObject, depth int) {
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
				nfo = fmt.Sprintf(" <%s type=%q>", el.LocalName(), el.GetAttribute("type"))
			}
		}
		if lb := o.LayoutBox(); lb != nil {
			if ls := rv.LayoutState(); ls != nil {
				g := ls.GeometryForBox(lb)
				x, y, w, h := g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight()
				fmt.Printf("%s%s (%.0f,%.0f %.0fx%.0f)%s\n", pad, name, x, y, w, h, nfo)
			}
		} else {
			fmt.Printf("%s%s%s\n", pad, name, nfo)
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walkRO(c, depth+1)
		}
	}
	walkRO(rendering.RenderObject(rv), 0)

	fmt.Println("\n=== TEST COMPLETE ===")
}
