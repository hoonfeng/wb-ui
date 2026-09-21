// Form-control UA geometry (mirrors Chromium/Blink html.css + HTML's
// intrinsic-size rules):
//
//	<input type=text> content width = size × avgCharWidth + maxCharWidth
//	                  (size defaults to 20) and content height = one line box;
//	<textarea>        content width = cols × avgCharWidth (cols defaults to 20),
//	                  content height = rows × line box (rows defaults to 2);
//	checkbox / radio  are 13×13 CSS px; a range control is 129×16.
//
// These are *content-box* sizes. The UA padding and border that give an
// <input> its familiar 21px border box (`padding: 1px 2px; border: 2px`) come
// from the style layer — see style.applyFormControlUserAgentDefaults — and are
// added on top by the box model.
//
// wb-ui used to invent a control size from the *line height* alone, so a
// 17-character input was as wide as its font-metric box and the fixtures'
// native-geometry checks all missed.
package layout

import (
	"strconv"
	"strings"

	"wb-ui/engine/dom"
	"wb-ui/engine/style"
)

const (
	// formControlAvgCharWidth / formControlMaxCharWidth approximate Blink's
	// character-width estimate for the UA control font (Arial at 13.3333px):
	// an <input size="20"> is 169px wide inside its (4px) padding and (4px)
	// border, i.e. 20×8 + 9.
	formControlAvgCharWidth = 8.0
	formControlMaxCharWidth = 9.0
	// HTML defaults: input size=20, textarea cols=20 (rows=2, see textareaRows).
	formControlDefaultSize = 20.0
	formControlDefaultCols = 20.0
	// Chromium paints checkbox/radio as 13×13 boxes and a range control as
	// 129×16 (its UA margins sit in style.applyFormControlUserAgentDefaults).
	formControlCheckboxSize = 13.0
	formControlRangeWidth   = 129.0
	formControlRangeHeight  = 16.0
)

// formControlBaselineFromBorderTop 返回表单控件盒 border-box top 到其基线的
// 距离（CSS 2.1 §10.8 / CSS Inline Layout），供行内基线对齐使用。
//
// 规则（依据 Edge 实测，见 dev/suites/consistency 的 form_controls）：
//   - checkbox / radio / range / progress / meter 与 <textarea>（overflow:auto）：
//     基线 = 盒子的底边框边 → 偏移 = border-box 高度；
//   - 文本类 <input> / <button> / <select>：基线 = 内部文本基线
//     → 偏移 = border-top + padding-top + 字体 ascent。
//
// Edge 实测（第一行共同基线 y=44，括号内为 border-box 高度→偏移）：
// textarea(36→36)、input[text](21→15)、checkbox(13→13)、range(16→16)、
// select(19→14)、button(21→15)——六个控件全部落在同一条基线上，而各控件
// 的 y 差别（28/29/30/31）正是「基线 + 高度」反推出来的。
//
// ok=false 表示不是表单控件，调用方保持原有 vertical-align 逻辑。
func formControlBaselineFromBorderTop(el *dom.Element, borderTop, paddingTop, borderBoxH, ascent float64) (float64, bool) {
	if el == nil {
		return 0, false
	}
	switch strings.ToLower(el.LocalName()) {
	case "input":
		switch strings.ToLower(el.GetAttribute("type")) {
		case "checkbox", "radio", "range":
			return borderBoxH, true
		case "hidden":
			return 0, false // 不生成盒子
		}
		return borderTop + paddingTop + ascent, true
	case "textarea":
		// 内部是滚动容器（UA overflow:auto）→ 基线取盒底边。
		return borderBoxH, true
	case "button", "select", "output":
		return borderTop + paddingTop + ascent, true
	case "progress", "meter":
		return borderBoxH, true
	}
	return 0, false
}

// formControlContentSize returns the UA intrinsic CONTENT-box size of a form
// control box. ok is false for elements whose width is not intrinsic (a
// <select> is as wide as its widest option) and for hidden inputs, so callers
// keep their own fallback.
func formControlContentSize(box *ElementBox) (w, h float64, ok bool) {
	el := box.Element()
	if el == nil {
		return 0, 0, false
	}
	lineH := fontLineGap(box)
	if lineH <= 0 {
		fs := fontSizeOf(box)
		if fs <= 0 {
			fs = 13.3333
		}
		lineH = fs * 1.2
	}
	switch strings.ToLower(el.LocalName()) {
	case "input":
		switch strings.ToLower(el.GetAttribute("type")) {
		case "hidden":
			return 0, 0, false
		case "checkbox", "radio":
			return formControlCheckboxSize, formControlCheckboxSize, true
		case "range":
			return formControlRangeWidth, formControlRangeHeight, true
		}
		size := formControlAttrFloat(el, "size", formControlDefaultSize)
		return size*formControlAvgCharWidth + formControlMaxCharWidth, lineH, true
	case "textarea":
		cols := formControlAttrFloat(el, "cols", formControlDefaultCols)
		return cols * formControlAvgCharWidth, textareaRows(box) * lineH, true
	}
	return 0, 0, false
}

// formControlAttrFloat parses a positive integer attribute (size/cols), falling
// back to def when it is absent, empty or not a positive integer.
func formControlAttrFloat(el *dom.Element, name string, def float64) float64 {
	if v := el.GetAttribute(name); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return float64(n)
		}
	}
	return def
}

// boxMarginHorizontal returns a box's resolved horizontal margins. Auto margins
// (flex free-space absorbers) and percentages without a definite containing
// block contribute nothing.
func boxMarginHorizontal(box *ElementBox) float64 {
	cs := box.Style()
	if cs == nil {
		return 0
	}
	fs := fontSizeOf(box)
	total := 0.0
	for _, m := range [2]style.Length{cs.MarginLeft, cs.MarginRight} {
		if m.IsAuto() {
			continue
		}
		if v, ok := definiteWidth(m, 0, fs); ok && v > 0 {
			total += v
		}
	}
	return total
}
