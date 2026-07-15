content = """package style

import (
\t"testing"

\t"wb-ui/css"
\t"wb-ui/dom"
)

func newSheet(t *testing.T, input string) *css.CSSStyleSheet {
\tt.Helper()
\tsheet := css.NewCSSStyleSheet()
\tp := css.NewParser(input)
\tp.ParseStyleSheetInto(sheet)
\treturn sheet
}

func TestResolver_BasicStyleRule(t *testing.T) {
\tdoc := dom.NewDocument()
\tel := dom.NewElement(doc, "p")
\tr := NewResolver()
\tr.AddStyleSheet(newSheet(t, "p { color: red; }"))
\tcs := r.ResolveElement(el)
\tif cs.Color.R != 255 || cs.Color.G != 0 || cs.Color.B != 0 {
\t\tt.Fatalf("color=%v want red", cs.Color)
\t}
}

func TestResolver_SpecificityCascade(t *testing.T) {
\tdoc := dom.NewDocument()
\tel := dom.NewElement(doc, "p")
\tel.SetId("main")
\tr := NewResolver()
\tr.AddStyleSheet(newSheet(t, "p { color: red; } #main { color: blue; }"))
\tcs := r.ResolveElement(el)
\tif cs.Color.B != 255 || cs.Color.R != 0 {
\t\tt.Fatalf("color=%v want blue (id specificity wins)", cs.Color)
\t}
}

func TestResolver_SourceOrder(t *testing.T) {
\tdoc := dom.NewDocument()
\tel := dom.NewElement(doc, "p")
\tr := NewResolver()
\tr.AddStyleSheet(newSheet(t, "p { color: red; } p { color: blue; }"))
\tcs := r.ResolveElement(el)
\tif cs.Color.B != 255 {
\t\tt.Fatalf("color=%v want blue (later rule wins)", cs.Color)
\t}
}

func TestResolver_Inheritance(t *testing.T) {
\tdoc := dom.NewDocument()
\tparent := dom.NewElement(doc, "div")
\tchild := dom.NewElement(doc, "p")
\tif err := parent.AppendChild(child); err != nil {
\t\tt.Fatal(err)
\t}
\tr := NewResolver()
\tr.AddStyleSheet(newSheet(t, "div { color: green; }"))
\tcs := r.ResolveElement(child)
\tif cs.Color.G != 255 {
\t\tt.Fatalf("color=%v want green inherited from div", cs.Color)
\t}
}

func TestResolver_ImportantOverride(t *testing.T) {
\tdoc := dom.NewDocument()
\tel := dom.NewElement(doc, "p")
\tr := NewResolver()
\tr.AddStyleSheet(newSheet(t, "p { color: blue !important; } p { color: red; }"))
\tcs := r.ResolveElement(el)
\tif cs.Color.B != 255 {
\t\tt.Fatalf("color=%v want blue (important wins)", cs.Color)
\t}
}

func TestResolver_InlineStyle(t *testing.T) {
\tdoc := dom.NewDocument()
\tel := dom.NewElement(doc, "p")
\tel.SetAttribute("style", "color: blue;")
\tr := NewResolver()
\tr.AddStyleSheet(newSheet(t, "p { color: red; } #main { color: green; }"))
\tcs := r.ResolveElement(el)
\tif cs.Color.B != 255 {
\t\tt.Fatalf("color=%v want blue (inline wins)", cs.Color)
\t}
}

func TestResolver_NonMatchingSelector(t *testing.T) {
\tdoc := dom.NewDocument()
\tel := dom.NewElement(doc, "div")
\tr := NewResolver()
\tr.AddStyleSheet(newSheet(t, "p { color: red; }"))
\tcs := r.ResolveElement(el)
\tif cs.Color.R != 0 || cs.Color.G != 0 || cs.Color.B != 0 {
\t\tt.Fatalf("color=%v want default black (no match)", cs.Color)
\t}
}

func TestResolver_CustomPropertyVarResolution(t *testing.T) {
\tdoc := dom.NewDocument()
\tel := dom.NewElement(doc, "p")
\tr := NewResolver()
\tr.AddStyleSheet(newSheet(t, "p { --main: red; --accent: var(--main); }"))
\tcs := r.ResolveElement(el)
\tmain := cs.GetCustomProperty("--main")
\tif len(main) == 0 || main[0].Value != "red" {
\t\tt.Fatalf("--main=%v want red", main)
\t}
\taccent := cs.GetCustomProperty("--accent")
\tif len(accent) == 0 || accent[0].Value != "red" {
\t\tt.Fatalf("--accent=%v want resolved red", accent)
\t}
}

func TestResolver_DisplayProperty(t *testing.T) {
\tdoc := dom.NewDocument()
\tel := dom.NewElement(doc, "div")
\tr := NewResolver()
\tr.AddStyleSheet(newSheet(t, "div { display: flex; }"))
\tcs := r.ResolveElement(el)
\tif cs.Display != DisplayFlex {
\t\tt.Fatalf("display=%v want flex", cs.Display)
\t}
}

func TestResolver_ResolveDocument(t *testing.T) {
\tdoc := dom.NewDocument()
\troot := dom.NewElement(doc, "html")
\tbody := dom.NewElement(doc, "body")
\tp := dom.NewElement(doc, "p")
\tif err := root.AppendChild(body); err != nil {
\t\tt.Fatal(err)
\t}
\tif err := body.AppendChild(p); err != nil {
\t\tt.Fatal(err)
\t}
\tif err := doc.AppendChild(root); err != nil {
\t\tt.Fatal(err)
\t}
\tr := NewResolver()
\tr.AddStyleSheet(newSheet(t, "body { color: red; }"))
\tout := r.ResolveDocument(doc)
\tif len(out) == 0 {
\t\tt.Fatalf("ResolveDocument returned empty map")
\t}
\tif _, ok := out[body]; !ok {
\t\tt.Fatalf("body not in resolved map")
\t}
\tif out[body].Color.R != 255 {
\t\tt.Fatalf("body color=%v want red", out[body].Color)
\t}
}
"""
with open(r"f:\syproject\wb-ui\style\resolver_test.go", "w", encoding="utf-8", newline="\n") as f:
    f.write(content)
print("written", len(content), "bytes")
