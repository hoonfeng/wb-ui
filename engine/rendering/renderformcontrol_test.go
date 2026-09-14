// Tests for engine/rendering/renderformcontrol.go, verifying that PaintFormControl
// dispatches to the correct painter and that the canvas primitives used by the
// form-control painters produce visible output.

package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// newFormControlBox builds a RenderBox whose Node() is an element with the given tag
// and attributes, positioned at (0,0) with the given size. The element is attached to a
// fresh document so html5 wrappers that look up form ancestors work.
func newFormControlBox(tag string, attrs map[string]string, w, h float64) *RenderBox {
	doc := dom.NewDocument()
	el := doc.CreateElement(tag)
	for k, v := range attrs {
		el.SetAttribute(k, v)
	}
	st := style.NewComputedStyle()
	box := NewRenderBox(el, st)
	box.SetLocation(0, 0)
	box.SetSize(w, h)
	return box
}

// TestPaintFormControl_Checkbox verifies that a checked checkbox is handled by the form
// control painter (returns true) and produces non-transparent pixels (the checkmark).
func TestPaintFormControl_Checkbox(t *testing.T) {
	canvas := graphics.NewCanvas(20, 20)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 20, Height: 20})
	box := newFormControlBox("input", map[string]string{
		"type":    "checkbox",
		"checked": "checked",
	}, 16, 16)

	handled := PaintFormControl(box, info)
	if !handled {
		t.Fatal("PaintFormControl(input[type=checkbox]) = false, want true")
	}
	// The checkbox border should produce non-transparent pixels near the edge.
	if canvas.PixelAt(0, 0).A == 0 && canvas.PixelAt(8, 8).A == 0 {
		t.Fatal("checkbox painted no visible pixels")
	}
}

// TestPaintFormControl_Radio verifies that a checked radio button is handled and
// produces a visible dot in the center.
func TestPaintFormControl_Radio(t *testing.T) {
	canvas := graphics.NewCanvas(20, 20)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 20, Height: 20})
	box := newFormControlBox("input", map[string]string{
		"type":    "radio",
		"checked": "checked",
	}, 16, 16)

	handled := PaintFormControl(box, info)
	if !handled {
		t.Fatal("PaintFormControl(input[type=radio]) = false, want true")
	}
	// The center pixel should be non-transparent (the dot or border).
	if canvas.PixelAt(8, 8).A == 0 {
		t.Fatal("radio painted no visible pixels at center")
	}
}

// TestPaintFormControl_Range verifies that a range slider is handled and paints a track.
func TestPaintFormControl_Range(t *testing.T) {
	canvas := graphics.NewCanvas(120, 20)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 120, Height: 20})
	box := newFormControlBox("input", map[string]string{
		"type": "range",
		"min":  "0",
		"max":  "100",
	}, 120, 16)

	handled := PaintFormControl(box, info)
	if !handled {
		t.Fatal("PaintFormControl(input[type=range]) = false, want true")
	}
	// The track should produce non-transparent pixels in the middle band.
	if canvas.PixelAt(60, 10).A == 0 {
		t.Fatal("range slider painted no visible track pixels")
	}
}

// TestPaintFormControl_Color verifies that a color input paints a colored swatch.
func TestPaintFormControl_Color(t *testing.T) {
	canvas := graphics.NewCanvas(40, 30)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 40, Height: 30})
	box := newFormControlBox("input", map[string]string{
		"type":  "color",
		"value": "#ff0000",
	}, 40, 30)

	handled := PaintFormControl(box, info)
	if !handled {
		t.Fatal("PaintFormControl(input[type=color]) = false, want true")
	}
	// The swatch should be red.
	got := canvas.PixelAt(20, 15)
	if got.R < 200 || got.A == 0 {
		t.Fatalf("color swatch pixel = %+v, want red", got)
	}
}

// TestPaintFormControl_Hidden verifies that hidden inputs are handled (return true) and
// paint nothing.
func TestPaintFormControl_Hidden(t *testing.T) {
	canvas := graphics.NewCanvas(20, 20)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 20, Height: 20})
	box := newFormControlBox("input", map[string]string{
		"type": "hidden",
	}, 16, 16)

	handled := PaintFormControl(box, info)
	if !handled {
		t.Fatal("PaintFormControl(input[type=hidden]) = false, want true")
	}
	// Nothing should be painted.
	if canvas.PixelAt(10, 10).A != 0 {
		t.Fatal("hidden input painted pixels, want transparent")
	}
}

// TestPaintFormControl_TextInput verifies that text inputs ARE handled by the form
// control painter (return true) and paint the value text.
func TestPaintFormControl_TextInput(t *testing.T) {
	canvas := graphics.NewCanvas(80, 20)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 80, Height: 20})
	box := newFormControlBox("input", map[string]string{
		"type":  "text",
		"value": "hello",
	}, 80, 16)

	handled := PaintFormControl(box, info)
	if !handled {
		t.Fatal("PaintFormControl(input[type=text]) = false, want true (value text painted)")
	}
	// The value text "hello" should produce visible pixels in the canvas.
	// Scan the content area for any non-transparent pixel.
	found := false
	for py := 0; py < 20 && !found; py++ {
		for px := 0; px < 80; px++ {
			if canvas.PixelAt(px, py).A != 0 {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("text input painted no visible pixels for value text")
	}
}

// TestPaintFormControl_Placeholder verifies that text inputs with no value but a
// placeholder show the placeholder text.
func TestPaintFormControl_Placeholder(t *testing.T) {
	canvas := graphics.NewCanvas(120, 20)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 120, Height: 20})
	box := newFormControlBox("input", map[string]string{
		"type":        "text",
		"placeholder": "Enter name...",
	}, 120, 16)

	handled := PaintFormControl(box, info)
	if !handled {
		t.Fatal("PaintFormControl(input[type=text] placeholder) = false, want true")
	}
	// The placeholder text should produce visible pixels.
	found := false
	for py := 0; py < 20 && !found; py++ {
		for px := 0; px < 120; px++ {
			if canvas.PixelAt(px, py).A != 0 {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("placeholder text painted no visible pixels")
	}
}

// TestPaintFormControl_Progress verifies that a progress bar is handled and paints a fill.
func TestPaintFormControl_Progress(t *testing.T) {
	canvas := graphics.NewCanvas(120, 20)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 120, Height: 20})
	box := newFormControlBox("progress", map[string]string{
		"value": "50",
		"max":   "100",
	}, 120, 16)

	handled := PaintFormControl(box, info)
	if !handled {
		t.Fatal("PaintFormControl(progress) = false, want true")
	}
	// The left half should have the fill color, the track on the right.
	left := canvas.PixelAt(20, 8)
	if left.A == 0 {
		t.Fatal("progress bar painted no visible fill pixels")
	}
}

