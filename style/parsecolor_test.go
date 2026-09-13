package style

import "testing"

// TestParseColorNamedColors 验证 CSS 命名色表：green 必须是 #008000（不是
// lime），且基础 16 之外的扩展关键字（lightgray/steelblue/…）也必须解析成功
// ——此前只实现了 20 个关键字，其余静默失效（`background:lightgray` 不绘制）。
func TestParseColorNamedColors(t *testing.T) {
	cases := []struct {
		in         string
		r, g, b    uint8
		shouldFail bool
	}{
		{"green", 0x00, 0x80, 0x00, false},
		{"lime", 0x00, 0xFF, 0x00, false},
		{"GREEN", 0x00, 0x80, 0x00, false}, // keywords are case-insensitive
		{"black", 0x00, 0x00, 0x00, false},
		{"white", 0xFF, 0xFF, 0xFF, false},
		{"gray", 0x80, 0x80, 0x80, false},
		{"grey", 0x80, 0x80, 0x80, false},
		{"silver", 0xC0, 0xC0, 0xC0, false},
		{"maroon", 0x80, 0x00, 0x00, false},
		{"olive", 0x80, 0x80, 0x00, false},
		{"navy", 0x00, 0x00, 0x80, false},
		{"teal", 0x00, 0x80, 0x80, false},
		{"purple", 0x80, 0x00, 0x80, false},
		{"fuchsia", 0xFF, 0x00, 0xFF, false},
		{"magenta", 0xFF, 0x00, 0xFF, false},
		{"aqua", 0x00, 0xFF, 0xFF, false},
		{"cyan", 0x00, 0xFF, 0xFF, false},
		{"orange", 0xFF, 0xA5, 0x00, false},
		// Extended keywords: absent from the table before.
		{"lightgray", 0xD3, 0xD3, 0xD3, false},
		{"darkgrey", 0xA9, 0xA9, 0xA9, false},
		{"steelblue", 0x46, 0x82, 0xB4, false},
		{"tomato", 0xFF, 0x63, 0x47, false},
		{"whitesmoke", 0xF5, 0xF5, 0xF5, false},
		{"dodgerblue", 0x1E, 0x90, 0xFF, false},
		{"rebeccapurple", 0x66, 0x33, 0x99, false},
		{"not-a-color", 0, 0, 0, true},
	}
	for _, tc := range cases {
		c, ok := parseColor(tc.in)
		if tc.shouldFail {
			if ok {
				t.Errorf("parseColor(%q) unexpectedly succeeded: %v", tc.in, c)
			}
			continue
		}
		if !ok {
			t.Errorf("parseColor(%q) failed", tc.in)
			continue
		}
		if c.R != tc.r || c.G != tc.g || c.B != tc.b || c.A != 0xFF {
			t.Errorf("parseColor(%q) = (%d,%d,%d,%d), want (%d,%d,%d,255)",
				tc.in, c.R, c.G, c.B, c.A, tc.r, tc.g, tc.b)
		}
	}
}

// TestParseColorRGBA 验证 rgba(74,128,232,.35) 解析（border-color 简写覆盖
// border 简写色失败排查——.wbox.off 边框色未生效）。
func TestParseColorRGBA(t *testing.T) {
	c, ok := parseColor("rgba(74,128,232,.35)")
	if !ok {
		t.Fatal("parseColor(rgba(74,128,232,.35)) failed")
	}
	t.Logf("parsed rgba(74,128,232,.35) = R:%d G:%d B:%d A:%d", c.R, c.G, c.B, c.A)
	if c.A != 89 { // 0.35*255 = 89.25 → 89
		t.Errorf("alpha = %d, want 89 (0.35*255)", c.A)
	}
	if c.R != 74 || c.G != 128 || c.B != 232 {
		t.Errorf("rgb = (%d,%d,%d), want (74,128,232)", c.R, c.G, c.B)
	}

	// border-color 简写路径
	colors, ok := parseBorderColorShorthand("rgba(74,128,232,.35)")
	if !ok {
		t.Fatal("parseBorderColorShorthand(rgba(74,128,232,.35)) failed")
	}
	if len(colors) != 1 {
		t.Fatalf("colors len = %d, want 1", len(colors))
	}
	if colors[0].A != 89 {
		t.Errorf("shorthand alpha = %d, want 89", colors[0].A)
	}
}
