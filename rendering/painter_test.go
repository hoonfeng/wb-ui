// Translation of: tests for Source/WebCore/rendering/painting/*Painter.cpp
// Completeness: 50%
// Simplifications:
//   - tests verify pixel-level output of the pure-Go rasterizer; no golden image
//     comparison is performed.
//   - border / outline geometry is checked at representative pixels rather than the
//     full outline.

package rendering

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// newPaintCanvas builds a 20x20 canvas and a PaintInfo covering it.
func newPaintCanvas(t *testing.T) (*graphics.Canvas, *PaintInfo) {
	t.Helper()
	c := graphics.NewCanvas(20, 20)
	return c, NewPaintInfo(c, Rect{X: 0, Y: 0, Width: 20, Height: 20})
}

// newBoxWithStyle builds a RenderBox positioned at (x,y) with the given size and style.
func newBoxWithStyle(st *style.ComputedStyle, x, y, w, h float64) *RenderBox {
	box := NewRenderBox(dom.NewDocument().CreateElement("div"), st)
	box.SetLocation(x, y)
	box.SetSize(w, h)
	return box
}

// TestPaintBackground verifies PaintBackground fills the border-box with the background
// color and leaves the surrounding area transparent.
func TestPaintBackground(t *testing.T) {
	canvas, info := newPaintCanvas(t)
	st := style.NewComputedStyle()
	st.BackgroundColor = style.Color{R: 0xFF, G: 0, B: 0, A: 0xFF}
	box := newBoxWithStyle(st, 2, 2, 8, 8)

	PaintBackground(box, info)

	red := graphics.Color{R: 0xFF, G: 0, B: 0, A: 0xFF}
	if got := canvas.PixelAt(5, 5); got != red {
		t.Fatalf("inside pixel = %+v, want %+v", got, red)
	}
	if got := canvas.PixelAt(9, 9); got != red {
		t.Fatalf("corner pixel = %+v, want %+v", got, red)
	}
	if got := canvas.PixelAt(0, 0); got != (graphics.Color{}) {
		t.Fatalf("outside pixel = %+v, want transparent", got)
	}
}

// TestPaintBackgroundTransparent verifies a fully transparent background paints nothing.
func TestPaintBackgroundTransparent(t *testing.T) {
	canvas, info := newPaintCanvas(t)
	st := style.NewComputedStyle()
	st.BackgroundColor = style.Color{R: 0xFF, G: 0, B: 0, A: 0} // transparent
	box := newBoxWithStyle(st, 2, 2, 8, 8)

	PaintBackground(box, info)

	if got := canvas.PixelAt(5, 5); got != (graphics.Color{}) {
		t.Fatalf("transparent background pixel = %+v, want transparent", got)
	}
}

// TestPaintBorder verifies PaintBorder draws the border on all four sides while leaving
// the interior untouched.
func TestPaintBorder(t *testing.T) {
	canvas, info := newPaintCanvas(t)
	st := style.NewComputedStyle()
	black := style.Color{R: 0, G: 0, B: 0, A: 0xFF}
	st.BorderTopWidth = style.Length{Value: 2, Unit: "px"}
	st.BorderTopColor = black
	st.BorderTopStyle = "solid"
	st.BorderRightWidth = style.Length{Value: 2, Unit: "px"}
	st.BorderRightColor = black
	st.BorderRightStyle = "solid"
	st.BorderBottomWidth = style.Length{Value: 2, Unit: "px"}
	st.BorderBottomColor = black
	st.BorderBottomStyle = "solid"
	st.BorderLeftWidth = style.Length{Value: 2, Unit: "px"}
	st.BorderLeftColor = black
	st.BorderLeftStyle = "solid"
	box := newBoxWithStyle(st, 4, 4, 10, 10)

	PaintBorder(box, info)

	blk := graphics.Color{R: 0, G: 0, B: 0, A: 0xFF}
	// Top border row painted.
	if got := canvas.PixelAt(6, 4); got != blk {
		t.Fatalf("top border pixel = %+v, want %+v", got, blk)
	}
	// Left border column painted.
	if got := canvas.PixelAt(4, 8); got != blk {
		t.Fatalf("left border pixel = %+v, want %+v", got, blk)
	}
	// Interior untouched.
	if got := canvas.PixelAt(8, 8); got != (graphics.Color{}) {
		t.Fatalf("interior pixel = %+v, want transparent", got)
	}
}

