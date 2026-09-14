// Translation of: Source/WebCore/rendering/updating/RenderTreeBuilder.cpp
//                  Source/WebCore/rendering/updating/RenderTreeBuilder.h
// Completeness: 55%
// Simplifications:
//   - anonymous-block generation for mixed inline/block content follows the same
//     grouping rule as the layout package's buildChildren (consecutive inline children
//     wrapped in an anonymous RenderBlockFlow); the builder does not maintain
//     continuation chains
//   - the style resolver is invoked per-element; no invalidation / recalc-scheduling
//   - replaced elements (img / iframe / video / canvas / input) map to a RenderBox with
//     a Replaced display rather than a dedicated RenderReplaced type
//   - the associated layout tree is built by layout.BuildLayoutTree and linked back to
//     each render object via SetLayoutBox

package rendering

import (
	"log"
	"strconv"
	"strings"
	"time"

	"wb-ui/css"
	"wb-ui/debugenv"
	"wb-ui/dom"
	"wb-ui/layout"
	"wb-ui/style"
)

// RenderTreeBuilder is the Go translation of WebCore::RenderTreeBuilder. It constructs the
// render tree from a DOM tree: for each Element it resolves the ComputedStyle, maps the
// display property to the appropriate RenderObject type and inserts it into the tree.
// display: none suppresses render object creation. Text nodes become RenderText leaves.
type RenderTreeBuilder struct {
	resolver *style.Resolver
}

// NewRenderTreeBuilder constructs a builder that uses the given style resolver.
func NewRenderTreeBuilder(resolver *style.Resolver) *RenderTreeBuilder {
	return &RenderTreeBuilder{resolver: resolver}
}

// Build constructs the render tree for the given document and returns the root
// RenderView. This mirrors RenderTreeBuilder::attachRenderTree() / the initial render
// tree construction entry point. The viewport size defaults to the document's body
// dimensions or 800x600 when unspecified.
func (b *RenderTreeBuilder) Build(doc *dom.Document) *RenderView {
	if doc == nil {
		return nil
	}
	root := doc.DocumentElement()
	if root == nil {
		// Empty document: still create a RenderView so callers can attach later.
		view := NewRenderView(doc, defaultStyle(doc))
		return view
	}
	profile := debugenv.Enabled("WB_REBUILD_PROFILE")
	var t0, t1, t2 time.Time
	if profile {
		t0 = time.Now()
	}
	rootStyle := b.resolveStyle(root)
	view := NewRenderView(doc, rootStyle)
	// Create a render object for the document element itself and attach it as the
	// view's first child, mirroring WebKit where RenderView's child is the <html>
	// render object.
	rootRO := b.createRenderObject(root, rootStyle)
	if rootRO != nil {
		view.AddChild(rootRO, nil)
		b.buildChildren(rootRO, root)
	}
	if profile {
		t1 = time.Now()
	}
	// Build the associated layout tree and link it back.
	b.attachLayoutTree(view, root)
	if profile {
		t2 = time.Now()
	}
	// ★ 提前填充 nodeRenderMap：使 node→RenderObject 在「布局前」即可 O(1)
	//   查找（供 RenderTreeUpdater 增量更新 / FindRenderObjectForNode 使用）。
	//   此前映射只在布局后 syncGeometry 填充，构建后到布局前的窗口为空。
	//   放在层树构建之前，确保层树阶段若需查映射也已就绪。
	view.rebuildNodeMap()
	// Build the layer tree.
	view.compositor.BuildLayerTree(view)
	view.SetRootLayer(view.compositor.RootLayer())
	// ★ 构建时自动挂接 resolver：RenderView 从此自带样式解析服务，
	// 动画系统（@keyframes 查询）按 rv.Resolver() 而非包级全局
	// KeyframesLookup 查找——多 WebView（宿主多挂件/多窗口）不再互相
	// 覆盖关键帧，每个页面动画只认自己文档的 @keyframes。
	view.SetResolver(b.resolver)
	if profile {
		t3 := time.Now()
		log.Printf("[rebuild-profile] buildChildren=%v attachLayout=%v layerTree=%v total=%v",
			t1.Sub(t0), t2.Sub(t1), t3.Sub(t2), t3.Sub(t0))
	}
	return view
}

