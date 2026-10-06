// Package jsc — Browser-standard Web APIs implemented in Go.
//
// These are NOT JS polyfills. Each API is implemented as a Go native function,
// using Go's standard library and goja's typed array support.
// This is the correct translation approach matching wb-ui's WebKit heritage.

package jsc

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strconv"
	"time"
)

// RegisterWebAPIs registers browser-standard Web APIs as Go native functions.
// Called from webkit.ensureJSRuntime after the interpreter is created and
// InjectBrowserEnv has set up the EventLoop-based timers.
func (r *Interpreter) RegisterWebAPIs() {
	r.registerTextEncoder()
	r.registerTextDecoder()
	r.registerStreams()
	r.registerCrypto()
	r.registerStructuredClone()
	r.registerPerformance()
	r.registerNavigator()
	r.registerAbortController()
	r.registerWeakRef()
	r.registerBlob()
	r.registerFileReader()
}

// ── Blob / File / FileReader ─────────────────────────────

// blobDataKey 是 Blob 原始字节的内部承载字段名。Go string 可无损承载任意字节，
// 且跨 Go↔JS 边界不需要逐元素拷贝；fetch 的 Response.blob() 与 FileReader 都按
// 这个约定读写，因而无需在包之间共享具体类型。
const blobDataKey = "__wbBlobData"

// promiseResolved 返回一个已解决的 Promise（包装 value）。
func (r *Interpreter) promiseResolved(v beValue) beValue {
	pr, resolve, _ := r.vm.NewPromise()
	_ = resolve(v)
	return r.vm.ToValue(pr)
}

// uint8ArrayValue 把字节切片转成 Uint8Array（无 Uint8Array 时退化为字符串）。
func (r *Interpreter) uint8ArrayValue(data []byte) beValue {
	ctor := r.vm.Get("Uint8Array")
	if ctor == nil {
		return r.vm.ToValue(string(data))
	}
	arr, err := r.vm.New(ctor, r.vm.ToValue(int64(len(data))))
	if err != nil {
		return r.vm.ToValue(string(data))
	}
	for i, b := range data {
		arr.Set(strconv.Itoa(i), r.vm.ToValue(int64(b)))
	}
	return arr
}

// blobPartBytes 把 Blob 构造参数的一个 part 转成字节：
// 另一个 Blob（内部字节字段）> TypedArray/Array（按元素）> 其他（字符串化）。
func (r *Interpreter) blobPartBytes(p beValue) []byte {
	if p == nil || beIsUndefined(p) || beIsNull(p) {
		return nil
	}
	if o := p.ToObject(r.vm); o != nil {
		if d := o.Get(blobDataKey); d != nil && !beIsUndefined(d) && !beIsNull(d) {
			return []byte(d.String())
		}
		if lv := o.Get("length"); lv != nil && !beIsUndefined(lv) && !beIsNull(lv) {
			n := int(lv.ToInteger())
			buf := make([]byte, 0, n)
			for i := 0; i < n; i++ {
				ev := o.Get(strconv.Itoa(i))
				if ev == nil || beIsUndefined(ev) || beIsNull(ev) {
					buf = append(buf, 0)
					continue
				}
				buf = append(buf, byte(ev.ToInteger()))
			}
			return buf
		}
	}
	return []byte(p.String())
}

