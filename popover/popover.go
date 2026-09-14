// Package popover 实现 HTML 的 Popover API（HTML Standard §6.12 The popover
// attribute）：popover 属性的状态机、showPopover() / hidePopover() /
// togglePopover() 三个算法、top layer 顺序、light dismiss（点击外部与 Esc
// 关闭请求）、popover 之间的父子关系，以及 popovertarget / commandfor
// 这类 invoker 的激活行为。
//
// 分层位置（为什么是一个独立包）：
//   - dom 包存状态（dom.PopoverState / dom.PopoverDocumentState，见
//     dom/popoverstate.go），不含算法——因为 CSS 选择器层要读可见状态判
//     `:popover-open`，而 css 不能依赖本包；
//   - 本包只依赖 dom，实现规范的全部算法并暴露给 JS 层（bindings）与宿主
//     （app / webkit）；
//   - 宿主相关的副作用（异步任务队列、聚焦、样式失效）通过下面的注入钩子
//     接入，本包不依赖 jsc / rendering / style，因此可以被 dom 级单测直接驱动。
//
// 与浏览器的已知差异（本端口没有 top layer，见下）集中记在
// docs/TECH_DEBT.md 的 popover 小节。
//
// 本端口的简化（都有注释标注）：
//   - 没有真正的 top layer：显示中的 popover 由 UA 样式表给予 position:fixed
//     与高 z-index（html5/defaultcss.go），绘制顺序与命中测试沿用既有的 fixed
//     定位语义。因此「后显示的 popover 一定在其他所有内容之上」只在 z-index
//     层面近似成立。
//   - 没有 close watcher 机制：close request（Esc）由宿主直接调用
//     CloseRequest，只作用在最上层的 auto/hint popover 上。
//   - 没有 implicit anchor element / CSS anchor positioning：invoker 与
//     popover 的锚点关联不落地（只记录 popover trigger 供事件 source 使用）。
//   - popover 元素从文档移除时的清理（规范 removal steps）未实现，改为
//     在列表构建时过滤已断开的元素（与 <dialog> 的模态状态同一取舍）。
package popover

import (
	"strings"

	"wb-ui/dom"
)

// Mode 是 popover 内容属性的状态（HTML §6.12 的枚举属性）。空值（ModeNone）
// 对应规范的 No Popover state。
type Mode string

const (
	// ModeNone 是 No Popover state：元素没有 popover 属性（缺省值默认）。
	// 此时元素不是 popover：showPopover() 抛 NotSupportedError。
	ModeNone Mode = ""

	// ModeAuto 是 Auto 状态：打开时关闭其他 auto popover、响应 light dismiss
	// 与 close request（空值默认：popover="" 也是 auto）。
	ModeAuto Mode = "auto"

	// ModeManual 是 Manual 状态：不关闭其他 popover、不响应 light dismiss 与
	// close request（无效值默认：popover="foo" 落到这里）。
	ModeManual Mode = "manual"

	// ModeHint 是 Hint 状态：打开时关闭其他 hint popover（但不关 auto），
	// 响应 light dismiss 与 close request。
	ModeHint Mode = "hint"
)

// ToggleEvent 的两个状态字符串（规范：ToggleEvent.oldState/newState 取
// "open" / "closed"）。与 dom.ToggleStateOpen / dom.ToggleStateClosed 同值，
// 这里再导出一次是为了让 popover 的实现不必反复引用 dom 的常量。
const (
	StateOpen   = dom.ToggleStateOpen
	StateClosed = dom.ToggleStateClosed
)

// ModeOf 解析元素的 popover 内容属性状态（HTML §6.12 的枚举属性规则）：
//
//	缺省值默认（无属性）     → ModeNone
//	空值默认（popover=""）   → ModeAuto
//	invalid value default    → ModeManual（未知值一律按 manual 处理）
//	auto / manual / hint     → 对应状态（ASCII 大小写不敏感）
func ModeOf(el *dom.Element) Mode {
	if el == nil {
		return ModeNone
	}
	return modeFromAttr(el.GetAttribute("popover"), el.HasAttribute("popover"))
}

