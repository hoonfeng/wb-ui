package style

import (
	"testing"
	"wb-ui/css"
	"wb-ui/dom"
)

// TestResolveRootVarsOnBody verifies that :root CSS variables resolve
// correctly on body element when body uses var() references.
func TestResolveRootVarsOnBody(t *testing.T) {
	cssText := `
:root {
  --bg-primary: #0d1117;
  --text-primary: #e6edf3;
  --accent: #58a6ff;
  --shadow-md: 0 4px 16px rgba(0,0,0,0.35);
}
body {
  color: var(--text-primary);
  background-color: var(--bg-primary);
  font-family: var(--font-ui);
  font-size: var(--font-size-base);
}
.box {
  box-shadow: var(--shadow-md);
  border: 1px solid var(--accent);
}
`

	doc := dom.NewDocument()
	html := dom.NewElement(doc, "html")
	html.SetClassName("theme-dark")
	doc.AppendChild(html)
	body := dom.NewElement(doc, "body")
	body.SetClassName("theme-dark")
	html.AppendChild(body)
	box := dom.NewElement(doc, "div")
	box.SetClassName("box")
	body.AppendChild(box)

	r := NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(cssText).ParseStyleSheetInto(sheet)
	r.AddStyleSheet(sheet)

	cs := r.ResolveElement(body)
	checkColor(t, cs.Color, 0xe6, 0xed, 0xf3, 0xff, "body text color")
	checkColor(t, cs.BackgroundColor, 0x0d, 0x11, 0x17, 0xff, "body background")

	bcs := r.ResolveElement(box)
	checkColor(t, bcs.BorderTopColor, 0x58, 0xa6, 0xff, 0xff, "box border color (accent)")
	if bcs.BoxShadow == "" {
		t.Error("box should have box-shadow")
	}
	t.Logf("box shadow: %q", bcs.BoxShadow)

	// Also verify body has inherited vars
	if bgVar, ok := cs.CustomProperties["--bg-primary"]; !ok || len(bgVar) == 0 {
		t.Error("body should inherit --bg-primary from :root")
	}
}

// TestPseudoElementDoesNotLeak verifies that ::selection rules don't
// contaminate the base element's computed style.
func TestPseudoElementDoesNotLeak(t *testing.T) {
	cssText := `
p { color: #00ff00; }
::selection { color: #ffffff; background: #58a6ff; }
`

	doc := dom.NewDocument()
	p := dom.NewElement(doc, "p")
	doc.AppendChild(p)
	txt := doc.CreateTextNode("hello world")
	p.AppendChild(txt)

	r := NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(cssText).ParseStyleSheetInto(sheet)
	r.AddStyleSheet(sheet)

	cs := r.ResolveElement(p)
	// p's color should be #00ff00, NOT leaked ::selection's #ffffff
	checkColor(t, cs.Color, 0x00, 0xff, 0x00, 0xff, "p text color (must not leak ::selection)")
}

// TestPseudoElementComplexDoesNotLeak tests that ::-webkit-scrollbar-thumb
// and other pseudo-element selectors don't contaminate.
func TestPseudoElementComplexDoesNotLeak(t *testing.T) {
	cssText := `
div { color: #111111; background-color: #eeeeee; }
::-webkit-scrollbar-thumb { background: #30363d; }
::-webkit-scrollbar-thumb:hover { background: #484f58; }
`

	doc := dom.NewDocument()
	div := dom.NewElement(doc, "div")
	doc.AppendChild(div)

	r := NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(cssText).ParseStyleSheetInto(sheet)
	r.AddStyleSheet(sheet)

	cs := r.ResolveElement(div)
	// div's background should be #eeeeee, NOT leaked ::-webkit-scrollbar-thumb's #30363d
	checkColor(t, cs.BackgroundColor, 0xee, 0xee, 0xee, 0xff, "div background (must not leak scrollbar colors)")
	checkColor(t, cs.Color, 0x11, 0x11, 0x11, 0xff, "div text color")
}

