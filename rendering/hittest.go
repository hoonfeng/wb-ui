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

// lastDive 记录最近一次 iframe 下钻命中的子 Frame 视图与子坐标系点击点。
// HitTestScrollContainer 命中子文档元素后，需要用子坐标在子 Frame 的
// RenderView 里递归查找滚动容器。wb-ui 渲染/交互是单线程模型（app 层
// 串行处理输入），包级记录无并发风险；多 WebView 时也是顺序调用。
var lastDive struct {
	sub IFrameSubdocument
	x, y float64
	ok   bool
}

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
	bestOrder := -1
	nextOrder := 0
	hitTestFixedInner(o, x, y, attrName, best, bestArea, rv, false, 0, &bestOrder, &nextOrder)
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
//   - stacking order wins ACROSS fixed layers: each fixed layer gets an
//     incrementing order in traversal (= paint order, later = on top), so a
//     click on a top dialog-overlay's backdrop hits that overlay, not the
//     same-size overlay underneath (two stacked dialogs — workspace create +
//     dir browser). Within one layer, smallest area wins, so buttons <
//     dialog-box < overlay resolve correctly.
func hitTestFixedInner(o RenderObject, x, y float64, attrName string, best **dom.Element, bestArea *float64, rv *RenderView, insideFixed bool, order int, bestOrder *int, nextOrder *int) {
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
		if isFixed {
			// Fixed layers are numbered in traversal order (= paint order,
			// same z-index siblings paint in tree order, later on top).
			order = *nextOrder
			*nextOrder++
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
			// Hit candidate. Higher layer order (painted on top) wins;
			// within the same layer the smallest area wins.
			if el, isEl := o.Node().(*dom.Element); isEl {
				if attrName == "" || el.GetAttribute(attrName) != "" {
					area := ow * oh
					if *best == nil || order > *bestOrder || (order == *bestOrder && area < *bestArea) {
						*best = el
						*bestArea = area
						*bestOrder = order
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
		hitTestFixedInner(c, childX, childY, attrName, best, bestArea, rv, childInside, order, bestOrder, nextOrder)
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
		// ★ 高度 0 的容器（如 body 无流内容时的 html/body）同样 pass-through：
		//   position:absolute 子元素溢出容器仍应可点击（浏览器 hit-test 语义
		//   ——absolute 元素不依赖祖先高度）。若按 inBounds 拦截，iframe 子
		//   文档的 absolute 内容在 html 高度 0 时全部不可命中。
		if ow == 0 || oh == 0 {
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

	// ★ iframe 下钻：命中 iframe 内容框时，把点击点转换到子 Frame
	// 坐标系，在子文档渲染树里继续 hit-test（WebKit HitTestResult
	// 递归到子 Frame）。子文档元素比 iframe 框本身更「深」，面积更
	// 小者胜出——点击 iframe 内容应命中子文档元素而非 iframe 元素。
	if ok && ow > 0 && oh > 0 {
		if el, isEl := o.Node().(*dom.Element); isEl && el.LocalName() == "iframe" {
			if sub := IFrameLookupFor(el); sub != nil && sub.RenderView() != nil {
				if box := asRenderBox(o); box != nil {
					if st := box.Style(); st != nil {
						pL := lengthValue(st.PaddingLeft)
						pT := lengthValue(st.PaddingTop)
						pR := lengthValue(st.PaddingRight)
						pB := lengthValue(st.PaddingBottom)
						cx := x - (ox + pL)
						cy := y - (oy + pT)
						if cx >= 0 && cy >= 0 && cx < ow-pL-pR && cy < oh-pT-pB {
							if sub.NeedsLayout() {
								sub.LayoutNow()
							}
							if child := HitTest(sub.RenderView(), cx, cy, attrName); child != nil {
								area := (ow - pL - pR) * (oh - pT - pB)
								// ★ 下钻候选直接优先：点击点在 iframe 内容框内，
								// 子文档元素比 iframe 框本身更深（WebKit
								// HitTestResult 递归进子 Frame）。不能用
								// area < bestArea 比较——内容框面积与 iframe
								// 元素面积相等（border 0 时）会漏掉覆盖。
								*best = child
								*bestArea = area
								// 记录下钻信息：滚动容器查找（HitTestScrollContainer）
								// 需要子坐标与子 Frame 视图。
								lastDive.sub = sub
								lastDive.x, lastDive.y = cx, cy
								lastDive.ok = true
							}
						}
					}
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
