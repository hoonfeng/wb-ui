// Translation of: tests for Source/WebCore/rendering/RenderLayer.cpp
//                  Source/WebCore/rendering/RenderLayerCompositor.cpp
// Completeness: 50%
// Simplifications:
//   - tests verify layer tree structure, requires-layer conditions and compositing
//     backing assignment; clip-rect computation is smoke-tested only

package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/layout"
	"wb-ui/engine/style"
)

// TestRequiresLayerConditions verifies the style conditions that force a layer.
func TestRequiresLayerConditions(t *testing.T) {
	doc := dom.NewDocument()

	// Static position, visible overflow, opacity 1 => no layer.
	plain := NewRenderBox(doc.CreateElement("div"), style.NewComputedStyle())
	if RequiresLayer(plain) {
		t.Error("plain box should not require a layer")
	}

	// opacity < 1 => layer.
	opacity := style.NewComputedStyle()
	opacity.Opacity = 0.5
	obj := NewRenderBox(doc.CreateElement("div"), opacity)
	if !RequiresLayer(obj) {
		t.Error("opacity<1 should require a layer")
	}

	// position: relative => layer.
	posStyle := style.NewComputedStyle()
	posStyle.Position = style.PositionRelative
	posObj := NewRenderBox(doc.CreateElement("div"), posStyle)
	if !RequiresLayer(posObj) {
		t.Error("position:relative should require a layer")
	}

	// overflow: hidden => layer.
	overflowStyle := style.NewComputedStyle()
	overflowStyle.OverflowX = style.OverflowHidden
	overflowObj := NewRenderBox(doc.CreateElement("div"), overflowStyle)
	if !RequiresLayer(overflowObj) {
		t.Error("overflow:hidden should require a layer")
	}

	// transform => layer.
	transformStyle := style.NewComputedStyle()
	transformStyle.Transform = "rotate(10deg)"
	transformObj := NewRenderBox(doc.CreateElement("div"), transformStyle)
	if !RequiresLayer(transformObj) {
		t.Error("transform should require a layer")
	}

	// filter => layer.
	filterStyle := style.NewComputedStyle()
	filterStyle.Filter = "blur(5px)"
	filterObj := NewRenderBox(doc.CreateElement("div"), filterStyle)
	if !RequiresLayer(filterObj) {
		t.Error("filter should require a layer")
	}
}

// TestRenderLayerTreeStructure verifies AddChild / RemoveChild on the layer tree.
func TestRenderLayerTreeStructure(t *testing.T) {
	doc := dom.NewDocument()
	root := NewRenderLayer(NewRenderView(doc, style.NewComputedStyle()))
	child1 := NewRenderLayer(NewRenderBox(doc.CreateElement("a"), style.NewComputedStyle()))
	child2 := NewRenderLayer(NewRenderBox(doc.CreateElement("b"), style.NewComputedStyle()))

	root.AddChild(child1)
	root.AddChild(child2)

	if root.FirstChild() != child1 || root.LastChild() != child2 {
		t.Fatal("layer children order incorrect")
	}
	if child1.NextSibling() != child2 || child2.PreviousSibling() != child1 {
		t.Fatal("layer sibling links incorrect")
	}

	root.RemoveChild(child1)
	if root.FirstChild() != child2 {
		t.Fatal("after removing child1, FirstChild should be child2")
	}
	if child1.Parent() != nil {
		t.Fatal("removed layer should have nil parent")
	}
}

// TestLayerCompositorBacking verifies that the compositor creates backings for layers
// that need compositing and clears backings for those that do not.
func TestLayerCompositorBacking(t *testing.T) {
	doc := dom.NewDocument()
	view := NewRenderView(doc, style.NewComputedStyle())

	// Create a child that needs compositing (opacity < 1).
	opacityStyle := style.NewComputedStyle()
	opacityStyle.Opacity = 0.5
	composited := NewRenderBox(doc.CreateElement("div"), opacityStyle)
	view.AddChild(composited, nil)

	// Create a child that does not need compositing.
	plain := NewRenderBox(doc.CreateElement("div"), style.NewComputedStyle())
	view.AddChild(plain, nil)

	compositor := NewRenderLayerCompositor(view)
	rootLayer := compositor.BuildLayerTree(view)
	if rootLayer == nil {
		t.Fatal("BuildLayerTree returned nil")
	}

	compositor.UpdateCompositingLayers()
	if compositor.CompositedCount() < 1 {
		t.Fatalf("CompositedCount = %d, want >= 1", compositor.CompositedCount())
	}
}