// TestPaintBorderNoneSkipped verifies that a side with border-style "none" is not painted
// even when its width is set.
func TestPaintBorderNoneSkipped(t *testing.T) {
	canvas, info := newPaintCanvas(t)
	st := style.NewComputedStyle()
	st.BorderTopWidth = style.Length{Value: 3, Unit: "px"}
	st.BorderTopColor = style.Color{R: 0xFF, A: 0xFF}
	st.BorderTopStyle = "none" // none -> skipped
	box := newBoxWithStyle(st, 2, 2, 10, 10)

	PaintBorder(box, info)

	if got := canvas.PixelAt(5, 2); got != (graphics.Color{}) {
		t.Fatalf("none-style border pixel = %+v, want transparent", got)
	}
}

// TestPaintText verifies PaintText draws the text color within a laid-out segment.
// The test scans the segment's bounding region for non-transparent pixels, since Skia's
// real font metrics place glyphs below the box top (baseline = top + ascent).
func TestPaintText(t *testing.T) {
	canvas, info := newPaintCanvas(t)
	st := style.NewComputedStyle()
	st.Color = style.Color{R: 0, G: 0, B: 0, A: 0xFF}
	st.FontSize = style.Length{Value: 12, Unit: "px"}
	rt := NewRenderTextWith(dom.NewDocument().CreateTextNode("Hi"), st, "Hi")
	rt.SetSegments([]InlineTextBox{{Start: 0, Len: 2, X: 2, Y: 2, Width: 10, Height: 12}})

	PaintText(rt, info)

	found := false
	for y := 2; y < 16 && !found; y++ {
		for x := 2; x < 14 && !found; x++ {
			if p := canvas.PixelAt(x, y); p.A > 0 && p.B == 0 {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no black text pixels found in segment region")
	}
}

// TestPaintTextNoSegments verifies PaintText does NOT draw anything when no
// inline segments are present (text was not laid out). Previously it drew at
// the origin, causing stray text in the top-left corner.
func TestPaintTextNoSegments(t *testing.T) {
	canvas, info := newPaintCanvas(t)
	st := style.NewComputedStyle()
	st.Color = style.Color{R: 0xFF, G: 0, B: 0, A: 0xFF}
	st.FontSize = style.Length{Value: 12, Unit: "px"}
	rt := NewRenderTextWith(dom.NewDocument().CreateTextNode("A"), st, "A")

	PaintText(rt, info)

	// Canvas should remain fully transparent — no text drawn.
	for y := 0; y < 18; y++ {
		for x := 0; x < 14; x++ {
			if p := canvas.PixelAt(x, y); p.A > 0 {
				t.Fatalf("unexpected pixel at (%d,%d): %+v — text should not be drawn without segments", x, y, p)
			}
		}
	}
}

// TestPaintOutline verifies PaintOutline draws a stroked rectangle outside the border-box.
func TestPaintOutline(t *testing.T) {
	canvas, info := newPaintCanvas(t)
	st := style.NewComputedStyle()
	st.SetProperty("outline-style", "solid")
	st.SetProperty("outline-width", "2px")
	st.SetProperty("outline-color", "#00ff00")
	box := newBoxWithStyle(st, 6, 6, 6, 6)

	PaintOutline(box, info)

	green := graphics.Color{R: 0, G: 0xFF, B: 0, A: 0xFF}
	// The outline sits outside the border-box: at x = 6 - 0 - 1 = 5 (half of width 2).
	if got := canvas.PixelAt(5, 6); got != green {
		t.Fatalf("outline pixel = %+v, want %+v", got, green)
	}
	// The interior of the box is not touched by the outline pass.
	if got := canvas.PixelAt(8, 8); got != (graphics.Color{}) {
		t.Fatalf("outline interior pixel = %+v, want transparent", got)
	}
}

// TestPaintOutlineNone verifies that outline-style "none" (or unset) paints nothing.
func TestPaintOutlineNone(t *testing.T) {
	canvas, info := newPaintCanvas(t)
	st := style.NewComputedStyle()
	box := newBoxWithStyle(st, 6, 6, 6, 6)

	PaintOutline(box, info)

	if got := canvas.PixelAt(5, 6); got != (graphics.Color{}) {
		t.Fatalf("no-outline pixel = %+v, want transparent", got)
	}
}

// createTestPNG generates a tiny RGBA PNG in memory for testing PaintImage.
func createTestPNG(t *testing.T, w, h int, r, g, b, a uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	col := color.RGBA{R: r, G: g, B: b, A: a}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, col)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

// TestPaintImage verifies that PaintImage draws a decoded image at the
// content-box position and that a box with no image paints nothing.
func TestPaintImage(t *testing.T) {
	canvas := graphics.NewCanvas(30, 30)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 30, Height: 30})

	// Create a 10x10 red PNG and attach it to a RenderBox representing <img>.
	pngData := createTestPNG(t, 10, 10, 0xFF, 0, 0, 0xFF)
	di := NewDecodedImage(pngData)
	if di == nil {
		t.Fatal("NewDecodedImage returned nil")
	}
	defer di.Release()

	doc := dom.NewDocument()
	el := doc.CreateElement("img")
	st := style.NewComputedStyle()
	box := NewRenderBox(el, st)
	box.SetLocation(5, 5)
	box.SetSize(20, 20)
	box.SetDecodedImage(di)

	painted := PaintImage(box, info)
	if !painted {
		t.Fatal("PaintImage returned false, want true (image should be painted)")
	}

	// Center of the box should be red (image drawn at content-box).
	red := graphics.Color{R: 0xFF, A: 0xFF}
	if got := canvas.PixelAt(15, 15); got != red {
		t.Fatalf("center pixel = %+v, want %+v (red)", got, red)
	}

	// Outside the box should be transparent.
	if got := canvas.PixelAt(2, 2); got != (graphics.Color{}) {
		t.Fatalf("outside pixel = %+v, want transparent", got)
	}
}

