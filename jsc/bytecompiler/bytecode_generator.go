// Package bytecompiler provides the bytecode generator.
// This file corresponds to WebKit BytecodeGenerator.h.
//
// BytecodeGenerator translates AST nodes into bytecode instructions.
// It is the central class of the bytecompiler module.

package bytecompiler

import (
	"wb-ui/jsc/runtime"
)

// ----- Enums -----

type ExpectedFunction int

const (
	NoExpectedFunction          ExpectedFunction = iota
	ExpectObjectConstructor
	ExpectArrayConstructor
)

type EmitAwait bool
const (
	EmitAwaitNo  EmitAwait = false
	EmitAwaitYes EmitAwait = true
)

type DebuggableCall bool
const (
	DebuggableCallNo  DebuggableCall = false
	DebuggableCallYes DebuggableCall = true
)

type ThisResolutionType int
const (
	ThisResolutionLocal  ThisResolutionType = 0
	ThisResolutionScoped ThisResolutionType = 1
)

type InvalidPrototypeMode uint8
const (
	InvalidPrototypeThrow  InvalidPrototypeMode = 0
	InvalidPrototypeIgnore InvalidPrototypeMode = 1
)

type CompletionType int
const (
	CompletionNormal       CompletionType = 0
	CompletionThrow        CompletionType = 1
	CompletionReturn       CompletionType = 2
	CompletionNumberOfTypes CompletionType = 3
)

func BytecodeOffsetToJumpID(offset int) CompletionType {
	return CompletionType(int(CompletionNumberOfTypes) + offset)
}

// ----- Nested Types -----

type CallArguments struct {
	argumentsNode  interface{}
	argv           []*RegisterID
	allocatedRegs  []*RegisterID
}

func NewCallArguments(generator *BytecodeGenerator, argumentsNode interface{}, additionalArguments int) *CallArguments {
	ca := &CallArguments{
		argumentsNode: argumentsNode,
	}
	thisReg := generator.NewTemporary()
	ca.argv = append(ca.argv, thisReg)
	_ = additionalArguments
	return ca
}

func (c *CallArguments) ThisRegister() *RegisterID                     { return c.argv[0] }
func (c *CallArguments) ArgumentRegister(i int) *RegisterID            { return c.argv[i+1] }
func (c *CallArguments) StackOffset() int                              { return -c.argv[0].Index() + CallFrameHeaderSizeInRegisters }
func (c *CallArguments) ArgumentCountIncludingThis() int               { return len(c.argv) }

const CallFrameHeaderSizeInRegisters = 6

type VariableKind int
const (
	NormalVariable  VariableKind = 0
	SpecialVariable VariableKind = 1
)

type Variable struct {
	ident                    string
	offset                   *VarOffset
	local                    *RegisterID
	attributes               uint8
	kind                     VariableKind
	symbolTableConstantIndex int
	isLexicallyScoped        bool
}

func NewVariable(ident string) Variable {
	return Variable{ident: ident}
}

func NewVariableFull(ident string, offset *VarOffset, local *RegisterID, attributes uint8, kind VariableKind, symbolTableConstantIndex int, isLexicallyScoped bool) Variable {
	return Variable{
		ident:                    ident,
		offset:                   offset,
		local:                    local,
		attributes:               attributes,
		kind:                     kind,
		symbolTableConstantIndex: symbolTableConstantIndex,
		isLexicallyScoped:        isLexicallyScoped,
	}
}

func (v *Variable) IsResolved() bool                     { return v.offset != nil }
func (v *Variable) SymbolTableConstantIndex() int        { return v.symbolTableConstantIndex }
func (v *Variable) Ident() string                       { return v.ident }
func (v *Variable) Offset() *VarOffset                  { return v.offset }
func (v *Variable) IsLocal() bool                       { return v.offset != nil && v.offset.IsStack() }
func (v *Variable) Local() *RegisterID                  { return v.local }
func (v *Variable) IsReadOnly() bool                    { return v.attributes&uint8(runtime.PropertyAttributeReadOnly) != 0 }
func (v *Variable) IsSpecial() bool                     { return v.kind != NormalVariable }
func (v *Variable) IsConst() bool                       { return v.IsReadOnly() && v.isLexicallyScoped }
func (v *Variable) SetIsReadOnly()                      { v.attributes |= uint8(runtime.PropertyAttributeReadOnly) }

