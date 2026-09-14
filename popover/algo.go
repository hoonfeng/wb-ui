// Popover 的算法实现（HTML Standard §6.12 的「show popover」「hide popover」
// 「hide popovers until」「popover light dismiss」各段）。
//
// 每个函数头部标注了规范里的对应算法名，步骤顺序与规范一一对应；规范里
// 「Assert」的步骤以注释标出（不运行时代码断言——引擎对页面脚本的非法调用
// 要像浏览器一样容错，而不是 panic）。
package popover

import (
	"wb-ui/dom"
)

// Error 是 popover 算法抛出的异常（规范里的 DOMException）。Name 取规范的
// 错误名（"NotSupportedError" / "InvalidStateError"），bindings 把它转成
// JS 侧的错误对象。
type Error struct {
	Name    string
	Message string
}

func (e *Error) Error() string {
	if e.Message == "" {
		return e.Name
	}
	return e.Name + ": " + e.Message
}

// modeFromAttr 按枚举属性规则把「popover 属性的值 + 是否存在」解析成状态
// （HTML §6.12：缺省值默认 No Popover、空值默认 Auto、无效值默认 Manual）。
func modeFromAttr(value string, exists bool) Mode {
	if !exists {
		return ModeNone
	}
	switch lowerTrim(value) {
	case "", "auto":
		return ModeAuto
	case "hint":
		return ModeHint
	}
	return ModeManual
}

// checkPopoverValidity 实现规范「check popover validity」：返回是否有效；
// 无效时第二个返回值是应抛出的异常（nil 表示「静默无效」）。
//
//	popover 属性处于 No Popover 状态            → NotSupportedError
//	期望显示但并非 hidden、或期望隐藏但并非 showing → 静默无效（false, nil）
//	元素未连接 / 文档不是期望的文档 /
//	元素是模态 dialog / 元素处于全屏            → InvalidStateError
func checkPopoverValidity(el *dom.Element, expectedToBeShowing bool, expectedDoc *dom.Document) (bool, error) {
	return checkPopoverValidityEx(el, expectedToBeShowing, expectedDoc, false)
}

// checkPopoverValidityEx 是 checkPopoverValidity 的可放宽版本：allowNoPopover
// 为 true 时跳过「popover 属性处于 No Popover 状态」这条检查。
//
// 它专供 attribute change steps 使用：属性变更时属性**已经被改掉**，而规范里
// 的 hide popover 第一步会因此抛 NotSupportedError 并静默返回（元素卡在
// showing 状态）。浏览器的实际行为是照常关闭——WPT
// html/semantics/popovers/popover-remove-attribute-during-focusing-steps.html
// 就期望「focus 处理器里 removeAttribute("popover")」之后照常派发
// toggle(open→closed)，因此本端口对齐浏览器而非规范字面。
func checkPopoverValidityEx(el *dom.Element, expectedToBeShowing bool, expectedDoc *dom.Document, allowNoPopover bool) (bool, error) {
	if el == nil {
		return false, nil
	}
	if ModeOf(el) == ModeNone && !allowNoPopover {
		return false, &Error{
			Name:    "NotSupportedError",
			Message: "Failed to execute popover operation: the element does not have a popover attribute.",
		}
	}
	st := el.Popover()
	if expectedToBeShowing {
		if !st.Showing {
			return false, nil
		}
	} else if st.Showing {
		return false, nil
	}
	doc := el.OwnerDocument()
	if doc == nil || !el.IsConnected() ||
		(expectedDoc != nil && doc != expectedDoc) ||
		el.IsModalDialog() ||
		doc.FullscreenElement() == el {
		return false, &Error{
			Name:    "InvalidStateError",
			Message: "Failed to execute popover operation: the element is not in a valid state for this operation.",
		}
	}
	return true, nil
}

