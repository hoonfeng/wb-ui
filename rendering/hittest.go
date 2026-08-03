// Translation of: Source/WebCore/rendering/HitTestResult.cpp
//                  Source/WebCore/rendering/RenderLayer.cpp (hitTestLayer)
// Completeness: 50%
// Simplifications:
//   - only hit-tests axis-aligned bounding boxes (no border-radius / clip-path)
//   - returns the topmost (deepest) element that contains the point and has a
//     matching attribute; no stacking-context / z-index ordering
//   - no pointer-events: none handling

package rendering

import (
	"log"
	"wb-ui/dom"
	"wb-ui/style"
)

// debugHitTest enables verbose hit-test diagnostics.
const debugHitTest = false

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
//
// Fixed-position elements paint on top of everything (viewport-anchored) but
// occupy a LARGE bounding box, so a plain "smallest area" walk would pick the
// underlying page element instead — clicking a dialog overlay would fall
// through to the file tree / activity bar below. We first hit-test fixed
// subtrees (topmost in paint order); only if none matches do we walk the
// normal tree.
func HitTest(rv *RenderView, x, y float64, attrName string) *dom.Element {
	// Pass 1: fixed-position subtrees win (dialog overlay / context menus).
	var best *dom.Element
	var bestArea float64 = -1
	hitTestFixedFirst(RenderObject(rv), x, y, attrName, &best, &bestArea, rv)
	if best != nil {
		return best
	}
	// Pass 2: normal tree.
	best = nil
	bestArea = -1
	hitTestWalk(RenderObject(rv), x, y, attrName, &best, &bestArea, rv)
	return best
}

// hitTestFixedFirst walks the render tree but only considers subtrees whose
// ancestor is position:fixed (they paint above everything). The smallest-area
// element inside such a subtree wins — clicking the dialog-box (small) inside
// the overlay (large) resolves to the box, and clicking the overlay itself
// (outside the box) resolves to the overlay.
func hitTestFixedFirst(o RenderObject, x, y float64, attrName string, best **dom.Element, bestArea *float64, rv *RenderView) {
	hitTestFixedInner(o, x, y, attrName, best, bestArea, rv, false)
}

// hitTestFixedInner is the recursive body of hitTestFixedFirst. Once the walk
// enters a fixed-position subtree (insideFixed=true), EVERY box participates in
// hit-testing — the dialog-box's buttons / inputs / labels are position:static
// but paint on top of the normal flow, so they must be hittable (a click on the
// "browse" button must reach the button, not fall through to the dialog-box).
// Rules:
//   - a fixed box whose bounds do NOT contain the point excludes its whole
//     subtree (fixed siblings never overlap — clicking outside the dialog-box
//     must not hit a control inside it)
//   - a plain box inside a fixed subtree that misses the point just skips
//     itself; a smaller descendant may still contain the point
//   - smallest area wins, so buttons < dialog-box < overlay resolve correctly
func hitTestFixedInner(o RenderObject, x, y float64, attrName string, best **dom.Element, bestArea *float64, rv *RenderView, insideFixed bool) {
	if o == nil {
		return
	}
	ox, oy, ow, oh, ok := boxCoords(o)
	isFixed := false
	if ok {
		if box := asRenderBox(o); box != nil {
			if st := box.Style(); st != nil {
				isFixed = st.Position == style.PositionFixed
			}
		}
	}
	if ok && (isFixed || insideFixed) {
		if ow > 0 && oh > 0 {
			inBounds := x >= ox && y >= oy && x < ox+ow && y < oy+oh
			if isFixed {
				// Fixed box outside the point: its whole subtree is
				// excluded (fixed siblings don't overlap).
				if !inBounds {
					return
				}
			} else if !inBounds {
				// Plain box inside a fixed subtree that misses: skip
				// itself, children could still contain the point.
				goto descend
			}
			// Hit candidate: consider it (smallest area wins).
			if el, isEl := o.Node().(*dom.Element); isEl {
				if attrName == "" || el.GetAttribute(attrName) != "" {
					area := ow * oh
					if *best == nil || area < *bestArea {
						*best = el
						*bestArea = area
					}
				}
			}
		}
	}
descend:
	// Descend into children (scroll-offset aware like the normal walk).
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
	childInside := insideFixed || isFixed
	for c := o.FirstChild(); c != nil; c = c.NextSibling() {
		hitTestFixedInner(c, childX, childY, attrName, best, bestArea, rv, childInside)
	}
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
		// Anonymous wrappers (RenderBlockFlow from buildChildren flush) with
		// zero area have no visual content. Skip bounds check so children are
		// still visited; they won't become hit candidates (area=0).
		if ow == 0 && oh == 0 {
			// pass through to children
		} else {
			inBounds := x >= ox && y >= oy && x < ox+ow && y < oy+oh
			if debugHitTest && o.Node() != nil {
				if el, isEl := o.Node().(*dom.Element); isEl {
					tn := el.TagName()
					cn := el.ClassName()
					log.Printf("[dbg/ht] %s.%s box=(%.0f,%.0f %.0fx%.0f) pt=(%.0f,%.0f) inBounds=%v",
						tn, cn, ox, oy, ow, oh, x, y, inBounds)
				}
			}
			if !inBounds {
				return // outside this box's bounds
			}
		}
	}
	// Check if this object's DOM node is an element with the attribute.
	// Only box-bearing objects (ok=true) are considered as hit targets;
	// non-box objects (RenderInline, RenderText) just pass through to their children.
	// Zero-area boxes (0x0, e.g. an empty position:fixed toast-container) must
	// NOT become hit candidates: once one is picked (best==nil, area 0) every
	// later element loses because its area > 0 can never beat 0, so the empty
	// container swallows ALL clicks across the viewport.
	if ok && ow > 0 && oh > 0 {
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
