// 图片资源的宿主接线（webViewImageLoader）：把 rendering 层的图片 URL 解析
// 与取字节接回 WebView —— 宿主 ResourceResolver 优先、运行模式门禁
// （ModeToolkit 拒绝 http(s)/file 引用）、相对引用按文档 URL 解析。
//
// 为什么接线在 webkit 层：只有 WebView 知道「当前文档 URL」与「运行模式」。
// 渲染层此前自己 httpGet 并直接读工作目录，代价是 UI 库模式下会真的联网、
// 真实页面里的相对图片必然加载失败（见 rendering/image_resource.go 包注释）。
//
// 接线链：Frame.ImageLoader（本文件的 loader）→ RenderTreeBuilder →
// RenderView → Paint 入口 → loadBackgroundImage。

package webkit

import (
	"errors"
	"strings"

	"wb-ui/dom"
	"wb-ui/rendering"
)

// webViewImageLoader 实现 rendering.ImageResourceLoader。
type webViewImageLoader struct {
	wv *WebView
	// docURL 非空时作为解析基准（iframe 子文档：基准是子文档 URL）；
	// 为空时用 WebView 当前文档 URL（主框架，跟随 LoadURL 与重定向）。
	docURL string
}

// 编译期断言：渲染层的图片资源接线契约。
var _ rendering.ImageResourceLoader = (*webViewImageLoader)(nil)

// baseURL 返回解析基准（可能为空：LoadHTML 直出内容没有来源 URL）。
func (l *webViewImageLoader) baseURL() string {
	if l == nil {
		return ""
	}
	if l.docURL != "" {
		return l.docURL
	}
	if l.wv == nil {
		return ""
	}
	return l.wv.documentURL()
}

// ResolveURL 按文档基准解析图片引用。返回空串表示「无需解析 / 无法解析」，
// 渲染层保留原引用继续处理：LoadHTML 直出内容（无文档 URL）时仍按宿主工作
// 目录读取本地文件——与历史行为一致；真实页面（LoadURL）下的相对引用
// （`<img src="logo.png">`）则被解析为文档同级的绝对 URL。
func (l *webViewImageLoader) ResolveURL(ref string) string {
	if ref == "" {
		return ""
	}
	// data: URL 自带内容（渲染层同步解码），不该被解析成外部引用。
	if strings.HasPrefix(ref, "data:") {
		return ""
	}
	base := l.baseURL()
	if base == "" {
		return ""
	}
	abs := dom.ResolveURL(base, ref)
	if abs == ref {
		return ""
	}
	return abs
}

// AllowsExternal 与外部资源通道同一条门禁：UI 库模式不允许网络/文件系统
// 图片引用（http(s)/file/相对路径），浏览器模式允许。渲染层据此在**缓存
// 查询之前**拒绝——否则进程级全局图片缓存会让别的 WebView 已取回的同名
// URL 穿透模式门禁。
func (l *webViewImageLoader) AllowsExternal() bool {
	if l == nil || l.wv == nil {
		return false
	}
	return l.wv.mode.allowsExternalURLs()
}

// Load 取图片字节：统一走 loadExternalResource（宿主 ResourceResolver →
// data: → 仅浏览器模式允许 http(s)/file）。UI 库模式下网络/文件引用返回
// ErrExternalResourceBlocked —— 渲染层因此不再有绕开模式门禁联网的路径。
func (l *webViewImageLoader) Load(absURL string) ([]byte, error) {
	if l == nil || l.wv == nil {
		return nil, errors.New("webkit: image loader detached from webview")
	}
	content, err := l.wv.loadExternalResource(absURL)
	if err != nil {
		return nil, err
	}
	return []byte(content), nil
}
