// Translation of: Source/WebCore/style/computed/StyleComputedStyle.h
//                  Source/WebCore/style/computed/StyleComputedStyle.cpp
// Completeness: 70%
// Architecture:
//   - InheritedData / NonInheritedData separate CSS properties by inheritance,
//     wrapped as struct fields for direct access
//   - DataRef used Internally by Clone() / InheritFrom() for COW optimization
//   - All original flat field access is preserved for backward compatibility

package style

import (
	"strconv"
	"strings"

	"wb-ui/css"
)

// DisplayType mirrors WebCore::DisplayType.
type DisplayType int

const (
	DisplayInline DisplayType = iota
	DisplayBlock
	DisplayInlineBlock
	DisplayListItem
	DisplayNone
	DisplayContents
	DisplayFlowRoot
	DisplayTable
	DisplayInlineTable
	DisplayTableRowGroup
	DisplayTableHeaderGroup
	DisplayTableFooterGroup
	DisplayTableRow
	DisplayTableColumnGroup
	DisplayTableColumn
	DisplayTableCell
	DisplayTableCaption
	DisplayFlex
	DisplayInlineFlex
	DisplayGrid
	DisplayInlineGrid
)

// PositionType mirrors WebCore::PositionType.
type PositionType int

const (
	PositionStatic PositionType = iota
	PositionRelative
	PositionAbsolute
	PositionFixed
	PositionSticky
)

// OverflowType mirrors WebCore::OverflowType.
type OverflowType int

const (
	OverflowVisible OverflowType = iota
	OverflowHidden
	OverflowScroll
	OverflowAuto
)

// BorderCollapseType mirrors WebCore::BorderCollapse.
type BorderCollapseType int

const (
	BorderCollapseSeparate BorderCollapseType = iota
	BorderCollapseCollapse
)

// WhiteSpaceType mirrors WebCore::WhiteSpaceType.
type WhiteSpaceType int

const (
	WhiteSpaceNormal WhiteSpaceType = iota
	WhiteSpacePre
	WhiteSpaceNoWrap
	WhiteSpacePreWrap
	WhiteSpacePreLine
	WhiteSpaceBreakSpaces
)

// TextOverflowType mirrors WebCore::TextOverflowType.
type TextOverflowType int

const (
	TextOverflowClip TextOverflowType = iota
	TextOverflowEllipsis
)
type TextAlignType int

const (
	TextAlignStart TextAlignType = iota
	TextAlignEnd
	TextAlignLeft
	TextAlignRight
	TextAlignCenter
	TextAlignJustify
	// ★ legacy 对齐值（HTML 的 <center> UA 样式 = text-align:-webkit-center）。
	// 行内内容表现与对应标准值相同，额外让**块级子盒**水平居中/靠边——
	// 见 TextAlignType.InlineEquivalent 与 blockformattingcontext 的定位分支。
	TextAlignWebkitCenter
	TextAlignWebkitLeft
	TextAlignWebkitRight
)

// Length represents a CSS length value with a numeric value and a unit.
type Length struct {
	Value float64
	Unit  string
	// CalcExpr holds the raw calc() inner expression (e.g. "100% - 40px")
	// when Unit == "calc". It is set when a calc() contains relative units
	// that cannot be resolved at style-resolution time and must be re-evaluated
	// by the layout engine with real containing-block / font-size context.
	CalcExpr string
}

// IsAuto reports whether the length is the keyword "auto".
func (l Length) IsAuto() bool { return l.Unit == "auto" }

// String renders the length back to CSS text.
func (l Length) String() string {
	if l.IsAuto() {
		return "auto"
	}
	if l.Unit == "normal" {
		return "normal"
	}
	return formatFloat(l.Value) + l.Unit
}

// Color represents an RGBA color.
type Color struct {
	R, G, B, A uint8
}

// String renders the color as #rrggbb (or #rrggbbaa if alpha < 0xff).
func (c Color) String() string {
	var sb strings.Builder
	sb.WriteByte('#')
	sb.WriteString(hexByte(c.R))
	sb.WriteString(hexByte(c.G))
	sb.WriteString(hexByte(c.B))
	if c.A != 0xFF {
		sb.WriteString(hexByte(c.A))
	}
	return sb.String()
}

func hexByte(b uint8) string {
	const hexDigits = "0123456789abcdef"
	return string([]byte{hexDigits[b>>4], hexDigits[b&0x0F]})
}

// ──────────────────────────────────────────────
// ComputedStyle — main style representation
// ──────────────────────────────────────────────

