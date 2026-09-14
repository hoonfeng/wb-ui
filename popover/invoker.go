// Popover 的 invoker 支持：popovertarget / popovertargetaction（HTML §6.12.1）
// 与 button 的 commandfor / command 属性（HTML「button 元素的激活行为」）。
//
// 这两条路径的差别（规范原文）：
//   - popovertarget 是「按钮 → popover」的专用属性，激活行为是「run the popover
//     target attribute activation behavior」，它按 popovertargetaction（toggle/
//     show/hide）显示或隐藏目标；
//   - commandfor + command 是通用命令机制，command 取 toggle-popover /
//     show-popover / hide-popover 时对目标 popover 做同样的三件事，并且会先派发
//     一个可取消的 `command` 事件；
//   - button 的激活行为里 commandfor 优先（见「get the target popover」），
//     commandfor 缺失或命令对目标无效时回退到 popovertarget 路径。
//
// 本端口的已知缺口：command 事件（CommandEvent，带 command / source 字段）
// 未派发——本引擎还没有 CommandEvent 接口，派发一个无字段的同名事件比不派发
// 更容易误导作者，因此留白并记录在 docs/TECH_DEBT.md。
package popover

import (
	"strings"

	"wb-ui/dom"
)

// TargetAction 是 popovertargetaction 的状态（枚举属性）。
type TargetAction string

const (
	// ActionToggle 是 Toggle 状态（缺省值默认与无效值默认都是它）。
	ActionToggle TargetAction = "toggle"
	// ActionShow 是 Show 状态。
	ActionShow TargetAction = "show"
	// ActionHide 是 Hide 状态。
	ActionHide TargetAction = "hide"
)

// command 属性的状态（只列出与 popover 有关的三项；其余状态
// close / request-close / show-modal 属于 <dialog>，本端口未实现对应命令步骤）。
const (
	commandTogglePopover = "toggle-popover"
	commandShowPopover   = "show-popover"
	commandHidePopover   = "hide-popover"
)

// TargetActionOf 解析 popovertargetaction（HTML §6.12.1：缺省值默认与无效值
// 默认都是 Toggle 状态）。
func TargetActionOf(node *dom.Element) TargetAction {
	if node == nil {
		return ActionToggle
	}
	switch lowerTrim(node.GetAttribute("popovertargetaction")) {
	case "show":
		return ActionShow
	case "hide":
		return ActionHide
	}
	return ActionToggle
}

// isPopoverInvoker 报告元素是否可以是 popover 的 invoker（button 元素，或
// type="button" 的 input 元素）。
func isPopoverInvoker(node *dom.Element) bool {
	if node == nil {
		return false
	}
	switch node.LocalName() {
	case "button":
		return true
	case "input":
		// input 只有 Button 状态（type="button"）可以当命令按钮；submit/reset/
		// image 属于表单提交语义，popover 的 invoker 判定明确排除它们。
		return lowerTrim(node.GetAttribute("type")) == "button"
	}
	return false
}

// hasFormOwner 报告元素是否有关联的表单（form 属性指向的 form，或 form 祖先）。
func hasFormOwner(node *dom.Element) bool {
	return formOwnerOf(node) != nil
}

// formOwnerOf 返回元素的表单所有者（无则 nil）：form 属性指向的 form 元素优先，
// 否则生成的 form 祖先。
func formOwnerOf(node *dom.Element) *dom.Element {
	if node == nil {
		return nil
	}
	if id := strings.TrimSpace(node.GetAttribute("form")); id != "" {
		if doc := node.OwnerDocument(); doc != nil {
			if f := doc.GetElementById(id); f != nil && f.LocalName() == "form" {
				return f
			}
		}
	}
	for p := node.ParentElement(); p != nil; p = p.ParentElement() {
		if p.LocalName() == "form" {
			return p
		}
	}
	return nil
}

