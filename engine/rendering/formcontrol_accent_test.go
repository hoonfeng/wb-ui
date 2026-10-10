package rendering

import (
	"testing"

	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// TestAccentColorFor：accent-color 计算值解析 —— 未声明/auto/非法值回退 UA
// 默认色，声明了则取该色（CSS-UI-4 §3）。
func TestAccentColorFor(t *testing.T) {
	fallback := graphics.Color{R: 9, G: 9, B: 9, A: 255}
	cases := []struct {
		name string
		val  string
		want graphics.Color
	}{
		{name: "未声明", val: "", want: fallback},
		{name: "auto", val: "auto", want: fallback},
		{name: "非法值", val: "not-a-color", want: fallback},
		{name: "十六进制", val: "#1473e6", want: graphics.Color{R: 20, G: 115, B: 230, A: 255}},
		{name: "rgb()", val: "rgb(20, 115, 230)", want: graphics.Color{R: 20, G: 115, B: 230, A: 255}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := style.NewComputedStyle()
			if tc.val != "" {
				st.Properties["accent-color"] = tc.val
			}
			got := accentColorFor(st, fallback)
			if got != tc.want {
				t.Fatalf("accentColorFor(%q) = %+v，want %+v", tc.val, got, tc.want)
			}
		})
	}
}

// TestCheckboxUsesAccentColorWhenChecked：checked checkbox 的底色取元素
// accent-color（Edge 实测：载体页声明 accent-color:var(--accent)=#1473E6 →
// 底色 (20,115,230)）；未声明时仍是 UA 默认蓝。
func TestCheckboxUsesAccentColorWhenChecked(t *testing.T) {
	draw := func(accent string) *graphics.Canvas {
		st := style.NewComputedStyle()
		if accent != "" {
			st.Properties["accent-color"] = accent
		}
		canvas := graphics.NewCanvas(16, 16)
		info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 16, Height: 16})
		paintCheckbox(info, st, 0, 0, 16, 16, true, 1)
		return canvas
	}

	withAccent := draw("#1473e6")
	defer withAccent.Release()
	found := false
	for x := 0; x < 16 && !found; x++ {
		for y := 0; y < 16; y++ {
			if px := withAccent.PixelAt(x, y); px.R == 20 && px.G == 115 && px.B == 230 {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("checked checkbox 未使用 accent-color #1473e6 作为底色")
	}

	noAccent := draw("")
	defer noAccent.Release()
	if px := noAccent.PixelAt(8, 2); px.R == 20 && px.G == 115 && px.B == 230 {
		t.Fatal("未声明 accent-color 时不应使用载体页的 accent 值")
	}
	if px := noAccent.PixelAt(8, 2); px.R != 26 || px.G != 115 || px.B != 232 {
		t.Fatalf("未声明 accent-color 的 checked 底 = %+v，want UA 默认 (26,115,232)", px)
	}
}
