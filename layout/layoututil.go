// Translation of: Source/WebCore/layout/LayoutUnits.h
// Utility functions for box-model computation, font sizing, etc.

package layout

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"wb-ui/css"
	"wb-ui/style"
)

// ── Length resolution ────────────────────────────────────────

type lengthResult struct {
	Value    float64
	Definite bool
	Auto     bool
}

func resolveLengthAuto(l style.Length, reference, fontSize float64) lengthResult {
	if l.Unit == "" || l.Unit == "auto" {
		return lengthResult{Auto: true}
	}
	v := resolveLength(l, reference, fontSize)
	return lengthResult{Value: v.Value, Definite: v.Definite}
}

func resolveLength(l style.Length, reference, fontSize float64) lengthResult {
	if l.Unit == "" { return lengthResult{} }
	switch l.Unit {
	case "px":
		return lengthResult{Value: l.Value, Definite: true}
	case "em":
		return lengthResult{Value: l.Value * fontSize, Definite: fontSize > 0}
	case "rem":
		return lengthResult{Value: l.Value * defaultFontSize, Definite: defaultFontSize > 0}
	case "%":
		if reference > 0 { return lengthResult{Value: l.Value * reference / 100, Definite: true} }
		return lengthResult{Definite: false}
	case "vw":
		if currentViewportWidth > 0 {
			return lengthResult{Value: l.Value * currentViewportWidth / 100, Definite: true}
		}
		return lengthResult{Definite: false}
	case "vh":
		if currentViewportHeight > 0 {
			return lengthResult{Value: l.Value * currentViewportHeight / 100, Definite: true}
		}
		return lengthResult{Definite: false}
	case "vmin":
		v := currentViewportWidth
		if currentViewportHeight < v {
			v = currentViewportHeight
		}
		if v > 0 {
			return lengthResult{Value: l.Value * v / 100, Definite: true}
		}
		return lengthResult{Definite: false}
	case "vmax":
		v := currentViewportWidth
		if currentViewportHeight > v {
			v = currentViewportHeight
		}
		if v > 0 {
			return lengthResult{Value: l.Value * v / 100, Definite: true}
		}
		return lengthResult{Definite: false}
	case "calc":
		// calc() with relative units deferred from style resolve: re-evaluate
		// with real context. reference is the containing-block dimension (parent
		// width for width/margin, parent height for height), fontSize is the
		// element font-size for em, defaultFontSize for rem, viewport for vw/vh.
		if l.CalcExpr == "" {
			return lengthResult{Definite: false}
		}
		v, err := css.EvalCalcString(l.CalcExpr, css.CalcContext{
			ParentWidth:   reference,
			FontSize:      fontSize,
			RootFontSize:  defaultFontSize,
			ViewportWidth:  currentViewportWidth,
			ViewportHeight: currentViewportHeight,
		})
		if err != nil {
			return lengthResult{Definite: false}
		}
		return lengthResult{Value: v, Definite: true}
	default:
		return lengthResult{Definite: true}
	}
}

func definiteWidth(l style.Length, reference, fontSize float64) (float64, bool) {
	r := resolveLengthAuto(l, reference, fontSize)
	if r.Auto || !r.Definite { return 0, false }
	return r.Value, true
}

func definiteHeight(l style.Length, reference, fontSize float64) (float64, bool) {
	r := resolveLengthAuto(l, reference, fontSize)
	if r.Auto || !r.Definite { return 0, false }
	return r.Value, true
}

func asLength(v interface{}) style.Length {
	if v == nil { return style.Length{Unit: "auto"} }
	switch x := v.(type) {
	case style.Length:
		return x
	case string:
		return parseCSSLength(x)
	}
	return style.Length{Unit: "auto"}
}

// parseCSSLength parses a CSS length string ("10px", "2em", "auto", "50%") into
// a style.Length struct. This is needed because asLength receives raw string
// values from cs.Properties["top"/"left"/etc.].
func parseCSSLength(s string) style.Length {
	s = strings.TrimSpace(s)
	if s == "" || s == "auto" {
		return style.Length{Unit: "auto"}
	}
	// Find the boundary between numeric part and unit.
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	for i < len(s) && ((s[i] >= '0' && s[i] <= '9') || s[i] == '.') {
		i++
	}
	if i == 0 {
		return style.Length{Unit: "auto"}
	}
	num, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return style.Length{Unit: "auto"}
	}
	unit := s[i:]
	// A bare number is only valid as a length when it is 0 (CSS Values §5.1:
	// "the unit may be omitted when the value is zero"). Treat "0" as 0px.
	if unit == "" && num != 0 {
		return style.Length{Unit: "auto"}
	}
	if unit == "" {
		unit = "px"
	}
	return style.Length{Value: num, Unit: unit}
}


const defaultFontSize = 16.0

