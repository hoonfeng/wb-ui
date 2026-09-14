// Package bindings — <dialog>（HTMLDialogElement）的 JS 接口。
//
// 状态模型：dialog 的打开状态就是 open 属性（反射属性），「模态状态」存在
// dom.Element 上（dom.Element.SetModalState）——因为 CSS 选择器层要消费它
// （:modal 伪类），而 css 不能依赖 html5。show() / showModal() / close() 与
// open 属性设置器共用同一套状态迁移：
//
//	打开 → 设置 open 属性（showModal 再置模态状态）→ 样式失效 → 异步 toggle
//	关闭 → 移除 open 属性 + 清模态状态 → 样式失效 → 异步 toggle + close
//
// 样式失效走 OnClassChanged 桥（webkit 侧清 resolver 缓存 + 重建渲染树），
// 否则 dialog[open] 的 UA 定位与作者写的 :modal / :open 规则不会生效。
//
// 简化：toggle 事件不带 ToggleEvent 的 newState 字段（本引擎没有 ToggleEvent
// 建模）；已打开时 show()/showModal() 按规范应抛 InvalidStateError，这里用
// 原生 TypeError 兜底（与 canvas2d 的 IndexSizeError 处理一致，脚本可 catch）。
package bindings

import (
	"wb-ui/dom"
	"wb-ui/jsc"
)

// dialogIsOpen 报告 dialog 是否已打开（open 属性存在）。
func dialogIsOpen(el *dom.Element) bool { return el.HasAttribute("open") }

// invalidateDialogStyle 让 dialog 的状态变化抵达样式/渲染：失效 computed
// style 缓存并触发渲染树重建（:modal / :open / dialog[open] 都依赖状态）。
func invalidateDialogStyle(el *dom.Element) {
	InvalidateComputedStyle(el)
	if OnClassChanged != nil {
		OnClassChanged(el)
	}
}

// queueDialogToggle 异步派发 toggle 事件（HTML §4.11.4：打开/关闭状态变化后
// 排队派发）。
func queueDialogToggle(in *jsc.Interpreter, el *dom.Element) {
	mediaRunLater(in, func() {
		el.DispatchEvent(dom.NewEvent("toggle", false, false, false))
	})
}

// dialogShow 实现 dialog.show() / dialog.showModal()：已打开时抛 TypeError
// （规范为 InvalidStateError），否则设置 open 属性与模态状态。
func dialogShow(in *jsc.Interpreter, el *dom.Element, method string, modal bool) {
	if dialogIsOpen(el) {
		panic(in.VM().NewTypeError("Failed to execute '" + method + "' on 'HTMLDialogElement': the dialog is already open."))
	}
	el.SetAttribute("open", "")
	el.SetModalState(modal)
	invalidateDialogStyle(el)
	queueDialogToggle(in, el)
}

// dialogClose 实现 dialog.close(returnValue?)：未打开时无操作；打开时清 open
// 属性与模态状态、异步派发 close 与 toggle 事件。
func dialogClose(in *jsc.Interpreter, el *dom.Element, args []jsc.JSValue) {
	if !dialogIsOpen(el) {
		return
	}
	if len(args) > 0 {
		if v := args[0]; !v.IsUndefined() && !v.IsNull() {
			el.SetAttribute("data-returnvalue", v.ToString())
		}
	}
	el.RemoveAttribute("open")
	el.SetModalState(false)
	invalidateDialogStyle(el)
	// close 与 toggle 在同一批任务里按序派发（规范：先 close，再 toggle）：
	// 分成两个定时器任务时顺序不受保证。
	mediaRunLater(in, func() {
		el.DispatchEvent(dom.NewEvent("close", false, false, false))
		el.DispatchEvent(dom.NewEvent("toggle", false, false, false))
	})
}
