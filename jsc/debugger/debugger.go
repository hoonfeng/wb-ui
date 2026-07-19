package debugger

// Debugger is the main JSC debugger controller.
// It manages breakpoints, stepping, and debugging sessions.
type Debugger struct {
	vm interface{} // *runtime.VM - avoid import cycle

	breakpoints          map[BreakpointID]*Breakpoint
	sourceBreakpoints    map[SourceID][]*Breakpoint
	paused               bool
	breakpointsActivated bool
	steppingMode         SteppingMode
}

// SteppingMode defines debugger stepping behavior.
type SteppingMode uint8

const (
	SteppingNone   SteppingMode = 0
	SteppingInto   SteppingMode = 1
	SteppingOver   SteppingMode = 2
	SteppingOut    SteppingMode = 3
)

// ReasonForDetach indicates why a debugging session ended.
type ReasonForDetach uint8

const (
	DetachReasonTerminatingDebuggingSession ReasonForDetach = 0
	DetachReasonGlobalObjectIsDestructing   ReasonForDetach = 1
)

// ProfilingReason indicates why profiling is being done.
type ProfilingReason uint8

// NewDebugger creates a new Debugger instance.
func NewDebugger(vm interface{}) *Debugger {
	return &Debugger{
		vm:                   vm,
		breakpoints:          make(map[BreakpointID]*Breakpoint),
		sourceBreakpoints:    make(map[SourceID][]*Breakpoint),
		breakpointsActivated: true,
	}
}

// SetBreakpoint sets a breakpoint.
func (d *Debugger) SetBreakpoint(bp *Breakpoint) bool {
	d.breakpoints[bp.ID] = bp
	d.sourceBreakpoints[bp.SourceID] = append(d.sourceBreakpoints[bp.SourceID], bp)
	return true
}

// RemoveBreakpoint removes a breakpoint.
func (d *Debugger) RemoveBreakpoint(bp *Breakpoint) bool {
	delete(d.breakpoints, bp.ID)
	srcBPs := d.sourceBreakpoints[bp.SourceID]
	for i, sbp := range srcBPs {
		if sbp.ID == bp.ID {
			d.sourceBreakpoints[bp.SourceID] = append(srcBPs[:i], srcBPs[i+1:]...)
			break
		}
	}
	return true
}

// ClearBreakpoints removes all breakpoints.
func (d *Debugger) ClearBreakpoints() {
	d.breakpoints = make(map[BreakpointID]*Breakpoint)
	d.sourceBreakpoints = make(map[SourceID][]*Breakpoint)
}

// ActivateBreakpoints enables breakpoints.
func (d *Debugger) ActivateBreakpoints() { d.breakpointsActivated = true }

// DeactivateBreakpoints disables breakpoints.
func (d *Debugger) DeactivateBreakpoints() { d.breakpointsActivated = false }

// BreakpointsActive checks if breakpoints are active.
func (d *Debugger) BreakpointsActive() bool { return d.breakpointsActivated }
