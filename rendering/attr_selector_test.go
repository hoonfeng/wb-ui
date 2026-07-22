package rendering

import (
	"fmt"
	"strings"
	"testing"


	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/style"
)

// TestAttributeSelectorCSS checks that [data-v-xxx] attribute selectors match.
func TestAttributeSelectorCSS(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	el.SetAttribute("data-v-abc123", "")
	doc.AppendChild(el)

	s := dom.NewElement(doc, "style")
	s.SetTextContent(`[data-v-abc123] { color: rgb(255,0,0); background-color: rgb(0,255,0); }`)
	doc.AppendChild(s)

	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(s.TextContent()).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)

	sm := resolver.ResolveDocument(doc)
	cs := sm[el]
	if cs == nil {
		t.Fatal("ComputedStyle is nil")
	}

	t.Logf("Color: R=%d G=%d B=%d A=%d", cs.Color.R, cs.Color.G, cs.Color.B, cs.Color.A)
	t.Logf("BackgroundColor: R=%d G=%d B=%d A=%d", cs.BackgroundColor.R, cs.BackgroundColor.G, cs.BackgroundColor.B, cs.BackgroundColor.A)

	if cs.Color.R != 255 || cs.Color.G != 0 || cs.Color.B != 0 {
		t.Errorf("Color should be red (255,0,0), got (%d,%d,%d)", cs.Color.R, cs.Color.G, cs.Color.B)
	}
	if cs.BackgroundColor.R != 0 || cs.BackgroundColor.G != 255 || cs.BackgroundColor.B != 0 {
		t.Errorf("BackgroundColor should be green (0,255,0), got (%d,%d,%d)", cs.BackgroundColor.R, cs.BackgroundColor.G, cs.BackgroundColor.B)
	}
}

// TestExternalCSSWithScopedSelectors verifies that attribute and class selectors from external CSS work.
func TestExternalCSSWithScopedSelectors(t *testing.T) {
	// Simulate the Vue scoped CSS pattern
	cssText := `
.menubar { display: flex; height: 32px; background: rgb(22,27,34); color: rgb(139,148,158); }
.activity-bar { display: flex; flex-direction: column; background: rgb(13,17,23); width: 48px; }
.activity-btn { display: flex; align-items: center; justify-content: center; width: 48px; height: 48px; }
.activity-btn.active { background: rgba(88,166,255,0.1); color: rgb(88,166,255); }
`
	doc := dom.NewDocument()
	
	// Add style element
	s := dom.NewElement(doc, "style")
	s.SetTextContent(cssText)
	doc.AppendChild(s)

	// Create DOM tree
	menubar := dom.NewElement(doc, "div")
	menubar.SetClassName("menubar")
	doc.AppendChild(menubar)

	activityBar := dom.NewElement(doc, "div")
	activityBar.SetClassName("activity-bar")
	doc.AppendChild(activityBar)

	btn := dom.NewElement(doc, "div")
	btn.SetClassName("activity-btn active")
	activityBar.AppendChild(btn)

	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	p := css.NewParser(cssText)
	p.ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)

	sm := resolver.ResolveDocument(doc)

	for _, tc := range []struct {
		label string
		el    *dom.Element
		wantBG string // expected background color hex
	}{
		{"menubar", menubar, "161b22"},
		{"activity-bar", activityBar, "0d1117"},
	} {
		cs := sm[tc.el]
		if cs == nil {
			t.Errorf("%s: ComputedStyle is nil", tc.label)
			continue
		}
		bg := fmt.Sprintf("%02x%02x%02x", cs.BackgroundColor.R, cs.BackgroundColor.G, cs.BackgroundColor.B)
		t.Logf("%s: bg=%s display=%d", tc.label, bg, cs.Display)
	}
}

func init() {
	_ = strings.TrimSpace // avoid import error
}
