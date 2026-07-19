// 版权所有 (C) 1999-2000 Harri Porten (porten@kde.org)
// 版权所有 (C) 2001 Peter Kelly (pmk@post.com)
// 版权所有 (C) 2003-2025 Apple Inc. 保留所有权利。
// 版权所有 (C) 2007 Cameron Zwarich (cwzwarich@uwaterloo.ca)
// 版权所有 (C) 2007 Maks Orlovich
// 版权所有 (C) 2007 Eric Seidel <eric@webkit.org>
//
// 本库是自由软件；你可以根据 GNU 宽通用公共许可证
// 第 2 版或（由你选择的）任何后续版本的条款重新分发和/或修改它。
//
// 本库的分发是希望它有用，
// 但没有任何担保；甚至没有隐含的适销性或特定用途适用性的担保。
// 详见 GNU 宽通用公共许可证。
//
// 你应该已经随本库收到一份 GNU 宽通用公共许可证副本；
// 如果没有，请写信给 Free Software Foundation, Inc.,
// 51 Franklin Street, Fifth Floor, Boston, MA 02110-1301, USA。
//
// 使用约定：BSD 许可证
//
// 从 WebKit Source/JavaScriptCore/parser/Nodes.h 翻译为 Go

package parser

import (
	"wb-ui/jsc/runtime"
	"wb-ui/jsc/bytecompiler"
)

// ===== 前向声明 =====

type BytecodeGenerator = bytecompiler.BytecodeGenerator
type RegisterID = bytecompiler.RegisterID
type Label = bytecompiler.Label

// ParserArena 前向声明（定义在 parser_arena.go）
type ParserArena struct{}

// JSTextPosition 表示文本位置（行、偏移、行起始偏移）
type JSTextPosition struct {
	Line            int
	Offset          int
	LineStartOffset int
}

// NewJSTextPosition 创建新的 JSTextPosition
func NewJSTextPosition(line, offset, lineStartOffset int) JSTextPosition {
	return JSTextPosition{Line: line, Offset: offset, LineStartOffset: lineStartOffset}
}

// JSTokenLocation 表示词法记号位置
type JSTokenLocation struct {
	Line            int
	Offset          int
	LineStartOffset int
	StartOffset     int
}

// NewJSTokenLocation 创建新的 JSTokenLocation
func NewJSTokenLocation(line, lineStartOffset int) JSTokenLocation {
	return JSTokenLocation{Line: line, LineStartOffset: lineStartOffset}
}

// ResultType 表示表达式结果类型
type ResultType struct {
	Bits uint8
}

func UnknownType() ResultType           { return ResultType{Bits: 0} }
func (r ResultType) IsKnown() bool      { return false }

// OpcodeID 操作码 ID（来自 bytecode 包，暂时用 uint32）
type OpcodeID uint32

// SourceCode 表示源代码
type SourceCode struct{}

func (s *SourceCode) ProviderID() uintptr { return 0 }

// VariableEnvironment 变量环境
type VariableEnvironment struct {
	// 暂未实现完整字段
}

func (v *VariableEnvironment) HasUsingDeclaration() bool       { return false }
func (v *VariableEnvironment) HasAwaitUsingDeclaration() bool  { return false }
func (v *VariableEnvironment) UsingDeclarationCount() uint32   { return 0 }
func (v *VariableEnvironment) HasCapturedVariables() bool      { return false }
func (v *VariableEnvironment) Captures(uid runtime.UniquedStringImplPtr) bool { return false }

// LexicallyScopedFeatures 词法作用域特征
type LexicallyScopedFeatures uint32

const (
	StrictModeLexicallyScopedFeature LexicallyScopedFeatures = 1 << iota
	// 其他特征按需添加
)

// CodeFeatures 代码特征位掩码
type CodeFeatures uint32

const (
	EvalFeature                 CodeFeatures = 1 << 0
	ArgumentsFeature            CodeFeatures = 1 << 1
	ShadowsArgumentsFeature     CodeFeatures = 1 << 2
	ArrowFunctionFeature        CodeFeatures = 1 << 3
	ThisFeature                 CodeFeatures = 1 << 4
	SuperCallFeature            CodeFeatures = 1 << 5
	SuperPropertyFeature        CodeFeatures = 1 << 6
	NewTargetFeature            CodeFeatures = 1 << 7
	WithFeature                 CodeFeatures = 1 << 8
	NonSimpleParameterListFeature CodeFeatures = 1 << 9
	AsyncFunctionWithoutAwaitFeature CodeFeatures = 1 << 10
	NoFeatures                  CodeFeatures = 0
)

// InnerArrowFunctionCodeFeatures 内部箭头函数代码特征
type InnerArrowFunctionCodeFeatures uint32

const (
	NoInnerArrowFunctionFeatures           InnerArrowFunctionCodeFeatures = 0
	ArgumentsInnerArrowFunctionFeature     InnerArrowFunctionCodeFeatures = 1 << 0
	SuperCallInnerArrowFunctionFeature     InnerArrowFunctionCodeFeatures = 1 << 1
	SuperPropertyInnerArrowFunctionFeature InnerArrowFunctionCodeFeatures = 1 << 2
	EvalInnerArrowFunctionFeature          InnerArrowFunctionCodeFeatures = 1 << 3
	ThisInnerArrowFunctionFeature          InnerArrowFunctionCodeFeatures = 1 << 4
	NewTargetInnerArrowFunctionFeature     InnerArrowFunctionCodeFeatures = 1 << 5
)

// ImplementationVisibility 实现可见性
type ImplementationVisibility uint8

const (
	ImplementationVisibilityPublic ImplementationVisibility = iota
	ImplementationVisibilityPrivate
)

// SourceParseMode 源码解析模式
type SourceParseMode uint8

const (
	SourceParseModeNormal       SourceParseMode = iota
	SourceParseModeArrowFunction
	SourceParseModeGeneratorBody
	SourceParseModeAsyncFunction
	SourceParseModeAsyncArrowFunction
	SourceParseModeAsyncGeneratorBody
	SourceParseModeFunctionOverride
)

// FunctionMode 函数模式
type FunctionMode uint8

const (
	FunctionModeNormal     FunctionMode = iota
	FunctionModeGetter     FunctionMode = 1
	FunctionModeSetter     FunctionMode = 2
)

// ConstructorKind 构造器种类
type ConstructorKind uint8

const (
	ConstructorKindNone    ConstructorKind = iota
	ConstructorKindBase    ConstructorKind = 1
	ConstructorKindDerived ConstructorKind = 2
)

// SuperBinding super 绑定
type SuperBinding uint8

const (
	SuperBindingNotNeeded SuperBinding = iota
	SuperBindingNeeded    SuperBinding = 1
)

// PrivateBrandRequirement 私有品牌要求
type PrivateBrandRequirement uint8

const (
	PrivateBrandRequirementNone PrivateBrandRequirement = iota
	PrivateBrandRequirementNeeded
)

// FunctionParameters 函数参数
type FunctionParameters struct {
	Patterns           []struct{ Pattern *DestructuringPatternNode; DefaultValue ExpressionNode }
	IsSimpleParamList  bool
}

func NewFunctionParameters() *FunctionParameters {
	return &FunctionParameters{IsSimpleParamList: true}
}

func (fp *FunctionParameters) Size() uint32 {
	return uint32(len(fp.Patterns))
}

func (fp *FunctionParameters) At(index uint32) (*DestructuringPatternNode, ExpressionNode) {
	if int(index) < len(fp.Patterns) {
		return fp.Patterns[index].Pattern, fp.Patterns[index].DefaultValue
	}
	return nil, nil
}

func (fp *FunctionParameters) Append(pattern *DestructuringPatternNode, defaultValue ExpressionNode) {
	hasDefault := defaultValue != nil
	isSimple := !hasDefault && pattern.IsBindingNode()
	fp.IsSimpleParamList = fp.IsSimpleParamList && isSimple
	fp.Patterns = append(fp.Patterns, struct{ Pattern *DestructuringPatternNode; DefaultValue ExpressionNode }{pattern, defaultValue})
}

func (fp *FunctionParameters) IsSimpleParameterList() bool { return fp.IsSimpleParamList }

// ModuleScopeData 模块作用域数据
type ModuleScopeData struct{}

// UnlinkedFunctionExecutable 未链接的函数可执行体（前向声明）
type UnlinkedFunctionExecutable struct{}

func (u *UnlinkedFunctionExecutable) ClassElementDefinition() {}

// ===== 操作符枚举 =====

type Operator uint8

const (
	OperatorEqual         Operator = iota
	OperatorPlusEq        Operator = 1
	OperatorMinusEq       Operator = 2
	OperatorMultEq        Operator = 3
	OperatorDivEq         Operator = 4
	OperatorPlusPlus      Operator = 5
	OperatorMinusMinus    Operator = 6
	OperatorBitAndEq      Operator = 7
	OperatorBitXOrEq      Operator = 8
	OperatorBitOrEq       Operator = 9
	OperatorModEq         Operator = 10
	OperatorPowEq         Operator = 11
	OperatorCoalesceEq    Operator = 12
	OperatorOrEq          Operator = 13
	OperatorAndEq         Operator = 14
	OperatorLShift        Operator = 15
	OperatorRShift        Operator = 16
	OperatorURShift       Operator = 17
)

type LogicalOperator uint8

const (
	LogicalOperatorAnd LogicalOperator = iota
	LogicalOperatorOr  LogicalOperator = 1
)

type FallThroughMode uint8

const (
	FallThroughMeansTrue  FallThroughMode = 0
	FallThroughMeansFalse FallThroughMode = 1
)

func InvertFallThroughMode(f FallThroughMode) FallThroughMode {
	if f == FallThroughMeansTrue {
		return FallThroughMeansFalse
	}
	return FallThroughMeansTrue
}

// ===== SwitchInfo =====

type SwitchInfo struct {
	BytecodeOffset uint32
	SwitchType     SwitchType
}

type SwitchType uint8

const (
	SwitchTypeNone          SwitchType = iota
	SwitchTypeImmediate     SwitchType = 1
	SwitchTypeCharacter     SwitchType = 2
	SwitchTypeImmediateList SwitchType = 3
	SwitchTypeCharacterList SwitchType = 4
	SwitchTypeString        SwitchType = 5
)

// ===== AssignmentContext =====

type AssignmentContext uint8

const (
	AssignmentContextDeclarationStatement            AssignmentContext = iota
	AssignmentContextConstDeclarationStatement       AssignmentContext = 1
	AssignmentContextUsingDeclarationStatement       AssignmentContext = 2
	AssignmentContextAwaitUsingDeclarationStatement  AssignmentContext = 3
	AssignmentContextAssignmentExpression            AssignmentContext = 4
)

// ===== ParserArenaFreeable 和 ParserArenaDeletable =====

type ParserArenaFreeable struct{}

type ParserArenaDeletable struct{}

// ===== Node — 所有 AST 节点的基类 =====

type Node struct {
	Position       JSTextPosition
	EndOffset      int
	NeedsDebugHook bool
}

func NewNode(loc JSTokenLocation) *Node {
	return &Node{
		Position:  JSTextPosition{Line: loc.Line, Offset: loc.Offset, LineStartOffset: loc.LineStartOffset},
		EndOffset: -1,
	}
}

func (n *Node) FirstLine() int                        { return n.Position.Line }
func (n *Node) StartOffset() int                      { return n.Position.Offset }
func (n *Node) LineStartOffset() int                  { return n.Position.LineStartOffset }
func (n *Node) GetPosition() JSTextPosition           { return n.Position }
func (n *Node) SetEndOffset(offset int)               { n.EndOffset = offset }
func (n *Node) SetStartOffset(offset int)             { n.Position.Offset = offset }
func (n *Node) NeedsDebugHookFlag() bool              { return n.NeedsDebugHook }
func (n *Node) SetNeedsDebugHook()                    { n.NeedsDebugHook = true }

// ===== ExpressionNode 接口 =====

type ExpressionNode interface {
	NodeInterface
	EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID

	IsNumber() bool
	IsString() bool
	IsBigInt() bool
	IsObjectLiteral() bool
	IsArrayLiteral() bool
	IsNull() bool
	IsPure(generator *BytecodeGenerator) bool
	IsConstant() bool
	IsLocation() bool
	IsPrivateLocation() bool
	IsAssignmentLocation() bool
	IsResolveNode() bool
	IsAssignResolveNode() bool
	IsBracketAccessorNode() bool
	IsDotAccessorNode() bool
	IsDestructuringNode() bool
	IsBaseFuncExprNode() bool
	IsFuncExprNode() bool
	IsArrowFuncExprNode() bool
	IsClassExprNode() bool
	IsCommaNode() bool
	IsSimpleArray() bool
	IsAdd() bool
	IsSubtract() bool
	IsBoolean() bool
	IsThisNode() bool
	IsSpreadExpression() bool
	IsSuperNode() bool
	IsImportNode() bool
	IsMetaProperty() bool
	IsNewTarget() bool
	IsImportMeta() bool
	IsBytecodeIntrinsicNode() bool
	IsBinaryOpNode() bool
	IsFunctionCall() bool
	IsDeleteNode() bool
	IsOptionalChain() bool
	IsOptionalCall() bool
	IsPrivateIdentifier() bool
	IsArgumentsLengthAccess(vm *runtime.VM) bool
	IsArguments(vm *runtime.VM) bool
	EmitBytecodeInConditionContext(generator *BytecodeGenerator, trueTarget, falseTarget *Label, fallThroughMode FallThroughMode)
	StripUnaryPlus() ExpressionNode
	ResultDescriptor() ResultType
	IsOptionalChainBase() bool
	SetIsOptionalChainBase()
}

type NodeInterface interface {
	FirstLine() int
	StartOffset() int
	EndOffset() int
	LineStartOffset() int
	GetPosition() JSTextPosition
	SetEndOffset(offset int)
	SetStartOffset(offset int)
	NeedsDebugHookFlag() bool
	SetNeedsDebugHook()
}

// ===== ExpressionNodeBase — ExpressionNode 基类实现 =====

type ExpressionNodeBase struct {
	*Node
	ResultTypeVal        ResultType
	IsOptionalChainBaseVal bool
}

func NewExpressionNodeBase(loc JSTokenLocation, resultType ResultType) ExpressionNodeBase {
	return ExpressionNodeBase{
		Node:          NewNode(loc),
		ResultTypeVal: resultType,
	}
}

func (b *ExpressionNodeBase) IsNumber() bool                                   { return false }
func (b *ExpressionNodeBase) IsString() bool                                   { return false }
func (b *ExpressionNodeBase) IsBigInt() bool                                   { return false }
func (b *ExpressionNodeBase) IsObjectLiteral() bool                           { return false }
func (b *ExpressionNodeBase) IsArrayLiteral() bool                            { return false }
func (b *ExpressionNodeBase) IsNull() bool                                    { return false }
func (b *ExpressionNodeBase) IsPure(generator *BytecodeGenerator) bool        { return false }
func (b *ExpressionNodeBase) IsConstant() bool                                { return false }
func (b *ExpressionNodeBase) IsLocation() bool                                { return false }
func (b *ExpressionNodeBase) IsPrivateLocation() bool                         { return false }
func (b *ExpressionNodeBase) IsAssignmentLocation() bool                      { return b.IsLocation() }
func (b *ExpressionNodeBase) IsResolveNode() bool                             { return false }
func (b *ExpressionNodeBase) IsAssignResolveNode() bool                       { return false }
func (b *ExpressionNodeBase) IsBracketAccessorNode() bool                     { return false }
func (b *ExpressionNodeBase) IsDotAccessorNode() bool                         { return false }
func (b *ExpressionNodeBase) IsDestructuringNode() bool                       { return false }
func (b *ExpressionNodeBase) IsBaseFuncExprNode() bool                        { return false }
func (b *ExpressionNodeBase) IsFuncExprNode() bool                            { return false }
func (b *ExpressionNodeBase) IsArrowFuncExprNode() bool                       { return false }
func (b *ExpressionNodeBase) IsClassExprNode() bool                           { return false }
func (b *ExpressionNodeBase) IsCommaNode() bool                               { return false }
func (b *ExpressionNodeBase) IsSimpleArray() bool                             { return false }
func (b *ExpressionNodeBase) IsAdd() bool                                     { return false }
func (b *ExpressionNodeBase) IsSubtract() bool                                { return false }
func (b *ExpressionNodeBase) IsBoolean() bool                                 { return false }
func (b *ExpressionNodeBase) IsThisNode() bool                                { return false }
func (b *ExpressionNodeBase) IsSpreadExpression() bool                        { return false }
func (b *ExpressionNodeBase) IsSuperNode() bool                               { return false }
func (b *ExpressionNodeBase) IsImportNode() bool                              { return false }
func (b *ExpressionNodeBase) IsMetaProperty() bool                            { return false }
func (b *ExpressionNodeBase) IsNewTarget() bool                               { return false }
func (b *ExpressionNodeBase) IsImportMeta() bool                              { return false }
func (b *ExpressionNodeBase) IsBytecodeIntrinsicNode() bool                   { return false }
func (b *ExpressionNodeBase) IsBinaryOpNode() bool                            { return false }
func (b *ExpressionNodeBase) IsFunctionCall() bool                            { return false }
func (b *ExpressionNodeBase) IsDeleteNode() bool                              { return false }
func (b *ExpressionNodeBase) IsOptionalChain() bool                           { return false }
func (b *ExpressionNodeBase) IsOptionalCall() bool                            { return false }
func (b *ExpressionNodeBase) IsPrivateIdentifier() bool                       { return false }
func (b *ExpressionNodeBase) IsArgumentsLengthAccess(vm *runtime.VM) bool     { return false }
func (b *ExpressionNodeBase) IsArguments(vm *runtime.VM) bool                 { return false }
func (b *ExpressionNodeBase) StripUnaryPlus() ExpressionNode                  { return nil }
func (b *ExpressionNodeBase) ResultDescriptor() ResultType                    { return b.ResultTypeVal }
func (b *ExpressionNodeBase) IsOptionalChainBase() bool                       { return b.IsOptionalChainBaseVal }
func (b *ExpressionNodeBase) SetIsOptionalChainBase()                         { b.IsOptionalChainBaseVal = true }

func (b *ExpressionNodeBase) EmitBytecodeInConditionContext(generator *BytecodeGenerator, trueTarget, falseTarget *Label, fallThroughMode FallThroughMode) {
	// 默认实现为空
}

// ===== StatementNode 接口 =====

