package rendering

import (
	"encoding/base64"
	"net/url"
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// TestParseSVGText: an SVG string parses to a paintable svgDocument.
func TestParseSVGText(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10" viewBox="0 0 10 10"><rect x="0" y="0" width="10" height="10" fill="red"/></svg>`
	doc := parseSVGText(svg)
	if doc == nil {
		t.Fatalf("parseSVGText returned nil")
	}
	if doc.width != 10 || doc.height != 10 {
		t.Fatalf("size = %vx%v, want 10x10", doc.width, doc.height)
	}
	if len(doc.shapes) == 0 {
		t.Fatalf("no shapes parsed")
	}
	if !doc.hasVB {
		t.Fatalf("viewBox not parsed")
	}
}

// TestLoadBackgroundSVGDataURI: data:image/svg+xml;base64 loads and parses.
func TestLoadBackgroundSVGDataURI(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8" viewBox="0 0 8 8"><rect width="8" height="8" fill="blue"/></svg>`
	uri := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
	doc := loadBackgroundSVG(uri)
	if doc == nil {
		t.Fatalf("loadBackgroundSVG(base64) returned nil")
	}
	if doc.width != 8 {
		t.Fatalf("width = %v, want 8", doc.width)
	}
}

// TestLoadBackgroundSVGURLEncoded: data:image/svg+xml,<encoded> loads.
func TestLoadBackgroundSVGURLEncoded(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8" viewBox="0 0 8 8"><circle cx="4" cy="4" r="4" fill="green"/></svg>`
	uri := "data:image/svg+xml," + url.QueryEscape(svg)
	doc := loadBackgroundSVG(uri)
	if doc == nil {
		t.Fatalf("loadBackgroundSVG(urlencoded) returned nil")
	}
	if len(doc.shapes) == 0 {
		t.Fatalf("no circle shape parsed")
	}
}

// TestPaintBackgroundSVG: an SVG data-URI background paints a red rect
// scaled into the box.
func TestPaintBackgroundSVG(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10" viewBox="0 0 10 10"><rect width="10" height="10" fill="red"/></svg>`
	uri := "data:image/svg+xml," + url.QueryEscape(svg)

	// Step 1: verify parse + shape list.
	sdoc := parseSVGText(svg)
	if sdoc == nil || len(sdoc.shapes) == 0 {
		t.Fatalf("parse failed")
	}
	t.Logf("shapes=%d width=%v height=%v hasVB=%v", len(sdoc.shapes), sdoc.width, sdoc.height, sdoc.hasVB)

	// Step 2: direct paintSVG (no scale) on a 20x20 canvas.
	canvas0 := graphics.NewCanvas(20, 20)
	defer canvas0.Release()
	paintSVG(canvas0, sdoc, 0, 0, graphics.Color{})
	if px := canvas0.PixelAt(5, 5); px.R != 255 {
		t.Logf("direct paintSVG (5,5) = %+v", px)
	} else {
		t.Logf("direct paintSVG (5,5) = red OK")
	}

	// Step 3: direct paintSVGScaled.
	canvas2 := graphics.NewCanvas(80, 40)
	defer canvas2.Release()
	paintSVGScaled(canvas2, sdoc, 20, 10, 40, 20)
	if px := canvas2.PixelAt(40, 20); px.R != 255 || px.G != 0 {
		t.Fatalf("direct paintSVGScaled center = %+v, want red", px)
	}

	canvas := graphics.NewCanvas(80, 40)
	defer canvas.Release()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 80, Height: 40})

	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	st := style.NewComputedStyle()
	st.BackgroundImage = "url(" + uri + ")"
	// Explicit size 40x20 centered in 80x40 → covers (20,10)-(60,30).
	st.BackgroundSize = "40px 20px"
	st.BackgroundPosition = "center"
	box := NewRenderBox(el, st)
	box.SetLocation(0, 0)
	box.SetSize(80, 40)

	paintObjectBackground(box, info)

	if px := canvas.PixelAt(40, 20); px.R != 255 || px.G != 0 {
		t.Fatalf("center (40,20) = %+v, want red", px)
	}
	if px := canvas.PixelAt(4, 4); px.A != 0 {
		t.Fatalf("outside svg (4,4) = %+v, want transparent", px)
	}
}

// TestPaintImageSVG: an <img src="data:image/svg+xml,..."> paints the SVG
// scaled into the content box.
func TestPaintImageSVG(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10" viewBox="0 0 10 10"><circle cx="5" cy="5" r="5" fill="red"/></svg>`
	uri := "data:image/svg+xml," + url.QueryEscape(svg)

	canvas := graphics.NewCanvas(60, 30)
	defer canvas.Release()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 60, Height: 30})

	doc := dom.NewDocument()
	imgEl := doc.CreateElement("img")
	imgEl.SetAttribute("src", uri)
	st := style.NewComputedStyle()
	box := NewRenderBox(imgEl, st)
	box.SetLocation(0, 0)
	box.SetSize(60, 30)

	if !PaintImage(box, info) {
		t.Fatalf("PaintImage returned false")
	}
	// Center of the SVG circle scaled into the box → red.
	if px := canvas.PixelAt(30, 15); px.R != 255 || px.G != 0 {
		t.Fatalf("center (30,15) = %+v, want red", px)
	}
	// Corner outside the circle stays transparent.
	if px := canvas.PixelAt(2, 2); px.A != 0 {
		t.Fatalf("corner (2,2) = %+v, want transparent", px)
	}
}

var _ = base64.StdEncoding
