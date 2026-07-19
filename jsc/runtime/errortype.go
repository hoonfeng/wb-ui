// ErrorType corresponds to JSC::ErrorType (runtime/ErrorType.h)
package runtime

// ErrorType corresponds to JSC::ErrorType.
type ErrorType uint8

const (
	ErrorTypeError           ErrorType = iota
	ErrorTypeEvalError
	ErrorTypeRangeError
	ErrorTypeReferenceError
	ErrorTypeSyntaxError
	ErrorTypeTypeError
	ErrorTypeURIError
	ErrorTypeAggregateError
	ErrorTypeSuppressedError
)

const NumberOfErrorType = 9

// ErrorTypeWithExtension includes OutOfMemoryError.
type ErrorTypeWithExtension uint8

const (
	ErrorTypeWithExtensionError           ErrorTypeWithExtension = iota
	ErrorTypeWithExtensionEvalError
	ErrorTypeWithExtensionRangeError
	ErrorTypeWithExtensionReferenceError
	ErrorTypeWithExtensionSyntaxError
	ErrorTypeWithExtensionTypeError
	ErrorTypeWithExtensionURIError
	ErrorTypeWithExtensionAggregateError
	ErrorTypeWithExtensionSuppressedError
	ErrorTypeWithExtensionOutOfMemoryError
)

// errorTypeName returns the string name of an ErrorType.
func errorTypeName(t ErrorType) string {
	switch t {
	case ErrorTypeError:
		return "Error"
	case ErrorTypeEvalError:
		return "EvalError"
	case ErrorTypeRangeError:
		return "RangeError"
	case ErrorTypeReferenceError:
		return "ReferenceError"
	case ErrorTypeSyntaxError:
		return "SyntaxError"
	case ErrorTypeTypeError:
		return "TypeError"
	case ErrorTypeURIError:
		return "URIError"
	case ErrorTypeAggregateError:
		return "AggregateError"
	case ErrorTypeSuppressedError:
		return "SuppressedError"
	default:
		return "Unknown"
	}
}

// errorTypeNameWithExtension returns the string name of an ErrorTypeWithExtension.
func errorTypeNameWithExtension(t ErrorTypeWithExtension) string {
	switch t {
	case ErrorTypeWithExtensionError:
		return "Error"
	case ErrorTypeWithExtensionEvalError:
		return "EvalError"
	case ErrorTypeWithExtensionRangeError:
		return "RangeError"
	case ErrorTypeWithExtensionReferenceError:
		return "ReferenceError"
	case ErrorTypeWithExtensionSyntaxError:
		return "SyntaxError"
	case ErrorTypeWithExtensionTypeError:
		return "TypeError"
	case ErrorTypeWithExtensionURIError:
		return "URIError"
	case ErrorTypeWithExtensionAggregateError:
		return "AggregateError"
	case ErrorTypeWithExtensionSuppressedError:
		return "SuppressedError"
	case ErrorTypeWithExtensionOutOfMemoryError:
		return "OutOfMemoryError"
	default:
		return "Unknown"
	}
}
