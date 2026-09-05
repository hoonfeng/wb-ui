// rendering/canvassurface.go — <canvas> 元素的后端位图（backing store）。
//
// Translation of: Source/WebCore/html/canvas/CanvasRenderingContext2D 的
// backend 部分（CanvasRenderingContext2D 在 WebKit 中经由
// HTMLCanvasElement 的 ImageBuffer 渲染）。
//
// Simplifications:
//   - 位图 = 一个离屏 graphics.Raster Canvas（Skia Surface），JS 侧
//     (bindings CanvasRenderingContext2D) 直接绘制到它；渲染管线在
//     PaintCanvas 中把当前内容快照 Blit 到页面画布的元素内容盒。
//   - 位图尺寸 = canvas width/height 属性（默认为 300×150），与 CSS
//     布局尺寸（content box）解耦；绘制时拉伸填充（浏览器语义）。
//   - CanvasBitmap 挂在 dom.Element.canvasSurface（内部桥，bindings
//     写入、rendering 读取），避免 dom → graphics 包依赖。
//   - 未实现：GPU 加速位图、filter 后备缓存、动画帧时序（代码直接
//     重绘由宿主帧循环驱动）。

package rendering

import (
	"sync"

	"github.com/hoonfeng/goskia/skia"
	"wb-ui/platform/graphics"
)

// CanvasBitmap 是 <canvas> 元素的离屏绘制表面。
type CanvasBitmap struct {
	mu sync.Mutex // 保护 Cv（JS 绘制与页面 Paint 可发生在不同 goroutine）
	Cv *graphics.Canvas
	W  int // 位图像素宽（canvas.width 属性）
	H  int // 位图像素高（canvas.height 属性）
}

// NewCanvasBitmap 创建指定尺寸的离屏位图。
func NewCanvasBitmap(w, h int) *CanvasBitmap {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return &CanvasBitmap{
		Cv: graphics.NewCanvas(w, h),
		W:  w,
		H:  h,
	}
}

// Resize 释放旧表面并按新尺寸重建位图（canvas.width/height 属性变化）。
// 内容清空（浏览器语义：重置位图）。
func (b *CanvasBitmap) Resize(w, h int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Cv != nil {
		b.Cv.Release()
	}
	b.Cv = graphics.NewCanvas(w, h)
	b.W = w
	b.H = h
}

// DrawTo 把位图当前内容快照绘制到目标画布的 (x, y, w, h)（拉伸填充）。
func (b *CanvasBitmap) DrawTo(dst *graphics.Canvas, x, y, w, h float64) {
	b.mu.Lock()
	cv := b.Cv
	b.mu.Unlock()
	if cv == nil || dst == nil {
		return
	}
	img := cv.Snapshot()
	if img == nil {
		return
	}
	defer img.Release()
	dst.DrawImage(img, x, y, w, h)
}

// SkiaImage 返回位图当前内容的快照（调用方负责 Release）。
func (b *CanvasBitmap) SkiaImage() *skia.Image {
	b.mu.Lock()
	cv := b.Cv
	b.mu.Unlock()
	if cv == nil {
		return nil
	}
	return cv.Snapshot()
}
