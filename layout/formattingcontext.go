// Translation of: Source/WebCore/layout/formattingContexts/FormattingContext.h
//
// A FormattingContext lays out the in-flow children of its root ElementBox.
// Geometry is written via state.GeometryForBox().

package layout

import "wb-ui/style"

// FormattingContext is the Go translation of WebCore::Layout::FormattingContext.
type FormattingContext interface {
	Layout(box *ElementBox, state *LayoutState)
}

// contextFor returns the formatting context for an ElementBox based on its style.
func contextFor(box Box) FormattingContext {
	switch {
	case box.IsTextRun():
		return &InlineFormattingContext{}
	case box.EstablishesFlexFormattingContext():
		return &FlexFormattingContext{}
	case box.EstablishesGridFormattingContext():
		return &GridFormattingContext{}
	case box.EstablishesTableFormattingContext():
		return &TableFormattingContext{}
	default:
		if HasColumns(box) {
			return &MultiColumnFormattingContext{}
		}
		return &BlockFormattingContext{}
	}
}

// layoutInFlowChildren lays out all in-flow visible children of box.
func layoutInFlowChildren(box *ElementBox, state *LayoutState) {
	for _, child := range box.Children() {
		if !child.IsInFlow() || !child.IsVisible() {
			continue
		}
		if eb, ok := child.(*ElementBox); ok {
			ctx := contextFor(eb)
			ctx.Layout(eb, state)
		}
	}
}

var _ = style.DisplayFlex
