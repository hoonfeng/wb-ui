// Translation of: Source/WebCore/layout/layouttree/LayoutBox.h
//                  Source/WebCore/layout/layouttree/LayoutElementBox.h
//                  Source/WebCore/layout/layouttree/LayoutInlineTextBox.h
//
// Architecture: Box (interface) → ElementBox (has children, wraps DOM element)
//                              → InlineTextBox (leaf, holds text)
// Geometry is stored separately in BoxGeometry (see boxgeometry.go) and accessed
// via LayoutState.GeometryForBox().

package layout

import (
	"strings"

	"wb-ui/css"
	"wb-ui/debugenv"
	"wb-ui/dom"
	"wb-ui/style"
)

// ─────────────────────────────────────────────────────────────
//  NodeType — mirrors WebCore::Layout::Box::NodeType
// ─────────────────────────────────────────────────────────────

type NodeType uint8

const (
	NodeText               NodeType = iota
	NodeGenericElement
	NodeReplacedElement
	NodeDocumentElement
	NodeBody
	NodeTableWrapperBox
	NodeTableBox
	NodeImage
	NodeIFrame
	NodeLineBreak
	NodeWordBreakOpportunity
	NodeListMarker
	NodeImplicitFlexBox
	NodePseudoElement
)

// ─────────────────────────────────────────────────────────────
//  Box interface — mirrors WebCore::Layout::Box
// ─────────────────────────────────────────────────────────────

type Box interface {
	NodeType() NodeType
	Style() *style.ComputedStyle
	IsAnonymous() bool
	Parent() *ElementBox
	IsBlock() bool
	IsInline() bool
	IsTextRun() bool
	IsReplaced() bool
	IsFloated() bool
	IsAbsolutelyPositioned() bool
	IsRelativelyPositioned() bool
	IsStickyPositioned() bool
	IsInFlow() bool
	IsFixedPositioned() bool
	IsVisible() bool
	EstablishesBlockFormattingContext() bool
	EstablishesFlexFormattingContext() bool
	EstablishesGridFormattingContext() bool
	EstablishesTableFormattingContext() bool
	IsInlineLevel() bool
	IsBlockLevel() bool
	IsDirty() bool
	MarkDirty()
	MarkClean()
	HasLayoutChanged() bool
}

// ─────────────────────────────────────────────────────────────
//  ElementBox — mirrors WebCore::Layout::ElementBox
// ─────────────────────────────────────────────────────────────

type ElementBox struct {
	nodeType        NodeType
	style           *style.ComputedStyle
	element         *dom.Element
	children        []Box
	parentBox       *ElementBox
	intrinsicWidth  float64
	intrinsicHeight float64
	TextSegments    []TextSegment
	columnInfo      *columnLayoutInfo
	layoutCache     layoutResult
	// parentSetHeight 记录「父布局上下文分配的固定高度」（definite height / grid 约束 / flex  stretch）。
	// 与 contentHeight（既做父输入又做自身 auto 高度输出）分离，避免增量布局复用
	// geometry 时，上一帧 auto 高度残留被误当成「父设置高度」→ auto 高度不收缩/不扩张。
	parentSetHeight float64
	// MarkerText holds the list-item marker ("•", "1.", "a.") when this box is
	// a display:list-item and its formatting context computed a marker. Empty
	// for non-list items.
	MarkerText string
}

// ─── ElementBox implements Box ───────────────────────────────

func (b *ElementBox) NodeType() NodeType            { return b.nodeType }
func (b *ElementBox) Style() *style.ComputedStyle   { return b.style }
// SetStyle 更新该 box 的计算样式并沿祖先链标记 dirty（布局增量剪枝用）：
// 样式变化影响本 box 几何 → 祖先的几何（高度/位置）也可能变化，必须
// 让整条祖先链在下一次布局时重新计算。
func (b *ElementBox) SetStyle(cs *style.ComputedStyle) {
	b.style = cs
	b.MarkDirty()
}
func (b *ElementBox) IsAnonymous() bool             { return false }
func (b *ElementBox) Parent() *ElementBox           { return b.parentBox }
func (b *ElementBox) ParentSetHeight() float64      { return b.parentSetHeight }
func (b *ElementBox) SetParentSetHeight(h float64)  { b.parentSetHeight = h }
func (b *ElementBox) IsTextRun() bool               { return false }
func (b *ElementBox) IsReplaced() bool              { return isReplacedNodeType(b.nodeType) }
func (b *ElementBox) Children() []Box               { return b.children }
func (b *ElementBox) Element() *dom.Element         { return b.element }
func (b *ElementBox) IntrinsicWidth() float64       { return b.intrinsicWidth }
func (b *ElementBox) IntrinsicHeight() float64      { return b.intrinsicHeight }
func (b *ElementBox) SetIntrinsicWidth(w float64)   { b.intrinsicWidth = w }
func (b *ElementBox) SetIntrinsicHeight(h float64)  { b.intrinsicHeight = h }

