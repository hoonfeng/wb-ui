// PreciseJumpTargets - precise jump target computation
package bytecode

import "sort"

type PreciseJumpTargets struct {
	targets []uint32
}

func NewPreciseJumpTargets() *PreciseJumpTargets {
	return &PreciseJumpTargets{targets: make([]uint32, 0)}
}

func (t *PreciseJumpTargets) AddTarget(offset uint32) {
	t.targets = append(t.targets, offset)
}

func (t *PreciseJumpTargets) Targets() []uint32 { return t.targets }
func (t *PreciseJumpTargets) IsTarget(offset uint32) bool {
	// Binary search
	idx := sort.Search(len(t.targets), func(i int) bool {
		return t.targets[i] >= offset
	})
	return idx < len(t.targets) && t.targets[idx] == offset
}

// ComputePreciseJumpTargets computes the set of all jump targets in a bytecode stream.
func ComputePreciseJumpTargets(code *InstructionStream) *PreciseJumpTargets {
	targets := NewPreciseJumpTargets()
	offset := uint32(0)
	for offset < uint32(code.Size()) {
		inst, size := DecodeInstruction(code.RawPointer(), offset)
		// Check if this instruction is a branch
		if IsBranch(inst.OpcodeID()) {
			target := inst.ReadOperand(1) // branch target is usually operand 1
			targets.AddTarget(uint32(target))
		}
		if size == 0 {
			break
		}
		offset += size
	}
	return targets
}
