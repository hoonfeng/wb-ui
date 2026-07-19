/*
 * Copyright (C) 2015-2023 Apple Inc. All rights reserved.
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
 * THIS SOFTWARE IS PROVIDED BY APPLE INC. ``AS IS'' AND ANY
 * EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
 * IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR
 * PURPOSE ARE DISCLAIMED.  IN NO EVENT SHALL APPLE INC. OR
 * CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL,
 * EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO,
 * PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR
 * PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY
 * OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
 * (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
 * OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 */

// VariableEnvironment.h — Variable scoping and environment tracking

package parser

// VariableEnvironmentEntry corresponds to JSC::VariableEnvironmentEntry
// Bitfield tracking properties of a variable declaration.
type VariableEnvironmentEntry struct {
	bits uint16
}

// VariableEntry traits
const (
	varEntryIsCaptured              uint16 = 1 << 0
	varEntryIsConst                 uint16 = 1 << 1
	varEntryIsVar                   uint16 = 1 << 2
	varEntryIsLet                   uint16 = 1 << 3
	varEntryIsExported              uint16 = 1 << 4
	varEntryIsImported              uint16 = 1 << 5
	varEntryIsImportedNamespace     uint16 = 1 << 6
	varEntryIsFunction              uint16 = 1 << 7
	varEntryIsParameter             uint16 = 1 << 8
	varEntryIsSloppyModeHoistedFunc uint16 = 1 << 9
	varEntryIsPrivateField          uint16 = 1 << 10
	varEntryIsPrivateMethod         uint16 = 1 << 11
	varEntryIsPrivateGetter         uint16 = 1 << 12
	varEntryIsPrivateSetter         uint16 = 1 << 13
	varEntryIsFunctionDeclaration   uint16 = 1 << 14
	varEntryIsUsing                 uint16 = 1 << 15
)

func (e *VariableEnvironmentEntry) IsCaptured() bool                    { return e.bits&varEntryIsCaptured != 0 }
func (e *VariableEnvironmentEntry) IsConst() bool                       { return e.bits&varEntryIsConst != 0 }
func (e *VariableEnvironmentEntry) IsVar() bool                         { return e.bits&varEntryIsVar != 0 }
func (e *VariableEnvironmentEntry) IsLet() bool                         { return e.bits&varEntryIsLet != 0 }
func (e *VariableEnvironmentEntry) IsExported() bool                    { return e.bits&varEntryIsExported != 0 }
func (e *VariableEnvironmentEntry) IsImported() bool                    { return e.bits&varEntryIsImported != 0 }
func (e *VariableEnvironmentEntry) IsImportedNamespace() bool           { return e.bits&varEntryIsImportedNamespace != 0 }
func (e *VariableEnvironmentEntry) IsFunction() bool                    { return e.bits&varEntryIsFunction != 0 }
func (e *VariableEnvironmentEntry) IsFunctionDeclaration() bool         { return e.bits&varEntryIsFunctionDeclaration != 0 }
func (e *VariableEnvironmentEntry) IsParameter() bool                   { return e.bits&varEntryIsParameter != 0 }
func (e *VariableEnvironmentEntry) IsSloppyModeHoistedFunction() bool   { return e.bits&varEntryIsSloppyModeHoistedFunc != 0 }
func (e *VariableEnvironmentEntry) IsPrivateField() bool                { return e.bits&varEntryIsPrivateField != 0 }
func (e *VariableEnvironmentEntry) IsPrivateMethod() bool               { return e.bits&varEntryIsPrivateMethod != 0 }
func (e *VariableEnvironmentEntry) IsPrivateSetter() bool               { return e.bits&varEntryIsPrivateSetter != 0 }
func (e *VariableEnvironmentEntry) IsPrivateGetter() bool               { return e.bits&varEntryIsPrivateGetter != 0 }
func (e *VariableEnvironmentEntry) IsUsing() bool                       { return e.bits&varEntryIsUsing != 0 }

