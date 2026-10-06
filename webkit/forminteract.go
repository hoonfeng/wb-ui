// 表单控件交互的**唯一实现**：复选框/单选框切换、<label> 点击转发、
// <select> 下拉弹层的构造与命中。
//
// 背景：app.Host（glfw 宿主）与 webkit.Interaction（裸 WebView 的标准交互
// 服务）各自维护过一整套同名实现，两边行为已经漂移：
//
//	① app 侧弹层**不注入兜底样式**——页面没有 .select-popup-option 规则族
//	   时，桌面端下拉裸装（无行高/内边距/hover/选中色）；Interaction 侧
//	   会注入 #wb-ui-select-popup-style；
//	② app 侧内联色 var(--bg-secondary) / var(--border-color) **无兜底值**，
//	   页面未定义这两个令牌时弹层无底色、无边框；
//	③ option 命中判据不同：app 用 data-value != ""（value="" 的合法空值
//	   选项点不动），Interaction 用 data-select-popup="1"；
//	④ option 取值回退语义不同：app 用 optVal == ""（把 value="" 当成
//	   「没有值」），Interaction 用 !HasAttribute("value")——后者才符合
//	   HTML 规范（option.value 仅在**无** value 属性时回退 textContent）；
//	⑤ 弹层定位不同：app 用「AbsoluteX 减自身滚动偏移」（近似，嵌套滚动
//	   容器下错位），Interaction 用 rendering.BoxViewportRect（扣掉所有
//	   祖先滚动容器偏移，与命中测试同一坐标空间）；
//	⑥ <label> 点击转发切 radio 时不同组互斥、也不派发 change（app 侧
//	   handleLabelToggle），与 checkbox/radio 的按下切换语义不一致。
//
// 本文件把语义收敛为一份：两个宿主的同名方法都只剩薄壳转发，宿主差异
// （app 额外 RebuildRenderTree / 调试日志、Interaction 的 dirty 标记）
// 留在各自薄壳里。
package webkit

import (
	"fmt"
	"strings"

	"wb-ui/engine/dom"
	"wb-ui/engine/html5"
	"wb-ui/engine/js/bindings"
	"wb-ui/engine/rendering"
)

// ── 复选框 / 单选框 ──

// ToggleCheckboxRadio 切换 el 的 checkbox/radio 选中态，并派发 change
// （Vue v-model 等靠 change 同步）。radio 按 name 同组互斥。
// 返回 true 表示 el 确实是可切换的 checkbox/radio 且已处理。
//
// 语义与浏览器一致：点击即用户交互 → MarkUserInteracted（:user-valid /
// :user-invalid 依赖它）。
func ToggleCheckboxRadio(el *dom.Element) bool {
	if el == nil || el.LocalName() != "input" {
		return false
	}
	typ := el.GetAttribute("type")
	if typ != "checkbox" && typ != "radio" {
		return false
	}
	in, ok := html5.ToInputElement(el)
	if !ok {
		return false
	}
	if typ == "checkbox" {
		in.SetChecked(!in.Checked())
	} else {
		// radio：同组互斥。用**元素所属文档**——iframe 内的 radio 在子文档
		// 里，只扫主文档会漏掉同组其它项（此前 app 与 Interaction 都扫主
		// 文档）。
		name := el.GetAttribute("name")
		if name != "" {
			if doc := el.OwnerDocument(); doc != nil {
				for _, r := range doc.GetElementsByTagName("input") {
					if r == el {
						continue
					}
					if r.GetAttribute("type") == "radio" && r.GetAttribute("name") == name {
						if r2, ok2 := html5.ToInputElement(r); ok2 {
							r2.SetChecked(false)
						}
					}
				}
			}
		}
		in.SetChecked(true)
	}
	bindings.MarkUserInteracted(el)
	el.DispatchEvent(dom.NewEvent("change", true, false, false))
	return true
}

