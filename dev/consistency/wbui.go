// wb-ui reference collector: renders the same HTML through the wb-ui full
// pipeline (html.Parse → style.Resolver → layout → RenderTreeBuilder →
// RenderView.Layout) and extracts per-element snapshots mirroring the fields
// collected by the Edge collector (see edge.go parseGeo).

package main

import (
	"fmt"
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

// wbuiCollect renders c.HTML through the wb-ui pipeline and returns the same
// ElementSnapshot shape as edgeCollect.
func wbuiCollect(c TestCase) ([]ElementSnapshot, error) {
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}

	// Strip the collector script before parsing (wb-ui executes no JS here).
	clean := stripCollector(c.HTML)
	doc, err := html.Parse(clean)
	if err != nil {
		return nil, fmt.Errorf("html parse: %w", err)
	}

	resolver := style.NewResolver()
	resolver.AddStyleSheet(html5.NewUAStyleSheet())
	mergeCSSFromDOM(doc, resolver)

	// Use the actual browser viewport when available so geometry matches.
	vw, vh := c.ViewportW, c.ViewportH
	if edgeViewportW > 0 {
		vw, vh = edgeViewportW, edgeViewportH
	}

	builder := rendering.NewRenderTreeBuilder(resolver)
	rv := builder.Build(doc)
	if rv == nil {
		return nil, fmt.Errorf("render tree build failed")
	}
	rv.SetViewportSize(float64(vw), float64(vh))
	state := layout.NewLayoutState(float64(vw), float64(vh))
	rv.Layout(state)

	var snaps []ElementSnapshot
	collectRenderTree(rv, &snaps)
	return snaps, nil
}

// stripCollector removes the injected <script> collector block.
func stripCollector(htmlText string) string {
	idx := strings.Index(htmlText, "<script>")
	if idx < 0 {
		return htmlText
	}
	end := strings.Index(htmlText[idx:], "</script>")
	if end < 0 {
		return htmlText[:idx]
	}
	return htmlText[:idx] + htmlText[idx+end+len("</script>"):]
}

// mergeCSSFromDOM extracts <style> blocks and inline style attributes and
// feeds them to the resolver.
func mergeCSSFromDOM(doc *dom.Document, resolver *style.Resolver) {
	var collect func(n dom.Node, styles *[]string)
	collect = func(n dom.Node, styles *[]string) {
		if el, ok := n.(*dom.Element); ok && el.LocalName() == "style" {
			if c := el.FirstChild(); c != nil {
				if t, ok := c.(*dom.Text); ok {
					*styles = append(*styles, t.Data())
				}
			}
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			collect(c, styles)
		}
	}
	var styles []string
	collect(doc, &styles)
	for _, s := range styles {
		resolver.AddStyleSheet(parseStyleSheet(s))
	}
}

func parseStyleSheet(text string) *css.CSSStyleSheet {
	sheet := css.NewCSSStyleSheet()
	sheet.SetOrigin(css.OriginAuthor)
	p := css.NewParser(text)
	p.SetOrigin(css.OriginAuthor)
	for _, r := range p.ParseStyleSheet() {
		sheet.AppendRule(r)
	}
	return sheet
}