// buildChildren recursively populates parent's children from the element's DOM children.
// Mixed inline/block siblings are grouped: consecutive inline children are wrapped in
// an anonymous RenderBlockFlow so the block container holds either all-block or
// all-inline children.
// walkComposedChildren 遍历 el 的组合树子节点：<slot> 展开为其分配节点，
// display:contents 元素展开为其子节点（该元素自身不生成渲染对象 ——
// CSS-DISPLAY-3 §2.5：contents 元素的子节点提升到父容器的格式化上下文中，
// 例如 flex 容器里的 contents 元素，其子元素直接成为该容器的 flex item）。
// 与 layout/box.go 的 walkComposedChildren 完全对应：两棵树必须逐节点配对，
// 否则 linkLayoutBoxes 会错位（元素拿不到布局几何、不参与绘制与 hit-test）。
func (b *RenderTreeBuilder) walkComposedChildren(el *dom.Element, emit func(dom.Node)) {
	for c := dom.FirstComposedChild(el); c != nil; c = c.NextSibling() {
		b.walkComposedNode(c, emit)
	}
}

// walkComposedNode 处理单个组合树节点：slot / display:contents 递归展开，
// 其余节点原样 emit（display:none 由调用方过滤，与展开前行为一致）。
func (b *RenderTreeBuilder) walkComposedNode(node dom.Node, emit func(dom.Node)) {
	e, ok := node.(*dom.Element)
	if !ok {
		emit(node)
		return
	}
	if e.LocalName() == "slot" {
		for _, an := range e.AssignedNodes() {
			b.walkComposedNode(an, emit)
		}
		return
	}
	if cs := b.resolveStyle(e); cs != nil && cs.Display == style.DisplayContents {
		b.walkComposedChildren(e, emit)
		return
	}
	emit(node)
}

func (b *RenderTreeBuilder) buildChildren(parent RenderObject, el *dom.Element) {
	// Replaced elements are leaf nodes in the render tree — they have no render
	// tree children. This mirrors WebKit where RenderReplaced / RenderMenuList do
	// not create child renderers. Without this, <select>'s <option> children would
	// produce RenderText nodes whose text gets drawn at wrong positions.
	if isReplacedElement(el.LocalName()) {
		return
	}
	// ::before 伪元素（第一个子节点）。
	if b.resolver != nil {
		if cs, content, ok := b.resolver.ResolvePseudoElement(el, css.PseudoElementBefore); ok && cs.Display != style.DisplayNone {
			parent.AddChild(b.createPseudoObject(cs, content), nil)
		}
	}
	// Flex / grid containers: per CSS (flexbox §4) every element child of a flex
	// container becomes a flex item directly — no anonymous block wrappers are
	// generated around inline-level children. Inline-level children are
	// "blockified": their render object is created as RenderBlockFlow so it
	// paints as a block. This mirrors the layout package's buildFlexChildren
	// so the render tree structure matches the layout tree and linkLayoutBoxes
	// can pair them by DOM element identity.
	if parent.Style() != nil && (isFlexContainerDisplay(parent.Style().Display) ||
		parent.Style().Display == style.DisplayGrid ||
		parent.Style().Display == style.DisplayInlineGrid) {
		b.buildFlexChildren(parent, el)
		b.appendPseudoAfter(parent, el)
		return
	}
	var inlineRun []RenderObject
	flush := func() {
		if len(inlineRun) == 0 {
			return
		}
		// Wrap the inline run in an anonymous block flow.
		anonStyle := inheritedStyle(parent.Style())
		anon := NewRenderBlockFlow(nil, anonStyle)
		anon.markAnonymous()
		anon.SetChildrenInline(true)
		for _, c := range inlineRun {
			anon.renderObjectBase.AddChild(c, nil)
		}
		parent.AddChild(anon, nil)
		inlineRun = nil
	}
	// appendChildNode 处理单个 DOM 节点（element/text），创建 render object 并
	// 追加到 parent。inline 节点进 inlineRun，block 节点 flush 后直接挂 parent。
	appendChildNode := func(node dom.Node) {
		switch v := node.(type) {
		case *dom.Element:
			cs := b.resolveStyle(v)
			if cs.Display == style.DisplayNone {
				return
			}
			// 模态 <dialog> 的 ::backdrop：作为 dialog 的前一个兄弟插入（绘制
			// 顺序在 dialog 之下、页面之上）。与 layout/box.go 的插入点一一
			// 对应，否则 linkLayoutBoxes 会错位。
			if bd := b.createBackdropObject(v); bd != nil {
				flush()
				parent.AddChild(bd, nil)
			}
			child := b.createRenderObject(v, cs)
			if child == nil {
				return
			}
			if cs.Float == "left" || cs.Float == "right" {
				// 与 layout/box.go 一致：浮动子不打断父的行内内容（不 flush
				// 匿名块、不进 inlineRun），保证两棵树逐节点对应。
				b.buildChildren(child, v)
				parent.AddChild(child, nil)
			} else if b.isInlineLevel(cs) {
				b.buildChildren(child, v)
				inlineRun = append(inlineRun, child)
			} else {
				flush()
				b.buildChildren(child, v)
				parent.AddChild(child, nil)
			}
		case *dom.Text:
			data := v.Data()
			if data == "" {
				return
			}
			// Skip whitespace-only text nodes that are not part of an inline
			// run. In HTML, inter-element whitespace between block-level
			// siblings (e.g. between </head> and <body>) should not generate
			// anonymous wrappers or visible content.
			if len(inlineRun) == 0 && isWhitespaceOnly(data) {
				return
			}
			rt := NewRenderText(v, inheritedStyle(parent.Style()))
			inlineRun = append(inlineRun, rt)
		}
	}
	// 组合树遍历：<slot> 展开为分配节点，display:contents 元素展开为子节点。
	b.walkComposedChildren(el, appendChildNode)
	flush()
	// ::after 伪元素（最后插入）。
	b.appendPseudoAfter(parent, el)
}

