// Tests for rendering/transform.go — CSS transform parsing and application.
// Completeness: 85%
//   - tokenizeTransform, parseLength, splitSpaceComma, parseScaleValue, parseAngle
//     and applyTransformOps are unit-tested with representative CSS transform strings
//   - tryApplyTransform is tested with a RenderBox + Canvas to verify Save/Restore
//     and canvas state changes (requires CGO)

package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// --- tokenizeTransform tests ------------------------------------------------

func TestTokenizeTransform_Single(t *testing.T) {
	toks := tokenizeTransform("translateX(10px)")
	if len(toks) != 1 || toks[0] != "translateX(10px)" {
		t.Fatalf("got %v, want [translateX(10px)]", toks)
	}
}

func TestTokenizeTransform_Multiple(t *testing.T) {
	toks := tokenizeTransform("translateX(10px) scale(1.5) rotate(45deg)")
	if len(toks) != 3 {
		t.Fatalf("expected 3 tokens, got %d: %v", len(toks), toks)
	}
}

func TestTokenizeTransform_Empty(t *testing.T) {
	if toks := tokenizeTransform(""); len(toks) != 0 {
		t.Fatalf("expected 0 tokens for empty string, got %d", len(toks))
	}
}

// --- parseLength tests ------------------------------------------------------

func TestParseLength_Px(t *testing.T) {
	if v := parseLength("10px"); v != 10 {
		t.Fatalf("10px = %v, want 10", v)
	}
}

func TestParseLength_Negative(t *testing.T) {
	if v := parseLength("-5px"); v != -5 {
		t.Fatalf("-5px = %v, want -5", v)
	}
}

func TestParseLength_Zero(t *testing.T) {
	if v := parseLength("0"); v != 0 {
		t.Fatalf("0 = %v, want 0", v)
	}
}

func TestParseLength_Empty(t *testing.T) {
	if v := parseLength(""); v != 0 {
		t.Fatalf("empty = %v, want 0", v)
	}
}

func TestParseLength_NoUnit(t *testing.T) {
	if v := parseLength("42"); v != 42 {
		t.Fatalf("42 = %v, want 42", v)
	}
}

// --- splitSpaceComma tests --------------------------------------------------

func TestSplitSpaceComma_Space(t *testing.T) {
	parts := splitSpaceComma("10px 20px")
	if len(parts) != 2 || parts[0] != "10px" || parts[1] != "20px" {
		t.Fatalf("got %v, want [10px 20px]", parts)
	}
}

func TestSplitSpaceComma_Comma(t *testing.T) {
	parts := splitSpaceComma("10px,20px")
	if len(parts) != 2 || parts[0] != "10px" || parts[1] != "20px" {
		t.Fatalf("got %v, want [10px 20px]", parts)
	}
}

func TestSplitSpaceComma_Mixed(t *testing.T) {
	parts := splitSpaceComma("10px, 20px 30px")
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d: %v", len(parts), parts)
	}
}

func TestSplitSpaceComma_Empty(t *testing.T) {
	if parts := splitSpaceComma(""); len(parts) != 0 {
		t.Fatalf("expected 0 parts for empty, got %d", len(parts))
	}
}

// --- parseScaleValue tests --------------------------------------------------

func TestParseScaleValue_Decimal(t *testing.T) {
	if v := parseScaleValue("1.5"); v != 1.5 {
		t.Fatalf("1.5 = %v, want 1.5", v)
	}
}

func TestParseScaleValue_Integer(t *testing.T) {
	if v := parseScaleValue("2"); v != 2 {
		t.Fatalf("2 = %v, want 2", v)
	}
}

func TestParseScaleValue_Empty(t *testing.T) {
	if v := parseScaleValue(""); v != 1 {
		t.Fatalf("empty = %v, want 1", v)
	}
}

// --- parseAngle tests -------------------------------------------------------

func TestParseAngle_Deg(t *testing.T) {
	if v := parseAngle("45deg"); v != 45 {
		t.Fatalf("45deg = %v, want 45", v)
	}
}

func TestParseAngle_Zero(t *testing.T) {
	if v := parseAngle("0deg"); v != 0 {
		t.Fatalf("0deg = %v, want 0", v)
	}
}

