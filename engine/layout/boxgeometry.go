// Translation of: Source/WebCore/layout/layouttree/BoxGeometry.h
// BoxGeometry stores computed geometry separate from the layout tree node.
// Accessed via state.GeometryForBox(box).

package layout

import "math"

// Edges groups four CSS box-model sides. Used in box-model computations.
type Edges struct {
	Top, Right, Bottom, Left float64
}

func (e Edges) IsZero() bool        { return e.Top == 0 && e.Right == 0 && e.Bottom == 0 && e.Left == 0 }
func (e Edges) Horizontal() float64 { return e.Left + e.Right }
func (e Edges) Vertical() float64   { return e.Top + e.Bottom }

// LayoutRect is a transitional type retained for the rendering/ package.
type LayoutRect struct {
	X, Y          float64
	Width, Height float64
	Margin        Edges
	Padding       Edges
	Border        Edges
}

func (r LayoutRect) ContentX() float64      { return r.X + r.Border.Left + r.Padding.Left }
func (r LayoutRect) ContentY() float64      { return r.Y + r.Border.Top + r.Padding.Top }
func (r LayoutRect) ContentWidth() float64  { return r.Width - r.Border.Horizontal() - r.Padding.Horizontal() }
func (r LayoutRect) ContentHeight() float64 { return r.Height - r.Border.Vertical() - r.Padding.Vertical() }
func (r LayoutRect) MarginBoxWidth() float64  { return r.Width + r.Margin.Horizontal() }
func (r LayoutRect) MarginBoxHeight() float64 { return r.Height + r.Margin.Vertical() }

// BoxGeometry mirrors WebCore::Layout::BoxGeometry.
// For horizontal writing-mode with LTR: before=top, after=bottom, start=left, end=right.
type BoxGeometry struct {
	top, left                      float64
	contentWidth, contentHeight    float64
	marginBefore, marginEnd        float64
	marginStart, marginAfter       float64
	borderTop, borderRight         float64
	borderBottom, borderLeft       float64
	paddingTop, paddingRight       float64
	paddingBottom, paddingLeft     float64
}

// ── border-box position ──

func (g *BoxGeometry) Top() float64                 { return g.top }
func (g *BoxGeometry) Left() float64                { return g.left }
func (g *BoxGeometry) SetTopLeft(top, left float64) { g.top = top; g.left = left }

// ── content-box size ──

func (g *BoxGeometry) ContentWidth() float64        { return g.contentWidth }
func (g *BoxGeometry) ContentHeight() float64       { return g.contentHeight }
func (g *BoxGeometry) SetContentWidth(w float64)    { g.contentWidth = w }
func (g *BoxGeometry) SetContentHeight(h float64)   { g.contentHeight = h }

// ── border-box size ──

func (g *BoxGeometry) BorderBoxWidth() float64 {
	return g.contentWidth + g.BorderLeft() + g.PaddingLeft() + g.BorderRight() + g.PaddingRight()
}
func (g *BoxGeometry) BorderBoxHeight() float64 {
	return g.contentHeight + g.BorderTop() + g.PaddingTop() + g.BorderBottom() + g.PaddingBottom()
}

// ── padding-box ──

func (g *BoxGeometry) PaddingBoxLeft() float64  { return g.left + g.BorderLeft() }
func (g *BoxGeometry) PaddingBoxTop() float64   { return g.top + g.BorderTop() }
func (g *BoxGeometry) PaddingBoxWidth() float64 { return g.contentWidth + g.PaddingLeft() + g.PaddingRight() }
func (g *BoxGeometry) PaddingBoxHeight() float64 { return g.contentHeight + g.PaddingTop() + g.PaddingBottom() }

// ── content-box rect ──

func (g *BoxGeometry) ContentBoxLeft() float64 { return g.left + g.BorderLeft() + g.PaddingLeft() }
func (g *BoxGeometry) ContentBoxTop() float64  { return g.top + g.BorderTop() + g.PaddingTop() }

// ── margin-box ──

func (g *BoxGeometry) MarginBoxLeft() float64   { return g.left - g.marginStart }
func (g *BoxGeometry) MarginBoxTop() float64    { return g.top - g.marginBefore }
func (g *BoxGeometry) MarginBoxWidth() float64  { return g.BorderBoxWidth() + g.marginStart + g.marginAfter }
func (g *BoxGeometry) MarginBoxHeight() float64 { return g.BorderBoxHeight() + g.marginBefore + g.marginEnd }

// ── margin (logical) ──

