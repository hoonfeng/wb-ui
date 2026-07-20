// Parser.h Go 翻译
package parser

// 枚举定义
type SourceElementsMode int; const (CheckForStrictMode SourceElementsMode = 0; DontCheckForStrictMode SourceElementsMode = 1)
type FunctionBodyType int; const (ArrowFunctionBodyExpression FunctionBodyType = iota; ArrowFunctionBodyBlock; StandardFunctionBodyBlock)
type FunctionNameRequirements int; const (NameNone FunctionNameRequirements = iota; NameNamed; NameUnnamed)
type DestructuringKind int; const (DestructureToVariables DestructuringKind = iota; DestructureToLet; DestructureToConst; DestructureToCatchParameters; DestructureToParameters; DestructureToExpressions)
type DeclarationType int; const (VarDeclaration DeclarationType = iota; LetDeclaration; ConstDeclaration; UsingDeclaration; AwaitUsingDeclaration)
type DeclarationImportType int; const (NotImported DeclarationImportType = iota; Imported; ImportedNamespace)
type DeclarationResult uint8; const (Valid DeclarationResult = 0; InvalidStrictMode DeclarationResult = 1<<0; InvalidDuplicateDeclaration DeclarationResult = 1<<1; InvalidPrivateStaticNonStatic DeclarationResult = 1<<2)
type DeclarationResultMask = uint8
type DeclarationDefaultContext int; const (Standard DeclarationDefaultContext = iota; ExportDefault)
type InferName int; const (InferAllowed InferName = iota; InferDisallowed)

type ScopeLabelInfo struct { UID string; IsLoop bool }
var GlobalParseCount uint32

// Scope
type Scope struct {
	ContainingScope *Scope; FunctionDeclarations []*FunctionMetadataNode
	shadowsArgs, usesEval, usesImportMeta, needsFullActivation bool
	hasDirectSuper, needsSuperBinding, allowsVarDecls, allowsLexDecls bool
	IsFunction, IsGeneratorFunction, IsGeneratorFunctionBoundary bool
	isArrowFn, isArrowFnBoundary bool
	IsAsyncFn, IsAsyncFnBoundary bool
	isLexScope, IsGlobalCode, IsModuleCode bool
	IsSimpleCatchParamScope, IsCatchBlockScope bool
	IsStaticBlock, isStaticBlockBoundary, IsFunctionBoundary bool
	isValidStrictMode, HasArgs, IsEvalCtx, hasNonSimpleParamList bool
	IsClassScope, asyncFnBodyNoAwait, usesAwait bool
	LoopDepth int
	ExpectedSuperBinding SuperBinding
	ImplementationVisibility ImplementationVisibility
	lexFeatures LexicallyScopedFeatures
	ConstructorKind ConstructorKind
	innerArrowFnFeatures InnerArrowFunctionCodeFeatures
	SloppyModeFnCandidates map[*FunctionMetadataNode]bool
	ClosedVarCandidates map[string]struct{}
	LexicalVariables *VariableEnvironment; DeclaredVariables *VariableEnvironment
	DeclaredParameters map[string]struct{}; VarsBeingHoisted map[string]struct{}
	Labels []ScopeLabelInfo; SwitchDepth int
	EvalContextType EvalContextType; DerivedContextType DerivedContextType
	UsedVariables []map[string]struct{}
}

func NewScope(cs *Scope, iv ImplementationVisibility, l LexicallyScopedFeatures, fn, gn, ar, ay, sb bool) *Scope {
	s := &Scope{ContainingScope: cs, IsFunction: fn, IsGeneratorFunction: gn, isArrowFn: ar, IsAsyncFn: ay, IsStaticBlock: sb,
		ImplementationVisibility: iv, lexFeatures: l, allowsVarDecls: true, allowsLexDecls: true, isValidStrictMode: true,
		LexicalVariables: NewVariableEnvironment(), DeclaredVariables: NewVariableEnvironment(),
		DeclaredParameters: make(map[string]struct{}), VarsBeingHoisted: make(map[string]struct{}),
		ClosedVarCandidates: make(map[string]struct{}), SloppyModeFnCandidates: make(map[*FunctionMetadataNode]bool)}
	s.UsedVariables = append(s.UsedVariables, make(map[string]struct{})); return s
}