type VarOffsetKind int
const (
	VarOffsetStack     VarOffsetKind = 0
	VarOffsetScope     VarOffsetKind = 1
	VarOffsetDirectArg VarOffsetKind = 2
	VarOffsetCaptured  VarOffsetKind = 3
)

type VarOffset struct {
	kind  VarOffsetKind
	value int
}

func NewVarOffset(kind VarOffsetKind, value int) *VarOffset {
	return &VarOffset{kind: kind, value: value}
}
func (v *VarOffset) IsStack() bool { return v.kind == VarOffsetStack }

type FinallyJump struct {
	JumpID               CompletionType
	TargetLexicalScopeIndex int
	TargetLabel          *Label
}

func NewFinallyJump(jumpID CompletionType, targetLexicalScopeIndex int, targetLabel *Label) *FinallyJump {
	return &FinallyJump{JumpID: jumpID, TargetLexicalScopeIndex: targetLexicalScopeIndex, TargetLabel: targetLabel}
}

type FinallyContext struct {
	outerContext      *FinallyContext
	finallyLabel      *Label
	numberOfBreaks    int
	handlesReturns    bool
	jumps             []*FinallyJump
	completionTypeReg *RegisterID
	completionValueReg *RegisterID
}

func NewFinallyContext(generator *BytecodeGenerator, finallyLabel *Label) *FinallyContext {
	return &FinallyContext{finallyLabel: finallyLabel}
}

func (f *FinallyContext) OuterContext() *FinallyContext                 { return f.outerContext }
func (f *FinallyContext) FinallyLabel() *Label                         { return f.finallyLabel }
func (f *FinallyContext) CompletionTypeRegister() *RegisterID          { return f.completionTypeReg }
func (f *FinallyContext) CompletionValueRegister() *RegisterID         { return f.completionValueReg }
func (f *FinallyContext) NumberOfBreaksOrContinues() int               { return f.numberOfBreaks }
func (f *FinallyContext) IncNumberOfBreaksOrContinues()                { f.numberOfBreaks++ }
func (f *FinallyContext) HandlesReturns() bool                         { return f.handlesReturns }
func (f *FinallyContext) SetHandlesReturns()                          { f.handlesReturns = true }
func (f *FinallyContext) RegisterJump(jumpID CompletionType, lexicalScopeIndex int, targetLabel *Label) {
	f.jumps = append(f.jumps, NewFinallyJump(jumpID, lexicalScopeIndex, targetLabel))
}
func (f *FinallyContext) NumberOfJumps() int    { return len(f.jumps) }
func (f *FinallyContext) Jump(i int) *FinallyJump { return f.jumps[i] }

const (
	ControlFlowLabel   int = 0
	ControlFlowFinally int = 1
)

type ControlFlowScope struct {
	Type              int
	LexicalScopeIndex int
	FinallyCtx        *FinallyContext
}
func (s *ControlFlowScope) IsLabelScope() bool  { return s.Type == ControlFlowLabel }
func (s *ControlFlowScope) IsFinallyScope() bool { return s.Type == ControlFlowFinally }

type ForInInstTuple struct {
	InstIndex        int
	PropertyRegIndex int
}

type ForInHasOwnPropertyJump struct {
	BranchInstIndex   int
	GenericPathTarget int
}

type ForInContext struct {
	localRegister             *RegisterID
	propertyName              *RegisterID
	propertyOffset            *RegisterID
	enumerator                *RegisterID
	mode                      *RegisterID
	baseVariable              *Variable
	bodyStartOffset           int
	isValid                   bool
	inInsts                   []ForInInstTuple
	getInsts                  []ForInInstTuple
	putInsts                  []ForInInstTuple
	hasOwnPropertyJumpInsts   []ForInHasOwnPropertyJump
}

