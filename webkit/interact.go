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
	"time"

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

	// 帧内合并的待处理移动（T4 事件派发聚合）：宿主在事件批里用
	// MouseMoveBatched 记录坐标、批末 FlushMoves 派发一次。pendingMove 为
	// false 时零开销，默认路径（MouseMove）完全不受影响。
	pendingMove        bool
	pendingMoveX       float64
	pendingMoveY       float64

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
	i.dispatchMouseAt(evtType, x, y, buttons, nil, false)
}

// dispatchMouseAt 派发鼠标事件到命中元素（未命中时按原语义把 move/up 派发到
// document 级监听器）。
//
// known=true 表示 target 是调用方**已经算出的**命中结果（Move 路径的 hover
// 追踪刚对同一坐标命中过）：鼠标移动是最高频的交互事件，每次移动重复命中
// 测试要遍历整棵渲染树，复用同一结果即可省掉一半命中开销（T4）。
func (i *Interaction) dispatchMouseAt(evtType string, x, y float64, buttons uint16, target *dom.Element, known bool) {
	if !known {
		rv := i.view()
		if rv == nil {
			return
		}
		target = rendering.HitTest(rv, x, y, "")
	}
	el := target
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
	i.mouseMoveNow(x, y)
}

// MouseMoveBatched 记录一次鼠标移动，待 FlushMoves 统一处理（帧内合并）。
//
// 鼠标移动是事件批里最密集的事件（系统重发 WM_MOUSEMOVE、拖拽时一轮
// PollEvents 收到多条）；逐条走完整管线意味着每条都做命中测试 + 构造事件
// 对象 + 冒泡派发，而中间坐标页面根本观察不到——JS 看到的是最后位置。
// 浏览器同样对 mousemove 做帧内合并（UI Events 的 coalesced events）。
// 宿主在事件批开始时改用本方法、批末调用 FlushMoves 即可合并一批移动。
func (i *Interaction) MouseMoveBatched(x, y float64) {
	i.pendingMove = true
	i.pendingMoveX, i.pendingMoveY = x, y
}

// FlushMoves 派发待处理的鼠标移动；无待处理时零开销（首行即返回）。
func (i *Interaction) FlushMoves() {
	if !i.pendingMove {
		return
	}
	i.pendingMove = false
	i.mouseMoveNow(i.pendingMoveX, i.pendingMoveY)
}

// mouseMoveNow 是一次完整的移动处理（MouseMove / FlushMoves 的共同实现）。
func (i *Interaction) mouseMoveNow(x, y float64) {
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
		oldHover := i.hoveredEl
		if oldHover != nil {
			oldHover.SetHovered(false)
		}
		if newEl != nil {
			newEl.SetHovered(true)
		}
		i.hoveredEl = newEl
		// ★ 单次派发完整 hover 序列（out/leave + over/enter，含最近共同祖先
		// 判定）。旧的两段式调用（oldEl != nil 时只派 out、oldEl == nil 时
		// 才派 over）丢掉了 mouseenter/mouseleave，且把「指针在同一组件内部
		// 移动」误判为离开组件（@mouseleave 绑在面板上的下拉菜单会因此自关闭）。
		i.dispatchHover(oldHover, newEl, x, y)
		i.markDirty() // :hover 样式重绘
	}
	// 事件派发：拖拽（按下中）必须持续派发（JS 拖拽/分隔条监听
	// document mousemove）；非按下时也派发（JS hover 效果），但
	// 不置 dirty（避免 鼠标移动→渲染→系统重发 WM_MOUSEMOVE 风暴）。
	if i.pressed {
		// ★ T4：复用上面 hover 追踪刚算出的命中结果，省一次渲染树命中测试。
		i.dispatchMouseAt(dom.EventMouseMove, x, y, 1, newEl, true)
		// ★ 拖选中扩展表单控件选区（拖选文本，见 FormFocus.ExtendMouseSelect；
		// 非控件/未按下时内部短路）。
		if ff := i.wv.FormFocus(); ff != nil {
			ff.ExtendMouseSelect(x, y)
		}
		i.markDirty()
	} else {
		i.dispatchMouseAt(dom.EventMouseMove, x, y, 0, newEl, true)
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

// dispatchHover 派发完整 hover 序列（UI Events）：mouseout/mouseleave 到离开
// 链、mouseover/mouseenter 到进入链。mouseover/mouseout 冒泡且带
// relatedTarget；mouseenter/mouseleave 不冒泡、relatedTarget 为 null。
//
// ★ 链的范围由最近共同祖先（LCA）决定：指针从 oldEl 移到 newEl 时，只有 LCA
// 之下的元素真的「离开/进入」。鼠标在同一组件内部移动（如面板 padding 命中
// 面板自身 → 移入其子菜单项）不得给面板派 mouseleave，否则组件的
// hover-to-close 逻辑会误关（与 app.Host.dispatchHoverEvents 同一语义）。
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
	if oldEl == nil && newEl == nil {
		return
	}
	common := hoverCommonAncestor(oldEl, newEl)
	if oldEl != nil {
		var mouseOutRel dom.EventTarget
		if newEl != nil {
			mouseOutRel = newEl
		}
		oldEl.DispatchEvent(dom.NewMouseEventFromInit(dom.EventMouseOut, dom.MouseEventInit{
			EventInit:     dom.EventInit{Bubbles: true, Cancelable: true},
			ClientX:       x,
			ClientY:       y,
			Button:        dom.MouseButtonLeft,
			Buttons:       0,
			Detail:        0,
			RelatedTarget: mouseOutRel,
		}))
		// mouseleave：沿离开链自内向外逐个派发（不冒泡），到 LCA 为止。
		for n := dom.Node(oldEl); n != nil && n != common; n = n.ParentNode() {
			leaveEl, ok := n.(*dom.Element)
			if !ok {
				break // Document/Text 等非元素节点不接收 mouseleave。
			}
			leaveEl.DispatchEvent(dom.NewMouseEventFromInit(dom.EventMouseLeave, dom.MouseEventInit{
				EventInit: dom.EventInit{Bubbles: false, Cancelable: false},
				ClientX:   x,
				ClientY:   y,
				Button:    dom.MouseButtonLeft,
				Buttons:   0,
				Detail:    0,
			}))
		}
	}
	if newEl != nil {
		var mouseOverRel dom.EventTarget
		if oldEl != nil {
			mouseOverRel = oldEl
		}
		newEl.DispatchEvent(dom.NewMouseEventFromInit(dom.EventMouseOver, dom.MouseEventInit{
			EventInit:     dom.EventInit{Bubbles: true, Cancelable: true},
			ClientX:       x,
			ClientY:       y,
			Button:        dom.MouseButtonLeft,
			Buttons:       0,
			Detail:        0,
			RelatedTarget: mouseOverRel,
		}))
		// mouseenter：沿进入链自外向内逐个派发（不冒泡），自 LCA 之下开始。
		var enterChain []*dom.Element
		for n := dom.Node(newEl); n != nil && n != common; n = n.ParentNode() {
			enterEl, ok := n.(*dom.Element)
			if !ok {
				break
			}
			enterChain = append(enterChain, enterEl)
		}
		for idx := len(enterChain) - 1; idx >= 0; idx-- {
			enterChain[idx].DispatchEvent(dom.NewMouseEventFromInit(dom.EventMouseEnter, dom.MouseEventInit{
				EventInit: dom.EventInit{Bubbles: false, Cancelable: false},
				ClientX:   x,
				ClientY:   y,
				Button:    dom.MouseButtonLeft,
				Buttons:   0,
				Detail:    0,
			}))
		}
	}
}

