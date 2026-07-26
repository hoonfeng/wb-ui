// Translation of: Source/WebCore/rendering/HitTestResult.cpp
//                  Source/WebCore/rendering/RenderLayer.cpp (hitTestLayer)
// Completeness: 50%
// Simplifications:
//   - only hit-tests axis-aligned bounding boxes (no border-radius / clip-path)
//   - returns the topmost (deepest) element that contains the point and has a
//     matching attribute; no stacking-context / z-index ordering
//   - no pointer-events: none handling

package rendering

import "wb-ui/dom"

// boxCoords extracts the bounding rectangle of a RenderObject if it is box-bearing.
// Returns ok=false for non-box objects (RenderInline, RenderText without a box).
func boxCoords(o RenderObject) (x, y, w, h float64, ok bool) {
	switch v := o.(type) {
	case *RenderBox:
		return v.X(), v.Y(), v.Width(), v.Height(), true
	case *RenderBlock:
		return v.RenderBox.X(), v.RenderBox.Y(), v.RenderBox.Width(), v.RenderBox.Height(), true
	case *RenderBlockFlow:
		return v.RenderBlock.RenderBox.X(), v.RenderBlock.RenderBox.Y(),
			v.RenderBlock.RenderBox.Width(), v.RenderBlock.RenderBox.Height(), true
	case *RenderView:
		return v.RenderBlockFlow.RenderBlock.RenderBox.X(),
			v.RenderBlockFlow.RenderBlock.RenderBox.Y(),
			v.RenderBlockFlow.RenderBlock.RenderBox.Width(),
			v.RenderBlockFlow.RenderBlock.RenderBox.Height(), true
	}
	return 0, 0, 0, 0, false
}

// HitTest walks the render tree rooted at rv and returns the deepest Element whose
// bounding box contains (x, y) and that has the given attribute set (e.g. "onclick").
// When attrName is empty, returns the deepest box-bearing element at the point.
// Returns nil when no element matches.
func HitTest(rv *RenderView, x, y float64, attrName string) *dom.Element {
	var best *dom.Element
	var bestArea float64 = -1
	hitTestWalk(RenderObject(rv), x, y, attrName, &best, &bestArea, rv)
	return best
}

// hitTestWalk recursively visits render objects, tracking the smallest (deepest)
// matching element. When descending into children of a scroll container with a
// per-box scroll offset (sx, sy), the hit-test point is adjusted by (sx, sy)
// so that visually scrolled children can still be hit at their apparent position.
func hitTestWalk(o RenderObject, x, y float64, attrName string, best **dom.Element, bestArea *float64, rv *RenderView) {
	if o == nil {
		return
	}
	ox, oy, ow, oh, ok := boxCoords(o)
	if !ok {
		// Non-box objects (inline, text) still recurse into children.
	} else {
		if x < ox || y < oy || x >= ox+ow || y >= oy+oh {
			return // outside this box's bounds
		}
	}
	// Check if this object's DOM node is an element with the attribute.
	if attrName != "" {
		node := o.Node()
		if el, isEl := node.(*dom.Element); isEl {
			if val := el.GetAttribute(attrName); val != "" {
				area := ow * oh
				if *best == nil || area < *bestArea {
					*best = el
					*bestArea = area
				}
			}
		}
	} else {
		node := o.Node()
		if el, isEl := node.(*dom.Element); isEl {
			area := ow * oh
			if *best == nil || area < *bestArea {
				*best = el
				*bestArea = area
			}
		}
	}

	// Determine if this box has a per-box scroll offset.
	// If it does, children are visually shifted by (-sx, -sy),
	// so we must add (sx, sy) to the hit-test point for children
	// to correctly map visual clicks to layout positions.
	childX, childY := x, y
	if rv != nil {
		if box := asRenderBox(o); box != nil {
			sx, sy := rv.BoxScrollOffset(box)
			if sx != 0 || sy != 0 {
				childX = x + sx
				childY = y + sy
			}
		}
	}

	for c := o.FirstChild(); c != nil; c = c.NextSibling() {
		hitTestWalk(c, childX, childY, attrName, best, bestArea, rv)
	}
}
