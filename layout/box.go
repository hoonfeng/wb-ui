// Translation of: Source/WebCore/layout/LayoutElementBox.h
//                  Source/WebCore/rendering/RenderBox.h
//                  Source/WebCore/rendering/RenderObject.h
// Completeness: 65%
// Simplifications:
//   - no subpixel layout (integer pixels only; floats used internally then rounded)
//   - no pagination/fragmentation
//   - RenderObject / RenderBox / ElementBox are collapsed into a single LayoutBox
//     struct that carries its own geometry (WebKit keeps geometry in a separate
//     BoxGeometry owned by LayoutState)
//   - anonymous box generation is simplified: inline content inside a block
//     container is wrapped lazily by the block formatting context rather than by
//     a dedicated anonymous-block builder
//   - replaced elements (img, iframe, ...) are represented by a Replaced box type
//     with a fixed intrinsic size; no async image sizing

package layout

import (
	"math"
	"strings"

	"wb-ui/dom"
	"wb-ui/style"
)

// BoxType mirrors the kind of layout box. It is the Go counterpart of the union of
// WebKit's RenderObject derivation flags (RenderBox, RenderInline, anonymous, etc.).
type BoxType int

const (
	// BoxBlock is a block-level box (display: block / list-item / flow-root).
	BoxBlock BoxType = iota
	// BoxInline is an inline-level box that participates in an inline formatting
	// context (display: inline / inline-block).
	BoxInline
	// BoxAnonymous is an anonymously generated block wrapper.
	BoxAnonymous
	// BoxReplaced is a replaced element with an intrinsic size (img, canvas, ...).
	BoxReplaced
	// BoxTextRun is an anonymous inline box holding a run of text content. It is the
	// layout counterpart of WebKit's InlineTextBox.
	BoxTextRun
)

// TextSegment is a laid-out fragment of a text run within a single line box. The inline
// formatting context produces these and stores them on the text run's LayoutBox so the
// render tree sync step can copy them onto RenderText.segments for the paint pipeline.
type TextSegment struct {
	// Start is the rune offset of this segment within the text run's original text.
	Start int
	// Len is the rune length of this segment.
	Len int
	// X / Y / Width / Height are the geometry of the text fragment (ascent +
	// descent height) in the containing block's content coordinate space. Y is
	// the top of the text box (already vertically centered within the line).
	X, Y          float64
	Width, Height float64
	// LineY / LineHeight describe the line box that contains this text
	// segment, mirroring WebKit's RootInlineBox geometry. Hit-testing and
	// selection use the line box bounds (not the text box) so that clicks in
	// the inter-line whitespace and selection highlights cover the full line
	// height, matching browser behavior.
	LineY, LineHeight float64
}

// Edges groups the four sides of a CSS box-model area (margin, padding or border).
type Edges struct {
	Top, Right, Bottom, Left float64
}

// IsZero reports whether all four sides are zero.
func (e Edges) IsZero() bool {
	return e.Top == 0 && e.Right == 0 && e.Bottom == 0 && e.Left == 0
}

// Horizontal returns Left + Right.
func (e Edges) Horizontal() float64 { return e.Left + e.Right }

// Vertical returns Top + Bottom.
func (e Edges) Vertical() float64 { return e.Top + e.Bottom }

// LayoutRect is the Go counterpart of WebCore::Layout::Rect / BoxGeometry. It stores
// the border-box position and size plus the surrounding margin and the inner padding
// and border edges. All values are in CSS pixels; subpixel layout is disabled so the
// final values are rounded to integers when read back.
type LayoutRect struct {
	X, Y          float64
	Width, Height float64
	Margin        Edges
	Padding       Edges
	Border        Edges
}

// ContentX returns the left edge of the content box (border-box X + border-left +
// padding-left).
func (r LayoutRect) ContentX() float64 { return r.X + r.Border.Left + r.Padding.Left }

// ContentY returns the top edge of the content box.
func (r LayoutRect) ContentY() float64 { return r.Y + r.Border.Top + r.Padding.Top }

// ContentWidth returns the width of the content box (border-box width minus
// horizontal border and padding). For border-box sizing this is the inner size.
func (r LayoutRect) ContentWidth() float64 {
	return r.Width - r.Border.Horizontal() - r.Padding.Horizontal()
}

