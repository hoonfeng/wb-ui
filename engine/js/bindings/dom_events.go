// Translation of: Source/WebCore/bindings/js/JSEventListener.cpp
//                  Source/WebCore/bindings/js/JSEventListener.h
//                  Source/WebCore/bindings/js/JSEventCustom.cpp
//                  Source/WebCore/bindings/js/JSEventTargetCustom.cpp
// Completeness: 60%
// Simplifications:
//   - JS event listeners are stored as Go EventListenerFunc closures that capture the
//     JSFunction value and the interpreter; on dispatch they convert the Event to a JS
//     object and call back via Interpreter.Call.
//   - no listener identity comparison across JS/Go (RemoveEventListener matches by the
//     original JS callback reference, which is recovered from the wrapper stored on the
//     Go listener via a small side-table).
//   - jsToEvent builds a best-effort Event or MouseEvent from a plain JS object; custom
//     event subclasses are not modelled.

package bindings

import (
	"fmt"
	"os"

	"wb-ui/engine/dom"
	"wb-ui/engine/js/jsc"
)

// listenerKey uniquely identifies one JS listener registration: the target it is
// installed on, the event type, the JS function identity, and the capture phase.
// The function identity is *JSFunction.String() (a stable pointer-based id like
// "js:0x..."); AsFunction() creates a new *JSFunction per call, so Go pointer
// identity is unreliable and the string id is used instead.
//
// ★ target 必须参与键：同一个 JS 回调可以挂在多个元素上，每个 target 要各自持有
//
//	独立的 jsListener（否则移除/派发会串台到别的元素）。
type listenerKey struct {
	target    dom.EventTarget
	eventType string
	fnID      string
	capture   bool
}

// registeredListeners is the side-table of live JS listeners, keyed by the full
// registration identity above. It serves two purposes:
//
//  1. addEventListener 去重：dom.EventTarget 按 Go listener 对象身份判重
//     （listenerEquals），而本包每次 add 都新建 jsListener —— 若不复用，同一 JS
//     回调重复注册就不会被去重（DOM 规范要求同 (type, callback, capture) 的重复
//     注册是 no-op），监听器只增不减，一次事件被处理多次。
//     实测（2026-09-26）：Vue 的 @change 在桌面端累积到 2 个 → 选择工具集时
//     「本对话已切换工具集为 X」toast 弹两条；浏览器（Blink 自带去重）只有一条。
//  2. removeEventListener 定位：JS 侧只有函数值，靠该表取回注册时的 Go listener，
//     dom 层的身份比较才能命中并真正移除（此前每次新建 listener → 永不相等）。
var registeredListeners = map[listenerKey]*jsListener{}

// jsListener wraps a JS callback so it can be installed on a dom.EventTarget. It
// implements dom.EventListener by converting the event to a JS object and invoking the
// captured JSFunction through the interpreter.
type jsListener struct {
	interp *jsc.Interpreter
	fn     jsc.JSValue
}

// HandleEvent converts the dom.Event to a JS event object and calls the JS callback.
func (l *jsListener) HandleEvent(e dom.Event) {
	if l.interp == nil || !l.fn.IsFunction() {
		return
	}
	if os.Getenv("WB_EVT_DEBUG") != "" {
		fmt.Printf("[evt] jsListener.HandleEvent type=%q\n", e.Type())
	}
	ev := eventToJS(l.interp, e)
	// ★ 浏览器语义（DOM 标准 inner invoke）：监听器是 Function 且事件的
	//   currentTarget 非 null 时，**this = currentTarget**（即绑定的元素）——
	//   这是 addEventListener 回调里 this.classList.add(...) 能工作的前提。
	//   此前固定传 Undefined（旧注释误以为「plain call」就是浏览器行为）→
	//   handler 内 this 为 undefined，this.classList 全部落空（实测：点击
	//   导航胶囊后 .active 被循环清空却加不到被点项，快照 active=count=0，
	//   视图 display 也不切换）。
	thisVal := jsc.Undefined()
	if ev.IsObject() {
		if o := ev.AsObject(); o != nil {
			if ct, ok := o.GetByKey("currentTarget"); ok && !ct.IsNull() && !ct.IsUndefined() {
				thisVal = ct
			}
		}
	}
	_, err := l.interp.Call(l.fn, thisVal, []jsc.JSValue{ev})
	// ★ Flush goja's Promise microtasks: Vue's @click handler mutates reactive
	// state and schedules the DOM update via Promise.resolve().then. Runtime.Call
	// does NOT run those jobs (only RunProgram does), so without this the DOM
	// stays stale and every click looks like it "does nothing".
	l.interp.RunJobs()
	if os.Getenv("WB_EVT_DEBUG") != "" && err != nil {
		fmt.Printf("[evt] jsListener call error: %v\n", err)
	}
}

