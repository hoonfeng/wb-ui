// Translation of: Source/WebCore/style/computed/StyleComputedStyle.h
//                  Source/WebCore/style/computed/StyleComputedStyle.cpp
//                  Source/WebCore/style/computed/StyleComputedStyleBase.h
//                  Source/WebCore/style/computed/StyleComputedStyleBase.cpp
// Completeness: 60%
// Simplifications:
//   - the C++ ComputedStyle is a bit-packed struct with dozens of inherited vs.
//     non-inherited data sub-structures; the Go port stores fields directly on a
//     single struct with a Properties map for unknown / unhandled keys
//   - all values are stored as their textual form; type-safe accessors (e.g.
//     GetDisplay returning DisplayType) wrap string parsing lazily
//   - inheritance is implemented via an InheritFrom call that copies the inherited
//     fields from the parent ComputedStyle
//   - custom properties (CSS variables) are stored in the CustomProperties map as
//     raw token slices; var() resolution is performed by the resolver
//   - many of the ~100 spec properties are exposed but not all have dedicated
//     type-safe accessors — callers can read raw values via GetProperty

package style

import (
	"strconv"
	"strings"

	"wb-ui/css"
	"wb-ui/platform/graphics"
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

// TextAlignType mirrors WebCore::TextAlignType.
type TextAlignType int

const (
	TextAlignStart TextAlignType = iota
	TextAlignEnd
	TextAlignLeft
	TextAlignRight
	TextAlignCenter
	TextAlignJustify
)

// Length represents a CSS length value with a numeric value and a unit. For lengths
// that are not absolute (px, pt, etc.) the resolver may store the original unit and
// defer resolution to layout time.
type Length struct {
	Value float64
	Unit  string
}

// IsAuto reports whether the length is the keyword "auto".
func (l Length) IsAuto() bool { return l.Unit == "auto" }

// String renders the length back to CSS text.
func (l Length) String() string {
	if l.IsAuto() {
		return "auto"
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

// ComputedStyle is the Go translation of WebCore::Style::ComputedStyle. It holds the
// final value of every CSS property for an element after the cascade. Properties that
// the resolver knows how to interpret have typed fields; everything else lives in the
// Properties map keyed by the canonical property name (lowercase).
type ComputedStyle struct {
	// Box model.
	Display           DisplayType
	Position          PositionType
	Float             string
	Clear             string
	OverflowX         OverflowType
	OverflowY         OverflowType
	Width             Length
	Height            Length
	MinWidth          Length
	MinHeight         Length
	MaxWidth          Length
	MaxHeight         Length
	MarginTop         Length
	MarginRight       Length
	MarginBottom      Length
	MarginLeft        Length
	PaddingTop        Length
	PaddingRight      Length
	PaddingBottom     Length
	PaddingLeft       Length
	BorderTopWidth    Length
	BorderRightWidth  Length
	BorderBottomWidth Length
	BorderLeftWidth   Length
	BorderTopColor    Color
	BorderRightColor  Color
	BorderBottomColor Color
	BorderLeftColor   Color
	BorderTopStyle    string
	BorderRightStyle  string
	BorderBottomStyle string
	BorderLeftStyle   string
	BoxSizing         string

	// Border radius (simple single-value model: border-radius: 8px).
	// All four corners share the same radius; per-corner radii are not yet modeled.
	BorderRadius Length

	// Colors.
	Color           Color
	BackgroundColor Color

	// Font.
	FontFamily     string
	FontSize       Length
	FontStyle      string
	FontWeight     string
	FontVariant    string
	LineHeight     Length
	LetterSpacing  Length
	WordSpacing    Length
	TextIndent     Length
	TextAlign      TextAlignType
	TextDecoration string
	TextTransform  string
	WhiteSpace     WhiteSpaceType
	Direction      string
	UnicodeBidi    string

	// Background.
	BackgroundImage      string
	BackgroundRepeat     string
	BackgroundPosition   string
	BackgroundSize       string
	BackgroundAttachment string
	BackgroundClip       string
	BackgroundOrigin     string

	// Flex / Grid container.
	FlexDirection       string
	FlexWrap            string
	JustifyContent      string
	AlignItems          string
	AlignContent        string
	GridTemplateColumns string
	GridTemplateRows    string
	GridTemplateAreas   string
	GridAutoFlow        string
	GridAutoColumns     string
	GridAutoRows        string
	Gap                 Length
	RowGap              Length
	ColumnGap           Length

	// Multi-column layout.
	ColumnCount      int    // 0 = auto (use column-width)
	ColumnWidth      Length // zero Value = auto
	ColumnRuleColor  string // "none" (default) or CSS color
	ColumnRuleStyle  string // "none" (default), "solid", "dotted", "dashed", "double"
	ColumnRuleWidth  Length
	ColumnFill       string // "balance" (default) or "auto"

	// Writing mode.
	WritingMode string // "horizontal-tb" (default), "vertical-rl", "vertical-lr"

	// Flex / Grid item.
	FlexBasis       Length
	FlexGrow        float64
	FlexShrink      float64
	Order           int
	AlignSelf       string
	JustifySelf     string
	GridRowStart    string
	GridRowEnd      string
	GridColumnStart string
	GridColumnEnd   string

	// Text / list.
	ListStyleType     string
	ListStylePosition string
	ListStyleImage    string
	VerticalAlign     string

	// Visibility / opacity.
	Visibility string
	Opacity    float64
	ZIndex     int

	// Misc commonly-used properties.
	Content                 string
	Cursor                  string
	UserSelect              string
	PointerEvents           string
	BoxShadow               string
	TextShadow              string
	Transform               string
	Transition              string
	Animation               string
	AnimationName           string
	AnimationDuration       float64 // seconds
	AnimationIterationCount int     // 0 = infinite
	AnimationDelay          float64 // seconds; 0 = no delay
	AnimationFillMode       string  // "none" (default), "forwards", "backwards", "both"
	AnimationDirection      string  // "normal" (default), "reverse", "alternate", "alternate-reverse"
	AnimationTimingFunction string  // "linear" (default), "ease", "ease-in", "ease-out", "ease-in-out"
	Filter                  string
	BackdropFilter          string

	// Animated transform properties (set by the animation engine, not by the resolver).
	// These are applied on top of any base Transform string.
	TranslateX float64
	TranslateY float64
	ScaleX     float64
	ScaleY     float64

	// Animated color properties (set by the animation engine for @keyframes color/background-color).
	AnimatedColor           graphics.Color
	AnimatedBackgroundColor graphics.Color

	// Inherited bit. Most font/text/color properties inherit; the resolver sets this
	// when copying from the parent.
	InheritedFrom *ComputedStyle

	// CustomProperties holds CSS variables (--foo) registered during the cascade as
	// raw token slices. The resolver resolves var() references against this map.
	CustomProperties map[string][]css.Token

	// Properties holds the raw string value of any property that does not have a
	// dedicated field above. This includes shorthands that the resolver did not
	// expand and unknown properties.
	Properties map[string]string

	// ImportantProperties records the names of properties that were set with
	// !important. Used by the cascade to override earlier declarations.
	ImportantProperties map[string]bool


	// CalcValues holds the raw token slices of calc() expressions that could not
	// be fully resolved at style resolution time (e.g. those containing % which
	// depends on parent layout width). Call ResolveLengthValue during layout with
	// the appropriate CalcContext to obtain the pixel value.
	CalcValues map[string][]css.Token


	// default (block for div/body/html, inline for span, etc.) so that elements
	// without an explicit display value render with the correct UA-default type.
	DisplaySet bool
}

// NewComputedStyle returns a ComputedStyle initialized with default property values
// that match WebKit's computed style defaults for the document root.
func NewComputedStyle() *ComputedStyle {
	return &ComputedStyle{
		Display:             DisplayInline,
		Position:            PositionStatic,
		OverflowX:           OverflowVisible,
		OverflowY:           OverflowVisible,
		Color:               Color{R: 0, G: 0, B: 0, A: 0xFF},
		BackgroundColor:     Color{R: 0, G: 0, B: 0, A: 0},
		FontSize:            Length{Value: 16, Unit: "px"},
		FontFamily:          "serif",
		FontWeight:          "400",
		FontStyle:           "normal",
		LineHeight:          Length{Value: 1.2, Unit: ""},
		TextAlign:           TextAlignStart,
		WhiteSpace:          WhiteSpaceNormal,
		Direction:           "ltr",
		Visibility:          "visible",
		Opacity:             1.0,
		ZIndex:              0,
		BorderTopStyle:      "none",
		BorderRightStyle:    "none",
		BorderBottomStyle:   "none",
		BorderLeftStyle:     "none",
		FlexGrow:            0,
		FlexShrink:          1,
		Order:               0,
		ColumnCount:         0,      // auto
		ColumnRuleStyle:     "none", // no column rule
		ColumnFill:          "balance",
		WritingMode:         "horizontal-tb",
		CustomProperties:    map[string][]css.Token{},
		Properties:          map[string]string{},
		ImportantProperties: map[string]bool{},
		CalcValues:          map[string][]css.Token{},
		DisplaySet:          false,
	}
}

// GetProperty returns the raw string value of the named property. Looks first at the
// typed fields, then at the Properties map. Returns "" when not set.
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
	case "visibility":
		return c.Visibility
	case "opacity":
		return formatFloat(c.Opacity)
	case "z-index":
		return intToString(c.ZIndex)
	case "flex-direction":
		return c.FlexDirection
	case "justify-content":
		return c.JustifyContent
	case "align-items":
		return c.AlignItems
	case "flex-grow":
		return formatFloat(c.FlexGrow)
	case "column-count":
		return intToString(c.ColumnCount)
	case "column-width":
		return c.ColumnWidth.String()
	case "column-gap":
		return c.ColumnGap.String()
	case "column-rule-color":
		return c.ColumnRuleColor
	case "column-rule-style":
		return c.ColumnRuleStyle
	case "column-rule-width":
		return c.ColumnRuleWidth.String()
	case "column-fill":
		return c.ColumnFill
	case "writing-mode":
		return c.WritingMode
	case "flex-shrink":
		return formatFloat(c.FlexShrink)
	case "order":
		return intToString(c.ZIndex)
	}
	return c.Properties[name]
}

// SetProperty stores the raw string value of a property in the Properties map. This
// is the fallback path for properties without a dedicated typed field; the typed
// accessors are populated by the resolver, not by this method.
func (c *ComputedStyle) SetProperty(name, value string) {
	c.Properties[strings.ToLower(name)] = value
}

// SetCustomProperty stores a CSS variable. Mirrors the custom property storage on
// StyleCustomPropertyData.
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

// InheritFrom copies inherited properties from parent. Per the spec, only
// inheritable properties (color, font, text, list, visibility, etc.) are copied.
// Custom properties are also inherited.
func (c *ComputedStyle) InheritFrom(parent *ComputedStyle) {
	if parent == nil {
		return
	}
	c.InheritedFrom = parent
	c.Color = parent.Color
	c.FontFamily = parent.FontFamily
	c.FontSize = parent.FontSize
	c.FontStyle = parent.FontStyle
	c.FontWeight = parent.FontWeight
	c.FontVariant = parent.FontVariant
	c.LineHeight = parent.LineHeight
	c.LetterSpacing = parent.LetterSpacing
	c.WordSpacing = parent.WordSpacing
	c.TextIndent = parent.TextIndent
	c.TextAlign = parent.TextAlign
	c.TextDecoration = parent.TextDecoration
	c.TextTransform = parent.TextTransform
	c.WhiteSpace = parent.WhiteSpace
	c.Direction = parent.Direction
	c.UnicodeBidi = parent.UnicodeBidi
	c.Visibility = parent.Visibility
	c.ListStyleType = parent.ListStyleType
	c.ListStylePosition = parent.ListStylePosition
	c.ListStyleImage = parent.ListStyleImage
	c.Cursor = parent.Cursor
	c.UserSelect = parent.UserSelect
	c.Opacity = parent.Opacity
	c.WritingMode = parent.WritingMode
	// Custom properties inherit.
	if c.CustomProperties == nil {
		c.CustomProperties = map[string][]css.Token{}
	for k, v := range parent.CustomProperties {
		if _, ok := c.CustomProperties[k]; !ok {
			c.CustomProperties[k] = v
		}
	}
}
}

// ResolveLengthValue evaluates a deferred calc() expression for the named property
// using the given layout context. Returns the resolved pixel value and true if the
// property has a deferred calc expression; otherwise returns (0, false).
//
// Call this during layout when the parent dimensions, font size, and viewport
// size are known. For non-calc properties or properties that were already resolved
// at style time, the method returns false.
func (c *ComputedStyle) ResolveLengthValue(name string, ctx css.CalcContext) (float64, bool) {
	tokens, ok := c.CalcValues[name]
	if !ok || len(tokens) == 0 {
		return 0, false
	}
	result, err := css.EvalCalc(tokens, ctx)
	if err != nil {
		return 0, false
	}
	return result, true
}

// displayTypeName returns the CSS keyword for a DisplayType.
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
	return ""
}