// ComputedStyle holds the final value of every CSS property for an element.
// Fields are directly accessible for performance; the struct also maintains
// DataRef backing for copy-on-write Clone/InheritFrom.
type ComputedStyle struct {
	// ── InheritedData (properties that inherit by default) ──
	InheritedData

	// ── NonInheritedData (properties that do NOT inherit) ──
	NonInheritedData

	// ── Per-element data (not shared via DataRef) ──
	CustomProperties    map[string][]css.Token
	Properties          map[string]string
	ImportantProperties map[string]bool
	CalcValues          map[string][]css.Token

	// ── Internal DataRef for COW optimization ──
	inheritedRef    DataRef[InheritedData]
	nonInheritedRef DataRef[NonInheritedData]
}

// NewComputedStyle returns a ComputedStyle with spec-default values.
func NewComputedStyle() *ComputedStyle {
	cs := &ComputedStyle{
		InheritedData:    *DefaultInheritedData(),
		NonInheritedData: *DefaultNonInheritedData(),
		CustomProperties:    map[string][]css.Token{},
		Properties:          map[string]string{},
		ImportantProperties: map[string]bool{},
		CalcValues:          map[string][]css.Token{},
	}
	cs.syncRefs()
	return cs
}

// syncRefs creates DataRefs pointing to the current field data.
func (c *ComputedStyle) syncRefs() {
	data := c.InheritedData
	c.inheritedRef = NewDataRef(&data)
	ndata := c.NonInheritedData
	c.nonInheritedRef = NewDataRef(&ndata)
}

// Clone returns a deep copy, sharing inherited data where possible.
func (c *ComputedStyle) Clone() *ComputedStyle {
	if c == nil {
		return nil
	}
	cp := &ComputedStyle{
		InheritedData:    c.InheritedData,
		NonInheritedData: c.NonInheritedData,

		CustomProperties:    cloneMapCT(c.CustomProperties),
		Properties:          cloneMapSS(c.Properties),
		ImportantProperties: cloneMapSB(c.ImportantProperties),
		CalcValues:          cloneMapCT(c.CalcValues),
	}
	return cp
}

