// Translation of: Source/WebCore/rendering/RenderView.cpp
package rendering

import (
	"wb-ui/dom"
	"wb-ui/html5"
	"wb-ui/layout"
	"wb-ui/platform/graphics"
	"wb-ui/style"
	"wb-ui/widgets"
)

func init() {
	// Set layout font metrics callback using Skia's actual font metrics.
	// This ensures line-height and text positioning match what Skia renders.
	layout.FontMetricsFunc = func(family string, size float64, weight int, style2 string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}
		a := graphics.GlobalFontAscent(f)
		d := graphics.GlobalFontDescent(f)
		lg := graphics.GlobalFontLineGap(f)
		return a, d, lg
	}
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
}

type RenderView struct {
	RenderBlockFlow
	document    *dom.Document
	viewWidth   float64
	viewHeight  float64
	compositor  *RenderLayerCompositor
	rootLayer   *RenderLayer
	layoutState *layout.LayoutState
	editorRegistry *widgets.EditorRegistry
	dirtyRect   Rect
	scrollOffsetX, scrollOffsetY float64
}

func NewRenderView(doc *dom.Document, st *style.ComputedStyle) *RenderView {
	rv := &RenderView{document: doc}
	rv.initBase(rv, doc, st)
	rv.compositor = NewRenderLayerCompositor(rv)
	rv.editorRegistry = widgets.NewEditorRegistry()
	return rv
}

func (v *RenderView) EditorRegistry() *widgets.EditorRegistry { return v.editorRegistry }
func (v *RenderView) Type() RenderObjectType                 { return ObjectView }
func (v *RenderView) IsRenderView() bool                     { return true }
func (v *RenderView) RenderName() string                     { return "RenderView" }
func (v *RenderView) Document() *dom.Document                { return v.document }
func (v *RenderView) View() *RenderView                     { return v }
func (v *RenderView) ViewWidth() float64                    { return v.viewWidth }
func (v *RenderView) ViewHeight() float64                   { return v.viewHeight }
func (v *RenderView) IsDirty() bool                          { return v.dirtyRect.Width > 0 && v.dirtyRect.Height > 0 }

func (v *RenderView) GetDirtyRect() Rect { return v.dirtyRect }

func (v *RenderView) MarkDirty(r Rect) {
	if v.dirtyRect.Width <= 0 || v.dirtyRect.Height <= 0 {
		v.dirtyRect = r; return
	}
	x0 := min2(v.dirtyRect.X, r.X)
	y0 := min2(v.dirtyRect.Y, r.Y)
	x1 := max2(v.dirtyRect.X+v.dirtyRect.Width, r.X+r.Width)
	y1 := max2(v.dirtyRect.Y+v.dirtyRect.Height, r.Y+r.Height)
	v.dirtyRect = Rect{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}
}

func (v *RenderView) MarkAllDirty() {
	v.dirtyRect = Rect{X: 0, Y: 0, Width: v.viewWidth, Height: v.viewHeight}
}

func (v *RenderView) ClearDirty() { v.dirtyRect = Rect{} }

func (v *RenderView) SetScrollOffset(x, y float64) {
	v.scrollOffsetX, v.scrollOffsetY = x, y
	if v.viewWidth > 0 && v.viewHeight > 0 { v.MarkAllDirty() } else { v.dirtyRect = Rect{X: 0, Y: 0, Width: 1, Height: 1} }
}

func (v *RenderView) ScrollOffset() (float64, float64) { return v.scrollOffsetX, v.scrollOffsetY }

func (v *RenderView) SetViewportSize(w, h float64) {
	if v.viewWidth != w || v.viewHeight != h { v.viewWidth, v.viewHeight = w, h; v.Dirty() }
}

func (v *RenderView) RootLayer() *RenderLayer          { return v.rootLayer }
func (v *RenderView) SetRootLayer(l *RenderLayer)       { v.rootLayer = l }
func (v *RenderView) Compositor() *RenderLayerCompositor { return v.compositor }

func (v *RenderView) Layout(state *layout.LayoutState) {
	if state == nil { state = layout.NewLayoutState(v.viewWidth, v.viewHeight) }
	v.layoutState = state
	v.frame.X, v.frame.Y = 0, 0
	v.frame.Width, v.frame.Height = v.viewWidth, v.viewHeight

	if lb := v.LayoutBox(); lb != nil {
		g := state.GeometryForBox(lb)
		g.SetTopLeft(0, 0)
		g.SetContentWidth(v.viewWidth)
		g.SetContentHeight(v.viewHeight)
	}
	if v.LayoutBox() == nil && v.document != nil {
		if root := v.document.DocumentElement(); root != nil {
			defaultResolver := style.NewResolver()
			defaultResolver.AddStyleSheet(html5.NewUAStyleSheet())
			layoutRoot := layout.BuildLayoutTree(root, defaultResolver)
			if layoutRoot != nil {
				if rootEb, ok := layoutRoot.(*layout.ElementBox); ok {
					v.SetLayoutBox(rootEb)
				}
			}
		}
	}
	v.RenderBlockFlow.Layout(state)
	v.syncGeometry()
	if v.compositor != nil { v.compositor.UpdateCompositingLayers() }
}