// appendPseudoAfter 为宿主插入 ::after 伪元素渲染对象。
func (b *RenderTreeBuilder) appendPseudoAfter(parent RenderObject, el *dom.Element) {
	if b.resolver == nil {
		return
	}
	if cs, content, ok := b.resolver.ResolvePseudoElement(el, css.PseudoElementAfter); ok && cs.Display != style.DisplayNone {
		parent.AddChild(b.createPseudoObject(cs, content), nil)
	}
}

	// createPseudoObject 为 ::before/::after 创建渲染对象。
	// content 非空时附加一个 RenderText（镜像 WebKit 伪元素文本内容）。
	func (b *RenderTreeBuilder) createPseudoObject(cs *style.ComputedStyle, content string) RenderObject {
		block := NewRenderBlockFlow(nil, cs)
		text := strings.TrimSpace(content)
		if text != "" && text != "none" {
			rt := NewRenderTextWith(nil, cs, text)
			block.AddChild(rt, nil)
		}
		return block
	}

// createBackdropObject 为 top layer 元素的 ::backdrop 创建渲染对象（模态
// <dialog>、显示中的 popover；无 ::backdrop 规则 / display:none 时返回 nil）。
// 它没有 DOM 节点，样式由 style.Resolver.ResolvePseudoElement 解析（UA 规则
// 给出 position:fixed + inset:0；模态 dialog 是半透明黑，popover 是透明不吃
// 指针事件），因此绘制时铺满视口。判定与 layout 侧的 backdropBoxFor 共用
// dom.Element.NeedsBackdrop。
func (b *RenderTreeBuilder) createBackdropObject(el *dom.Element) RenderObject {
	if b.resolver == nil || el == nil || !el.NeedsBackdrop() {
		return nil
	}
	cs, _, ok := b.resolver.ResolvePseudoElement(el, css.PseudoElementBackdrop)
	if !ok || cs.Display == style.DisplayNone {
		return nil
	}
	obj := NewRenderBlockFlow(nil, cs)
	obj.markAnonymous()
	return obj
}