func (e *VariableEnvironmentEntry) SetIsCaptured()           { e.bits |= varEntryIsCaptured }
func (e *VariableEnvironmentEntry) SetIsConst()              { e.bits |= varEntryIsConst }
func (e *VariableEnvironmentEntry) SetIsVar()                { e.bits |= varEntryIsVar }
func (e *VariableEnvironmentEntry) SetIsLet()                { e.bits |= varEntryIsLet }
func (e *VariableEnvironmentEntry) SetIsExported()           { e.bits |= varEntryIsExported }
func (e *VariableEnvironmentEntry) SetIsImported()           { e.bits |= varEntryIsImported }
func (e *VariableEnvironmentEntry) SetIsImportedNamespace()  { e.bits |= varEntryIsImportedNamespace }
func (e *VariableEnvironmentEntry) SetIsFunction()           { e.bits |= varEntryIsFunction }
func (e *VariableEnvironmentEntry) SetIsFunctionDeclaration(){ e.bits |= varEntryIsFunctionDeclaration }
func (e *VariableEnvironmentEntry) SetIsParameter()          { e.bits |= varEntryIsParameter }
func (e *VariableEnvironmentEntry) SetIsSloppyModeHoistedFunction() { e.bits |= varEntryIsSloppyModeHoistedFunc }
func (e *VariableEnvironmentEntry) SetIsPrivateField()       { e.bits |= varEntryIsPrivateField }
func (e *VariableEnvironmentEntry) SetIsPrivateMethod()      { e.bits |= varEntryIsPrivateMethod }
func (e *VariableEnvironmentEntry) SetIsPrivateSetter()      { e.bits |= varEntryIsPrivateSetter }
func (e *VariableEnvironmentEntry) SetIsPrivateGetter()      { e.bits |= varEntryIsPrivateGetter }
func (e *VariableEnvironmentEntry) SetIsUsing()              { e.bits |= varEntryIsUsing }
func (e *VariableEnvironmentEntry) ClearIsVar()              { e.bits &^= varEntryIsVar }

func (e *VariableEnvironmentEntry) Bits() uint16 { return e.bits }

// PrivateNameEntry corresponds to JSC::PrivateNameEntry
type PrivateNameEntry struct {
	bits uint16
}

const (
	privateNameEntryNone      uint16 = 0
	privateNameEntryIsMethod  uint16 = 1 << 0
	privateNameEntryIsGetter  uint16 = 1 << 1
	privateNameEntryIsSetter  uint16 = 1 << 2
	privateNameEntryIsStatic  uint16 = 1 << 3
)

func NewPrivateNameEntry(traits uint16) PrivateNameEntry { return PrivateNameEntry{bits: traits} }

func (e PrivateNameEntry) IsMethod() bool                 { return e.bits&privateNameEntryIsMethod != 0 }
func (e PrivateNameEntry) IsSetter() bool                 { return e.bits&privateNameEntryIsSetter != 0 }
func (e PrivateNameEntry) IsGetter() bool                 { return e.bits&privateNameEntryIsGetter != 0 }
func (e PrivateNameEntry) IsField() bool                  { return !e.IsPrivateMethodOrAccessor() }
func (e PrivateNameEntry) IsStatic() bool                 { return e.bits&privateNameEntryIsStatic != 0 }
func (e PrivateNameEntry) IsPrivateMethodOrAccessor() bool { return e.IsMethod() || e.IsSetter() || e.IsGetter() }
func (e PrivateNameEntry) Bits() uint16                    { return e.bits }

// VariableEnvironment corresponds to JSC::VariableEnvironment
type VariableEnvironment struct {
	variables               map[string]VariableEnvironmentEntry
	privateNames            map[string]PrivateNameEntry
	isEverythingCaptured    bool
	hasAwaitUsingDeclaration bool
}

func NewVariableEnvironment() *VariableEnvironment {
	return &VariableEnvironment{
		variables:    make(map[string]VariableEnvironmentEntry),
		privateNames: make(map[string]PrivateNameEntry),
	}
}

