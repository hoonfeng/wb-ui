// Package bindings — <dialog>（HTMLDialogElement）的 JS 接口。
//
// 状态模型（严格按 HTML §4.11.6 的算法）：dialog 的打开状态就是 open 属性，
// 而「模态状态」是独立状态（is modal），存放在 dom.Element 上
// （dom.Element.SetModalState）——因为 CSS 选择器层要消费它（:modal），而
// css 不能依赖 html5。
//
// 关键语义（规范如此，容易写错的地方都注明了）：
//   - open 是**纯反射属性**：设置/移除属性本身不改变模态状态、不派发事件。
//     移除 open 属性只让 dialog 隐藏（UA 规则 dialog:not([open]){display:none}），
//     由 showModal() 打开的对话框仍处于模态（文档仍被阻塞、:modal 仍匹配、
//     ::backdrop 仍在）——规范因此建议作者用 close() 而不是移除属性。
//   - show()：已打开且**已是**非模态 → 静默返回；已打开且为模态 → 抛错。
//     showModal()：已打开且已是模态 → 静默返回；已打开且为非模态 → 抛错。
//   - 打开/关闭都先同步派发可取消的 beforetoggle，再排队异步 toggle；
//     关闭时还额外排队 close 事件（同一批任务内 toggle 先、close 后）。
//
// toggle / beforetoggle 用 ToggleEvent 派发（带 oldState / newState / source）——
// 页面常在一个处理器里按 `e.newState` 区分「正在打开」与「正在关闭」。
//
// source（谁触发了切换）在本端口恒为 null，这是规范行为而非简化：dialog 的
// 每一条打开/关闭路径都传 null——close() / requestClose() 调「close the dialog
// with <result> and null」（HTML §4.11.6），form method=dialog 提交调「close the
// dialog subject with <result> and null」（form submission algorithm），close
// watcher 读 dialog 的 request close source element 槽（只有 requestClose()
// 写过它，写的也是 null），show()/showModal() 没有调用者建模；<details> 的
// details toggle 任务只初始化 oldState/newState。规范里 source 非 null 的场景
// 是 popover 的 invoker（popovertarget / command 元素）——本端口尚无 Popover API
// （见 docs/TECH_DEBT.md）。字段仍然实现，是为了 `e.source === null` 与 MDN 的
// `event.source === undefined` 特性检测表现得和浏览器一致。
//
// 其余简化：已打开时的状态冲突按规范应抛 InvalidStateError，这里用原生
// TypeError 兜底（与 canvas2d 的 IndexSizeError 处理一致，脚本可 catch）。
package bindings

import (
	"wb-ui/engine/dom"
	"wb-ui/engine/js/jsc"
)

// dialogIsOpen 报告 dialog 是否已打开（open 属性存在）。
func dialogIsOpen(el *dom.Element) bool { return el.HasAttribute("open") }

// invalidateStateStyle 让元素的状态变化抵达样式/渲染：失效 computed style
// 缓存并触发渲染树重建。所有「属性没变但选择器匹配结果变了」的状态迁移都走
// 这里（<dialog> 的模态/打开状态、<details> 的 open、input 的 indeterminate），
// 否则 :modal / :open / :indeterminate / dialog[open] / ::backdrop 会停留在
// 上一次的样式。
func invalidateStateStyle(el *dom.Element) {
	InvalidateComputedStyle(el)
	if OnClassChanged != nil {
		OnClassChanged(el)
	}
}

// fireBeforeToggle 同步派发 beforetoggle（cancelable，带状态），返回是否未被
// 取消（规范：打开/关闭算法在第一步就让它有机会否决）。
func fireBeforeToggle(el *dom.Element, oldState, newState string, source *dom.Element) bool {
	ev := dom.NewToggleEvent("beforetoggle", false, true, oldState, newState, source)
	el.DispatchEvent(ev)
	return !ev.DefaultPrevented()
}

// dialogShow 实现 dialog.show()（modal=false）与 dialog.showModal()（modal=true）。
func dialogShow(in *jsc.Interpreter, el *dom.Element, method string, modal bool) {
	open := dialogIsOpen(el)
	if open && el.ModalState() == modal {
		return // 规范：已经是目标状态 → 静默返回（不抛错、不派发事件）
	}
	if open {
		panic(in.VM().NewTypeError("Failed to execute '" + method +
			"' on 'HTMLDialogElement': the dialog is already open with a different modality."))
	}
	if !fireBeforeToggle(el, dom.ToggleStateClosed, dom.ToggleStateOpen, nil) {
		return
	}
	if dialogIsOpen(el) {
		return
	}
	queueDialogToggle(in, el, dom.ToggleStateClosed, dom.ToggleStateOpen, nil)
	el.SetAttribute("open", "")
	el.SetModalState(modal)
	invalidateStateStyle(el)
}

// dialogClose 实现 dialog.close(returnValue?)：未打开时无操作；否则按规范顺序
// 派发 beforetoggle（可取消）→ 排队 toggle + close → 移除 open → 清模态状态。
//
// 注意「未打开时无操作」是规范第一步：若页面直接 removeAttribute("open") 造成
// 模态状态残留（见文件头注释），close() 会因缺 open 属性而直接返回、清不掉残留
// ——规范确实如此（残留只能靠元素移除步骤清理），所以 close() 里不做额外兜底。
func dialogClose(in *jsc.Interpreter, el *dom.Element, args []jsc.JSValue) {
	if !dialogIsOpen(el) {
		return
	}
	if !fireBeforeToggle(el, dom.ToggleStateOpen, dom.ToggleStateClosed, nil) {
		return
	}
	if !dialogIsOpen(el) {
		return
	}
	// toggle 与 close 在同一批任务里按序派发（规范：先 toggle 再 close）。
	// 分成两个定时器任务时顺序不受保证，实测会被颠倒。
	mediaRunLater(in, func() {
		el.DispatchEvent(dom.NewToggleEvent("toggle", false, false,
			dom.ToggleStateOpen, dom.ToggleStateClosed, nil))
		el.DispatchEvent(dom.NewEvent("close", false, false, false))
	})
	el.RemoveAttribute("open")
	el.SetModalState(false)
	if len(args) > 0 {
		if v := args[0]; !v.IsUndefined() && !v.IsNull() {
			el.SetAttribute("data-returnvalue", v.ToString())
		}
	}
	invalidateStateStyle(el)
}

// queueDialogToggle 异步派发 toggle 事件（HTML §4.11.6 / §4.11.4：状态变化后
// 排队派发）。oldState / newState 描述这次迁移的方向；source 见文件头注释
// （本端口的所有调用点都传 nil）。
func queueDialogToggle(in *jsc.Interpreter, el *dom.Element, oldState, newState string, source *dom.Element) {
	mediaRunLater(in, func() {
		el.DispatchEvent(dom.NewToggleEvent("toggle", false, false, oldState, newState, source))
	})
}