func isReplacedNodeType(t NodeType) bool {
	return t == NodeReplacedElement || t == NodeImage || t == NodeIFrame
}

func (b *ElementBox) IsBlock() bool { return !b.IsInline() && !b.IsReplaced() }

func (b *ElementBox) IsInline() bool {
	if b.style == nil { return false }
	switch b.style.Display {
	case style.DisplayInline, style.DisplayInlineBlock, style.DisplayInlineFlex,
		style.DisplayInlineGrid, style.DisplayInlineTable:
		return true
	}
	return false
}

func (b *ElementBox) IsFloated() bool {
	return b.style != nil && (b.style.Float == "left" || b.style.Float == "right")
}

func (b *ElementBox) IsAbsolutelyPositioned() bool {
	if b.style == nil { return false }
	return b.style.Position == style.PositionAbsolute || b.style.Position == style.PositionFixed
}

func (b *ElementBox) IsRelativelyPositioned() bool {
	return b.style != nil && (b.style.Position == style.PositionRelative || b.style.Position == style.PositionSticky)
}
func (b *ElementBox) IsStickyPositioned() bool {
	return b.style != nil && b.style.Position == style.PositionSticky
}
func (b *ElementBox) IsInFlow() bool        { return !b.IsFloated() && !b.IsAbsolutelyPositioned() }
func (b *ElementBox) IsFixedPositioned() bool { return b.style != nil && b.style.Position == style.PositionFixed }
func (b *ElementBox) IsVisible() bool        { return b.style == nil || b.style.Display != style.DisplayNone }

func (b *ElementBox) EstablishesBlockFormattingContext() bool {
	if b.style == nil { return false }
	if b.IsFloated() || b.IsAbsolutelyPositioned() { return true }
	switch b.style.Display {
	case style.DisplayFlowRoot, style.DisplayInlineBlock,
		style.DisplayTable, style.DisplayTableCell, style.DisplayTableCaption,
		style.DisplayFlex, style.DisplayInlineFlex,
		style.DisplayGrid, style.DisplayInlineGrid:
		return true
	}
	if b.style.OverflowX != style.OverflowVisible || b.style.OverflowY != style.OverflowVisible {
		return true
	}
	return false
}

func (b *ElementBox) EstablishesFlexFormattingContext() bool {
	return b.style != nil && (b.style.Display == style.DisplayFlex || b.style.Display == style.DisplayInlineFlex)
}
func (b *ElementBox) EstablishesGridFormattingContext() bool {
	return b.style != nil && (b.style.Display == style.DisplayGrid || b.style.Display == style.DisplayInlineGrid)
}
func (b *ElementBox) EstablishesTableFormattingContext() bool {
	return b.style != nil && (b.style.Display == style.DisplayTable || b.style.Display == style.DisplayInlineTable)
}

func (b *ElementBox) IsInlineLevel() bool {
	if b.style == nil { return false }
	// Out-of-flow (absolute/fixed) elements are blockified per CSS — they
	// never join an inline run / anonymous block wrapper. 渲染树的
	// isInlineLevel 已有此检查（rendertreebuilder.go）；layout 侧漏掉会
	// 导致两树结构错位：position:fixed 的 inline 元素（如 iframe，默认
	// display:inline）在 layout 树被包进匿名 wrapper、渲染树直接挂
	// parent → syncChildren 配对失败 → 元素拿不到布局几何、不参与
	// hit-test（fixed iframe 不可点击）。
	if b.IsAbsolutelyPositioned() {
		return false
	}
	// Flex items are blockified (CSS-DISPLAY-3 §2.7): a span inside a flex
	// container behaves as a block-level flex item even though its computed
	// display stays inline. Without this, getComputedStyle-equivalent
	// reporting (and IFC auto-width expansion) treat flex items as inline.
	if isFlexItem(b) {
		return false
	}
	switch b.style.Display {
	case style.DisplayInline, style.DisplayInlineBlock, style.DisplayInlineFlex,
		style.DisplayInlineGrid, style.DisplayInlineTable:
		return true
	}
	return false
}

