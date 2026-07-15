package layout

import (
	"testing"

	"wb-ui/style"
)

// mkBlock returns a block-level box with a default ComputedStyle (display: block).
func mkBlock() *LayoutBox {
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayBlock
	return &LayoutBox{Type: BoxBlock, Style: cs}
}

// mkBlockWH returns a block box with the given width and height in px.
func mkBlockWH(w, h float64) *LayoutBox {
	b := mkBlock()
	if w > 0 {
		b.Style.Width = style.Length{Value: w, Unit: "px"}
	}
	if h > 0 {
		b.Style.Height = style.Length{Value: h, Unit: "px"}
	}
	return b
}

// mkFlex returns a flex container box.
func mkFlex() *LayoutBox {
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayFlex
	return &LayoutBox{Type: BoxBlock, Style: cs}
}

// mkGrid returns a grid container box.
func mkGrid() *LayoutBox {
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayGrid
	return &LayoutBox{Type: BoxBlock, Style: cs}
}

// mkTable returns a table box.
func mkTable() *LayoutBox {
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayTable
	return &LayoutBox{Type: BoxBlock, Style: cs}
}

// mkTableRow returns a table-row box.
func mkTableRow() *LayoutBox {
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayTableRow
	return &LayoutBox{Type: BoxBlock, Style: cs}
}

// mkTableCell returns a table-cell box.
func mkTableCell() *LayoutBox {
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayTableCell
	return &LayoutBox{Type: BoxBlock, Style: cs}
}

// mkAnon returns an anonymous block wrapper (for inline content).
func mkAnon() *LayoutBox {
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayBlock
	return &LayoutBox{Type: BoxAnonymous, Style: cs}
}

// mkTextRun returns a text-run box holding the given text.
func mkTextRun(text string) *LayoutBox {
	cs := style.NewComputedStyle()
	return &LayoutBox{Type: BoxTextRun, Text: text, Style: cs}
}

// setMargin sets the four margin sides of box to v px.
func setMargin(box *LayoutBox, v float64) {
	box.Style.MarginTop = style.Length{Value: v, Unit: "px"}
	box.Style.MarginRight = style.Length{Value: v, Unit: "px"}
	box.Style.MarginBottom = style.Length{Value: v, Unit: "px"}
	box.Style.MarginLeft = style.Length{Value: v, Unit: "px"}
}

// setProp sets a raw string property on box.Style (for properties without typed fields).
func setProp(box *LayoutBox, name, value string) {
	box.Style.Properties[name] = value
}

// approxEq reports whether a and b differ by less than 0.5 (sub-pixel tolerance).
func approxEq(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 0.5
}

// assertApprox checks that a ~= b and fails the test otherwise.
func assertApprox(t *testing.T, name string, got, want float64) {
	t.Helper()
	if !approxEq(got, want) {
		t.Errorf("%s = %g, want %g", name, got, want)
	}
}