// Getters
func (s *Scope) UsesEval() bool { return s.usesEval }
func (s *Scope) UsesImportMeta() bool { return s.usesImportMeta }
func (s *Scope) UsesAwait() bool { return s.usesAwait }
func (s *Scope) SetUsesAwait() { s.usesAwait = true }
func (s *Scope) SetUsesImportMeta() { s.usesImportMeta = true }
func (s *Scope) SetNeedsFullActivation() { s.needsFullActivation = true }
func (s *Scope) NeedsFullActivation() bool { return s.needsFullActivation }
func (s *Scope) HasDirectSuper() bool { return s.hasDirectSuper }
func (s *Scope) SetHasDirectSuper() { s.hasDirectSuper = true }
func (s *Scope) NeedsSuperBinding() bool { return s.needsSuperBinding }
func (s *Scope) SetNeedsSuperBinding() { s.needsSuperBinding = true }
func (s *Scope) ShadowsArguments() bool { return s.shadowsArgs }
func (s *Scope) IsArrowFunction() bool { return s.isArrowFn }
func (s *Scope) IsArrowFunctionBoundary() bool { return s.isArrowFnBoundary }
func (s *Scope) IsLexicalScope() bool { return s.isLexScope }
func (s *Scope) IsStaticBlockBoundary() bool { return s.isStaticBlockBoundary }
func (s *Scope) IsPrivateNameScope() bool { return s.IsClassScope }
func (s *Scope) IsValidStrictMode() bool { return s.isValidStrictMode }
func (s *Scope) HasNonSimpleParameterList() bool { return s.hasNonSimpleParamList }
func (s *Scope) AsyncFunctionBodyDoesNotUseAwait() bool { return s.asyncFnBodyNoAwait }
func (s *Scope) SetAsyncFunctionBodyDoesNotUseAwait() { s.asyncFnBodyNoAwait = true }
func (s *Scope) AllowsVarDeclarations() bool { return s.allowsVarDecls }
func (s *Scope) AllowsLexicalDeclarations() bool { return s.allowsLexDecls }
func (s *Scope) LexicallyScopedFeatures() LexicallyScopedFeatures { return s.lexFeatures }
func (s *Scope) InnerArrowFunctionFeatures() InnerArrowFunctionCodeFeatures { return s.innerArrowFnFeatures }
func (s *Scope) StrictMode() bool { return (s.lexFeatures & StrictModeLexicallyScopedFeature) != 0 }
func (s *Scope) SetImplementationVisibility(v ImplementationVisibility) { s.ImplementationVisibility = v }
func (s *Scope) ResetImplementationVisibility() { s.ImplementationVisibility = ImplementationVisibilityPublic }
func (s *Scope) SetLexicallyScopedFeatures(f LexicallyScopedFeatures) { s.lexFeatures = f }
func (s *Scope) SetStrictMode() { s.lexFeatures |= StrictModeLexicallyScopedFeature }
func (s *Scope) SetHasNonSimpleParameterList() { s.isValidStrictMode = false; s.hasNonSimpleParamList = true }
func (s *Scope) HasUsingDeclaration() bool { return s.LexicalVariables != nil && s.LexicalVariables.HasUsingDeclaration() }
func (s *Scope) StartSwitch() { s.SwitchDepth++ }
func (s *Scope) EndSwitch() { s.SwitchDepth-- }
func (s *Scope) StartLoop() { s.LoopDepth++ }
func (s *Scope) EndLoop() { s.LoopDepth-- }
func (s *Scope) InLoop() bool { return s.LoopDepth > 0 }
func (s *Scope) BreakIsValid() bool { return s.LoopDepth > 0 || s.SwitchDepth > 0 }
func (s *Scope) ContinueIsValid() bool { return s.LoopDepth > 0 }
func (s *Scope) HasContainingScope() bool { return s.ContainingScope != nil && !s.IsFunctionBoundary }
func (s *Scope) PushLabel(l *ScopeLabelInfo) { s.Labels = append(s.Labels, *l) }
func (s *Scope) PopLabel() { if len(s.Labels) > 0 { s.Labels = s.Labels[:len(s.Labels)-1] } }
func (s *Scope) GetLabel(uid string) *ScopeLabelInfo { for i := len(s.Labels)-1; i >= 0; i-- { if s.Labels[i].UID == uid { return &s.Labels[i] } }; return nil }

