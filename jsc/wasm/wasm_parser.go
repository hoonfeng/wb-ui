package wasm

import "fmt"

// WasmParser parses WebAssembly binary format.
type WasmParser struct {
	Data    []byte
	Offset  uint32
}

// NewWasmParser creates a new Wasm parser.
func NewWasmParser(data []byte) *WasmParser {
	return &WasmParser{Data: data, Offset: 0}
}

// ReadU8 reads a uint8 from the data.
func (p *WasmParser) ReadU8() (uint8, error) {
	if p.Offset >= uint32(len(p.Data)) {
		return 0, fmt.Errorf("unexpected end of Wasm data")
	}
	val := p.Data[p.Offset]
	p.Offset++
	return val, nil
}

// ReadU32 reads a LEB128-encoded uint32 from the data.
func (p *WasmParser) ReadU32() (uint32, error) {
	var result uint32
	var shift uint32
	for {
		if p.Offset >= uint32(len(p.Data)) {
			return 0, fmt.Errorf("unexpected end of Wasm data")
		}
		byte := p.Data[p.Offset]
		p.Offset++
		result |= uint32(byte&0x7F) << shift
		if byte&0x80 == 0 {
			break
		}
		shift += 7
		if shift > 35 {
			return 0, fmt.Errorf("invalid LEB128 encoding")
		}
	}
	return result, nil
}

// SectionParser parses a Wasm section.
type SectionParser struct{}

// WasmStreamingParser parses Wasm in streaming mode.
type WasmStreamingParser struct{}

// WasmStreamingCompiler compiles Wasm in streaming mode.
type WasmStreamingCompiler struct{}

// WasmEntryPlan is the plan for entering a Wasm module.
type WasmEntryPlan struct {
	Module *WasmModule
}

// WasmPlan is a base plan for Wasm compilation.
type WasmPlan struct {
	Module *WasmModule
}

// WasmWorklist manages a list of Wasm compilation work items.
type WasmWorklist struct {
	Items []*WasmPlan
}

// WasmThunks provides thunk generation for Wasm.
type WasmThunks struct{}
