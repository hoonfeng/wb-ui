// 导出测试辅助方法（供外部 probe 验证 select popup 交互）。
package app

import (
	"wb-ui/dom"
	"wb-ui/rendering"
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
