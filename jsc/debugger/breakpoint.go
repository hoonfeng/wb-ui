package debugger

// Breakpoint represents a debugger breakpoint.
type Breakpoint struct {
	ID       BreakpointID
	SourceID SourceID
	Line     int
	Column   int
	Enabled  bool
	Condition string
}

// NewBreakpoint creates a new breakpoint.
func NewBreakpoint(id BreakpointID, sourceID SourceID, line, column int) *Breakpoint {
	return &Breakpoint{
		ID:       id,
		SourceID: sourceID,
		Line:     line,
		Column:   column,
		Enabled:  true,
	}
}
