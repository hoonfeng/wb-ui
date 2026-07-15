// Translation of: Source/WebCore/page/FrameView.h
//                  Source/WebCore/page/LocalFrameView.h
//                  Source/WebCore/page/LocalFrameView.cpp
// Completeness: 45%
// Simplifications:
//   - layout delegates to RenderView.Layout; FrameView is a thin viewport holder
//   - no scrolling, no scrollbars, no paint, no composited-layer management beyond
//     what RenderView already owns
//   - no header/footer heights, no fixed-position containment, no coordinate
//     conversion between renderer and view space
//   - needsLayout is a plain boolean; there is no layout-scheduling / layout
//     phase state machine

package page

// FrameView is the Go translation of WebCore::LocalFrameView. It manages the
// viewport (width/height) for a Frame and is the entry point for triggering layout.
// In WebKit LocalFrameView subclasses ScrollView and owns the layout context, the
// render-layer backing store and a host of scrolling/painting machinery; this port
// keeps only the viewport size and a dirty flag, and delegates the actual layout
// pass to the frame's RenderView.
type FrameView struct {
	frame *Frame

	// width / height are the viewport dimensions in CSS pixels, mirroring
	// FrameView::visibleWidth() / visibleHeight() (the widget bounds).
	width  int
	height int

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

// Layout runs a layout pass for the frame's render tree, mirroring
// LocalFrameView::layout(). It propagates the viewport size to the RenderView and
// invokes RenderView.Layout with a fresh LayoutState. After a successful layout the
// needs-layout flag is cleared. It is a no-op when no render view is available.
func (v *FrameView) Layout() {
	if v.frame == nil || v.frame.renderView == nil {
		v.needsLayout = false
		return
	}
	rv := v.frame.renderView
	rv.SetViewportSize(float64(v.width), float64(v.height))
	rv.Layout(nil)
	v.needsLayout = false
}
