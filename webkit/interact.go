// 引擎级鼠标交互管线（Interaction）——wb-ui 的标准交互服务。
//
// 背景：app.Host 内置完整鼠标管线（active/hover/mousedown/mouseup/click/
// select 弹层/checkbox-radio/range/滚动）但只服务于 glfw Host；裸 WebView
// 宿主（如直播挂件助手的配置窗口）此前在宿主侧各写一套简化管线：
//   - select 下拉弹层缺失 → 「点击设备选择无任何效果」（弹层是 Host 机制）
//   - hover/click/onclick 属性执行/dblclick 判定重复实现且行为有偏差
//
// 本文件把浏览器标准鼠标交互下沉为引擎服务：任何 WebView（无论是否配套
// app.Host）调用 HandleMouseButton/MouseMove/Wheel 即可获得完整交互：
// active → mousedown → select 弹层开关 → checkbox/radio → mouseup →
// click（onclick 属性执行/DOM click 事件/label 转发）→ dblclick；
// 移动端 hover 追踪 + mouseover/mouseout 派发；滚轮滚动容器。
//
// 坐标约定：宿主喂入客户区 CSS 像素（与 HitTest 视口坐标一致），DPI 缩放
// 由宿主换算（与既有 configwin 行为一致）。
//
// 事件顺序（对齐浏览器）：
//
//	press:   mousedown
//	release: mouseup → click（按下/释放同元素才触发）→ dblclick（400ms 内）
package webkit

import (
	"log"
	"os"
	"strings"
	"time"

	"wb-ui/engine/js/bindings"
	"wb-ui/engine/dom"
	"wb-ui/engine/html5"
	"wb-ui/engine/popover"
	"wb-ui/engine/rendering"
)

// Interaction 是 WebView 的鼠标交互管线段。每个 WebView 一个（惰性创建），
// 状态不跨 WebView 共享。
type Interaction struct {
	wv *WebView

	// active 状态（mousedown 按下元素，对应浏览器 :active）
	activeEl *dom.Element
	// hoveredEl 当前 :hover 元素
	hoveredEl *dom.Element
	// pressed 左键是否按下中
	pressed bool
	// downKey 按下时的点击目标标识（释放时同元素才触发 click）
	downKey string
	// downOnclickKey 按下时「最深元素或其带 onclick 祖先」的目标标识
	//（容器 onclick 冒泡判定：press/release 最深目标不同但都在同一
	// onclick 祖先内时，以该祖先为 click 目标——configwin 树头/条目）
	downOnclickKey string

	// select 下拉弹层（固定定位浮层，nil 表示未打开）
	selectPopup       *dom.Element
	selectPopupSelect *dom.Element

	// 双击判定：同 key 400ms 内两次 click → dblclick
	lastClickKey string
	lastClickAt  time.Time

	// 最近光标位置（wheel/拖拽用）
	lastX, lastY float64

	// dirty 交互改变了渲染状态（hover/active/弹层/滚动等）——宿主渲染
	// 循环读取 NeedsRender() 后调用 ClearDirty()，避免事件风暴无限渲染。
	dirty bool
}

func (i *Interaction) markDirty() { i.dirty = true }

// NeedsRender 报告交互是否改变了需要重绘的状态。
func (i *Interaction) NeedsRender() bool { return i.dirty }

// ClearDirty 清除渲染标记（宿主渲染完成后调用）。
func (i *Interaction) ClearDirty() { i.dirty = false }

// IsPressed 报告左键是否按下中（宿主渲染节流/拖拽反馈用）。
func (i *Interaction) IsPressed() bool { return i.pressed }

// Hovered 返回当前 :hover 命中的元素（调试/测试查询用）。
func (i *Interaction) Hovered() *dom.Element { return i.hoveredEl }

// PopupOpen 报告 select 下拉弹层是否打开中（调试/测试查询用）。
func (i *Interaction) PopupOpen() bool { return i.selectPopup != nil }

// Cursor 返回最近一次光标位置（CSS 像素）。
func (i *Interaction) Cursor() (x, y float64) { return i.lastX, i.lastY }

