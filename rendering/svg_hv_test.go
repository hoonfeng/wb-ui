package rendering

import (
	"testing"

	"wb-ui/dom"
)

// TestSVGPathHVCmds: a path using H/h (horizontal) and V/v (vertical) line
// commands must produce the full outline. Regression: the folder icon in the
// desktop app drew only stray arcs because H/V were ignored, breaking the
// path (icon body missing).
func TestSVGPathHVCmds(t *testing.T) {
	doc := dom.NewDocument()
	svgEl := doc.CreateElement("svg")
	svgEl.SetAttribute("viewBox", "0 0 24 24")
	pathEl := doc.CreateElement("path")
	// Folder outline: M22 19 H4 V5 h5 l2 3 h9 z — the H/V/h commands
	// were previously skipped, so only M/l/z survived.
	pathEl.SetAttribute("d", "M22 19H4V5h5l2 3h9z")
	svgEl.AppendChild(pathEl)

	sdoc := buildSVGDocument(svgEl)
	if sdoc == nil || len(sdoc.shapes) == 0 {
		t.Fatal("no shapes parsed")
	}
	fs, ok := sdoc.shapes[0].(*svgFilledShape)
	if !ok {
		t.Fatalf("shape type %T, want *svgFilledShape", sdoc.shapes[0])
	}
	sp, ok := fs.shape.(*svgPath)
	if !ok {
		t.Fatalf("wrapped shape type %T, want *svgPath", fs.shape)
	}
	// The path has 7 commands: M22 19, H4, V5, h5, l2 3, h9, z.
	if len(sp.commands) != 7 {
		t.Fatalf("commands=%d, want 7 (H/V/h must be parsed)", len(sp.commands))
	}
	kinds := map[byte]bool{}
	for _, c := range sp.commands {
		kinds[c.kind] = true
	}
	for _, want := range []byte{'H', 'V', 'h', 'l', 'z', 'M'} {
		if !kinds[want] {
			t.Fatalf("missing path command %q in %v", string(want), kinds)
		}
	}
}
