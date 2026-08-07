// Package rendering — iframe 子文档渲染支持。
//
// <iframe> 在 WebKit 中是 RenderIFrame，内部嵌入一个独立的子 Frame。
// 本端口把 iframe 元素与其子 Frame 的映射放在 page 包（page/iframe.go），
// 渲染侧通过 IFrameLookup 回调取回子文档渲染视图（避免 rendering→page
// 的包循环依赖：page 已依赖 rendering）。IFrameLookup 由宿主
// （webkit.WebView）在初始化时注入。
package rendering

import "wb-ui/dom"

// IFrameSubdocument 是 iframe 子文档渲染视图的最小接口，由 page.Frame
// 实现（RenderView 已有；LayoutNow/ViewportWidth/ViewportHeight 由
// Frame 包装其 FrameView 提供）。
type IFrameSubdocument interface {
	RenderView() *RenderView
	NeedsLayout() bool
	LayoutNow()
	ViewportWidth() int
	ViewportHeight() int
}

// IFrameLookup 返回 iframe 元素对应的子文档渲染视图；无子文档时返回
// nil。默认 nil（iframe 无子文档能力），由 webkit.NewWebView 注入。
var IFrameLookup func(el *dom.Element) IFrameSubdocument

// IFrameLookupFor 是 paint/hit-test 侧的取用入口：未注入时安全返回 nil。
func IFrameLookupFor(el *dom.Element) IFrameSubdocument {
	if IFrameLookup == nil || el == nil {
		return nil
	}
	return IFrameLookup(el)
}
