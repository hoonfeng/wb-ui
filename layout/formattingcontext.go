// Translation of: Source/WebCore/layout/formattingContexts/FormattingContext.h
//
// FormattingContext — abstract base for all formatting contexts.

package layout

// FormattingContext is the Go translation of WebCore::Layout::FormattingContext.
type FormattingContext interface {
	Layout(box *ElementBox, state *LayoutState)
	Root() *ElementBox
	LayoutState() *LayoutState
	FormattingGeometry() *FormattingGeometry
	FormattingQuirks() *FormattingQuirks
}

// FormattingContextBase provides shared fields/methods for all formatting contexts.
type FormattingContextBase struct {
	root        *ElementBox
	layoutState *LayoutState
	geometry    *FormattingGeometry
	quirks      *FormattingQuirks
}

// InitBase initialises the base fields. Called by each formatting context.
func (b *FormattingContextBase) InitBase(root *ElementBox, state *LayoutState) {
	b.root = root
	b.layoutState = state
}

func (b *FormattingContextBase) Root() *ElementBox                     { return b.root }
func (b *FormattingContextBase) LayoutState() *LayoutState              { return b.layoutState }
func (b *FormattingContextBase) FormattingGeometry() *FormattingGeometry { return b.geometry }
func (b *FormattingContextBase) FormattingQuirks() *FormattingQuirks     { return b.quirks }

// SetGeometry installs the formatting-context-specific geometry helper.
func (b *FormattingContextBase) SetGeometry(g *FormattingGeometry) { b.geometry = g }

// SetQuirks installs the formatting-context-specific quirks helper.
func (b *FormattingContextBase) SetQuirks(q *FormattingQuirks) { b.quirks = q }

// ── contextFor dispatches ──

// contextFor returns the formatting context for an ElementBox based on its style.
func contextFor(box Box, state *LayoutState) FormattingContext {
	eb, ok := box.(*ElementBox)
	if !ok || eb == nil {
		return nil
	}
	var ctx FormattingContext
	switch {
	case box.IsTextRun():
		ctx = &InlineFormattingContext{}
	case box.EstablishesFlexFormattingContext():
		ctx = &FlexFormattingContext{}
	case box.EstablishesGridFormattingContext():
		ctx = &GridFormattingContext{}
	case box.EstablishesTableFormattingContext():
		ctx = &TableFormattingContext{}
	default:
		if HasColumns(eb) {
			ctx = &MultiColumnFormattingContext{}
		} else if hasInlineChildren(eb) {
			ctx = &InlineFormattingContext{}
		} else {
			ctx = &BlockFormattingContext{}
		}
	}
	if ctx != nil {
		if base, ok := ctx.(interface{ InitBase(*ElementBox, *LayoutState) }); ok {
			base.InitBase(eb, state)
		}
	}
	return ctx
}

// layoutInFlowChildren lays out all in-flow visible children of box.
func layoutInFlowChildren(box *ElementBox, state *LayoutState) {
	for _, child := range box.Children() {
		if !child.IsInFlow() || !child.IsVisible() {
			continue
		}
		if eb, ok := child.(*ElementBox); ok {
			ctx := contextFor(eb, state)
			if ctx != nil {
				ctx.Layout(eb, state)
			}
		}
	}
}
