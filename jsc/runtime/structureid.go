// Translation of: Source/JavaScriptCore/runtime/StructureID.h
//
// StructureID identifies a Structure* via a 32-bit ID. Go port uses a simple
// uint32 index into a global structure table (no pointer encoding needed).

package runtime

import "sync/atomic"

// StructureID corresponds to JSC::StructureID.
// In the Go port, this is a uint32 index into vm.structureTable.
type StructureID uint32

const (
	// NukedStructureIDBit flags a StructureID as "nuked" (invalidated).
	NukedStructureIDBit uint32 = 1

	// InvalidStructureID is the zero value (no structure).
	InvalidStructureID StructureID = 0
)

// Nuke returns a nuked copy of this ID.
func (id StructureID) Nuke() StructureID {
	return StructureID(uint32(id) | NukedStructureIDBit)
}

// IsNuked returns true if this ID has been nuked.
func (id StructureID) IsNuked() bool {
	return uint32(id)&NukedStructureIDBit != 0
}

// Decontaminate strips the nuked bit.
func (id StructureID) Decontaminate() StructureID {
	return StructureID(uint32(id) & ^NukedStructureIDBit)
}

// IsValid returns true if the ID is non-zero and not nuked.
func (id StructureID) IsValid() bool {
	return uint32(id) != 0 && !id.IsNuked()
}

// globalStructureTable is the global mapping from StructureID to *Structure.
// TODO: move into VM when VM is refactored per-cell.
var globalStructureTable = struct {
	entries []atomic.Pointer[Structure]
}{}

func init() {
	globalStructureTable.entries = make([]atomic.Pointer[Structure], 0, 1024)
}

// AllocateStructureID reserves a new StructureID and returns it.
func AllocateStructureID() StructureID {
	idx := len(globalStructureTable.entries)
	globalStructureTable.entries = append(globalStructureTable.entries, atomic.Pointer[Structure]{})
	return StructureID(idx + 1) // 0 = invalid
}

// RegisterStructure associates a Structure with its ID.
func RegisterStructure(id StructureID, s *Structure) {
	idx := int(uint32(id) & ^NukedStructureIDBit) - 1
	if idx >= 0 && idx < len(globalStructureTable.entries) {
		globalStructureTable.entries[idx].Store(s)
	}
}

// Decode looks up the *Structure for this ID.
func (id StructureID) Decode() *Structure {
	id = id.Decontaminate()
	if uint32(id) == 0 {
		return nil
	}
	idx := int(uint32(id)) - 1
	if idx < 0 || idx >= len(globalStructureTable.entries) {
		return nil
	}
	return globalStructureTable.entries[idx].Load()
}

// TryDecode returns the Structure or nil if invalid.
func (id StructureID) TryDecode() *Structure {
	return id.Decode()
}

// Encode creates a StructureID for the given Structure.
func EncodeStructureID(s *Structure) StructureID {
	if s == nil {
		return InvalidStructureID
	}
	return s.structureID
}
