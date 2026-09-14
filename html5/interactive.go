package html5

import "wb-ui/dom"

// ─── 交互校验（interactive validation）────────────────────────────────
//
// HTML §4.10.21.2「Interactively validate the constraints」与 checkValidity()
// 的区别在于「报告问题」：对每个无效控件派发 invalid 事件，由 UA 聚焦第一个
// 无效控件并显示提示。表单提交前走这条路径（除非表单有 novalidate 属性，
// 或提交者声明了 formnovalidate）。
//
// invalid 事件：不冒泡、cancelable 初始为 true（但取消它不影响校验结果——
// 规范原文 "though canceling has no effect"）。事件是同步派发的（报告发生在
// 校验返回之前，页面处理器能在 submit 之前改好输入）。

// fireInvalidEvent 在元素上派发 invalid 事件。
func fireInvalidEvent(el *dom.Element) {
	el.DispatchEvent(dom.NewEvent("invalid", false, true, false))
}

// InteractiveValidity 对元素做交互校验：
//   - (true, true)：元素参与约束校验且通过（或元素被 barred——barred 元素
//     不参与校验，checkValidity 恒为 true）；
//   - (false, true)：校验失败，并在元素上派发了 invalid 事件；
//   - (true, false)：元素不参与约束校验（普通元素、output/fieldset 等）。
func InteractiveValidity(el *dom.Element) (valid bool, ok bool) {
	st, isControl := ConstraintValidity(el)
	if !isControl {
		return true, false
	}
	if !st.WillValidate || st.Valid {
		return true, true
	}
	fireInvalidEvent(el)
	return false, true
}

// ValidateInteractively 实现表单级交互校验：按 tree order 对每个无效控件派发
// invalid 事件，返回无效控件列表（空 = 全部通过）。聚焦第一个无效控件属于
// UA 行为，由宿主在拿到列表后完成（bindings 层会调用 FocusBridge）。
//
// 注意：checkValidity() 不派发 invalid 事件，本方法与它是两条路径。
func (f HTMLFormElement) ValidateInteractively() []*dom.Element {
	var invalid []*dom.Element
	for _, el := range f.Elements() {
		if valid, ok := InteractiveValidity(el); ok && !valid {
			invalid = append(invalid, el)
		}
	}
	return invalid
}

// submitterSkipsValidation 报告提交者是否声明了 formnovalidate（点击该按钮
// 提交时跳过约束校验）。
func submitterSkipsValidation(submitter *dom.Element) bool {
	return submitter != nil && submitter.HasAttribute("formnovalidate")
}
