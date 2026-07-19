package wasm

// AddressType represents a Wasm memory address type.
type AddressType uint8

const (
	AddressTypeMemory32 AddressType = 0
	AddressTypeMemory64 AddressType = 1
)

// WasmBinding represents the binding between Wasm functions and JSC.
type WasmBinding struct {
	FunctionIndex uint32
	ModuleIndex   uint32
}

// CallProfile stores profiling data for Wasm function calls.
type CallProfile struct {
	CalleeIndex uint32
	CallCount   uint64
}

// WasmCallee represents a compiled Wasm function.
type WasmCallee struct {
	Index          uint32
	CodeSize       uint64
	CompilationMode CompilationMode
}

// WasmCalleeGroup manages a group of callees for a module.
type WasmCalleeGroup struct {
	Callees []*WasmCallee
}

// WasmCallingConvention defines the calling convention for Wasm.
type WasmCallingConvention struct{}

// WasmCapabilities reports Wasm capability support.
type WasmCapabilities struct{}

// HasWasmCapabilities checks if Wasm is supported in this environment.
func HasWasmCapabilities() bool { return false }

// WasmCompilationContext provides context for Wasm compilation.
type WasmCompilationContext struct {
	Module      *WasmModule
	CompileMode CompilationMode
}

// WasmContext holds the current Wasm execution context.
type WasmContext struct {
	Memory *WasmMemory
	Table  *WasmTable
}

// WasmExceptionType defines Wasm exception types.
type WasmExceptionType uint8

const (
	WasmExceptionTypeNone        WasmExceptionType = 0
	WasmExceptionTypeStackOverflow WasmExceptionType = 1
	WasmExceptionTypeOutOfBounds  WasmExceptionType = 2
	WasmExceptionTypeOutOfMemory  WasmExceptionType = 3
	WasmExceptionTypeRemapped     WasmExceptionType = 4
	WasmExceptionTypeTypeMismatch WasmExceptionType = 5
)
