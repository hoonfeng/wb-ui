package rendering

import (
	"strings"
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// TestButtonLabelSkipsDisplayNoneDescendants 回归：<button> 的 label 必须只取
// **可见**文本 —— display:none 子孙的文本不得进入 label（浏览器不画）。
//
// 背景（2026-09-26，agent-teams「团队」按钮，桌面端与浏览器不一致）：旧实现
// paintButtonText 用 el.TextContent() 拼 label，把隐藏徽标
// <span style="display:none">0</span> 的 "0" 拼成「团队0」画在顶栏；
// 浏览器只画「团队」。
func TestButtonLabelSkipsDisplayNoneDescendants(t *testing.T) {
	doc := dom.NewDocument()
	btn := doc.CreateElement("button")
	btn.AppendChild(doc.CreateTextNode("团队"))
	span := doc.CreateElement("span")
	span.SetAttribute("style", "display:none")
	span.AppendChild(doc.CreateTextNode("WWWWWWWW"))
	doc.AppendChild(btn)
	btn.AppendChild(span)

	st := style.NewComputedStyle()
	st.FontSize = style.Length{Value: 20, Unit: "px"}

	// 渲染树：display:none 的子孙没有渲染对象（由 TestBuildDisplayNone 保证），
	// 按钮的可见文本只有「团队」。
	box := NewRenderBlockFlow(btn, st)
	box.AddChild(NewRenderTextWith(nil, st, "团队"), nil)

	// 前置条件：DOM 全量文本确实含隐藏子孙（旧实现会把它画出来）。
	if got := strings.TrimSpace(btn.TextContent()); got != "团队WWWWWWWW" {
		t.Fatalf("前置条件: textContent = %q, want 团队WWWWWWWW", got)
	}
	if got := buttonLabelText(box, btn); got != "团队" {
		t.Fatalf("buttonLabelText = %q, want 团队（必须忽略 display:none 子孙文本）", got)
	}
}

// TestButtonLabelSkipsInvisibleText：visibility:hidden / collapse 的文本同样
// 不进 label —— PaintText 也不绘制这类文本，label 必须与之一致。
func TestButtonLabelSkipsInvisibleText(t *testing.T) {
	doc := dom.NewDocument()
	btn := doc.CreateElement("button")
	doc.AppendChild(btn)

	visible := style.NewComputedStyle()
	visible.Visibility = "visible"
	hidden := style.NewComputedStyle()
	hidden.Visibility = "hidden"

	box := NewRenderBlockFlow(btn, visible)
	box.AddChild(NewRenderTextWith(nil, visible, "保存"), nil)
	box.AddChild(NewRenderTextWith(nil, hidden, "草稿"), nil)

	if got := buttonLabelText(box, btn); got != "保存" {
		t.Fatalf("buttonLabelText = %q, want 保存", got)
	}
}

// TestButtonLabelFallbackWhenNoRenderSubtree：渲染子树完全没有建立时回退
// textContent 兜底，避免按钮彻底无字（布局尚未建立等退化场景）。
func TestButtonLabelFallbackWhenNoRenderSubtree(t *testing.T) {
	doc := dom.NewDocument()
	btn := doc.CreateElement("button")
	btn.AppendChild(doc.CreateTextNode("确定"))
	doc.AppendChild(btn)

	st := style.NewComputedStyle()
	box := NewRenderBlockFlow(btn, st) // 刻意不挂任何子对象
	if got := buttonLabelText(box, btn); got != "确定" {
		t.Fatalf("buttonLabelText = %q, want 确定（无渲染子树时兜底 textContent）", got)
	}
}

// TestButtonLabelEmptyWhenOnlyHiddenChildren：有渲染子树但其中没有可见文本
// （例如只有图标）时 label 为空 —— 不得回退 textContent 把隐藏文本捞回来。
func TestButtonLabelEmptyWhenOnlyHiddenChildren(t *testing.T) {
	doc := dom.NewDocument()
	btn := doc.CreateElement("button")
	hiddenSpan := doc.CreateElement("span")
	hiddenSpan.SetAttribute("style", "display:none")
	hiddenSpan.AppendChild(doc.CreateTextNode("0"))
	btn.AppendChild(hiddenSpan)
	doc.AppendChild(btn)

	st := style.NewComputedStyle()
	box := NewRenderBlockFlow(btn, st)
	// 按钮里只有一个 <svg> 图标（无文本对象）。
	box.AddChild(NewRenderBox(doc.CreateElement("svg"), st), nil)

	if got := buttonLabelText(box, btn); got != "" {
		t.Fatalf("buttonLabelText = %q, want 空（只有图标时不得画出隐藏子孙文本）", got)
	}
}

// TestPaintFormControlFlexButtonFallsThrough：flex/grid 容器的 <button> 不由
// PaintFormControl 的「居中 label」特判处理（其内容按 flex/grid 布局排列，
// 文本由子树 RenderText 在正确位置绘制），返回 false 让常规路径接管；
// 普通 button 仍由该特判居中绘制 label（返回 true）。
func TestPaintFormControlFlexButtonFallsThrough(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("button")
	el.AppendChild(doc.CreateTextNode("团队"))
	doc.AppendChild(el)

	canvas := graphics.NewCanvas(120, 40)
	defer canvas.Release()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 120, Height: 40})

	st := style.NewComputedStyle()
	st.Display = style.DisplayInlineFlex
	st.FontSize = style.Length{Value: 12, Unit: "px"}
	flexBox := NewRenderBox(el, st)
	flexBox.SetLocation(0, 0)
	flexBox.SetSize(120, 40)
	if PaintFormControl(flexBox, info) {
		t.Fatal("flex 容器 button 应由常规路径绘制（PaintFormControl 必须返回 false）")
	}

	plain := style.NewComputedStyle()
	plain.FontSize = style.Length{Value: 12, Unit: "px"}
	plainBox := NewRenderBox(el, plain)
	plainBox.SetLocation(0, 0)
	plainBox.SetSize(120, 40)
	if !PaintFormControl(plainBox, info) {
		t.Fatal("普通 button 应由 PaintFormControl 居中绘制 label（返回 true）")
	}
}
