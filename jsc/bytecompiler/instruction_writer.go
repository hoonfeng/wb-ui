// Package bytecompiler provides the bytecode generator.
// This file contains the InstructionStreamWriter and related types
// used by the BytecodeGeneratorBase to emit bytecode instructions.

package bytecompiler

// InstructionStreamWriter is a simplified writer for bytecode instructions.
// In WebKit this is a template over InstructionType; simplified to []int.
type InstructionStreamWriter struct {
	buffer []int
	pos    int
}

// NewInstructionStreamWriter creates a new writer.
func NewInstructionStreamWriter() *InstructionStreamWriter {
	return &InstructionStreamWriter{buffer: make([]int, 0, 1024)}
}

// Write appends values to the stream.
func (w *InstructionStreamWriter) Write(values ...int) {
	w.buffer = append(w.buffer, values...)
	w.pos = len(w.buffer)
}

// Position returns the current write position.
func (w *InstructionStreamWriter) Position() int {
	return w.pos
}

// Ref creates a mutable reference at the current position.
func (w *InstructionStreamWriter) Ref() *InstructionStreamMutableRef {
	return &InstructionStreamMutableRef{offset: w.pos}
}

// Swap exchanges the buffer with another writer.
func (w *InstructionStreamWriter) Swap(other *InstructionStreamWriter) {
	w.buffer, other.buffer = other.buffer, w.buffer
	w.pos, other.pos = other.pos, w.pos
}

// InstructionStreamMutableRef corresponds to WebKit's InstructionStream::MutableRef.
type InstructionStreamMutableRef struct {
	offset int
}

// Offset returns the bytecode offset of this reference.
func (r *InstructionStreamMutableRef) Offset() int { return r.offset }

// UnlinkedCodeBlockGenerator is a simplified proxy for WebKit's UnlinkedCodeBlockGenerator.
// In the full translation, this would be defined in the bytecode package.
type UnlinkedCodeBlockGenerator struct {
	numCalleeLocals   uint32
	numVars           uint32
	numJumpTargets    uint32
	lastJumpTarget    uint32
	hasCheckpoints    bool
}

// NewUnlinkedCodeBlockGenerator creates a new generator.
func NewUnlinkedCodeBlockGenerator() *UnlinkedCodeBlockGenerator {
	return &UnlinkedCodeBlockGenerator{}
}

// NumCalleeLocals returns the number of callee-local registers.
func (b *UnlinkedCodeBlockGenerator) NumCalleeLocals() uint32 { return b.numCalleeLocals }

// SetNumCalleeLocals sets the number of callee-local registers.
func (b *UnlinkedCodeBlockGenerator) SetNumCalleeLocals(n uint32) { b.numCalleeLocals = n }

// NumVars returns the number of variables.
func (b *UnlinkedCodeBlockGenerator) NumVars() uint32 { return b.numVars }

// SetNumVars sets the number of variables.
func (b *UnlinkedCodeBlockGenerator) SetNumVars(n uint32) { b.numVars = n }

// NumberOfJumpTargets returns the count of jump targets.
func (b *UnlinkedCodeBlockGenerator) NumberOfJumpTargets() uint32 { return b.numJumpTargets }

// LastJumpTarget returns the last jump target.
func (b *UnlinkedCodeBlockGenerator) LastJumpTarget() uint32 { return b.lastJumpTarget }

// AddJumpTarget adds a new jump target.
func (b *UnlinkedCodeBlockGenerator) AddJumpTarget(target uint32) {
	b.lastJumpTarget = target
	b.numJumpTargets++
}

// SetHasCheckpoints marks that the code block uses checkpoints.
func (b *UnlinkedCodeBlockGenerator) SetHasCheckpoints() { b.hasCheckpoints = true }