// ContentHeight returns the height of the content box.
func (r LayoutRect) ContentHeight() float64 {
	return r.Height - r.Border.Vertical() - r.Padding.Vertical()
}

// MarginBoxWidth returns the outer width including margins.
func (r LayoutRect) MarginBoxWidth() float64 { return r.Width + r.Margin.Horizontal() }

// MarginBoxHeight returns the outer height including margins.
func (r LayoutRect) MarginBoxHeight() float64 { return r.Height + r.Margin.Vertical() }

// LayoutBox is the Go translation of WebCore::Layout::ElementBox / RenderBox. It is
// the node of the layout tree: each box has a type, an optional owning DOM element,
// an optional text payload, its resolved ComputedStyle, a list of child boxes and the
// geometry produced by the formatting contexts.
type LayoutBox struct {
	Type     BoxType
	Element  *dom.Element
	Text     string
	Style    *style.ComputedStyle
	Children []*LayoutBox
	Rect     LayoutRect

	// parent points to the containing box in the layout tree. It is populated by
	// AddChild / buildChildren so the positioned-layout helpers can walk the
	// ancestor chain to find the containing block for absolutely-positioned boxes.
	parent *LayoutBox

	// IntrinsicWidth/Height carry the intrinsic size of replaced elements (img, ...).
	// Zero means "not specified"; the formatting context will fall back to defaults.
	IntrinsicWidth  float64
	IntrinsicHeight float64

	// TextSegments holds the laid-out text fragments produced by the inline
	// formatting context for BoxTextRun boxes. The render tree sync step reads
	// these to populate RenderText.segments so the paint pipeline can draw each
	// segment at its correct position. Nil for non-text-run boxes.
	TextSegments []TextSegment

	// columnInfo stores multi-column layout geometry computed during layout.
	// Non-nil only for boxes that participated in multi-column layout.
	columnInfo *columnLayoutInfo

	// layoutCache stores the last computed layout geometry to detect changes.
	// When the style and children haven't changed, layout can be skipped.
	layoutCache layoutResult
}

// layoutResult caches the most recent layout pass result for dirty-checking.
type layoutResult struct {
	width, height float64
	x, y          float64
	childCount    int
	dirty         bool
}

// MarkDirty marks this box and all ancestors as needing re-layout.
func (b *LayoutBox) MarkDirty() {
	if b.layoutCache.dirty {
		return // already dirty
	}
	b.layoutCache.dirty = true
	b.layoutCache.childCount = len(b.Children)
	if b.parent != nil {
		b.parent.MarkDirty()
	}
}

// IsDirty reports whether this box needs re-layout.
func (b *LayoutBox) IsDirty() bool { return b.layoutCache.dirty }

// MarkClean clears the dirty flag and caches current geometry.
func (b *LayoutBox) MarkClean() {
	b.layoutCache = layoutResult{
		width:      b.Rect.Width,
		height:     b.Rect.Height,
		x:          b.Rect.X,
		y:          b.Rect.Y,
		childCount: len(b.Children),
		dirty:      false,
	}
}

// (b *LayoutBox) HasLayoutChanged returns true if the box's geometry or children
// have changed since the last MarkClean call.
func (b *LayoutBox) HasLayoutChanged() bool {
	return b.layoutCache.width != b.Rect.Width ||
		b.layoutCache.height != b.Rect.Height ||
		b.layoutCache.x != b.Rect.X ||
		b.layoutCache.y != b.Rect.Y ||
		b.layoutCache.childCount != len(b.Children)
}

// NewLayoutBox constructs a leaf layout box with the given type and style.
func NewLayoutBox(t BoxType, st *style.ComputedStyle) *LayoutBox {
	return &LayoutBox{Type: t, Style: st}
}

// AddChild appends child to b's child list and records b as child's parent.
func (b *LayoutBox) AddChild(child *LayoutBox) {
	child.parent = b
	b.Children = append(b.Children, child)
}

// IsBlock reports whether b is a block-level container box.
func (b *LayoutBox) IsBlock() bool { return b.Type == BoxBlock || b.Type == BoxAnonymous }

