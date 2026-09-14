// 复现用户报告：圆角容器（overflow:hidden + border-radius）内部的内容
// （方角块 / 渐变）未被圆角裁切，方角盖住圆角（QQ20260804-193558）。
// 验证：圆角弧外区域不得出现内容色。
package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// 公共构造：overflow:hidden + border-radius:6px 的容器，内含方角子元素。
func buildRoundedContainer(t *testing.T) (*RenderView, *graphics.Canvas, *RenderBox, *RenderBox) {
	canvas := graphics.NewCanvas(200, 60)
	doc := dom.NewDocument()
	rv := NewRenderView(doc, style.NewComputedStyle())
	rv.SetViewportSize(200, 60)

	containerStyle := style.NewComputedStyle()
	containerStyle.OverflowX = style.OverflowHidden
	containerStyle.OverflowY = style.OverflowHidden
	containerStyle.BorderRadius = style.Length{Value: 6, Unit: "px"}
	containerStyle.BackgroundColor = style.Color{R: 0x2A, G: 0x2A, B: 0x2A, A: 0xFF}
	container := NewRenderBox(doc.CreateElement("div"), containerStyle)
	container.SetLocation(10, 10)
	container.SetSize(160, 20)

	childStyle := style.NewComputedStyle()
	childStyle.BackgroundColor = style.Color{R: 0xFF, G: 0, B: 0, A: 0xFF}
	child := NewRenderBox(doc.CreateElement("div"), childStyle)
	child.SetLocation(10, 10)
	child.SetSize(160, 20)

	rv.AddChild(container, nil)
	container.AddChild(child, nil)
	return rv, canvas, container, child
}

func TestRoundedOverflowClipChild(t *testing.T) {
	rv, canvas, _, _ := buildRoundedContainer(t)
	comp := NewRenderLayerCompositor(rv)
	rootLayer := comp.BuildLayerTree(RenderObject(rv))
	rv.SetRootLayer(rootLayer)
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 200, Height: 60})

	isRed := func(c graphics.Color) bool { return c.R > 0xE0 && c.G < 0x20 && c.B < 0x20 }
	// 容器左上角 (10,10)：r=6 圆弧外 → 必须被裁掉（透明/背景），不得渗入红。
	if isRed(canvas.PixelAt(10, 10)) {
		t.Errorf("corner (10,10) = red — 方角子元素盖住圆角 (rounded overflow clip FAILED)")
	}
	// 中间行弧顶 (10,16)：圆弧内 → 红（子元素可见）。
	if !isRed(canvas.PixelAt(10, 16)) {
		t.Errorf("mid (10,16) = %+v, want red (child visible at arc top)", canvas.PixelAt(10, 16))
	}
	// 顶部直线边 (16,10)：x>弧心 → 圆弧内 → 红。
	if !isRed(canvas.PixelAt(16, 10)) {
		t.Errorf("top (16,10) = %+v, want red (straight top edge inside radius)", canvas.PixelAt(16, 10))
	}
	// 容器右上角 (170,10)：弧外 → 无红。
	if isRed(canvas.PixelAt(170, 10)) {
		t.Errorf("corner (170,10) = red — right corner NOT clipped")
	}
	// 容器左下角 (10,30)：弧外 → 无红。
	if isRed(canvas.PixelAt(10, 30)) {
		t.Errorf("corner (10,30) = red — bottom-left NOT clipped")
	}
}

// 渐变子元素（ctx-bar-fill 同型）：painter 对 r=0 的渐变走方角
// paintLinearGradient，只能依赖容器 layer 圆角 clip 裁切。
func TestRoundedOverflowClipGradientChild(t *testing.T) {
	canvas := graphics.NewCanvas(200, 60)
	doc := dom.NewDocument()
	rv := NewRenderView(doc, style.NewComputedStyle())
	rv.SetViewportSize(200, 60)

	containerStyle := style.NewComputedStyle()
	containerStyle.OverflowX = style.OverflowHidden
	containerStyle.OverflowY = style.OverflowHidden
	containerStyle.BorderRadius = style.Length{Value: 6, Unit: "px"}
	containerStyle.BackgroundColor = style.Color{R: 0x2A, G: 0x2A, B: 0x2A, A: 0xFF}
	container := NewRenderBox(doc.CreateElement("div"), containerStyle)
	container.SetLocation(10, 10)
	container.SetSize(160, 20)

	// 渐变子元素：BorderRadius=0 → painter 直接画方角渐变。
	gradStyle := style.NewComputedStyle()
	gradStyle.BackgroundImage = "linear-gradient(to right, #ff0000, #0000ff)"
	grad := NewRenderBox(doc.CreateElement("div"), gradStyle)
	grad.SetLocation(10, 10)
	grad.SetSize(160, 20)

	rv.AddChild(container, nil)
	container.AddChild(grad, nil)

	comp := NewRenderLayerCompositor(rv)
	rootLayer := comp.BuildLayerTree(RenderObject(rv))
	rv.SetRootLayer(rootLayer)
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 200, Height: 60})

	isRed := func(c graphics.Color) bool { return c.R > 0xE0 && c.B < 0x20 }
	// 左上角弧外 (10,10)：不得渗入渐变的红色端。
	if isRed(canvas.PixelAt(10, 10)) {
		t.Errorf("corner (10,10) = %+v — 渐变方角盖住圆角 (rounded clip FAILED)", canvas.PixelAt(10, 10))
	}
	// 中间行弧顶 (10,16)：渐变可见（红端）。
	if !isRed(canvas.PixelAt(10, 16)) {
		t.Errorf("mid (10,16) = %+v, want gradient red", canvas.PixelAt(10, 16))
	}
}
