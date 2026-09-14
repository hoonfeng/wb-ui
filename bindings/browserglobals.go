// 浏览器能力全局的按模式裁剪（UI 库模式）。
//
// RegisterDOMBindings 会无条件注册全部浏览器全局（DOM/window/WebSocket/
// Worker/...），这是「嵌入浏览器」语义需要的。当宿主把 wb-ui 当 UI 库
// 用时（webkit.ModeToolkit），引擎提供的能力应当可预测：页面不该以为
// 自己能起线程（Worker）或开长连接（WebSocket）——库的 feature detect
// （typeof Worker === "undefined"）必须得到正确结论，否则会在运行时
// 走到不存在的实现上。
//
// 实现方式：把对应全局从 global 上**真正删除**（jsc.JSObject.Delete →
// goja Object.Delete）。因此 `typeof Worker === "undefined"` 与
// `"Worker" in window === false` 都成立——与浏览器里「这个 API 不存在」完全
// 一致。属性不可配置（删除失败）时退回置 undefined，至少保证 typeof 判定正确。

package bindings

import (
	"wb-ui/jsc"
)

// HideGlobal 把 JS 全局 name 从 global 上删除（幂等）；删除失败时置 undefined。
func HideGlobal(rt *jsc.Interpreter, name string) {
	if rt == nil || name == "" {
		return
	}
	g := rt.GlobalObject()
	if g == nil {
		return
	}
	if g.Delete(name) {
		return
	}
	g.Set(name, jsc.Undefined())
}

// HideBrowserThreadGlobals 隐藏浏览器并发/长连接全局：Worker（真实
// 线程 + 独立 goja 运行时）与 WebSocket。UI 库模式（
// webkit.ModeToolkit）在 RegisterDOMBindings 之后调用它。
//
// MessageEvent 不在此列：它是通用事件构造器（postMessage/自定义事件、
// 组件库都用），隐藏它会误伤页面代码。
func HideBrowserThreadGlobals(rt *jsc.Interpreter) {
	for _, name := range []string{"Worker", "WebSocket"} {
		HideGlobal(rt, name)
	}
}
