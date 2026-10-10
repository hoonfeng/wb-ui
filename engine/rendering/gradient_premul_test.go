package rendering

import (
	"math"
	"testing"

	"wb-ui/engine/platform/graphics"
)

// --- D3：渐变色标在**预乘空间**插值（CSS/Chromium 语义）--------------------
//
// 证据链：docs/render-consistency-fixes.md §3 的 D3 行；夹具
// dev/output/tmp/gradfact.html 的 .h3（200x100 盒子、#101010 底）。
// 红像素口径 = 盒子内部 R>150 的像素数：
//
//	Edge 8800 / wb-ui 修复前 4800（逐行 R 吻合「非预乘」模型 255(1-t)²+16t）
//	修复后 wb-ui 8800（逐行 R 吻合「预乘」模型 (1-t)*255+16t）

// TestPremulLerpKeepsColorWhenAlphaFades 直接锁定插值空间：alpha 衰减时颜色
// 不向黑衰减（预乘语义）；非预乘插值会把它拉向 transparent 的黑。
func TestPremulLerpKeepsColorWhenAlphaFades(t *testing.T) {
	red := graphics.Color{R: 255, A: 255}
	clear := graphics.Color{} // transparent = rgba(0,0,0,0)
	mid := premulLerp(red, clear, 0.5)
	if mid.R != 255 {
		t.Errorf("预乘插值中点 R = %d, 期望 255（颜色不衰减）", mid.R)
	}
	if mid.A < 126 || mid.A > 130 {
		t.Errorf("预乘插值中点 A = %d, 期望 ~128", mid.A)
	}
	// 对照：非预乘插值（旧行为）在 t=0.5 处 R 已掉到 ~128。
	if old := lerpColor(red, clear, 0.5); old.R < 126 || old.R > 130 {
		t.Fatalf("对照：非预乘插值中点 R = %d, 期望 ~128", old.R)
	}
}

// TestPremulStopsSkipWhenNoAlphaVariation 锁定「无 alpha 变化 → 零行为变化」：
// 这类序列必须**原样返回**（逐字段相同、不细分、不新增色标）。
func TestPremulStopsSkipWhenNoAlphaVariation(t *testing.T) {
	cases := map[string][]ColorStop{
		"不透明两色": {
			{Color: graphics.Color{R: 255, A: 255}, Position: 0},
			{Color: graphics.Color{B: 255, A: 255}, Position: 1},
		},
		"同 alpha 半透明": {
			{Color: graphics.Color{R: 255, A: 128}, Position: 0},
			{Color: graphics.Color{B: 255, A: 128}, Position: 1},
		},
		"三色同 alpha": {
			{Color: graphics.Color{R: 255, A: 255}, Position: 0},
			{Color: graphics.Color{G: 255, A: 255}, Position: 0.5},
			{Color: graphics.Color{B: 255, A: 255}, Position: 1},
		},
		"全透明": {
			{Color: graphics.Color{A: 0}, Position: 0},
			{Color: graphics.Color{A: 0}, Position: 1},
		},
	}
	for name, stops := range cases {
		got := premulStopsForShader(stops, 100)
		if len(got) != len(stops) {
			t.Fatalf("%s: 色标数 %d, 期望 %d（无 alpha 变化不得细分）", name, len(got), len(stops))
		}
		for i := range stops {
			if got[i] != stops[i] {
				t.Errorf("%s: 第 %d 个色标 %+v, 期望逐字段不变 %+v", name, i, got[i], stops[i])
			}
		}
	}
}

