package rendering

import (
	"math"
	"testing"

	"wb-ui/engine/platform/graphics"
)

// TestParseColorStopLengthPosition: 色标位置必须支持**长度**（`1px`）——
// `linear-gradient(#2a2a2a 1px, transparent 1px)` 这类硬边条纹/网格背景的
// 真实写法（MediaStudio `#stage .grid`、各类 skeleton/分隔线）。
//
// 此前 parseColorStop 只认 `%`：`"#2a2a2a 1px"` 整串被当作颜色交给
// parseColorSimple → 解析失败 → 该色标被丢；两个色标都带长度位置时
// parseGradient 因 stops<2 返回 nil → **整条渐变被丢弃**（实测真实页面里
// 网格背景整片消失，Edge 29924 个网格线像素 / wb-ui 16）。
func TestParseColorStopLengthPosition(t *testing.T) {
	cases := []struct {
		in       string
		want     graphics.Color
		wantPx   float64
		wantPct  float64
		wantSet  bool // Position 是否已确定
		isPctPos bool
	}{
		{"#2a2a2a 1px", graphics.Color{R: 42, G: 42, B: 42, A: 255}, 1, -1, false, false},
		{"transparent 1px", graphics.Color{}, 1, -1, false, false},
		{"rgba(0, 0, 0, 0) 2px", graphics.Color{}, 2, -1, false, false},
		{"#ff0000 0%", graphics.Color{R: 255, A: 255}, -1, 0, true, true},
		{"#ff0000 50%", graphics.Color{R: 255, A: 255}, -1, 0.5, true, true},
		{"#ff0000", graphics.Color{R: 255, A: 255}, -1, -1, false, false},
		{"red", graphics.Color{R: 255, A: 255}, -1, -1, false, false},
		{"rgb(255, 0, 0) 1px", graphics.Color{R: 255, A: 255}, 1, -1, false, false},
	}
	for _, tc := range cases {
		cs := parseColorStop(tc.in)
		if cs == nil {
			t.Errorf("parseColorStop(%q) = nil，色标被丢弃", tc.in)
			continue
		}
		if cs.Color != tc.want {
			t.Errorf("parseColorStop(%q) color = %+v, want %+v", tc.in, cs.Color, tc.want)
		}
		if cs.PositionPx != tc.wantPx {
			t.Errorf("parseColorStop(%q) PositionPx = %v, want %v", tc.in, cs.PositionPx, tc.wantPx)
		}
		if tc.isPctPos {
			if math.Abs(cs.Position-tc.wantPct) > 1e-9 {
				t.Errorf("parseColorStop(%q) Position = %v, want %v", tc.in, cs.Position, tc.wantPct)
			}
		} else if tc.wantPct < 0 && cs.Position != -1 {
			t.Errorf("parseColorStop(%q) Position = %v, want -1（未指定）", tc.in, cs.Position)
		}
	}
	// 空串 / 纯位置 token 不是色标。
	if parseColorStop("") != nil {
		t.Error("parseColorStop(\"\") 应为 nil")
	}
}

// TestParseGradientHardStopWithLength: 长度位置的双色标渐变必须解析成功
// （stops=2）——旧实现下它是 nil，这正是「硬边网格背景整片消失」的直接原因。
func TestParseGradientHardStopWithLength(t *testing.T) {
	for _, in := range []string{
		"linear-gradient(#2a2a2a 1px, transparent 1px)",
		"linear-gradient(90deg, #2a2a2a 1px, transparent 1px)",
		"linear-gradient(rgb(42, 42, 42) 1px, rgba(0, 0, 0, 0) 1px)",
		"linear-gradient(#2a2a2a 1px, #2a2a2a 1px)",
	} {
		lg := parseGradient(in)
		if lg == nil {
			t.Errorf("parseGradient(%q) = nil（旧缺陷：长度位置色标被丢弃）", in)
			continue
		}
		if len(lg.Stops) != 2 {
			t.Errorf("parseGradient(%q) stops = %d, want 2", in, len(lg.Stops))
		}
		for i, s := range lg.Stops {
			if s.PositionPx != 1 {
				t.Errorf("parseGradient(%q) stop[%d].PositionPx = %v, want 1", in, i, s.PositionPx)
			}
		}
	}
}

// TestResolveGradientStopsLengthConversion: 长度位置按渐变线长度换算成比例
// （1px / 32px = 0.03125），未指定位置的色标随后被插值补全；无长度色标时
// 结果与解析期一致（零回归）。
func TestResolveGradientStopsLengthConversion(t *testing.T) {
	lg := parseGradient("linear-gradient(#2a2a2a 1px, transparent 1px)")
	if lg == nil {
		t.Fatal("parseGradient = nil")
	}
	// 解析期因含长度位置而跳过位置补全 → 此刻 Position 仍为 -1。
	if lg.Stops[0].Position != -1 {
		t.Fatalf("含长度色标时解析期不应补全位置，got %v", lg.Stops[0].Position)
	}
	stops := resolveGradientStops(lg.Stops, 32)
	want := 1.0 / 32.0
	for i, s := range stops {
		if math.Abs(s.Position-want) > 1e-9 {
			t.Errorf("stop[%d].Position = %v, want %v（1px / 32px）", i, s.Position, want)
		}
		if s.PositionPx != -1 {
			t.Errorf("stop[%d].PositionPx 未清零：%v", i, s.PositionPx)
		}
	}
	// 长度超过渐变线长度 → clamp 到 1。
	clamped := resolveGradientStops(lg.Stops, 1)
	for i, s := range clamped {
		if s.Position != 1 {
			t.Errorf("超长位置应 clamp 到 1，stop[%d] = %v", i, s.Position)
		}
	}
	// 无长度色标：resolve 不改变已解析结果（零回归）。
	plain := parseGradient("linear-gradient(#ff0000, #0000ff)")
	if plain == nil {
		t.Fatal("plain parseGradient = nil")
	}
	before := make([]float64, len(plain.Stops))
	for i, s := range plain.Stops {
		before[i] = s.Position
	}
	after := resolveGradientStops(plain.Stops, 100)
	for i := range after {
		if after[i].Position != before[i] {
			t.Errorf("无长度色标时 resolve 改变了位置：stop[%d] %v → %v", i, before[i], after[i].Position)
		}
	}
}
