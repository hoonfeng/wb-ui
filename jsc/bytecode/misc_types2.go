// ToThisStatus - tracks the result of to_this conversion
package bytecode

type ToThisStatus uint8

const (
	ToThisStatusUninitialized ToThisStatus = 0
	ToThisStatusOK            ToThisStatus = 1
	ToThisStatusGood          ToThisStatus = 2
)

// TrackedReferences - tracks JSCell references for GC
type TrackedReferences struct {
	references []uint64
}

func NewTrackedReferences() *TrackedReferences {
	return &TrackedReferences{references: make([]uint64, 0)}
}

func (t *TrackedReferences) Add(ref uint64) {
	t.references = append(t.references, ref)
}

func (t *TrackedReferences) Contains(ref uint64) bool {
	for _, r := range t.references {
		if r == ref {
			return true
		}
	}
	return false
}

// TypeLocation - stores type inference location info
type TypeLocation struct {
	globalVariable    uint32
	identifier        uint32
	sourceID          SourceID
	sourceOffset      uint32
	globalTypeSet     uint64 // simplified: bitset of types
}

// VariableWriteFireDetail - details about a variable write that fires a watchpoint
type VariableWriteFireDetail struct {
	name string
}
