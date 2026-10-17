package page

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"wb-ui/engine/js/jsc"
)

// webapi.go —— Fetch 相关 Web IDL 对象（Headers / Response / Request）。
//
// 为什么需要：
//   - 此前 fetch 返回的 `response.headers` 是**普通 JS 对象**，页面按浏览器习惯
//     调用 `r.headers.get('Content-Length')` 直接抛 "Object has no member 'get'"
//     （实测 gou-ide 导出/下载链路即因此中断）；
//   - 全局 `Headers` / `Response` / `Request` 构造器不存在（typeof === "undefined"），
//     `new Headers({...})`、`new Response(body)` 这类标准写法无法使用；
//   - `Response` 实例缺 `clone` / `arrayBuffer` / `formData`。
//
// 实现口径：大小写不敏感的名字表（HTTP 语义）、按名字排序的迭代顺序
// （WHATWG Fetch §2.2），每个方法都挂在构造器的 prototype 上，实例只带
// Go 侧状态（Internal），因此 `r.headers instanceof Headers` 成立。

// ─── Headers 状态 ────────────────────────────────────────────────────

// headersState 是 Headers 实例的 Go 侧状态。
// names 保持首次出现的顺序；values 按规范化后的小写名索引（HTTP 头名
// 大小写不敏感）。迭代时按名字字典序输出（WHATWG Fetch §2.2.3）。
type headersState struct {
	names  []string
	values map[string][]string
}

func newHeadersState() *headersState {
	return &headersState{values: map[string][]string{}}
}

// normalizeHeaderName 规范化头名：去空白 + 小写（HTTP 头名大小写不敏感）。
func normalizeHeaderName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (h *headersState) append(name, value string) {
	n := normalizeHeaderName(name)
	if n == "" {
		return
	}
	if _, ok := h.values[n]; !ok {
		h.names = append(h.names, n)
	}
	h.values[n] = append(h.values[n], strings.TrimSpace(value))
}

func (h *headersState) set(name, value string) {
	n := normalizeHeaderName(name)
	if n == "" {
		return
	}
	if _, ok := h.values[n]; !ok {
		h.names = append(h.names, n)
	}
	h.values[n] = []string{strings.TrimSpace(value)}
}

func (h *headersState) delete(name string) {
	n := normalizeHeaderName(name)
	if _, ok := h.values[n]; !ok {
		return
	}
	delete(h.values, n)
	kept := h.names[:0]
	for _, x := range h.names {
		if x != n {
			kept = append(kept, x)
		}
	}
	h.names = kept
}

// get 返回逗号连接后的头值（浏览器 Headers.get 语义）。
func (h *headersState) get(name string) (string, bool) {
	vs, ok := h.values[normalizeHeaderName(name)]
	if !ok || len(vs) == 0 {
		return "", false
	}
	return strings.Join(vs, ", "), true
}

// getAll 返回全部头值（Headers.getAll 语义；非标准但被广泛使用）。
func (h *headersState) getAll(name string) []string {
	vs := h.values[normalizeHeaderName(name)]
	out := make([]string, len(vs))
	copy(out, vs)
	return out
}

func (h *headersState) has(name string) bool {
	_, ok := h.values[normalizeHeaderName(name)]
	return ok
}

// sortedNames 返回按字典序排列的头名（Headers 迭代顺序）。
func (h *headersState) sortedNames() []string {
	out := make([]string, len(h.names))
	copy(out, h.names)
	sort.Strings(out)
	return out
}

// ─── Headers JS 对象 ─────────────────────────────────────────────────

// headersStateOf 从 JS 侧的 this 取回 Headers 状态（非 Headers 实例时返回 nil）。
func headersStateOf(this jsc.JSValue) *headersState {
	if !this.IsObject() {
		return nil
	}
	obj := this.AsObject()
	if obj == nil {
		return nil
	}
	st, _ := obj.Internal().(*headersState)
	return st
}

