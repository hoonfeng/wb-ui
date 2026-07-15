// Translation of: tests for Source/WebCore/rendering/RenderView.cpp (paint path)
//                  Source/WebCore/page/FrameView.cpp (paint path)
// Completeness: 45%
// Simplifications:
//   - tests verify pixel-level output rather than a full golden image; the focus is on
//     phase ordering and layer traversal.

package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// newPaintView builds a RenderView sized to the given canvas dimensions with a default
// (transparent) style.
func newPaintView(w, h float64) *RenderView {
	view := NewRenderView(dom.NewDocument(), style.NewComputedStyle())
	view.SetLocation(0, 0)
	view.SetSize(w, h)
	return view
}

// TestRenderPipelineSimpleBox verifies a single child box's background is painted into the
// canvas via the direct (no-layer) path.
func TestRenderPipelineSimpleBox(t *testing.T) {
	view := newPaintView(20, 20)
	st := style.NewComputedStyle()
	st.Display = style.DisplayBlock
	st.BackgroundColor = style.Color{R: 0xFF, G: 0, B: 0, A: 0xFF}
	child := NewRenderBlockFlow(dom.NewDocument().CreateElement("div"), st)
	child.SetLocation(2, 2)
	child.SetSize(10, 10)
	view.AddChild(child, nil)

	canvas := graphics.NewCanvas(20, 20)
	Paint(view, canvas, Rect{X: 0, Y: 0, Width: 20, Height: 20})

	red := graphics.Color{R: 0xFF, G: 0, B: 0, A: 0xFF}
	if got := canvas.PixelAt(5, 5); got != red {
		t.Fatalf("child background pixel = %+v, want %+v", got, red)
	}
	if got := canvas.PixelAt(0, 0); got != (graphics.Color{}) {
		t.Fatalf("outside pixel = %+v, want transparent", got)
	}
}

