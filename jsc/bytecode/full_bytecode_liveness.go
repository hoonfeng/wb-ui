// FullBytecodeLiveness - tracks which registers are live at each bytecode offset
package bytecode

type FullBytecodeLiveness struct {
	liveness map[BytecodeIndex][]bool // per-offset live register map
}

func NewFullBytecodeLiveness() *FullBytecodeLiveness {
	return &FullBytecodeLiveness{liveness: make(map[BytecodeIndex][]bool)}
}

func (l *FullBytecodeLiveness) LiveAt(index BytecodeIndex, reg VirtualRegister) bool {
	if live, ok := l.liveness[index]; ok {
		idx := reg.Offset() + 1000 // offset to ensure positive
		if idx >= 0 && int(idx) < len(live) {
			return live[idx]
		}
	}
	return true // conservative: assume live
}
