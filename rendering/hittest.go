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

// BoxViewportRect 返回 box 的视口矩形：layout 坐标减去所有祖先滚动容器
// 的滚动偏移（与 hitTestWalk 的坐标空间一致——鼠标事件用视口坐标，而
// RenderBox.X/Y 是未平移的 layout 坐标；滚动容器内容由 paint 阶段
// canvas translate 呈现）。textarea resize 手柄命中检测（app/host.go
// Press/updateCursor）必须用视口坐标比较，否则 settings-body 滚动后
// 手柄区域对不上鼠标位置。
func BoxViewportRect(rv *RenderView, o RenderObject) (x, y, w, h float64) {
	var ok bool
	x, y, w, h, ok = boxCoords(o)
	if rv == nil || !ok {
		return
	}
	// 沿父链向上累加滚动偏移（box 自身的滚动偏移不影响 box 自身的
	// 视口位置——只有祖先的滚动会平移它）。
	for p := o.Parent(); p != nil; p = p.Parent() {
		if pb := asRenderBox(p); pb != nil {
			sx, sy := rv.BoxScrollOffset(pb)
			if sx != 0 || sy != 0 {
				x -= sx
				y -= sy
			}
		}
	}
	return
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
	// Pass 2: ★ 层叠感知命中（镜像 paintLayerTree 的绘制顺序：后绘制的在
	// 上、先命中）。z-index/定位浮层（遮罩/弹窗）必须挡下层元素，否则
	// 点击弹窗空白区会穿透命中下层小面积元素（configwin 弹窗「事件
	// 穿透到下层」根因：hitTestWalk 按树序+最小面积，z-index:999 遮罩
	// 的面积远大于下层 wbox → wbox 胜出 → 穿透）。
	if root := rv.RootLayer(); root != nil {
		if el := hitTestLayer(root, x, y, attrName, rv); el != nil {
			return el
		}
	}
	// Pass 3: fallback normal tree walk（层树异常缺失时保持旧行为）。
	best = nil
	bestArea = -1
	hitTestWalk(RenderObject(rv), x, y, attrName, &best, &bestArea, rv)
	return best
}

// ── 层叠感知命中（mirrors paintLayerTree: layers paint in neg → auto →
// pos z-order; hit-testing walks the reverse so topmost wins）────────────

// hitTestLayer 在单个层内命中：命中顺序 = 子层 pos（z 大先）→ 子层 auto
// → 本层普通内容 → 子层 neg（负 z 最底）。同层叠级内按面积最小/后代
// 优先（与 hitTestWalk 一致）。
func hitTestLayer(layer *RenderLayer, x, y float64, attrName string, rv *RenderView) *dom.Element {
	// ★ 层起点滚动补偿：层树递归（hitTestLayer 逐层进入）不是从渲染树
	// 根连续递归，walkLayerContent 的 childX/childY 逐层补偿在此丢失
	// 了「祖先滚动偏移」。层 owner 在滚动容器内时（如滚动面板里的
	// z-index 弹层/浮层），owner 的 box 检查用未补偿视口坐标直接 miss
	// → 层内容整体漏掉 → 点击穿透到下层元素（「滚动后点弹层内容
	// 穿透/点错」）。绘制端 paintLayerContents 的内容 translate 按
	// 完整祖先链累计（滚动容器的 translate 包住内容+子层），命中必须
	// 镜像同一坐标系变换。用 PresentedBoxScrollOffset（已渲染帧快照）
	// 保持所见即所点。必须在 transform 逆变换**之前**（transform 空间
	// 位于滚动 translate 之内，先平移到布局坐标再逆变换到元素本地）。
	// ★ xIn/yIn：子层递归必须以「视口坐标」进入 —— 每层 hitTestLayer
	// 入口的都是视口坐标，自己一次性补偿祖先滚动。此前 bucket 递归
	// 传递的是「本层补偿过的坐标」，层树多层嵌套（滚动容器 > relative
	// 容器 > absolute 控件）时每层重复加同一祖先滚动偏移 → 深层元素
	// 命中坐标越界 miss → 「滚动容器内点击 input/文本不可命中（命中
	// 到容器空白区），光标不出现/出现位置错」。
	xIn, yIn := x, y
	if layer != nil && layer.owner != nil && rv != nil {
		if sx, sy := rv.ScrollStackOffsetFor(layer.owner); sx != 0 || sy != 0 {
			x += sx
			y += sy
		}
	}
	// ★ 层 owner 可能带 transform（弹窗 translate(-50%,-50%) 居中/旋转
	// 图标等）：命中坐标先逆变换到层本地空间（镜像 paint 的正向 canvas
	// 变换——视觉位置与布局位置不一致，不逆变换则点击视觉位置 miss）。
	if layer != nil && layer.owner != nil {
		if st := layer.owner.Style(); st != nil && st.Transform != "" && st.Transform != "none" {
			w, h := 0.0, 0.0
			if bx, _, bw, bh, ok := boxCoords(layer.owner); ok {
				_, _, w, h = bx, 0, bw, bh
			}
			x, y, _ = hitInverseTransform(st.Transform, w, h, x, y)
		}
	}
	// 子层（后绘制的在上）：先测 pos+auto（跳过 neg——它们在内容之下）。
	// ★ 传 xIn/yIn（视口坐标）：子层由自身 hitTestLayer 一次性补偿祖先
	// 滚动（见函数头注释）；传补偿后坐标会双重补偿（层树多层嵌套——
	// 滚动容器 > relative 容器 > absolute 控件——深层元素命中越界。
	// 复现：滚动容器内点击 input/文本命中容器空白，光标不出现）。
	if el := hitTestLayersBucket(layer, xIn, yIn, attrName, rv, false); el != nil {
		return el
	}
	// 本层普通内容（layer owner + 非层后代）
	if el := hitTestLayerContent(layer, x, y, attrName, rv); el != nil {
		return el
	}
	// 负 z 子层（最底）
	return hitTestLayersBucket(layer, xIn, yIn, attrName, rv, true)
}

