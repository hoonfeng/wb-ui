package jsc

// browser_extras.go —— 其余浏览器构造器的最小可用实现。
//
// 背景（2026-10 三模式探针）：`File` / `FormData` / `EventSource` /
// `customElements` / `indexedDB` / `caches` / `Notification` /
// `BroadcastChannel` / `SharedWorker` 在引擎里全部不存在（typeof 为
// undefined），页面只要 `typeof X !== 'undefined'` 判断能力就会走错分支；
// 直接构造则抛 "X is not a constructor" 中断脚本。
//
// 口径：
//   - `File` / `FormData` 是真正可用的（与 Blob 同一字节承载约定）；
//   - `EventSource` / `customElements` / `BroadcastChannel` 提供完整的对象
//     形状与事件通路（不建真实网络流/跨标签通道，宿主可注入消息）；
//   - `indexedDB` / `caches` 的请求对象会**立即**以错误/空结果终结，绝不让
//     调用方永久挂起（await 场景下退回 fallback 而不是卡死）；
//   - `Notification` / `SharedWorker` 按"能力不可用"语义返回拒绝/空端口。

import (
	"errors"
	"strconv"
	"time"
)

// beArray 把 []beValue 转成 vm.NewArray 需要的 []interface{} 变参。
func beArray(items []beValue) []interface{} {
	out := make([]interface{}, len(items))
	for i, v := range items {
		out[i] = v
	}
	return out
}

// registerFile 注册 File（Blob 的超集：name / lastModified）。
func (r *Interpreter) registerFile() {
	r.vm.Set("File", func(call beConstructorCall) *beObject {
		obj := r.vm.NewObject()
		var data []byte
		if len(call.Arguments) >= 1 {
			if parts := call.Argument(0).ToObject(r.vm); parts != nil {
				if lv := parts.Get("length"); lv != nil && !beIsUndefined(lv) && !beIsNull(lv) {
					n := int(lv.ToInteger())
					for i := 0; i < n; i++ {
						data = append(data, r.blobPartBytes(parts.Get(strconv.Itoa(i)))...)
					}
				}
			}
		}
		name := ""
		if len(call.Arguments) >= 2 {
			a := call.Argument(1)
			if a != nil && !beIsUndefined(a) && !beIsNull(a) {
				name = a.String()
			}
		}
		mimeType := ""
		lastModified := time.Now().UnixMilli()
		if len(call.Arguments) >= 3 {
			if opts := call.Argument(2).ToObject(r.vm); opts != nil {
				if v := opts.Get("type"); v != nil && !beIsUndefined(v) && !beIsNull(v) {
					mimeType = v.String()
				}
				if v := opts.Get("lastModified"); v != nil && !beIsUndefined(v) && !beIsNull(v) {
					lastModified = v.ToInteger()
				}
			}
		}
		obj.Set("name", r.vm.ToValue(name))
		obj.Set("size", r.vm.ToValue(int64(len(data))))
		obj.Set("type", r.vm.ToValue(mimeType))
		obj.Set("lastModified", r.vm.ToValue(lastModified))
		obj.Set(blobDataKey, r.vm.ToValue(string(data)))
		obj.Set("text", r.vm.ToValue(func(_ beFunctionCall) beValue {
			return r.promiseResolved(r.vm.ToValue(string(data)))
		}))
		obj.Set("arrayBuffer", r.vm.ToValue(func(_ beFunctionCall) beValue {
			return r.promiseResolved(r.uint8ArrayValue(data))
		}))
		return obj
	})
}

