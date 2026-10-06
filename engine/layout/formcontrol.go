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

// isFormControlElement 报告元素是否为 UA 表单控件。
//
// ★ 用途：IFC 的行内基线定位有**两条**分支 —— 通用替换元素分支（基线 =
//   margin box 底边，见 inlineformattingcontext.go 的 va/baseline 分支）
//   与表单控件分支（基线 = 内部文本基线 = borderTop+paddingTop+字体 ascent）。
//   input/button/select/output 的基线是**内部文本基线**，若同时被通用分支
//   按「底边」处理，会把 maxBaseline 抬到 border-box 高（实测
//   h2_baseline_formula 的 f 用例：input 显式 vertical-align:baseline 时
//   maxBaseline 被抬到 21 → relTop 6，而 Edge 为 4）。此处集中判定，
//   让表单控件**只**走表单控件分支。
func isFormControlElement(el *dom.Element) bool {
	if el == nil {
		return false
	}
	switch strings.ToLower(el.LocalName()) {
	case "input", "textarea", "button", "select", "output", "progress", "meter":
		return true
	}
	return false
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

// selectContentWidth 返回 <select> 的内在**内容**宽度：最宽 <option> 的文本宽
// （CSS-SIZING-3 §5.1：select 的 max-content 由最宽选项决定；<optgroup> 内的
// 选项同样参与）。
//
// 为什么单独需要：formControlContentSize **有意**不为 select 给出固有宽度
// （见其文档注释 "a <select> is as wide as its widest option"），IFC 侧另有
// 45px 的 UA 兜底，但 flex 上下文没有那把兜底——flex item 的 base size 走
// intrinsicContentWidth，于是 select 的内容宽被算成 0，只剩 padding+border：
// gou-ide 输入卡模型选择器（.ibb-btns > .sp-wrap > select.sp-select）在 wb-ui
// 里宽 38px（浏览器 240px，命中 max-width 上限），option 文本全部截断，并把
// 兄弟项（margin-left:auto 的 "Enter 发送 · Shift+Enter 换行"）带偏 105px。
func selectContentWidth(box *ElementBox) (float64, bool) {
	el := box.Element()
	if el == nil || !strings.EqualFold(el.LocalName(), "select") {
		return 0, false
	}
	best := 0.0
	var scan func(dom.Node)
	scan = func(n dom.Node) {
		for _, child := range n.ChildNodes() {
			ce, ok := child.(*dom.Element)
			if !ok {
				continue
			}
			switch strings.ToLower(ce.LocalName()) {
			case "option":
				if t := strings.TrimSpace(ce.TextContent()); t != "" {
					if w := measureText(box, t); w > best {
						best = w
					}
				}
			case "optgroup":
				scan(ce)
			}
		}
	}
	scan(el)
	if best <= 0 {
		return 0, false
	}
	// ★ max-width 钳制：select 的内容宽同样受自身 max-width 约束。gou-ide 的
	//   .sp-select 声明 max-width:240px，最宽 option 需要 ≈259px 内容宽——浏览器
	//   给 240px 边框盒；不钳制时 intrinsicContentWidth 会把 297px 外盒宽上传给
	//   inline-flex 容器 .sp-wrap 的 max-content（容器自身没有 max-width，钳不住），
	//   于是 .ibb-btns 超出输入卡可用宽度，把三个按钮挤到第二行。
	if cs := box.Style(); cs != nil {
		if maxW, ok := definiteWidth(cs.MaxWidth, 0, fontSizeOf(box)); ok && maxW > 0 {
			if isBorderBox(box) {
				// max-width 是**边框盒**上限（box-sizing:border-box）→ 换算成内容宽上限。
				_, pb, bd := computeBoxModel(box, 0, fontSizeOf(box))
				if lim := maxW - pb.Horizontal() - bd.Horizontal(); lim > 0 && best > lim {
					best = lim
				}
			} else if best > maxW {
				best = maxW
			}
		}
		// ★ 返回**外盒**宽：intrinsicContentWidth 末尾对走到结尾的路径统一加
		//   自身 padding+border，但提前 return 的分支不会被加（既有的
		//   formControlContentSize 路径返回内容宽，由 flex 的 paddingMain
		//   机制承担差值）。本分支的消费方是 flex 的 base size 与 inline-flex
		//   容器的 max-content 递归——两处都按**外盒**语义使用，漏加会让
		//   select 的边框盒比浏览器少 38px（实测 202 vs 240）。
		_, p, b := computeBoxModel(box, 0, fontSizeOf(box))
		best += p.Left + p.Right + b.Left + b.Right
	}
	return best, true
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
