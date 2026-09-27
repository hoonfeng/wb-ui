// Translation of: Source/WebCore/rendering/RenderView.cpp
package rendering

import (
	"fmt"
	"log"
	"strings"

	"wb-ui/engine/dom"
	"wb-ui/engine/debugenv"
	"wb-ui/engine/html5"
	"wb-ui/engine/layout"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
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
	dirtyRect   Rect
	scrollOffsetX, scrollOffsetY float64
	// scrollOffsets stores per-box scroll offsets for overflow:scroll/auto.
	// Keyed by the DOM node (NOT the RenderBox pointer): the render tree is
	// rebuilt frequently (hover :style changes, DOM mutations → brand-new
	// RenderBox instances), so pointer keys go stale and a wheel/thumb
	// offset written against the old box would never be seen by paint of
	// the new box — "scrollbar thumb moves but content does not". DOM nodes
	// survive rebuilds; paint/hit-test/scrollbar code reads by box.Node().
	boxScrollOffsets map[dom.Node]graphics.Point

	// presentedBoxScrollOffsets 是「已渲染帧」的 box 滚动偏移快照：
	// Paint 开始时从 boxScrollOffsets 拷贝（该帧将用这些偏移绘制，
	// 也就是用户此刻在屏幕上看到的滚动状态）。命中测试（HitTest 族）
	// 一律读快照而非实时值——渲染节流（configwin 50ms≈20fps）下滚轮
	// 更新偏移后屏幕仍停留在旧帧，若命中解析用实时值，点击将按「滚动
	// 后的位置」解释，与用户所见错开一个滚动量（「滚动后点颜色行，
	// 生效位置偏移——不是绝对出现但会触发」根因：滚轮后快速点击时
	// 视觉帧未更新）。快照语义=所见即所点；下一帧 Paint 完成时快照
	// 同步到新偏移，此后命中与视觉一致。键与 boxScrollOffsets 相同
	//（DOM 节点，跨渲染树重建存活）。
	presentedBoxScrollOffsets map[dom.Node]graphics.Point

	// nodeRenderMap maps DOM nodes to their corresponding RenderObject.
	// Populated during syncGeometry() so hit-test and scroll container lookup
	// can go from DOM element → RenderBox without O(n) tree traversal.
	nodeRenderMap map[dom.Node]RenderObject

	// cursorX/cursorY track the last known cursor position in CSS pixels,
	// used by paint code for scrollbar hover highlighting.
	cursorX, cursorY float64

	// resolver holds the style resolver used to build this tree; paint code
	// (e.g. PaintSelection for ::selection colors) reads it lazily.
	resolver *style.Resolver

	// imageLoader 是本文档的图片资源接线（宿主按运行模式提供，见
	// image_resource.go）。paint 期间由 Paint 入口设为「当前 loader」，
	// loadBackgroundImage 据此解析 URL 并取字节——宿主 ResourceResolver
	// 优先、UI 库模式拒绝网络。由 RenderTreeBuilder 在重建渲染树时重新
	// 赋值（每次 Build 都是新的 RenderView）。
	imageLoader ImageResourceLoader

	// boxContentSizeCache 缓存 BoxContentSize 的结果（键为 RenderBox 指针）。
	//
	// ★ 为什么需要：CM6 的 scroll handler 每轮滚动会连续读
	//   scrollHeight/clientHeight/scrollWidth/clientWidth，每次都经 webkit 的
	//   几何桥（getElementScrollMetrics → forceLayout + BoxContentSize），而
	//   BoxContentSize 是全子树递归（且对每个 overflow 子盒再递归一次）。
	//   3 轮滚动窗口的 CPU profile 实测该路径占 89.62%（BoxContentSize 递归
	//   自身 76.82% flat），是滚动 1.8s 的唯一大头。
	//
	// 失效：syncGeometry()（每次布局后的几何刷新）与 ApplyTextChange()（文本段
	// 原地更新）。渲染树重建会整体换新 RenderView，缓存随旧对象一同丢弃，
	// 因此不存在跨重建的陈旧项。
	boxContentSizeCache map[*RenderBox][2]float64
}

// SetResolver attaches the style resolver used to build the render tree.
func (v *RenderView) SetResolver(r *style.Resolver) { v.resolver = r }

// Resolver returns the attached style resolver (may be nil).
func (v *RenderView) Resolver() *style.Resolver { return v.resolver }

// SetImageLoader attaches the host image-resource wiring (may be nil: the
// built-in data:/file/http behavior is used then).
func (v *RenderView) SetImageLoader(l ImageResourceLoader) { v.imageLoader = l }

// ImageLoader returns the attached image-resource wiring (may be nil).
func (v *RenderView) ImageLoader() ImageResourceLoader { return v.imageLoader }

func NewRenderView(doc *dom.Document, st *style.ComputedStyle) *RenderView {
	rv := &RenderView{
		document:         doc,
		boxScrollOffsets: make(map[dom.Node]graphics.Point),
		nodeRenderMap:    make(map[dom.Node]RenderObject),
	}
	rv.initBase(rv, doc, st)
	rv.compositor = NewRenderLayerCompositor(rv)
	return rv
}

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

// SetBoxScrollOffset stores a scroll offset for an overflow:scroll box.
// Keyed by DOM node so the offset survives render-tree rebuilds (see the
// boxScrollOffsets field comment).
func (v *RenderView) SetBoxScrollOffset(box *RenderBox, x, y float64) {
	if box == nil {
		return
	}
	if v.boxScrollOffsets == nil {
		v.boxScrollOffsets = make(map[dom.Node]graphics.Point)
	}
	if debugenv.Enabled("WB_SCROLL_DEBUG") {
		if el, ok := box.Node().(*dom.Element); ok {
			log.Printf("[scroll/set] BoxScrollOffset %s → (%.1f, %.1f)", el.LocalName(), x, y)
		}
	}
	v.boxScrollOffsets[box.Node()] = graphics.Point{X: x, Y: y}
	// ★ 滚动偏移变化必须标记全脏：paint 的 dirty-rect 检查（intersects）
	// 用未 translate 的绝对坐标判断对象是否在脏区内。滚动后新进入
	// 视口的内容（绝对坐标仍在旧视口下方）会被误判为"不在脏区"而
	// 跳过绘制 → "滚动条 thumb 动了、内容却空白"或"滚上来后内容
	// 不显示"。与 SetScrollOffset（页面级）一致，每次 box 滚动都
	// MarkAllDirty，下一帧全量重绘。
	if v.viewWidth > 0 && v.viewHeight > 0 {
		v.MarkAllDirty()
	}
}

