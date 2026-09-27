package page

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"wb-ui/bridge"
	"wb-ui/engine/dom"
	"wb-ui/engine/js/jsc"
)

// RegisterFetch registers the global fetch() function on the JS interpreter.
// RegisterFetch 注册 fetch（允许真实网络回退，即嵌入浏览器语义）。
func RegisterFetch(rt *jsc.Interpreter) {
	RegisterFetchWithPolicy(rt, true)
}

// RegisterFetchWithPolicy 注册 fetch。allowNetwork=false 时 fetch 只命中
// 宿主注册的 bridge 路由（UI 取数据用），无匹配路由则 reject——绝不发起
// 真实网络请求。适配 UI 库模式：无隐式外部输入，且引擎的 fetch 是同步
// 实现（阻塞 UI 线程），对 UI 宿主是危险操作。
func RegisterFetchWithPolicy(rt *jsc.Interpreter, allowNetwork bool) {
	fetchFn := jsc.NewNativeFunction("fetch", func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) < 1 {
			return rejectPromise(in, fmt.Errorf("fetch: missing url argument"))
		}
		raw := jscToString(args[0])
		if raw == "" {
			return rejectPromise(in, fmt.Errorf("fetch: url must be a non-empty string"))
		}
		// ★ 相对 URL 以「文档 URL」为基准解析（浏览器语义）：真实页面几乎
		//   都写 fetch("/api/x")，原样交给 http.NewRequest 会直接失败
		//   （unsupported protocol scheme ""）。无文档 URL（LoadHTML 直出
		//   的内容）时原样返回，行为不变。
		url := dom.ResolveURL(DocumentBase(in), raw)

		method := "GET"
		if len(args) >= 2 && args[1].IsObject() {
			if o := args[1].AsObject(); o != nil {
				if m, ok := o.GetByKey("method"); ok && !m.IsUndefined() && !m.IsNull() {
					method = jscToString(m)
				}
			}
		}

		// Check bridge routes first (GUI-mode API interception).
		// If the URL matches a registered Go handler, call it directly
		// instead of making an HTTP request. Method-aware matching: a route
		// registered with a specific method only intercepts that method.
		//
		// 匹配顺序：先按脚本写下的原样 URL（既有行为：宿主按 "/api/x"
		// 注册路由），未命中再按解析后的绝对 URL 匹配（宿主把路由注册成
		// 绝对 URL 的场景）。
		matchRoute := func(u string) *bridge.Route {
			if r := bridge.MatchMethod("", u); r != nil {
				return r
			}
			return bridge.MatchMethod(method, u)
		}
		route := matchRoute(raw)
		if route == nil && url != raw {
			route = matchRoute(url)
		}
		if route != nil {
			return bridgeFetch(in, args, url, route)
		}
		if !allowNetwork {
			return rejectPromise(in, fmt.Errorf("fetch(%s): 网络请求在 UI 库模式下被禁用（只有宿主注册的桥路由可用）", raw))
		}

		var body io.Reader
		headers := http.Header{}
		if len(args) >= 2 {
			opts := args[1]
			if opts.IsObject() {
				o := opts.AsObject()
				if o != nil {
					if b, ok := o.GetByKey("body"); ok && !b.IsUndefined() && !b.IsNull() {
						body = strings.NewReader(jscToString(b))
					}
					if h, ok := o.GetByKey("headers"); ok && h.IsObject() {
						// ★ AbortSignal 支持（AbortController.signal）：引擎 fetch 是同步实现，
						//   无法中途取消，但**已中止**的信号必须立即拒绝（调用方 opts.signal
						//   级联取消 / AbortSignal.abort() 场景），否则请求照发、语义错。
						if sg, ok := o.GetByKey("signal"); ok && sg.IsObject() {
							if so := sg.AsObject(); so != nil {
								if ab, ok2 := so.GetByKey("aborted"); ok2 && ab.ToBoolean() {
									return rejectPromise(in, fmt.Errorf("fetch: request aborted"))
								}
							}
						}
						hObj := h.AsObject()
						if hObj != nil {
							for _, key := range hObj.Keys() {
								if val, ok2 := hObj.GetByKey(key); ok2 {
									headers.Set(key, jscToString(val))
								}
							}
						}
					}
				}
			}
		}
		req, err := http.NewRequest(method, url, body)
		if err != nil {
			return rejectPromise(in, fmt.Errorf("fetch: invalid request: %w", err))
		}
		req.Header = headers
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return rejectPromise(in, fmt.Errorf("fetch: request failed: %w", err))
		}
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return rejectPromise(in, fmt.Errorf("fetch: read response failed: %w", err))
		}
		respObj := jsc.NewObject(in.ObjectPrototype())
		respObj.SetClassName("Response")
		respObj.Set("status", jsc.NumberValue(float64(resp.StatusCode)))
		respObj.Set("ok", jsc.BooleanValue(resp.StatusCode >= 200 && resp.StatusCode < 300))
		respObj.Set("statusText", jsc.StringValue(resp.Status))
		respObj.Set("url", jsc.StringValue(url))
		bodyText := string(respBody)
		textFn := jsc.NewNativeFunction("text", func(in2 *jsc.Interpreter, this2 jsc.JSValue, args2 []jsc.JSValue) jsc.JSValue {
			return resolvePromise(in2, jsc.StringValue(bodyText))
		}, 0)
		respObj.Set("text", jsc.FunctionValue(textFn))
		jsonFn := jsc.NewNativeFunction("json", func(in2 *jsc.Interpreter, this2 jsc.JSValue, args2 []jsc.JSValue) jsc.JSValue {
			// ★ 与 bridgeFetch 同一实现：Go 侧 encoding/json 解析 → ToJSValue
			//   直转 JS 对象/数组。绝不能用 Run/RunJS 把响应体当 JS 代码执行：
			//   JSON 文本 {"rev":…} 处于语句位置会被读成「块 + 标签语句」→
			//   SyntaxError: Unexpected token :（实测 /api/ui-boot 即死于此）；
			//   即便写成表达式（[…]），旧代码还会用 fmt.Sprintf("%v") 把结果
			//   转成**字符串**返回 —— 调用方 `await res.json()` 拿到字符串而非
			//   对象/数组，前端装配链路（gou-ide：/api/ui-boot → 区域包 client
			//   半装载）整条断掉，槽位全部空态占位。
			var parsed any
			if err := json.Unmarshal([]byte(bodyText), &parsed); err != nil {
				return rejectPromise(in2, fmt.Errorf("fetch: json parse failed: %w", err))
			}
			// ★ 必须在**当前解释器**里构造：bindings.ToJSValue 内部走
			//   jsc.NewObject(nil)，proto 为 nil 时会新建一个独立 goja runtime，
			//   返回的对象属于另一个 runtime → goja 抛
			//   "Illegal runtime transition of an Object"。解释器自带的 ValueOf
			//   用本 runtime 的 vm.NewObject/NewArray 构造，且覆盖 json.Unmarshal
			//   的全部产出类型（nil/bool/float64/string/[]any/map[string]any）。
			return resolvePromise(in2, in2.ValueOf(parsed))
		}, 0)
		respObj.Set("json", jsc.FunctionValue(jsonFn))
		// Response.blob()：字节以内部字段 __wbBlobData 承载（与 jsc 包 registerBlob/
		// registerFileReader 约定一致），FileReader.readAsDataURL 直接消费。
		blobFn := jsc.NewNativeFunction("blob", func(in2 *jsc.Interpreter, this2 jsc.JSValue, args2 []jsc.JSValue) jsc.JSValue {
			bo := jsc.NewObject(in2.ObjectPrototype())
			bo.Set("size", jsc.NumberValue(float64(len(respBody))))
			bo.Set("type", jsc.StringValue(resp.Header.Get("Content-Type")))
			bo.Set("__wbBlobData", jsc.StringValue(bodyText))
			bo.Set("text", jsc.FunctionValue(jsc.NewNativeFunction("text",
				func(in3 *jsc.Interpreter, t3 jsc.JSValue, a3 []jsc.JSValue) jsc.JSValue {
					return resolvePromise(in3, jsc.StringValue(bodyText))
				}, 0)))
			bo.Set("arrayBuffer", jsc.FunctionValue(jsc.NewNativeFunction("arrayBuffer",
				func(in3 *jsc.Interpreter, t3 jsc.JSValue, a3 []jsc.JSValue) jsc.JSValue {
					return resolvePromise(in3, jsc.StringValue(bodyText))
				}, 0)))
			return resolvePromise(in2, jsc.ObjectValue(bo))
		}, 0)
		respObj.Set("blob", jsc.FunctionValue(blobFn))
		headersObj := jsc.NewObject(in.ObjectPrototype())
		for k, vs := range resp.Header {
			headersObj.Set(k, jsc.StringValue(strings.Join(vs, ", ")))
		}
		respObj.Set("headers", jsc.ObjectValue(headersObj))
		return resolvePromise(in, jsc.ObjectValue(respObj))
	}, 1)
	rt.GlobalObject().Set("fetch", jsc.FunctionValue(fetchFn))
}

