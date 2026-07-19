package tools

// FunctionOverrides provides a mechanism to override function source code for testing.
type FunctionOverrides struct {
	overrides map[string]FunctionOverrideInfo
}

// FunctionOverrideInfo stores the source override info for a function.
type FunctionOverrideInfo struct {
	FirstLine             uint32
	LineCount             uint32
	StartColumn           uint32
	EndColumn             uint32
	ParametersStartOffset uint32
	FunctionStart         uint32
	FunctionEnd           uint32
	SourceCode            string
}

// NewFunctionOverrides creates a new FunctionOverrides instance.
func NewFunctionOverrides() *FunctionOverrides {
	return &FunctionOverrides{overrides: make(map[string]FunctionOverrideInfo)}
}
