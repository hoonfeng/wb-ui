package debugger

// DebuggerCallFrame represents a call frame in the debugger.
type DebuggerCallFrame struct {
	CallFrameIndex int
	FunctionName   string
	SourceID       SourceID
	Line           int
	Column         int
	ScopeChain     []*DebuggerScope
}

// DebuggerScope represents a scope in the debugger's call frame.
type DebuggerScope struct {
	Type       ScopeType
	Object     interface{} // *runtime.JSObject
	Name       string
	Properties map[string]interface{}
}

// ScopeType defines the type of scope.
type ScopeType uint8

const (
	ScopeTypeGlobal       ScopeType = 0
	ScopeTypeWith         ScopeType = 1
	ScopeTypeLocal        ScopeType = 2
	ScopeTypeClosure      ScopeType = 3
	ScopeTypeCatch        ScopeType = 4
	ScopeTypeFunctionName ScopeType = 5
	ScopeTypeModule       ScopeType = 6
)

// DebuggerEvalEnabler enables eval in debugger context.
type DebuggerEvalEnabler struct {
	vm      interface{}
	enabled bool
}

// NewDebuggerEvalEnabler creates a new eval enabler.
func NewDebuggerEvalEnabler(vm interface{}) *DebuggerEvalEnabler {
	return &DebuggerEvalEnabler{vm: vm, enabled: true}
}