// RegisterXMLHttpRequest registers the XMLHttpRequest constructor on the JS interpreter.
func RegisterXMLHttpRequest(rt *jsc.Interpreter) {
	proto := jsc.NewObject(rt.ObjectPrototype())
	proto.SetClassName("XMLHttpRequestPrototype")
	proto.Set("UNSENT", jsc.NumberValue(0))
	proto.Set("OPENED", jsc.NumberValue(1))
	proto.Set("HEADERS_RECEIVED", jsc.NumberValue(2))
	proto.Set("LOADING", jsc.NumberValue(3))
	proto.Set("DONE", jsc.NumberValue(4))

	// ─── 事件监听（2026-09-27 补齐）─────────────────────────────────
	// 浏览器里 `onload` 属性与 `addEventListener("load")` 两条通路**互不替代**，
	// 都要触发。此前 wb-ui 只认 onreadystatechange，前端最标准的
	// `xhr.onload = function(){ resolve(...) }` 写法因此永远停在 pending。
	addEventListenerFn := jsc.NewNativeFunction("addEventListener", func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if this.IsObject() && len(args) >= 2 {
			if o := this.AsObject(); o != nil {
				xhrListenersAdd(o, jscToString(args[0]), args[1])
			}
		}
		return jsc.Undefined()
	}, 2)
	proto.Set("addEventListener", jsc.FunctionValue(addEventListenerFn))

	removeEventListenerFn := jsc.NewNativeFunction("removeEventListener", func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if this.IsObject() && len(args) >= 2 {
			if o := this.AsObject(); o != nil {
				xhrListenersRemove(o, jscToString(args[0]), args[1])
			}
		}
		return jsc.Undefined()
	}, 2)
	proto.Set("removeEventListener", jsc.FunctionValue(removeEventListenerFn))

	openFn := jsc.NewNativeFunction("open", func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if !this.IsObject() {
			return jsc.Undefined()
		}
		obj := this.AsObject()
		if len(args) >= 1 {
			method := strings.ToUpper(jscToString(args[0]))
			obj.Set("_method", jsc.StringValue(method))
		}
		if len(args) >= 2 {
			obj.Set("_url", args[1])
		}
		if len(args) >= 3 {
			obj.Set("_async", args[2])
		} else {
			obj.Set("_async", jsc.BooleanValue(true))
		}
		obj.Set("readyState", jsc.NumberValue(1))
		obj.Set("_requestHeaders", jsc.ObjectValue(jsc.NewObject(in.ObjectPrototype())))
		xhrFireReadyStateChange(in, obj)
		return jsc.Undefined()
	}, 5)
	proto.Set("open", jsc.FunctionValue(openFn))

	setReqHeaderFn := jsc.NewNativeFunction("setRequestHeader", func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if !this.IsObject() || len(args) < 2 {
			return jsc.Undefined()
		}
		obj := this.AsObject()
		hv, ok := obj.GetByKey("_requestHeaders")
		if ok && hv.IsObject() {
			hObj := hv.AsObject()
			name := jscToString(args[0])
			val := jscToString(args[1])
			hObj.Set(name, jsc.StringValue(val))
		}
		return jsc.Undefined()
	}, 2)
	proto.Set("setRequestHeader", jsc.FunctionValue(setReqHeaderFn))

	sendFn := jsc.NewNativeFunction("send", func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if !this.IsObject() {
			return jsc.Undefined()
		}
		obj := this.AsObject()
		method := jscToString2(obj, "_method")
		// ★ 相对 URL 以文档 URL 为基准解析（与 fetch 同一规则）。
		rawURL := jscToString2(obj, "_url")
		url := dom.ResolveURL(DocumentBase(in), rawURL)
		if method == "" {
			method = "GET"
		}
		var bodyStr string
		if len(args) >= 1 && !args[0].IsUndefined() && !args[0].IsNull() {
			bodyStr = jscToString(args[0])
		}
		var body io.Reader
		if bodyStr != "" {
			body = strings.NewReader(bodyStr)
		}
		headers := http.Header{}
		hv, ok := obj.GetByKey("_requestHeaders")
		if ok && hv.IsObject() {
			hObj := hv.AsObject()
			for _, key := range hObj.Keys() {
				if val, ok2 := hObj.GetByKey(key); ok2 {
					headers.Set(key, jscToString(val))
				}
			}
		}
		// ★ bridge 路由拦截（2026-09-27）：与 fetch 完全同规则。桌面壳里宿主
		//   API 全部由 bridge 提供（没有本地 HTTP 服务），此前 XHR 直接发真实
		//   请求 → 必然失败，且失败也不触发 onerror → 调用方永远 pending。
		obj.Set("_sent", jsc.BooleanValue(true))
		matchRoute := func(u string) *bridge.Route {
			if r := bridge.MatchMethod("", u); r != nil {
				return r
			}
			return bridge.MatchMethod(method, u)
		}
		route := matchRoute(rawURL)
		if route == nil && url != rawURL {
			route = matchRoute(url)
		}
		if route != nil {
			status, bodyText := xhrBridgeCall(in, route, rawURL, method, bodyStr, headers)
			obj.Set("readyState", jsc.NumberValue(2))
			xhrFireReadyStateChange(in, obj)
			obj.Set("readyState", jsc.NumberValue(3))
			xhrFireReadyStateChange(in, obj)
			xhrSetResponse(in, obj, status, bodyText)
			obj.Set("readyState", jsc.NumberValue(4))
			xhrFireReadyStateChange(in, obj)
			xhrDispatch(in, obj, "load")
			xhrDispatch(in, obj, "loadend")
			return jsc.Undefined()
		}

		obj.Set("readyState", jsc.NumberValue(3))
		xhrFireReadyStateChange(in, obj)
		req, err := http.NewRequest(method, url, body)
		if err != nil {
			return xhrFail(in, obj, err)
		}
		req.Header = headers
		// ★ timeout 属性真正生效（2026-09-27）：此前是死属性——前端写
		//   `xhr.timeout = 8000` 期待 ontimeout，实际会无限等下去。
		client := &http.Client{}
		if tmo := xhrNumber2(obj, "timeout"); tmo > 0 {
			client.Timeout = time.Duration(tmo) * time.Millisecond
		}
		resp, err := client.Do(req)
		if err != nil {
			return xhrFail(in, obj, err)
		}
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return xhrFail(in, obj, err)
		}
		obj.Set("status", jsc.NumberValue(float64(resp.StatusCode)))
		var hdrLines []string
		for k, vs := range resp.Header {
			hdrLines = append(hdrLines, fmt.Sprintf("%s: %s", k, strings.Join(vs, ", ")))
		}
		obj.Set("_responseHeaders", jsc.StringValue(strings.Join(hdrLines, "\r\n")))
		getAllRespHeadersFn := jsc.NewNativeFunction("getAllResponseHeaders", func(in2 *jsc.Interpreter, this2 jsc.JSValue, args2 []jsc.JSValue) jsc.JSValue {
			if v, ok := obj.GetByKey("_responseHeaders"); ok {
				return v
			}
			return jsc.StringValue("")
		}, 0)
		obj.Set("getAllResponseHeaders", jsc.FunctionValue(getAllRespHeadersFn))
		obj.Set("responseText", jsc.StringValue(string(respBody)))
		obj.Set("response", jsc.StringValue(string(respBody)))
		obj.Set("readyState", jsc.NumberValue(4))
		xhrFireReadyStateChange(in, obj)
		xhrDispatch(in, obj, "load")
		xhrDispatch(in, obj, "loadend")
		return jsc.Undefined()
	}, 1)
	proto.Set("send", jsc.FunctionValue(sendFn))

	abortFn := jsc.NewNativeFunction("abort", func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if !this.IsObject() {
			return jsc.Undefined()
		}
		obj := this.AsObject()
		// 浏览器语义：已发出的请求被中止 → readystatechange(4) + abort + loadend，
		// 随后 readyState 归 UNSENT(0)；未 send 过则只归零、不发事件。
		wasSent := false
		if v, ok := obj.GetByKey("_sent"); ok && v.ToBoolean() {
			wasSent = true
		}
		if wasSent {
			obj.Set("readyState", jsc.NumberValue(4))
			xhrFireReadyStateChange(in, obj)
			xhrDispatch(in, obj, "abort")
			xhrDispatch(in, obj, "loadend")
		}
		obj.Set("_sent", jsc.BooleanValue(false))
		obj.Set("readyState", jsc.NumberValue(0))
		return jsc.Undefined()
	}, 0)
	proto.Set("abort", jsc.FunctionValue(abortFn))

	ctor := rt.NewConstructor("XMLHttpRequest", func(_ *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
		xhr := this.AsObject()
		xhr.SetClassName("XMLHttpRequest")
		xhr.Set("readyState", jsc.NumberValue(0))
		xhr.Set("status", jsc.NumberValue(0))
		xhr.Set("statusText", jsc.StringValue(""))
		xhr.Set("responseText", jsc.StringValue(""))
		xhr.Set("response", jsc.Null())
		xhr.Set("timeout", jsc.NumberValue(0))
		xhr.Set("withCredentials", jsc.BooleanValue(false))
		return xhr
	})
	// Link constructor.prototype = proto so new XMLHttpRequest() inherits methods
	ctorObj := jsc.FunctionValue(ctor).AsObject()
	ctorObj.Set("prototype", jsc.ObjectValue(proto))
	proto.Set("constructor", jsc.FunctionValue(ctor))
	rt.GlobalObject().Set("XMLHttpRequest", jsc.FunctionValue(ctor))
}

