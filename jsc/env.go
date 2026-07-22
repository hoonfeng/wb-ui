// Package jsc — Browser environment polyfill injection.
//
// InjectBrowserEnv 在 JS 运行时中注入浏览器全局 API。
//
// 调用方（webkit.WebView）在 ensureJSRuntime 之后调用本函数，
// 确保所有 JS 代码执行前浏览器全局 API 已可用。

package jsc

import "wb-ui.com/goja"

// InjectBrowserEnv 注入浏览器环境全局 API 到 JS 运行时。
// 调用时机：ensureJSRuntime() 之后，LoadHTML() 之前。
func (r *Interpreter) InjectBrowserEnv() {
	r.injectPolyfills()
	r.injectTimers()
	r.injectNetStubs()
}

// ─── 纯 JS polyfill ──────────────────────────────────────

func (r *Interpreter) injectPolyfills() {
	polyfills := []string{
		// Object 扩展
		`if(!Object.getPrototypeOf)Object.getPrototypeOf=function(o){return o&&o.constructor?o.constructor.prototype:null}`,
		`if(!Object.setPrototypeOf)Object.setPrototypeOf=function(o,p){o.__proto__=p;return o}`,

		// Array.from
		`if(!Array.from)Array.from=function(a,fn,ctx){var r=[];for(var i=0;i<a.length;i++)r.push(fn?fn.call(ctx||null,a[i],i):a[i]);return r}`,

		// TextEncoder / TextDecoder
		`if(typeof TextEncoder=='undefined')TextEncoder=function(){this.encode=function(s){var a=new Uint8Array(s.length);for(var i=0;i<s.length;i++)a[i]=s.charCodeAt(i);return a}}`,
		`if(typeof TextDecoder=='undefined')TextDecoder=function(){this.decode=function(a){return String.fromCharCode.apply(null,a)}}`,

		// structuredClone
		`if(typeof structuredClone=='undefined')structuredClone=function(o){return JSON.parse(JSON.stringify(o))}`,

		// crypto.getRandomValues
		`if(typeof crypto=='undefined')crypto={getRandomValues:function(arr){for(var i=0;i<arr.length;i++)arr[i]=Math.floor(Math.random()*256);return arr},randomUUID:function(){return'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g,function(c){var r=Math.random()*16|0;return(c=='x'?r:r&0x3|0x8).toString(16)})}}`,

		// navigator
		`if(typeof navigator=='undefined')navigator={userAgent:'PairCode/1.0',platform:'Win32',language:'zh-CN'}`,

		// performance
		`if(typeof performance=='undefined')performance={now:function(){return Date.now()},mark:function(){},measure:function(){},getEntriesByName:function(){return[]}}`,

		// queueMicrotask — 如果有 EventLoop 就用 RunJS 注册
		// (见 injectTimers)，这里用 Promise 做回退
		`if(typeof queueMicrotask=='undefined'&&typeof Promise!='undefined')queueMicrotask=function(fn){Promise.resolve().then(function(){try{fn()}catch(e){console.error(e)}})}`,

		// window / globalThis 别名
		`if(typeof window=='undefined')var window=this;if(typeof globalThis=='undefined')var globalThis=this;`,
	}
	for _, p := range polyfills {
		r.vm.RunString(p)
	}
}

// ─── 定时器 ───────────────────────────────────────────────

func (r *Interpreter) injectTimers() {
	if r.eventLoop == nil {
		// 没有 EventLoop — 同步桩（测试用）
		r.vm.RunString(`if(typeof setTimeout=='undefined'){window.setTimeout=function(fn){try{if(typeof fn=='function')fn()}catch(e){};return 0};window.clearTimeout=function(){}}`)
		r.vm.RunString(`if(typeof setInterval=='undefined'){window.setInterval=function(){return 0};window.clearInterval=function(){}}`)
		r.vm.RunString(`if(typeof requestAnimationFrame=='undefined'){window.requestAnimationFrame=function(fn){try{if(typeof fn=='function')fn()}catch(e){};return 0};window.cancelAnimationFrame=function(){}}`)
		return
	}

	el := r.eventLoop

	// setTimeout(fn, delay) — 在下一行定义的 fn 闭包中使用
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

	// setInterval(fn, interval)
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

	// requestAnimationFrame(fn)
	r.vm.Set("requestAnimationFrame", func(call goja.FunctionCall) goja.Value {
		cb := JSValue{v: call.Argument(0), interp: r}
		id := el.RequestAnimationFrame(cb)
		return r.vm.ToValue(float64(id))
	})
	r.vm.Set("cancelAnimationFrame", func(call goja.FunctionCall) goja.Value {
		el.CancelAnimationFrame(int(call.Argument(0).ToInteger()))
		return goja.Undefined()
	})

	// queueMicrotask(fn)
	r.vm.Set("queueMicrotask", func(call goja.FunctionCall) goja.Value {
		cb := JSValue{v: call.Argument(0), interp: r}
		el.QueueMicrotask(cb)
		return goja.Undefined()
	})
}

// ─── 网络桩 ───────────────────────────────────────────────

func (r *Interpreter) injectNetStubs() {
	r.vm.RunString(`if(typeof fetch=='undefined')fetch=function(url,opts){return new Promise(function(resolve){resolve({ok:true,status:200,json:function(){return Promise.resolve({})},text:function(){return Promise.resolve('')},headers:new Map()})})}`)
	r.vm.RunString(`if(typeof WebSocket=='undefined')WebSocket=function(url,protocols){this.url=url;this.readyState=0;this.send=function(){};this.close=function(){};this.addEventListener=function(){}}`)
}