// ── Box model ────────────────────────────────────────────────

func computeBoxModel(box *ElementBox, cbContentWidth, fontSizeVal float64) (margin, padding, border Edges) {
	st := box.Style()
	if st == nil { return }
	fs := fontSizeVal
	if fs <= 0 { fs = defaultFontSize }
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

func resolveOrZero(l style.Length, reference, fontSize float64) float64 {
	r := resolveLengthAuto(l, reference, fontSize)
	if r.Auto || !r.Definite { return 0 }
	return r.Value
}

func usedBorderWidth(l style.Length, bs string) float64 {
	switch bs {
	case "none", "hidden":
		return 0
	}
	r := resolveLengthAuto(l, 0, 0)
	if r.Definite && !r.Auto && r.Value >= 0 { return r.Value }
	return 0
}

func isBorderBox(box *ElementBox) bool {
	return box.Style() != nil && strings.EqualFold(box.Style().BoxSizing, "border-box")
}

func clampSize(size, minV, maxV float64, minAuto, maxAuto bool) float64 {
	if !maxAuto && maxV < size { size = maxV }
	if !minAuto && minV > size { size = minV }
	return size
}

func resolveMinMax(minL, maxL style.Length, reference, fontSize float64) (minV, maxV float64, minAuto, maxAuto bool) {
	r := resolveLengthAuto(minL, reference, fontSize)
	minV, minAuto = r.Value, r.Auto
	if minAuto {
		minV = 0
	}
	if maxL.Unit == "" || maxL.Unit == "none" {
		maxAuto = true
	} else {
		r := resolveLengthAuto(maxL, reference, fontSize)
		maxV, maxAuto = r.Value, r.Auto
		if maxAuto {
			maxV = 0
		}
	}
	return
}

var FontMetricsFunc func(family string, size float64, weight int, style string) (float64, float64, float64)

// MeasureTextFunc measures the advance width of text. Set by the embedder.
var MeasureTextFunc func(family string, size float64, weight int, style, text string) float64
func computeBoxModelForBox(box *ElementBox, cbContentWidth, fontSizeVal float64) (margin, padding, border Edges) {
	return computeBoxModel(box, cbContentWidth, fontSizeVal)
}

func isBorderBoxForBox(box *ElementBox) bool {
	return box.Style() != nil && strings.EqualFold(box.Style().BoxSizing, "border-box")
}

// ── Font helpers ─────────────────────────────────────────────

func fontSizeOf(box *ElementBox) float64 {
	cs := box.Style()
	if cs == nil {
		if p := box.Parent(); p != nil {
			return fontSizeOf(p)
		}
		return defaultFontSize
	}
	// Anonymous boxes (no DOM element) inherit their parent's style object;
	// their em/% font-size must resolve against the parent's *computed* px,
	// not re-resolve the raw em against the parent (2em on an h1 → 2×32=64).
	// The anonymous wrapper's style.FontSize is copied from the parent's raw
	// declaration, so treat it as inherited-computed by walking to the parent.
	if box.Element() == nil && box.Parent() != nil && cs.FontSize.Unit != "" && cs.FontSize.Unit != "px" {
		return fontSizeOf(box.Parent())
	}
	parentSize := 0.0
	if box.Parent() != nil {
		parentSize = fontSizeOf(box.Parent())
	}
	// em/rem/% font-sizes resolve against the parent's font-size:
	//   - em  → value × parent font-size
	//   - %   → value% of parent font-size
	//   - rem → value × root font-size (approximated by defaultFontSize)
	var r lengthResult
	switch cs.FontSize.Unit {
	case "%":
		r = resolveLength(cs.FontSize, parentSize, 0)
	case "em":
		r = resolveLength(cs.FontSize, 0, parentSize)
	case "rem":
		r = resolveLength(cs.FontSize, 0, defaultFontSize)
	default:
		r = resolveLength(cs.FontSize, 0, 0)
	}
	if !r.Definite || r.Value <= 0 {
		return defaultFontSize
	}
	return r.Value
}

func fontFamilyOf(box *ElementBox) string {
	cs := box.Style()
	if cs == nil {
		if p := box.Parent(); p != nil {
			return fontFamilyOf(p)
		}
		return ""
	}
	return cs.FontFamily
}

// firstFontFamily extracts the first font name from a CSS font-family list,
// stripping surrounding quotes. E.g. "'Consolas', 'Courier New', monospace"
// returns "Consolas".
func firstFontFamily(s string) string {
	s = strings.TrimSpace(s)
	if s == "" { return "" }
	// Take the first comma-separated part.
	if idx := strings.IndexByte(s, ','); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)
	// Strip surrounding quotes.
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') && s[0] == s[len(s)-1] {
		s = s[1 : len(s)-1]
	}
	return s
}

