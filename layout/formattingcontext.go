// Translation of: Source/WebCore/layout/formattingContexts/FormattingContext.h
//                  Source/WebCore/layout/formattingContexts/FormattingContext.cpp
// Completeness: 50%
// Simplifications:
//   - no subpixel layout (integer pixels only)
//   - no pagination/fragmentation
//   - the C++ FormattingContext base owns a reference to the formatting-context root
//     ElementBox and the LayoutState, and exposes geometry/escape-reason helpers; the
//     Go port models a formatting context as a small interface implemented by each
//     concrete context (Block / Inline / Flex / Grid / Table) and dispatches on the
//     box's display
//   - the IntrinsicWidthConstraints / usedContentHeight hooks are omitted; intrinsic
//     sizing is computed inline where needed

package layout

import "wb-ui/style"

// FormattingContext is the Go translation of WebCore::Layout::FormattingContext. Each
// concrete formatting context lays out a single box (the "formatting context root")
// and its in-flow descendants, writing the resulting geometry onto the LayoutBox.Rect
// fields.
type FormattingContext interface {
	// Layout lays out box and its descendants using state for viewport/quirks/float
	// information. The caller is responsible for positioning box (margin/padding/border
	// and border-box origin) before calling Layout for the root box; for descendant
	// boxes the formatting context computes positions itself.
	Layout(box *LayoutBox, state *LayoutState)
}

// contextFor returns the formatting context that should lay out box based on its
// display. Block / inline-block / list-item / flow-root use the block formatting
// context; flex containers use FlexFormattingContext; grid containers use
// GridFormattingContext; tables use TableLayout. Multi-column containers
// (column-count > 0 or column-width set) use MultiColumnFormattingContext.
func contextFor(box *LayoutBox) FormattingContext {
	// Anonymous boxes wrap inline-level content (produced by BuildLayoutTree when a
	// block container mixes inline and block children); they are laid out by the
	// inline formatting context.
	if box.Type == BoxAnonymous {
		return &InlineFormattingContext{}
	}
	if box.Style == nil {
		return &BlockFormattingContext{}
	}
	switch box.Style.Display {
	case style.DisplayFlex, style.DisplayInlineFlex:
		return &FlexFormattingContext{}
	case style.DisplayGrid, style.DisplayInlineGrid:
		return &GridFormattingContext{}
	case style.DisplayTable, style.DisplayInlineTable:
		return &TableFormattingContext{}
	default:
		if HasColumns(box) {
			return &MultiColumnFormattingContext{}
		}
		return &BlockFormattingContext{}
	}
}

// layoutInFlowChildren recursively lays out box's in-flow children. Each child is
// positioned by the formatting context that owns it. Out-of-flow (floated / absolute)
// children are handled by the parent formatting context separately.
func layoutInFlowChildren(box *LayoutBox, state *LayoutState) {
	for _, child := range box.Children {
		if !child.IsInFlow() || !child.IsVisible() {
			continue
		}
		ctx := contextFor(child)
		ctx.Layout(child, state)
	}
}

// style import is referenced here to keep the package alias resolvable for the
// switch above even when the file is compiled in isolation.
var _ = style.DisplayFlex
