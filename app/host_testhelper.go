// 导出测试辅助方法（供外部 probe 验证 select popup 交互）。
package app

import (
	"fmt"
	"log"
	"os"

	"github.com/go-gl/glfw/v3.3/glfw"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/ime"
	"wb-ui/engine/platform/window"
	"wb-ui/engine/rendering"
	"wb-ui/webkit"
)

// NewHostForTest 创建无窗口 Host（不启动 Run，供 probe 直接调用
// handleSelectClick / closeSelectPopup 等逻辑）。
func NewHostForTest(wv *webkit.WebView, width, height int) *Host {
	return &Host{
		wv:  wv,
		win: nil,
	}
}

// SelectPopupOpen 报告下拉浮层当前是否打开。
func (h *Host) SelectPopupOpen() bool { return h.selectPopup != nil }

// SelectPopupInfo 返回浮层调试信息（打开时）。
func (h *Host) SelectPopupInfo() string {
	if h.selectPopup == nil {
		return "nil"
	}
	return "layer=" + h.selectPopup.ClassName() + " for=" + h.selectPopupSelect.LocalName()
}

// MockSelectClick 模拟点击 select 元素（打开浮层）。
func (h *Host) MockSelectClick(sel *dom.Element, rv *rendering.RenderView) {
	if rv == nil {
		return
	}
	h.handleSelectClick(sel, rv, 0, 0)
}

// MockSelectOption 模拟点击浮层内指定 data-value 的 option。
func (h *Host) MockSelectOption(value string) {
	if h.selectPopup == nil {
		return
	}
	for c := h.selectPopup.FirstChild(); c != nil; c = c.NextSibling() {
		el, ok := c.(*dom.Element)
		if !ok {
			continue
		}
		if el.GetAttribute("data-value") == value {
			h.selectPopupOptionClicked(el)
			h.closeSelectPopup()
			return
		}
	}
}

// MockSelectOptionAt 模拟点击指定 option 元素（供 HitTest 命中后验证）。
func (h *Host) MockSelectOptionAt(el *dom.Element) {
	h.selectPopupOptionClicked(el)
	h.closeSelectPopup()
}

// MockSelectClose 模拟点击浮层外关闭。
func (h *Host) MockSelectClose() {
	h.closeSelectPopup()
}

// MockRangePress 模拟在 cssX 处按下 <input type="range">（真实交互路径）：
// 设置 :active 状态 → 立即吸附 value（step 取整）→ 派发 input →
// 进入拖动跟踪。返回新的 value。
func (h *Host) MockRangePress(el *dom.Element, rv *rendering.RenderView, cssX float64) string {
	if el == nil || rv == nil {
		return ""
	}
	if h.activeEl != nil {
		h.activeEl.SetActive(false)
	}
	el.SetActive(true)
	h.activeEl = el
	if h.setRangeValueFromX(el, rv, cssX) {
		el.DispatchEvent(dom.NewEvent("input", true, false, false))
	}
	h.rangeDragEl = el
	h.rangeDragRV = rv
	return el.GetAttribute("value")
}

// MockRangeMove 模拟拖动中的鼠标移动（range thumb 跟随），返回当前 value。
func (h *Host) MockRangeMove(cssX float64) string {
	if h.rangeDragEl == nil {
		return ""
	}
	if h.setRangeValueFromX(h.rangeDragEl, h.rangeDragRV, cssX) {
		h.rangeDragEl.DispatchEvent(dom.NewEvent("input", true, false, false))
	}
	return h.rangeDragEl.GetAttribute("value")
}