// headersPrototypeFor 返回当前解释器里 Headers 构造器的 prototype。
// 未注册时退回 ObjectPrototype（退化但不会崩）。
func headersPrototypeFor(in *jsc.Interpreter) *jsc.JSObject {
	if in == nil {
		return nil
	}
	if v, ok := in.GlobalObject().GetByKey("Headers"); ok && v.IsFunction() {
		if obj := v.AsObject(); obj != nil {
			if p := obj.GetStr("prototype"); p.IsObject() {
				return p.AsObject()
			}
		}
	}
	return in.ObjectPrototype()
}

// newHeadersInstance 构建一个 Headers 实例（供 Response.headers 等内部使用）。
func newHeadersInstance(in *jsc.Interpreter, st *headersState) *jsc.JSObject {
	if st == nil {
		st = newHeadersState()
	}
	obj := jsc.NewObject(headersPrototypeFor(in))
	obj.SetClassName("Headers")
	obj.SetInternal(st)
	return obj
}

// headersStateFromHTTP 把 net/http 的 Header 映射成 Headers 状态。
func headersStateFromHTTP(hdrs map[string][]string) *headersState {
	st := newHeadersState()
	for k, vs := range hdrs {
		for _, v := range vs {
			st.append(k, v)
		}
	}
	return st
}

// headersFromHTTP 把 net/http 的 Header 映射成 Headers 实例。
func headersFromHTTP(in *jsc.Interpreter, hdrs map[string][]string) *jsc.JSObject {
	return newHeadersInstance(in, headersStateFromHTTP(hdrs))
}

// headersApplyInit 用构造参数初始化 Headers 状态：
//   - 对象字面量  { "Content-Type": "text/plain" }
//   - 序列 [[name, value], ...]（也接受任意可迭代的二元组数组）
//   - 另一个 Headers 实例
func headersApplyInit(in *jsc.Interpreter, st *headersState, init jsc.JSValue) {
	if st == nil || !init.IsObject() {
		return
	}
	obj := init.AsObject()
	if obj == nil {
		return
	}
	// 另一个 Headers 实例：复制全部值。
	if src, ok := obj.Internal().(*headersState); ok && src != nil {
		for _, name := range src.names {
			for _, v := range src.values[name] {
				st.append(name, v)
			}
		}
		return
	}
	// 数组形式 [[k, v], ...]
	if lengthFn, ok := obj.GetByKey("length"); ok && lengthFn.IsNumber() {
		n := int(lengthFn.ToNumber())
		for i := 0; i < n; i++ {
			pair, ok := obj.GetByKey(fmt.Sprintf("%d", i))
			if !ok || !pair.IsObject() {
				continue
			}
			pobj := pair.AsObject()
			if pobj == nil {
				continue
			}
			kv, ok1 := pobj.GetByKey("0")
			vv, ok2 := pobj.GetByKey("1")
			if ok1 && ok2 {
				st.append(jscToString(kv), jscToString(vv))
			}
		}
		return
	}
	// 对象字面量：自有键逐个 append。
	for _, key := range obj.Keys() {
		if v, ok := obj.GetByKey(key); ok {
			st.append(key, jscToString(v))
		}
	}
}