// interactionKey 标识元素用于 click/dblclick 归属判定。用属性而非元素指针
// （宿主 JS 常以 innerHTML 重建 DOM，指针在两次事件间失效——configwin 的
// 双击判定即因此用 data-id/onclick/data-kind）。
func interactionKey(el *dom.Element) string {
	if el == nil {
		return ""
	}
	for _, attr := range []string{"data-id", "data-kind", "data-value", "data-cf"} {
		if v := el.GetAttribute(attr); v != "" {
			return v
		}
	}
	if v := el.GetAttribute("onclick"); v != "" {
		return v
	}
	return el.LocalName() + "." + el.GetAttribute("class")
}

// onclickAncestor 返回 el 自身或最近祖先中带 onclick 属性的元素（浏览器
// inline 事件处理器冒泡语义：点击子元素时祖先 onclick 也触发）。无则 nil。
func onclickAncestor(el *dom.Element) *dom.Element {
	for c := el; c != nil; c = c.ParentElement() {
		if c.GetAttribute("onclick") != "" {
			return c
		}
	}
	return nil
}

func (i *Interaction) view() *rendering.RenderView {
	if i.wv == nil || i.wv.destroyed {
		return nil
	}
	return i.wv.RenderView()
}

// dispatchMouse 把鼠标事件派发到命中元素（无命中 → document，拖拽/标题栏
// 场景需要 document 级监听器持续收到 mousemove）。坐标即 clientX/clientY。
func (i *Interaction) dispatchMouse(evtType string, x, y float64, buttons uint16) {
	rv := i.view()
	if rv == nil {
		return
	}
	el := rendering.HitTest(rv, x, y, "")
	if el == nil {
		if evtType == dom.EventMouseMove || evtType == dom.EventMouseUp {
			if fr := i.wv.MainFrame(); fr != nil {
				if f := fr.Frame(); f != nil {
					if doc := f.Document(); doc != nil {
						doc.DispatchEvent(dom.NewMouseEventFromInit(evtType, dom.MouseEventInit{
							EventInit: dom.EventInit{Bubbles: true, Cancelable: true},
							ClientX:   x,
							ClientY:   y,
							Button:    dom.MouseButtonLeft,
							Buttons:   buttons,
							Detail:    1,
						}))
					}
				}
			}
		}
		return
	}
	el.DispatchEvent(dom.NewMouseEventFromInit(evtType, dom.MouseEventInit{
		EventInit: dom.EventInit{Bubbles: true, Cancelable: true},
		ClientX:   x,
		ClientY:   y,
		Button:    dom.MouseButtonLeft,
		Buttons:   buttons,
		Detail:    1,
	}))
}