// isSubmitButton 报告元素是否是提交按钮。规范（form-elements §button）：
//
//	<button>：type 处于 Submit Button 状态；或 type 处于 Auto 状态（缺省或
//	          无效值）**且 command / commandfor 都不存在**、父节点也不是
//	          <select>（select 内的按钮是占位按钮，不提交表单）；
//	<input> ：type 为 submit 或 image。
func isSubmitButton(node *dom.Element) bool {
	if node == nil {
		return false
	}
	switch node.LocalName() {
	case "button":
		t := lowerTrim(node.GetAttribute("type"))
		switch t {
		case "submit":
			return true
		case "reset", "button":
			return false
		}
		// Auto 状态：带命令属性的按钮不是提交按钮（这是 popover invoker 的
		// 常见写法 `<button commandfor=… command="show-popover">`）。
		if node.HasAttribute("command") || node.HasAttribute("commandfor") {
			return false
		}
		if p := node.ParentElement(); p != nil && p.LocalName() == "select" {
			return false
		}
		return true
	case "input":
		t := lowerTrim(node.GetAttribute("type"))
		return t == "submit" || t == "image"
	}
	return false
}

// isDisabledInvoker 报告 invoker 是否被禁用（disabled 属性；本端口不处理
// fieldset 继承的禁用状态——与 html5 的按钮禁用判定保持一致）。
func isDisabledInvoker(node *dom.Element) bool {
	return node.HasAttribute("disabled")
}

// elementByID 按 ID 在同一文档中查找元素（规范的「associated element」语义：
// 用属性值作为 ID 在树的 root 中查找；本端口不区分 shadow tree scope）。
func elementByID(node *dom.Element, id string) *dom.Element {
	id = strings.TrimSpace(id)
	if node == nil || id == "" {
		return nil
	}
	if doc := node.OwnerDocument(); doc != nil {
		return doc.GetElementById(id)
	}
	return nil
}

// TargetElementOf 实现规范「get the popover target element」（popovertarget
// 路径）：
//
//	不是按钮 / 被禁用 / 有表单所有者且是提交按钮 → null
//	popovertarget 找不到元素、或目标没有 popover 属性 → null
func TargetElementOf(node *dom.Element) *dom.Element {
	if !isPopoverInvoker(node) || isDisabledInvoker(node) {
		return nil
	}
	if hasFormOwner(node) && isSubmitButton(node) {
		return nil
	}
	target := elementByID(node, node.GetAttribute("popovertarget"))
	if target == nil || ModeOf(target) == ModeNone {
		return nil
	}
	return target
}

// commandTargetOf 实现规范「get the commandfor-associated element」+ 命令有效性
// 的 popover 部分（「get the target popover」）：commandfor 缺失或指向的元素
// 不是 popover 元素、或 command 不是 popover 三条命令之一时返回 nil。
func commandTargetOf(node *dom.Element) *dom.Element {
	if !isPopoverInvoker(node) || isDisabledInvoker(node) {
		return nil
	}
	target := elementByID(node, node.GetAttribute("commandfor"))
	if target == nil {
		return nil
	}
	// 规范：有表单所有者时，提交按钮 / 重置按钮 / Auto 状态的按钮直接返回 null。
	if hasFormOwner(node) && (isSubmitButton(node) || lowerTrim(node.GetAttribute("type")) == "reset") {
		return nil
	}
	switch lowerTrim(node.GetAttribute("command")) {
	case commandTogglePopover, commandShowPopover, commandHidePopover:
	default:
		return nil
	}
	if ModeOf(target) == ModeNone {
		return nil
	}
	return target
}

// targetPopoverOf 实现规范「get the target popover」：commandfor 路径优先，
// 目标无效时回退到 popover target element（popovertarget 路径）。light dismiss
// 用它判断「点击位置所属的 invoker 指向哪个 popover」。
func targetPopoverOf(node *dom.Element) *dom.Element {
	if cmdTarget := commandTargetOf(node); cmdTarget != nil {
		return cmdTarget
	}
	// 非 button 元素（例如 input type=button）没有 commandfor 路径，规范直接取
	// popover target element —— 与上面 commandTargetOf 的前置检查叠加后等价。
	return TargetElementOf(node)
}

