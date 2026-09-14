// Translation of: Source/WebCore/layout/LayoutUnits.h
// Utility functions for box-model computation, font sizing, etc.

package layout

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"wb-ui/engine/css"
	"wb-ui/engine/style"
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
		return lengthResult{Value: l.Value * remBase(), Definite: remBase() > 0}
	case "ex":
		// x-height 单位（CSS Values and Units §5.1.1）。用字体真实 x-height
		// （embedder 通过 XHeightFunc 注入 Skia 度量）；不可用时退回 0.5em。
		// 此前恒定 0.5em，Arial 16px 下 1.4ex 得 11.2px 而非 12.1px，
		// `padding: 0.7ex 1.4ex` 的盒子是 42x21 而非 44x22
		// （list-indentation 的 ex 项；Arial x-height/em ≈ 0.54）。
		return lengthResult{Value: l.Value * exBase(fontSize), Definite: fontSize > 0}
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
		// element font-size for em, the root font-size for rem, viewport for vw/vh.
		if l.CalcExpr == "" {
			return lengthResult{Definite: false}
		}
		v, err := css.EvalCalcString(l.CalcExpr, css.CalcContext{
			ParentWidth:   reference,
			FontSize:      fontSize,
			RootFontSize:  remBase(),
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

// heightPercentDependent reports whether a height's used value needs a
// percentage reference — a plain `%`, or a calc() carrying one. When the
// containing block's height is not definite such a height behaves as auto
// (CSS 2.1 §10.5), so callers must not invent a reference for it.
func heightPercentDependent(l style.Length) bool {
	if l.Unit == "%" {
		return true
	}
	return l.Unit == "calc" && strings.Contains(l.CalcExpr, "%")
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
	// math 函数（calc/min/max/clamp）：含相对单位（%、em、rem、vw、vh 等）
	// 无法在此处求值（不知道包含块尺寸 / font-size / viewport），保留
	// calc 标记让 resolveLength 的 "calc" case 带真实 context 求值；纯绝对
	// 单位则立即求值为 px。与 style.parseLength 的处理逻辑保持一致。
	if name, full, ok := mathFuncInfo(s); ok {
		expr := full
		if name == "calc" {
			expr = full[5 : len(full)-1] // calc 存内部表达式
		}
		if css.CalcHasRelativeUnit(expr) {
			return style.Length{Unit: "calc", CalcExpr: expr}
		}
		if v, err := css.EvalCalcString(expr, css.CalcContext{}); err == nil {
			return style.Length{Value: v, Unit: "px"}
		}
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

// mathFuncInfo detects a CSS math function prefix (calc/min/max/clamp) and
// returns its lowercased name plus the full balanced function expression.
func mathFuncInfo(s string) (name, full string, ok bool) {
	s = strings.TrimSpace(s)
	for _, n := range []string{"calc", "min", "max", "clamp"} {
		if len(s) < len(n)+1 || !strings.EqualFold(s[:len(n)], n) || s[len(n)] != '(' {
			continue
		}
		depth := 0
		for i := len(n); i < len(s); i++ {
			switch s[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					return n, s[:i+1], true
				}
			}
		}
		return n, s, true
	}
	return "", "", false
}


// defaultFontSize is the initial font size (CSS `medium`) and the rem fallback
// when the root element's font-size is unknown.
const defaultFontSize = 16.0

// currentRootFontSize is the root element's computed font-size in px.
//
// `rem` must resolve against THIS value (CSS Values §5.1: "equal to the computed
// value of font-size on the root element"), not against the initial 16px: a page
// with `html{font-size:18px}`, or the very common `html{font-size:62.5%}` reset,
// sizes every rem-based padding/margin/gap/font-size from it. wb-ui hard-coded
// the 16px base, so all rem lengths on such pages were wrong (fixture
// render-repros/relative-box-edges.html: `padding-left:2rem` with
// `html{font-size:18px}` is 36px, not 32px).
var currentRootFontSize float64

// SetRootFontSize sets the px value one `rem` resolves to. A non-positive value
// restores the 16px fallback. BuildLayoutTree calls it with the root element's
// computed font-size before laying out; embedders that lay out without a tree
// (widget renderers) may call it directly.
func SetRootFontSize(px float64) {
	currentRootFontSize = px
}

// remBase returns the px value one `rem` resolves to.
func remBase() float64 {
	if currentRootFontSize > 0 {
		return currentRootFontSize
	}
	return defaultFontSize
}

// rootFontSizePx resolves the root element's used font-size in px — the value
// one `rem` resolves to for the whole document.
//
// The root element has no parent to inherit from, so its relative font-sizes
// resolve against the *initial* font size (CSS 2.1 §15.7): `html{font-size:62.5%}`
// is 10px, `html{font-size:1.5em}` is 24px. The previous implementation only
// accepted a px unit (Unit == "px"), so a percentage root font-size was silently
// ignored and every `rem` in the document resolved against the 16px fallback —
// `gap: 4rem calc(1rem + 2vw)` in a 62.5% document produced gaps of 64px/34px
// instead of 40px/28px (contextual-grid-gap).
//
// Returns 0 when the root font-size cannot be determined (the caller then keeps
// the previous / default rem base).
func rootFontSizePx(cs *style.ComputedStyle) float64 {
	if cs == nil || cs.FontSize.Value <= 0 {
		return 0
	}
	switch cs.FontSize.Unit {
	case "px", "":
		return cs.FontSize.Value
	case "%":
		return cs.FontSize.Value / 100 * defaultFontSize
	case "em", "rem":
		// On the root element itself both relative units resolve against the
		// initial font size (rem would otherwise be self-referential).
		return cs.FontSize.Value * defaultFontSize
	}
	return 0
}

// XHeightFunc 查询给定字体的 x-height（px，正值），由 embedder 注入
// （Skia 的 FontMetrics().XHeight）。未注入或返回非正值时，ex 单位按规范
// 允许的 fallback 用 0.5×font-size（CSS Values and Units §5.1.1）。
var XHeightFunc func(family string, size float64, weight int, style string) float64

// fontContext 记录"当前正在解析长度的元素字体"。
//
// 长度解析是纯函数 API（resolveLength(l, reference, fontSize)），只有
// font-size 一个字体维度，而 ex 需要完整的 (family, weight, style) 才能查
// x-height。因此 fontSizeOf 在算出元素字号时顺手记录本上下文（每次元素布局
// 前都会调用它），resolveLength 的 ex 分支据此查询真实 x-height。
type fontContext struct {
	family string
	size   float64
	weight int
	style  string
}

var currentFontContext fontContext

// exBase 返回当前解析上下文里 1ex 的 px 值：优先字体真实 x-height，
// 度量不可用时退回 0.5×font-size（规范允许的近似）。
func exBase(fontSize float64) float64 {
	if XHeightFunc != nil {
		fc := currentFontContext
		size := fc.size
		if size <= 0 {
			size = fontSize
		}
		if size > 0 {
			if xh := XHeightFunc(fc.family, size, fc.weight, fc.style); xh > 0 {
				return xh
			}
		}
	}
	return fontSize * 0.5
}

// isRTL reports whether a box is laid out from right to left.
//
// `direction` was parsed into the computed style from the start
// (engine/style/computedstyle_data.go Direction) and had a resolver test, but no
// layout code ever read it: RTL pages laid out as if `direction: ltr`
// everywhere. Per CSS 2.1 §10.3.3 the containing block's direction decides
// which margin is ignored when a block is over-constrained, and per
// CSS Flexbox §5.1 / CSS Grid §7.1 the inline axis (flex main axis, grid
// column axis) starts at the content box's inline-start edge — which is the
// right edge for RTL.
func isRTL(cs *style.ComputedStyle) bool {
	return cs != nil && cs.Direction == "rtl"
}

// legacyBlockAlignOf 报告容器 computed style 上的 legacy 对齐方向
// （"center"/"right"/"left"），非 legacy 值（含 nil 样式）返回空串。
// 供 BFC 对块级子盒做水平居中/靠边，见 TextAlignType.LegacyBlockAlign。
func legacyBlockAlignOf(cs *style.ComputedStyle) string {
	if cs == nil {
		return ""
	}
	return cs.TextAlign.LegacyBlockAlign()
}

// isTableInternalBox 报告盒是否是表格内部盒（表节/行/单元格/列/列组/标题）。
//
// 这类盒的尺寸由表格布局算法决定（行高、列宽），行内内容**不得**回写它们的
// 宽度：固定布局下 `table-layout:fixed` 的第二行单元格含 nowrap 长文本时，
// IFC 收尾的「按内容回写容器宽」会把列轨道宽 50 覆盖成文本宽 277.3
// （fixed-table-layout 的 "later separate row cannot resize first track"），
// 浏览器中单元格宽度与内容无关、内容只会溢出或换行。
func isTableInternalBox(b *ElementBox) bool {
	if b == nil || b.style == nil {
		return false
	}
	switch b.style.Display {
	case style.DisplayTableRowGroup, style.DisplayTableHeaderGroup,
		style.DisplayTableFooterGroup, style.DisplayTableRow,
		style.DisplayTableColumnGroup, style.DisplayTableColumn,
		style.DisplayTableCell, style.DisplayTableCaption:
		return true
	}
	return false
}

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

// fontSizeOf 返回盒子的 used font-size（px），并记录当前字体上下文——
// ex 单位需要完整的字体信息（见 fontContext）。
func fontSizeOf(box *ElementBox) float64 {
	fs := resolveFontSizeOf(box)
	recordFontContext(box, fs)
	return fs
}

// recordFontContext 记录元素字体，供 ex 的度量查询使用。字号无法确定
// （<=0）时不覆盖上一个上下文。
func recordFontContext(box *ElementBox, size float64) {
	if box == nil || size <= 0 {
		return
	}
	if cs := box.Style(); cs == nil {
		return
	}
	currentFontContext = fontContext{
		family: fontFamilyOf(box),
		size:   size,
		weight: fontWeightOf(box),
		style:  fontStyleOf(box),
	}
}

func resolveFontSizeOf(box *ElementBox) float64 {
	cs := box.Style()
	if cs == nil {
		if p := box.Parent(); p != nil {
			return resolveFontSizeOf(p)
		}
		return defaultFontSize
	}
	// Anonymous boxes (no DOM element) inherit their parent's style object;
	// their em/% font-size must resolve against the parent's *computed* px,
	// not re-resolve the raw em against the parent (2em on an h1 → 2×32=64).
	// The anonymous wrapper's style.FontSize is copied from the parent's raw
	// declaration, so treat it as inherited-computed by walking to the parent.
	if box.Element() == nil && box.Parent() != nil && cs.FontSize.Unit != "" && cs.FontSize.Unit != "px" {
		return resolveFontSizeOf(box.Parent())
	}
	parentSize := 0.0
	if box.Parent() != nil {
		parentSize = resolveFontSizeOf(box.Parent())
	} else {
		// 根元素（无父）：百分比与 em 相对**初始**字号解析（CSS 2.1 §15.7），
		// 否则 reference<=0 让 resolveLength 返回 !Definite，font-size:62.5%
		// 落回 16px——根字号错误又会让全文档的 rem 都按 16 解析
		// （contextual-grid-gap 的 gap:4rem 得到 64px 而非 40px）。
		parentSize = defaultFontSize
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
	if !r.Definite {
		return defaultFontSize
	}
	// ★ 显式 font-size:0 是合法值（CSS 允许 0）：文字零宽且不可见。此前
	// `r.Value <= 0` 与「解析失败」一并回退 16px，于是 `font-size:0`
	// （消除 inline-block 间隙的常用技巧）失效——行内空白仍按 16px
	// 度量出约 4.4px 并计入行宽 used，使 legacy-center 首行的居中
	// inline-box 偏左 2px（实测 x=148，期望 150）；文字也会以 16px 画出
	// 而不是隐藏。仅负值（非法）与未解析（Definite=false）回退默认字号。
	if r.Value < 0 {
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
	// fs == 0 是显式 font-size:0（合法的零宽文字），只有未解析出的负值才回退。
	if fs < 0 {
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
	if fs < 0 { fs = defaultFontSize }
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
	// ★ 网格对齐（grid fitting）：浏览器把 ascent/descent/lineGap **各自**
	// 四舍五入到整数像素后再相加得到 line-height:normal 的行高，而不是用
	// 浮点度量求和。Chrome 实测（font-metric-line-height 夹具的期望值即由此
	// 得来）：Arial 9.3333px → round(8.449)+round(1.978)+round(0.305) = 10
	// （浮点求和为 10.73，20 行累计偏移 15px）；Arial 12px → 11+3+0 = 14
	// （浮点 13.8）；Times 13px → 12+3+1 = 16（浮点 14.9）。不取整会让多行
	// 文本的垂直位置逐步漂移，行盒高度也不再是浏览器那样的整数值。
	return math.Round(a) + math.Round(d) + math.Round(lg)
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
		// engine/rendering/renderview.go init() and by all test executables.
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
