// Translation of: Source/WebCore/layout/formattingContexts/FormattingQuirks.h
// FormattingQuirks handles quirky layout behaviours (margin collapsing, etc.)

package layout

// FormattingQuirks is the Go translation of WebCore::Layout::FormattingQuirks.
// It handles edge-case layout behaviours.
type FormattingQuirks struct {
	root *ElementBox
	ls   *LayoutState
}

// NewFormattingQuirks creates a FormattingQuirks for the given context root.
func NewFormattingQuirks(root *ElementBox, ls *LayoutState) *FormattingQuirks {
	return &FormattingQuirks{root: root, ls: ls}
}

// MarginCollapsesWithParent reports whether the box's top margin can collapse
// with its parent's bottom margin. This occurs when there is no border/padding
// separating them, mirroring CSS 2.2 § 8.3.1.
func (fq *FormattingQuirks) MarginCollapsesWithParent(box *ElementBox, g *BoxGeometry) bool {
	// Top margin of first in-flow child can collapse with parent's top margin
	// when parent has no top border or padding.
	parent := box.Parent()
	if parent == nil {
		return false
	}
	pg := fq.ls.GeometryForBox(parent)
	return pg.BorderTop() == 0 && pg.PaddingTop() == 0
}

// MarginCollapsesWithNextSibling reports whether the box's bottom margin can
// collapse with its next sibling's top margin.
func (fq *FormattingQuirks) MarginCollapsesWithNextSibling(box *ElementBox, g *BoxGeometry) bool {
	return g.MarginEnd() >= 0 && g.BorderBottom() == 0 && g.PaddingBottom() == 0
}

// CollapsedMargin computes the collaped margin between two adjacent margins.
// Per CSS 2.2 § 8.3.1: the collapsed margin is the larger of the two, or if
// one is negative it is subtracted from the positive one.
func (fq *FormattingQuirks) CollapsedMargin(a, b float64) float64 {
	if a >= 0 && b >= 0 {
		if a > b {
			return a
		}
		return b
	}
	if a <= 0 && b <= 0 {
		if a < b {
			return a
		}
		return b
	}
	// One positive, one negative.
	return a + b
}
