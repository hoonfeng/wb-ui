package wasm

import "fmt"

// WasmSectionParser parses a specific Wasm section.
type WasmSectionParser struct {
	Data   []byte
	Offset uint32
}

// NewWasmSectionParser creates a new section parser.
func NewWasmSectionParser(data []byte) *WasmSectionParser {
	return &WasmSectionParser{Data: data}
}

// ReadU8 reads a single byte.
func (p *WasmSectionParser) ReadU8() (uint8, error) {
	if p.Offset >= uint32(len(p.Data)) {
		return 0, fmt.Errorf("unexpected end of section")
	}
	val := p.Data[p.Offset]
	p.Offset++
	return val, nil
}

// ReadVarU32 reads a variable-length unsigned 32-bit integer.
func (p *WasmSectionParser) ReadVarU32() (uint32, error) {
	var result uint32
	var shift uint32
	for {
		if p.Offset >= uint32(len(p.Data)) {
			return 0, fmt.Errorf("unexpected end of section")
		}
		b := p.Data[p.Offset]
		p.Offset++
		result |= uint32(b&0x7F) << shift
		if b&0x80 == 0 {
			break
		}
		shift += 7
		if shift > 35 {
			return 0, fmt.Errorf("invalid LEB128 encoding")
		}
	}
	return result, nil
}

// WasmSourceMappingURLSectionParser parses the source mapping URL section.
type WasmSourceMappingURLSectionParser struct{}

// WasmLimits defines WebAssembly limits.
type WasmLimits struct {
	MaxFunctionCodeSize       uint32
	MaxFunctionLocals         uint32
	MaxFunctionParams         uint32
	MaxFunctionReturns        uint32
	MaxTableSize              uint32
	MaxMemoryPages            uint32
	MaxModuleSize             uint32
	MaxFunctionCount          uint32
	MaxImports                uint32
	MaxExports                uint32
	MaxDataSegments           uint32
	MaxElementSegments        uint32
	MaxGlobals                uint32
	MaxIndirectFunctionTable  uint32
	MaxTagCount               uint32
}
