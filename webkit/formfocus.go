package webkit

// FormFocus 为不使用 app.Host 的 WebView 提供标准表单交互：焦点管理、
// 点击定位光标、文本编辑（光标处插入/选择区间替换）、按键编辑、光标
// 闪烁驱动。
//
// 背景：app.Host 内置完整表单交互（FocusElementByKeyboard/handleCharInput/
// deleteFocusedChar/moveFocusedCaret + 500ms 闪烁循环），但其宿主语义绑定
// Host 整体（窗口事件/IME），配置窗口等「裸 WebView + 自定义窗口」场景
// 无法复用，此前由应用层各自复制（configwin 的 focusEl/insertRune/
// syncCaretSel/闪烁块）——功能残缺：无选择区间替换、无 readonly/maxlength
// 语义、无 input/change 事件派发、点击不定位光标。
//
// 本类型是这些能力的引擎层形态：与窗口解耦（鼠标/键盘事件仍由宿主分发
// 到 FormFocus 方法），复用 Host 相同的渲染/事件语义。Host 自身的实现
// 不受影响（两条路径并存，后续可逐步收敛到本服务）。
//
// 已知边界（首版）：contenteditable 编辑（CM6 等）不在本服务范围——走
// Host 的 IME/composition 链路；IME 组合输入亦由宿主接入（本服务只处理
// 成品字符/按键）。

import (
	"strconv"
	"strings"
	"time"

	"wb-ui/dom"
	"wb-ui/html5"
	"wb-ui/platform/graphics"
	"wb-ui/rendering"
	"wb-ui/style"
)

// FormFocus 服务状态。
type FormFocus struct {
	wv        *WebView
	el        *dom.Element // 当前聚焦的表单控件（nil=无焦点）
	value     string       // 聚焦时的值快照（Changed 比较基准）
	blinkTime time.Time    // 上次光标可见性翻转时刻
	blinkOn   bool         // 当前闪烁相位（跟随 rendering.CaretVisibleControl）
	dirty     bool         // 未消费的变更标记（宿主每帧 PollDirty 取走）
}

// NewFormFocus 创建绑定 wv 的表单交互服务。
func NewFormFocus(wv *WebView) *FormFocus {
	return &FormFocus{wv: wv, blinkTime: time.Now(), blinkOn: true}
}

// WebView 返回服务绑定的 WebView。
func (f *FormFocus) WebView() *WebView { return f.wv }

// Focused 返回当前聚焦的表单控件（无则 nil）。
func (f *FormFocus) Focused() *dom.Element { return f.el }

// Value 返回聚焦控件的当前文本。
func (f *FormFocus) Value() string {
	if f.el == nil {
		return ""
	}
	return formControlValue(f.el)
}

// Changed 报告聚焦控件值是否相对聚焦时快照发生变化（宿主用于
// commit/onchange 决策）。
func (f *FormFocus) Changed() bool {
	return f.el != nil && formControlValue(f.el) != f.value
}

// PollDirty 取走未消费的变更标记（值/焦点/光标变化，宿主应重绘）。
func (f *FormFocus) PollDirty() bool {
	d := f.dirty
	f.dirty = false
	return d
}

// ── 聚焦 ──────────────────────────────────────────────────────────────

// Focus 聚焦 el（光标定位到文本末尾；el 为 nil 等价 Clear）。
// 旧焦点自动失焦（blur 事件 + 清光标登记）；聚焦派发 focus 事件
// （浏览器语义：不冒泡）。
func (f *FormFocus) Focus(el *dom.Element) {
	if f.el != nil && f.el != el {
		f.clear()
	}
	if f.el == el {
		// 已聚焦：刷新快照并重定位光标到末尾。
		if el == nil {
			return
		}
		f.value = formControlValue(el)
		f.setCaret(len([]rune(f.value)))
		f.markDirty()
		return
	}
	if el == nil || !isFormTextControl(el) {
		return
	}
	f.el = el
	f.value = formControlValue(el)
	el.SetFocused(true)
	el.SetFocusByKeyboard(false)
	el.DispatchEvent(dom.NewEvent("focus", false, false, false))
	// ★ 登记到渲染层光标绘制子系统（paintFormControlCaret 仅当
	// el == rendering.FocusedFormControl 时绘制光标）——与 app.Host 的
	// FocusElementByKeyboard 同语义。
	rendering.FocusedFormControl = el
	f.syncBlinkPhase()
	f.setCaret(len([]rune(f.value)))
	f.markDirty()
}