// TestLayerBackingGeometry verifies that UpdateGeometry syncs the backing's graphics
// layer to the owner's border box.
func TestLayerBackingGeometry(t *testing.T) {
	doc := dom.NewDocument()
	box := NewRenderBox(doc.CreateElement("div"), style.NewComputedStyle())
	box.SetLocation(10, 20)
	box.SetSize(100, 200)

	layer := NewRenderLayer(box)
	backing := layer.EnsureBacking()
	backing.UpdateGeometry()

	gl := backing.GraphicsLayer()
	if gl.X() != 10 || gl.Y() != 20 {
		t.Fatalf("graphics layer position = (%v,%v), want (10,20)", gl.X(), gl.Y())
	}
	if gl.Width() != 100 || gl.Height() != 200 {
		t.Fatalf("graphics layer size = (%v,%v), want (100,200)", gl.Width(), gl.Height())
	}
}

// TestCalculateRects verifies that CalculateRects returns the owner's border box as the
// layer rect.
func TestCalculateRects(t *testing.T) {
	doc := dom.NewDocument()
	box := NewRenderBox(doc.CreateElement("div"), style.NewComputedStyle())
	box.SetLocation(5, 10)
	box.SetSize(50, 60)

	layer := NewRenderLayer(box)
	layerRect, clipRect, _ := layer.CalculateRects()
	if layerRect.X != 5 || layerRect.Y != 10 || layerRect.Width != 50 || layerRect.Height != 60 {
		t.Fatalf("layerRect = %+v, want {5,10,50,60}", layerRect)
	}
	// With no overflow ancestor, clip rect is empty: an overflow:visible
	// layer must NOT clip its subtree (box-shadow, negative margins,
	// absolutely positioned children can overflow the border box).
	if clipRect.Width != 0 || clipRect.Height != 0 {
		t.Fatalf("clipRect = %+v, want zero (overflow:visible → no clip)", clipRect)
	}
	// With overflow:hidden on the layer itself, clip equals the layer rect.
	overflowStyle := style.NewComputedStyle()
	overflowStyle.OverflowX = style.OverflowHidden
	overflowStyle.OverflowY = style.OverflowHidden
	box2 := NewRenderBox(doc.CreateElement("div"), overflowStyle)
	box2.SetLocation(5, 10)
	box2.SetSize(50, 60)
	layer2 := NewRenderLayer(box2)
	_, clipRect2, _ := layer2.CalculateRects()
	if clipRect2 != layerRect {
		t.Fatalf("clipRect(overflow:hidden) = %+v, want %+v", clipRect2, layerRect)
	}
}

// TestLayerWithOverflowClip verifies that an overflow ancestor clips the layer rect.
func TestLayerWithOverflowClip(t *testing.T) {
	doc := dom.NewDocument()
	// Parent with overflow:hidden.
	parentStyle := style.NewComputedStyle()
	parentStyle.OverflowX = style.OverflowHidden
	parentStyle.OverflowY = style.OverflowHidden
	parent := NewRenderBox(doc.CreateElement("div"), parentStyle)
	parent.SetLocation(0, 0)
	parent.SetSize(40, 40)

	// Child layer that extends beyond the parent.
	childStyle := style.NewComputedStyle()
	childStyle.Position = style.PositionRelative
	child := NewRenderBox(doc.CreateElement("div"), childStyle)
	child.SetLocation(0, 0)
	child.SetSize(100, 100)

	parentLayer := NewRenderLayer(parent)
	childLayer := NewRenderLayer(child)
	parentLayer.AddChild(childLayer)

	_, clipRect, _ := childLayer.CalculateRects()
	// The clip should be intersected with the parent's padding box (40x40).
	if clipRect.Width > 40 {
		t.Fatalf("clipRect.Width = %v, want <= 40 (clipped by overflow)", clipRect.Width)
	}
}

