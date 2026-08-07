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
	"strconv"
	"strings"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/layout"
	"wb-ui/style"
	"wb-ui/widgets"
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
	// Transform <wb-markdown> elements: parse their text content as
	// markdown and replace with rendered DOM nodes before building the
	// render tree.
	widgets.ProcessMarkdownElements(doc)
	root := doc.DocumentElement()
	if root == nil {
		// Empty document: still create a RenderView so callers can attach later.
		view := NewRenderView(doc, defaultStyle(doc))
		return view
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
	// Build the associated layout tree and link it back.
	b.attachLayoutTree(view, root)
	// Build the layer tree.
	view.compositor.BuildLayerTree(view)
	view.SetRootLayer(view.compositor.RootLayer())
	return view
}

// buildChildren recursively populates parent's children from the element's DOM children.
// Mixed inline/block siblings are grouped: consecutive inline children are wrapped in
// an anonymous RenderBlockFlow so the block container holds either all-block or
// all-inline children.
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
	for c := el.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *dom.Element:
			cs := b.resolveStyle(v)
			if cs.Display == style.DisplayNone {
				continue
			}
			child := b.createRenderObject(v, cs)
			if child == nil {
				continue
			}
			if b.isInlineLevel(cs) {
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
				continue
			}
			// Skip whitespace-only text nodes that are not part of an inline
			// run. In HTML, inter-element whitespace between block-level
			// siblings (e.g. between </head> and <body>) should not generate
			// anonymous wrappers or visible content.
			if len(inlineRun) == 0 && isWhitespaceOnly(data) {
				continue
			}
			rt := NewRenderText(v, inheritedStyle(parent.Style()))
			inlineRun = append(inlineRun, rt)
		}
	}
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

// buildFlexChildren populates a flex/grid container's children directly as flex
// items, without anonymous-block wrappers. Inline-level children are blockified
// (created as RenderBlockFlow) per CSS flexbox §4, mirroring the layout package's
// buildFlexChildren so the two trees have matching structure.
func (b *RenderTreeBuilder) buildFlexChildren(parent RenderObject, el *dom.Element) {
	for c := el.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *dom.Element:
			cs := b.resolveStyle(v)
			if cs.Display == style.DisplayNone {
				continue
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
				continue
			}
			b.buildChildren(child, v)
			parent.AddChild(child, nil)
		case *dom.Text:
			data := v.Data()
			if data == "" {
				continue
			}
			// Pure whitespace text nodes between flex items do not generate
			if isWhitespaceOnly(data) {
				continue
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
	if el.LocalName() == "svg" || el.LocalName() == "img" || el.LocalName() == "iframe" {
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
		"details", "summary", "dialog",
		"wb-editor", "wb-markdown":
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