// MouseButton 处理鼠标按键事件。button：0=左键；action：0=Press 1=Release。
func (i *Interaction) MouseButton(x, y float64, button, action int) {
	if i.wv == nil || i.wv.destroyed {
		return
	}
	// 命中测试前同步渲染树（宿主 JS（openProp/openTextEditor 等）改
	// DOM 后渲染树要等渲染循环才重建；立即点击新弹窗元素会命中不到 →
	// 事件穿透到后方元素。EnsureHitTestReady = FlushRenderTreeDirty）。
	i.wv.EnsureHitTestReady()
	rv := i.wv.RenderView()
	if rv == nil {
		return
	}
	i.lastX, i.lastY = x, y

	if action == 0 { // Press
		if button == 2 {
			// 右键按下：不触发点击/聚焦交互（浏览器标准：右键只预备
			// contextmenu）；记录坐标供 Release 使用。
			i.lastX, i.lastY = x, y
			i.markDirty()
			return
		}
		if button != 0 {
			return // 仅左键触发点击交互（鼠标标准）
		}
		// ── Active 状态 + mousedown 派发 ──
		activeEl := rendering.HitTest(rv, x, y, "")
		if os.Getenv("WB_CONFIG_DEBUG") != "" {
			elInfo := "nil"
			if activeEl != nil {
				elInfo = activeEl.LocalName() + "#" + activeEl.GetAttribute("id") + "." + activeEl.GetAttribute("class")
			}
			log.Printf("[interact] press x=%.0f y=%.0f hit=%s", x, y, elInfo)
		}
		if activeEl != nil {
			activeEl.SetActive(true)
			i.activeEl = activeEl
			i.dispatchMouse(dom.EventMouseDown, x, y, 1)
		} else {
			i.activeEl = nil
		}
		// ★ 点击聚焦（浏览器标准：mousedown 时聚焦可编辑控件并定位光标；
		// 点非编辑区域失焦）。FormFocus 内建 blur 提交（onchange）——
		// 应用侧无需再实现 updateFocusAfterClick（已下沉）。
		if ff := i.wv.FormFocus(); ff != nil {
			ff.FocusFromHit(x, y)
			// ★ 按下即开始鼠标选择（拖选/双击词选）：聚焦控件内按下为
			// 锚点，拖动扩展，释放保留（见 FormFocus.BeginMouseSelect）。
			ff.BeginMouseSelect(x, y)
		}
		i.pressed = true
		i.downKey = interactionKey(activeEl)
		i.downOnclickKey = interactionKey(onclickAncestor(activeEl))
		// ── select 下拉弹层：打开中 → 选 option / 点外关闭；命中 select
		//   且未打开 → 打开 ──
		i.handleSelectPopup(rv, activeEl, x, y)
		// ── checkbox / radio 点击切换（浏览器标准）──
		i.handleCheckboxRadio(activeEl)
		// ── popover light dismiss（HTML §6.12.2）──
		// 规范把 light dismiss 挂在 pointerdown / pointerup 两个阶段：按下时
		// 记下「最上层的被点击 popover」，抬起时只有命中同一个才真正关闭——
		// 于是「在 popover 内部按住、拖到外面松开」（选文本）不会误关。
		// pointerdown 必须发生在点击的默认行为（打开 popover 的 invoker）之前，
		// 因此放在这里而不是 handleClick 里。
		popover.LightDismissPointerDown(i.wv.Document(), activeEl)
		i.markDirty()
		return
	}

	// ── Release ──
	if button == 2 {
		// 浏览器：右键 mouseup 后派发 contextmenu（可取消、可冒泡）；
		// 前端 @contextmenu.prevent 可接管菜单——未阻止且命中文本编辑
		// 控件时弹引擎默认编辑菜单（下沉 FormFocus.ShowDefaultEditMenu）。
		i.handleContextMenu(rv, x, y)
		i.markDirty()
		return
	}
	i.dispatchMouse(dom.EventMouseUp, x, y, 0)
	if !i.pressed {
		return
	}
	i.pressed = false
	// ── popover light dismiss（抬起阶段）──
	// 命中元素用抬起位置重新做一次（可能已跨出 popover），由 light dismiss
	// 自己比对与 pointerdown 是否同一目标。位置在 active 状态清除之前，与
	// 下面的 click 目标判定使用同一套命中结果。
	if popover.LightDismissPointerUp(i.wv.Document(), rendering.HitTest(rv, x, y, "")) {
		// 关闭了 popover：后面的 click 仍按浏览器语义派发（作者可能同时
		// 绑定了点击），只是命中结果可能因 popover 消失而变化。
		i.markDirty()
	}
	// ★ 释放结束拖选（选区保留高亮，供输入替换/Ctrl+C）。
	if ff := i.wv.FormFocus(); ff != nil {
		ff.EndMouseSelect()
	}
	// click：按下/释放命中同一目标（按 key 判定；DOM 重建时指针失效）
	clickEl := rendering.HitTest(rv, x, y, "onclick")
	if clickEl == nil {
		clickEl = rendering.HitTest(rv, x, y, "")
	}
	clickTarget := clickEl // 实际 click 目标（跨目标时可能替换为 onclick 祖先）
	if clickEl != nil && interactionKey(clickEl) != i.downKey {
		// press/release 最深目标不同：浏览器 click 目标 = mousedown/mouseup
		// target 的共同祖先。容器带 onclick 属性而子元素覆盖大片区域时
		// （configwin 树头 tree-head 内含 arrow/gname/gcount、tree-item 内含
		// dot/iname），press 命中最深子元素、release 命中容器 → downKey 不等
		// → click 被放弃（点击不生效）。补救：press/release 都落在同一
		// onclick 祖先内时以该祖先为 click 目标（浏览器 inline 处理器冒泡语义）。
		rEl := rendering.HitTest(rv, x, y, "")
		if oa := onclickAncestor(rEl); oa != nil &&
			interactionKey(oa) == i.downOnclickKey && i.downOnclickKey != "" {
			clickTarget = oa
		} else {
			clickTarget = nil // 跨目标（拖出/落入其他元素）不触发 click
		}
	}
	if clickTarget != nil {
		i.handleClick(rv, clickTarget, x, y)
		// dblclick：同一目标 400ms 内第二次 click → 派发 dblclick 事件
		//（浏览器顺序：click（第 2 次）→ dblclick；两次 click 的 onClick
		// 都执行，ondblclick/监听 dblclick 额外收到一次通知）。
		key := interactionKey(clickTarget)
		now := time.Now()
		if key != "" && key == i.lastClickKey && now.Sub(i.lastClickAt) < 400*time.Millisecond {
			clickTarget.DispatchEvent(dom.NewMouseEventFromInit(dom.EventDblClick, dom.MouseEventInit{
				EventInit: dom.EventInit{Bubbles: true, Cancelable: true},
				ClientX:   x,
				ClientY:   y,
				Button:    dom.MouseButtonLeft,
				Buttons:   1,
				Detail:    2,
			}))
			i.lastClickKey = ""
		} else {
			i.lastClickKey = key
			i.lastClickAt = now
		}
	}
	i.markDirty()
}