// installHeadersMethods 把 Headers 方法装到原型上。
func installHeadersMethods(proto *jsc.JSObject) {
	if proto == nil {
		return
	}
	proto.Set("append", jsc.FunctionValue(jsc.NewNativeFunction("append",
		func(_ *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			st := headersStateOf(this)
			if st == nil || len(args) < 2 {
				return jsc.Undefined()
			}
			st.append(jscToString(args[0]), jscToString(args[1]))
			return jsc.Undefined()
		}, 2)))
	proto.Set("set", jsc.FunctionValue(jsc.NewNativeFunction("set",
		func(_ *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			st := headersStateOf(this)
			if st == nil || len(args) < 2 {
				return jsc.Undefined()
			}
			st.set(jscToString(args[0]), jscToString(args[1]))
			return jsc.Undefined()
		}, 2)))
	proto.Set("delete", jsc.FunctionValue(jsc.NewNativeFunction("delete",
		func(_ *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			st := headersStateOf(this)
			if st == nil || len(args) < 1 {
				return jsc.Undefined()
			}
			st.delete(jscToString(args[0]))
			return jsc.Undefined()
		}, 1)))
	proto.Set("get", jsc.FunctionValue(jsc.NewNativeFunction("get",
		func(_ *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			st := headersStateOf(this)
			if st == nil || len(args) < 1 {
				return jsc.Null()
			}
			if v, ok := st.get(jscToString(args[0])); ok {
				return jsc.StringValue(v)
			}
			return jsc.Null()
		}, 1)))
	proto.Set("getAll", jsc.FunctionValue(jsc.NewNativeFunction("getAll",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			st := headersStateOf(this)
			if st == nil || len(args) < 1 {
				return jsc.ObjectValue(jsc.NewArrayForInterp(in, nil))
			}
			items := make([]jsc.JSValue, 0, 4)
			for _, v := range st.getAll(jscToString(args[0])) {
				items = append(items, jsc.StringValue(v))
			}
			return jsc.ObjectValue(jsc.NewArrayForInterp(in, items))
		}, 1)))
	proto.Set("has", jsc.FunctionValue(jsc.NewNativeFunction("has",
		func(_ *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			st := headersStateOf(this)
			if st == nil || len(args) < 1 {
				return jsc.BooleanValue(false)
			}
			return jsc.BooleanValue(st.has(jscToString(args[0])))
		}, 1)))
	proto.Set("getSetCookie", jsc.FunctionValue(jsc.NewNativeFunction("getSetCookie",
		func(in *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			st := headersStateOf(this)
			if st == nil {
				return jsc.ObjectValue(jsc.NewArrayForInterp(in, nil))
			}
			items := make([]jsc.JSValue, 0, 2)
			for _, v := range st.getAll("set-cookie") {
				items = append(items, jsc.StringValue(v))
			}
			return jsc.ObjectValue(jsc.NewArrayForInterp(in, items))
		}, 0)))
	proto.Set("forEach", jsc.FunctionValue(jsc.NewNativeFunction("forEach",
		func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			st := headersStateOf(this)
			if st == nil || len(args) < 1 || !args[0].IsFunction() {
				return jsc.Undefined()
			}
			for _, name := range st.sortedNames() {
				val := strings.Join(st.values[name], ", ")
				_, _ = in.Call(args[0], this, []jsc.JSValue{
					jsc.StringValue(val), jsc.StringValue(name), this,
				})
			}
			return jsc.Undefined()
		}, 1)))
	// entries/keys/values：按规范返回迭代器；这里返回可迭代数组（展开、for...of、
	// Array.from 全部可用——前端最常用的就是这三种消费方式）。
	proto.Set("entries", jsc.FunctionValue(jsc.NewNativeFunction("entries",
		func(in *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			st := headersStateOf(this)
			if st == nil {
				return jsc.ObjectValue(jsc.NewArrayForInterp(in, nil))
			}
			items := make([]jsc.JSValue, 0, len(st.names))
			for _, name := range st.sortedNames() {
				pair := jsc.NewArrayForInterp(in, []jsc.JSValue{
					jsc.StringValue(name),
					jsc.StringValue(strings.Join(st.values[name], ", ")),
				})
				items = append(items, jsc.ObjectValue(pair))
			}
			return jsc.ObjectValue(jsc.NewArrayForInterp(in, items))
		}, 0)))
	proto.Set("keys", jsc.FunctionValue(jsc.NewNativeFunction("keys",
		func(in *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			st := headersStateOf(this)
			if st == nil {
				return jsc.ObjectValue(jsc.NewArrayForInterp(in, nil))
			}
			items := make([]jsc.JSValue, 0, len(st.names))
			for _, name := range st.sortedNames() {
				items = append(items, jsc.StringValue(name))
			}
			return jsc.ObjectValue(jsc.NewArrayForInterp(in, items))
		}, 0)))
	proto.Set("values", jsc.FunctionValue(jsc.NewNativeFunction("values",
		func(in *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			st := headersStateOf(this)
			if st == nil {
				return jsc.ObjectValue(jsc.NewArrayForInterp(in, nil))
			}
			items := make([]jsc.JSValue, 0, len(st.names))
			for _, name := range st.sortedNames() {
				items = append(items, jsc.StringValue(strings.Join(st.values[name], ", ")))
			}
			return jsc.ObjectValue(jsc.NewArrayForInterp(in, items))
		}, 0)))
	proto.Set("toString", jsc.FunctionValue(jsc.NewNativeFunction("toString",
		func(_ *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			st := headersStateOf(this)
			if st == nil {
				return jsc.StringValue("")
			}
			var b strings.Builder
			for i, name := range st.sortedNames() {
				if i > 0 {
					b.WriteString("\r\n")
				}
				b.WriteString(name)
				b.WriteString(": ")
				b.WriteString(strings.Join(st.values[name], ", "))
			}
			return jsc.StringValue(b.String())
		}, 0)))
}