// Clear 失焦：blur 事件 + 清理光标登记（渲染层停止绘制光标）。
func (f *FormFocus) Clear() {
	if f.el == nil {
		return
	}
	f.clear()
	f.markDirty()
}

func (f *FormFocus) clear() {
	el := f.el
	f.el = nil
	f.value = ""
	el.SetFocused(false)
	el.DispatchEvent(dom.NewEvent("blur", false, false, false))
	rendering.FocusedFormControl = nil
	rendering.FocusedFormControlSel = nil
	rendering.CaretVisible = false
	rendering.CaretVisibleControl = false
	f.blinkOn = false
}

// syncBlinkPhase 把服务内闪烁相位与渲染层全局光标可见性对齐
// （焦点切换/重建后二者可能脱节——clear() 会把 CaretVisibleControl
// 复位，若相位不同步，首次翻转写入与当前值相同的可见性，视觉上
// 表现为「闪烁不翻转」）。
func (f *FormFocus) syncBlinkPhase() {
	f.blinkOn = rendering.CaretVisibleControl
	f.blinkTime = time.Now()
}

// FocusFromHit 按命中更新焦点（浏览器语义：点在可编辑表单控件上→
// 聚焦并把光标定位到点击字符处；点其他区域→失焦）。命中测试前同步
// 渲染树（交互前同步，见 EnsureHitTestReady）。
//
// 返回 (prev, hit)：prev 为焦点迁移前的元素（nil=原无焦点），hit 为新
// 焦点元素（nil=已失焦/非控件）。宿主若需「失焦提交」（onchange 等）
// 在焦点迁移后对 prev 执行（提交语义在引擎之外，由宿主决定）。
func (f *FormFocus) FocusFromHit(cssX, cssY float64) (prev, hit *dom.Element) {
	f.wv.EnsureHitTestReady()
	prev = f.el
	rv := f.wv.RenderView()
	if rv == nil {
		return prev, nil
	}
	h := rendering.HitTest(rv, cssX, cssY, "")
	if !isFormTextControl(h) {
		f.Clear()
		return prev, nil
	}
	// 点击定位：输入控件按字符宽度/行高定位到点击处（浏览器语义）。
	pos := f.calcClickCaret(h, cssX, cssY)
	if f.el == h {
		f.value = formControlValue(h)
		f.setCaret(pos) // 已聚焦：仅重新定位光标（点击文本不同处光标跟随）
	} else {
		f.Focus(h)
		f.setCaret(pos)
	}
	return prev, h
}

