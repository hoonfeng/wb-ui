package debugger

// DebuggerParseData stores parsed source positions for the debugger.
type DebuggerParseData struct {
	SourceID SourceID
	Lines    []DebuggerParseLine
}

// DebuggerParseLine stores parse data for a single source line.
type DebuggerParseLine struct {
	LineNumber  int
	StartOffset int
	EndOffset   int
}

// DebuggerLocation represents a source location in the debugger.
type DebuggerLocation struct {
	SourceID     SourceID
	LineNumber   int
	ColumnNumber int
}

// ScriptProfilingScope profiles script execution timing.
type ScriptProfilingScope struct {
	Name string
}