// registerHeadersCtor 注册全局 Headers 构造器（幂等）。
func registerHeadersCtor(in *jsc.Interpreter) {
	if in == nil {
		return
	}
	if v, ok := in.GlobalObject().GetByKey("Headers"); ok && v.IsFunction() {
		return
	}
	proto := jsc.NewObject(in.ObjectPrototype())
	proto.SetClassName("HeadersPrototype")
	installHeadersMethods(proto)
	ctor := in.NewConstructor("Headers", func(i *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
		obj := this.AsObject()
		if obj == nil {
			obj = jsc.NewObject(headersPrototypeFor(i))
		}
		obj.SetClassName("Headers")
		st := newHeadersState()
		obj.SetInternal(st)
		if len(args) > 0 {
			headersApplyInit(i, st, args[0])
		}
		return obj
	})
	ctorObj := jsc.FunctionValue(ctor).AsObject()
	if ctorObj != nil {
		ctorObj.Set("prototype", jsc.ObjectValue(proto))
	}
	proto.Set("constructor", jsc.FunctionValue(ctor))
	in.GlobalObject().Set("Headers", jsc.FunctionValue(ctor))
}

// ─── Response ────────────────────────────────────────────────────────

// responseState 是 Response 实例的 Go 侧状态。
type responseState struct {
	body       string
	headers    *headersState
	status     int
	statusText string
	url        string
	bodyUsed   bool
}

func responseStateOf(this jsc.JSValue) *responseState {
	if !this.IsObject() {
		return nil
	}
	obj := this.AsObject()
	if obj == nil {
		return nil
	}
	st, _ := obj.Internal().(*responseState)
	return st
}

// newResponseInstance 构建一个 Response 实例并填充标准属性。
// body 是已读出的文本（引擎的 fetch 是同步实现，响应体一次性读完）。
func newResponseInstance(in *jsc.Interpreter, st *responseState) *jsc.JSObject {
	if st == nil {
		st = &responseState{status: 200, statusText: "OK"}
	}
	if st.headers == nil {
		st.headers = newHeadersState()
	}
	proto := in.ObjectPrototype()
	if v, ok := in.GlobalObject().GetByKey("Response"); ok && v.IsFunction() {
		if obj := v.AsObject(); obj != nil {
			if p := obj.GetStr("prototype"); p.IsObject() {
				proto = p.AsObject()
			}
		}
	}
	obj := jsc.NewObject(proto)
	obj.SetClassName("Response")
	obj.SetInternal(st)
	obj.Set("type", jsc.StringValue("basic"))
	obj.Set("url", jsc.StringValue(st.url))
	obj.Set("redirected", jsc.BooleanValue(false))
	obj.Set("status", jsc.NumberValue(float64(st.status)))
	obj.Set("ok", jsc.BooleanValue(st.status >= 200 && st.status < 300))
	obj.Set("statusText", jsc.StringValue(st.statusText))
	obj.Set("headers", jsc.ObjectValue(newHeadersInstance(in, st.headers)))
	obj.Set("bodyUsed", jsc.BooleanValue(false))
	return obj
}