// fireToggleEvent 派发 beforetoggle / toggle（带 oldState / newState / source）。
// beforetoggle 是可取消的（规范：打开/关闭算法在第一步给它否决机会）。
// 返回事件是否未被取消（true = 继续）。
func fireToggleEvent(el *dom.Element, evType string, cancelable bool, oldState, newState string, source *dom.Element) bool {
	ev := dom.NewToggleEvent(evType, false, cancelable, oldState, newState, source)
	el.DispatchEvent(ev)
	return !ev.DefaultPrevented()
}

// queuePopoverToggleTask 实现规范「queue a popover toggle event task」：
// 异步派发 toggle。规范要求同一元素只保留一个待派发的任务——新任务到来时
// 复用旧任务的 oldState 并移除旧任务，因此在同一批任务内连续 open→closed
// 只会观察到一次 toggle（oldState 是第一次迁移前的状态）。本端口的任务队列
// 无法真正取消已排队的任务，改用序号让过期任务自行退出（语义等价）。
func queuePopoverToggleTask(el *dom.Element, oldState, newState string, source *dom.Element) {
	st := el.Popover()
	if st.TogglePending {
		oldState = st.ToggleOldState
	}
	st.TogglePending = true
	st.ToggleOldState = oldState
	st.ToggleSeq++
	seq := st.ToggleSeq
	task := func() {
		cur := el.Popover()
		if cur.ToggleSeq != seq {
			return // 已被更晚的状态迁移取代
		}
		cur.TogglePending = false
		cur.ToggleOldState = ""
		el.DispatchEvent(dom.NewToggleEvent("toggle", false, false, oldState, newState, source))
	}
	runTask(el, task)
}

// runPopoverFocusingSteps 实现规范「popover focusing steps」：
//
//	subject 自身带 autofocus → 聚焦它；
//	否则取 subject 的 autofocus delegate（tree order 中第一个带 autofocus 的
//	后代）——这就是 MDN 示例里「popover 内第一个按钮加 autofocus，打开时自动
//	获得键盘焦点」的依据；
//	两者都没有 → 不动焦点（作者没有要求初始焦点）。
//
// 简化：subject 是 <dialog> 时规范要求改走 dialog focusing steps（含「默认
// 焦点元素」等规则），本端口 dialog 的 focusing steps 未实现，对 dialog 上的
// popover 也走上面的通用规则。
func runPopoverFocusingSteps(el *dom.Element) {
	control := el
	if !control.HasAttribute("autofocus") {
		control = autofocusDelegate(el)
	}
	if control != nil {
		focusElement(control)
	}
}

// autofocusDelegate 实现规范的「autofocus delegate」（type="other"）：在 root
// 的后代里按 tree order 找第一个带 autofocus 属性的元素（不含 root 自身）。
func autofocusDelegate(root *dom.Element) *dom.Element {
	for c := root.FirstChild(); c != nil; c = c.NextSibling() {
		e, ok := c.(*dom.Element)
		if !ok {
			continue
		}
		if e.HasAttribute("autofocus") {
			return e
		}
		if d := autofocusDelegate(e); d != nil {
			return d
		}
	}
	return nil
}

