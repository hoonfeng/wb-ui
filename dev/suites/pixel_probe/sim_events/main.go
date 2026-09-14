// Command sim_events renders step17_form.html through the full pipeline and
// simulates mouse clicks on form controls, checking element state and pixel
// output after each click.
package main

import (
	"fmt"
	"os"
	"strings"

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

	doc, err := html.Parse(htmlSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: parse HTML: %v\n", err)
		os.Exit(1)
	}

	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	if doc.DocumentElement() != nil {
		extractStyles(doc.DocumentElement(), resolver)
	}

	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	if rv == nil {
		fmt.Fprintln(os.Stderr, "ERROR: RenderView is nil")
		os.Exit(1)
	}
	rv.SetViewportSize(600, 1000)
	rv.Layout(nil)

	findEl := func(localName, attrName, attrVal string) *dom.Element {
		root := doc.DocumentElement()
		if root == nil {
			return nil
		}
		return findElement(root, localName, attrName, attrVal)
	}

	// ── 1. CHECKBOX TEST ──
	fmt.Println("=== CHECKBOX CLICK TEST ===")
	cb1 := findEl("input", "type", "checkbox")
	if cb1 == nil {
		fmt.Println("ERROR: checkbox not found!")
		os.Exit(1)
	}
	fmt.Printf("Checkbox found: name=%q\n", cb1.GetAttribute("name"))

	// Check initial checked state
	cbIn, cbOk := html5.ToInputElement(cb1)
	if cbOk {
		fmt.Printf("  Initial checked=%v (via HTMLInputElement)\n", cbIn.Checked())
		fmt.Printf("  Initial attr checked=%q\n", cb1.GetAttribute("checked"))
	} else {
		fmt.Println("  ERROR: ToInputElement returned false!")
	}

	// HitTest the checkbox position
	hitEl := rendering.HitTest(rv, 44, 609, "")
	fmt.Printf("  HitTest at (44,609) -> localName=%q", func() string {
		if hitEl != nil { return hitEl.LocalName() }; return "nil"
	}())

	// Simulate click: toggle via html5.ToInputElement (same as first block in host.go)
	if cbOk {
		cbIn.SetChecked(!cbIn.Checked())
		fmt.Printf("  After toggle: checked=%v attr=%q\n", cbIn.Checked(), cb1.GetAttribute("checked"))
		rv.Layout(nil)
	}

	// Toggle back
	if cbOk {
		cbIn.SetChecked(!cbIn.Checked())
		fmt.Printf("  After toggle back: checked=%v attr=%q\n", cbIn.Checked(), cb1.GetAttribute("checked"))
		rv.Layout(nil)
	}

	// ── 2. RADIO TEST ──
	fmt.Println("\n=== RADIO CLICK TEST ===")
	radio1 := findEl("input", "type", "radio")
	if radio1 == nil {
		fmt.Println("ERROR: radio not found!")
		os.Exit(1)
	}
	radioName := radio1.GetAttribute("name")
	fmt.Printf("Radio found: name=%q\n", radioName)

	rIn, rOk := html5.ToInputElement(radio1)
	if rOk {
		fmt.Printf("  Initial checked=%v (via HTMLInputElement)\n", rIn.Checked())
		fmt.Printf("  Initial attr checked=%q\n", radio1.GetAttribute("checked"))
	}

	// Find all radios with same name
	root := doc.DocumentElement()
	allRadios := findAllElements(root, "input", "type", "radio", radioName)
	fmt.Printf("  Found %d radios with name=%q\n", len(allRadios), radioName)
	for i, r := range allRadios {
		ri, _ := html5.ToInputElement(r)
		fmt.Printf("    radio[%d]: checked=%v attr=%q\n", i, ri.Checked(), r.GetAttribute("checked"))
	}

	// Simulate click on radio1
	if rOk {
		rIn.SetChecked(true)
		fmt.Printf("  After radio1.SetChecked(true): checked=%v\n", rIn.Checked())
	}

	// ── 3. HIT TEST ALL FORM CONTROLS ──
	fmt.Println("\n=== HIT TEST: ALL FORM CONTROLS ===")
	hitPoints := []struct {
		x, y int
		desc string
	}{
		{44, 102, "Username input"},
		{44, 159, "Password input"},
		{44, 217, "Email input"},
		{44, 369, "Textarea"},
		{60, 497, "Primary button"},
		{44, 609, "Checkbox Option A"},
		{44, 634, "Checkbox Option B"},
		{44, 659, "Checkbox Option C"},
		{44, 772, "Radio Option 1"},
		{44, 797, "Radio Option 2"},
		{44, 822, "Radio Option 3"},
		{60, 935, "Select"},
	}

	for _, hp := range hitPoints {
		hit := rendering.HitTest(rv, float64(hp.x), float64(hp.y), "")
		if hit != nil {
			t := hit.GetAttribute("type")
			fmt.Printf("  (%3d,%3d) %-20s -> %s type=%q\n", hp.x, hp.y, hp.desc, hit.LocalName(), t)
		} else {
			fmt.Printf("  (%3d,%3d) %-20s -> nil (NOT HIT!)\n", hp.x, hp.y, hp.desc)
		}
	}

	// ── 4. VERIFY isTextFormControl for textarea ──
	fmt.Println("\n=== TEXT FORM CONTROL CHECK ===")
	textarea := findEl("textarea", "", "")
	if textarea != nil {
		fmt.Printf("  textarea: localName=%q, isTextFormControl=%v\n",
			textarea.LocalName(), isTextFormControl(textarea))
	} else {
		fmt.Println("  textarea not found")
	}
	
	// Check button
	btn := findEl("button", "class", "primary")
	if btn != nil {
		hitBtn := rendering.HitTest(rv, 60, 497, "")
		fmt.Printf("  button.primary: localName=%q, .hasAttr(type)=%q, hitByEmptyAttr=%v\n",
			btn.LocalName(), btn.GetAttribute("type"),
			hitBtn != nil && hitBtn == btn)
	}

	// ── 5. PIXEL CHECK ──
	fmt.Println("\n=== PIXEL CHECK ===")
	canvas := graphics.NewCanvas(600, 1000)
	defer canvas.Release()

	rendering.Paint(rv, canvas, rendering.Rect{X: 0, Y: 0, Width: 600, Height: 1000})

	// Check button primary (should be #1f6feb blue)
	p := canvas.PixelAt(60, 497)
	fmt.Printf("  Primary button (60,497) = #%02x%02x%02x (expect 1f6feb)\n", p.R, p.G, p.B)

	// Check checkbox
	p = canvas.PixelAt(44, 609)
	fmt.Printf("  Checkbox (44,609) = #%02x%02x%02x\n", p.R, p.G, p.B)

	// Check radio
	p = canvas.PixelAt(44, 772)
	fmt.Printf("  Radio (44,772) = #%02x%02x%02x\n", p.R, p.G, p.B)

	fmt.Println("\nDONE")
}

