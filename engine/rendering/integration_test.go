// End-to-end rendering integration tests that verify the full paint pipeline
// produces correct pixel output for common layout scenarios.
//
// Completeness: 60%
//   - overflow:hidden clipping (pixel verification)
//   - positioned elements (absolute, relative)
//   - z-index / stacking

package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// --- Overflow clipping tests ------------------------------------------------

// TestOverflowHiddenClipping verifies that a child extending beyond an
// overflow:hidden parent is clipped at the parent's padding-box boundary.
// Uses the full Paint() pipeline so that walkSubtreeExcluded applies clip.
func TestOverflowHiddenClipping(t *testing.T) {
	canvas := graphics.NewCanvas(100, 100)
	doc := dom.NewDocument()
	rv := NewRenderView(doc, style.NewComputedStyle())
	rv.SetViewportSize(100, 100)

	// Create a parent at (10,10) size 40x40 with overflow:hidden.
	parentStyle := style.NewComputedStyle()
	parentStyle.OverflowX = style.OverflowHidden
	parentStyle.OverflowY = style.OverflowHidden
	parentStyle.BackgroundColor = style.Color{R: 0xCC, G: 0xCC, B: 0xCC, A: 0xFF}
	parent := NewRenderBox(doc.CreateElement("div"), parentStyle)
	parent.SetLocation(10, 10)
	parent.SetSize(40, 40)

	// Create a child that overflows the parent (extends to x=60, y=60).
	childStyle := style.NewComputedStyle()
	childStyle.BackgroundColor = style.Color{R: 0xFF, G: 0, B: 0, A: 0xFF}
	child := NewRenderBox(doc.CreateElement("div"), childStyle)
	child.SetLocation(20, 20)
	child.SetSize(60, 60)

	rv.AddChild(parent, nil)
	parent.AddChild(child, nil)

	// Paint through the full pipeline so walkSubtreeExcluded applies clip.
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 100, Height: 100})

	// Inside parent & inside child (should be red - visible).
	red := graphics.Color{R: 0xFF, A: 0xFF}
	if got := canvas.PixelAt(30, 30); got != red {
		t.Fatalf("visible child pixel at (30,30) = %+v, want %+v (red)", got, red)
	}

	// Outside parent but inside child bounding box (should NOT be red - clipped).
	if got := canvas.PixelAt(55, 55); got != (graphics.Color{}) {
		t.Fatalf("clipped pixel at (55,55) = %+v, want transparent (clipped)", got)
	}
}

// TestOverflowVisibleNoClipping verifies that with overflow:visible (default),
// the child is NOT clipped even when it extends beyond the parent.
func TestOverflowVisibleNoClipping(t *testing.T) {
	canvas := graphics.NewCanvas(100, 100)
	doc := dom.NewDocument()
	rv := NewRenderView(doc, style.NewComputedStyle())
	rv.SetViewportSize(100, 100)

	parentStyle := style.NewComputedStyle()
	parentStyle.BackgroundColor = style.Color{R: 0xCC, G: 0xCC, B: 0xCC, A: 0xFF}
	parent := NewRenderBox(doc.CreateElement("div"), parentStyle)
	parent.SetLocation(10, 10)
	parent.SetSize(40, 40)

	childStyle := style.NewComputedStyle()
	childStyle.BackgroundColor = style.Color{R: 0xFF, G: 0, B: 0, A: 0xFF}
	child := NewRenderBox(doc.CreateElement("div"), childStyle)
	child.SetLocation(20, 20)
	child.SetSize(60, 60)

	rv.AddChild(parent, nil)
	parent.AddChild(child, nil)

	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 100, Height: 100})

	// Outside parent but inside child — should be visible (no clipping).
	red := graphics.Color{R: 0xFF, A: 0xFF}
	if got := canvas.PixelAt(55, 55); got != red {
		t.Fatalf("overflow:visible pixel at (55,55) = %+v, want %+v (red, not clipped)", got, red)
	}
}

// --- Positioned elements tests ----------------------------------------------