// TestPremulStopsHardStopTransparentDrawsIdenticalPixels 对照用例（含 alpha
// 变化、但可见像素必须逐字段不变）：硬边条纹/网格 `#2a2a2a 1px,transparent 1px`
// （MediaStudio 的 `#stage .grid` 就是这种背景）。预乘语义下透明端的 RGB
// 不可见，把它改写成可见端的 RGB 不改变任何合成结果。
func TestPremulStopsHardStopTransparentDrawsIdenticalPixels(t *testing.T) {
	lg := parseGradient("linear-gradient(#2a2a2a 1px, transparent 1px)")
	if lg == nil {
		t.Fatal("夹具解析失败")
	}
	direct := resolveGradientStops(lg.Stops, 100)
	premul := premulStopsForShader(direct, 100)

	if len(premul) != len(direct) {
		t.Fatalf("色标数 %d, 期望 %d", len(premul), len(direct))
	}
	for i := range direct {
		if direct[i].Position != premul[i].Position || direct[i].Color.A != premul[i].Color.A {
			t.Errorf("第 %d 个色标位置/alpha 变了: %+v → %+v", i, direct[i], premul[i])
		}
		if direct[i].Color.A != 0 && direct[i].Color != premul[i].Color {
			t.Errorf("第 %d 个**非透明**色标被改写: %+v → %+v", i, direct[i], premul[i])
		}
	}

	a := drawStopsOnGray(t, direct)
	defer a.Release()
	b := drawStopsOnGray(t, premul)
	defer b.Release()
	for y := 0; y < 40; y++ {
		for x := 0; x < 60; x++ {
			if pa, pb := a.PixelAt(x, y), b.PixelAt(x, y); pa != pb {
				t.Fatalf("可见像素 (%d,%d) 变化: %+v → %+v", x, y, pa, pb)
			}
		}
	}
}

// drawStopsOnGray 在 #101010 不透明底上按给定色标画一条竖直渐变（60x40），
// 供「像素逐字段对照」使用（同一轴端点、同一底色，唯一变量是色标序列）。
func drawStopsOnGray(t *testing.T, stops []ColorStop) *graphics.Canvas {
	t.Helper()
	const w, h = 60, 40
	c := graphics.NewCanvas(w, h)
	c.FillRect(0, 0, w, h, graphics.Color{R: 16, G: 16, B: 16, A: 255})
	colors := make([]graphics.Color, len(stops))
	positions := make([]float32, len(stops))
	for i, s := range stops {
		colors[i] = s.Color
		positions[i] = float32(s.Position)
	}
	c.FillRectLinearGradientStops(0, 0, w, h, float64(w)/2, 0, float64(w)/2, h, colors, positions)
	return c
}

// TestLinearGradientTransparentPremultipliedEndToEnd 端到端锁定 D3 的实际症状：
// `linear-gradient(#ff0000,transparent)` 在 #101010 底上合成后，红像素（R>150）
// 应 ≈ Edge 的 8800（修复前 4800），逐行 R 通道吻合预乘模型。
func TestLinearGradientTransparentPremultipliedEndToEnd(t *testing.T) {
	lg := parseGradient("linear-gradient(#ff0000,transparent)")
	if lg == nil {
		t.Fatal("夹具解析失败")
	}
	const w, h = 200, 100
	c := graphics.NewCanvas(w, h)
	defer c.Release()
	c.FillRect(0, 0, w, h, graphics.Color{R: 16, G: 16, B: 16, A: 255}) // 夹具底色
	paintLinearGradient(c, 0, 0, w, h, lg)

	redCount := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if c.PixelAt(x, y).R > 150 {
				redCount++
			}
		}
	}
	if redCount < 8300 || redCount > 9300 {
		t.Errorf("红像素(R>150) = %d, 期望 ≈8800（Edge 实测；非预乘旧行为 ≈4800）", redCount)
	}

	for _, y := range []int{3, 30, 60, 87, 97} {
		tt := (float64(y) + 0.5) / float64(h)
		prem := (1-tt)*255 + 16*tt           // 预乘语义
		nonPrem := 255*(1-tt)*(1-tt) + 16*tt // 非预乘语义（旧行为）
		got := float64(c.PixelAt(w/2, y).R)
		if math.Abs(got-prem) > 4 {
			t.Errorf("y=%d: R = %v, 预乘模型 = %.1f（非预乘模型 = %.1f）", y, got, prem, nonPrem)
		}
		// 区分性断言只在两个模型差距足够大时才有意义：两端附近（t→0 或
		// t→1）两条曲线都收敛到底色，差值 < 10。
		if prem-nonPrem > 12 && got < nonPrem+8 {
			t.Errorf("y=%d: R = %v 偏暗，仍像非预乘插值（模型 %.1f）", y, got, nonPrem)
		}
	}
}