func findElement(n dom.Node, localName, attrName, attrVal string) *dom.Element {
	el, ok := n.(*dom.Element)
	if ok {
		if el.LocalName() == localName {
			if attrName == "" {
				return el
			}
			if el.GetAttribute(attrName) == attrVal {
				return el
			}
		}
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if found := findElement(c, localName, attrName, attrVal); found != nil {
			return found
		}
	}
	return nil
}

func findAllElements(n dom.Node, localName, attrName, attrVal, nameAttr string) []*dom.Element {
	var result []*dom.Element
	el, ok := n.(*dom.Element)
	if ok && el.LocalName() == localName {
		if attrName == "" || el.GetAttribute(attrName) == attrVal {
			if nameAttr == "" || el.GetAttribute("name") == nameAttr {
				result = append(result, el)
			}
		}
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		result = append(result, findAllElements(c, localName, attrName, attrVal, nameAttr)...)
	}
	return result
}

func extractStyles(el dom.Node, resolver *style.Resolver) {
	for c := el.FirstChild(); c != nil; c = c.NextSibling() {
		if child, ok := c.(*dom.Element); ok && child.LocalName() == "style" {
			cssText := child.TextContent()
			if strings.TrimSpace(cssText) != "" {
				sheet := css.NewCSSStyleSheet()
				css.NewParser(cssText).ParseStyleSheetInto(sheet)
				resolver.AddStyleSheet(sheet)
			}
		}
		extractStyles(c, resolver)
	}
}

func isTextFormControl(el *dom.Element) bool {
	if el == nil {
		return false
	}
	switch el.LocalName() {
	case "textarea":
		return true
	case "input":
		t := el.GetAttribute("type")
		switch strings.ToLower(t) {
		case "checkbox", "radio", "range", "color", "file",
			"submit", "reset", "button", "image", "hidden":
			return false
		}
		return true
	default:
		return false
	}
}
