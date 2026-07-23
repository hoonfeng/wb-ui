package rendering

import (
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

func TestBodySync(t *testing.T) {
	htmlSrc := "<!DOCTYPE html>\n<html><head><style>\n" +
		"body { margin: 0; }\n" +
		"#box { background-color: #16213e; width: 200px; height: 100px; }\n" +
		"</style></head><body>\n" +
		"<div id=\"box\"></div>\n" +
		"</body></html>"

	doc, err := html.Parse(htmlSrc)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	resolver := style.NewResolver()
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		if el, ok := c.(*dom.Element); ok {
			if el.TagName() == "HTML" {
				for cc := el.FirstChild(); cc != nil; cc = cc.NextSibling() {
					if se, ok := cc.(*dom.Element); ok && se.TagName() == "HEAD" {
						for s := se.FirstChild(); s != nil; s = s.NextSibling() {
							if ste, ok := s.(*dom.Element); ok && ste.TagName() == "STYLE" {
								sheet := css.NewCSSStyleSheet()
								css.NewParser(ste.TextContent()).ParseStyleSheetInto(sheet)
								resolver.AddStyleSheet(sheet)
							}
						}
					}
				}
			}
		}
	}

	// Check layout tree structure
	layoutRoot := layout.BuildLayoutTree(doc.DocumentElement(), resolver)
	rootEb, ok := layoutRoot.(*layout.ElementBox)
	if ok {
		t.Logf("layoutRoot children=%d", len(rootEb.Children()))
		for i, ch := range rootEb.Children() {
			tag := "?"
			if eb, ok := ch.(*layout.ElementBox); ok {
				if el := eb.Element(); el != nil {
					tag = el.TagName()
				}
			}
			t.Logf("  [%d] type=%d tag=%s", i, ch.NodeType(), tag)
		}
	}

	rv := NewRenderTreeBuilder(resolver).Build(doc)
	rv.SetViewportSize(1280, 800)
	rv.Layout(nil)

	// Find body render box
	var bodyRO RenderObject
	var walk func(ro RenderObject)
	walk = func(ro RenderObject) {
		if el, ok := ro.Node().(*dom.Element); ok && el.TagName() == "BODY" {
			bodyRO = ro
		}
		for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(rv)

	if bodyRO != nil {
		box := asRenderBox(bodyRO)
		if box != nil {
			t.Logf("body render frame: (%.0f,%.0f %.0fx%.0f)", box.X(), box.Y(), box.Width(), box.Height())
		}
		lb := bodyRO.LayoutBox()
		if lb != nil {
			state := rv.LayoutState()
			if state != nil {
				g := state.GeometryForBox(lb)
				t.Logf("body layout box: (%.0f,%.0f %.0fx%.0f)", g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight())
			}
		}
	}

	canvas := graphics.NewCanvas(1280, 800)
	defer canvas.Release()
	canvas.Clear(graphics.Color{R: 255, G: 0, B: 0, A: 255})
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 1280, Height: 800})

	w := canvas.Width()
	pixels := canvas.Pixels()
	idx := (50*w + 50) * 4
	r, g, b := pixels[idx], pixels[idx+1], pixels[idx+2]
	t.Logf("pixel(50,50): #%02x%02x%02x", r, g, b)
}
