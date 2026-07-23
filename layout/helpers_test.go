package layout

import (
	"testing"

	"wb-ui/style"
)

// mkBlock returns a block-level box with a default ComputedStyle (display: block).
func mkBlock() *ElementBox {
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayBlock
	return &ElementBox{nodeType: NodeGenericElement, style: cs}
}

// mkColumnBlock returns a block box with multi-column layout (column-count: N).
func mkColumnBlock(colCount int) *ElementBox {
	b := mkBlock()
	b.style.ColumnCount = colCount
	b.style.ColumnGap = style.Length{Value: 16, Unit: "px"}
	return b
}

// mkVerticalBlock returns a block box with vertical writing-mode.
func mkVerticalBlock(wm string) *ElementBox {
	b := mkBlock()
	b.style.WritingMode = wm
	return b
}

// mkBlockWH returns a block box with the given width and height in px.
func mkBlockWH(w, h float64) *ElementBox {
	b := mkBlock()
	if w > 0 {
		b.style.Width = style.Length{Value: w, Unit: "px"}
	}
	if h > 0 {
		b.style.Height = style.Length{Value: h, Unit: "px"}
	}
	return b
}

// mkFlex returns a flex container box.
func mkFlex() *ElementBox {
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayFlex
	return &ElementBox{nodeType: NodeGenericElement, style: cs}
}

// mkGrid returns a grid container box.
func mkGrid() *ElementBox {
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayGrid
	return &ElementBox{nodeType: NodeGenericElement, style: cs}
}

// mkTable returns a table box.
func mkTable() *ElementBox {
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayTable
	return &ElementBox{nodeType: NodeTableBox, style: cs}
}

// mkTableRow returns a table-row box.
func mkTableRow() *ElementBox {
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayTableRow
	return &ElementBox{nodeType: NodeTableWrapperBox, style: cs}
}

// mkTableCell returns a table-cell box.
func mkTableCell() *ElementBox {
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayTableCell
	return &ElementBox{nodeType: NodeTableWrapperBox, style: cs}
}

// mkAnon returns an anonymous block wrapper (for inline content).
func mkAnon() *ElementBox {
	cs := style.NewComputedStyle()
	cs.Display = style.DisplayBlock
	return &ElementBox{nodeType: NodeGenericElement, style: cs}
}

// mkTextRun returns a text-run box holding the given text.
func mkTextRun(text string) *ElementBox {
	cs := style.NewComputedStyle()
	return &ElementBox{nodeType: NodeText, style: cs}
}

// setMargin sets the four margin sides of box to v px.
func setMargin(box *ElementBox, v float64) {
	box.style.MarginTop = style.Length{Value: v, Unit: "px"}
	box.style.MarginRight = style.Length{Value: v, Unit: "px"}
	box.style.MarginBottom = style.Length{Value: v, Unit: "px"}
	box.style.MarginLeft = style.Length{Value: v, Unit: "px"}
}

// setProp sets a raw string property on box.Style (for properties without typed fields).
func setProp(box *ElementBox, name, value string) {
	box.style.Properties[name] = value
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

// rectOf returns the border-box geometry of a box for use in test assertions.
func rectOf(box *ElementBox, state *LayoutState) (x, y, w, h float64) {
	g := state.GeometryForBox(box)
	return g.Left(), g.Top(), g.BorderBoxWidth(), g.BorderBoxHeight()
}