func (r *Interpreter) registerBlob() {
	r.vm.Set("Blob", func(call beConstructorCall) *beObject {
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
		mimeType := ""
		if len(call.Arguments) >= 2 {
			if opts := call.Argument(1).ToObject(r.vm); opts != nil {
				if t := opts.Get("type"); t != nil && !beIsUndefined(t) && !beIsNull(t) {
					mimeType = t.String()
				}
			}
		}
		obj.Set("size", r.vm.ToValue(int64(len(data))))
		obj.Set("type", r.vm.ToValue(mimeType))
		obj.Set(blobDataKey, r.vm.ToValue(string(data)))
		obj.Set("text", r.vm.ToValue(func(call2 beFunctionCall) beValue {
			return r.promiseResolved(r.vm.ToValue(string(data)))
		}))
		obj.Set("arrayBuffer", r.vm.ToValue(func(call2 beFunctionCall) beValue {
			return r.promiseResolved(r.uint8ArrayValue(data))
		}))
		obj.Set("slice", r.vm.ToValue(func(call2 beFunctionCall) beValue {
			start, end := 0, len(data)
			if len(call2.Arguments) >= 1 {
				start = int(call2.Argument(0).ToInteger())
			}
			if len(call2.Arguments) >= 2 {
				end = int(call2.Argument(1).ToInteger())
			}
			if start < 0 {
				start += len(data)
			}
			if end < 0 {
				end += len(data)
			}
			if start < 0 {
				start = 0
			}
			if end > len(data) {
				end = len(data)
			}
			if start > end {
				start = end
			}
			sub := r.vm.NewObject()
			sub.Set("size", r.vm.ToValue(int64(end-start)))
			sub.Set("type", r.vm.ToValue(mimeType))
			sub.Set(blobDataKey, r.vm.ToValue(string(data[start:end])))
			return r.vm.ToValue(sub)
		}))
		return obj
	})
}