// Show 实现规范「show popover」：把元素加入 top layer 并置为 showing。
//
// throwExceptions 为 true 时把规范要求的 DOMException 作为 error 返回（JS 层
// 的 showPopover()/togglePopover() 传 true，invoker 的激活行为传 false）。
func Show(el *dom.Element, throwExceptions bool, source *dom.Element) error {
	if el == nil {
		return nil
	}
	doc := el.OwnerDocument()
	if doc == nil {
		return nil
	}
	dp := doc.Popover()
	// 规范第一步：正在显示/隐藏其他 popover 时不得再打开（防止 beforetoggle
	// 处理器里递归打开引发不一致）。
	if dp.Showing || dp.HidingCount != 0 {
		if throwExceptions {
			return &Error{
				Name:    "InvalidStateError",
				Message: "Failed to execute 'showPopover' on 'HTMLElement': this operation is not allowed while another popover is being shown or hidden.",
			}
		}
		return nil
	}

	ok, err := checkPopoverValidity(el, false, nil)
	if err != nil {
		if throwExceptions {
			return err
		}
		return nil
	}
	if !ok {
		return nil
	}

	dp.Showing = true
	cleanupShowing := func() { dp.Showing = false }

	if !fireToggleEvent(el, "beforetoggle", true, StateClosed, StateOpen, source) {
		cleanupShowing()
		return nil
	}
	// beforetoggle 处理器可能断开了元素或改了 popover 属性 → 重新校验。
	ok, err = checkPopoverValidity(el, false, doc)
	if err != nil {
		cleanupShowing()
		if throwExceptions {
			return err
		}
		return nil
	}
	if !ok {
		cleanupShowing()
		return nil
	}

	shouldRestoreFocus := false
	originalType := ModeOf(el)
	effectiveType := originalType
	var ancestor *dom.Element
	if originalType == ModeAuto || originalType == ModeHint {
		ancestor = topmostPopoverAncestor(el, source, true)
		// auto popover 不能以 hint popover 为父：降级为 hint（否则 hint 的
		// light dismiss 语义会穿过 auto 层级）。
		if ancestor != nil && ancestor.Popover().Mode == string(ModeHint) && effectiveType == ModeAuto {
			effectiveType = ModeHint
		}
		hidePopoverStackUntil(doc, ancestor, ModeHint, false, true)
		if effectiveType == ModeAuto {
			hidePopoverStackUntil(doc, ancestor, ModeAuto, false, true)
		}
		// hide 过程里可能触发 beforetoggle 改掉本元素的 popover 属性。
		if originalType != ModeOf(el) {
			cleanupShowing()
			if throwExceptions {
				return &Error{
					Name:    "InvalidStateError",
					Message: "Failed to execute 'showPopover' on 'HTMLElement': the popover attribute changed while showing.",
				}
			}
			return nil
		}
		ok, err = checkPopoverValidity(el, false, doc)
		if err != nil {
			cleanupShowing()
			if throwExceptions {
				return err
			}
			return nil
		}
		if !ok {
			cleanupShowing()
			return nil
		}
	}

	// 栈中原本没有任何 auto/hint popover 时，这个 popover 是「栈底」——只有
	// 栈底才记录 previously focused element 并在关闭时恢复焦点。
	if topmostAutoOrHintPopover(doc) == nil {
		shouldRestoreFocus = true
	}

	switch effectiveType {
	case ModeAuto:
		el.Popover().Mode = string(ModeAuto)
	case ModeHint:
		el.Popover().Mode = string(ModeHint)
	}
	// manual：opened in popover mode 保持 null（规范只在 auto/hint 分支设置），
	// 于是它不出现在两个 list 中——这正是 manual 不参与 light dismiss 的原因。

	el.Popover().PreviouslyFocused = nil
	originallyFocused := doc.FocusedElement()

	// add an element to the top layer
	dp.Stack = append(dp.Stack, el)
	if effectiveType == ModeHint && ancestor != nil && ancestor.Popover().Mode == string(ModeAuto) {
		dp.HintStackParent = ancestor
	}
	el.Popover().Showing = true
	el.Popover().Trigger = source
	invalidate(el) // :popover-open 的匹配结果变了 → 样式/渲染树失效

	runPopoverFocusingSteps(el)
	if shouldRestoreFocus && ModeOf(el) != ModeNone {
		el.Popover().PreviouslyFocused = originallyFocused
	}

	cleanupShowing()
	queuePopoverToggleTask(el, StateClosed, StateOpen, source)
	return nil
}

// Hide 实现规范「hide popover」。
//
//	el                 : 目标元素
//	focusPreviousElement : 关闭后是否把焦点还给打开前聚焦的元素
//	fireEvents         : 是否派发 beforetoggle / 排队 toggle
//	throwExceptions    : 是否把 DOMException 作为 error 返回
//	source             : 触发者（invoker 元素；其余路径为 nil）
func Hide(el *dom.Element, focusPreviousElement, fireEvents, throwExceptions bool, source *dom.Element) error {
	return hideInternal(el, focusPreviousElement, fireEvents, throwExceptions, source, false)
}

