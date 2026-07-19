// InternalFunctionAllocationProfile - tracks allocation of InternalFunction instances
package bytecode

type InternalFunctionAllocationProfile struct {
	structureID uint64
	allocated   bool
}

func (p *InternalFunctionAllocationProfile) Allocated() bool { return p.allocated }
func (p *InternalFunctionAllocationProfile) SetAllocated(v bool) { p.allocated = v }
func (p *InternalFunctionAllocationProfile) StructureID() uint64 { return p.structureID }

// IterationModeMetadata - metadata for iteration bytecodes
type IterationModeMetadata struct {
	kind uint8 // 0=generic, 1=fast-array, 2=fast-object
}

func NewIterationModeMetadata() IterationModeMetadata { return IterationModeMetadata{} }
func (m *IterationModeMetadata) Kind() uint8 { return m.kind }
func (m *IterationModeMetadata) SetKind(k uint8) { m.kind = k }
