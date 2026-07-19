// InlineCallFrame - describes an inlined function call (DFG JIT)
package bytecode

type InlineCallFrame struct {
	codeBlock        *CodeBlock
	arguments        []VirtualRegister
	directCall       bool
	kind             CallMode
	argumentCount    int
}

type InlineCallFrameSet struct {
	frames []InlineCallFrame
}

func (s *InlineCallFrameSet) Add(frame InlineCallFrame) {
	s.frames = append(s.frames, frame)
}

func (s *InlineCallFrameSet) Size() int { return len(s.frames) }