func (r *Interpreter) registerFileReader() {
	r.vm.Set("FileReader", func(call beConstructorCall) *beObject {
		obj := r.vm.NewObject()
		obj.Set("EMPTY", r.vm.ToValue(int64(0)))
		obj.Set("LOADING", r.vm.ToValue(int64(1)))
		obj.Set("DONE", r.vm.ToValue(int64(2)))
		obj.Set("readyState", r.vm.ToValue(int64(0)))
		obj.Set("result", beNull())
		obj.Set("error", beNull())

		// fire 依次触发 onX 内容属性与 addEventListener 注册的同名监听器。
		fire := func(typ string) {
			ev := r.vm.NewObject()
			ev.Set("type", r.vm.ToValue(typ))
			ev.Set("target", obj)
			ev.Set("currentTarget", obj)
			if h := obj.Get("on" + typ); h != nil && !beIsUndefined(h) && !beIsNull(h) {
				if fn, ok := beAssertFunction(h); ok {
					_, _ = fn(obj, ev)
				}
			}
			if lo := obj.Get("__wbFRListeners"); lo != nil && !beIsUndefined(lo) && !beIsNull(lo) {
				if lobj := lo.ToObject(r.vm); lobj != nil {
					if av := lobj.Get(typ); av != nil && !beIsUndefined(av) && !beIsNull(av) {
						if ao := av.ToObject(r.vm); ao != nil {
							if nv := ao.Get("length"); nv != nil {
								for i := 0; i < int(nv.ToInteger()); i++ {
									if f, ok := beAssertFunction(ao.Get(strconv.Itoa(i))); ok {
										_, _ = f(obj, ev)
									}
								}
							}
						}
					}
				}
			}
		}

		read := func(blob beValue, mode string) {
			var data []byte
			mimeType := ""
			if blob != nil && !beIsUndefined(blob) && !beIsNull(blob) {
				if bo := blob.ToObject(r.vm); bo != nil {
					if d := bo.Get(blobDataKey); d != nil && !beIsUndefined(d) && !beIsNull(d) {
						data = []byte(d.String())
					} else {
						data = r.blobPartBytes(blob)
					}
					if t := bo.Get("type"); t != nil && !beIsUndefined(t) && !beIsNull(t) {
						mimeType = t.String()
					}
				}
			}
			obj.Set("readyState", r.vm.ToValue(int64(1)))
			doRead := func() {
				switch mode {
				case "text":
					obj.Set("result", r.vm.ToValue(string(data)))
				case "dataurl":
					obj.Set("result", r.vm.ToValue("data:"+mimeType+";base64,"+
						base64.StdEncoding.EncodeToString(data)))
				case "arraybuffer":
					obj.Set("result", r.uint8ArrayValue(data))
				default: // binarystring
					obj.Set("result", r.vm.ToValue(string(data)))
				}
				obj.Set("readyState", r.vm.ToValue(int64(2)))
				fire("load")
				fire("loadend")
			}
			// 规范要求异步完成（事件在调用返回后触发）；有 setTimeout 就走异步。
			if tm := r.vm.Get("setTimeout"); tm != nil && !beIsUndefined(tm) && !beIsNull(tm) {
				if f, ok := beAssertFunction(tm); ok {
					_, _ = f(beUndefined(),
						r.vm.ToValue(func(beFunctionCall) beValue {
							doRead()
							return beUndefined()
						}),
						r.vm.ToValue(int64(0)))
					return
				}
			}
			doRead()
		}

		obj.Set("readAsText", r.vm.ToValue(func(call2 beFunctionCall) beValue {
			read(call2.Argument(0), "text")
			return beUndefined()
		}))
		obj.Set("readAsDataURL", r.vm.ToValue(func(call2 beFunctionCall) beValue {
			read(call2.Argument(0), "dataurl")
			return beUndefined()
		}))
		obj.Set("readAsArrayBuffer", r.vm.ToValue(func(call2 beFunctionCall) beValue {
			read(call2.Argument(0), "arraybuffer")
			return beUndefined()
		}))
		obj.Set("readAsBinaryString", r.vm.ToValue(func(call2 beFunctionCall) beValue {
			read(call2.Argument(0), "binarystring")
			return beUndefined()
		}))
		obj.Set("abort", r.vm.ToValue(func(call2 beFunctionCall) beValue {
			obj.Set("readyState", r.vm.ToValue(int64(2)))
			obj.Set("result", beNull())
			fire("abort")
			fire("loadend")
			return beUndefined()
		}))
		obj.Set("addEventListener", r.vm.ToValue(func(call2 beFunctionCall) beValue {
			typ := call2.Argument(0).String()
			var lobj *beObject
			if lo := obj.Get("__wbFRListeners"); lo == nil || beIsUndefined(lo) || beIsNull(lo) {
				lobj = r.vm.NewObject()
				obj.Set("__wbFRListeners", r.vm.ToValue(lobj))
			} else {
				lobj = lo.ToObject(r.vm)
			}
			var arr *beObject
			if av := lobj.Get(typ); av == nil || beIsUndefined(av) || beIsNull(av) {
				arr = r.vm.NewObject()
				arr.Set("length", r.vm.ToValue(int64(0)))
				lobj.Set(typ, r.vm.ToValue(arr))
			} else {
				arr = av.ToObject(r.vm)
			}
			n := int(arr.Get("length").ToInteger())
			arr.Set(strconv.Itoa(n), call2.Argument(1))
			arr.Set("length", r.vm.ToValue(int64(n+1)))
			return beUndefined()
		}))
		obj.Set("removeEventListener", r.vm.ToValue(func(call2 beFunctionCall) beValue {
			return beUndefined()
		}))
		return obj
	})
}

// ── TextEncoder ─────────────────────────────────────────

func (r *Interpreter) registerTextEncoder() {
	r.vm.Set("TextEncoder", func(call beConstructorCall) *beObject {
		obj := r.vm.NewObject()

		obj.Set("encoding", r.vm.ToValue("utf-8"))

		// encode(s: string) → Uint8Array
		obj.Set("encode", r.vm.ToValue(func(call beFunctionCall) beValue {
			s := call.Argument(0).String()
			n := len(s)
			ctor := r.vm.Get("Uint8Array")
			arr, err := r.vm.New(ctor, r.vm.ToValue(int64(n)))
			if err != nil {
				panic(r.vm.NewTypeError("TextEncoder.encode: %v", err))
			}
			for i := 0; i < n; i++ {
				arr.Set(strconv.Itoa(i), r.vm.ToValue(int64(s[i])))
			}
			return arr
		}))

		// encodeInto(source: string, destination: Uint8Array) → { read, written }
		obj.Set("encodeInto", r.vm.ToValue(func(call beFunctionCall) beValue {
			s := call.Argument(0).String()
			destObj := call.Argument(1).ToObject(r.vm)
			var destLen int64
			if lv := destObj.Get("length"); lv != nil {
				destLen = lv.ToInteger()
			}
			n := int64(len(s))
			if n > destLen {
				n = destLen
			}
			for i := int64(0); i < n; i++ {
				destObj.Set(strconv.FormatInt(i, 10), r.vm.ToValue(int64(s[i])))
			}
			result := r.vm.NewObject()
			result.Set("read", r.vm.ToValue(n))
			result.Set("written", r.vm.ToValue(n))
			return result
		}))

		return obj
	})
}

