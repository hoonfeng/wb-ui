package bindings

// canvas 2D measureText 原生 Skia 路径回归测试：
//   - measureText('W') 返回精确 advance（7.1475 而非 len*fs*0.6 估算）
//   - ctx.font 简写解析（px 字号 + 家族 + weight/style）
import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"wb-ui/layout"
	"wb-ui/platform/graphics"
)

func TestParseCanvasFontSpec(t *testing.T) {
	cases := []struct {
		spec         string
		family       string
		size         float64
		weight       int
		style        string
	}{
		{"13px Consolas, monospace", "Consolas, monospace", 13, 400, "normal"},
		{"bold 13px 'Cascadia Mono'", "'Cascadia Mono'", 13, 700, "normal"},
		{"italic 700 14px serif", "serif", 14, 700, "italic"},
		{"10px sans-serif", "sans-serif", 10, 400, "normal"},
		{"", "sans-serif", 10, 400, "normal"},
		{"13px", "sans-serif", 13, 400, "normal"},
	}
	for _, c := range cases {
		fam, size, weight, style := parseCanvasFontSpec(c.spec)
		if fam != c.family || size != c.size || weight != c.weight || style != c.style {
			t.Errorf("parseCanvasFontSpec(%q) = (%q,%.1f,%d,%q), want (%q,%.1f,%d,%q)",
				c.spec, fam, size, weight, style, c.family, c.size, c.weight, c.style)
		}
	}
}

// TestCanvasMeasureTextNativeSkia 验证 canvas measureText 走原生 Skia 路径：
// width = 精确 advance（Consolas 13px W = 7.1475），不再是 len×fs×0.6。
func TestCanvasMeasureTextNativeSkia(t *testing.T) {
	if graphics.GetFontManager() == nil {
		_ = graphics.InitFontManager("")
		graphics.GetFontManager().LoadSystemFonts()
	}
	prevM, prevF := layout.MeasureTextFunc, layout.FontMetricsFunc
	layout.MeasureTextFunc = func(family string, size float64, weight int, style, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style}, text)
	}
	layout.FontMetricsFunc = func(family string, size float64, weight int, style string) (float64, float64, float64) {
		f := graphics.Font{Family: family, Size: size, Weight: weight, Style: style}
		return graphics.GlobalFontAscent(f), graphics.GlobalFontDescent(f), graphics.GlobalFontLineGap(f)
	}
	defer func() { layout.MeasureTextFunc, layout.FontMetricsFunc = prevM, prevF }()

	rt, _, _ := newRuntimeWithDoc(t)
	v, err := rt.RunJS(`(function(){
		var c = document.createElement('canvas');
		var ctx = c.getContext('2d');
		ctx.font = '13px Consolas, monospace';
		var m = ctx.measureText('W');
		var sp = ctx.measureText(' ');
		return JSON.stringify({w: m.width, h: m.height,
			fba: m.fontBoundingBoxAscent, fbd: m.fontBoundingBoxDescent,
			sw: sp.width});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	js := strings.TrimSpace(v.ToString())
	var r struct {
		W, H, Fba, Fbd, Sw float64
	}
	if err := json.Unmarshal([]byte(js), &r); err != nil {
		t.Fatalf("parse %q: %v", js, err)
	}
	// Skia：Consolas 13px W advance = 7.1475（浏览器同值）。
	if math.Abs(r.W-7.1475) > 0.01 {
		t.Fatalf("measureText('W').width = %.4f, want ~7.1475 (Skia advance)", r.W)
	}
	// 旧 DOM 路径布局失败时的估算值是 13*0.6=7.8——必须消失。
	if math.Abs(r.Sw-7.1475) > 0.01 {
		t.Fatalf("measureText(' ').width = %.4f, want ~7.1475 (not len*fs*0.6=7.8)", r.Sw)
	}
	if math.Abs(r.H-15) > 0.01 {
		t.Fatalf("measureText height = %.4f, want 15 (round of Skia ascent+descent)", r.H)
	}
	if math.Abs(r.Fba-12) > 0.01 || math.Abs(r.Fbd-3) > 0.01 {
		t.Fatalf("fontBoundingBox = %.2f/%.2f, want 12/3", r.Fba, r.Fbd)
	}
}
