// Translation of: Source/WebCore/page/FrameView.h
//                  Source/WebCore/page/LocalFrameView.h
//                  Source/WebCore/page/LocalFrameView.cpp
// Completeness: 85%
// Simplifications:
//   - layout delegates to RenderView.Layout; FrameView is a thin viewport holder
//   - no scrollbars, no paint, no composited-layer management beyond
//     what RenderView already owns
//   - no header/footer heights, no fixed-position containment, no coordinate
//     conversion between renderer and view space
//   - layout phase tracked via LayoutPhase enum (none/needs) replacing
//     a plain boolean needsLayout flag

package page

import (
	"log"

	"wb-ui/rendering"
)

// LayoutPhase tracks the current state of the layout scheduler.
type LayoutPhase int

const (
	LayoutPhaseNone        LayoutPhase = iota
	LayoutPhaseNeedsLayout
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
	scrollX int
	scrollY int

	// contentWidth / contentHeight record the total laid-out content size.
	contentWidth  int
	contentHeight int

	// layoutPhase tracks the current layout scheduling state.
	layoutPhase LayoutPhase
}

// NewFrameView constructs a FrameView for the given frame.
func NewFrameView(frame *Frame, width, height int) *FrameView {
	return &FrameView{
		frame:       frame,
		width:       width,
		height:      height,
		layoutPhase: LayoutPhaseNeedsLayout,
	}
}

// Frame returns the owning frame.
func (v *FrameView) Frame() *Frame { return v.frame }

// Width returns the viewport width in CSS pixels.
func (v *FrameView) Width() int { return v.width }

// Height returns the viewport height in CSS pixels.
func (v *FrameView) Height() int { return v.height }

// SetWidth sets the viewport width, marking layout as needed if changed.
func (v *FrameView) SetWidth(w int) {
	if v.width != w {
		v.width = w
		if v.layoutPhase != LayoutPhaseNeedsLayout {
			v.layoutPhase = LayoutPhaseNeedsLayout
		}
	}
}

// SetHeight sets the viewport height, marking layout as needed if changed.
func (v *FrameView) SetHeight(h int) {
	if v.height != h {
		v.height = h
		if v.layoutPhase != LayoutPhaseNeedsLayout {
			v.layoutPhase = LayoutPhaseNeedsLayout
		}
	}
}

// SetSize sets both dimensions, marking layout as needed if either changed.
func (v *FrameView) SetSize(w, h int) {
	changed := w != v.width || h != v.height
	if w != v.width {
		v.width = w
	}
	if h != v.height {
		v.height = h
	}
	if changed && v.layoutPhase != LayoutPhaseNeedsLayout {
		v.layoutPhase = LayoutPhaseNeedsLayout
	}
}

// NeedsLayout reports whether a layout pass is pending.
func (v *FrameView) NeedsLayout() bool { return v.layoutPhase == LayoutPhaseNeedsLayout }

// SetNeedsLayout forces the needs-layout flag.
func (v *FrameView) SetNeedsLayout(needs bool) {
	if needs {
		if v.layoutPhase != LayoutPhaseNeedsLayout {
			v.layoutPhase = LayoutPhaseNeedsLayout
		}
	} else {
		v.layoutPhase = LayoutPhaseNone
	}
}

// ScheduleLayout requests an asynchronous layout pass (marks layout as needed).
func (v *FrameView) ScheduleLayout() {
	if v.layoutPhase == LayoutPhaseNone {
		v.layoutPhase = LayoutPhaseNeedsLayout
	}
}

// LayoutPhase returns the current layout scheduling phase.
func (v *FrameView) LayoutPhase() LayoutPhase { return v.layoutPhase }

// --- Scroll offset ----------------------------------------------------------

// ScrollX returns the current horizontal scroll offset.
func (v *FrameView) ScrollX() int { return v.scrollX }

// ScrollY returns the current vertical scroll offset.
func (v *FrameView) ScrollY() int { return v.scrollY }