type StatementNode interface {
	NodeInterface
	EmitBytecode(generator *BytecodeGenerator, destination *RegisterID)

	SetLoc(firstLine, lastLine uint32, startOffset, lineStartOffset int)
	LastLine() uint32
	Next() StatementNode
	SetNext(next StatementNode)
	HasCompletionValue() bool
	HasEarlyBreakOrContinue() bool
	IsEmptyStatement() bool
	IsDebuggerStatement() bool
	IsFunctionNode() bool
	IsReturnNode() bool
	IsExprStatement() bool
	IsBreak() bool
	IsContinue() bool
	IsLabel() bool
	IsBlock() bool
	IsFuncDeclNode() bool
	IsModuleDeclarationNode() bool
	IsForOfNode() bool
	IsDefineFieldNode() bool
}

// ===== StatementNodeBase — StatementNode 基类实现 =====

type StatementNodeBase struct {
	*Node
	LastLineVal int
	NextStmt    StatementNode
}

func NewStatementNodeBase(loc JSTokenLocation) StatementNodeBase {
	return StatementNodeBase{
		Node:      NewNode(loc),
		LastLineVal: -1,
	}
}

func (b *StatementNodeBase) SetLoc(firstLine, lastLine uint32, startOffset, lineStartOffset int) {
	b.LastLineVal = int(lastLine)
	b.Position = JSTextPosition{Line: int(firstLine), Offset: startOffset, LineStartOffset: lineStartOffset}
}
func (b *StatementNodeBase) LastLine() uint32                            { return uint32(b.LastLineVal) }
func (b *StatementNodeBase) Next() StatementNode                        { return b.NextStmt }
func (b *StatementNodeBase) SetNext(next StatementNode)                 { b.NextStmt = next }
func (b *StatementNodeBase) HasCompletionValue() bool                   { return true }
func (b *StatementNodeBase) HasEarlyBreakOrContinue() bool              { return false }
func (b *StatementNodeBase) IsEmptyStatement() bool                     { return false }
func (b *StatementNodeBase) IsDebuggerStatement() bool                  { return false }
func (b *StatementNodeBase) IsFunctionNode() bool                       { return false }
func (b *StatementNodeBase) IsReturnNode() bool                         { return false }
func (b *StatementNodeBase) IsExprStatement() bool                      { return false }
func (b *StatementNodeBase) IsBreak() bool                              { return false }
func (b *StatementNodeBase) IsContinue() bool                           { return false }
func (b *StatementNodeBase) IsLabel() bool                              { return false }
func (b *StatementNodeBase) IsBlock() bool                              { return false }
func (b *StatementNodeBase) IsFuncDeclNode() bool                       { return false }
func (b *StatementNodeBase) IsModuleDeclarationNode() bool              { return false }
func (b *StatementNodeBase) IsForOfNode() bool                          { return false }
func (b *StatementNodeBase) IsDefineFieldNode() bool                    { return false }

// ===== VariableEnvironmentNode =====

type VariableEnvironmentNode struct {
	LexicalVariables VariableEnvironment
	FunctionStack    []*FunctionMetadataNode
}

func NewVariableEnvironmentNode() *VariableEnvironmentNode {
	return &VariableEnvironmentNode{}
}

func NewVariableEnvironmentNodeWithVars(lexicalDeclaredVariables VariableEnvironment) *VariableEnvironmentNode {
	return &VariableEnvironmentNode{
		LexicalVariables: lexicalDeclaredVariables,
	}
}

func NewVariableEnvironmentNodeWithFuncs(lexicalDeclaredVariables VariableEnvironment, functionStack []*FunctionMetadataNode) *VariableEnvironmentNode {
	return &VariableEnvironmentNode{
		LexicalVariables: lexicalDeclaredVariables,
		FunctionStack:    functionStack,
	}
}

func (v *VariableEnvironmentNode) HasUsingDeclaration() bool       { return v.LexicalVariables.HasUsingDeclaration() }
func (v *VariableEnvironmentNode) HasAwaitUsingDeclaration() bool  { return v.LexicalVariables.HasAwaitUsingDeclaration() }
func (v *VariableEnvironmentNode) UsingDeclarationCount() uint32   { return v.LexicalVariables.UsingDeclarationCount() }

// ===== ConstantNode =====

type ConstantNode struct {
	ExpressionNodeBase
}

func NewConstantNode(loc JSTokenLocation, resultType ResultType) *ConstantNode {
	return &ConstantNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, resultType),
	}
}

func (c *ConstantNode) IsPure(generator *BytecodeGenerator) bool { return true }
func (c *ConstantNode) IsConstant() bool                         { return true }
func (c *ConstantNode) JsValue(generator *BytecodeGenerator) runtime.JSValue { return runtime.JSValue{} }
func (c *ConstantNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }
func (c *ConstantNode) EmitBytecodeInConditionContext(generator *BytecodeGenerator, trueTarget, falseTarget *Label, fallThroughMode FallThroughMode) {}

// ===== NullNode =====

type NullNode struct {
	ConstantNode
}

func NewNullNode(loc JSTokenLocation) *NullNode {
	return &NullNode{
		ConstantNode: *NewConstantNode(loc, UnknownType()),
	}
}

func (n *NullNode) IsNull() bool { return true }
func (n *NullNode) JsValue(generator *BytecodeGenerator) runtime.JSValue {
	return runtime.JsNull()
}

// ===== BooleanNode =====

type BooleanNode struct {
	ConstantNode
	Value bool
}

func NewBooleanNode(loc JSTokenLocation, value bool) *BooleanNode {
	return &BooleanNode{
		ConstantNode: *NewConstantNode(loc, UnknownType()),
		Value:        value,
	}
}

func (b *BooleanNode) IsBoolean() bool { return true }
func (b *BooleanNode) JsValue(generator *BytecodeGenerator) runtime.JSValue {
	return runtime.JsBoolean(b.Value)
}

// ===== NumberNode =====

type NumberNode struct {
	ConstantNode
	Value float64
}

func NewNumberNode(loc JSTokenLocation, value float64) *NumberNode {
	return &NumberNode{
		ConstantNode: *NewConstantNode(loc, UnknownType()),
		Value:        value,
	}
}

func (n *NumberNode) IsNumber() bool { return true }
func (n *NumberNode) IsIntegerNode() bool { return false }
func (n *NumberNode) JsValue(generator *BytecodeGenerator) runtime.JSValue {
	return runtime.JsNumber(generator, n.Value)
}
func (n *NumberNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== DoubleNode =====

type DoubleNode struct {
	NumberNode
}

func NewDoubleNode(loc JSTokenLocation, value float64) *DoubleNode {
	return &DoubleNode{
		NumberNode: *NewNumberNode(loc, value),
	}
}

func (d *DoubleNode) IsIntegerNode() bool { return false }

// ===== IntegerNode =====

type IntegerNode struct {
	DoubleNode
}

func NewIntegerNode(loc JSTokenLocation, value float64) *IntegerNode {
	return &IntegerNode{
		DoubleNode: *NewDoubleNode(loc, value),
	}
}

func (i *IntegerNode) IsIntegerNode() bool { return true }

// ===== StringNode =====

type StringNode struct {
	ConstantNode
	Value runtime.Identifier
}

func NewStringNode(loc JSTokenLocation, value *runtime.Identifier) *StringNode {
	return &StringNode{
		ConstantNode: *NewConstantNode(loc, UnknownType()),
		Value:        *value,
	}
}

func (s *StringNode) IsString() bool { return true }
func (s *StringNode) JsValue(generator *BytecodeGenerator) runtime.JSValue {
	return runtime.JSValue{} // 暂未实现
}

// ===== BigIntNode =====

type BigIntNode struct {
	ConstantNode
	Value runtime.Identifier
	Radix uint8
	Sign  bool
}

func NewBigIntNode(loc JSTokenLocation, value *runtime.Identifier, radix uint8) *BigIntNode {
	return &BigIntNode{
		ConstantNode: *NewConstantNode(loc, UnknownType()),
		Value:        *value,
		Radix:        radix,
	}
}

func NewBigIntNodeWithSign(loc JSTokenLocation, value *runtime.Identifier, radix uint8, sign bool) *BigIntNode {
	return &BigIntNode{
		ConstantNode: *NewConstantNode(loc, UnknownType()),
		Value:        *value,
		Radix:        radix,
		Sign:         sign,
	}
}

func (b *BigIntNode) IsBigInt() bool { return true }
func (b *BigIntNode) JsValue(generator *BytecodeGenerator) runtime.JSValue {
	return runtime.JSValue{} // 暂未实现
}

// ===== ThrowableExpressionData =====

type ThrowableExpressionData struct {
	Divot       JSTextPosition
	DivotStart  JSTextPosition
	DivotEnd    JSTextPosition
}

func NewThrowableExpressionData() ThrowableExpressionData {
	return ThrowableExpressionData{}
}

func NewThrowableExpressionDataFrom(divot, divotStart, divotEnd JSTextPosition) ThrowableExpressionData {
	data := ThrowableExpressionData{
		Divot:      divot,
		DivotStart: divotStart,
		DivotEnd:   divotEnd,
	}
	data.CheckConsistency()
	return data
}

func (t *ThrowableExpressionData) SetExceptionSourceCode(divot, divotStart, divotEnd JSTextPosition) {
	t.Divot = divot
	t.DivotStart = divotStart
	t.DivotEnd = divotEnd
	t.CheckConsistency()
}

func (t *ThrowableExpressionData) CheckConsistency() {
	// 断言检查
}

func (t *ThrowableExpressionData) EmitThrowReferenceError(generator *BytecodeGenerator, message string, dst *RegisterID) *RegisterID {
	return dst
}

// ===== ThrowableSubExpressionData =====

type ThrowableSubExpressionData struct {
	ThrowableExpressionData
	SubexpressionDivotOffset    uint16
	SubexpressionEndOffset       uint16
	SubexpressionLineOffset      uint16
	SubexpressionLineStartOffset uint16
}

func NewThrowableSubExpressionData() ThrowableSubExpressionData {
	return ThrowableSubExpressionData{}
}

func NewThrowableSubExpressionDataFrom(divot, divotStart, divotEnd JSTextPosition) ThrowableSubExpressionData {
	return ThrowableSubExpressionData{
		ThrowableExpressionData: NewThrowableExpressionDataFrom(divot, divotStart, divotEnd),
	}
}

func (t *ThrowableSubExpressionData) SetSubexpressionInfo(subexpressionDivot JSTextPosition, subexpressionOffset int) {
	// 简化实现
}

func (t *ThrowableSubExpressionData) SubexpressionDivot() JSTextPosition {
	newLine := t.Divot.Line - int(t.SubexpressionLineOffset)
	newOffset := t.Divot.Offset - int(t.SubexpressionDivotOffset)
	newLineStartOffset := t.Divot.LineStartOffset - int(t.SubexpressionLineStartOffset)
	return JSTextPosition{Line: newLine, Offset: newOffset, LineStartOffset: newLineStartOffset}
}

func (t *ThrowableSubExpressionData) SubexpressionStart() JSTextPosition { return t.DivotStart }
func (t *ThrowableSubExpressionData) SubexpressionEnd() JSTextPosition {
	return JSTextPosition{
		Line:            t.DivotEnd.Line,
		Offset:          t.DivotEnd.Offset - int(t.SubexpressionEndOffset),
		LineStartOffset: t.DivotEnd.LineStartOffset,
	}
}

// ===== ThrowablePrefixedSubExpressionData =====

type ThrowablePrefixedSubExpressionData struct {
	ThrowableExpressionData
	SubexpressionDivotOffset    uint16
	SubexpressionStartOffset    uint16
	SubexpressionLineOffset     uint16
	SubexpressionLineStartOffset uint16
}

func NewThrowablePrefixedSubExpressionData() ThrowablePrefixedSubExpressionData {
	return ThrowablePrefixedSubExpressionData{}
}

func NewThrowablePrefixedSubExpressionDataFrom(divot, start, end JSTextPosition) ThrowablePrefixedSubExpressionData {
	return ThrowablePrefixedSubExpressionData{
		ThrowableExpressionData: NewThrowableExpressionDataFrom(divot, start, end),
	}
}

func (t *ThrowablePrefixedSubExpressionData) SubexpressionDivot() JSTextPosition {
	newLine := t.Divot.Line + int(t.SubexpressionLineOffset)
	newOffset := t.Divot.Offset + int(t.SubexpressionDivotOffset)
	newLineStartOffset := t.Divot.LineStartOffset + int(t.SubexpressionLineStartOffset)
	return JSTextPosition{Line: newLine, Offset: newOffset, LineStartOffset: newLineStartOffset}
}

func (t *ThrowablePrefixedSubExpressionData) SubexpressionStart() JSTextPosition {
	return JSTextPosition{
		Line:            t.DivotStart.Line,
		Offset:          t.DivotStart.Offset + int(t.SubexpressionStartOffset),
		LineStartOffset: t.DivotStart.LineStartOffset,
	}
}

func (t *ThrowablePrefixedSubExpressionData) SubexpressionEnd() JSTextPosition { return t.DivotEnd }

// ===== TemplateExpressionListNode =====

type TemplateExpressionListNode struct {
	Next *TemplateExpressionListNode
	Node ExpressionNode
}

func NewTemplateExpressionListNode(expr ExpressionNode) *TemplateExpressionListNode {
	return &TemplateExpressionListNode{Node: expr}
}

func NewTemplateExpressionListNodeFrom(prev *TemplateExpressionListNode, expr ExpressionNode) *TemplateExpressionListNode {
	return &TemplateExpressionListNode{Next: prev, Node: expr}
}

func (t *TemplateExpressionListNode) Value() ExpressionNode { return t.Node }

// ===== TemplateStringNode =====

type TemplateStringNode struct {
	ExpressionNodeBase
	Cooked *runtime.Identifier
	Raw    *runtime.Identifier
}

func NewTemplateStringNode(loc JSTokenLocation, cooked, raw *runtime.Identifier) *TemplateStringNode {
	return &TemplateStringNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Cooked:             cooked,
		Raw:                raw,
	}
}

func (t *TemplateStringNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== TemplateStringListNode =====

type TemplateStringListNode struct {
	Next *TemplateStringListNode
	Node *TemplateStringNode
}

func NewTemplateStringListNode(node *TemplateStringNode) *TemplateStringListNode {
	return &TemplateStringListNode{Node: node}
}

func NewTemplateStringListNodeFrom(prev *TemplateStringListNode, node *TemplateStringNode) *TemplateStringListNode {
	return &TemplateStringListNode{Next: prev, Node: node}
}

func (t *TemplateStringListNode) Value() *TemplateStringNode { return t.Node }

// ===== TemplateLiteralNode =====

type TemplateLiteralNode struct {
	ExpressionNodeBase
	TemplateStrings      *TemplateStringListNode
	TemplateExpressions  *TemplateExpressionListNode
}

func NewTemplateLiteralNode(loc JSTokenLocation, strings *TemplateStringListNode) *TemplateLiteralNode {
	return &TemplateLiteralNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		TemplateStrings:    strings,
	}
}

func NewTemplateLiteralNodeWithExprs(loc JSTokenLocation, strings *TemplateStringListNode, exprs *TemplateExpressionListNode) *TemplateLiteralNode {
	return &TemplateLiteralNode{
		ExpressionNodeBase:  NewExpressionNodeBase(loc, UnknownType()),
		TemplateStrings:     strings,
		TemplateExpressions: exprs,
	}
}

func (t *TemplateLiteralNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== TaggedTemplateNode =====

type TaggedTemplateNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Tag              ExpressionNode
	TemplateLiteralVal *TemplateLiteralNode
}

func NewTaggedTemplateNode(loc JSTokenLocation, tag ExpressionNode, literal *TemplateLiteralNode) *TaggedTemplateNode {
	return &TaggedTemplateNode{
		ExpressionNodeBase:    NewExpressionNodeBase(loc, UnknownType()),
		Tag:                   tag,
		TemplateLiteralVal:    literal,
	}
}

func (t *TaggedTemplateNode) TemplateLiteral() *TemplateLiteralNode { return t.TemplateLiteralVal }
func (t *TaggedTemplateNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== RegExpNode =====

type RegExpNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Pattern runtime.Identifier
	Flags   runtime.Identifier
}

func NewRegExpNode(loc JSTokenLocation, pattern, flags *runtime.Identifier) *RegExpNode {
	return &RegExpNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Pattern:            *pattern,
		Flags:              *flags,
	}
}

func (r *RegExpNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== ThisNode =====

type ThisNode struct {
	ExpressionNodeBase
}

func NewThisNode(loc JSTokenLocation) *ThisNode {
	return &ThisNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
	}
}

func (t *ThisNode) IsThisNode() bool { return true }
func (t *ThisNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== SuperNode =====

type SuperNode struct {
	ExpressionNodeBase
}

func NewSuperNode(loc JSTokenLocation) *SuperNode {
	return &SuperNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
	}
}

func (s *SuperNode) IsSuperNode() bool { return true }
func (s *SuperNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== ImportNode =====

type ImportNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Expr     ExpressionNode
	Option   ExpressionNode
	Deferred bool
}

func NewImportNode(loc JSTokenLocation, expr, option ExpressionNode, deferred bool) *ImportNode {
	return &ImportNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Expr:               expr,
		Option:             option,
		Deferred:           deferred,
	}
}

func (i *ImportNode) IsImportNode() bool { return true }
func (i *ImportNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== MetaPropertyNode =====

type MetaPropertyNode struct {
	ExpressionNodeBase
}

func NewMetaPropertyNode(loc JSTokenLocation) *MetaPropertyNode {
	return &MetaPropertyNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
	}
}

func (m *MetaPropertyNode) IsMetaProperty() bool { return true }

// ===== NewTargetNode =====

type NewTargetNode struct {
	MetaPropertyNode
}

func NewNewTargetNode(loc JSTokenLocation) *NewTargetNode {
	return &NewTargetNode{
		MetaPropertyNode: *NewMetaPropertyNode(loc),
	}
}