// registerFormData 注册 FormData（真正的键值集合；值可以是字符串或 Blob/File）。
func (r *Interpreter) registerFormData() {
	r.vm.Set("FormData", func(call beConstructorCall) *beObject {
		obj := r.vm.NewObject()
		// entries 的每个元素是 [name, value] 二元数组（保持插入顺序）。
		var entries []beValue

		findName := func(pair beValue, name string) bool {
			o := pair.ToObject(r.vm)
			if o == nil {
				return false
			}
			nv := o.Get("0")
			return nv != nil && !beIsUndefined(nv) && !beIsNull(nv) && nv.String() == name
		}
		valueOf := func(pair beValue) beValue {
			o := pair.ToObject(r.vm)
			if o == nil {
				return beUndefined()
			}
			return o.Get("1")
		}

		obj.Set("append", r.vm.ToValue(func(c beFunctionCall) beValue {
			name := c.Argument(0).String()
			entries = append(entries, r.vm.NewArray([]interface{}{r.vm.ToValue(name), c.Argument(1)}...))
			return beUndefined()
		}))
		obj.Set("set", r.vm.ToValue(func(c beFunctionCall) beValue {
			name := c.Argument(0).String()
			kept := entries[:0]
			for _, e := range entries {
				if findName(e, name) {
					continue
				}
				kept = append(kept, e)
			}
			entries = kept
			entries = append(entries, r.vm.NewArray([]interface{}{r.vm.ToValue(name), c.Argument(1)}...))
			return beUndefined()
		}))
		obj.Set("delete", r.vm.ToValue(func(c beFunctionCall) beValue {
			name := c.Argument(0).String()
			kept := entries[:0]
			for _, e := range entries {
				if findName(e, name) {
					continue
				}
				kept = append(kept, e)
			}
			entries = kept
			return beUndefined()
		}))
		obj.Set("get", r.vm.ToValue(func(c beFunctionCall) beValue {
			name := c.Argument(0).String()
			for _, e := range entries {
				if findName(e, name) {
					return valueOf(e)
				}
			}
			return beNull()
		}))
		obj.Set("getAll", r.vm.ToValue(func(c beFunctionCall) beValue {
			name := c.Argument(0).String()
			var out []beValue
			for _, e := range entries {
				if findName(e, name) {
					out = append(out, valueOf(e))
				}
			}
			return r.vm.ToValue(r.vm.NewArray(beArray(out)...))
		}))
		obj.Set("has", r.vm.ToValue(func(c beFunctionCall) beValue {
			name := c.Argument(0).String()
			for _, e := range entries {
				if findName(e, name) {
					return r.vm.ToValue(true)
				}
			}
			return r.vm.ToValue(false)
		}))
		obj.Set("forEach", r.vm.ToValue(func(c beFunctionCall) beValue {
			fn, ok := beAssertFunction(c.Argument(0))
			if !ok {
				return beUndefined()
			}
			for _, e := range entries {
				o := e.ToObject(r.vm)
				if o == nil {
					continue
				}
				_, _ = fn(obj, o.Get("1"), o.Get("0"), obj)
			}
			return beUndefined()
		}))
		obj.Set("entries", r.vm.ToValue(func(_ beFunctionCall) beValue {
			out := make([]beValue, 0, len(entries))
			for _, e := range entries {
				out = append(out, e)
			}
			return r.vm.NewArray(beArray(out)...)
		}))
		obj.Set("keys", r.vm.ToValue(func(_ beFunctionCall) beValue {
			out := make([]beValue, 0, len(entries))
			for _, e := range entries {
				o := e.ToObject(r.vm)
				if o != nil {
					out = append(out, o.Get("0"))
				}
			}
			return r.vm.NewArray(beArray(out)...)
		}))
		obj.Set("values", r.vm.ToValue(func(_ beFunctionCall) beValue {
			out := make([]beValue, 0, len(entries))
			for _, e := range entries {
				o := e.ToObject(r.vm)
				if o != nil {
					out = append(out, o.Get("1"))
				}
			}
			return r.vm.NewArray(beArray(out)...)
		}))
		return obj
	})
}

// newEventTargetObject 构造一个带 addEventListener/removeEventListener 的最小
// 事件目标，返回对象与派发函数（供 stub 类构造器复用）。
func (r *Interpreter) newEventTargetObject() (*beObject, func(evType string, extra map[string]beValue)) {
	obj := r.vm.NewObject()
	ls := map[string][]beValue{}
	obj.Set("addEventListener", r.vm.ToValue(func(c beFunctionCall) beValue {
		if v := c.Argument(1); v != nil {
			if _, ok := beAssertFunction(v); ok {
				t := c.Argument(0).String()
				ls[t] = append(ls[t], v)
			}
		}
		return beUndefined()
	}))
	obj.Set("removeEventListener", r.vm.ToValue(func(c beFunctionCall) beValue {
		t := c.Argument(0).String()
		cur := ls[t]
		kept := cur[:0]
		for _, fn := range cur {
			// 监听器是同一个函数对象时移除（goja 的函数值是可比较的指针）。
			if fn == c.Argument(1) {
				continue
			}
			kept = append(kept, fn)
		}
		ls[t] = kept
		return beUndefined()
	}))
	dispatch := func(evType string, extra map[string]beValue) {
		ev := r.vm.NewObject()
		ev.Set("type", r.vm.ToValue(evType))
		ev.Set("target", obj)
		ev.Set("currentTarget", obj)
		for k, v := range extra {
			ev.Set(k, v)
		}
		// on<type> 内容属性
		if h := obj.Get("on" + evType); h != nil && !beIsUndefined(h) && !beIsNull(h) {
			if fn, ok := beAssertFunction(h); ok {
				_, _ = fn(obj, ev)
			}
		}
		for _, fn := range ls[evType] {
			if fn2, ok := beAssertFunction(fn); ok {
				_, _ = fn2(obj, ev)
			}
		}
	}
	return obj, dispatch
}

