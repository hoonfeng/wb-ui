package page

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
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
	// ★ 先装 Headers / Response / Request 构造器：fetch 返回的 Response 实例
	//   要挂在这些构造器的 prototype 上（`r.headers instanceof Headers`、
	//   `r instanceof Response` 成立），且页面可直接 `new Headers({...})`。
	registerWebAPIObjects(rt)
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

		// ★ blob: URL（URL.createObjectURL 产物）由引擎内存表直接供给：
		//   浏览器同样不发起网络请求。blob: 是绝对 scheme，ResolveURL 原样
		//   返回；登记表在 jsc 层（与 URL.createObjectURL 同一张表，见
		//   jsc/bloburl.go）。未登记/已 revoke 的 URL 按浏览器语义 reject。
		if strings.HasPrefix(url, "blob:") {
			entry, ok := jsc.LookupBlobURL(url)
			if !ok {
				return rejectPromise(in, fmt.Errorf("fetch: unknown blob URL: %s", url))
			}
			hdrs := newHeadersState()
			if entry.MIMEType != "" {
				hdrs.set("Content-Type", entry.MIMEType)
			}
			return resolvePromise(in, jsc.ObjectValue(newResponseInstance(in, &responseState{
				body:       entry.Data,
				headers:    hdrs,
				status:     200,
				statusText: "OK",
				url:        url,
			})))
		}

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
		// ★ fetch 的 CORS 选项（见 page/cors.go）：credentials 决定跨源
		//   响应是否必须带 ACAC；mode 决定是否允许跨源/是否返回 opaque。
		credentials := "same-origin"
		mode := corsModeCORS
		if len(args) >= 2 {
			opts := args[1]
			if opts.IsObject() {
				o := opts.AsObject()
				if o != nil {
					if v, ok := o.GetByKey("credentials"); ok && !v.IsUndefined() && !v.IsNull() {
						credentials = strings.ToLower(jscToString(v))
					}
					if v, ok := o.GetByKey("mode"); ok && !v.IsUndefined() && !v.IsNull() {
						switch m := strings.ToLower(jscToString(v)); m {
						case "cors", "no-cors", "same-origin":
							mode = corsMode(m)
						}
					}
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
		// ★ CORS：跨源请求按浏览器语义校验（同源请求不受影响；文档源为
		//   file:/data:（不透明源）时跳过——见 cors.go 的已知差异）。
		//   ★ 文档源必须取**文档 URL**（location.href），不能取 DocumentBase：
		//   baseURI 受页面 `<base href>` 影响，用它会允许页面把 Origin 伪造成
		//   任意源（见 document_base.go 的说明）。相对 URL 解析仍用 DocumentBase。
		corsReq := newCORSRequest(DocumentURL(in), url, method, headers, credentials == "include", mode)
		opaque, cerr := corsReq.prepare(client)
		if cerr != nil {
			return rejectPromise(in, fmt.Errorf("fetch(%s): %w", raw, cerr))
		}
		corsReq.applyOriginHeader(req)
		resp, err := client.Do(req)
		if err != nil {
			return rejectPromise(in, fmt.Errorf("fetch: request failed: %w", err))
		}
		defer resp.Body.Close()
		if opaque {
			// mode=no-cors：浏览器返回 opaque 响应——status 0、body/headers
			// 不可读（页面拿不到跨源内容）。
			return resolvePromise(in, jsc.ObjectValue(newResponseInstance(in, &responseState{
				status:       0,
				responseType: "opaque",
			})))
		}
		if err := corsReq.validateFinal(resp); err != nil {
			return rejectPromise(in, fmt.Errorf("fetch(%s): %w", raw, err))
		}
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return rejectPromise(in, fmt.Errorf("fetch: read response failed: %w", err))
		}
		// ★ 返回真正的 Response 实例（全局 Response 构造器的 prototype）：
		//   headers 是 Headers 对象（get/has/forEach/entries 齐全），body 由
		//   responseState 承载（text/json/blob/clone/arrayBuffer）。此前 headers
		//   是纯数据对象，页面按浏览器习惯调 r.headers.get(...) 直接抛
		//   "Object has no member 'get'"（gou-ide 导出/下载链路实测中断）。
		respObj := newResponseInstance(in, &responseState{
			body:       string(respBody),
			// ★ 跨源响应头可读性限制（Access-Control-Expose-Headers）：
			//   只暴露 safelisted 头 + ACEH 列出的头，Set-Cookie 恒不可见。
			//   同源/不透明源原样返回（filterResponseHeaders 内部判定）。
			headers:    headersStateFromHTTP(corsReq.filterResponseHeaders(resp.Header)),
			status:     resp.StatusCode,
			statusText: http.StatusText(resp.StatusCode),
			url:        url,
		})
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
		// ★ CORS：XHR 同样受跨源限制（浏览器里 XHR 与 fetch 共用同一套
		//   CORS 校验）。withCredentials 决定 ACAO 能否为 *。
		withCredentials := false
		if v, ok := obj.GetByKey("withCredentials"); ok {
			withCredentials = v.ToBoolean()
		}
		// ★ 文档源取文档 URL（非 baseURI）：理由同 fetch 侧。
		corsReq := newCORSRequest(DocumentURL(in), url, method, headers, withCredentials, corsModeCORS)
		if _, cerr := corsReq.prepare(client); cerr != nil {
			return xhrFail(in, obj, cerr)
		}
		corsReq.applyOriginHeader(req)
		resp, err := client.Do(req)
		if err != nil {
			return xhrFail(in, obj, err)
		}
		defer resp.Body.Close()
		if cerr := corsReq.validateFinal(resp); cerr != nil {
			return xhrFail(in, obj, cerr)
		}
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return xhrFail(in, obj, err)
		}
		obj.Set("status", jsc.NumberValue(float64(resp.StatusCode)))
		// ★ 与 fetch 共用同一份过滤逻辑（cors.go filterResponseHeaders）：
		//   同源/跨源都应剔除 Set-Cookie（forbidden response header），
		//   跨源时只能看到 safelisted + ACEH 头。
		//   序列化与 fetch 的 Headers 语义一致，且只此一处实现。
		visibleHeaders := corsReq.filterResponseHeaders(resp.Header)
		obj.Set("_responseHeaders", jsc.StringValue(serializeResponseHeaders(visibleHeaders)))
		getAllRespHeadersFn := jsc.NewNativeFunction("getAllResponseHeaders", func(in2 *jsc.Interpreter, this2 jsc.JSValue, args2 []jsc.JSValue) jsc.JSValue {
			if v, ok := obj.GetByKey("_responseHeaders"); ok {
				return v
			}
			return jsc.StringValue("")
		}, 0)
		obj.Set("getAllResponseHeaders", jsc.FunctionValue(getAllRespHeadersFn))
		// ★ XMLHttpRequest.getResponseHeader / responseURL：此前只有
		//   getAllResponseHeaders，前端最常见的 `xhr.getResponseHeader('Content-Type')`
		//   直接抛 "Object has no member 'getResponseHeader'"（探针实测）。
		respHeaderState := headersStateFromHTTP(visibleHeaders)
		obj.Set("getResponseHeader", jsc.FunctionValue(jsc.NewNativeFunction("getResponseHeader",
			func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				if len(args) < 1 {
					return jsc.Null()
				}
				if v, ok := respHeaderState.get(jscToString(args[0])); ok {
					return jsc.StringValue(v)
				}
				return jsc.Null()
			}, 1)))
		obj.Set("responseURL", jsc.StringValue(url))
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
		xhr.Set("responseType", jsc.StringValue(""))
		xhr.Set("responseURL", jsc.StringValue(""))
		// upload（XMLHttpRequestUpload）：前端上传进度条读 xhr.upload.onprogress；
		// 引擎没有真上传进度，但对象与方法必须存在（注册不抛错即可）。
		upload := jsc.NewObject(nil)
		upload.Set("addEventListener", jsc.FunctionValue(jsc.NewNativeFunction("addEventListener",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue { return jsc.Undefined() }, 2)))
		upload.Set("removeEventListener", jsc.FunctionValue(jsc.NewNativeFunction("removeEventListener",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue { return jsc.Undefined() }, 2)))
		xhr.Set("upload", jsc.ObjectValue(upload))
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

