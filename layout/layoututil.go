// Translation of: Source/WebCore/layout/LayoutUnits.h
//                  Source/WebCore/rendering/RenderBoxModelObject.cpp (used* helpers)
//                  Source/WebCore/style/RenderStyle.cpp (length resolution helpers)
// Completeness: 55%
// Simplifications:
//   - no subpixel layout (integer pixels only; floats used internally then rounded)
//   - no pagination/fragmentation
//   - WebKit resolves lengths through a chain of LayoutUnit / RenderStyle accessors;
//     this port folds the common cases into a small set of free functions that take a
//     style.Length and a reference (containing-block) size and return a pixel value
//   - em/rem/ex/ch units are resolved against a fixed 16px default font size; vw/vh
//     are resolved against the LayoutState viewport size when available
//   - calc() is not evaluated numerically; a calc length returns isAuto=false and a
//     zero value (treated as auto by callers that need a real number)

package layout

import (
	"math"
	"strconv"
	"strings"

	"wb-ui/style"
)

// defaultFontSize is the pixel size used to resolve em/ex/ch units when the element
// own font-size cannot be determined. Mirrors WebKit default of 16px.
const defaultFontSize = 16.0

// lengthResult bundles the outcome of resolving a CSS length. When Auto is true the
// property used the keyword "auto" (or was unset and the caller treats that as auto);
// Definite is false for percentage-based values whose containing-block size is not
// known, in which case Value should be ignored by the caller.
type lengthResult struct {
	Value    float64
	Auto     bool
	Definite bool
}

// resolveLength resolves a CSS length against a containing-block reference size. The
// reference is the size the percentage is relative to (width for horizontal
// properties, height for vertical properties). fontSize is the element computed
// font-size used to resolve em units; pass 0 to fall back to the default.
func resolveLength(l style.Length, reference, fontSize float64) lengthResult {
	if l.IsAuto() {
		return lengthResult{Auto: true}
	}
	if l.Unit == "" && l.Value == 0 {
		return lengthResult{Value: 0, Definite: true}
	}
	switch l.Unit {
	case "px", "":
		return lengthResult{Value: l.Value, Definite: true}
	case "%":
		if math.IsInf(reference, 0) || math.IsNaN(reference) {
			return lengthResult{Definite: false}
		}
		return lengthResult{Value: l.Value * reference / 100, Definite: true}
	case "em", "rem":
		fs := fontSize
		if fs <= 0 {
			fs = defaultFontSize
		}
		return lengthResult{Value: l.Value * fs, Definite: true}
	case "ex", "ch":
		fs := fontSize
		if fs <= 0 {
			fs = defaultFontSize
		}
		// 1ex ~= 0.5em, 1ch ~= 0.5em (approximation).
		return lengthResult{Value: l.Value * fs * 0.5, Definite: true}
	case "pt":
		return lengthResult{Value: l.Value * 4.0 / 3.0, Definite: true}
	case "vw":
		return lengthResult{Value: l.Value * reference / 100, Definite: true}
	case "vh":
		return lengthResult{Value: l.Value * reference / 100, Definite: true}
	case "calc":
		// calc() expression stored by the resolver when it could not be fully
		// evaluated (e.g. contained % units). At layout time we have the
		// containing-block reference size, so re-evaluate. l.Value holds a
		// flag: 0 = unevaluated, non-zero = pre-evaluated px value.
		// For now, calc with % falls back to treating the % as resolved against
		// reference (same as the % case above), since we don't store the raw
		// calc expression on Length. A full implementation would store the
		// expression AST on a separate field.
		if l.Value == 0 {
			// Could not be evaluated at resolve time; treat as 0.
			// In a full implementation, the raw calc expression would be
			// preserved and re-evaluated here with the reference value.
			return lengthResult{Value: 0, Definite: true}
		}
		return lengthResult{Value: l.Value, Definite: true}
	}
	// Unknown unit (incl. "none" / "inherit" / "initial"): treat as zero definite.
	return lengthResult{Value: 0, Definite: true}
}