func (s *Scope) SetSourceParseMode(mode SourceParseMode) {
	switch mode {
	case AsyncGeneratorBodyMode: s.setIsAsyncGeneratorFunctionBody()
	case AsyncArrowFunctionBodyMode: s.setIsAsyncArrowFunctionBody()
	case AsyncFunctionBodyMode: s.setIsAsyncFunctionBody()
	case GeneratorBodyMode: s.setIsGeneratorFunctionBody()
	case GeneratorWrapperFunctionMode, GeneratorWrapperMethodMode: s.setIsGeneratorFunction()
	case AsyncGeneratorWrapperMethodMode, AsyncGeneratorWrapperFunctionMode: s.setIsAsyncGeneratorFunction()
	case NormalFunctionMode, GetterMode, SetterMode, MethodMode, ClassFieldInitializerMode: s.setIsFunction()
	case ClassStaticBlockMode: s.setIsFunction(); s.setIsStaticBlock()
	case ArrowFunctionMode: s.setIsArrowFunction()
	case AsyncFunctionMode, AsyncMethodMode: s.setIsAsyncFunction()
	case AsyncArrowFunctionMode: s.setIsAsyncArrowFunction()
	case ProgramMode: s.setIsGlobalCode()
	case ModuleAnalyzeMode, ModuleEvaluateMode: s.setIsModuleCode()
	}
}

func (s *Scope) setIsFunction() {
	s.IsFunction = true; s.IsFunctionBoundary = true; s.HasArgs = true; s.isLexScope = true
	s.IsGeneratorFunction = false; s.IsGeneratorFunctionBoundary = false; s.isArrowFnBoundary = false
	s.isArrowFn = false; s.IsAsyncFn = false; s.IsAsyncFnBoundary = false; s.IsStaticBlock = false; s.isStaticBlockBoundary = false
}
func (s *Scope) setIsGeneratorFunction() { s.setIsFunction(); s.IsGeneratorFunction = true }
func (s *Scope) setIsGeneratorFunctionBody() { s.setIsFunction(); s.HasArgs = false; s.IsGeneratorFunction = true; s.IsGeneratorFunctionBoundary = true }
func (s *Scope) setIsArrowFunction() { s.setIsFunction(); s.isArrowFnBoundary = true; s.isArrowFn = true }
func (s *Scope) setIsAsyncArrowFunction() { s.setIsArrowFunction(); s.IsAsyncFn = true }
func (s *Scope) setIsAsyncFunction() { s.setIsFunction(); s.IsAsyncFn = true }
func (s *Scope) setIsAsyncGeneratorFunction() { s.setIsFunction(); s.IsAsyncFn = true; s.IsGeneratorFunction = true }
func (s *Scope) setIsAsyncGeneratorFunctionBody() { s.setIsFunction(); s.HasArgs = false; s.IsGeneratorFunction = true; s.IsGeneratorFunctionBoundary = true; s.IsAsyncFn = true; s.IsAsyncFnBoundary = true }
func (s *Scope) setIsAsyncFunctionBody() { s.setIsFunction(); s.HasArgs = false; s.IsAsyncFn = true; s.IsAsyncFnBoundary = true }
func (s *Scope) setIsAsyncArrowFunctionBody() { s.setIsArrowFunction(); s.HasArgs = false; s.IsAsyncFn = true; s.IsAsyncFnBoundary = true }
func (s *Scope) setIsGlobalCode() { s.IsGlobalCode = true }
func (s *Scope) setIsModuleCode() { s.setIsGlobalCode(); s.IsModuleCode = true }
func (s *Scope) setIsStaticBlock() { s.IsStaticBlock = true; s.isStaticBlockBoundary = true }