// ClampBoxScrollOffsets 把每个滚动容器的偏移裁进当前合法范围
// [0, max]（CSSOM View 的滚动上限）。必须在**布局完成之后、绘制之前**
// 调用：容器（viewport）与内容的尺寸随时可能变化——折叠侧栏、展开面板、
// 消息增删——而 scrollTop 是按**旧几何**写入并存放的，不重新裁剪就会
// 停在旧位置。
//
// ★ 实测根因（自主模式「监督者收缩后下一步推荐不跟随下移」）：消息区
//   滚动容器可视高 443→801（监督者面板 320→34 折叠），内容高不变 →
//   maxScroll 变小；scrollTop 仍是旧值 → 内容被多 translate 约 300px，
//   引导卡停在 y312..443 不动，而容器底已经到 y801。浏览器在每次 layout
//   后对每个 scrollable 容器做同样的事（Blink 的
//   ScrollableArea::clampScrollPositionAfterLayout / SetScrollOffset 上限
//   裁剪），这也是「容器变大后底部内容自动跟上来」的来源。
//
// 轴判定复用 ScrollRange（与 scrollTop 赋值、滚动条几何同一口径）。
// ★ 只在**该轴确实可滚**（can* = true）时裁剪上下界；不可滚的轴保持原值
//   ——内容尺寸/可滚性判定并不覆盖所有容器（表单控件的内部文本滚动、
//   overflow 判定瞬态），一律归零会把用户/JS 设定的滚动位置清零
//   （vscroll_test「textarea 滚动 3 行后 Paint 不得清掉偏移」即此情形）。
//   本函数只做「容器几何变化后把越界偏移拉回合法范围」这一件事，
//   不改变引擎原本「不裁剪」的行为边界。
func (v *RenderView) ClampBoxScrollOffsets() {
	if v == nil || len(v.boxScrollOffsets) == 0 {
		return
	}
	changed := false
	for node, p := range v.boxScrollOffsets {
		box := v.FindRenderBoxForNode(node)
		if box == nil {
			// 盒子当前不在渲染树（display:none、树已重建）：保留偏移。
			continue
		}
		if _, _, tw, th := boxViewAndContent(v, box); tw == 0 && th == 0 {
			// 内容尺寸测不到（子树未布局/空内容几何瞬态）：不裁剪。
			continue
		}
		maxX, maxY, canX, canY := ScrollRange(v, box)
		nx, ny := float64(p.X), float64(p.Y)
		if canX {
			if nx < 0 {
				nx = 0
			}
			if nx > maxX {
				nx = maxX
			}
		}
		if canY {
			if ny < 0 {
				ny = 0
			}
			if ny > maxY {
				ny = maxY
			}
		}
		if nx == p.X && ny == p.Y {
			continue
		}
		v.boxScrollOffsets[node] = graphics.Point{X: nx, Y: ny}
		changed = true
		if debugenv.Enabled("WB_SCROLL_DEBUG") {
			log.Printf("[scroll/clamp] %T (%.1f,%.1f) → (%.1f,%.1f) max=(%.1f,%.1f) can=(%v,%v)",
				node, float64(p.X), float64(p.Y), nx, ny, maxX, maxY, canX, canY)
		}
	}
	if changed && v.viewWidth > 0 && v.viewHeight > 0 {
		// 偏移变化必须全脏重绘（同 SetBoxScrollOffset 的理由：dirty 判定
		// 用未 translate 的绝对坐标，clamp 会移动内容位置）。
		v.MarkAllDirty()
	}
}

// RestoreScrollOffsetsFrom carries per-box scroll offsets from a previous
// incarnation of the render tree into this one, mapping boxes through their
// DOM nodes. RebuildRenderTree() builds a brand-new tree (new RenderBox
// objects, empty boxScrollOffsets), so without this every rebuild — e.g.
// each keystroke in a textarea — silently reset all vertical scroll to 0,
// then auto-scroll yanked the content to the caret row ("content jumps out
// of view as soon as I type"). Horizontal form-control text scroll lives in
// a per-ELEMENT map and survives rebuilds; this covers the vertical /
// overflow-container path.
func (v *RenderView) RestoreScrollOffsetsFrom(old *RenderView) {
	if old == nil || len(old.boxScrollOffsets) == 0 {
		return
	}
	if v.boxScrollOffsets == nil {
		v.boxScrollOffsets = make(map[dom.Node]graphics.Point)
	}
	// Keyed by DOM node, which survives the rebuild — copy directly.
	for node, p := range old.boxScrollOffsets {
		if node == nil {
			continue
		}
		v.boxScrollOffsets[node] = p
	}
	// ★ 已呈现帧快照同样迁移：渲染树重建后到下一帧 Paint 之间的命中
	// 测试（重建 Trigger 后的交互）读旧快照=旧视觉，保持所见即所点；
	// 不迁移则快照缺失回退实时值——重建恰在滚动后发生时（如点击重建
	// 面板），会瞬间回到「滚动后未渲染」的错误解析。
	for node, p := range old.presentedBoxScrollOffsets {
		if node == nil {
			continue
		}
		if v.presentedBoxScrollOffsets == nil {
			v.presentedBoxScrollOffsets = make(map[dom.Node]graphics.Point)
		}
		v.presentedBoxScrollOffsets[node] = p
	}
}

// ScrollOffsetCount returns the number of boxes with a stored scroll
// offset (diagnostics).
func (v *RenderView) ScrollOffsetCount() int {
	if v == nil || v.boxScrollOffsets == nil {
		return 0
	}
	return len(v.boxScrollOffsets)
}

// FindRenderBoxForNode returns the RenderBox for a given DOM node, or nil

// BoxScrollOffset returns the stored scroll offset for an overflow:scroll box.
func (v *RenderView) BoxScrollOffset(box *RenderBox) (float64, float64) {
	if v.boxScrollOffsets == nil || box == nil || box.Node() == nil {
		return 0, 0
	}
	p, ok := v.boxScrollOffsets[box.Node()]
	if !ok {
		return 0, 0
	}
	return float64(p.X), float64(p.Y)
}

// SnapshotScrollOffsets 把当前 boxScrollOffsets 拷入 presentedBoxScrollOffsets：
// 必须在 Paint 绘制开始前调用（该帧视觉将按这些偏移绘制）。此后命中
// 测试读到的就是当前帧的滚动状态（所见即所点），直到下一帧 Paint。
// ★ 即使 boxScrollOffsets 为空也建立（空）快照：map 存在=画面已呈现
// （偏移皆 0）——若空时提前返回，presented 保持 nil，PresentedBoxScrollOffset
// 会回退实时值，快照机制形同虚设（「滚动后点击偏移」复现测试失败根因）。
func (v *RenderView) SnapshotScrollOffsets() {
	if v.presentedBoxScrollOffsets == nil {
		v.presentedBoxScrollOffsets = make(map[dom.Node]graphics.Point, len(v.boxScrollOffsets))
	} else {
		// 复用 map：先清空再拷贝（避免每帧分配）。
		for k := range v.presentedBoxScrollOffsets {
			delete(v.presentedBoxScrollOffsets, k)
		}
	}
	for k, p := range v.boxScrollOffsets {
		v.presentedBoxScrollOffsets[k] = p
	}
	if debugenv.Enabled("WB_SCROLL_DEBUG") {
		var names []string
		for n, p := range v.presentedBoxScrollOffsets {
			names = append(names, fmt.Sprintf("%T:(%.0f,%.0f)", n, p.X, p.Y))
		}
		log.Printf("[snapshot] presented=%v live=%d", names, len(v.boxScrollOffsets))
	}
}

// PresentedBoxScrollOffset 返回「当前已渲染帧」的 box 滚动偏移（命中
// 测试专用）。快照 map 存在（画面已呈现）时**一律**按快照解析——查不到
// 即该容器未滚动（0,0），**绝不回退实时值**（快照非空但无此键的
// 「滚动后未渲染」窗口内回退实时值会复现偏移）。仅快照 map 为 nil
// （从未渲染过，无视觉可依）才回退实时值。
func (v *RenderView) PresentedBoxScrollOffset(box *RenderBox) (float64, float64) {
	if box == nil || box.Node() == nil {
		return 0, 0
	}
	if v.presentedBoxScrollOffsets == nil {
		// 快照从未建立（该视图从未 Paint 过）：无视觉帧可依，回退实时值。
		return v.BoxScrollOffset(box)
	}
	if p, ok := v.presentedBoxScrollOffsets[box.Node()]; ok {
		return float64(p.X), float64(p.Y)
	}
	// 快照 map 存在但无此键（建快照时该 box 未滚动）：就是 0。
	return 0, 0
}

// ScrollStackOffsetFor 返回 o 的**祖先链**（不含 o 自身）上所有滚动容器
// 的「已呈现」滚动偏移之和。层树递归（hitTestLayer）里每层 owner 的
// box 检查需要把命中点从视口坐标平移成布局坐标——层起点不是从渲染树
// 根连续递归而来，逐层的 childX/childY 补偿丢失了祖先滚动偏移（层
// owner 在滚动容器内时 walkLayerContent 的 owner box 检查直接 miss：
// 「滚动后点弹层/浮层内容穿透到下层」）。绘制端 paintLayerContents 对
// 滚动内容同样按完整祖先链 translate（滚动容器的 translate 包住其
// 内容与子层），命中必须镜像同样的坐标系变换。
func (v *RenderView) ScrollStackOffsetFor(o RenderObject) (float64, float64) {
	var sx, sy float64
	for p := o.Parent(); p != nil; p = p.Parent() {
		if box := asRenderBox(p); box != nil {
			px, py := v.PresentedBoxScrollOffset(box)
			sx += px
			sy += py
		}
	}
	return sx, sy
}