// buildFlexChildren populates a flex/grid container's children directly as flex
// items, without anonymous-block wrappers. Inline-level children are blockified
// (created as RenderBlockFlow) per CSS flexbox §4, mirroring the layout package's
// buildFlexChildren so the two trees have matching structure.
func (b *RenderTreeBuilder) buildFlexChildren(parent RenderObject, el *dom.Element) {
	appendFlexChild := func(node dom.Node) {
		switch v := node.(type) {
		case *dom.Element:
			cs := b.resolveStyle(v)
			if cs.Display == style.DisplayNone {
				return
			}
			// 模态 <dialog> 的 ::backdrop：flex/grid 容器里的 dialog 同样要有
			// 遮罩对象（与 layout/box.go 的 buildFlexChildren 插入点一一对应，
			// 否则 linkLayoutBoxes 会因结构错位而配错盒）。
			if bd := b.createBackdropObject(v); bd != nil {
				parent.AddChild(bd, nil)
			}
			var child RenderObject
			if isReplacedElement(v.LocalName()) {
				child = NewRenderBox(v, cs)
			} else if b.isInlineLevel(cs) {
				// Blockify: inline-level flex item → block-level render object.
				child = NewRenderBlockFlow(v, cs)
			} else {
				child = b.createRenderObject(v, cs)
			}
			if child == nil {
				return
			}
			b.buildChildren(child, v)
			parent.AddChild(child, nil)
		case *dom.Text:
			data := v.Data()
			if data == "" {
				return
			}
			// Pure whitespace text nodes between flex items do not generate
			if isWhitespaceOnly(data) {
				return
			}
			// Text nodes inside flex containers must be wrapped in an anonymous
			// flex items that are block-level).
			anonStyle := inheritedStyle(parent.Style())
			anonStyle.Display = style.DisplayBlock
			anon := NewRenderBlockFlow(nil, anonStyle)

			rt := NewRenderText(v, inheritedStyle(parent.Style()))
			anon.AddChild(rt, nil)
			parent.AddChild(anon, nil)
		}
	}
	// 同上：contents 元素的子节点直接成为 flex/grid 容器的 item。
	b.walkComposedChildren(el, appendFlexChild)
}

// isFlexContainerDisplay reports whether the display value produces a flex
// container.
func isFlexContainerDisplay(d style.DisplayType) bool {
	return d == style.DisplayFlex || d == style.DisplayInlineFlex
}

// createRenderObject maps a DOM element + its computed style to the appropriate render
// object type, mirroring RenderTreeBuilder::createRenderer. The display property drives
// the type; replaced elements (img / iframe / ...) always get a RenderBox.
func (b *RenderTreeBuilder) createRenderObject(el *dom.Element, cs *style.ComputedStyle) RenderObject {
	if isReplacedElement(el.LocalName()) {
		return NewRenderBox(el, cs)
	}
	// CSS Display §2.7 blockification: an out-of-flow box (position:absolute or
	// fixed) is blockified — `<span style="position:absolute">` generates a
	// block-level box, never an inline one. A RenderInline owns no box of its own
	// (it is a participant in the parent's inline formatting context), so leaving
	// the span inline meant it had no box at all: its insets were never resolved,
	// and its background/border were never painted. The "absolutely positioned
	// <span>" idiom (badges, bubbles, close buttons, loading overlays) therefore
	// rendered as nothing. isInlineLevel() already blockified such elements for
	// anonymous-run grouping; the render object type has to follow.
	if cs != nil && (cs.Position == style.PositionAbsolute || cs.Position == style.PositionFixed) {
		return NewRenderBlockFlow(el, cs)
	}
	switch cs.Display {
	case style.DisplayInline:
		// True inline-level elements (span / a / em) become RenderInline, which
		// does not establish a box and participates in the parent's inline
		// formatting context. Inline-block / inline-flex / inline-grid / inline-
		// table are "atomic inline-level" boxes: they establish a box that paints
		// its own background/border, so they are blockified to RenderBlockFlow
		// below (matching WebKit's RenderInline → RenderBlockFlow mapping for
		// atomic inline-level boxes).
		return NewRenderInline(el, cs)
	case style.DisplayInlineBlock, style.DisplayInlineFlex, style.DisplayInlineGrid,
		style.DisplayInlineTable, style.DisplayFlex, style.DisplayGrid,
		style.DisplayTable, style.DisplayBlock, style.DisplayListItem,
		style.DisplayFlowRoot:
		return NewRenderBlockFlow(el, cs)
	default:
		return NewRenderBlockFlow(el, cs)
	}
}

// isInlineLevel reports whether the display value produces an inline-level box.
func (b *RenderTreeBuilder) isInlineLevel(cs *style.ComputedStyle) bool {
	// Out-of-flow (absolute/fixed) elements are blockified per CSS — they never
	// join an inline run / anonymous block wrapper.
	if cs.Position == style.PositionAbsolute || cs.Position == style.PositionFixed {
		return false
	}
	switch cs.Display {
	case style.DisplayInline, style.DisplayInlineBlock, style.DisplayInlineFlex,
		style.DisplayInlineGrid, style.DisplayInlineTable:
		return true
	}
	return false
}

