/*
 * Copyright (C) 2012-2013, 2015-2016 Apple Inc. All rights reserved.
 *
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions
 * are met:
 * 1. Redistributions of source code must retain the above copyright
 *    notice, this list of conditions and the following disclaimer.
 * 2. Redistributions in binary form must reproduce the above copyright
 *    notice, this list of conditions and the following disclaimer in the
 *    documentation and/or other materials provided with the distribution.
 *
 * THIS SOFTWARE IS PROVIDED BY APPLE INC. AND ITS CONTRIBUTORS ``AS IS''
 * AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO,
 * THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR
 * PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL APPLE INC. OR ITS CONTRIBUTORS
 * BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
 * CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
 * SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
 * INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
 * CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
 * ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF
 * THE POSSIBILITY OF SUCH DAMAGE.
 */

// ParserModes.h — Parser mode enumerations and helper functions

package parser

// JSParserBuiltinMode corresponds to JSC::JSParserBuiltinMode
type JSParserBuiltinMode uint8

const (
	JSParserBuiltinModeNotBuiltin JSParserBuiltinMode = 0
	JSParserBuiltinModeBuiltin    JSParserBuiltinMode = 1
)

// JSParserScriptMode corresponds to JSC::JSParserScriptMode
type JSParserScriptMode uint8

const (
	JSParserScriptModeClassic JSParserScriptMode = 0
	JSParserScriptModeModule  JSParserScriptMode = 1
)

// SuperBinding corresponds to JSC::SuperBinding
type SuperBinding uint8

const (
	SuperBindingNeeded    SuperBinding = 0
	SuperBindingNotNeeded SuperBinding = 1
)

// PrivateBrandRequirement corresponds to JSC::PrivateBrandRequirement
type PrivateBrandRequirement uint8

const (
	PrivateBrandRequirementNone   PrivateBrandRequirement = 0
	PrivateBrandRequirementNeeded PrivateBrandRequirement = 1
)

// CodeGenerationMode corresponds to JSC::CodeGenerationMode
type CodeGenerationMode uint8

const (
	CodeGenerationModeDebugger             CodeGenerationMode = 1 << 0
	CodeGenerationModeTypeProfiler         CodeGenerationMode = 1 << 1
	CodeGenerationModeControlFlowProfiler  CodeGenerationMode = 1 << 2
)

// FunctionMode corresponds to JSC::FunctionMode
type FunctionMode uint8

const (
	FunctionModeNone                FunctionMode = 0
	FunctionModeFunctionExpression  FunctionMode = 1
	FunctionModeFunctionDeclaration FunctionMode = 2
	FunctionModeMethodDefinition    FunctionMode = 3
)

// FunctionConstructionMode corresponds to JSC::FunctionConstructionMode (from ParserModes.h)
type FunctionConstructionMode uint8

const (
	FunctionConstructionModeFunction        FunctionConstructionMode = 0
	FunctionConstructionModeGenerator       FunctionConstructionMode = 1
	FunctionConstructionModeAsync           FunctionConstructionMode = 2
	FunctionConstructionModeAsyncGenerator  FunctionConstructionMode = 3
)

// SourceParseMode corresponds to JSC::SourceParseMode
type SourceParseMode uint8

