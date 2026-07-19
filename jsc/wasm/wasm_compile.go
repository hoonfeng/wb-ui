package wasm

// WasmFunctionParser parses Wasm function bodies.
type WasmFunctionParser struct {
	FunctionIndex uint32
	Body          []byte
	Locals        []WasmValueType
	Offset        uint32
}

// NewWasmFunctionParser creates a new function body parser.
func NewWasmFunctionParser(funcIndex uint32, body []byte) *WasmFunctionParser {
	return &WasmFunctionParser{
		FunctionIndex: funcIndex,
		Body:          body,
		Offset:        0,
	}
}

// WasmIRGeneratorHelpers provides helpers for Wasm IR generation.
type WasmIRGeneratorHelpers struct{}

// WasmBBQJIT is the skeleton for the BBQ baseline JIT.
type WasmBBQJIT struct{}

// WasmBBQPlan is the plan for BBQ compilation.
type WasmBBQPlan struct {
	Module *WasmModule
}

// WasmOMGIRGenerator is the skeleton for OMG optimized IR generation.
type WasmOMGIRGenerator struct{}

// WasmOMGPlan is the plan for OMG compilation.
type WasmOMGPlan struct {
	Module *WasmModule
}

// WasmOSREntryData stores OSR entry data for Wasm.
type WasmOSREntryData struct {
	FunctionIndex uint32
	LoopDepth     uint32
}

// WasmOSREntryPlan is the plan for OSR entry.
type WasmOSREntryPlan struct{}

// WasmIPIntGenerator generates IP interpreter code for Wasm.
type WasmIPIntGenerator struct{}

// WasmIPIntPlan is the plan for IP interpreter compilation.
type WasmIPIntPlan struct{}

// WasmIPIntSlowPaths provides slow paths for the Wasm interpreter.
type WasmIPIntSlowPaths struct{}

// WasmIPIntTierUpCounter tracks tier-up for the IP interpreter.
type WasmIPIntTierUpCounter struct {
	Count     uint32
	Threshold uint32
}

// WasmTierUpCount tracks tier-up counters.
type WasmTierUpCount struct {
	Counts map[uint32]uint32
}

// WasmConstExprGenerator generates constant expressions.
type WasmConstExprGenerator struct{}

// WasmBaselineData stores baseline compilation data.
type WasmBaselineData struct {
	FunctionIndex uint32
	CodeSize      uint32
}