// calcClickCaret 计算点击位置在控件文本中的 rune 索引（Host
// calcTextControlOffset 的引擎层移植）。定位失败返回文本末尾。
func (f *FormFocus) calcClickCaret(el *dom.Element, cssX, cssY float64) int {
	text := formControlValue(el)
	if text == "" {
		return 0
	}
	_, bx, by, bw, _, sy, st := f.findFormControlBox(el)
	if bx == 0 && bw == 0 {
		return len([]rune(text))
	}
	fontSize := 14.0
	family := "Consolas"
	weight := 400
	if st != nil {
		if st.FontSize.Value > 0 && !st.FontSize.IsAuto() {
			fontSize = st.FontSize.Value
		}
		if st.FontFamily != "" {
			family = st.FontFamily
		}
		if w, err := strconv.Atoi(st.FontWeight); err == nil && w >= 600 {
			weight = 700
		} else if strings.EqualFold(st.FontWeight, "bold") {
			weight = 700
		}
	}
	font := graphics.Font{Family: family, Size: fontSize, Weight: weight}
	ascent := graphics.GlobalFontAscent(font)
	if ascent <= 0 {
		ascent = fontSize * 0.8
	}
	descent := graphics.GlobalFontDescent(font)
	if descent < 0 {
		descent = 0
	}
	lineH := 0.0
	if st != nil {
		switch st.LineHeight.Unit {
		case "px":
			if st.LineHeight.Value > 0 {
				lineH = st.LineHeight.Value
			}
		case "%":
			if st.LineHeight.Value > 0 {
				lineH = st.LineHeight.Value / 100 * fontSize
			}
		case "":
			if st.LineHeight.Value > 0 {
				lineH = st.LineHeight.Value * fontSize
			}
		}
	}
	if lineH <= 0 {
		lineH = ascent + descent
	}
	if lineH <= 0 {
		lineH = fontSize * 1.2
	}
	padX := 4.0
	padY := 4.0
	if st != nil {
		if v := st.PaddingLeft.Value; v > 0 && !st.PaddingLeft.IsAuto() {
			padX = v
		}
		if v := st.PaddingTop.Value; v > 0 && !st.PaddingTop.IsAuto() {
			padY = v
		}
	}
	wrapMode := 0
	if st != nil && el.LocalName() == "textarea" {
		wrapMode = rendering.TextareaWrapMode(st, el)
	}
	pos := rendering.CalcFormControlCaretOffset(text, el.LocalName() == "textarea",
		cssX, cssY, bx, by, bw, font, padX, padY, lineH, wrapMode, sy)
	if pos < 0 {
		return len([]rune(text))
	}
	return pos
}

// findFormControlBox 在渲染树中定位表单控件的绝对边框盒位置/尺寸与
// 计算样式（Host findFormControlBox 的引擎层移植）。
func (f *FormFocus) findFormControlBox(el *dom.Element) (box *rendering.RenderBox, bx, by, bw, bh, sy float64, st *style.ComputedStyle) {
	if el == nil || f.wv == nil {
		return nil, 0, 0, 0, 0, 0, nil
	}
	rv := f.wv.RenderView()
	if rv == nil {
		return nil, 0, 0, 0, 0, 0, nil
	}
	box = rv.FindRenderBoxForNode(el)
	if box == nil {
		return nil, 0, 0, 0, 0, 0, nil
	}
	bx = box.AbsoluteX()
	by = box.AbsoluteY()
	bw = box.Width()
	bh = box.Height()
	st = box.Style()
	_, sy = rv.BoxScrollOffset(box)
	return box, bx, by, bw, bh, sy, st
}

// ── 编辑 ──────────────────────────────────────────────────────────────

// CharInput 输入一个字符（浏览器语义：光标处插入、替换选中区间、受
// readonly/disabled/maxlength 限制；派发 input(insertText) + change 事件）。
// '\r'：textarea 插入换行；input 返回 false（表单提交由宿主决定）。
//
// 返回 false = 未消费（宿主应自行处理，如 input 上的 Enter 提交）。
func (f *FormFocus) CharInput(ch rune) bool {
	el := f.el
	if el == nil || !editableFormControl(el) {
		return true
	}
	if ch == '\r' {
		if el.LocalName() != "textarea" {
			return false // input 的 Enter 交给宿主（提交）
		}
		ch = '\n'
	}
	if ch < 0x20 {
		return true // 控制字符忽略（\r 已处理）
	}
	val := formControlValue(el)
	runes := []rune(val)
	start, end := f.sel()
	nv := string(runes[:start]) + string(ch) + string(runes[end:])
	if !f.applyValue(nv, start+1) {
		return true // 超 maxlength / readonly：消费但不修改
	}
	el.DispatchEvent(dom.NewInputEvent("insertText", string(ch), false))
	el.DispatchEvent(dom.NewEvent("change", true, false, false))
	return true
}