// collectRenderTree walks the render tree, mapping each element-bearing
// RenderObject to a snapshot (geometry + computed style). Geometry comes from
// rendering.BoxGeometry which handles both box and non-box objects.
func collectRenderTree(ro rendering.RenderObject, out *[]ElementSnapshot) {
	if ro == nil {
		return
	}
	if el := ro.Node(); el != nil {
		if e, ok := el.(*dom.Element); ok {
			x, y, w, h, hasGeo := rendering.BoxGeometry(ro)
			st := ro.Style()
			s := ElementSnapshot{
				Tag:   e.LocalName(),
				ID:    e.GetAttribute("id"),
				Class: e.GetAttribute("class"),
			}
			if hasGeo {
				s.X, s.Y, s.W, s.H = x, y, w, h
			}
			if st != nil {
				s.Display = displayName(st.Display)
				s.Color = st.Color.String()
				s.BG = st.BackgroundColor.String()
				s.FontSz = st.FontSize.String()
			}			// Input value / checked state.
			switch e.LocalName() {
			case "input":
				s.Value = e.GetAttribute("value")
				if e.GetAttribute("type") == "checkbox" || e.GetAttribute("type") == "radio" {
					if e.GetAttribute("checked") != "" {
						s.Checked = "1"
					}
				}
			case "select":
				s.Value = firstOptionText(e)
			case "textarea":
				s.Value = e.TextContent()
			}
	// Only emit snapshots for elements with geometry or styles;
	// skip script/style/none-display internals, and skip the
	// html/body roots to align with Edge's `body *` traversal.
	// <option> elements never render (the <select> draws its own
	// widget) — Edge reports them at 0x0; skip them for parity.
	if e.LocalName() != "html" && e.LocalName() != "body" && e.LocalName() != "style" &&
		e.LocalName() != "script" && e.LocalName() != "option" {
			// Aggregate descendant text for content comparison (truncate to
			// match Edge's collector which trims then slices to 20 chars).
			// Container elements (form/ul/ol/select) whose children are not
			// all in the render tree (options, hidden inputs) use the DOM
			// textContent, which matches the browser's textContent exactly.
			text := ""
			if e.LocalName() == "form" || e.LocalName() == "ul" || e.LocalName() == "ol" ||
				e.LocalName() == "select" || e.LocalName() == "textarea" || e.LocalName() == "div" {
				text = e.TextContent()
			}
			if text == "" {
				text = aggregateText(ro)
			}
			text = strings.TrimSpace(text)
			if len(text) > 20 {
				text = text[:20]
			}
			s.TextContent = text
		// Inline elements (em/strong/span) have no box geometry; use the
		// first descendant text segment's position as an approximation.
		if !hasGeo {
			x, y, w, h, ok := inlineGeo(ro)
			if ok {
				s.X, s.Y, s.W, s.H = x, y, w, h
				hasGeo = true
			}
		}
		if hasGeo || st != nil {
			*out = append(*out, s)
		}
	}
		}
	}
	// Recurse children.
	for rc := ro.FirstChild(); rc != nil; rc = rc.NextSibling() {
		collectRenderTree(rc, out)
	}
}
func firstOptionText(sel *dom.Element) string {
	for c := sel.FirstChild(); c != nil; c = c.NextSibling() {
		if e, ok := c.(*dom.Element); ok && e.LocalName() == "option" {
			return e.TextContent()
		}
	}
	return ""
}

// aggregateText concatenates the text of all RenderText descendants, matching
// the DOM textContent semantics (raw concatenation, no injected separators).
func aggregateText(ro rendering.RenderObject) string {
	var sb strings.Builder
	var walk func(o rendering.RenderObject)
	walk = func(o rendering.RenderObject) {
		if o == nil {
			return
		}
		if o.IsRenderText() {
			if t, ok := o.(*rendering.RenderText); ok {
				sb.WriteString(t.Text())
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(ro)
	return sb.String()
}

// inlineGeo returns an approximate geometry for inline elements without a
// box, using the first descendant text segment's position.
func inlineGeo(ro rendering.RenderObject) (x, y, w, h float64, ok bool) {
	var found *rendering.RenderText
	var find func(o rendering.RenderObject)
	find = func(o rendering.RenderObject) {
		if found != nil || o == nil {
			return
		}
		if o.IsRenderText() {
			found = o.(*rendering.RenderText)
			return
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			find(c)
		}
	}
	find(ro)
	if found == nil {
		return 0, 0, 0, 0, false
	}
	segs := found.Segments()
	if len(segs) == 0 {
		// No segments: fall back to the text object's own geometry.
		return rendering.BoxGeometry(found)
	}
	s := segs[0]
	return s.X, s.Y, s.Width, s.Height, true
}

// displayName converts a style.DisplayType to its CSS string.
func displayName(d style.DisplayType) string {
	switch d {
	case style.DisplayBlock:
		return "block"
	case style.DisplayInline:
		return "inline"
	case style.DisplayInlineBlock:
		return "inline-block"
	case style.DisplayListItem:
		return "list-item"
	case style.DisplayNone:
		return "none"
	case style.DisplayFlex:
		return "flex"
	case style.DisplayInlineFlex:
		return "inline-flex"
	case style.DisplayGrid:
		return "grid"
	case style.DisplayTable:
		return "table"
	case style.DisplayTableCell:
		return "table-cell"
	case style.DisplayTableRow:
		return "table-row"
	}
	return "inline"
}