func (s *Scope) FinalizeLexicalEnvironment() {
	if s.usesEval || s.needsFullActivation { if s.LexicalVariables != nil { s.LexicalVariables.MarkAllVariablesAsCaptured() } }
	s.ComputeLexicallyCapturedVariablesAndPurgeCandidates()
}
func (s *Scope) TakeLexicalEnvironment() *VariableEnvironment { e := s.LexicalVariables; s.LexicalVariables = NewVariableEnvironment(); return e }
func (s *Scope) TakeDeclaredVariables() *VariableEnvironment { e := s.DeclaredVariables; s.DeclaredVariables = NewVariableEnvironment(); return e }
func (s *Scope) ComputeLexicallyCapturedVariablesAndPurgeCandidates() {
	if s.LexicalVariables != nil && s.LexicalVariables.Size() > 0 && len(s.ClosedVarCandidates) > 0 {
		for name := range s.ClosedVarCandidates { s.LexicalVariables.MarkVariableAsCapturedIfDefined(name) }
	}
}
func (s *Scope) DeclareCallee(name string) DeclarationResultMask { s.DeclaredVariables.Add(name); s.DeclaredVariables.ClearIsVar(name); return DeclarationResultMask(Valid) }
func (s *Scope) DeclareVariable(name string) DeclarationResultMask { s.DeclaredVariables.Add(name); s.DeclaredVariables.SetIsVar(name); return DeclarationResultMask(Valid) }
func (s *Scope) DeclareFunctionAsVar(name string) DeclarationResultMask {
	r := DeclarationResult(Valid); s.DeclaredVariables.Add(name); s.DeclaredVariables.SetIsVar(name); s.DeclaredVariables.SetIsFunction(name)
	if s.LexicalVariables != nil && s.LexicalVariables.Has(name) { r |= InvalidDuplicateDeclaration }; return DeclarationResultMask(r)
}
func (s *Scope) DeclareFunctionAsLet(name string, isFuncDecl bool) DeclarationResultMask {
	r := DeclarationResult(Valid); isNew := s.LexicalVariables.Add(name)
	s.LexicalVariables.SetIsLet(name); s.LexicalVariables.SetIsFunction(name)
	if isNew == nil && (s.StrictMode() || !s.LexicalVariables.IsFunctionDeclaration(name) || !isFuncDecl) { r |= InvalidDuplicateDeclaration }
	if s.DeclaredVariables.Has(name) || s.hasVarBeingHoisted(name) { r |= InvalidDuplicateDeclaration }
	if isFuncDecl { s.LexicalVariables.SetIsFunctionDeclaration(name) }; return DeclarationResultMask(r)
}
func (s *Scope) hasVarBeingHoisted(name string) bool { _, ok := s.VarsBeingHoisted[name]; return ok }
func (s *Scope) AddVariableBeingHoisted(name string) { s.VarsBeingHoisted[name] = struct{}{} }
func (s *Scope) AddSloppyModeFunctionHoistingCandidate(n *FunctionMetadataNode, _ bool) { s.SloppyModeFnCandidates[n] = true }
func (s *Scope) AppendFunction(n *FunctionMetadataNode) { s.FunctionDeclarations = append(s.FunctionDeclarations, n) }
func (s *Scope) TakeFunctionDeclarations() []*FunctionMetadataNode { d := s.FunctionDeclarations; s.FunctionDeclarations = nil; return d }
func (s *Scope) DeclareLexicalVariable(name string, isConst, isUsing, isAwaitUsing bool) DeclarationResultMask {
	r := DeclarationResult(Valid); isNew := s.LexicalVariables.Add(name)
	if isConst { s.LexicalVariables.SetIsConst(name) } else { s.LexicalVariables.SetIsLet(name) }
	if isUsing { s.LexicalVariables.SetIsUsing(name) }
	if isAwaitUsing { s.LexicalVariables.SetHasAwaitUsingDeclaration() }
	if isNew == nil || s.hasVarBeingHoisted(name) { r |= InvalidDuplicateDeclaration }; return DeclarationResultMask(r)
}
func (s *Scope) HasDeclaredVariable(name string) bool { e := s.DeclaredVariables.Find(name); return e != nil && e.IsVar() }
func (s *Scope) HasLexicallyDeclaredVariable(name string) bool { return s.LexicalVariables != nil && s.LexicalVariables.Has(name) }
func (s *Scope) HasVariableBeingHoisted(name string) bool { return s.hasVarBeingHoisted(name) }
func (s *Scope) HasPrivateName(name string) bool { return s.LexicalVariables != nil && s.LexicalVariables.HasPrivateName(name) }
func (s *Scope) DeclarePrivateMethod(name string, isStatic bool) DeclarationResultMask {
	ok := s.LexicalVariables.DeclarePrivateMethod(name)
	if isStatic { ok = s.LexicalVariables.DeclareStaticPrivateMethod(name) }
	if !ok { return DeclarationResultMask(InvalidDuplicateDeclaration) }; return DeclarationResultMask(Valid)
}
func (s *Scope) DeclarePrivateField(name string) DeclarationResultMask {
	if !s.LexicalVariables.DeclarePrivateField(name) { return DeclarationResultMask(InvalidDuplicateDeclaration) }; return DeclarationResultMask(Valid)
}
func (s *Scope) HasDeclaredParameter(name string) bool { _, ok := s.DeclaredParameters[name]; return ok || s.HasDeclaredVariable(name) }
func (s *Scope) PreventAllVariableDeclarations() { s.allowsVarDecls = false; s.allowsLexDecls = false }
func (s *Scope) PreventVarDeclarations() { s.allowsVarDecls = false }
func (s *Scope) DeclareParameter(name string, isStrictOK bool) DeclarationResultMask {
	r := DeclarationResult(Valid); isNew := s.DeclaredVariables.Add(name)
	dup := isNew == nil && s.DeclaredVariables.IsParameter(name); ok := !dup && isStrictOK
	s.DeclaredVariables.ClearIsVar(name); s.DeclaredVariables.SetIsParameter(name)
	s.isValidStrictMode = s.isValidStrictMode && ok; s.DeclaredParameters[name] = struct{}{}
	if !ok { r |= InvalidStrictMode }; if dup { r |= InvalidDuplicateDeclaration }; return DeclarationResultMask(r)
}
func (s *Scope) UsedVariablesContains(name string) bool {
	for _, set := range s.UsedVariables { if _, ok := set[name]; ok { return true } }; return false
}
func (s *Scope) UseVariable(name string, isEval bool) {
	s.usesEval = s.usesEval || isEval
	if len(s.UsedVariables) > 0 { s.UsedVariables[len(s.UsedVariables)-1][name] = struct{}{} }
}
func (s *Scope) PushUsedVariableSet() { s.UsedVariables = append(s.UsedVariables, make(map[string]struct{})) }
func (s *Scope) CurrentUsedVariablesSize() int { return len(s.UsedVariables) }
func (s *Scope) RevertToPreviousUsedVariables(sz int) { s.UsedVariables = s.UsedVariables[:sz] }
func (s *Scope) MarkLastUsedVariablesSetAsCaptured(from int) {
	for i := from; i < len(s.UsedVariables); i++ { for n := range s.UsedVariables[i] { s.ClosedVarCandidates[n] = struct{}{} } }
}
func (s *Scope) CollectFreeVariables(ns *Scope, track bool) {
	if ns.usesEval { s.usesEval = true }; if ns.usesImportMeta { s.usesImportMeta = true }
	dst := s.UsedVariables[len(s.UsedVariables)-1]
	for _, uv := range ns.UsedVariables { for name := range uv {
		if ns.DeclaredVariables.Has(name) || ns.LexicalVariables.Has(name) { continue }
		if ns.IsFunctionBoundary && ns.HasArgs && !ns.isArrowFnBoundary { continue }
		dst[name] = struct{}{}
		if track && (ns.IsFunctionBoundary || !ns.isLexScope) { s.ClosedVarCandidates[name] = struct{}{} }
	}}
	if track && !ns.IsFunctionBoundary { for n := range ns.ClosedVarCandidates { s.ClosedVarCandidates[n] = struct{}{} } }
}
func (s *Scope) MergeInnerArrowFunctionFeatures(f InnerArrowFunctionCodeFeatures) { s.innerArrowFnFeatures = s.innerArrowFnFeatures | f }
func (s *Scope) FinalizeSloppyModeFunctionHoisting() {}
func (s *Scope) BubbleSloppyModeFunctionHoistingCandidates(*Scope) {}
func (s *Scope) GetCapturedVars(captured map[string]struct{}) {
	if s.needsFullActivation || s.usesEval {
		for name := range s.DeclaredVariables.Map() { captured[name] = struct{}{} }; return
	}
	for name := range s.ClosedVarCandidates { if s.DeclaredVariables.Has(name) { captured[name] = struct{}{} } }
}
func (s *Scope) CopyCapturedVariablesToVector(used map[string]struct{}) []string {
	var r []string; for name := range used { if !s.DeclaredVariables.Has(name) && !s.LexicalVariables.Has(name) { r = append(r, name) } }; return r
}
func (s *Scope) setInnerArrowFnUsesEvalAndArgsIfNeeded() { if s.usesEval { s.innerArrowFnFeatures |= EvalInnerArrowFunctionFeature } }