// KeyInput 处理编辑键（浏览器语义）。name 取值：Backspace/Delete/
// ArrowLeft/ArrowRight/Home/End/Enter。焦点元素非可编辑表单控件时返回
// false（未消费）。
//
// Enter：textarea 插入换行；input 返回 false（宿主提交语义）。
func (f *FormFocus) KeyInput(name string) bool {
	el := f.el
	if el == nil || !isFormTextControl(el) {
		return false
	}
	if name == "Enter" {
		if el.LocalName() != "textarea" {
			return false
		}
		return f.insertAtSel('\n')
	}
	val := formControlValue(el)
	runes := []rune(val)
	start, end := f.sel()
	switch name {
	case "Backspace":
		if start == end {
			if start <= 0 {
				return true // 光标在开头：无删除
			}
			start--
		}
		nv := string(runes[:start]) + string(runes[end:])
		if !f.applyValue(nv, start) {
			return true
		}
		el.DispatchEvent(dom.NewInputEvent("deleteContentBackward", "", false))
		el.DispatchEvent(dom.NewEvent("change", true, false, false))
		return true
	case "Delete":
		if start == end {
			if end >= len(runes) {
				return true
			}
			end++
		}
		nv := string(runes[:start]) + string(runes[end:])
		if !f.applyValue(nv, start) {
			return true
		}
		el.DispatchEvent(dom.NewInputEvent("deleteContentForward", "", false))
		el.DispatchEvent(dom.NewEvent("change", true, false, false))
		return true
	case "ArrowLeft":
		if start > 0 {
			f.setCaret(start - 1)
		}
		return true
	case "ArrowRight":
		if end < len(runes) {
			f.setCaret(end + 1)
		}
		return true
	case "Home":
		f.setCaret(0)
		return true
	case "End":
		f.setCaret(len(runes))
		return true
	}
	return false
}

// insertAtSel 在光标处插入单个字符（textarea 换行路径复用）。
func (f *FormFocus) insertAtSel(ch rune) bool {
	el := f.el
	val := formControlValue(el)
	runes := []rune(val)
	start, end := f.sel()
	nv := string(runes[:start]) + string(ch) + string(runes[end:])
	if !f.applyValue(nv, start+1) {
		return true
	}
	el.DispatchEvent(dom.NewInputEvent("insertText", string(ch), false))
	el.DispatchEvent(dom.NewEvent("change", true, false, false))
	return true
}

// applyValue 写入新值并移动光标（受 readonly/disabled/maxlength 限制）。
// 返回是否实际写入。
func (f *FormFocus) applyValue(nv string, caret int) bool {
	el := f.el
	if ml := editLimit(el); ml == 0 {
		return false // readonly/disabled
	} else if ml > 0 && len([]rune(nv)) > ml {
		return false // 超过 maxlength：浏览器拒绝插入
	}
	setFormControlValue(el, nv)
	f.setCaret(caret)
	f.markDirty()
	return true
}

// sel 返回 (start, end) 光标/选区区间（无选区时 start==end；无登记时
// 默认文本末尾——浏览器聚焦默认末尾）。
func (f *FormFocus) sel() (start, end int) {
	l := len([]rune(f.Value()))
	if s := rendering.FocusedFormControlSel; s != nil {
		start, end = s.Start, s.End
	} else {
		return l, l
	}
	if start > end {
		start, end = end, start
	}
	if start < 0 {
		start = 0
	}
	if end > l {
		end = l
	}
	return start, end
}

// setCaret 更新光标（仅重绘标记，不重建渲染树）。
func (f *FormFocus) setCaret(pos int) {
	l := len([]rune(f.Value()))
	if pos < 0 {
		pos = 0
	}
	if pos > l {
		pos = l
	}
	rendering.FocusedFormControlSel = &rendering.FormControlSelection{Start: pos, End: pos}
	if rv := f.wv.RenderView(); rv != nil {
		rv.MarkAllDirty()
	}
	f.dirty = true
}