func (env *VariableEnvironment) Add(name string) VariableEnvironmentEntry {
	e, ok := env.variables[name]
	if !ok {
		e = VariableEnvironmentEntry{}
	}
	env.variables[name] = e
	return e
}

func (env *VariableEnvironment) Contains(name string) bool {
	_, ok := env.variables[name]
	return ok
}

func (env *VariableEnvironment) Remove(name string) {
	delete(env.variables, name)
}

func (env *VariableEnvironment) Get(name string) (VariableEnvironmentEntry, bool) {
	e, ok := env.variables[name]
	return e, ok
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

func (env *VariableEnvironment) IsEverythingCaptured() bool { return env.isEverythingCaptured }

func (env *VariableEnvironment) MarkVariableAsCaptured(name string) {
	if e, ok := env.variables[name]; ok {
		e.SetIsCaptured()
		env.variables[name] = e
	}
}

func (env *VariableEnvironment) MarkVariableAsCapturedIfDefined(name string) {
	if _, ok := env.variables[name]; ok {
		env.MarkVariableAsCaptured(name)
	}
}

func (env *VariableEnvironment) MarkAllVariablesAsCaptured() {
	env.isEverythingCaptured = true
	for k, e := range env.variables {
		e.SetIsCaptured()
		env.variables[k] = e
	}
}

func (env *VariableEnvironment) HasCapturedVariables() bool {
	for _, e := range env.variables {
		if e.IsCaptured() {
			return true
		}
	}
	return false
}

func (env *VariableEnvironment) Captures(name string) bool {
	if e, ok := env.variables[name]; ok {
		return e.IsCaptured()
	}
	return false
}

func (env *VariableEnvironment) MarkVariableAsImported(name string) {
	if e, ok := env.variables[name]; ok {
		e.SetIsImported()
		env.variables[name] = e
	}
}

func (env *VariableEnvironment) MarkVariableAsExported(name string) {
	if e, ok := env.variables[name]; ok {
		e.SetIsExported()
		env.variables[name] = e
	}
}

func (env *VariableEnvironment) HasUsingDeclaration() bool {
	for _, e := range env.variables {
		if e.IsUsing() {
			return true
		}
	}
	return false
}

func (env *VariableEnvironment) UsingDeclarationCount() int {
	count := 0
	for _, e := range env.variables {
		if e.IsUsing() {
			count++
		}
	}
	return count
}

func (env *VariableEnvironment) HasAwaitUsingDeclaration() bool     { return env.hasAwaitUsingDeclaration }
func (env *VariableEnvironment) SetHasAwaitUsingDeclaration()       { env.hasAwaitUsingDeclaration = true }

func (env *VariableEnvironment) AddPrivateName(name string) PrivateNameEntry {
	e, ok := env.privateNames[name]
	if !ok {
		e = NewPrivateNameEntry(0)
	}
	env.privateNames[name] = e
	return e
}

func (env *VariableEnvironment) HasPrivateName(name string) bool {
	_, ok := env.privateNames[name]
	return ok
}

func (env *VariableEnvironment) HasStaticPrivateMethodOrAccessor() bool {
	for _, e := range env.privateNames {
		if e.IsPrivateMethodOrAccessor() && e.IsStatic() {
			return true
		}
	}
	return false
}

func (env *VariableEnvironment) HasInstancePrivateMethodOrAccessor() bool {
	for _, e := range env.privateNames {
		if e.IsPrivateMethodOrAccessor() && !e.IsStatic() {
			return true
		}
	}
	return false
}

func (env *VariableEnvironment) PrivateNameEnvironment() map[string]PrivateNameEntry {
	return env.privateNames
}

// TDZEnvironment corresponds to JSC::TDZEnvironment
type TDZEnvironment map[string]struct{}

func NewTDZEnvironment() TDZEnvironment {
	return make(TDZEnvironment)
}

func (e TDZEnvironment) Add(name string) { e[name] = struct{}{} }
func (e TDZEnvironment) Contains(name string) bool {
	_, ok := e[name]
	return ok
}
func (e TDZEnvironment) Remove(name string) { delete(e, name) }