func (b *ElementBox) IsBlockLevel() bool {
	if b.style == nil { return false }
	switch b.style.Display {
	case style.DisplayBlock, style.DisplayListItem, style.DisplayFlowRoot,
		style.DisplayTable, style.DisplayFlex, style.DisplayGrid:
		return true
	}
	return false
}

// ── Grid item placement ──

func (b *ElementBox) GridColumnStart() string {
	if b.style == nil { return "" }
	return b.style.GridColumnStart
}
func (b *ElementBox) GridColumnEnd() string {
	if b.style == nil { return "" }
	return b.style.GridColumnEnd
}
func (b *ElementBox) GridRowStart() string {
	if b.style == nil { return "" }
	return b.style.GridRowStart
}
func (b *ElementBox) GridRowEnd() string {
	if b.style == nil { return "" }
	return b.style.GridRowEnd
}
func (b *ElementBox) GridArea() string {
	if b.style == nil { return "" }
	return b.style.GetProperty("grid-area")
}

func (b *ElementBox) AddChild(child Box) {
	if child == nil { return }
	b.children = append(b.children, child)
	switch c := child.(type) {
	case *ElementBox: c.parentBox = b
	case *InlineTextBox: c.parentBox = b
	}
}

type layoutResult struct {
	childCount int
	dirty      bool
	laidOut    bool
	lastX      float64
	lastY      float64
	lastWidth  float64
	lastHeight float64
}

func (b *ElementBox) MarkDirty() {
	if b.layoutCache.dirty { return }
	b.layoutCache.dirty = true
	if b.parentBox != nil { b.parentBox.MarkDirty() }
}
func (b *ElementBox) IsDirty() bool          { return b.layoutCache.dirty }
func (b *ElementBox) MarkClean()             { b.layoutCache = layoutResult{childCount: len(b.children)} }

// MarkCleanWithGeom 在布局完成后记录几何快照（位置 + 内容尺寸），供增量布局
// 剪枝判断「位置未变 + 尺寸未变可跳过」。位置用 border-box top/left（绝对坐标），
// 尺寸用最终内容宽高（含 auto 高度自行计算的结果）。
func (b *ElementBox) MarkCleanWithGeom(x, y, w, h float64) {
	b.layoutCache = layoutResult{childCount: len(b.children), laidOut: true, lastX: x, lastY: y, lastWidth: w, lastHeight: h}
}

// CanSkipLayout 判断该 box 的内容布局是否可跳过（增量布局 B 剪枝）：
// ① 已布局过（laidOut）② 内容未脏（!dirty）③ 子结构未变（childCount 匹配）
// ④ 位置未变（父更新后的 border-box top/left == 上一帧）⑤ 内容尺寸未变。
// 位置变化会带动 children 的绝对坐标平移，即使内容不脏也必须重算（否则残影）；
// 尺寸变化会影响换行/位置，同样必须重算。
func (b *ElementBox) CanSkipLayout(x, y, w, h float64) bool {
	if debugenv.Enabled("WB_NO_LAYOUT_SKIP") {
		return false
	}
	return b.layoutCache.laidOut && !b.layoutCache.dirty &&
		b.layoutCache.childCount == len(b.children) &&
		b.layoutCache.lastX == x && b.layoutCache.lastY == y &&
		b.layoutCache.lastWidth == w && b.layoutCache.lastHeight == h
}

// hasSpecialChildren 判断是否有浮动/绝对定位子：这些子的布局依赖父几何，
// 父 clean 跳过会遗漏它们的位置更新，保守起见有则不剪枝。
func (b *ElementBox) hasSpecialChildren() bool {
	for _, c := range b.children {
		if eb, ok := c.(*ElementBox); ok {
			if eb.IsFloated() || eb.IsAbsolutelyPositioned() {
				return true
			}
		}
	}
	return false
}

func (b *ElementBox) HasLayoutChanged() bool { return b.layoutCache.dirty }

// ─────────────────────────────────────────────────────────────
//  InlineTextBox — mirrors WebCore::Layout::InlineTextBox
// ─────────────────────────────────────────────────────────────

type InlineTextBox struct {
	text         string
	style        *style.ComputedStyle
	parentBox    *ElementBox
	node         dom.Node // 源 DOM Text 节点（增量 text 更新回溯 + node 匹配用）
	TextSegments []TextSegment
}

