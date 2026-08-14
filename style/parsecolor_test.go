package style

import "testing"

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
