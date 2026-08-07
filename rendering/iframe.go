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
	// Document 返回子文档的 DOM Document。hit-test 下钻命中子文档元素
	// 后，滚动容器查找经 Document 匹配到所属子 Frame（IFrameContaining）。
	Document() *dom.Document
}

// IFrameLookup 返回 iframe 元素对应的子文档渲染视图；无子文档时返回
// nil。默认 nil（iframe 无子文档能力），由 webkit.NewWebView 注入。
var IFrameLookup func(el *dom.Element) IFrameSubdocument

// IFrameContaining 反查：给定子文档 Document，返回承载它的 iframe 子
// Frame 视图（滚动/hit-test 需要从子文档元素回到子 Frame）。由
// webkit.NewWebView 注入，遍历 iframe 注册表匹配 OwnerDocument。
var IFrameContaining func(doc *dom.Document) IFrameSubdocument

// IFrameLookupFor 是 paint/hit-test 侧的取用入口：未注入时安全返回 nil。
func IFrameLookupFor(el *dom.Element) IFrameSubdocument {
	if IFrameLookup == nil || el == nil {
		return nil
	}
	return IFrameLookup(el)
}

// IFrameContainingFor 是滚动容器查找侧的取用入口：未注入时安全返回 nil。
func IFrameContainingFor(doc *dom.Document) IFrameSubdocument {
	if IFrameContaining == nil || doc == nil {
		return nil
	}
	return IFrameContaining(doc)
}

// SetCursorPosRecursive records the cursor position on v and propagates it
// into iframe sub-frames: when the cursor falls inside an iframe's content
// box, the coordinate is translated to the child document's local space and
// set on the sub-frame's RenderView (recursively for nested iframes).
//
// Sub-document scrollbar hover highlight is driven by the sub-rv's cursor
// (renderpipeline.go isHover reads info.rv.CursorPos()); previously only the
// main RenderView got SetCursorPos, so a child-frame scrollbar never
// highlighted on hover. Cursors outside a sub-frame are cleared (large
// negative) so a hover highlight doesn't linger after the mouse leaves.
func SetCursorPosRecursive(v *RenderView, x, y float64) {
	if v == nil {
		return
	}
	v.SetCursorPos(x, y)
	propagateCursorToFrames(v, x, y)
}

func propagateCursorToFrames(v *RenderView, x, y float64) {
	if v == nil {
		return
	}
	var walk func(o RenderObject)
	walk = func(o RenderObject) {
		if o == nil {
			return
		}
		if el, isEl := o.Node().(*dom.Element); isEl && el.LocalName() == "iframe" {
			sub := IFrameLookupFor(el)
			if sub != nil && sub.RenderView() != nil {
				if rb, ok := o.(*RenderBox); ok {
					// Content-box origin: border-box + padding (matches
					// PaintIFrame's coordinate math).
					st := rb.Style()
					var pL, pT, pR, pB float64
					if st != nil {
						pL = lengthValue(st.PaddingLeft)
						pT = lengthValue(st.PaddingTop)
						pR = lengthValue(st.PaddingRight)
						pB = lengthValue(st.PaddingBottom)
					}
					ax := rb.AbsoluteX() + pL
					ay := rb.AbsoluteY() + pT
					cw := rb.Width() - pL - pR
					ch := rb.Height() - pT - pB
					lx, ly := x-ax, y-ay
					if lx >= 0 && ly >= 0 && lx <= cw && ly <= ch {
						sub.RenderView().SetCursorPos(lx, ly)
						propagateCursorToFrames(sub.RenderView(), lx, ly)
					} else {
						// Cursor outside: clear sub-frame cursor (incl.
						// nested children) so no stale hover remains.
						sub.RenderView().SetCursorPos(-1e9, -1e9)
						propagateCursorToFrames(sub.RenderView(), -1e9, -1e9)
					}
				}
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(RenderObject(v))
}
