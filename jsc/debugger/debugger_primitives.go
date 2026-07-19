package debugger

// DebuggerPrimitives defines the basic types used by the debugger.

// SourceID uniquely identifies a source in the debugger.
type SourceID uint64

const NoSourceID SourceID = 0

// BreakpointID uniquely identifies a breakpoint.
type BreakpointID uint64

const NoBreakpointID BreakpointID = 0