// IsInline reports whether b participates in inline layout.
func (b *LayoutBox) IsInline() bool {
	return b.Type == BoxInline || b.Type == BoxTextRun
}

// IsTextRun reports whether b holds text content.
func (b *LayoutBox) IsTextRun() bool { return b.Type == BoxTextRun }

// IsReplaced reports whether b is a replaced element.
func (b *LayoutBox) IsReplaced() bool { return b.Type == BoxReplaced }

// IsFloated reports whether b is floated (float: left/right).
func (b *LayoutBox) IsFloated() bool {
	return b.Style != nil && (b.Style.Float == "left" || b.Style.Float == "right")
}

// IsAbsolutelyPositioned reports whether b is out-of-flow positioned (absolute/fixed).
func (b *LayoutBox) IsAbsolutelyPositioned() bool {
	if b.Style == nil {
		return false
	}
	return b.Style.Position == style.PositionAbsolute || b.Style.Position == style.PositionFixed
}

// IsRelativelyPositioned reports whether b is relatively positioned.
func (b *LayoutBox) IsRelativelyPositioned() bool {
	return b.Style != nil && b.Style.Position == style.PositionRelative
}

// IsInFlow reports whether b participates in normal flow (not floated nor absolutely
// positioned). Relatively-positioned boxes remain in flow.
func (b *LayoutBox) IsInFlow() bool {
	return !b.IsFloated() && !b.IsAbsolutelyPositioned()
}

// IsVisible reports whether b should generate boxes (display: none suppresses layout).
func (b *LayoutBox) IsVisible() bool {
	return b.Style == nil || b.Style.Display != style.DisplayNone
}

// establishesBlockFormattingContext reports whether b establishes a new block
// formatting context (flow-root, overflow != visible, float, inline-block, table
// cell, flex/grid container, absolutely positioned). Used to decide when a block
// container's children are laid out by a fresh BFC.
func (b *LayoutBox) establishesBlockFormattingContext() bool {
	if b.Style == nil {
		return false
	}
	if b.IsFloated() || b.IsAbsolutelyPositioned() {
		return true
	}
	switch b.Style.Display {
	case style.DisplayFlowRoot, style.DisplayInlineBlock,
		style.DisplayTable, style.DisplayTableCell, style.DisplayTableCaption,
		style.DisplayFlex, style.DisplayInlineFlex,
		style.DisplayGrid, style.DisplayInlineGrid:
		return true
	}
	if b.Style.OverflowX != style.OverflowVisible || b.Style.OverflowY != style.OverflowVisible {
		return true
	}
	return false
}

// isInlineLevel reports whether b generates an inline-level box per its display.
func (b *LayoutBox) IsInlineLevel() bool {
	if b.Style == nil {
		return false
	}
	switch b.Style.Display {
	case style.DisplayInline, style.DisplayInlineBlock, style.DisplayInlineFlex,
		style.DisplayInlineGrid, style.DisplayInlineTable:
		return true
	}
	return false
}

// isBlockLevel reports whether b generates a block-level box per its display.
func (b *LayoutBox) isBlockLevel() bool {
	if b.Style == nil {
		return false
	}
	switch b.Style.Display {
	case style.DisplayBlock, style.DisplayListItem, style.DisplayFlowRoot,
		style.DisplayTable, style.DisplayFlex, style.DisplayGrid:
		return true
	}
	return false
}

// Round rounds the box geometry to whole CSS pixels. Subpixel layout is disabled.
func (b *LayoutBox) Round() {
	r := &b.Rect
	r.X = math.Round(r.X)
	r.Y = math.Round(r.Y)
	r.Width = math.Round(r.Width)
	r.Height = math.Round(r.Height)
	roundEdges(&r.Margin)
	roundEdges(&r.Padding)
	roundEdges(&r.Border)
}

func roundEdges(e *Edges) {
	e.Top = math.Round(e.Top)
	e.Right = math.Round(e.Right)
	e.Bottom = math.Round(e.Bottom)
	e.Left = math.Round(e.Left)
}

