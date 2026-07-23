// Top-level layout entry point.
package layout

import "math"

// Layout is the top-level entry point. It sizes rootBox within viewport and lays
// out its descendants. Returns LayoutState with computed geometry.
func Layout(rootBox *ElementBox, viewportWidth, viewportHeight int) *LayoutState {
	state := NewLayoutState(float64(viewportWidth), float64(viewportHeight))
	if rootBox == nil { return state }
	stretchRootToViewport(rootBox, state)
	LayoutRoot(rootBox, state)
	roundTree(rootBox, state)
	return state
}

func stretchRootToViewport(box *ElementBox, state *LayoutState) {
	g := state.GeometryForBox(box)
	g.SetTopLeft(0, 0)
	g.SetContentWidth(state.ViewportWidth)
	g.SetContentHeight(state.ViewportHeight)
	g.SetMargin(0, 0, 0, 0)
	if box.Style() != nil && box.Style().BoxSizing == "border-box" { return }
	g.SetContentWidth(math.Max(0, state.ViewportWidth-g.HorizontalBorderAndPadding()))
	g.SetContentHeight(math.Max(0, state.ViewportHeight-g.VerticalBorderAndPadding()))
}

// LayoutRoot dispatches root box to its formatting context.
func LayoutRoot(box *ElementBox, state *LayoutState) {
	ctx := contextFor(box)
	ctx.Layout(box, state)
}

func roundTree(box Box, state *LayoutState) {
	state.GeometryForBox(box).Round()
	if eb, ok := box.(*ElementBox); ok {
		for _, c := range eb.Children() { roundTree(c, state) }
	}
}