// --- Helpers ---

func jscToString(v jsc.JSValue) string {
	if v.IsUndefined() || v.IsNull() {
		return ""
	}
	if v.IsString() {
		return v.ToString()
	}
	return fmt.Sprintf("%v", v.Export())
}
func jscToString2(obj *jsc.JSObject, key string) string {
	if v, ok := obj.GetByKey(key); ok {
		return jscToString(v)
	}
	return ""
}

func resolvePromise(in *jsc.Interpreter, val jsc.JSValue) jsc.JSValue {
	return in.ResolvePromise(val)
}

func rejectPromise(in *jsc.Interpreter, err error) jsc.JSValue {
	return in.RejectPromise(jsc.StringValue(err.Error()))
}

func xhrFireReadyStateChange(in *jsc.Interpreter, obj *jsc.JSObject) {
	xhrDispatch(in, obj, "readystatechange")
}

// ─── XHR 事件系统（2026-09-27 补齐）──────────────────────────────────
//
// 浏览器语义：每次 readyState 变化触发 readystatechange；请求终结时按结果再触发
// load / error / timeout / abort，最后统一 loadend。`on<type>` 属性处理器与
// addEventListener 注册的监听器**都会被调用**。
//
// 此前 wb-ui 只实现了 onreadystatechange 一项，于是「xhr.onload = () => resolve()」
// 这类最标准的前端写法永远停在 pending（宿主 PairCode 的插件面板卡在
// 「加载插件…」即此因）。