// makeAddEventListener returns a native JS function that registers a JS callback as a
// DOM event listener on target. The JS callback is wrapped in a jsListener so the Go
// dispatcher can invoke it. The capture flag (third arg) is honoured. The created
// listener is recorded in the side-table so removeEventListener can recover it.
func makeAddEventListener(target dom.EventTarget) *jsc.JSFunction {
	return jsc.NewNativeFunction("addEventListener",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 2 || !args[1].IsFunction() {
				return jsc.Undefined()
			}
			eventType := args[0].ToString()
			jsFn := args[1]
			capture := false
			if len(args) >= 3 {
				capture = args[2].ToBoolean()
			}
			key := listenerKey{target: target, eventType: eventType, fnID: jsFn.AsFunction().String(), capture: capture}
			if _, dup := registeredListeners[key]; dup {
				// ★ DOM 规范 §2.7（addEventListener）：type/callback/capture 完全
				//   相同的重复注册是 no-op，不产生第二个监听器。此前每次调用都新建
				//   jsListener，dom 层按对象身份判重必然失败 → 重复注册全部生效，
				//   事件被处理多次（Vue @change 双触发 → toast 双弹）。
				if os.Getenv("WB_EVT_DEBUG") != "" {
					fmt.Printf("[evt] addEventListener DUPLICATE(ignored) target=%s type=%q\n", targetDesc(target), eventType)
				}
				return jsc.Undefined()
			}
			listener := &jsListener{interp: in, fn: jsFn}
			// 只有真正装上（未被 dom 层判重拒绝）才记录，保证表与实际注册一致。
			if target.AddEventListener(eventType, listener, capture) {
				registeredListeners[key] = listener
				if os.Getenv("WB_EVT_DEBUG") != "" {
					// %p 打印 EventTarget 动态值（元素指针），用于区分「同一元素重复注册」
					// 与「多个同类元素各注册一次」——两者在 tag.class 上无法分辨。
					fmt.Printf("[evt] addEventListener OK target=%s(%p) type=%q capture=%v fnID=%s\n",
						targetDesc(target), target, eventType, capture, key.fnID)
				}
			}
			return jsc.Undefined()
		}, 2)
}

// targetDesc 把 EventTarget 描述成调试可读的 "tag.class"（非元素时退化为 Go 类型名）。
func targetDesc(t dom.EventTarget) string {
	if el, ok := t.(*dom.Element); ok {
		if cls := el.ClassName(); cls != "" {
			return el.LocalName() + "." + cls
		}
		return el.LocalName()
	}
	return fmt.Sprintf("%T", t)
}

// makeRemoveEventListener returns a native JS function that unregisters a previously
// registered JS callback, recovering the Go listener from the side-table by JS
// function identity.
func makeRemoveEventListener(target dom.EventTarget) *jsc.JSFunction {
	return jsc.NewNativeFunction("removeEventListener",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 2 || !args[1].IsFunction() {
				return jsc.Undefined()
			}
			eventType := args[0].ToString()
			capture := false
			if len(args) >= 3 {
				capture = args[2].ToBoolean()
			}
			key := listenerKey{target: target, eventType: eventType, fnID: args[1].AsFunction().String(), capture: capture}
			if l, ok := registeredListeners[key]; ok {
				if target.RemoveEventListener(eventType, l, capture) {
					delete(registeredListeners, key)
				}
			}
			return jsc.Undefined()
		}, 2)
}