// hideInternal 是 Hide 的实现；allowNoPopover 见 checkPopoverValidityEx
// （attribute change steps 路径需要它能关掉「属性已经消失」的元素）。
func hideInternal(el *dom.Element, focusPreviousElement, fireEvents, throwExceptions bool, source *dom.Element, allowNoPopover bool) error {
	if el == nil {
		return nil
	}
	ok, err := checkPopoverValidityEx(el, true, nil, allowNoPopover)
	if err != nil {
		if throwExceptions {
			return err
		}
		return nil
	}
	if !ok {
		return nil
	}
	doc := el.OwnerDocument()
	dp := doc.Popover()
	st := el.Popover()

	nestedHide := st.Hiding
	st.Hiding = true
	if nestedHide {
		// 嵌套 hide（例如元素自己的 beforetoggle 处理器里再次隐藏它）：
		// 只有最外层派发事件。
		fireEvents = false
	}
	dp.HidingCount++
	cleanup := func() {
		if !nestedHide {
			st.Hiding = false
		}
		dp.HidingCount--
	}

	autoPopoverListContainsElement := listContains(showingAutoList(doc), el)
	hintPopoverListContainsElement := listContains(showingHintList(doc), el)

	if st.Mode == string(ModeAuto) || st.Mode == string(ModeHint) {
		if hintPopoverListContainsElement {
			hidePopoverStackUntil(doc, el, ModeHint, focusPreviousElement, fireEvents)
		}
		// 它是 hint 栈的父 auto popover 时，所有 hint popover 一起关闭。
		if dp.HintStackParent == el {
			hidePopoverStackUntil(doc, nil, ModeHint, focusPreviousElement, fireEvents)
		}
		if autoPopoverListContainsElement {
			hidePopoverStackUntil(doc, el, ModeAuto, focusPreviousElement, fireEvents)
		}
		// hide stack 过程里元素可能被断开或属性被改。
		ok, err = checkPopoverValidityEx(el, true, nil, allowNoPopover)
		if err != nil {
			cleanup()
			if throwExceptions {
				return err
			}
			return nil
		}
		if !ok {
			cleanup()
			return nil
		}
	}

	if fireEvents {
		// 关闭方向的 beforetoggle 不可取消（规范：它是通知，不是否决点）。
		fireToggleEvent(el, "beforetoggle", false, StateOpen, StateClosed, source)
		ok, err = checkPopoverValidityEx(el, true, nil, allowNoPopover)
		if err != nil {
			cleanup()
			if throwExceptions {
				return err
			}
			return nil
		}
		if !ok {
			cleanup()
			return nil
		}
	}

	// remove an element from the top layer
	removeFromStack(doc, el)
	st.Trigger = nil
	st.Mode = ""
	st.Showing = false
	if dp.HintStackParent == el || len(showingHintList(doc)) == 0 {
		dp.HintStackParent = nil
	}
	if fireEvents {
		queuePopoverToggleTask(el, StateOpen, StateClosed, source)
	}

	if prev := st.PreviouslyFocused; prev != nil {
		st.PreviouslyFocused = nil
		if focusPreviousElement {
			if focused := doc.FocusedElement(); focused != nil && focused != el && isFlatTreeAncestor(el, focused) {
				focusElement(prev)
			}
		}
	}

	invalidate(el)
	cleanup()
	return nil
}

