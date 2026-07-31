package layout

import (
	"testing"

	"wb-ui/style"
)

// TestInline_SingleLine verifies that a short text run produces a single line whose
// height matches the line-height (font-size * 1.2 by default).
func TestInline_SingleLine(t *testing.T) {
	root := mkBlock()
	anon := mkAnon()
	anon.AddChild(mkTextRun("hi"))
	root.AddChild(anon)

	state := Layout(root, 800, 600)

	// Default font-size 16, line-height 1.2 -> 19.2.
	_, _, _, ah := rectOf(anon, state)
	assertApprox(t, "anon.Height", ah, 19.2)
}

// TestInline_TextWraps verifies that long text wraps across multiple lines when it
// exceeds the container width.
func TestInline_TextWraps(t *testing.T) {
	root := mkBlock()
	anon := mkAnon()
	// "hello world": with Skia ~105.6px, with fallback estimate ~88px; both
	// exceed 80px so wrapping is triggered regardless of the measurer.
	anon.AddChild(mkTextRun("hello world"))
	root.AddChild(anon)

	state := Layout(root, 80, 600)

	// Two lines: line 1 "hello ", line 2 "world". Height = 2 * 19.2 = 38.4.
	_, _, _, ah := rectOf(anon, state)
	if ah < 30 {
		t.Errorf("expected wrapped text to span multiple lines, height = %g", ah)
	}
	assertApprox(t, "anon.Height", ah, 38.4)
}

// TestInline_TextAlignCenter verifies that text-align: center shifts the line content
// to the centre of the container.
func TestInline_TextAlignCenter(t *testing.T) {
	root := mkBlock()
	anon := mkAnon()
	anon.Style().TextAlign = style.TextAlignCenter
	anon.AddChild(mkTextRun("hi"))
	root.AddChild(anon)

	Layout(root, 800, 600)

	// The text item "hi" (2 chars) has advance 2*9.6 = 19.2. Centred in 800 means
	// it starts at (800 - 19.2)/2 = 390.4.
	for _, c := range anon.Children() {
		if tb, ok := c.(*InlineTextBox); ok && len(tb.TextSegments) > 0 {
			cx := tb.TextSegments[0].X
			if cx < 300 {
				t.Errorf("centred text X = %g, expected >= 300", cx)
			}
		}
	}
}

// TestInline_MultipleRuns verifies that multiple inline children are laid out
// left-to-right on the same line.
func TestInline_MultipleRuns(t *testing.T) {
	root := mkBlock()
	anon := mkAnon()
	anon.AddChild(mkTextRun("ab"))
	anon.AddChild(mkTextRun("cd"))
	root.AddChild(anon)

	state := Layout(root, 800, 600)

	// Both runs fit on one line; container height is one line.
	_, _, _, ah := rectOf(anon, state)
	assertApprox(t, "anon.Height", ah, 19.2)
}
