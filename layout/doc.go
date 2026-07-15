// Package layout implements the layout engine (Source/WebCore/layout) in Go.
//
// Formatting contexts mirror WebCore/layout/integration/formattingContexts:
//   - BlockFormattingContext
//   - InlineFormattingContext
//   - FlexFormattingContext (based on RenderFlexibleBox)
//   - GridFormattingContext (based on RenderGrid)
//   - TableLayout (auto/fixed)
//   - Float + clearance
//   - Positioned layout (absolute/fixed/sticky/relative)
package layout
