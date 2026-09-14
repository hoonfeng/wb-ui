package layout

import "testing"

// TestListAlphaRomanFormatting verifies the alphabetic and Roman marker
// helpers used by listMarkerFor (CSS Lists & Counters §4.1).
func TestListAlphaRomanFormatting(t *testing.T) {
	cases := []struct {
		n    int
		base rune
		want string
	}{
		{1, 'a', "a"},
		{2, 'a', "b"},
		{26, 'a', "z"},
		{27, 'a', "aa"},
		{52, 'a', "az"},
		{53, 'a', "ba"},
		{1, 'A', "A"},
		{27, 'A', "AA"},
	}
	for _, c := range cases {
		if got := listAlphaMarker(c.n, c.base); got != c.want {
			t.Errorf("listAlphaMarker(%d, %q) = %q, want %q", c.n, c.base, got, c.want)
		}
	}

	romanCases := []struct {
		n    int
		up   bool
		want string
	}{
		{1, true, "I"},
		{4, true, "IV"},
		{9, true, "IX"},
		{40, true, "XL"},
		{90, true, "XC"},
		{400, true, "CD"},
		{900, true, "CM"},
		{3999, true, "MMMCMXCIX"},
		{4, false, "iv"},
		{9, false, "ix"},
	}
	for _, c := range romanCases {
		if got := listRomanMarker(c.n, c.up); got != c.want {
			t.Errorf("listRomanMarker(%d, %v) = %q, want %q", c.n, c.up, got, c.want)
		}
	}
}