// ScopeStack
type ScopeStack struct { Scopes []*Scope }
func NewScopeStack() *ScopeStack { return &ScopeStack{make([]*Scope, 0, 20)} }
func (ss *ScopeStack) Alloc(cs *Scope, iv ImplementationVisibility, l LexicallyScopedFeatures, fn, gn, ar, ay, sb bool) *Scope {
	s := NewScope(cs, iv, l, fn, gn, ar, ay, sb); ss.Scopes = append(ss.Scopes, s); return s
}
func (ss *ScopeStack) Size() int { return len(ss.Scopes) }
func (ss *ScopeStack) RemoveLast() { if len(ss.Scopes) > 0 { ss.Scopes = ss.Scopes[:len(ss.Scopes)-1] } }
func (ss *ScopeStack) At(i int) *Scope { if i >= 0 && i < len(ss.Scopes) { return ss.Scopes[i] }; return nil }

type ArgumentType int; const (ArgumentNormal ArgumentType = iota; ArgumentSpread)
type ParsingContext int; const (ParsingNormal ParsingContext = iota; ParsingFunctionConstructor)
type FunctionParsePhase int; const (FunctionParsePhaseParameters FunctionParsePhase = iota; FunctionParsePhaseBody)
type ParserState struct {
	AssignmentCount, NonLHSCount, NonTrivialExpressionCount, ReturnStatementCount, UnaryTokenStackDepth int
	FunctionParsePhase FunctionParsePhase; LastIdentifier, LastFunctionName, LastPrivateName string
	AllowAwait, IsParsingClassFieldInitializer, ClassFieldInitMasksAsync bool
}
type LexerState struct { StartOffset, OldLineStartOffset, OldLineNumber int; HasLineTerminatorBeforeToken bool; LastTokenTypeVal JSTokenType }
type SavePoint struct { ParserStateVal ParserState; LexerStateVal LexerState }
type SavePointWithError struct { SavePoint; LexerError bool; LexerErrorMessage string; ParserErrorMessage string }
type ParseInnerResult struct {
	Parameters *FunctionParameters; SourceElements *SourceElements
	FunctionDeclarations []*FunctionMetadataNode
	VarDeclarations, LexicalVariables *VariableEnvironment
	Features CodeFeatures; NumConstants int
}

