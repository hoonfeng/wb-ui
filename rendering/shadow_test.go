package rendering

import (
	"testing"
)

func TestParseBoxShadow_Simple(t *testing.T) {
	shadows := parseShadowList("2px 2px 4px rgba(0,0,0,0.3)")
	if len(shadows) != 1 {
		t.Fatalf("got %d shadows, want 1", len(shadows))
	}
	if shadows[0].OffsetX != 2 {
		t.Errorf("OffsetX = %v, want 2", shadows[0].OffsetX)
	}
	if shadows[0].OffsetY != 2 {
		t.Errorf("OffsetY = %v, want 2", shadows[0].OffsetY)
	}
	if shadows[0].Blur != 4 {
		t.Errorf("Blur = %v, want 4", shadows[0].Blur)
	}
	if shadows[0].Inset {
		t.Error("Inset = true, want false")
	}
}

func TestParseBoxShadow_DefaultColor(t *testing.T) {
	shadows := parseShadowList("5px 5px 10px")
	if len(shadows) != 1 {
		t.Fatalf("got %d shadows, want 1", len(shadows))
	}
	if shadows[0].OffsetX != 5 || shadows[0].OffsetY != 5 {
		t.Errorf("offset = (%v,%v), want (5,5)", shadows[0].OffsetX, shadows[0].OffsetY)
	}
}

func TestParseBoxShadow_Multiple(t *testing.T) {
	shadows := parseShadowList("2px 2px 0px rgba(0,0,0,0.5), 4px 4px 2px #000")
	if len(shadows) != 2 {
		t.Fatalf("got %d shadows, want 2", len(shadows))
	}
}

func TestParseBoxShadow_Inset(t *testing.T) {
	shadows := parseShadowList("inset 2px 2px 4px #000")
	if len(shadows) != 1 {
		t.Fatalf("got %d shadows", len(shadows))
	}
	if !shadows[0].Inset {
		t.Error("Inset should be true")
	}
}

func TestParseBoxShadow_None(t *testing.T) {
	shadows := parseShadowList("none")
	if len(shadows) != 0 {
		t.Fatalf("got %d shadows, want 0", len(shadows))
	}
	shadows = parseShadowList("")
	if len(shadows) != 0 {
		t.Fatalf("got %d shadows for empty, want 0", len(shadows))
	}
}

func TestParseBoxShadow_WithSpread(t *testing.T) {
	shadows := parseShadowList("1px 2px 3px 4px #888")
	if len(shadows) != 1 {
		t.Fatalf("got %d shadows", len(shadows))
	}
	if shadows[0].Spread != 4 {
		t.Errorf("Spread = %v, want 4", shadows[0].Spread)
	}
}
