// Translation of: Source/WebCore/rendering/RenderLayerBacking.h
//                  Source/WebCore/rendering/RenderLayerBacking.cpp
// Completeness: 40%
// Simplifications:
//   - GraphicsLayer is represented as a lightweight placeholder struct; the real
//     GPU-backed layer lives in the platform/graphics package (Phase 8)
//   - the painting / update-graphics-layer state machine is omitted
//   - the ancestor-clipping / descendant-clipping layer stacks are omitted
//   - only the primary graphics layer is tracked; no separate foreground / mask /
//     contents / clipping layers

package rendering

import "wb-ui/layout"

// GraphicsLayer is a placeholder for the composited output target. In the full WebKit
// port this wraps a platform GraphicsLayer that holds a GPU texture or display list.
type GraphicsLayer struct {
	name    string
	x, y    float64
	width   float64
	height  float64
	opacity float64
	visible bool
}

// NewGraphicsLayer constructs a placeholder graphics layer with the given debug name.
func NewGraphicsLayer(name string) *GraphicsLayer {
	return &GraphicsLayer{name: name, opacity: 1.0, visible: true}
}

// Name returns the debug name.
func (g *GraphicsLayer) Name() string { return g.name }

// X / Y / Width / Height return the layer geometry.
func (g *GraphicsLayer) X() float64      { return g.x }
func (g *GraphicsLayer) Y() float64      { return g.y }
func (g *GraphicsLayer) Width() float64  { return g.width }
func (g *GraphicsLayer) Height() float64 { return g.height }

// SetPosition sets the layer origin, mirroring GraphicsLayer::setPosition().
func (g *GraphicsLayer) SetPosition(x, y float64) { g.x, g.y = x, y }

// SetSize sets the layer dimensions, mirroring GraphicsLayer::setSize().
func (g *GraphicsLayer) SetSize(w, h float64) { g.width, g.height = w, h }

// Opacity returns the layer opacity.
func (g *GraphicsLayer) Opacity() float64 { return g.opacity }

// SetOpacity sets the layer opacity, mirroring GraphicsLayer::setOpacity().
func (g *GraphicsLayer) SetOpacity(o float64) { g.opacity = o }

// IsVisible reports whether the layer is drawn.
func (g *GraphicsLayer) IsVisible() bool { return g.visible }

// SetVisible controls whether the layer is drawn, mirroring GraphicsLayer::setVisible().
func (g *GraphicsLayer) SetVisible(v bool) { g.visible = v }

// RenderLayerBacking is the Go translation of WebCore::RenderLayerBacking. It owns the
// GraphicsLayer(s) that back a composited RenderLayer and tracks the layer's compositing
// configuration. In this simplified port only the primary graphics layer is tracked.
type RenderLayerBacking struct {
	layer            *RenderLayer
	graphicsLayer    *GraphicsLayer
	contentsScale    float64
	needsDisplay     bool
}

// NewRenderLayerBacking constructs a backing for the given layer, creating a primary
// graphics layer.
func NewRenderLayerBacking(layer *RenderLayer) *RenderLayerBacking {
	b := &RenderLayerBacking{
		layer:         layer,
		contentsScale: 1.0,
	}
	b.graphicsLayer = NewGraphicsLayer("backing for " + layerName(layer))
	return b
}

// Layer returns the owning render layer.
func (b *RenderLayerBacking) Layer() *RenderLayer { return b.layer }

// GraphicsLayer returns the primary graphics layer, mirroring
// RenderLayerBacking::graphicsLayer().
func (b *RenderLayerBacking) GraphicsLayer() *GraphicsLayer { return b.graphicsLayer }

// ContentsScale returns the backing store scale factor.
func (b *RenderLayerBacking) ContentsScale() float64 { return b.contentsScale }

// SetContentsScale sets the backing store scale factor.
func (b *RenderLayerBacking) SetContentsScale(s float64) { b.contentsScale = s }

// NeedsDisplay reports whether the backing needs to be repainted.
func (b *RenderLayerBacking) NeedsDisplay() bool { return b.needsDisplay }

// SetNeedsDisplay marks the backing as needing a repaint.
func (b *RenderLayerBacking) SetNeedsDisplay(v bool) { b.needsDisplay = v }

// UpdateGeometry syncs the graphics layer's position and size to the owner's border box,
// mirroring RenderLayerBacking::updateGeometry().
func (b *RenderLayerBacking) UpdateGeometry() {
	if b.layer == nil || b.layer.owner == nil {
		return
	}
	if box, ok := b.layer.owner.(*RenderBox); ok {
		rect := box.BorderBoxRect()
		b.graphicsLayer.SetPosition(rect.X, rect.Y)
		b.graphicsLayer.SetSize(rect.Width, rect.Height)
	}
	if st := b.layer.owner.Style(); st != nil {
		b.graphicsLayer.SetOpacity(st.Opacity)
		b.graphicsLayer.SetVisible(st.Visibility != "hidden")
	}
}

// layerName returns a debug name for the layer's owner.
func layerName(l *RenderLayer) string {
	if l == nil || l.owner == nil {
		return "?"
	}
	return l.owner.RenderName()
}

// UpdateAfterLayout refreshes the backing's geometry after a layout pass, mirroring
// RenderLayerBacking::updateAfterLayout().
func (b *RenderLayerBacking) UpdateAfterLayout() {
	b.UpdateGeometry()
}

// Rect returns the backing's graphics layer rectangle.
func (b *RenderLayerBacking) Rect() layout.LayoutRect {
	if b.graphicsLayer == nil {
		return layout.LayoutRect{}
	}
	return layout.LayoutRect{
		X:      b.graphicsLayer.X(),
		Y:      b.graphicsLayer.Y(),
		Width:  b.graphicsLayer.Width(),
		Height: b.graphicsLayer.Height(),
	}
}