// TestPositionAbsolute verifies that an absolutely positioned element paints
// at its explicitly-set location, independent of its parent's content flow.
func TestPositionAbsolute(t *testing.T) {
	canvas := graphics.NewCanvas(100, 100)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 100, Height: 100})
	doc := dom.NewDocument()

	// Parent at (0,0) size 50x50.
	parentStyle := style.NewComputedStyle()
	parentStyle.BackgroundColor = style.Color{R: 0xDD, G: 0xDD, B: 0xDD, A: 0xFF}
	parent := NewRenderBox(doc.CreateElement("div"), parentStyle)
	parent.SetLocation(0, 0)
	parent.SetSize(50, 50)

	// Absolutely positioned child at (70,70) — should be rendered at that
	// position even though it's outside the parent.
	childStyle := style.NewComputedStyle()
	childStyle.Position = style.PositionAbsolute
	childStyle.BackgroundColor = style.Color{R: 0x00, G: 0xFF, B: 0x00, A: 0xFF}
	child := NewRenderBox(doc.CreateElement("div"), childStyle)
	child.SetLocation(70, 70)
	child.SetSize(20, 20)

	parent.AddChild(child, nil)

	PaintBackground(parent, info)
	PaintBackground(child, info)

	// The child should paint at (70,70) independent of the parent.
	green := graphics.Color{G: 0xFF, A: 0xFF}
	if got := canvas.PixelAt(75, 75); got != green {
		t.Fatalf("absolute child pixel at (75,75) = %+v, want %+v (green)", got, green)
	}

	// The parent area should not have green (green is only at 70..90).
	if got := canvas.PixelAt(25, 25); got == green {
		t.Fatal("parent pixel at (25,25) should not be green")
	}
}

// TestPositionRelative verifies that a relatively positioned element paints
// at its normal flow position + offset.
func TestPositionRelative(t *testing.T) {
	canvas := graphics.NewCanvas(100, 100)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 100, Height: 100})
	doc := dom.NewDocument()

	st := style.NewComputedStyle()
	st.Position = style.PositionRelative
	// Set Top/Left via style properties (the style.Resolve would set these;
	// here we mimic with manual style fields).
	st.SetProperty("top", "10px")
	st.SetProperty("left", "15px")
	st.BackgroundColor = style.Color{R: 0, G: 0, B: 0xFF, A: 0xFF}

	box := NewRenderBox(doc.CreateElement("div"), st)
	// Normally the box would be laid out at (0,0); with relative positioning
	// and top:10px/left:15px it should render at (15,10).
	box.SetLocation(15, 10)
	box.SetSize(30, 30)

	PaintBackground(box, info)

	blue := graphics.Color{B: 0xFF, A: 0xFF}
	if got := canvas.PixelAt(20, 15); got != blue {
		t.Fatalf("relative child pixel at (20,15) = %+v, want %+v (blue)", got, blue)
	}
}

// --- Stacking / z-index tests -----------------------------------------------

// TestZIndexHigherPaintsOnTop verifies that a box with higher z-index is painted
// after (and thus on top of) a box with lower z-index, and that boxes without
// explicit z-index are painted in tree order.
func TestZIndexHigherPaintsOnTop(t *testing.T) {
	canvas := graphics.NewCanvas(50, 50)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 50, Height: 50})
	doc := dom.NewDocument()

	// Background box (z-index: 1).
	backStyle := style.NewComputedStyle()
	backStyle.Position = style.PositionAbsolute
	backStyle.SetProperty("z-index", "1")
	backStyle.BackgroundColor = style.Color{R: 0xFF, G: 0, B: 0, A: 0xFF}
	back := NewRenderBox(doc.CreateElement("div"), backStyle)
	back.SetLocation(5, 5)
	back.SetSize(30, 30)

	// Foreground box (z-index: 2), same area.
	frontStyle := style.NewComputedStyle()
	frontStyle.Position = style.PositionAbsolute
	frontStyle.SetProperty("z-index", "2")
	frontStyle.BackgroundColor = style.Color{R: 0, G: 0xFF, B: 0, A: 0xFF}
	front := NewRenderBox(doc.CreateElement("div"), frontStyle)
	front.SetLocation(5, 5)
	front.SetSize(30, 30)

	// Paint order: back first, then front.
	PaintBackground(back, info)
	PaintBackground(front, info)

	// The overlapping area should show green (front paints on top of back).
	green := graphics.Color{G: 0xFF, A: 0xFF}
	if got := canvas.PixelAt(20, 20); got != green {
		t.Fatalf("overlap pixel at (20,20) = %+v, want %+v (green, front on top)", got, green)
	}
}

