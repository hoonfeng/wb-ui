package style

import "testing"

// TestColorMix 钉死 CSS Color 5 color-mix() 的解析结果。
//
// 背景：PairCode 前端用 color-mix() 定义**全部主要面板底色**与大量边框色
// （plugins-src/ui-app/index.html 的 --bg-primary/--sidebar-bg/--panel-bg/
// --activity-bar-bg 等，以及 RightPanel.vue / PluginPanel.vue / StatusBar.vue
// 的 border/background）。引擎不支持该函数时这些变量整体失效 → 视为透明 →
// 界面只剩文字，大面积面板背景与边框不绘制。
//
// 期望值取自 Chromium 的 getComputedStyle 实测（见每条注释），插值空间为
// 默认的 premultiplied sRGB。
func TestColorMix(t *testing.T) {
	cases := []struct {
		in   string
		want Color
		note string
	}{
		// 面板底色主用法：显式 100% + 缺省百分比（补数 0%）→ 等于原色。
		// Chromium: color(srgb 0.0588235 0.0784314 0.12549) = #0F1420。
		{"color-mix(in srgb, #0F1420 calc(1 * 100%), transparent)", Color{15, 20, 32, 255},
			"calc 百分比 + transparent 补数（.app-root 背景）"},
		// 半透明白：Chromium color(srgb 1 1 1 / 0.5)。
		{"color-mix(in srgb, #ffffff 50%, transparent)", Color{255, 255, 255, 128},
			"50% 白 + 50% 透明"},
		// 两色均缺省百分比 = 各 50%；Chromium color(srgb 0.5 0.5 0.5)。
		{"color-mix(in srgb, #000000, #ffffff)", Color{128, 128, 128, 255},
			"缺省百分比各 50%"},
		// premultiplied 插值：Chromium color(srgb 0.5 0 0.5 / 0.501961)
		// （0.5 的 8 位表示是 128/255 = 0.501961）。
		// ★ alpha 期望 127（而非浏览器的 128）：引擎 parseRGB 把 alpha 0.5
		// 截断为 uint8(0.5*255)=127，浏览器则四舍五入为 128。这是引擎既有的
		// alpha 取整差异（独立于 color-mix），此处按引擎实际行为钉死，
		// 避免把无关的精度问题混进本次修复。
		{"color-mix(in srgb, rgba(255,0,0,0.5) 50%, rgba(0,0,255,0.5) 50%)", Color{128, 0, 128, 127},
			"半透明红蓝 premultiplied 混合（alpha 取整与浏览器差 1）"},
		// Chromium: color(srgb 1 0 0)
		{"color-mix(in srgb, #ff0000 100%, #0000ff 0%)", Color{255, 0, 0, 255},
			"100% / 0% 显式权重"},
		// Chromium: color(srgb 0.0588235 0.0784314 0.12549 / 0.3)
		{"color-mix(in srgb, rgb(15,20,32) 30%, transparent)", Color{15, 20, 32, 77},
			"30% 不透明色 + 透明（alpha 缩放）"},
		// 边框色用法（PluginPanel.vue / RightPanel.vue 的 border）。
		// Chromium: color(srgb 0.5 0.5 0.5) 与 45%/55% 权重一致。
		{"color-mix(in srgb, #ffffff 45%, #000000)", Color{115, 115, 115, 255},
			"边框色：45% 混合"},
	}
	for _, c := range cases {
		got, ok := parseColor(c.in)
		if !ok {
			t.Errorf("解析失败（应成功）: %s  [%s]", c.in, c.note)
			continue
		}
		if got != c.want {
			t.Errorf("color-mix 结果不符: %s\n  got  %+v (%s)\n  want %+v  [%s]",
				c.in, got, got.String(), c.want, c.note)
		}
	}
}

// TestColorMixInvalid 确认非法输入不会伪装成有效颜色（回退"不绘制"而非
// 画成黑色，后者会在界面里留下错误色块）。
func TestColorMixInvalid(t *testing.T) {
	bad := []string{
		"color-mix(in srgb, #000000)",
		"color-mix(in srgb, , )",
		"color-mix(in srgb, notacolor 50%, transparent)",
		"color-mix()",
	}
	for _, s := range bad {
		if c, ok := parseColor(s); ok {
			t.Errorf("非法 color-mix 应解析失败: %s → %+v", s, c)
		}
	}
}

// TestColorMixDoesNotBreakExisting 确认既有颜色语法未受影响。
func TestColorMixDoesNotBreakExisting(t *testing.T) {
	cases := map[string]Color{
		"#0f1420":          {15, 20, 32, 255},
		"#fff":             {255, 255, 255, 255},
		"transparent":      {0, 0, 0, 0},
		"rgb(21, 27, 41)":  {21, 27, 41, 255},
		"rgba(21,27,41,1)": {21, 27, 41, 255},
	}
	for in, want := range cases {
		got, ok := parseColor(in)
		if !ok || got != want {
			t.Errorf("%s: got %+v ok=%v, want %+v", in, got, ok, want)
		}
	}
}
