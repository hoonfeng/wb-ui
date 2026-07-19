// Repatch / SharedJITStubSet (JIT only)
package bytecode

type Repatch struct{}
type SharedJITStubSet struct{}

// StructureSet - a set of StructureIDs
type StructureSet struct {
	structures []uint64
}

func NewStructureSet() *StructureSet {
	return &StructureSet{structures: make([]uint64, 0, 4)}
}

func (s *StructureSet) Add(id uint64) { s.structures = append(s.structures, id) }
func (s *StructureSet) Contains(id uint64) bool {
	for _, sid := range s.structures {
		if sid == id {
			return true
		}
	}
	return false
}
func (s *StructureSet) Size() int { return len(s.structures) }
func (s *StructureSet) At(i int) uint64 { return s.structures[i] }

// SuperSampler - statistical profiling
type SuperSampler struct{}
func SuperSamplerBegin() {}
func SuperSamplerEnd() {}