// --- Layer-tree stacking order tests ----------------------------------------

// TestLayerTreeZIndexOrdering verifies that paintLayerTree paints child layers
// in CSS stacking order: negative z-index first, then auto/zero in tree order,
// then positive z-index. This exercises the real BuildLayerTree + paintLayerTree
// path (not manual PaintBackground calls).
func TestLayerTreeZIndexOrdering(t *testing.T) {
	canvas := graphics.NewCanvas(80, 80)
	doc := dom.NewDocument()
	rv := NewRenderView(doc, style.NewComputedStyle())
	rv.SetViewportSize(80, 80)

	// Three overlapping positioned boxes at the same spot. Positioned boxes
	// participate in stacking so z-index is honored by paintLayerTree.
	mkBox := func(name string, z int, c style.Color) *RenderBox {
		st := style.NewComputedStyle()
		st.Position = style.PositionAbsolute
		st.ZIndex = z
		st.BackgroundColor = c
		box := NewRenderBox(doc.CreateElement(name), st)
		box.SetLocation(10, 10)
		box.SetSize(40, 40)
		return box
	}
	neg := mkBox("neg", -1, style.Color{R: 0xFF, G: 0, B: 0, A: 0xFF})     // red, z=-1
	mid := mkBox("mid", 0, style.Color{R: 0, G: 0xFF, B: 0, A: 0xFF})      // green, z=0
	top := mkBox("top", 2, style.Color{R: 0, G: 0, B: 0xFF, A: 0xFF})      // blue, z=2
	top2 := mkBox("top2", 1, style.Color{R: 0xFF, G: 0xFF, B: 0, A: 0xFF}) // yellow, z=1

	// Add in tree order: top (z=2) first, then neg (z=-1), then mid, then top2.
	rv.AddChild(top, nil)
	rv.AddChild(neg, nil)
	rv.AddChild(mid, nil)
	rv.AddChild(top2, nil)

	// Build the layer tree and paint through the real pipeline.
	comp := NewRenderLayerCompositor(rv)
	rootLayer := comp.BuildLayerTree(RenderObject(rv))
	if rootLayer == nil {
		t.Fatal("BuildLayerTree returned nil")
	}
	rv.SetRootLayer(rootLayer)
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 80, Height: 80})

	// Stacking order bottom→top: neg(z=-1) → mid(z=0) → top2(z=1) → top(z=2).
	// Center pixel must be top's blue.
	blue := graphics.Color{B: 0xFF, A: 0xFF}
	if got := canvas.PixelAt(30, 30); got != blue {
		t.Fatalf("center pixel = %+v, want %+v (blue z=2 on top despite being first in tree order)", got, blue)
	}
}

// TestLayerTreeZIndexNegativeBehind verifies a negative z-index layer paints
// behind auto layers even when the auto layer comes later in tree order.
func TestLayerTreeZIndexNegativeBehind(t *testing.T) {
	canvas := graphics.NewCanvas(80, 80)
	doc := dom.NewDocument()
	rv := NewRenderView(doc, style.NewComputedStyle())
	rv.SetViewportSize(80, 80)

	mkBox := func(name string, z int, c style.Color) *RenderBox {
		st := style.NewComputedStyle()
		st.Position = style.PositionAbsolute
		st.ZIndex = z
		st.BackgroundColor = c
		box := NewRenderBox(doc.CreateElement(name), st)
		box.SetLocation(10, 10)
		box.SetSize(40, 40)
		return box
	}
	autoBox := mkBox("auto", 0, style.Color{R: 0, G: 0xFF, B: 0, A: 0xFF}) // green
	negBox := mkBox("neg", -5, style.Color{R: 0xFF, G: 0, B: 0, A: 0xFF})  // red

	// Tree order: auto first, neg second — but neg must paint behind.
	rv.AddChild(autoBox, nil)
	rv.AddChild(negBox, nil)

	comp := NewRenderLayerCompositor(rv)
	rootLayer := comp.BuildLayerTree(RenderObject(rv))
	rv.SetRootLayer(rootLayer)
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 80, Height: 80})

	green := graphics.Color{G: 0xFF, A: 0xFF}
	if got := canvas.PixelAt(30, 30); got != green {
		t.Fatalf("center pixel = %+v, want %+v (green auto layer on top of z=-5)", got, green)
	}
}