// MouseMove 处理鼠标移动：hover 追踪 + mousemove 派发。
func (i *Interaction) MouseMove(x, y float64) {
	i.lastX, i.lastY = x, y
	if i.wv == nil || i.wv.destroyed {
		return
	}
	// ★ 命中测试前同步渲染树（与 MouseButton 同款）：宿主 JS 改 DOM 后
	// 渲染树重建可能被变更风暴 cooldown 推迟，此窗口内 hover 追踪/
	// mousemove 派发会在旧树上解析——新元素收不到事件、已删除元素
	// 仍触发 hover。FlushRenderTreeDirty 树干净时零开销。
	i.wv.EnsureHitTestReady()
	rv := i.view()
	if rv == nil {
		return
	}
	newEl := rendering.HitTest(rv, x, y, "")
	if newEl != i.hoveredEl {
		if i.hoveredEl != nil {
			i.hoveredEl.SetHovered(false)
			i.dispatchHover(i.hoveredEl, newEl, x, y)
		}
		if newEl != nil {
			newEl.SetHovered(true)
			if i.hoveredEl == nil {
				i.dispatchHover(nil, newEl, x, y)
			}
		}
		i.hoveredEl = newEl
		i.markDirty() // :hover 样式重绘
	}
	// 事件派发：拖拽（按下中）必须持续派发（JS 拖拽/分隔条监听
	// document mousemove）；非按下时也派发（JS hover 效果），但
	// 不置 dirty（避免 鼠标移动→渲染→系统重发 WM_MOUSEMOVE 风暴）。
	if i.pressed {
		i.dispatchMouse(dom.EventMouseMove, x, y, 1)
		// ★ 拖选中扩展表单控件选区（拖选文本，见 FormFocus.ExtendMouseSelect；
		// 非控件/未按下时内部短路）。
		if ff := i.wv.FormFocus(); ff != nil {
			ff.ExtendMouseSelect(x, y)
		}
		i.markDirty()
	} else {
		i.dispatchMouse(dom.EventMouseMove, x, y, 0)
	}
}

// MouseLeave 鼠标离开窗口：清除 :hover 残留态。
func (i *Interaction) MouseLeave() {
	if i.hoveredEl != nil {
		i.hoveredEl.SetHovered(false)
		if fr := i.wv.MainFrame(); fr != nil {
			if f := fr.Frame(); f != nil {
				if doc := f.Document(); doc != nil {
					doc.DispatchEvent(dom.NewMouseEventFromInit(dom.EventMouseOut, dom.MouseEventInit{
						EventInit: dom.EventInit{Bubbles: true, Cancelable: true},
						ClientX:   i.lastX,
						ClientY:   i.lastY,
						Button:    dom.MouseButtonLeft,
						Buttons:   0,
						Detail:    0,
					}))
				}
			}
		}
		i.hoveredEl = nil
		i.markDirty()
	}
}

