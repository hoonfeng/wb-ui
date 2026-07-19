package wasm

// WasmModule represents a compiled WebAssembly module.
type WasmModule struct {
	ModuleInfo *WasmModuleInformation
	CalleeGroup *WasmCalleeGroup
}

// WasmModuleInformation stores the parsed structure of a Wasm module.
type WasmModuleInformation struct {
	Functions    []WasmFunctionData
	Imports      []WasmImport
	Exports      []WasmExport
	Memory       *Limits
	Table        *Limits
	GlobalCount  uint32
	TypeCount    uint32
	ImportCount  uint32
	ExportCount  uint32
	FunctionCount uint32
	TableCount   uint32
	MemoryCount  uint32
	ElementCount uint32
	DataCount    uint32
}

// WasmFunctionData stores information about a function in the module.
type WasmFunctionData struct {
	Index        uint32
	TypeIndex    uint32
	Name         string
	CodeSize     uint64
	LocalsCount  uint32
}

// WasmImport represents an imported entity in a Wasm module.
type WasmImport struct {
	ModuleName  string
	FieldName   string
	Kind        WasmExternalKind
	TypeIndex   uint32
}

// WasmExport represents an exported entity in a Wasm module.
type WasmExport struct {
	Name    string
	Kind    WasmExternalKind
	Index   uint32
}

// WasmExternalKind defines the kind of external Wasm entity.
type WasmExternalKind uint8

const (
	WasmExternalKindFunction WasmExternalKind = 0
	WasmExternalKindTable    WasmExternalKind = 1
	WasmExternalKindMemory   WasmExternalKind = 2
	WasmExternalKindGlobal   WasmExternalKind = 3
	WasmExternalKindTag      WasmExternalKind = 4
)

// WasmName stores a UTF-8 name for Wasm entities.
type WasmName struct {
	Name string
}

// WasmNameSection stores the name section data for debugging.
type WasmNameSection struct {
	ModuleName     string
	FunctionNames  map[uint32]string
	LocalNames     map[uint32]map[uint32]string
}

// WasmNameSectionParser parses the Wasm name custom section.
type WasmNameSectionParser struct{}