// HasBoxScrollOffset reports whether ANY overflow:scroll/auto box currently
// carries a non-zero scroll offset. Paint uses this to force a full repaint:
// per-box scroll translate moves content into the viewport whose ABSOLUTE
// (un-translated) coordinates still lie outside the dirty rect, so the
// painter's intersects() check would skip it ("scrollbar moves, scrolled-in
// content is blank"). When any box is scrolled, dirty-checking is disabled
// for the frame.
func (v *RenderView) HasBoxScrollOffset() bool {
	if v == nil || len(v.boxScrollOffsets) == 0 {
		return false
	}
	for _, p := range v.boxScrollOffsets {
		if p.X != 0 || p.Y != 0 {
			return true
		}
	}
	return false
}

// FindRenderBoxForNode returns the RenderBox for a given DOM node, or nil
func (v *RenderView) FindRenderBoxForNode(n dom.Node) *RenderBox {
	if v.nodeRenderMap == nil || n == nil {
		return nil
	}
	ro, ok := v.nodeRenderMap[n]
	if !ok {
		return nil
	}
	return asRenderBox(ro)
}

// FindRenderObjectForNode returns the raw RenderObject for a given DOM node
// (not coerced to RenderBox). Inline elements (RenderInline, e.g. CM6 语法
// 高亮 span) do not generate a CSS box so asRenderBox returns nil for them,
// but they DO carry a layout box with geometry — callers like
// bindings.GetElementBoxRect need the raw object to read inline geometry.
func (v *RenderView) FindRenderObjectForNode(n dom.Node) RenderObject {
	if v.nodeRenderMap == nil || n == nil {
		return nil
	}
	ro, ok := v.nodeRenderMap[n]
	if !ok {
		return nil
	}
	return ro
}

// rebuildNodeMap 遍历整棵渲染树，重建 nodeRenderMap（DOM node → RenderObject）。
// 在渲染树构建完成后立即调用（RenderTreeBuilder.Build 末尾），使
// FindRenderObjectForNode / FindRenderBoxForNode 在「布局前」即可 O(1) 定位。
// 此前 nodeRenderMap 只在布局后 syncGeometry 填充，「重建后→布局前」窗口为空，
// 无法支撑 RenderTreeUpdater 的增量渲染树更新（它需要在布局前定位节点）。
// 匿名 wrapper / 伪元素无 DOM node（Node()==nil），跳过不登记。
func (v *RenderView) rebuildNodeMap() {
	if v.nodeRenderMap == nil {
		v.nodeRenderMap = make(map[dom.Node]RenderObject)
	}
	for ro := RenderObject(v); ro != nil; ro = ro.NextInPreOrder() {
		if n := ro.Node(); n != nil {
			v.nodeRenderMap[n] = ro
		}
	}
}

// ApplyTextChange incrementally updates the render tree and layout tree after a
// DOM Text node's data changed, avoiding a full rebuild. It re-syncs the
// RenderText's text (clearing cached segments), re-syncs the corresponding layout
// InlineTextBox's text (the IFC reads InlineTextBox.Text() during relayout), and
// marks the containing block's layout box dirty so the next layout re-lays out
// just that block (incremental layout B prunes clean sibling subtrees).
//
// The caller (page.Frame.ApplyTextChange) must also flag the FrameView as needing
// layout. Returns false when the node has no render object (caller should fall
// back to a full rebuild).
func (v *RenderView) ApplyTextChange(node dom.Node) bool {
	if node == nil {
		return false
	}
	// 文本段被原地更新（不重建渲染树）→ 内容尺寸缓存必须作废。
	v.InvalidateContentSizeCache()
	ro := v.FindRenderObjectForNode(node)
	rt, ok := ro.(*RenderText)
	if !ok {
		return false
	}
	t, ok := node.(*dom.Text)
	if !ok {
		return false
	}
	newText := t.Data()
	rt.SetText(newText)

	blockRO := containingBlockForText(rt)
	if blockRO == nil {
		return false
	}
	blockLB := blockRO.LayoutBox()
	if blockLB == nil {
		return false
	}
	syncInlineTextBoxText(blockLB, node, newText)
	blockLB.MarkDirty()
	return true
}

// containingBlockForText walks up from a RenderText to the first RenderBlockFlow
// (the inline formatting context container). Anonymous wrappers are traversed.
func containingBlockForText(rt *RenderText) RenderObject {
	for cur := RenderObject(rt); cur != nil; cur = cur.Parent() {
		if cur.IsRenderBlockFlow() {
			return cur
		}
	}
	return nil
}

// syncInlineTextBoxText finds the InlineTextBox backed by the given DOM node within
// the block's layout subtree and updates its text (matching by node identity, added
// in the node-reference change). The IFC reads InlineTextBox.Text() during relayout,
// so this must be synced before the block is re-laid out.
func syncInlineTextBoxText(lb *layout.ElementBox, node dom.Node, newText string) {
	for _, c := range lb.Children() {
		switch t := c.(type) {
		case *layout.InlineTextBox:
			if t.Node() == node {
				t.SetText(newText)
			}
		case *layout.ElementBox:
			syncInlineTextBoxText(t, node, newText)
		}
	}
}

// FindScrollContainerForNode walks up from node (through DOM ancestors)
// looking for the first element whose RenderBox has overflow:scroll or
// overflow:auto on EITHER axis. The painter's scrollbar gating
// (needsScrollbars) is per-axis (overflow-y:auto with content overflow
// draws a vertical scrollbar even when overflow-x stays visible), so a
// container with only overflow-y:auto must be hit-testable as a scroll
// container too — otherwise wheel events over such boxes fall through to
// the FrameView (page maxY=0) and every single-axis scroll container
// becomes unscrollable.
func (v *RenderView) FindScrollContainerForNode(n dom.Node) *RenderBox {
	// ★ WB_SCROLL_DEBUG=1：定点诊断（纯只读，不改变判定逻辑）——打印命中点祖先链上
	//   每个 RenderBox 的 frame / overflow / BoxContentSize / ScrollRange，用于回答
	//   「为什么该容器被判为不可滚」：是「渲染盒高被压成父高」还是「盒未参与内容测量」。
	if debugenv.Enabled("WB_SCROLL_DEBUG") {
		log.Printf("[scroll-dump] --- chain for hit node %s ---", nodeLabel(n))
		for cur := n; cur != nil; cur = cur.ParentNode() {
			box := v.FindRenderBoxForNode(cur)
			if box == nil {
				log.Printf("[scroll-dump]   %s -> NO RenderBox", nodeLabel(cur))
				continue
			}
			cw, ch := v.BoxContentSize(box)
			mx, my, _, _ := ScrollRange(v, box)
			// 注：OverflowX/Y 是 style.OverflowType（int 枚举）→ 用 %v 打印。
			ox, oy := "?", "?"
			if st := box.Style(); st != nil {
				ox = fmt.Sprintf("%v", st.OverflowX)
				oy = fmt.Sprintf("%v", st.OverflowY)
			}
			log.Printf("[scroll-dump]   %-42s frame=(%.0f,%.0f %.0fx%.0f) overflow=(%s,%s) content=(%.0f,%.0f) maxScroll=(%.0f,%.0f)",
				nodeLabel(cur), box.frame.X, box.frame.Y, box.frame.Width, box.frame.Height, ox, oy, cw, ch, mx, my)
		}
	}
	for cur := n; cur != nil; cur = cur.ParentNode() {
		box := v.FindRenderBoxForNode(cur)
		if box == nil {
			continue
		}
		st := box.Style()
		if st == nil {
			continue
		}
		isScroll := st.OverflowX == style.OverflowScroll ||
			st.OverflowX == style.OverflowAuto ||
			st.OverflowY == style.OverflowScroll ||
			st.OverflowY == style.OverflowAuto
		if isScroll {
			// ★ 浏览器 scroll chaining：只有**真正可滚**（内容在该轴超出）的容器才是
			//   滚动目标；仅声明 overflow:auto/scroll 而内容未超出的盒子跳过、继续
			//   向上找可滚祖先（如滚轮落在 .settings-content 而真正可滚的是祖先
			//   .modal-content）。
			//   判定统一走 ScrollRange（与滚动条绘制、scrollTop 赋值同一口径）；
			//   它依赖的 BoxContentSize 已修正「无差别跳过 overflow 子盒子树」的
			//   过度剪裁，因此结论与 DOM 的 scrollHeight/clientHeight 语义一致。
			maxX, maxY, _, _ := ScrollRange(v, box)
			if maxY > 0.5 || maxX > 0.5 {
				return box
			}
			continue
		}
	}
	return nil
}