// makeDispatchEvent returns a native JS function that dispatches a JS event object on
// the target. The JS object is converted to a dom.Event via jsToEvent.
func makeDispatchEvent(target dom.EventTarget) *jsc.JSFunction {
	return jsc.NewNativeFunction("dispatchEvent",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.BooleanValue(true)
			}
			ev := jsToEvent(args[0])
			if ev == nil {
				return jsc.BooleanValue(true)
			}
			return jsc.BooleanValue(target.DispatchEvent(ev))
		}, 1)
}

// eventInterfaceName 返回 dom.Event 对应的 JS 事件接口名（第 19 轮原型链分派用）。
// 引擎派发/创建的事件对象据此挂上对应接口的 prototype，使
// `e instanceof PointerEvent`、`e.constructor.name === "InputEvent"` 等成立。
func eventInterfaceName(e dom.Event) string {
	switch e.(type) {
	case *dom.MouseEvent:
		return "MouseEvent"
	case *dom.KeyboardEvent:
		return "KeyboardEvent"
	case *dom.WheelEvent:
		return "WheelEvent"
	case *dom.InputEvent:
		return "InputEvent"
	case *dom.ToggleEvent:
		return "ToggleEvent"
	case *dom.CompositionEvent:
		return "CompositionEvent"
	}
	return "Event"
}

