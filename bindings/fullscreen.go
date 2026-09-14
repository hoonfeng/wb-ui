// Package bindings — 全屏 API（HTML §4.11.6）。
//
// 状态模型：document.fullscreenElement 由 Element.requestFullscreen() 与
// document.exitFullscreen() 维护，CSS 的 :fullscreen 伪类与 fullscreenchange
// 事件消费它。本引擎**不**接管窗口/顶层的真实全屏渲染（全屏元素不会自动铺满
// 视口）：宿主可以用 OnFullscreenChanged 把真实窗口切到全屏，或由页面自己写
// `:fullscreen { position: fixed; inset: 0 }` 达到同样的效果。
//
// 事件语义（规范）：fullscreenchange 异步派发，且**先派发到元素、再派发到
// document**；请求失败派发 fullscreenerror。
package bindings

import (
	"wb-ui/dom"
	"wb-ui/jsc"
)

// FullscreenElement 返回该文档当前的全屏元素（无则 nil）。
func FullscreenElement(doc *dom.Document) *dom.Element {
	if doc == nil {
		return nil
	}
	return doc.FullscreenElement()
}

// setFullscreenElement 更新文档的全屏状态并通知宿主/样式链，返回状态是否发生
// 变化（规范：全屏元素没变时「nothing has changed」，不派发事件）。
func setFullscreenElement(doc *dom.Document, el *dom.Element) bool {
	if doc == nil {
		return false
	}
	prev := doc.FullscreenElement()
	if prev == el {
		return false
	}
	doc.SetFullscreenElement(el)
	// :fullscreen 的匹配结果变了 → 失效 computed style 缓存；复用
	// OnClassChanged 的「渲染树重建」链路（与 class 变化同类）。
	if el != nil {
		InvalidateComputedStyle(el)
	}
	if prev != nil {
		InvalidateComputedStyle(prev)
	}
	if OnClassChanged != nil {
		if el != nil {
			OnClassChanged(el)
		} else {
			OnClassChanged(prev)
		}
	}
	if OnFullscreenChanged != nil {
		if el != nil {
			OnFullscreenChanged(el)
		} else {
			OnFullscreenChanged(prev)
		}
	}
	return true
}

// dispatchFullscreenEvent 派发全屏事件：先元素、后 document（规范顺序），
// 经宏任务异步派发（与其它媒体/资源事件一致）。
func dispatchFullscreenEvent(in *jsc.Interpreter, el *dom.Element, doc *dom.Document, evType string) {
	fire := func() {
		if el != nil {
			el.DispatchEvent(dom.NewEvent(evType, false, false, false))
		}
		if doc != nil {
			doc.DispatchEvent(dom.NewEvent(evType, false, false, false))
		}
	}
	mediaRunLater(in, fire)
}

// requestFullscreenFor 实现 Element.requestFullscreen()：设置文档的全屏元素、
// 异步派发 fullscreenchange，返回 Promise。
func requestFullscreenFor(in *jsc.Interpreter, el *dom.Element) jsc.JSValue {
	if el == nil || in == nil {
		return jsc.Undefined()
	}
	doc := el.OwnerDocument()
	// Fullscreen spec §3.1：requestFullscreen 先做「fullscreen element ready
	// check」（元素必须 connected），失败派发 fullscreenerror 并 reject
	// TypeError。本引擎没有 user activation / 顶层文档授权模型，因此只剩
	// 「已连接」这一条可判定条件。
	if doc == nil || !el.IsConnected() {
		dispatchFullscreenEvent(in, el, doc, "fullscreenerror")
		return in.RejectPromise(newDOMExceptionValue(in, "TypeError",
			"The element is not connected to a document."))
	}
	if !setFullscreenElement(doc, el) {
		// 已经是全屏元素：状态没变，规范要求不派发 fullscreenchange。
		return in.ResolvePromise(jsc.Undefined())
	}
	dispatchFullscreenEvent(in, el, doc, "fullscreenchange")
	return in.ResolvePromise(jsc.Undefined())
}

// exitFullscreenFor 实现 document.exitFullscreen()：清除全屏元素并派发事件。
// 没有全屏元素时按规范 reject TypeError。
func exitFullscreenFor(in *jsc.Interpreter, doc *dom.Document) jsc.JSValue {
	if in == nil {
		return jsc.Undefined()
	}
	if doc == nil || doc.FullscreenElement() == nil {
		return in.RejectPromise(newDOMExceptionValue(in, "TypeError",
			"Document not active."))
	}
	prev := doc.FullscreenElement()
	setFullscreenElement(doc, nil)
	dispatchFullscreenEvent(in, prev, doc, "fullscreenchange")
	return in.ResolvePromise(jsc.Undefined())
}
