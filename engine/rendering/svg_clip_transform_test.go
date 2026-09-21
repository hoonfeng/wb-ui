package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
)

// TestSVGClipPathFollowsElementTransform: for clipPathUnits="userSpaceOnUse"
// (the default) the clipPath resolves in the user coordinate system of the
// element referencing it — i.e. the clip has to follow that element's own
// transform. Setting the clip before applying the element transform leaves it
// baked into the untransformed space, which can clip the element away entirely.
func TestSVGClipPathFollowsElementTransform(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "48")
	svgEl.SetAttribute("height", "24")
	svgEl.SetAttribute("viewBox", "0 0 48 24")

	defs := doc.CreateElement("defs")
	cp := doc.CreateElement("clipPath")
	cp.SetAttribute("id", "c")
	cr := doc.CreateElement("rect")
	cr.SetAttribute("x", "4")
	cr.SetAttribute("y", "4")
	cr.SetAttribute("width", "12")
	cr.SetAttribute("height", "12")
	cp.AppendChild(cr)
	defs.AppendChild(cp)
	svgEl.AppendChild(defs)

	r := doc.CreateElement("rect")
	r.SetAttribute("x", "4")
	r.SetAttribute("y", "4")
	r.SetAttribute("width", "12")
	r.SetAttribute("height", "12")
	r.SetAttribute("fill", "black")
	r.SetAttribute("clip-path", "url(#c)")
	r.SetAttribute("transform", "translate(20,0)")
	svgEl.AppendChild(r)

	sd := buildSVGDocument(svgEl)
	if sd == nil {
		t.Fatal("buildSVGDocument returned nil")
	}
	canvas := graphics.NewCanvas(48, 24)
	defer canvas.Release()
	paintSVGTo(canvas, sd, 0, 0, 48, 24, graphics.Color{})

	// translate(20,0) moves the 12x12 rect from (4,4) to (24,4)-(36,16). The
	// clip must travel with it, so the moved area stays painted.
	if px := canvas.PixelAt(30, 10); px.A == 0 {
		t.Errorf("(30,10) 透明：clip-path 没有跟随元素 transform（裁剪被烘焙在未变换坐标系里）")
	}
	if px := canvas.PixelAt(8, 8); px.A != 0 {
		t.Errorf("(8,8) = %+v，期望透明（元素已 translate 走）", px)
	}
}