// eventToJS converts a dom.Event to a JS object with the standard Event properties and
// methods (type/target/bubbles/cancelable/... plus preventDefault/stopPropagation).
// MouseEvent adds the coordinate/modifier fields.
func eventToJS(in *jsc.Interpreter, e dom.Event) jsc.JSValue {
	if e == nil {
		return jsc.Undefined()
	}
	// ★ 第 19 轮：事件实例的原型指向对应事件接口 prototype（此前是
	// Object.prototype + 仅 className），因此引擎派发的事件同样满足
	// `e instanceof MouseEvent` / `e.constructor.name === "MouseEvent"`。
	evName := eventInterfaceName(e)
	obj := jsc.NewObject(domIfaceProtoOr(evName, in.ObjectPrototype()))
	obj.SetClassName(evName)
	obj.SetInternal(e)
	obj.Set("type", jsc.StringValue(e.Type()))
	obj.Set("bubbles", jsc.BooleanValue(e.Bubbles()))
	obj.Set("cancelable", jsc.BooleanValue(e.Cancelable()))
	obj.Set("composed", jsc.BooleanValue(e.Composed()))
	obj.Set("defaultPrevented", jsc.BooleanValue(e.DefaultPrevented()))
	obj.Set("eventPhase", jsc.NumberValue(float64(e.EventPhase())))
	obj.Set("timeStamp", jsc.NumberValue(float64(e.TimeStamp().UnixNano())/1e6))
	// ★ target/currentTarget: Vue's .self modifier compiles to
	// `$event.target === $event.currentTarget` — without these the
	// comparison is `undefined === undefined` (always true) AND @click.self
	// on the dialog overlay can't distinguish clicks on the overlay vs its
	// children. Expose both as JS wrappers of the DOM nodes.
	if t := e.Target(); t != nil {
		if el, ok := t.(*dom.Element); ok {
			obj.Set("target", jsc.ObjectValue(wrapElement(in, el)))
		}
	}
	if ct := e.CurrentTarget(); ct != nil {
		if el, ok := ct.(*dom.Element); ok {
			obj.Set("currentTarget", jsc.ObjectValue(wrapElement(in, el)))
		}
	}
	// ★ composedPath（浏览器标准）：返回事件路径 [target, ..., 根, window]。
	// CM6 的 mousedown handler 用 event.composedPath() 判断点击是否落在
	// 编辑器内容区（.cm-content 在路径中）——此前 undefined → 点击行
	// 定位逻辑走错分支（「点击行事件行偏移」根因之一）。
	obj.Set("composedPath", jsc.FunctionValue(jsc.NewNativeFunction("composedPath",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			var path []jsc.JSValue
			if t := e.Target(); t != nil {
				if n, ok := t.(dom.Node); ok {
					for n != nil {
						path = append(path, nodeToJS(in, n))
						// 非 composed 事件不穿透 shadow boundary。
						if !e.Composed() {
							if _, isSR := n.ParentNode().(*dom.ShadowRoot); isSR {
								break
							}
						}
						n = dom.ComposedParent(n)
					}
				}
			}
			path = append(path, jsc.ObjectValue(in.GlobalObject()))
			return jsc.ObjectValue(jsc.NewArray(nil, path))
		}, 0)))
	obj.Set("preventDefault", jsc.FunctionValue(jsc.NewNativeFunction("preventDefault",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			e.PreventDefault()
			return jsc.Undefined()
		}, 0)))
	obj.Set("stopPropagation", jsc.FunctionValue(jsc.NewNativeFunction("stopPropagation",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			e.StopPropagation()
			return jsc.Undefined()
		}, 0)))
	obj.Set("stopImmediatePropagation", jsc.FunctionValue(jsc.NewNativeFunction("stopImmediatePropagation",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			e.StopImmediatePropagation()
			return jsc.Undefined()
		}, 0)))
	if me, ok := e.(*dom.MouseEvent); ok {
		obj.SetClassName("MouseEvent")
		obj.Set("clientX", jsc.NumberValue(me.ClientX()))
		obj.Set("clientY", jsc.NumberValue(me.ClientY()))
		obj.Set("screenX", jsc.NumberValue(me.ScreenX()))
		obj.Set("screenY", jsc.NumberValue(me.ScreenY()))
		obj.Set("button", jsc.NumberValue(float64(me.Button())))
		obj.Set("buttons", jsc.NumberValue(float64(me.Buttons())))
		obj.Set("detail", jsc.NumberValue(float64(me.Detail())))
		obj.Set("ctrlKey", jsc.BooleanValue(me.CtrlKey()))
		obj.Set("altKey", jsc.BooleanValue(me.AltKey()))
		obj.Set("shiftKey", jsc.BooleanValue(me.ShiftKey()))
		obj.Set("metaKey", jsc.BooleanValue(me.MetaKey()))
	}
	// ★ KeyboardEvent 属性（浏览器标准）：xterm/旧式库读 e.keyCode /
	// e.key 判断按键（Enter=13、Ctrl+C 等）。此前 eventToJS 未暴露 →
	// JS 侧 key/keyCode 恒 undefined → xterm 的 Enter 分支永不匹配
	// （「回车不能触发执行」根因）。
	if ke, ok := e.(*dom.KeyboardEvent); ok {
		obj.SetClassName("KeyboardEvent")
		obj.Set("key", jsc.StringValue(ke.Key()))
		obj.Set("code", jsc.StringValue(ke.Code()))
		obj.Set("keyCode", jsc.NumberValue(float64(ke.KeyCode())))
		obj.Set("which", jsc.NumberValue(float64(ke.KeyCode())))
		obj.Set("charCode", jsc.NumberValue(float64(ke.CharCode())))
		obj.Set("repeat", jsc.BooleanValue(ke.Repeat()))
		obj.Set("isComposing", jsc.BooleanValue(ke.IsComposing()))
		obj.Set("ctrlKey", jsc.BooleanValue(ke.CtrlKey()))
		obj.Set("altKey", jsc.BooleanValue(ke.AltKey()))
		obj.Set("shiftKey", jsc.BooleanValue(ke.ShiftKey()))
		obj.Set("metaKey", jsc.BooleanValue(ke.MetaKey()))
		obj.Set("location", jsc.NumberValue(float64(ke.Location())))
	}
	// ★ WheelEvent 属性（浏览器标准）：xterm 6 的 ScrollableElement 滚轮
	// 处理读 e.deltaY（deltaMode=像素时 100 左右/格）——不暴露则
	// StandardWheelEvent.deltaY=0 → 终端滚轮无效。
	if we, ok := e.(*dom.WheelEvent); ok {
		obj.SetClassName("WheelEvent")
		obj.Set("deltaX", jsc.NumberValue(we.DeltaX()))
		obj.Set("deltaY", jsc.NumberValue(we.DeltaY()))
		obj.Set("deltaZ", jsc.NumberValue(we.DeltaZ()))
		obj.Set("deltaMode", jsc.NumberValue(float64(we.DeltaMode())))
	}
	// ★ CompositionEvent 属性（浏览器标准）：e.data 为组合字符串。
	// CM6 的 compositionstart/update/end 处理读 e.data——此前不暴露 →
	// data 恒 undefined → 组合文本进不了 CM6 state（IME 输入异常的
	// 一部分根因）。
	if ce, ok := e.(*dom.CompositionEvent); ok {
		obj.SetClassName("CompositionEvent")
		obj.Set("data", jsc.StringValue(ce.Data()))
	}
	// ★ InputEvent 属性（浏览器标准）：e.inputType / e.data / e.isComposing。
	// CM6 的 input 处理（inputHandler/readDOMChange）按 inputType 区分
	// insertText / insertCompositionText / insertFromComposition——
	// 此前 inputType 恒 undefined，组合提交被当成普通输入处理。
	if ie, ok := e.(*dom.InputEvent); ok {
		obj.SetClassName("InputEvent")
		obj.Set("inputType", jsc.StringValue(ie.InputType()))
		obj.Set("data", jsc.StringValue(ie.Data()))
		obj.Set("isComposing", jsc.BooleanValue(ie.IsComposing()))
	}
	// ★ ToggleEvent 属性（HTML §4.11.4 / §4.11.6）：<details> 与 <dialog> 的
	// toggle / beforetoggle 事件带 oldState / newState，页面常用 `e.newState
	// === "open"` 在一个处理器里区分「正在打开」和「正在关闭」——此前派发的是
	// 普通 Event，两个字段恒 undefined。
	//
	// source（IDL 类型 Element?）：只有 popover 的 invoker（popovertarget /
	// command 元素）触发的路径传非 null，其余全部是 null（规范如此）。无论哪种，
	// 属性都必须存在：MDN 的示例用 `event.source === undefined` 做特性检测，
	// 缺字段会被误判成「浏览器不支持」，而 `e.source === null` 的写法也会失真。
	if te, ok := e.(*dom.ToggleEvent); ok {
		obj.SetClassName("ToggleEvent")
		obj.Set("oldState", jsc.StringValue(te.OldState()))
		obj.Set("newState", jsc.StringValue(te.NewState()))
		if src := te.Source(); src != nil {
			obj.Set("source", nodeToJS(in, src))
		} else {
			obj.Set("source", jsc.Null())
		}
	}
	return jsc.ObjectValue(obj)
}

