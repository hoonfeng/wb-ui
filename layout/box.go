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
}

// ─── ElementBox implements Box ───────────────────────────────

func (b *ElementBox) NodeType() NodeType            { return b.nodeType }
func (b *ElementBox) Style() *style.ComputedStyle   { return b.style }
func (b *ElementBox) IsAnonymous() bool             { return false }
func (b *ElementBox) Parent() *ElementBox           { return b.parentBox }
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
	return b.style != nil && b.style.Position == style.PositionRelative
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
}

func (b *ElementBox) MarkDirty() {
	if b.layoutCache.dirty { return }
	b.layoutCache.dirty = true
	if b.parentBox != nil { b.parentBox.MarkDirty() }
}
func (b *ElementBox) IsDirty() bool          { return b.layoutCache.dirty }
func (b *ElementBox) MarkClean()             { b.layoutCache = layoutResult{childCount: len(b.children)} }
func (b *ElementBox) HasLayoutChanged() bool { return b.layoutCache.dirty }

// ─────────────────────────────────────────────────────────────
//  InlineTextBox — mirrors WebCore::Layout::InlineTextBox
// ─────────────────────────────────────────────────────────────

type InlineTextBox struct {
	text         string
	style        *style.ComputedStyle
	parentBox    *ElementBox
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

// ─────────────────────────────────────────────────────────────
//  TextSegment
// ─────────────────────────────────────────────────────────────

type TextSegment struct {
	Start, Len int
	X, Y, Width, Height float64
	LineY, LineHeight float64
}

// hasInlineChildren reports whether box directly holds InlineTextBox children
// (i.e., its children are inline-level text runs that need InlineFormattingContext).
func hasInlineChildren(box *ElementBox) bool {
	for _, c := range box.children {
		if _, ok := c.(*InlineTextBox); ok {
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────
//  BuildLayoutTree
// ─────────────────────────────────────────────────────────────

func BuildLayoutTree(root *dom.Element, resolver *style.Resolver) Box {
	if root == nil { return nil }
	cs := resolveStyleOrDefault(resolver, root)
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
	case "img", "iframe", "video", "canvas", "embed", "object", "svg", "input", "select":
		return true
	}
	return false
}

func buildChildren(box *ElementBox, el *dom.Element, resolver *style.Resolver) {
	if isReplacedElement(el.LocalName()) { return }
	if box.style != nil && (isFlexContainerDisplay(box.style.Display) ||
		box.style.Display == style.DisplayGrid || box.style.Display == style.DisplayInlineGrid) {
		buildFlexChildren(box, el, resolver)
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
		// Anonymous inline wrappers should not inherit padding/border from
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
	for c := el.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *dom.Element:
			cs := resolveStyleOrDefault(resolver, v)
			if cs.Display == style.DisplayNone { continue }
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
			if data == "" { continue }
			inlineRun = append(inlineRun, &InlineTextBox{text: data, style: box.style})
		}
	}
	flush()
}

func isFlexContainerDisplay(d style.DisplayType) bool {
	return d == style.DisplayFlex || d == style.DisplayInlineFlex
}

func buildFlexChildren(box *ElementBox, el *dom.Element, resolver *style.Resolver) {
	for c := el.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *dom.Element:
			cs := resolveStyleOrDefault(resolver, v)
			if cs.Display == style.DisplayNone { continue }
			child := newBoxForElement(v, cs)
			buildChildren(child, v, resolver)
			box.AddChild(child)
		case *dom.Text:
			data := v.Data()
			if strings.TrimSpace(data) == "" { continue }
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
				children: []Box{&InlineTextBox{text: data, style: anonStyle}},
			}
			box.AddChild(anon)
		}
	}
}

// ─── Transitional type alias ────────────────────────────────

// LayoutBox is a type alias for backward compatibility. New code should use
// Box interface or *ElementBox/*InlineTextBox directly.
type LayoutBox = ElementBox
