// Translation of: Source/JavaScriptCore/builtins/BuiltinUtils.h
//
// BuiltinUtils provides macros and declarations used by the builtin names system.

package builtins

import "wb-ui/jsc/runtime"

// CreateBuiltinExecutable is a helper that creates an UnlinkedFunctionExecutable
// for a builtin function from SourceCode. In the Go translation, this creates
// a stub entry in the BuiltinExecutables registry.
func CreateBuiltinExecutable(vm *runtime.VM, name string) *BuiltinExecStub {
	return &BuiltinExecStub{
		name: name,
		vm:   vm,
	}
}

// BuiltinExecStub is a placeholder executable for builtin functions.
type BuiltinExecStub struct {
	name string
	vm   *runtime.VM
}

// Name returns the builtin function name.
func (s *BuiltinExecStub) Name() string { return s.name }