// jsToEvent builds a dom.Event (or MouseEvent) from a JS event object. The type,
// bubbles and cancelable properties are read; if clientX/clientY are present a
// MouseEventInit is used so coordinates survive the trip.
func jsToEvent(v jsc.JSValue) dom.Event {
	if !v.IsObject() {
		return nil
	}
	o := v.AsObject()
	typ := stringProp(o, "type")
	bubbles := boolProp(o, "bubbles")
	cancelable := boolProp(o, "cancelable")
	if hasNumber(o, "deltaY") || hasNumber(o, "deltaX") || hasNumber(o, "deltaMode") {
		init := dom.WheelEventInit{
			MouseEventInit: dom.MouseEventInit{
				EventInit: dom.EventInit{Bubbles: bubbles, Cancelable: cancelable},
				ClientX:   numProp(o, "clientX"),
				ClientY:   numProp(o, "clientY"),
				ScreenX:   numProp(o, "screenX"),
				ScreenY:   numProp(o, "screenY"),
				Button:    dom.MouseButton(int(numProp(o, "button"))),
				CtrlKey:   boolProp(o, "ctrlKey"),
				AltKey:    boolProp(o, "altKey"),
				ShiftKey:  boolProp(o, "shiftKey"),
				MetaKey:   boolProp(o, "metaKey"),
			},
			DeltaX:    numProp(o, "deltaX"),
			DeltaY:    numProp(o, "deltaY"),
			DeltaZ:    numProp(o, "deltaZ"),
			DeltaMode: dom.DeltaMode(int(numProp(o, "deltaMode"))),
		}
		return dom.NewWheelEventFromInit(typ, init)
	}
	if hasNumber(o, "clientX") || hasNumber(o, "clientY") {
		init := dom.MouseEventInit{
			EventInit: dom.EventInit{Bubbles: bubbles, Cancelable: cancelable},
			ClientX:   numProp(o, "clientX"),
			ClientY:   numProp(o, "clientY"),
			ScreenX:   numProp(o, "screenX"),
			ScreenY:   numProp(o, "screenY"),
			Button:    dom.MouseButton(int(numProp(o, "button"))),
			Buttons:   uint16(numProp(o, "buttons")),
			Detail:    int(numProp(o, "detail")),
			CtrlKey:   boolProp(o, "ctrlKey"),
			AltKey:    boolProp(o, "altKey"),
			ShiftKey:  boolProp(o, "shiftKey"),
			MetaKey:   boolProp(o, "metaKey"),
		}
		return dom.NewMouseEventFromInit(typ, init)
	}
	// ★ ToggleEvent（HTML §4.11.4）：页面自己 `new ToggleEvent(type, {oldState,
	// newState, source})` 再用 dispatchEvent 派发时，下面这条兜底会把三个字段
	// 全部丢掉——处理器里读到 undefined（而 `e.newState === "open"` 这种最常见的
	// 写法直接失效）。判定看构造器名（`new ToggleEvent` 会写入
	// constructor.name）或是否带了两个状态字段。
	if isToggleEventObject(o) {
		init := dom.ToggleEventInit{
			EventInit: dom.EventInit{Bubbles: bubbles, Cancelable: cancelable, Composed: boolProp(o, "composed")},
			OldState:  stringProp(o, "oldState"),
			NewState:  stringProp(o, "newState"),
		}
		if sv, ok := o.GetByKey("source"); ok {
			init.Source = jsElementValue(sv)
		}
		return dom.NewToggleEventFromInit(typ, init)
	}
	return dom.NewEvent(typ, bubbles, cancelable, false)
}

