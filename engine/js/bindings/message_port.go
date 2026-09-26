package bindings

import (
	"wb-ui/engine/js/jsc"
)

// MessageChannel / MessagePort —— 消息通道（此前记录于 docs/TECH_DEBT.md 的缺口）。
//
// 为什么重要：React（react-dom 的 Scheduler）把 MessageChannel 当作**宏任务源**
// 用于时间切片调度，检测顺序为 isInputPending → setImmediate → MessageChannel
// → setTimeout；前两者缺失时调度粒度退化为 setTimeout(0)，交互响应变差。
//
// 语义要点：
//  1. postMessage 永不与调用同步执行 —— 必须排一个宏任务（这里复用 EventLoop 的
//     immediate 队列：它先于到期定时器执行，与 MessageChannel 的「宏任务」定位一致）。
//  2. 消息在 port 启动前排队 —— 收消息时若对端既无 onmessage 也无 'message'
//     监听器，则压入 pending，等 start() 或 handler 就绪后补发。这样「先赋
//     onmessage、再 postMessage」（React 的写法）自然生效，无需拦截属性赋值
//     （obj.Set 是 Go 侧直写，没有属性访问器钩子）。

// messagePortState 是 MessagePort 的 Go 侧状态；同一 MessageChannel 的两端
// 通过 peer 互相引用。
type messagePortState struct {
	peer      *messagePortState
	obj       *jsc.JSObject
	started   bool
	closed    bool
	listeners []jsc.JSValue
	pending   []jsc.JSValue
}

// hasHandler 报告本 port 是否已有消息处理者（'message' 监听器或 onmessage 属性）。
// 规范中给 onmessage 赋值会隐式 start()；这里用「投递时动态检查」达到同一效果。
func (st *messagePortState) hasHandler() bool {
	if len(st.listeners) > 0 {
		return true
	}
	if st.obj == nil {
		return false
	}
	if v, ok := st.obj.GetByKey("onmessage"); ok && v.IsCallable() {
		return true
	}
	return false
}

// deliverMessage 在主线程向本 port 派发一条 message 事件（onmessage + 监听器）。
func (st *messagePortState) deliverMessage(in *jsc.Interpreter, data jsc.JSValue) {
	if st.closed || st.obj == nil {
		return
	}
	this := jsc.ObjectValue(st.obj)
	ev := jsc.NewObject(in.ObjectPrototype())
	ev.SetClassName("MessageEvent")
	ev.Set("type", jsc.StringValue("message"))
	ev.Set("data", data)
	ev.Set("origin", jsc.StringValue(""))
	ev.Set("lastEventId", jsc.StringValue(""))
	ev.Set("source", jsc.Null())
	ev.Set("ports", jsc.ObjectValue(jsc.NewArrayForInterp(in, nil)))
	ev.Set("target", this)
	ev.Set("currentTarget", this)
	evVal := jsc.ObjectValue(ev)

	if v, ok := st.obj.GetByKey("onmessage"); ok && v.IsCallable() {
		_, _ = in.Call(v, this, []jsc.JSValue{evVal})
	}
	for _, fn := range st.listeners {
		if fn.IsCallable() {
			_, _ = in.Call(fn, this, []jsc.JSValue{evVal})
		}
	}
}

// postMessage 把数据异步投递到对端 port。
func (st *messagePortState) postMessage(in *jsc.Interpreter, data jsc.JSValue) {
	peer := st.peer
	if st.closed || peer == nil || peer.closed {
		return // 规范：closed port 上的消息被丢弃（不抛错）
	}
	el := in.EnsureEventLoop()
	el.SetImmediate(jsc.FunctionValue(jsc.NewNativeFunction("onMessage",
		func(in2 *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			if peer.hasHandler() {
				peer.deliverMessage(in2, data)
			} else {
				peer.pending = append(peer.pending, data)
			}
			return jsc.Undefined()
		}, 0)))
}

// start 启动端口：补发启动前排队的消息。
func (st *messagePortState) start(in *jsc.Interpreter) {
	if st.started || st.closed {
		return
	}
	st.started = true
	pending := st.pending
	st.pending = nil
	for _, d := range pending {
		st.deliverMessage(in, d)
	}
}

// close 关闭端口：不再收发消息（规范还会向对端派发 close 事件，此处从简）。
func (st *messagePortState) close() {
	st.closed = true
	st.pending = nil
	st.listeners = nil
}

// newMessagePortObject 构造 MessagePort JS 对象并接线到 st。
func newMessagePortObject(in *jsc.Interpreter, st *messagePortState) *jsc.JSObject {
	obj := jsc.NewObject(in.ObjectPrototype())
	obj.SetClassName("MessagePort")
	st.obj = obj

	obj.Set("onmessage", jsc.Null())
	obj.Set("onmessageerror", jsc.Null())

	obj.Set("postMessage", jsc.FunctionValue(jsc.NewNativeFunction("postMessage",
		func(in2 *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			var data jsc.JSValue = jsc.Undefined()
			if len(args) >= 1 {
				data = args[0]
			}
			st.postMessage(in2, data)
			return jsc.Undefined()
		}, 1)))

	obj.Set("start", jsc.FunctionValue(jsc.NewNativeFunction("start",
		func(in2 *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			st.start(in2)
			return jsc.Undefined()
		}, 0)))

	obj.Set("close", jsc.FunctionValue(jsc.NewNativeFunction("close",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			st.close()
			return jsc.Undefined()
		}, 0)))

	obj.Set("addEventListener", jsc.FunctionValue(jsc.NewNativeFunction("addEventListener",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) >= 2 && args[1].IsCallable() && args[0].ToString() == "message" {
				st.listeners = append(st.listeners, args[1])
			}
			return jsc.Undefined()
		}, 2)))

	obj.Set("removeEventListener", jsc.FunctionValue(jsc.NewNativeFunction("removeEventListener",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 2 {
				return jsc.Undefined()
			}
			out := st.listeners[:0]
			for _, fn := range st.listeners {
				if !fn.SameAs(args[1]) {
					out = append(out, fn)
				}
			}
			st.listeners = out
			return jsc.Undefined()
		}, 2)))

	return obj
}

// installMessageChannel 注册 MessageChannel（以及 MessagePort 构造器）。
func installMessageChannel(rt *jsc.Interpreter, g *jsc.JSObject) {
	g.Set("MessageChannel", jsc.FunctionValue(rt.NewConstructor("MessageChannel",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) *jsc.JSObject {
			p1 := &messagePortState{}
			p2 := &messagePortState{}
			p1.peer = p2
			p2.peer = p1
			ch := jsc.NewObject(in.ObjectPrototype())
			ch.SetClassName("MessageChannel")
			ch.Set("port1", jsc.ObjectValue(newMessagePortObject(in, p1)))
			ch.Set("port2", jsc.ObjectValue(newMessagePortObject(in, p2)))
			return ch
		})))

	// MessagePort：规范上不可直接构造（Illegal constructor），但注册它才能支持
	// `typeof MessagePort === 'function'` / `x instanceof MessagePort` 式特性检测。
	g.Set("MessagePort", jsc.FunctionValue(rt.NewConstructor("MessagePort",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) *jsc.JSObject {
			return newMessagePortObject(in, &messagePortState{})
		})))
}
