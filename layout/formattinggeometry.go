// Translation of: Source/WebCore/layout/formattingContexts/FormattingGeometry.h
// FormattingGeometry provides border/padding/margin/sizing computations.
// Used by all formatting context types.

package layout

// FormattingGeometry is the Go translation of WebCore::Layout::FormattingGeometry.
// It provides geometry computation helpers shared across formatting contexts.
type FormattingGeometry struct {
	root *ElementBox
	ls   *LayoutState
}

// NewFormattingGeometry creates a FormattingGeometry for the given context root.
func NewFormattingGeometry(root *ElementBox, ls *LayoutState) *FormattingGeometry {
	return &FormattingGeometry{root: root, ls: ls}
}

// ComputedBorder returns the border Edges for box based on its style.
func (fg *FormattingGeometry) ComputedBorder(box *ElementBox) Edges {
	cs := box.Style()
	if cs == nil {
		return Edges{}
	}
	return Edges{
		Top:    usedBorderWidth(cs.BorderTopWidth, cs.BorderTopStyle),
		Right:  usedBorderWidth(cs.BorderRightWidth, cs.BorderRightStyle),
		Bottom: usedBorderWidth(cs.BorderBottomWidth, cs.BorderBottomStyle),
		Left:   usedBorderWidth(cs.BorderLeftWidth, cs.BorderLeftStyle),
	}
}

// ComputedPadding returns the padding Edges for box based on its style and CB width.
func (fg *FormattingGeometry) ComputedPadding(box *ElementBox, cbContentWidth float64) Edges {
	cs := box.Style()
	if cs == nil {
		return Edges{}
	}
	fs := fontSizeOf(box)
	return Edges{
		Top:    maxF(0, resolveOrZero(cs.PaddingTop, cbContentWidth, fs)),
		Right:  maxF(0, resolveOrZero(cs.PaddingRight, cbContentWidth, fs)),
		Bottom: maxF(0, resolveOrZero(cs.PaddingBottom, cbContentWidth, fs)),
		Left:   maxF(0, resolveOrZero(cs.PaddingLeft, cbContentWidth, fs)),
	}
}

// ComputedMargin returns the margin Edges for box.
func (fg *FormattingGeometry) ComputedMargin(box *ElementBox, cbContentWidth float64) Edges {
	cs := box.Style()
	if cs == nil {
		return Edges{}
	}
	fs := fontSizeOf(box)
	return Edges{
		Top:    resolveOrZero(cs.MarginTop, cbContentWidth, fs),
		Right:  resolveOrZero(cs.MarginRight, cbContentWidth, fs),
		Bottom: resolveOrZero(cs.MarginBottom, cbContentWidth, fs),
		Left:   resolveOrZero(cs.MarginLeft, cbContentWidth, fs),
	}
}

// ComputeBorderPaddingMargin returns all three Edges at once (optimisation).
func (fg *FormattingGeometry) ComputeBorderPaddingMargin(box *ElementBox, cbContentWidth float64) (border, padding, margin Edges) {
	border = fg.ComputedBorder(box)
	padding = fg.ComputedPadding(box, cbContentWidth)
	margin = fg.ComputedMargin(box, cbContentWidth)
	return
}

// ComputedWidth returns the resolved content-width for box, or false if auto.
func (fg *FormattingGeometry) ComputedWidth(box *ElementBox, cbContentWidth float64) (float64, bool) {
	cs := box.Style()
	if cs == nil {
		return 0, false
	}
	return definiteWidth(cs.Width, cbContentWidth, fontSizeOf(box))
}

// ComputedHeight returns the resolved content-height for box, or false if auto.
func (fg *FormattingGeometry) ComputedHeight(box *ElementBox, cbHeight float64) (float64, bool) {
	cs := box.Style()
	if cs == nil {
		return 0, false
	}
	return definiteHeight(cs.Height, cbHeight, fontSizeOf(box))
}

// ClampByMinMaxWidth returns width clamped by min/max-width.
func (fg *FormattingGeometry) ClampByMinMaxWidth(box *ElementBox, w, cbWidth float64) float64 {
	minW, maxW, _, _ := resolveMinMax(box.Style().MinWidth, box.Style().MaxWidth, cbWidth, fontSizeOf(box))
	return clampSize(w, minW, maxW, false, false)
}

// ClampByMinMaxHeight returns height clamped by min/max-height.
func (fg *FormattingGeometry) ClampByMinMaxHeight(box *ElementBox, h, cbHeight float64) float64 {
	minH, maxH, _, _ := resolveMinMax(box.Style().MinHeight, box.Style().MaxHeight, cbHeight, fontSizeOf(box))
	return clampSize(h, minH, maxH, false, false)
}

// ContentWidthForWidth resolves the content width for box given its containing
// block width. Returns (contentWidth, isDefinite).
func (fg *FormattingGeometry) ContentWidthForWidth(box *ElementBox, cbContentWidth float64, border, margin Edges) (float64, bool) {
	cs := box.Style()
	if cs == nil {
		return 0, false
	}
	fs := fontSizeOf(box)
	if w, ok := definiteWidth(cs.Width, cbContentWidth, fs); ok {
		padding := fg.ComputedPadding(box, cbContentWidth)
		if isBorderBox(box) {
			w -= border.Horizontal() + padding.Horizontal()
		}
		minW, maxW, _, _ := resolveMinMax(cs.MinWidth, cs.MaxWidth, cbContentWidth, fs)
		return clampSize(w, minW, maxW, false, false), true
	}
	avail := cbContentWidth - margin.Horizontal() - border.Horizontal()
	if avail < 0 {
		avail = 0
	}
	minW, maxW, _, _ := resolveMinMax(cs.MinWidth, cs.MaxWidth, cbContentWidth, fs)
	return clampSize(avail, minW, maxW, false, false), false
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