// MockEventCursorMove 走真实 EventCursorMove 处理路径：无条件
// MarkAllDirty（滚动条 hover 高亮依赖 cursor 位置）+ 活跃 range 拖拽
// 分支（setRangeValueFromX → input 事件 → MarkRenderTreeDirty）。
// 用于 probe 验证「拖拽中是否每帧重绘」——与 MockRangeMove 的区别是
// MockRangeMove 只改值不标记重绘，而真实桌面每帧 move 都会 MarkAllDirty。
func (h *Host) MockEventCursorMove(wv *webkit.WebView, cssX, cssY float64) {
	if wv == nil {
		return
	}
	rv := wv.RenderView()
	if rv == nil {
		return
	}
	// 与真实 EventCursorMove 分支一致：光标位置（scrollbar hover 判定）
	// + 无条件 MarkAllDirty。
	rendering.SetCursorPosRecursive(rv, cssX, cssY)
	rv.MarkAllDirty()
	if h.rangeDragEl != nil {
		// 与真实 EventCursorMove 分支一致：渲染树重建后旧
		// rangeDragRV 过期（box 几何错配 → value 抖动），按 DOM
		// 节点解析当前实例。
		drv := h.resolveDragRV(h.rangeDragEl, h.rangeDragRV)
		h.rangeDragRV = drv
		if h.setRangeValueFromX(h.rangeDragEl, drv, cssX) {
			h.rangeDragEl.DispatchEvent(dom.NewEvent("input", true, false, false))
			if mf := wv.MainFrame(); mf != nil {
				if fr := mf.Frame(); fr != nil {
					if !fr.RebuildStyleForElement(h.rangeDragEl) {
						fr.MarkRenderTreeDirty()
					}
				}
			}
		}
	}
	// 与真实 EventCursorMove 分支一致：textarea CSS resize 拖拽
	// （渲染树重建后旧 resizeDragRV 过期 → 按 DOM 节点解析当前实例）。
	if h.resizeDragEl != nil {
		drv := h.resolveDragRV(h.resizeDragEl, h.resizeDragRV)
		h.resizeDragRV = drv
		rb := drv.FindRenderBoxForNode(h.resizeDragEl)
		if os.Getenv("WB_RESIZE_DEBUG") != "" {
			log.Printf("[resize-move] el=%s drv=%p rb=%v cssY=%.1f startY=%.1f startH=%.1f",
				h.resizeDragEl.LocalName()+"."+h.resizeDragEl.GetAttribute("class"),
				drv, rb != nil, cssY, h.resizeDragStartY, h.resizeDragStartH)
		}
		if rb == nil {
			h.resizeDragEl = nil
		} else {
			newH := h.resizeDragStartH + (cssY - h.resizeDragStartY)
			// ★ 与浏览器一致：style 写入 raw 高度（min/max-height 由 CSS
			// 布局层 clamp），仅保留 10px 下限。
			if newH < 10 {
				newH = 10
			}
			h.resizeDragEl.SetAttribute("style", fmt.Sprintf("height:%.0fpx", newH))
			if mf := wv.MainFrame(); mf != nil {
				if fr := mf.Frame(); fr != nil {
					if !fr.RebuildStyleForElement(h.resizeDragEl) {
						fr.MarkRenderTreeDirty()
					}
				}
			}
		}
	}
}

// MockRangeRelease 模拟释放鼠标结束拖动：派发 change + 清除 :active/拖动态。
// 返回最终 value。
func (h *Host) MockRangeRelease() string {
	if h.rangeDragEl == nil {
		return ""
	}
	el := h.rangeDragEl
	v := el.GetAttribute("value")
	el.DispatchEvent(dom.NewEvent("change", true, false, false))
	el.SetActive(false)
	if h.activeEl == el {
		h.activeEl = nil
	}
	h.rangeDragEl = nil
	h.rangeDragRV = nil
	return v
}

// MockContextMenu 模拟在 (cssX, cssY) 右键释放：走与真实 Release 分支
// 一致的 handleContextMenu（HitTest → 派发 contextmenu → 微任务 → 重建）。
// win 为 nil（NewHostForTest）时按 scale=1 处理。返回命中元素。
func (h *Host) MockContextMenu(wv *webkit.WebView, cssX, cssY float64) *dom.Element {
	if wv == nil {
		return nil
	}
	rv := wv.RenderView()
	if rv == nil {
		return nil
	}
	h.handleContextMenu(rv, window.Event{
		Type:   window.EventMouseButton,
		X:      cssX,
		Y:      cssY,
		Button: int(glfw.MouseButton2),
		Action: int(glfw.Release),
	})
	el := rendering.HitTest(rv, cssX, cssY, "")
	return el
}

