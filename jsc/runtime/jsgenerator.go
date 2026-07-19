// JSGenerator corresponds to JSC::JSGenerator.
package runtime

// JSGenerator corresponds to JSC::JSGenerator.
// Holds internal state for a generator function execution.
// Full implementation requires the interpreter.
type JSGenerator struct {
	JSObject
	generatorState int32
	generatorValue JSValue
	resumeMode     int32
	frame          JSValue
}

const GeneratorStructureFlags uint32 = JSObjectStructureFlags

const (
	GeneratorResumeModeNormal int32 = 0
	GeneratorResumeModeReturn int32 = 1
	GeneratorResumeModeThrow  int32 = 2
)

const (
	GeneratorStateCompleted  int32 = -1
	GeneratorStateExecuting  int32 = -2
	GeneratorStateInit       int32 = 0
)

func NewJSGenerator(vm *VM, structure *Structure) *JSGenerator {
	g := &JSGenerator{
		generatorState: GeneratorStateInit,
	}
	g.structureID = structure.structureID
	g.typ = JSGeneratorType
	g.cellState = DefinitelyWhite
	g.properties = make(map[string]JSValue)
	return g
}

func (g *JSGenerator) State() int32        { return g.generatorState }
func (g *JSGenerator) SetState(s int32)    { g.generatorState = s }
func (g *JSGenerator) Value() JSValue      { return g.generatorValue }
func (g *JSGenerator) SetValue(v JSValue)  { g.generatorValue = v }
func (g *JSGenerator) ResumeMode() int32   { return g.resumeMode }
func (g *JSGenerator) SetResumeMode(m int32) { g.resumeMode = m }