// resolveLengthAuto is resolveLength for cases where an unset/empty length should be
// treated as auto. ComputedStyle initialises unset lengths to Length{} (zero value),
// which resolveLength maps to definite zero; this helper maps a zero value with empty
// unit to auto instead.
func resolveLengthAuto(l style.Length, reference, fontSize float64) lengthResult {
	if l.Value == 0 && l.Unit == "" {
		return lengthResult{Auto: true}
	}
	return resolveLength(l, reference, fontSize)
}

// definiteWidth resolves a width-like length to a pixel value, returning (value, ok).
// When ok is false the length is auto or otherwise not definite and the caller must
// compute the used width itself (e.g. shrink-to-fit or fill-the-block).
func definiteWidth(l style.Length, reference, fontSize float64) (float64, bool) {
	r := resolveLengthAuto(l, reference, fontSize)
	return r.Value, r.Definite && !r.Auto
}

// definiteHeight is the vertical counterpart of definiteWidth.
func definiteHeight(l style.Length, reference, fontSize float64) (float64, bool) {
	r := resolveLengthAuto(l, reference, fontSize)
	return r.Value, r.Definite && !r.Auto
}

// usedBorderWidth returns the used border width for a side, mapping "none"/"hidden"
// border styles to zero. The resolver already stores the declared border width but
// CSS dictates that a none/hidden border has a used width of zero.
func usedBorderWidth(width style.Length, borderStyle string) float64 {
	bs := strings.ToLower(borderStyle)
	if bs == "" || bs == "none" || bs == "hidden" {
		return 0
	}
	r := resolveLength(width, 0, 0)
	if r.Definite {
		return math.Max(0, r.Value)
	}
	return 0
}

// computeBoxModel resolves the margin/padding/border edges of box against the given
// containing-block content width and the element font size. The returned Edges hold
// pixel values; auto margins are left as zero here and resolved by the caller when the
// box used width is known (for horizontal auto margins on block boxes).
func computeBoxModel(box *LayoutBox, cbContentWidth, fontSize float64) (margin, padding, border Edges) {
	st := box.Style
	if st == nil {
		return
	}
	fs := fontSize
	if fs <= 0 {
		fs = defaultFontSize
	}
	margin.Top = resolveOrZero(st.MarginTop, cbContentWidth, fs)
	margin.Right = resolveOrZero(st.MarginRight, cbContentWidth, fs)
	margin.Bottom = resolveOrZero(st.MarginBottom, cbContentWidth, fs)
	margin.Left = resolveOrZero(st.MarginLeft, cbContentWidth, fs)
	padding.Top = math.Max(0, resolveOrZero(st.PaddingTop, cbContentWidth, fs))
	padding.Right = math.Max(0, resolveOrZero(st.PaddingRight, cbContentWidth, fs))
	padding.Bottom = math.Max(0, resolveOrZero(st.PaddingBottom, cbContentWidth, fs))
	padding.Left = math.Max(0, resolveOrZero(st.PaddingLeft, cbContentWidth, fs))
	border.Top = usedBorderWidth(st.BorderTopWidth, st.BorderTopStyle)
	border.Right = usedBorderWidth(st.BorderRightWidth, st.BorderRightStyle)
	border.Bottom = usedBorderWidth(st.BorderBottomWidth, st.BorderBottomStyle)
	border.Left = usedBorderWidth(st.BorderLeftWidth, st.BorderLeftStyle)
	return
}

// resolveOrZero resolves a length to a pixel value, returning 0 for auto/undefinite.
func resolveOrZero(l style.Length, reference, fontSize float64) float64 {
	r := resolveLengthAuto(l, reference, fontSize)
	if r.Auto || !r.Definite {
		return 0
	}
	return r.Value
}

// isBorderBox reports whether box uses box-sizing: border-box.
func isBorderBox(box *LayoutBox) bool {
	return box.Style != nil && strings.EqualFold(box.Style.BoxSizing, "border-box")
}