// Parser struct
type Parser struct {
	Token JSToken; Source *SourceCode; ParserArena *ParserArena
	LastTokenLocation JSTokenLocation; CurrentScopePtr *Scope
	ErrorMessage string; LastTokenType JSTokenType
	StatementDepth int; FunctionModeVal FunctionMode; AllowsIn bool
	ImplementationVisibilityVal ImplementationVisibility; InsideSwitchCaseBody bool
	ParserStateVal ParserState; ParseMode SourceParseMode
	ConstructorKindForTopLevelFunctionExpressions ConstructorKind
	IsInsideOrdinaryFunction, ParsingBuiltin, IsEvalCtx bool
	SeenTaggedTemplate, SeenPrivateNameUse, SeenArgumentsDotLength bool
	FunctionCache *SourceProviderCache
	CallOrApplyDepthScope *CallOrApplyDepthScope
	ModuleScopeData *ModuleScopeData
	ScriptMode JSParserScriptMode; SuperBinding SuperBinding
	HasStackOverflow bool; ScopeStack *ScopeStack
}

func NewParser(source *SourceCode, iv ImplementationVisibility, pm SourceParseMode, fm FunctionMode, sb SuperBinding, sm JSParserScriptMode) *Parser {
	return &Parser{Source: source, ParserArena: NewParserArena(), LastTokenType: ERRORTOK,
		FunctionModeVal: fm, AllowsIn: true, ImplementationVisibilityVal: iv, ParseMode: pm, ScriptMode: sm,
		SuperBinding: sb, ScopeStack: NewScopeStack(), ConstructorKindForTopLevelFunctionExpressions: ConstructorKindNone}
}

// CallOrApplyDepthScope
type CallOrApplyDepthScope struct { P *Parser; Parent *CallOrApplyDepthScope; Depth, DepthOfInnermostChild int }
func NewCallOrApplyDepthScope(p *Parser) *CallOrApplyDepthScope {
	s := &CallOrApplyDepthScope{P: p}
	if p.CallOrApplyDepthScope != nil { s.Depth = p.CallOrApplyDepthScope.Depth + 1; s.DepthOfInnermostChild = s.Depth }
	s.Parent = p.CallOrApplyDepthScope; p.CallOrApplyDepthScope = s; return s
}
func (s *CallOrApplyDepthScope) DistanceToInnermostChild() int { return s.DepthOfInnermostChild - s.Depth }
func (s *CallOrApplyDepthScope) Close() {
	if s.Parent != nil && s.DepthOfInnermostChild > s.Parent.DepthOfInnermostChild { s.Parent.DepthOfInnermostChild = s.DepthOfInnermostChild }
	s.P.CallOrApplyDepthScope = s.Parent
}

// AllowInOverride
type AllowInOverride struct { P *Parser; Old bool }
func NewAllowInOverride(p *Parser) *AllowInOverride { return &AllowInOverride{P: p, Old: p.AllowsIn} }
func (o *AllowInOverride) Close() { o.P.AllowsIn = o.Old }

// AutoPopScope
type AutoPopScope struct { S *Scope; P *Parser; Popped bool }
func (a *AutoPopScope) SetPopped() { a.Popped = true }
func (a *AutoPopScope) Close() { if !a.Popped && a.P != nil { a.P.PopScope(a.S, false) } }

