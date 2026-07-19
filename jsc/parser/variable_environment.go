// 版权所有 (C) 2015-2023 Apple Inc. 保留所有权利。
//
// 使用约定：BSD 许可证
//
// 从 WebKit Source/JavaScriptCore/parser/VariableEnvironment.h 翻译为 Go

package parser

import "fmt"

// VariableEnvironmentEntry 变量环境条目（位掩码）
type VariableEnvironmentEntry struct {
	bits uint16
}

const (
	veIsCaptured                  uint16 = 1 << 0
	veIsConst                     uint16 = 1 << 1
	veIsVar                       uint16 = 1 << 2
	veIsLet                       uint16 = 1 << 3
	veIsExported                  uint16 = 1 << 4
	veIsImported                  uint16 = 1 << 5
	veIsImportedNamespace         uint16 = 1 << 6
	veIsFunction                  uint16 = 1 << 7
	veIsParameter                 uint16 = 1 << 8
	veIsSloppyModeHoistedFunction uint16 = 1 << 9
	veIsPrivateField              uint16 = 1 << 10
	veIsPrivateMethod             uint16 = 1 << 11
	veIsPrivateGetter             uint16 = 1 << 12
	veIsPrivateSetter             uint16 = 1 << 13
	veIsFunctionDeclaration       uint16 = 1 << 14
	veIsUsing                     uint16 = 1 << 15
)

func (e VariableEnvironmentEntry) IsCaptured() bool                  { return e.bits&veIsCaptured != 0 }
func (e VariableEnvironmentEntry) IsConst() bool                     { return e.bits&veIsConst != 0 }
func (e VariableEnvironmentEntry) IsVar() bool                       { return e.bits&veIsVar != 0 }
func (e VariableEnvironmentEntry) IsLet() bool                       { return e.bits&veIsLet != 0 }
func (e VariableEnvironmentEntry) IsExported() bool                  { return e.bits&veIsExported != 0 }
func (e VariableEnvironmentEntry) IsImported() bool                  { return e.bits&veIsImported != 0 }
func (e VariableEnvironmentEntry) IsImportedNamespace() bool         { return e.bits&veIsImportedNamespace != 0 }
func (e VariableEnvironmentEntry) IsFunction() bool                  { return e.bits&veIsFunction != 0 }
func (e VariableEnvironmentEntry) IsFunctionDeclaration() bool       { return e.bits&veIsFunctionDeclaration != 0 }
func (e VariableEnvironmentEntry) IsParameter() bool                 { return e.bits&veIsParameter != 0 }
func (e VariableEnvironmentEntry) IsSloppyModeHoistedFunction() bool { return e.bits&veIsSloppyModeHoistedFunction != 0 }
func (e VariableEnvironmentEntry) IsPrivateField() bool              { return e.bits&veIsPrivateField != 0 }
func (e VariableEnvironmentEntry) IsPrivateMethod() bool             { return e.bits&veIsPrivateMethod != 0 }
func (e VariableEnvironmentEntry) IsPrivateSetter() bool             { return e.bits&veIsPrivateSetter != 0 }
func (e VariableEnvironmentEntry) IsPrivateGetter() bool             { return e.bits&veIsPrivateGetter != 0 }
func (e VariableEnvironmentEntry) IsUsing() bool                     { return e.bits&veIsUsing != 0 }
func (e VariableEnvironmentEntry) Bits() uint16                      { return e.bits }

func (e *VariableEnvironmentEntry) SetIsCaptured()                  { e.bits |= veIsCaptured }
func (e *VariableEnvironmentEntry) SetIsConst()                     { e.bits |= veIsConst }
func (e *VariableEnvironmentEntry) SetIsVar()                       { e.bits |= veIsVar }
func (e *VariableEnvironmentEntry) SetIsLet()                       { e.bits |= veIsLet }
func (e *VariableEnvironmentEntry) SetIsExported()                  { e.bits |= veIsExported }
func (e *VariableEnvironmentEntry) SetIsImported()                  { e.bits |= veIsImported }
func (e *VariableEnvironmentEntry) SetIsImportedNamespace()         { e.bits |= veIsImportedNamespace }
func (e *VariableEnvironmentEntry) SetIsFunction()                  { e.bits |= veIsFunction }
func (e *VariableEnvironmentEntry) SetIsFunctionDeclaration()       { e.bits |= veIsFunctionDeclaration }
func (e *VariableEnvironmentEntry) SetIsParameter()                 { e.bits |= veIsParameter }
func (e *VariableEnvironmentEntry) SetIsSloppyModeHoistedFunction() { e.bits |= veIsSloppyModeHoistedFunction }
func (e *VariableEnvironmentEntry) SetIsPrivateField()              { e.bits |= veIsPrivateField }
func (e *VariableEnvironmentEntry) SetIsPrivateMethod()             { e.bits |= veIsPrivateMethod }
func (e *VariableEnvironmentEntry) SetIsPrivateSetter()             { e.bits |= veIsPrivateSetter }
func (e *VariableEnvironmentEntry) SetIsPrivateGetter()             { e.bits |= veIsPrivateGetter }
func (e *VariableEnvironmentEntry) SetIsUsing()                     { e.bits |= veIsUsing }
func (e *VariableEnvironmentEntry) ClearIsVar()                     { e.bits &^= veIsVar }

// PrivateNameEntry 私有名称条目
type PrivateNameEntry struct {
	bits uint16
}

const (
	pneIsMethod uint16 = 1 << 0
	pneIsGetter uint16 = 1 << 1
	pneIsSetter uint16 = 1 << 2
	pneIsStatic uint16 = 1 << 3
)