// nodeLabel 诊断用短标签：元素 → tag.class（首类），其它节点 → NodeName。
func nodeLabel(n dom.Node) string {
	if n == nil {
		return "(nil)"
	}
	if el, ok := n.(*dom.Element); ok {
		if cls := el.ClassName(); cls != "" {
			return el.LocalName() + "." + strings.SplitN(cls, " ", 2)[0]
		}
		return el.LocalName()
	}
	return n.NodeName()
}

// HitTestScrollContainer hit-tests the render tree at (x, y) and walks up
// to find the nearest scrollable ancestor RenderBox. Returns nil if no
// scroll container is found.
func (v *RenderView) HitTestScrollContainer(x, y float64) *RenderBox {
	el := HitTest(v, x, y, "")
	if el == nil {
		return nil
	}
	// ★ iframe 子文档元素：点击点在子 Frame 内。滚动容器应在子 Frame 的
	// RenderView 里查找（子文档元素不在主渲染树，FindScrollContainerForNode
	// 查不到）。坐标系已由 hit-test 下钻记录（lastDive：子 Frame 视图 +
	// 子坐标）。若子 Frame 里也没有滚动容器，返回 nil——不继续向主文档
	// 找（浏览器 iframe 边界语义：鼠标在 iframe 上时滚动只作用于子文档）。
	if od := el.OwnerDocument(); od != nil && od != v.Document() {
		if lastDive.ok && lastDive.sub != nil && lastDive.sub.Document() == od {
			sub := lastDive.sub
			lastDive.ok = false // 一次性消费
			if sub.RenderView() != nil {
				if sub.NeedsLayout() {
					sub.LayoutNow()
				}
				return sub.RenderView().HitTestScrollContainer(lastDive.x, lastDive.y)
			}
		}
		return nil
	}
	return v.FindScrollContainerForNode(el)
}

// ScrollTarget 是某点下滚动容器的解析结果。Box 可能属于 iframe 子文档
// （滚动容器在子渲染树里），其偏移表存在子 RenderView 里——子文档绘制
// 时读的是子 RenderView.BoxScrollOffset。滚动事件处理必须用 RV（拥有
// 偏移表的 RenderView）读写偏移，否则主 rv 查不到/写入不生效。
type ScrollTarget struct {
	RV  *RenderView
	Box *RenderBox
}

// ScrollTargetAt 解析 (x, y) 下的滚动容器及其所属 RenderView。滚轮/键盘
// 滚动写入偏移时用 tgt.RV（iframe 内滚动 = 子 Frame 的 RenderView）而非
// 主视图——这是「app 层滚动事件路由到子 Frame」的接入点。
func (v *RenderView) ScrollTargetAt(x, y float64) ScrollTarget {
	box := v.HitTestScrollContainer(x, y)
	if box == nil {
		return ScrollTarget{}
	}
	rv := v
	if n := box.Node(); n != nil {
		if od := n.OwnerDocument(); od != nil && od != v.Document() {
			if sub := IFrameContainingFor(od); sub != nil && sub.RenderView() != nil {
				rv = sub.RenderView()
			}
		}
	}
	return ScrollTarget{RV: rv, Box: box}
}

// BoxContentSize returns the content width and height of a scrollable box,
// computed as the bounding box of all render children relative to the
// padding box. Returns (0,0) if no children.
//
// ★ 带缓存：同一「几何版本」内的重复读取直接命中（见 boxContentSizeCache
// 字段的说明）。命中时 O(1)，未命中才走全子树递归。
func (v *RenderView) BoxContentSize(box *RenderBox) (float64, float64) {
	if box == nil {
		return 0, 0
	}
	if c, ok := v.boxContentSizeCache[box]; ok {
		return c[0], c[1]
	}
	w, h := v.boxContentSizeUncached(box)
	if v.boxContentSizeCache == nil {
		v.boxContentSizeCache = make(map[*RenderBox][2]float64, 8)
	}
	v.boxContentSizeCache[box] = [2]float64{w, h}
	return w, h
}

// InvalidateContentSizeCache 丢弃 BoxContentSize 的缓存结果。任何会改变
// 「子树几何 / 文本段」的操作之后必须调用（syncGeometry 与 ApplyTextChange
// 已内置调用）；滚动偏移变化不影响内容尺寸，无需失效。
func (v *RenderView) InvalidateContentSizeCache() { v.boxContentSizeCache = nil }

