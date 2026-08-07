// Package page — iframe 子文档支持。
//
// WebKit 中 <iframe> 由 RenderIFrame 承载一个独立的子 Frame（LocalFrame），
// 其 Document/RenderView/FrameView 完全独立，绘制时通过 translate+clip
// 嵌进父文档的 iframe 元素框内。本端口以「iframe 元素 → 子 Frame」的
// 注册表模拟之：布局/绘制/滚动互不干扰，paint 阶段（PaintIFrame）按
// iframe 元素的内容框取子 Frame 的渲染结果。
package page

import (
	"sync"

	"wb-ui/dom"
)

// iframeRegistry 把主文档中的 <iframe> 元素映射到其子 Frame（子文档）。
// 键用元素指针——不同 WebView 的元素天然隔离，多 WebView 场景安全。
var (
	iframeMu       sync.RWMutex
	iframeRegistry = map[*dom.Element]*Frame{}
)

// RegisterIFrame 注册 iframe 元素的子 Frame。
func RegisterIFrame(el *dom.Element, f *Frame) {
	if el == nil || f == nil {
		return
	}
	iframeMu.Lock()
	iframeRegistry[el] = f
	iframeMu.Unlock()
}

// IFrameFrame 返回 iframe 元素的子 Frame；未注册时返回 nil。
func IFrameFrame(el *dom.Element) *Frame {
	if el == nil {
		return nil
	}
	iframeMu.RLock()
	defer iframeMu.RUnlock()
	return iframeRegistry[el]
}

// IFrameFrameForFrame 反向查询：返回持有子 Frame f 的 iframe 元素；
// 该子 Frame 未注册时返回 nil。用于清理孤儿子 Frame（如 LoadHTML 后
// 注册表 Prune 掉旧文档的 iframe）。
func IFrameFrameForFrame(f *Frame) *dom.Element {
	if f == nil {
		return nil
	}
	iframeMu.RLock()
	defer iframeMu.RUnlock()
	for el, fr := range iframeRegistry {
		if fr == f {
			return el
		}
	}
	return nil
}

// UnregisterIFrame 注销 iframe 元素的子 Frame（元素从文档移除时调用）。
func UnregisterIFrame(el *dom.Element) {
	if el == nil {
		return
	}
	iframeMu.Lock()
	delete(iframeRegistry, el)
	iframeMu.Unlock()
}

// PruneIFrames 删除不再属于 doc 文档的 iframe 注册。LoadHTML 加载新主
// 文档后调用：旧文档的 iframe 元素已不在树中，其子 Frame 应被释放。
// （不采用全量 Clear：保留同文档内尚未重新扫描到的注册，避免重复加载。）
func PruneIFrames(doc *dom.Document) {
	if doc == nil {
		return
	}
	iframeMu.Lock()
	defer iframeMu.Unlock()
	for el := range iframeRegistry {
		if el.OwnerDocument() != doc {
			delete(iframeRegistry, el)
		}
	}
}

// ForEachIFrame 遍历全部已注册的 iframe 子 Frame（读锁下执行 fn）。
// 用于布局同步（syncIFrameSizes）等需要枚举的场景。
func ForEachIFrame(fn func(el *dom.Element, f *Frame)) {
	if fn == nil {
		return
	}
	iframeMu.RLock()
	defer iframeMu.RUnlock()
	for el, f := range iframeRegistry {
		fn(el, f)
	}
}

// IFrameCount 返回已注册 iframe 子 Frame 数量（诊断用）。
func IFrameCount() int {
	iframeMu.RLock()
	defer iframeMu.RUnlock()
	return len(iframeRegistry)
}