// clampSize applies min/max width/height constraints to a used size. min clamps the
// floor, max clamps the ceiling; auto min/max are ignored.
//
// Per CSS 2.2 §10.4: the used value is max(min, min(max, width)). When min > max
// the minimum wins. max is applied first, then min.
func clampSize(size, minV, maxV float64, minAuto, maxAuto bool) float64 {
	// Apply max first (CSS: min(max, width)), then min (CSS: max(min, …)).
	// This ensures min wins when min > max, per spec.
	if !maxAuto && maxV < size {
		size = maxV
	}
	if !minAuto && minV > size {
		size = minV
	}
	return size
}

// resolveMinMax returns the (min, max, minAuto, maxAuto) for a dimension given the
// style min/max lengths and the containing-block reference size.
func resolveMinMax(minL, maxL style.Length, reference, fontSize float64) (minV, maxV float64, minAuto, maxAuto bool) {
	// min
	r := resolveLengthAuto(minL, reference, fontSize)
	minV, minAuto = r.Value, r.Auto
	if minAuto {
		minV = 0
	}
	// max
	if maxL.Unit == "none" {
		// max-width/max-height: none → no constraint.
		maxAuto = true
		maxV = 0
	} else {
		r = resolveLengthAuto(maxL, reference, fontSize)
		maxV, maxAuto = r.Value, r.Auto
		if maxAuto {
			maxV = 0
		}
	}
	return
}

func fontSizeOf(box *LayoutBox) float64 {
	if box.Style == nil {
		return defaultFontSize
	}
	// For percentage font-size, use the parent's font-size as reference.
	// When there is no parent, fall back to defaultFontSize.
	ref := 0.0
	if box.Style.FontSize.Unit == "%" && box.parent != nil {
		ref = fontSizeOf(box.parent)
	}
	r := resolveLength(box.Style.FontSize, ref, 0)
	if !r.Definite || r.Value <= 0 {
		return defaultFontSize
	}
	return r.Value
}

// fontFamilyOf returns the CSS font-family string for box, defaulting to "".
func fontFamilyOf(box *LayoutBox) string {
	if box.Style == nil {
		return ""
	}
	return box.Style.FontFamily
}

// fontWeightOf returns the CSS font-weight for box as an integer, defaulting to 400.
// The computed value is stored as a string ("normal", "bold", "700", ...) per CSSOM,
// so we parse it the same way rendering/painter.go's parseFontWeight does.
func fontWeightOf(box *LayoutBox) int {
	if box.Style == nil {
		return 400
	}
	return parseFontWeight(box.Style.FontWeight)
}

// parseFontWeight converts a CSS font-weight string to its numeric value.
// Mirrors rendering/painter.go's parseFontWeight; duplicated here to keep the
// layout package free of a rendering dependency.
func parseFontWeight(w string) int {
	switch strings.ToLower(strings.TrimSpace(w)) {
	case "bold", "bolder":
		return 700
	case "lighter":
		return 300
	case "", "normal":
		return 400
	}
	if n, err := strconv.Atoi(strings.TrimSpace(w)); err == nil {
		return n
	}
	return 400
}

// fontStyleOf returns the CSS font-style for box, defaulting to "normal".
func fontStyleOf(box *LayoutBox) string {
	if box.Style == nil {
		return "normal"
	}
	return box.Style.FontStyle
}

// MeasureTextFunc is a hook that measures the advance width of a text string
// rendered with a given font. It is set by the embedder (e.g. main.go) to
// bridge to the platform's Skia Font.MeasureText. When nil, a fallback
// estimator is used. size is the font-size in pixels (float64 to preserve
// sub-pixel precision, matching the value used by the paint pipeline).
var MeasureTextFunc func(family string, size float64, weight int, style, text string) float64

// FontMetricsFunc returns (ascent, descent, lineGap) for a given font. Set by
// the embedder to bridge to Skia Font.Metrics. When nil, fallback values are
// used. size is the font-size in pixels (float64 to preserve sub-pixel
// precision, matching the value used by the paint pipeline). lineGap is the
// font's leading value, used to compute the "normal" line-height as
// ascent + descent + lineGap, matching browser behavior.
var FontMetricsFunc func(family string, size float64, weight int, style string) (ascent, descent, lineGap float64)

