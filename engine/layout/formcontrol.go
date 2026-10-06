// Form-control UA geometry (mirrors Chromium/Blink html.css + HTML's
// intrinsic-size rules):
//
//	<input type=text> content width = ceil(avgCharWidth × size)
//	                                + (maxCharWidth − avgCharWidth)
//	                  (size defaults to 20) and content height = one line box;
//	<textarea>        content width = cols × round(0.5 × fontSize) + 15
//	                  (cols defaults to 20; see textareaColumnWidth),
//	                  content height = rows × line box (rows defaults to 2);
//	<select>          content width = round(widest option text) + 20 (arrow area),
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
	"math"
	"strconv"
	"strings"

	"wb-ui/engine/dom"
	"wb-ui/engine/style"
)

const (
	// formControlAvgCharWidth / formControlMaxCharWidth 是 <input> 固有内容宽的
	// **回退常数**：只有字体度量取不到时（无嵌入方、字体解析失败）才用；取自
	// Arial 13.3333px 这一组实测值（20×8 + 9 = 169，恰为 a 行 / Edge 177−8）。
	// 正常路径见 inputIntrinsicContentWidth（随 font-size 与字体度量缩放）。
	formControlAvgCharWidth = 8.0
	formControlMaxCharWidth = 9.0
	// HTML defaults: input size=20, textarea cols=20 (rows=2, see textareaRows).
	formControlDefaultSize = 20.0
	formControlDefaultCols = 20.0
	// ★ Edge 实测（11 组 font-size 扫描 + 4 组 cols 扫描）：<textarea> 的内容宽
	//   = cols × 列宽 + **15**，该 15 与字号无关（13.3333px 与 20px 两组均为 15）。
	//   叠加 UA 的 padding:2px + border:1px（共 6px）后，cols=20 的默认 textarea
	//   边框盒 = 20×7 + 21 = **161**（Edge 161 / 修复前 wbui 166）。
	formTextareaColExtra = 15.0
	// ★ Edge 实测（7 组 option 长度 × 2 种字体）：<select> 在「最宽 option 文本宽」
	//   之外还有 **20px** 的下拉箭头/指示区（加 1px border×2 共 +22）。
	formSelectArrowWidth = 20.0
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
		return inputIntrinsicContentWidth(box, size), lineH, true
	case "textarea":
		cols := formControlAttrFloat(el, "cols", formControlDefaultCols)
		return cols*textareaColumnWidth(box) + formTextareaColExtra, textareaRows(box) * lineH, true
	}
	return 0, 0, false
}

// inputIntrinsicContentWidth 返回 <input type=text> 的 UA 固有**内容**宽度。
//
// 公式取自 WebKit/Blink（WebCore/rendering/RenderTextControlSingleLine.cpp 的
// preferredContentLogicalWidth()）：
//
//	内容宽 = ceil(avgCharWidth × size) + (maxCharWidth − avgCharWidth)
//
// 其中 avgCharWidth 是字体的平均字符宽（Windows/GDI 下等价于 'x' 的 advance，
// WebKit 的 fastAverageCharWidthIfAvailable 对它取 roundf），maxCharWidth 在
// Skia 平台是 round(fXMax − fXMin)（platform/graphics/skia/FontSkia.cpp 的
// initCharWidths()）。两者都**随 font-size 和字体度量缩放**。
//
// ★ 为什么不能用固定常数：旧实现是 `size×8 + 9`（按 Arial 13.3333px 一组值
// 硬编码），与 font-size / font-family 完全无关。于是 a 行（继承 UA 默认
// 13.3333px Arial）恰好落在 169 看着「对」，而 j 行
// `<input style="font:16px sans-serif">` Edge 内容宽 215（边框盒 223 = 215+8）
// 而 wbui 仍给 169（边框盒 177）—— 差 46px。
//
// Edge 实测反推（font-size 扫描 10 组 × 2 个 size + 字体扫描 9 组，见
// dev/fixtures/webshot/h2_input_width_scan.html；度量见 dev/tools/fontmetric）：
//
//	字体 @16px        avgCharWidth  maxCharWidth   size=20 内容宽
//	Arial                8.0000       42.6328          195
//	Noto Sans SC         7.9680       62.7360          215
//	Courier New          9.6016       11.9062          202
//	Times New Roman      8.0000       41.8281          194
//	Arial @13.3333px     6.6666       35.5273          169
//
// 逐项与 Edge 的 <input size=N> 实测宽一致（195/215/202/194/169）。
func inputIntrinsicContentWidth(box *ElementBox, size float64) float64 {
	fs := fontSizeOf(box)
	if fs <= 0 {
		fs = 13.3333
	}
	fam := fontFamilyOf(box)
	wt := fontWeightOf(box)
	st := fontStyleOf(box)
	var avg, maxC float64
	if MeasureTextFunc != nil {
		// 'x' 的 advance 即 GDI tmAveCharWidth 的等价量（本机 SkiaSharp 后端的
		// fAvgCharWidth 恒为 0，见 dev/tools/fontmetric），WebKit 对平均字符宽取 roundf。
		avg = math.Round(MeasureTextFunc(fam, fs, wt, st, "x"))
	}
	if FontMaxCharWidthFunc != nil {
		maxC = math.Round(FontMaxCharWidthFunc(fam, fs, wt, st))
	}
	if avg > 0 && maxC > 0 {
		return math.Ceil(avg*size) + (maxC - avg)
	}
	// 字体度量不可用时的回退：保留旧的实测常数（Arial 13.3333px）。
	return size*formControlAvgCharWidth + formControlMaxCharWidth
}