// positionTypeName returns the CSS keyword for a PositionType.
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
	return ""
}

// LookupDisplayType returns the DisplayType for a keyword, defaulting to inline for
// unknown values.
func LookupDisplayType(name string) DisplayType {
	switch strings.ToLower(name) {
	case "inline":
		return DisplayInline
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
	case "table":
		return DisplayTable
	case "inline-table":
		return DisplayInlineTable
	case "table-row-group":
		return DisplayTableRowGroup
	case "table-header-group":
		return DisplayTableHeaderGroup
	case "table-footer-group":
		return DisplayTableFooterGroup
	case "table-row":
		return DisplayTableRow
	case "table-column-group":
		return DisplayTableColumnGroup
	case "table-column":
		return DisplayTableColumn
	case "table-cell":
		return DisplayTableCell
	case "table-caption":
		return DisplayTableCaption
	case "flex":
		return DisplayFlex
	case "inline-flex":
		return DisplayInlineFlex
	case "grid":
		return DisplayGrid
	case "inline-grid":
		return DisplayInlineGrid
	}
	return DisplayInline
}

// LookupPositionType returns the PositionType for a keyword.
func LookupPositionType(name string) PositionType {
	switch strings.ToLower(name) {
	case "static":
		return PositionStatic
	case "relative":
		return PositionRelative
	case "absolute":
		return PositionAbsolute
	case "fixed":
		return PositionFixed
	case "sticky":
		return PositionSticky
	}
	return PositionStatic
}

// LookupOverflow returns the OverflowType for a keyword.
func LookupOverflow(name string) OverflowType {
	switch strings.ToLower(name) {
	case "hidden":
		return OverflowHidden
	case "scroll":
		return OverflowScroll
	case "auto":
		return OverflowAuto
	}
	return OverflowVisible
}

// LookupWhiteSpace returns the WhiteSpaceType for a keyword.
func LookupWhiteSpace(name string) WhiteSpaceType {
	switch strings.ToLower(name) {
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
	}
	return WhiteSpaceNormal
}

// formatFloat renders a float using strconv to avoid reimplementing formatting.
func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// intToString renders an integer without depending on strconv.
func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append(digits, byte('0'+(n%10)))
		n /= 10
	}
	if neg {
		digits = append(digits, '-')
	}
	// reverse
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}