// MockTextareaResizePress 模拟在 textarea 右下角手柄（视口坐标 bx+by+bw+bh
// 附近 15px 区域）按下：与真实 EventCursorPress 的 resize 分支一致——
// 记录 resizeDragEl/RV/StartY/StartH，开始高度拖拽。
func (h *Host) MockTextareaResizePress(el *dom.Element, rv *rendering.RenderView, cssY float64) {
	if el == nil || rv == nil {
		return
	}
	h.resizeDragEl = el
	h.resizeDragRV = rv
	h.resizeDragStartY = cssY
	if rb := rv.FindRenderBoxForNode(el); rb != nil {
		h.resizeDragStartH = rb.Height()
	}
}

// MockTextareaResizeRelease 模拟释放鼠标结束 resize 拖拽：清空拖动态。
func (h *Host) MockTextareaResizeRelease() {
	h.resizeDragEl = nil
	h.resizeDragRV = nil
}

// MockFocus 在 headless（win=nil）下模拟点击聚焦：设置 imeFocusedEl +
// focused 标志。真实 desktop 用 FocusElement（win 非 nil）；probe 用
// MockFocus 跳过 GLFW 窗口依赖。保持与 FocusElement 相同的 imeFocusedEl
// 语义（EventChar 判定 contenteditable 的依据）。
func (h *Host) MockFocus(el *dom.Element) {
	if h.imeFocusedEl != nil && h.imeFocusedEl != el {
		h.imeFocusedEl.SetFocused(false)
	}
	h.imeFocusedEl = el
	h.imeComposing = false
	h.imeComposeText = ""
	h.imeCompRoot = nil
	h.imeCompFrom = 0
	h.imeCompLen = 0
	h.imeCompText = ""
	h.imeCompStarted = false
	h.imeCompEndFired = false
	if el != nil {
		el.SetFocused(true)
		el.SetFocusByKeyboard(false)
	}
}

// MockKeyChar 走真实 EventChar 处理路径（handleCharInput，与 processEvents
// 的 case window.EventChar 共享同一实现）：在 imeFocusedEl 处插入字符并派发
// input 事件。用于验证 contenteditable（CM6）真实键盘链路——
// FocusElement 设置 imeFocusedEl + collapse 填充 sstate.ranges 后，按键字符
// 应插入 DOM 并触发 CM6 readDOMChange 同步 state。
func (h *Host) MockKeyChar(char rune) {
	if h.wv == nil {
		return
	}
	h.handleCharInput(window.Event{
		Type: window.EventChar,
		Char: char,
	})
}

// ApplyIMEEventsForTest 走真实 IME 事件处理路径（applyIMEEvents，与
// processEvents 的 PollIMEEvents → applyIMEEvents 同一实现）：模拟
// Windows IME 组合（compositionupdate）+ 提交（char）序列。用于验证
// contenteditable（CM6）的 IME 输入链路与文本插入位置。
func (h *Host) ApplyIMEEventsForTest(events []ime.Event) {
	h.applyIMEEvents(events)
}

// MockMouseMove 模拟鼠标移动到 (cssX, cssY)，走真实 hover 路径：
// HitTest → SetHovered(旧元素清除/新元素设置) → hoverStyleFastPath
// （只重算受影响元素的 :hover 样式，不重建渲染树）。返回新 hover 元素。
func (h *Host) MockMouseMove(wv *webkit.WebView, cssX, cssY float64) *dom.Element {
	if wv == nil {
		return nil
	}
	rv := wv.RenderView()
	if rv == nil {
		return nil
	}
	newEl := rendering.HitTest(rv, cssX, cssY, "")
	// 同步 RenderView 光标位置（真实 EventCursorMove 路径会调用
	// SetCursorPosRecursive；paintRangeSlider 用 CursorPos 区分
	// 「悬停到圆」与「悬停到条」，Mock 必须一致）。
	rendering.SetCursorPosRecursive(rv, cssX, cssY)
	oldHover := h.hoveredEl
	if oldHover != nil {
		oldHover.SetHovered(false)
	}
	if newEl != nil {
		newEl.SetHovered(true)
	}
	h.hoveredEl = newEl
	if mf := wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			h.hoverStyleFastPath(rv, fr, oldHover, newEl)
		}
	}
	return newEl
}