// installResponseMethods 把 Response 方法装到原型上。
func installResponseMethods(proto *jsc.JSObject) {
	if proto == nil {
		return
	}
	proto.Set("text", jsc.FunctionValue(jsc.NewNativeFunction("text",
		func(in *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			st := responseStateOf(this)
			if st == nil {
				return rejectPromise(in, fmt.Errorf("Response.text: not a Response"))
			}
			st.bodyUsed = true
			return resolvePromise(in, jsc.StringValue(st.body))
		}, 0)))
	proto.Set("json", jsc.FunctionValue(jsc.NewNativeFunction("json",
		func(in *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			st := responseStateOf(this)
			if st == nil {
				return rejectPromise(in, fmt.Errorf("Response.json: not a Response"))
			}
			st.bodyUsed = true
			var parsed any
			if err := json.Unmarshal([]byte(st.body), &parsed); err != nil {
				return rejectPromise(in, fmt.Errorf("Response.json: %w", err))
			}
			return resolvePromise(in, in.ValueOf(parsed))
		}, 0)))
	proto.Set("blob", jsc.FunctionValue(jsc.NewNativeFunction("blob",
		func(in *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			st := responseStateOf(this)
			if st == nil {
				return rejectPromise(in, fmt.Errorf("Response.blob: not a Response"))
			}
			st.bodyUsed = true
			bo := jsc.NewObject(in.ObjectPrototype())
			bo.SetClassName("Blob")
			bo.Set("size", jsc.NumberValue(float64(len(st.body))))
			ctype := "text/plain"
			if st.headers != nil {
				if v, ok := st.headers.get("content-type"); ok {
					ctype = v
				}
			}
			bo.Set("type", jsc.StringValue(ctype))
			bo.Set("__wbBlobData", jsc.StringValue(st.body))
			bo.Set("text", jsc.FunctionValue(jsc.NewNativeFunction("text",
				func(i2 *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return resolvePromise(i2, jsc.StringValue(st.body))
				}, 0)))
			bo.Set("arrayBuffer", jsc.FunctionValue(jsc.NewNativeFunction("arrayBuffer",
				func(i2 *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
					return resolvePromise(i2, jsc.StringValue(st.body))
				}, 0)))
			return resolvePromise(in, jsc.ObjectValue(bo))
		}, 0)))
	proto.Set("arrayBuffer", jsc.FunctionValue(jsc.NewNativeFunction("arrayBuffer",
		func(in *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			st := responseStateOf(this)
			if st == nil {
				return rejectPromise(in, fmt.Errorf("Response.arrayBuffer: not a Response"))
			}
			st.bodyUsed = true
			// 引擎的 Blob/FileReader 约定：字节以内部字符串字段承载。
			return resolvePromise(in, jsc.StringValue(st.body))
		}, 0)))
	proto.Set("formData", jsc.FunctionValue(jsc.NewNativeFunction("formData",
		func(in *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			st := responseStateOf(this)
			if st == nil {
				return rejectPromise(in, fmt.Errorf("Response.formData: not a Response"))
			}
			st.bodyUsed = true
			return rejectPromise(in, fmt.Errorf("Response.formData: multipart 解析尚未实现"))
		}, 0)))
	proto.Set("clone", jsc.FunctionValue(jsc.NewNativeFunction("clone",
		func(in *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			st := responseStateOf(this)
			if st == nil {
				return rejectPromise(in, fmt.Errorf("Response.clone: not a Response"))
			}
			cp := &responseState{
				body:       st.body,
				headers:    cloneHeadersState(st.headers),
				status:     st.status,
				statusText: st.statusText,
				url:        st.url,
			}
			return jsc.ObjectValue(newResponseInstance(in, cp))
		}, 0)))
}

// cloneHeadersState 复制一份 Headers 状态（clone() 用）。
func cloneHeadersState(src *headersState) *headersState {
	dst := newHeadersState()
	if src == nil {
		return dst
	}
	for _, name := range src.names {
		for _, v := range src.values[name] {
			dst.append(name, v)
		}
	}
	return dst
}

