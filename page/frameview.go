// Translation of: Source/WebCore/page/FrameView.h
//                  Source/WebCore/page/LocalFrameView.h
//                  Source/WebCore/page/LocalFrameView.cpp
// Completeness: 55%
// Simplifications:
//   - layout delegates to RenderView.Layout; FrameView is a thin viewport holder
//   - no scrollbars, no paint, no composited-layer management beyond
//     what RenderView already owns
//   - no header/footer heights, no fixed-position containment, no coordinate
//     conversion between renderer and view space
//   - needsLayout is a plain boolean; there is no layout-scheduling / layout
//     phase state machine

package page

import (
	"wb-ui/rendering"
)

// FrameView is the Go translation of WebCore::LocalFrameView. It manages the
// viewport (width/height) for a Frame and is the entry point for triggering layout.
// In WebKit LocalFrameView subclasses ScrollView and owns the layout context, the
// render-layer backing store and a host of scrolling/painting machinery; this port
// adds scroll offset and content size tracking, delegating the actual layout pass
// to the frame's RenderView.
type FrameView struct {
	frame *Frame

	// width / height are the viewport dimensions in CSS pixels, mirroring
	// FrameView::visibleWidth() / visibleHeight() (the widget bounds).
	width  int
	height int

	// scrollX / scrollY are the current scroll offset in CSS pixels.
	// Positive values mean content is scrolled up/left (content moves
	// opposite to the scroll direction), mirroring window.scrollX/Y.
	scrollX int
	scrollY int

	// contentWidth / contentHeight record the total laid-out content size
	// in CSS pixels, set after each layout pass. Used to clamp scroll offsets.
	contentWidth  int
	contentHeight int

	// needsLayout records whether a layout pass is pending, mirroring
	// FrameView::needsLayout(). Resizing the view or loading a new document marks
	// it true; a successful Layout() clears it.
	needsLayout bool
}

// NewFrameView constructs a FrameView for the given frame with the supplied
// viewport dimensions, mirroring LocalFrameView::create(frame, initialSize).
func NewFrameView(frame *Frame, width, height int) *FrameView {
	return &FrameView{
		frame:       frame,
		width:       width,
		height:      height,
		needsLayout: true,
	}
}

// Frame returns the owning frame, mirroring FrameView::frame().
func (v *FrameView) Frame() *Frame { return v.frame }

// Width returns the viewport width in CSS pixels, mirroring
// FrameView::visibleWidth().
func (v *FrameView) Width() int { return v.width }

// Height returns the viewport height in CSS pixels, mirroring
// FrameView::visibleHeight().
func (v *FrameView) Height() int { return v.height }

// SetWidth sets the viewport width and marks the view as needing layout when the
// value changes, mirroring FrameView's layout-invalidation on resize.
func (v *FrameView) SetWidth(w int) {
	if v.width != w {
		v.width = w
		v.needsLayout = true
	}
}

// SetHeight sets the viewport height and marks the view as needing layout when the
// value changes, mirroring FrameView's layout-invalidation on resize.
func (v *FrameView) SetHeight(h int) {
	if v.height != h {
		v.height = h
		v.needsLayout = true
	}
}

// SetSize sets both dimensions at once and marks the view as needing layout when
// either changes, mirroring FrameView::setFrameRect() / resize logic.
func (v *FrameView) SetSize(w, h int) {
	v.SetWidth(w)
	v.SetHeight(h)
}

// NeedsLayout reports whether a layout pass is pending, mirroring
// FrameView::needsLayout().
func (v *FrameView) NeedsLayout() bool { return v.needsLayout }

// SetNeedsLayout forces the needs-layout flag, mirroring
// FrameView::setNeedsLayout().
func (v *FrameView) SetNeedsLayout(needs bool) { v.needsLayout = needs }

// --- Scroll offset ----------------------------------------------------------

// ScrollX returns the current horizontal scroll offset in CSS pixels.
func (v *FrameView) ScrollX() int { return v.scrollX }

// ScrollY returns the current vertical scroll offset in CSS pixels.
func (v *FrameView) ScrollY() int { return v.scrollY }

