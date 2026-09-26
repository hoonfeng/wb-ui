// Package jsc — Browser-standard Web APIs implemented in Go.
//
// These are NOT JS polyfills. Each API is implemented as a Go native function,
// using Go's standard library and goja's typed array support.
// This is the correct translation approach matching wb-ui's WebKit heritage.

package jsc

import (
	"crypto/rand"
	"errors"
	"strconv"
	"time"

	"wb-ui/engine/js/goja"
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
}

// ── TextEncoder ─────────────────────────────────────────

func (r *Interpreter) registerTextEncoder() {
	r.vm.Set("TextEncoder", func(call goja.ConstructorCall) *goja.Object {
		obj := r.vm.NewObject()

		obj.Set("encoding", r.vm.ToValue("utf-8"))

		// encode(s: string) → Uint8Array
		obj.Set("encode", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
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
		obj.Set("encodeInto", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
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
	r.vm.Set("TextDecoder", func(call goja.ConstructorCall) *goja.Object {
		obj := r.vm.NewObject()
		obj.Set("encoding", r.vm.ToValue("utf-8"))
		obj.Set("fatal", r.vm.ToValue(false))
		obj.Set("ignoreBOM", r.vm.ToValue(false))

		// decode(input?: Uint8Array) → string
		obj.Set("decode", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
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
	cryptoObj.Set("getRandomValues", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
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
	r.vm.Set("structuredClone", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		jsonFn := r.vm.Get("JSON").ToObject(r.vm).Get("stringify")
		jsonCall, _ := r.vm.Call(jsonFn, goja.Undefined(), call.Argument(0))
		if jsonCall == nil {
			return goja.Undefined()
		}
		v, err := r.vm.RunString(jsonCall.String())
		if err != nil {
			return goja.Undefined()
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
	r.vm.Set("WeakRef", r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("WeakRef: 1 argument required, but only 0 present."))
		}
		target := call.Argument(0)
		if target == nil || target == goja.Undefined() || target == goja.Null() {
			panic(r.vm.NewTypeError("WeakRef: target must be an object"))
		}
		obj := r.vm.NewObject()
		obj.Set("deref", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			return target
		}))
		return obj
	}))

	r.vm.Set("FinalizationRegistry", r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		obj := r.vm.NewObject()
		obj.Set("register", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			return goja.Undefined()
		}))
		obj.Set("unregister", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			return r.vm.ToValue(false)
		}))
		return obj
	}))
}

// ── performance.now ────────────────────────────────────

func (r *Interpreter) registerPerformance() {
	start := time.Now()
	perfObj := r.vm.NewObject()
	perfObj.Set("now", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
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
		fn   goja.Value
		once bool
	}
	type signalState struct {
		obj     *goja.Object
		aborted bool
		reason  goja.Value
		ls      map[string][]listener
	}
	// abortReason：无 reason 时用 Error("signal is aborted without reason")，
	// 语义对齐浏览器（DOMException AbortError）。
	abortReason := func(reason goja.Value) goja.Value {
		if reason != nil && !goja.IsUndefined(reason) && !goja.IsNull(reason) {
			return reason
		}
		return r.vm.NewGoError(errors.New("signal is aborted without reason"))
	}
	// dispatch：先走 addEventListener 的监听器，再走 on<type> 属性处理器。
	dispatch := func(st *signalState, ev *goja.Object, typ string) {
		ls := st.ls[typ]
		delete(st.ls, typ)
		for _, l := range ls {
			if fn, ok := goja.AssertFunction(l.fn); ok {
				_, _ = fn(st.obj, ev)
			}
		}
		if h := st.obj.Get("on" + typ); h != nil {
			if fn, ok := goja.AssertFunction(h); ok {
				_, _ = fn(st.obj, ev)
			}
		}
	}
	makeSignal := func() *signalState {
		st := &signalState{ls: map[string][]listener{}}
		obj := r.vm.NewObject()
		obj.Set("aborted", r.vm.ToValue(false))
		obj.Set("reason", goja.Undefined())
		st.obj = obj
		obj.Set("addEventListener", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if _, ok := goja.AssertFunction(call.Argument(1)); ok {
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
			return goja.Undefined()
		}))
		obj.Set("removeEventListener", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
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
			return goja.Undefined()
		}))
		obj.Set("dispatchEvent", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			ev, ok := call.Argument(0).(*goja.Object)
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
		obj.Set("throwIfAborted", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if st.aborted {
				panic(st.reason)
			}
			return goja.Undefined()
		}))
		return st
	}
	doAbort := func(st *signalState, reason goja.Value) {
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
	r.vm.Set("AbortController", func(call goja.ConstructorCall) *goja.Object {
		st := makeSignal()
		ctrl := r.vm.NewObject()
		ctrl.Set("signal", st.obj)
		ctrl.Set("abort", r.vm.ToValue(func(call2 goja.FunctionCall) goja.Value {
			var reason goja.Value
			if len(call2.Arguments) >= 1 {
				reason = call2.Argument(0)
			}
			doAbort(st, reason)
			return goja.Undefined()
		}))
		return ctrl
	})
	// AbortSignal：全局构造器 + 静态 abort(reason)（规范里 signal 不可 new，
	// 但深拷贝/结构化克隆场景会 `AbortSignal.abort(reason)`）。
	sigCtor := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		return makeSignal().obj
	})
	if o := sigCtor.ToObject(r.vm); o != nil {
		o.Set("abort", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			var reason goja.Value
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