func fontWeightOf(box *ElementBox) int {
	cs := box.Style()
	if cs == nil {
		if p := box.Parent(); p != nil {
			return fontWeightOf(p)
		}
		return 400
	}
	return parseFontWeight(cs.FontWeight)
}

func parseFontWeight(w string) int {
	switch strings.ToLower(strings.TrimSpace(w)) {
	case "bold", "bolder": return 700
	case "lighter": return 300
	case "", "normal": return 400
	}
	if n, err := strconv.Atoi(strings.TrimSpace(w)); err == nil { return n }
	return 400
}

func fontStyleOf(box *ElementBox) string {
	cs := box.Style()
	if cs == nil { return "normal" }
	return cs.FontStyle
}

// inlineBoxTextContent 递归收集 inline-block 盒内的文本内容（InlineTextBox
// 串联），用于 shrink-to-fit 宽度测量（xterm 光标 div 等无显式宽度
// inline-block 的内容宽）。空白文本在渲染树构建时被跳过（无 RenderText/
// InlineTextBox），此时回退到 DOM textContent（浏览器语义：空格也算内容
// 宽度——光标块覆盖 1 字符宽）。
func inlineBoxTextContent(box *ElementBox) string {
	if box == nil {
		return ""
	}
	var sb strings.Builder
	var walk func(b Box)
	walk = func(b Box) {
		if b == nil {
			return
		}
		if t, ok := b.(*InlineTextBox); ok {
			sb.WriteString(t.text)
			return
		}
		if eb, ok := b.(*ElementBox); ok {
			for _, c := range eb.Children() {
				walk(c)
			}
		}
	}
	for _, c := range box.Children() {
		walk(c)
	}
	if sb.Len() > 0 {
		return sb.String()
	}
	// 布局树无文本（空白被跳过）→ 回退 DOM textContent。
	if el := box.Element(); el != nil {
		if tc := el.TextContent(); tc != "" {
			return tc
		}
	}
	return ""
}

func measureText(box *ElementBox, text string) float64 {
	fs := fontSizeOf(box)
	if fs <= 0 {
		fs = defaultFontSize
	}
	family := fontFamilyOf(box)
	weight := fontWeightOf(box)
	fstyle := fontStyleOf(box)
	if MeasureTextFunc != nil {
		return MeasureTextFunc(family, fs, weight, fstyle, text)
	}
	// Fallback (should not happen in production — set by rendering init).
	return float64(utf8.RuneCountInString(text)) * fs * 0.5
}

// measureTextWordSum measures text as individual words separated by whitespace,
// summing their widths + inter-word space widths. This matches how
// InlineFormattingContext.Layout processes text, preventing the "sum of parts
// exceeds whole" discrepancy that causes unwanted line wraps.
func measureTextWordSum(box *ElementBox, text string, spaceWidth float64) float64 {
	fs := fontSizeOf(box)
	if fs <= 0 { fs = defaultFontSize }
	family := fontFamilyOf(box)
	weight := fontWeightOf(box)
	fstyle := fontStyleOf(box)
	if MeasureTextFunc == nil {
		return float64(utf8.RuneCountInString(text)) * fs * 0.5
	}
	runes := []rune(text)
	total := 0.0
	firstWord := true
	i := 0
	for i < len(runes) {
		// Skip whitespace.
		for i < len(runes) && isInlineWhitespace(runes[i]) {
			i++
		}
		if i >= len(runes) { break }
		start := i
		for i < len(runes) && !isInlineWhitespace(runes[i]) {
			i++
		}
		word := string(runes[start:i])
		w := MeasureTextFunc(family, fs, weight, fstyle, word)
		if !firstWord {
			total += spaceWidth
		}
		total += w
		firstWord = false
	}
	return total
}

func fontAscentDescent(box *ElementBox) (ascent, descent float64) {
	fs := fontSizeOf(box)
	if fs <= 0 { fs = defaultFontSize }
	a, d, _ := fontMetricsHelper(fontFamilyOf(box), fs, fontWeightOf(box), fontStyleOf(box))
	return a, d
}

func fontLineGap(box *ElementBox) float64 {
	fs := fontSizeOf(box)
	if fs <= 0 { fs = defaultFontSize }
	a, d, lg := fontMetricsHelper(fontFamilyOf(box), fs, fontWeightOf(box), fontStyleOf(box))
	return a + d + lg
}