// TestLinearGradientTransparentMidStopMultiSegment 多色标 + 中段透明端
// （alpha 255 → 0 → 255）：位置非递减、两段各自恒色（红段恒红、蓝段恒蓝）、
// 透明处露底色。
func TestLinearGradientTransparentMidStopMultiSegment(t *testing.T) {
	lg := parseGradient("linear-gradient(#ff0000, transparent, #0000ff)")
	if lg == nil {
		t.Fatal("夹具解析失败")
	}
	const w, h = 200, 100
	c := graphics.NewCanvas(w, h)
	defer c.Release()
	c.FillRect(0, 0, w, h, graphics.Color{R: 16, G: 16, B: 16, A: 255})
	paintLinearGradient(c, 0, 0, w, h, lg)

	if top := c.PixelAt(w/2, 2); top.R < 230 || top.B > 40 {
		t.Errorf("起始端应为纯红（预乘语义下颜色不衰减），实测 %+v", top)
	}
	if mid := c.PixelAt(w/2, h/2); mid.R > 40 || mid.G > 40 || mid.B > 40 {
		t.Errorf("中段全透明处应露底色 #101010，实测 %+v", mid)
	}
	if bot := c.PixelAt(w/2, h-3); bot.B < 230 || bot.R > 40 {
		t.Errorf("末端应为纯蓝，实测 %+v", bot)
	}
}

// --- D3 性能对照（验收项：新增预乘路径的耗时）--------------------------------

// BenchmarkPaintLinearGradientPremulPath 对照「含 alpha 变化（走预乘等价色标
// 重算）」与「无 alpha 变化（原路径）」的绘制耗时。预乘路径只增加色标个数
// ——仍是**单次** Skia 渐变绘制，不是逐像素——用于证明无显著回退。
//
//	go test ./engine/rendering/ -run=NONE -bench=PaintLinearGradientPremul -benchtime=2000x
func BenchmarkPaintLinearGradientPremulPath(b *testing.B) {
	cases := []struct {
		name string
		css  string
		raw  bool // true = 绕过预乘重算，直接用解析出的色标（= 修复前的色标数）
	}{
		{"premul-to-transparent", "linear-gradient(#ff0000,transparent)", false},
		{"premul-both-semi", "linear-gradient(#ff000080,#0000ff33)", false},
		{"premul-hard-edge-grid", "linear-gradient(#2a2a2a 1px,transparent 1px)", false},
		{"raw-both-semi-2stops", "linear-gradient(#ff000080,#0000ff33)", true},
		{"direct-opaque-2stops", "linear-gradient(#ff0000,#0000ff)", false},
		{"direct-opaque-8stops", "linear-gradient(#ff0000,#ff8000,#ffff00,#80ff00,#00ff00,#00ff80,#00ffff,#0080ff)", false},
	}
	const w, h = 200, 100
	for _, c := range cases {
		lg := parseGradient(c.css)
		if lg == nil {
			b.Fatalf("夹具解析失败: %s", c.css)
		}
		raw := c.raw
		b.Run(c.name, func(b *testing.B) {
			canvas := graphics.NewCanvas(w, h)
			defer canvas.Release()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if raw {
					stops := resolveGradientStops(lg.Stops, h)
					colors := make([]graphics.Color, len(stops))
					positions := make([]float32, len(stops))
					for i, s := range stops {
						colors[i] = s.Color
						positions[i] = float32(s.Position)
					}
					canvas.FillRectLinearGradientStops(0, 0, w, h, w/2, 0, w/2, h, colors, positions)
				} else {
					paintLinearGradient(canvas, 0, 0, w, h, lg)
				}
				_ = canvas.PixelAt(0, 0) // 触发光栅化落地，测真实绘制而非仅命令录制
			}
		})
	}
}
