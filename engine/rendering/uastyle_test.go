// UA stylesheet end-to-end tests.
// These verify that the User-Agent default stylesheet (engine/html5/defaultcss.go)
// is applied through the full parse → resolve → layout pipeline, producing
// browser-matching defaults for headings, links, and lists.

package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/html"
	"wb-ui/engine/html5"
	"wb-ui/engine/layout"
	"wb-ui/engine/style"
)

// findElement walks the DOM and returns the first element whose local name
// matches name (or whose tag+class matches "tag.class").
func findElement(root *dom.Element, sel string) *dom.Element {
	tag, class := sel, ""
	for i := 0; i < len(sel); i++ {
		if sel[i] == '.' {
			tag, class = sel[:i], sel[i+1:]
			break
		}
	}
	var found *dom.Element
	var walk func(el *dom.Element)
	walk = func(el *dom.Element) {
		if found != nil {
			return
		}
		if el.LocalName() == tag && (class == "" || el.GetAttribute("class") == class) {
			found = el
			return
		}
		for c := el.FirstChild(); c != nil; c = c.NextSibling() {
			if e, ok := c.(*dom.Element); ok {
				walk(e)
			}
		}
	}
	walk(root)
	return found
}

// resolveWithUA parses src with the UA sheet and returns the computed style of
// the first element matching sel ("h1", "ul", "li", ...).
func resolveWithUA(t *testing.T, src, sel string) *style.ComputedStyle {
	t.Helper()
	doc, err := html.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	root := doc.DocumentElement()
	if root == nil {
		t.Fatal("no document element")
	}
	el := findElement(root, sel)
	if el == nil {
		t.Fatalf("no element matches %q in %q", sel, src)
	}
	return resolver.ResolveElement(el)
}

func TestUAStyleHeadingFontSize(t *testing.T) {
	cs := resolveWithUA(t, `<html><body><h1>Title</h1><h6>small</h6></body></html>`, "h1")
	// UA stylesheet declares font-size:2em; the computed style keeps the
	// relative unit, and the layout engine resolves it against the parent
	// font-size (16px × 2 = 32px). Verify the declaration was applied.
	if cs.FontSize.Value != 2 || cs.FontSize.Unit != "em" {
		t.Errorf("h1 font-size = %v, want 2em (UA default)", cs.FontSize)
	}
	if cs.FontWeight != "bold" {
		t.Errorf("h1 font-weight = %q, want bold", cs.FontWeight)
	}

	cs6 := resolveWithUA(t, `<html><body><h1>Title</h1><h6>small</h6></body></html>`, "h6")
	if cs6.FontSize.Unit != "em" || cs6.FontSize.Value != 0.67 {
		t.Errorf("h6 font-size = %v, want 0.67em (UA default)", cs6.FontSize)
	}
}

func TestUAStyleLinkColor(t *testing.T) {
	cs := resolveWithUA(t, `<html><body><a href="x">link</a></body></html>`, "a")
	if cs.Color.R != 0x00 || cs.Color.G != 0x00 || cs.Color.B != 0xEE {
		t.Errorf("a color = %+v, want link blue (0,0,EE)", cs.Color)
	}
	if cs.TextDecoration != "underline" {
		t.Errorf("a text-decoration = %q, want underline", cs.TextDecoration)
	}
}

func TestUAStyleListIndent(t *testing.T) {
	cs := resolveWithUA(t, `<html><body><ul><li>item</li></ul></body></html>`, "ul")
	if cs.Display != style.DisplayBlock {
		t.Errorf("ul display = %v, want block", cs.Display)
	}
	if cs.PaddingLeft.Value != 40 || cs.PaddingLeft.Unit != "px" {
		t.Errorf("ul padding-left = %v, want 40px", cs.PaddingLeft)
	}

	csLi := resolveWithUA(t, `<html><body><ul><li>item</li></ul></body></html>`, "li")
	if csLi.Display != style.DisplayListItem {
		t.Errorf("li display = %v, want list-item", csLi.Display)
	}
}

func TestUAStyleBodyMargin(t *testing.T) {
	cs := resolveWithUA(t, `<html><body><p>text</p></body></html>`, "p")
	if cs.Display != style.DisplayBlock {
		t.Errorf("p display = %v, want block", cs.Display)
	}
	if cs.MarginTop.Value != 1 || cs.MarginTop.Unit != "em" {
		t.Errorf("p margin-top = %v, want 1em", cs.MarginTop)
	}
}

