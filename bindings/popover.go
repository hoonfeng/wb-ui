// Package bindings — Popover API（HTML §6.12）的 JS 接口。
//
// 本文件只做「IDL ↔ 算法」的翻译，算法在 wb-ui/popover 包：
//
//	HTMLElement.popover            元素是否 popover + 状态（枚举属性反射）
//	showPopover(options)           → popover.Show(el, throwExceptions=true, source)
//	hidePopover()                  → popover.Hide(el, true, true, true, nil)
//	togglePopover(options)         → popover.Toggle(el, force, source)
//	popoverTargetElement/-Action   invoker 属性（button / input type=button）
//	commandForElement / command    通用命令属性（button）
//
// 状态来源（dom.PopoverState）见 dom/popoverstate.go；实现细节与已知差异见
// wb-ui/popover 的包注释。
//
// 状态变化后必须走 invalidateStateStyle（= 清 computed style 缓存 +
// OnClassChanged → 渲染树重建）：`:popover-open`、UA 的
// `[popover]:not(:popover-open){display:none}` 与 `::backdrop` 的生成都依赖它，
// 否则 popover 打开/关闭后页面停留在旧样式（与 dialog / details 同一坑）。
package bindings

import (
	"strings"

	"wb-ui/dom"
	"wb-ui/jsc"
	"wb-ui/popover"
)

func init() {
	// popover 子系统的宿主钩子。样式失效走 dialog/details 同一条链路；聚焦走
	// bindings.focusElement（先置引擎焦点状态，再交给宿主注入的 FocusBridge——
	// app.Host 借此把 IME 目标切过去）。异步任务队列不在这里注入：它需要
	// WebView 的 JS 事件循环，由 webkit 按元素归属注入；未注入时 popover 同步
	// 派发 toggle 事件（无 JS 引擎的宿主与 dom 层单测场景）。
	popover.InvalidateStyle = invalidateStateStyle
	popover.FocusElement = focusElement
}

// popoverErrorPanic 把 popover 算法返回的错误抛给脚本（DOMException 等价对象，
// 带 name/message —— 本引擎没有 DOMException 构造器，脚本只读这两个字段）。
//
// ★ 必须 panic 一个 **goja 原生的 *goja.Object**：goja 只把 `*goja.Object`
// （以及内建 error）识别为「抛给 JS 的异常」，panic 一个 jsc.JSValue 包装会直接
// 穿透成 Go panic —— 脚本的 try/catch 抓不到，异常会一路掀到宿主
// （RunProgram 里 repanic，整个脚本执行乃至进程崩掉）。dialog.go /
// go2js.go 的抛错同样走 in.VM()。
func popoverErrorPanic(in *jsc.Interpreter, err error) {
	if in == nil || in.VM() == nil {
		return
	}
	name, msg := "Error", err.Error()
	if perr, ok := err.(*popover.Error); ok {
		name, msg = perr.Name, perr.Message
	}
	o := in.VM().NewObject()
	_ = o.Set("name", name)
	_ = o.Set("message", msg)
	panic(o)
}

// popoverIDLValue 返回 HTMLElement.popover 的 IDL 值：无 popover 属性 → null；
// 否则返回规范化后的关键字（空值/auto → "auto"、manual → "manual"、
// 无效值 → "manual"、hint → "hint"）。
func popoverIDLValue(el *dom.Element) jsc.JSValue {
	switch popover.ModeOf(el) {
	case popover.ModeAuto:
		return jsc.StringValue("auto")
	case popover.ModeManual:
		return jsc.StringValue("manual")
	case popover.ModeHint:
		return jsc.StringValue("hint")
	}
	return jsc.Null()
}

// jsElementValue 从 JS 值取出元素（非元素对象返回 nil）。
func jsElementValue(v jsc.JSValue) *dom.Element {
	if !v.IsObject() {
		return nil
	}
	o := v.AsObject()
	if o == nil {
		return nil
	}
	el, _ := o.Internal().(*dom.Element)
	return el
}

// popoverOptionsSource 解析 showPopover({source}) / togglePopover({force, source})
// 里的 source 字段（HTML：PopoverShowOptions / PopoverToggleOptions 的
// source: Element）。非对象参数返回 nil。
func popoverOptionsSource(args []jsc.JSValue) *dom.Element {
	if len(args) == 0 || !args[0].IsObject() {
		return nil
	}
	o := args[0].AsObject()
	if o == nil {
		return nil
	}
	if v, ok := o.GetByKey("source"); ok {
		return jsElementValue(v)
	}
	return nil
}

// popoverToggleForce 解析 togglePopover(options) 的 force 语义（HTML：options
// 可以是 boolean，也可以是带 force 的对象；两者都没有时返回 nil = 翻转）。
func popoverToggleForce(args []jsc.JSValue) *bool {
	if len(args) == 0 {
		return nil
	}
	v := args[0]
	if v.IsUndefined() || v.IsNull() {
		return nil
	}
	if v.IsBoolean() {
		b := v.ToBoolean()
		return &b
	}
	if v.IsObject() {
		o := v.AsObject()
		if o == nil {
			return nil
		}
		if fv, ok := o.GetByKey("force"); ok && !fv.IsUndefined() && !fv.IsNull() {
			b := fv.ToBoolean()
			return &b
		}
	}
	return nil
}

