// Translation of: Source/WebCore/rendering/RenderLayerCompositor.h
//                  Source/WebCore/rendering/RenderLayerCompositor.cpp
// Completeness: 40%
// Simplifications:
//   - the overlap-map / TiledBacking / viewport-constraint machinery is omitted
//   - compositing layer assignment uses a simple rule: a layer gets its own backing when
//     it requires one (position / opacity / transform / overflow / filter) or when its
//     z-index places it in a separate stacking context; no overlap testing is performed
//   - the recompute-compositing-requirements sweep is omitted; UpdateCompositingLayers
//     walks the layer tree once and creates/destroys backings as needed
//   - the frame-view / page / scrolling-coordinator integration is omitted

package rendering

import "wb-ui/style"

// RenderLayerCompositor is the Go translation of WebCore::RenderLayerCompositor. It owns
// the compositing state for the render tree: it decides which RenderLayers get their own
// GraphicsLayer backing and updates the backing tree after layout.
type RenderLayerCompositor struct {
	renderView      *RenderView
	rootLayer       *RenderLayer
	compositedCount int
	dirty           bool
}

// NewRenderLayerCompositor constructs a compositor owned by the given RenderView.
func NewRenderLayerCompositor(rv *RenderView) *RenderLayerCompositor {
	return &RenderLayerCompositor{renderView: rv, dirty: true}
}

// RenderView returns the owning render view.
func (c *RenderLayerCompositor) RenderView() *RenderView { return c.renderView }

// SetRootLayer installs the root render layer that the compositor manages.
func (c *RenderLayerCompositor) SetRootLayer(l *RenderLayer) {
	c.rootLayer = l
	c.dirty = true
}

// RootLayer returns the root render layer.
func (c *RenderLayerCompositor) RootLayer() *RenderLayer { return c.rootLayer }

// NeedsCompositing reports whether the given layer should have its own compositing
// backing. This mirrors the conditions in RenderLayerCompositor's
// requiresCompositingLayer():
//   - the owner's style requires a layer (position / opacity / transform / overflow /
//     filter), AND
//   - the layer establishes a stacking context or has a non-default z-index
func (c *RenderLayerCompositor) NeedsCompositing(layer *RenderLayer) bool {
	if layer == nil || layer.owner == nil {
		return false
	}
	st := layer.owner.Style()
	if st == nil {
		return false
	}
	// Hardware-accelerated conditions that force compositing.
	if st.Opacity < 1.0 {
		return true
	}
	if st.Transform != "" {
		return true
	}
	if st.Filter != "" {
		return true
	}
	if st.OverflowX != style.OverflowVisible || st.OverflowY != style.OverflowVisible {
		return true
	}
	if st.Position == style.PositionFixed || st.Position == style.PositionSticky {
		return true
	}
	if st.GetProperty("will-change") != "" && st.GetProperty("will-change") != "auto" {
		return true
	}
	return false
}

// NeedsCompositingForStyle checks the backing-need condition derived purely from the
// computed style (used by the builder to decide whether to create a layer eagerly).
func NeedsCompositingForStyle(st *style.ComputedStyle) bool {
	if st == nil {
		return false
	}
	if st.Opacity < 1.0 {
		return true
	}
	if st.Transform != "" {
		return true
	}
	if st.Filter != "" {
		return true
	}
	if st.OverflowX != style.OverflowVisible || st.OverflowY != style.OverflowVisible {
		return true
	}
	if st.Position == style.PositionFixed || st.Position == style.PositionSticky {
		return true
	}
	return false
}

// UpdateCompositingLayers walks the layer tree and creates or destroys backings so that
// every layer that NeedsCompositing has a backing and every other layer does not. This
// mirrors RenderLayerCompositor::updateCompositingLayers().
func (c *RenderLayerCompositor) UpdateCompositingLayers() {
	if c.rootLayer == nil {
		return
	}
	c.compositedCount = 0
	c.updateLayerRecursive(c.rootLayer)
	c.dirty = false
}

// updateLayerRecursive visits a layer and its descendants, creating or clearing backings.
func (c *RenderLayerCompositor) updateLayerRecursive(layer *RenderLayer) {
	if c.NeedsCompositing(layer) {
		if layer.backing == nil {
			layer.EnsureBacking()
			if layer.backing != nil {
				layer.backing.UpdateGeometry()
			}
		} else {
			layer.backing.UpdateGeometry()
		}
		c.compositedCount++
	} else {
		if layer.backing != nil {
			layer.ClearBacking()
		}
	}
	for child := layer.firstChild; child != nil; child = child.nextSibling {
		c.updateLayerRecursive(child)
	}
}

// CompositedCount returns the number of layers that currently have backings.
func (c *RenderLayerCompositor) CompositedCount() int { return c.compositedCount }

// IsDirty reports whether the compositor needs a recomputing pass.
func (c *RenderLayerCompositor) IsDirty() bool { return c.dirty }

// MarkDirty flags the compositor as needing an update.
func (c *RenderLayerCompositor) MarkDirty() { c.dirty = true }

// BuildLayerTree constructs the render-layer tree from the render tree, creating a layer
// for every render object whose style satisfies RequiresLayer. The root render view
// always gets a layer. This mirrors the layer-creation pass in RenderView::layout().
func (c *RenderLayerCompositor) BuildLayerTree(root RenderObject) *RenderLayer {
	if root == nil {
		return nil
	}
	var build func(obj RenderObject, parentLayer *RenderLayer) *RenderLayer
	build = func(obj RenderObject, parentLayer *RenderLayer) *RenderLayer {
		var layer *RenderLayer
		if obj.IsRenderView() || RequiresLayer(obj) {
			layer = NewRenderLayer(obj)
			if parentLayer != nil {
				parentLayer.AddChild(layer)
			}
		} else {
			layer = parentLayer
		}
		for child := obj.FirstChild(); child != nil; child = child.NextSibling() {
			build(child, layer)
		}
		return layer
	}
	rootLayer := build(root, nil)
	c.SetRootLayer(rootLayer)
	return rootLayer
}
