// Package bytecompiler provides the bytecode generator.
// This file corresponds to WebKit BytecodeGeneratorBase.h + BytecodeGeneratorBaseInlines.h.

package bytecompiler

// virtualRegisterForLocal returns a register offset for a local variable index.
func virtualRegisterForLocal(index int) int {
	return -1 - index // VirtualRegister local convention
}

// BytecodeGeneratorBase corresponds to WebKit's BytecodeGeneratorBase<JSGeneratorTraits>.
// It provides the base functionality for bytecode generation (register allocation, labels, writer).
type BytecodeGeneratorBase struct {
	writer                *InstructionStreamWriter
	codeBlock             *UnlinkedCodeBlockGenerator
	outOfMemoryDuringConstruction bool
	lastOpcodeID          int // op_debug or actual opcode ID
	m_labels              []*Label
	calleeLocals          []*RegisterID
}

const opcodeForDisablingOptimizations = 0 // Use op_debug to disable peephole; simplified.

// NewBytecodeGeneratorBase creates a new BytecodeGeneratorBase.
func NewBytecodeGeneratorBase(codeBlock *UnlinkedCodeBlockGenerator, virtualRegisterCountForCalleeSaves uint32) *BytecodeGeneratorBase {
	g := &BytecodeGeneratorBase{
		writer:    NewInstructionStreamWriter(),
		codeBlock: codeBlock,
	}
	g.allocateCalleeSaveSpace(virtualRegisterCountForCalleeSaves)
	return g
}

// allocateCalleeSaveSpace allocates callee save registers.
func (g *BytecodeGeneratorBase) allocateCalleeSaveSpace(count uint32) {
	for i := uint32(0); i < count; i++ {
		g.AddVar()
	}
}

// NewLabel creates a new label.
func (g *BytecodeGeneratorBase) NewLabel() *Label {
	g.shrinkToFitLabels()
	label := NewLabel()
	g.m_labels = append(g.m_labels, label)
	return label
}

// NewEmittedLabel creates a label and immediately emits it.
func (g *BytecodeGeneratorBase) NewEmittedLabel() *Label {
	label := g.NewLabel()
	g.EmitLabel(label)
	return label
}

// NewRegister creates a new callee-local register.
func (g *BytecodeGeneratorBase) NewRegister() *RegisterID {
	vr := virtualRegisterForLocal(len(g.calleeLocals))
	reg := NewRegisterIDFromVirtual(vr)
	g.calleeLocals = append(g.calleeLocals, reg)

	numCalleeLocals := max(len(g.calleeLocals), int(g.codeBlock.NumCalleeLocals()))
	// round up to stack alignment
	alignment := 2 // stackAlignmentRegisters() simplified
	numCalleeLocals = ((numCalleeLocals + alignment - 1) / alignment) * alignment
	g.codeBlock.SetNumCalleeLocals(uint32(numCalleeLocals))
	return reg
}

// NewTemporary creates a new temporary register.
func (g *BytecodeGeneratorBase) NewTemporary() *RegisterID {
	g.reclaimFreeRegisters()
	result := g.NewRegister()
	result.SetTemporary()
	return result
}

// NewTemporaries creates multiple temporary registers.
func (g *BytecodeGeneratorBase) NewTemporaries(count int, fn func(*RegisterID)) {
	g.reclaimFreeRegisters()
	for i := 0; i < count; i++ {
		result := g.NewRegister()
		result.SetTemporary()
		fn(result)
	}
}

// AddVar adds an anonymous local var slot.
func (g *BytecodeGeneratorBase) AddVar() *RegisterID {
	numVars := int(g.codeBlock.NumVars())
	g.codeBlock.SetNumVars(uint32(numVars + 1))
	result := g.NewRegister()
	// We should never free this slot.
	result.Ref()
	return result
}

// EmitLabel emits a label at the current writer position.
func (g *BytecodeGeneratorBase) EmitLabel(label *Label) {
	newLabelIndex := g.writer.Position()
	label.SetLocation(g, newLabelIndex) // g embeds BytecodeGeneratorBase

	if g.codeBlock.NumberOfJumpTargets() > 0 {
		lastLabelIndex := int(g.codeBlock.LastJumpTarget())
		if newLabelIndex == lastLabelIndex {
			// Peephole optimizations have already been disabled by emitting the last label
			return
		}
	}

	g.codeBlock.AddJumpTarget(uint32(newLabelIndex))
	g.lastOpcodeID = opcodeForDisablingOptimizations
}

// RecordOpcode records the current opcode for peephole optimization.
func (g *BytecodeGeneratorBase) RecordOpcode(opcodeID int) {
	// Simplified: just record the last opcode
	g.lastOpcodeID = opcodeID
}

// AlignWideOpcode16 emits NOPs for alignment (simplified).
func (g *BytecodeGeneratorBase) AlignWideOpcode16() {
	// Simplified: no-op in pure Go interpreter (no CPU alignment requirements)
}

// AlignWideOpcode32 emits NOPs for alignment (simplified).
func (g *BytecodeGeneratorBase) AlignWideOpcode32() {
	// Simplified: no-op in pure Go interpreter
}

// Write writes values to the instruction stream.
func (g *BytecodeGeneratorBase) Write(values ...int) {
	for _, v := range values {
		g.writer.Write(v)
	}
}

// reclaimFreeRegisters shrinks the calleeLocals list by removing trailing unused registers.
func (g *BytecodeGeneratorBase) reclaimFreeRegisters() {
	g.shrinkToFitCalleeLocals()
}

// shrinkToFitLabels removes trailing labels with no refs.
func (g *BytecodeGeneratorBase) shrinkToFitLabels() {
	for len(g.m_labels) > 0 && g.m_labels[len(g.m_labels)-1].RefCount() == 0 {
		g.m_labels = g.m_labels[:len(g.m_labels)-1]
	}
}

// shrinkToFitCalleeLocals removes trailing registers with no refs.
func (g *BytecodeGeneratorBase) shrinkToFitCalleeLocals() {
	for len(g.calleeLocals) > 0 && g.calleeLocals[len(g.calleeLocals)-1].RefCount() == 0 {
		g.calleeLocals = g.calleeLocals[:len(g.calleeLocals)-1]
	}
}

// Writer returns the instruction stream writer.
func (g *BytecodeGeneratorBase) Writer() *InstructionStreamWriter {
	return g.writer
}

// CodeBlock returns the unlinked code block being generated.
func (g *BytecodeGeneratorBase) CodeBlock() *UnlinkedCodeBlockGenerator {
	return g.codeBlock
}