// registerEventSource 注册 EventSource（SSE）。引擎不建真实网络流，但对象形状、
// 事件通路与 close() 完整；宿主可通过 `__dispatch(type, data)` 注入服务端消息。
func (r *Interpreter) registerEventSource() {
	r.vm.Set("EventSource", func(call beConstructorCall) *beObject {
		obj, dispatch := r.newEventTargetObject()
		url := ""
		if len(call.Arguments) >= 1 {
			url = call.Argument(0).String()
		}
		// 常量（构造器与实例两侧都可读，浏览器行为）。
		for name, v := range map[string]int64{"CONNECTING": 0, "OPEN": 1, "CLOSED": 2} {
			obj.Set(name, r.vm.ToValue(v))
		}
		obj.Set("url", r.vm.ToValue(url))
		obj.Set("readyState", r.vm.ToValue(int64(0))) // CONNECTING：宿主接线后才 open
		obj.Set("withCredentials", r.vm.ToValue(false))
		obj.Set("close", r.vm.ToValue(func(_ beFunctionCall) beValue {
			obj.Set("readyState", r.vm.ToValue(int64(2)))
			return beUndefined()
		}))
		// 宿主注入口：把一条 SSE 消息派发给 onmessage/addEventListener("message")。
		obj.Set("__dispatch", r.vm.ToValue(func(c beFunctionCall) beValue {
			extra := map[string]beValue{"data": c.Argument(1)}
			dispatch(c.Argument(0).String(), extra)
			return beUndefined()
		}))
		return obj
	})
}

