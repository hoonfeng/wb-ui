// DataFormat / SpeculatedType (JIT value representation)
package bytecode

type DataFormat uint8
const (
	DataFormatNone       DataFormat = 0
	DataFormatInt32      DataFormat = 1
	DataFormatDouble     DataFormat = 2
	DataFormatCell       DataFormat = 3
	DataFormatBoolean    DataFormat = 4
	DataFormatJS         DataFormat = 5
	DataFormatDead       DataFormat = 6
)

// SpeculatedType - DFG speculative types
type SpeculatedType uint64
const (
	SpecNone             SpeculatedType = 0
	SpecTop              SpeculatedType = SpeculatedType(^uint64(0))
	SpecInt32Only        SpeculatedType = 1 << 0
	SpecBoolean          SpeculatedType = 1 << 1
	SpecOther            SpeculatedType = 1 << 2
	SpecString           SpeculatedType = 1 << 3
	SpecObject           SpeculatedType = 1 << 4
	SpecDouble           SpeculatedType = 1 << 5
	SpecBytecodeTop      SpeculatedType = SpecInt32Only | SpecBoolean | SpecOther | SpecString | SpecObject | SpecDouble
)

// ValueProfile - profiling data for a value-producing instruction
type ValueProfile struct {
	observedValue uint64
	observedTypes SpeculatedType
}

func (p *ValueProfile) ObservedValue() uint64 { return p.observedValue }
func (p *ValueProfile) SetObservedValue(v uint64) { p.observedValue = v }

// LazyOperandValueProfile - lazy value profile for operands
type LazyOperandValueProfile struct {
	operand    VirtualRegister
	value      uint64
}

// LazyValueProfile - a lazily-computed value profile
type LazyValueProfile struct{}

// MethodOfGettingAValueProfile - describes how to get a value profile
type MethodOfGettingAValueProfile struct{}

// ValueRecovery - describes how to recover DFG values in the interpreter
type ValueRecovery struct {
	technique uint8
	data      uint64
}

const (
	RecoveryAlreadyInJSStack        = 1
	RecoveryAlreadyInJSStackAsUnboxedInt32 = 2
	RecoveryAlreadyInJSStackAsUnboxedDouble = 3
	RecoveryConstant                 = 4
	RecoveryDirectArgumentsThatWereNotCreated = 5
	RecoveryClonedArgumentsThatWereNotCreated = 6
)
