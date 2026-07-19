package wasm

// CompilationMode indicates how WebAssembly is compiled.
type CompilationMode uint8

const (
	CompilationModeBBQ        CompilationMode = 0 // Bulk baseline
	CompilationModeOMG        CompilationMode = 1 // Optimized
	CompilationModeIPInt      CompilationMode = 2 // IP interpreter
	CompilationModeEagerBBQ   CompilationMode = 3
	CompilationModeEagerOMG   CompilationMode = 4
)

// CreationMode indicates how a Wasm module is created.
type CreationMode uint8

const (
	CreationModeFromModule   CreationMode = 0
	CreationModeFromCompiled CreationMode = 1
)

// CompileMode is a type alias for CompilationMode.
type CompileMode = CompilationMode

// WasmOpcodeID represents a WebAssembly opcode.
type WasmOpcodeID uint16

// WasmTypeDefinitionID identifies a type definition.
type WasmTypeDefinitionID uint32

// WasmFunctionSpace indicates the function index space.
type WasmFunctionSpace uint8

const (
	WasmFunctionSpaceImport  WasmFunctionSpace = 0
	WasmFunctionSpaceLocal   WasmFunctionSpace = 1
	WasmFunctionSpaceMax     WasmFunctionSpace = 2
)

// IndexOrName represents either a numeric index or a name.
type IndexOrName struct {
	Index uint32
	Name  string
	IsName bool
}

// Limits represents WebAssembly memory/table limits.
type Limits struct {
	Minimum uint64
	Maximum uint64
	HasMax  bool
}