// SetScrollOffset sets both scroll offsets, clamping to valid range.
func (v *FrameView) SetScrollOffset(x, y int) {
	v.scrollX = clamp(x, 0, v.MaxScrollX())
	v.scrollY = clamp(y, 0, v.MaxScrollY())
}

// ScrollBy adds (dx, dy) to the current scroll offset.
func (v *FrameView) ScrollBy(dx, dy int) {
	v.SetScrollOffset(v.scrollX+dx, v.scrollY+dy)
}

// MaxScrollX returns the maximum horizontal scroll offset.
func (v *FrameView) MaxScrollX() int {
	m := v.contentWidth - v.width
	if m < 0 {
		return 0
	}
	return m
}

// MaxScrollY returns the maximum vertical scroll offset.
func (v *FrameView) MaxScrollY() int {
	m := v.contentHeight - v.height
	if m < 0 {
		return 0
	}
	return m
}

// IsScrollable reports whether the content is larger than the viewport.
func (v *FrameView) IsScrollable() bool {
	return v.MaxScrollX() > 0 || v.MaxScrollY() > 0
}

// EnsureVisible scrolls the viewport so the rectangle (x,y,w,h) is visible.
func (v *FrameView) EnsureVisible(x, y, w, h int) {
	needScroll := false
	if w > v.width {
		v.scrollX = x
		needScroll = true
	} else if x < v.scrollX {
		v.scrollX = x
		needScroll = true
	} else if x+w > v.scrollX+v.width {
		v.scrollX = x + w - v.width
		needScroll = true
	}
	if h > v.height {
		v.scrollY = y
		needScroll = true
	} else if y < v.scrollY {
		v.scrollY = y
		needScroll = true
	} else if y+h > v.scrollY+v.height {
		v.scrollY = y + h - v.height
		needScroll = true
	}
	if needScroll {
		v.SetScrollOffset(v.scrollX, v.scrollY)
	}
}

// --- Content size -----------------------------------------------------------

// ContentWidth returns the total laid-out content width.
func (v *FrameView) ContentWidth() int { return v.contentWidth }

// ContentHeight returns the total laid-out content height.
func (v *FrameView) ContentHeight() int { return v.contentHeight }

// SetContentSize records the total laid-out content dimensions.
func (v *FrameView) SetContentSize(w, h int) {
	v.contentWidth = w
	v.contentHeight = h
	v.SetScrollOffset(v.scrollX, v.scrollY)
}

// --- Layout -----------------------------------------------------------------

// Layout runs a layout pass for the frame's render tree.
func (v *FrameView) Layout() {
	Logf("Layout", "start viewport=%dx%d needsLayout=%v",
		v.width, v.height, v.layoutPhase == LayoutPhaseNeedsLayout)
	if v.frame == nil || v.frame.renderView == nil {
		v.layoutPhase = LayoutPhaseNone
		Logf("Layout", "skip: no frame/renderView")
		return
	}
	// Flush any pending render-tree rebuild from DOM mutations before layout.
	v.frame.RebuildRenderTreeIfNeeded()
	rv := v.frame.renderView
	rv.SetViewportSize(float64(v.width), float64(v.height))
	rv.Layout(nil)
	v.updateContentSize(rv)
	v.layoutPhase = LayoutPhaseNone
	Logf("Layout", "done contentSize=%dx%d", v.contentWidth, v.contentHeight)
}

// updateContentSize

// updateContentSize walks the render tree to find the maximum extent.
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
	log.Printf("[scroll] updateContentSize: maxX=%d maxY=%d rvType=%T", maxX, maxY, rv)
	v.SetScrollOffset(v.scrollX, v.scrollY)
}

// asRenderBox attempts to cast a RenderObject to *rendering.RenderBox.
func asRenderBox(o rendering.RenderObject) *rendering.RenderBox {
	if o == nil {
		return nil
	}
	box, ok := o.(*rendering.RenderBox)
	if ok {
		return box
	}
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