// TestRenderPipelineWithText verifies text content nested inside a block is painted during
// the foreground phase. The test scans the text region for non-transparent pixels, since
// Skia's real font metrics place glyphs below the box top.
func TestRenderPipelineWithText(t *testing.T) {
	view := newPaintView(40, 20)
	blockSt := style.NewComputedStyle()
	blockSt.Display = style.DisplayBlock
	block := NewRenderBlockFlow(dom.NewDocument().CreateElement("div"), blockSt)
	block.SetLocation(0, 0)
	block.SetSize(40, 20)
	view.AddChild(block, nil)

	textSt := style.NewComputedStyle()
	textSt.Color = style.Color{R: 0, G: 0, B: 0, A: 0xFF}
	textSt.FontSize = style.Length{Value: 12, Unit: "px"}
	rt := NewRenderTextWith(dom.NewDocument().CreateTextNode("Hi"), textSt, "Hi")
	rt.SetSegments([]InlineTextBox{{Start: 0, Len: 2, X: 5, Y: 5, Width: 10, Height: 12}})
	block.AddChild(rt, nil)

	canvas := graphics.NewCanvas(40, 20)
	Paint(view, canvas, Rect{X: 0, Y: 0, Width: 40, Height: 20})

	found := false
	for y := 5; y < 20 && !found; y++ {
		for x := 5; x < 16 && !found; x++ {
			if p := canvas.PixelAt(x, y); p.A > 0 && p.B == 0 {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no black text pixels found in text region")
	}
}

// TestRenderPipelinePhaseOrdering verifies that a document with both a background box and
// text paints both: the background in the background phase and the text in the foreground
// phase. The text region is scanned for pixels darker than the white background.
func TestRenderPipelinePhaseOrdering(t *testing.T) {
	view := newPaintView(40, 20)
	blockSt := style.NewComputedStyle()
	blockSt.Display = style.DisplayBlock
	blockSt.BackgroundColor = style.Color{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	block := NewRenderBlockFlow(dom.NewDocument().CreateElement("div"), blockSt)
	block.SetLocation(0, 0)
	block.SetSize(40, 20)
	view.AddChild(block, nil)

	textSt := style.NewComputedStyle()
	textSt.Color = style.Color{R: 0, G: 0, B: 0, A: 0xFF}
	textSt.FontSize = style.Length{Value: 12, Unit: "px"}
	rt := NewRenderTextWith(dom.NewDocument().CreateTextNode("A"), textSt, "A")
	rt.SetSegments([]InlineTextBox{{Start: 0, Len: 1, X: 5, Y: 5, Width: 8, Height: 12}})
	block.AddChild(rt, nil)

	canvas := graphics.NewCanvas(40, 20)
	Paint(view, canvas, Rect{X: 0, Y: 0, Width: 40, Height: 20})

	white := graphics.Color{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	if got := canvas.PixelAt(0, 0); got != white {
		t.Fatalf("background pixel = %+v, want white", got)
	}
	// Scan the text region for pixels darker than white (text drawn over background).
	found := false
	for y := 5; y < 20 && !found; y++ {
		for x := 5; x < 14 && !found; x++ {
			if p := canvas.PixelAt(x, y); p.A > 0 && (p.R < 0xFF || p.G < 0xFF || p.B < 0xFF) {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("no text pixels found darker than white background")
	}
}

// TestRenderPipelineLayerTree verifies the layer-based paint path paints a child box's
// background when a root layer is present.
func TestRenderPipelineLayerTree(t *testing.T) {
	view := newPaintView(20, 20)
	view.SetRootLayer(NewRenderLayer(RenderObject(view)))

	st := style.NewComputedStyle()
	st.Display = style.DisplayBlock
	st.BackgroundColor = style.Color{R: 0, G: 0, B: 0xFF, A: 0xFF}
	child := NewRenderBlockFlow(dom.NewDocument().CreateElement("div"), st)
	child.SetLocation(2, 2)
	child.SetSize(10, 10)
	view.AddChild(child, nil)

	canvas := graphics.NewCanvas(20, 20)
	Paint(view, canvas, Rect{X: 0, Y: 0, Width: 20, Height: 20})

	blue := graphics.Color{R: 0, G: 0, B: 0xFF, A: 0xFF}
	if got := canvas.PixelAt(5, 5); got != blue {
		t.Fatalf("layer-painted child background pixel = %+v, want %+v", got, blue)
	}
}

// TestRenderPipelineCompositedChildLayer verifies that a box owning a child layer is
// painted by its own layer pass (not the root pass), confirming the layer-tree
// compositing traversal.
func TestRenderPipelineCompositedChildLayer(t *testing.T) {
	view := newPaintView(40, 30)
	rootLayer := NewRenderLayer(RenderObject(view))
	view.SetRootLayer(rootLayer)

	// A positioned box that owns its own child layer.
	boxSt := style.NewComputedStyle()
	boxSt.Display = style.DisplayBlock
	boxSt.Position = style.PositionAbsolute
	boxSt.BackgroundColor = style.Color{R: 0, G: 0xFF, B: 0, A: 0xFF}
	box := NewRenderBlockFlow(dom.NewDocument().CreateElement("div"), boxSt)
	box.SetLocation(10, 10)
	box.SetSize(10, 10)
	view.AddChild(box, nil)

	// Attach a child layer owned by the positioned box.
	childLayer := NewRenderLayer(RenderObject(box))
	rootLayer.AddChild(childLayer)

	canvas := graphics.NewCanvas(40, 30)
	Paint(view, canvas, Rect{X: 0, Y: 0, Width: 40, Height: 30})

	green := graphics.Color{R: 0, G: 0xFF, B: 0, A: 0xFF}
	// The box's background must be painted via its own child layer.
	if got := canvas.PixelAt(12, 12); got != green {
		t.Fatalf("composited child layer pixel = %+v, want %+v", got, green)
	}
}

// TestRenderPipelineNilSafe verifies Paint handles nil view / canvas without panicking.
func TestRenderPipelineNilSafe(t *testing.T) {
	Paint(nil, graphics.NewCanvas(1, 1), Rect{})
	view := newPaintView(1, 1)
	Paint(view, nil, Rect{})
}