// serializeResponseHeaders 按浏览器 XMLHttpRequest.getAllResponseHeaders()
// 的语义序列化响应头：
//
//   - 头名小写；
//   - 按头名做字典序排序；
//   - 每个头一行 `name: value\r\n`（同名多值用 ", " 连接）；
//   - 整体以 `\r\n` 结尾；
//   - 没有任何可见头时返回空字符串（不是 "\r\n"）。
//
// ★ 唯一的响应头序列化实现：真实 HTTP 的 XHR 走这里；fetch 用 Headers
// 对象（headersStateFromHTTP）读取同一份 filterResponseHeaders 结果，
// 二者都不允许各自拼字符串。
func serializeResponseHeaders(h http.Header) string {
	if len(h) == 0 {
		return ""
	}
	type headerLine struct {
		name  string
		value string
	}
	lines := make([]headerLine, 0, len(h))
	for name, values := range h {
		lower := strings.ToLower(strings.TrimSpace(name))
		if lower == "" {
			continue
		}
		lines = append(lines, headerLine{name: lower, value: strings.Join(values, ", ")})
	}
	if len(lines) == 0 {
		return ""
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].name < lines[j].name })
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.name)
		b.WriteString(": ")
		b.WriteString(l.value)
		b.WriteString("\r\n")
	}
	return b.String()
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
	// bridge 路径没有真实响应头：getResponseHeader 恒返回 null（浏览器语义：
	// 不存在的头返回 null），但方法本身必须存在，否则调用即抛错。
	obj.Set("getResponseHeader", jsc.FunctionValue(jsc.NewNativeFunction("getResponseHeader",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Null()
		}, 1)))
	obj.Set("responseURL", jsc.StringValue(""))
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
	// ★ 与真实 HTTP 分支一致：返回 Response 实例（headers 是 Headers 对象，
	//   页面可直接 headers.get/has/forEach/entries）。
	hdrs := newHeadersState()
	hdrs.set("Content-Type", "application/json")
	return resolvePromise(in, jsc.ObjectValue(newResponseInstance(in, &responseState{
		body:       bodyText,
		headers:    hdrs,
		status:     status,
		statusText: statusText,
		url:        url,
	})))
}
