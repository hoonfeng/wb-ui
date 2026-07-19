// 版权所有 (C) 2012-2013, 2015-2016 Apple Inc. 保留所有权利。
//
// 使用约定：BSD 许可证
//
// 从 WebKit Source/JavaScriptCore/parser/ParserModes.h 翻译为 Go

package parser

import (
	"wb-ui/jsc/runtime"
)

// JSParserBuiltinMode 表示内置模式
type JSParserBuiltinMode uint8

const (
	NotBuiltin JSParserBuiltinMode = 0
	Builtin    JSParserBuiltinMode = 1
)

// JSParserScriptMode 表示脚本模式
type JSParserScriptMode uint8

const (
	Classic JSParserScriptMode = 0
	Module  JSParserScriptMode = 1
)

// CodeGenerationMode 表示代码生成模式（位掩码）
type CodeGenerationMode uint8

const (
	CodeGenerationDebugger             CodeGenerationMode = 1 << 0
	CodeGenerationTypeProfiler         CodeGenerationMode = 1 << 1
	CodeGenerationControlFlowProfiler  CodeGenerationMode = 1 << 2
)

// FunctionConstructionMode 表示函数构造模式
type FunctionConstructionMode uint8

const (
	FunctionConstructionFunction       FunctionConstructionMode = 0
	FunctionConstructionGenerator      FunctionConstructionMode = 1
	FunctionConstructionAsync          FunctionConstructionMode = 2
	FunctionConstructionAsyncGenerator FunctionConstructionMode = 3
)

// SourceParseModeSet 表示解析模式集合的位掩码
type SourceParseModeSet struct {
	mask uint32
}

// NewSourceParseModeSet 创建包含指定模式的集合
func NewSourceParseModeSet(modes ...SourceParseMode) SourceParseModeSet {
	s := SourceParseModeSet{}
	for _, mode := range modes {
		s.mask |= 1 << uint32(mode)
	}
	return s
}

// Contains 检查集合中是否包含指定的模式
func (s SourceParseModeSet) Contains(mode SourceParseMode) bool {
	return (1<<uint32(mode))&s.mask != 0
}

// ===== 解析模式判断函数 =====

// IsFunctionParseMode 判断是否是函数解析模式
func IsFunctionParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		NormalFunctionMode,
		GeneratorBodyMode,
		GeneratorWrapperFunctionMode,
		GeneratorWrapperMethodMode,
		GetterMode,
		SetterMode,
		MethodMode,
		ArrowFunctionMode,
		AsyncFunctionBodyMode,
		AsyncFunctionMode,
		AsyncMethodMode,
		AsyncArrowFunctionMode,
		AsyncArrowFunctionBodyMode,
		AsyncGeneratorBodyMode,
		AsyncGeneratorWrapperFunctionMode,
		AsyncGeneratorWrapperMethodMode,
		ClassFieldInitializerMode,
		ClassStaticBlockMode,
	).Contains(mode)
}

// IsAsyncFunctionParseMode 判断是否是异步函数解析模式
func IsAsyncFunctionParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		AsyncGeneratorWrapperFunctionMode,
		AsyncGeneratorBodyMode,
		AsyncGeneratorWrapperMethodMode,
		AsyncFunctionBodyMode,
		AsyncFunctionMode,
		AsyncMethodMode,
		AsyncArrowFunctionMode,
		AsyncArrowFunctionBodyMode,
	).Contains(mode)
}

// IsAsyncArrowFunctionParseMode 判断是否是异步箭头函数解析模式
func IsAsyncArrowFunctionParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		AsyncArrowFunctionMode,
		AsyncArrowFunctionBodyMode,
	).Contains(mode)
}

// IsAsyncGeneratorParseMode 判断是否是异步生成器解析模式
func IsAsyncGeneratorParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		AsyncGeneratorWrapperFunctionMode,
		AsyncGeneratorWrapperMethodMode,
		AsyncGeneratorBodyMode,
	).Contains(mode)
}

// IsAsyncGeneratorWrapperParseMode 判断是否是异步生成器包装解析模式
func IsAsyncGeneratorWrapperParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		AsyncGeneratorWrapperFunctionMode,
		AsyncGeneratorWrapperMethodMode,
	).Contains(mode)
}

// IsAsyncFunctionOrAsyncGeneratorWrapperParseMode 判断是否是异步函数或异步生成器包装解析模式
func IsAsyncFunctionOrAsyncGeneratorWrapperParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		AsyncArrowFunctionMode,
		AsyncFunctionMode,
		AsyncGeneratorWrapperFunctionMode,
		AsyncGeneratorWrapperMethodMode,
		AsyncMethodMode,
	).Contains(mode)
}