// xhrListenerStore 挂在 XHR 对象的 Internal 上（随对象回收，无全局泄漏）。
type xhrListenerStore struct {
	byType map[string][]jsc.JSValue
}

func xhrListenersAdd(obj *jsc.JSObject, eventType string, fn jsc.JSValue) {
	if obj == nil || eventType == "" || fn.IsUndefined() || fn.IsNull() {
		return
	}
	store, _ := obj.Internal().(*xhrListenerStore)
	if store == nil {
		store = &xhrListenerStore{byType: map[string][]jsc.JSValue{}}
		obj.SetInternal(store)
	}
	store.byType[eventType] = append(store.byType[eventType], fn)
}

func xhrListenersRemove(obj *jsc.JSObject, eventType string, fn jsc.JSValue) {
	if obj == nil {
		return
	}
	store, _ := obj.Internal().(*xhrListenerStore)
	if store == nil {
		return
	}
	list := store.byType[eventType]
	kept := make([]jsc.JSValue, 0, len(list))
	removed := false
	for _, l := range list {
		// 按引用相等移除**第一个**匹配项（浏览器 removeEventListener 语义）。
		if !removed && l.SameAs(fn) {
			removed = true
			continue
		}
		kept = append(kept, l)
	}
	store.byType[eventType] = kept
}

