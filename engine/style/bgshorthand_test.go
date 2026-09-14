package style

import (
	"testing"

	"wb-ui/engine/css"
)

// TestBackgroundShorthandResetsColor: "background: none" (no color) must reset
// background-color to transparent per CSS Backgrounds-3 shorthand semantics.
// Regression: the UA button face #f0f0f0 leaked through Vue buttons styled
// with `background: none` because the shorthand only cleared background-image.
func TestBackgroundShorthandResetsColor(t *testing.T) {
	cs := NewComputedStyle()
	// Simulate the UA sheet having set a background color first.
	cs.BackgroundColor = Color{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}
	cs.BackgroundImage = "linear-gradient(to bottom, #efefef 0%, #fff 28%)"

	p := css.NewParser("background: none")
	decls := p.ParseDeclarationList()
	for _, d := range decls {
		applyDeclaration(cs, d)
	}
	if cs.BackgroundColor.A != 0 {
		t.Fatalf("background: none should reset color to transparent, got %+v", cs.BackgroundColor)
	}
	if cs.BackgroundImage != "" {
		t.Fatalf("background: none should clear background-image, got %q", cs.BackgroundImage)
	}
}

// TestBackgroundShorthandKeepsColor: "background: #ff0000" sets color and
// clears the image.
func TestBackgroundShorthandKeepsColor(t *testing.T) {
	cs := NewComputedStyle()
	cs.BackgroundImage = "url(x.png)"
	p := css.NewParser("background: #ff0000")
	for _, d := range p.ParseDeclarationList() {
		applyDeclaration(cs, d)
	}
	if cs.BackgroundColor.R != 0xFF || cs.BackgroundColor.A != 0xFF {
		t.Fatalf("background: #ff0000 should set red, got %+v", cs.BackgroundColor)
	}
	if cs.BackgroundImage != "" {
		t.Fatalf("background shorthand must clear image, got %q", cs.BackgroundImage)
	}
}

// TestBackgroundShorthandGradient: "background: linear-gradient(...)" keeps
// the image and resets the color.
func TestBackgroundShorthandGradient(t *testing.T) {
	cs := NewComputedStyle()
	cs.BackgroundColor = Color{R: 1, G: 2, B: 3, A: 0xFF}
	p := css.NewParser("background: linear-gradient(to right, red, blue)")
	for _, d := range p.ParseDeclarationList() {
		applyDeclaration(cs, d)
	}
	if cs.BackgroundImage == "" {
		t.Fatal("gradient shorthand should keep background-image")
	}
	if cs.BackgroundColor.A != 0 {
		t.Fatalf("gradient shorthand should reset color to transparent, got %+v", cs.BackgroundColor)
	}
}