const (
	SourceParseModeNormalFunctionMode                SourceParseMode = 0
	SourceParseModeGeneratorBodyMode                 SourceParseMode = 1
	SourceParseModeGeneratorWrapperFunctionMode      SourceParseMode = 2
	SourceParseModeGetterMode                        SourceParseMode = 3
	SourceParseModeSetterMode                        SourceParseMode = 4
	SourceParseModeMethodMode                        SourceParseMode = 5
	SourceParseModeArrowFunctionMode                 SourceParseMode = 6
	SourceParseModeAsyncFunctionBodyMode             SourceParseMode = 7
	SourceParseModeAsyncArrowFunctionBodyMode        SourceParseMode = 8
	SourceParseModeAsyncFunctionMode                 SourceParseMode = 9
	SourceParseModeAsyncMethodMode                   SourceParseMode = 10
	SourceParseModeAsyncArrowFunctionMode            SourceParseMode = 11
	SourceParseModeProgramMode                       SourceParseMode = 12
	SourceParseModeModuleAnalyzeMode                 SourceParseMode = 13
	SourceParseModeModuleEvaluateMode                SourceParseMode = 14
	SourceParseModeAsyncGeneratorBodyMode            SourceParseMode = 15
	SourceParseModeAsyncGeneratorWrapperFunctionMode SourceParseMode = 16
	SourceParseModeAsyncGeneratorWrapperMethodMode   SourceParseMode = 17
	SourceParseModeGeneratorWrapperMethodMode        SourceParseMode = 18
	SourceParseModeClassFieldInitializerMode         SourceParseMode = 19
	SourceParseModeClassStaticBlockMode              SourceParseMode = 20
)

// SourceParseModeSet — bitmask set of SourceParseMode values
type SourceParseModeSet struct {
	mask uint32
}

func NewSourceParseModeSet(modes ...SourceParseMode) SourceParseModeSet {
	var s SourceParseModeSet
	for _, m := range modes {
		s.mask |= 1 << uint32(m)
	}
	return s
}

func (s SourceParseModeSet) Contains(mode SourceParseMode) bool {
	return (1<<uint32(mode))&s.mask != 0
}

// IsFunctionParseMode — checks if parse mode is a function mode
func IsFunctionParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeNormalFunctionMode,
		SourceParseModeGeneratorBodyMode,
		SourceParseModeGeneratorWrapperFunctionMode,
		SourceParseModeGeneratorWrapperMethodMode,
		SourceParseModeGetterMode,
		SourceParseModeSetterMode,
		SourceParseModeMethodMode,
		SourceParseModeArrowFunctionMode,
		SourceParseModeAsyncFunctionBodyMode,
		SourceParseModeAsyncFunctionMode,
		SourceParseModeAsyncMethodMode,
		SourceParseModeAsyncArrowFunctionMode,
		SourceParseModeAsyncArrowFunctionBodyMode,
		SourceParseModeAsyncGeneratorBodyMode,
		SourceParseModeAsyncGeneratorWrapperFunctionMode,
		SourceParseModeAsyncGeneratorWrapperMethodMode,
		SourceParseModeClassFieldInitializerMode,
		SourceParseModeClassStaticBlockMode,
	).Contains(mode)
}

func IsAsyncFunctionParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeAsyncGeneratorWrapperFunctionMode,
		SourceParseModeAsyncGeneratorBodyMode,
		SourceParseModeAsyncGeneratorWrapperMethodMode,
		SourceParseModeAsyncFunctionBodyMode,
		SourceParseModeAsyncFunctionMode,
		SourceParseModeAsyncMethodMode,
		SourceParseModeAsyncArrowFunctionMode,
		SourceParseModeAsyncArrowFunctionBodyMode,
	).Contains(mode)
}

func IsAsyncArrowFunctionParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeAsyncArrowFunctionMode,
		SourceParseModeAsyncArrowFunctionBodyMode,
	).Contains(mode)
}

func IsAsyncGeneratorParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeAsyncGeneratorWrapperFunctionMode,
		SourceParseModeAsyncGeneratorWrapperMethodMode,
		SourceParseModeAsyncGeneratorBodyMode,
	).Contains(mode)
}

func IsAsyncGeneratorWrapperParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeAsyncGeneratorWrapperFunctionMode,
		SourceParseModeAsyncGeneratorWrapperMethodMode,
	).Contains(mode)
}

func IsAsyncFunctionOrAsyncGeneratorWrapperParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeAsyncArrowFunctionMode,
		SourceParseModeAsyncFunctionMode,
		SourceParseModeAsyncGeneratorWrapperFunctionMode,
		SourceParseModeAsyncGeneratorWrapperMethodMode,
		SourceParseModeAsyncMethodMode,
	).Contains(mode)
}

func IsAsyncFunctionWrapperParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeAsyncArrowFunctionMode,
		SourceParseModeAsyncFunctionMode,
		SourceParseModeAsyncMethodMode,
	).Contains(mode)
}

func IsAsyncFunctionBodyParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeAsyncFunctionBodyMode,
		SourceParseModeAsyncGeneratorBodyMode,
		SourceParseModeAsyncArrowFunctionBodyMode,
	).Contains(mode)
}

func IsGeneratorMethodParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeGeneratorWrapperMethodMode,
	).Contains(mode)
}

func IsAsyncMethodParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeAsyncMethodMode,
	).Contains(mode)
}

func IsAsyncGeneratorMethodParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeAsyncGeneratorWrapperMethodMode,
	).Contains(mode)
}

func IsMethodParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeGeneratorWrapperMethodMode,
		SourceParseModeGetterMode,
		SourceParseModeSetterMode,
		SourceParseModeMethodMode,
		SourceParseModeAsyncMethodMode,
		SourceParseModeAsyncGeneratorWrapperMethodMode,
		SourceParseModeClassStaticBlockMode,
	).Contains(mode)
}

func IsGeneratorOrAsyncFunctionBodyParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeGeneratorBodyMode,
		SourceParseModeAsyncFunctionBodyMode,
		SourceParseModeAsyncGeneratorBodyMode,
		SourceParseModeAsyncArrowFunctionBodyMode,
	).Contains(mode)
}

func IsGeneratorOrAsyncFunctionWrapperParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeGeneratorWrapperFunctionMode,
		SourceParseModeGeneratorWrapperMethodMode,
		SourceParseModeAsyncFunctionMode,
		SourceParseModeAsyncArrowFunctionMode,
		SourceParseModeAsyncGeneratorWrapperFunctionMode,
		SourceParseModeAsyncMethodMode,
		SourceParseModeAsyncGeneratorWrapperMethodMode,
	).Contains(mode)
}

func IsGeneratorOrAsyncGeneratorWrapperParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeGeneratorWrapperFunctionMode,
		SourceParseModeGeneratorWrapperMethodMode,
		SourceParseModeAsyncGeneratorWrapperFunctionMode,
		SourceParseModeAsyncGeneratorWrapperMethodMode,
	).Contains(mode)
}

func IsGeneratorParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeGeneratorBodyMode,
		SourceParseModeGeneratorWrapperFunctionMode,
		SourceParseModeGeneratorWrapperMethodMode,
	).Contains(mode)
}

func IsGeneratorWrapperParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeGeneratorWrapperFunctionMode,
		SourceParseModeGeneratorWrapperMethodMode,
	).Contains(mode)
}

func IsArrowFunctionParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeArrowFunctionMode,
		SourceParseModeAsyncArrowFunctionMode,
		SourceParseModeAsyncArrowFunctionBodyMode,
	).Contains(mode)
}

func IsModuleParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeModuleAnalyzeMode,
		SourceParseModeModuleEvaluateMode,
	).Contains(mode)
}

func IsProgramParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeProgramMode,
	).Contains(mode)
}

func IsProgramOrModuleParseMode(mode SourceParseMode) bool {
	return NewSourceParseModeSet(
		SourceParseModeProgramMode,
		SourceParseModeModuleAnalyzeMode,
		SourceParseModeModuleEvaluateMode,
	).Contains(mode)
}

// ConstructAbilityForParseMode returns ConstructAbility based on parse mode
func ConstructAbilityForParseMode(mode SourceParseMode) int {
	if mode == SourceParseModeNormalFunctionMode {
		return 1 // CanConstruct
	}
	return 0 // CannotConstruct
}