func (t *InlineTextBox) NodeType() NodeType                     { return NodeText }
func (t *InlineTextBox) Style() *style.ComputedStyle            { return t.style }
func (t *InlineTextBox) IsAnonymous() bool                      { return true }
func (t *InlineTextBox) Parent() *ElementBox                    { return t.parentBox }
func (t *InlineTextBox) IsBlock() bool                          { return false }
func (t *InlineTextBox) IsInline() bool                         { return true }
func (t *InlineTextBox) IsTextRun() bool                        { return true }
func (t *InlineTextBox) IsReplaced() bool                       { return false }
func (t *InlineTextBox) IsFloated() bool                        { return false }
func (t *InlineTextBox) IsAbsolutelyPositioned() bool           { return false }
func (t *InlineTextBox) IsRelativelyPositioned() bool           { return false }
func (t *InlineTextBox) IsStickyPositioned() bool               { return false }
func (t *InlineTextBox) IsInFlow() bool                         { return true }
func (t *InlineTextBox) IsFixedPositioned() bool                { return false }
func (t *InlineTextBox) IsVisible() bool                        { return true }
func (t *InlineTextBox) EstablishesBlockFormattingContext() bool { return false }
func (t *InlineTextBox) EstablishesFlexFormattingContext() bool  { return false }
func (t *InlineTextBox) EstablishesGridFormattingContext() bool  { return false }
func (t *InlineTextBox) EstablishesTableFormattingContext() bool { return false }
func (t *InlineTextBox) IsInlineLevel() bool                     { return true }
func (t *InlineTextBox) IsBlockLevel() bool                      { return false }
func (t *InlineTextBox) IsDirty() bool                           { return false }
func (t *InlineTextBox) MarkDirty()                              {}
func (t *InlineTextBox) MarkClean()                              {}
func (t *InlineTextBox) HasLayoutChanged() bool                  { return false }
func (t *InlineTextBox) Text() string                            { return t.text }
func (t *InlineTextBox) SetText(s string)                        { t.text = s }
func (t *InlineTextBox) Node() dom.Node                          { return t.node }

// ─────────────────────────────────────────────────────────────
//  TextSegment
// ─────────────────────────────────────────────────────────────

type TextSegment struct {
	Start, Len int
	X, Y, Width, Height float64
	LineY, LineHeight float64
}

// hasInlineChildren reports whether box's children are *all* inline-level, i.e.
// the box is a block container holding inline content that the
// InlineFormattingContext must lay out as line boxes.
//
// "All" is load-bearing: a block container that mixes inline-level and
// block-level children is not an inline-content container — the block-level
// children are laid out by the block formatting context (with the runs of
// inline-level children between them grouped into anonymous blocks). Testing
// only for the *presence* of an inline child made any mixed container dispatch
// to the InlineFormattingContext, which lays out line boxes and silently drops
// block-level children: e.g. `body::before{content:""}` (an inline-level
// pseudo box that buildChildren appends directly, bypassing the anonymous-block
// grouping) turned `body` into an inline container, so its block-level children
// were never laid out and collapsed to 0x0 — every descendant then painted at
// zero size (`*::before/*::after` resets, the common
// `*,:before,:after{box-sizing:inherit}` idiom, hit exactly this).
func hasInlineChildren(box *ElementBox) bool {
	hasInline := false
	for _, c := range box.children {
		if _, ok := c.(*InlineTextBox); ok {
			hasInline = true
			continue
		}
		eb, ok := c.(*ElementBox)
		if !ok {
			return false
		}
		if !eb.IsInlineLevel() {
			// A block-level (or out-of-flow) child means this container's
			// content is not one inline formatting context.
			return false
		}
		hasInline = true
	}
	return hasInline
}

// ─────────────────────────────────────────────────────────────
//  BuildLayoutTree
// ─────────────────────────────────────────────────────────────

func BuildLayoutTree(root *dom.Element, resolver *style.Resolver) Box {
	if root == nil { return nil }
	cs := resolveStyleOrDefault(resolver, root)
	// The root element's computed font-size is the `rem` base for the whole
	// document (CSS Values §5.1) — see remBase.
	if px := rootFontSizePx(cs); px > 0 {
		SetRootFontSize(px)
	}
	box := newBoxForElement(root, cs)
	buildChildren(box, root, resolver)
	return box
}