// hitTestLayersBucket 按 paint bucket（neg / auto / pos）逆序尝试子层。
// negOnly=true 只测负 z 桶（内容之后）；否则按 pos(倒序) → auto(倒序)。
func hitTestLayersBucket(parent *RenderLayer, x, y float64, attrName string, rv *RenderView, negOnly bool) *dom.Element {
	var neg, auto, pos []*RenderLayer
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		z := layerZIndex(child)
		switch {
		case z < 0:
			neg = append(neg, child)
		case z > 0:
			pos = append(pos, child)
		default:
			auto = append(auto, child)
		}
	}
	sortLayerByZ(pos, true)
	sortLayerByZ(neg, true)
	try := func(l *RenderLayer) *dom.Element {
		return hitTestLayer(l, x, y, attrName, rv)
	}
	if negOnly {
		for i := len(neg) - 1; i >= 0; i-- {
			if el := try(neg[i]); el != nil {
				return el
			}
		}
		return nil
	}
	for i := len(pos) - 1; i >= 0; i-- {
		if el := try(pos[i]); el != nil {
			return el
		}
	}
	for i := len(auto) - 1; i >= 0; i-- {
		if el := try(auto[i]); el != nil {
			return el
		}
	}
	return nil
}

// hitTestLayerContent 在层的 owner 子树内找命中（候选=带 attr 的元素，
// 面积最小/后代优先，与 hitTestWalk 相同规则；跳过有独立子层的后代
// ——它们由 hitTestLayersBucket 按层叠顺序处理）。
func hitTestLayerContent(layer *RenderLayer, x, y float64, attrName string, rv *RenderView) *dom.Element {
	if layer == nil || layer.owner == nil {
		return nil
	}
	skip := make(map[RenderObject]bool, 4)
	for child := layer.FirstChild(); child != nil; child = child.NextSibling() {
		skip[child.owner] = true
	}
	var best *dom.Element
	var bestArea float64 = -1
	walkLayerContent(layer.owner, x, y, attrName, &best, &bestArea, rv, skip)
	return best
}

