// Copyright (C) 2011-2023 Apple Inc. All rights reserved.
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

// JSBoundFunction corresponds to JSC::JSBoundFunction.
// Created by Function.prototype.bind().
type JSBoundFunction struct {
	JSFunction
	targetFunction *JSObject
	boundThis      JSValue
	boundArgs      []JSValue
	boundArg0      JSValue
	boundArg1      JSValue
	boundArg2      JSValue
	nameMayBeNull  string
	m_length       float64
	boundArgsLen   uint32
	canConstruct   TriState
	isTainted      bool
}

const BoundFunctionStructureFlags uint32 = JSFunctionStructureFlags & ^uint32(ImplementsDefaultHasInstance)

// NewJSBoundFunction creates a bound function.
func NewJSBoundFunction(vm *VM, globalObject *JSGlobalObject, target *JSObject, boundThis JSValue, boundArgs []JSValue, length float64, name string) *JSBoundFunction {
	_ = vm
	bf := &JSBoundFunction{
		targetFunction: target,
		boundThis:      boundThis,
		boundArgs:      boundArgs,
		m_length:       length,
		boundArgsLen:   uint32(len(boundArgs)),
		nameMayBeNull:  name,
		canConstruct:   TriStateIndeterminate,
	}
	bf.typ = JSFunctionType
	bf.cellState = DefinitelyWhite
	bf.properties = make(map[string]JSValue)
	bf.functionName = "bound " + boundFunctionName(target)

	// Copy embedded args (up to 3)
	if len(boundArgs) > 0 {
		bf.boundArg0 = boundArgs[0]
	}
	if len(boundArgs) > 1 {
		bf.boundArg1 = boundArgs[1]
	}
	if len(boundArgs) > 2 {
		bf.boundArg2 = boundArgs[2]
	}

	return bf
}

func (bf *JSBoundFunction) TargetFunction() *JSObject { return bf.targetFunction }
func (bf *JSBoundFunction) BoundThis() JSValue         { return bf.boundThis }
func (bf *JSBoundFunction) BoundArgsLength() uint32    { return bf.boundArgsLen }

// combinedArgs combines bound args with call-time args.
func (bf *JSBoundFunction) combinedArgs(args []JSValue) []JSValue {
	total := len(bf.boundArgs) + len(args)
	result := make([]JSValue, 0, total)
	result = append(result, bf.boundArgs...)
	result = append(result, args...)
	return result
}

// Call implements [[Call]] for a bound function.
func (bf *JSBoundFunction) Call(globalObject *JSGlobalObject, thisValue JSValue, args []JSValue) (JSValue, error) {
	_ = thisValue
	combined := bf.combinedArgs(args)
	return bf.targetFunction.Call(globalObject, bf.boundThis, combined)
}

// Construct implements [[Construct]] for a bound function.
func (bf *JSBoundFunction) Construct(globalObject *JSGlobalObject, args []JSValue) (JSValue, error) {
	if bf.canConstruct == TriStateFalse {
		return JSValueUndefined, nil // not constructable
	}

	// If target is a JSFunction with construct
	if fn, ok := interface{}(bf.targetFunction).(*JSFunction); ok {
		combined := bf.combinedArgs(args)
		return fn.Construct(globalObject, combined)
	}

	return JSValueUndefined, nil
}

// boundFunctionName gets the name from a target function for the "bound " prefix.
func boundFunctionName(target *JSObject) string {
	if target == nil {
		return ""
	}
	// Try to get the "name" property
	nameVal := target.Get(nil, NewPropertyName("name"))
	if !nameVal.IsUndefined() && nameVal.IsString() {
		s := nameVal.ToString()
		return s
	}
	return ""
}

// IsBoundFunction checks if a JSValue is a bound function.
func IsBoundFunction(val JSValue) bool {
	if !val.IsObject() {
		return false
	}
	obj := val.GetObject()
	_, ok := interface{}(obj).(*JSBoundFunction)
	return ok
}
