package style

import (
	"testing"

	"wb-ui/css"
)

// TestTransparentBorderColor: `border: 1px solid transparent` must yield a
// border color with A=0 so painters can skip it (rounded-rect path).
func TestTransparentBorderColor(t *testing.T) {
	cs := NewComputedStyle()
	decls := css.NewParser("border: 1px solid transparent").ParseDeclarationList()
	if len(decls) == 0 {
		t.Fatal("no declarations parsed")
	}
	applyDeclaration(cs, decls[0])
	top := cs.BorderColor("top")
	t.Logf("border-top-color = %+v (A=%d)", top, top.A)
	if top.A != 0 {
		t.Fatalf("transparent border color A=%d, want 0 (must stay invisible)", top.A)
	}
	// An unset border color must still fall back to currentColor (element color).
	cs2 := NewComputedStyle()
	cs2.Color = Color{R: 0x12, G: 0x34, B: 0x56, A: 0xFF}
	decls2 := css.NewParser("border: 1px solid").ParseDeclarationList()
	applyDeclaration(cs2, decls2[0])
	top2 := cs2.BorderColor("top")
	t.Logf("unset border-top-color = %+v (want currentColor #123456)", top2)
	if top2.R != 0x12 || top2.G != 0x34 || top2.B != 0x56 || top2.A != 0xFF {
		t.Fatalf("unset border color = %+v, want currentColor #123456", top2)
	}
}