// lowerTrim 归一化属性值（枚举属性关键字是 ASCII 大小写不敏感）。
func lowerTrim(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// IsShowing 报告元素是否处于 popover 的 showing 状态（`:popover-open` 的判据，
// 委托 dom.Element.IsPopoverOpen）。
func IsShowing(el *dom.Element) bool { return el.IsPopoverOpen() }

// ── 宿主注入点 ──────────────────────────────────────────────────────────────
//
// popover 的算法有三个副作用无法在 dom 层完成：异步派发 toggle 事件（需要
// JS 事件循环）、聚焦元素（需要宿主的焦点/IME 状态）、样式失效（需要样式解析
// 器与渲染树）。本包因此暴露三个包级钩子，由 bindings / app / webkit 在初始化
// 时注入；未注入时退化为「同步执行 / 只改 dom 状态 / 不做失效」，使 dom 级
// 单测与无 JS 引擎的宿主仍能工作。

// QueueTask 排队一个「元素任务」（规范：queue an element task on the DOM
// manipulation task source）。popover 的 toggle 事件必须异步派发——同步派发
// 会让「先 showPopover() 再读状态」的脚本看到错误的顺序，也会让 beforetoggle
// 期间的状态迁移与浏览器不同。
//
// bindings 注入的实现走 jsc 事件循环（setTimeout 0，与 <dialog> 的
// mediaRunLater 同一机制）；webkit/app 注入各自的事件循环。未注入时同步执行。
var QueueTask func(el *dom.Element, fn func())

// FocusElement 把焦点移到元素上（宿主的焦点能力：app.Host.FocusElement /
// webkit.FormFocus）。popover focusing steps 与关闭时的焦点恢复用它。
// 未注入时只更新 dom 层的焦点标记（:focus 与 document.activeElement 生效，
// 但不驱动 IME/宿主状态）。
var FocusElement func(el *dom.Element)

// InvalidateStyle 让元素的状态变化抵达样式与渲染（失效 computed style 缓存 +
// 重建渲染树），对应 bindings.InvalidateComputedStyle + OnClassChanged 那条
// 链路。popover 的显示/隐藏改变 `:popover-open` 的匹配结果，因此每次状态迁移
// 都必须调用它。未注入时无操作。
var InvalidateStyle func(el *dom.Element)

// runTask 执行排队任务（未注入钩子时同步执行）。
func runTask(el *dom.Element, fn func()) {
	if QueueTask != nil {
		QueueTask(el, fn)
		return
	}
	fn()
}

// focusElement 移动焦点（未注入钩子时只标记 dom 状态）。
func focusElement(el *dom.Element) {
	if el == nil {
		return
	}
	if FocusElement != nil {
		FocusElement(el)
		return
	}
	if prev := el.OwnerDocument().FocusedElement(); prev != nil && prev != el {
		prev.SetFocused(false)
	}
	el.SetFocused(true)
}

// invalidate 触发样式失效（未注入钩子时无操作）。
func invalidate(el *dom.Element) {
	if InvalidateStyle != nil {
		InvalidateStyle(el)
	}
}

// ── 列表助手（规范 §6.12 的 showing auto/hint popover list）─────────────────

// liveStack 返回文档 top layer 中仍然连接在文档里的 popover 序列。规范用
// removal steps 在元素离开文档时清理 top layer；本端口未实现移除步骤，改为
// 在此过滤掉已断开的元素（否则 light dismiss 会对着幽灵元素做判断）。
func liveStack(doc *dom.Document) []*dom.Element {
	if doc == nil {
		return nil
	}
	st := doc.Popover().Stack
	if len(st) == 0 {
		return nil
	}
	out := make([]*dom.Element, 0, len(st))
	for _, el := range st {
		if el != nil && el.IsConnected() {
			out = append(out, el)
		}
	}
	return out
}

// showingAutoList 对应规范「get the showing auto popover list」：top layer 中
// opened in popover mode 为 "auto" 且处于 showing 状态的元素，按 top layer
// 顺序（先加入在前）。
func showingAutoList(doc *dom.Document) []*dom.Element {
	var out []*dom.Element
	for _, el := range liveStack(doc) {
		if st := el.Popover(); st.Mode == string(ModeAuto) && st.Showing {
			out = append(out, el)
		}
	}
	return out
}

// showingHintList 对应规范「get the showing hint popover list」（hint 版）。
func showingHintList(doc *dom.Document) []*dom.Element {
	var out []*dom.Element
	for _, el := range liveStack(doc) {
		if st := el.Popover(); st.Mode == string(ModeHint) && st.Showing {
			out = append(out, el)
		}
	}
	return out
}

// topmostAutoOrHintPopover 对应规范「find the topmost auto or hint popover」：
// hint 列表最后一项优先（hint 恒在 auto 之上），否则 auto 列表最后一项。
func topmostAutoOrHintPopover(doc *dom.Document) *dom.Element {
	if hl := showingHintList(doc); len(hl) > 0 {
		return hl[len(hl)-1]
	}
	if al := showingAutoList(doc); len(al) > 0 {
		return al[len(al)-1]
	}
	return nil
}

// nearestInclusiveOpenPopover 对应规范「find the nearest inclusive open
// popover」：从 node 起沿祖先链（含自身）找第一个 opened in popover mode 为
// auto/hint 且 showing 的元素。
func nearestInclusiveOpenPopover(node *dom.Element) *dom.Element {
	for cur := node; cur != nil; cur = cur.ParentElement() {
		if st := cur.Popover(); st.Showing && (st.Mode == string(ModeAuto) || st.Mode == string(ModeHint)) {
			return cur
		}
	}
	return nil
}

// popoverStackPosition 对应规范「get the popover stack position」：hint 列表中
// 的位置排在 auto 列表之后（即 hint 恒在 auto 之上），不在两个列表中的返回 0。
func popoverStackPosition(el *dom.Element) int {
	if el == nil {
		return 0
	}
	doc := el.OwnerDocument()
	al := showingAutoList(doc)
	if hl := showingHintList(doc); len(hl) > 0 {
		for i, p := range hl {
			if p == el {
				return i + len(al) + 1
			}
		}
	}
	for i, p := range al {
		if p == el {
			return i + 1
		}
	}
	return 0
}

// isFlatTreeAncestor 报告 outer 是否是 inner 的祖先（含自身）。本端口没有
// slot 投影参与 popover 判断（shadow 树里的 popover 极少见），flat tree 与
// 普通树一致，直接用祖先链判定。
func isFlatTreeAncestor(outer, inner *dom.Element) bool {
	for cur := inner; cur != nil; cur = cur.ParentElement() {
		if cur == outer {
			return true
		}
	}
	return false
}

// isStrictDescendant 报告 inner 是否是 outer 的**严格**后代（不含自身）。
// invoker 的自引用保护要用严格后代语义：`<button popover popovertarget=自己>`
// 点击应当切换它自己（WPT popover-self-invoke），而「popover 嵌在按钮内部」
// 时点击 popover 内部才需要被忽略。
func isStrictDescendant(outer, inner *dom.Element) bool {
	if outer == nil || inner == nil || outer == inner {
		return false
	}
	for cur := inner.ParentElement(); cur != nil; cur = cur.ParentElement() {
		if cur == outer {
			return true
		}
	}
	return false
}

// topmostPopoverAncestor 对应规范「find the topmost popover ancestor」。
//
// 它决定「打开这个 popover 时要先关闭哪些 popover」：combinedPopovers 是
// auto 列表拼接 hint 列表（auto 在前），取「最后一个包含 newPopover（作为
// 后代）或 source（作为后代）的元素」——即严格早于新 popover 的最深「父
// popover」。返回 null 表示没有父级（打开时会关掉栈里其他所有 auto）。
//
// isPopover=false 的形态（topLayerElement）在本端口只用于 dialog 场景，
// 当前调用点只有 isPopover=true，保留参数以对应规范签名。
func topmostPopoverAncestor(newPopover *dom.Element, source *dom.Element, isPopover bool) *dom.Element {
	if newPopover == nil {
		return nil
	}
	doc := newPopover.OwnerDocument()
	combined := append(showingAutoList(doc), showingHintList(doc)...)
	// isPopover 在规范里只对应三条断言（调用方保证 newPopover 是 popover 元素、
	// 属性状态不是 None/Manual、尚未显示；非 popover 形态的 top layer 元素则
	// 要求 source 为 null）。两种情况下的「组合列表 + 索引取最大」完全相同，
	// 因此这里不分支——参数保留是为了调用点与规范一一对应。
	_ = isPopover
	popoverAncestorIndex := -1
	for i, p := range combined {
		if isFlatTreeAncestor(p, newPopover) {
			popoverAncestorIndex = i
		}
	}
	sourceAncestorIndex := -1
	if source != nil {
		for i, p := range combined {
			if isFlatTreeAncestor(p, source) {
				sourceAncestorIndex = i
			}
		}
	}
	ancestorIndex := popoverAncestorIndex
	if sourceAncestorIndex > ancestorIndex {
		ancestorIndex = sourceAncestorIndex
	}
	if ancestorIndex == -1 {
		return nil
	}
	return combined[ancestorIndex]
}