// BuildLayoutTree constructs the layout tree from a DOM subtree. For each Element the
// resolver computes a ComputedStyle; text nodes become anonymous BoxTextRun children
// of their parent element's box. Block containers that mix inline and block children
// get anonymous wrappers so the layout tree satisfies the CSS requirement that a
// block container holds either block-level or inline-level boxes.
func BuildLayoutTree(root *dom.Element, resolver *style.Resolver) *LayoutBox {
	if root == nil {
		return nil
	}
	cs := resolveStyleOrDefault(resolver, root)
	box := newBoxForElement(root, cs)
	buildChildren(box, root, resolver)
	return box
}

// resolveStyleOrDefault resolves the element's style, falling back to a default
// ComputedStyle when no resolver is available (used in tests). When the resolver is
// available but CSS did not explicitly set 'display', the tag-based default display
// (block for div/body/html, inline for span) is applied to mirror the UA stylesheet.
func resolveStyleOrDefault(resolver *style.Resolver, el *dom.Element) *style.ComputedStyle {
	if resolver != nil {
		cs := resolver.ResolveElement(el)
		if !cs.DisplaySet {
			cs.Display = defaultDisplayForTag(el.LocalName())
		}
		return cs
	}
	return style.NewComputedStyle()
}

// defaultDisplayForTag returns the default display type for an HTML tag, mirroring the
// UA stylesheet's display rules.
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
		// Form controls default to inline-block (matches the UA stylesheet
		// in html5/defaultcss.go and the rendering package's defaultDisplayForTag).
		// Without this, width:100% and fixed height/width would not apply to
		// inline-level replaced elements, resulting in zero-sized boxes.
		return style.DisplayInlineBlock
	}
	return style.DisplayInline
}

// newBoxForElement maps a DOM element + its computed style to the appropriate box
// type. The display property drives the box type.
func newBoxForElement(el *dom.Element, cs *style.ComputedStyle) *LayoutBox {
	if isReplacedElement(el.LocalName()) {
		return &LayoutBox{Type: BoxReplaced, Element: el, Style: cs}
	}
	switch cs.Display {
	case style.DisplayInline, style.DisplayInlineBlock, style.DisplayInlineFlex,
		style.DisplayInlineGrid, style.DisplayInlineTable:
		return &LayoutBox{Type: BoxInline, Element: el, Style: cs}
	default:
		return &LayoutBox{Type: BoxBlock, Element: el, Style: cs}
	}
}

// isReplacedElement reports whether the tag name corresponds to a replaced element.
// <select> is included because it renders as a closed dropdown — its <option> children
// must not produce layout tree nodes (matching the rendering package's treatment).
func isReplacedElement(localName string) bool {
	switch localName {
	case "img", "iframe", "video", "canvas", "embed", "object", "svg", "input", "select":
		return true
	}
	return false
}

// buildChildren recursively populates box.Children from the element's child nodes.
// Mixed inline/block siblings are grouped: consecutive inline boxes are wrapped in
// an anonymous block-level box so that the parent's block formatting context can lay
// them out via an inline formatting context.
func buildChildren(box *LayoutBox, el *dom.Element, resolver *style.Resolver) {
	// Replaced elements are leaf nodes in the layout tree — they have no layout
	// children. This mirrors the rendering package's buildChildren skip.
	if isReplacedElement(el.LocalName()) {
		return
	}
	// Flex / grid containers: per CSS (flexbox §4 / grid §3) every element child
	// becomes a flex/grid item directly — no anonymous block wrappers are generated
	// around inline-level children. Inline-level children are "blockified": their
	// display is treated as block-level for layout so the formatting context sees
	// each item as a direct block child. Pure whitespace text nodes between items
	// are ignored (they do not generate items); non-whitespace text becomes an
	// anonymous flex/grid item.
	if box.Style != nil && (isFlexContainerDisplay(box.Style.Display) ||
		box.Style.Display == style.DisplayGrid ||
		box.Style.Display == style.DisplayInlineGrid) {
		buildFlexChildren(box, el, resolver)
		return
	}
	var inlineRun []*LayoutBox
	flush := func() {
		if len(inlineRun) == 0 {
			return
		}
		// Skip anonymous wrappers that contain only whitespace text. These arise
		// from inter-element newlines in the HTML source (e.g. between </div>
		// and <div>). Without this skip the inline formatting context assigns
		// each wrapper a line-height (~17px for a 14px font), adding spurious
		// vertical gaps between block siblings. Per CSS, block-level siblings
		// separated by whitespace-only text do not generate anonymous boxes.
		allWhitespace := true
		for _, r := range inlineRun {
			if r.Type == BoxTextRun {
				if strings.TrimSpace(r.Text) != "" {
					allWhitespace = false
					break
				}
			} else {
				allWhitespace = false
				break
			}
		}
		if allWhitespace {
			inlineRun = nil
			return
		}
		kids := append([]*LayoutBox(nil), inlineRun...)
		wrap := &LayoutBox{
			Type:     BoxAnonymous,
			Style:    inheritedOrNew(box.Style),
			Children: kids,
		}
		for _, k := range kids {
			k.parent = wrap
		}
		box.AddChild(wrap)
		inlineRun = nil
	}
	for c := el.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *dom.Element:
			cs := resolveStyleOrDefault(resolver, v)
			if cs.Display == style.DisplayNone {
				continue
			}
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
			if data == "" {
				continue
			}
			run := &LayoutBox{Type: BoxTextRun, Text: data, Style: box.Style}
			inlineRun = append(inlineRun, run)
		}
	}
	flush()
}

