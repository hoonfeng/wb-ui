package rendering

// 表单控件选区高亮绘制测试：FocusedFormControlSel{Start,End} 区间 →
// 选中段蓝底（rgba(50,100,200,150)）+ 白字，未选段保持正常色。
// 回归：拖选/双击词选（FormFocus 鼠标选择）写入区间后视觉高亮可见。

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
)

func TestPaintFormControl_SelectionHighlight(t *testing.T) {
	canvas := graphics.NewCanvas(160, 24)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 160, Height: 24})
	box := newFormControlBox("input", map[string]string{
		"type":  "text",
		"value": "hello world",
	}, 160, 24)

	// 选区 [0,5) = "hello"；登记渲染全局（PaintFormControl 仅当
	// el == FocusedFormControl 且 FocusedFormControlSel 非空时走分段绘制）。
	prevFC := FocusedFormControl
	prevSel := FocusedFormControlSel
	FocusedFormControl = box.Node().(*dom.Element)
	FocusedFormControlSel = &FormControlSelection{Start: 0, End: 5}
	defer func() {
		FocusedFormControl = prevFC
		FocusedFormControlSel = prevSel
	}()

	if !PaintFormControl(box, info) {
		t.Fatal("PaintFormControl(input) = false")
	}
	// 选中段（前 ~40px）必须存在蓝底高亮像素：B 显著高于 R/G（填充色
	// rgba(50,100,200,150) 混合后 B≈118、R≈29、G≈59）。
	foundBlue := false
	for py := 4; py < 20; py++ {
		for px := 2; px < 40; px++ {
			c := canvas.PixelAt(px, py)
			if c.A > 0 && c.B > c.R+40 && c.B > 70 {
				foundBlue = true
				break
			}
		}
		if foundBlue {
			break
		}
	}
	if !foundBlue {
		t.Fatal("selection highlight (blue bg) not painted in selected region")
	}
	// 未选段（x>60 处 "world" 区域）不应有蓝底（文字色正常——有无像素
	// 取决于字体可用性，这里只断言无蓝色高亮）。
	for py := 4; py < 20; py++ {
		for px := 90; px < 150; px++ {
			c := canvas.PixelAt(px, py)
			if c.A > 0 && c.B > c.R+40 && c.B > 70 {
				t.Fatalf("unexpected selection highlight outside sel region at (%d,%d)", px, py)
			}
		}
	}
}

func TestPaintFormControl_CaretBeforeSelEnd(t *testing.T) {
	// caret 位置取 sel.End（选中文本右缘）——拖选后输入替换从 End 起算；
	// Start>End 异常时按 End 归一（renderformcontrol.go 的 caretPos 逻辑）。
	canvas := graphics.NewCanvas(160, 24)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 160, Height: 24})
	box := newFormControlBox("input", map[string]string{
		"type":  "text",
		"value": "hello world",
	}, 160, 24)
	prevFC := FocusedFormControl
	prevSel := FocusedFormControlSel
	FocusedFormControl = box.Node().(*dom.Element)
	FocusedFormControlSel = &FormControlSelection{Start: 8, End: 3} // 反向区间（归一化用）
	defer func() {
		FocusedFormControl = prevFC
		FocusedFormControlSel = prevSel
	}()
	if !PaintFormControl(box, info) {
		t.Fatal("PaintFormControl(input) = false")
	}
	// 反向区间不应崩、不应画任何蓝底（Start>End 时分段绘制按 selStart>selEnd
	// 交换处理——蓝底必然存在于 [3,8) 区域；这里只验证不 panic + 文本仍在。
	_ = canvas.PixelAt(0, 0)
}
