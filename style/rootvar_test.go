package style

import (
	"strings"
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
	htmlpkg "wb-ui/html"
)

// TestRootVarToBody verifies that CSS variables defined on :root
// are inherited by body and correctly resolved in var() references.
func TestRootVarToBody(t *testing.T) {
	src := `<html><head><style>
		:root {
			--bg-primary: #0d1117;
			--text-primary: #e6edf3;
			--font-size-base: 13px;
		}
		body {
			background-color: var(--bg-primary);
			color: var(--text-primary);
			font-size: var(--font-size-base);
		}
	</style></head><body></body></html>`

	doc, err := htmlpkg.Parse(src)
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewResolver()
	addInlineStyles(doc, resolver, t)

	// Resolve :root (html element)
	root := doc.DocumentElement()
	if root == nil {
		t.Fatal("no root element")
	}
	rootCS := resolver.ResolveElement(root)
	t.Logf(":root CustomProperties: %d entries", len(rootCS.CustomProperties))
	for k, v := range rootCS.CustomProperties {
		t.Logf("  %s = %d tokens", k, len(v))
		for _, tok := range v {
			t.Logf("    type=%d val=%q num=%f unit=%q", tok.Type, tok.Value, tok.Numeric, tok.Unit)
		}
	}

	// Resolve body
	body := doc.Body()
	if body == nil {
		t.Fatal("no body")
	}
	bodyCS := resolver.ResolveElement(body)

	t.Logf("body CustomProperties: %d entries", len(bodyCS.CustomProperties))
	for k, v := range bodyCS.CustomProperties {
		t.Logf("  %s = %d tokens", k, len(v))
	}
	t.Logf("body Properties: %d entries", len(bodyCS.Properties))
	for k, v := range bodyCS.Properties {
		t.Logf("  %s = %q", k, v)
	}
	t.Logf("body bg = #%02x%02x%02x a=%d", bodyCS.BackgroundColor.R, bodyCS.BackgroundColor.G, bodyCS.BackgroundColor.B, bodyCS.BackgroundColor.A)
	t.Logf("body fg = #%02x%02x%02x a=%d", bodyCS.Color.R, bodyCS.Color.G, bodyCS.Color.B, bodyCS.Color.A)
	t.Logf("body fs = %s", bodyCS.FontSize.String())

	if bodyCS.BackgroundColor.A == 0 {
		t.Error("body background-color is TRANSPARENT (var() not resolved)")
	} else if bodyCS.BackgroundColor.R == 0 && bodyCS.BackgroundColor.G == 0 && bodyCS.BackgroundColor.B == 0 {
		t.Error("body background-color is BLACK (parseColor failed)")
	} else {
		t.Logf("body bg = #%02x%02x%02x", bodyCS.BackgroundColor.R, bodyCS.BackgroundColor.G, bodyCS.BackgroundColor.B)
	}
	if bodyCS.Color.A == 0 {
		t.Error("body color is TRANSPARENT")
	}
}

func addInlineStyles(n dom.Node, resolver *Resolver, t *testing.T) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if el, ok := c.(*dom.Element); ok {
			tag := strings.ToLower(el.TagName())
			if tag == "style" {
				text := el.TextContent()
				sheet := css.NewCSSStyleSheet()
				css.NewParser(text).ParseStyleSheetInto(sheet)
				resolver.AddStyleSheet(sheet)
				t.Logf("Added style sheet: %d rules", len(sheet.Rules()))
			}
			addInlineStyles(c, resolver, t)
		}
	}
}