func TestUAStyleLayoutHeading(t *testing.T) {
	// Full layout: h1 must render at 2em font size and occupy its own line.
	doc, err := html.Parse(`<html><body><h1>Hello</h1></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	root := doc.DocumentElement()
	lb := layout.BuildLayoutTree(root, resolver)
	if lb == nil {
		t.Fatal("layout tree nil")
	}
	rootEb, ok := lb.(*layout.ElementBox)
	if !ok {
		t.Fatalf("root not *ElementBox: %T", lb)
	}
	state := layout.Layout(rootEb, 800, 600)
	// Find the h1 box.
	var h1box *layout.ElementBox
	var walk func(b layout.Box)
	walk = func(b layout.Box) {
		if eb, ok := b.(*layout.ElementBox); ok {
			if el := eb.Element(); el != nil && el.LocalName() == "h1" {
				h1box = eb
				return
			}
			for _, c := range eb.Children() {
				walk(c)
			}
		}
	}
	walk(rootEb)
	if h1box == nil {
		t.Fatal("h1 box not found")
	}
	g := state.GeometryForBox(h1box)
	// 2em at default 16px = 32px line box (line-height 1.2 → ~38px). Box must
	// be at least 30px tall, proving font-size:2em took effect.
	if g.BorderBoxHeight() < 30 {
		t.Errorf("h1 box height = %g, want >= 30 (2em font rendered)", g.BorderBoxHeight())
	}
}

// TestCurrentColorBorder verifies that `border: 1px solid currentcolor`
// (and the default border color, which is currentcolor per CSS) falls back
// to the element's color property at paint time.
func TestCurrentColorBorder(t *testing.T) {
	cs := resolveWithUA(t, `<html><body><div style="color: #123456; border: 2px solid currentcolor">x</div></body></html>`, "div")
	// Zero border color means "unset" → BorderColor falls back to Color.
	if got := cs.BorderColor("top"); got.R != 0x12 || got.G != 0x34 || got.B != 0x56 {
		t.Errorf("border top color = %+v, want currentcolor (#123456)", got)
	}
	if cs.BorderTopWidth.Value != 2 || cs.BorderTopStyle != "solid" {
		t.Errorf("border-top = %v %q, want 2px solid", cs.BorderTopWidth, cs.BorderTopStyle)
	}
}

// TestDefaultBorderColorIsCurrentColor verifies that a border with no explicit
// color (e.g. `border: 1px solid`) uses the element color, matching browsers.
func TestDefaultBorderColorIsCurrentColor(t *testing.T) {
	cs := resolveWithUA(t, `<html><body><div style="color: rgb(10,20,30); border: 1px solid">x</div></body></html>`, "div")
	if got := cs.BorderColor("top"); got.R != 10 || got.G != 20 || got.B != 30 {
		t.Errorf("default border color = %+v, want element color (10,20,30)", got)
	}
}

// TestListMarkerGeneration verifies that display:list-item boxes get a computed
// marker during layout: disc for <ul>, decimal ordinals for <ol>.
func TestListMarkerGeneration(t *testing.T) {
	doc, err := html.Parse(`<html><body><ul><li>one</li><li>two</li></ul><ol><li>a</li><li>b</li><li>c</li></ol></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	root := doc.DocumentElement()
	lb := layout.BuildLayoutTree(root, resolver)
	rootEb := lb.(*layout.ElementBox)
	_ = layout.Layout(rootEb, 800, 600)

	// Collect MarkerText for all li boxes in document order.
	var markers []string
	var walk func(b layout.Box)
	walk = func(b layout.Box) {
		if eb, ok := b.(*layout.ElementBox); ok {
			if eb.MarkerText != "" {
				markers = append(markers, eb.MarkerText)
			}
			for _, c := range eb.Children() {
				walk(c)
			}
		}
	}
	walk(rootEb)

	// ul → "•", "•"; ol → "1.", "2.", "3."
	want := []string{"•", "•", "1.", "2.", "3."}
	if len(markers) != len(want) {
		t.Fatalf("markers = %v, want %v", markers, want)
	}
	for i := range want {
		if markers[i] != want[i] {
			t.Errorf("marker[%d] = %q, want %q", i, markers[i], want[i])
		}
	}
}