func (n *NewTargetNode) IsNewTarget() bool { return true }
func (n *NewTargetNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== ImportMetaNode =====

type ImportMetaNode struct {
	MetaPropertyNode
	Expr ExpressionNode
}

func NewImportMetaNode(loc JSTokenLocation, expr ExpressionNode) *ImportMetaNode {
	return &ImportMetaNode{
		MetaPropertyNode: *NewMetaPropertyNode(loc),
		Expr:             expr,
	}
}

func (i *ImportMetaNode) IsImportMeta() bool { return true }
func (i *ImportMetaNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== ResolveNode =====

type ResolveNode struct {
	ExpressionNodeBase
	Ident runtime.Identifier
	Start JSTextPosition
}

func NewResolveNode(loc JSTokenLocation, ident *runtime.Identifier, start JSTextPosition) *ResolveNode {
	return &ResolveNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Ident:              *ident,
		Start:              start,
	}
}

func (r *ResolveNode) Identifier() runtime.Identifier                     { return r.Ident }
func (r *ResolveNode) IsPure(generator *BytecodeGenerator) bool           { return false } // 简化实现
func (r *ResolveNode) IsLocation() bool                                   { return true }
func (r *ResolveNode) IsResolveNode() bool                                { return true }
func (r *ResolveNode) IsArguments(vm *runtime.VM) bool                    { return false } // 简化
func (r *ResolveNode) GetFromScopeCanThrow(generator *BytecodeGenerator) bool { return false }
func (r *ResolveNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== PrivateIdentifierNode =====

type PrivateIdentifierNode struct {
	ExpressionNodeBase
	Ident runtime.Identifier
}

func NewPrivateIdentifierNode(loc JSTokenLocation, ident *runtime.Identifier) *PrivateIdentifierNode {
	return &PrivateIdentifierNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Ident:              *ident,
	}
}

func (p *PrivateIdentifierNode) IsPrivateIdentifier() bool { return true }
func (p *PrivateIdentifierNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== ElementNode =====

type ElementNode struct {
	Next    *ElementNode
	Node    ExpressionNode
	Elision int
}

func NewElementNode(elision int, expr ExpressionNode) *ElementNode {
	return &ElementNode{
		Elision: elision,
		Node:    expr,
	}
}

func NewElementNodeFrom(prev *ElementNode, elision int, expr ExpressionNode) *ElementNode {
	return &ElementNode{
		Next:    prev,
		Elision: elision,
		Node:    expr,
	}
}

// ===== ArrayNode =====

type ArrayNode struct {
	ExpressionNodeBase
	Element *ElementNode
	Elision int
}

func NewArrayNode(loc JSTokenLocation, elision int) *ArrayNode {
	return &ArrayNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Elision:            elision,
	}
}

func NewArrayNodeWithElement(loc JSTokenLocation, element *ElementNode) *ArrayNode {
	return &ArrayNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Element:            element,
	}
}

func NewArrayNodeWithElision(loc JSTokenLocation, elision int, element *ElementNode) *ArrayNode {
	return &ArrayNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Element:            element,
		Elision:            elision,
	}
}

func (a *ArrayNode) IsArrayLiteral() bool { return true }
func (a *ArrayNode) IsSimpleArray() bool  { return false }
func (a *ArrayNode) ToArgumentList(arena *ParserArena, line, lineStartOffset int) *ArgumentListNode {
	return nil // 暂未实现
}
func (a *ArrayNode) Elements() *ElementNode { return a.Element }
func (a *ArrayNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== ClassElementTag =====

type ClassElementTag uint8

const (
	ClassElementTagNo       ClassElementTag = iota
	ClassElementTagInstance ClassElementTag = 1
	ClassElementTagStatic   ClassElementTag = 2
	ClassElementTagLastTag  ClassElementTag = 3
)

// ===== PropertyNode =====

type PropertyNode struct {
	NameVal                 *runtime.Identifier
	Expression              ExpressionNode
	Assign                  ExpressionNode
	TypeBits                uint16
	NeedsSuperBindingVal    bool
	ClassElementTagVal      uint8
	IsOverriddenByDuplicateVal bool
}

type PropertyNodeType uint16

const (
	PropertyConstant    PropertyNodeType = 1
	PropertyGetter      PropertyNodeType = 2
	PropertySetter      PropertyNodeType = 4
	PropertyComputed    PropertyNodeType = 8
	PropertyShorthand   PropertyNodeType = 16
	PropertySpread      PropertyNodeType = 32
	PropertyPrivateField PropertyNodeType = 64
	PropertyPrivateMethod PropertyNodeType = 128
	PropertyPrivateSetter PropertyNodeType = 256
	PropertyPrivateGetter PropertyNodeType = 512
	PropertyBlock       PropertyNodeType = 1024
)

func NewPropertyNode(name *runtime.Identifier, typ PropertyNodeType, superBinding SuperBinding, tag ClassElementTag) *PropertyNode {
	return &PropertyNode{
		NameVal:              name,
		TypeBits:             uint16(typ),
		NeedsSuperBindingVal: superBinding == SuperBindingNeeded,
		ClassElementTagVal:   uint8(tag),
	}
}

func NewPropertyNodeWithExpr(name *runtime.Identifier, assign ExpressionNode, typ PropertyNodeType, superBinding SuperBinding, tag ClassElementTag) *PropertyNode {
	return &PropertyNode{
		NameVal:              name,
		Assign:               assign,
		TypeBits:             uint16(typ),
		NeedsSuperBindingVal: superBinding == SuperBindingNeeded,
		ClassElementTagVal:   uint8(tag),
	}
}

func NewPropertyNodeExpr(expr ExpressionNode, typ PropertyNodeType, superBinding SuperBinding, tag ClassElementTag) *PropertyNode {
	return &PropertyNode{
		Expression:           expr,
		TypeBits:             uint16(typ),
		NeedsSuperBindingVal: superBinding == SuperBindingNeeded,
		ClassElementTagVal:   uint8(tag),
	}
}

func NewPropertyNodeWithPropertyName(propertyName ExpressionNode, assign ExpressionNode, typ PropertyNodeType, superBinding SuperBinding, tag ClassElementTag) *PropertyNode {
	return &PropertyNode{
		Expression:           propertyName,
		Assign:               assign,
		TypeBits:             uint16(typ),
		NeedsSuperBindingVal: superBinding == SuperBindingNeeded,
		ClassElementTagVal:   uint8(tag),
	}
}

func NewPropertyNodeFull(name *runtime.Identifier, propertyName ExpressionNode, assign ExpressionNode, typ PropertyNodeType, superBinding SuperBinding, tag ClassElementTag) *PropertyNode {
	return &PropertyNode{
		NameVal:              name,
		Expression:           propertyName,
		Assign:               assign,
		TypeBits:             uint16(typ),
		NeedsSuperBindingVal: superBinding == SuperBindingNeeded,
		ClassElementTagVal:   uint8(tag),
	}
}

func (p *PropertyNode) ExpressionName() ExpressionNode { return p.Expression }
func (p *PropertyNode) Name() *runtime.Identifier { return p.NameVal }
func (p *PropertyNode) Type() PropertyNodeType { return PropertyNodeType(p.TypeBits) }
func (p *PropertyNode) NeedsSuperBinding() bool { return p.NeedsSuperBindingVal }
func (p *PropertyNode) IsClassProperty() bool { return ClassElementTag(p.ClassElementTagVal) != ClassElementTagNo }
func (p *PropertyNode) IsStaticClassProperty() bool { return ClassElementTag(p.ClassElementTagVal) == ClassElementTagStatic }
func (p *PropertyNode) IsInstanceClassProperty() bool { return ClassElementTag(p.ClassElementTagVal) == ClassElementTagInstance }
func (p *PropertyNode) IsClassField() bool { return p.IsClassProperty() && !p.NeedsSuperBinding() }
func (p *PropertyNode) IsInstanceClassField() bool { return p.IsInstanceClassProperty() && !p.NeedsSuperBinding() }
func (p *PropertyNode) IsStaticClassField() bool { return p.IsStaticClassProperty() && !p.NeedsSuperBinding() }
func (p *PropertyNode) IsStaticClassBlock() bool { return p.TypeBits&uint16(PropertyBlock) != 0 }
func (p *PropertyNode) IsStaticClassElement() bool { return p.IsStaticClassBlock() || p.IsStaticClassField() }
func (p *PropertyNode) IsOverriddenByDuplicate() bool { return p.IsOverriddenByDuplicateVal }
func (p *PropertyNode) IsPrivate() bool {
	return p.TypeBits&(uint16(PropertyPrivateField|PropertyPrivateMethod|PropertyPrivateGetter|PropertyPrivateSetter)) != 0
}
func (p *PropertyNode) HasComputedName() bool { return p.Expression != nil }
func (p *PropertyNode) IsComputedClassField() bool { return p.IsClassField() && p.HasComputedName() }
func (p *PropertyNode) SetIsOverriddenByDuplicate() { p.IsOverriddenByDuplicateVal = true }

// ===== PropertyListNode =====

type PropertyListNode struct {
	ExpressionNodeBase
	Node              *PropertyNode
	Next              *PropertyListNode
	HasPrivateAccessorsVal bool
}

func NewPropertyListNode(loc JSTokenLocation, node *PropertyNode) *PropertyListNode {
	return &PropertyListNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Node:               node,
	}
}

func NewPropertyListNodeFrom(loc JSTokenLocation, node *PropertyNode, next *PropertyListNode) *PropertyListNode {
	return &PropertyListNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Node:               node,
		Next:               next,
	}
}

func NewPropertyListNodeFull(loc JSTokenLocation, node *PropertyNode) *PropertyListNode {
	return &PropertyListNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Node:               node,
	}
}

func (p *PropertyListNode) HasStaticallyNamedProperty(propName *runtime.Identifier) bool {
	return false
}
func (p *PropertyListNode) IsComputedClassField() bool     { return p.Node.IsComputedClassField() }
func (p *PropertyListNode) IsInstanceClassField() bool      { return p.Node.IsInstanceClassField() }
func (p *PropertyListNode) HasInstanceFields() bool         { return false }
func (p *PropertyListNode) IsStaticClassField() bool        { return p.Node.IsStaticClassField() }
func (p *PropertyListNode) IsStaticClassBlock() bool        { return p.Node.IsStaticClassBlock() }
func (p *PropertyListNode) IsStaticClassElement() bool      { return p.Node.IsStaticClassElement() }
func (p *PropertyListNode) SetHasPrivateAccessors(v bool)   { p.HasPrivateAccessorsVal = v }
func (p *PropertyListNode) HasPrivateAccessors() bool       { return p.HasPrivateAccessorsVal }

func ShouldCreateLexicalScopeForClass(list *PropertyListNode) bool { return false }

func (p *PropertyListNode) EmitBytecode(generator *BytecodeGenerator, dst *RegisterID) *RegisterID {
	return p.EmitBytecodeFull(generator, dst, nil, nil, nil)
}

func (p *PropertyListNode) EmitBytecodeFull(generator *BytecodeGenerator, dst *RegisterID, reg2 *RegisterID, vec1 *[]UnlinkedFunctionExecutable_ClassElementDefinition, vec2 *[]UnlinkedFunctionExecutable_ClassElementDefinition) *RegisterID {
	return dst
}

// UnlinkedFunctionExecutable_ClassElementDefinition 类型别名
type UnlinkedFunctionExecutable_ClassElementDefinition = struct{}

func (p *PropertyListNode) EmitDeclarePrivateFieldNames(generator *BytecodeGenerator, scope *RegisterID) {}
func (p *PropertyListNode) EmitPutConstantProperty(generator *BytecodeGenerator, reg *RegisterID, prop PropertyNode) {}
func (p *PropertyListNode) EmitSaveComputedFieldName(generator *BytecodeGenerator, prop PropertyNode) {}

// ===== ObjectLiteralNode =====

type ObjectLiteralNode struct {
	ExpressionNodeBase
	List *PropertyListNode
}

func NewObjectLiteralNode(loc JSTokenLocation) *ObjectLiteralNode {
	return &ObjectLiteralNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
	}
}

func NewObjectLiteralNodeWithList(loc JSTokenLocation, list *PropertyListNode) *ObjectLiteralNode {
	return &ObjectLiteralNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		List:               list,
	}
}

func (o *ObjectLiteralNode) IsObjectLiteral() bool { return true }
func (o *ObjectLiteralNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== BracketAccessorNode =====

type BracketAccessorNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Base                    ExpressionNode
	Subscript               ExpressionNode
	SubscriptHasAssignments bool
}

func NewBracketAccessorNode(loc JSTokenLocation, base, subscript ExpressionNode, subscriptHasAssignments bool) *BracketAccessorNode {
	return &BracketAccessorNode{
		ExpressionNodeBase:      NewExpressionNodeBase(loc, UnknownType()),
		Base:                    base,
		Subscript:               subscript,
		SubscriptHasAssignments: subscriptHasAssignments,
	}
}

func (b *BracketAccessorNode) IsLocation() bool               { return true }
func (b *BracketAccessorNode) IsBracketAccessorNode() bool   { return true }
func (b *BracketAccessorNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== DotType =====

type DotType uint8

const (
	DotTypeName         DotType = iota
	DotTypePrivateMember DotType = 1
)

// ===== BaseDotNode =====

type BaseDotNode struct {
	ExpressionNodeBase
	BaseExpr   ExpressionNode
	Ident      runtime.Identifier
	DotTypeVal DotType
}

func NewBaseDotNode(loc JSTokenLocation, base ExpressionNode, ident *runtime.Identifier, dotType DotType) BaseDotNode {
	return BaseDotNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		BaseExpr:           base,
		Ident:              *ident,
		DotTypeVal:         dotType,
	}
}

func (b *BaseDotNode) Base() ExpressionNode                    { return b.BaseExpr }
func (b *BaseDotNode) Identifier() runtime.Identifier           { return b.Ident }
func (b *BaseDotNode) Type() DotType                            { return b.DotTypeVal }
func (b *BaseDotNode) IsPrivateMember() bool                    { return b.DotTypeVal == DotTypePrivateMember }
func (b *BaseDotNode) IsArgumentsLengthAccess(vm *runtime.VM) bool {
	// 简化实现
	return false
}

func (b *BaseDotNode) EmitGetPropertyValue(generator *BytecodeGenerator, dst, base *RegisterID, thisValue *runtime.RefPtr) *RegisterID {
	return dst
}
func (b *BaseDotNode) EmitGetPropertyValueSimple(generator *BytecodeGenerator, dst, base *RegisterID) *RegisterID {
	return dst
}
func (b *BaseDotNode) EmitPutProperty(generator *BytecodeGenerator, base, value *RegisterID, thisValue *runtime.RefPtr) *RegisterID {
	return value
}
func (b *BaseDotNode) EmitPutPropertySimple(generator *BytecodeGenerator, base, value *RegisterID) *RegisterID {
	return value
}

// ===== DotAccessorNode =====

type DotAccessorNode struct {
	BaseDotNode
	ThrowableExpressionData
}

func NewDotAccessorNode(loc JSTokenLocation, base ExpressionNode, ident *runtime.Identifier, dotType DotType) *DotAccessorNode {
	baseDot := NewBaseDotNode(loc, base, ident, dotType)
	return &DotAccessorNode{
		BaseDotNode: baseDot,
	}
}

func (d *DotAccessorNode) IsLocation() bool               { return true }
func (d *DotAccessorNode) IsPrivateLocation() bool         { return d.DotTypeVal == DotTypePrivateMember }
func (d *DotAccessorNode) IsDotAccessorNode() bool         { return true }
func (d *DotAccessorNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== SpreadExpressionNode =====

type SpreadExpressionNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	SpreadExpr ExpressionNode
}

func NewSpreadExpressionNode(loc JSTokenLocation, expr ExpressionNode) *SpreadExpressionNode {
	return &SpreadExpressionNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		SpreadExpr:         expr,
	}
}

func (s *SpreadExpressionNode) Expression() ExpressionNode { return s.SpreadExpr }
func (s *SpreadExpressionNode) IsSpreadExpression() bool    { return true }
func (s *SpreadExpressionNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== ObjectSpreadExpressionNode =====

type ObjectSpreadExpressionNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	SpreadExpr ExpressionNode
}

func NewObjectSpreadExpressionNode(loc JSTokenLocation, expr ExpressionNode) *ObjectSpreadExpressionNode {
	return &ObjectSpreadExpressionNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		SpreadExpr:         expr,
	}
}

func (o *ObjectSpreadExpressionNode) Expression() ExpressionNode { return o.SpreadExpr }
func (o *ObjectSpreadExpressionNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== ArgumentListNode =====

type ArgumentListNode struct {
	ExpressionNodeBase
	Next *ArgumentListNode
	Expr ExpressionNode
}

func NewArgumentListNode(loc JSTokenLocation, expr ExpressionNode) *ArgumentListNode {
	return &ArgumentListNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Expr:               expr,
	}
}

func NewArgumentListNodeFrom(loc JSTokenLocation, prev *ArgumentListNode, expr ExpressionNode) *ArgumentListNode {
	return &ArgumentListNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Next:               prev,
		Expr:               expr,
	}
}

func (a *ArgumentListNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== ArgumentsNode =====

type ArgumentsNode struct {
	ListNode      *ArgumentListNode
	HasAssignmentsVal bool
}

func NewArgumentsNode() *ArgumentsNode {
	return &ArgumentsNode{}
}

func NewArgumentsNodeWithList(listNode *ArgumentListNode, hasAssignments bool) *ArgumentsNode {
	return &ArgumentsNode{
		ListNode:          listNode,
		HasAssignmentsVal: hasAssignments,
	}
}

func (a *ArgumentsNode) HasAssignments() bool { return a.HasAssignmentsVal }

// ===== NewExprNode =====

type NewExprNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Expr ExpressionNode
	Args *ArgumentsNode
}

func NewNewExprNode(loc JSTokenLocation, expr ExpressionNode) *NewExprNode {
	return &NewExprNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Expr:               expr,
	}
}

func NewNewExprNodeWithArgs(loc JSTokenLocation, expr ExpressionNode, args *ArgumentsNode) *NewExprNode {
	return &NewExprNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Expr:               expr,
		Args:               args,
	}
}

func (n *NewExprNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== EvalFunctionCallNode =====

type EvalFunctionCallNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Args *ArgumentsNode
}

func NewEvalFunctionCallNode(loc JSTokenLocation, args *ArgumentsNode, divot, divotStart, divotEnd JSTextPosition) *EvalFunctionCallNode {
	return &EvalFunctionCallNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		ThrowableExpressionData: NewThrowableExpressionDataFrom(divot, divotStart, divotEnd),
		Args:               args,
	}
}

func (e *EvalFunctionCallNode) IsFunctionCall() bool { return true }
func (e *EvalFunctionCallNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== FunctionCallValueNode =====

type FunctionCallValueNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Expr          ExpressionNode
	Args          *ArgumentsNode
	IsOptionalCallVal bool
}

func NewFunctionCallValueNode(loc JSTokenLocation, expr ExpressionNode, args *ArgumentsNode, divot, divotStart, divotEnd JSTextPosition, isOptionalCall bool) *FunctionCallValueNode {
	return &FunctionCallValueNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		ThrowableExpressionData: NewThrowableExpressionDataFrom(divot, divotStart, divotEnd),
		Expr:              expr,
		Args:              args,
		IsOptionalCallVal: isOptionalCall,
	}
}

