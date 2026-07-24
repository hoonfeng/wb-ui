// Translation of: Source/WebCore/layout/LayoutState.h
//                  Source/WebCore/layout/LayoutState.cpp
// Completeness: 80%
//
// LayoutState now stores per-box geometry in a map, mirroring WebKit's
// LayoutState-owned geometry cache. Formatting contexts access geometry
// via state.GeometryForBox(box).

package layout

// LayoutState carries viewport dimensions, quirks-mode flag, float tracking,
// cross-axis constraint propagation, and a map of per-box geometry.
type LayoutState struct {
	ViewportWidth  float64
	ViewportHeight float64
	QuirksMode     bool

	floats *floatContext

	// Constraint propagation for flex/grid cross-axis.
	CrossSizeDefinite bool
	CrossSizeFallback float64
	MainSizeDefinite  bool
	MainSizeFallback  float64

	// Saved values for push/pop.
	savedCrossDefinite bool
	savedCrossFallback float64
	savedMainDefinite  bool
	savedMainFallback  float64

	// Per-box geometry cache (mirrors WebKit's BoxGeometry storage).
	geometry map[Box]*BoxGeometry
}

func NewLayoutState(vw, vh float64) *LayoutState {
	currentViewportWidth = vw
	currentViewportHeight = vh
	return &LayoutState{
		ViewportWidth:  vw,
		ViewportHeight: vh,
		geometry:       make(map[Box]*BoxGeometry),
	}
}

func (s *LayoutState) inQuirksMode() bool  { return s.QuirksMode }
func (s *LayoutState) inStandardsMode() bool { return !s.QuirksMode }

// GeometryForBox returns the BoxGeometry for the given box, creating a new
// zero-value one if none exists yet (mirroring WebKit's on-demand creation).
func (s *LayoutState) GeometryForBox(box Box) *BoxGeometry {
	if s.geometry == nil {
		s.geometry = make(map[Box]*BoxGeometry)
	}
	g, ok := s.geometry[box]
	if !ok {
		g = &BoxGeometry{}
		s.geometry[box] = g
	}
	return g
}
// currentFloatContext returns the active float context, or nil.
func (s *LayoutState) currentFloatContext() *floatContext { return s.floats }
// SetBoxGeometry stores geometry for a box (used when restoring/importing).
func (s *LayoutState) SetBoxGeometry(box Box, g *BoxGeometry) {
	if s.geometry == nil {
		s.geometry = make(map[Box]*BoxGeometry)
	}
	s.geometry[box] = g
}

func (s *LayoutState) setFloatContext(fc *floatContext) *floatContext {
	prev := s.floats
	s.floats = fc
	return prev
}

func (s *LayoutState) restoreFloatContext(prev *floatContext) {
	s.floats = prev
}

// Push saves the current constraint state for nested layout.
func (s *LayoutState) Push() {
	s.savedCrossDefinite = s.CrossSizeDefinite
	s.savedCrossFallback = s.CrossSizeFallback
	s.savedMainDefinite = s.MainSizeDefinite
	s.savedMainFallback = s.MainSizeFallback
}

// Pop restores constraint state saved by Push.
func (s *LayoutState) Pop() {
	s.CrossSizeDefinite = s.savedCrossDefinite
	s.CrossSizeFallback = s.savedCrossFallback
	s.MainSizeDefinite = s.savedMainDefinite
	s.MainSizeFallback = s.savedMainFallback
}