// TestFixedAncestorStopsClipChain verifies that an overflow clip of an ancestor
// OUTSIDE a fixed-position layer does not reach a descendant layer. A fixed
// element establishes a viewport containing block (it left the normal flow), so
// the browser never clips it by the overflow of a scroll container it happens to
// be nested in. Regression test for the dialog "create" button that became its
// own layer (opacity<1 => RequiresLayer) and painted via the normal layer
// branch, where CalculateRects wrongly re-applied the sidebar-content overflow
// clip, culling the whole button.
func TestFixedAncestorStopsClipChain(t *testing.T) {
	doc := dom.NewDocument()

	// Scroll container (sidebar-content): overflow auto, small panel.
	scrollStyle := style.NewComputedStyle()
	scrollStyle.OverflowX = style.OverflowAuto
	scrollStyle.OverflowY = style.OverflowAuto
	scrollBox := NewRenderBox(doc.CreateElement("div"), scrollStyle)
	scrollBox.SetLocation(0, 0)
	scrollBox.SetSize(40, 40)

	// Fixed overlay nested inside the scroll container.
	fixedStyle := style.NewComputedStyle()
	fixedStyle.Position = style.PositionFixed
	fixedBox := NewRenderBox(doc.CreateElement("div"), fixedStyle)
	fixedBox.SetLocation(0, 0)
	fixedBox.SetSize(300, 200)

	// Opacity<1 child inside the fixed overlay (its own layer).
	btnStyle := style.NewComputedStyle()
	btnStyle.Opacity = 0.5
	btnBox := NewRenderBox(doc.CreateElement("button"), btnStyle)
	btnBox.SetLocation(100, 80)
	btnBox.SetSize(60, 20)

	scrollLayer := NewRenderLayer(scrollBox)
	fixedLayer := NewRenderLayer(fixedBox)
	btnLayer := NewRenderLayer(btnBox)
	scrollLayer.AddChild(fixedLayer)
	fixedLayer.AddChild(btnLayer)

	_, clipRect, _ := btnLayer.CalculateRects()
	// The fixed ancestor stops the chain: the scroll container's 40x40
	// overflow clip must NOT be applied to the fixed overlay's child.
	if clipRect.Width != 0 || clipRect.Height != 0 {
		t.Fatalf("clipRect = %+v, want zero (fixed ancestor stops outer overflow clip)", clipRect)
	}
}

// TestFixedAncestorOwnClipStillApplies verifies that a fixed ancestor's OWN
// overflow still clips its subtree (the chain only drops clips OUTSIDE it).
func TestFixedAncestorOwnClipStillApplies(t *testing.T) {
	doc := dom.NewDocument()

	// Fixed dialog with overflow:hidden (rounded-corner clipping etc).
	fixedStyle := style.NewComputedStyle()
	fixedStyle.Position = style.PositionFixed
	fixedStyle.OverflowX = style.OverflowHidden
	fixedStyle.OverflowY = style.OverflowHidden
	fixedBox := NewRenderBox(doc.CreateElement("div"), fixedStyle)
	fixedBox.SetLocation(0, 0)
	fixedBox.SetSize(300, 200)

	// Opacity<1 child.
	btnStyle := style.NewComputedStyle()
	btnStyle.Opacity = 0.5
	btnBox := NewRenderBox(doc.CreateElement("button"), btnStyle)
	btnBox.SetLocation(10, 10)
	btnBox.SetSize(60, 20)

	fixedLayer := NewRenderLayer(fixedBox)
	btnLayer := NewRenderLayer(btnBox)
	fixedLayer.AddChild(btnLayer)

	_, clipRect, _ := btnLayer.CalculateRects()
	if clipRect.Width != 300 || clipRect.Height != 200 {
		t.Fatalf("clipRect = %+v, want the fixed ancestor's own padding box (300x200)", clipRect)
	}
}

// TestGraphicsLayerProperties verifies the placeholder GraphicsLayer property setters.
func TestGraphicsLayerProperties(t *testing.T) {
	gl := NewGraphicsLayer("test")
	gl.SetPosition(1, 2)
	gl.SetSize(3, 4)
	gl.SetOpacity(0.7)
	gl.SetVisible(false)

	if gl.Name() != "test" {
		t.Fatalf("Name = %q, want test", gl.Name())
	}
	if gl.X() != 1 || gl.Y() != 2 {
		t.Fatalf("position = (%v,%v), want (1,2)", gl.X(), gl.Y())
	}
	if gl.Width() != 3 || gl.Height() != 4 {
		t.Fatalf("size = (%v,%v), want (3,4)", gl.Width(), gl.Height())
	}
	if gl.Opacity() != 0.7 {
		t.Fatalf("opacity = %v, want 0.7", gl.Opacity())
	}
	if gl.IsVisible() {
		t.Fatal("expected invisible")
	}
}