// textareaColumnWidth 返回 <textarea> 每个 cols 列的**内容**宽度。
//
// Edge 实测（11 组 font-size 扫描：8/10/12/13.3333/14/16/18/20/24/26.6667/32px，
// 各 cols=10，反推 列宽=(边框盒宽−21)/10）：
//
//	8→4, 10→5, 12→6, 13.3333→7, 14→7, 16→8, 18→9, 20→10, 24→12, 32→16
//
// 即 **列宽 = round(0.5 × font-size)**，且与作者 font-family **无关** ——
// 13.3333px 下 Arial 与 monospace 得到同宽（cols=10 → 均为 91，cols=20 → 均为 161）。
// 这说明 Chromium 用 UA 控制字体的度量而非作者声明的 font-family，故此处只按
// 字号推导，不查字体度量。**必须取整**：不取整时 13.3333px 得 6.667/列，
// cols=20 会算成 154.3（Edge 161）。
//
// 已知局限：font-size:26.6667px（比例非整数）实测每列 13.4，本公式给 13
// （差 0.4/列）。该字号在真实页面中极罕见，且不影响 UA 默认的 13.3333px
// （验收点 g1_formctl 的 t7）。
func textareaColumnWidth(box *ElementBox) float64 {
	fs := fontSizeOf(box)
	if fs <= 0 {
		fs = 13.3333
	}
	return math.Round(fs * 0.5)
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
	best, ok := selectMaxOptionTextWidth(box)
	if !ok {
		return 0, false
	}
	best = clampSelectMaxWidth(box, best)
	if box.Style() != nil {
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

// selectMaxOptionTextWidth 扫描 <select> 的所有 <option>（含 <optgroup> 内），
// 返回「最宽 option 文本宽」经 **向上取整 + 20px 下拉箭头区** 后的值。
// ok=false 表示不是 select 或没有任何非空 option。
//
// ★ 取整方式与箭头区由 Edge 实测确定（dev/fixtures/webshot/select_width_scan.html：
// 13 组单字符/多字符 option，同页对照，文本宽取 canvas measureText）：
//
//	option   Edge 文本宽   ceil   内容宽   Edge 边框盒
//	"G"        10.3685      11      31        33
//	"A"         8.8910       9      29        31
//	"i"         2.9615       3      23        25
//	"W"        12.5815      13      33        35
//	"M"        11.1040      12      32        34
//	"0"         7.4135       8      28        30
//	5×"A"      44.4550      45      65        67
//	10×"A"     88.9099      89     109       111
//	20×"A"    177.8198     178     198       200
//
// ★ 是 **ceil 而非 round**：G / M / 0 / m / g / 5×"A" 六例的文本宽小数部分 < 0.5，
// round 会各少 1px（g 行 `<select><option>G</option></select>` 实测 Edge 33 /
// 修复前 wbui 32），ceil 才逐项吻合（13/13）。叠加 UA 的 1px border×2 后即得
// 边框盒宽（上表末列 = 内容宽 + 2）。
// 取整是必需的：不取整时 "A" 只得 30.891（Edge 31）。修复前 wbui 完全没有箭头区
// （g1_formctl 的 t6 = 10.893，恰为 option 文本宽 8.893 + border 2）。
func selectMaxOptionTextWidth(box *ElementBox) (float64, bool) {
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
	// 箭头区在 max-width 钳制**之前**加入 —— 浏览器先算 max-content
	// （文本 + 箭头 + padding + border），再用 max-width 钳制。
	return math.Ceil(best) + formSelectArrowWidth, true
}

// selectIntrinsicContentWidth 返回 <select> 的固有**内容**宽度（已含箭头区，
// 并受自身 max-width 钳制），供 IFC 推导 inline-block 尺寸使用。
//
// 为什么 IFC 必须走这里：IFC 对 inline-block 子盒会用其**文本内容**撑开
// （见 inlineformattingcontext.go 的 inlineBoxTextContent 路径），而
// formControlContentSize 有意不为 select 返回尺寸（它服务 flex 的
// intrinsicContentWidth 那条路径），于是 select 被裸 option 文本撑开 ——
// g1_formctl 的 t6 实测 10.893（Edge 31）。
func selectIntrinsicContentWidth(box *ElementBox) (float64, bool) {
	best, ok := selectMaxOptionTextWidth(box)
	if !ok {
		return 0, false
	}
	return clampSelectMaxWidth(box, best), true
}

// clampSelectMaxWidth 对 select 的**内容**宽施加自身 max-width 约束。
// ★ max-width:240px 的 .sp-select（gou-ide）：最宽 option 需要 ≈259px 内容宽 ——
// 浏览器给 240px 边框盒；不钳制时 intrinsicContentWidth 会把 297px 外盒宽上传给
// inline-flex 容器 .sp-wrap 的 max-content（容器自身没有 max-width，钳不住），
// 于是 .ibb-btns 超出输入卡可用宽度，把三个按钮挤到第二行。
func clampSelectMaxWidth(box *ElementBox, best float64) float64 {
	cs := box.Style()
	if cs == nil {
		return best
	}
	if maxW, ok := definiteWidth(cs.MaxWidth, 0, fontSizeOf(box)); ok && maxW > 0 {
		if isBorderBox(box) {
			// max-width 是**边框盒**上限（box-sizing:border-box）→ 换算成内容宽上限。
			_, pb, bd := computeBoxModel(box, 0, fontSizeOf(box))
			if lim := maxW - pb.Horizontal() - bd.Horizontal(); lim > 0 && best > lim {
				return lim
			}
		} else if best > maxW {
			return maxW
		}
	}
	return best
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