func (f *FunctionCallValueNode) IsFunctionCall() bool { return true }
func (f *FunctionCallValueNode) IsOptionalCall() bool  { return f.IsOptionalCallVal }
func (f *FunctionCallValueNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== StaticBlockFunctionCallNode =====

type StaticBlockFunctionCallNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Expr ExpressionNode
}

func NewStaticBlockFunctionCallNode(loc JSTokenLocation, expr ExpressionNode, divot, divotStart, divotEnd JSTextPosition) *StaticBlockFunctionCallNode {
	return &StaticBlockFunctionCallNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		ThrowableExpressionData: NewThrowableExpressionDataFrom(divot, divotStart, divotEnd),
		Expr:              expr,
	}
}

func (s *StaticBlockFunctionCallNode) IsFunctionCall() bool { return true }
func (s *StaticBlockFunctionCallNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== FunctionCallResolveNode =====

type FunctionCallResolveNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Ident             runtime.Identifier
	Args              *ArgumentsNode
	IsOptionalCallVal bool
}

func NewFunctionCallResolveNode(loc JSTokenLocation, ident *runtime.Identifier, args *ArgumentsNode, divot, divotStart, divotEnd JSTextPosition, isOptionalCall bool) *FunctionCallResolveNode {
	return &FunctionCallResolveNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		ThrowableExpressionData: NewThrowableExpressionDataFrom(divot, divotStart, divotEnd),
		Ident:             *ident,
		Args:              args,
		IsOptionalCallVal: isOptionalCall,
	}
}

func (f *FunctionCallResolveNode) IsFunctionCall() bool { return true }
func (f *FunctionCallResolveNode) IsOptionalCall() bool  { return f.IsOptionalCallVal }
func (f *FunctionCallResolveNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== FunctionCallBracketNode =====

type FunctionCallBracketNode struct {
	ExpressionNodeBase
	ThrowableSubExpressionData
	Base                    ExpressionNode
	Subscript               ExpressionNode
	Args                    *ArgumentsNode
	SubscriptHasAssignments bool
	IsOptionalCallVal       bool
}

func NewFunctionCallBracketNode(loc JSTokenLocation, base, subscript ExpressionNode, subscriptHasAssignments bool, args *ArgumentsNode, divot, divotStart, divotEnd JSTextPosition, isOptionalCall bool) *FunctionCallBracketNode {
	return &FunctionCallBracketNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		ThrowableSubExpressionData: NewThrowableSubExpressionDataFrom(divot, divotStart, divotEnd),
		Base:                    base,
		Subscript:               subscript,
		Args:                    args,
		SubscriptHasAssignments: subscriptHasAssignments,
		IsOptionalCallVal:       isOptionalCall,
	}
}

func (f *FunctionCallBracketNode) IsFunctionCall() bool { return true }
func (f *FunctionCallBracketNode) IsOptionalCall() bool  { return f.IsOptionalCallVal }
func (f *FunctionCallBracketNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== FunctionCallDotNode =====

type FunctionCallDotNode struct {
	BaseDotNode
	ThrowableSubExpressionData
	Args              *ArgumentsNode
	IsOptionalCallVal bool
}

func NewFunctionCallDotNode(loc JSTokenLocation, base ExpressionNode, ident *runtime.Identifier, dotType DotType, args *ArgumentsNode, divot, divotStart, divotEnd JSTextPosition, isOptionalCall bool) *FunctionCallDotNode {
	baseDot := NewBaseDotNode(loc, base, ident, dotType)
	return &FunctionCallDotNode{
		BaseDotNode: baseDot,
		ThrowableSubExpressionData: NewThrowableSubExpressionDataFrom(divot, divotStart, divotEnd),
		Args:              args,
		IsOptionalCallVal: isOptionalCall,
	}
}

func (f *FunctionCallDotNode) IsFunctionCall() bool { return true }
func (f *FunctionCallDotNode) IsOptionalCall() bool  { return f.IsOptionalCallVal }
func (f *FunctionCallDotNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== BytecodeIntrinsicNode =====

type BytecodeIntrinsicType uint8

const (
	BytecodeIntrinsicConstant  BytecodeIntrinsicType = iota
	BytecodeIntrinsicFunction  BytecodeIntrinsicType = 1
)

type BytecodeIntrinsicNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Entry     BytecodeIntrinsicRegistryEntry
	Ident     runtime.Identifier
	Args      *ArgumentsNode
	IntrinsicType BytecodeIntrinsicType
}

// BytecodeIntrinsicRegistryEntry 前向声明
type BytecodeIntrinsicRegistryEntry = uintptr

func NewBytecodeIntrinsicNode(typ BytecodeIntrinsicType, loc JSTokenLocation, entry BytecodeIntrinsicRegistryEntry, ident *runtime.Identifier, args *ArgumentsNode, divot, divotStart, divotEnd JSTextPosition) *BytecodeIntrinsicNode {
	return &BytecodeIntrinsicNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		ThrowableExpressionData: NewThrowableExpressionDataFrom(divot, divotStart, divotEnd),
		Entry:             entry,
		Ident:             *ident,
		Args:              args,
		IntrinsicType:     typ,
	}
}

func (b *BytecodeIntrinsicNode) IsBytecodeIntrinsicNode() bool { return true }
func (b *BytecodeIntrinsicNode) IsFunctionCall() bool { return b.IntrinsicType == BytecodeIntrinsicFunction }
func (b *BytecodeIntrinsicNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== CallFunctionCallDotNode =====

type CallFunctionCallDotNode struct {
	FunctionCallDotNode
	DistanceToInnermostCallOrApply uintptr
}

func NewCallFunctionCallDotNode(loc JSTokenLocation, base ExpressionNode, ident *runtime.Identifier, dotType DotType, args *ArgumentsNode, divot, divotStart, divotEnd JSTextPosition, isOptionalCall bool, distance uintptr) *CallFunctionCallDotNode {
	return &CallFunctionCallDotNode{
		FunctionCallDotNode: *NewFunctionCallDotNode(loc, base, ident, dotType, args, divot, divotStart, divotEnd, isOptionalCall),
		DistanceToInnermostCallOrApply: distance,
	}
}

func (c *CallFunctionCallDotNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== ApplyFunctionCallDotNode =====

type ApplyFunctionCallDotNode struct {
	FunctionCallDotNode
	DistanceToInnermostCallOrApply uintptr
}

func NewApplyFunctionCallDotNode(loc JSTokenLocation, base ExpressionNode, ident *runtime.Identifier, dotType DotType, args *ArgumentsNode, divot, divotStart, divotEnd JSTextPosition, isOptionalCall bool, distance uintptr) *ApplyFunctionCallDotNode {
	return &ApplyFunctionCallDotNode{
		FunctionCallDotNode: *NewFunctionCallDotNode(loc, base, ident, dotType, args, divot, divotStart, divotEnd, isOptionalCall),
		DistanceToInnermostCallOrApply: distance,
	}
}

func (a *ApplyFunctionCallDotNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== HasOwnPropertyFunctionCallDotNode =====

type HasOwnPropertyFunctionCallDotNode struct {
	FunctionCallDotNode
}

func NewHasOwnPropertyFunctionCallDotNode(loc JSTokenLocation, base ExpressionNode, ident *runtime.Identifier, dotType DotType, args *ArgumentsNode, divot, divotStart, divotEnd JSTextPosition, isOptionalCall bool) *HasOwnPropertyFunctionCallDotNode {
	return &HasOwnPropertyFunctionCallDotNode{
		FunctionCallDotNode: *NewFunctionCallDotNode(loc, base, ident, dotType, args, divot, divotStart, divotEnd, isOptionalCall),
	}
}

func (h *HasOwnPropertyFunctionCallDotNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== DeleteResolveNode =====

type DeleteResolveNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Ident runtime.Identifier
}

func NewDeleteResolveNode(loc JSTokenLocation, ident *runtime.Identifier, divot, divotStart, divotEnd JSTextPosition) *DeleteResolveNode {
	return &DeleteResolveNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		ThrowableExpressionData: NewThrowableExpressionDataFrom(divot, divotStart, divotEnd),
		Ident:             *ident,
	}
}

func (d *DeleteResolveNode) IsDeleteNode() bool { return true }
func (d *DeleteResolveNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== DeleteBracketNode =====

type DeleteBracketNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Base      ExpressionNode
	Subscript ExpressionNode
}

func NewDeleteBracketNode(loc JSTokenLocation, base, subscript ExpressionNode, divot, divotStart, divotEnd JSTextPosition) *DeleteBracketNode {
	return &DeleteBracketNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		ThrowableExpressionData: NewThrowableExpressionDataFrom(divot, divotStart, divotEnd),
		Base:              base,
		Subscript:         subscript,
	}
}

func (d *DeleteBracketNode) IsDeleteNode() bool { return true }
func (d *DeleteBracketNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== DeleteDotNode =====

type DeleteDotNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Base      ExpressionNode
	Ident     runtime.Identifier
}

func NewDeleteDotNode(loc JSTokenLocation, base ExpressionNode, ident *runtime.Identifier, divot, divotStart, divotEnd JSTextPosition) *DeleteDotNode {
	return &DeleteDotNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		ThrowableExpressionData: NewThrowableExpressionDataFrom(divot, divotStart, divotEnd),
		Base:              base,
		Ident:             *ident,
	}
}

func (d *DeleteDotNode) IsDeleteNode() bool { return true }
func (d *DeleteDotNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== DeleteValueNode =====

type DeleteValueNode struct {
	ExpressionNodeBase
	Expr ExpressionNode
}

func NewDeleteValueNode(loc JSTokenLocation, expr ExpressionNode) *DeleteValueNode {
	return &DeleteValueNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Expr:               expr,
	}
}

func (d *DeleteValueNode) IsDeleteNode() bool { return true }
func (d *DeleteValueNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== VoidNode =====

type VoidNode struct {
	ExpressionNodeBase
	Expr ExpressionNode
}

func NewVoidNode(loc JSTokenLocation, expr ExpressionNode) *VoidNode {
	return &VoidNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Expr:               expr,
	}
}

func (v *VoidNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== TypeOfResolveNode =====

type TypeOfResolveNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Ident runtime.Identifier
}

func NewTypeOfResolveNode(loc JSTokenLocation, ident *runtime.Identifier, pos1, pos2, pos3 JSTextPosition) *TypeOfResolveNode {
	return &TypeOfResolveNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		ThrowableExpressionData: NewThrowableExpressionDataFrom(pos1, pos2, pos3),
		Ident:             *ident,
	}
}

func (t *TypeOfResolveNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== TypeOfValueNode =====

type TypeOfValueNode struct {
	ExpressionNodeBase
	Expr ExpressionNode
}

func NewTypeOfValueNode(loc JSTokenLocation, expr ExpressionNode) *TypeOfValueNode {
	return &TypeOfValueNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Expr:               expr,
	}
}

func (t *TypeOfValueNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== PrefixNode =====

type PrefixNode struct {
	ExpressionNodeBase
	ThrowablePrefixedSubExpressionData
	Expr     ExpressionNode
	Operator Operator
}

func NewPrefixNode(loc JSTokenLocation, expr ExpressionNode, op Operator, divot, divotStart, divotEnd JSTextPosition) *PrefixNode {
	return &PrefixNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		ThrowablePrefixedSubExpressionData: NewThrowablePrefixedSubExpressionDataFrom(divot, divotStart, divotEnd),
		Expr:               expr,
		Operator:           op,
	}
}

func (p *PrefixNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}
func (p *PrefixNode) EmitResolve(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}
func (p *PrefixNode) EmitBracket(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}
func (p *PrefixNode) EmitDot(generator *BytecodeGenerator, destination *RegisterID) *RegisterID {
	return destination
}

// ===== PostfixNode =====

type PostfixNode struct {
	PrefixNode
}

func NewPostfixNode(loc JSTokenLocation, expr ExpressionNode, op Operator, divot, divotStart, divotEnd JSTextPosition) *PostfixNode {
	return &PostfixNode{
		PrefixNode: *NewPrefixNode(loc, expr, op, divot, divotStart, divotEnd),
	}
}

func (p *PostfixNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }
func (p *PostfixNode) EmitResolve(generator *BytecodeGenerator, destination *RegisterID) *RegisterID   { return destination }
func (p *PostfixNode) EmitBracket(generator *BytecodeGenerator, destination *RegisterID) *RegisterID  { return destination }
func (p *PostfixNode) EmitDot(generator *BytecodeGenerator, destination *RegisterID) *RegisterID      { return destination }

// ===== UnaryOpNode =====

type UnaryOpNode struct {
	ExpressionNodeBase
	Expr      ExpressionNode
	OpcodeID  OpcodeID
}

func NewUnaryOpNode(loc JSTokenLocation, resultType ResultType, expr ExpressionNode, opcodeID OpcodeID) *UnaryOpNode {
	return &UnaryOpNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, resultType),
		Expr:               expr,
		OpcodeID:           opcodeID,
	}
}

func (u *UnaryOpNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== UnaryPlusNode =====

type UnaryPlusNode struct {
	UnaryOpNode
}

func NewUnaryPlusNode(loc JSTokenLocation, expr ExpressionNode) *UnaryPlusNode {
	return &UnaryPlusNode{
		UnaryOpNode: *NewUnaryOpNode(loc, UnknownType(), expr, 0),
	}
}

func (u *UnaryPlusNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }
func (u *UnaryPlusNode) StripUnaryPlus() ExpressionNode { return u.Expr }

// ===== NegateNode =====

type NegateNode struct {
	UnaryOpNode
}

func NewNegateNode(loc JSTokenLocation, expr ExpressionNode) *NegateNode {
	return &NegateNode{
		UnaryOpNode: *NewUnaryOpNode(loc, UnknownType(), expr, 0),
	}
}

// ===== BitwiseNotNode =====

type BitwiseNotNode struct {
	UnaryOpNode
}

func NewBitwiseNotNode(loc JSTokenLocation, expr ExpressionNode) *BitwiseNotNode {
	return &BitwiseNotNode{
		UnaryOpNode: *NewUnaryOpNode(loc, UnknownType(), expr, 0),
	}
}

// ===== LogicalNotNode =====

type LogicalNotNode struct {
	UnaryOpNode
}

func NewLogicalNotNode(loc JSTokenLocation, expr ExpressionNode) *LogicalNotNode {
	return &LogicalNotNode{
		UnaryOpNode: *NewUnaryOpNode(loc, UnknownType(), expr, 0),
	}
}

func (l *LogicalNotNode) EmitBytecodeInConditionContext(generator *BytecodeGenerator, trueTarget, falseTarget *Label, fallThroughMode FallThroughMode) {}

// ===== BinaryOpNode =====

type BinaryOpNode struct {
	ExpressionNodeBase
	RightHasAssignments bool
	ShouldToUnsignedResult bool
	OpcodeID           OpcodeID
	Expr1              ExpressionNode
	Expr2              ExpressionNode
}

func NewBinaryOpNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, opcodeID OpcodeID, rightHasAssignments bool) *BinaryOpNode {
	return &BinaryOpNode{
		ExpressionNodeBase:    NewExpressionNodeBase(loc, UnknownType()),
		Expr1:                 expr1,
		Expr2:                 expr2,
		OpcodeID:              opcodeID,
		RightHasAssignments:   rightHasAssignments,
		ShouldToUnsignedResult: true,
	}
}

func NewBinaryOpNodeWithResult(loc JSTokenLocation, resultType ResultType, expr1, expr2 ExpressionNode, opcodeID OpcodeID, rightHasAssignments bool) *BinaryOpNode {
	return &BinaryOpNode{
		ExpressionNodeBase:    NewExpressionNodeBase(loc, resultType),
		Expr1:                 expr1,
		Expr2:                 expr2,
		OpcodeID:              opcodeID,
		RightHasAssignments:   rightHasAssignments,
		ShouldToUnsignedResult: true,
	}
}

func (b *BinaryOpNode) IsBinaryOpNode() bool { return true }
func (b *BinaryOpNode) Lhs() ExpressionNode  { return b.Expr1 }
func (b *BinaryOpNode) Rhs() ExpressionNode  { return b.Expr2 }

func (b *BinaryOpNode) EmitStrcat(generator *BytecodeGenerator, destination, lhs *RegisterID, emitExpressionInfoForMe *ReadModifyResolveNode) *RegisterID {
	return destination
}
func (b *BinaryOpNode) EmitBytecodeInConditionContext(generator *BytecodeGenerator, trueTarget, falseTarget *Label, fallThroughMode FallThroughMode) {}
func (b *BinaryOpNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== BinaryOp 子类 =====

type PowNode struct{ BinaryOpNode }
func NewPowNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *PowNode {
	return &PowNode{BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, 0, rightHasAssignments)}
}

type MultNode struct{ BinaryOpNode }
func NewMultNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *MultNode {
	return &MultNode{BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, 0, rightHasAssignments)}
}

type DivNode struct{ BinaryOpNode }
func NewDivNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *DivNode {
	return &DivNode{BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, 0, rightHasAssignments)}
}

type ModNode struct{ BinaryOpNode }
func NewModNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *ModNode {
	return &ModNode{BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, 0, rightHasAssignments)}
}

type AddNode struct{ BinaryOpNode }
func NewAddNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *AddNode {
	return &AddNode{BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, 0, rightHasAssignments)}
}
func (a *AddNode) IsAdd() bool { return true }

type SubNode struct{ BinaryOpNode }
func NewSubNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *SubNode {
	return &SubNode{BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, 0, rightHasAssignments)}
}
func (s *SubNode) IsSubtract() bool { return true }

type LeftShiftNode struct{ BinaryOpNode }
func NewLeftShiftNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *LeftShiftNode {
	return &LeftShiftNode{BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, 0, rightHasAssignments)}
}

type RightShiftNode struct{ BinaryOpNode }
func NewRightShiftNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *RightShiftNode {
	return &RightShiftNode{BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, 0, rightHasAssignments)}
}

type UnsignedRightShiftNode struct{ BinaryOpNode }
func NewUnsignedRightShiftNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *UnsignedRightShiftNode {
	return &UnsignedRightShiftNode{BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, 0, rightHasAssignments)}
}

type LessNode struct{ BinaryOpNode }
func NewLessNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *LessNode {
	return &LessNode{BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, 0, rightHasAssignments)}
}

type GreaterNode struct{ BinaryOpNode }
func NewGreaterNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *GreaterNode {
	return &GreaterNode{BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, 0, rightHasAssignments)}
}

type LessEqNode struct{ BinaryOpNode }
func NewLessEqNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *LessEqNode {
	return &LessEqNode{BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, 0, rightHasAssignments)}
}

type GreaterEqNode struct{ BinaryOpNode }
func NewGreaterEqNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *GreaterEqNode {
	return &GreaterEqNode{BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, 0, rightHasAssignments)}
}

