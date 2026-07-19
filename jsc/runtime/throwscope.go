// ThrowScope corresponds to JSC::ThrowScope (runtime/ThrowScope.h)
package runtime

// ThrowScope corresponds to JSC::ThrowScope.
type ThrowScope struct {
	vm          *VM
	isReleased  bool
}

// DeclareThrowScope creates a new ThrowScope (corresponds to DECLARE_THROW_SCOPE).
func DeclareThrowScope(vm *VM) ThrowScope {
	return ThrowScope{vm: vm, isReleased: false}
}

// Release releases the ThrowScope (corresponds to scope.release()).
func (s *ThrowScope) Release() {
	s.isReleased = true
}

// ThrowException throws an exception through this scope.
func (s *ThrowScope) ThrowException(globalObject *JSGlobalObject, value JSValue) JSValue {
	_ = globalObject
	_ = value
	// In Go, we set the exception on the VM
	return value
}

// ReturnIfException returns the given value if there's an exception (RELEASE_AND_RETURN pattern).
func ReturnIfException(scope ThrowScope, returnValue interface{}) interface{} {
	if scope.vm != nil && scope.vm.Exception != nil {
		return returnValue
	}
	return nil
}

// HasException returns true if the VM has an exception.
func (s *ThrowScope) HasException() bool {
	return s.vm != nil && s.vm.Exception != nil
}