// Toggle 实现规范 togglePopover(options)：force 为 nil 时翻转，否则按 force
// 显示/隐藏。返回调用后的显示状态。
func Toggle(el *dom.Element, force *bool, source *dom.Element) (bool, error) {
	if el == nil {
		return false, nil
	}
	showing := el.Popover().Showing
	switch {
	case showing && (force == nil || !*force):
		if err := Hide(el, true, true, true, nil); err != nil {
			return showing, err
		}
	case force == nil || *force:
		if err := Show(el, true, source); err != nil {
			return el.Popover().Showing, err
		}
	default:
		// force 与当前状态一致 → 什么都不做，但仍按规范做一次校验（无效调用
		// 要抛 NotSupportedError / InvalidStateError）。
		if _, err := checkPopoverValidity(el, showing, nil); err != nil {
			return showing, err
		}
	}
	return el.Popover().Showing, nil
}

// HidePopoversUntil 实现规范「hide popovers until」：关闭到 endpoint 为止
// （endpoint 自身保持打开，先 hint 后 auto）。
func HidePopoversUntil(doc *dom.Document, endpoint *dom.Element, focusPreviousElement, fireEvents bool) {
	if doc == nil {
		return
	}
	endpointIsHint := listContains(showingHintList(doc), endpoint)
	hidePopoverStackUntil(doc, endpoint, ModeHint, focusPreviousElement, fireEvents)
	autoEndpoint := endpoint
	if endpointIsHint {
		// hint popover 的父 auto popover 要保留：先把 auto 栈关到它为止。
		autoEndpoint = doc.Popover().HintStackParent
	}
	hidePopoverStackUntil(doc, autoEndpoint, ModeAuto, focusPreviousElement, fireEvents)
}

// hidePopoverStackUntil 实现规范「hide popover stack until」：把 auto（或
// hint）栈中 endpoint 之后的 popover 逆序关闭；随后再扫一遍，把「hide 过程
// 里又被脚本打开」的 popover 强制关闭（这一遍不派发事件）。
func hidePopoverStackUntil(doc *dom.Document, endpoint *dom.Element, stackType Mode, focusPreviousElement, fireEvents bool) {
	if doc == nil {
		return
	}
	list := func() []*dom.Element {
		if stackType == ModeAuto {
			return showingAutoList(doc)
		}
		return showingHintList(doc)
	}
	popoverList := list()
	lastHideIndex := 0
	for i, p := range popoverList {
		if endpoint != nil && p == endpoint {
			lastHideIndex = i + 1
		}
	}
	toHide := popoverList[lastHideIndex:]
	for i := len(toHide) - 1; i >= 0; i-- {
		// throwExceptions=false：内部级联关闭遇到非法状态时静默跳过。
		_ = Hide(toHide[i], focusPreviousElement, fireEvents, false, nil)
	}
	toRemain := make(map[*dom.Element]bool, lastHideIndex)
	for _, p := range popoverList[:lastHideIndex] {
		toRemain[p] = true
	}
	newList := list()
	for i := len(newList) - 1; i >= 0; i-- {
		p := newList[i]
		if toRemain[p] {
			continue
		}
		// 第二遍：hide 期间新冒出来的 popover，强制关闭且不派发事件。
		_ = Hide(p, focusPreviousElement, false, false, nil)
	}
}

// listContains 报告列表是否包含元素（按身份）。
func listContains(list []*dom.Element, el *dom.Element) bool {
	if el == nil {
		return false
	}
	for _, p := range list {
		if p == el {
			return true
		}
	}
	return false
}

// removeFromStack 把元素移出 top layer 序列（保持其余元素的相对顺序）。
func removeFromStack(doc *dom.Document, el *dom.Element) {
	st := doc.Popover().Stack
	for i, p := range st {
		if p == el {
			st = append(st[:i], st[i+1:]...)
			break
		}
	}
	doc.Popover().Stack = st
}

// ── Light dismiss（HTML §6.12.2）────────────────────────────────────────────

// topmostClickedPopover 实现规范「find the topmost clicked popover」：在
// 「点击位置的 popover 祖先」与「点击位置所属 invoker 的目标 popover」中取
// 栈位置更高的那个。
func topmostClickedPopover(node *dom.Element) *dom.Element {
	clicked := nearestInclusiveOpenPopover(node)
	target := nearestInclusiveTargetPopover(node)
	if popoverStackPosition(clicked) > popoverStackPosition(target) {
		return clicked
	}
	return target
}

