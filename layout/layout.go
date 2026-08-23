// Top-level layout entry point.
package layout

import "math"

// currentViewportHeight stores the viewport height for vh unit resolution.
// Set by Layout() before any layout pass begins. Safe because layout is
// single-threaded.
var (
	currentViewportHeight float64
	currentViewportWidth  float64
)

// SetViewportSize 同步全局 viewport（vh/vw 单位解析用）。
// ★ 多 WebView 共享包级变量：每个视图布局开始前必须设置自己的
// viewport——否则 A 视图（如 260x80 挂件）布局后，B 视图（1280x800）
// 复用 layoutState 时 calc(100vh-76px) 会用 A 的 vh 求值 → 高度塌陷。
func SetViewportSize(w, h float64) {
	currentViewportWidth = w
	currentViewportHeight = h
}

// Layout is the top-level entry point. It sizes rootBox within viewport and lays
// out its descendants. Returns LayoutState with computed geometry.
func Layout(rootBox *ElementBox, viewportWidth, viewportHeight int) *LayoutState {
	state := NewLayoutState(float64(viewportWidth), float64(viewportHeight))
	currentViewportHeight = float64(viewportHeight)
	currentViewportWidth = float64(viewportWidth)
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
	ctx := contextFor(box, state)
	ctx.Layout(box, state)
}

func roundTree(box Box, state *LayoutState) {
	state.GeometryForBox(box).Round()
	if eb, ok := box.(*ElementBox); ok {
		for _, c := range eb.Children() { roundTree(c, state) }
	}
}
