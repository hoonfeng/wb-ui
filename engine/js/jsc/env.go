// Package jsc — Browser environment exposed from Go implementations.
//
// InjectBrowserEnv 暴露 wb-ui 已有 Go 实现的浏览器全局 API 到 JS 运行时。
// 与 WebKit 翻译一致：每个 API 由 Go 代码实现，不注入 JS polyfill 字符串。
// 缺失的 API 会自然暴露为 "xxx is not defined"，驱动正确的实现工作。
//
// 当前已暴露（Go 实现）：
//   - setTimeout / clearTimeout      → EventLoop.SetTimeout
//   - setInterval / clearInterval    → EventLoop.SetInterval
//   - requestAnimationFrame          → EventLoop.RequestAnimationFrame
//   - queueMicrotask                 → EventLoop.QueueMicrotask
//
// 不在此处的 API（应在对应模块实现）：
//   - TextEncoder / TextDecoder      → jsengine 或 jsc Go NativeFunction
//   - crypto.getRandomValues         → jsc Go NativeFunction
//   - Event / CustomEvent            → bindings RegisterDOMBindings
//   - navigator / performance        → page 模块注册
//   - fetch / XMLHttpRequest         → page.RegisterFetch / page.RegisterXMLHttpRequest
//   - MutationObserver / ResizeObserver → bindings 或新增 observe 包
//   - window / globalThis            → goja 自带，无需额外注入

package jsc

import "wb-ui/engine/js/goja"

// InjectBrowserEnv 注入已有 Go 实现的浏览器 API。
func (r *Interpreter) InjectBrowserEnv() {
	if r.eventLoop == nil {
		return
	}
	el := r.eventLoop

	// ── setTimeout(fn, delay) ──
	r.vm.Set("setTimeout", func(call goja.FunctionCall) goja.Value {
		delay := call.Argument(1).ToInteger()
		if delay < 0 {
			delay = 0
		}
		cb := JSValue{v: call.Argument(0), interp: r}
		id := el.SetTimeout(cb, delay)
		return r.vm.ToValue(float64(id))
	})
	r.vm.Set("clearTimeout", func(call goja.FunctionCall) goja.Value {
		el.ClearTimeout(int(call.Argument(0).ToInteger()))
		return goja.Undefined()
	})

	// ── setInterval(fn, interval) ──
	r.vm.Set("setInterval", func(call goja.FunctionCall) goja.Value {
		interval := call.Argument(1).ToInteger()
		if interval < 1 {
			interval = 1
		}
		cb := JSValue{v: call.Argument(0), interp: r}
		id := el.SetInterval(cb, interval)
		return r.vm.ToValue(float64(id))
	})
	r.vm.Set("clearInterval", func(call goja.FunctionCall) goja.Value {
		el.ClearInterval(int(call.Argument(0).ToInteger()))
		return goja.Undefined()
	})

	// ── requestAnimationFrame(fn) ──
	r.vm.Set("requestAnimationFrame", func(call goja.FunctionCall) goja.Value {
		cb := JSValue{v: call.Argument(0), interp: r}
		id := el.RequestAnimationFrame(cb)
		return r.vm.ToValue(float64(id))
	})
	r.vm.Set("cancelAnimationFrame", func(call goja.FunctionCall) goja.Value {
		el.CancelAnimationFrame(int(call.Argument(0).ToInteger()))
		return goja.Undefined()
	})

	// ── queueMicrotask(fn) ──
	r.vm.Set("queueMicrotask", func(call goja.FunctionCall) goja.Value {
		cb := JSValue{v: call.Argument(0), interp: r}
		el.QueueMicrotask(cb)
		return goja.Undefined()
	})
}