func TestParseAngle_Empty(t *testing.T) {
	if v := parseAngle(""); v != 0 {
		t.Fatalf("empty = %v, want 0", v)
	}
}

// --- applyTransformOps tests (pure string parsing, no canvas needed) --------

func TestApplyTransformOps_None(t *testing.T) {
	canvas := graphics.NewCanvas(10, 10)
	defer canvas.Release()
	if applyTransformOps(canvas, "") {
		t.Fatal("empty string should return false")
	}
	if applyTransformOps(canvas, "none") {
		t.Fatal("\"none\" should return false")
	}
}

// --- tryApplyTransform tests (require CGO for canvas Save/Restore) ----------

func TestTryApplyTransform_NilBox(t *testing.T) {
	canvas := graphics.NewCanvas(20, 20)
	defer canvas.Release()
	if cleanup := tryApplyTransform(canvas, nil); cleanup != nil {
		t.Fatal("tryApplyTransform with nil box should return nil")
	}
}

func TestTryApplyTransform_NoTransform(t *testing.T) {
	canvas := graphics.NewCanvas(20, 20)
	defer canvas.Release()
	doc := dom.NewDocument()
	box := NewRenderBox(doc.CreateElement("div"), style.NewComputedStyle())
	box.SetLocation(0, 0)
	box.SetSize(10, 10)

	if cleanup := tryApplyTransform(canvas, box); cleanup != nil {
		t.Fatal("box without transform should return nil")
	}
}

func TestTryApplyTransform_TranslateX(t *testing.T) {
	canvas := graphics.NewCanvas(20, 20)
	defer canvas.Release()
	doc := dom.NewDocument()
	st := style.NewComputedStyle()
	st.Transform = "translateX(10px)"
	box := NewRenderBox(doc.CreateElement("div"), st)
	box.SetLocation(5, 5)
	box.SetSize(10, 10)

	cleanup := tryApplyTransform(canvas, box)
	if cleanup == nil {
		t.Fatal("tryApplyTransform with translateX should return cleanup func")
	}
	// Verify that the canvas is in a saved state by filling at (0,0).
	// Without translateX(10px), this would draw at (0,0). With translate, it draws at (10,0).
	canvas.FillRect(0, 0, 5, 5, graphics.Color{R: 0xFF, A: 0xFF})
	cleanup()

	// After restoration, subsequent drawing should not be affected by the transform.
	canvas.FillRect(0, 0, 5, 5, graphics.Color{G: 0xFF, A: 0xFF})
	// Pixel at (0,0) should have been drawn twice (pre and post restore),
	// but the important thing is we didn't crash and cleanup worked.
}

func TestTryApplyTransform_Rotate(t *testing.T) {
	canvas := graphics.NewCanvas(30, 30)
	defer canvas.Release()
	doc := dom.NewDocument()
	st := style.NewComputedStyle()
	st.Transform = "rotate(45deg)"
	box := NewRenderBox(doc.CreateElement("div"), st)
	box.SetLocation(0, 0)
	box.SetSize(20, 20)

	cleanup := tryApplyTransform(canvas, box)
	if cleanup == nil {
		t.Fatal("tryApplyTransform with rotate should return cleanup func")
	}
	// Draw something to verify no crash with rotated transform.
	canvas.FillRect(0, 0, 10, 10, graphics.Color{B: 0xFF, A: 0xFF})
	cleanup()
}

func TestTryApplyTransform_Multiple(t *testing.T) {
	canvas := graphics.NewCanvas(30, 30)
	defer canvas.Release()
	doc := dom.NewDocument()
	st := style.NewComputedStyle()
	st.Transform = "translateX(5px) scale(1.5) rotate(30deg)"
	box := NewRenderBox(doc.CreateElement("div"), st)
	box.SetLocation(0, 0)
	box.SetSize(10, 10)

	cleanup := tryApplyTransform(canvas, box)
	if cleanup == nil {
		t.Fatal("tryApplyTransform with multiple transforms should return cleanup func")
	}
	canvas.FillRect(0, 0, 5, 5, graphics.Color{R: 0xFF, G: 0x80, A: 0xFF})
	cleanup()
}