// dispatchHover 派发 mouseover/mouseout（进入/离开目标）。
func (i *Interaction) dispatchHover(oldEl, newEl *dom.Element, x, y float64) {
	fr := i.wv.MainFrame()
	if fr == nil {
		return
	}
	f := fr.Frame()
	if f == nil {
		return
	}
	doc := f.Document()
	if doc == nil {
		return
	}
	if oldEl != nil {
		oldEl.DispatchEvent(dom.NewMouseEventFromInit(dom.EventMouseOut, dom.MouseEventInit{
			EventInit: dom.EventInit{Bubbles: true, Cancelable: true},
			ClientX:   x,
			ClientY:   y,
			Button:    dom.MouseButtonLeft,
			Buttons:   0,
			Detail:    0,
		}))
	}
	if newEl != nil {
		newEl.DispatchEvent(dom.NewMouseEventFromInit(dom.EventMouseOver, dom.MouseEventInit{
			EventInit: dom.EventInit{Bubbles: true, Cancelable: true},
			ClientX:   x,
			ClientY:   y,
			Button:    dom.MouseButtonLeft,
			Buttons:   0,
			Detail:    0,
		}))
	}
}

// Wheel 处理滚轮：滚动命中容器 + 派发 wheel DOM 事件。deltaY 为滚轮
// 增量（Win32 符号：正=向上滚，取负为向下滚动）。
func (i *Interaction) Wheel(deltaY float64) {
	if i.wv == nil || i.wv.destroyed {
		return
	}
	// ★ 命中测试前同步渲染树（与 MouseButton 同款）：宿主 JS 改 DOM
	//（innerHTML 重建列表等）后渲染树重建被 cooldown 降频推迟的窗口内，
	// ScrollTargetAt 在旧树上解析 → 滚动目标丢失 → 滚轮不滚动（DOM 更新
	// 后立刻滚动失效根因）。FlushRenderTreeDirty 树干净时零开销，滚轮
	// 事件低频，强制重建成本可忽略。
	i.wv.EnsureHitTestReady()
	rv := i.view()
	if rv == nil {
		return
	}
	x, y := i.lastX, i.lastY
	tgt := rv.ScrollTargetAt(x, y)
	if tgt.Box == nil {
		// 无滚动容器：仍派发 wheel 事件（JS 监听器可处理）
		i.dispatchMouse(dom.EventWheel, x, y, 0)
		return
	}
	sx, sy := rv.BoxScrollOffset(tgt.Box)
	step := -deltaY / 120 * 48
	rvFor := tgt.RV
	if rvFor == nil {
		rvFor = rv
	}
	maxSy := 0.0
	if vm := rendering.VerticalScrollbarMetrics(rvFor, tgt.Box); vm.OK {
		maxSy = vm.MaxScroll
	}
	newSy := sy + step
	if newSy < 0 {
		newSy = 0
	}
	if newSy > maxSy {
		newSy = maxSy
	}
	if tgt.RV != nil {
		tgt.RV.SetBoxScrollOffset(tgt.Box, sx, newSy)
	}
	i.dispatchMouse(dom.EventWheel, x, y, 0)
	i.markDirty()
}

// handleContextMenu 右键释放：派发 contextmenu DOM 事件到命中元素
// （浏览器标准：mouseup 后触发，可冒泡、可取消、button=2）。事件未被
// JS preventDefault 且命中文本编辑控件 → 弹引擎默认编辑菜单（下沉
// FormFocus.ShowDefaultEditMenu——宿主注入菜单后端）。
func (i *Interaction) handleContextMenu(rv *rendering.RenderView, x, y float64) {
	if rv == nil {
		return
	}
	el := rendering.HitTest(rv, x, y, "")
	prevented := false
	if el != nil {
		prevented = !el.DispatchEvent(dom.NewMouseEventFromInit(dom.EventContextMenu, dom.MouseEventInit{
			EventInit: dom.EventInit{Bubbles: true, Cancelable: true},
			ClientX:   x,
			ClientY:   y,
			Button:    dom.MouseButtonRight,
			Detail:    1,
		}))
	}
	if !prevented {
		if ff := i.wv.FormFocus(); ff != nil {
			ff.ShowDefaultEditMenu(x, y, el)
		}
	}
}

// ── click 处理 ──