// registerResponseCtor 注册全局 Response 构造器（幂等）。
func registerResponseCtor(in *jsc.Interpreter) {
	if in == nil {
		return
	}
	if v, ok := in.GlobalObject().GetByKey("Response"); ok && v.IsFunction() {
		return
	}
	proto := jsc.NewObject(in.ObjectPrototype())
	proto.SetClassName("ResponsePrototype")
	installResponseMethods(proto)
	ctor := in.NewConstructor("Response", func(i *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
		st := &responseState{status: 200, statusText: "OK", url: ""}
		if len(args) > 0 && !args[0].IsUndefined() && !args[0].IsNull() {
			st.body = jscToString(args[0])
		}
		if len(args) > 1 && args[1].IsObject() {
			if init := args[1].AsObject(); init != nil {
				if v, ok := init.GetByKey("status"); ok && v.IsNumber() {
					st.status = int(v.ToNumber())
				}
				if v, ok := init.GetByKey("statusText"); ok {
					st.statusText = jscToString(v)
				}
				if v, ok := init.GetByKey("headers"); ok {
					st.headers = newHeadersState()
					headersApplyInit(i, st.headers, v)
				}
			}
		}
		return newResponseInstance(i, st)
	})
	ctorObj := jsc.FunctionValue(ctor).AsObject()
	if ctorObj != nil {
		ctorObj.Set("prototype", jsc.ObjectValue(proto))
	}
	proto.Set("constructor", jsc.FunctionValue(ctor))
	in.GlobalObject().Set("Response", jsc.FunctionValue(ctor))
}

// ─── Request ─────────────────────────────────────────────────────────

// requestState 是 Request 实例的 Go 侧状态。
type requestState struct {
	url        string
	method     string
	headers    *headersState
	body       string
	bodyUsed   bool
	credentials string
	mode       string
	cache      string
}

func requestStateOf(this jsc.JSValue) *requestState {
	if !this.IsObject() {
		return nil
	}
	obj := this.AsObject()
	if obj == nil {
		return nil
	}
	st, _ := obj.Internal().(*requestState)
	return st
}

// newRequestInstance 构建 Request 实例。
func newRequestInstance(in *jsc.Interpreter, st *requestState) *jsc.JSObject {
	if st == nil {
		st = &requestState{method: "GET", credentials: "same-origin", mode: "cors", cache: "default"}
	}
	if st.headers == nil {
		st.headers = newHeadersState()
	}
	proto := in.ObjectPrototype()
	if v, ok := in.GlobalObject().GetByKey("Request"); ok && v.IsFunction() {
		if obj := v.AsObject(); obj != nil {
			if p := obj.GetStr("prototype"); p.IsObject() {
				proto = p.AsObject()
			}
		}
	}
	obj := jsc.NewObject(proto)
	obj.SetClassName("Request")
	obj.SetInternal(st)
	obj.Set("url", jsc.StringValue(st.url))
	obj.Set("method", jsc.StringValue(st.method))
	obj.Set("headers", jsc.ObjectValue(newHeadersInstance(in, st.headers)))
	obj.Set("body", jsc.StringValue(st.body))
	obj.Set("bodyUsed", jsc.BooleanValue(false))
	obj.Set("credentials", jsc.StringValue(st.credentials))
	obj.Set("mode", jsc.StringValue(st.mode))
	obj.Set("cache", jsc.StringValue(st.cache))
	obj.Set("redirect", jsc.StringValue("follow"))
	obj.Set("referrer", jsc.StringValue("about:client"))
	obj.Set("destination", jsc.StringValue(""))
	obj.Set("keepalive", jsc.BooleanValue(false))
	return obj
}