// AutoCleanupLexicalScope
type AutoCleanupLexicalScope struct { S *Scope; P *Parser }
func (a *AutoCleanupLexicalScope) SetIsValid(s *Scope, p *Parser) { a.S = s; a.P = p }
func (a *AutoCleanupLexicalScope) IsValid() bool { return a.P != nil }
func (a *AutoCleanupLexicalScope) SetPopped() { a.P = nil }
func (a *AutoCleanupLexicalScope) Close() { if a.IsValid() { a.P.PopScope(a.S, false) } }

// Parser methods
func (p *Parser) CurrentScope() *Scope { return p.CurrentScopePtr }
func (p *Parser) SourceParseMode() SourceParseMode { return p.ParseMode }
func (p *Parser) FunctionMode() FunctionMode { return p.FunctionModeVal }
func (p *Parser) HasError() bool { return p.ErrorMessage != "" }
func (p *Parser) StrictMode() bool { if p.CurrentScopePtr != nil { return p.CurrentScopePtr.StrictMode() }; return false }
func (p *Parser) ImplementationVisibility() ImplementationVisibility {
	if p.CurrentScopePtr != nil { return p.CurrentScopePtr.ImplementationVisibility }; return p.ImplementationVisibilityVal
}
func (p *Parser) UpperScope(n int) *Scope { if n < len(p.ScopeStack.Scopes) { return p.ScopeStack.Scopes[len(p.ScopeStack.Scopes)-1-n] }; return nil }
func (p *Parser) CurrentVariableScope() *Scope {
	s := p.CurrentScopePtr; for s != nil && !s.allowsVarDecls { s = s.ContainingScope }; return s
}
func (p *Parser) CurrentLexicalDeclarationScope() *Scope {
	s := p.CurrentScopePtr; for s != nil && !s.allowsLexDecls { s = s.ContainingScope }; return s
}
func (p *Parser) CurrentFunctionScope() *Scope {
	s := p.CurrentScopePtr; for s.ContainingScope != nil && !s.IsFunctionBoundary { s = s.ContainingScope }; return s
}
func (p *Parser) FindPrivateNameScope() *Scope {
	s := p.CurrentScopePtr; for s.ContainingScope != nil && !s.IsPrivateNameScope() { s = s.ContainingScope }
	if s.IsPrivateNameScope() { return s }; return nil
}
func (p *Parser) PushScope() *Scope {
	iv := p.ImplementationVisibilityVal; l := LexicallyScopedFeatures(0)
	fn, gn, ar, ay, sb := false, false, false, false, false
	ps := p.CurrentScopePtr
	if ps != nil { iv = ps.ImplementationVisibility; l = ps.lexFeatures
		fn = ps.IsFunction; gn = ps.IsGeneratorFunction; ar = ps.isArrowFn; ay = ps.IsAsyncFn; sb = ps.IsStaticBlock }
	p.CurrentScopePtr = p.ScopeStack.Alloc(ps, iv, l, fn, gn, ar, ay, sb); return p.CurrentScopePtr
}
func (p *Parser) ResetImplementationVisibilityIfNeeded() {
	if p.CurrentScopePtr == nil || !p.CurrentScopePtr.IsFunctionBoundary { return }
	for s := p.CurrentScopePtr.ContainingScope; s != nil; s = s.ContainingScope {
		if !s.IsFunctionBoundary { continue }
		if s.ImplementationVisibility != ImplementationVisibilityPrivateRecursive { p.CurrentScopePtr.ResetImplementationVisibility() }; break
	}
}
func (p *Parser) PopScopeInternal(s *Scope, track bool) (*VariableEnvironment, []*FunctionMetadataNode) {
	ls := p.CurrentScopePtr; ps := ls.ContainingScope
	ls.FinalizeLexicalEnvironment(); ps.CollectFreeVariables(ls, track)
	if ls.HasSloppyModeFunctionHoistingCandidates() { ls.BubbleSloppyModeFunctionHoistingCandidates(ps) }
	if ls.isArrowFn { ls.setInnerArrowFnUsesEvalAndArgsIfNeeded() }
	if !(ls.IsFunctionBoundary && !ls.isArrowFnBoundary) { ps.MergeInnerArrowFunctionFeatures(ls.innerArrowFnFeatures) }
	if !ls.IsFunctionBoundary && ls.needsFullActivation { ps.SetNeedsFullActivation() }
	le := ls.TakeLexicalEnvironment(); fd := ls.TakeFunctionDeclarations()
	p.ScopeStack.RemoveLast(); p.CurrentScopePtr = ps; return le, fd
}
func (p *Parser) PopScope(s *Scope, track bool) (*VariableEnvironment, []*FunctionMetadataNode) { return p.PopScopeInternal(s, track) }
func (p *Parser) PopScopeAuto(a *AutoPopScope, track bool) (*VariableEnvironment, []*FunctionMetadataNode) { a.SetPopped(); return p.PopScopeInternal(a.S, track) }
func (p *Parser) PopScopeCleanup(a *AutoCleanupLexicalScope, track bool) (*VariableEnvironment, []*FunctionMetadataNode) { s := a.S; a.SetPopped(); return p.PopScopeInternal(s, track) }

