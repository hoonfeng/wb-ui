package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
)

// TestSVGPathSmoothBezier: S/s (smooth cubic) and T/t (smooth quadratic)
// commands must be parsed and sampled. Regression: the shield icon
// (M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z) used only 's' for its top
// arc — without S/s support the approval/review button icon drew a broken
// outline.
func TestSVGPathSmoothBezier(t *testing.T) {
	svgEl := dom.NewDocument().CreateElement("svg")
	svgEl.SetAttribute("viewBox", "0 0 24 24")
	p := dom.NewDocument().CreateElement("path")
	p.SetAttribute("d", "M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z")
	svgEl.AppendChild(p)

	sdoc := buildSVGDocument(svgEl)
	if sdoc == nil || len(sdoc.shapes) == 0 {
		t.Fatal("no shapes parsed")
	}
	fs, ok := sdoc.shapes[0].(*svgFilledShape)
	if !ok {
		t.Fatalf("shape type %T", sdoc.shapes[0])
	}
	sp, ok := fs.shape.(*svgPath)
	if !ok {
		t.Fatalf("wrapped %T, want *svgPath", fs.shape)
	}
	kinds := map[byte]bool{}
	for _, c := range sp.commands {
		kinds[c.kind] = true
	}
	for _, want := range []byte{'M', 's', 'V', 'l', 'v', 'c', 'z'} {
		if !kinds[want] {
			t.Fatalf("missing command %q in %v", string(want), kinds)
		}
	}

	// Sample the same path through the painter's point stream (indirectly via
	// a stroke: the path must produce far more than the 6 non-s points).
	fs.stroke = graphics.Color{R: 0, G: 255, B: 0, A: 0xFF}
	fs.strokeWidth = 2
	canvas := graphics.NewCanvas(48, 48)
	defer canvas.Release()
	ctx := defaultSVGContext()
	ctx.stroke = fs.stroke
	ctx.strokeWidth = fs.strokeWidth
	ctx.gradients = sdoc.gradients
	ctx.clips = sdoc.clips
	fs.paint(canvas, ctx)
	ink := 0
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			px := canvas.PixelAt(x, y)
			if px.A > 0 && px.G > 100 {
				ink++
			}
		}
	}
	if ink < 30 {
		t.Fatalf("shield stroke produced only %d ink px, want >30", ink)
	}
}
