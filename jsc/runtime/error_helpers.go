// Error helpers correspond to JSC::Error.h helper functions
package runtime

// createError creates an Error object for the given error type.
func createError(globalObject *JSGlobalObject, errType ErrorType, message string) *ErrorInstance {
	return createErrorInstance(globalObject, errType, message)
}

// createErrorWithExtension creates an Error object for the given extended error type.
func createErrorWithExtension(globalObject *JSGlobalObject, errType ErrorTypeWithExtension, message string) *ErrorInstance {
	// Map ErrorTypeWithExtension to ErrorType
	switch errType {
	case ErrorTypeWithExtensionError:
		return createError(globalObject, ErrorTypeError, message)
	case ErrorTypeWithExtensionTypeError:
		return createError(globalObject, ErrorTypeTypeError, message)
	case ErrorTypeWithExtensionRangeError:
		return createError(globalObject, ErrorTypeRangeError, message)
	case ErrorTypeWithExtensionSyntaxError:
		return createError(globalObject, ErrorTypeSyntaxError, message)
	case ErrorTypeWithExtensionReferenceError:
		return createError(globalObject, ErrorTypeReferenceError, message)
	case ErrorTypeWithExtensionEvalError:
		return createError(globalObject, ErrorTypeEvalError, message)
	case ErrorTypeWithExtensionURIError:
		return createError(globalObject, ErrorTypeURIError, message)
	case ErrorTypeWithExtensionAggregateError:
		return createError(globalObject, ErrorTypeAggregateError, message)
	case ErrorTypeWithExtensionSuppressedError:
		return createError(globalObject, ErrorTypeSuppressedError, message)
	default:
		return createError(globalObject, ErrorTypeError, "Out of memory")
	}
}

// createErrorInstance creates a named Error instance.
func createErrorInstance(globalObject *JSGlobalObject, errType ErrorType, message string) *ErrorInstance {
	vm := globalObject.VM()
	structure := globalObject.ErrorStructure(errType)
	inst := NewErrorInstance(vm, structure, errType)
	inst.FinishCreation(vm, message, LineColumn{}, "", "", "")
	return inst
}

// throwTypeError throws a TypeError (convenience).
func throwTypeError2(globalObject *JSGlobalObject, scope ThrowScope) {
	_ = scope
	globalObject.VM().ThrowException(globalObject, "TypeError")
}

// throwTypeErrorMsg throws a TypeError with a message.
func throwTypeErrorMsg(globalObject *JSGlobalObject, scope ThrowScope, msg string) {
	_ = scope
	globalObject.VM().ThrowException(globalObject, msg)
}

// throwSyntaxError throws a SyntaxError.
func throwSyntaxError2(globalObject *JSGlobalObject, scope ThrowScope, msg string) {
	_ = scope
	globalObject.VM().ThrowException(globalObject, msg)
}

// throwRangeError throws a RangeError.
func throwRangeError(globalObject *JSGlobalObject, scope ThrowScope, msg string) {
	_ = scope
	globalObject.VM().ThrowException(globalObject, msg)
}
