// ExecutionCounter / ExitFlag / ExitKind / ExitingInlineKind / ExitingJITType
package bytecode

type ExecutionCounter struct {
	count     uint32
	threshold uint32
}

func (c *ExecutionCounter) Count() uint32 { return c.count }
func (c *ExecutionCounter) Increment() { c.count++ }
func (c *ExecutionCounter) HasCrossedThreshold() bool { return c.count >= c.threshold }

type ExitFlag struct{ bits uint8 }
type ExitKind uint8
const (
	ExitKindNormal             ExitKind = 0
	ExitKindOSRExit            ExitKind = 1
	ExitKindInvalidation       ExitKind = 2
	ExitKindWatchpointFire     ExitKind = 3
)

type ExitingInlineKind uint8
type ExitingJITType uint8

const ExitingJITTypeInterpreter ExitingJITType = 0

// DFGExitProfile - exit profiling for DFG JIT
type DFGExitProfile struct{}
