package css

import (
	"strings"
	"testing"

	"wb-ui/dom"
)

// buildSel parses a single complex selector from CSS text.
func buildSel(t *testing.T, s string) ComplexSelector {
	t.Helper()
	// Use the public parse path: "x { }" as a rule then extract the selector.
	sheet := NewCSSStyleSheet()
	pp := NewParser(s + " { color: red; }")
	pp.ParseStyleSheetInto(sheet)
	rules := sheet.Rules()
	for _, r := range rules {
		if sr, ok := r.(*StyleRule); ok && sr.Selectors != nil && len(sr.Selectors.Selectors) > 0 {
			return sr.Selectors.Selectors[0]
		}
	}
	t.Fatalf("no selector parsed from %q", s)
	return ComplexSelector{}
}

func TestDescendantSelectorClassTag(t *testing.T) {
	// <div class="activity-bar"><button></button></div>
	doc := dom.NewDocument()
	div := doc.CreateElement("div")
	div.SetAttribute("class", "activity-bar")
	btn := doc.CreateElement("button")
	div.AppendChild(btn)
	doc.AppendChild(div)

	sel := buildSel(t, ".activity-bar button")
	checker := NewSelectorChecker()
	if !checker.Match(sel, btn) {
		t.Fatalf(".activity-bar button should match <button> inside .activity-bar")
	}
}

func TestDescendantSelectorClassClass(t *testing.T) {
	doc := dom.NewDocument()
	div := doc.CreateElement("div")
	div.SetAttribute("class", "sidebar")
	item := doc.CreateElement("div")
	item.SetAttribute("class", "s-item")
	div.AppendChild(item)
	doc.AppendChild(div)

	sel := buildSel(t, ".sidebar .s-item")
	checker := NewSelectorChecker()
	if !checker.Match(sel, item) {
		t.Fatalf(".sidebar .s-item should match nested .s-item")
	}
}

func TestTagSelectorMatches(t *testing.T) {
	doc := dom.NewDocument()
	btn := doc.CreateElement("button")
	doc.AppendChild(btn)
	sel := buildSel(t, "button")
	checker := NewSelectorChecker()
	if !checker.Match(sel, btn) {
		t.Fatalf("button tag selector should match <button>")
	}
}

func TestDescendantChildSelector(t *testing.T) {
	doc := dom.NewDocument()
	div := doc.CreateElement("div")
	div.SetAttribute("class", "a")
	child := doc.CreateElement("span")
	div.AppendChild(child)
	doc.AppendChild(div)

	sel := buildSel(t, ".a > span")
	checker := NewSelectorChecker()
	if !checker.Match(sel, child) {
		t.Fatalf(".a > span should match direct child")
	}
}

func TestDescendantSelectorDebug(t *testing.T) {
	doc := dom.NewDocument()
	div := doc.CreateElement("div")
	div.SetAttribute("class", "activity-bar")
	btn := doc.CreateElement("button")
	div.AppendChild(btn)
	doc.AppendChild(div)

	sel := buildSel(t, ".activity-bar button")
	t.Logf("compounds=%d", len(sel.Compounds))
	for i, c := range sel.Compounds {
		var ss []string
		for _, s := range c.Selectors {
			ss = append(ss, s.Value)
		}
		t.Logf("  compound[%d] relation=%d selectors=%v", i, c.Relation, strings.Join(ss, ","))
	}
}