// xhrMakeEvent 构造最小事件对象（type/target/currentTarget + 常用只读字段）。
func xhrMakeEvent(in *jsc.Interpreter, eventType string, target *jsc.JSObject) jsc.JSValue {
	ev := jsc.NewObject(in.ObjectPrototype())
	ev.Set("type", jsc.StringValue(eventType))
	ev.Set("target", jsc.ObjectValue(target))
	ev.Set("currentTarget", jsc.ObjectValue(target))
	ev.Set("bubbles", jsc.BooleanValue(false))
	ev.Set("cancelable", jsc.BooleanValue(false))
	ev.Set("defaultPrevented", jsc.BooleanValue(false))
	ev.Set("timeStamp", jsc.NumberValue(float64(time.Now().UnixMilli())))
	return jsc.ObjectValue(ev)
}

// xhrDispatch 分发一个 XHR 事件：先 on<type> 属性处理器，再按注册顺序调用
// addEventListener 的监听器。
func xhrDispatch(in *jsc.Interpreter, obj *jsc.JSObject, eventType string) {
	if obj == nil {
		return
	}
	ev := xhrMakeEvent(in, eventType, obj)
	if h, ok := obj.GetByKey("on" + eventType); ok && !h.IsUndefined() && !h.IsNull() {
		in.Call(h, jsc.ObjectValue(obj), []jsc.JSValue{ev})
	}
	store, _ := obj.Internal().(*xhrListenerStore)
	if store == nil {
		return
	}
	for _, l := range store.byType[eventType] {
		in.Call(l, jsc.ObjectValue(obj), []jsc.JSValue{ev})
	}
}

