package style

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
)

// TestRuleIndex_MatchesFullScan is the correctness cornerstone for the rule
// index: for a battery of selectors + element class/id/tag combos, the indexed
// candidate path must produce the EXACT same matched rules as the full scan.
func TestRuleIndex_MatchesFullScan(t *testing.T) {
	cssTexts := []string{
		`.a { color: red; }
		 .b { color: blue; }
		 .a:hover { color: green; }
		 .a .b { color: orange; }
		 .a > .b { color: purple; }
		 div { color: black; }
		 #main { color: teal; }
		 div.a { color: maroon; }
		 .x:not(.y) { color: navy; }
		 :hover { color: silver; }
		 [data-x] { color: lime; }
		 .a, .b { color: gold; }
		 .p .q .r { color: pink; }
		 .a:hover .b { color: brown; }
		 * { color: gray; }
		 ::selection { color: white; }
		 .clearfix::after { content: ""; }
		 .nested-outer { color: #111; }
		 .media-rule { color: #222; }
		 span.foo { color: #333; }
		 .scoped-attr[data-v-abc] { color: #444; }
		 .attr-only[data-v-abc] .deep-child { color: #555; }`,
		// second sheet (nested + media)
		`.outer { color: #aaa; }
		 .outer .inner { color: #bbb; }
		 @media (min-width: 10px) { .mq { color: #ccc; } }
		 .q:hover > .z { color: #ddd; }`,
	}

	mkEl := func(tag string, id string, classes ...string) *dom.Element {
		doc := dom.NewDocument()
		el := doc.CreateElement(tag)
		if id != "" {
			el.SetAttribute("id", id)
		}
		if len(classes) > 0 {
			el.SetAttribute("class", strings.Join(classes, " "))
		}
		return el
	}

	// build one resolver with both sheets
	build := func() *Resolver {
		r := NewResolver()
		for _, txt := range cssTexts {
			p := css.NewParser(txt)
			sheet := css.NewCSSStyleSheet()
			p.ParseStyleSheetInto(sheet)
			r.AddStyleSheet(sheet)
		}
		return r
	}
	type combo struct {
		desc string
		el   *dom.Element
	}
	combos := []combo{
		{"a", mkEl("div", "", "a")},
		{"b", mkEl("div", "", "b")},
		{"a-b", mkEl("div", "", "a", "b")},
		{"plain div", mkEl("div", "")},
		{"span", mkEl("span", "")},
		{"span foo", mkEl("span", "", "foo")},
		{"id main", mkEl("section", "main", "a")},
		{"x-not-y", mkEl("p", "", "x")},
		{"x-y", mkEl("p", "", "x", "y")},
		{"data-x", mkEl("div", "", "a")}, // no data-x attr; still must match nothing extra
		{"p q r", mkEl("p", "", "p", "q", "r")},
		{"nested-outer", mkEl("div", "", "nested-outer")},
		{"media-rule", mkEl("div", "", "media-rule", "mq")},
		{"outer", mkEl("div", "", "outer")},
		{"scoped-attr", mkEl("div", "", "scoped-attr")},
		{"attr-only", mkEl("div", "", "attr-only")},
		{"deep-child", mkEl("div", "", "deep-child")},
		{"attr-missing", mkEl("div", "", "deep-child")},
	}

	fullScan := func(r *Resolver, el *dom.Element) []string {
		var collected []collectedDecl
		for _, sheet := range r.sheets {
			r.collectDeclarations(sheet.Rules(), sheet.Origin(), el, &collected, 0, sheetScopeDepth(sheet), sheetBaseURL(sheet))
		}
		sort.SliceStable(collected, func(i, j int) bool {
			return collected[i].sourceOrder < collected[j].sourceOrder
		})
		seen := map[string]bool{}
		var out []string
		for _, cd := range collected {
			k := cd.selector + "|" + cd.decl.Name + "=" + cd.decl.ValueString() +
				fmt.Sprintf("|spec=%d.%d.%d|so=%d", cd.specificity.A, cd.specificity.B, cd.specificity.C, cd.sourceOrder)
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
		return out
	}

	indexed := func(r *Resolver, el *dom.Element) []string {
		var collected []collectedDecl
		for _, sheet := range r.sheets {
			r.collectSheetDeclarations(sheet, el, &collected)
		}
		sort.SliceStable(collected, func(i, j int) bool {
			return collected[i].sourceOrder < collected[j].sourceOrder
		})
		seen := map[string]bool{}
		var out []string
		for _, cd := range collected {
			k := cd.selector + "|" + cd.decl.Name + "=" + cd.decl.ValueString() +
				fmt.Sprintf("|spec=%d.%d.%d|so=%d", cd.specificity.A, cd.specificity.B, cd.specificity.C, cd.sourceOrder)
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
		return out
	}

	for _, c := range combos {
		r1 := build()
		r2 := build()
		// force hover states equal on both resolvers
		if strings.Contains(c.desc, "hover") {
			c.el.SetHovered(true)
		}
		full := fullScan(r1, c.el)
		idx := indexed(r2, c.el)
		if len(full) != len(idx) {
			t.Errorf("[%s] rule count mismatch: full=%d idx=%d\nfull: %v\nidx:  %v", c.desc, len(full), len(idx), full, idx)
			continue
		}
		for i := range full {
			if full[i] != idx[i] {
				t.Errorf("[%s] rule %d mismatch:\n full: %v\n idx:  %v", c.desc, i, full[i], idx[i])
			}
		}
	}
}
