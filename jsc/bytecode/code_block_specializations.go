// DirectEvalCodeCache - cache for direct eval() calls
package bytecode



type DirectEvalCodeCache struct {
	cache map[string]*UnlinkedCodeBlock
}

func NewDirectEvalCodeCache() *DirectEvalCodeCache {
	return &DirectEvalCodeCache{cache: make(map[string]*UnlinkedCodeBlock)}
}

func (c *DirectEvalCodeCache) Get(source string) *UnlinkedCodeBlock {
	return c.cache[source]
}

func (c *DirectEvalCodeCache) Put(source string, codeBlock *UnlinkedCodeBlock) {
	c.cache[source] = codeBlock
}

func (c *DirectEvalCodeCache) IsEmpty() bool { return len(c.cache) == 0 }

// EvalCodeBlock / FunctionCodeBlock / GlobalCodeBlock / ModuleProgramCodeBlock / ProgramCodeBlock
// These are specializations of CodeBlock for different code types.

type EvalCodeBlock struct {
	CodeBlock
}

func NewEvalCodeBlock(kind CodeSpecializationKind, owner CodeBlockOwner) *EvalCodeBlock {
	cb := NewCodeBlock(EvalCode, kind, owner)
	return &EvalCodeBlock{CodeBlock: *cb}
}

type FunctionCodeBlock struct {
	CodeBlock
}
func NewFunctionCodeBlock(kind CodeSpecializationKind, owner CodeBlockOwner) *FunctionCodeBlock {
	cb := NewCodeBlock(FunctionCode, kind, owner)
	return &FunctionCodeBlock{CodeBlock: *cb}
}

type GlobalCodeBlock struct {
	CodeBlock
}
func NewGlobalCodeBlock(kind CodeSpecializationKind, owner CodeBlockOwner) *GlobalCodeBlock {
	cb := NewCodeBlock(GlobalCode, kind, owner)
	return &GlobalCodeBlock{CodeBlock: *cb}
}

type ModuleProgramCodeBlock struct {
	CodeBlock
}
func NewModuleProgramCodeBlock(kind CodeSpecializationKind, owner CodeBlockOwner) *ModuleProgramCodeBlock {
	cb := NewCodeBlock(ModuleCode, kind, owner)
	return &ModuleProgramCodeBlock{CodeBlock: *cb}
}

type ProgramCodeBlock struct {
	CodeBlock
}
func NewProgramCodeBlock(kind CodeSpecializationKind, owner CodeBlockOwner) *ProgramCodeBlock {
	cb := NewCodeBlock(GlobalCode, kind, owner)
	return &ProgramCodeBlock{CodeBlock: *cb}
}

// Unlinked versions
type UnlinkedEvalCodeBlock struct{ UnlinkedCodeBlock }
type UnlinkedFunctionCodeBlock struct{ UnlinkedCodeBlock }
type UnlinkedGlobalCodeBlock struct{ UnlinkedCodeBlock }
type UnlinkedModuleProgramCodeBlock struct{ UnlinkedCodeBlock }
type UnlinkedProgramCodeBlock struct{ UnlinkedCodeBlock }

type UnlinkedFunctionExecutable struct {
	name            string
	inferredName    string
	sourceID        SourceID
	sourceURL       string
	lineCount       uint32
	unlinkedCodeBlock *UnlinkedCodeBlock
}

func NewUnlinkedFunctionExecutable(name string) *UnlinkedFunctionExecutable {
	return &UnlinkedFunctionExecutable{name: name}
}

func (e *UnlinkedFunctionExecutable) Name() string { return e.name }
func (e *UnlinkedFunctionExecutable) InferredName() string { return e.inferredName }
func (e *UnlinkedFunctionExecutable) SetInferredName(n string) { e.inferredName = n }
func (e *UnlinkedFunctionExecutable) LinkCodeBlock(cb *CodeBlock) {
	// Link unlinked code block data to the code block
	// (simplified - just copy constants)
}