// nearestInclusiveTargetPopover 实现规范「find the nearest inclusive target
// popover」：沿祖先链找「它的 target popover（invoker 的目标）是 auto/hint 且
// 正在显示」的第一个元素。
func nearestInclusiveTargetPopover(node *dom.Element) *dom.Element {
	for cur := node; cur != nil; cur = cur.ParentElement() {
		tp := targetPopoverOf(cur)
		if tp == nil {
			continue
		}
		st := tp.Popover()
		if (st.Mode == string(ModeAuto) || st.Mode == string(ModeHint)) && st.Showing {
			return tp
		}
	}
	return nil
}

// LightDismissPointerDown 处理 light dismiss 的第一阶段：记录「pointerdown
// 时命中的最上层 popover」。只有存在 auto/hint popover 时才记录（manual 不
// 参与）。宿主在真实鼠标按下路径调用，target 为命中的元素，nil 表示点在
// 空白处（此时记忆被清空——规范同样把「点空白」记为 null，抬起时才会关闭）。
func LightDismissPointerDown(doc *dom.Document, target *dom.Element) {
	if doc == nil || topmostAutoOrHintPopover(doc) == nil {
		return
	}
	doc.Popover().PointerdownTarget = topmostClickedPopover(target)
}

// LightDismissPointerUp 处理 light dismiss 的第二阶段：只有当 pointerup 命中
// 的最上层 popover 与 pointerdown 记录的一致时才关闭——这是为了「在 popover
// 内部拖选文本、在 popover 外松开」不会误关。返回是否因此关闭了 popover
// （宿主据此决定要不要立刻重绘）。
func LightDismissPointerUp(doc *dom.Document, target *dom.Element) bool {
	if doc == nil || topmostAutoOrHintPopover(doc) == nil {
		return false
	}
	ancestor := topmostClickedPopover(target)
	sameTarget := ancestor == doc.Popover().PointerdownTarget
	doc.Popover().PointerdownTarget = nil
	if !sameTarget {
		return false
	}
	before := topmostAutoOrHintPopover(doc)
	HidePopoversUntil(doc, ancestor, false, true)
	return topmostAutoOrHintPopover(doc) != before
}

// CloseRequest 处理 close request（本端口的 Esc 语义，规范里由 close watcher
// 承载）：关闭最上层的 auto/hint popover。返回是否消费了该请求（false 表示
// 没有可关闭的 popover，宿主可以让事件继续传递）。
//
// manual popover 不响应 close request（规范如此）。
func CloseRequest(doc *dom.Document) bool {
	if doc == nil {
		return false
	}
	top := topmostAutoOrHintPopover(doc)
	if top == nil {
		return false
	}
	_ = Hide(top, true, true, false, nil)
	return true
}

// AttributeChanged 实现规范的 popover attribute change steps：popover 属性的
// **状态**发生变化且元素正在显示时，关闭它（不抛异常、派发事件）。
//
// oldValue/newValue 是属性值的字符串，oldExisted/newExisted 说明属性当时是否
// 存在（"popover" 的属性值空串与「属性被移除」都对应空串，但状态不同：
// 空串是 auto、不存在是 No Popover）。dom 的属性变更钩子（
// dom.OnElementAttributeChanged）负责转发。
func AttributeChanged(el *dom.Element, oldValue string, oldExisted bool, newValue string, newExisted bool) {
	if el == nil {
		return
	}
	st := el.Popover()
	if !st.Showing {
		return
	}
	if modeFromAttr(oldValue, oldExisted) == modeFromAttr(newValue, newExisted) {
		return
	}
	// allowNoPopover=true：属性此刻已经被改掉（极端情形是属性被移除，元素已
	// 不是 popover 元素），但显示状态必须清干净（见 checkPopoverValidityEx）。
	_ = hideInternal(el, true, true, false, nil, true)
}
