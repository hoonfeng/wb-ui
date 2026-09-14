// 复现用户报告（QQ20260804-193558）：2px 窄内容在圆角容器左端，
// 圆弧外 1px 处出现内容色渗入（浏览器 193539 该处为纯背景）。
package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

func buildNarrowRoundedContainer(t *testing.T) (*RenderView, *graphics.Canvas) {
	canvas := graphics.NewCanvas(120, 40)
	doc := dom.NewDocument()
	rv := NewRenderView(doc, style.NewComputedStyle())
	rv.SetViewportSize(120, 40)

	// 12px 高圆角容器（用户 bar 高度），overflow:hidden + border-radius 6px。
	containerStyle := style.NewComputedStyle()
	containerStyle.OverflowX = style.OverflowHidden
	containerStyle.OverflowY = style.OverflowHidden
	containerStyle.BorderRadius = style.Length{Value: 6, Unit: "px"}
	containerStyle.BackgroundColor = style.Color{R: 0x08, G: 0x19, B: 0x19, A: 0xFF}
	container := NewRenderBox(doc.CreateElement("div"), containerStyle)
	container.SetLocation(10, 10)
	container.SetSize(100, 12)

	// 2px 宽红色内容，贴容器左端（全部位于圆弧区）。
	childStyle := style.NewComputedStyle()
	childStyle.BackgroundColor = style.Color{R: 0xFF, G: 0, B: 0, A: 0xFF}
	child := NewRenderBox(doc.CreateElement("div"), childStyle)
	child.SetLocation(10, 10)
	child.SetSize(2, 12)

	rv.AddChild(container, nil)
	container.AddChild(child, nil)

	comp := NewRenderLayerCompositor(rv)
	rootLayer := comp.BuildLayerTree(RenderObject(rv))
	rv.SetRootLayer(rootLayer)
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 120, Height: 40})
	return rv, canvas
}

func isReddish(c graphics.Color) bool { return c.R > 0x80 && c.G < 0x60 && c.B < 0x60 }

// 用户场景：2px 内容在圆角容器左端。容器 (10,10)-(110,22) r=6，
// 圆弧圆心 (16,16)。内容 (10,10)-(12,22)。
// 圆弧外判定：dist((x,y),(16,16)) > 6。
func TestNarrowContentRoundedClipNoBleed(t *testing.T) {
	_, canvas := buildNarrowRoundedContainer(t)

	// 容器左上角圆弧外 (10,10)：dist=8.49>6 → 不得有红渗入。
	if isReddish(canvas.PixelAt(10, 10)) {
		t.Errorf("(10,10) = %+v — 圆弧外 1px 处有内容色渗入 (bleed into rounded corner)", canvas.PixelAt(10, 10))
	}
	// 中间行弧顶 (10,16)：dist=6 边界 → 内容应可见（红）。
	if !isReddish(canvas.PixelAt(10, 16)) {
		t.Errorf("(10,16) = %+v, want red (arc top)", canvas.PixelAt(10, 16))
	}
	// 顶部行 (11,10)：dist=sqrt(25+36)=7.8>6 圆弧外 → 不得渗入。
	if isReddish(canvas.PixelAt(11, 10)) {
		t.Errorf("(11,10) = %+v — 圆弧外内容渗入", canvas.PixelAt(11, 10))
	}
	// 底部行 (11,22)：dist=7.8>6 圆弧外 → 不得渗入。
	if isReddish(canvas.PixelAt(11, 22)) {
		t.Errorf("(11,22) = %+v — 底部圆弧外渗入", canvas.PixelAt(11, 22))
	}
	// 内容内部 (11,16)：dist=5<6 圆弧内 → 红。
	if !isReddish(canvas.PixelAt(11, 16)) {
		t.Errorf("(11,16) = %+v, want red", canvas.PixelAt(11, 16))
	}
}
