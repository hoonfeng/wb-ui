package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// newVideoPanelBox 造一个 w×h 的 <video> 渲染盒（属性由 attrs 给出），返回盒、
// 画布与 PaintInfo（画布尺寸 = 盒尺寸，便于直接读像素）。
func newVideoPanelBox(t *testing.T, attrs map[string]string, w, h float64) (*RenderBox, *graphics.Canvas, *PaintInfo) {
	t.Helper()
	doc := dom.NewDocument()
	el := doc.CreateElement("video")
	for k, v := range attrs {
		el.SetAttribute(k, v)
	}
	canvas := graphics.NewCanvas(int(w), int(h))
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: w, Height: h})
	box := NewRenderBox(el, style.NewComputedStyle())
	box.SetLocation(0, 0)
	box.SetSize(w, h)
	return box, canvas, info
}

// TestVideoPanelPaintedForControlsWithoutSource：<video controls> 无有效源时绘制
// UA 占位面板 —— 上部 #333333 平台 + 底锚定渐变到 #000000（Edge 实测口径）。
func TestVideoPanelPaintedForControlsWithoutSource(t *testing.T) {
	box, canvas, info := newVideoPanelBox(t, map[string]string{"controls": ""}, 100, 200)
	defer canvas.Release()

	PaintBackground(box, info)

	if px := canvas.PixelAt(50, 5); px.R != 0x33 || px.G != 0x33 || px.B != 0x33 {
		t.Fatalf("平台像素 = %+v，want #333333", px)
	}
	if px := canvas.PixelAt(50, 199); px.R > 2 || px.G > 2 || px.B > 2 {
		t.Fatalf("面板底部像素 = %+v，want ≈#000000", px)
	}
	// 渐变起点（距底 105px）应接近平台色，距底 40px 处应明显更暗（单调、底锚定）。
	top := canvas.PixelAt(50, 95)
	low := canvas.PixelAt(50, 160)
	if top.R < 45 {
		t.Fatalf("距底 105px 像素 = %+v，want ≈#333333（渐变起点贴平台）", top)
	}
	if !(low.R < top.R && low.R < 30) {
		t.Fatalf("渐变不单调/不底锚定: 距底105=%d 距底40=%d", top.R, low.R)
	}
}

// TestVideoPanelGradientDecoupledFromBoxHeight：渐变跨度与盒高解耦 —— 两个不同
// 盒高的样本，同一「距盒底距离」处颜色一致（实测 Edge 148/300 完全同构）。
func TestVideoPanelGradientDecoupledFromBoxHeight(t *testing.T) {
	heights := []float64{148, 300}
	samples := make([]map[int]uint8, len(heights))
	for i, h := range heights {
		box, canvas, info := newVideoPanelBox(t, map[string]string{"controls": ""}, 100, h)
		PaintBackground(box, info)
		m := map[int]uint8{}
		for _, d := range []int{100, 80, 60, 40, 20, 5} {
			m[d] = canvas.PixelAt(50, int(h)-1-d).R
		}
		samples[i] = m
		canvas.Release()
	}
	for d, v0 := range samples[0] {
		if v1 := samples[1][d]; diff(v0, v1) > 2 {
			t.Fatalf("距底 %dpx：盒高 148 值=%d、盒高 300 值=%d（跨度应与盒高解耦）", d, v0, v1)
		}
	}
}