// ===== ThrowableBinaryOpNode =====

type ThrowableBinaryOpNode struct {
	BinaryOpNode
	ThrowableExpressionData
}

func NewThrowableBinaryOpNode(loc JSTokenLocation, resultType ResultType, expr1, expr2 ExpressionNode, opcodeID OpcodeID, rightHasAssignments bool) *ThrowableBinaryOpNode {
	return &ThrowableBinaryOpNode{
		BinaryOpNode: *NewBinaryOpNodeWithResult(loc, resultType, expr1, expr2, opcodeID, rightHasAssignments),
	}
}

func NewThrowableBinaryOpNodeSimple(loc JSTokenLocation, expr1, expr2 ExpressionNode, opcodeID OpcodeID, rightHasAssignments bool) *ThrowableBinaryOpNode {
	return &ThrowableBinaryOpNode{
		BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, opcodeID, rightHasAssignments),
	}
}

func (t *ThrowableBinaryOpNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== InstanceOfNode =====

type InstanceOfNode struct {
	ThrowableBinaryOpNode
}

func NewInstanceOfNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *InstanceOfNode {
	return &InstanceOfNode{
		ThrowableBinaryOpNode: *NewThrowableBinaryOpNodeSimple(loc, expr1, expr2, 0, rightHasAssignments),
	}
}

func (i *InstanceOfNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== InNode =====

type InNode struct {
	ThrowableBinaryOpNode
}

func NewInNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *InNode {
	return &InNode{
		ThrowableBinaryOpNode: *NewThrowableBinaryOpNodeSimple(loc, expr1, expr2, 0, rightHasAssignments),
	}
}

func (i *InNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== EqualNode =====

type EqualNode struct{ BinaryOpNode }
func NewEqualNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *EqualNode {
	return &EqualNode{BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, 0, rightHasAssignments)}
}
func (e *EqualNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

type NotEqualNode struct{ BinaryOpNode }
func NewNotEqualNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *NotEqualNode {
	return &NotEqualNode{BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, 0, rightHasAssignments)}
}

type StrictEqualNode struct{ BinaryOpNode }
func NewStrictEqualNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *StrictEqualNode {
	return &StrictEqualNode{BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, 0, rightHasAssignments)}
}
func (s *StrictEqualNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

type NotStrictEqualNode struct{ BinaryOpNode }
func NewNotStrictEqualNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *NotStrictEqualNode {
	return &NotStrictEqualNode{BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, 0, rightHasAssignments)}
}

type BitAndNode struct{ BinaryOpNode }
func NewBitAndNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *BitAndNode {
	return &BitAndNode{BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, 0, rightHasAssignments)}
}

type BitOrNode struct{ BinaryOpNode }
func NewBitOrNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *BitOrNode {
	return &BitOrNode{BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, 0, rightHasAssignments)}
}

type BitXOrNode struct{ BinaryOpNode }
func NewBitXOrNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, rightHasAssignments bool) *BitXOrNode {
	return &BitXOrNode{BinaryOpNode: *NewBinaryOpNode(loc, expr1, expr2, 0, rightHasAssignments)}
}

// ===== LogicalOpNode =====

type LogicalOpNode struct {
	ExpressionNodeBase
	LogicalOp LogicalOperator
	Expr1     ExpressionNode
	Expr2     ExpressionNode
}

func NewLogicalOpNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, op LogicalOperator) *LogicalOpNode {
	return &LogicalOpNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Expr1:              expr1,
		Expr2:              expr2,
		LogicalOp:          op,
	}
}

func (l *LogicalOpNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }
func (l *LogicalOpNode) EmitBytecodeInConditionContext(generator *BytecodeGenerator, trueTarget, falseTarget *Label, fallThroughMode FallThroughMode) {}

// ===== CoalesceNode =====

type CoalesceNode struct {
	ExpressionNodeBase
	Expr1                     ExpressionNode
	Expr2                     ExpressionNode
	HasAbsorbedOptionalChain  bool
}

func NewCoalesceNode(loc JSTokenLocation, expr1, expr2 ExpressionNode, hasAbsorbed bool) *CoalesceNode {
	return &CoalesceNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Expr1:              expr1,
		Expr2:              expr2,
		HasAbsorbedOptionalChain: hasAbsorbed,
	}
}

func (c *CoalesceNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }
func (c *CoalesceNode) EmitBytecodeInConditionContext(generator *BytecodeGenerator, trueTarget, falseTarget *Label, fallThroughMode FallThroughMode) {}

// ===== OptionalChainNode =====

type OptionalChainNode struct {
	ExpressionNodeBase
	Expr          ExpressionNode
	IsOutermost   bool
}

func NewOptionalChainNode(loc JSTokenLocation, expr ExpressionNode, isOutermost bool) *OptionalChainNode {
	return &OptionalChainNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Expr:               expr,
		IsOutermost:        isOutermost,
	}
}

func (o *OptionalChainNode) IsOptionalChain() bool { return true }
func (o *OptionalChainNode) SetExpr(expr ExpressionNode) { o.Expr = expr }
func (o *OptionalChainNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }
func (o *OptionalChainNode) EmitBytecodeInConditionContext(generator *BytecodeGenerator, trueTarget, falseTarget *Label, fallThroughMode FallThroughMode) {}

// ===== ConditionalNode =====

type ConditionalNode struct {
	ExpressionNodeBase
	Logical ExpressionNode
	Expr1   ExpressionNode
	Expr2   ExpressionNode
}

func NewConditionalNode(loc JSTokenLocation, logical, expr1, expr2 ExpressionNode) *ConditionalNode {
	return &ConditionalNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Logical:            logical,
		Expr1:              expr1,
		Expr2:              expr2,
	}
}

func (c *ConditionalNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }
func (c *ConditionalNode) EmitBytecodeInConditionContext(generator *BytecodeGenerator, trueTarget, falseTarget *Label, fallThroughMode FallThroughMode) {}

// ===== ReadModifyResolveNode =====

type ReadModifyResolveNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Ident              runtime.Identifier
	Right              ExpressionNode
	Operator           Operator
	RightHasAssignments bool
}

func NewReadModifyResolveNode(loc JSTokenLocation, ident *runtime.Identifier, op Operator, right ExpressionNode, rightHasAssignments bool, divot, divotStart, divotEnd JSTextPosition) *ReadModifyResolveNode {
	return &ReadModifyResolveNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		ThrowableExpressionData: NewThrowableExpressionDataFrom(divot, divotStart, divotEnd),
		Ident:              *ident,
		Right:              right,
		Operator:           op,
		RightHasAssignments: rightHasAssignments,
	}
}

func (r *ReadModifyResolveNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== ShortCircuitReadModifyResolveNode =====

type ShortCircuitReadModifyResolveNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Ident              runtime.Identifier
	Right              ExpressionNode
	Operator           Operator
	RightHasAssignments bool
}

func NewShortCircuitReadModifyResolveNode(loc JSTokenLocation, ident *runtime.Identifier, op Operator, right ExpressionNode, rightHasAssignments bool, divot, divotStart, divotEnd JSTextPosition) *ShortCircuitReadModifyResolveNode {
	return &ShortCircuitReadModifyResolveNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		ThrowableExpressionData: NewThrowableExpressionDataFrom(divot, divotStart, divotEnd),
		Ident:              *ident,
		Right:              right,
		Operator:           op,
		RightHasAssignments: rightHasAssignments,
	}
}

func (s *ShortCircuitReadModifyResolveNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== AssignResolveNode =====

type AssignResolveNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Ident              runtime.Identifier
	Right              ExpressionNode
	AssignmentContext   AssignmentContext
}

func NewAssignResolveNode(loc JSTokenLocation, ident *runtime.Identifier, right ExpressionNode, ctx AssignmentContext) *AssignResolveNode {
	return &AssignResolveNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Ident:             *ident,
		Right:             right,
		AssignmentContext: ctx,
	}
}

func (a *AssignResolveNode) IsAssignResolveNode() bool { return true }
func (a *AssignResolveNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== ReadModifyBracketNode =====

type ReadModifyBracketNode struct {
	ExpressionNodeBase
	ThrowableSubExpressionData
	Base      ExpressionNode
	Subscript ExpressionNode
	Right     ExpressionNode
	Operator  Operator
	SubscriptHasAssignments bool
	RightHasAssignments     bool
}

func NewReadModifyBracketNode(loc JSTokenLocation, base, subscript ExpressionNode, op Operator, right ExpressionNode, subscriptHasAssignments, rightHasAssignments bool, divot, divotStart, divotEnd JSTextPosition) *ReadModifyBracketNode {
	return &ReadModifyBracketNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		ThrowableSubExpressionData: NewThrowableSubExpressionDataFrom(divot, divotStart, divotEnd),
		Base:                    base,
		Subscript:               subscript,
		Right:                   right,
		Operator:                op,
		SubscriptHasAssignments: subscriptHasAssignments,
		RightHasAssignments:     rightHasAssignments,
	}
}

func (r *ReadModifyBracketNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== ShortCircuitReadModifyBracketNode =====

type ShortCircuitReadModifyBracketNode struct {
	ExpressionNodeBase
	ThrowableSubExpressionData
	Base      ExpressionNode
	Subscript ExpressionNode
	Right     ExpressionNode
	Operator  Operator
	SubscriptHasAssignments bool
	RightHasAssignments     bool
}

func NewShortCircuitReadModifyBracketNode(loc JSTokenLocation, base, subscript ExpressionNode, op Operator, right ExpressionNode, subscriptHasAssignments, rightHasAssignments bool, divot, divotStart, divotEnd JSTextPosition) *ShortCircuitReadModifyBracketNode {
	return &ShortCircuitReadModifyBracketNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		ThrowableSubExpressionData: NewThrowableSubExpressionDataFrom(divot, divotStart, divotEnd),
		Base:                    base,
		Subscript:               subscript,
		Right:                   right,
		Operator:                op,
		SubscriptHasAssignments: subscriptHasAssignments,
		RightHasAssignments:     rightHasAssignments,
	}
}

func (s *ShortCircuitReadModifyBracketNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== AssignBracketNode =====

type AssignBracketNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Base      ExpressionNode
	Subscript ExpressionNode
	Right     ExpressionNode
	SubscriptHasAssignments bool
	RightHasAssignments     bool
}

func NewAssignBracketNode(loc JSTokenLocation, base, subscript, right ExpressionNode, subscriptHasAssignments, rightHasAssignments bool, divot, divotStart, divotEnd JSTextPosition) *AssignBracketNode {
	return &AssignBracketNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		ThrowableExpressionData: NewThrowableExpressionDataFrom(divot, divotStart, divotEnd),
		Base:                    base,
		Subscript:               subscript,
		Right:                   right,
		SubscriptHasAssignments: subscriptHasAssignments,
		RightHasAssignments:     rightHasAssignments,
	}
}

