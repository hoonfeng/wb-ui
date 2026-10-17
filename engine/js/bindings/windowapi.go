package bindings

// windowapi.go —— window 的浏览器基础方法（open / close / postMessage /
// alert / confirm / prompt）。
//
// 为什么需要（实测探针）：这些标识符此前全部缺失——`window.open` 读取即抛
// "Object has no member 'open'"，`typeof window.open === "undefined"`。
// 页面里最常见的「导出/下载」按钮就是 `window.open('/api/toolsets/export…')`，
// 一调用就中断整段脚本（后续 UI 状态更新全部不执行）。
//
// 设计：引擎不自己拉窗口/弹系统对话框（跨平台与安全边界都不合适），
// 统一走宿主注入的 hook：
//   - OpenExternalURL：宿主决定打开方式（系统浏览器 / 内部新视图）；
//   - DialogHook：alert/confirm/prompt 的宿主实现；
// 未注入时按浏览器「被阻止/无 UI」语义退化：open 返回 null、confirm 返回
// false、prompt 返回 null、alert 静默 —— 绝不抛错。

import (
	"wb-ui/engine/js/jsc"
)

// windowStr 把 JS 值转成字符串（undefined/null → 空串）。引擎的 JSValue.ToString
// 对数字/布尔/对象都有定义，这里只做空值兜底。
func windowStr(v jsc.JSValue) string {
	if v.IsUndefined() || v.IsNull() {
		return ""
	}
	return v.ToString()
}

// OpenExternalURL 由宿主（webkit 接线）注入：打开一个 URL（通常是系统默认
// 浏览器）。url 是页面写下的原始引用，宿主负责按文档基准解析为绝对 URL。
// 返回 true 表示已交给宿主处理；false/未注入时 window.open 返回 null
// （浏览器语义：弹窗被阻止）。
var OpenExternalURL func(url string) bool

// DialogHookKind 标识同步对话框类型。
type DialogHookKind int

const (
	DialogAlert DialogHookKind = iota
	DialogConfirm
	DialogPrompt
)

// DialogHook 由宿主注入：同步对话框实现。返回 (ok, text)：
//   - alert：ok 忽略，返回值忽略；
//   - confirm：ok 即用户选择；
//   - prompt：ok 为用户确认，text 为输入内容（取消时 ok=false）。
//
// 未注入时按「无 UI」退化（confirm=false / prompt=null），不抛错。
var DialogHook func(kind DialogHookKind, message, defaultValue string) (ok bool, text string)

// registerWindowExtras 注册 window 上的浏览器基础方法。g 是全局对象（window）。
func registerWindowExtras(rt *jsc.Interpreter, g *jsc.JSObject) {
	if rt == nil || g == nil {
		return
	}
	// window.open(url, target, features)
	g.Set("open", jsc.FunctionValue(jsc.NewNativeFunction("open",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			url := ""
			if len(args) > 0 {
				url = windowStr(args[0])
			}
			target := "_blank"
			if len(args) > 1 && !args[1].IsUndefined() && !args[1].IsNull() {
				target = windowStr(args[1])
			}
			if url == "" {
				// 浏览器语义：空 URL 打开 about:blank 新窗口；引擎无新窗口能力。
				return jsc.Null()
			}
			// 只把真正要导航的引用交给宿主；target 由宿主自行决定（当前实现
			// 统一用系统浏览器打开，"内部新视图"属于宿主能力）。
			for _, t := range []string{"_self", "_top", "_parent"} {
				if target == t {
					// 同窗口导航：交给 location.assign（引擎已有实现）。
					if loc, ok := g.GetByKey("location"); ok && loc.IsObject() {
						if assign, ok2 := loc.AsObject().GetByKey("assign"); ok2 && assign.IsCallable() {
							_, _ = rt.Call(assign, loc, []jsc.JSValue{jsc.StringValue(url)})
							return jsc.Null()
						}
					}
					break
				}
			}
			if OpenExternalURL != nil && OpenExternalURL(url) {
				// 返回 null：引擎不提供 WindowProxy（页面通常不检查返回值；
				// 浏览器在弹窗被阻止时同样返回 null）。
				return jsc.Null()
			}
			return jsc.Null()
		}, 3)))

	// window.close()：脚本关闭窗口 —— 引擎里是 no-op（窗口生命周期归宿主）。
	g.Set("close", jsc.FunctionValue(jsc.NewNativeFunction("close",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			return jsc.Undefined()
		}, 0)))

	// window.postMessage(message, targetOrigin, transfer)：投递到本 window 的
	// message 监听器（同窗口/iframe 模拟）。浏览器是异步任务；引擎无独立
	// 事件队列时同步派发，保证 data 不丢（比缺失方法直接抛错安全得多）。
	g.Set("postMessage", jsc.FunctionValue(jsc.NewNativeFunction("postMessage",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) == 0 {
				return jsc.Undefined()
			}
			data := args[0]
			origin := ""
			if len(args) > 1 && !args[1].IsUndefined() && !args[1].IsNull() {
				o := windowStr(args[1])
				if o != "/" && o != "*" {
					origin = o
				}
			}
			ev := jsc.NewObject(in.ObjectPrototype())
			ev.SetClassName("MessageEvent")
			ev.Set("type", jsc.StringValue("message"))
			ev.Set("data", data)
			ev.Set("origin", jsc.StringValue(origin))
			ev.Set("source", jsc.ObjectValue(g))
			ev.Set("lastEventId", jsc.StringValue(""))
			ev.Set("ports", jsc.ObjectValue(jsc.NewArrayForInterp(in, nil)))
			evv := jsc.ObjectValue(ev)
			if listeners, ok := windowEventListeners["message"]; ok {
				for _, fn := range listeners {
					if fn.IsCallable() {
						_, _ = in.Call(fn, jsc.ObjectValue(g), []jsc.JSValue{evv})
					}
				}
			}
			return jsc.Undefined()
		}, 3)))

	// window.alert(message)
	g.Set("alert", jsc.FunctionValue(jsc.NewNativeFunction("alert",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			msg := ""
			if len(args) > 0 {
				msg = windowStr(args[0])
			}
			if DialogHook != nil {
				_, _ = DialogHook(DialogAlert, msg, "")
			}
			return jsc.Undefined()
		}, 1)))

	// window.confirm(message) → bool（无 UI 时 false）
	g.Set("confirm", jsc.FunctionValue(jsc.NewNativeFunction("confirm",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			msg := ""
			if len(args) > 0 {
				msg = windowStr(args[0])
			}
			if DialogHook != nil {
				ok, _ := DialogHook(DialogConfirm, msg, "")
				return jsc.BooleanValue(ok)
			}
			return jsc.BooleanValue(false)
		}, 1)))

	// window.prompt(message, defaultValue) → string | null
	g.Set("prompt", jsc.FunctionValue(jsc.NewNativeFunction("prompt",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			msg := ""
			if len(args) > 0 {
				msg = windowStr(args[0])
			}
			def := ""
			if len(args) > 1 && !args[1].IsUndefined() && !args[1].IsNull() {
				def = windowStr(args[1])
			}
			if DialogHook != nil {
				if ok, text := DialogHook(DialogPrompt, msg, def); ok {
					return jsc.StringValue(text)
				}
				return jsc.Null()
			}
			return jsc.Null()
		}, 2)))
}
