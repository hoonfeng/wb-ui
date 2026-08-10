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

	"wb-ui/dom"
	"wb-ui/jsc"
)

// listenerKey identifies a registered JS callback by its (event type, JS function
// identity). The function identity is the *JSFunction.String() (which is a stable
// pointer-based id like "js:0x..."), because AsFunction() creates a new *JSFunction
// each call so pointer identity is unreliable.
type listenerKey struct {
	eventType string
	fnID      string
}

// registeredListeners is the side-table mapping a listenerKey to the jsListener
// instances created for it. It exists because dom.EventTarget.RemoveEventListener
// matches listeners by Go identity, while JS callers only have the JS function value;
// the side-table lets us recover the Go listener for a given JS callback.
var registeredListeners = map[listenerKey][]*jsListener{}

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
	// 'this' for a bare function callback is undefined (matching addEventListener
	// semantics where the callback is invoked as a plain call, not a method).
	_, err := l.interp.Call(l.fn, jsc.Undefined(), []jsc.JSValue{ev})
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
			if len(args) < 2 {
				return jsc.Undefined()
			}
			eventType := args[0].ToString()
			jsFn := args[1]
			capture := false
			if len(args) >= 3 {
				capture = args[2].ToBoolean()
			}
			listener := &jsListener{interp: in, fn: jsFn}
			// dom.EventTarget.AddEventListener rejects an identical (callback, capture)
			// duplicate per the DOM spec; we ignore the result and still record the
			// listener so removal can find the active instance.
			target.AddEventListener(eventType, listener, capture)
			if jsFn.IsFunction() {
				key := listenerKey{eventType: eventType, fnID: jsFn.AsFunction().String()}
				registeredListeners[key] = append(registeredListeners[key], listener)
			}
			return jsc.Undefined()
		}, 2)
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
			key := listenerKey{eventType: eventType, fnID: args[1].AsFunction().String()}
			list := registeredListeners[key]
			for i, l := range list {
				if target.RemoveEventListener(eventType, l, capture) {
					registeredListeners[key] = append(list[:i], list[i+1:]...)
					break
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

// eventToJS converts a dom.Event to a JS object with the standard Event properties and
// methods (type/target/bubbles/cancelable/... plus preventDefault/stopPropagation).
// MouseEvent adds the coordinate/modifier fields.
func eventToJS(in *jsc.Interpreter, e dom.Event) jsc.JSValue {
	if e == nil {
		return jsc.Undefined()
	}
	obj := jsc.NewObject(in.ObjectPrototype())
	obj.SetClassName("Event")
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
					for ; n != nil; n = n.ParentNode() {
						path = append(path, nodeToJS(in, n))
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
			CtrlKey:   boolProp(o, "ctrlKey"),
			AltKey:    boolProp(o, "altKey"),
			ShiftKey:  boolProp(o, "shiftKey"),
			MetaKey:   boolProp(o, "metaKey"),
		}
		return dom.NewMouseEventFromInit(typ, init)
	}
	return dom.NewEvent(typ, bubbles, cancelable, false)
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