func NewForInContext(local, propertyName, propertyOffset, enumerator, mode *RegisterID, baseVariable *Variable, bodyStartOffset int) *ForInContext {
	return &ForInContext{
		localRegister:   local,
		propertyName:    propertyName,
		propertyOffset:  propertyOffset,
		enumerator:      enumerator,
		mode:            mode,
		baseVariable:    baseVariable,
		bodyStartOffset: bodyStartOffset,
		isValid:         true,
	}
}
func (c *ForInContext) IsValid() bool                 { return c.isValid }
func (c *ForInContext) Invalidate()                   { c.isValid = false }
func (c *ForInContext) Local() *RegisterID            { return c.localRegister }
func (c *ForInContext) PropertyName() *RegisterID     { return c.propertyName }
func (c *ForInContext) PropertyOffset() *RegisterID   { return c.propertyOffset }
func (c *ForInContext) Enumerator() *RegisterID       { return c.enumerator }
func (c *ForInContext) Mode() *RegisterID             { return c.mode }
func (c *ForInContext) BaseVariable() *Variable       { return c.baseVariable }
func (c *ForInContext) BodyBytecodeStartOffset() int  { return c.bodyStartOffset }
func (c *ForInContext) AddGetInst(instIndex, propertyRegIndex int) {
	c.getInsts = append(c.getInsts, ForInInstTuple{instIndex, propertyRegIndex})
}
func (c *ForInContext) AddPutInst(instIndex, propertyRegIndex int) {
	c.putInsts = append(c.putInsts, ForInInstTuple{instIndex, propertyRegIndex})
}
func (c *ForInContext) AddInInst(instIndex, propertyRegIndex int) {
	c.inInsts = append(c.inInsts, ForInInstTuple{instIndex, propertyRegIndex})
}
func (c *ForInContext) AddHasOwnPropertyJump(branchInstIndex, genericPathTarget int) {
	c.hasOwnPropertyJumpInsts = append(c.hasOwnPropertyJumpInsts, ForInHasOwnPropertyJump{branchInstIndex, genericPathTarget})
}

type TryData struct {
	Target      *Label
	HandlerType int
}
func NewTryData(target *Label, handlerType int) *TryData { return &TryData{Target: target, HandlerType: handlerType} }

type TryContext struct {
	Start   *Label
	TryData *TryData
}

type TryRange struct {
	Start   *Label
	End     *Label
	TryData *TryData
}

type UsingSlot struct {
	Value   *RegisterID
	Method  *RegisterID
	Reached *RegisterID
	IsAsync bool
}

type UsingScope struct {
	Slots         []UsingSlot
	NextSlot      int
	HasAwaitUsing bool
}

type ScopeType uint8
const (
	CatchScope                    ScopeType = 0
	CatchScopeWithSimpleParameter ScopeType = 1
	LetConstScope                 ScopeType = 2
	FunctionNameScope             ScopeType = 3
	ClassScope                    ScopeType = 4
)

type TDZCheckOptimization int
const (
	TDZCheckOptimize      TDZCheckOptimization = 0
	TDZCheckDoNotOptimize TDZCheckOptimization = 1
)

type NestedScopeType int
const (
	NestedScopeIsNested    NestedScopeType = 0
	NestedScopeIsNotNested NestedScopeType = 1
)

const (
	PropertyConfigurable = 1
	PropertyWritable     = 1 << 1
	PropertyEnumerable   = 1 << 2
)

type FunctionInitEntry struct {
	Node    interface{} // FunctionMetadataNode*
	VarType int
}

// ----- BytecodeGenerator -----

