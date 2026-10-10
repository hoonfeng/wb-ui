package rendering

// CSS `image-rendering` 的引擎侧契约：`pixelated` / `crisp-edges` 放大时必须走
// **最近邻**（只产出源色），默认 `auto` 走双线性（产出插值中间色）。
//
// 背景（TC-M-303）：绘制路径此前固定用双线性，`image-rendering` 只出现在 CSS 已知
// 属性表里、绘制层从没读过 ⇒ `pixelated` 与 `auto` 的渲染结果完全相同（引擎缺口）。
// 现在 painter 按元素的 image-rendering 选采样模式，本测试锁住底层契约。

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"wb-ui/engine/platform/graphics"
)

// fourColorPNG 生成 2x2 四色 PNG（红/绿/蓝/黄）。2x2 是能同时区分两种采样的最小
// 尺寸：放大 20 倍时，最近邻只会产出这 4 色，双线性必然产出交界的插值色。
func fourColorPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 255})
	img.SetNRGBA(0, 1, color.NRGBA{B: 255, A: 255})
	img.SetNRGBA(1, 1, color.NRGBA{R: 255, G: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode 2x2 png: %v", err)
	}
	return buf.Bytes()
}

// countDistinctOnCanvas 在画布中央区域取 n×n 个网格点，返回不同颜色数。
func countDistinctOnCanvas(canvas *graphics.Canvas, side, n int) int {
	seen := map[graphics.Color]bool{}
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			px := 1 + i*(side-2)/(n-1)
			py := 1 + j*(side-2)/(n-1)
			seen[canvas.PixelAt(px, py)] = true
		}
	}
	return len(seen)
}

func TestImageRenderingPixelatedUsesNearestSampling(t *testing.T) {
	di := NewDecodedImage(fourColorPNG(t))
	if di == nil || !di.Loaded() {
		t.Fatalf("2x2 PNG 应解码成功")
	}
	const side = 40
	nearest := graphics.NewCanvas(side, side)
	defer nearest.Release()
	linear := graphics.NewCanvas(side, side)
	defer linear.Release()

	di.DrawSampling(nearest, 0, 0, side, side, graphics.SamplingNearest)
	di.DrawSampling(linear, 0, 0, side, side, graphics.SamplingLinear)

	nNearest := countDistinctOnCanvas(nearest, side, 9)
	nLinear := countDistinctOnCanvas(linear, side, 9)

	// 最近邻：每个目标像素直接取源像素 ⇒ 只可能看到源色（4 色）——交界处不会混合。
	if nNearest > 4 {
		t.Errorf("最近邻放大应只产出源色（≤4 色），实得 %d 色", nNearest)
	}
	// 双线性：交界处必然混合出中间色。
	if nLinear <= nNearest {
		t.Errorf("双线性放大应产出插值中间色（多于最近邻），实得 nearest=%d linear=%d", nNearest, nLinear)
	}
}

// TestDrawImageKeepsLinearDefault：不指定采样模式时（DrawImage）必须仍是双线性——
// `image-rendering: auto` 是默认值，不能让所有图片绘制都被改成最近邻。
func TestDrawImageKeepsLinearDefault(t *testing.T) {
	di := NewDecodedImage(fourColorPNG(t))
	if di == nil || !di.Loaded() {
		t.Fatalf("2x2 PNG 应解码成功")
	}
	const side = 40
	legacy := graphics.NewCanvas(side, side)
	defer legacy.Release()
	linear := graphics.NewCanvas(side, side)
	defer linear.Release()

	di.Draw(legacy, 0, 0, side, side)
	di.DrawSampling(linear, 0, 0, side, side, graphics.SamplingLinear)

	nLegacy := countDistinctOnCanvas(legacy, side, 9)
	nLinear := countDistinctOnCanvas(linear, side, 9)
	if nLegacy != nLinear {
		t.Errorf("DrawImage 默认采样应与 SamplingLinear 一致：DrawImage=%d SamplingLinear=%d", nLegacy, nLinear)
	}
	if nLegacy <= 4 {
		t.Errorf("默认（auto）放大应产出插值中间色，实得 %d 色", nLegacy)
	}
}
