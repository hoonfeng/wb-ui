// Translation of: Source/JavaScriptCore/builtins/BuiltinExecutables.h
//                  Source/JavaScriptCore/builtins/BuiltinExecutables.cpp
//
// BuiltinExecutables manages the lifecycle of builtin function executables.
// In JSC, each builtin JS function has an UnlinkedFunctionExecutable lazily
// created from embedded JS source. In Go, builtins are native implementations,
// so this is a simplified registry.

package builtins

import (
	"wb-ui/jsc/runtime"
)

// BuiltinCodeIndex enumerates all builtin code entries.
type BuiltinCodeIndex int

const (
	BuiltinCodeIndexArrayConstructor    BuiltinCodeIndex = iota + 1
	BuiltinCodeIndexArrayPrototype
	BuiltinCodeIndexArrayIteratorPrototype
	BuiltinCodeIndexAsyncDisposableStackPrototype
	BuiltinCodeIndexAsyncFromSyncIteratorPrototype
	BuiltinCodeIndexAsyncGeneratorPrototype
	BuiltinCodeIndexAsyncIteratorPrototype
	BuiltinCodeIndexDisposableStackPrototype
	BuiltinCodeIndexFunctionPrototype
	BuiltinCodeIndexGeneratorPrototype
	BuiltinCodeIndexGlobalOperations
	BuiltinCodeIndexIteratorHelpers
	BuiltinCodeIndexJSIteratorConstructor
	BuiltinCodeIndexJSIteratorHelperPrototype
	BuiltinCodeIndexJSIteratorPrototype
	BuiltinCodeIndexMapConstructor
	BuiltinCodeIndexMapIteratorPrototype
	BuiltinCodeIndexMapPrototype
	BuiltinCodeIndexObjectConstructor
	BuiltinCodeIndexPromiseConstructor
	BuiltinCodeIndexPromiseOperations
	BuiltinCodeIndexProxyHelpers
	BuiltinCodeIndexReflectObject
	BuiltinCodeIndexSetIteratorPrototype
	BuiltinCodeIndexSetPrototype
	BuiltinCodeIndexShadowRealmPrototype
	BuiltinCodeIndexStringConstructor
	BuiltinCodeIndexTypedArrayConstructor
	BuiltinCodeIndexTypedArrayPrototype
	BuiltinCodeIndexWrapForValidIteratorPrototype

	BuiltinCodeIndexNumberOfBuiltinCodes
)

// BuiltinExecutables corresponds to JSC::BuiltinExecutables.
type BuiltinExecutables struct {
	vm          *runtime.VM
	executables [BuiltinCodeIndexNumberOfBuiltinCodes]interface{}
}

// NewBuiltinExecutables creates a new BuiltinExecutables.
func NewBuiltinExecutables(vm *runtime.VM) *BuiltinExecutables {
	return &BuiltinExecutables{vm: vm}
}

// GetExecutable returns the cached executable for a builtin code index.
func (be *BuiltinExecutables) GetExecutable(index BuiltinCodeIndex) interface{} {
	if index < 0 || int(index) >= len(be.executables) {
		return nil
	}
	if be.executables[index] == nil {
		be.executables[index] = be.createBuiltinExecutable(index)
	}
	return be.executables[index]
}

// createBuiltinExecutable creates a new stub executable for the given index.
func (be *BuiltinExecutables) createBuiltinExecutable(index BuiltinCodeIndex) interface{} {
	name := be.builtinNameForIndex(index)
	return &builtinExecStub{
		name:  name,
		vm:    be.vm,
		index: int(index),
	}
}

// builtinExecStub is a placeholder for builtin function executables.
type builtinExecStub struct {
	name  string
	vm    *runtime.VM
	index int
}

// Name returns the builtin function name.
func (s *builtinExecStub) Name() string { return s.name }

// Clear clears all cached executables.
func (be *BuiltinExecutables) Clear() {
	for i := range be.executables {
		be.executables[i] = nil
	}
}

// builtinNameForIndex returns the builtin function name for a code index.
func (be *BuiltinExecutables) builtinNameForIndex(index BuiltinCodeIndex) string {
	names := map[BuiltinCodeIndex]string{
		BuiltinCodeIndexArrayConstructor:              "ArrayConstructor",
		BuiltinCodeIndexArrayPrototype:                "ArrayPrototype",
		BuiltinCodeIndexArrayIteratorPrototype:        "ArrayIteratorPrototype",
		BuiltinCodeIndexAsyncDisposableStackPrototype: "AsyncDisposableStackPrototype",
		BuiltinCodeIndexFunctionPrototype:             "FunctionPrototype",
		BuiltinCodeIndexGeneratorPrototype:            "GeneratorPrototype",
		BuiltinCodeIndexGlobalOperations:              "GlobalOperations",
		BuiltinCodeIndexIteratorHelpers:               "IteratorHelpers",
		BuiltinCodeIndexJSIteratorConstructor:         "JSIteratorConstructor",
		BuiltinCodeIndexJSIteratorPrototype:           "JSIteratorPrototype",
		BuiltinCodeIndexMapConstructor:                "MapConstructor",
		BuiltinCodeIndexMapIteratorPrototype:          "MapIteratorPrototype",
		BuiltinCodeIndexMapPrototype:                  "MapPrototype",
		BuiltinCodeIndexObjectConstructor:             "ObjectConstructor",
		BuiltinCodeIndexPromiseConstructor:            "PromiseConstructor",
		BuiltinCodeIndexPromiseOperations:             "PromiseOperations",
		BuiltinCodeIndexProxyHelpers:                  "ProxyHelpers",
		BuiltinCodeIndexReflectObject:                 "ReflectObject",
		BuiltinCodeIndexSetIteratorPrototype:          "SetIteratorPrototype",
		BuiltinCodeIndexSetPrototype:                  "SetPrototype",
		BuiltinCodeIndexShadowRealmPrototype:          "ShadowRealmPrototype",
		BuiltinCodeIndexStringConstructor:             "StringConstructor",
		BuiltinCodeIndexTypedArrayConstructor:         "TypedArrayConstructor",
		BuiltinCodeIndexTypedArrayPrototype:           "TypedArrayPrototype",
	}
	if name, ok := names[index]; ok {
		return name
	}
	return "UnknownBuiltin"
}
