package rendering

import (
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

func TestLayoutBoxExists(t *testing.T) {
	htmlSrc := "<!DOCTYPE html>\n<html><head><style>\n" +
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

	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("RenderView is nil")
	}

	t.Logf("RenderView LayoutBox: %v", rv.LayoutBox())
	if rv.FirstChild() != nil {
		t.Logf("first child LayoutBox: %v", rv.FirstChild().LayoutBox())
	}

	rv.SetViewportSize(1280, 800)

	t.Logf("BEFORE Layout - RenderView LayoutBox: %v", rv.LayoutBox())
	if lb := rv.LayoutBox(); lb != nil {
		t.Logf("  lb.Rect = %.0f,%.0f %.0fx%.0f", lb.Rect.X, lb.Rect.Y, lb.Rect.Width, lb.Rect.Height)
	}

	rv.Layout(nil)

	t.Logf("AFTER Layout - RenderView LayoutBox: %v", rv.LayoutBox())
	if lb := rv.LayoutBox(); lb != nil {
		t.Logf("  lb.Rect = %.0f,%.0f %.0fx%.0f", lb.Rect.X, lb.Rect.Y, lb.Rect.Width, lb.Rect.Height)
	}

	// Paint
	canvas := graphics.NewCanvas(1280, 800)
	defer canvas.Release()
	canvas.Clear(graphics.Color{R: 255, G: 0, B: 0, A: 255})
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 1280, Height: 800})

	w := canvas.Width()
	pixels := canvas.Pixels()
	idx := (50*w + 50) * 4
	r, g, b := pixels[idx], pixels[idx+1], pixels[idx+2]
	t.Logf("pixel(50,50): #%02x%02x%02x", r, g, b)
	if r == 0x16 && g == 0x21 && b == 0x3e {
		t.Log("PAINT WORKS!")
	} else if r == 255 && g == 0 && b == 0 {
		t.Error("still red")
	}
}