func (a *AssignBracketNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== AssignDotNode =====

type AssignDotNode struct {
	BaseDotNode
	ThrowableExpressionData
	Right              ExpressionNode
	RightHasAssignments bool
}

func NewAssignDotNode(loc JSTokenLocation, base ExpressionNode, ident *runtime.Identifier, dotType DotType, right ExpressionNode, rightHasAssignments bool, divot, divotStart, divotEnd JSTextPosition) *AssignDotNode {
	baseDot := NewBaseDotNode(loc, base, ident, dotType)
	return &AssignDotNode{
		BaseDotNode: baseDot,
		ThrowableExpressionData: NewThrowableExpressionDataFrom(divot, divotStart, divotEnd),
		Right:              right,
		RightHasAssignments: rightHasAssignments,
	}
}

func (a *AssignDotNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== ReadModifyDotNode =====

type ReadModifyDotNode struct {
	BaseDotNode
	ThrowableSubExpressionData
	Right              ExpressionNode
	Operator           Operator
	RightHasAssignments bool
}

func NewReadModifyDotNode(loc JSTokenLocation, base ExpressionNode, ident *runtime.Identifier, dotType DotType, op Operator, right ExpressionNode, rightHasAssignments bool, divot, divotStart, divotEnd JSTextPosition) *ReadModifyDotNode {
	baseDot := NewBaseDotNode(loc, base, ident, dotType)
	return &ReadModifyDotNode{
		BaseDotNode: baseDot,
		ThrowableSubExpressionData: NewThrowableSubExpressionDataFrom(divot, divotStart, divotEnd),
		Right:              right,
		Operator:           op,
		RightHasAssignments: rightHasAssignments,
	}
}

func (r *ReadModifyDotNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== ShortCircuitReadModifyDotNode =====

type ShortCircuitReadModifyDotNode struct {
	BaseDotNode
	ThrowableSubExpressionData
	Right              ExpressionNode
	Operator           Operator
	RightHasAssignments bool
}

func NewShortCircuitReadModifyDotNode(loc JSTokenLocation, base ExpressionNode, ident *runtime.Identifier, dotType DotType, op Operator, right ExpressionNode, rightHasAssignments bool, divot, divotStart, divotEnd JSTextPosition) *ShortCircuitReadModifyDotNode {
	baseDot := NewBaseDotNode(loc, base, ident, dotType)
	return &ShortCircuitReadModifyDotNode{
		BaseDotNode: baseDot,
		ThrowableSubExpressionData: NewThrowableSubExpressionDataFrom(divot, divotStart, divotEnd),
		Right:              right,
		Operator:           op,
		RightHasAssignments: rightHasAssignments,
	}
}

func (s *ShortCircuitReadModifyDotNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== AssignErrorNode =====

type AssignErrorNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Left ExpressionNode
}

func NewAssignErrorNode(loc JSTokenLocation, left ExpressionNode, divot, divotStart, divotEnd JSTextPosition) *AssignErrorNode {
	return &AssignErrorNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		ThrowableExpressionData: NewThrowableExpressionDataFrom(divot, divotStart, divotEnd),
		Left:              left,
	}
}

func (a *AssignErrorNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== CommaNode =====

type CommaNode struct {
	ExpressionNodeBase
	Expr ExpressionNode
	NextComma *CommaNode
}

func NewCommaNode(loc JSTokenLocation, expr ExpressionNode) *CommaNode {
	return &CommaNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Expr:               expr,
	}
}

func (c *CommaNode) IsCommaNode() bool { return true }
func (c *CommaNode) SetNext(next *CommaNode) { c.NextComma = next }
func (c *CommaNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }
func (c *CommaNode) EmitBytecodeInConditionContext(generator *BytecodeGenerator, trueTarget, falseTarget *Label, fallThroughMode FallThroughMode) {}

// ===== SourceElements =====

type SourceElements struct {
	Head StatementNode
	Tail StatementNode
}

func NewSourceElements() *SourceElements {
	return &SourceElements{}
}

func (s *SourceElements) Append(stmt StatementNode) {
	if s.Head == nil {
		s.Head = stmt
		s.Tail = stmt
	} else {
		s.Tail.SetNext(stmt)
		s.Tail = stmt
	}
}

func (s *SourceElements) SingleStatement() StatementNode { return s.Head }
func (s *SourceElements) LastStatement() StatementNode   { return s.Tail }
func (s *SourceElements) HasCompletionValue() bool       { return false }
func (s *SourceElements) HasEarlyBreakOrContinue() bool  { return false }
func (s *SourceElements) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}
func (s *SourceElements) AnalyzeModule(analyzer *ModuleAnalyzer) bool { return true }

// ModuleAnalyzer 前向声明
type ModuleAnalyzer struct{}

// ===== BlockNode =====

type BlockNode struct {
	StatementNodeBase
	VariableEnvironmentNode
	Statements *SourceElements
}

func NewBlockNode(loc JSTokenLocation, statements *SourceElements, lexicalVars VariableEnvironment, functionStack []*FunctionMetadataNode) *BlockNode {
	return &BlockNode{
		StatementNodeBase:      NewStatementNodeBase(loc),
		VariableEnvironmentNode: *NewVariableEnvironmentNodeWithFuncs(lexicalVars, functionStack),
		Statements:             statements,
	}
}

func (b *BlockNode) IsBlock() bool { return true }
func (b *BlockNode) SingleStatement() StatementNode {
	if b.Statements != nil {
		return b.Statements.SingleStatement()
	}
	return nil
}
func (b *BlockNode) LastStatement() StatementNode {
	if b.Statements != nil {
		return b.Statements.LastStatement()
	}
	return nil
}
func (b *BlockNode) HasCompletionValue() bool {
	if b.Statements != nil {
		return b.Statements.HasCompletionValue()
	}
	return false
}
func (b *BlockNode) HasEarlyBreakOrContinue() bool {
	if b.Statements != nil {
		return b.Statements.HasEarlyBreakOrContinue()
	}
	return false
}
func (b *BlockNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {
	if b.Statements != nil {
		b.Statements.EmitBytecode(generator, destination)
	}
}

// ===== EmptyStatementNode =====

type EmptyStatementNode struct {
	StatementNodeBase
}

func NewEmptyStatementNode(loc JSTokenLocation) *EmptyStatementNode {
	return &EmptyStatementNode{
		StatementNodeBase: NewStatementNodeBase(loc),
	}
}

func (e *EmptyStatementNode) HasCompletionValue() bool { return false }
func (e *EmptyStatementNode) IsEmptyStatement() bool    { return true }
func (e *EmptyStatementNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== DebuggerStatementNode =====

type DebuggerStatementNode struct {
	StatementNodeBase
}

func NewDebuggerStatementNode(loc JSTokenLocation) *DebuggerStatementNode {
	return &DebuggerStatementNode{
		StatementNodeBase: NewStatementNodeBase(loc),
	}
}

func (d *DebuggerStatementNode) HasCompletionValue() bool    { return false }
func (d *DebuggerStatementNode) IsDebuggerStatement() bool   { return true }
func (d *DebuggerStatementNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== ExprStatementNode =====

type ExprStatementNode struct {
	StatementNodeBase
	Expr ExpressionNode
}

func NewExprStatementNode(loc JSTokenLocation, expr ExpressionNode) *ExprStatementNode {
	return &ExprStatementNode{
		StatementNodeBase: NewStatementNodeBase(loc),
		Expr:              expr,
	}
}

func (e *ExprStatementNode) IsExprStatement() bool { return true }
func (e *ExprStatementNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== DeclarationStatement =====

type DeclarationStatement struct {
	StatementNodeBase
	Expr ExpressionNode
}

func NewDeclarationStatement(loc JSTokenLocation, expr ExpressionNode) *DeclarationStatement {
	return &DeclarationStatement{
		StatementNodeBase: NewStatementNodeBase(loc),
		Expr:              expr,
	}
}

func (d *DeclarationStatement) HasCompletionValue() bool { return false }
func (d *DeclarationStatement) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== EmptyVarExpression =====

type EmptyVarExpression struct {
	ExpressionNodeBase
	Ident runtime.Identifier
}

func NewEmptyVarExpression(loc JSTokenLocation, ident *runtime.Identifier) *EmptyVarExpression {
	return &EmptyVarExpression{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Ident:              *ident,
	}
}

func (e *EmptyVarExpression) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== EmptyLetExpression =====

type EmptyLetExpression struct {
	ExpressionNodeBase
	Ident runtime.Identifier
}

func NewEmptyLetExpression(loc JSTokenLocation, ident *runtime.Identifier) *EmptyLetExpression {
	return &EmptyLetExpression{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Ident:              *ident,
	}
}

func (e *EmptyLetExpression) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== IfElseNode =====

type IfElseNode struct {
	StatementNodeBase
	Condition ExpressionNode
	IfBlock   StatementNode
	ElseBlock StatementNode
}

func NewIfElseNode(loc JSTokenLocation, condition ExpressionNode, ifBlock, elseBlock StatementNode) *IfElseNode {
	return &IfElseNode{
		StatementNodeBase: NewStatementNodeBase(loc),
		Condition:         condition,
		IfBlock:           ifBlock,
		ElseBlock:         elseBlock,
	}
}

func (i *IfElseNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== DoWhileNode =====

type DoWhileNode struct {
	StatementNodeBase
	Statement StatementNode
	Expr      ExpressionNode
}

func NewDoWhileNode(loc JSTokenLocation, stmt StatementNode, expr ExpressionNode) *DoWhileNode {
	return &DoWhileNode{
		StatementNodeBase: NewStatementNodeBase(loc),
		Statement:         stmt,
		Expr:              expr,
	}
}

func (d *DoWhileNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== WhileNode =====

type WhileNode struct {
	StatementNodeBase
	Expr      ExpressionNode
	Statement StatementNode
}

func NewWhileNode(loc JSTokenLocation, expr ExpressionNode, stmt StatementNode) *WhileNode {
	return &WhileNode{
		StatementNodeBase: NewStatementNodeBase(loc),
		Expr:              expr,
		Statement:         stmt,
	}
}

func (w *WhileNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== ForNode =====

type ForNode struct {
	StatementNodeBase
	VariableEnvironmentNode
	Expr1                    ExpressionNode
	Expr2                    ExpressionNode
	Expr3                    ExpressionNode
	Statement                StatementNode
	InitializerContainsClosure bool
}

func NewForNode(loc JSTokenLocation, expr1, expr2, expr3 ExpressionNode, stmt StatementNode, lexicalVars VariableEnvironment, initializerContainsClosure bool) *ForNode {
	return &ForNode{
		StatementNodeBase:      NewStatementNodeBase(loc),
		VariableEnvironmentNode: *NewVariableEnvironmentNodeWithVars(lexicalVars),
		Expr1:                   expr1,
		Expr2:                   expr2,
		Expr3:                   expr3,
		Statement:               stmt,
		InitializerContainsClosure: initializerContainsClosure,
	}
}

func (f *ForNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== EnumerationNode =====

type EnumerationNode struct {
	StatementNodeBase
	ThrowableExpressionData
	VariableEnvironmentNode
	Lexpr     ExpressionNode
	Expr      ExpressionNode
	Statement StatementNode
}

func NewEnumerationNode(loc JSTokenLocation, lexpr, expr ExpressionNode, stmt StatementNode, lexicalVars VariableEnvironment) *EnumerationNode {
	return &EnumerationNode{
		StatementNodeBase:      NewStatementNodeBase(loc),
		VariableEnvironmentNode: *NewVariableEnvironmentNodeWithVars(lexicalVars),
		Lexpr:                   lexpr,
		Expr:                    expr,
		Statement:               stmt,
	}
}

// ===== ForInNode =====

type ForInNode struct {
	EnumerationNode
}

func NewForInNode(loc JSTokenLocation, lexpr, expr ExpressionNode, stmt StatementNode, lexicalVars VariableEnvironment) *ForInNode {
	return &ForInNode{
		EnumerationNode: *NewEnumerationNode(loc, lexpr, expr, stmt, lexicalVars),
	}
}

func (f *ForInNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}
func (f *ForInNode) TryGetBoundLocal(generator *BytecodeGenerator) *RegisterID { return nil }
func (f *ForInNode) EmitLoopHeader(generator *BytecodeGenerator, propertyName *RegisterID) {}

// ===== ForOfNode =====

type ForOfNode struct {
	EnumerationNode
	IsForAwait bool
}

func NewForOfNode(isForAwait bool, loc JSTokenLocation, lexpr, expr ExpressionNode, stmt StatementNode, lexicalVars VariableEnvironment) *ForOfNode {
	return &ForOfNode{
		EnumerationNode: *NewEnumerationNode(loc, lexpr, expr, stmt, lexicalVars),
		IsForAwait:      isForAwait,
	}
}

func (f *ForOfNode) IsForOfNode() bool { return true }
func (f *ForOfNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== ContinueNode =====

type ContinueNode struct {
	StatementNodeBase
	ThrowableExpressionData
	Ident runtime.Identifier
}

func NewContinueNode(loc JSTokenLocation, ident *runtime.Identifier) *ContinueNode {
	return &ContinueNode{
		StatementNodeBase: NewStatementNodeBase(loc),
		Ident:             *ident,
	}
}

func (c *ContinueNode) HasCompletionValue() bool              { return false }
func (c *ContinueNode) HasEarlyBreakOrContinue() bool          { return true }
func (c *ContinueNode) IsContinue() bool                       { return true }
func (c *ContinueNode) TrivialTarget(generator *BytecodeGenerator) *Label { return nil }
func (c *ContinueNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== BreakNode =====

type BreakNode struct {
	StatementNodeBase
	ThrowableExpressionData
	Ident runtime.Identifier
}

func NewBreakNode(loc JSTokenLocation, ident *runtime.Identifier) *BreakNode {
	return &BreakNode{
		StatementNodeBase: NewStatementNodeBase(loc),
		Ident:             *ident,
	}
}

func (b *BreakNode) HasCompletionValue() bool              { return false }
func (b *BreakNode) HasEarlyBreakOrContinue() bool          { return true }
func (b *BreakNode) IsBreak() bool                          { return true }
func (b *BreakNode) TrivialTarget(generator *BytecodeGenerator) *Label { return nil }
func (b *BreakNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== ReturnNode =====

type ReturnNode struct {
	StatementNodeBase
	ThrowableExpressionData
	Value ExpressionNode
}

func NewReturnNode(loc JSTokenLocation, value ExpressionNode) *ReturnNode {
	return &ReturnNode{
		StatementNodeBase: NewStatementNodeBase(loc),
		Value:             value,
	}
}

func (r *ReturnNode) IsReturnNode() bool { return true }
func (r *ReturnNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== WithNode =====

type WithNode struct {
	StatementNodeBase
	Expr             ExpressionNode
	Statement        StatementNode
	Divot            JSTextPosition
	ExpressionLength uint32
}

func NewWithNode(loc JSTokenLocation, expr ExpressionNode, stmt StatementNode, divot JSTextPosition, expressionLength uint32) *WithNode {
	return &WithNode{
		StatementNodeBase: NewStatementNodeBase(loc),
		Expr:              expr,
		Statement:         stmt,
		Divot:             divot,
		ExpressionLength:  expressionLength,
	}
}

func (w *WithNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== LabelNode =====

type LabelNode struct {
	StatementNodeBase
	ThrowableExpressionData
	Name      runtime.Identifier
	Statement StatementNode
}

func NewLabelNode(loc JSTokenLocation, name *runtime.Identifier, stmt StatementNode) *LabelNode {
	return &LabelNode{
		StatementNodeBase: NewStatementNodeBase(loc),
		Name:              *name,
		Statement:         stmt,
	}
}

func (l *LabelNode) IsLabel() bool { return true }
func (l *LabelNode) HasCompletionValue() bool        { return l.Statement.HasCompletionValue() }
func (l *LabelNode) HasEarlyBreakOrContinue() bool   { return l.Statement.HasEarlyBreakOrContinue() }
func (l *LabelNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== ThrowNode =====

type ThrowNode struct {
	StatementNodeBase
	ThrowableExpressionData
	Expr ExpressionNode
}

func NewThrowNode(loc JSTokenLocation, expr ExpressionNode) *ThrowNode {
	return &ThrowNode{
		StatementNodeBase: NewStatementNodeBase(loc),
		Expr:              expr,
	}
}

func (t *ThrowNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== TryNode =====

type TryNode struct {
	StatementNodeBase
	VariableEnvironmentNode
	TryBlock     StatementNode
	CatchPattern *DestructuringPatternNode
	CatchBlock   StatementNode
	FinallyBlock StatementNode
}

func NewTryNode(loc JSTokenLocation, tryBlock StatementNode, catchPattern *DestructuringPatternNode, catchBlock StatementNode, catchEnvironment VariableEnvironment, finallyBlock StatementNode) *TryNode {
	return &TryNode{
		StatementNodeBase:      NewStatementNodeBase(loc),
		VariableEnvironmentNode: *NewVariableEnvironmentNodeWithVars(catchEnvironment),
		TryBlock:               tryBlock,
		CatchPattern:           catchPattern,
		CatchBlock:             catchBlock,
		FinallyBlock:           finallyBlock,
	}
}

func (t *TryNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== ScopeNode =====

type ScopeNode struct {
	StatementNodeBase
	ParserArenaRoot
	VariableEnvironmentNode
	StartLineNumber               int
	StartStartOffsetVal           uint32
	StartLineStartOffsetVal       uint32
	Features                       CodeFeatures
	LexicallyScopedFeaturesVal     LexicallyScopedFeatures
	InnerArrowFunctionCodeFeaturesVal InnerArrowFunctionCodeFeatures
	Source                         SourceCode
	VarDeclarations                VariableEnvironment
	NumConstants                   int
	Statements                     *SourceElements
}

type ParserArenaRoot struct {
	Arena *ParserArena
}

func NewParserArenaRoot(arena *ParserArena) *ParserArenaRoot {
	return &ParserArenaRoot{Arena: arena}
}

func NewScopeNode(arena *ParserArena, start, end JSTokenLocation, lexicallyScopedFeatures LexicallyScopedFeatures) *ScopeNode {
	return &ScopeNode{
		StatementNodeBase:          NewStatementNodeBase(start),
		ParserArenaRoot:            *NewParserArenaRoot(arena),
		LexicallyScopedFeaturesVal: lexicallyScopedFeatures,
	}
}

func NewScopeNodeFull(arena *ParserArena, start, end JSTokenLocation, source SourceCode, statements *SourceElements, lexicalVars VariableEnvironment, functionStack []*FunctionMetadataNode, varDeclarations VariableEnvironment, features CodeFeatures, lexicallyScopedFeatures LexicallyScopedFeatures, innerArrowFeatures InnerArrowFunctionCodeFeatures, numConstants int) *ScopeNode {
	return &ScopeNode{
		StatementNodeBase:                 NewStatementNodeBase(start),
		ParserArenaRoot:                   *NewParserArenaRoot(arena),
		VariableEnvironmentNode:           *NewVariableEnvironmentNodeWithFuncs(lexicalVars, functionStack),
		Features:                          features,
		LexicallyScopedFeaturesVal:        lexicallyScopedFeatures,
		InnerArrowFunctionCodeFeaturesVal: innerArrowFeatures,
		Source:                            source,
		VarDeclarations:                   varDeclarations,
		NumConstants:                      numConstants,
		Statements:                        statements,
	}
}

func (s *ScopeNode) SourceCode() *SourceCode { return &s.Source }
func (s *ScopeNode) SourceID() uintptr       { return s.Source.ProviderID() }
func (s *ScopeNode) StartLine() int          { return s.StartLineNumber }
func (s *ScopeNode) StartStartOffset() int   { return int(s.StartStartOffsetVal) }
func (s *ScopeNode) StartLineStartOffset() int { return int(s.StartLineStartOffsetVal) }
func (s *ScopeNode) GetFeatures() CodeFeatures                                          { return s.Features }
func (s *ScopeNode) GetLexicallyScopedFeatures() LexicallyScopedFeatures                { return s.LexicallyScopedFeaturesVal }
func (s *ScopeNode) GetInnerArrowFunctionCodeFeatures() InnerArrowFunctionCodeFeatures  { return s.InnerArrowFunctionCodeFeaturesVal }
func (s *ScopeNode) DoAnyInnerArrowFunctionsUseAnyFeature() bool                         { return s.InnerArrowFunctionCodeFeaturesVal != NoInnerArrowFunctionFeatures }
func (s *ScopeNode) DoAnyInnerArrowFunctionsUseArguments() bool                          { return s.InnerArrowFunctionCodeFeaturesVal&ArgumentsInnerArrowFunctionFeature != 0 }
func (s *ScopeNode) DoAnyInnerArrowFunctionsUseSuperCall() bool                          { return s.InnerArrowFunctionCodeFeaturesVal&SuperCallInnerArrowFunctionFeature != 0 }
func (s *ScopeNode) DoAnyInnerArrowFunctionsUseSuperProperty() bool                      { return s.InnerArrowFunctionCodeFeaturesVal&SuperPropertyInnerArrowFunctionFeature != 0 }
func (s *ScopeNode) DoAnyInnerArrowFunctionsUseEval() bool                               { return s.InnerArrowFunctionCodeFeaturesVal&EvalInnerArrowFunctionFeature != 0 }
func (s *ScopeNode) DoAnyInnerArrowFunctionsUseThis() bool                               { return s.InnerArrowFunctionCodeFeaturesVal&ThisInnerArrowFunctionFeature != 0 }
func (s *ScopeNode) DoAnyInnerArrowFunctionsUseNewTarget() bool                           { return s.InnerArrowFunctionCodeFeaturesVal&NewTargetInnerArrowFunctionFeature != 0 }

func (s *ScopeNode) UsesEval() bool     { return s.Features&EvalFeature != 0 }
func (s *ScopeNode) HasShadowsArgumentsFeature() bool { return s.Features&ShadowsArgumentsFeature != 0 }
func (s *ScopeNode) UsesArguments() bool { return (s.Features&ArgumentsFeature != 0) && (s.Features&ShadowsArgumentsFeature == 0) }
func (s *ScopeNode) UsesArrowFunction() bool          { return s.Features&ArrowFunctionFeature != 0 }
func (s *ScopeNode) IsStrictMode() bool               { return s.LexicallyScopedFeaturesVal&StrictModeLexicallyScopedFeature != 0 }
func (s *ScopeNode) UsesThis() bool                   { return s.Features&ThisFeature != 0 }
func (s *ScopeNode) UsesSuperCall() bool              { return s.Features&SuperCallFeature != 0 }
func (s *ScopeNode) UsesSuperProperty() bool          { return s.Features&SuperPropertyFeature != 0 }
func (s *ScopeNode) UsesNewTarget() bool              { return s.Features&NewTargetFeature != 0 }
func (s *ScopeNode) IsAsyncFunctionWithoutAwait() bool { return s.Features&AsyncFunctionWithoutAwaitFeature != 0 }
func (s *ScopeNode) NeedsActivation() bool             { return s.HasCapturedVariables() || (s.Features&(EvalFeature|WithFeature) != 0) }
func (s *ScopeNode) HasCapturedVariables() bool        { return s.VarDeclarations.HasCapturedVariables() }
func (s *ScopeNode) Captures(uid runtime.UniquedStringImplPtr) bool { return s.VarDeclarations.Captures(uid) }
func (s *ScopeNode) UsesNonSimpleParameterList() bool  { return s.Features&NonSimpleParameterListFeature != 0 }
func (s *ScopeNode) NeedsNewTargetRegisterForThisScope() bool { return s.UsesSuperCall() || s.UsesNewTarget() }
func (s *ScopeNode) VarDeclarationsRef() *VariableEnvironment { return &s.VarDeclarations }
func (s *ScopeNode) NeededConstants() int               { return s.NumConstants + 2 }
func (s *ScopeNode) SingleStatement() StatementNode {
	if s.Statements != nil {
		return s.Statements.SingleStatement()
	}
	return nil
}
func (s *ScopeNode) IsEmptyBody() bool { return s.Statements == nil }
func (s *ScopeNode) HasCompletionValue() bool {
	if s.Statements != nil {
		return s.Statements.HasCompletionValue()
	}
	return false
}
func (s *ScopeNode) HasEarlyBreakOrContinue() bool {
	if s.Statements != nil {
		return s.Statements.HasEarlyBreakOrContinue()
	}
	return false
}
func (s *ScopeNode) EmitStatementsBytecode(generator *BytecodeGenerator, destination *RegisterID) {
	if s.Statements != nil {
		s.Statements.EmitBytecode(generator, destination)
	}
}
func (s *ScopeNode) AnalyzeModule(analyzer *ModuleAnalyzer) bool {
	if s.Statements != nil {
		return s.Statements.AnalyzeModule(analyzer)
	}
	return true
}
func (s *ScopeNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {
	s.EmitStatementsBytecode(generator, destination)
}

// ===== ProgramNode =====

type ProgramNode struct {
	ScopeNode
	StartColumnVal uint32
	EndColumnVal   uint32
	ScopeIsFunction bool
}

func NewProgramNode(arena *ParserArena, start, end JSTokenLocation, startColumn, endColumn uint32, statements *SourceElements, lexicalVars VariableEnvironment, functionStack []*FunctionMetadataNode, varDeclarations VariableEnvironment, params *FunctionParameters, source SourceCode, features CodeFeatures, lexicallyScopedFeatures LexicallyScopedFeatures, innerArrowFeatures InnerArrowFunctionCodeFeatures, numConstants int, moduleScopeData *ModuleScopeData) *ProgramNode {
	return &ProgramNode{
		ScopeNode:       *NewScopeNodeFull(arena, start, end, source, statements, lexicalVars, functionStack, varDeclarations, features, lexicallyScopedFeatures, innerArrowFeatures, numConstants),
		StartColumnVal:  startColumn,
		EndColumnVal:    endColumn,
		ScopeIsFunction: false,
	}
}

func (p *ProgramNode) StartColumn() uint32 { return p.StartColumnVal }
func (p *ProgramNode) EndColumn() uint32   { return p.EndColumnVal }
func (p *ProgramNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {
	p.EmitStatementsBytecode(generator, destination)
}

// ===== EvalNode =====

type EvalNode struct {
	ScopeNode
	EndColumnVal   uint32
	ScopeIsFunction bool
}

func NewEvalNode(arena *ParserArena, start, end JSTokenLocation, startColumn, endColumn uint32, statements *SourceElements, lexicalVars VariableEnvironment, functionStack []*FunctionMetadataNode, varDeclarations VariableEnvironment, params *FunctionParameters, source SourceCode, features CodeFeatures, lexicallyScopedFeatures LexicallyScopedFeatures, innerArrowFeatures InnerArrowFunctionCodeFeatures, numConstants int, moduleScopeData *ModuleScopeData) *EvalNode {
	return &EvalNode{
		ScopeNode:       *NewScopeNodeFull(arena, start, end, source, statements, lexicalVars, functionStack, varDeclarations, features, lexicallyScopedFeatures, innerArrowFeatures, numConstants),
		EndColumnVal:    endColumn,
		ScopeIsFunction: false,
	}
}

func (e *EvalNode) StartColumn() uint32 { return 0 }
func (e *EvalNode) EndColumn() uint32   { return e.EndColumnVal }
func (e *EvalNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {
	e.EmitStatementsBytecode(generator, destination)
}

// ===== ModuleProgramNode =====

type ModuleProgramNode struct {
	ScopeNode
	StartColumnVal  uint32
	EndColumnVal    uint32
	UsesAwait       bool
	ModuleScopeDataVal *ModuleScopeData
	ScopeIsFunction bool
}

func NewModuleProgramNode(arena *ParserArena, start, end JSTokenLocation, startColumn, endColumn uint32, statements *SourceElements, lexicalVars VariableEnvironment, functionStack []*FunctionMetadataNode, varDeclarations VariableEnvironment, params *FunctionParameters, source SourceCode, features CodeFeatures, lexicallyScopedFeatures LexicallyScopedFeatures, innerArrowFeatures InnerArrowFunctionCodeFeatures, numConstants int, moduleScopeData *ModuleScopeData) *ModuleProgramNode {
	return &ModuleProgramNode{
		ScopeNode:          *NewScopeNodeFull(arena, start, end, source, statements, lexicalVars, functionStack, varDeclarations, features, lexicallyScopedFeatures, innerArrowFeatures, numConstants),
		StartColumnVal:     startColumn,
		EndColumnVal:       endColumn,
		ModuleScopeDataVal: moduleScopeData,
		ScopeIsFunction:    false,
	}
}

func (m *ModuleProgramNode) StartColumn() uint32 { return m.StartColumnVal }
func (m *ModuleProgramNode) EndColumn() uint32   { return m.EndColumnVal }
func (m *ModuleProgramNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {
	m.EmitStatementsBytecode(generator, destination)
}

// ===== ModuleNameNode =====

type ModuleNameNode struct {
	Node
	ModuleName runtime.Identifier
}

func NewModuleNameNode(loc JSTokenLocation, moduleName *runtime.Identifier) *ModuleNameNode {
	return &ModuleNameNode{
		Node:       *NewNode(loc),
		ModuleName: *moduleName,
	}
}

// ===== ImportSpecifierNode =====

type ImportSpecifierNode struct {
	Node
	ImportedName runtime.Identifier
	LocalName    runtime.Identifier
}

func NewImportSpecifierNode(loc JSTokenLocation, importedName, localName *runtime.Identifier) *ImportSpecifierNode {
	return &ImportSpecifierNode{
		Node:         *NewNode(loc),
		ImportedName: *importedName,
		LocalName:    *localName,
	}
}

// ===== ImportSpecifierListNode =====

type ImportSpecifierListNode struct {
	Specifiers []*ImportSpecifierNode
}

func (l *ImportSpecifierListNode) Append(specifier *ImportSpecifierNode) {
	l.Specifiers = append(l.Specifiers, specifier)
}

// ===== ImportAttributesListNode =====

type ImportAttributesListNode struct {
	Attributes []struct{ Key, Value *runtime.Identifier }
}

func (l *ImportAttributesListNode) Append(key, value *runtime.Identifier) {
	l.Attributes = append(l.Attributes, struct{ Key, Value *runtime.Identifier }{key, value})
}

// ===== ModuleDeclarationNode =====

type ModuleDeclarationNode struct {
	StatementNodeBase
}

func NewModuleDeclarationNode(loc JSTokenLocation) *ModuleDeclarationNode {
	return &ModuleDeclarationNode{
		StatementNodeBase: NewStatementNodeBase(loc),
	}
}

func (m *ModuleDeclarationNode) HasCompletionValue() bool          { return false }
func (m *ModuleDeclarationNode) IsModuleDeclarationNode() bool     { return true }
func (m *ModuleDeclarationNode) AnalyzeModule(analyzer *ModuleAnalyzer) bool { return true }
func (m *ModuleDeclarationNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== ImportDeclarationNode =====

type ImportType uint8

const (
	ImportTypeNormal   ImportType = iota
	ImportTypeDeferred ImportType = 1
)

type ImportDeclarationNode struct {
	ModuleDeclarationNode
	SpecifierList  *ImportSpecifierListNode
	ModuleNameVal  *ModuleNameNode
	AttributesList *ImportAttributesListNode
	ImportTypeVal  ImportType
}

func NewImportDeclarationNode(loc JSTokenLocation, importType ImportType, specifiers *ImportSpecifierListNode, moduleName *ModuleNameNode, attributes *ImportAttributesListNode) *ImportDeclarationNode {
	return &ImportDeclarationNode{
		ModuleDeclarationNode: *NewModuleDeclarationNode(loc),
		SpecifierList:        specifiers,
		ModuleNameVal:        moduleName,
		AttributesList:       attributes,
		ImportTypeVal:        importType,
	}
}

func (i *ImportDeclarationNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}
func (i *ImportDeclarationNode) AnalyzeModule(analyzer *ModuleAnalyzer) bool { return true }

// ===== ExportAllDeclarationNode =====

type ExportAllDeclarationNode struct {
	ModuleDeclarationNode
	ModuleNameVal  *ModuleNameNode
	AttributesList *ImportAttributesListNode
}

func NewExportAllDeclarationNode(loc JSTokenLocation, moduleName *ModuleNameNode, attributes *ImportAttributesListNode) *ExportAllDeclarationNode {
	return &ExportAllDeclarationNode{
		ModuleDeclarationNode: *NewModuleDeclarationNode(loc),
		ModuleNameVal:        moduleName,
		AttributesList:       attributes,
	}
}

func (e *ExportAllDeclarationNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}
func (e *ExportAllDeclarationNode) AnalyzeModule(analyzer *ModuleAnalyzer) bool { return true }

// ===== ExportDefaultDeclarationNode =====

type ExportDefaultDeclarationNode struct {
	ModuleDeclarationNode
	Declaration StatementNode
	LocalName   runtime.Identifier
}

func NewExportDefaultDeclarationNode(loc JSTokenLocation, decl StatementNode, localName *runtime.Identifier) *ExportDefaultDeclarationNode {
	return &ExportDefaultDeclarationNode{
		ModuleDeclarationNode: *NewModuleDeclarationNode(loc),
		Declaration:          decl,
		LocalName:            *localName,
	}
}

func (e *ExportDefaultDeclarationNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}
func (e *ExportDefaultDeclarationNode) AnalyzeModule(analyzer *ModuleAnalyzer) bool { return true }

// ===== ExportLocalDeclarationNode =====

type ExportLocalDeclarationNode struct {
	ModuleDeclarationNode
	Declaration StatementNode
}

func NewExportLocalDeclarationNode(loc JSTokenLocation, decl StatementNode) *ExportLocalDeclarationNode {
	return &ExportLocalDeclarationNode{
		ModuleDeclarationNode: *NewModuleDeclarationNode(loc),
		Declaration:          decl,
	}
}

func (e *ExportLocalDeclarationNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}
func (e *ExportLocalDeclarationNode) AnalyzeModule(analyzer *ModuleAnalyzer) bool { return true }

// ===== ExportSpecifierNode =====

type ExportSpecifierNode struct {
	Node
	LocalName    runtime.Identifier
	ExportedName runtime.Identifier
}

func NewExportSpecifierNode(loc JSTokenLocation, localName, exportedName *runtime.Identifier) *ExportSpecifierNode {
	return &ExportSpecifierNode{
		Node:         *NewNode(loc),
		LocalName:    *localName,
		ExportedName: *exportedName,
	}
}

// ===== ExportSpecifierListNode =====

type ExportSpecifierListNode struct {
	Specifiers []*ExportSpecifierNode
}

func (l *ExportSpecifierListNode) Append(specifier *ExportSpecifierNode) {
	l.Specifiers = append(l.Specifiers, specifier)
}

// ===== ExportNamedDeclarationNode =====

type ExportNamedDeclarationNode struct {
	ModuleDeclarationNode
	SpecifierList  *ExportSpecifierListNode
	ModuleNameVal  *ModuleNameNode
	AttributesList *ImportAttributesListNode
}

func NewExportNamedDeclarationNode(loc JSTokenLocation, specifiers *ExportSpecifierListNode, moduleName *ModuleNameNode, attributes *ImportAttributesListNode) *ExportNamedDeclarationNode {
	return &ExportNamedDeclarationNode{
		ModuleDeclarationNode: *NewModuleDeclarationNode(loc),
		SpecifierList:        specifiers,
		ModuleNameVal:        moduleName,
		AttributesList:       attributes,
	}
}

func (e *ExportNamedDeclarationNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}
func (e *ExportNamedDeclarationNode) AnalyzeModule(analyzer *ModuleAnalyzer) bool { return true }

// ===== FunctionMetadataNode =====

type FunctionMetadataNode struct {
	Node
	ImplementationVisibilityVal    ImplementationVisibility
	LexicallyScopedFeaturesVal     LexicallyScopedFeatures
	SuperBindingVal                SuperBinding
	ConstructorKindVal             ConstructorKind
	NeedsClassFieldInitializer     bool
	IsArrowFunctionBodyExpression  bool
	IsSloppyModeHoistedFunctionVal bool
	PrivateBrandRequirementVal     PrivateBrandRequirement
	ParseMode                      SourceParseMode
	FunctionMode                   FunctionMode
	Ident                          runtime.Identifier
	EcmaNameVal                   runtime.Identifier
	StartColumnVal                 uint32
	EndColumnVal                   uint32
	FunctionStart                  uint32
	FunctionNameStartVal           int
	ParametersStartVal             int
	Source                         SourceCode
	ClassSource                    SourceCode
	StartStartOffset               int
	ParameterCountVal              uint32
	LastLineVal                    int
	BitWidthOfImplementationVisibility uint32
}

func NewFunctionMetadataNode(arena *ParserArena, start, end JSTokenLocation, startColumn, endColumn uint32, functionStart uint32, functionNameStart, parametersStart int, implementationVisibility ImplementationVisibility, lexicallyScopedFeatures LexicallyScopedFeatures, constructorKind ConstructorKind, superBinding SuperBinding, parameterCount uint32, parseMode SourceParseMode, isArrowFunctionBodyExpression bool) *FunctionMetadataNode {
	return &FunctionMetadataNode{
		Node:                             *NewNode(start),
		ImplementationVisibilityVal:      implementationVisibility,
		LexicallyScopedFeaturesVal:       lexicallyScopedFeatures,
		ConstructorKindVal:               constructorKind,
		SuperBindingVal:                  superBinding,
		ParameterCountVal:                parameterCount,
		ParseMode:                        parseMode,
		IsArrowFunctionBodyExpression:    isArrowFunctionBodyExpression,
		StartColumnVal:                   startColumn,
		EndColumnVal:                     endColumn,
		FunctionStart:                    functionStart,
		FunctionNameStartVal:           functionNameStart,
		ParametersStartVal:              parametersStart,
		LastLineVal:                      0,
	}
}

func NewFunctionMetadataNodeSimple(start, end JSTokenLocation, startColumn, endColumn uint32, functionStart uint32, functionNameStart, parametersStart int, implementationVisibility ImplementationVisibility, lexicallyScopedFeatures LexicallyScopedFeatures, constructorKind ConstructorKind, superBinding SuperBinding, parameterCount uint32, parseMode SourceParseMode, isArrowFunctionBodyExpression bool) *FunctionMetadataNode {
	return NewFunctionMetadataNode(nil, start, end, startColumn, endColumn, functionStart, functionNameStart, parametersStart, implementationVisibility, lexicallyScopedFeatures, constructorKind, superBinding, parameterCount, parseMode, isArrowFunctionBodyExpression)
}

func (f *FunctionMetadataNode) Dump() {}

func (f *FunctionMetadataNode) FinishParsing(source SourceCode, ident *runtime.Identifier, mode FunctionMode) {
	f.Source = source
	f.Ident = *ident
	f.FunctionMode = mode
}

func (f *FunctionMetadataNode) OverrideName(ident *runtime.Identifier) { f.Ident = *ident }
func (f *FunctionMetadataNode) SetEcmaName(ecmaName *runtime.Identifier) { f.EcmaNameVal = *ecmaName }
func (f *FunctionMetadataNode) EcmaName() runtime.Identifier {
	if f.Ident.IsEmpty() {
		return f.EcmaNameVal
	}
	return f.Ident
}
func (f *FunctionMetadataNode) SetPrivateBrandRequirement(req PrivateBrandRequirement) { f.PrivateBrandRequirementVal = req }
func (f *FunctionMetadataNode) GetPrivateBrandRequirement() PrivateBrandRequirement { return f.PrivateBrandRequirementVal }
func (f *FunctionMetadataNode) GetFunctionMode() FunctionMode                       { return f.FunctionMode }
func (f *FunctionMetadataNode) FunctionNameStart() int                              { return f.FunctionNameStartVal }
func (f *FunctionMetadataNode) FunctionStartPos() uint32                            { return f.FunctionStart }
func (f *FunctionMetadataNode) ParametersStartPos() int                             { return f.ParametersStartVal }
func (f *FunctionMetadataNode) StartColumn() uint32                                 { return f.StartColumnVal }
func (f *FunctionMetadataNode) EndColumn() uint32                                   { return f.EndColumnVal }
func (f *FunctionMetadataNode) ParameterCount() uint32                              { return f.ParameterCountVal }
func (f *FunctionMetadataNode) ParseModeVal() SourceParseMode                       { return f.ParseMode }
func (f *FunctionMetadataNode) SetEndPosition(pos JSTextPosition) {}
func (f *FunctionMetadataNode) SourceRef() *SourceCode                              { return &f.Source }
func (f *FunctionMetadataNode) ClassSourceRef() *SourceCode                         { return &f.ClassSource }
func (f *FunctionMetadataNode) SetClassSource(source SourceCode)                    { f.ClassSource = source }
func (f *FunctionMetadataNode) StartStartOffsetVal() int                            { return f.StartStartOffset }
func (f *FunctionMetadataNode) GetImplementationVisibility() ImplementationVisibility { return f.ImplementationVisibilityVal }
func (f *FunctionMetadataNode) GetLexicallyScopedFeatures() LexicallyScopedFeatures { return f.LexicallyScopedFeaturesVal }
func (f *FunctionMetadataNode) GetSuperBinding() SuperBinding                       { return f.SuperBindingVal }
func (f *FunctionMetadataNode) GetConstructorKind() ConstructorKind                 { return f.ConstructorKindVal }
func (f *FunctionMetadataNode) IsConstructorAndNeedsClassFieldInitializer() bool    { return f.NeedsClassFieldInitializer }
func (f *FunctionMetadataNode) SetNeedsClassFieldInitializer(value bool)            { f.NeedsClassFieldInitializer = value }

func (f *FunctionMetadataNode) IsSloppyModeHoistedFunction() bool           { return f.IsSloppyModeHoistedFunctionVal }
func (f *FunctionMetadataNode) SetIsSloppyModeHoistedFunction()             { f.IsSloppyModeHoistedFunctionVal = true }
func (f *FunctionMetadataNode) IsArrowFunctionBodyExpressionFlag() bool     { return f.IsArrowFunctionBodyExpression }

func (f *FunctionMetadataNode) SetLoc(firstLine, lastLine uint32, startOffset, lineStartOffset int) {
	f.LastLineVal = int(lastLine)
	f.Position = JSTextPosition{Line: int(firstLine), Offset: startOffset, LineStartOffset: lineStartOffset}
}
func (f *FunctionMetadataNode) LastLine() uint32 { return uint32(f.LastLineVal) }

func (f *FunctionMetadataNode) Equals(other *FunctionMetadataNode) bool {
	return false // 简化实现
}

// ===== FunctionNode =====

type FunctionNode struct {
	ScopeNode
	Ident        runtime.Identifier
	FunctionMode FunctionMode
	Parameters   *FunctionParameters
	StartColumnVal uint32
	EndColumnVal   uint32
	ScopeIsFunction bool
}

func NewFunctionNode(arena *ParserArena, start, end JSTokenLocation, startColumn, endColumn uint32, statements *SourceElements, lexicalVars VariableEnvironment, functionStack []*FunctionMetadataNode, varDeclarations VariableEnvironment, params *FunctionParameters, source SourceCode, features CodeFeatures, lexicallyScopedFeatures LexicallyScopedFeatures, innerArrowFeatures InnerArrowFunctionCodeFeatures, numConstants int, moduleScopeData *ModuleScopeData) *FunctionNode {
	return &FunctionNode{
		ScopeNode:       *NewScopeNodeFull(arena, start, end, source, statements, lexicalVars, functionStack, varDeclarations, features, lexicallyScopedFeatures, innerArrowFeatures, numConstants),
		Parameters:      params,
		StartColumnVal:  startColumn,
		EndColumnVal:    endColumn,
		ScopeIsFunction: true,
	}
}

func (f *FunctionNode) IsFunctionNode() bool { return true }
func (f *FunctionNode) FinishParsing(ident *runtime.Identifier, mode FunctionMode) {
	f.Ident = *ident
	f.FunctionMode = mode
}
func (f *FunctionNode) StartColumn() uint32 { return f.StartColumnVal }
func (f *FunctionNode) EndColumn() uint32   { return f.EndColumnVal }
func (f *FunctionNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== BaseFuncExprNode =====

type BaseFuncExprNode struct {
	ExpressionNodeBase
	Metadata *FunctionMetadataNode
}

func NewBaseFuncExprNode(loc JSTokenLocation, ident *runtime.Identifier, metadata *FunctionMetadataNode, source SourceCode, mode FunctionMode) *BaseFuncExprNode {
	return &BaseFuncExprNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Metadata:           metadata,
	}
}

func (b *BaseFuncExprNode) IsBaseFuncExprNode() bool { return true }

// ===== FuncExprNode =====

type FuncExprNode struct {
	BaseFuncExprNode
}

func NewFuncExprNode(loc JSTokenLocation, ident *runtime.Identifier, metadata *FunctionMetadataNode, source SourceCode) *FuncExprNode {
	return &FuncExprNode{
		BaseFuncExprNode: *NewBaseFuncExprNode(loc, ident, metadata, source, FunctionModeNormal),
	}
}

func NewFuncExprNodeWithMode(loc JSTokenLocation, ident *runtime.Identifier, metadata *FunctionMetadataNode, source SourceCode, mode FunctionMode) *FuncExprNode {
	return &FuncExprNode{
		BaseFuncExprNode: *NewBaseFuncExprNode(loc, ident, metadata, source, mode),
	}
}

func (f *FuncExprNode) IsFuncExprNode() bool { return true }
func (f *FuncExprNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== ArrowFuncExprNode =====

type ArrowFuncExprNode struct {
	BaseFuncExprNode
}

func NewArrowFuncExprNode(loc JSTokenLocation, ident *runtime.Identifier, metadata *FunctionMetadataNode, source SourceCode) *ArrowFuncExprNode {
	return &ArrowFuncExprNode{
		BaseFuncExprNode: *NewBaseFuncExprNode(loc, ident, metadata, source, FunctionModeNormal),
	}
}

func (a *ArrowFuncExprNode) IsArrowFuncExprNode() bool { return true }
func (a *ArrowFuncExprNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== MethodDefinitionNode =====

type MethodDefinitionNode struct {
	FuncExprNode
}

func NewMethodDefinitionNode(loc JSTokenLocation, ident *runtime.Identifier, metadata *FunctionMetadataNode, source SourceCode) *MethodDefinitionNode {
	return &MethodDefinitionNode{
		FuncExprNode: *NewFuncExprNode(loc, ident, metadata, source),
	}
}

func (m *MethodDefinitionNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== YieldExprNode =====

type YieldExprNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Argument  ExpressionNode
	Delegate  bool
}

func NewYieldExprNode(loc JSTokenLocation, argument ExpressionNode, delegate bool) *YieldExprNode {
	return &YieldExprNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Argument:           argument,
		Delegate:           delegate,
	}
}

func (y *YieldExprNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== AwaitExprNode =====

type AwaitExprNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	Argument ExpressionNode
}

func NewAwaitExprNode(loc JSTokenLocation, argument ExpressionNode) *AwaitExprNode {
	return &AwaitExprNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Argument:           argument,
	}
}

func (a *AwaitExprNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== DefineFieldNode =====

type DefineFieldType uint8

const (
	DefineFieldTypeName        DefineFieldType = iota
	DefineFieldTypePrivateName DefineFieldType = 1
	DefineFieldTypeComputedName DefineFieldType = 2
)

type DefineFieldNode struct {
	StatementNodeBase
	Ident  runtime.Identifier
	Assign ExpressionNode
	Type   DefineFieldType
}

func NewDefineFieldNode(loc JSTokenLocation, ident *runtime.Identifier, assign ExpressionNode, typ DefineFieldType) *DefineFieldNode {
	return &DefineFieldNode{
		StatementNodeBase: NewStatementNodeBase(loc),
		Ident:             *ident,
		Assign:            assign,
		Type:              typ,
	}
}

func (d *DefineFieldNode) IsDefineFieldNode() bool { return true }
func (d *DefineFieldNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== ClassExprNode =====

type ClassExprNode struct {
	ExpressionNodeBase
	ThrowableExpressionData
	VariableEnvironmentNode
	ClassHeadEnvironment  VariableEnvironment
	ClassSource           SourceCode
	Name                  runtime.Identifier
	EcmaNamePtr           *runtime.Identifier
	ConstructorExpression ExpressionNode
	ClassHeritage         ExpressionNode
	ClassElements         *PropertyListNode
	NeedsLexicalScope     bool
}

func NewClassExprNode(loc JSTokenLocation, name *runtime.Identifier, classSource SourceCode, classHeadEnvironment VariableEnvironment, classEnvironment VariableEnvironment, constructorExpression, parentClass ExpressionNode, classElements *PropertyListNode) *ClassExprNode {
	return &ClassExprNode{
		ExpressionNodeBase:      NewExpressionNodeBase(loc, UnknownType()),
		VariableEnvironmentNode: *NewVariableEnvironmentNodeWithVars(classEnvironment),
		ClassHeadEnvironment:    classHeadEnvironment,
		ClassSource:             classSource,
		Name:                    *name,
		ConstructorExpression:   constructorExpression,
		ClassHeritage:           parentClass,
		ClassElements:           classElements,
	}
}

func (c *ClassExprNode) IsClassExprNode() bool { return true }
func (c *ClassExprNode) EcmaName() runtime.Identifier {
	if c.EcmaNamePtr != nil {
		return *c.EcmaNamePtr
	}
	return c.Name
}
func (c *ClassExprNode) SetEcmaName(name *runtime.Identifier) {
	if c.Name.IsNull() {
		c.EcmaNamePtr = name
	} else {
		c.EcmaNamePtr = &c.Name
	}
}
func (c *ClassExprNode) HasStaticProperty(propName *runtime.Identifier) bool {
	return c.ClassElements != nil && c.ClassElements.HasStaticallyNamedProperty(propName)
}
func (c *ClassExprNode) HasInstanceFieldsFlag() bool {
	return c.ClassElements != nil && c.ClassElements.HasInstanceFields()
}
func (c *ClassExprNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== DestructuringPatternNode =====

type DestructuringPatternNode struct {
}

func NewDestructuringPatternNode() *DestructuringPatternNode {
	return &DestructuringPatternNode{}
}

func (d *DestructuringPatternNode) CollectBoundIdentifiers() []runtime.Identifier { return nil }
func (d *DestructuringPatternNode) BindValue(generator *BytecodeGenerator, source *RegisterID) {}
func (d *DestructuringPatternNode) ToString() string { return "" }
func (d *DestructuringPatternNode) IsBindingNode() bool { return false }
func (d *DestructuringPatternNode) IsAssignmentElementNode() bool { return false }
func (d *DestructuringPatternNode) IsRestParameter() bool { return false }
func (d *DestructuringPatternNode) BindValueCanThrow(generator *BytecodeGenerator) bool { return true }
func (d *DestructuringPatternNode) WritableDirectBindingIfPossible(generator *BytecodeGenerator) *RegisterID { return nil }
func (d *DestructuringPatternNode) FinishDirectBindingAssignment(generator *BytecodeGenerator) {}

// DestructuringPatternNode 接口扩展（由子类实现）
type DestructuringPatternNodeInterface interface {
	CollectBoundIdentifiers() []runtime.Identifier
	BindValue(generator *BytecodeGenerator, source *RegisterID)
	ToString() string
	IsBindingNode() bool
	IsAssignmentElementNode() bool
	IsRestParameter() bool
	BindValueCanThrow(generator *BytecodeGenerator) bool
	WritableDirectBindingIfPossible(generator *BytecodeGenerator) *RegisterID
	FinishDirectBindingAssignment(generator *BytecodeGenerator)
}

// ===== ArrayPatternNode =====

type BindingType uint8

const (
	BindingTypeElision    BindingType = iota
	BindingTypeElement    BindingType = 1
	BindingTypeRestElement BindingType = 2
)

type ArrayPatternEntry struct {
	BindingType   BindingType
	Pattern       *DestructuringPatternNode
	DefaultValue  ExpressionNode
}

type ArrayPatternNode struct {
	DestructuringPatternNode
	TargetPatterns []ArrayPatternEntry
}

func NewArrayPatternNode() *ArrayPatternNode {
	return &ArrayPatternNode{}
}

func (a *ArrayPatternNode) AppendIndex(bindingType BindingType, loc JSTokenLocation, node *DestructuringPatternNode, defaultValue ExpressionNode) {
	a.TargetPatterns = append(a.TargetPatterns, ArrayPatternEntry{
		BindingType:  bindingType,
		Pattern:      node,
		DefaultValue: defaultValue,
	})
}

func (a *ArrayPatternNode) CollectBoundIdentifiers() []runtime.Identifier { return nil }
func (a *ArrayPatternNode) BindValue(generator *BytecodeGenerator, reg *RegisterID) {}
func (a *ArrayPatternNode) ToString() string { return "" }

// ===== ObjectPatternNode =====

type ObjectPatternEntry struct {
	PropertyName     runtime.Identifier
	PropertyExpression ExpressionNode
	WasString        bool
	Pattern          *DestructuringPatternNode
	DefaultValue     ExpressionNode
	BindingType      BindingType
}

type ObjectPatternNode struct {
	DestructuringPatternNode
	ContainsRestElement      bool
	ContainsComputedProperty bool
	TargetPatterns           []ObjectPatternEntry
}

func NewObjectPatternNode() *ObjectPatternNode {
	return &ObjectPatternNode{}
}

func (o *ObjectPatternNode) AppendEntry(loc JSTokenLocation, identifier *runtime.Identifier, wasString bool, pattern *DestructuringPatternNode, defaultValue ExpressionNode, bindingType BindingType) {
	o.TargetPatterns = append(o.TargetPatterns, ObjectPatternEntry{
		PropertyName: *identifier,
		WasString:    wasString,
		Pattern:      pattern,
		DefaultValue: defaultValue,
		BindingType:  bindingType,
	})
}

func (o *ObjectPatternNode) AppendEntryExpr(vm *runtime.VM, loc JSTokenLocation, propertyExpression ExpressionNode, pattern *DestructuringPatternNode, defaultValue ExpressionNode, bindingType BindingType) {
	o.TargetPatterns = append(o.TargetPatterns, ObjectPatternEntry{
		PropertyExpression: propertyExpression,
		Pattern:            pattern,
		DefaultValue:       defaultValue,
		BindingType:        bindingType,
	})
}

func (o *ObjectPatternNode) SetContainsRestElement(v bool)    { o.ContainsRestElement = v }
func (o *ObjectPatternNode) SetContainsComputedProperty(v bool) { o.ContainsComputedProperty = v }
func (o *ObjectPatternNode) CollectBoundIdentifiers() []runtime.Identifier { return nil }
func (o *ObjectPatternNode) BindValue(generator *BytecodeGenerator, reg *RegisterID) {}
func (o *ObjectPatternNode) ToString() string { return "" }

// ===== BindingNode =====

type BindingNode struct {
	DestructuringPatternNode
	DivotStart     JSTextPosition
	DivotEnd       JSTextPosition
	BoundProperty  runtime.Identifier
	BindingContext AssignmentContext
}

func NewBindingNode(boundProperty *runtime.Identifier, start, end JSTextPosition, ctx AssignmentContext) *BindingNode {
	return &BindingNode{
		BoundProperty:  *boundProperty,
		DivotStart:     start,
		DivotEnd:       end,
		BindingContext: ctx,
	}
}

func (b *BindingNode) IsBindingNode() bool { return true }
func (b *BindingNode) CollectBoundIdentifiers() []runtime.Identifier { return nil }
func (b *BindingNode) BindValue(generator *BytecodeGenerator, reg *RegisterID) {}
func (b *BindingNode) ToString() string { return "" }
func (b *BindingNode) BindValueCanThrow(generator *BytecodeGenerator) bool { return false }
func (b *BindingNode) WritableDirectBindingIfPossible(generator *BytecodeGenerator) *RegisterID { return nil }
func (b *BindingNode) FinishDirectBindingAssignment(generator *BytecodeGenerator) {}

// ===== RestParameterNode =====

type RestParameterNode struct {
	DestructuringPatternNode
	Pattern             *DestructuringPatternNode
	NumParametersToSkip uint32
}

func NewRestParameterNode(pattern *DestructuringPatternNode, numParamsToSkip uint32) *RestParameterNode {
	return &RestParameterNode{
		Pattern:             pattern,
		NumParametersToSkip: numParamsToSkip,
	}
}

func (r *RestParameterNode) IsRestParameter() bool { return true }
func (r *RestParameterNode) Emit(generator *BytecodeGenerator) {}
func (r *RestParameterNode) CollectBoundIdentifiers() []runtime.Identifier { return nil }
func (r *RestParameterNode) BindValue(generator *BytecodeGenerator, reg *RegisterID) {}
func (r *RestParameterNode) ToString() string { return "" }

// ===== AssignmentElementNode =====

type AssignmentElementNode struct {
	DestructuringPatternNode
	DivotStart       JSTextPosition
	DivotEnd         JSTextPosition
	AssignmentTarget ExpressionNode
}

func NewAssignmentElementNode(assignmentTarget ExpressionNode, start, end JSTextPosition) *AssignmentElementNode {
	return &AssignmentElementNode{
		AssignmentTarget: assignmentTarget,
		DivotStart:       start,
		DivotEnd:         end,
	}
}

func (a *AssignmentElementNode) IsAssignmentElementNode() bool { return true }
func (a *AssignmentElementNode) CollectBoundIdentifiers() []runtime.Identifier { return nil }
func (a *AssignmentElementNode) BindValue(generator *BytecodeGenerator, reg *RegisterID) {}
func (a *AssignmentElementNode) ToString() string { return "" }
func (a *AssignmentElementNode) BindValueCanThrow(generator *BytecodeGenerator) bool { return false }
func (a *AssignmentElementNode) WritableDirectBindingIfPossible(generator *BytecodeGenerator) *RegisterID { return nil }
func (a *AssignmentElementNode) FinishDirectBindingAssignment(generator *BytecodeGenerator) {}
func (a *AssignmentElementNode) EmitNodesForDestructuring(generator *BytecodeGenerator, base, propertyName *runtime.RefPtr) *DestructuringPatternNode_BaseAndPropertyName { return nil }
func (a *AssignmentElementNode) BindValueWithEmittedNodes(generator *BytecodeGenerator, pair *DestructuringPatternNode_BaseAndPropertyName, reg *RegisterID) {}

// DestructuringPatternNode_BaseAndPropertyName 类型
type DestructuringPatternNode_BaseAndPropertyName struct{}

// ===== DestructuringAssignmentNode =====

type DestructuringAssignmentNode struct {
	ExpressionNodeBase
	Bindings     *DestructuringPatternNode
	Initializer  ExpressionNode
}

func NewDestructuringAssignmentNode(loc JSTokenLocation, bindings *DestructuringPatternNode, initializer ExpressionNode) *DestructuringAssignmentNode {
	return &DestructuringAssignmentNode{
		ExpressionNodeBase: NewExpressionNodeBase(loc, UnknownType()),
		Bindings:           bindings,
		Initializer:        initializer,
	}
}

func (d *DestructuringAssignmentNode) IsAssignmentLocation() bool { return true }
func (d *DestructuringAssignmentNode) IsDestructuringNode() bool  { return true }
func (d *DestructuringAssignmentNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) *RegisterID { return destination }

// ===== FuncDeclNode =====

type FuncDeclNode struct {
	StatementNodeBase
	Metadata *FunctionMetadataNode
}

func NewFuncDeclNode(loc JSTokenLocation, ident *runtime.Identifier, metadata *FunctionMetadataNode, source SourceCode) *FuncDeclNode {
	return &FuncDeclNode{
		StatementNodeBase: NewStatementNodeBase(loc),
		Metadata:          metadata,
	}
}

func (f *FuncDeclNode) HasCompletionValue() bool { return false }
func (f *FuncDeclNode) IsFuncDeclNode() bool     { return true }
func (f *FuncDeclNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== ClassDeclNode =====

type ClassDeclNode struct {
	StatementNodeBase
	ClassDeclaration ExpressionNode
}

func NewClassDeclNode(loc JSTokenLocation, classExpression ExpressionNode) *ClassDeclNode {
	return &ClassDeclNode{
		StatementNodeBase:  NewStatementNodeBase(loc),
		ClassDeclaration:   classExpression,
	}
}

func (c *ClassDeclNode) HasCompletionValue() bool { return false }
func (c *ClassDeclNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== CaseClauseNode =====

type CaseClauseNode struct {
	Expr       ExpressionNode
	Statements *SourceElements
	StartOffset int
}

func NewCaseClauseNode(expr ExpressionNode, statements *SourceElements) *CaseClauseNode {
	return &CaseClauseNode{
		Expr:       expr,
		Statements: statements,
	}
}

func (c *CaseClauseNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}
func (c *CaseClauseNode) SetStartOffset(offset int) { c.StartOffset = offset }

// ===== ClauseListNode =====

type ClauseListNode struct {
	Clause *CaseClauseNode
	Next   *ClauseListNode
}

func NewClauseListNode(clause *CaseClauseNode) *ClauseListNode {
	return &ClauseListNode{Clause: clause}
}

func NewClauseListNodeFrom(prev *ClauseListNode, clause *CaseClauseNode) *ClauseListNode {
	return &ClauseListNode{
		Clause: clause,
		Next:   prev,
	}
}

func (c *ClauseListNode) GetClause() *CaseClauseNode { return c.Clause }
func (c *ClauseListNode) GetNext() *ClauseListNode   { return c.Next }

// ===== CaseBlockNode =====

type CaseBlockNode struct {
	List1          *ClauseListNode
	DefaultClause  *CaseClauseNode
	List2          *ClauseListNode
}

func NewCaseBlockNode(list1 *ClauseListNode, defaultClause *CaseClauseNode, list2 *ClauseListNode) *CaseBlockNode {
	return &CaseBlockNode{
		List1:         list1,
		DefaultClause: defaultClause,
		List2:         list2,
	}
}

func (c *CaseBlockNode) EmitBytecodeForBlock(generator *BytecodeGenerator, input, destination *RegisterID) {}
func (c *CaseBlockNode) TryTableSwitch(literalVector []ExpressionNode, minNum, maxNum *int32) SwitchType {
	return SwitchTypeNone
}

const TableSwitchMinimum uintptr = 3

// ===== SwitchNode =====

type SwitchNode struct {
	StatementNodeBase
	VariableEnvironmentNode
	Expr  ExpressionNode
	Block *CaseBlockNode
}

func NewSwitchNode(loc JSTokenLocation, expr ExpressionNode, block *CaseBlockNode, lexicalVars VariableEnvironment, functionStack []*FunctionMetadataNode) *SwitchNode {
	return &SwitchNode{
		StatementNodeBase:      NewStatementNodeBase(loc),
		VariableEnvironmentNode: *NewVariableEnvironmentNodeWithFuncs(lexicalVars, functionStack),
		Expr:                   expr,
		Block:                  block,
	}
}

func (s *SwitchNode) EmitBytecode(generator *BytecodeGenerator, destination *RegisterID) {}

// ===== 列表结构体 =====

type ElementList struct {
	Head *ElementNode
	Tail *ElementNode
}

type PropertyList struct {
	Head *PropertyListNode
	Tail *PropertyListNode
}

type ArgumentList struct {
	Head *ArgumentListNode
	Tail *ArgumentListNode
}

type ClauseList struct {
	Head *ClauseListNode
	Tail *ClauseListNode
}