// hoverCommonAncestor is the *dom.Element flavour of dom.CommonAncestor: it
// guards the typed-nil case (a nil *dom.Element boxed into a dom.Node is not a
// nil interface) and delegates to the single engine-side implementation.
func hoverCommonAncestor(a, b *dom.Element) dom.Node {
	if a == nil || b == nil {
		return nil
	}
	return dom.CommonAncestor(a, b)
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
		// <summary> 的点击激活行为（HTML §4.11.4）：切换所属 <details> 的 open
		// 并派发 toggle。open 变化会经 UA 规则 details:not([open]) > :not(summary)
		// 改变子树 display → 立即同步渲染树与布局（与 select 弹层同款一次性同步）。
		if html5.ActivateSummary(el) {
			i.wv.RebuildRenderTree()
			i.wv.EnsureLayout()
		}
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

// handleLabelToggle 实现 <label> 的点击转发（切换 label 内第一个
// checkbox/radio）。实现在 ToggleLabeledControl——与 app.Host 共用同一份，
// 避免「按下切换」与「label 转发」两套语义（radio 同组互斥、change 派发）。
func handleLabelToggle(el *dom.Element) {
	ToggleLabeledControl(el)
}

// handleCheckboxRadio mousedown 时的 checkbox/radio 点击切换（浏览器在
// click 时切换；引擎统一在按下时切换并派发 change）。实现在
// ToggleCheckboxRadio——与 app.Host 共用同一份。
func (i *Interaction) handleCheckboxRadio(activeEl *dom.Element) {
	ToggleCheckboxRadio(activeEl)
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

// handleSelectClick 打开 select 下拉弹层：定位、构造、选项样式、
// 兜底样式表注入都在 BuildSelectPopup（与 app.Host 共用同一份）。
func (i *Interaction) handleSelectClick(sel *dom.Element, rv *rendering.RenderView, cssX, cssY float64) {
	overlay := BuildSelectPopup(i.wv, sel, rv)
	if overlay == nil {
		return
	}
	i.selectPopup = overlay
	i.selectPopupSelect = sel
	// ★ 一次性重建+布局（DOM 变更（弹层/样式）可能已标记渲染树脏，重建
	// 在脏标记存在时立即执行；无脏则同步当前树）
	i.wv.RebuildRenderTree()
	i.wv.EnsureLayout()
}

// closeSelectPopup 移除下拉弹层（共享实现 RemoveSelectPopup 顺带重建渲染树）。
func (i *Interaction) closeSelectPopup() {
	if i.selectPopup == nil {
		return
	}
	RemoveSelectPopup(i.wv, i.selectPopup)
	i.selectPopup = nil
	i.selectPopupSelect = nil
}

// popupContains 报告 el 是否在弹层内。
func (i *Interaction) popupContains(el *dom.Element) bool {
	return SelectPopupContains(i.selectPopup, el)
}

// selectPopupOptionClicked 应用弹层选项的点击（共享实现
// ApplySelectPopupOption：写值 + user validity 置位 + change 派发）。
func (i *Interaction) selectPopupOptionClicked(el *dom.Element) {
	ApplySelectPopupOption(i.selectPopupSelect, el)
}
