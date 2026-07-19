// 版权所有 (C) 2010-2019 Apple Inc. 保留所有权利。
//
// 使用约定：BSD 许可证
//
// 从 WebKit Source/JavaScriptCore/parser/SyntaxChecker.h 翻译为 Go

package parser

import "wb-ui/jsc/runtime"

// SyntaxChecker 语法检查器（不构建 AST，只做语法验证）
type SyntaxChecker struct {
	VM            *runtime.VM
	TopBinaryExpr int
	TopUnaryToken int
}

const SyntaxCheckerMetaPropertyBit = 0x80000000

const (
	SyntaxCheckerNoneExpr                  = 0
	SyntaxCheckerResolveEvalExpr           = 1
	SyntaxCheckerResolveExpr               = 2
	SyntaxCheckerIntegerExpr               = 3
	SyntaxCheckerDoubleExpr                = 4
	SyntaxCheckerStringExpr                = 5
	SyntaxCheckerBigIntExpr                = 6
	SyntaxCheckerThisExpr                  = 7
	SyntaxCheckerNullExpr                  = 8
	SyntaxCheckerBoolExpr                  = 9
	SyntaxCheckerRegExpExpr                = 10
	SyntaxCheckerObjectLiteralExpr         = 11
	SyntaxCheckerFunctionExpr              = 12
	SyntaxCheckerClassExpr                 = 13
	SyntaxCheckerSuperExpr                 = 14
	SyntaxCheckerImportExpr                = 15
	SyntaxCheckerBracketExpr               = 16
	SyntaxCheckerDotExpr                   = 17
	SyntaxCheckerCallExpr                  = 18
	SyntaxCheckerNewExpr                   = 19
	SyntaxCheckerPreExpr                   = 20
	SyntaxCheckerPostExpr                  = 21
	SyntaxCheckerUnaryExpr                 = 22
	SyntaxCheckerBinaryExpr                = 23
	SyntaxCheckerOptionalChain             = 24
	SyntaxCheckerPrivateDotExpr            = 25
	SyntaxCheckerConditionalExpr           = 26
	SyntaxCheckerAssignmentExpr            = 27
	SyntaxCheckerTypeofExpr                = 28
	SyntaxCheckerDeleteExpr                = 29
	SyntaxCheckerArrayLiteralExpr          = 30
	SyntaxCheckerBindingDestructuring      = 31
	SyntaxCheckerRestParameter             = 32
	SyntaxCheckerArrayDestructuring        = 33
	SyntaxCheckerObjectDestructuring       = 34
	SyntaxCheckerSourceElementsResult      = 35
	SyntaxCheckerFunctionBodyResult        = 36
	SyntaxCheckerSpreadExpr                = 37
	SyntaxCheckerObjectSpreadExpr          = 38
	SyntaxCheckerArgumentsResult           = 39
	SyntaxCheckerPropertyListResult        = 40
	SyntaxCheckerArgumentsListResult       = 41
	SyntaxCheckerElementsListResult        = 42
	SyntaxCheckerStatementResult           = 43
	SyntaxCheckerFormalParameterListResult = 44
	SyntaxCheckerClauseResult              = 45
	SyntaxCheckerClauseListResult          = 46
	SyntaxCheckerCommaExpr                 = 47
	SyntaxCheckerDestructuringAssignment   = 48
	SyntaxCheckerTemplateStringResult      = 49
	SyntaxCheckerTemplateStringListResult  = 50
	SyntaxCheckerTemplateExpressionListResult = 51
	SyntaxCheckerTemplateExpr              = 52
	SyntaxCheckerTaggedTemplateExpr        = 53
	SyntaxCheckerYieldExpr                 = 54
	SyntaxCheckerAwaitExpr                 = 55
	SyntaxCheckerModuleNameResult          = 56
	SyntaxCheckerPrivateIdentifier         = 57
	SyntaxCheckerImportSpecifierResult     = 58
	SyntaxCheckerImportSpecifierListResult = 59
	SyntaxCheckerImportAttributesListResult = 60
	SyntaxCheckerExportSpecifierResult     = 61
	SyntaxCheckerExportSpecifierListResult = 62

	SyntaxCheckerNewTargetExpr  = SyntaxCheckerMetaPropertyBit | 0
	SyntaxCheckerImportMetaExpr = SyntaxCheckerMetaPropertyBit | 1
)

type SyntaxCheckerExpressionType = int

func NewSyntaxChecker(vm *runtime.VM, _ interface{}) SyntaxChecker {
	return SyntaxChecker{VM: vm}
}

const SyntaxCheckerCreatesAST = false
const SyntaxCheckerNeedsFreeVariableInfo = false
const SyntaxCheckerCanUseFunctionCache = true

func (s *SyntaxChecker) CreateSourceElements() int { return SyntaxCheckerSourceElementsResult }

func (s *SyntaxChecker) IsMetaProperty(typ int) bool { return typ&SyntaxCheckerMetaPropertyBit != 0 }
func (s *SyntaxChecker) IsNewTarget(typ int) bool    { return typ == SyntaxCheckerNewTargetExpr }
func (s *SyntaxChecker) IsImportMeta(typ int) bool   { return typ == SyntaxCheckerImportMetaExpr }
func (s *SyntaxChecker) IsBindingNode(_ int) bool    { return true }
func (s *SyntaxChecker) IsLocation(typ int) bool {
	return typ == SyntaxCheckerResolveExpr || typ == SyntaxCheckerDotExpr || typ == SyntaxCheckerPrivateDotExpr || typ == SyntaxCheckerBracketExpr
}
func (s *SyntaxChecker) IsPrivateLocation(typ int) bool { return typ == SyntaxCheckerPrivateDotExpr }
func (s *SyntaxChecker) IsAssignmentLocation(typ int) bool {
	return s.IsLocation(typ) || typ == SyntaxCheckerDestructuringAssignment
}
func (s *SyntaxChecker) IsObjectLiteral(typ int) bool  { return typ == SyntaxCheckerObjectLiteralExpr }
func (s *SyntaxChecker) IsArrayLiteral(typ int) bool   { return typ == SyntaxCheckerArrayLiteralExpr }
func (s *SyntaxChecker) IsObjectOrArrayLiteral(typ int) bool {
	return s.IsObjectLiteral(typ) || s.IsArrayLiteral(typ)
}
func (s *SyntaxChecker) IsFunctionCall(typ int) bool   { return typ == SyntaxCheckerCallExpr }
func (s *SyntaxChecker) IsResolve(typ int) bool {
	return typ == SyntaxCheckerResolveExpr || typ == SyntaxCheckerResolveEvalExpr
}
func (s *SyntaxChecker) ShouldSkipPauseLocation(_ int) bool { return true }
func (s *SyntaxChecker) EvalCount() int                     { return 0 }
func (s *SyntaxChecker) PropagateArgumentsUse()             {}
func (s *SyntaxChecker) HasArgumentsFeature() bool          { return true }
func (s *SyntaxChecker) SetEndOffset(_, _ int)              {}
func (s *SyntaxChecker) EndOffset(_ int) int                { return 0 }
func (s *SyntaxChecker) SetStartOffset(_, _ int)           {}
