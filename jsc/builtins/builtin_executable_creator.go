// Translation of: Source/JavaScriptCore/builtins/BuiltinExecutableCreator.h
//                  Source/JavaScriptCore/builtins/BuiltinExecutableCreator.cpp
//
// BuiltinExecutableCreator provides the createBuiltinExecutable helper.

package builtins

import "wb-ui/jsc/runtime"

// CreateBuiltinExecutableFromParams corresponds to JSC::createBuiltinExecutable().
// It creates a builtin executable with the given parameters.
func CreateBuiltinExecutableFromParams(vm *runtime.VM, name string) *BuiltinExecStub {
	return &BuiltinExecStub{
		name: name,
		vm:   vm,
	}
}