func cloneMapSS(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneMapSB(src map[string]bool) map[string]bool {
	if src == nil {
		return nil
	}
	dst := make(map[string]bool, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneMapCT(src map[string][]css.Token) map[string][]css.Token {
	if src == nil {
		return nil
	}
	dst := make(map[string][]css.Token, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}


// InheritFrom copies inherited properties from parent.
func (c *ComputedStyle) InheritFrom(parent *ComputedStyle) {
	if parent == nil {
		return
	}
	// Copy all inherited fields.
	c.InheritedData = parent.InheritedData
	// CSS custom properties are inherited — copy them from parent
	// so that var(--xxx) references in child elements resolve correctly.
	for k, v := range parent.CustomProperties {
		c.SetCustomProperty(k, v)
	}
	c.syncRefs()
}

// BorderColor returns the effective border color for a side, implementing the
// CSS `currentcolor` semantics: when no explicit border color was specified
// (the initial value is currentcolor), the border uses the element's color.
// A side whose Border*Color is fully transparent (zero alpha, the zero value)
// and whose style is not "none" is treated as unset and falls back to Color.
func (c *ComputedStyle) BorderColor(side string) Color {
	col := Color{R: 0, G: 0, B: 0, A: 0}
	switch side {
	case "top":
		col = c.BorderTopColor
	case "right":
		col = c.BorderRightColor
	case "bottom":
		col = c.BorderBottomColor
	case "left":
		col = c.BorderLeftColor
	}
	// Zero-value (transparent) border color → currentcolor fallback, UNLESS
	// the color was explicitly set (e.g. `border: 1px solid transparent`),
	// in which case the transparent alpha is preserved so painters skip it.
	if col.A == 0 && col.R == 0 && col.G == 0 && col.B == 0 {
		var set bool
		switch side {
		case "top":
			set = c.BorderTopColorSet
		case "right":
			set = c.BorderRightColorSet
		case "bottom":
			set = c.BorderBottomColorSet
		case "left":
			set = c.BorderLeftColorSet
		}
		if !set {
			return c.Color
		}
	}
	return col
}

// GetProperty returns the raw string value of the named property.
func (c *ComputedStyle) GetProperty(name string) string {
	name = strings.ToLower(name)
	switch name {
	case "display":
		return displayTypeName(c.Display)
	case "position":
		return positionTypeName(c.Position)
	case "color":
		return c.Color.String()
	case "background-color":
		return c.BackgroundColor.String()
	case "font-size":
		return c.FontSize.String()
	case "font-weight":
		return c.FontWeight
	case "font-style":
		return c.FontStyle
	case "line-height":
		return c.LineHeight.String()
	case "width":
		return c.Width.String()
	case "height":
		return c.Height.String()
	case "margin-top":
		return c.MarginTop.String()
	case "margin-right":
		return c.MarginRight.String()
	case "margin-bottom":
		return c.MarginBottom.String()
	case "margin-left":
		return c.MarginLeft.String()
	case "padding-top":
		return c.PaddingTop.String()
	case "padding-right":
		return c.PaddingRight.String()
	case "padding-bottom":
		return c.PaddingBottom.String()
	case "padding-left":
		return c.PaddingLeft.String()
	case "border-top-width":
		return c.BorderTopWidth.String()
	case "border-top-style":
		return c.BorderTopStyle
	case "border-top-color":
		return c.BorderTopColor.String()
	case "border-right-width":
		return c.BorderRightWidth.String()
	case "border-right-style":
		return c.BorderRightStyle
	case "border-right-color":
		return c.BorderRightColor.String()
	case "border-bottom-width":
		return c.BorderBottomWidth.String()
	case "border-bottom-style":
		return c.BorderBottomStyle
	case "border-bottom-color":
		return c.BorderBottomColor.String()
	case "border-left-width":
		return c.BorderLeftWidth.String()
	case "border-left-style":
		return c.BorderLeftStyle
	case "border-left-color":
		return c.BorderLeftColor.String()
	case "box-sizing":
		return c.BoxSizing
	case "border-radius":
		return c.BorderRadius.String()
	case "overflow-x":
		return overflowTypeName(c.OverflowX)
	case "overflow-y":
		return overflowTypeName(c.OverflowY)
	case "float":
		return c.Float
	case "clear":
		return c.Clear
	case "font-family":
		return c.FontFamily
	case "text-align":
		return textAlignTypeName(c.TextAlign)
	case "white-space":
		return whiteSpaceTypeName(c.WhiteSpace)
	case "word-break":
		if c.WordBreak != "" {
			return c.WordBreak
		}
		if c.Properties != nil {
			return c.Properties["word-break"]
		}
		return ""
	case "overflow-wrap":
		if c.OverflowWrap != "" {
			return c.OverflowWrap
		}
		if c.Properties != nil {
			return c.Properties["overflow-wrap"]
		}
		return ""
	case "direction":
		return c.Direction
	case "writing-mode":
		return c.WritingMode
	case "visibility":
		return c.Visibility
	case "opacity":
		return formatFloat(c.Opacity)
	case "z-index":
		return strconv.Itoa(c.ZIndex)
	case "flex-direction":
		return c.FlexDirection
	case "flex-wrap":
		return c.FlexWrap
	case "flex-grow":
		return formatFloat(c.FlexGrow)
	case "flex-shrink":
		return formatFloat(c.FlexShrink)
	case "flex-basis":
		return c.FlexBasis.String()
	case "order":
		return strconv.Itoa(c.Order)
	case "justify-content":
		return c.JustifyContent
	case "align-items":
		return c.AlignItems
	case "align-content":
		return c.AlignContent
	case "align-self":
		return c.AlignSelf
	case "justify-self":
		return c.JustifySelf
	case "justify-items":
		return c.JustifyItems
	case "gap":
		return c.Gap.String()
	case "row-gap":
		return c.RowGap.String()
	case "column-gap":
		return c.ColumnGap.String()
	case "grid-template-columns":
		return c.GridTemplateColumns
	case "grid-template-rows":
		return c.GridTemplateRows
	case "grid-auto-flow":
		return c.GridAutoFlow
	case "column-count":
		return strconv.Itoa(c.ColumnCount)
	case "column-width":
		return c.ColumnWidth.String()
	case "column-fill":
		return c.ColumnFill
	case "cursor":
		return c.Cursor
	case "user-select":
		return c.UserSelect
	case "pointer-events":
		return c.PointerEvents
	}
	if c.Properties != nil {
		return c.Properties[name]
	}
	return ""
}

// SetProperty stores a raw property value.
func (c *ComputedStyle) SetProperty(name, value string) {
	if c.Properties == nil {
		c.Properties = map[string]string{}
	}
	c.Properties[strings.ToLower(name)] = value
}

// SetCustomProperty stores a CSS variable.
func (c *ComputedStyle) SetCustomProperty(name string, value []css.Token) {
	if c.CustomProperties == nil {
		c.CustomProperties = map[string][]css.Token{}
	}
	c.CustomProperties[name] = value
}

// GetCustomProperty returns the raw token slice for a CSS variable, or nil.
func (c *ComputedStyle) GetCustomProperty(name string) []css.Token {
	if c.CustomProperties == nil {
		return nil
	}
	return c.CustomProperties[name]
}

// String renders important style properties for debugging.
func (c *ComputedStyle) String() string {
	if c == nil {
		return "nil"
	}
	var sb strings.Builder
	sb.WriteString("ComputedStyle{")
	sb.WriteString("display=")
	sb.WriteString(displayTypeName(c.Display))
	sb.WriteString(", position=")
	sb.WriteString(positionTypeName(c.Position))
	sb.WriteString(", fontSize=")
	sb.WriteString(c.FontSize.String())
	sb.WriteString("}")
	return sb.String()
}

// ── format helpers ──

func formatFloat(v float64) string {
	if v == float64(int(v)) {
		return strconv.Itoa(int(v))
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func displayTypeName(d DisplayType) string {
	switch d {
	case DisplayInline:
		return "inline"
	case DisplayBlock:
		return "block"
	case DisplayInlineBlock:
		return "inline-block"
	case DisplayListItem:
		return "list-item"
	case DisplayNone:
		return "none"
	case DisplayContents:
		return "contents"
	case DisplayFlowRoot:
		return "flow-root"
	case DisplayTable:
		return "table"
	case DisplayInlineTable:
		return "inline-table"
	case DisplayTableRowGroup:
		return "table-row-group"
	case DisplayTableHeaderGroup:
		return "table-header-group"
	case DisplayTableFooterGroup:
		return "table-footer-group"
	case DisplayTableRow:
		return "table-row"
	case DisplayTableColumnGroup:
		return "table-column-group"
	case DisplayTableColumn:
		return "table-column"
	case DisplayTableCell:
		return "table-cell"
	case DisplayTableCaption:
		return "table-caption"
	case DisplayFlex:
		return "flex"
	case DisplayInlineFlex:
		return "inline-flex"
	case DisplayGrid:
		return "grid"
	case DisplayInlineGrid:
		return "inline-grid"
	}
	return "inline"
}

// String returns the CSS display keyword for the DisplayType (e.g. "flex").
func (d DisplayType) String() string { return displayTypeName(d) }

func positionTypeName(p PositionType) string {
	switch p {
	case PositionStatic:
		return "static"
	case PositionRelative:
		return "relative"
	case PositionAbsolute:
		return "absolute"
	case PositionFixed:
		return "fixed"
	case PositionSticky:
		return "sticky"
	}
	return "static"
}

func overflowTypeName(o OverflowType) string {
	switch o {
	case OverflowVisible:
		return "visible"
	case OverflowHidden:
		return "hidden"
	case OverflowScroll:
		return "scroll"
	case OverflowAuto:
		return "auto"
	}
	return "visible"
}

// String returns the CSS keyword for the OverflowType (e.g. "auto").
func (o OverflowType) String() string { return overflowTypeName(o) }

func whiteSpaceTypeName(w WhiteSpaceType) string {
	switch w {
	case WhiteSpaceNormal:
		return "normal"
	case WhiteSpacePre:
		return "pre"
	case WhiteSpaceNoWrap:
		return "nowrap"
	case WhiteSpacePreWrap:
		return "pre-wrap"
	case WhiteSpacePreLine:
		return "pre-line"
	case WhiteSpaceBreakSpaces:
		return "break-spaces"
	}
	return "normal"
}

func textAlignTypeName(t TextAlignType) string {
	switch t {
	case TextAlignStart:
		return "start"
	case TextAlignEnd:
		return "end"
	case TextAlignLeft:
		return "left"
	case TextAlignRight:
		return "right"
	case TextAlignCenter:
		return "center"
	case TextAlignJustify:
		return "justify"
	case TextAlignWebkitCenter:
		return "-webkit-center"
	case TextAlignWebkitLeft:
		return "-webkit-left"
	case TextAlignWebkitRight:
		return "-webkit-right"
	}
	return "start"
}

// InlineEquivalent 返回 legacy 对齐值在**行内内容**上的等价标准值。
//
// HTML 的 <center> 元素 UA 样式是 `text-align: -webkit-center`：行内内容的
// 居中表现与 `center` 完全一致（legacy-center 夹具的 #pure-center 行），
// 两者的区别只在于 legacy 值还让**块级子盒**水平居中。因此行内布局统一
// 通过本方法归一后再对齐，块级子盒居中则由 BFC 的定位分支处理。
func (t TextAlignType) InlineEquivalent() TextAlignType {
	switch t {
	case TextAlignWebkitCenter:
		return TextAlignCenter
	case TextAlignWebkitLeft:
		return TextAlignLeft
	case TextAlignWebkitRight:
		return TextAlignRight
	}
	return t
}

// LegacyBlockAlign 报告 legacy 对齐值的块级子盒对齐方向："center"（居中）、
// "right"（靠右）、"left"（靠左）；非 legacy 值返回空串。
func (t TextAlignType) LegacyBlockAlign() string {
	switch t {
	case TextAlignWebkitCenter:
		return "center"
	case TextAlignWebkitRight:
		return "right"
	case TextAlignWebkitLeft:
		return "left"
	}
	return ""
}

// ── Lookup functions (used by the resolver) ──

// LookupDisplayType returns the DisplayType for a CSS display string.
func LookupDisplayType(s string) DisplayType {
	s = strings.TrimSpace(s)
	switch s {
	case "block":
		return DisplayBlock
	case "inline-block":
		return DisplayInlineBlock
	case "list-item":
		return DisplayListItem
	case "none":
		return DisplayNone
	case "contents":
		return DisplayContents
	case "flow-root":
		return DisplayFlowRoot
	case "flex":
		return DisplayFlex
	case "inline-flex":
		return DisplayInlineFlex
	case "grid":
		return DisplayGrid
	case "inline-grid":
		return DisplayInlineGrid
	case "table":
		return DisplayTable
	case "inline-table":
		return DisplayInlineTable
	case "table-row":
		return DisplayTableRow
	case "table-cell":
		return DisplayTableCell
	case "table-caption":
		return DisplayTableCaption
	case "table-row-group":
		return DisplayTableRowGroup
	case "table-header-group":
		return DisplayTableHeaderGroup
	case "table-footer-group":
		return DisplayTableFooterGroup
	case "table-column":
		return DisplayTableColumn
	case "table-column-group":
		return DisplayTableColumnGroup
	default:
		return DisplayInline
	}
}

// LookupPositionType returns the PositionType for a CSS position string.
func LookupPositionType(s string) PositionType {
	s = strings.TrimSpace(s)
	switch s {
	case "relative":
		return PositionRelative
	case "absolute":
		return PositionAbsolute
	case "fixed":
		return PositionFixed
	case "sticky":
		return PositionSticky
	default:
		return PositionStatic
	}
}

// LookupWhiteSpace returns the WhiteSpaceType for a CSS white-space string.
func LookupWhiteSpace(s string) WhiteSpaceType {
	s = strings.TrimSpace(s)
	switch s {
	case "pre":
		return WhiteSpacePre
	case "nowrap":
		return WhiteSpaceNoWrap
	case "pre-wrap":
		return WhiteSpacePreWrap
	case "pre-line":
		return WhiteSpacePreLine
	case "break-spaces":
		return WhiteSpaceBreakSpaces
	default:
		return WhiteSpaceNormal
	}
}

// LookupOverflow returns the OverflowType for a CSS overflow string.
func LookupOverflow(s string) OverflowType {
	s = strings.TrimSpace(s)
	switch s {
	case "hidden":
		return OverflowHidden
	case "scroll":
		return OverflowScroll
	case "auto":
		return OverflowAuto
	default:
		return OverflowVisible
	}
}

// parseBorderCollapse resolves the border-collapse CSS keyword.
func parseBorderCollapse(s string) BorderCollapseType {
	switch s {
	case "collapse":
		return BorderCollapseCollapse
	default:
		return BorderCollapseSeparate
	}
}

// BorderCollapseName returns the CSS string for a BorderCollapseType.
func BorderCollapseName(t BorderCollapseType) string {
	if t == BorderCollapseCollapse {
		return "collapse"
	}
	return "separate"
}

// TextOverflowTypeName returns the CSS string for a TextOverflowType.
func TextOverflowTypeName(t TextOverflowType) string {
	switch t {
	case TextOverflowClip:
		return "clip"
	case TextOverflowEllipsis:
		return "ellipsis"
	default:
		return "clip"
	}
}

// LookupTextOverflow returns the TextOverflowType for a CSS text-overflow string.
func LookupTextOverflow(s string) TextOverflowType {
	s = strings.TrimSpace(s)
	switch s {
	case "ellipsis":
		return TextOverflowEllipsis
	default:
		return TextOverflowClip
	}
}
