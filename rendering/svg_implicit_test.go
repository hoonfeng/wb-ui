package rendering

import (
	"testing"
)

// TestParseSVGPathImplicitRepeat: consecutive parameter groups after a
// command are implicit repetitions — "l-8-3-8 3" must produce TWO l commands
// (this is what feather's shield icon uses; the second group was previously
// dropped, breaking the left half of the shield).
func TestParseSVGPathImplicitRepeat(t *testing.T) {
	// Shield outline: M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z
	cmds := parseSVGPathData("M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z")
	// Expect: M(12,22) s(-8,-4,8,-10) V(5) l(-8,-3) l(-8,3) v(7) c(0,6,8,10,8,10) z
	want := []struct {
		kind byte
		n    int
	}{
		{'M', 2}, {'s', 4}, {'V', 1}, {'l', 2}, {'l', 2}, {'v', 1}, {'c', 6}, {'z', 0},
	}
	if len(cmds) != len(want) {
		t.Fatalf("cmds=%d (%+v), want %d", len(cmds), cmds, len(want))
	}
	for i, w := range want {
		if cmds[i].kind != w.kind {
			t.Errorf("cmds[%d].kind=%c, want %c", i, cmds[i].kind, w.kind)
		}
		if len(cmds[i].args) != w.n {
			t.Errorf("cmds[%d].args len=%d, want %d", i, len(cmds[i].args), w.n)
		}
	}
	// The second l must be (-8, 3) — the missing left-bottom segment.
	last := cmds[4]
	if len(last.args) == 2 && last.args[0] != -8 || len(last.args) == 2 && last.args[1] != 3 {
		t.Errorf("second l args=%v, want [-8 3]", last.args)
	}
}

// TestParseSVGPathGearArcs: the settings gear path repeats the 'a' command
// implicitly — each 7-number group is a separate arc.
func TestParseSVGPathGearArcs(t *testing.T) {
	d := "M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0"
	cmds := parseSVGPathData(d)
	var arcs int
	for _, c := range cmds {
		if c.kind == 'a' {
			arcs++
			if len(c.args) != 7 {
				t.Errorf("arc args=%v len=%d, want 7", c.args, len(c.args))
			}
		}
	}
	if arcs != 3 {
		t.Errorf("arcs=%d, want 3 (a1.65..., a2 2 0 0 1 0 2.83, a2 2 0 0 1-2.83 0)", arcs)
	}
}

// TestParseSVGPathMultiMove: "M10 10 20 20 30 30" → M(10,10) L(20,20) L(30,30).
func TestParseSVGPathMultiMove(t *testing.T) {
	cmds := parseSVGPathData("M10 10 20 20 30 30")
	if len(cmds) != 3 {
		t.Fatalf("cmds=%d, want 3", len(cmds))
	}
	if cmds[0].kind != 'M' || cmds[1].kind != 'L' || cmds[2].kind != 'L' {
		t.Errorf("kinds=%c%c%c, want MLL", cmds[0].kind, cmds[1].kind, cmds[2].kind)
	}
	if cmds[2].args[0] != 30 || cmds[2].args[1] != 30 {
		t.Errorf("last args=%v, want [30 30]", cmds[2].args)
	}
}
