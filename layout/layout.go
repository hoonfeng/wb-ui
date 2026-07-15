// Translation of: Source/WebCore/layout/LayoutContext.h
//                  Source/WebCore/layout/LayoutContext.cpp
//                  Source/WebCore/layout/integration/LayoutIntegration.cpp
// Completeness: 60%
// Simplifications:
//   - no subpixel layout (integer pixels only; floats used internally then rounded)
//   - no pagination/fragmentation
//   - the C++ LayoutContext drives a full layout pass over the layout tree, scheduling
//     invalidation and layout for each formatting context; this port exposes a single
//     top-level Layout function that sets up the LayoutState and dispatches to the
//     formatting context selected by the root box display
//   - the initial containing block (viewport) is sized by the caller and assigned to
//     the root box; the body-stretches-to-viewport quirk is not implemented
//   - there is no two-pass intrinsic-width computation; each formatting context
//     computes widths in a single pass using the containing-block size it receives

package layout

import "math"

// Layout is the top-level entry point of the layout engine. It sizes and positions
// rootBox within a viewport of viewportWidth x viewportHeight CSS pixels and lays out
// its descendants. The returned LayoutState holds the viewport size used during the
// pass and may carry cached state for nested layout invocations.
//
// rootBox is expected to be the documentElement layout box (typically the <html>
// element). Its border-box is stretched to fill the viewport; children are laid out
// by the formatting context chosen by rootBox display (block by default).
func Layout(rootBox *LayoutBox, viewportWidth, viewportHeight int) *LayoutState {
	state := NewLayoutState(float64(viewportWidth), float64(viewportHeight))
	if rootBox == nil {
		return state
	}
	stretchRootToViewport(rootBox, state)
	layoutRoot(rootBox, state)
	roundTree(rootBox)
	return state
}

// stretchRootToViewport sizes the root box border-box to the viewport. The root
// element (html) has its width/height set to the viewport and its margin/padding
// cleared, mirroring RenderView::layout().
func stretchRootToViewport(box *LayoutBox, state *LayoutState) {
	r := &box.Rect
	r.X = 0
	r.Y = 0
	r.Width = state.ViewportWidth
	r.Height = state.ViewportHeight
	r.Margin = Edges{}
	if box.Style != nil && isBorderBox(box) {
		// border-box sizing: keep width/height as the border-box; content size shrinks.
		return
	}
	// content-box sizing: subtract border/padding so the border-box still fills the
	// viewport. For the root element border/padding default to zero so this is a no-op
	// in the common case.
	r.Width = math.Max(0, r.Width-r.Border.Horizontal()-r.Padding.Horizontal())
	r.Height = math.Max(0, r.Height-r.Border.Vertical()-r.Padding.Vertical())
}

// layoutRoot dispatches the root box to its formatting context.
func layoutRoot(box *LayoutBox, state *LayoutState) {
	ctx := contextFor(box)
	ctx.Layout(box, state)
}

// roundTree recursively rounds the geometry of every box to whole CSS pixels. This
// implements the "no subpixel layout" simplification: intermediate computations use
// floats for accuracy but the published geometry is integer.
func roundTree(box *LayoutBox) {
	box.Round()
	for _, c := range box.Children {
		roundTree(c)
	}
}