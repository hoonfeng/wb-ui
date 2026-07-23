// Translation of: Source/WebCore/layout/LayoutState.h
//                  Source/WebCore/layout/LayoutState.cpp
// Completeness: 80%
// Simplifications:
//   - no subpixel layout (integer pixels only)
//   - no pagination/fragmentation
//   - WebKit's LayoutState owns a BoxGeometry cache keyed by layout box; this port
//     stores geometry directly on LayoutBox.Rect and uses LayoutState only for the
//     viewport size, the quirks-mode flag and the float tracker for the current BFC
//   - no per-formatting-context state objects (BlockFormattingState, etc.); each
//     formatting context keeps its transient state as local variables during Layout
//   - layout cache/dirty tracking on LayoutBox replaces the per-box geometry cache

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

	// ─── 约束传播（Constraint Propagation） ──────────────────────────
	// These fields communicate parent-to-child sizing information through the
	// formatting context chain. A parent that knows its content size (e.g. a
	// fixed-sized grid cell) sets Definite=true. A parent with auto/unknown size
	// (e.g. an auto-height grid row) sets Definite=false and provides a Fallback
	// value so children can run a provisional measurement pass.
	//
	// Flex: reads CrossSizeDefinite to decide whether to apply stretch.
	// Grid: sets CrossSizeDefinite=false for auto-height rows.
	// Block: uses FallbackHeight for percentage-based child sizing.

	// CrossSizeDefinite indicates the current containing block has a definite
	// cross-axis size. Flex containers skip stretch when false.
	CrossSizeDefinite bool
	// CrossSizeFallback provides a provisional cross-axis size when the
	// actual size is indefinite. Child formatting contexts use this for
	// measurement but must NOT treat it as the final size.
	CrossSizeFallback float64

	// MainSizeDefinite / MainSizeFallback — same concept for the main axis.
	// Used by column flex containers whose container's main axis (height)
	// is indefinite.
	MainSizeDefinite bool
	MainSizeFallback float64

	// CrossAxisRelayout indicates the current layout call is a cross-axis
	// re-layout triggered by a parent flex/grid container's stretch.
	// When true, FFC.positionAndFinalize MUST skip the auto-height step (3d)
	// because the container's main-axis size was already set by flex-grow
	// heights (which is wrong when flex-grow should give the item more space).
	CrossAxisRelayout bool

	// saved stores previous values when Push/pop is used.
	// heights (which is wrong when flex-grow should give the item more space).

	// saved stores previous values when Push/pop is used.
	savedCrossDefinite bool
	savedCrossFallback float64
	savedMainDefinite  bool
	savedMainFallback  float64
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

// ─── 约束传播方法 ──────────────────────────────────────────

// PushCrossConstraint saves the current cross-axis constraint and installs
// the given definite/fallback values. Callers MUST defer PopCrossConstraint().
func (s *LayoutState) PushCrossConstraint(definite bool, fallback float64) {
	s.savedCrossDefinite = s.CrossSizeDefinite
	s.savedCrossFallback = s.CrossSizeFallback
	s.CrossSizeDefinite = definite
	s.CrossSizeFallback = fallback
}

// PopCrossConstraint restores the previous cross-axis constraint.
func (s *LayoutState) PopCrossConstraint() {
	s.CrossSizeDefinite = s.savedCrossDefinite
	s.CrossSizeFallback = s.savedCrossFallback
}

// PushMainConstraint saves and installs main-axis constraint values.
func (s *LayoutState) PushMainConstraint(definite bool, fallback float64) {
	s.savedMainDefinite = s.MainSizeDefinite
	s.savedMainFallback = s.MainSizeFallback
	s.MainSizeDefinite = definite
	s.MainSizeFallback = fallback
}

// PopMainConstraint restores the previous main-axis constraint.
func (s *LayoutState) PopMainConstraint() {
	s.MainSizeDefinite = s.savedMainDefinite
	s.MainSizeFallback = s.savedMainFallback
}
