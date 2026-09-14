// Package bindings — 表单约束校验（constraint validation）的 JS IDL。
//
// 覆盖 HTML §4.10.21.3「The constraint validation API」：
//
//	元素级（<input> / <select> / <textarea> / <button>）：
//	  validity / validationMessage / willValidate
//	  checkValidity() / reportValidity() / setCustomValidity(message)
//	表单级（<form>）：
//	  checkValidity() / reportValidity() / noValidate
//
// 判定逻辑全部来自 html5 包（ValidityState / WillValidate / CheckValidity /
// InteractiveValidity），本文件只负责「按标签取到对应的包装器」与「映射成
// JS 对象」——同一套状态也被 CSS 的 :valid / :invalid / :in-range /
// :out-of-range 消费（见 css/validity.go）。
//
// 语义要点：
//   - barred（disabled / readonly / datalist 后代等）元素的 checkValidity()
//     恒为 true，但 validity.valid 仍可能为 false——与浏览器一致。
//   - reportValidity() 是交互版本：无效时派发 invalid 事件（不冒泡、可取消
//     但取消无效）并聚焦元素；checkValidity() 不派发事件。
//   - setCustomValidity() 改变 :valid/:invalid 的匹配结果，因此走
//     invalidateStateStyle（失效 computed style 缓存 + 渲染树重建）。
//   - validity 每次读取构造新对象（浏览器里是同一个持久对象；对象身份不保持
//     是本端口的次要差异，字段值始终是最新的）。
package bindings

import (
	"strings"

	"wb-ui/dom"
	"wb-ui/html5"
	"wb-ui/jsc"
)

// formControl 是 <input>/<select>/<textarea>/<button> 的约束校验统一视图。
type formControl struct {
	el        *dom.Element
	validity  func() html5.ValidityState
	will      func() bool
	check     func() bool
	setCustom func(string)
}

// formControlOf 按标签取到对应控件的校验入口。
func formControlOf(el *dom.Element) (formControl, bool) {
	if el == nil {
		return formControl{}, false
	}
	switch el.LocalName() {
	case "input":
		in, ok := html5.ToInputElement(el)
		if !ok {
			return formControl{}, false
		}
		return formControl{el: el, validity: in.Validity, will: in.WillValidate,
			check: in.CheckValidity, setCustom: in.SetCustomValidity}, true
	case "select":
		sel, ok := html5.ToSelectElement(el)
		if !ok {
			return formControl{}, false
		}
		return formControl{el: el, validity: sel.Validity, will: sel.WillValidate,
			check: sel.CheckValidity, setCustom: sel.SetCustomValidity}, true
	case "textarea":
		ta, ok := html5.ToTextAreaElement(el)
		if !ok {
			return formControl{}, false
		}
		return formControl{el: el, validity: ta.Validity, will: ta.WillValidate,
			check: ta.CheckValidity, setCustom: ta.SetCustomValidity}, true
	case "button":
		b, ok := html5.ToButtonElement(el)
		if !ok {
			return formControl{}, false
		}
		return formControl{el: el, validity: b.Validity, will: b.WillValidate,
			check: b.CheckValidity, setCustom: b.SetCustomValidity}, true
	}
	return formControl{}, false
}

// validityStateObject 把 html5.ValidityState 映射成 WebIDL 的 ValidityState
// 对象（11 个只读布尔属性）。
func validityStateObject(rt *jsc.Interpreter, v html5.ValidityState) jsc.JSValue {
	o := jsc.NewObject(rt.ObjectPrototype())
	o.SetClassName("ValidityState")
	set := func(name string, b bool) { o.Set(name, jsc.BooleanValue(b)) }
	set("valueMissing", v.ValueMissing)
	set("typeMismatch", v.TypeMismatch)
	set("patternMismatch", v.PatternMismatch)
	set("tooLong", v.TooLong)
	set("tooShort", v.TooShort)
	set("rangeUnderflow", v.RangeUnderflow)
	set("rangeOverflow", v.RangeOverflow)
	set("stepMismatch", v.StepMismatch)
	set("badInput", v.BadInput)
	set("customError", v.CustomError)
	set("valid", v.Valid())
	return jsc.ObjectValue(o)
}

// focusElement 把焦点移到元素上（DOM 焦点状态 + 宿主焦点桥）。
func focusElement(el *dom.Element) {
	if el == nil {
		return
	}
	el.SetFocused(true)
	if FocusBridge != nil {
		FocusBridge(el, true)
	}
}

// invalidateConstraintState 让约束校验状态的变化抵达样式与渲染：:valid /
// :invalid / :in-range / :out-of-range 的匹配结果可能已经变了，但属性本身
// 没有变化（IDL 状态），必须显式失效。
func invalidateConstraintState(el *dom.Element) {
	invalidateStateStyle(el)
}