// handleClick 执行单击语义：命中带 onclick 属性的元素 → 执行内联处理器
// （this=元素，浏览器标准）；否则派发冒泡 click 事件；两者都命中时执行
// label 转发（浏览器标准）。
func (i *Interaction) handleClick(rv *rendering.RenderView, el *dom.Element, x, y float64) {
	if code := el.GetAttribute("onclick"); code != "" {
		if execInlineHandler(i.wv, el, "onclick") {
			// JS 执行已完成，DOM 可能已改（selectWidget 等重建 innerHTML）/
			// 类变化（toggleGroup 等翻转 open）——立即同步渲染树+布局
			//（与 selectPopup 同款一次性同步；点击是低频操作，重建开销
			// 可忽略）。不在此同步时，重建依赖渲染循环的脏标记 + cooldown
			// 降频（最长 ~2 帧 + 节流延迟）——实测「点击后延迟生效」。
			i.wv.RebuildRenderTree()
			i.wv.EnsureLayout()
			return
		}
	}
	clickEv := dom.NewMouseEventFromInit(dom.EventClick, dom.MouseEventInit{
		EventInit: dom.EventInit{Bubbles: true, Cancelable: true},
		ClientX:   x,
		ClientY:   y,
		Button:    dom.MouseButtonLeft,
		Buttons:   1,
		Detail:    1,
	})
	prevented := !el.DispatchEvent(clickEv)
	// Popover invoker 的激活行为（HTML §6.12.1 与 form-elements 里 button 的
	// 激活行为）：popovertarget / commandfor 指向 popover 时切换/显示/隐藏它。
	// 规范里这是 click 的默认行为，因此被 preventDefault 时不做。
	if !prevented {
		popover.RunActivation(el, el)
	}
	handleLabelToggle(el)
}

// execInlineHandler 执行元素的内联事件属性处理器（onclick/onchange）：
// 浏览器语义 this=元素、event=事件对象。引擎 EvalJS 为全局作用域执行
// （this 非元素）——用临时属性标记 + 脚本内 querySelector 定位 +
// new Function 绑定 this（与 configwin commitInput 的 data-cf 机制一致，
// 页面处理器普遍引用 this.value（select/input 的 onchange））。
// 返回 true=处理器存在且已执行。
func execInlineHandler(wv *WebView, el *dom.Element, attr string) bool {
	code := el.GetAttribute(attr)
	if code == "" || wv == nil {
		return false
	}
	const mark = "data-inline-h"
	el.SetAttribute(mark, "1")
	script := `(function(){
		var el = document.querySelector('[data-inline-h="1"]');
		if (!el) return;
		el.removeAttribute('data-inline-h');
		var code = el.getAttribute('` + attr + `');
		if (!code) return;
		var fn = new Function('event', code);
		fn.call(el, null);
	})()`
	_, err := wv.EvalJS(script)
	el.RemoveAttribute(mark) // 属性清理兜底（JS 已删；幂等）
	return err == nil
}

// handleLabelToggle 实现 <label> 的点击转发：点击 label 或其任意后代，
// 切换内部包裹的 checkbox/radio（浏览器 label 语义）。
func handleLabelToggle(el *dom.Element) {
	if el == nil {
		return
	}
	lab := el
	for lab != nil && lab.LocalName() != "label" {
		lab = lab.ParentElement()
	}
	if lab == nil {
		return
	}
	for c := lab.FirstChild(); c != nil; c = c.NextSibling() {
		e, ok := c.(*dom.Element)
		if !ok || e.LocalName() != "input" {
			continue
		}
		typ := e.GetAttribute("type")
		if typ != "checkbox" && typ != "radio" {
			continue
		}
		in, ok := html5.ToInputElement(e)
		if !ok {
			continue
		}
		if typ == "checkbox" {
			in.SetChecked(!in.Checked())
		} else {
			in.SetChecked(true)
		}
		return
	}
}