// TestVideoPanelGradientClippedNotSquashedWhenBoxShorterThanSpan：盒高小于 105px
// 跨度时，Chromium **裁掉渐变顶部**而不是把渐变压扁 —— 实测盒高 60 的顶部像素
// 为 28，等于盒高 160 样本「距底 59px」处的值。
func TestVideoPanelGradientClippedNotSquashedWhenBoxShorterThanSpan(t *testing.T) {
	shortBox, shortCanvas, shortInfo := newVideoPanelBox(t, map[string]string{"controls": ""}, 100, 60)
	PaintBackground(shortBox, shortInfo)
	defer shortCanvas.Release()

	tallBox, tallCanvas, tallInfo := newVideoPanelBox(t, map[string]string{"controls": ""}, 100, 160)
	PaintBackground(tallBox, tallInfo)
	defer tallCanvas.Release()

	shortTop := shortCanvas.PixelAt(50, 0).R
	tallAt59 := tallCanvas.PixelAt(50, 160-1-59).R
	if diff(shortTop, tallAt59) > 3 {
		t.Fatalf("盒高 60 顶部 = %d，盒高 160「距底 59」= %d（应一致：裁切而非压缩）", shortTop, tallAt59)
	}
	if shortTop > 35 {
		t.Fatalf("盒高 60 顶部 = %d，want ≈28（若为 51 说明渐变压扁了）", shortTop)
	}
}

// TestVideoPanelNotPaintedWithoutControls：没有 controls 属性的 <video> 完全不画
// （Edge 实测：整块透出页面背景）。
func TestVideoPanelNotPaintedWithoutControls(t *testing.T) {
	box, canvas, info := newVideoPanelBox(t, nil, 100, 100)
	defer canvas.Release()

	PaintBackground(box, info)

	if px := canvas.PixelAt(50, 50); px.A != 0 {
		t.Fatalf("无 controls 仍绘制面板: %+v", px)
	}
}

// TestVideoPanelNotPaintedWithSource：声明了可用媒体源时不画占位面板 —— src 属性、
// 子 <source src>、以及媒体绑定层声明的显示状态（宿主注入帧）三种都算「有源」。
func TestVideoPanelNotPaintedWithSource(t *testing.T) {
	cases := []struct {
		name  string
		attrs map[string]string
		child []map[string]string
		state *VideoElementState
	}{
		{name: "src 属性", attrs: map[string]string{"controls": "", "src": "clip.mp4"}},
		{name: "子 source", attrs: map[string]string{"controls": ""},
			child: []map[string]string{{"src": "clip.mp4"}}},
		{name: "宿主帧状态", attrs: map[string]string{"controls": ""},
			state: &VideoElementState{URL: "clip.mp4", Time: 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := dom.NewDocument()
			el := doc.CreateElement("video")
			for k, v := range tc.attrs {
				el.SetAttribute(k, v)
			}
			for _, ca := range tc.child {
				src := doc.CreateElement("source")
				for k, v := range ca {
					src.SetAttribute(k, v)
				}
				el.AppendChild(src)
			}
			if tc.state != nil {
				SetElementVideoState(el, *tc.state)
				t.Cleanup(func() { SetElementVideoState(el, VideoElementState{}) })
			}
			canvas := graphics.NewCanvas(100, 100)
			defer canvas.Release()
			info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 100, Height: 100})
			box := NewRenderBox(el, style.NewComputedStyle())
			box.SetLocation(0, 0)
			box.SetSize(100, 100)

			PaintBackground(box, info)

			if px := canvas.PixelAt(50, 50); px.A != 0 {
				t.Fatalf("有媒体源仍绘制占位面板: %+v", px)
			}
		})
	}
}

// TestVideoPanelOpaqueOverAuthorBackground：面板不透明 —— 作者 background 先画，
// 面板盖在上面（Edge 实测：<video controls style="background:#00ff00"> 的绿完全
// 看不见）。
func TestVideoPanelOpaqueOverAuthorBackground(t *testing.T) {
	box, canvas, info := newVideoPanelBox(t, map[string]string{"controls": ""}, 100, 160)
	defer canvas.Release()
	box.Style().BackgroundColor = style.Color{R: 0x00, G: 0xFF, B: 0x00, A: 0xFF}

	PaintBackground(box, info)

	px := canvas.PixelAt(50, 10) // 平台区
	if px.G > 0x40 || px.R != 0x33 {
		t.Fatalf("平台区像素 = %+v，want #333333（面板应盖住作者绿色背景）", px)
	}
}

// diff 返回两个字节值的绝对差。
func diff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}
