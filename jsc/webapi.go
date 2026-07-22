// Package jsc — Browser-standard Web APIs implemented in Go.
//
// These are NOT JS polyfills. Each API is implemented as a Go native function,
// using Go's standard library and goja's typed array support.
// This is the correct translation approach matching wb-ui's WebKit heritage.

package jsc

import (
	"crypto/rand"
	"strconv"
	"time"

	"wb-ui.com/goja"
)

// RegisterWebAPIs registers browser-standard Web APIs as Go native functions.
// Called from webkit.ensureJSRuntime after the interpreter is created and
// InjectBrowserEnv has set up the EventLoop-based timers.
func (r *Interpreter) RegisterWebAPIs() {
	r.registerTextEncoder()
	r.registerTextDecoder()
	r.registerCrypto()
	r.registerStructuredClone()
	r.registerPerformance()
	r.registerNavigator()
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
