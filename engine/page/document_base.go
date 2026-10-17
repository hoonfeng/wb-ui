package page

// 文档基地址登记表：解释器 → 当前文档 URL 提供器。
//
// 浏览器语义：页面里写下的相对引用（`<link href="a.css">`、`<script
// src="app.js">`、`fetch("/api")`、`xhr.open("GET", "items.json")`）都以
// **文档 URL** 为基准解析成绝对 URL 后再取内容（解析函数见
// dom.ResolveURL）。引擎此前把引用原样交给加载方，于是「能联网」只对绝对
// URL 成立——真实网页几乎必然写相对路径，这是 LoadURL 加载真实页面后暴露
// 的第一个缺口（相对 CSS 读不到、fetch("/x") 报 unsupported protocol
// scheme ""）。
//
// 登记的是**提供器**而不是字符串快照，以便跟随导航：LoadURL 换文档后，
// 同一解释器上的 fetch/XHR 必须立刻按新文档 URL 解析。WebView 在装配时
// 登记、销毁时摘除（否则这张全局表永久持有解释器闭包与已销毁的 WebView）。

import (
	"sync"

	"wb-ui/engine/js/jsc"
)

var (
	documentBaseProvidersMu sync.RWMutex
	documentBaseProviders   = map[*jsc.Interpreter]func() string{}

	documentURLProvidersMu sync.RWMutex
	documentURLProviders   = map[*jsc.Interpreter]func() string{}
)

// SetDocumentBaseProvider 登记解释器所属文档的 URL 提供器（nil 等于摘除）。
func SetDocumentBaseProvider(rt *jsc.Interpreter, fn func() string) {
	if rt == nil {
		return
	}
	documentBaseProvidersMu.Lock()
	defer documentBaseProvidersMu.Unlock()
	if fn == nil {
		delete(documentBaseProviders, rt)
		return
	}
	documentBaseProviders[rt] = fn
}

// ClearDocumentBaseProvider 摘除登记（WebView 销毁时调用）。
func ClearDocumentBaseProvider(rt *jsc.Interpreter) {
	SetDocumentBaseProvider(rt, nil)
}

// DocumentBase 返回解释器所属文档的当前 URL；未登记时为 ""。
func DocumentBase(rt *jsc.Interpreter) string {
	if rt == nil {
		return ""
	}
	documentBaseProvidersMu.RLock()
	fn := documentBaseProviders[rt]
	documentBaseProvidersMu.RUnlock()
	if fn == nil {
		return ""
	}
	return fn()
}

// 文档 URL 与解析基准是**两个概念**，必须分开登记：
//
//   - 文档 URL = 浏览器地址栏里的地址（`location.href`），决定**源**（Origin）；
//   - 解析基准 = `document.baseURI`，受 `<base href>` 影响，只决定相对引用
//     解析到哪个绝对 URL。
//
// 两者混用会打开一个真实的绕过通道：页面写 `<base href="http://evil.test/">`
// 后，若 CORS 用 baseURI 算文档源，请求就会声称自己来自 evil.test——跨源
// 限制被页面自己「改写」掉。浏览器里 `<base href>` 只影响 URL 解析，**从不**
// 改变文档的源。

// SetDocumentURLProvider 登记解释器所属**文档 URL**（不含 `<base href>` 影响）
// 的提供器；nil 等于摘除。
func SetDocumentURLProvider(rt *jsc.Interpreter, fn func() string) {
	if rt == nil {
		return
	}
	documentURLProvidersMu.Lock()
	defer documentURLProvidersMu.Unlock()
	if fn == nil {
		delete(documentURLProviders, rt)
		return
	}
	documentURLProviders[rt] = fn
}

// ClearDocumentURLProvider 摘除文档 URL 登记（WebView 销毁时调用）。
func ClearDocumentURLProvider(rt *jsc.Interpreter) {
	SetDocumentURLProvider(rt, nil)
}

// DocumentURL 返回解释器所属文档的当前 URL（**不是** baseURI）；未登记时为 ""。
//
// CORS 的文档源只能从这里取：baseURI 可能被页面用 `<base href>` 改写。
func DocumentURL(rt *jsc.Interpreter) string {
	if rt == nil {
		return ""
	}
	documentURLProvidersMu.RLock()
	fn := documentURLProviders[rt]
	documentURLProvidersMu.RUnlock()
	if fn == nil {
		return ""
	}
	return fn()
}