// resolveStyle resolves the element's computed style, falling back to a tag-based
// default when no resolver is available. The tag-based default mirrors the UA stylesheet
// behavior for common HTML elements (block for div/p/body/html, inline for span/a).
// When the resolver is available but CSS did not explicitly set 'display', the
// tag-based default is used as the UA stylesheet would.
func (b *RenderTreeBuilder) resolveStyle(el *dom.Element) *style.ComputedStyle {
	var cs *style.ComputedStyle
	if b.resolver != nil {
		cs = b.resolver.ResolveElement(el)
		// Apply tag-based default display when no CSS rule explicitly set it,
		// mirroring the UA stylesheet defaults in a real browser.
		if !cs.DisplaySet {
			cs.Display = defaultDisplayForTag(el.LocalName())
		}
	} else {
		cs = defaultStyleForTag(el.LocalName())
	}
	// SVG / replaced-element presentation attributes: the width/height
	// attributes map to CSS width/height when no stylesheet rule declared
	// them (WebKit treats them as low-priority presentation attributes).
	// Without this, <svg width="18" height="18"> sized to 0×18 and every
	// icon in the Vue app rendered as a zero-width sliver. iframe 同样：
	// <iframe width="200" height="100"> 需把属性映射为 CSS 尺寸，子文档
	// 视口（syncIFrameSizes）才能同步到内容框大小。
	if el.LocalName() == "svg" || el.LocalName() == "img" || el.LocalName() == "iframe" || el.LocalName() == "canvas" {
		if _, declared := cs.Properties["width"]; !declared {
			if aw := el.GetAttribute("width"); aw != "" {
				if l, ok := parseAttrLength(aw); ok {
					cs.Width = l
				}
			}
		}
		if _, declared := cs.Properties["height"]; !declared {
			if ah := el.GetAttribute("height"); ah != "" {
				if l, ok := parseAttrLength(ah); ok {
					cs.Height = l
				}
			}
		}
	}
	return cs
}

// parseAttrLength converts an HTML presentation-attribute length ("18" or
// "18px", unitless = px) into a style.Length.
func parseAttrLength(s string) (style.Length, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return style.Length{}, false
	}
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.' || s[i] == '-' || s[i] == '+') {
		i++
	}
	num := s[:i]
	if num == "" {
		return style.Length{}, false
	}
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return style.Length{}, false
	}
	unit := s[i:]
	switch unit {
	case "", "px":
		return style.Length{Value: v, Unit: "px"}, true
	}
	return style.Length{}, false
}

// attachLayoutTree builds the layout tree via layout.BuildLayoutTree and links each
// layout box back to the corresponding render object by matching DOM elements.
func (b *RenderTreeBuilder) attachLayoutTree(view *RenderView, root *dom.Element) {
	if b.resolver == nil {
		return
	}
	layoutRoot := layout.BuildLayoutTree(root, b.resolver)
	if layoutRoot == nil {
		return
	}
	rootEb, _ := layoutRoot.(*layout.ElementBox)
	view.SetLayoutBox(rootEb)
	if firstChild := view.FirstChild(); firstChild != nil {
		b.linkLayoutBoxes(firstChild, rootEb)
	}
}

// linkLayoutBoxes recursively links layout boxes to render objects by matching the
func (b *RenderTreeBuilder) linkLayoutBoxes(rObj RenderObject, lBox *layout.ElementBox) {
	if rObj == nil || lBox == nil { return }
	rObj.SetLayoutBox(lBox)
	lChildren := make([]layout.Box, 0, len(lBox.Children()))
	lChildren = append(lChildren, lBox.Children()...)
	for rc := rObj.FirstChild(); rc != nil && len(lChildren) > 0; rc = rc.NextSibling() {
		matched := -1
		for i, lc := range lChildren {
			if childEb, ok := lc.(*layout.ElementBox); ok {
				if sameOwner(rc, childEb) { matched = i; break }
			}
		}
		if matched >= 0 {
			if childEb, ok := lChildren[matched].(*layout.ElementBox); ok {
				b.linkLayoutBoxes(rc, childEb)
			}
			lChildren = append(lChildren[:matched], lChildren[matched+1:]...)
		} else if rc.Node() == nil {
			// Anonymous render child: match with the next anonymous layout child
			// by position (no DOM element to compare).
			for i, lc := range lChildren {
				if childEb, ok := lc.(*layout.ElementBox); ok && childEb.Element() == nil {
					b.linkLayoutBoxes(rc, childEb)
					lChildren = append(lChildren[:i], lChildren[i+1:]...)
					break
				}
			}
		}
	}
}

// sameOwner reports whether the render object and layout box share a DOM owner.