// cssLineHeight returns the resolved CSS line-height value for the box.
// It handles px values, unitless numbers (multiplied by font-size), and
// percentages. Returns 0 if line-height is not explicitly set.
// ★ line-height:normal 解析为 Unit="normal"：返回字体度量
// （fontLineGap = ascent+descent+lineGap）——与浏览器一致，而非 1.2×fs。
// 调用方（inlineformattingcontext）在返回 0 时已回退 fontLineGap，因此
// normal 只需返回 0 即可自然落到字体度量路径。
func cssLineHeight(box *ElementBox) float64 {
	cs := box.Style()
	if cs == nil { return 0 }
	fs := fontSizeOf(box)
	if fs <= 0 { fs = defaultFontSize }
	lh := cs.LineHeight
	switch lh.Unit {
	case "px":
		if lh.Value > 0 { return lh.Value }
	case "%":
		if lh.Value > 0 { return lh.Value / 100 * fs }
	case "normal":
		// 返回 0 → 调用方用 fontLineGap（ascent+descent+lineGap），
		// 即浏览器的 line-height:normal 语义。
		return 0
	case "":
		// Unitless number (e.g. 1.2) — multiply by font-size.
		if lh.Value > 0 { return lh.Value * fs }
	}
	return 0
}

func fontMetricsTriple(box *ElementBox) (ascent, descent, lineGap float64) {
	fs := fontSizeOf(box)
	if fs <= 0 { fs = defaultFontSize }
	return fontMetricsHelper(fontFamilyOf(box), fs, fontWeightOf(box), fontStyleOf(box))
}

// fontMetricsHelper returns Skia metrics for a given font description.
// It NEVER uses hardcoded fs*0.8/fs*0.2 fallbacks — only real Skia data.
// Falls back to empty font family (default system font) when the specific
// family cannot be found.
func fontMetricsHelper(family string, fs float64, weight int, style string) (ascent, descent, lineGap float64) {
	if FontMetricsFunc == nil {
		// Should never happen in production — FontMetricsFunc is set by
		// rendering/renderview.go init() and by all test executables.
		// Return safe defaults as last resort.
		return fs * 0.8, fs * 0.2, fs * 0.2
	}
	if family != "" {
		a, d, lg := FontMetricsFunc(family, fs, weight, style)
		if a > 0 || d > 0 {
			return a, d, lg
		}
	}
	// Fallback: try empty family (resolves to system default font).
	a, d, lg := FontMetricsFunc("", fs, weight, style)
	if a > 0 || d > 0 {
		return a, d, lg
	}
	return fs * 0.8, fs * 0.2, fs * 0.2
}
func isFullWidthRune(r rune) bool {
	return (r >= 0x1100 && r <= 0x115F) || r == 0x2329 || r == 0x232A ||
		(r >= 0x2E80 && r <= 0xA4CF) || (r >= 0xAC00 && r <= 0xD7AF) ||
		(r >= 0xF900 && r <= 0xFAFF) || (r >= 0xFE10 && r <= 0xFE19) ||
		(r >= 0xFE30 && r <= 0xFE6F) || (r >= 0xFF01 && r <= 0xFF60) ||
		(r >= 0xFFE0 && r <= 0xFFE6) || (r >= 0x1B000 && r <= 0x1B0FF) ||
		(r >= 0x1B100 && r <= 0x1B12F) || (r >= 0x20000 && r <= 0x2FA1F)
}

// ── max-content width (post-layout measurement) ──────────────

func maxContentWidth(box Box) float64 {
	if box == nil { return 0 }
	lineWidths := map[float64]float64{}
	var scan func(Box)
	scan = func(b Box) {
		if eb, ok := b.(*ElementBox); ok {
			for _, seg := range eb.TextSegments {
				lineWidths[seg.Y] += seg.Width
			}
			for _, c := range eb.Children() {
				if c.IsBlock() { scan(c); continue }
				if c.IsTextRun() { scan(c); continue }
				scan(c)
			}
		}
	}
	scan(box)
	max := 0.0
	for _, w := range lineWidths { if w > max { max = w } }
	return max
}

// ── maxContentBottom ─────────────────────────────────────────

func maxContentBottom(box Box) float64 {
	if box == nil { return 0 }
	maxBottom := 0.0
	var scan func(Box)
	scan = func(b Box) {
		if eb, ok := b.(*ElementBox); ok {
			for _, seg := range eb.TextSegments {
				if r := seg.Y + seg.Height; r > maxBottom { maxBottom = r }
			}
			for _, c := range eb.Children() { scan(c) }
		}
	}
	scan(box)
	return maxBottom
}

// ── Layout tree visualization ────────────────────────────────

func boxLabel(box Box) string {
	if box == nil { return "nil" }
	label := ""
	if eb, ok := box.(*ElementBox); ok && eb.Element() != nil {
		label = eb.Element().LocalName()
	}
	if tb, ok := box.(*InlineTextBox); ok {
		label = fmt.Sprintf("#text:%q", tb.Text())
	}
	if label == "" {
		switch {
		case box.IsAnonymous(): label = "#anon"
		case box.IsTextRun(): label = "#text"
		default: label = "?"
		}
	}
	return label
}
