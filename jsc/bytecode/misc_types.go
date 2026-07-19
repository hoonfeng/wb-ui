// LineColumn - source position
package bytecode

type LineColumn struct {
	line   uint32
	column uint32
}

func NewLineColumn(line, column uint32) LineColumn {
	return LineColumn{line: line, column: column}
}

func (lc LineColumn) Line() uint32   { return lc.line }
func (lc LineColumn) Column() uint32 { return lc.column }

// LinkTimeConstant - constants resolved at link time
type LinkTimeConstant struct {
	index uint32
}

// ObjectAllocationProfile - profiling for object allocations
type ObjectAllocationProfile struct {
	structureID  uint64
	allocated    bool
}

func (p *ObjectAllocationProfile) StructureID() uint64 { return p.structureID }
func (p *ObjectAllocationProfile) SetStructureID(id uint64) { p.structureID = id }
func (p *ObjectAllocationProfile) Allocated() bool { return p.allocated }

// Operands - description of operand layout
type Operands struct {
	operands []VirtualRegister
}

func NewOperands(size int) *Operands {
	return &Operands{operands: make([]VirtualRegister, size)}
}

func (o *Operands) Size() int { return len(o.operands) }
func (o *Operands) Operand(i int) VirtualRegister { return o.operands[i] }
func (o *Operands) SetOperand(i int, v VirtualRegister) { o.operands[i] = v }
