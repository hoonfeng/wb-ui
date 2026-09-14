package style

import (
	"strings"
	"testing"
)

// TestFontShorthand 验证 font 简写展开（浏览器标准语义）。
func TestFontShorthand(t *testing.T) {
	cases := []struct {
		in         string
		wantStyle  string
		wantWeight string
		wantSize   string
		wantLH     string
		wantFamily string
	}{
		{"13px/1.4 monospace", "", "", "13px", "1.4", "monospace"},
		{"bold 14px/1.2 Arial, sans-serif", "", "bold", "14px", "1.2", "Arial, sans-serif"},
		{"italic 12px/1.5 'Times New Roman', serif", "italic", "", "12px", "1.5", "'Times New Roman', serif"},
		{"16px sans-serif", "", "", "16px", "", "sans-serif"},
		{"600 12px/1.6 Consolas, 'Courier New', monospace", "", "600", "12px", "1.6", "Consolas, 'Courier New', monospace"},
		{"small-caps 700 14px/1.3 Georgia", "", "700", "14px", "1.3", "Georgia"},
	}
	for _, c := range cases {
		st, v, wt, sz, lh, fam, ok := parseFontShorthand(c.in)
		if !ok {
			t.Errorf("parseFontShorthand(%q) ok=false", c.in)
			continue
		}
		if st != c.wantStyle {
			t.Errorf("parseFontShorthand(%q) style=%q want %q", c.in, st, c.wantStyle)
		}
		if strings.Contains(c.in, "small-caps") && v != "small-caps" {
			t.Errorf("parseFontShorthand(%q) variant=%q want small-caps", c.in, v)
		}
		if !strings.Contains(c.in, "small-caps") && v != "" {
			t.Errorf("parseFontShorthand(%q) variant=%q want empty", c.in, v)
		}
		if wt != c.wantWeight {
			t.Errorf("parseFontShorthand(%q) weight=%q want %q", c.in, wt, c.wantWeight)
		}
		if sz != c.wantSize {
			t.Errorf("parseFontShorthand(%q) size=%q want %q", c.in, sz, c.wantSize)
		}
		if lh != c.wantLH {
			t.Errorf("parseFontShorthand(%q) lh=%q want %q", c.in, lh, c.wantLH)
		}
		if !strings.EqualFold(strings.Trim(fam, `"'`), strings.Trim(c.wantFamily, `"'`)) {
			t.Errorf("parseFontShorthand(%q) family=%q want %q", c.in, fam, c.wantFamily)
		}
	}
}