// ToggleLabeledControl 实现 <label> 的点击转发：点击 label 或其任意后代，
// 切换 label 内第一个 checkbox/radio（浏览器 label 语义）。开关（switch）
// 的 track span 点击因此能 toggle 内嵌 checkbox，`input:checked + .track::after`
// 滑块随之移动，Vue v-model 也随 change 同步。
//
// 转发走 ToggleCheckboxRadio 同一条路径（radio 同组互斥 + change 派发）：
// 此前 label 转发只翻 checked 位，与按下切换是两套语义（分叉点⑥）。
func ToggleLabeledControl(el *dom.Element) bool {
	if el == nil {
		return false
	}
	lab := el
	for lab != nil && lab.LocalName() != "label" {
		lab = lab.ParentElement()
	}
	if lab == nil {
		return false
	}
	for c := lab.FirstChild(); c != nil; c = c.NextSibling() {
		e, ok := c.(*dom.Element)
		if !ok || e.LocalName() != "input" {
			continue
		}
		typ := e.GetAttribute("type")
		if typ != "checkbox" && typ != "radio" {
			continue
		}
		return ToggleCheckboxRadio(e)
	}
	return false
}

// ── <select> 下拉弹层 ──

// SelectClickTarget 返回 activeEl 自身或其祖先里最近的 <select>；
// 不是 select 相关点击时返回 nil。
func SelectClickTarget(activeEl *dom.Element) *dom.Element {
	if activeEl == nil {
		return nil
	}
	if activeEl.LocalName() == "select" {
		return activeEl
	}
	for p := activeEl.ParentElement(); p != nil; p = p.ParentElement() {
		if p.LocalName() == "select" {
			return p
		}
	}
	return nil
}

// EnsureSelectPopupStyle 注入弹层**兜底**样式（幂等：页面自带
// .select-popup-option 规则族时以页面为准）。
//
// 两个要点，缺一个就会污染页面：
//
//	① 值与页面规则族一致（14px / 24px 行高 / padding 0 8px / 主题令牌），
//	   不写死 #3b6fd4、#2a3a5f 这类脱离主题的色值；
//	② 插入 <head> **开头**：同特异性下按源顺序后者优先，只有放在开头，
//	   页面自己的规则（更晚解析）才能覆盖兜底。此前 append 到 head 末尾，
//	   反而抢在页面规则之后生效，把主题化的选中态压成亮蓝 #3b6fd4。
func EnsureSelectPopupStyle(doc *dom.Document) {
	if doc == nil {
		return
	}
	if doc.GetElementById("wb-ui-select-popup-style") != nil {
		return
	}
	style := doc.CreateElement("style")
	style.SetAttribute("id", "wb-ui-select-popup-style")
	css := "\n.select-popup-option{display:block;height:24px;line-height:24px;padding:0 8px;font-size:14px;color:var(--text-primary,#E7ECF5);cursor:pointer;overflow:hidden;white-space:nowrap;text-overflow:ellipsis}\n" +
		".select-popup-option:hover{background:var(--bg-hover,rgba(255,255,255,0.06))}\n" +
		".select-popup-option-selected{background:var(--accent-bg,rgba(111,168,255,0.10))}\n" +
		".select-popup-option-selected:hover{background:var(--bg-active,rgba(255,255,255,0.10))}\n" +
		".select-popup-option-disabled{color:var(--text-muted,#8593AA);opacity:.45;cursor:not-allowed}\n" +
		".select-popup-option-disabled:hover{background:transparent}\n"
	_ = style.AppendChild(doc.CreateTextNode(css))
	if head := doc.Head(); head != nil {
		if first := head.FirstChild(); first != nil {
			_ = head.InsertBefore(style, first)
		} else {
			_ = head.AppendChild(style)
		}
	} else if body := doc.Body(); body != nil {
		_ = body.AppendChild(style)
	}
}