// SetScrollOffset sets both scroll offsets, clamping them to the valid range
// [0, maxScroll] so the content never reveals a gap beyond the last laid-out pixel.
func (v *FrameView) SetScrollOffset(x, y int) {
	v.scrollX = clamp(x, 0, v.MaxScrollX())
	v.scrollY = clamp(y, 0, v.MaxScrollY())
}

// ScrollBy adds (dx, dy) to the current scroll offset, clamping to the valid range.
func (v *FrameView) ScrollBy(dx, dy int) {
	v.SetScrollOffset(v.scrollX+dx, v.scrollY+dy)
}

// MaxScrollX returns the maximum horizontal scroll offset in CSS pixels.
// The content can scroll horizontally only when contentWidth exceeds viewport width.
func (v *FrameView) MaxScrollX() int {
	m := v.contentWidth - v.width
	if m < 0 {
		return 0
	}
	return m
}

// MaxScrollY returns the maximum vertical scroll offset in CSS pixels.
func (v *FrameView) MaxScrollY() int {
	m := v.contentHeight - v.height
	if m < 0 {
		return 0
	}
	return m
}

// IsScrollable returns true when the content is larger than the viewport in either
// dimension, indicating that scrollbars would be shown in a real browser.
func (v *FrameView) IsScrollable() bool {
	return v.MaxScrollX() > 0 || v.MaxScrollY() > 0
}

// --- Content size -----------------------------------------------------------

// ContentWidth returns the total laid-out content width in CSS pixels.
func (v *FrameView) ContentWidth() int { return v.contentWidth }

// ContentHeight returns the total laid-out content height in CSS pixels.
func (v *FrameView) ContentHeight() int { return v.contentHeight }

// SetContentSize records the total laid-out content dimensions. This is called
// automatically by Layout() after each layout pass. The caller can also call it
// directly if the content bounds are determined externally.
func (v *FrameView) SetContentSize(w, h int) {
	v.contentWidth = w
	v.contentHeight = h
	// Clamp scroll offsets to the new content size.
	v.SetScrollOffset(v.scrollX, v.scrollY)
}

// --- Layout -----------------------------------------------------------------

// Layout runs a layout pass for the frame's render tree, mirroring
// LocalFrameView::layout(). It propagates the viewport size to the RenderView and
// invokes RenderView.Layout with a fresh LayoutState. After a successful layout the
// needs-layout flag is cleared and the content size is recomputed from the render
// tree. It is a no-op when no render view is available.
func (v *FrameView) Layout() {
	if v.frame == nil || v.frame.renderView == nil {
		v.needsLayout = false
		return
	}
	rv := v.frame.renderView
	rv.SetViewportSize(float64(v.width), float64(v.height))
	rv.Layout(nil)
	// Recompute content size from the laid-out render tree.
	v.updateContentSize(rv)
	v.needsLayout = false
}

// updateContentSize walks the render tree to find the maximum extent of all
// render boxes, then records it as the content size. This is the simplest
// approach that works with any render tree structure.
func (v *FrameView) updateContentSize(rv *rendering.RenderView) {
	if rv == nil {
		return
	}
	maxX, maxY := 0, 0
	var walk func(o rendering.RenderObject)
	walk = func(o rendering.RenderObject) {
		if o == nil {
			return
		}
		if box := asRenderBox(o); box != nil {
			right := int(box.AbsoluteX() + box.Width())
			bottom := int(box.AbsoluteY() + box.Height())
			if right > maxX {
				maxX = right
			}
			if bottom > maxY {
				maxY = bottom
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(rendering.RenderObject(rv))
	v.contentWidth = maxX
	v.contentHeight = maxY
	// Clamp scroll offsets so they don't exceed the new content bounds.
	v.SetScrollOffset(v.scrollX, v.scrollY)
}

// asRenderBox attempts to cast a RenderObject to *rendering.RenderBox.
// Returns nil if the object doesn't carry a box (e.g. RenderText).
func asRenderBox(o rendering.RenderObject) *rendering.RenderBox {
	if o == nil {
		return nil
	}
	box, ok := o.(*rendering.RenderBox)
	if ok {
		return box
	}
	// Some render objects embed RenderBox; check interface.
	if b, ok := o.(interface{ AsRenderBox() *rendering.RenderBox }); ok {
		return b.AsRenderBox()
	}
	return nil
}

// clamp returns x constrained to [lo, hi].
func clamp(x, lo, hi int) int {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}
