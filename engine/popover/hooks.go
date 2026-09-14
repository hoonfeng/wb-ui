// popover 包的引擎钩子注册：把 dom 的属性变更步骤接到 popover 的 attribute
// change steps（HTML §6.12：popover 属性的状态变化且元素正在显示时关闭它）。
//
// dom 包不能依赖 popover（popover 依赖 dom），因此用包级回调注册——与本引擎
// 其它「dom 状态 + 上层算法」的组合方式一致（html5 注册 dom.OnElementFocused、
// bindings 注入 dom 的样式失效回调等）。
package popover

import "wb-ui/engine/dom"

func init() {
	dom.OnElementAttributeChanged = func(el *dom.Element, localName, oldValue string, oldExisted bool, newValue string, newExisted bool) {
		if localName != "popover" {
			return
		}
		AttributeChanged(el, oldValue, oldExisted, newValue, newExisted)
	}
}