// IsAsyncFunctionWrapperParseMode 判断是否是异步函数包装解析模式
func IsAsyncFunctionWrapperParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		AsyncArrowFunctionMode,
		AsyncFunctionMode,
		AsyncMethodMode,
	).Contains(mode)
}

// IsAsyncFunctionBodyParseMode 判断是否是异步函数体解析模式
func IsAsyncFunctionBodyParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		AsyncFunctionBodyMode,
		AsyncGeneratorBodyMode,
		AsyncArrowFunctionBodyMode,
	).Contains(mode)
}

// IsGeneratorMethodParseMode 判断是否是生成器方法解析模式
func IsGeneratorMethodParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		GeneratorWrapperMethodMode,
	).Contains(mode)
}

// IsAsyncMethodParseMode 判断是否是异步方法解析模式
func IsAsyncMethodParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(AsyncMethodMode).Contains(mode)
}

// IsAsyncGeneratorMethodParseMode 判断是否是异步生成器方法解析模式
func IsAsyncGeneratorMethodParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(AsyncGeneratorWrapperMethodMode).Contains(mode)
}

// IsMethodParseMode 判断是否是方法解析模式
func IsMethodParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		GeneratorWrapperMethodMode,
		GetterMode,
		SetterMode,
		MethodMode,
		AsyncMethodMode,
		AsyncGeneratorWrapperMethodMode,
		ClassStaticBlockMode,
	).Contains(mode)
}

// IsGeneratorOrAsyncFunctionBodyParseMode 判断是否是生成器或异步函数体解析模式
func IsGeneratorOrAsyncFunctionBodyParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		GeneratorBodyMode,
		AsyncFunctionBodyMode,
		AsyncGeneratorBodyMode,
		AsyncArrowFunctionBodyMode,
	).Contains(mode)
}

// IsGeneratorOrAsyncFunctionWrapperParseMode 判断是否是生成器或异步函数包装解析模式
func IsGeneratorOrAsyncFunctionWrapperParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		GeneratorWrapperFunctionMode,
		GeneratorWrapperMethodMode,
		AsyncFunctionMode,
		AsyncArrowFunctionMode,
		AsyncGeneratorWrapperFunctionMode,
		AsyncMethodMode,
		AsyncGeneratorWrapperMethodMode,
	).Contains(mode)
}

// IsGeneratorOrAsyncGeneratorWrapperParseMode 判断是否是生成器或异步生成器包装解析模式
func IsGeneratorOrAsyncGeneratorWrapperParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		GeneratorWrapperFunctionMode,
		GeneratorWrapperMethodMode,
		AsyncGeneratorWrapperFunctionMode,
		AsyncGeneratorWrapperMethodMode,
	).Contains(mode)
}

// IsGeneratorParseMode 判断是否是生成器解析模式
func IsGeneratorParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		GeneratorBodyMode,
		GeneratorWrapperFunctionMode,
		GeneratorWrapperMethodMode,
	).Contains(mode)
}

// IsGeneratorWrapperParseMode 判断是否是生成器包装解析模式
func IsGeneratorWrapperParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		GeneratorWrapperFunctionMode,
		GeneratorWrapperMethodMode,
	).Contains(mode)
}

// IsArrowFunctionParseMode 判断是否是箭头函数解析模式
func IsArrowFunctionParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		ArrowFunctionMode,
		AsyncArrowFunctionMode,
		AsyncArrowFunctionBodyMode,
	).Contains(mode)
}

// IsModuleParseMode 判断是否是模块解析模式
func IsModuleParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		ModuleAnalyzeMode,
		ModuleEvaluateMode,
	).Contains(mode)
}

// IsProgramParseMode 判断是否是程序解析模式
func IsProgramParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(ProgramMode).Contains(mode)
}

// IsProgramOrModuleParseMode 判断是否是程序或模块解析模式
func IsProgramOrModuleParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		ProgramMode,
		ModuleAnalyzeMode,
		ModuleEvaluateMode,
	).Contains(mode)
}

// ConstructAbilityForParseMode 根据解析模式返回构造能力
func ConstructAbilityForParseMode(mode SourceParseMode) runtime.ConstructAbility {
	if mode == NormalFunctionMode {
		return runtime.CanConstruct
	}
	return runtime.CannotConstruct
}

// FunctionNameIsInScope 判断函数名是否在作用域中
func FunctionNameIsInScope(name runtime.Identifier, functionMode FunctionMode) bool {
	if name.IsNull() {
		return false
	}
	if functionMode != FunctionModeFunctionExpression {
		return false
	}
	return true
}

// FunctionNameScopeIsDynamic 判断函数名作用域是否是动态的
func FunctionNameScopeIsDynamic(usesEval bool, isStrictMode bool) bool {
	if !usesEval {
		return false
	}
	if isStrictMode {
		return false
	}
	return true
}