// ── TextDecoder ─────────────────────────────────────────

func (r *Interpreter) registerTextDecoder() {
	r.vm.Set("TextDecoder", func(call beConstructorCall) *beObject {
		obj := r.vm.NewObject()
		obj.Set("encoding", r.vm.ToValue("utf-8"))
		obj.Set("fatal", r.vm.ToValue(false))
		obj.Set("ignoreBOM", r.vm.ToValue(false))

		// decode(input?: Uint8Array) → string
		obj.Set("decode", r.vm.ToValue(func(call beFunctionCall) beValue {
			if len(call.Arguments) == 0 {
				return r.vm.ToValue("")
			}
			arg := call.Argument(0)
			argObj := arg.ToObject(r.vm)
			length := int(argObj.Get("length").ToInteger())
			bytes := make([]byte, length)
			for i := 0; i < length; i++ {
				idx := strconv.Itoa(i)
				b := int(argObj.Get(idx).ToInteger())
				if b > 255 {
					b = 0xFFFD // replacement character for invalid UTF-8
				}
				bytes[i] = byte(b)
			}
			return r.vm.ToValue(string(bytes))
		}))

		return obj
	})
}

// ── crypto.getRandomValues ──────────────────────────────

func (r *Interpreter) registerCrypto() {
	cryptoObj := r.vm.NewObject()
	cryptoObj.Set("getRandomValues", r.vm.ToValue(func(call beFunctionCall) beValue {
		arg := call.Argument(0)
		argObj := arg.ToObject(r.vm)
		length := int(argObj.Get("length").ToInteger())
		buf := make([]byte, length)
		if _, err := rand.Read(buf); err != nil {
			panic(r.vm.NewTypeError("crypto.getRandomValues: %v", err))
		}
		for i := 0; i < length; i++ {
			argObj.Set(strconv.Itoa(i), r.vm.ToValue(int64(buf[i])))
		}
		return arg
	}))

	r.vm.Set("crypto", cryptoObj)
}

// ── structuredClone ─────────────────────────────────────

func (r *Interpreter) registerStructuredClone() {
	// structuredClone(value) — deep copy using goja's JSON round-trip.
	// Covers Pinia/Vue use cases (plain objects, arrays, primitives).
	r.vm.Set("structuredClone", func(call beFunctionCall) beValue {
		if len(call.Arguments) == 0 {
			return beUndefined()
		}
		jsonFn := r.vm.Get("JSON").ToObject(r.vm).Get("stringify")
		jsonCall, _ := r.vm.Call(jsonFn, beUndefined(), call.Argument(0))
		if jsonCall == nil {
			return beUndefined()
		}
		v, err := r.vm.RunString(jsonCall.String())
		if err != nil {
			return beUndefined()
		}
		return v
	})
}

// ── WeakRef / FinalizationRegistry ──────────────────────

