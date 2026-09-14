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

	"wb-ui/jsc"
)

var (
	documentBaseProvidersMu sync.RWMutex
	documentBaseProviders   = map[*jsc.Interpreter]func() string{}
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
