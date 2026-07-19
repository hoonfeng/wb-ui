// ChainedWatchpoint / Watchpoint (JIT optimization types)
package bytecode

type ChainedWatchpoint struct{}
type Watchpoint struct {
	state uint8 // IsWatched / IsInvalidated
}
const (
	WatchpointStateIsWatched     uint8 = 0
	WatchpointStateIsInvalidated uint8 = 1
)

type LLIntPrototypeLoadAdaptiveStructureWatchpoint struct{}

// InlineWatchpointSet is a set of watchpoints inlined in objects.
type InlineWatchpointSet struct {
	state uint8
	data  uint64
}

func NewInlineWatchpointSet() *InlineWatchpointSet {
	return &InlineWatchpointSet{state: WatchpointStateIsWatched}
}

func (s *InlineWatchpointSet) IsStillValid() bool { return s.state == WatchpointStateIsWatched }
func (s *InlineWatchpointSet) Invalidate() { s.state = WatchpointStateIsInvalidated }