// registerWeakRef 注册 WeakRef / FinalizationRegistry。
//
// 语义说明（为何是合规的降级实现）：WeakRef 的规范价值是「不阻止目标被回收」，
// 而本引擎的对象由 Go GC 管理、deref() 又必须能返回目标——在没有 GC 钩子的前提下
// 无法实现真弱引用。规范只要求「目标未被回收时返回之」，**允许** deref() 永不返回
// undefined（回收时机是实现细节、不构成可观察保证），故用强引用实现是合规的。
// 目的：让无守卫的 `new WeakRef(x)`（如 Vue 3 遍历 window 链的工具函数、ui-editor
// 区域 bundle 中的同名工具）不再抛 ReferenceError。
// 同理，FinalizationRegistry 的回调永不触发（对应「目标未被回收」），register/
// unregister 按规范分别返回 undefined / false。
func (r *Interpreter) registerWeakRef() {
	r.vm.Set("WeakRef", r.vm.ToValue(func(call beConstructorCall) *beObject {
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("WeakRef: 1 argument required, but only 0 present."))
		}
		target := call.Argument(0)
		if target == nil || target == beUndefined() || target == beNull() {
			panic(r.vm.NewTypeError("WeakRef: target must be an object"))
		}
		obj := r.vm.NewObject()
		obj.Set("deref", r.vm.ToValue(func(beFunctionCall) beValue {
			return target
		}))
		return obj
	}))

	r.vm.Set("FinalizationRegistry", r.vm.ToValue(func(call beConstructorCall) *beObject {
		obj := r.vm.NewObject()
		obj.Set("register", r.vm.ToValue(func(beFunctionCall) beValue {
			return beUndefined()
		}))
		obj.Set("unregister", r.vm.ToValue(func(beFunctionCall) beValue {
			return r.vm.ToValue(false)
		}))
		return obj
	}))
}

// ── performance.now ────────────────────────────────────

func (r *Interpreter) registerPerformance() {
	start := time.Now()
	perfObj := r.vm.NewObject()
	perfObj.Set("now", r.vm.ToValue(func(call beFunctionCall) beValue {
		elapsed := time.Since(start).Seconds() * 1000
		return r.vm.ToValue(elapsed)
	}))
	r.vm.Set("performance", perfObj)
}

// ── navigator ──────────────────────────────────────────

func (r *Interpreter) registerNavigator() {
	navObj := r.vm.NewObject()
	navObj.Set("userAgent", r.vm.ToValue("Mozilla/5.0 (compatible; wb-ui/1.0)"))
	navObj.Set("platform", r.vm.ToValue("Win32"))
	navObj.Set("language", r.vm.ToValue("zh-CN"))
	navObj.Set("onLine", r.vm.ToValue(true))
	r.vm.Set("navigator", navObj)
}