// isToggleEventObject 判断一个 JS 事件对象是否应还原成 dom.ToggleEvent：
// 构造器名是 ToggleEvent（`new ToggleEvent(...)` 的产物），或对象自带
// oldState / newState（页面手写的事件字面量）。
func isToggleEventObject(o *jsc.JSObject) bool {
	if o == nil {
		return false
	}
	if ctor, ok := o.GetByKey("constructor"); ok && ctor.IsObject() {
		if co := ctor.AsObject(); co != nil {
			if nv, ok := co.GetByKey("name"); ok && nv.ToString() == "ToggleEvent" {
				return true
			}
		}
	}
	if _, ok := o.GetByKey("oldState"); ok {
		return true
	}
	_, ok := o.GetByKey("newState")
	return ok
}

// --- property helpers on a JSObject ---

func stringProp(o *jsc.JSObject, name string) string {
	if v, ok := o.GetByKey(name); ok {
		return v.ToString()
	}
	return ""
}

func boolProp(o *jsc.JSObject, name string) bool {
	if v, ok := o.GetByKey(name); ok {
		return v.ToBoolean()
	}
	return false
}

func numProp(o *jsc.JSObject, name string) float64 {
	if v, ok := o.GetByKey(name); ok {
		return v.ToNumber()
	}
	return 0
}

func hasNumber(o *jsc.JSObject, name string) bool {
	v, ok := o.GetByKey(name)
	return ok && v.IsNumber()
}