// TestCalculateRectsScrollDeviceCoords verifies that CalculateRects returns
// the clip in DEVICE coordinates once a scroll container has scrolled.
// Scrolling is implemented as a canvas translate (paintLayerContents) applied
// AFTER CalculateRects — paintLayerTree re-adds the accumulated
// scrollTranslate and Clip() maps back to the screen-fixed viewport — so:
//
//   - the layer's own border box must be shifted by the sum of ALL scroll
//     ancestors' offsets (its content moved up inside the fixed viewport);
//   - each scroll ancestor's padding box (its viewport) stays FIXED in device
//     space: it must be shifted only by the scroll of ancestors OUTSIDE it.
//
// Without this correction a layer initially BELOW a scroll container's
// viewport (content coords) intersects the un-shifted padding box to zero and
// is never painted after being scrolled into view — the conv-title rows that
// start below the conv-list fold never appear when the user scrolls up, and
// rows that started inside the viewport keep a clip pinned to the OLD
// viewport bottom and get wrongly culled once scrolled away.
func TestCalculateRectsScrollDeviceCoords(t *testing.T) {
	doc := dom.NewDocument()
	view := NewRenderView(doc, style.NewComputedStyle())

	// conv-list: overflow auto, viewport (1031,98) 249x304, scrolled 300 down.
	scrollStyle := style.NewComputedStyle()
	scrollStyle.OverflowX = style.OverflowAuto
	scrollStyle.OverflowY = style.OverflowAuto
	scrollBox := NewRenderBox(doc.CreateElement("div"), scrollStyle)
	scrollBox.SetLocation(1031, 98)
	scrollBox.SetSize(249, 304)
	view.AddChild(scrollBox, nil)
	view.SetBoxScrollOffset(scrollBox, 0, 300)

	// conv-item #10: overflow:visible plain box at y=278 (content coords).
	item := NewRenderBox(doc.CreateElement("div"), style.NewComputedStyle())
	item.SetLocation(1031, 278)
	item.SetSize(249, 34)
	scrollBox.AddChild(item, nil)

	// conv-title #10: its own layer (position:relative) at content y=556 —
	// initially BELOW the 98..402 viewport, so it is only visible after
	// scrolling 300 (device y = 556-300 = 256, inside the viewport). Like
	// the real conv-title it has overflow:hidden (single-line ellipsis),
	// so its own device border box is the clip start and the intersection
	// with the ancestor viewport yields the visible device rect.
	titleStyle := style.NewComputedStyle()
	titleStyle.Position = style.PositionRelative
	titleStyle.OverflowX = style.OverflowHidden
	titleStyle.OverflowY = style.OverflowHidden
	title := NewRenderBox(doc.CreateElement("div"), titleStyle)
	title.SetLocation(1047, 556)
	title.SetSize(135, 17)
	item.AddChild(title, nil)

	scrollLayer := NewRenderLayer(scrollBox)
	itemLayer := NewRenderLayer(item)
	titleLayer := NewRenderLayer(title)
	scrollLayer.AddChild(itemLayer)
	itemLayer.AddChild(titleLayer)

	_, clip, _ := titleLayer.CalculateRects()
	// Device y = 556 - 300 = 256: the title is inside the 98..402 viewport
	// and must paint. Old behavior: content-coords rect (y=556..573) vs
	// un-shifted padding box (98..402) → zero → culled → never drawn after
	// scrolling it into view.
	if clip.Width == 0 || clip.Height == 0 {
		t.Fatalf("scrolled-into-view title clip = %+v, want non-zero (device y 256..273 inside viewport 98..402)", clip)
	}
	if clip.Y != 256 {
		t.Fatalf("clip.Y = %v, want 256 (device coords = content 556 - scroll 300)", clip.Y)
	}
	if clip.Width != 135 {
		t.Fatalf("clip.Width = %v, want 135 (fully inside viewport horizontally)", clip.Width)
	}

	// A title even further down (content y=746 → device y=446, BELOW the
	// 402 viewport bottom) must stay culled.
	title2 := NewRenderBox(doc.CreateElement("div"), titleStyle)
	title2.SetLocation(1047, 746)
	title2.SetSize(135, 17)
	item.AddChild(title2, nil)
	titleLayer2 := NewRenderLayer(title2)
	itemLayer.AddChild(titleLayer2)
	_, clip2, _ := titleLayer2.CalculateRects()
	if clip2.Width != 0 || clip2.Height != 0 {
		t.Fatalf("still-below-viewport title clip = %+v, want zero (device y 446..463 > 402)", clip2)
	}

	// A title ABOVE the viewport after scroll (content y=110 → device
	// y=-190) must also be culled.
	title3 := NewRenderBox(doc.CreateElement("div"), titleStyle)
	title3.SetLocation(1047, 110)
	title3.SetSize(135, 17)
	item.AddChild(title3, nil)
	titleLayer3 := NewRenderLayer(title3)
	itemLayer.AddChild(titleLayer3)
	_, clip3, _ := titleLayer3.CalculateRects()
	if clip3.Width != 0 || clip3.Height != 0 {
		t.Fatalf("scrolled-above-viewport title clip = %+v, want zero (device y -190..-173)", clip3)
	}
}