// TestPaintImageNoImage verifies that PaintImage returns false when the box
// has no decoded image attached (not loaded yet).
func TestPaintImageNoImage(t *testing.T) {
	canvas := graphics.NewCanvas(20, 20)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 20, Height: 20})

	doc := dom.NewDocument()
	box := NewRenderBox(doc.CreateElement("img"), style.NewComputedStyle())
	box.SetLocation(0, 0)
	box.SetSize(16, 16)

	painted := PaintImage(box, info)
	if painted {
		t.Fatal("PaintImage returned true, want false (no image attached)")
	}

	// Canvas should remain fully transparent.
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			if p := canvas.PixelAt(x, y); p.A > 0 {
				t.Fatalf("unexpected pixel at (%d,%d): %+v", x, y, p)
			}
		}
	}
}

// TestPaintImageScaled verifies that PaintImage scales the image to fill
// the content-box when the box is larger than the image.
func TestPaintImageScaled(t *testing.T) {
	canvas := graphics.NewCanvas(40, 40)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 40, Height: 40})

	// Create a tiny 2x2 green PNG.
	pngData := createTestPNG(t, 2, 2, 0, 0xFF, 0, 0xFF)
	di := NewDecodedImage(pngData)
	if di == nil {
		t.Fatal("NewDecodedImage returned nil")
	}
	defer di.Release()

	doc := dom.NewDocument()
	box := NewRenderBox(doc.CreateElement("img"), style.NewComputedStyle())
	box.SetLocation(5, 5)
	box.SetSize(30, 30)
	box.SetDecodedImage(di)

	painted := PaintImage(box, info)
	if !painted {
		t.Fatal("PaintImage returned false, want true")
	}

	// The 30x30 content area should be filled with green.
	green := graphics.Color{G: 0xFF, A: 0xFF}
	if got := canvas.PixelAt(20, 20); got != green {
		t.Fatalf("center pixel = %+v, want %+v (green)", got, green)
	}
	// Corner of content box should also be green.
	if got := canvas.PixelAt(6, 6); got != green {
		t.Fatalf("content-box corner pixel = %+v, want %+v (green)", got, green)
	}
}

// TestAsRenderBox verifies the embedded-box recovery for every box-bearing concrete type.
func TestAsRenderBox(t *testing.T) {
	doc := dom.NewDocument()
	view := NewRenderView(doc, style.NewComputedStyle())
	block := NewRenderBlock(doc.CreateElement("div"), style.NewComputedStyle())
	flow := NewRenderBlockFlow(doc.CreateElement("div"), style.NewComputedStyle())
	box := NewRenderBox(doc.CreateElement("div"), style.NewComputedStyle())
	inline := NewRenderInline(doc.CreateElement("span"), style.NewComputedStyle())

	if asRenderBox(view) == nil {
		t.Error("asRenderBox(RenderView) = nil")
	}
	if asRenderBox(block) == nil {
		t.Error("asRenderBox(RenderBlock) = nil")
	}
	if asRenderBox(flow) == nil {
		t.Error("asRenderBox(RenderBlockFlow) = nil")
	}
	if asRenderBox(box) == nil {
		t.Error("asRenderBox(RenderBox) = nil")
	}
	if asRenderBox(inline) != nil {
		t.Error("asRenderBox(RenderInline) should be nil")
	}
}

