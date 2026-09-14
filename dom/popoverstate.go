// Popover 子系统的状态存储（HTML §6.12 The popover attribute）。
//
// 规范把 popover 的状态分成「每元素槽」与「每文档槽」两组；本引擎把两者分别
// 放在 dom.Element / dom.Document 上（与 modal 状态、indeterminate 同一模式），
// 而把**算法**放在独立包 wb-ui/popover —— 这样：
//   - CSS 选择器层（css）能直接读元素的可见状态判 `:popover-open`，无需依赖
//     popover 包（css 只依赖 dom）；
//   - 渲染/宿主能读同样的状态决定是否绘制、是否参与命中测试；
//   - dom 包保持「只存状态、不含 HTML 算法」的定位（算法的唯一实现在
//     wb-ui/popover，dom 本身从不改变这些字段）。
//
// 与 <dialog> 的模态状态同理：状态落在这里，是因为它横跨 css / rendering /
//
//	宿主三层，而不是因为它属于 DOM 规范。
package dom

// PopoverState 是元素上的 popover 状态（规范 §6.12 的每元素槽）。
//
// 字段含义与规范槽一一对应，命名以可读性优先；所有字段由 wb-ui/popover 包的
// show/hide 算法维护，dom 包自身不修改它们。
type PopoverState struct {
	// Showing 是 HTML 的 popover visibility state：true = showing 状态、
	// false = hidden 状态（初值）。它是 `:popover-open` 伪类的唯一判据，也是
	// UA 样式表 `[popover]:not(:popover-open){display:none}` 的依据。
	Showing bool

	// Mode 是 opened in popover mode："" （规范里的 null）/ "auto" / "hint"。
	//
	// ★ 注意 manual 状态的 popover 显示时这里仍是 ""：规范的 show popover
	// 算法只在 effectiveType 为 Auto 或 Hint 时设置该槽，于是 manual popover
	// 既不出现在 showing auto/hint popover list 中（因此不参与 light dismiss、
	// 不被其他 popover 的打开所关闭），也不参与 topmost popover ancestor 计算。
	Mode string

	// Trigger 是 popover trigger（打开该 popover 的 invoker 元素，规范 §6.12），
	// 关闭时置空。show/hide 算法把它当作 ToggleEvent.source 的来源之一。
	Trigger *Element

	// PreviouslyFocused 是打开 popover 时记住的「原本聚焦的元素」，关闭时按
	// hide 算法的 focusPreviousElement 参数决定是否把焦点还给它（规范：只有
	// 栈中第一个 popover 关闭时才恢复）。
	PreviouslyFocused *Element

	// Hiding 是 popover hiding 标记：hide 算法在运行时置位，嵌套 hide（例如
	// 元素自身的 beforetoggle 处理器里再次调用 hidePopover）为 true 时不再
	// 派发事件（规范 §6.12 的 nestedHide 语义）。
	Hiding bool

	// TogglePending / ToggleOldState 是 popover toggle task tracker 的简化：
	// 已排队但尚未派发的 `toggle` 任务。规范要求同一元素只保留一个待派发的
	// toggle 任务——新的状态迁移到来时复用旧任务的 oldState 并重排任务，
	// 因此在同一批任务内连续 open→closed 只会观察到一次 toggle（oldState 取
	// 第一次迁移前的值）。
	TogglePending  bool
	ToggleOldState string

	// ToggleSeq 是 toggle 任务的序号（每次排队递增）。本端口的任务队列
	// （宿主 setTimeout）无法取消已排队的任务，改为让过期任务在运行时比对
	// 序号后自行退出——语义与规范的「移除旧任务」等价。
	ToggleSeq uint64
}

// Popover 返回元素的 popover 状态（可写指针）。调用方为 wb-ui/popover 包的
// 算法实现；只读消费者（css / rendering / 宿主）读具体字段即可。
func (e *Element) Popover() *PopoverState {
	if e == nil {
		return nil
	}
	return &e.popover
}

// IsPopoverOpen 报告元素是否处于 popover 的 showing 状态（规范里「popover
// visibility state is showing」），即 `:popover-open` 的匹配条件。
func (e *Element) IsPopoverOpen() bool {
	return e != nil && e.popover.Showing
}

// HasPopoverAttribute 报告元素是否带 popover 内容属性（即是否是一个 popover
// 元素）。注意与 IsPopoverOpen 的区别：带属性但未显示的元素匹配
// `[popover]`、不匹配 `:popover-open`。
func (e *Element) HasPopoverAttribute() bool {
	return e != nil && e.HasAttribute("popover")
}

// NeedsBackdrop 报告元素是否应生成 ::backdrop 伪元素盒/渲染对象。
//
// HTML 渲染规范里 ::backdrop 属于 top layer 中的元素：模态 <dialog>、显示中的
// popover（以及全屏元素——本引擎的全屏没有 top layer 化，故未纳入）。本引擎
// 由 layout/rendering 的两条路径（块级与 flex/grid）共用本判定生成遮罩盒。
// 样式仍然由选择器决定：模态 dialog 命中通用 `::backdrop` 规则（半透明黑），
// popover 命中更具体的 `:popover-open::backdrop`（透明 + 不吃指针事件）。
func (e *Element) NeedsBackdrop() bool {
	return e.IsModalDialog() || e.IsPopoverOpen()
}

// PopoverDocumentState 是文档上的 popover 状态（规范 §6.12 的每文档槽）。
type PopoverDocumentState struct {
	// Stack 是本端口的「top layer 顺序」：显示中的 popover 按加入先后排列
	// （先加入的在前面）。规范的 showing auto popover list / showing hint
	// popover list 都是从 top layer 过滤出来的，这里用 Stack + 元素的 Mode
	// 过滤等价得到；popover stack position（light dismiss 的比较依据）也由它
	// 计算。
	//
	// manual 状态的 popover 同样在 Stack 里（它在 top layer 中、绘制在其他
	// 内容之上），只是 Mode 为 "" 因而被两个 list 过滤掉。
	Stack []*Element

	// HintStackParent 是规范的同名槽：showing hint popover list 的第一项所属
	// 的那个 auto popover（null 表示它是独立的）。light dismiss 会利用它让
	// hint 与它的父 auto popover 一起关闭。
	HintStackParent *Element

	// PointerdownTarget 是 light dismiss 的 pointerdown 记忆（规范同名槽）：
	// pointerdown 时记下「最上层的被点击 popover」，pointerup 时若命中同一个
	// 才真正关闭——这样在 popover 内部拖动选择文本不会误关。
	PointerdownTarget *Element

	// Showing 是「正在显示某个 popover」的重入标记（规范 §6.12 第一步用它
	// 防止 popover 在自己的显示/隐藏过程中再打开另一个）。HidingCount 是同章
	// 的 hiding popover nesting count。
	Showing     bool
	HidingCount int
}

// Popover 返回文档的 popover 状态（可写指针，见 PopoverDocumentState）。
func (d *Document) Popover() *PopoverDocumentState {
	if d == nil {
		return nil
	}
	return &d.popover
}