// ── AbortController / AbortSignal ───────────────────────
//
// gou-ide 前端（api.js）对每个请求都 `new AbortController()` 并用
// `ctrl.signal` 做超时/级联取消；引擎缺这个 API 时所有 fetch 调用在
// **构造阶段**就抛 "AbortController is not defined" —— /api/ui-boot、
// /api/plugins 等数据链路全断，桌面端只剩空壳（实测 2026-09-25）。
//
// 实现范围（对齐 WHATWG DOM「AbortController」「AbortSignal」）：
//   - AbortController.prototype.signal（只读属性）+ abort(reason?)
//   - AbortSignal：aborted / reason / onabort / addEventListener('abort')
//     / removeEventListener / dispatchEvent / throwIfAborted
//   - abort() 幂等（只派发一次）；监听器先于 onabort 执行；{once:true} 生效
func (r *Interpreter) registerAbortController() {
	type listener struct {
		fn   beValue
		once bool
	}
	type signalState struct {
		obj     *beObject
		aborted bool
		reason  beValue
		ls      map[string][]listener
	}
	// abortReason：无 reason 时用 Error("signal is aborted without reason")，
	// 语义对齐浏览器（DOMException AbortError）。
	abortReason := func(reason beValue) beValue {
		if reason != nil && !beIsUndefined(reason) && !beIsNull(reason) {
			return reason
		}
		return r.vm.NewGoError(errors.New("signal is aborted without reason"))
	}
	// dispatch：先走 addEventListener 的监听器，再走 on<type> 属性处理器。
	dispatch := func(st *signalState, ev *beObject, typ string) {
		ls := st.ls[typ]
		delete(st.ls, typ)
		for _, l := range ls {
			if fn, ok := beAssertFunction(l.fn); ok {
				_, _ = fn(st.obj, ev)
			}
		}
		if h := st.obj.Get("on" + typ); h != nil {
			if fn, ok := beAssertFunction(h); ok {
				_, _ = fn(st.obj, ev)
			}
		}
	}
	makeSignal := func() *signalState {
		st := &signalState{ls: map[string][]listener{}}
		obj := r.vm.NewObject()
		obj.Set("aborted", r.vm.ToValue(false))
		obj.Set("reason", beUndefined())
		st.obj = obj
		obj.Set("addEventListener", r.vm.ToValue(func(call beFunctionCall) beValue {
			if _, ok := beAssertFunction(call.Argument(1)); ok {
				t := call.Argument(0).String()
				once := false
				if len(call.Arguments) >= 3 {
					if o := call.Argument(2).ToObject(r.vm); o != nil {
						if v := o.Get("once"); v != nil && v.ToBoolean() {
							once = true
						}
					}
				}
				st.ls[t] = append(st.ls[t], listener{fn: call.Argument(1), once: once})
			}
			return beUndefined()
		}))
		obj.Set("removeEventListener", r.vm.ToValue(func(call beFunctionCall) beValue {
			t := call.Argument(0).String()
			cur := st.ls[t]
			out := cur[:0]
			for _, l := range cur {
				if l.fn == call.Argument(1) {
					continue
				}
				out = append(out, l)
			}
			st.ls[t] = out
			return beUndefined()
		}))
		obj.Set("dispatchEvent", r.vm.ToValue(func(call beFunctionCall) beValue {
			ev, ok := call.Argument(0).(*beObject)
			if !ok {
				return r.vm.ToValue(false)
			}
			typ := ""
			if v := ev.Get("type"); v != nil {
				typ = v.String()
			}
			ev.Set("target", obj)
			dispatch(st, ev, typ)
			return r.vm.ToValue(true)
		}))
		obj.Set("throwIfAborted", r.vm.ToValue(func(call beFunctionCall) beValue {
			if st.aborted {
				panic(st.reason)
			}
			return beUndefined()
		}))
		return st
	}
	doAbort := func(st *signalState, reason beValue) {
		if st.aborted {
			return
		}
		st.aborted = true
		st.reason = abortReason(reason)
		st.obj.Set("aborted", r.vm.ToValue(true))
		st.obj.Set("reason", st.reason)
		ev := r.vm.NewObject()
		ev.Set("type", r.vm.ToValue("abort"))
		ev.Set("target", st.obj)
		dispatch(st, ev, "abort")
	}
	r.vm.Set("AbortController", func(call beConstructorCall) *beObject {
		st := makeSignal()
		ctrl := r.vm.NewObject()
		ctrl.Set("signal", st.obj)
		ctrl.Set("abort", r.vm.ToValue(func(call2 beFunctionCall) beValue {
			var reason beValue
			if len(call2.Arguments) >= 1 {
				reason = call2.Argument(0)
			}
			doAbort(st, reason)
			return beUndefined()
		}))
		return ctrl
	})
	// AbortSignal：全局构造器 + 静态 abort(reason)（规范里 signal 不可 new，
	// 但深拷贝/结构化克隆场景会 `AbortSignal.abort(reason)`）。
	sigCtor := r.vm.ToValue(func(call beConstructorCall) *beObject {
		return makeSignal().obj
	})
	if o := sigCtor.ToObject(r.vm); o != nil {
		o.Set("abort", r.vm.ToValue(func(call beFunctionCall) beValue {
			var reason beValue
			if len(call.Arguments) >= 1 {
				reason = call.Argument(0)
			}
			st := makeSignal()
			doAbort(st, reason)
			return st.obj
		}))
	}
	r.vm.Set("AbortSignal", sigCtor)
}