// BuildSelectPopup 为 sel 构造并挂载下拉弹层，返回 overlay 元素；
// 返回 nil 表示未打开（不是 <select>、已禁用、缺文档/body）。
//
// 定位：fixed 浮层用**视口坐标**，取自 rendering.BoxViewportRect（扣掉所有
// 祖先滚动容器的偏移，与命中测试同一坐标空间）。默认贴在 select 下沿，
// 底部空间不足时向上展开（浮层顶边不低于 0）。行高固定 24px，超 8 项滚动。
//
// 样式：容器只给定位 + 主题令牌（var 带兜底值，令牌缺失时仍有可读底色），
// 选项全部 class 驱动——内联 style 优先级高于外部规则，给每个选项打内联色
// 会连 :hover / 选中态一起压掉。兜底样式表由 EnsureSelectPopupStyle 注入。
func BuildSelectPopup(wv *WebView, sel *dom.Element, rv *rendering.RenderView) *dom.Element {
	if wv == nil || sel == nil || rv == nil {
		return nil
	}
	selEl, ok := html5.ToSelectElement(sel)
	if !ok || selEl.Disabled() {
		return nil
	}
	frame := wv.MainFrame()
	if frame == nil {
		return nil
	}
	doc := frame.Document()
	if doc == nil {
		return nil
	}
	body := doc.Body()
	if body == nil {
		return nil
	}
	var sx, sy, boxW, boxH float64
	boxW = 180
	if box := rv.FindRenderBoxForNode(sel); box != nil {
		vx, vy, vw, vh := rendering.BoxViewportRect(rv, box)
		sx, sy = vx, vy
		boxH = vh
		if vw > 0 {
			boxW = vw
		}
	}
	opts := selEl.Options()
	const rowH = 24
	n := len(opts)
	if n > 8 {
		n = 8
	}
	popH := n*rowH + 4
	popTop := sy + boxH
	if popTop+float64(popH) > float64(wv.Height())-8 {
		popTop = sy - float64(popH)
		if popTop < 0 {
			popTop = 0
		}
	}
	overlay := doc.CreateElement("div")
	overlay.SetAttribute("class", "select-popup")
	overlay.SetAttribute("style", fmt.Sprintf(
		"position:fixed;left:%.0fpx;top:%.0fpx;width:%.0fpx;height:%dpx;"+
			"background:var(--bg-secondary,#151B29);border:1px solid var(--border-color,#2A3550);"+
			"border-radius:4px;box-shadow:0 4px 12px rgba(0,0,0,0.35);z-index:9999;overflow-y:auto;",
		sx, popTop, boxW, popH))
	current := selEl.Value()
	for _, opt := range opts {
		optEl := doc.CreateElement("div")
		// option.value 的 IDL 语义：仅在**无** value 属性时回退 textContent
		// （value="" 是合法空值，不能当成「没有值」）。
		optVal := opt.GetAttribute("value")
		if !opt.HasAttribute("value") {
			optVal = opt.TextContent()
		}
		cls := "select-popup-option"
		if opt.HasAttribute("disabled") {
			cls += " select-popup-option-disabled"
		} else if optVal == current {
			cls += " select-popup-option-selected"
		}
		optEl.SetAttribute("class", cls)
		optEl.SetAttribute("data-value", optVal)
		optEl.SetAttribute("data-select-popup", "1")
		_ = optEl.AppendChild(doc.CreateTextNode(opt.TextContent()))
		_ = overlay.AppendChild(optEl)
	}
	_ = body.AppendChild(overlay)
	EnsureSelectPopupStyle(doc)
	return overlay
}

// SelectPopupContains 报告 el 是否在弹层 overlay 内（含 overlay 自身）。
func SelectPopupContains(overlay, el *dom.Element) bool {
	if overlay == nil || el == nil {
		return false
	}
	for p := el; p != nil; p = p.ParentElement() {
		if p == overlay {
			return true
		}
	}
	return false
}

// RemoveSelectPopup 从文档里摘掉弹层并重建渲染树；overlay 为 nil 时空操作。
func RemoveSelectPopup(wv *WebView, overlay *dom.Element) {
	if wv == nil || overlay == nil {
		return
	}
	if frame := wv.MainFrame(); frame != nil {
		if doc := frame.Document(); doc != nil {
			if body := doc.Body(); body != nil {
				_ = body.RemoveChild(overlay)
			}
		}
	}
	wv.RebuildRenderTree()
}

// ApplySelectPopupOption 应用弹层选项点击：把 optionEl 的 data-value 写回
// 所属 <select> 并派发 change（Vue v-model / onchange 属性处理器因此同步）。
// 返回 true 表示确实应用了一次选择（禁用项与非法目标返回 false）。
func ApplySelectPopupOption(sel, optionEl *dom.Element) bool {
	if sel == nil || optionEl == nil {
		return false
	}
	if optionEl.LocalName() != "div" || optionEl.GetAttribute("data-select-popup") != "1" {
		return false
	}
	if strings.Contains(optionEl.GetAttribute("class"), "disabled") {
		return false
	}
	selEl, ok := html5.ToSelectElement(sel)
	if !ok {
		return false
	}
	selEl.SetValue(optionEl.GetAttribute("data-value"))
	// 用户做出了选择 → user validity 置位；change 冒泡给 Vue v-model。
	bindings.MarkUserInteracted(sel)
	sel.DispatchEvent(dom.NewEvent("change", true, false, false))
	return true
}