func resolveStyleOrDefault(resolver *style.Resolver, el *dom.Element) *style.ComputedStyle {
	if resolver != nil {
		cs := resolver.ResolveElement(el)
		if !cs.DisplaySet { cs.Display = defaultDisplayForTag(el.LocalName()) }
		return cs
	}
	return style.NewComputedStyle()
}

func defaultDisplayForTag(localName string) style.DisplayType {
	switch localName {
	case "html", "body", "div", "p", "section", "article", "header", "footer",
		"main", "aside", "nav", "address", "blockquote", "pre", "figure",
		"fieldset", "form", "h1", "h2", "h3", "h4", "h5", "h6",
		"ul", "ol", "li", "dl", "dt", "dd", "table", "thead", "tbody",
		"tfoot", "tr", "td", "th", "caption", "colgroup", "col":
		return style.DisplayBlock
	case "head", "title", "meta", "link", "style", "script", "base":
		return style.DisplayNone
	case "input", "button", "select", "textarea":
		return style.DisplayInlineBlock
	}
	return style.DisplayInline
}

func newBoxForElement(el *dom.Element, cs *style.ComputedStyle) *ElementBox {
	nt := NodeGenericElement
	if isReplacedElement(el.LocalName()) { nt = NodeReplacedElement }
	return &ElementBox{nodeType: nt, style: cs, element: el}
}


func isReplacedElement(localName string) bool {
	switch localName {
	case "img", "iframe", "video", "canvas", "embed", "object", "svg", "input", "select", "textarea":
		return true
	}
	return false
}
func buildChildren(box *ElementBox, el *dom.Element, resolver *style.Resolver) {
	if isReplacedElement(el.LocalName()) { return }
	// ::before 伪元素（镜像 WebKit：伪元素作为宿主的子 box，before 在最前）。
	// 无 DOM 节点（Element()==nil），通过 NodePseudoElement 标记；linkLayoutBoxes
	// 按匿名位置配对 render/layout 两侧的伪元素 box。
	appendPseudoBefore(box, el, resolver)
	if box.style != nil && (isFlexContainerDisplay(box.style.Display) ||
		box.style.Display == style.DisplayGrid || box.style.Display == style.DisplayInlineGrid) {
		buildFlexChildren(box, el, resolver)
		appendPseudoAfter(box, el, resolver)
		return
	}
	var inlineRun []Box
	flush := func() {
		if len(inlineRun) == 0 { return }
		for _, r := range inlineRun {
			if tb, ok := r.(*InlineTextBox); ok {
				if strings.TrimSpace(tb.text) != "" { goto hasContent }
			} else { goto hasContent }
		}
		inlineRun = nil; return
	hasContent:
		kids := append([]Box(nil), inlineRun...)
		wrap := &ElementBox{nodeType: NodeGenericElement, style: box.style, children: kids}
		// Anonymous inline wrappers should not inherit padding/border/margin from
		// parent — those belong to the parent box's content-box boundaries.
		anonStyle := *wrap.style
		anonStyle.PaddingTop = style.Length{}
		anonStyle.PaddingRight = style.Length{}
		anonStyle.PaddingBottom = style.Length{}
		anonStyle.PaddingLeft = style.Length{}
		anonStyle.BorderTopWidth = style.Length{}
		anonStyle.BorderRightWidth = style.Length{}
		anonStyle.BorderBottomWidth = style.Length{}
		anonStyle.BorderLeftWidth = style.Length{}
		anonStyle.MarginTop = style.Length{}
		anonStyle.MarginRight = style.Length{}
		anonStyle.MarginBottom = style.Length{}
		anonStyle.MarginLeft = style.Length{}
		anonStyle.Position = style.PositionStatic
		anonStyle.Float = "none"
		// Anonymous inline wrappers must NOT inherit min/max width either —
		// a parent with min-width:320px (e.g. a dialog box) would otherwise
		// stretch the wrapper to 320px, making a child input width:100%
		// overflow the parent's real content width.
		anonStyle.MinWidth = style.Length{}
		anonStyle.MaxWidth = style.Length{}
		anonStyle.MinHeight = style.Length{}
		anonStyle.MaxHeight = style.Length{}
		// ★ 匿名 inline wrapper 不得继承父的显式宽高：父 width:224px
		// （如 absolute 弹窗 cp-panel）会让 wrapper 224px 宽 → 内部
		// input/textarea 的 width:100% 按 wrapper（224）而非父内容区
		// （198）计算 → 输入框超出面板右边界（欢迎语编辑弹窗 textarea
		// 溢出根因）。wrapper 应为 width/height:auto → 撑满父内容区。
		anonStyle.Width = style.Length{}
		anonStyle.Height = style.Length{}
		wrap.style = &anonStyle
		for _, k := range kids {
			switch c := k.(type) {
			case *ElementBox: c.parentBox = wrap
			case *InlineTextBox: c.parentBox = wrap
			}
		}
		box.AddChild(wrap)
		inlineRun = nil
	}
	appendChildNode := func(node dom.Node) {
		switch v := node.(type) {
		case *dom.Element:
			cs := resolveStyleOrDefault(resolver, v)
			if cs.Display == style.DisplayNone { return }
			child := newBoxForElement(v, cs)
			if child.IsInlineLevel() {
				buildChildren(child, v, resolver)
				inlineRun = append(inlineRun, child)
			} else {
				flush()
				buildChildren(child, v, resolver)
				box.AddChild(child)
			}
		case *dom.Text:
			data := v.Data()
			if data == "" { return }
			inlineRun = append(inlineRun, &InlineTextBox{text: data, style: box.style, node: v})
		}
	}
	for c := dom.FirstComposedChild(el); c != nil; c = c.NextSibling() {
		if e, ok := c.(*dom.Element); ok && e.LocalName() == "slot" {
			for _, an := range e.AssignedNodes() {
				appendChildNode(an)
			}
			continue
		}
		appendChildNode(c)
	}
	flush()
	// ::after 伪元素（最后）。
	appendPseudoAfter(box, el, resolver)
}

