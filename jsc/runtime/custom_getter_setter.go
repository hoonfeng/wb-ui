// Copyright (C) 2014-2022 Apple Inc. All rights reserved.
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
// THIS SOFTWARE IS PROVIDED BY APPLE INC. AND ITS CONTRIBUTORS ``AS IS''
// AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO,
// THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR
// PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL APPLE INC. OR ITS CONTRIBUTORS
// BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
// CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
// SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
// INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
// CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
// ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF
// THE POSSIBILITY OF SUCH DAMAGE.

package runtime

// CustomGetter is the Go type for GetValueFunc
type CustomGetter func(globalObject *JSGlobalObject, thisValue JSValue, propertyName PropertyName) JSValue

// CustomSetter is the Go type for PutValueFunc
type CustomSetter func(globalObject *JSGlobalObject, thisValue JSValue, propertyName PropertyName, value JSValue) bool

// CustomGetterSetter corresponds to JSC::CustomGetterSetter
// Stores native getter/setter function pointers for custom properties.
type CustomGetterSetter struct {
	JSCell
	m_getter CustomGetter
	m_setter CustomSetter
}

const CustomGetterSetterStructureFlags uint32 = JSCellStructureFlags | OverridesGetOwnPropertySlot | OverridesPut | StructureIsImmortal

func NewCustomGetterSetter(vm *VM, getter CustomGetter, setter CustomSetter) *CustomGetterSetter {
	cgs := &CustomGetterSetter{
		JSCell:   *NewJSCell(vm, vm.customGetterSetterStructure),
		m_getter: getter,
		m_setter: setter,
	}
	cgs.finishCreation(vm)
	return cgs
}

func (cgs *CustomGetterSetter) Getter() CustomGetter { return cgs.m_getter }
func (cgs *CustomGetterSetter) Setter() CustomSetter { return cgs.m_setter }