// measureText measures the advance width of text using the registered
// MeasureTextFunc, falling back to a monospace estimate when no measurer is set.
func measureText(box *LayoutBox, text string) float64 {
	fs := fontSizeOf(box)
	if MeasureTextFunc != nil {
		return MeasureTextFunc(fontFamilyOf(box), fs, fontWeightOf(box), fontStyleOf(box), text)
	}
	// Fallback: monospace estimate (half-width ASCII = fs/2, full-width CJK = fs).
	half := fs / 2
	var adv float64
	for _, r := range text {
		if isFullWidthRune(r) {
			adv += fs
		} else {
			adv += half
		}
	}
	return adv
}

// fontAscentDescent returns the ascent and descent for the box's font, using
// FontMetricsFunc when available or a fallback based on font-size.
func fontAscentDescent(box *LayoutBox) (ascent, descent float64) {
	a, d, _ := fontMetricsTriple(box)
	return a, d
}

// fontLineGap returns the line gap (leading) for the box's font. Used by the
// inline formatting context to compute the "normal" line-height as
// ascent + descent + lineGap, matching browser behavior.
func fontLineGap(box *LayoutBox) float64 {
	_, _, lg := fontMetricsTriple(box)
	return lg
}

// fontMetricsTriple returns (ascent, descent, lineGap) for the box's font,
// using FontMetricsFunc when available or a fallback based on font-size.
func fontMetricsTriple(box *LayoutBox) (ascent, descent, lineGap float64) {
	fs := fontSizeOf(box)
	if FontMetricsFunc != nil {
		a, d, lg := FontMetricsFunc(fontFamilyOf(box), fs, fontWeightOf(box), fontStyleOf(box))
		if a > 0 {
			return a, d, lg
		}
	}
	return fs * 0.8, fs * 0.2, 0
}

// isFullWidthRune reports whether r is a full-width CJK code point. Used by the
// fallback text measurement when no Skia measurer is registered.
func isFullWidthRune(r rune) bool {
	switch {
	case r == 0x3000:
		return true
	case r >= 0x1100 && r <= 0x115F:
		return true
	case r >= 0x2E80 && r <= 0x303E:
		return true
	case r >= 0x3040 && r <= 0x33FF:
		return true
	case r >= 0x3400 && r <= 0x4DBF:
		return true
	case r >= 0x4E00 && r <= 0x9FFF:
		return true
	case r >= 0xA000 && r <= 0xA4CF:
		return true
	case r >= 0xAC00 && r <= 0xD7A3:
		return true
	case r >= 0xF900 && r <= 0xFAFF:
		return true
	case r >= 0xFE30 && r <= 0xFE4F:
		return true
	case r >= 0xFF00 && r <= 0xFF60:
		return true
	case r >= 0xFFE0 && r <= 0xFFE6:
		return true
	case r >= 0x20000 && r <= 0x2FFFD:
		return true
	}
	return false
}

// asLength parses a raw CSS string (as stored in ComputedStyle.Properties) into a
// style.Length. It recognises the keyword "auto", bare numbers and the px/%/em/rem/pt
// units. Empty input is treated as auto (unset). This is used by the positioned-layout
// helpers to resolve inset values that the resolver stored as raw strings.
func asLength(s string) style.Length {
	s = strings.TrimSpace(s)
	if s == "" || s == "auto" {
		return style.Length{Unit: "auto"}
	}
	// Split the leading number from the trailing unit.
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	numPart := s[:i]
	unitPart := strings.ToLower(strings.TrimSpace(s[i:]))
	if numPart == "" || numPart == "+" || numPart == "-" {
		return style.Length{Unit: "auto"}
	}
	v, err := strconv.ParseFloat(numPart, 64)
	if err != nil {
		return style.Length{Unit: "auto"}
	}
	if unitPart == "" {
		// Bare numbers (e.g. for line-height) stay unit-less.
		// Exception: "0" without a unit is equivalent to "0px" for lengths
		// like width/height/insets, per CSS spec.
		if v == 0 {
			return style.Length{Value: 0, Unit: "px"}
		}
		return style.Length{Value: v}
	}
	return style.Length{Value: v, Unit: unitPart}
}