// appendPseudoBefore/appendPseudoAfter 为宿主元素插入 ::before/::after 伪元素 box
//（无 DOM 节点）。样式经 resolver.ResolvePseudoElement 解析；display:none 时跳过。
func appendPseudoBefore(box *ElementBox, el *dom.Element, resolver *style.Resolver) {
	if resolver == nil {
		return
	}
	if cs, _, ok := resolver.ResolvePseudoElement(el, css.PseudoElementBefore); ok && cs.Display != style.DisplayNone {
		box.AddChild(&ElementBox{nodeType: NodePseudoElement, style: cs})
	}
}

func appendPseudoAfter(box *ElementBox, el *dom.Element, resolver *style.Resolver) {
	if resolver == nil {
		return
	}
	if cs, _, ok := resolver.ResolvePseudoElement(el, css.PseudoElementAfter); ok && cs.Display != style.DisplayNone {
		box.AddChild(&ElementBox{nodeType: NodePseudoElement, style: cs})
	}
}

func isFlexContainerDisplay(d style.DisplayType) bool {
	return d == style.DisplayFlex || d == style.DisplayInlineFlex
}

func buildFlexChildren(box *ElementBox, el *dom.Element, resolver *style.Resolver) {
	appendFlexChild := func(node dom.Node) {
		switch v := node.(type) {
		case *dom.Element:
			cs := resolveStyleOrDefault(resolver, v)
			if cs.Display == style.DisplayNone { return }
			child := newBoxForElement(v, cs)
			buildChildren(child, v, resolver)
			box.AddChild(child)
		case *dom.Text:
			data := v.Data()
			if strings.TrimSpace(data) == "" { return }
			// Anonymous wrapper must NOT inherit non-inherited properties
			// (width/height/border/padding/margin) from parent — only inherited ones.
			// This mirrors rendering.inheritedStyle so layout & render trees match.
			anonStyle := style.NewComputedStyle()
			if box.style != nil {
				anonStyle.InheritFrom(box.style)
			}
			anonStyle.Display = style.DisplayBlock
			anon := &ElementBox{
				nodeType: NodeGenericElement, style: anonStyle,
				children: []Box{&InlineTextBox{text: data, style: anonStyle, node: v}},
			}
			box.AddChild(anon)
		}
	}
	for c := dom.FirstComposedChild(el); c != nil; c = c.NextSibling() {
		if e, ok := c.(*dom.Element); ok && e.LocalName() == "slot" {
			for _, an := range e.AssignedNodes() {
				appendFlexChild(an)
			}
			continue
		}
		appendFlexChild(c)
	}
}

// ─── Transitional type alias ────────────────────────────────

// LayoutBox is a type alias for backward compatibility. New code should use
// Box interface or *ElementBox/*InlineTextBox directly.
type LayoutBox = ElementBox
