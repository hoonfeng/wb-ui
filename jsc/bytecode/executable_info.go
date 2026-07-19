// Copyright (C) 2012-2015 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/ExecutableInfo.h

package bytecode

// DerivedContextType indicates the type of derived class context.
type DerivedContextType uint8

const (
	DerivedContextTypeNone                   DerivedContextType = 0
	DerivedContextTypeDerivedConstructorContext DerivedContextType = 1
	DerivedContextTypeDerivedMethodContext    DerivedContextType = 2
)

// EvalContextType indicates the type of eval context.
type EvalContextType uint8

const (
	EvalContextTypeNone                   EvalContextType = 0
	EvalContextTypeFunctionEvalContext    EvalContextType = 1
	EvalContextTypeInstanceFieldEvalContext EvalContextType = 2
)

// NeedsClassFieldInitializer indicates whether class field initializers are needed.
type NeedsClassFieldInitializer bool

const (
	NeedsClassFieldInitializerNo  NeedsClassFieldInitializer = false
	NeedsClassFieldInitializerYes NeedsClassFieldInitializer = true
)

// ExecutableInfo holds metadata about a function/script's executable.
// These flags, ParserModes and propagation to XXXCodeBlocks should be reorganized.
type ExecutableInfo struct {
	isConstructor                   bool
	privateBrandRequirement         bool
	isBuiltinFunction               bool
	constructorKind                 uint8 // ConstructorKind enum
	superBinding                    uint8 // SuperBinding enum
	scriptMode                      uint8 // JSParserScriptMode enum
	parseMode                       uint8 // SourceParseMode enum
	derivedContextType              uint8 // DerivedContextType enum
	needsClassFieldInitializer      bool
	isArrowFunctionContext          bool
	isClassContext                  bool
	evalContextType                 uint8 // EvalContextType enum
	isBuiltinDefaultClassConstructor bool
}

func NewExecutableInfo(
	isConstructor bool,
	privateBrandRequirement bool,
	isBuiltinFunction bool,
	constructorKind uint8,
	scriptMode uint8,
	superBinding uint8,
	parseMode uint8,
	derivedContextType DerivedContextType,
	needsClassFieldInitializer NeedsClassFieldInitializer,
	isArrowFunctionContext bool,
	isClassContext bool,
	evalContextType EvalContextType,
	isBuiltinDefaultClassConstructor ...bool,
) ExecutableInfo {
	info := ExecutableInfo{
		isConstructor:                   isConstructor,
		privateBrandRequirement:         privateBrandRequirement,
		isBuiltinFunction:               isBuiltinFunction,
		constructorKind:                 constructorKind,
		superBinding:                    superBinding,
		scriptMode:                      scriptMode,
		parseMode:                       parseMode,
		derivedContextType:              uint8(derivedContextType),
		needsClassFieldInitializer:      bool(needsClassFieldInitializer),
		isArrowFunctionContext:          isArrowFunctionContext,
		isClassContext:                  isClassContext,
		evalContextType:                 uint8(evalContextType),
	}
	if len(isBuiltinDefaultClassConstructor) > 0 {
		info.isBuiltinDefaultClassConstructor = isBuiltinDefaultClassConstructor[0]
	}
	return info
}

func (e *ExecutableInfo) IsConstructor() bool                          { return e.isConstructor }
func (e *ExecutableInfo) PrivateBrandRequirement() bool                { return e.privateBrandRequirement }
func (e *ExecutableInfo) IsBuiltinFunction() bool                      { return e.isBuiltinFunction }
func (e *ExecutableInfo) ConstructorKind() uint8                       { return e.constructorKind }
func (e *ExecutableInfo) SuperBinding() uint8                          { return e.superBinding }
func (e *ExecutableInfo) ScriptMode() uint8                            { return e.scriptMode }
func (e *ExecutableInfo) ParseMode() uint8                             { return e.parseMode }
func (e *ExecutableInfo) DerivedContextType() DerivedContextType       { return DerivedContextType(e.derivedContextType) }
func (e *ExecutableInfo) EvalContextType() EvalContextType             { return EvalContextType(e.evalContextType) }
func (e *ExecutableInfo) IsArrowFunctionContext() bool                 { return e.isArrowFunctionContext }
func (e *ExecutableInfo) IsClassContext() bool                         { return e.isClassContext }
func (e *ExecutableInfo) NeedsClassFieldInitializer() NeedsClassFieldInitializer {
	return NeedsClassFieldInitializer(e.needsClassFieldInitializer)
}
func (e *ExecutableInfo) IsBuiltinDefaultClassConstructor() bool       { return e.isBuiltinDefaultClassConstructor }
