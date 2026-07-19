// Copyright (C) 1999-2000 Harri Porten (porten@kde.org)
// Copyright (C) 2007-2022 Apple Inc. All rights reserved.
// Translated to Go by the jsc-translator.
//
// StringObject corresponds to JSC::StringObject — wrapper object for string primitives.

package runtime

// StringObject corresponds to JSC::StringObject.
// Created by new String(value) — wraps a string primitive.
type StringObject struct {
	JSObject
	m_internalValue string
}

const StringObjectStructureFlags uint32 = JSObjectStructureFlags | OverridesGetOwnPropertySlot | OverridesGetOwnPropertyNames | OverridesPut | InterceptsGetOwnPropertySlotByIndexEvenWhenLengthIsNotZero

func NewStringObject(vm *VM, structure *Structure) *StringObject {
	obj := &StringObject{}
	obj.structureID = structure.structureID
	obj.typ = StringObjectType
	obj.cellState = DefinitelyWhite
	obj.properties = make(map[string]JSValue)
	return obj
}

func NewStringObjectWithString(vm *VM, structure *Structure, str string) *StringObject {
	obj := NewStringObject(vm, structure)
	obj.m_internalValue = str
	return obj
}

func (so *StringObject) InternalValue() string { return so.m_internalValue }
func (so *StringObject) SetInternalValue(vm *VM, val string) {
	so.m_internalValue = val
}

// ConstructString is a helper to create a StringObject wrapping a value (from StringConstructor).
func ConstructString(vm *VM, globalObject *JSGlobalObject, val JSValue) *StringObject {
	var str string
	if val.IsString() {
		str = val.ToString()
	} else {
		str = val.ToString()
	}
	return NewStringObjectWithString(vm, globalObject.stringObjectStructure(), str)
}