func NewPrivateNameEntry(traits uint16) PrivateNameEntry {
	return PrivateNameEntry{bits: traits}
}
func (e PrivateNameEntry) IsMethod() bool                  { return e.bits&pneIsMethod != 0 }
func (e PrivateNameEntry) IsSetter() bool                  { return e.bits&pneIsSetter != 0 }
func (e PrivateNameEntry) IsGetter() bool                  { return e.bits&pneIsGetter != 0 }
func (e PrivateNameEntry) IsField() bool                   { return !e.IsPrivateMethodOrAccessor() }
func (e PrivateNameEntry) IsStatic() bool                  { return e.bits&pneIsStatic != 0 }
func (e PrivateNameEntry) IsPrivateMethodOrAccessor() bool { return e.IsMethod() || e.IsSetter() || e.IsGetter() }
func (e PrivateNameEntry) Bits() uint16                    { return e.bits }

// VariableEnvironment 变量环境
type VariableEnvironment struct {
	variables            map[string]*VariableEnvironmentEntry
	privateNames         map[string]*PrivateNameEntry
	isEverythingCaptured bool
	hasAwaitUsingDecl    bool
}

func NewVariableEnvironment() *VariableEnvironment {
	return &VariableEnvironment{
		variables:    make(map[string]*VariableEnvironmentEntry),
		privateNames: make(map[string]*PrivateNameEntry),
	}
}

func (env *VariableEnvironment) Add(ident string) *VariableEnvironmentEntry {
	entry := &VariableEnvironmentEntry{}
	env.variables[ident] = entry
	return entry
}

func (env *VariableEnvironment) Contains(ident string) bool {
	_, ok := env.variables[ident]
	return ok
}

func (env *VariableEnvironment) Remove(ident string) bool {
	_, ok := env.variables[ident]
	delete(env.variables, ident)
	return ok
}

func (env *VariableEnvironment) Find(ident string) *VariableEnvironmentEntry {
	return env.variables[ident]
}

func (env *VariableEnvironment) Size() int {
	return len(env.variables) + len(env.privateNames)
}

func (env *VariableEnvironment) MapSize() int {
	return len(env.variables)
}

func (env *VariableEnvironment) IsEmpty() bool {
	return len(env.variables) == 0 && len(env.privateNames) == 0
}

func (env *VariableEnvironment) MarkVariableAsCapturedIfDefined(ident string) {
	if entry, ok := env.variables[ident]; ok {
		entry.SetIsCaptured()
	}
}

func (env *VariableEnvironment) MarkVariableAsCaptured(ident string) {
	if entry, ok := env.variables[ident]; ok {
		entry.SetIsCaptured()
	}
}

func (env *VariableEnvironment) Captures(ident string) bool {
	if entry, ok := env.variables[ident]; ok {
		return entry.IsCaptured()
	}
	return false
}

func (env *VariableEnvironment) CapturesUid(uid uintptr) bool {
	return env.Captures(fmt.Sprintf("%d", uid))
}

func (env *VariableEnvironment) MarkAllVariablesAsCaptured() {
	env.isEverythingCaptured = true
	for _, entry := range env.variables {
		entry.SetIsCaptured()
	}
}

func (env *VariableEnvironment) HasCapturedVariables() bool {
	for _, entry := range env.variables {
		if entry.IsCaptured() {
			return true
		}
	}
	return false
}

func (env *VariableEnvironment) MarkVariableAsImported(ident string) {
	if entry, ok := env.variables[ident]; ok {
		entry.SetIsImported()
	}
}

func (env *VariableEnvironment) MarkVariableAsExported(ident string) {
	if entry, ok := env.variables[ident]; ok {
		entry.SetIsExported()
	}
}

func (env *VariableEnvironment) IsEverythingCaptured() bool { return env.isEverythingCaptured }

func (env *VariableEnvironment) HasUsingDeclaration() bool {
	for _, entry := range env.variables {
		if entry.IsUsing() {
			return true
		}
	}
	return false
}

func (env *VariableEnvironment) UsingDeclarationCount() uint32 {
	count := uint32(0)
	for _, entry := range env.variables {
		if entry.IsUsing() {
			count++
		}
	}
	return count
}

func (env *VariableEnvironment) HasAwaitUsingDeclaration() bool     { return env.hasAwaitUsingDecl }
func (env *VariableEnvironment) SetHasAwaitUsingDeclaration()       { env.hasAwaitUsingDecl = true }

func (env *VariableEnvironment) AddPrivateName(ident string) {
	env.privateNames[ident] = &PrivateNameEntry{}
}

func (env *VariableEnvironment) PrivateNamesSize() int {
	return len(env.privateNames)
}

func (env *VariableEnvironment) HasPrivateName(ident string) bool {
	_, ok := env.privateNames[ident]
	return ok
}

func (env *VariableEnvironment) Swap(other *VariableEnvironment) {
	env.variables, other.variables = other.variables, env.variables
	env.privateNames, other.privateNames = other.privateNames, env.privateNames
	env.isEverythingCaptured, other.isEverythingCaptured = other.isEverythingCaptured, env.isEverythingCaptured
	env.hasAwaitUsingDecl, other.hasAwaitUsingDecl = other.hasAwaitUsingDecl, env.hasAwaitUsingDecl
}

// TDZEnvironmentSet 简化的 TDZ 环境
type TDZEnvironmentSet = map[string]struct{}

func NewTDZEnvironmentSet() TDZEnvironmentSet {
	return make(map[string]struct{})
}

func TDZSetContains(set TDZEnvironmentSet, key string) bool {
	_, ok := set[key]
	return ok
}

func TDZSetAdd(set TDZEnvironmentSet, key string) {
	set[key] = struct{}{}
}

func TDZSetRemove(set TDZEnvironmentSet, key string) {
	delete(set, key)
}
