// Copyright (C) 1999-2001 Harri Porten (porten@kde.org)
// Copyright (C) 2001 Peter Kelly (pmk@post.com)
// Copyright (C) 2003-2022 Apple Inc. All rights reserved.
// Translated to Go by the jsc-translator.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions
// are met:
// 1. Redistributions of source code must retain the above copyright
//    notice, this list of conditions and the following disclaimer.
// 2. Redistributions in binary form must reproduce the above copyright
//    notice, this list of conditions and the following disclaimer in the
//    documentation and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY APPLE INC. ``AS IS'' AND ANY
// EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
// IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR
// PURPOSE ARE DISCLAIMED.  IN NO EVENT SHALL APPLE INC. OR
// CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL,
// EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO,
// PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR
// PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY
// OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package runtime

// GetterSetter corresponds to JSC::GetterSetter
// This is an internal value object which stores getter and setter functions
// for a property. Once a getter or setter is set to a non-null value, they cannot be changed.
type GetterSetter struct {
	JSCell
	m_getter *JSObject
	m_setter *JSObject
}

const GetterSetterStructureFlags uint32 = JSCellStructureFlags | OverridesGetOwnPropertySlot | OverridesPut | StructureIsImmortal

func NewGetterSetter(vm *VM, globalObject *JSGlobalObject, getter *JSObject, setter *JSObject) *GetterSetter {
	gs := &GetterSetter{
		JSCell: *NewJSCell(vm, vm.getterSetterStructure),
	}
	if getter != nil {
		gs.m_getter = getter
	} else {
		gs.m_getter = globalObject.nullGetterFunction()
	}
	if setter != nil {
		gs.m_setter = setter
	} else {
		gs.m_setter = globalObject.nullSetterFunction()
	}
	gs.finishCreation(vm)
	return gs
}

func NewGetterSetterFromValues(vm *VM, globalObject *JSGlobalObject, getter JSValue, setter JSValue) *GetterSetter {
	var getterObj *JSObject
	var setterObj *JSObject
	if getter.IsObject() {
		getterObj = asObject(getter)
	}
	if setter.IsObject() {
		setterObj = asObject(setter)
	}
	return NewGetterSetter(vm, globalObject, getterObj, setterObj)
}

func (gs *GetterSetter) Getter() *JSObject { return gs.m_getter }
func (gs *GetterSetter) Setter() *JSObject { return gs.m_setter }

func (gs *GetterSetter) IsGetterNull() bool {
	_, ok := interface{}(gs.m_getter).(*NullGetterFunction)
	return ok
}

func (gs *GetterSetter) IsSetterNull() bool {
	_, ok := interface{}(gs.m_setter).(*NullSetterFunction)
	return ok
}

func (gs *GetterSetter) CallGetter(globalObject *JSGlobalObject, thisValue JSValue) JSValue {
	getter := gs.m_getter
	callData := getCallDataInline(NewJSValueObject(getter))
	return call(globalObject, NewJSValueObject(getter), callData, thisValue, nil)
}

func (gs *GetterSetter) CallSetter(globalObject *JSGlobalObject, thisValue JSValue, value JSValue) {
	setter := gs.m_setter
	callData := getCallDataInline(NewJSValueObject(setter))
	call(globalObject, NewJSValueObject(setter), callData, thisValue, []JSValue{value})
}
