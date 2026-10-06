// 文档生命周期事件（DOMContentLoaded / load）的宿主派发入口。
//
// Translation of: Source/WebCore/loader/DocumentLoader.cpp（load 派发时机）
//
//	Source/WebCore/dom/Document.cpp（implicitClose → DOMContentLoaded）
//
// Completeness: 70%（未建模：defer/async 脚本完成时机、图片 onload 汇入
//
//	load 计数、document.readyState 的 "interactive" 时序细化）
//
// ★ 缺陷背景（wb-ui 审计 D5 的直接根因）：引擎此前只把 frame.readyState
// 推进到 "complete"，**从不派发 window 的 load，也从不派发 document 的
// DOMContentLoaded**。挂在两者上的页面脚本因此永不执行，而且不抛错——页面
// 静默停在初始状态，极难定位。
//
// 实测（dev/fixtures/css-stack/sticky.html，2026-10-06）：
//
//	window.addEventListener('load', function () { sc.scrollTop = 60; });
//	→ 探针 loadFired=false、scrollTop=0（Edge 为 60）→ 滚动从未发生，
//	  与 Edge 对照的像素差异因此集中在整个滚动容器区域。
//
// 同类受影响写法（都是前端最常见的入口）：window.onload = fn、
// document.addEventListener('DOMContentLoaded', fn)、jQuery 的 $(function(){}),
// 以及「资源/字体就绪后再测量」的库（图表、虚拟列表、布局探针）。
//
// 规范顺序（HTML §3.1.4 "the end"）：
//
//  1. 解析完成 + 同步脚本执行完 → document.readyState = "interactive"
//  2. DOMContentLoaded（document，冒泡）
//  3. 资源全部到位 → document.readyState = "complete"
//  4. load（window）
package bindings

import (
	"wb-ui/engine/dom"
	"wb-ui/engine/js/jsc"
)

// FireDocumentEvent 在 document 上派发一个事件（DOMContentLoaded 等），走
// dom 层事件系统：document 上的监听器按 DOM event flow 收到通知
// （canBubble=true 与浏览器一致；document 自身即 target 与 currentTarget）。
func FireDocumentEvent(rt *jsc.Interpreter, doc *dom.Document, typ string) {
	if rt == nil || doc == nil || typ == "" {
		return
	}
	doc.DispatchEvent(dom.NewEvent(typ, true, false, false))
	// 监听器里可能 setState/改 DOM（Vue/React 的水合续体走 Promise 微任务）。
	rt.RunJobs()
}

// FireWindowEvent 派发 window 级事件（load 等）。
//
// ★ window 在引擎里就是全局对象本身（Install 里 `window → g`），其监听器
// 存放在 windowEventListeners（window.addEventListener 写入）。派发必须覆盖
// **两条互不替代的通路**（规范如此，同 FireResourceEvent 的处理）：
//
//	① window.on<type> = fn 属性处理器；
//	② window.addEventListener(<type>, fn) 注册的监听器（按注册顺序）。
func FireWindowEvent(rt *jsc.Interpreter, typ string) {
	if rt == nil || typ == "" {
		return
	}
	g := rt.GlobalObject()
	if g == nil {
		return
	}
	gv := jsc.ObjectValue(g)
	ev := rt.ObjectPrototype()
	ev.SetClassName("Event")
	ev.Set("type", jsc.StringValue(typ))
	ev.Set("target", gv)
	ev.Set("currentTarget", gv)
	evVal := jsc.ObjectValue(ev)
	if h, ok := g.GetByKey("on" + typ); ok && h.IsCallable() {
		_, _ = rt.Call(h, gv, []jsc.JSValue{evVal})
	}
	// ★ 复制一份再遍历：监听器里可能 removeEventListener（Vue 的 once 包装、
	// 页面脚本在 load 里卸载自身）——边遍历边改 windowEventListeners 会让
	// 切片在迭代中被替换，跳过后续监听器。
	listeners := make([]jsc.JSValue, len(windowEventListeners[typ]))
	copy(listeners, windowEventListeners[typ])
	for _, fn := range listeners {
		if fn.IsCallable() {
			_, _ = rt.Call(fn, gv, []jsc.JSValue{evVal})
		}
	}
	// Promise 微任务：监听器（如 sticky.html）里改滚动位置、Vue 的 nextTick
	// 更新都在微任务里落地，不 flush 则本帧看到的是半成品状态。
	rt.RunJobs()
}
