package style

import (
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
)

func newSheet(t *testing.T, input string) *css.CSSStyleSheet {
	t.Helper()
	sheet := css.NewCSSStyleSheet()
	p := css.NewParser(input)
	p.ParseStyleSheetInto(sheet)
	return sheet
}

func TestResolver_BasicStyleRule(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { color: red; }"))
	cs := r.ResolveElement(el)
	if cs.Color.R != 255 || cs.Color.G != 0 || cs.Color.B != 0 {
		t.Fatalf("color=%v want red", cs.Color)
	}
}

func TestResolver_SpecificityCascade(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	el.SetId("main")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { color: red; } #main { color: blue; }"))
	cs := r.ResolveElement(el)
	if cs.Color.B != 255 || cs.Color.R != 0 {
		t.Fatalf("color=%v want blue (id specificity wins)", cs.Color)
	}
}

func TestResolver_SourceOrder(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { color: red; } p { color: blue; }"))
	cs := r.ResolveElement(el)
	if cs.Color.B != 255 {
		t.Fatalf("color=%v want blue (later rule wins)", cs.Color)
	}
}

func TestResolver_Inheritance(t *testing.T) {
	doc := dom.NewDocument()
	parent := dom.NewElement(doc, "div")
	child := dom.NewElement(doc, "p")
	if err := parent.AppendChild(child); err != nil {
		t.Fatal(err)
	}
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "div { color: green; }"))
	cs := r.ResolveElement(child)
	if cs.Color.G != 255 {
		t.Fatalf("color=%v want green inherited from div", cs.Color)
	}
}

func TestResolver_ImportantOverride(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { color: blue !important; } p { color: red; }"))
	cs := r.ResolveElement(el)
	if cs.Color.B != 255 {
		t.Fatalf("color=%v want blue (important wins)", cs.Color)
	}
}

func TestResolver_InlineStyle(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	el.SetAttribute("style", "color: blue;")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { color: red; } #main { color: green; }"))
	cs := r.ResolveElement(el)
	if cs.Color.B != 255 {
		t.Fatalf("color=%v want blue (inline wins)", cs.Color)
	}
}

func TestResolver_NonMatchingSelector(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { color: red; }"))
	cs := r.ResolveElement(el)
	if cs.Color.R != 0 || cs.Color.G != 0 || cs.Color.B != 0 {
		t.Fatalf("color=%v want default black (no match)", cs.Color)
	}
}

func TestResolver_CustomPropertyVarResolution(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { --main: red; --accent: var(--main); }"))
	cs := r.ResolveElement(el)
	main := cs.GetCustomProperty("--main")
	if len(main) == 0 || main[0].Value != "red" {
		t.Fatalf("--main=%v want red", main)
	}
	accent := cs.GetCustomProperty("--accent")
	if len(accent) == 0 || accent[0].Value != "red" {
		t.Fatalf("--accent=%v want resolved red", accent)
	}
}

func TestResolver_DisplayProperty(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "div { display: flex; }"))
	cs := r.ResolveElement(el)
	if cs.Display != DisplayFlex {
		t.Fatalf("display=%v want flex", cs.Display)
	}
}

func TestResolver_BorderShorthand(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "div { border: 1px solid #e5e7eb; }"))
	cs := r.ResolveElement(el)
	if cs.BorderTopStyle != "solid" {
		t.Fatalf("BorderTopStyle=%q want solid", cs.BorderTopStyle)
	}
	if cs.BorderTopWidth.Value != 1 || cs.BorderTopWidth.Unit != "px" {
		t.Fatalf("BorderTopWidth=%+v want 1px", cs.BorderTopWidth)
	}
	if cs.BorderBottomStyle != "solid" {
		t.Fatalf("BorderBottomStyle=%q want solid", cs.BorderBottomStyle)
	}
}

func TestResolver_BorderBottomShorthand(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "div { border-bottom: 2px solid #e5e7eb; }"))
	cs := r.ResolveElement(el)
	if cs.BorderBottomStyle != "solid" {
		t.Fatalf("BorderBottomStyle=%q want solid", cs.BorderBottomStyle)
	}
	if cs.BorderBottomWidth.Value != 2 || cs.BorderBottomWidth.Unit != "px" {
		t.Fatalf("BorderBottomWidth=%+v want 2px", cs.BorderBottomWidth)
	}
}

func TestResolver_ResolveDocument(t *testing.T) {
	doc := dom.NewDocument()
	root := dom.NewElement(doc, "html")
	body := dom.NewElement(doc, "body")
	p := dom.NewElement(doc, "p")
	if err := root.AppendChild(body); err != nil {
		t.Fatal(err)
	}
	if err := body.AppendChild(p); err != nil {
		t.Fatal(err)
	}
	if err := doc.AppendChild(root); err != nil {
		t.Fatal(err)
	}
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "body { color: red; }"))
	out := r.ResolveDocument(doc)
	if len(out) == 0 {
		t.Fatalf("ResolveDocument returned empty map")
	}
	if _, ok := out[body]; !ok {
		t.Fatalf("body not in resolved map")
	}
	if out[body].Color.R != 255 {
		t.Fatalf("body color=%v want red", out[body].Color)
	}
}
