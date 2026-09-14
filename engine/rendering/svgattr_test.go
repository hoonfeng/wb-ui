package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/style"
)

// TestSVGPresentationAttributeSize: an <svg width="18" height="18"> must size
// 18×18 (presentation attributes map to CSS width/height when no stylesheet
// declared them). Regression: every SVG icon in the Vue app was 0px wide.
func TestSVGPresentationAttributeSize(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "18")
	svgEl.SetAttribute("height", "18")
	svgEl.SetAttribute("viewBox", "0 0 24 24")
	doc.AppendChild(svgEl)

	builder := NewRenderTreeBuilder(style.NewResolver())
	rv := builder.Build(doc)
	if rv == nil {
		t.Fatal("render view nil")
	}
	// Walk to find the svg render box.
	var svgBox *RenderBox
	var walk func(o interface{ FirstChild() interface{} })
	_ = walk
	var walk2 func(o RenderObject)
	walk2 = func(o RenderObject) {
		if rb, ok := o.(*RenderBox); ok {
			if el, ok2 := rb.Node().(*dom.Element); ok2 && el.LocalName() == "svg" {
				svgBox = rb
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk2(c)
		}
	}
	walk2(rv)
	if svgBox == nil {
		t.Fatal("svg render box not found")
	}
	st := svgBox.Style()
	if st.Width.Value != 18 || st.Width.Unit != "px" {
		t.Fatalf("svg width=%v%s, want 18px", st.Width.Value, st.Width.Unit)
	}
	if st.Height.Value != 18 || st.Height.Unit != "px" {
		t.Fatalf("svg height=%v%s, want 18px", st.Height.Value, st.Height.Unit)
	}
}

// TestImgPresentationAttributeSize: an <img width="30" height="20"> sizes
// 30×20 even without any CSS.
func TestImgPresentationAttributeSize(t *testing.T) {
	doc := dom.NewDocument()
	imgEl := doc.CreateElement("img")
	imgEl.SetAttribute("width", "30")
	imgEl.SetAttribute("height", "20")
	imgEl.SetAttribute("src", "data:image/png;base64,AAAA")
	doc.AppendChild(imgEl)

	builder := NewRenderTreeBuilder(style.NewResolver())
	rv := builder.Build(doc)
	if rv == nil {
		t.Fatal("render view nil")
	}
	var imgBox *RenderBox
	var walk func(o RenderObject)
	walk = func(o RenderObject) {
		if rb, ok := o.(*RenderBox); ok {
			if el, ok2 := rb.Node().(*dom.Element); ok2 && el.LocalName() == "img" {
				imgBox = rb
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(rv)
	if imgBox == nil {
		t.Fatal("img render box not found")
	}
	st := imgBox.Style()
	if st.Width.Value != 30 || st.Height.Value != 20 {
		t.Fatalf("img wh=%vx%v, want 30x20", st.Width.Value, st.Height.Value)
	}
}
