package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
)

// TestSVGPolygonStroke: a polygon with fill=none + stroke must draw its
// outline. Regression: svgPolygon.paint only filled triangles and never
// stroked, so stroke-only icons (e.g. the send-btn paper plane with
// fill="none" stroke="currentColor") drew nothing.
func TestSVGPolygonStroke(t *testing.T) {
	svgEl := dom.NewDocument().CreateElement("svg")
	svgEl.SetAttribute("viewBox", "0 0 24 24")
	// Triangle polygon, stroke-only (fill none).
	pg := dom.NewDocument().CreateElement("polygon")
	pg.SetAttribute("points", "0,0 24,0 12,24")
	pg.SetAttribute("fill", "none")
	pg.SetAttribute("stroke", "#ff0000")
	pg.SetAttribute("stroke-width", "2")
	svgEl.AppendChild(pg)

	sdoc := buildSVGDocument(svgEl)
	if sdoc == nil || len(sdoc.shapes) == 0 {
		t.Fatal("no shapes parsed")
	}
	fs, ok := sdoc.shapes[0].(*svgFilledShape)
	if !ok {
		t.Fatalf("shape type %T", sdoc.shapes[0])
	}
	if fs.stroke.A != 0xFF || fs.stroke.R != 0xFF {
		t.Fatalf("stroke=%+v, want red opaque", fs.stroke)
	}
	t.Logf("fs: stroke=%+v strokeWidth=%v shape=%T fill=%+v", fs.stroke, fs.strokeWidth, fs.shape, fs.fill)
	if sp, ok := fs.shape.(*svgPolygon); ok {
		t.Logf("polygon points=%v closed=%v", sp.points, sp.closed)
	}
	// Paint to an offscreen canvas and verify ink exists (stroke drawn).
	canvas := graphics.NewCanvas(48, 48)
	defer canvas.Release()
	ctx := defaultSVGContext()
	ctx.stroke = fs.stroke
	ctx.strokeWidth = fs.strokeWidth
	ctx.gradients = sdoc.gradients
	ctx.clips = sdoc.clips
	fs.paint(canvas, ctx)
	t.Logf("canvas: %dx%d", canvas.Width(), canvas.Height())
	ink := 0
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			p := canvas.PixelAt(x, y)
			if p.A > 0 && (p.R > 200 || p.B > 200) {
				ink++
			}
		}
	}
	t.Logf("ink=%d", ink)
	if ink < 10 {
		t.Fatalf("polygon stroke produced only %d ink px, want >10", ink)
	}
}