// xhrFail 把 XHR 置为失败终态：超时 → timeout 事件，其余网络错误 → error 事件，
// 最后统一 loadend（浏览器语义）。status 归 0（无响应）。
func xhrFail(in *jsc.Interpreter, obj *jsc.JSObject, err error) jsc.JSValue {
	obj.Set("status", jsc.NumberValue(0))
	obj.Set("statusText", jsc.StringValue(err.Error()))
	obj.Set("readyState", jsc.NumberValue(4))
	xhrFireReadyStateChange(in, obj)
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		xhrDispatch(in, obj, "timeout")
	} else {
		xhrDispatch(in, obj, "error")
	}
	xhrDispatch(in, obj, "loadend")
	return jsc.Undefined()
}

// xhrSetResponse 填终态响应字段（status/statusText/responseText/response 与
// getAllResponseHeaders），供 bridge 路径使用，保证与真实 HTTP 路径语义一致。
func xhrSetResponse(in *jsc.Interpreter, obj *jsc.JSObject, status int, bodyText string) {
	obj.Set("status", jsc.NumberValue(float64(status)))
	obj.Set("statusText", jsc.StringValue(http.StatusText(status)))
	obj.Set("responseText", jsc.StringValue(bodyText))
	obj.Set("response", jsc.StringValue(bodyText))
	obj.Set("_responseHeaders", jsc.StringValue(""))
	obj.Set("getAllResponseHeaders", jsc.FunctionValue(jsc.NewNativeFunction("getAllResponseHeaders", func(in2 *jsc.Interpreter, this2 jsc.JSValue, args2 []jsc.JSValue) jsc.JSValue {
		if v, ok := obj.GetByKey("_responseHeaders"); ok {
			return v
		}
		return jsc.StringValue("")
	}, 0)))
}

