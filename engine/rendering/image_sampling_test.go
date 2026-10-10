package rendering

// CSS `image-rendering` 的引擎侧契约：`pixelated` / `crisp-edges` 放大时必须走
// **最近邻**（只产出源色），默认 `auto` 走双线性（产出插值中间色）。
//
// 背景（TC-M-303）：绘制路径此前固定用双线性，`image-rendering` 只出现在 CSS 已知
// 属性表里、绘制层从没读过 ⇒ `pixelated` 与 `auto` 的渲染结果完全相同（引擎缺口）。
// 现在 painter 按元素的 image-rendering 选采样模式，本测试锁住底层契约。

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"

	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
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

// TestImageSamplingForCSSValues 锁住 `image-rendering` 的取值口径：引擎里只有
// imageSamplingFor 一处判定（<img> 与 background-image 共用），三个「最近邻」关键字
// 之外一律线性，大小写/空白容忍，nil 样式安全。
func TestImageSamplingForCSSValues(t *testing.T) {
	cases := []struct {
		css  string
		want graphics.ImageSampling
	}{
		{"pixelated", graphics.SamplingNearest},
		{"crisp-edges", graphics.SamplingNearest},
		{"-webkit-optimize-contrast", graphics.SamplingNearest},
		{"PIXELATED", graphics.SamplingNearest},     // 大小写不敏感
		{"  pixelated\t", graphics.SamplingNearest}, // 空白容忍
		{"auto", graphics.SamplingLinear},
		{"smooth", graphics.SamplingLinear},
		{"high-quality", graphics.SamplingLinear},
		{"bogus-value", graphics.SamplingLinear},
	}
	for _, c := range cases {
		st := &style.ComputedStyle{Properties: map[string]string{"image-rendering": c.css}}
		if got := imageSamplingFor(st); got != c.want {
			t.Errorf("image-rendering=%q ⇒ %v，期望 %v", c.css, got, c.want)
		}
	}
	// 属性缺省（Properties 为 nil）与 nil 样式：都必须是线性默认。
	if got := imageSamplingFor(&style.ComputedStyle{}); got != graphics.SamplingLinear {
		t.Errorf("缺省 image-rendering ⇒ %v，期望 SamplingLinear", got)
	}
	if got := imageSamplingFor(nil); got != graphics.SamplingLinear {
		t.Errorf("nil 样式 ⇒ %v，期望 SamplingLinear", got)
	}
}

// TestImageRenderingOnBackgroundImage：background-image 路径同样遵守 CSS
// `image-rendering`（CSS Images §3：该属性适用于元素的一切图像源，含背景图）。
//
// 走真实绘制链路 paintBackgroundFill → paintBackgroundImageTiled → DrawSampling：
// 2x2 四色图放大为 40x40 背景块，`pixelated` ⇒ 最近邻只产源色（≤4 色，交界不混合）；
// 默认 `auto` ⇒ 双线性产插值中间色。此前背景图路径固定双线性 ⇒ pixelated 与 auto
// 结果完全相同（与 Edge 不符），本测试锁住修复。
func TestImageRenderingOnBackgroundImage(t *testing.T) {
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(fourColorPNG(t))
	// 先确认背景图本身能加载：否则下面的色数比较会因「什么都没画」而失去意义。
	if img := loadBackgroundImage(uri, ""); img == nil || !img.Loaded() {
		t.Fatalf("2x2 data: URI 背景图应能加载，否则用例无效")
	}

	const side = 40
	paint := func(imageRendering string) *graphics.Canvas {
		// BackgroundImage/Size/Repeat 是嵌入 NonInheritedData 的字段：只能经
		// NewComputedStyle() 拿到（Properties 等 map 也一并在那里初始化）。
		st := style.NewComputedStyle()
		st.BackgroundImage = "url(" + uri + ")"
		st.BackgroundSize = "40px 40px" // 固有 2x2 → 放大，才能观测到采样差异
		st.BackgroundRepeat = "no-repeat"
		if imageRendering != "" {
			st.Properties["image-rendering"] = imageRendering
		}
		c := graphics.NewCanvas(side, side)
		paintBackgroundFill(&RenderBox{}, st, &PaintInfo{canvas: c},
			Rect{X: 0, Y: 0, Width: side, Height: side}, 1)
		return c
	}

	pixelated := paint("pixelated")
	defer pixelated.Release()
	auto := paint("auto")
	defer auto.Release()

	nPixelated := countDistinctOnCanvas(pixelated, side, 9)
	nAuto := countDistinctOnCanvas(auto, side, 9)

	if nPixelated > 4 {
		t.Errorf("background-image + pixelated 应只产出源色（≤4 色），实得 %d 色", nPixelated)
	}
	if nAuto <= 4 {
		t.Errorf("background-image + auto 应产出插值中间色，实得 %d 色", nAuto)
	}
	if nAuto <= nPixelated {
		t.Errorf("background-image 的 pixelated/auto 应可区分：pixelated=%d auto=%d", nPixelated, nAuto)
	}
}