// walkLayerContent 是 hitTestWalk 的层内版本：规则一致（box 命中 +
// attr 匹配 + 面积最小/后代优先 + iframe 下钻），但遇到「有独立层的
// 后代」不再深入（其子树由层树按层叠顺序命中）。
func walkLayerContent(o RenderObject, x, y float64, attrName string, best **dom.Element, bestArea *float64, rv *RenderView, skip map[RenderObject]bool) {
	if o == nil {
		return
	}
	if skip[o] {
		return
	}
	ox, oy, ow, oh, ok := boxCoords(o)
	inBounds := true
	if !ok {
		// Non-box objects (inline, text) still recurse into children.
	} else {
		if ow > 0 && oh > 0 {
			inBounds = x >= ox && y >= oy && x < ox+ow && y < oy+oh
			if !inBounds {
				if el, isEl := o.Node().(*dom.Element); isEl &&
					(el.LocalName() == "html" || el.LocalName() == "body") {
					inBounds = true // pass-through
				} else {
					return
				}
			}
		}
	}
	if ok && ow > 0 && oh > 0 && inBounds {
		if attrName != "" {
			node := o.Node()
			if el, isEl := node.(*dom.Element); isEl {
				if val := el.GetAttribute(attrName); val != "" {
					area := ow * oh
					if !pointerEventsNone(o) && (*best == nil || area < *bestArea || descendantOf(el, *best)) {
						*best = el
						*bestArea = area
					}
				}
			}
		} else {
			node := o.Node()
			if el, isEl := node.(*dom.Element); isEl {
				area := ow * oh
				if !pointerEventsNone(o) && (*best == nil || area < *bestArea || descendantOf(el, *best)) {
					*best = el
					*bestArea = area
				}
			}
		}
	}
	// ★ 滚动容器偏移补偿（同 hitTestWalk / hitTestFixedInner）：层内命中
	// 此前直接用视口坐标递归——滚动容器（per-box offset≠0）内子元素的
	// 布局坐标与视觉位置相差 (sx,sy)，不补偿则滚动后点击命中「未滚动
	// 位置」的元素：点颜色行实际命中上方字体行（用户「鼠标点击位置与
	// 生效位置不匹配，不是绝对出现但会触发」根因——仅滚动后出现）。
	// 注意必须在 iframe 下钻之前：iframe 位于滚动容器内时子帧坐标
	// 同样要按 (sx,sy) 平移。
	// ★ 用 PresentedBoxScrollOffset（已渲染帧快照）：视觉帧滞后时命中
	// 依然按「用户所见」解析（所见即所点），避免滚动后快速点击时
	// 命中按新偏移解读（时序偏移根因）。
	childX, childY := x, y
	if rv != nil {
		if box := asRenderBox(o); box != nil {
			sx, sy := rv.PresentedBoxScrollOffset(box)
			if sx != 0 || sy != 0 {
				childX = x + sx
				childY = y + sy
			}
		}
	}
	// iframe 下钻（与 hitTestWalk 一致）
	if ok && ow > 0 && oh > 0 {
		if el, isEl := o.Node().(*dom.Element); isEl && el.LocalName() == "iframe" {
			if sub := IFrameLookupFor(el); sub != nil && sub.RenderView() != nil {
				if box := asRenderBox(o); box != nil {
					if st := box.Style(); st != nil {
						pL := lengthValue(st.PaddingLeft)
						pT := lengthValue(st.PaddingTop)
						pR := lengthValue(st.PaddingRight)
						pB := lengthValue(st.PaddingBottom)
						cx := childX - (ox + pL)
						cy := childY - (oy + pT)
						if cx >= 0 && cy >= 0 && cx < ow-pL-pR && cy < oh-pT-pB {
							if sub.NeedsLayout() {
								sub.LayoutNow()
							}
							if child := HitTest(sub.RenderView(), cx, cy, attrName); child != nil {
								area := (ow - pL - pR) * (oh - pT - pB)
								*best = child
								*bestArea = area
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
	// 递归（childX/childY 已在函数头按滚动容器的 per-box offset 补偿）
	for child := o.FirstChild(); child != nil; child = child.NextSibling() {
		walkLayerContent(child, childX, childY, attrName, best, bestArea, rv, skip)
	}
}

// pointerEventsNone reports whether the object's element is excluded from
// hit-testing via pointer-events: none. Browser semantics: the element and its
// subtree are skipped unless a descendant explicitly re-enables pointer-events:
// auto — checking the element itself suffices (re-enabled descendants hit by
// their own check). Applied identically on all three hit-test walks
// (fixed-first / layer / fallback tree) so a drag-ghost or toast overlay with
// pointer-events:none never steals clicks or the :hover target underneath.
func pointerEventsNone(o RenderObject) bool {
	if b := asRenderBox(o); b != nil {
		if st := b.Style(); st != nil && st.PointerEvents == "none" {
			return true
		}
	}
	return false
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
					// ★ 同层内同样「后代优先」：fixed 子树中嵌套元素
					//   （如 modal 里的按钮比其父容器高）也应命中更深者。
					if !pointerEventsNone(o) && (*best == nil || order > *bestOrder || (order == *bestOrder && (area < *bestArea || descendantOf(el, *best)))) {
						*best = el
						*bestArea = area
						*bestOrder = order
					}
				}
			}
		}
	}
descend:
	// ★ iframe 下钻（fixed 定位 iframe）：fixed 子树的 iframe 内容也应
	// 递归到子 Frame hit-test——fixed iframe（如固定悬浮的嵌入面板）
	// 点击其内容应命中子文档元素。order 继承 iframe 所在 fixed 层，
	// 子文档元素与 iframe 框同层竞争（面积小者赢）。
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
								// HitTestResult 递归进子 Frame）。同层
								// （order == bestOrder）时无条件覆盖——内容框
								// 面积与 iframe 元素面积相等（border 0 时），
								// 用 area < bestArea 严格比较会漏掉覆盖（fixed
								// iframe 点击永远命中 iframe 框而非子文档元素）。
								if *best == nil || order > *bestOrder || order == *bestOrder {
									*best = child
									*bestArea = area
									*bestOrder = order
								}
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
	// Descend into children (scroll-offset aware like the normal walk).
	// ★ PresentedBoxScrollOffset（已渲染帧快照）：所见即所点（时序偏移修复）。
	childX, childY := x, y
	if rv != nil {
		if box := asRenderBox(o); box != nil {
			sx, sy := rv.PresentedBoxScrollOffset(box)
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


// descendantOf reports whether el is a strict DOM descendant of anc
// (walking el's parent chain hits anc). Used by the hit-test pick rule:
// a deeper element that contains the point must win over its ancestor
// even if its bounding-box area is LARGER — e.g. a menu button whose
// height (30px) exceeds its flex container .menubar (29px) by a pixel:
// area-pick alone would resolve to the container (no listeners) and the
// click bubbles to a parent handler, so the menu never opens. Browsers
// always resolve to the deepest element, not the smallest-area one.
func descendantOf(el, anc *dom.Element) bool {
	if el == nil || anc == nil {
		return false
	}
	for n := el.ParentNode(); n != nil; n = n.ParentNode() {
		if e, ok := n.(*dom.Element); ok && e == anc {
			return true
		}
	}
	return false
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
	inBounds := true
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
		if ow > 0 && oh > 0 {
			inBounds = x >= ox && y >= oy && x < ox+ow && y < oy+oh
			if !inBounds {
				// ★ html/body 视口背景盒不拦截：absolute 子元素可溢出
				// 塌陷的 html/body（html 高度=内容高而非视口高——裸
				// position:absolute 控件不撑开文档），出界即跳会漏命
				// （absolute 输入框在 html 高度塌陷时不可点击/穿透）。
				// 浏览器 hit-test 语义：html/body 是背景盒，不裁剪其
				// 溢出子内容（无 overflow 裁剪时）。
				if el, isEl := o.Node().(*dom.Element); isEl &&
					(el.LocalName() == "html" || el.LocalName() == "body") {
					inBounds = true // pass-through：跳过自身候选，继续下钻
				} else {
					return // outside this box's bounds
				}
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
	if ok && ow > 0 && oh > 0 && inBounds {
		if attrName != "" {
			node := o.Node()
			if el, isEl := node.(*dom.Element); isEl {
				if val := el.GetAttribute(attrName); val != "" {
					area := ow * oh
					// ★ 后代优先：el 是已选 best 的 DOM 后代时无条件替换——
					//   深度 > 面积（浏览器 hit-test 语义：命中最深元素）。
					if !pointerEventsNone(o) && (*best == nil || area < *bestArea || descendantOf(el, *best)) {
						*best = el
						*bestArea = area
					}
				}
			}
		} else {
			node := o.Node()
			if el, isEl := node.(*dom.Element); isEl {
				area := ow * oh
				if !pointerEventsNone(o) && (*best == nil || area < *bestArea || descendantOf(el, *best)) {
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
	// ★ PresentedBoxScrollOffset（已渲染帧快照）：所见即所点（时序偏移修复）。
	childX, childY := x, y
	if rv != nil {
		if box := asRenderBox(o); box != nil {
			sx, sy := rv.PresentedBoxScrollOffset(box)
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