// isFlexContainerDisplay reports whether display establishes a flex
// formatting context (flex / inline-flex). Grid is handled separately.
func isFlexContainerDisplay(d style.DisplayType) bool {
	return d == style.DisplayFlex || d == style.DisplayInlineFlex
}

// buildFlexChildren populates a flex/grid container's children. Per CSS flexbox
// §4, every element child of a flex container becomes a flex item directly; no
// anonymous block wrappers are generated. Inline-level children are blockified
// (their box type becomes BoxBlock) so the flex formatting context treats them as
// block-level flex items. Pure whitespace-only text nodes between items are
// dropped (they do not generate flex items); non-whitespace text becomes an
// anonymous block-level flex item wrapping the text.
func buildFlexChildren(box *LayoutBox, el *dom.Element, resolver *style.Resolver) {
	for c := el.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *dom.Element:
			cs := resolveStyleOrDefault(resolver, v)
			if cs.Display == style.DisplayNone {
				continue
			}
			child := newBoxForElement(v, cs)
			// Blockify: an inline-level flex item (inline / inline-block /
			// inline-flex) is treated as block-level for layout. The box type
			// is promoted to BoxBlock; the display value is left untouched so
			// the flex formatting context can still inspect the original
			// display if needed.
			if child.Type == BoxInline {
				child.Type = BoxBlock
			}
			buildChildren(child, v, resolver)
			box.AddChild(child)
		case *dom.Text:
			data := v.Data()
			if data == "" {
				continue
			}
			// Whitespace-only text between flex items does not generate a flex
			// item (CSS flexbox §4: a flex container's inline-level content
			// is blockified; whitespace runs between items are collapsed).
			if strings.TrimSpace(data) == "" {
				continue
			}
			// Non-whitespace text becomes an anonymous block-level flex item.
			// Use inheritedOrNew to avoid leaking non-inheritable properties
			// (like display:flex) into the text run, which would cause it to
			// be incorrectly laid out as a flex container.
			run := &LayoutBox{Type: BoxTextRun, Text: data, Style: inheritedOrNew(box.Style)}
			box.AddChild(run)
		}
	}
}

// inheritedOrNew returns a fresh ComputedStyle for an anonymous box that inherits
// only the inheritable properties (color, font, text, etc.) from the parent.
// Anonymous boxes have no element so they cannot be styled directly; they inherit
// from the parent per CSS spec.
func inheritedOrNew(parent *style.ComputedStyle) *style.ComputedStyle {
	// Anonymous wrappers must inherit only inheritable properties (color,
	// font, text, etc.) from the parent — not non-inherited properties like
	// width/height/display/background. Sharing the parent's ComputedStyle
	// pointer directly causes those properties to leak (e.g. a parent's
	// height:50px makes the wrapper's heightIsAuto() return false, so the
	// inline formatting context never computes the wrapper's height).
	cs := style.NewComputedStyle()
	if parent != nil {
		cs.InheritFrom(parent)
	}
	return cs
}
