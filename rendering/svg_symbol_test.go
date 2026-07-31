package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
)

// TestSVGSymbol: a <symbol> is not rendered directly; <use href="#sym">
// renders the symbol's child shapes at the use position.
func TestSVGSymbol(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("width", "100")
	svgEl.SetAttribute("height", "60")

	sym := doc.CreateElement("symbol")
	sym.SetAttribute("id", "star")
	r1 := doc.CreateElement("rect")
	r1.SetAttribute("x", "0")
	r1.SetAttribute("y", "0")
	r1.SetAttribute("width", "20")
	r1.SetAttribute("height", "20")
	r1.SetAttribute("fill", "red")
	r2 := doc.CreateElement("rect")
	r2.SetAttribute("x", "25")
	r2.SetAttribute("y", "5")
	r2.SetAttribute("width", "10")
	r2.SetAttribute("height", "10")
	r2.SetAttribute("fill", "blue")
	sym.AppendChild(r1)
	sym.AppendChild(r2)
	svgEl.AppendChild(sym)

	use := doc.CreateElement("use")
	use.SetAttribute("href", "#star")
	use.SetAttribute("x", "10")
	use.SetAttribute("y", "10")
	svgEl.AppendChild(use)

	sd := buildSVGDocument(svgEl)
	// Symbol children rendered once via the use: 2 shapes.
	if sd == nil || len(sd.shapes) != 2 {
		t.Fatalf("shapes=%v, want 2 (symbol children via use)", len(sd.shapes))
	}
	canvas := graphics.NewCanvas(100, 60)
	defer canvas.Release()
	paintSVG(canvas, sd, 0, 0, graphics.Color{})

	// Red rect at use(10,10)+offset(0,0) → (10,10)-(30,30).
	if px := canvas.PixelAt(20, 20); px.R != 255 || px.G != 0 {
		t.Fatalf("symbol red rect (20,20) = %+v, want red", px)
	}
	// Blue rect at (10+25, 10+5) → (35,15)-(45,25).
	if px := canvas.PixelAt(40, 20); px.B != 255 || px.R != 0 {
		t.Fatalf("symbol blue rect (40,20) = %+v, want blue", px)
	}
}
