// JSAsyncGenerator corresponds to JSC::JSAsyncGenerator.
package runtime

// JSAsyncGenerator corresponds to JSC::JSAsyncGenerator.
// Holds internal state for an async generator function execution.
// Full implementation requires the interpreter + Promise system.
type JSAsyncGenerator struct {
	JSObject
	generatorState int32
	generatorValue JSValue
	resumeMode     int32
	frame          JSValue
	queue          []JSValue // Promise queue
}

const AsyncGeneratorStructureFlags uint32 = JSObjectStructureFlags

const (
	AsyncGeneratorStateInit          int32 = 0
	AsyncGeneratorStateCompleted     int32 = 1
	AsyncGeneratorStateExecuting     int32 = 2
	AsyncGeneratorStateDrainingQueue int32 = 3
	AsyncGeneratorStateSuspendedStart int32 = 4
	AsyncGeneratorStateSuspendedYield int32 = 5
)

func NewJSAsyncGenerator(vm *VM, structure *Structure) *JSAsyncGenerator {
	g := &JSAsyncGenerator{
		generatorState: AsyncGeneratorStateSuspendedStart,
	}
	g.structureID = structure.structureID
	g.typ = JSAsyncGeneratorType
	g.cellState = DefinitelyWhite
	g.properties = make(map[string]JSValue)
	return g
}

func (g *JSAsyncGenerator) State() int32        { return g.generatorState }
func (g *JSAsyncGenerator) SetState(s int32)    { g.generatorState = s }
func (g *JSAsyncGenerator) Value() JSValue      { return g.generatorValue }
func (g *JSAsyncGenerator) SetValue(v JSValue)  { g.generatorValue = v }
func (g *JSAsyncGenerator) ResumeMode() int32   { return g.resumeMode }
func (g *JSAsyncGenerator) SetResumeMode(m int32) { g.resumeMode = m }
