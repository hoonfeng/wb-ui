// graphics/canvas2dsupport_test.go — canvas 2D 支持原语（DrawImageFull 等）
// 的像素级单元测试。
package graphics

import (
	"testing"

	"github.com/hoonfeng/goskia/skia"
)

// TestConcatMatrix：graphics.Canvas.Concat（矩阵变换）是否真实生效。
func TestConcatMatrix(t *testing.T) {
	cv := NewCanvas(200, 200)
	defer cv.Release()
	cv.Concat(skia.Matrix{
		ScaleX: 2, SkewX: 0, TransX: 0,
		SkewY: 0, ScaleY: 2, TransY: 0,
	})
	cv.FillRect(0, 0, 10, 10, Color{R: 0, G: 0, B: 255, A: 255})
	// 2x 缩放后 fillRect(0,0,10,10) 占 (0,0)-(20,20)
	p := cv.PixelAt(15, 15)
	if p.B < 200 {
		t.Fatalf("Concat scale pixel (15,15)=%+v, want blue", p)
	}
	out := cv.PixelAt(50, 50)
	if out.A != 0 {
		t.Fatalf("Concat scale outside (50,50)=%+v, want transparent", out)
	}
}

func TestDrawImageFullBlits(t *testing.T) {
	cv := NewCanvas(100, 100)
	defer cv.Release()
	// 构建 4×4 红色图（premul ∟: r=255,g=0,b=0,a=255）
	img, err := skia.NewImageFromPixels(
		skia.NewImageInfo(4, 4, skia.ColorTypeRGBA8888, skia.AlphaTypePremul),
		[]byte{
			255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255,
			255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255,
			255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255,
			255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255,
		}, 4*4)
	if err != nil || img == nil {
		t.Fatalf("NewImageFromPixels: err=%v img=%v", err, img != nil)
	}
	defer img.Release()
	cv.DrawImageFull(img, 0, 0, 4, 4, 10, 10, 40, 40, 1, skia.BlendModeSrcOver)
	p := cv.PixelAt(20, 20)
	if p.R < 200 || p.A < 200 {
		t.Fatalf("DrawImageFull pixel = %+v, want red", p)
	}
	out := cv.PixelAt(60, 60)
	if out.A != 0 {
		t.Fatalf("outside = %+v, want transparent", out)
	}
	// 带 alpha 的子矩形绘制
	cv.Clear(Color{})
	cv.DrawImageFull(img, 1, 1, 2, 2, 0, 0, 50, 50, 0.5, skia.BlendModeSrcOver)
	half := cv.PixelAt(25, 25)
	if half.A > 180 || half.A < 100 {
		t.Fatalf("alpha 0.5 pixel A=%d, want ~128", half.A)
	}
}

// TestDrawVerticesBlits：DrawVerticesFull（Skia drawVertices）是否真实
// 绘制——4 顶点四边形（2 三角形）全量 uv → 中心像素应为红色。
func TestDrawVerticesBlits(t *testing.T) {
	cv := NewCanvas(100, 100)
	defer cv.Release()
	img, err := skia.NewImageFromPixels(
		skia.NewImageInfo(4, 4, skia.ColorTypeRGBA8888, skia.AlphaTypePremul),
		[]byte{
			255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255,
			255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255,
			255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255,
			255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255, 255, 0, 0, 255,
		}, 4*4)
	if err != nil || img == nil {
		t.Fatalf("NewImageFromPixels: err=%v img=%v", err, img != nil)
	}
	defer img.Release()
	// 四边形 (10,10)-(50,10)-(10,50)-(50,50)，uv 0..0.5（红区域，全图
	// 0..1 会采样到 (2,2) 蓝区）
	pos := []float32{10, 10, 50, 10, 10, 50, 50, 50}
	uvA := []float32{0, 0, 0.5, 0, 0, 0.5, 0.5, 0.5}
	idx := []uint16{0, 1, 2, 1, 3, 2}
	cv.DrawVerticesFull(img, pos, uvA, idx, 1, skia.BlendModeSrcOver)
	p := cv.PixelAt(30, 30)
	if p.R < 200 || p.A < 200 {
		t.Fatalf("DrawVerticesFull center = %+v, want red", p)
	}
	out := cv.PixelAt(60, 60)
	if out.A != 0 {
		t.Fatalf("outside = %+v, want transparent", out)
	}
}

// TestDrawVerticesUvSem：判定 DrawVerticesFull 的 uv 换算是否正确——
// 4×4 图左上 2×2 红、右下 2×2 蓝；uv 取 0.5..1（右下象限）且内部
// 0..1 → 像素（×4）→ 采样 (2,2)~(4,4) 像素 = 蓝区。
func TestDrawVerticesUvSem(t *testing.T) {
	cv := NewCanvas(100, 100)
	defer cv.Release()
	mk := func() []byte {
		px := make([]byte, 4*4*4)
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				i := (y*4 + x) * 4
				if x < 2 && y < 2 {
					px[i], px[i+3] = 255, 255 // 红
				} else {
					px[i+2], px[i+3] = 255, 255 // 蓝
				}
			}
		}
		return px
	}
	img, err := skia.NewImageFromPixels(
		skia.NewImageInfo(4, 4, skia.ColorTypeRGBA8888, skia.AlphaTypePremul),
		mk(), 4*4)
	if err != nil || img == nil {
		t.Fatalf("NewImageFromPixels: err=%v", err)
	}
	defer img.Release()
	pos := []float32{10, 10, 60, 10, 10, 60, 60, 60}
	uvQ := []float32{0.5, 0.5, 1, 0.5, 0.5, 1, 1, 1}
	idx := []uint16{0, 1, 2, 1, 3, 2}
	cv.DrawVerticesFull(img, pos, uvQ, idx, 1, skia.BlendModeSrcOver)
	p := cv.PixelAt(35, 35)
	t.Logf("uv 0.5..1 quad center = %+v (期望 B 高 = 像素空间换算生效)", p)
	if p.B < 200 {
		t.Fatalf("uv 0.5..1 → 期望采样到蓝区 (B 高)，得到 %+v", p)
	}
}
