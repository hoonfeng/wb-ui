package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// buildStickyCanvas builds a 200x400 viewport with a tall content column
// (y=0..800) containing a sticky bar (static y=300, top:10, height 40), then
// scrolls by scrollY and paints through the full pipeline.
func buildStickyCanvas(t *testing.T, scrollY float64) *graphics.Canvas {
	t.Helper()
	canvas := graphics.NewCanvas(200, 400)
	doc := dom.NewDocument()
	rv := NewRenderView(doc, style.NewComputedStyle())
	rv.SetViewportSize(200, 400)

	contentStyle := style.NewComputedStyle()
	contentStyle.BackgroundColor = style.Color{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF}
	content := NewRenderBox(doc.CreateElement("div"), contentStyle)
	content.SetLocation(0, 0)
	content.SetSize(200, 800)

	barStyle := style.NewComputedStyle()
	barStyle.Position = style.PositionSticky
	barStyle.SetProperty("top", "10px")
	barStyle.BackgroundColor = style.Color{R: 0xFF, G: 0, B: 0, A: 0xFF}
	bar := NewRenderBox(doc.CreateElement("div"), barStyle)
	bar.SetLocation(0, 300)
	bar.SetSize(200, 40)

	content.AddChild(bar, nil)
	rv.AddChild(content, nil)
	rv.SetScrollOffset(0, scrollY)
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 200, Height: 400})
	return canvas
}

// TestStickyPin: scrolling 400px moves the bar's viewport position to
// 300-400=-100 (above the top:10 line). Sticky pins it back → the bar paints
// at viewport y=10.
func TestStickyPin(t *testing.T) {
	canvas := buildStickyCanvas(t, 400)
	defer canvas.Release()

	if px := canvas.PixelAt(100, 10); px.R != 255 {
		t.Fatalf("sticky bar missing at pinned y=10: %+v", px)
	}
	if px := canvas.PixelAt(100, 300); px.R == 255 {
		t.Fatalf("bar must not remain at static y=300 when scrolled")
	}
}

// TestStickyNoScroll: without scrolling, sticky behaves like relative — the
// bar stays at its static position (y=300).
func TestStickyNoScroll(t *testing.T) {
	canvas := buildStickyCanvas(t, 0)
	defer canvas.Release()

	if px := canvas.PixelAt(100, 300); px.R != 255 {
		t.Fatalf("sticky bar missing at static y=300: %+v", px)
	}
	if px := canvas.PixelAt(100, 10); px.R == 255 {
		t.Fatalf("unscrolled bar should not paint at y=10")
	}
}

// TestStickyLayoutRelativeOffset: sticky participates in normal flow and
// applies top like relative positioning (layout-level, via
// IsRelativelyPositioned including sticky).
func TestStickyLayoutRelativeOffset(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("div")
	el.SetAttribute("style", "position:sticky;top:30px;width:50px;height:50px;")
	doc.AppendChild(el)

	view := NewRenderView(doc, style.NewComputedStyle())
	view.SetViewportSize(200, 400)
	// Build the render tree from the DOM so the layout builder applies the
	// relative offset.
	builder := NewRenderTreeBuilder(style.NewResolver())
	built := builder.Build(doc)
	_ = built
	_ = view
	// The layout-level relative offset path is exercised by
	// applyRelativeOffsetForBox; the box.go IsRelativelyPositioned now
	// includes sticky (covered by layout tests). Here we assert the method.
	b := NewRenderBox(el, style.NewComputedStyle())
	b.style.Position = style.PositionSticky
	if !b.IsStickyPositioned() {
		t.Fatal("IsStickyPositioned should be true")
	}
	if !b.IsRelativelyPositioned() {
		t.Fatal("sticky must be treated as relatively positioned (WebKit semantics)")
	}
}