func (g *BoxGeometry) MarginBefore() float64 { return g.marginBefore }
func (g *BoxGeometry) MarginAfter() float64  { return g.marginEnd }
func (g *BoxGeometry) MarginStart() float64  { return g.marginStart }
func (g *BoxGeometry) MarginEnd() float64    { return g.marginAfter }
func (g *BoxGeometry) SetMarginBefore(v float64) { g.marginBefore = v }
func (g *BoxGeometry) SetMarginAfter(v float64)  { g.marginEnd = v }
func (g *BoxGeometry) SetMarginStart(v float64)  { g.marginStart = v }
func (g *BoxGeometry) SetMarginEnd(v float64)    { g.marginAfter = v }
func (g *BoxGeometry) SetMargin(top, right, bottom, left float64) {
	g.marginBefore, g.marginEnd = top, bottom
	g.marginStart, g.marginAfter = left, right
}

// ── border (physical) ──

func (g *BoxGeometry) BorderTop() float64    { return g.borderTop }
func (g *BoxGeometry) BorderRight() float64  { return g.borderRight }
func (g *BoxGeometry) BorderBottom() float64 { return g.borderBottom }
func (g *BoxGeometry) BorderLeft() float64   { return g.borderLeft }
func (g *BoxGeometry) SetBorderTop(v float64)    { g.borderTop = v }
func (g *BoxGeometry) SetBorderRight(v float64)  { g.borderRight = v }
func (g *BoxGeometry) SetBorderBottom(v float64) { g.borderBottom = v }
func (g *BoxGeometry) SetBorderLeft(v float64)   { g.borderLeft = v }
func (g *BoxGeometry) SetBorder(top, right, bottom, left float64) {
	g.borderTop, g.borderRight = top, right
	g.borderBottom, g.borderLeft = bottom, left
}
func (g *BoxGeometry) HorizontalBorder() float64 { return g.borderLeft + g.borderRight }
func (g *BoxGeometry) VerticalBorder() float64   { return g.borderTop + g.borderBottom }

// ── padding (physical) ──

func (g *BoxGeometry) PaddingTop() float64    { return g.paddingTop }
func (g *BoxGeometry) PaddingRight() float64  { return g.paddingRight }
func (g *BoxGeometry) PaddingBottom() float64 { return g.paddingBottom }
func (g *BoxGeometry) PaddingLeft() float64   { return g.paddingLeft }
func (g *BoxGeometry) SetPaddingTop(v float64)    { g.paddingTop = v }
func (g *BoxGeometry) SetPaddingRight(v float64)  { g.paddingRight = v }
func (g *BoxGeometry) SetPaddingBottom(v float64) { g.paddingBottom = v }
func (g *BoxGeometry) SetPaddingLeft(v float64)   { g.paddingLeft = v }
func (g *BoxGeometry) SetPadding(top, right, bottom, left float64) {
	g.paddingTop, g.paddingRight = top, right
	g.paddingBottom, g.paddingLeft = bottom, left
}
func (g *BoxGeometry) HorizontalPadding() float64 { return g.paddingLeft + g.paddingRight }
func (g *BoxGeometry) VerticalPadding() float64   { return g.paddingTop + g.paddingBottom }

// ── composite ──

func (g *BoxGeometry) HorizontalBorderAndPadding() float64 { return g.HorizontalBorder() + g.HorizontalPadding() }
func (g *BoxGeometry) VerticalBorderAndPadding() float64   { return g.VerticalBorder() + g.VerticalPadding() }

// ── rounding ──

func (g *BoxGeometry) Round() {
	g.top = math.Round(g.top)
	g.left = math.Round(g.left)
	g.contentWidth = math.Round(g.contentWidth)
	g.contentHeight = math.Round(g.contentHeight)
	g.marginBefore = math.Round(g.marginBefore)
	g.marginEnd = math.Round(g.marginEnd)
	g.marginStart = math.Round(g.marginStart)
	g.marginAfter = math.Round(g.marginAfter)
	g.borderTop = math.Round(g.borderTop)
	g.borderRight = math.Round(g.borderRight)
	g.borderBottom = math.Round(g.borderBottom)
	g.borderLeft = math.Round(g.borderLeft)
	g.paddingTop = math.Round(g.paddingTop)
	g.paddingRight = math.Round(g.paddingRight)
	g.paddingBottom = math.Round(g.paddingBottom)
	g.paddingLeft = math.Round(g.paddingLeft)
}

// ToRect converts BoxGeometry to the legacy LayoutRect for rendering sync.
func (g *BoxGeometry) ToRect() LayoutRect {
	return LayoutRect{
		X: g.left, Y: g.top,
		Width:  g.BorderBoxWidth(),
		Height: g.BorderBoxHeight(),
		Margin: Edges{Top: g.marginBefore, Right: g.marginAfter, Bottom: g.marginEnd, Left: g.marginStart},
		Padding: Edges{Top: g.paddingTop, Right: g.paddingRight, Bottom: g.paddingBottom, Left: g.paddingLeft},
		Border:  Edges{Top: g.borderTop, Right: g.borderRight, Bottom: g.borderBottom, Left: g.borderLeft},
	}
}

// ── rounding ──