func (p *Parser) Next(LexerFlags) { p.LastTokenLocation = p.Token.Location(); p.LastTokenType = p.Token.Type }
func (p *Parser) NextWithoutClearingLineTerminator(LexerFlags) { p.Next(0) }
func (p *Parser) NextExpectIdentifier(LexerFlags) { p.Next(0) }
func (p *Parser) Match(e JSTokenType) bool { return p.Token.Type == e }
func (p *Parser) Consume(e JSTokenType, _ ...LexerFlags) bool { if p.Token.Type == e { p.Next(0); return true }; return false }
func (p *Parser) MatchIdentifierOrKeyword() bool { return p.Token.Type == IDENT || (p.Token.Type&KeywordTokenFlag) != 0 }
func (p *Parser) TokenStart() int { return p.Token.StartPosition.Offset }
func (p *Parser) TokenLine() int { return p.Token.StartPosition.Line }
func (p *Parser) TokenColumn() int { return p.TokenStart() - p.TokenLineStart() }
func (p *Parser) TokenLineStart() int { return p.Token.StartPosition.LineStartOffset }
func (p *Parser) TokenLocation() JSTokenLocation { return p.Token.Location() }
func (p *Parser) SetErrorMessage(msg string) { p.ErrorMessage = msg; if p.ErrorMessage == "" { p.ErrorMessage = "Unparseable script" } }
func (p *Parser) AutoSemiColon() bool { if p.Token.Type == SEMICOLON { p.Next(0); return true }; return p.AllowAutomaticSemicolon() }
func (p *Parser) AllowAutomaticSemicolon() bool { return p.Token.Type == CLOSEBRACE || p.Token.Type == EOFTOK }
func (p *Parser) IsBinaryOperator(tok JSTokenType) bool { switch tok { case OR, AND, BITOR, BITXOR, BITAND, EQEQ, NE, STREQ, STRNEQ, LT, GT, LE, GE, INSTANCEOF, INTOKEN, LSHIFT, RSHIFT, URSHIFT, PLUS, MINUS, TIMES, DIVIDE, MOD, POW: return true; default: return false } }
func (p *Parser) StartLoop() { if p.CurrentScopePtr != nil { p.CurrentScopePtr.StartLoop() } }
func (p *Parser) EndLoop() { if p.CurrentScopePtr != nil { p.CurrentScopePtr.EndLoop() } }
func (p *Parser) StartSwitch() { if p.CurrentScopePtr != nil { p.CurrentScopePtr.StartSwitch() } }
func (p *Parser) EndSwitch() { if p.CurrentScopePtr != nil { p.CurrentScopePtr.EndSwitch() } }
func (p *Parser) PushLabel(l *ScopeLabelInfo) { if p.CurrentScopePtr != nil { p.CurrentScopePtr.PushLabel(l) } }
func (p *Parser) PopLabel(s *Scope) { s.PopLabel() }
func (p *Parser) SetStrictMode() { if p.CurrentScopePtr != nil { p.CurrentScopePtr.SetStrictMode() } }
func (p *Parser) DeclareParameter(name string, ok bool) DeclarationResultMask { if p.CurrentScopePtr != nil { return p.CurrentScopePtr.DeclareParameter(name, ok) }; return DeclarationResultMask(Valid) }
func (p *Parser) BreakIsValid() bool { if p.CurrentScopePtr == nil { return false }; c := p.CurrentScopePtr
	for !c.BreakIsValid() { if !c.HasContainingScope() || c.isStaticBlockBoundary { return false }; c = c.ContainingScope }; return true }
func (p *Parser) ContinueIsValid() bool { if p.CurrentScopePtr == nil { return false }; c := p.CurrentScopePtr
	for !c.ContinueIsValid() { if !c.HasContainingScope() || c.isStaticBlockBoundary { return false }; c = c.ContainingScope }; return true }

func (s *Scope) HasSloppyModeFunctionHoistingCandidates() bool { return len(s.SloppyModeFnCandidates) > 0 }

// 辅助类型
type EvalContextType int; type DerivedContextType int; type LexerFlags int
const (JSParserNotBuiltin JSParserBuiltinMode = NotBuiltin; JSParserBuiltin JSParserBuiltinMode = Builtin)
