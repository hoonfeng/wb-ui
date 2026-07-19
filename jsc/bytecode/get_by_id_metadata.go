// GetByIdMetadata - metadata for property access (JIT IC)
package bytecode

type GetByIdMode uint8
const (
	GetByIdModeDefault GetByIdMode = 0
	GetByIdModeProto   GetByIdMode = 1
	GetByIdModeUnset   GetByIdMode = 2
)

type GetByIdModeMetadata struct {
	mode   GetByIdMode
	hitCountForLLIntCaching uint32
}

type GetByIdMetadata struct {
	modeMetadata GetByIdModeMetadata
}