// boxContentSizeUncached 是 BoxContentSize 的原始实现（全子树递归）。内部对
// overflow 子盒的递归调用仍走带缓存的 BoxContentSize，因此一次遍历即可把
// 沿途所有子盒的结果都填进缓存。
func (v *RenderView) boxContentSizeUncached(box *RenderBox) (float64, float64) {
	pb := box.PaddingBoxRect()
	var maxRight, maxBottom float64
	found := false
	// ★ Custom recursion that SKIPS subtrees of overflow-clipping containers
	// (auto/scroll/hidden): their clipped content (e.g. .project-section's
	// file tree reaching y=2080) must not inflate an ANCESTOR's content size,
	// or sidebar-content grows a spurious scrollbar on top of the container's
	// own one ("three scrollbars" / hover-background-covers-scrollbar).
	var walk func(o RenderObject)
	walk = func(o RenderObject) {
		if o == nil {
			return
		}
		if cb := asRenderBox(o); cb != nil {
			// Fixed-position boxes are viewport-anchored and NEVER contribute
			// to an ancestor's scroll size (CSS 2.1 §10.1). Without this a
			// dialog overlay (position:fixed; inset:0; 1280px wide) inside
			// file-explorer inflates sidebar-content's content width to
			// 1280-48=1232px → spurious horizontal scrollbar over the whole
			// sidebar after clicking "新建工作区".
			if st := cb.Style(); st != nil && st.Position == style.PositionFixed {
				return
			}
			if r := cb.frame.X + cb.frame.Width; r > maxRight {
				maxRight = r
			}
			if b := cb.frame.Y + cb.frame.Height; b > maxBottom {
				maxBottom = b
			}
			found = true
			if st := cb.Style(); st != nil && overflowClipsContentStyle(st) {
				// ★ 只有当**该子盒自身真的可滚**（内容超出它自己的可视区）时，
				//   它的子树才不贡献给外层的滚动区域（CSSOM View：内层滚动容器
				//   自己滚动其溢出内容）。若内层只是声明了 overflow:auto 而内容
				//   并不超出它自己（或它的高由内容撑开），这些溢出内容属于
				//   **外层**滚动区域，必须继续递归 —— 否则外层容器的内容尺寸会
				//   塌成可视高：gou-ide 设置面板 .modal-content（子盒
				//   .settings-content 声明 overflow:auto）实测 DOM 侧
				//   scrollHeight−clientHeight = 422（内容明确超出），而渲染层
				//   ScrollRange 返回 0 → 既不滚动、也不绘滚动条（两套口径不一致）。
				if iw, ih := v.BoxContentSize(cb); iw > cb.frame.Width+0.5 || ih > cb.frame.Height+0.5 {
					return // 内层自身可滚：其溢出内容不计入外层的滚动区域
				}
			}
		}
		if rt, ok := o.(*RenderText); ok {
			for _, seg := range rt.Segments() {
				if r := seg.X + seg.Width; r > maxRight {
					maxRight = r
				}
				if b := seg.Y + seg.Height; b > maxBottom {
					maxBottom = b
				}
				found = true
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	for c := box.FirstChild(); c != nil; c = c.NextSibling() {
		walk(c)
	}
	_ = pb

	// overflowClipsContentStyle reports whether either overflow axis clips content
	// (auto/scroll/hidden). BoxContentSize skips clipped subtrees so an ancestor's
	// content size never includes a scroll container's overflowing children.
	// (Defined below; see also engine/page/frameview.go updateContentSize for the same
	// rule at frame level.)

// Form controls (input/textarea) carry their text in value/textContent,
	// not as render-tree children — measure it so scrollbars appear when the
	// text overflows (a pre-mode textarea scrolls horizontally, a long input
	// scrolls too). The scroll extent must at least cover the control.
	if el, ok := box.Node().(*dom.Element); ok {
		local := el.LocalName()
		if local == "textarea" || local == "input" {
			var text string
			if local == "textarea" {
				text = el.TextContent()
			} else {
				text = el.GetAttribute("value")
			}
			if st := box.Style(); st != nil {
				font := toGraphicsFont(st)
				lineH := 0.0
				// Content-box viewport padding (scrollWidth/scrollHeight
				// include the padding — a scrolled control must show the
				// padding at the far edge like a browser).
				padL := lengthValue(st.PaddingLeft)
				padR := lengthValue(st.PaddingRight)
				padT := lengthValue(st.PaddingTop)
				padB := lengthValue(st.PaddingBottom)
				if padL < 0 {
					padL = 0
				}
				if padR < 0 {
					padR = 0
				}
				if padT < 0 {
					padT = 0
				}
				if padB < 0 {
					padB = 0
				}
				if text != "" {
					lines := strings.Split(text, "\n")
					maxW := 0.0
					for i, line := range lines {
						w := graphics.MeasureText(font, line)
						if w > maxW {
							maxW = w
						}
						if i == 0 {
							lineH = cssControlLineHeight(st, font.Size)
						}
					}
					// ★ pre-wrap（textarea UA 默认）会软换行：长行折行后行宽
					// ≤ contentW，横向不会真正溢出。直接用整行未折行宽会把
					// 略超 contentW 的行误判为横向溢出 → 显示不该出现的横向
					// 滚动条（浏览器软换行下不显示横向滚动条）。仅
					// wrapModeNone（pre/nowrap/wrap=off）保留整行宽——那才是
					// 真横向溢出。input 是单行控件，横向滚动本就正常，不受限。
					contentW := pb.Width - padL - padR
					if contentW < 1 {
						contentW = 1
					}
					mode := textareaWrapMode(st, el)
					if local == "textarea" && mode != wrapModeNone && maxW > contentW {
						maxW = contentW
					}
					// Horizontal extent = left padding + text + right padding.
					if padL+maxW+padR > maxRight-pb.X {
						maxRight = pb.X + padL + maxW + padR
					}
					if local == "textarea" && lineH > 0 {
						// Vertical extent must count SOFT-WRAPPED rows (a
						// pre-wrap textarea wraps long lines into several
						// visual rows), not just hard '\n' breaks — otherwise
						// the scrollbar's total height / thumb ratio is too
						// small and long text can't scroll far enough.
						wrapped := wrapTextAreaLines(text, font, contentW, mode)
						rows := float64(len(wrapped))
						if padT+rows*lineH+padB > maxBottom-pb.Y {
							maxBottom = pb.Y + padT + rows*lineH + padB
						}
					}
				}
				// Form controls always contribute their (possibly empty)
				// content so needsX tests compare against the content-box
				// viewport, never fall into the no-children fallback below.
				found = true
			}
		}
	}

	// No content at all: report zero extent. Callers compare against the
	// content-box viewport, so an empty box must NOT claim the padding-box
	// size (that would spuriously enable scrollbars on padding alone).
	if !found {
		return 0, 0
	}
	cw := maxRight - pb.X
	ch := maxBottom - pb.Y
	if cw < 0 {
		cw = 0
	}
	if ch < 0 {
		ch = 0
	}
	return cw, ch
}

// overflowClipsContentStyle reports whether either overflow axis clips content
// (auto/scroll/hidden). BoxContentSize skips clipped subtrees so an ancestor's
// content size never includes a scroll container's overflowing children
// (same rule as engine/page/frameview.go updateContentSize).
func overflowClipsContentStyle(st *style.ComputedStyle) bool {
	if st == nil {
		return false
	}
	return st.OverflowX == style.OverflowHidden ||
		st.OverflowX == style.OverflowAuto ||
		st.OverflowX == style.OverflowScroll ||
		st.OverflowY == style.OverflowHidden ||
		st.OverflowY == style.OverflowAuto ||
		st.OverflowY == style.OverflowScroll
}

// walkRenderChildren recursively visits all descendants of root.
func walkRenderChildren(root RenderObject, fn func(RenderObject)) {
	for c := root.FirstChild(); c != nil; c = c.NextSibling() {
		fn(c)
		walkRenderChildren(c, fn)
	}
}

// ScrollbarHit describes which scrollbar element was hit at a given point.
type ScrollbarHit struct {
	Box *RenderBox
	// RV 是滚动条所属的 RenderView。iframe 子文档的滚动条属于子 Frame 的
	// RenderView（偏移表/几何存在子 rv）——宿主读写偏移、计算 thumb 几何
	// 必须用 RV 而非主视图（与 ScrollTarget.RV 同一语义）。
	RV *RenderView
	IsVThumb    bool // vertical thumb hit
	IsVTrack    bool // vertical track (non-thumb, non-arrow area)
	IsVUpArrow  bool // vertical up arrow button
	IsVDownArrow bool // vertical down arrow button
	IsHThumb    bool // horizontal thumb hit
	IsHTrack    bool // horizontal track (non-thumb, non-arrow area)
	IsHLeftArrow  bool // horizontal left arrow button
	IsHRightArrow bool // horizontal right arrow button
	IsCorner    bool // corner overlap area
}

// HitTestScrollbar checks whether (x,y) hits a scrollbar thumb or track
// of any scrollable box in the render tree. Returns nil if nothing hit.
// 命中 iframe 内容时递归进子 Frame（子文档滚动条由子 RenderView 判定），
// 返回的 ScrollbarHit.RV 是滚动条所属的 RenderView。
func HitTestScrollbar(rv *RenderView, x, y float64) *ScrollbarHit {
	if rv == nil {
		return nil
	}
	el := HitTest(rv, x, y, "")
	if el == nil {
		return nil
	}
	// ★ iframe 子文档元素：点击点在子 Frame 内，滚动条判定用子坐标在子
	// Frame 的 RenderView 里递归（子文档滚动容器不在主渲染树）。坐标系
	// 已由 hit-test 下钻记录（lastDive：子 Frame 视图 + 子坐标）。
	if od := el.OwnerDocument(); od != nil && od != rv.Document() {
		if lastDive.ok && lastDive.sub != nil && lastDive.sub.Document() == od &&
			lastDive.sub.RenderView() != nil {
			sub := lastDive.sub
			lastDive.ok = false // 一次性消费
			if sub.NeedsLayout() {
				sub.LayoutNow()
			}
			return HitTestScrollbar(sub.RenderView(), lastDive.x, lastDive.y)
		}
		return nil
	}
	h := hitTestScrollbarInner(rv, x, y)
	if h != nil {
		h.RV = rv
	}
	return h
}

// hitTestScrollbarInner is the single-RenderView scrollbar hit-test body.
func hitTestScrollbarInner(rv *RenderView, x, y float64) *ScrollbarHit {
	// First find the deepest element at (x,y), then find its scroll container.
	el := HitTest(rv, x, y, "")
	if el == nil {
		return nil
	}
	scrollBox := rv.FindScrollContainerForNode(el)
	if scrollBox == nil {
		return nil
	}
	// Check if (x,y) is within the scrollbar area of scrollBox.
	st := scrollBox.Style()
	if st == nil {
		return nil
	}
	pb := scrollBox.PaddingBoxRect()
	scrollW := scrollbarWidthFor(st)
	arrowSize := 12.0
	arrowGap := 5.0
	if scrollW <= 0 || pb.Width <= scrollW*2 || pb.Height <= scrollW*2 {
		return nil
	}
	webkit := webkitCustomScrollbar(st)

	// Content size via BoxContentSize — this handles form controls
	// (input/textarea) whose text lives in value/textContent instead of
	// render-tree children, so their scrollbars are hit-testable too.
	cw, ch := rv.BoxContentSize(scrollBox)
	totalW := cw
	totalH := ch

	// Scroll viewport is the CONTENT box (padding-box minus padding), the
	// same viewport the paint code uses for thumb geometry.
	padL := lengthValue(st.PaddingLeft)
	padR := lengthValue(st.PaddingRight)
	padT := lengthValue(st.PaddingTop)
	padB := lengthValue(st.PaddingBottom)
	if padL < 0 {
		padL = 0
	}
	if padR < 0 {
		padR = 0
	}
	if padT < 0 {
		padT = 0
	}
	if padB < 0 {
		padB = 0
	}
	// clientWidth/clientHeight include padding (CSSOM) — see paint gating in
	// renderpipeline.go. Comparing against the content-box height makes every
	// padded overflow:auto container spuriously scrollable.
	contentW := pb.Width
	if contentW < 1 {
		contentW = 1
	}
	contentH := pb.Height
	if contentH < 1 {
		contentH = 1
	}
	needsV := (st.OverflowY == style.OverflowScroll || (st.OverflowY == style.OverflowAuto && totalH > contentH)) && st.OverflowY != style.OverflowHidden
	needsH := (st.OverflowX == style.OverflowScroll || (st.OverflowX == style.OverflowAuto && totalW > contentW)) && st.OverflowX != style.OverflowHidden

	// Vertical scrollbar rect
	vx := pb.X + pb.Width - scrollW
	vy := pb.Y
	vh := pb.Height
	if needsH {
		vh -= scrollW
	}

	// Horizontal scrollbar rect
	hx := pb.X
	hy := pb.Y + pb.Height - scrollW
	hw := pb.Width
	if needsV {
		hw -= scrollW
	}

	// ★ textarea resize 手柄让位：CSS resize:vertical/both 的 textarea 右下角
	// 15px 是拖拽手柄区（浏览器中滚动条 track 底部不覆盖它）。若滚动条矩形
	// 覆盖手柄区，Press 的 scrollbar hit 先于 resize 检测命中 → 手柄永远
	// 拖不动（子 agent 编辑框内容溢出有垂直滚动条时必现）。按 Chromium
	// 语义：垂直滚动条底端让位 15px、水平滚动条右端让位 15px。
	if el2, ok := scrollBox.Node().(*dom.Element); ok && el2.LocalName() == "textarea" {
		if st := scrollBox.Style(); st != nil && ResizeModeOf(st) != 0 {
			const rHandle = 15.0
			if needsV {
				vh -= rHandle
				if vh < 1 {
					vh = 1
				}
			}
			if needsH {
				hw -= rHandle
				if hw < 1 {
					hw = 1
				}
			}
		}
	}

	// Get scroll offsets for thumb position calculations. Form controls
	// scroll their text through per-element FormControlTextScroll, not
	// BoxScrollOffset — mirror that so the thumb position matches paint.
	sx, sy := rv.BoxScrollOffset(scrollBox)
	if el2, ok := scrollBox.Node().(*dom.Element); ok {
		if el2.LocalName() == "textarea" || el2.LocalName() == "input" {
			sx = FormControlTextScroll(el2)
		}
	}

	// Corner: check first so it takes priority over individual bar hits.
	if needsV && needsH {
		cx := pb.X + pb.Width - scrollW
		cy := pb.Y + pb.Height - scrollW
		if x >= cx && x <= cx+scrollW && y >= cy && y <= cy+scrollW {
			return &ScrollbarHit{Box: scrollBox, IsCorner: true}
		}
	}

	// ── Vertical scrollbar hit test ──
	if needsV && x >= vx && x <= vx+scrollW && y >= vy && y <= vy+vh {
		h := &ScrollbarHit{Box: scrollBox}
		if !webkit && vh <= arrowSize*2 {
			return nil
		}
		if !webkit {
			upBtnY := vy
			dnBtnY := vy + vh - arrowSize

			// Check arrow buttons first.
			if y >= upBtnY && y < upBtnY+arrowSize {
				h.IsVUpArrow = true
				return h
			}
			if y >= dnBtnY && y < dnBtnY+arrowSize {
				h.IsVDownArrow = true
				return h
			}
		}

		// Track (non-thumb area) or thumb — same geometry as the painter
		// (shared ScrollbarMetrics) so a press lands on the drawn thumb.
		h.IsVTrack = true
		if m := VerticalScrollbarMetrics(rv, scrollBox); m.OK {
			syRatio := sy / m.MaxScroll
			if syRatio < 0 {
				syRatio = 0
			}
			if syRatio > 1 {
				syRatio = 1
			}
			thumbTrackSpace := m.TrackLen - m.ThumbLen
			thumbY := vy + syRatio*thumbTrackSpace
			if !webkit {
				thumbY += arrowSize + arrowGap
			}
			if y >= thumbY && y <= thumbY+m.ThumbLen {
				h.IsVThumb = true
			}
		}
		return h
	}

	// ── Horizontal scrollbar hit test ──
	if needsH && y >= hy && y <= hy+scrollW && x >= hx && x <= hx+hw {
		h := &ScrollbarHit{Box: scrollBox}
		if !webkit && hw <= arrowSize*2 {
			return nil
		}
		if !webkit {
			ltBtnX := hx
			rtBtnX := hx + hw - arrowSize

			// Check arrow buttons first.
			if x >= ltBtnX && x < ltBtnX+arrowSize {
				h.IsHLeftArrow = true
				return h
			}
			if x >= rtBtnX && x < rtBtnX+arrowSize {
				h.IsHRightArrow = true
				return h
			}
		}

		// Track or thumb — same geometry as the painter (shared metrics).
		h.IsHTrack = true
		if m := HorizontalScrollbarMetrics(rv, scrollBox); m.OK {
			sxRatio := sx / m.MaxScroll
			if sxRatio < 0 {
				sxRatio = 0
			}
			if sxRatio > 1 {
				sxRatio = 1
			}
			thumbTrackSpace := m.TrackLen - m.ThumbLen
			thumbX := hx + sxRatio*thumbTrackSpace
			if !webkit {
				thumbX += arrowSize + arrowGap
			}
			if x >= thumbX && x <= thumbX+m.ThumbLen {
				h.IsHThumb = true
			}
		}
		return h
	}

	return nil
}

func (v *RenderView) SetViewportSize(w, h float64) {
	if v.viewWidth != w || v.viewHeight != h { v.viewWidth, v.viewHeight = w, h; v.Dirty() }
	// ★ 媒体查询上下文必须跟随视图尺寸：style.Resolver.mediaQueryCtx 是
	// @media 求值的唯一依据，而它默认是零值（0×0）。此前只有 bindings 的
	// MediaQueryContextProvider（JS 侧 matchMedia）拿到真实尺寸，样式解析
	// 侧仍按 0×0 评估——`@media (min-width: 600px)` 恒不匹配、
	// `@media (max-width: 950px)` 恒匹配（0 ≤ 950），即所有宽度/高度媒体
	// 查询都落在错误分支上（fixture viewport-consistency 的
	// `@media (min-height: 900px)` 未命中即是此根因）。
	v.syncMediaQueryViewport()
}

// syncMediaQueryViewport mirrors the view size into the style resolver's media
// query context (the viewport width/height @media is evaluated against).
// Non-size features (color scheme, hover, pointer) keep their own defaults.
func (v *RenderView) syncMediaQueryViewport() {
	if v.resolver == nil {
		return
	}
	v.resolver.SetViewportSize(int(v.viewWidth), int(v.viewHeight))
}

// CursorPos returns the last tracked cursor position in CSS pixels.
func (v *RenderView) CursorPos() (float64, float64) { return v.cursorX, v.cursorY }

// SetCursorPos records the cursor position (CSS pixels) for scrollbar hover highlight.
func (v *RenderView) SetCursorPos(x, y float64) { v.cursorX, v.cursorY = x, y }

func (v *RenderView) RootLayer() *RenderLayer          { return v.rootLayer }

// TopLayerRects 返回渲染层树中「绘制在文档内容之上」的层 owner 的视口
// 矩形：z-index>0 的定位/堆叠层（遮罩 z:999/弹窗 1000/toast 1200/下拉）、
// position:fixed 浮层——镜像 paintLayerTree 的层叠顺序（正 z 层与 fixed
// 恒在文档内容之上绘制）。★ 应用层「外部合成内容」（配置画布预览的挂件
// 像素 blit）在语义上是文档内容层——合成前查询本函数，与弹层矩形相交
// 则跳过/裁剪 → 弹窗/遮罩/下拉对内容的遮挡自动正确（引擎层提供层叠
// 唯一真相，应用无需 JS 探测弹窗、无需维护弹窗类型清单——此前逐 id
// 探测漏掉 dcMask 导致删除弹窗打开时媒体帧覆盖遮罩）。
func (v *RenderView) TopLayerRects() []layout.LayoutRect {
	var out []layout.LayoutRect
	if v.rootLayer == nil {
		return out
	}
	var walk func(layer *RenderLayer)
	walk = func(layer *RenderLayer) {
		if layer == nil {
			return
		}
		if layer.owner != nil && !layer.owner.IsRenderView() {
			// ★ display:none 的层（弹窗/遮罩关闭后层树残留）：painter
			// 不绘制隐藏元素，遮挡查询同样忽略（否则关闭弹窗后矩形
			// 残留——合成层误以为弹层仍遮挡而持续跳过挂件）。
			// 双层过滤：渲染树 style（快）+ DOM 内联 style（层树可能
			// 未随 display:inline 变更重建，DOM 是最新事实）。
			if st := layer.owner.Style(); st != nil && st.Display == style.DisplayNone {
				return // 隐藏子树无任何绘制，整体跳过
			}
			if el, ok := layer.owner.Node().(*dom.Element); ok {
				if stAttr := el.GetAttribute("style"); strings.Contains(stAttr, "display:none") {
					return
				}
			}
			z := layerZIndex(layer)
			isFixed := false
			if st := layer.owner.Style(); st != nil {
				isFixed = st.Position == style.PositionFixed
			}
			if z > 0 || isFixed {
				// owner 的视口矩形（扣除祖先滚动偏移——与元素
				// getBoundingClientRect 语义一致；弹层多为文档级
				// absolute/fixed，无祖先滚动，此处兜底保证正确）。
				x, y, w, h, ok := boxCoords(layer.owner)
				if ok && w > 0 && h > 0 {
					vx, vy := x, y
					for p := layer.owner.Parent(); p != nil; p = p.Parent() {
						if pb := asRenderBox(p); pb != nil {
							sx, sy := v.BoxScrollOffset(pb)
							if sx != 0 || sy != 0 {
								vx -= sx
								vy -= sy
							}
						}
					}
					out = append(out, layout.LayoutRect{X: vx, Y: vy, Width: w, Height: h})
				}
			}
		}
		for child := layer.FirstChild(); child != nil; child = child.NextSibling() {
			walk(child)
		}
	}
	walk(v.rootLayer)
	return out
}
func (v *RenderView) SetRootLayer(l *RenderLayer)       { v.rootLayer = l }
func (v *RenderView) Compositor() *RenderLayerCompositor { return v.compositor }

func (v *RenderView) Layout(state *layout.LayoutState) {
	if state == nil {
		// ★ 复用上一帧的 LayoutState（geometry map 跨帧保留）：布局仍然
		// 全量执行（每个 box 几何被覆盖重算），仅省去每帧新建 map +
		// 全部 box 零值几何的开销（1000+ 节点页面可省数 ms/帧）。
		// viewport 尺寸变化时不能复用（所有 box 宽度可能变），新建。
		// ★ 无论如何必须同步包级全局 viewport（vh/vw/calc 解析用）——
		// 多 WebView 共享该变量：本视图复用 layoutState（viewport 未变）
		// 时若不同步，会残留其他视图（如 260x80 挂件）的 viewport，
		// 本页 calc(100vh - Npx) 按错误 vh 求值 → 高度塌陷/布局错乱。
		layout.SetViewportSize(v.viewWidth, v.viewHeight)
		if v.layoutState != nil && v.layoutState.ViewportWidth == v.viewWidth &&
			v.layoutState.ViewportHeight == v.viewHeight {
			state = v.layoutState
		} else {
			state = layout.NewLayoutState(v.viewWidth, v.viewHeight)
		}
	}
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
	// ★ 常驻滚动条预留的收敛 pass（见 engine/layout/scrollbarreserve.go）：
	//   为什么不在布局（BFC）内判定：.pp-list{flex:1} 这类滚动容器的高度由
	//   父 flex 容器分配，而它的 BFC 收尾时读到的高度还是「内容撑开」的值
	//   （实测 contentH=1287 / viewportH=1295 → 恒判不溢出），flex 压到
	//   638px 发生在 BFC 返回之后。因此判定必须放在这里：几何全部就位后，
	//   用与**滚动条绘制完全相同**的口径（boxViewAndContent + needsScrollbars）
	//   判定，结论写回布局盒的粘性标记；一旦有标记翻转，本轮已算出的子元素
	//   几何即作废，必须立刻按新宽度重排——不能等「下一次布局」：静态页面
	//   布局一次后不会再有下一次，预留将永远不生效。
	//   收敛性：预留只会让内容更窄更高，不会来回翻转，通常一轮即收敛
	//   （上限 3 轮兜底防病态震荡）。
	for pass := 0; pass < 3; pass++ {
		if !v.syncScrollbarReserve() {
			break
		}
		v.RenderBlockFlow.Layout(state)
		v.syncGeometry()
	}
	if v.compositor != nil { v.compositor.UpdateCompositingLayers() }
}

// syncScrollbarReserve 用「滚动条绘制同口径」判定渲染树里每个滚动容器是否
// 需要垂直滚动条，把结论写入布局盒的常驻预留标记（见 engine/layout/
// scrollbarreserve.go）。返回是否有任一标记发生变化（= 需要补跑一轮布局）。
//
// 只处理 overflow-y 为 auto/scroll 的盒（其余盒永远不会为滚动条预留），
// 因此遍历代价集中在真正可能滚动的容器上。
func (v *RenderView) syncScrollbarReserve() bool {
	if v == nil {
		return false
	}
	changed := false
	var walk func(ro RenderObject)
	walk = func(ro RenderObject) {
		if ro == nil {
			return
		}
		if st := ro.Style(); st != nil &&
			(st.OverflowY == style.OverflowAuto || st.OverflowY == style.OverflowScroll) {
			if lb := ro.LayoutBox(); lb != nil {
				if box := asRenderBox(ro); box != nil {
					viewW, viewH, totalW, totalH := boxViewAndContent(v, box)
					needV, _ := needsScrollbars(st, totalW, totalH, viewW, viewH)
					if lb.SetVerticalScrollbarReserved(needV) {
						changed = true
					}
				}
			}
		}
		for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	for rc := v.FirstChild(); rc != nil; rc = rc.NextSibling() {
		walk(rc)
	}
	return changed
}

func (v *RenderView) LayoutState() *layout.LayoutState { return v.layoutState }

func (v *RenderView) syncGeometry() {
	// 布局刚把新的几何写入各 box.frame（含文本段）→ 之前缓存的 BoxContentSize
	// 结果全部作废。
	v.InvalidateContentSizeCache()
	layoutRoot := v.LayoutBox()
	if layoutRoot == nil { return }
	state := v.layoutState
	if state == nil { return }
	if box := asRenderBox(v); box != nil {
		// ★ RenderView 是视口，其几何恒为 viewport 尺寸——不能被 layout
		// root（html 元素）的高度覆盖。反例：iframe 子文档 body 无流内容
		// 时 html 高度为 0，若用 layoutRoot 覆盖则 RenderView 高度变 0，
		// hit-test 在根 box 就被 inBounds 拦截（所有子元素不可命中）。
		if v.IsRenderView() {
			box.frame.X, box.frame.Y = 0, 0
			box.frame.Width, box.frame.Height = v.viewWidth, v.viewHeight
		} else {
			box.frame = state.GeometryForBox(layoutRoot).ToRect()
		}
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

	// Populate node render map for hit-test / scroll container lookup.
	if rv := ro.View(); rv != nil && ro.Node() != nil {
		rv.nodeRenderMap[ro.Node()] = ro
	}

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
				// Expand the wrapper frame to encompass text content.
				// ★ 表格内部盒（单元格/行/表节）例外：它们的尺寸由表格布局算法
				// 决定（列轨道宽），frame 必须严格等于布局几何。含 nowrap 长文本
				// 的单元格在此被撑到文本宽——fixed-table-layout 的
				// "later separate row cannot resize first track" 背景从 50px 画成
				// 277.3px；浏览器中内容只溢出、不改变单元格尺寸。
				if box := asRenderBox(ro); box != nil && len(segs) > 0 && !renderIsTableInternalBox(ro) {
					textRight := segs[0].X + segs[0].Width
					frameRight := box.frame.X + box.frame.Width
					if textRight > frameRight && !renderIsFlexItem(ro) {
						box.frame.Width = textRight - box.frame.X
					}
					textBottom := segs[0].Y + segs[0].Height
					frameBottom := box.frame.Y + box.frame.Height
					if textBottom > frameBottom {
						box.frame.Height = textBottom - box.frame.Y
					}
				}
				break
			}
		}
	}
	syncChildren(ro, lb, state)

	// After children are synced, expand this box's frame to encompass
	// any child text that extends beyond the geometry-based frame. This
	// prevents overflow:hidden from clipping text in flex items whose
	// layout geometry is narrower than actual text content.
	// ★ 表格内部盒同样排除：单元格的 frame 由列轨道与行高决定（见
	// renderIsTableInternalBox），溢出文本不得撑开背景框。
	if box := asRenderBox(ro); box != nil && box.Parent() != nil && !renderIsTableInternalBox(ro) {
		var maxRight, maxBottom float64
		frameRight := box.frame.X + box.frame.Width
		frameBottom := box.frame.Y + box.frame.Height
		for rc := ro.FirstChild(); rc != nil; rc = rc.NextSibling() {
			if rt, ok := rc.(*RenderText); ok {
				segs := rt.Segments()
				if len(segs) > 0 {
					s := segs[0]
					if r := s.X + s.Width; r > maxRight { maxRight = r }
					if b := s.Y + s.Height; b > maxBottom { maxBottom = b }
				}
			}
			if rbf, ok := rc.(*RenderBlockFlow); ok {
				for cc := rbf.FirstChild(); cc != nil; cc = cc.NextSibling() {
					if rt, ok := cc.(*RenderText); ok {
						segs := rt.Segments()
						if len(segs) > 0 {
							s := segs[0]
							if r := s.X + s.Width; r > maxRight { maxRight = r }
							if b := s.Y + s.Height; b > maxBottom { maxBottom = b }
						}
					}
				}
			}
		}
		if maxRight > frameRight {
			if !renderIsFlexItem(ro) {
				box.frame.Width = maxRight - box.frame.X
			}
		}
		if maxBottom > frameBottom {
			box.frame.Height = maxBottom - box.frame.Y
		}
	}
}

// renderIsFlexItem reports whether ro is an in-flow child of a flex container.
// Flex items' frame width must stay at the flex-resolved size (the text may
// overflow and be ellipsized/clipped) — syncOne must NOT widen them back to
// the raw text extent (that made a 238px flex-shrunk .item-name render 260px
// and overflow its item-row).
func renderIsFlexItem(ro RenderObject) bool {
	lb := ro.LayoutBox()
	if lb == nil || lb.Parent() == nil {
		return false
	}
	pcs := lb.Parent().Style()
	if pcs == nil {
		return false
	}
	d := pcs.Display
	return (d == style.DisplayFlex || d == style.DisplayInlineFlex) && !lb.IsAbsolutelyPositioned()
}

// renderIsTableInternalBox 报告渲染对象对应的布局盒是否是表格内部盒（表节/行/
// 单元格）。这类盒的尺寸由表格布局算法决定（列轨道宽、行高），syncOne 不得按文本
// 内容扩展它们的 frame：`table-layout:fixed` 的单元格含 nowrap 长文本时，单元格
// 背景会被撑到文本宽（fixed-table-layout 的 "later separate row cannot resize
// first track"：50px 的轨道画成 277.3px），而浏览器中内容只会溢出单元格。
func renderIsTableInternalBox(ro RenderObject) bool {
	lb := ro.LayoutBox()
	if lb == nil {
		return false
	}
	cs := lb.Style()
	if cs == nil {
		return false
	}
	switch cs.Display {
	case style.DisplayTableCell, style.DisplayTableRow,
		style.DisplayTableRowGroup, style.DisplayTableHeaderGroup,
		style.DisplayTableFooterGroup:
		return true
	}
	return false
}

func syncChildren(parentRO RenderObject, parentLB *layout.ElementBox, state *layout.LayoutState) {
	// Must copy the slice to avoid mutating parentLB.children's backing array.
	// lChildren := parentLB.Children() shares the backing array; any
	// append(lChildren[:i], lChildren[i+1:]...) will overwrite the original
	// array, corrupting the layout tree (e.g. col-left gets replaced by col-right).
	orig := parentLB.Children()
	lChildren := make([]layout.Box, len(orig))
	copy(lChildren, orig)
	for rc := parentRO.FirstChild(); rc != nil && len(lChildren) > 0; rc = rc.NextSibling() {
		// ★ 填 nodeRenderMap：syncOne 只在 ElementBox 匹配时调用，RenderText
		// 等非 box 节点（CM6 行内裸文本 `(`、`)  `）走 rt 分支 SetSegments
		// 却不登记 map → bindings.GetTextBasePos（Range.getClientRects 的
		// 裸文本段位置查询）FindRenderObjectForNode 返回 nil → 子区间 rect
		// 用父 box 左（行首）→「空格多的行」posAtCoords 错乱。
		if rv := rc.View(); rv != nil && rc.Node() != nil {
			rv.nodeRenderMap[rc.Node()] = rc
		}
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
		} else if rt, ok := rc.(*RenderText); ok {
			// RenderText: match either direct InlineTextBox or anonymous-wrapper-wrapped one.
			for i, lc := range lChildren {
				// Case 1: direct InlineTextBox child.
				if tb, ok := lc.(*layout.InlineTextBox); ok && tb.Node() == rt.Node() {
					if len(tb.TextSegments) > 0 {
						segs := make([]InlineTextBox, len(tb.TextSegments))
						for j, s := range tb.TextSegments {
							segs[j] = InlineTextBox{
								Start: s.Start, Len: s.Len,
								X: s.X, Y: s.Y, Width: s.Width, Height: s.Height,
								LineY: s.LineY, LineHeight: s.LineHeight,
							}
						}
						rt.SetSegments(segs)
					}
					lChildren = append(lChildren[:i], lChildren[i+1:]...)
					break
				}
				// Case 2: anonymous ElementBox wrapper around InlineTextBox.
				if childEb, ok := lc.(*layout.ElementBox); ok && childEb.Element() == nil {
					for _, cc := range childEb.Children() {
						if tb, ok := cc.(*layout.InlineTextBox); ok && tb.Node() == rt.Node() {
							if len(tb.TextSegments) > 0 {
								segs := make([]InlineTextBox, len(tb.TextSegments))
								for j, s := range tb.TextSegments {
									segs[j] = InlineTextBox{
										Start: s.Start, Len: s.Len,
										X: s.X, Y: s.Y, Width: s.Width, Height: s.Height,
										LineY: s.LineY, LineHeight: s.LineHeight,
									}
								}
								rt.SetSegments(segs)
							}
							lChildren = append(lChildren[:i], lChildren[i+1:]...)
							break
						}
					}
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
