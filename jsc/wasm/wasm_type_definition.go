package wasm

// WasmTypeDefinition represents a Wasm type definition.
type WasmTypeDefinition struct {
	ID          WasmTypeDefinitionID
	Kind        WasmTypeDefinitionKind
	Params      []WasmValueType
	Results     []WasmValueType
	SuperTypes  []WasmTypeDefinitionID
}

// WasmTypeDefinitionKind defines the kind of type definition.
type WasmTypeDefinitionKind uint8

const (
	WasmTypeDefinitionFunc       WasmTypeDefinitionKind = 0
	WasmTypeDefinitionStruct     WasmTypeDefinitionKind = 1
	WasmTypeDefinitionArray      WasmTypeDefinitionKind = 2
	WasmTypeDefinitionSub        WasmTypeDefinitionKind = 3
	WasmTypeDefinitionSubFinal   WasmTypeDefinitionKind = 4
)

// WasmValueType represents Wasm value types.
type WasmValueType uint8

const (
	WasmValueTypeI32    WasmValueType = 0x7F
	WasmValueTypeI64    WasmValueType = 0x7E
	WasmValueTypeF32    WasmValueType = 0x7D
	WasmValueTypeF64    WasmValueType = 0x7C
	WasmValueTypeV128   WasmValueType = 0x7B
	WasmValueTypeExternRef WasmValueType = 0x6F
	WasmValueTypeFuncref   WasmValueType = 0x70
)

// WasmSIMDOpcodes defines SIMD opcode constants.
type WasmSIMDOpcodes uint8

const (
	WasmSIMDOpcodesV128Load       WasmSIMDOpcodes = 0
	WasmSIMDOpcodesV128Store      WasmSIMDOpcodes = 1
	WasmSIMDOpcodesV128Const      WasmSIMDOpcodes = 2
	WasmSIMDOpcodesI8x16Splat     WasmSIMDOpcodes = 3
	WasmSIMDOpcodesI16x8Splat     WasmSIMDOpcodes = 4
	WasmSIMDOpcodesI32x4Splat     WasmSIMDOpcodes = 5
	WasmSIMDOpcodesI64x2Splat     WasmSIMDOpcodes = 6
	WasmSIMDOpcodesF32x4Splat     WasmSIMDOpcodes = 7
	WasmSIMDOpcodesF64x2Splat     WasmSIMDOpcodes = 8
)

// WasmFormat defines Wasm binary format constants.
type WasmFormat struct{}

// WasmSections defines Wasm section IDs.
type WasmSectionID uint8

const (
	WasmSectionCustom     WasmSectionID = 0
	WasmSectionType       WasmSectionID = 1
	WasmSectionImport     WasmSectionID = 2
	WasmSectionFunction   WasmSectionID = 3
	WasmSectionTable      WasmSectionID = 4
	WasmSectionMemory     WasmSectionID = 5
	WasmSectionGlobal     WasmSectionID = 6
	WasmSectionExport     WasmSectionID = 7
	WasmSectionStart      WasmSectionID = 8
	WasmSectionElement    WasmSectionID = 9
	WasmSectionCode       WasmSectionID = 10
	WasmSectionData       WasmSectionID = 11
	WasmSectionDataCount  WasmSectionID = 12
	WasmSectionTag        WasmSectionID = 13
)

// WasmBranchHints stores branch hint information for Wasm.
type WasmBranchHints struct {
	Hints map[uint32]WasmBranchHint
}

// WasmBranchHint defines branch prediction hints.
type WasmBranchHint uint8

const (
	WasmBranchHintUnlikely WasmBranchHint = 0
	WasmBranchHintLikely   WasmBranchHint = 1
)

// WasmBranchHintsSectionParser parses the branch hints custom section.
type WasmBranchHintsSectionParser struct{}
