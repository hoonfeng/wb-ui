package rendering

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/html5"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/style"
)

// TestFileInputPaint: a file input paints a button block on the right side —
// a light gray rounded rect distinct from the input background — plus the
// filename label text on the left.
func TestFileInputPaint(t *testing.T) {
	canvas := graphics.NewCanvas(260, 40)
	defer canvas.Release()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 260, Height: 40})
	doc := dom.NewDocument()
	el := doc.CreateElement("input")
	el.SetAttribute("type", "file")
	el.SetAttribute("value", "C:\\reports\\summary.pdf")
	doc.AppendChild(el)
	st := style.NewComputedStyle()
	st.BackgroundColor = style.Color{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	st.FontSize = style.Length{Value: 14, Unit: "px"}

	box := NewRenderBox(el, st)
	box.SetLocation(0, 0)
	box.SetSize(260, 40)

	// Paint the input background (white) then the form-control content.
	paintObjectBackground(box, info)
	ret := PaintFormControl(box, info)
	t.Logf("PaintFormControl returned %v", ret)
	t.Logf("(200,20)=%+v", canvas.PixelAt(200, 20))

	// Button area is around x = 260-96-2=162..258, y=2..38: light gray fill
	// (0xE8). The label text occupies the center, so scan for the fill color
	// anywhere in the button region.
	btn := canvas.PixelAt(200, 20)
	t.Logf("(200,20)=%+v (200,35)=%+v", btn, canvas.PixelAt(200, 35))
	btnOK := false
	for x := 165; x <= 255; x++ {
		for y := 4; y <= 36; y++ {
			if p := canvas.PixelAt(x, y); p.R > 220 && p.G > 220 && p.B > 220 {
				btnOK = true
			}
		}
	}
	if !btnOK {
		t.Fatalf("no light-gray button fill found in button region")
	}
	// Gap between filename and button (x≈155) is the white input background.
	gap := canvas.PixelAt(155, 20)
	if gap.R < 250 || gap.B < 250 {
		t.Fatalf("gap area (155,20) = %+v, want white input background", gap)
	}
	// Some ink (filename text) must be present left of the button.
	ink := false
	for x := 5; x < 150; x++ {
		for y := 10; y < 30; y++ {
			if p := canvas.PixelAt(x, y); p.A > 60 && p.R < 200 {
				ink = true
			}
		}
	}
	if !ink {
		t.Fatalf("no filename text ink on the left side")
	}
}

// TestInputFileTypeMapping: html5 recognizes type=file.
func TestInputFileTypeMapping(t *testing.T) {
	doc := dom.NewDocument()
	el := doc.CreateElement("input")
	el.SetAttribute("type", "file")
	in, ok := html5.ToInputElement(el)
	if !ok {
		t.Fatal("ToInputElement failed")
	}
	if in.Type() != html5.InputFile {
		t.Fatalf("type=%q, want InputFile", in.Type())
	}
}