// TestPaintFormControl_Meter verifies that a meter is handled and paints a fill.
func TestPaintFormControl_Meter(t *testing.T) {
	canvas := graphics.NewCanvas(80, 20)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 80, Height: 20})
	box := newFormControlBox("meter", map[string]string{
		"value":   "0.7",
		"min":     "0",
		"max":     "1",
		"optimum": "1",
	}, 80, 16)

	handled := PaintFormControl(box, info)
	if !handled {
		t.Fatal("PaintFormControl(meter) = false, want true")
	}
	if canvas.PixelAt(20, 8).A == 0 {
		t.Fatal("meter painted no visible fill pixels")
	}
}

// TestPaintFormControl_Select verifies that select returns true (fully handled —
// no child text path) and still paints the arrow indicator.
func TestPaintFormControl_Select(t *testing.T) {
	canvas := graphics.NewCanvas(120, 20)
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 120, Height: 20})
	box := newFormControlBox("select", nil, 120, 16)

	handled := PaintFormControl(box, info)
	if !handled {
		t.Fatal("PaintFormControl(select) = false, want true (select is a replaced element)")
	}
	// The chevron is the Chromium kHTMLSelectArrow V-stroke (Edge pixel-fitted:
	// pts (cx-3,cy-1)→(cx,cy+2)→(cx+3,cy-1), stroke 2 round). box w=120 h=16 →
	// cx=110.5, cy=8. V spans y7..10, arms at x107.5/113.5. Check the arm and
	// tip regions have non-transparent pixels (x 106..115, y 4..12).
	visible := false
	for yy := 4; yy <= 12 && !visible; yy++ {
		for xx := 106; xx <= 115 && !visible; xx++ {
			if canvas.PixelAt(xx, yy).A > 0 {
				visible = true
			}
		}
	}
	if !visible {
		t.Fatal("select arrow painted no visible pixels")
	}
}

// TestPaintFormControl_NilBox verifies that nil box/info don't panic.
func TestPaintFormControl_NilBox(t *testing.T) {
	if PaintFormControl(nil, &PaintInfo{}) {
		t.Fatal("PaintFormControl(nil, info) = true, want false")
	}
}

// TestParseHexColor verifies hex color parsing for #rgb, #rrggbb, #rrggbbaa formats.
func TestParseHexColor(t *testing.T) {
	tests := []struct {
		in   string
		want graphics.Color
	}{
		{"#f00", graphics.Color{R: 255, G: 0, B: 0, A: 255}},
		{"#ff0000", graphics.Color{R: 255, G: 0, B: 0, A: 255}},
		{"#ff000080", graphics.Color{R: 255, G: 0, B: 0, A: 128}},
		{"fff", graphics.Color{R: 255, G: 255, B: 255, A: 255}},
		{"", graphics.Color{R: 0, G: 0, B: 0, A: 255}},
		{"#zzz", graphics.Color{R: 0, G: 0, B: 0, A: 255}},
	}
	for _, tt := range tests {
		got := parseHexColor(tt.in)
		if got != tt.want {
			t.Errorf("parseHexColor(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

// TestCanvasFillCircle verifies the new FillCircle canvas primitive produces visible output.
func TestCanvasFillCircle(t *testing.T) {
	canvas := graphics.NewCanvas(20, 20)
	red := graphics.Color{R: 255, G: 0, B: 0, A: 255}
	canvas.FillCircle(10, 10, 5, red)
	if canvas.PixelAt(10, 10) != red {
		t.Fatalf("center pixel = %+v, want %+v", canvas.PixelAt(10, 10), red)
	}
}

// TestCanvasStrokeLine verifies the new StrokeLine canvas primitive produces visible output.
func TestCanvasStrokeLine(t *testing.T) {
	canvas := graphics.NewCanvas(20, 20)
	red := graphics.Color{R: 255, G: 0, B: 0, A: 255}
	canvas.StrokeLine(2, 10, 18, 10, 2, red)
	if canvas.PixelAt(10, 10) != red {
		t.Fatalf("line pixel = %+v, want %+v", canvas.PixelAt(10, 10), red)
	}
}

// TestCanvasFillTriangle verifies the new FillTriangle canvas primitive produces visible output.
func TestCanvasFillTriangle(t *testing.T) {
	canvas := graphics.NewCanvas(20, 20)
	red := graphics.Color{R: 255, G: 0, B: 0, A: 255}
	canvas.FillTriangle(10, 2, 18, 18, 2, 18, red)
	if canvas.PixelAt(10, 12) != red {
		t.Fatalf("triangle pixel = %+v, want %+v", canvas.PixelAt(10, 12), red)
	}
}