// xhrBridgeCall 调用命中的 bridge 路由，返回 (status, body)。与 fetch 的
// bridgeFetch 共用 Route.Handler 契约：args = [url, options]，返回
// {"status": N, "body": "..."}（见 bridge.dispatchHTTP）。
func xhrBridgeCall(in *jsc.Interpreter, route *bridge.Route, rawURL, method, bodyStr string, headers http.Header) (int, string) {
	opts := jsc.NewObject(in.ObjectPrototype())
	opts.Set("method", jsc.StringValue(method))
	if bodyStr != "" {
		opts.Set("body", jsc.StringValue(bodyStr))
	}
	if len(headers) > 0 {
		hObj := jsc.NewObject(in.ObjectPrototype())
		for k, vs := range headers {
			if len(vs) > 0 {
				hObj.Set(k, jsc.StringValue(vs[0]))
			}
		}
		opts.Set("headers", jsc.ObjectValue(hObj))
	}

	result, err := route.Handler([]jsc.JSValue{jsc.StringValue(rawURL), jsc.ObjectValue(opts)})
	if err != nil {
		return 500, fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	status := 200
	bodyText := "null"
	if result.IsObject() {
		if o := result.AsObject(); o != nil {
			if st, ok := o.GetByKey("status"); ok && st.IsNumber() {
				status = int(st.ToNumber())
			}
			if b, ok := o.GetByKey("body"); ok {
				if b.IsString() {
					bodyText = b.ToString()
				} else if !b.IsUndefined() && !b.IsNull() {
					bodyText = fmt.Sprintf("%v", b.Export())
				}
			}
		}
	} else if result.IsString() {
		bodyText = result.ToString()
	}
	return status, bodyText
}

// xhrNumber2 读取 XHR 对象上的数值属性（属性缺失/非数字 → 0）。
func xhrNumber2(obj *jsc.JSObject, key string) float64 {
	if v, ok := obj.GetByKey(key); ok && v.IsNumber() {
		return v.ToNumber()
	}
	return 0
}

// bridgeFetch handles a fetch() call that matched a registered bridge route.
// It calls the Go handler directly and returns a synthetic Response.
// handler args: [url, options] — the full fetch arguments, so Go-side handlers
// can inspect method/path/query/body/headers (see bridge.RegisterHTTP).
func bridgeFetch(in *jsc.Interpreter, args []jsc.JSValue, url string, route *bridge.Route) jsc.JSValue {
	// Build handler args: [url, options]
	var handlerArgs []jsc.JSValue
	handlerArgs = append(handlerArgs, jsc.StringValue(url))
	if len(args) >= 2 {
		handlerArgs = append(handlerArgs, args[1])
	} else {
		handlerArgs = append(handlerArgs, jsc.Null())
	}

	// Call the Go handler.
	result, err := route.Handler(handlerArgs)
	if err != nil {
		return rejectPromise(in, fmt.Errorf("bridge: handler error: %w", err))
	}

	// Build a synthetic Response.
	status := 200
	var bodyText string
	if result.IsObject() {
		obj := result.AsObject()
		if obj != nil {
			// RegisterHTTP handlers return {"status": N, "body": "..."}.
			if st, ok := obj.GetByKey("status"); ok && st.IsNumber() {
				status = int(st.ToNumber())
			}
			if b, ok := obj.GetByKey("body"); ok && b.IsString() {
				bodyText = b.ToString()
			} else if b, ok := obj.GetByKey("body"); ok && !b.IsUndefined() && !b.IsNull() {
				bodyText = fmt.Sprintf("%v", b.Export())
			} else {
				bodyText = fmt.Sprintf("%v", result.Export())
			}
		} else {
			bodyText = fmt.Sprintf("%v", result.Export())
		}
	} else if result.IsString() {
		bodyText = result.ToString()
	} else if result.IsUndefined() || result.IsNull() {
		bodyText = "null"
	} else {
		bodyText = fmt.Sprintf("%v", result.Export())
	}

	statusText := "OK"
	if status >= 400 {
		statusText = "Error"
	}
	respObj := jsc.NewObject(in.ObjectPrototype())
	respObj.SetClassName("Response")
	respObj.Set("status", jsc.NumberValue(float64(status)))
	respObj.Set("ok", jsc.BooleanValue(status >= 200 && status < 300))
	respObj.Set("statusText", jsc.StringValue(statusText))
	respObj.Set("url", jsc.StringValue(url))

	textFn := jsc.NewNativeFunction("text", func(in2 *jsc.Interpreter, this2 jsc.JSValue, args2 []jsc.JSValue) jsc.JSValue {
		return resolvePromise(in2, jsc.StringValue(bodyText))
	}, 0)
	respObj.Set("text", jsc.FunctionValue(textFn))

	jsonFn := jsc.NewNativeFunction("json", func(in2 *jsc.Interpreter, this2 jsc.JSValue, args2 []jsc.JSValue) jsc.JSValue {
		// ★ 性能：不要用 RunJS("(" + body + ")") 把 JSON 当 JS 代码解析——
		//   goja 对 MB 级 JSON 的 parse+codegen 极慢（启动 22 秒热点之一）。
		//   改用 Go 侧 encoding/json 解析 + ToJSValue 直转 JS 对象，快一个量级。
		var parsed any
		if err := json.Unmarshal([]byte(bodyText), &parsed); err != nil {
			return rejectPromise(in2, fmt.Errorf("bridge: json parse failed: %w", err))
		}
		// ★ 用当前解释器的 ValueOf，不要 bindings.ToJSValue：后者经
		//   jsc.NewObject(nil) 会新建独立 runtime，对象跨 runtime 使用即抛
		//   "Illegal runtime transition of an Object"（与真实 HTTP 分支同因）。
		return resolvePromise(in2, in2.ValueOf(parsed))
	}, 0)
	respObj.Set("json", jsc.FunctionValue(jsonFn))
	blobFn := jsc.NewNativeFunction("blob", func(in2 *jsc.Interpreter, this2 jsc.JSValue, args2 []jsc.JSValue) jsc.JSValue {
		bo := jsc.NewObject(in2.ObjectPrototype())
		bo.Set("size", jsc.NumberValue(float64(len(bodyText))))
		bo.Set("type", jsc.StringValue("application/json"))
		bo.Set("__wbBlobData", jsc.StringValue(bodyText))
		bo.Set("text", jsc.FunctionValue(jsc.NewNativeFunction("text",
			func(in3 *jsc.Interpreter, t3 jsc.JSValue, a3 []jsc.JSValue) jsc.JSValue {
				return resolvePromise(in3, jsc.StringValue(bodyText))
			}, 0)))
		bo.Set("arrayBuffer", jsc.FunctionValue(jsc.NewNativeFunction("arrayBuffer",
			func(in3 *jsc.Interpreter, t3 jsc.JSValue, a3 []jsc.JSValue) jsc.JSValue {
				return resolvePromise(in3, jsc.StringValue(bodyText))
			}, 0)))
		return resolvePromise(in2, jsc.ObjectValue(bo))
	}, 0)
	respObj.Set("blob", jsc.FunctionValue(blobFn))

	headersObj := jsc.NewObject(in.ObjectPrototype())
	headersObj.Set("Content-Type", jsc.StringValue("application/json"))
	respObj.Set("headers", jsc.ObjectValue(headersObj))

	return resolvePromise(in, jsc.ObjectValue(respObj))
}