// Tick 驱动光标闪烁（宿主每帧调用，挂钟驱动——无输入事件时也翻转：
// 标准浏览器 ≈500ms 节奏）。仅在聚焦时翻转；返回 true = 本帧发生了
// 翻转（宿主应重绘）。
func (f *FormFocus) Tick(now time.Time) bool {
	if f.el == nil {
		return false
	}
	if now.Sub(f.blinkTime) < 500*time.Millisecond {
		return false
	}
	f.blinkTime = now
	f.blinkOn = !f.blinkOn
	rendering.CaretVisibleControl = f.blinkOn
	rendering.CaretVisible = f.blinkOn
	f.dirty = true
	return true
}

// markDirty 值/焦点变化：重建渲染树 + 布局（下一帧 Layout 时批量执行）。
func (f *FormFocus) markDirty() {
	if fr := f.wv.mainFrame.Frame(); fr != nil {
		fr.MarkRenderTreeDirty()
		fr.SetNeedsLayout(true)
	}
	if rv := f.wv.RenderView(); rv != nil {
		rv.MarkAllDirty()
	}
	f.dirty = true
}

// ── 工具 ──────────────────────────────────────────────────────────────

// isFormTextControl 报告 el 是否为文本型表单控件：textarea，或 input 且
// type 非 checkbox/radio/range/color/file/submit/reset/button/image/hidden
// （镜像 HTMLTextFormControlElement::childShouldCreateRenderer）。
func isFormTextControl(el *dom.Element) bool {
	if el == nil {
		return false
	}
	switch el.LocalName() {
	case "textarea":
		return true
	case "input":
		t := strings.ToLower(el.GetAttribute("type"))
		switch t {
		case "checkbox", "radio", "range", "color", "file",
			"submit", "reset", "button", "image", "hidden":
			return false
		}
		return true
	}
	return false
}

// editableFormControl 报告控件是否接受文本编辑（text 型控件且非
// readonly/disabled——浏览器语义：只读控件仍可聚焦但不可修改）。
func editableFormControl(el *dom.Element) bool {
	return isFormTextControl(el) && editLimit(el) != 0
}

// editLimit 返回编辑限制：-1 = 无限制；0 = readonly/disabled（不可编辑）；
// >0 = maxlength（rune 数）。
func editLimit(el *dom.Element) int {
	if el == nil {
		return -1
	}
	// ★ 布尔属性（readonly/disabled 裸写无值）GetAttribute 返回 ""，
	// 必须用 HasAttribute 判断存在（Go HTML 布尔属性解析语义）。
	if el.HasAttribute("readonly") || el.HasAttribute("disabled") {
		return 0
	}
	if in, ok := html5.ToInputElement(el); ok {
		return in.MaxLength()
	}
	if ta, ok := html5.ToTextAreaElement(el); ok {
		return ta.MaxLength()
	}
	return -1
}

// formControlValue 读取表单控件当前值（textarea=文本内容；input=value
// 属性；非控件=文本内容）。
func formControlValue(el *dom.Element) string {
	switch el.LocalName() {
	case "textarea":
		if ta, ok := html5.ToTextAreaElement(el); ok {
			return ta.Value()
		}
	case "input":
		if in, ok := html5.ToInputElement(el); ok {
			return in.Value()
		}
	}
	return el.TextContent()
}

// setFormControlValue 写表单控件值（textarea 写文本内容——触发
// SetTreeChangeCallback 树变更感知；input 写 value 属性）。
func setFormControlValue(el *dom.Element, v string) {
	switch el.LocalName() {
	case "textarea":
		if ta, ok := html5.ToTextAreaElement(el); ok {
			ta.SetValue(v)
		}
	case "input":
		if in, ok := html5.ToInputElement(el); ok {
			in.SetValue(v)
		}
	}
}