// RunTargetAttributeActivation 实现规范「run the popover target attribute
// activation behavior」（popovertarget）：返回 true 表示本次激活被 popover 消费。
//
// source 参数是触发者（即 node 自身），它会成为 ToggleEvent.source —— 这是本
// 引擎中 source 唯一非 null 的场景（<dialog> 的所有路径都传 null）。
func RunTargetAttributeActivation(node, eventTarget *dom.Element) bool {
	target := TargetElementOf(node)
	if target == nil {
		return false
	}
	// 规范：事件目标是 popover 的（含自身的）后代、而 popover 是 node 的**严格**
	// 后代时不做任何事——即「popover 嵌在按钮内部」时，点击 popover 自身内部不会
	// 反复切换它。注意两处后代判定不同：eventTarget 允许就是 popover 本身，
	// 而 popover 不能是 node 自己，否则按钮自身即 popover 的写法
	// （`<button popover popovertarget=自己>`）就永远开不了它
	// （WPT popover-self-invoke 期望它能打开）。
	if eventTarget != nil && isFlatTreeAncestor(target, eventTarget) && isStrictDescendant(node, target) {
		return false
	}
	action := TargetActionOf(node)
	showing := target.Popover().Showing
	if action == ActionShow && showing {
		return true
	}
	if action == ActionHide && !showing {
		return true
	}
	if showing {
		_ = Hide(target, true, true, false, node)
		return true
	}
	if ok, _ := checkPopoverValidity(target, false, nil); ok {
		// throwExceptions=false：invoker 激活不把状态异常抛给脚本（规范如此）。
		_ = Show(target, false, node)
	}
	return true
}

// runCommandActivation 执行 commandfor + command 的 popover 命令
// （show-popover / hide-popover / toggle-popover）。
func runCommandActivation(node, target *dom.Element) bool {
	command := lowerTrim(node.GetAttribute("command"))
	switch command {
	case commandShowPopover:
		if ok, _ := checkPopoverValidity(target, false, nil); ok {
			_ = Show(target, false, node)
		}
	case commandHidePopover:
		if ok, _ := checkPopoverValidity(target, true, nil); ok {
			_ = Hide(target, true, true, false, node)
		}
	case commandTogglePopover:
		// 规范：toggle 先按「期望正在显示」校验，有效则隐藏；否则按「期望隐藏」
		// 校验，有效则显示。
		if ok, _ := checkPopoverValidity(target, true, nil); ok {
			_ = Hide(target, true, true, false, node)
		} else if ok, _ := checkPopoverValidity(target, false, nil); ok {
			_ = Show(target, false, node)
		}
	default:
		return false
	}
	return true
}

// RunActivation 实现 button / （type="button" 的）input 的激活行为中与 popover
// 有关的部分，宿主在真实点击的默认行为阶段调用：返回 true 表示这次点击已被
// popover 消费（宿主不必再做别的默认行为）。
//
// 与表单提交的关系（规范顺序）：有表单所有者的提交按钮、重置按钮、以及
// type 处于 Auto 状态的按钮，其激活行为先由表单逻辑处理，本函数直接返回 false，
// 让宿主继续走 handleFormSubmitClick。
func RunActivation(node, eventTarget *dom.Element) bool {
	if !isPopoverInvoker(node) || isDisabledInvoker(node) {
		return false
	}
	if hasFormOwner(node) {
		if isSubmitButton(node) {
			return false // 提交按钮：先走表单提交
		}
		if lowerTrim(node.GetAttribute("type")) == "reset" {
			return false // 重置按钮
		}
	}
	// commandfor 路径优先（规范：get the target popover）。
	if target := commandTargetOf(node); target != nil {
		return runCommandActivation(node, target)
	}
	// 回退到 popovertarget 路径。
	return RunTargetAttributeActivation(node, eventTarget)
}