type BytecodeGenerator struct {
	BytecodeGeneratorBase
	vm      *runtime.VM
	scopeNode interface{} // ScopeNode*

	thisRegister                       RegisterID
	calleeRegister                     RegisterID
	scopeRegister                      *RegisterID
	topLevelScopeRegister              *RegisterID
	argumentsRegister                  *RegisterID
	lexicalEnvironmentRegister         *RegisterID
	generatorRegister                  *RegisterID
	emptyValueRegister                 *RegisterID
	newTargetRegister                  *RegisterID
	isDerivedConstructor               *RegisterID
	arrowFunctionCtxLexEnvRegister     *RegisterID
	promiseRegister                    *RegisterID

	currentFinallyContext              *FinallyContext
	parameters                         []*RegisterID
	labelScopes                        []*LabelScope
	constantPoolRegisters              []*RegisterID
	finallyDepth                       int
	localScopeDepth                    int
	localScopeCount                    int
	codeType                           int
	controlFlowScopeStack              []*ControlFlowScope
	switchContextStack                 []interface{}
	forInContextStack                  []*ForInContext
	tryContextStack                    []*TryContext
	usingScopeStack                    []*UsingScope
	yieldPoints                        int
	needsGeneratorification            bool
	generatorFrameSymbolTable          interface{}
	generatorFrameSymbolTableIndex     int
	functionsToInitialize              []FunctionInitEntry
	needToInitializeArguments          bool
	restParameter                      interface{}
	tryRanges                          []*TryRange
	tryData                            []*TryData
	optionalChainTargetStack           []*Label
	nextConstantOffset                 int
	identifierMap                      map[string]int
	jsValueMap                         map[uint64]int
	stringMap                          map[string]int
	bigIntMap                          map[string]int
	templateObjectDescriptorSet        map[interface{}]struct{}
	templateDescriptorMap              map[uint64]interface{}
	staticPropertyAnalyzer             *StaticPropertyAnalyzer

	codeGenerationMode                 int
	defaultAllowCallIgnoreResult       bool
	usesExceptions                     bool
	expressionTooDeep                  bool
	isBuiltinFunction                  bool
	isBuiltinDefaultClassConstructor   bool
	usesSloppyEval                     bool
	allowTailCallOptimization          bool
	allowCallIgnoreResultOpt           bool
	needsToUpdateArrowFunctionCtx      bool
	needsArguments                     bool
	ecmaMode                           int
	derivedContextType                 int
}

func NewBytecodeGenerator(vm *runtime.VM, scopeNode interface{}) *BytecodeGenerator {
	codeBlock := NewUnlinkedCodeBlockGenerator()
	g := &BytecodeGenerator{
		BytecodeGeneratorBase:  *NewBytecodeGeneratorBase(codeBlock, 0),
		vm:                    vm,
		scopeNode:             scopeNode,
		staticPropertyAnalyzer: NewStaticPropertyAnalyzer(),
		identifierMap:         make(map[string]int),
		jsValueMap:            make(map[uint64]int),
		stringMap:             make(map[string]int),
		bigIntMap:             make(map[string]int),
		templateObjectDescriptorSet: make(map[interface{}]struct{}),
		templateDescriptorMap:       make(map[uint64]interface{}),
	}
	return g
}

func (g *BytecodeGenerator) VM() *runtime.VM { return g.vm }

// Register management

func (g *BytecodeGenerator) IgnoredResult() *RegisterID {
	// Return a dummy register
	return &g.thisRegister
}

func (g *BytecodeGenerator) NewBlockScopeVariable() *RegisterID {
	return g.NewTemporary()
}

func (g *BytecodeGenerator) TempDestination(dst *RegisterID) *RegisterID {
	if dst != nil && dst != g.IgnoredResult() && dst.IsTemporary() {
		return dst
	}
	return g.NewTemporary()
}

func (g *BytecodeGenerator) FinalDestination(originalDst, tempDst *RegisterID) *RegisterID {
	if originalDst != nil && originalDst != g.IgnoredResult() {
		return originalDst
	}
	if tempDst != nil && tempDst != g.IgnoredResult() && tempDst.IsTemporary() {
		return tempDst
	}
	return g.NewTemporary()
}

func (g *BytecodeGenerator) DestinationForAssignResult(dst *RegisterID) *RegisterID {
	if dst != nil && dst != g.IgnoredResult() {
		if dst.IsTemporary() {
			return dst
		}
		return g.NewTemporary()
	}
	return nil
}

func (g *BytecodeGenerator) Move(dst, src *RegisterID) *RegisterID {
	if dst == g.IgnoredResult() {
		return nil
	}
	if dst != nil && dst != src {
		return g.EmitMove(dst, src)
	}
	return src
}

