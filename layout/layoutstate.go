// Translation of: Source/WebCore/layout/LayoutState.h
//                  Source/WebCore/layout/LayoutState.cpp
// Completeness: 55%
// Simplifications:
//   - no subpixel layout (integer pixels only)
//   - no pagination/fragmentation
//   - WebKit's LayoutState owns a BoxGeometry cache keyed by layout box; this port
//     stores geometry directly on LayoutBox.Rect and uses LayoutState only for the
//     viewport size, the quirks-mode flag and the float tracker for the current BFC
//   - no per-formatting-context state objects (BlockFormattingState, etc.); each
//     formatting context keeps its transient state as local variables during Layout

package layout

// LayoutState is the Go translation of WebCore::Layout::LayoutState. It carries the
// viewport dimensions, the document's quirks-mode flag and the active float context
// for the current block formatting context. Layout geometry is stored on each
// LayoutBox directly (LayoutBox.Rect) rather than in a side geometry cache.
type LayoutState struct {
	// ViewportWidth / ViewportHeight are the dimensions of the initial containing
	// block (the viewport) in CSS pixels.
	ViewportWidth  float64
	ViewportHeight float64

	// QuirksMode mirrors LayoutState::inQuirksMode(). Quirks mode affects the
	// percentage-height resolution and the body-stretches-to-viewport quirk.
	QuirksMode bool

	// floats tracks the placed floats for the currently-active block formatting
	// context so that subsequent in-flow content can be shortened to avoid overlaps.
	floats *floatContext
}

// NewLayoutState constructs a LayoutState with the given viewport size and standards
// mode (no quirks).
func NewLayoutState(vw, vh float64) *LayoutState {
	return &LayoutState{ViewportWidth: vw, ViewportHeight: vh}
}

// inQuirksMode reports whether the document is in quirks mode.
func (s *LayoutState) inQuirksMode() bool { return s.QuirksMode }

// inStandardsMode reports whether the document is in standards mode (no quirks).
func (s *LayoutState) inStandardsMode() bool { return !s.QuirksMode }

// setFloatContext installs fc as the active float context for the duration of the
// caller's block formatting context. The previous context is returned so the caller
// can restore it when laying out nested BFCs.
func (s *LayoutState) setFloatContext(fc *floatContext) *floatContext {
	prev := s.floats
	s.floats = fc
	return prev
}

// restoreFloatContext reinstalls prev as the active float context.
func (s *LayoutState) restoreFloatContext(prev *floatContext) {
	s.floats = prev
}

// currentFloatContext returns the active float context, or nil when none.
func (s *LayoutState) currentFloatContext() *floatContext { return s.floats }
