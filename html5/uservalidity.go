package html5

import "wb-ui/dom"

// ─── user validity 的「焦点会话」记忆（HTML §4.10.18.1 / MDN :user-valid）───
//
// :user-valid / :user-invalid 的匹配要求控件的 user validity 为 true——即
// 「用户交互过」。MDN 把「交互过」定义为下列任一情况（:user-valid 页原文，
// :user-invalid 为镜像规则）：
//
//  1. 用户改变了控件的值并提交了该改变（例如把焦点移到别处）——引擎在真实
//     change 路径上调用 bindings.MarkUserInteracted（bindings/validity.go）；
//  2. 用户尝试提交了表单（即使没有改动控件）——交互校验
//     （InteractiveValidity）里置位；
//  3. 值在控件获得焦点时是无效的，而用户在焦点仍在控件内时把它改成了有效
//     （:user-invalid 镜像：聚焦时有效 → 用户改成无效）——需要本文件的
//     「焦点会话记忆」才能判定。
//
// 第 3 条同时避免了两种糟糕表现：页面一开始就把所有必填项标红，以及用户
// 正在纠正输入时样式反复闪烁。只有当有效性相对「获得焦点时」发生了翻转，
// 才认为用户可以看见反馈了。
//
// 规范另有一句「一旦该伪类生效，控件聚焦时每次击键都重新校验」——本端口的
// 值写入本身会改变属性（input 的 value）或文本内容（textarea），attrVersion
// 随之递增使 per-element 样式缓存失效，因此击键后的重算不需要额外机制；
// user validity 自身的翻转（无属性变化）才需要显式失效，由
// bindings.MarkUserInteracted / html5.markUserInteracted 负责。
//
// 引擎接线（两条宿主路径都要覆盖，漏掉任一条这一规则都会静默失效）：
//   - 焦点：dom 层 SetFocused(true) 调 dom.OnElementFocused（本包 init 注入
//     NoteFocusGained）；SetFocused(false) 清除记忆。
//   - 值写入：app.Host.setFocusedElementValue（IME/组合输入）、
//     webkit.FormFocus.applyValue（键盘键入/退格/删除/粘贴）与 range 的点击
//     拖动（app.Host.setRangeValueFromX）在写完值后调 NoteUserInput。

// NoteFocusGained 记录控件「获得焦点时」的约束校验结果。只有候选
// （willValidate）控件需要记忆——被排除在约束校验之外（barred）的元素既不
// 匹配 :user-valid 也不匹配 :user-invalid，记了也用不上。
func NoteFocusGained(el *dom.Element) {
	if el == nil {
		return
	}
	st, ok := ConstraintValidity(el)
	if !ok || !st.WillValidate {
		// 清掉可能残留的记忆：元素在焦点会话内从 barred 变为候选时，不应
		// 拿上一次的结果做判断。
		el.ClearFocusValidity()
		return
	}
	el.SetFocusValidity(st.Valid)
}

// NoteUserInput 记录「用户在焦点会话内修改了控件的值」：若这次修改让有效性
// 相对「获得焦点时」发生了翻转，控件就此获得 user validity（MDN 第 3 条）。
// 值写入路径在写完值之后调用它。
//
// 只对「已建立焦点会话记忆」的控件生效：没有焦点会话时（脚本赋值、程序化
// 写值）不置位——与浏览器一致，脚本改变值不构成用户交互。
func NoteUserInput(el *dom.Element) {
	if el == nil || el.UserInteracted() {
		return
	}
	validAtFocus, known := el.FocusValidity()
	if !known {
		return
	}
	st, ok := ConstraintValidity(el)
	if !ok || !st.WillValidate {
		return
	}
	if st.Valid != validAtFocus {
		markUserInteracted(el)
	}
}

// init 把 dom 的「元素获得焦点」回调接到本包（见文件头「引擎接线」）。
func init() {
	dom.OnElementFocused = NoteFocusGained
}