// installRequestMethods 把 Request 方法装到原型上。
func installRequestMethods(proto *jsc.JSObject) {
	if proto == nil {
		return
	}
	proto.Set("clone", jsc.FunctionValue(jsc.NewNativeFunction("clone",
		func(in *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			st := requestStateOf(this)
			if st == nil {
				return rejectPromise(in, fmt.Errorf("Request.clone: not a Request"))
			}
			cp := &requestState{
				url: st.url, method: st.method, body: st.body,
				headers: cloneHeadersState(st.headers),
				credentials: st.credentials, mode: st.mode, cache: st.cache,
			}
			return jsc.ObjectValue(newRequestInstance(in, cp))
		}, 0)))
	proto.Set("text", jsc.FunctionValue(jsc.NewNativeFunction("text",
		func(in *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			st := requestStateOf(this)
			if st == nil {
				return rejectPromise(in, fmt.Errorf("Request.text: not a Request"))
			}
			st.bodyUsed = true
			return resolvePromise(in, jsc.StringValue(st.body))
		}, 0)))
	proto.Set("json", jsc.FunctionValue(jsc.NewNativeFunction("json",
		func(in *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			st := requestStateOf(this)
			if st == nil {
				return rejectPromise(in, fmt.Errorf("Request.json: not a Request"))
			}
			st.bodyUsed = true
			var parsed any
			if err := json.Unmarshal([]byte(st.body), &parsed); err != nil {
				return rejectPromise(in, fmt.Errorf("Request.json: %w", err))
			}
			return resolvePromise(in, in.ValueOf(parsed))
		}, 0)))
	proto.Set("arrayBuffer", jsc.FunctionValue(jsc.NewNativeFunction("arrayBuffer",
		func(in *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			st := requestStateOf(this)
			if st == nil {
				return rejectPromise(in, fmt.Errorf("Request.arrayBuffer: not a Request"))
			}
			st.bodyUsed = true
			return resolvePromise(in, jsc.StringValue(st.body))
		}, 0)))
}

// registerRequestCtor 注册全局 Request 构造器（幂等）。
func registerRequestCtor(in *jsc.Interpreter) {
	if in == nil {
		return
	}
	if v, ok := in.GlobalObject().GetByKey("Request"); ok && v.IsFunction() {
		return
	}
	proto := jsc.NewObject(in.ObjectPrototype())
	proto.SetClassName("RequestPrototype")
	installRequestMethods(proto)
	ctor := in.NewConstructor("Request", func(i *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
		st := &requestState{method: "GET", credentials: "same-origin", mode: "cors", cache: "default"}
		if len(args) > 0 {
			// new Request(existingRequest)
			if rs := requestStateOf(args[0]); rs != nil {
				cp := *rs
				cp.headers = cloneHeadersState(rs.headers)
				return newRequestInstance(i, &cp)
			}
			st.url = jscToString(args[0])
		}
		if len(args) > 1 && args[1].IsObject() {
			if init := args[1].AsObject(); init != nil {
				if v, ok := init.GetByKey("method"); ok && !v.IsUndefined() {
					st.method = strings.ToUpper(jscToString(v))
				}
				if v, ok := init.GetByKey("body"); ok && !v.IsUndefined() && !v.IsNull() {
					st.body = jscToString(v)
				}
				if v, ok := init.GetByKey("headers"); ok {
					st.headers = newHeadersState()
					headersApplyInit(i, st.headers, v)
				}
				if v, ok := init.GetByKey("credentials"); ok && !v.IsUndefined() {
					st.credentials = jscToString(v)
				}
				if v, ok := init.GetByKey("mode"); ok && !v.IsUndefined() {
					st.mode = jscToString(v)
				}
				if v, ok := init.GetByKey("cache"); ok && !v.IsUndefined() {
					st.cache = jscToString(v)
				}
			}
		}
		return newRequestInstance(i, st)
	})
	ctorObj := jsc.FunctionValue(ctor).AsObject()
	if ctorObj != nil {
		ctorObj.Set("prototype", jsc.ObjectValue(proto))
	}
	proto.Set("constructor", jsc.FunctionValue(ctor))
	in.GlobalObject().Set("Request", jsc.FunctionValue(ctor))
}

// registerWebAPIObjects 注册 fetch 家族的全局对象（幂等；每次 RegisterFetch 时
// 调用，保证同一解释器只装一次、且 Response.headers 能拿到 Headers 原型）。
func registerWebAPIObjects(in *jsc.Interpreter) {
	registerHeadersCtor(in)
	registerResponseCtor(in)
	registerRequestCtor(in)
}
