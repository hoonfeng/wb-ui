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
