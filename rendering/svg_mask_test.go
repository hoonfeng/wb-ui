package rendering

import (
	"encoding/base64"
	"strings"
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// TestSVGMaskParseAndRender verifies <mask> element parsing and rasterization:
// the default maskUnits (objectBoundingBox) maps the -10%/-10%/120%/120% region
// of a 40×40 target to a 48×48 image, and the child shape is drawn inside it.
func TestSVGMaskParseAndRender(t *testing.T) {
	doc := parseSVGText(`<svg xmlns="http://www.w3.org/2000/svg"><defs><mask id="m"><rect x="0" y="0" width="20" height="20" fill="white"/></mask></defs></svg>`)
	if doc == nil {
		t.Fatal("parseSVGText returned nil")
	}
	m := doc.masks["m"]
	if m == nil {
		t.Fatal("mask #m not parsed")
	}
	if m.maskType != "" && !strings.EqualFold(m.maskType, "luminance") {
		t.Fatalf("mask-type = %q, want luminance default", m.maskType)
	}
	// objectBoundingBox default: x=-10%, y=-10%, w=120%, h=120% of 40×40 → 48×48.
	img := renderSVGMask(m, 40, 40)
	if img == nil {
		t.Fatal("renderSVGMask returned nil")
	}
	defer img.Release()
	if img.Width() != 48 || img.Height() != 48 {
		t.Fatalf("mask image = %dx%d, want 48x48", img.Width(), img.Height())
	}
}

// TestMaskImageSVGMask verifies mask-image: url(data:image/svg+xml;base64,...#id)
// references an SVG <mask> element and masks the whole subtree. The mask's white
// rect covers the left half, so the left half survives and the right half is
// masked out.
func TestMaskImageSVGMask(t *testing.T) {
	svgText := `<svg xmlns="http://www.w3.org/2000/svg"><defs><mask id="m"><rect x="0" y="0" width="20" height="40" fill="white"/></mask></defs></svg>`
	uri := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svgText)) + "#m"

	cs := style.NewComputedStyle()
	cs.BackgroundColor = style.Color{R: 255, A: 255}
	cs.SetProperty("mask-image", "url("+uri+")")
	canvas := paintMaskLayerFixture(t, 40, 40, cs, nil)
	defer canvas.Release()

	if px := canvas.PixelAt(10, 20); px.R != 255 || px.A != 255 {
		t.Fatalf("left half (mask kept) = %+v, want red", px)
	}
	if px := canvas.PixelAt(30, 20); px.A != 0 {
		t.Fatalf("right half (masked out) = %+v, want transparent", px)
	}
}

// TestMaskImageSVGMaskAlpha verifies mask-type: alpha makes the mask value come
// from the child shape's alpha rather than its luminance.
func TestMaskImageSVGMaskAlpha(t *testing.T) {
	svgText := `<svg xmlns="http://www.w3.org/2000/svg"><defs><mask id="m" mask-type="alpha"><rect x="0" y="0" width="20" height="40" fill="black"/></mask></defs></svg>`
	uri := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svgText)) + "#m"

	cs := style.NewComputedStyle()
	cs.BackgroundColor = style.Color{R: 255, A: 255}
	cs.SetProperty("mask-image", "url("+uri+")")
	canvas := paintMaskLayerFixture(t, 40, 40, cs, nil)
	defer canvas.Release()

	// mask-type: alpha → black fill still has alpha 255 (opaque) → left kept.
	if px := canvas.PixelAt(10, 20); px.R != 255 || px.A != 255 {
		t.Fatalf("left half (alpha mask kept) = %+v, want red", px)
	}
	// luminance mode would have masked the black out (luminance 0); alpha keeps it.
	if px := canvas.PixelAt(30, 20); px.A != 0 {
		t.Fatalf("right half (masked out) = %+v, want transparent", px)
	}
}

// TestMaskImageSameDocumentReference verifies mask-image: url(#id) resolves an
// inline <svg><mask id="id"> in the SAME document (not an external file / data
// URI). This was the last mask-image gap: same-document fragment references.
func TestMaskImageSameDocumentReference(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	defsEl := doc.CreateElement("defs")
	maskEl := doc.CreateElement("mask")
	maskEl.SetAttribute("id", "m")
	maskEl.SetAttribute("maskContentUnits", "objectBoundingBox")
	rectEl := doc.CreateElement("rect")
	rectEl.SetAttribute("x", "0.5")
	rectEl.SetAttribute("y", "0")
	rectEl.SetAttribute("width", "0.5")
	rectEl.SetAttribute("height", "1")
	rectEl.SetAttribute("fill", "white")
	_ = maskEl.AppendChild(rectEl)
	_ = defsEl.AppendChild(maskEl)
	_ = svgEl.AppendChild(defsEl)
	_ = doc.AppendChild(svgEl)

	cs := style.NewComputedStyle()
	cs.BackgroundColor = style.Color{R: 255, A: 255}
	cs.SetProperty("mask-image", "url(#m)")
	canvas := paintSameDocMaskFixture(t, 40, 40, cs, doc)
	defer canvas.Release()

	// Left half has no mask content → masked out (transparent).
	if px := canvas.PixelAt(10, 20); px.A != 0 {
		t.Fatalf("left half (no mask content) = %+v, want transparent", px)
	}
	// Right half covered by the white rect → kept (red background survives).
	if px := canvas.PixelAt(30, 20); px.R != 255 || px.A != 255 {
		t.Fatalf("right half (mask kept) = %+v, want red", px)
	}
}

// paintSameDocMaskFixture is paintMaskLayerFixture but takes a caller-built
// document so the test can mount an inline <svg><mask> tree for same-document
// url(#id) resolution.
func paintSameDocMaskFixture(t *testing.T, w, h int, cs *style.ComputedStyle, doc *dom.Document) *graphics.Canvas {
	t.Helper()
	canvas := graphics.NewCanvas(w, h)
	rv := NewRenderView(doc, style.NewComputedStyle())
	rv.SetViewportSize(float64(w), float64(h))
	box := NewRenderBox(doc.CreateElement("div"), cs)
	box.SetLocation(0, 0)
	box.SetSize(float64(w), float64(h))
	rv.AddChild(box, nil)
	comp := NewRenderLayerCompositor(rv)
	rootLayer := comp.BuildLayerTree(RenderObject(rv))
	if rootLayer == nil {
		t.Fatal("BuildLayerTree returned nil")
	}
	rv.SetRootLayer(rootLayer)
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: float64(w), Height: float64(h)})
	return canvas
}