// registerBrowserStubs 注册其余浏览器对象（能力不可用但形状完整）。
func (r *Interpreter) registerBrowserStubs() {
	// ── customElements ──
	defined := r.vm.NewObject()
	customElements := r.vm.NewObject()
	customElements.Set("define", r.vm.ToValue(func(c beFunctionCall) beValue {
		defined.Set(c.Argument(0).String(), c.Argument(1))
		return beUndefined()
	}))
	customElements.Set("get", r.vm.ToValue(func(c beFunctionCall) beValue {
		v := defined.Get(c.Argument(0).String())
		if v == nil || beIsUndefined(v) {
			return beUndefined()
		}
		return v
	}))
	customElements.Set("whenDefined", r.vm.ToValue(func(c beFunctionCall) beValue {
		v := defined.Get(c.Argument(0).String())
		if v == nil || beIsUndefined(v) {
			return r.promiseResolved(beUndefined())
		}
		return r.promiseResolved(v)
	}))
	customElements.Set("upgrade", r.vm.ToValue(func(_ beFunctionCall) beValue { return beUndefined() }))
	r.vm.Set("customElements", customElements)

	// ── indexedDB（请求对象立即以 error 终结，绝不挂起）──
	idb := r.vm.NewObject()
	makeRequest := func(errMsg string, result beValue) *beObject {
		req, dispatch := r.newEventTargetObject()
		req.Set("readyState", r.vm.ToValue("done"))
		req.Set("result", result)
		req.Set("error", r.vm.NewGoError(errors.New(errMsg)))
		if errMsg != "" {
			dispatch("error", nil)
		} else {
			dispatch("success", nil)
		}
		return req
	}
	idb.Set("open", r.vm.ToValue(func(_ beFunctionCall) beValue {
		return makeRequest("IndexedDB 未实现（wb-ui 引擎）", beNull())
	}))
	idb.Set("deleteDatabase", r.vm.ToValue(func(_ beFunctionCall) beValue {
		return makeRequest("IndexedDB 未实现（wb-ui 引擎）", beNull())
	}))
	idb.Set("databases", r.vm.ToValue(func(_ beFunctionCall) beValue {
		return r.promiseResolved(r.vm.NewArray())
	}))
	r.vm.Set("indexedDB", idb)

	// ── caches（CacheStorage）：Promise 立即以空结果 resolve ──
	caches := r.vm.NewObject()
	newCacheObject := func() *beObject {
		c := r.vm.NewObject()
		c.Set("match", r.vm.ToValue(func(_ beFunctionCall) beValue { return r.promiseResolved(beNull()) }))
		c.Set("matchAll", r.vm.ToValue(func(_ beFunctionCall) beValue {
			return r.promiseResolved(r.vm.NewArray())
		}))
		c.Set("put", r.vm.ToValue(func(_ beFunctionCall) beValue { return r.promiseResolved(beUndefined()) }))
		c.Set("add", r.vm.ToValue(func(_ beFunctionCall) beValue { return r.promiseResolved(beUndefined()) }))
		c.Set("addAll", r.vm.ToValue(func(_ beFunctionCall) beValue { return r.promiseResolved(beUndefined()) }))
		c.Set("delete", r.vm.ToValue(func(_ beFunctionCall) beValue { return r.promiseResolved(r.vm.ToValue(true)) }))
		c.Set("keys", r.vm.ToValue(func(_ beFunctionCall) beValue { return r.promiseResolved(r.vm.NewArray()) }))
		return c
	}
	caches.Set("open", r.vm.ToValue(func(_ beFunctionCall) beValue {
		return r.promiseResolved(newCacheObject())
	}))
	caches.Set("match", r.vm.ToValue(func(_ beFunctionCall) beValue { return r.promiseResolved(beNull()) }))
	caches.Set("has", r.vm.ToValue(func(_ beFunctionCall) beValue { return r.promiseResolved(r.vm.ToValue(false)) }))
	caches.Set("delete", r.vm.ToValue(func(_ beFunctionCall) beValue { return r.promiseResolved(r.vm.ToValue(false)) }))
	caches.Set("keys", r.vm.ToValue(func(_ beFunctionCall) beValue { return r.promiseResolved(r.vm.NewArray()) }))
	r.vm.Set("caches", caches)

	// ── Notification：权限恒 denied（桌面壳不申请系统通知） ──
	notifCtor := r.vm.ToValue(func(call beConstructorCall) *beObject {
		obj, _ := r.newEventTargetObject()
		obj.Set("title", call.Argument(0))
		obj.Set("body", r.vm.ToValue(""))
		obj.Set("close", r.vm.ToValue(func(_ beFunctionCall) beValue { return beUndefined() }))
		return obj
	})
	if o := notifCtor.ToObject(r.vm); o != nil {
		o.Set("permission", r.vm.ToValue("denied"))
		o.Set("requestPermission", r.vm.ToValue(func(_ beFunctionCall) beValue {
			return r.promiseResolved(r.vm.ToValue("denied"))
		}))
		o.Set("maxActions", r.vm.ToValue(int64(0)))
	}
	r.vm.Set("Notification", notifCtor)

	// ── BroadcastChannel：同解释器内同名频道互投（跨标签页不属于单引擎语义）──
	channels := map[string][]*beObject{}
	r.vm.Set("BroadcastChannel", func(call beConstructorCall) *beObject {
		obj, _ := r.newEventTargetObject()
		name := ""
		if len(call.Arguments) >= 1 && call.Argument(0) != nil {
			name = call.Argument(0).String()
		}
		obj.Set("name", r.vm.ToValue(name))
		obj.Set("onmessage", beNull())
		obj.Set("onmessageerror", beNull())
		channels[name] = append(channels[name], obj)
		obj.Set("postMessage", r.vm.ToValue(func(c beFunctionCall) beValue {
			for _, other := range channels[name] {
				if other == obj {
					continue
				}
				ev := r.vm.NewObject()
				ev.Set("type", r.vm.ToValue("message"))
				ev.Set("data", c.Argument(0))
				ev.Set("target", other)
				if h := other.Get("onmessage"); h != nil && !beIsUndefined(h) && !beIsNull(h) {
					if fn, ok := beAssertFunction(h); ok {
						_, _ = fn(other, ev)
					}
				}
			}
			return beUndefined()
		}))
		obj.Set("close", r.vm.ToValue(func(_ beFunctionCall) beValue {
			kept := channels[name][:0]
			for _, ch := range channels[name] {
				if ch != obj {
					kept = append(kept, ch)
				}
			}
			channels[name] = kept
			return beUndefined()
		}))
		return obj
	})

	// ── SharedWorker：返回带 port 的对象（postMessage 丢弃，不抛错）──
	r.vm.Set("SharedWorker", func(call beConstructorCall) *beObject {
		obj, _ := r.newEventTargetObject()
		port, _ := r.newEventTargetObject()
		port.Set("onmessage", beNull())
		port.Set("postMessage", r.vm.ToValue(func(_ beFunctionCall) beValue { return beUndefined() }))
		port.Set("start", r.vm.ToValue(func(_ beFunctionCall) beValue { return beUndefined() }))
		port.Set("close", r.vm.ToValue(func(_ beFunctionCall) beValue { return beUndefined() }))
		obj.Set("port", port)
		obj.Set("onerror", beNull())
		return obj
	})
}