// handleCheckboxRadio mousedown 时的 checkbox/radio 点击切换（浏览器
// 在 click 时切换；引擎统一在按下时切换并派发 change——与 app.Host 一致）。
func (i *Interaction) handleCheckboxRadio(activeEl *dom.Element) {
	if activeEl == nil || activeEl.LocalName() != "input" {
		return
	}
	inputType := activeEl.GetAttribute("type")
	if inputType != "checkbox" && inputType != "radio" {
		return
	}
	in, ok := html5.ToInputElement(activeEl)
	if !ok {
		return
	}
	if inputType == "checkbox" {
		in.SetChecked(!in.Checked())
	} else {
		// radio：同行内同名互斥
		name := activeEl.GetAttribute("name")
		if name != "" && i.wv.MainFrame() != nil {
			if doc := i.wv.MainFrame().Document(); doc != nil {
				for _, r := range doc.GetElementsByTagName("input") {
					if r.GetAttribute("type") == "radio" && r.GetAttribute("name") == name {
						if r2, ok2 := html5.ToInputElement(r); ok2 {
							r2.SetChecked(false)
						}
					}
				}
			}
		}
		in.SetChecked(true)
	}
	// 点击切换 checkbox/radio = 用户交互 → user validity 置位
	// （:user-valid / :user-invalid 依赖它）。
	bindings.MarkUserInteracted(activeEl)
	activeEl.DispatchEvent(dom.NewEvent("change", true, false, false))
}

// ── select 下拉弹层 ──

// handleSelectPopup 弹层开关：打开中点 popup 内 option → 选择并关闭；
// 点 popup 外 → 关闭；命中 select 且未打开 → 打开。
func (i *Interaction) handleSelectPopup(rv *rendering.RenderView, activeEl *dom.Element, x, y float64) {
	if i.selectPopup != nil {
		if i.popupContains(activeEl) {
			i.selectPopupOptionClicked(activeEl)
			i.closeSelectPopup()
		} else {
			i.closeSelectPopup()
		}
	}
	sel := activeEl
	if sel != nil && sel.LocalName() != "select" {
		for p := sel.ParentElement(); p != nil; p = p.ParentElement() {
			if p.LocalName() == "select" {
				sel = p
				break
			}
		}
	}
	if sel != nil && sel.LocalName() == "select" && i.selectPopup == nil {
		i.handleSelectClick(sel, rv, x, y)
	}
}

// handleSelectClick 打开 select 下拉弹层：固定定位浮层层列出 <option>，
// 点击 option 设值 + 派发 change（宿主 JS 的 onchange 由 change 事件驱动）。
func (i *Interaction) handleSelectClick(sel *dom.Element, rv *rendering.RenderView, cssX, cssY float64) {
	selEl, ok := html5.ToSelectElement(sel)
	if !ok {
		return
	}
	if selEl.Disabled() {
		return
	}
	var sx, sy float64
	var boxW float64 = 180
	var boxH float64 = 0
	if box := rv.FindRenderBoxForNode(sel); box != nil {
		// BoxViewportRect 返回视口坐标（布局坐标减去所有祖先滚动容器的滚动偏移），
		// 与 position:fixed 定位一致；AbsoluteX/Y 是布局坐标，不含滚动补偿。
		vx, vy, vw, vh := rendering.BoxViewportRect(rv, box)
		sx, sy = vx, vy
		boxH = vh
		if vw > 0 {
			boxW = vw
		}
	}
	doc := i.wv.MainFrame().Document()
	if doc == nil {
		return
	}
	overlay := doc.CreateElement("div")
	overlay.SetAttribute("class", "select-popup")
	opts := selEl.Options()
	rowH := 24
	n := len(opts)
	if n > 8 {
		n = 8
	}
	popH := n*rowH + 4
	popTop := sy + boxH
	viewH := i.wv.Height()
	if popTop+float64(popH) > float64(viewH)-8 {
		popTop = sy - float64(popH)
		if popTop < 0 {
			popTop = 0
		}
	}
	overlay.SetAttribute("style", "position:fixed;left:"+itoa(int(sx))+"px;top:"+itoa(int(popTop))+"px;width:"+itoa(int(boxW))+"px;height:"+itoa(popH)+"px;background:var(--select-popup-bg,#1c2333);border:1px solid var(--select-popup-border,#3a4a75);border-radius:4px;box-shadow:0 4px 12px rgba(0,0,0,0.4);z-index:9999;overflow-y:auto;")
	current := selEl.Value()
	for _, opt := range opts {
		optEl := doc.CreateElement("div")
		optEl.SetAttribute("class", "select-popup-option")
		optVal := opt.GetAttribute("value")
		if !opt.HasAttribute("value") {
			optVal = opt.TextContent()
		}
		optEl.SetAttribute("data-value", optVal)
		optEl.SetAttribute("data-select-popup", "1")
		cls := "select-popup-option"
		if opt.HasAttribute("disabled") {
			cls += " select-popup-option-disabled"
		} else if optVal == current {
			cls += " select-popup-option-selected"
		}
		optEl.SetAttribute("class", cls)
		optEl.SetAttribute("style", "display:block;padding:4px 10px;font-size:14px;line-height:16px;color:var(--select-popup-fg,#e8eaf0);cursor:pointer;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;")
		txt := doc.CreateTextNode(opt.TextContent())
		_ = optEl.AppendChild(txt)
		_ = overlay.AppendChild(optEl)
	}
	body := doc.Body()
	if body == nil {
		return
	}
	_ = body.AppendChild(overlay)
	// :hover 选项高亮（页面可覆盖样式；引擎兜底规则防「弹层可见无样式」）
	i.ensurePopupStyle(doc)
	i.selectPopup = overlay
	i.selectPopupSelect = sel
	// ★ 一次性重建+布局（DOM 变更（弹层/样式）可能已标记渲染树脏，重建
	// 在脏标记存在时立即执行；无脏则同步当前树）
	i.wv.RebuildRenderTree()
	i.wv.EnsureLayout()
}