func (v *RenderView) LayoutState() *layout.LayoutState { return v.layoutState }

func (v *RenderView) syncGeometry() {
	layoutRoot := v.LayoutBox()
	if layoutRoot == nil { return }
	state := v.layoutState
	if state == nil { return }
	if box := asRenderBox(v); box != nil {
		box.frame = state.GeometryForBox(layoutRoot).ToRect()
	}
	for rc := v.FirstChild(); rc != nil; rc = rc.NextSibling() {
		syncOne(rc, layoutRoot, state)
		break
	}
}

func syncOne(ro RenderObject, lb *layout.ElementBox, state *layout.LayoutState) {
	if ro == nil || lb == nil || state == nil { return }
	rect := state.GeometryForBox(lb).ToRect()
	if rect.Width < 0 { rect.Width = 0 }
	if rect.Height < 0 { rect.Height = 0 }
	if rect.X < 0 { rect.X = 0 }
	if rect.Y < 0 { rect.Y = 0 }
	if box := asRenderBox(ro); box != nil { box.frame = rect }
	ro.SetLayoutBox(lb)

	textLB := lb
	var textSegments []layout.TextSegment
	if len(textLB.TextSegments) > 0 {
		textSegments = textLB.TextSegments
	} else {
		textSegments = findTextSegments(textLB)
	}
	if rt, ok := ro.(*RenderText); ok && len(textSegments) > 0 {
		segs := make([]InlineTextBox, len(textSegments))
		for i, s := range textSegments {
			segs[i] = InlineTextBox{
				Start: s.Start, Len: s.Len,
				X: s.X, Y: s.Y, Width: s.Width, Height: s.Height,
				LineY: s.LineY, LineHeight: s.LineHeight,
			}
		}
		rt.SetSegments(segs)
		if box := asRenderBox(ro); box != nil && len(segs) > 0 {
			box.frame.Width = segs[0].Width
			box.frame.Height = segs[0].Height
		}
	} else if len(textSegments) > 0 {
		// ro is NOT a RenderText (e.g. anonymous wrapper). Propagate
		// segments to the first RenderText child.
		for rc := ro.FirstChild(); rc != nil; rc = rc.NextSibling() {
			if rt, ok := rc.(*RenderText); ok {
				segs := make([]InlineTextBox, len(textSegments))
				for i, s := range textSegments {
					segs[i] = InlineTextBox{
						Start: s.Start, Len: s.Len,
						X: s.X, Y: s.Y, Width: s.Width, Height: s.Height,
						LineY: s.LineY, LineHeight: s.LineHeight,
					}
				}
				rt.SetSegments(segs)
				break
			}
		}
	}
	syncChildren(ro, lb, state)
}

func syncChildren(parentRO RenderObject, parentLB *layout.ElementBox, state *layout.LayoutState) {
	lChildren := parentLB.Children()
	for rc := parentRO.FirstChild(); rc != nil && len(lChildren) > 0; rc = rc.NextSibling() {
		matched := -1
		for i, lc := range lChildren {
			if childEb, ok := lc.(*layout.ElementBox); ok {
				if sameOwner(rc, childEb) { matched = i; break }
			}
		}
		if matched >= 0 {
			if childEb, ok := lChildren[matched].(*layout.ElementBox); ok {
				syncOne(rc, childEb, state)
			}
			lChildren = append(lChildren[:matched], lChildren[matched+1:]...)
		} else if rc.Node() == nil {
			// Anonymous render child: match with next anonymous layout child.
			for i, lc := range lChildren {
				if childEb, ok := lc.(*layout.ElementBox); ok && childEb.Element() == nil {
					syncOne(rc, childEb, state)
					lChildren = append(lChildren[:i], lChildren[i+1:]...)
					break
				}
			}
		}
	}
}


// findTextSegments walks the layout tree to find TextSegments, checking both
// ElementBox.TextSegments and InlineTextBox.TextSegments at each level.
func findTextSegments(lb *layout.ElementBox) []layout.TextSegment {
	if lb == nil { return nil }
	if len(lb.TextSegments) > 0 { return lb.TextSegments }
	for _, c := range lb.Children() {
		if tb, ok := c.(*layout.InlineTextBox); ok && len(tb.TextSegments) > 0 {
			return tb.TextSegments
		}
		if childEb, ok := c.(*layout.ElementBox); ok {
			if segs := findTextSegments(childEb); segs != nil {
				return segs
			}
		}
	}
	return nil
}
