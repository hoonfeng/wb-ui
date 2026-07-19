// Copyright (C) 2008-2023 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/interpreter/Register.h

package interpreter

import (
	"wb-ui/jsc/runtime"
)

// Register represents a single register in the virtual machine.
// In WebKit, this is a union that can hold a JSValue, pointer, or unboxed value.
// In Go, we store the JSValue directly and provide accessors for different views.
type Register struct {
	jsValue runtime.JSValue
}

func NewRegister() Register {
	return Register{jsValue: runtime.JSValueUndefined}
}

func RegisterFromJSValue(val runtime.JSValue) Register {
	return Register{jsValue: val}
}

func RegisterFromInt32(val int32) Register {
	return Register{jsValue: runtime.NewJSValueInt32(val)}
}

func RegisterFromDouble(val float64) Register {
	return Register{jsValue: runtime.NewJSValueNumber(val)}
}

func RegisterFromBoolean(val bool) Register {
	return Register{jsValue: runtime.NewJSValueBool(val)}
}

func (r Register) JSValue() runtime.JSValue { return r.jsValue }
func (r *Register) SetJSValue(val runtime.JSValue) { r.jsValue = val }

func (r Register) UnboxedInt32() int32 {
	if r.jsValue.IsInt32() {
		return int32(r.jsValue.GetNumber())
	}
	return 0
}

func (r Register) UnboxedDouble() float64 {
	if r.jsValue.IsNumber() {
		return r.jsValue.GetNumber()
	}
	return 0
}

func (r Register) UnboxedBoolean() bool {
	return r.jsValue.ToBoolean()
}

func (r Register) Pointer() *runtime.JSCell {
	if obj := r.jsValue.GetObject(); obj != nil {
		return obj.JSCellRef()
	}
	return nil
}
