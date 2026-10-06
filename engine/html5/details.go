package html5

import "wb-ui/engine/dom"

// InvalidateStyle 让元素的状态变化抵达样式与渲染（失效 computed style 缓存 +
// 触发渲染树重建），由 bindings 包注入（与 popover.InvalidateStyle 同一链路：
// bindings.invalidateStateStyle = InvalidateComputedStyle + OnClassChanged）。
// 未注入时无操作（纯 dom 层单测场景）。
//
// 凡是「属性由引擎直接改、但选择器匹配结果变了」的状态迁移都必须走这里：
//
//	<details> 的 open —— [open] / :open / :closed，以及 UA 规则
//	  details:not([open]) > :not(summary){display:none}；
//	checkbox/radio 的 checked —— :checked（`input:checked + .track::after`
//	  开关滑块、勾选态配色）；
//	select/option 的 selected —— option:checked（自绘触发条的选中项样式）。
//
// JS 侧 setAttribute 自带失效（bindings 的 InvalidateComputedStyle 钩子），
// 引擎内部路径没有——缺这一步时 computed style 与渲染树会停留在上一次的状态
// （实例：引擎内部 SetChecked 后 getComputedStyle 仍返回旧值，见
// webkit/formstate_invalidation_test.go）。
var InvalidateStyle func(el *dom.Element)

// invalidateState 在状态属性变更后触发样式失效（钩子未注入时无操作，
// 便于纯 dom 层单测）。
func invalidateState(el *dom.Element) {
	if el != nil && InvalidateStyle != nil {
		InvalidateStyle(el)
	}
}

// --- HTMLDetailsElement ---

// HTMLDetailsElement wraps a <details> element. Mirrors
// WebCore::HTMLDetailsElement.
type HTMLDetailsElement struct {
	El *dom.Element
}

// ToDetailsElement wraps an element as an HTMLDetailsElement.
func ToDetailsElement(el *dom.Element) (HTMLDetailsElement, bool) {
	if el == nil || el.LocalName() != "details" {
		return HTMLDetailsElement{}, false
	}
	return HTMLDetailsElement{El: el}, true
}

// Open reports whether the details are visible (the "open" attribute is
// present). Mirrors HTMLDetailsElement::open().
func (d HTMLDetailsElement) Open() bool {
	return d.El.HasAttribute("open")
}

// SetOpen toggles the visibility of the details content. When set to true,
// the "open" attribute is added and the content becomes visible. When set
// to false, the "open" attribute is removed and the content is hidden
// (via the UA stylesheet rule details:not([open]) > :not(summary)).
// Mirrors HTMLDetailsElement::setOpen().
func (d HTMLDetailsElement) SetOpen(o bool) {
	if o {
		d.El.SetAttribute("open", "open")
	} else {
		d.El.RemoveAttribute("open")
	}
}

// Toggle switches the open state and returns the new state.
func (d HTMLDetailsElement) Toggle() bool {
	d.SetOpen(!d.Open())
	return d.Open()
}

// Summary returns the first <summary> child element, or nil if none exists.
func (d HTMLDetailsElement) Summary() *dom.Element {
	for c := d.El.FirstChild(); c != nil; c = c.NextSibling() {
		if el, ok := c.(*dom.Element); ok && el.LocalName() == "summary" {
			return el
		}
	}
	return nil
}

// ActivateSummary 实现 <summary> 的点击激活行为（HTML §4.11.4「details 元素的
// activation behavior」/ WebKit HTMLSummaryElement::defaultEventHandler）：点击
// summary 或其任意后代 → 切换所属 <details> 的 open 属性，并派发 toggle 事件。
//
// 返回 true 表示这次点击被 summary 消费（调用方据此知道 open 已变化，需要重建
// 渲染树/布局 —— 【open】属性驱动的 UA 规则 details:not([open]) > :not(summary)
// 会改变整棵子树的 display）。
//
// 触发者取**最近的** summary 祖先（含自身）：嵌套 <details> 时点内层 summary
// 只切换内层 details。该 summary 还必须是所属 details 的**第一个** summary 子
// 元素（规范：非首个 summary 不参与折叠交互）。
//
// toggle 事件在此同步派发（浏览器是排队任务，但对页面处理器而言时序等价：
// 事件到达时 open 属性已是新值）。details 的 toggle 任务 source 为 null
// （见 dom.ToggleEvent.Source 的注释）。
func ActivateSummary(el *dom.Element) bool {
	if el == nil {
		return false
	}
	sum := el
	for sum != nil && sum.LocalName() != "summary" {
		sum = sum.ParentElement()
	}
	if sum == nil {
		return false
	}
	parent := sum.ParentElement()
	if parent == nil {
		return false
	}
	d, ok := ToDetailsElement(parent)
	if !ok || d.Summary() != sum {
		return false
	}
	wasOpen := d.Open()
	d.SetOpen(!wasOpen)
	// open 属性变了 → [open] / :open / :closed 的匹配结果与 UA 规则
	// （details:not([open]) > :not(summary){display:none}）都变，必须失效样式。
	invalidateState(parent)
	oldState, newState := dom.ToggleStateClosed, dom.ToggleStateOpen
	if wasOpen {
		oldState, newState = dom.ToggleStateOpen, dom.ToggleStateClosed
	}
	parent.DispatchEvent(dom.NewToggleEvent("toggle", false, false, oldState, newState, nil))
	return true
}

// --- HTMLSummaryElement ---

// HTMLSummaryElement wraps a <summary> element. Mirrors
// WebCore::HTMLSummaryElement.
type HTMLSummaryElement struct {
	El *dom.Element
}

// ToSummaryElement wraps an element as an HTMLSummaryElement.
func ToSummaryElement(el *dom.Element) (HTMLSummaryElement, bool) {
	if el == nil || el.LocalName() != "summary" {
		return HTMLSummaryElement{}, false
	}
	return HTMLSummaryElement{El: el}, true
}