// maxContentWidth returns the maximum content width of box by measuring the
// total width of inline content per line. Unlike a rightmost-edge measurement,
// this is not affected by text-align: text-align shifts the inline content
// within the containing block but does not change the intrinsic line width, so
// measuring segment widths (rather than absolute right edges) yields the
// correct max-content / shrink-to-fit width even when text is centered or
// right-aligned.
//
// For each line (identified by its Y coordinate), the widths of text segments
// and inline-level non-text-run child boxes on that line are summed. The
// maximum across all lines is returned. Block-level children's Rect.Width is
// the fill width and is not used; their inline descendants contribute via
// recursion. BoxTextRun children are skipped for the direct width contribution
// because their content width is already captured by their TextSegments
// (adding their Rect.Width would double-count).
//
// Important: anonymous inline wrappers often have their Rect.Width set to the
// containing block width during IFC layout, which can mask the real content
// width. We detect this case: if a child is an inline/anonymous box whose
// TextSegments exist (directly or in descendants), we use only the text
// segment widths and skip the Rect.Width contribution.
func maxContentWidth(box *LayoutBox) float64 {
	if box == nil {
		return 0
	}
	lineWidths := map[float64]float64{}
	var scan func(*LayoutBox)
	var hasTextSegments func(bb *LayoutBox) bool
	hasTextSegments = func(bb *LayoutBox) bool {
		if len(bb.TextSegments) > 0 {
			return true
		}
		for _, c := range bb.Children {
			if hasTextSegments(c) {
				return true
			}
		}
		return false
	}
	scan = func(b *LayoutBox) {
		for _, seg := range b.TextSegments {
			lineWidths[seg.Y] += seg.Width
		}
		for _, c := range b.Children {
			if c.IsBlock() {
				// Block-level child: its Rect.Width is the fill width of its
				// containing block, not its content's intrinsic width. Recurse
				// into descendants to find the real content width.
				scan(c)
				continue
			}
			if c.IsTextRun() {
				// Text run: its content width is captured by its
				// TextSegments; adding Rect.Width would double-count.
				scan(c)
				continue
			}
			// Inline-level replaced / element box: its border-box width
			// contributes to the line it sits on. Recurse to pick up any
			// text segments inside it.
			// BUT: if this inline box is an anonymous wrapper whose width
			// was inherited from the parent container, skip the Rect.Width
			// to avoid masking the real text content width.
			isAnonInline := c.Type == BoxAnonymous || (c.IsInline() && hasTextSegments(c))
			if c.Rect.Width > 0 && !isAnonInline {
				lineWidths[c.Rect.Y] += c.Rect.Width
			}
			scan(c)
		}
	}
	scan(box)
	max := 0.0
	for _, w := range lineWidths {
		if w > max {
			max = w
		}
	}
	return max
}

// maxContentBottom returns the bottommost content edge among box's descendants and
// its text segments, measured along the vertical axis. Used for column-direction
// flex items whose content height must be measured from descendant edges. Unlike
// the horizontal case, block-level children's height reflects their actual content
// (auto height = content height), so their Rect is used directly.
func maxContentBottom(box *LayoutBox) float64 {
	if box == nil {
		return 0
	}
	maxBottom := box.Rect.ContentY()
	var scan func(*LayoutBox)
	scan = func(b *LayoutBox) {
		for _, seg := range b.TextSegments {
			if r := seg.Y + seg.Height; r > maxBottom {
				maxBottom = r
			}
		}
		for _, c := range b.Children {
			if r := c.Rect.Y + c.Rect.Height; r > maxBottom {
				maxBottom = r
			}
			scan(c)
		}
	}
	scan(box)
	return maxBottom
}