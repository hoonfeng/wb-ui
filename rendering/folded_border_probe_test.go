package rendering

import (
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// TestFoldedSummaryBorderVarCascade is a regression test for the CSS custom
// property cascade bug: `border: 1px solid var(--border-color)` followed by
// `border-left: 3px solid var(--accent)` in the SAME rule (with a Vue scoped
// attribute selector) must resolve the left border to 3px accent blue.
//
// Root cause: resolveVarInProperties re-applied shorthand declarations by
// iterating a Go map (random order), so `border` could be re-applied AFTER
// `border-left`, silently reverting the left accent to the gray 1px border.
// Fixed by expanding var() in cascade declaration order before applying.
func TestFoldedSummaryBorderVarCascade(t *testing.T) {
	htmlSrc := `<html><head><style>
:root { --bg-primary: #0d1117; --border-color: #30363d; --accent: #58a6ff; }
body { margin: 0; font-family: sans-serif; font-size: 12px; }
.folded-summary[data-v-38389705] { display: flex; align-items: center; gap: 5px; padding: 5px 10px; background: var(--bg-primary); border: 1px solid var(--border-color); border-left: 3px solid var(--accent); border-radius: 6px; font-size: 12px; }
.folded-title[data-v-38389705] { color: #e6edf3; font-weight: 500; }
.folded-desc[data-v-38389705] { color: #6e7681; }
</style></head><body>
<div class="folded-summary" data-v-38389705><span class="folded-title" data-v-38389705>完成摘要</span><span class="folded-desc" data-v-38389705>1 步工具调用 · 「已经…</span></div>
</body></html>`

	doc, err := html.ParseDocument(htmlSrc)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	cssText := `
:root { --bg-primary: #0d1117; --border-color: #30363d; --accent: #58a6ff; }
body { margin: 0; font-family: sans-serif; font-size: 12px; }
.folded-summary[data-v-38389705] { display: flex; align-items: center; gap: 5px; padding: 5px 10px; background: var(--bg-primary); border: 1px solid var(--border-color); border-left: 3px solid var(--accent); border-radius: 6px; font-size: 12px; }
.folded-title[data-v-38389705] { color: #e6edf3; font-weight: 500; }
.folded-desc[data-v-38389705] { color: #6e7681; }
`
	css.NewParser(cssText).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	if rv == nil {
		t.Fatal("nil rv")
	}
	rv.SetViewportSize(600, 400)
	rv.Layout(nil)

	var sumRO RenderObject
	var findSum func(RenderObject)
	findSum = func(ro RenderObject) {
		if sumRO != nil {
			return
		}
		if el, ok := ro.Node().(*dom.Element); ok && el.GetClassName() == "folded-summary" {
			sumRO = ro
			return
		}
		for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
			findSum(c)
		}
	}
	findSum(rv)
	if sumRO == nil {
		t.Fatal("no .folded-summary render object")
	}
	rb := asRenderBox(sumRO)
	if rb == nil {
		t.Fatal("no RenderBox for summary")
	}
	st := rb.Style()
	if st == nil {
		t.Fatal("nil style")
	}
	// border-left must win over the `border` shorthand.
	if lw := lengthValue(st.BorderLeftWidth); lw != 3 {
		t.Errorf("BorderLeftWidth = %.1f, want 3 (border-left shorthand lost to `border`)", lw)
	}
	lc := st.BorderColor("left")
	want := style.Color{R: 0x58, G: 0xa6, B: 0xff, A: 0xff}
	if lc != want {
		t.Errorf("BorderLeftColor = %+v, want accent %+v", lc, want)
	}

	// Paint and sample the left edge: the 3px accent stripe must be visible.
	canvas := graphics.NewCanvas(600, 400)
	defer canvas.Release()
	info := NewPaintInfo(canvas, Rect{X: 0, Y: 0, Width: 600, Height: 400})
	PaintBackground(rb, info)
	PaintBorder(rb, info)
	x, y := rb.X(), rb.Y()
	h := rb.Height()
	sample := canvas.PixelAt(int(x)+1, int(y)+int(h)/2)
	if sample.R != 0x58 || sample.G != 0xa6 || sample.B != 0xff {
		t.Errorf("left-edge pixel = #%02x%02x%02x, want accent #58a6ff", sample.R, sample.G, sample.B)
	}
}