func (g *BytecodeGenerator) NewTemporaryOr(suggestion *RegisterID) *RegisterID {
	if suggestion.IsTemporary() {
		return suggestion
	}
	return g.NewTemporary()
}

// Scope register accessors

func (g *BytecodeGenerator) ScopeRegister() *RegisterID                 { return g.scopeRegister }
func (g *BytecodeGenerator) ThisRegister() *RegisterID                  { return &g.thisRegister }
func (g *BytecodeGenerator) ArgumentsRegister() *RegisterID             { return g.argumentsRegister }
func (g *BytecodeGenerator) NewTarget() *RegisterID                    { return g.newTargetRegister }
func (g *BytecodeGenerator) GeneratorRegister() *RegisterID            { return g.generatorRegister }
func (g *BytecodeGenerator) PromiseRegister() *RegisterID              { return g.promiseRegister }

// LabelScope

func (g *BytecodeGenerator) NewLabelScope(typ LabelScopeType, name *string) *LabelScope {
	scope := NewLabelScope(typ, name, g.labelScopeDepth(), g.NewLabel(), nil)
	if typ == LabelScopeTypeLoop {
		scope.continueTarget = g.NewLabel()
	}
	g.labelScopes = append(g.labelScopes, scope)
	return scope
}

func (g *BytecodeGenerator) PushFinallyControlFlowScope(ctx *FinallyContext) {
	g.currentFinallyContext = ctx
	g.controlFlowScopeStack = append(g.controlFlowScopeStack, &ControlFlowScope{
		Type: ControlFlowFinally, FinallyCtx: ctx,
	})
}

func (g *BytecodeGenerator) PopFinallyControlFlowScope() {
	if len(g.controlFlowScopeStack) > 0 {
		g.controlFlowScopeStack = g.controlFlowScopeStack[:len(g.controlFlowScopeStack)-1]
	}
	if len(g.controlFlowScopeStack) > 0 {
		top := g.controlFlowScopeStack[len(g.controlFlowScopeStack)-1]
		if top.IsFinallyScope() {
			g.currentFinallyContext = top.FinallyCtx
		} else {
			g.currentFinallyContext = nil
		}
	} else {
		g.currentFinallyContext = nil
	}
}

func (g *BytecodeGenerator) HasFinallyScopes() bool { return g.currentFinallyContext != nil }

// Emit stubs

func (g *BytecodeGenerator) EmitMove(dst, src *RegisterID) *RegisterID  { return dst }
func (g *BytecodeGenerator) EmitLoad(dst *RegisterID, value interface{}) *RegisterID { return dst }
func (g *BytecodeGenerator) EmitUnaryOp(opcodeID int, dst, src *RegisterID, resultType int) *RegisterID { return dst }
func (g *BytecodeGenerator) EmitBinaryOp(opcodeID int, dst, src1, src2 *RegisterID, types int) *RegisterID { return dst }
func (g *BytecodeGenerator) EmitNode(dst *RegisterID, node interface{}) {}
func (g *BytecodeGenerator) EmitNodeInTailPosition(dst *RegisterID, node interface{}) *RegisterID { return dst }
func (g *BytecodeGenerator) EmitNodeForProperty(dst *RegisterID, node interface{}) *RegisterID { return dst }
func (g *BytecodeGenerator) EmitJump(target *Label)                      {}
func (g *BytecodeGenerator) EmitJumpIfTrue(cond *RegisterID, target *Label) {}
func (g *BytecodeGenerator) EmitJumpIfFalse(cond *RegisterID, target *Label) {}
func (g *BytecodeGenerator) EmitThrow(val *RegisterID)                  {}
func (g *BytecodeGenerator) EmitReturn(src *RegisterID) *RegisterID     { return src }

func (g *BytecodeGenerator) EmitCall(dst, fn *RegisterID, expected ExpectedFunction, args *CallArguments, divot, divotStart, divotEnd int, debuggable DebuggableCall) *RegisterID {
	return dst
}

func (g *BytecodeGenerator) EmitCallInTailPosition(dst, fn *RegisterID, expected ExpectedFunction, args *CallArguments, divot, divotStart, divotEnd int, debuggable DebuggableCall) *RegisterID {
	return dst
}

