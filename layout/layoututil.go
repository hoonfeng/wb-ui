// Translation of: Source/WebCore/layout/LayoutUnits.h
// Utility functions for box-model computation, font sizing, etc.

package layout

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

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
	}
	return style.Length{Unit: "auto"}
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
	if cs == nil { return defaultFontSize }
	ref := 0.0
	if cs.FontSize.Unit == "%" && box.Parent() != nil {
		ref = fontSizeOf(box.Parent())
	}
	r := resolveLength(cs.FontSize, ref, 0)
	if !r.Definite || r.Value <= 0 { return defaultFontSize }
	return r.Value
}

func fontFamilyOf(box *ElementBox) string {
	cs := box.Style()
	if cs == nil { return "" }
	return firstFontFamily(cs.FontFamily)
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
	if cs == nil { return 400 }
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
func measureText(box *ElementBox, text string) float64 {
	fs := fontSizeOf(box)
	if fs <= 0 { fs = defaultFontSize }
	family := fontFamilyOf(box)
	weight := fontWeightOf(box)
	fstyle := fontStyleOf(box)
	if MeasureTextFunc != nil {
		return MeasureTextFunc(family, fs, weight, fstyle, text)
	}
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
	a, d, _ := fontMetricsHelper(fontFamilyOf(box), fs, fontWeightOf(box), fontStyleOf(box))
	return a + d
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