// TestVarResolutionInBoxShadow verifies that var() references inside
// box-shadow properties are properly expanded.
func TestVarResolutionInBoxShadow(t *testing.T) {
	cssText := `
:root { --shadow-md: 0 4px 16px rgba(0,0,0,0.35); }
.card { box-shadow: var(--shadow-md); }
`

	doc := dom.NewDocument()
	card := dom.NewElement(doc, "div")
	card.SetClassName("card")
	doc.AppendChild(card)

	r := NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(cssText).ParseStyleSheetInto(sheet)
	r.AddStyleSheet(sheet)

	cs := r.ResolveElement(card)
	if cs.BoxShadow == "" {
		t.Error("card should have box-shadow from var(--shadow-md)")
	}
	// Should NOT contain "var(" in the resolved value
	if contains(cs.BoxShadow, "var(") {
		t.Errorf("box-shadow should be resolved, got: %q (contains unresolved var())", cs.BoxShadow)
	}
	t.Logf("resolved box-shadow: %q", cs.BoxShadow)
}

// TestVarResolutionInMargin verifies var() resolution in margin properties.
func TestVarResolutionInMargin(t *testing.T) {
	cssText := `
:root { --spacing-lg: 20px; --spacing-sm: 10px; }
.hero { margin-top: var(--spacing-lg); margin-left: var(--spacing-sm); }
`

	doc := dom.NewDocument()
	hero := dom.NewElement(doc, "div")
	hero.SetClassName("hero")
	doc.AppendChild(hero)

	r := NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(cssText).ParseStyleSheetInto(sheet)
	r.AddStyleSheet(sheet)

	cs := r.ResolveElement(hero)
	if cs.MarginTop.String() != "20px" {
		t.Errorf("margin-top should be 20px (from var), got %v", cs.MarginTop)
	}
	if cs.MarginLeft.String() != "10px" {
		t.Errorf("margin-left should be 10px (from var), got %v", cs.MarginLeft)
	}
}

// TestVarResolutionInBorderRadius verifies var() in border-radius.
func TestVarResolutionInBorderRadius(t *testing.T) {
	cssText := `
:root { --radius-lg: 8px; --radius-sm: 4px; }
.btn { border-radius: var(--radius-lg); }
`

	doc := dom.NewDocument()
	btn := dom.NewElement(doc, "button")
	btn.SetClassName("btn")
	doc.AppendChild(btn)

	r := NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(cssText).ParseStyleSheetInto(sheet)
	r.AddStyleSheet(sheet)

	cs := r.ResolveElement(btn)
	if cs.BorderRadius.String() != "8px" {
		t.Errorf("border-radius should be 8px (from var), got %v", cs.BorderRadius)
	}
}

// TestBodyBackgroundFromIndexHTML simulates the exact conditions from
// the desktop app's index.html — :root defines variables, body uses them.
func TestBodyBackgroundFromIndexHTML(t *testing.T) {
	// This is the exact CSS structure from index.html (simplified)
	cssText := `
:root, .theme-dark {
  --bg-primary: #0d1117;
  --text-primary: #e6edf3;
  --accent: #58a6ff;
  --shadow-md: 0 4px 16px rgba(0,0,0,0.35);
  --font-ui: 'Inter', system-ui, -apple-system, sans-serif;
  --font-size-base: 13px;
}
body {
  font-family: var(--font-ui);
  font-size: var(--font-size-base);
  color: var(--text-primary);
  background-color: var(--bg-primary);
}
`

	doc := dom.NewDocument()
	html := dom.NewElement(doc, "html")
	html.SetClassName("theme-dark")
	doc.AppendChild(html)
	body := dom.NewElement(doc, "body")
	body.SetClassName("theme-dark")
	html.AppendChild(body)

	r := NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(cssText).ParseStyleSheetInto(sheet)
	r.AddStyleSheet(sheet)

	cs := r.ResolveElement(body)

	// THESE ARE THE CRITICAL ASSERTIONS:
	// body background must be dark (#0d1117), NOT black/transparent
	checkColor(t, cs.BackgroundColor, 0x0d, 0x11, 0x17, 0xff, "body background (dark theme)")
	// body text must be light (#e6edf3), NOT black
	checkColor(t, cs.Color, 0xe6, 0xed, 0xf3, 0xff, "body text (light)")
	// font must be set
	if cs.FontFamily == "" || cs.FontFamily == "serif" {
		t.Errorf("body font family should be Inter, got %q", cs.FontFamily)
	}
}

func checkColor(t *testing.T, got Color, r, g, b, a uint8, what string) {
	t.Helper()
	if got.R != r || got.G != g || got.B != b || got.A != a {
		t.Errorf("%s: expected #%02x%02x%02x%02x, got #%02x%02x%02x%02x",
			what, r, g, b, a, got.R, got.G, got.B, got.A)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
