package wasm

// WasmMemory represents a WebAssembly linear memory.
type WasmMemory struct {
	Buffer   []byte
	Size     uint64
	MaxSize  uint64
	IsShared bool
	Is64Bit  bool
}

// NewWasmMemory creates a new Wasm memory with the given initial size.
func NewWasmMemory(initialSize uint64, maxSize uint64, is64Bit bool) *WasmMemory {
	return &WasmMemory{
		Buffer:  make([]byte, initialSize),
		Size:    initialSize,
		MaxSize: maxSize,
		Is64Bit: is64Bit,
	}
}

// Grow grows the memory by the given number of pages (64KB each).
func (m *WasmMemory) Grow(pages uint64) bool {
	newSize := m.Size + pages*65536
	if m.MaxSize > 0 && newSize > m.MaxSize {
		return false
	}
	newBuf := make([]byte, newSize)
	copy(newBuf, m.Buffer)
	m.Buffer = newBuf
	m.Size = newSize
	return true
}

// MemoryInformation stores the memory information from a Wasm module.
type MemoryInformation struct {
	Initial uint64
	Maximum uint64
	IsShared bool
	Is64Bit  bool
}

// WasmTable represents a WebAssembly table (array of function references).
type WasmTable struct {
	Elements []uint32
	Size     uint32
	MaxSize  uint32
}

// NewWasmTable creates a new Wasm table.
func NewWasmTable(initialSize uint32, maxSize uint32) *WasmTable {
	return &WasmTable{
		Elements: make([]uint32, initialSize),
		Size:     initialSize,
		MaxSize:  maxSize,
	}
}

// WasmTag represents a Wasm exception tag.
type WasmTag struct {
	TypeIndex uint32
	Attribute WasmTagAttribute
}

// WasmTagAttribute defines tag attributes.
type WasmTagAttribute uint8

const (
	WasmTagAttributeException WasmTagAttribute = 0
)

// WasmGlobal represents a Wasm global variable.
type WasmGlobal struct {
	Type    WasmValueType
	Mutable bool
	Value   uint64
}

// WasmValueLocation describes where a Wasm value is stored.
type WasmValueLocation struct {
	Kind      WasmValueLocationKind
	StackOffset int32
}

// WasmValueLocationKind describes the storage kind for a value.
type WasmValueLocationKind uint8

const (
	WasmValueLocationStack    WasmValueLocationKind = 0
	WasmValueLocationRegister WasmValueLocationKind = 1
	WasmValueLocationGlobal   WasmValueLocationKind = 2
	WasmValueLocationLocal    WasmValueLocationKind = 3
	WasmValueLocationConst    WasmValueLocationKind = 4
)
