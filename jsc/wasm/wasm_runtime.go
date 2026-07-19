package wasm

import "fmt"

// WasmOperations implements WebAssembly runtime operations.
type WasmOperations struct{}

// WasmJS provides the JavaScript API for WebAssembly.
type WasmJS struct{}

// WasmMachineThreads manages machine thread state for Wasm.
type WasmMachineThreads struct{}

// WasmFaultSignalHandler handles faults during Wasm execution.
type WasmFaultSignalHandler struct{}

// WasmHandlerInfo stores exception handler information for Wasm.
type WasmHandlerInfo struct {
	TryStart    uint32
	TryEnd      uint32
	CatchTarget uint32
	Kind        WasmHandlerKind
}

// WasmHandlerKind defines handler types.
type WasmHandlerKind uint8

const (
	WasmHandlerCatch    WasmHandlerKind = 0
	WasmHandlerCatchAll WasmHandlerKind = 1
	WasmHandlerDelegate WasmHandlerKind = 2
)

// WasmOpcodeOrigin tracks the origin of a Wasm opcode for debugging.
type WasmOpcodeOrigin struct {
	FunctionIndex uint32
	OpcodeOffset  uint32
}

// WasmOpcodeCounter counts Wasm opcode execution.
type WasmOpcodeCounter struct {
	Counts map[WasmOpcodeID]uint64
}

// WasmMergedProfile merges multiple Wasm profiles.
type WasmMergedProfile struct {
	Profiles []interface{}
}

// WasmInstanceAnchor anchors a Wasm instance for GC.
type WasmInstanceAnchor struct {
	Instance interface{}
}

// String returns the string representation of a value location.
func (v WasmValueLocation) String() string {
	switch v.Kind {
	case WasmValueLocationStack:
		return fmt.Sprintf("stack(%d)", v.StackOffset)
	case WasmValueLocationRegister:
		return fmt.Sprintf("register(%d)", v.StackOffset)
	case WasmValueLocationGlobal:
		return fmt.Sprintf("global(%d)", v.StackOffset)
	case WasmValueLocationLocal:
		return fmt.Sprintf("local(%d)", v.StackOffset)
	case WasmValueLocationConst:
		return fmt.Sprintf("const(%d)", v.StackOffset)
	default:
		return fmt.Sprintf("unknown(%d)", v.StackOffset)
	}
}