// MarkUserInteracted 记录「用户已与该表单控件交互」——控件的 user validity
// 置为 true（HTML §4.10.18.1）。引擎在真实用户输入路径上调用它：change 事件
// 是「用户改变了值并提交了该改变」（例如失焦、点击 checkbox/radio、选择
// option、拖动 range）的规范信号，:user-valid / :user-invalid 依赖这个状态。
//
// 脚本 dispatchEvent(new Event('change')) 不经过这里，与浏览器一致（脚本
// 派发的事件不代表用户交互）。
func MarkUserInteracted(el *dom.Element) {
	if el == nil || el.UserInteracted() {
		return
	}
	switch el.LocalName() {
	case "input", "select", "textarea":
	default:
		return
	}
	el.SetUserInteracted(true)
	// user validity 变化会改变 :user-valid/:user-invalid 的匹配，而属性没变
	// → 走样式失效链。
	invalidateStateStyle(el)
}

// init 把 html5 的「user validity 变化」通知接到样式失效链上（html5 层不依赖
// bindings，所以用注入的方式）。
func init() {
	html5.OnUserValidityChanged = func(el *dom.Element) {
		invalidateStateStyle(el)
	}
}

// installFormValidationProperty 物化约束校验相关的属性/方法。key 不在本表内
// 或元素类型不匹配时返回 ok=false。
func installFormValidationProperty(rt *jsc.Interpreter, el *dom.Element, key string) (jsc.JSValue, *elemAccessor, bool) {
	if strings.ToLower(el.LocalName()) == "form" {
		return installFormElementValidationProperty(rt, el, key)
	}
	ctl, ok := formControlOf(el)
	if !ok {
		return jsc.JSValue{}, nil, false
	}
	switch key {
	case "validity":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return validityStateObject(rt, ctl.validity())
		}}, true
	case "willValidate":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return jsc.BooleanValue(ctl.will())
		}}, true
	case "validationMessage":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			return jsc.StringValue(ctl.validity().ValidationMessage(el))
		}}, true
	case "checkValidity":
		return jsc.FunctionValue(jsc.NewNativeFunction("checkValidity",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				return jsc.BooleanValue(ctl.check())
			}, 0)), nil, true
	case "reportValidity":
		return jsc.FunctionValue(jsc.NewNativeFunction("reportValidity",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				valid, _ := html5.InteractiveValidity(el)
				if !valid {
					focusElement(el)
				}
				return jsc.BooleanValue(valid)
			}, 0)), nil, true
	case "setCustomValidity":
		return jsc.FunctionValue(jsc.NewNativeFunction("setCustomValidity",
			func(_ *jsc.Interpreter, _ jsc.JSValue, a []jsc.JSValue) jsc.JSValue {
				msg := ""
				if len(a) > 0 && !a[0].IsUndefined() && !a[0].IsNull() {
					msg = a[0].ToString()
				}
				ctl.setCustom(msg)
				// IDL 状态变化（无属性变化）→ 必须显式失效，否则
				// :valid/:invalid 会停留在上一次的样式。
				invalidateConstraintState(el)
				return jsc.Undefined()
			}, 1)), nil, true
	}
	return jsc.JSValue{}, nil, false
}

// installFormElementValidationProperty 处理 <form> 的校验 API。
func installFormElementValidationProperty(rt *jsc.Interpreter, el *dom.Element, key string) (jsc.JSValue, *elemAccessor, bool) {
	switch key {
	case "checkValidity":
		return jsc.FunctionValue(jsc.NewNativeFunction("checkValidity",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				f, ok := html5.ToFormElement(el)
				if !ok {
					return jsc.BooleanValue(true)
				}
				return jsc.BooleanValue(f.CheckValidity())
			}, 0)), nil, true
	case "reportValidity":
		return jsc.FunctionValue(jsc.NewNativeFunction("reportValidity",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				f, ok := html5.ToFormElement(el)
				if !ok {
					return jsc.BooleanValue(true)
				}
				if f.CheckValidity() {
					return jsc.BooleanValue(true)
				}
				// 交互校验：对每个无效控件派发 invalid 事件，聚焦第一个。
				if invalid := f.ValidateInteractively(); len(invalid) > 0 {
					focusElement(invalid[0])
				}
				return jsc.BooleanValue(false)
			}, 0)), nil, true
	case "noValidate":
		// noValidate 是 IDL 属性，反映 novalidate 内容属性（可读可写）。
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue {
				return jsc.BooleanValue(el.HasAttribute("novalidate"))
			},
			set: func(v jsc.JSValue) {
				if v.ToBoolean() {
					el.SetAttribute("novalidate", "")
				} else {
					el.RemoveAttribute("novalidate")
				}
				invalidateConstraintState(el)
			}}, true
	}
	return jsc.JSValue{}, nil, false
}