// reflectedIDElementValue 实现「反射 ID 的 Element? 属性」的 getter（用元素的
// 某个属性值作为 ID 在当前文档中查找）。
func reflectedIDElementValue(in *jsc.Interpreter, el *dom.Element, attr string) jsc.JSValue {
	id := strings.TrimSpace(el.GetAttribute(attr))
	if id == "" {
		return jsc.Null()
	}
	doc := el.OwnerDocument()
	if doc == nil {
		return jsc.Null()
	}
	if target := doc.GetElementById(id); target != nil {
		return jsc.ObjectValue(wrapElement(in, target))
	}
	return jsc.Null()
}

// setReflectedIDElement 实现「反射 ID 的 Element? 属性」的 setter：null →
// 移除属性；非元素 → 不动作（浏览器抛 TypeError，本端口的属性 setter 一贯宽松）；
// 元素无 id → 不动作（浏览器抛 InvalidStateError，同上）。
func setReflectedIDElement(el *dom.Element, v jsc.JSValue, attr string) {
	if v.IsNull() || v.IsUndefined() {
		el.RemoveAttribute(attr)
		invalidateStateStyle(el)
		return
	}
	target := jsElementValue(v)
	if target == nil {
		return
	}
	id := target.GetAttribute("id")
	if id == "" {
		return
	}
	el.SetAttribute(attr, id)
	invalidateStateStyle(el)
}

// commandIDLValue 返回 button.command 的 IDL 值（HTML form-elements）：自定义
// 命令（以 "--" 开头）原样返回、未知值返回空串、其余返回规范关键字（小写）。
func commandIDLValue(el *dom.Element) jsc.JSValue {
	raw := el.GetAttribute("command")
	if !el.HasAttribute("command") {
		return jsc.StringValue("")
	}
	if strings.HasPrefix(raw, "--") {
		return jsc.StringValue(raw)
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "toggle-popover", "show-popover", "hide-popover",
		"close", "request-close", "show-modal":
		return jsc.StringValue(strings.ToLower(strings.TrimSpace(raw)))
	}
	return jsc.StringValue("")
}

// installPopoverProperty 物化元素包装器上与 popover 有关的属性/方法（返回
// ok=false 表示 key 不属于本子系统，交给 lazyelement 的其它分支）。
func installPopoverProperty(rt *jsc.Interpreter, el *dom.Element, key string) (jsc.JSValue, *elemAccessor, bool) {
	tag := strings.ToLower(el.LocalName())
	isInvokerTag := tag == "button" || tag == "input"

	switch key {
	case "popover":
		// 全局属性：所有 HTML 元素都有（本引擎的元素包装器对所有标签统一，因此
		// 不做标签判定——与 element.popover 在浏览器里对 SVG 元素返回 undefined
		// 的差异可以忽略）。
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return popoverIDLValue(el) },
			// setter 是纯反射：字符串原样写入属性（浏览器实测如此——WPT
			// popover-attribute-basic.html 里 popover='aUtO' 后 getAttribute
			// 返回 'aUtO'、popover='' 后属性值为空串；规范化只发生在 getter
			// 上）。属性变化本身会经 dom 的属性变更钩子处理 popover 状态迁移
			// （显示的 popover 属性状态变化 → 关闭）。
			set: func(v jsc.JSValue) {
				el.SetAttribute("popover", v.ToString())
				invalidateStateStyle(el)
			}}, true

	case "showPopover":
		return funcVal(rt.NewNativeFunction("showPopover",
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				if err := popover.Show(el, true, popoverOptionsSource(args)); err != nil {
					popoverErrorPanic(in, err)
				}
				return jsc.Undefined()
			}, 0)), nil, true

	case "hidePopover":
		return funcVal(rt.NewNativeFunction("hidePopover",
			func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				// 规范 hidePopover()：focusPreviousElement=true、fireEvents=true、
				// throwExceptions=true、source=null。
				if err := popover.Hide(el, true, true, true, nil); err != nil {
					popoverErrorPanic(in, err)
				}
				return jsc.Undefined()
			}, 0)), nil, true

	case "togglePopover":
		return funcVal(rt.NewNativeFunction("togglePopover",
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				showing, err := popover.Toggle(el, popoverToggleForce(args), popoverOptionsSource(args))
				if err != nil {
					popoverErrorPanic(in, err)
				}
				return jsc.BooleanValue(showing)
			}, 0)), nil, true

	case "popoverTargetElement":
		if !isInvokerTag {
			return jsc.JSValue{}, nil, false
		}
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue {
				return reflectedIDElementValue(rt, el, "popovertarget")
			},
			set: func(v jsc.JSValue) { setReflectedIDElement(el, v, "popovertarget") }}, true

	case "popoverTargetAction":
		if !isInvokerTag {
			return jsc.JSValue{}, nil, false
		}
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.StringValue(string(popover.TargetActionOf(el))) },
			set: func(v jsc.JSValue) {
				el.SetAttribute("popovertargetaction", v.ToString())
				invalidateStateStyle(el)
			}}, true

	case "commandForElement":
		if tag != "button" {
			return jsc.JSValue{}, nil, false
		}
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue {
				return reflectedIDElementValue(rt, el, "commandfor")
			},
			set: func(v jsc.JSValue) { setReflectedIDElement(el, v, "commandfor") }}, true

	case "command":
		if tag != "button" {
			return jsc.JSValue{}, nil, false
		}
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return commandIDLValue(el) },
			// command 的 setter 是 ReflectSetter：原样写入属性（getter 才做
			// 规范化与 Custom/Unknown 的区分）。
			set: func(v jsc.JSValue) {
				el.SetAttribute("command", v.ToString())
				invalidateStateStyle(el)
			}}, true
	}
	return jsc.JSValue{}, nil, false
}