func (g *BytecodeGenerator) EmitConstruct(dst, fn, lazyThis *RegisterID, expected ExpectedFunction, args *CallArguments, divot, divotStart, divotEnd int) *RegisterID {
	return dst
}

func (g *BytecodeGenerator) EmitGetById(dst, base *RegisterID, property string) *RegisterID   { return dst }
func (g *BytecodeGenerator) EmitPutById(base *RegisterID, property string, value *RegisterID) *RegisterID { return value }
func (g *BytecodeGenerator) EmitGetByVal(dst, base, property *RegisterID) *RegisterID         { return dst }
func (g *BytecodeGenerator) EmitPutByVal(base, property, value *RegisterID) *RegisterID        { return value }
func (g *BytecodeGenerator) EmitNewObject(dst *RegisterID) *RegisterID                         { return dst }
func (g *BytecodeGenerator) EmitNewArray(dst *RegisterID, elements interface{}, length int, indexingType int) *RegisterID { return dst }
func (g *BytecodeGenerator) EmitNewFunction(dst *RegisterID, metadata interface{}) *RegisterID  { return dst }
func (g *BytecodeGenerator) EmitNewRegExp(dst *RegisterID, regExp interface{}) *RegisterID     { return dst }

func (g *BytecodeGenerator) EmitProfileType(reg *RegisterID, flag ProfileTypeBytecodeFlag) {}
func (g *BytecodeGenerator) EmitProfileTypeWithPosition(reg *RegisterID, flag ProfileTypeBytecodeFlag, startPos, endPos int) {}
func (g *BytecodeGenerator) EmitProfileControlFlow(id int) {}
func (g *BytecodeGenerator) EmitLoopHint() {}
func (g *BytecodeGenerator) EmitEnter() {}
func (g *BytecodeGenerator) EmitThrowStaticError(errorType int, reg *RegisterID) {}
func (g *BytecodeGenerator) EmitThrowTypeError(message string) {}
func (g *BytecodeGenerator) EmitThrowRangeError(message string) {}
func (g *BytecodeGenerator) EmitThrowOutOfMemoryError() {}

func (g *BytecodeGenerator) EmitThrowExpressionTooDeepException() *RegisterID {
	return g.IgnoredResult()
}

func (g *BytecodeGenerator) EmitSuperSamplerBegin() {}
func (g *BytecodeGenerator) EmitSuperSamplerEnd()   {}
func (g *BytecodeGenerator) EmitDebugHook(hookType int, position int) {}
func (g *BytecodeGenerator) EmitExpressionInfo(divot, divotStart, divotEnd int) {}

// Label scope depth
func (g *BytecodeGenerator) labelScopeDepth() int { return len(g.labelScopes) }

// Completion type helpers
func (g *BytecodeGenerator) EmitLoadCompletionType(dst *RegisterID, typ CompletionType) *RegisterID {
	return g.EmitLoad(dst, int(typ))
}

// Variable handling

func (g *BytecodeGenerator) Variable(ident string, resolveType ThisResolutionType) Variable {
	return NewVariable(ident)
}

func (g *BytecodeGenerator) CreateVariable(ident string, varKind int, symbolTable interface{}, mode int) {}

func (g *BytecodeGenerator) Generate(size *int) error {
	*size = g.writer.Position()
	return nil
}

func (g *BytecodeGenerator) HasConstant(ident string) bool {
	_, ok := g.identifierMap[ident]
	return ok
}

func (g *BytecodeGenerator) AddConstant(ident string) int {
	if idx, ok := g.identifierMap[ident]; ok {
		return idx
	}
	idx := g.nextConstantOffset
	g.identifierMap[ident] = idx
	g.nextConstantOffset++
	return idx
}

func (g *BytecodeGenerator) AddConstantValue(value runtime.JSValue, repr int) *RegisterID {
	reg := NewRegisterID()
	g.constantPoolRegisters = append(g.constantPoolRegisters, reg)
	return reg
}

// Ensure BytecodeGenerator implements the Label's generator interface
func (g *BytecodeGenerator) Writer() *InstructionStreamWriter {
	return g.writer
}
