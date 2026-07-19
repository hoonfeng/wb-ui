// 版权所有 (C) 2010-2019 Apple Inc. 保留所有权利。
//
// 使用约定：BSD 许可证
//
// 从 WebKit Source/JavaScriptCore/parser/ASTBuilder.h 翻译为 Go

package parser

import "wb-ui/jsc/runtime"

// BinaryOpInfo 二元操作信息
type BinaryOpInfo struct {
	Start         JSTextPosition
	Divot         JSTextPosition
	End           JSTextPosition
	HasAssignment bool
}

func NewBinaryOpInfo(otherStart, otherDivot, otherEnd JSTextPosition, rhsHasAssignment bool) BinaryOpInfo {
	return BinaryOpInfo{Start: otherStart, Divot: otherDivot, End: otherEnd, HasAssignment: rhsHasAssignment}
}

func NewBinaryOpInfoFromLHS(lhs, rhs BinaryOpInfo) BinaryOpInfo {
	return BinaryOpInfo{Start: lhs.Start, Divot: rhs.Start, End: rhs.End, HasAssignment: lhs.HasAssignment || rhs.HasAssignment}
}

// AssignmentInfo 赋值信息
type AssignmentInfo struct {
	Node           ExpressionNode
	Start          JSTextPosition
	Divot          JSTextPosition
	InitAssignments int
	Op             Operator
}

func NewAssignmentInfo(node ExpressionNode, start, divot JSTextPosition, initAssignments int, op Operator) AssignmentInfo {
	return AssignmentInfo{Node: node, Start: start, Divot: divot, InitAssignments: initAssignments, Op: op}
}

// ASTBuilder AST 构建器
type ASTBuilder struct {
	VM                  *runtime.VM
	ParserArena         *ParserArena
	SourceCode          *SourceCode
	Scope               ASTBuilderScope
	BinaryOperandStack  []BinaryOperandPair
	AssignmentInfoStack []AssignmentInfo
	BinaryOperatorStack []BinaryOperatorEntry
	UnaryTokenStack     []UnaryTokenEntry
	EvalCount           int
}

type ASTBuilderScope struct {
	Features     int
	NumConstants int
}

type BinaryOperandPair struct {
	Node ExpressionNode
	Info BinaryOpInfo
}

type BinaryOperatorEntry struct {
	Op         int
	Precedence int
}

type UnaryTokenEntry struct {
	Type  int
	Start JSTextPosition
}

func NewASTBuilder(vm *runtime.VM, parserArena *ParserArena, sourceCode *SourceCode) *ASTBuilder {
	return &ASTBuilder{VM: vm, ParserArena: parserArena, SourceCode: sourceCode}
}

type BinaryExprContext struct{}
func NewBinaryExprContext(ASTBuilder) BinaryExprContext { return BinaryExprContext{} }

type UnaryExprContext struct{}
func NewUnaryExprContext(ASTBuilder) UnaryExprContext { return UnaryExprContext{} }

const CreatesAST = true
const NeedsFreeVariableInfo = true
const CanUseFunctionCache = true

func (b *ASTBuilder) CreateSourceElements() *SourceElements  { return &SourceElements{} }
func (b *ASTBuilder) Features() int                          { return b.Scope.Features }
func (b *ASTBuilder) NumConstantsVal() int                   { return b.Scope.NumConstants }
func (b *ASTBuilder) EvalCountVal() int                      { return b.EvalCount }

func (b *ASTBuilder) IncConstants()    { b.Scope.NumConstants++ }
func (b *ASTBuilder) UsesThis()        { b.Scope.Features |= int(ThisFeature) }
func (b *ASTBuilder) UsesArguments()   { b.Scope.Features |= int(ArgumentsFeature) }
func (b *ASTBuilder) UsesWith()        { b.Scope.Features |= int(WithFeature) }
func (b *ASTBuilder) UsesSuperCall()   { b.Scope.Features |= int(SuperCallFeature) }
func (b *ASTBuilder) UsesSuperProperty() { b.Scope.Features |= int(SuperPropertyFeature) }
func (b *ASTBuilder) UsesNewTarget()   { b.Scope.Features |= int(NewTargetFeature) }
func (b *ASTBuilder) UsesAwait()        { b.Scope.Features |= 1 << 7 /* AwaitFeature */ }
func (b *ASTBuilder) UsesArrowFunction() { b.Scope.Features |= int(ArrowFunctionFeature) }
func (b *ASTBuilder) UsesEval()        { b.EvalCount++; b.Scope.Features |= int(EvalFeature) }