// FunctionNameIsInScope — checks if function name is in scope
func FunctionNameIsInScope(name *string, functionMode FunctionMode) bool {
	if name == nil || *name == "" {
		return false
	}
	if functionMode != FunctionModeFunctionExpression {
		return false
	}
	return true
}

// FunctionNameScopeIsDynamic — checks if function name scope is dynamic
func FunctionNameScopeIsDynamic(usesEval bool, isStrictMode bool) bool {
	if !usesEval {
		return false
	}
	if isStrictMode {
		return false
	}
	return true
}

// LexicallyScopedFeatures — bitmask type
type LexicallyScopedFeatures uint8

const (
	NoLexicallyScopedFeatures                    LexicallyScopedFeatures = 0
	StrictModeLexicallyScopedFeature             LexicallyScopedFeatures = 1 << 0
	TaintedByWithScopeLexicallyScopedFeature      LexicallyScopedFeatures = 1 << 1
	AllLexicallyScopedFeatures                     LexicallyScopedFeatures = StrictModeLexicallyScopedFeature | TaintedByWithScopeLexicallyScopedFeature
)

// CodeFeatures — bitmask type for code analysis features
type CodeFeatures uint16

const (
	NoFeatures                           CodeFeatures = 0
	EvalFeature                          CodeFeatures = 1 << 0
	ArgumentsFeature                     CodeFeatures = 1 << 1
	WithFeature                          CodeFeatures = 1 << 2
	ThisFeature                          CodeFeatures = 1 << 3
	NonSimpleParameterListFeature         CodeFeatures = 1 << 4
	ShadowsArgumentsFeature               CodeFeatures = 1 << 5
	ArrowFunctionFeature                 CodeFeatures = 1 << 6
	AwaitFeature                         CodeFeatures = 1 << 7
	SuperCallFeature                     CodeFeatures = 1 << 8
	SuperPropertyFeature                 CodeFeatures = 1 << 9
	NewTargetFeature                     CodeFeatures = 1 << 10
	NoEvalCacheFeature                   CodeFeatures = 1 << 11
	ImportMetaFeature                    CodeFeatures = 1 << 12
	AsyncFunctionWithoutAwaitFeature     CodeFeatures = 1 << 13
	AllFeatures                          CodeFeatures = EvalFeature | ArgumentsFeature | WithFeature | ThisFeature | NonSimpleParameterListFeature | ShadowsArgumentsFeature | ArrowFunctionFeature | AwaitFeature | SuperCallFeature | SuperPropertyFeature | NewTargetFeature | NoEvalCacheFeature | ImportMetaFeature | AsyncFunctionWithoutAwaitFeature
)

// InnerArrowFunctionCodeFeatures — bitmask type
type InnerArrowFunctionCodeFeatures uint8

const (
	NoInnerArrowFunctionFeatures                    InnerArrowFunctionCodeFeatures = 0
	EvalInnerArrowFunctionFeature                   InnerArrowFunctionCodeFeatures = 1 << 0
	ArgumentsInnerArrowFunctionFeature              InnerArrowFunctionCodeFeatures = 1 << 1
	ThisInnerArrowFunctionFeature                   InnerArrowFunctionCodeFeatures = 1 << 2
	SuperCallInnerArrowFunctionFeature              InnerArrowFunctionCodeFeatures = 1 << 3
	SuperPropertyInnerArrowFunctionFeature          InnerArrowFunctionCodeFeatures = 1 << 4
	NewTargetInnerArrowFunctionFeature              InnerArrowFunctionCodeFeatures = 1 << 5
	AllInnerArrowFunctionCodeFeatures               InnerArrowFunctionCodeFeatures = EvalInnerArrowFunctionFeature | ArgumentsInnerArrowFunctionFeature | ThisInnerArrowFunctionFeature | SuperCallInnerArrowFunctionFeature | SuperPropertyInnerArrowFunctionFeature | NewTargetInnerArrowFunctionFeature
)