// ensurePopupStyle 注入弹层默认样式（幂等：页面有同名规则时以页面为准，
// 追加的 style 元素不覆盖页面规则——页面规则在 style 注入前已解析，
// 相同选择器后者优先：注入在 body 尾部的 style 晚于 head 规则 → 引擎
// 兜底生效；页面在 head 中预置规则则更早解析 → 页面规则优先）。
func (i *Interaction) ensurePopupStyle(doc *dom.Document) {
	if doc == nil {
		return
	}
	if doc.GetElementById("wb-ui-select-popup-style") != nil {
		return
	}
	style := doc.CreateElement("style")
	style.SetAttribute("id", "wb-ui-select-popup-style")
	css := "\n.select-popup-option:hover{background:#2a3a5f}\n.select-popup-option-selected{background:#3b6fd4;color:#fff}\n.select-popup-option-disabled{opacity:.45;cursor:not-allowed}\n"
	_ = style.AppendChild(doc.CreateTextNode(css))
	if head := doc.Head(); head != nil {
		_ = head.AppendChild(style)
	} else if body := doc.Body(); body != nil {
		_ = body.AppendChild(style)
	}
}

// closeSelectPopup 移除下拉弹层。
func (i *Interaction) closeSelectPopup() {
	if i.selectPopup == nil {
		return
	}
	body := i.wv.MainFrame().Document().Body()
	if body != nil {
		_ = body.RemoveChild(i.selectPopup)
	}
	i.selectPopup = nil
	i.selectPopupSelect = nil
	i.wv.RebuildRenderTree()
}

// popupContains 报告 el 是否在弹层内。
func (i *Interaction) popupContains(el *dom.Element) bool {
	if el == nil || i.selectPopup == nil {
		return false
	}
	for p := el; p != nil; p = p.ParentElement() {
		if p == i.selectPopup {
			return true
		}
	}
	return false
}

// selectPopupOptionClicked 应用弹层选项的点击：设置 select 值 + change。
func (i *Interaction) selectPopupOptionClicked(el *dom.Element) {
	if el == nil || i.selectPopupSelect == nil {
		return
	}
	if el.LocalName() == "div" && el.GetAttribute("data-select-popup") == "1" {
		cls := el.GetAttribute("class")
		if strings.Contains(cls, "disabled") {
			return
		}
		sel := i.selectPopupSelect
		if selEl, ok := html5.ToSelectElement(sel); ok {
			selEl.SetValue(el.GetAttribute("data-value"))
		}
		// change 派发（onchange 属性处理器由 dom 层 InlineEventAttrRunner
		// 钩子在派发路径统一执行——宿主 JS 面板的 onchange="apply('id',
		// 'device',this.value)" 生效，此前依赖此处手动执行，现钩子接管）。
		// 用户选择了 option → user validity 置位。
		bindings.MarkUserInteracted(sel)
		sel.DispatchEvent(dom.NewEvent("change", true, false, false))
	}
}

// itoa 整数转十进制。
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [12]byte
	pos := len(buf)
	for v > 0 {
		pos--
		buf[pos] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