// sameOwner reports whether the render object and layout box share a DOM owner.
// sameOwner reports whether the render object and layout box share a DOM owner.
// For element-backed nodes the match is by DOM element pointer identity.
// For non-element nodes (text, anonymous wrappers) the match falls back to
// position-based pairing (sibling index) so that RenderText ↔ BoxTextRun and
// anonymous wrappers are correctly linked.
func sameOwner(rObj RenderObject, lBox *layout.ElementBox) bool {
	if lBox.Element() != nil {
		if el, ok := rObj.Node().(*dom.Element); ok {
			return el == lBox.Element()
		}
		return false
	}
	return false
}

func roIsAnonymous(ro RenderObject) bool {
	return ro.Node() == nil
}

// inheritedStyle returns a style suitable for an anonymous child: it creates a
// fresh ComputedStyle that inherits only the inheritable properties (color, font,
// text, etc.) from the parent. Sharing the parent's ComputedStyle pointer directly
// would leak non-inherited properties like border / padding / margin / width into
// the anonymous wrapper, causing duplicate border painting (e.g. a card-title's
// border-bottom drawn twice) and incorrect box-model application. This mirrors
// layout.inheritedOrNew so the render tree and layout tree stay consistent.
func inheritedStyle(parent *style.ComputedStyle) *style.ComputedStyle {
	cs := style.NewComputedStyle()
	if parent != nil {
		// InheritFrom 拷贝全部继承属性（含 paint-order——规范继承属性；
		// -webkit-text-stroke 的描边/填充绘制顺序随 [父元素→文本节点]
		// 传递）。
		cs.InheritFrom(parent)
	}
	return cs
}

// isWhitespaceOnly reports whether s consists entirely of CSS whitespace
// (spaces, tabs, newlines, carriage returns, form feeds).
func isWhitespaceOnly(s string) bool {
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r', '\f':
			continue
		default:
			return false
		}
	}
	return true
}

// defaultStyle returns a fresh default ComputedStyle for the given document.
func defaultStyle(doc *dom.Document) *style.ComputedStyle {
	_ = doc
	return style.NewComputedStyle()
}

// defaultStyleForTag returns a ComputedStyle with a display value matching the UA
// stylesheet defaults for common HTML elements. Block-level elements (div, p, body,
// html, h1-h6, ul, ol, li, section, article, header, footer, main) default to block;
// everything else defaults to inline.
func defaultStyleForTag(localName string) *style.ComputedStyle {
	cs := style.NewComputedStyle()
	cs.Display = defaultDisplayForTag(localName)
	return cs
}

// defaultDisplayForTag returns the default display type for an HTML tag, mirroring the
// UA stylesheet's display rules.
func defaultDisplayForTag(localName string) style.DisplayType {
	switch localName {
	case "html", "body", "div", "p", "section", "article", "header", "footer",
		"main", "aside", "nav", "address", "blockquote", "pre", "figure",
		"fieldset", "form", "h1", "h2", "h3", "h4", "h5", "h6",
		"ul", "ol", "li", "dl", "dt", "dd", "table", "thead", "tbody",
		"tfoot", "tr", "td", "th", "caption", "colgroup", "col",
		"details", "summary", "dialog":
		return style.DisplayBlock
	case "head", "title", "meta", "link", "style", "script", "base":
		return style.DisplayNone
	case "input", "button", "select", "textarea", "img":
		// Replaced/form controls default to inline-block (matches the UA
		// stylesheet in html5/defaultcss.go). Without this, width:100% and
		// fixed height/width would not apply to inline-level replaced
		// elements, resulting in zero-sized boxes that HitTest cannot find.
		return style.DisplayInlineBlock
	}
	return style.DisplayInline
}

// isReplacedElement reports whether the tag name corresponds to a replaced element. This
// mirrors the layout package's isReplacedElement check. <select> is included because it
// renders as a closed dropdown (RenderMenuList in WebKit) — its <option> children must not
// produce render tree nodes (their text would otherwise be drawn at wrong positions by
// the normal text paint path).
func isReplacedElement(localName string) bool {
	switch localName {
	case "img", "iframe", "video", "canvas", "embed", "object", "svg", "input", "select", "textarea":
		return true
	}
	return false
}

// Attach builds and returns the render tree, equivalent to calling Build.
func (b *RenderTreeBuilder) Attach(doc *dom.Document) *RenderView {
	return b.Build(doc)
}