// saveRestoreSelection saves the global CurrentSelection and restores it
// after the test completes, preventing interference between selection tests.
func saveRestoreSelection(t *testing.T) {
	t.Helper()
	old := CurrentSelection
	t.Cleanup(func() { CurrentSelection = old })
}

// TestPaintSelection verifies that PaintSelection draws semi-transparent blue
// rectangles over selected text segments and that clearing the selection
// produces no highlight.
func TestPaintSelection(t *testing.T) {
	saveRestoreSelection(t)

	canvas := graphics.NewCanvas(100, 50)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 100, Height: 50})
	doc := dom.NewDocument()
	rv := NewRenderView(doc, style.NewComputedStyle())

	// Create a RenderText with a segment covering "Hello" at (10,10) size 40x16.
	rt := NewRenderTextWith(doc.CreateTextNode("Hello World"), style.NewComputedStyle(), "Hello World")
	rt.SetSegments([]InlineTextBox{
		{Start: 0, Len: 11, X: 10, Y: 10, Width: 80, Height: 16, LineY: 10, LineHeight: 20},
	})
	rv.AddChild(rt, nil)

	// Select the first 5 characters ("Hello").
	CurrentSelection = &Selection{
		Start: TextPosition{RT: rt, Offset: 0},
		End:   TextPosition{RT: rt, Offset: 5},
	}

	PaintSelection(rv, info)

	// The selection highlight should produce non-transparent pixels in the
	// selected area (around x=10..50, y=10..30).
	found := false
	for y := 10; y < 30; y++ {
		for x := 10; x < 50; x++ {
			if p := canvas.PixelAt(x, y); p.A > 0 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("no selection highlight pixels found in the selected area")
	}
}

// TestPaintSelectionCleared verifies that after ClearSelection, PaintSelection
// paints nothing.
func TestPaintSelectionCleared(t *testing.T) {
	saveRestoreSelection(t)

	canvas := graphics.NewCanvas(100, 50)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 100, Height: 50})
	doc := dom.NewDocument()
	rv := NewRenderView(doc, style.NewComputedStyle())

	CurrentSelection = nil // explicitly cleared
	PaintSelection(rv, info)

	for y := 0; y < 50; y++ {
		for x := 0; x < 100; x++ {
			if p := canvas.PixelAt(x, y); p.A > 0 {
				t.Fatalf("unexpected pixel at (%d,%d): %+v", x, y, p)
			}
		}
	}
}

// TestPaintSelectionMultiSegment verifies that when a selection spans multiple
// segments on the same RenderText, each segment draws highlight rectangles.
func TestPaintSelectionMultiSegment(t *testing.T) {
	saveRestoreSelection(t)

	canvas := graphics.NewCanvas(200, 50)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 200, Height: 50})
	doc := dom.NewDocument()
	rv := NewRenderView(doc, style.NewComputedStyle())

	// Two segments: "Hello" at (10,10) and "World" at (90,10).
	rt := NewRenderTextWith(doc.CreateTextNode("Hello World"), style.NewComputedStyle(), "Hello World")
	rt.SetSegments([]InlineTextBox{
		{Start: 0, Len: 5, X: 10, Y: 10, Width: 40, Height: 16, LineY: 10, LineHeight: 20},
		{Start: 6, Len: 5, X: 90, Y: 10, Width: 40, Height: 16, LineY: 10, LineHeight: 20},
	})
	rv.AddChild(rt, nil)

	// Select the full range (both segments).
	CurrentSelection = &Selection{
		Start: TextPosition{RT: rt, Offset: 0},
		End:   TextPosition{RT: rt, Offset: 11},
	}

	PaintSelection(rv, info)

	// Both segment areas should have non-transparent pixels.
	firstSeg := false
	for y := 10; y < 30; y++ {
		for x := 10; x < 50; x++ {
			if p := canvas.PixelAt(x, y); p.A > 0 {
				firstSeg = true
			}
		}
	}
	if !firstSeg {
		t.Fatal("no highlight pixels found in first segment")
	}
	secondSeg := false
	for y := 10; y < 30; y++ {
		for x := 90; x < 130; x++ {
			if p := canvas.PixelAt(x, y); p.A > 0 {
				secondSeg = true
			}
		}
	}
	if !secondSeg {
		t.Fatal("no highlight pixels found in second segment")
	}
}