// TestCalculateRectsNestedScroll verifies that a scroll container nested
// inside another scroll container clips with its viewport in the correct
// device position: the inner viewport is fixed in device space, shifted only
// by the scroll of the OUTER container (not by its own scroll).
func TestCalculateRectsNestedScroll(t *testing.T) {
	doc := dom.NewDocument()
	view := NewRenderView(doc, style.NewComputedStyle())

	// Outer scroll container A: overflow auto, viewport (10,10) 200x300,
	// scrolled 100 down.
	aStyle := style.NewComputedStyle()
	aStyle.OverflowX = style.OverflowAuto
	aStyle.OverflowY = style.OverflowAuto
	aBox := NewRenderBox(doc.CreateElement("div"), aStyle)
	aBox.SetLocation(10, 10)
	aBox.SetSize(200, 300)
	view.AddChild(aBox, nil)
	view.SetBoxScrollOffset(aBox, 0, 100)

	// Inner scroll container B: overflow auto, viewport (50,50) 100x200,
	// scrolled 50 down. Device viewport y = 50 - 100 (outer scroll) = -50.
	bStyle := style.NewComputedStyle()
	bStyle.OverflowX = style.OverflowAuto
	bStyle.OverflowY = style.OverflowAuto
	bBox := NewRenderBox(doc.CreateElement("div"), bStyle)
	bBox.SetLocation(50, 50)
	bBox.SetSize(100, 200)
	aBox.AddChild(bBox, nil)
	view.SetBoxScrollOffset(bBox, 0, 50)

	// Layer C at content (60,150): device y = 150 - 100 - 50 = 0. It is
	// inside B's device viewport (-50..150) and A's device viewport
	// (10..310) only for y >= 10, so the clip is (60,10) 20x10.
	cStyle := style.NewComputedStyle()
	cStyle.Position = style.PositionRelative
	cStyle.OverflowX = style.OverflowHidden
	cStyle.OverflowY = style.OverflowHidden
	cBox := NewRenderBox(doc.CreateElement("div"), cStyle)
	cBox.SetLocation(60, 150)
	cBox.SetSize(20, 20)
	bBox.AddChild(cBox, nil)

	aLayer := NewRenderLayer(aBox)
	bLayer := NewRenderLayer(bBox)
	cLayer := NewRenderLayer(cBox)
	aLayer.AddChild(bLayer)
	bLayer.AddChild(cLayer)

	_, clip, _ := cLayer.CalculateRects()
	if clip.Width == 0 || clip.Height == 0 {
		t.Fatalf("nested-scroll layer clip = %+v, want non-zero", clip)
	}
	if clip.Y != 10 {
		t.Fatalf("clip.Y = %v, want 10 (outer A viewport top in device coords)", clip.Y)
	}
	if clip.Height != 10 {
		t.Fatalf("clip.Height = %v, want 10 (device y 0..20 ∩ A viewport 10..310)", clip.Height)
	}
}

// TestLayoutRectIntersection verifies the rect intersection helper used by CalculateRects.
func TestLayoutRectIntersection(t *testing.T) {
	a := layout.LayoutRect{X: 0, Y: 0, Width: 100, Height: 100}
	b := layout.LayoutRect{X: 50, Y: 50, Width: 100, Height: 100}
	r := intersectRects(a, b)
	if r.X != 50 || r.Y != 50 || r.Width != 50 || r.Height != 50 {
		t.Fatalf("intersection = %+v, want {50,50,50,50}", r)
	}

	// Non-overlapping.
	c := layout.LayoutRect{X: 0, Y: 0, Width: 10, Height: 10}
	d := layout.LayoutRect{X: 20, Y: 20, Width: 10, Height: 10}
	r2 := intersectRects(c, d)
	if r2.Width != 0 || r2.Height != 0 {
		t.Fatalf("non-overlapping intersection = %+v, want zero", r2)
	}
}
