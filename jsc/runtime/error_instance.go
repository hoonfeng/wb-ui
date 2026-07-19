// ErrorInstance corresponds to JSC::ErrorInstance (runtime/ErrorInstance.h/.cpp)
package runtime

// ErrorInstance corresponds to JSC::ErrorInstance.
type ErrorInstance struct {
	JSNonFinalObject
	errorType                    ErrorType
	sourceAppender               func(originalMessage string, sourceText string, runtimeType RuntimeType, sourceOccurred SourceTextWhereErrorOccurred) string
	runtimeTypeForCause          RuntimeType
	stackOverflowError           bool
	outOfMemoryError             bool
	errorInfoMaterialized        bool
	stackPropertyAlreadyMaterialized bool
	nativeGetterTypeError        bool
	parseError                   bool
	catchableFromWasm            bool
}

// SourceTextWhereErrorOccurred indicates whether the exact source was found.
type SourceTextWhereErrorOccurred bool

const (
	FoundExactSource      SourceTextWhereErrorOccurred = true
	FoundApproximateSource SourceTextWhereErrorOccurred = false
)

// SourceAppender is a function type for appending source text to error messages.
type SourceAppender func(originalMessage string, sourceText string, runtimeType RuntimeType, sourceOccurred SourceTextWhereErrorOccurred) string

// NewErrorInstance creates a new ErrorInstance.
func NewErrorInstance(vm *VM, structure *Structure, errorType ErrorType) *ErrorInstance {
	inst := &ErrorInstance{
		errorType:          errorType,
		stackOverflowError: false,
		outOfMemoryError:   false,
		catchableFromWasm:  true,
	}
	inst.structureID = structure.structureID
	inst.typ = ObjectType
	inst.cellState = DefinitelyWhite
	inst.properties = make(map[string]JSValue)
	_ = vm
	return inst
}

// CreateErrorInstance creates an ErrorInstance with a message (static factory).
func CreateErrorInstance(globalObject *JSGlobalObject, message string, errorType ErrorType, lineColumn LineColumn, sourceURL string, stackString string, cause string) *ErrorInstance {
	vm := globalObject.VM()
	structure := globalObject.ErrorStructure(errorType)
	inst := NewErrorInstance(vm, structure, errorType)
	inst.FinishCreation(vm, message, lineColumn, sourceURL, stackString, cause)
	return inst
}

// FinishCreation completes ErrorInstance initialization.
func (e *ErrorInstance) FinishCreation(vm *VM, message string, lineColumn LineColumn, sourceURL string, stackString string, cause string) {
	_ = vm
	_ = message
	_ = lineColumn
	_ = sourceURL
	_ = stackString
	_ = cause
	// In JSC, this sets up the error's message, stack, and cause properties
	// Simplified Go translation
	e.putDirectWithoutTransition(vm, NewPropertyName("message"), NewJSValueString(message), PropertyAttributeDontEnum)
	if cause != "" {
		e.putDirectWithoutTransition(vm, NewPropertyName("cause"), NewJSValueString(cause), PropertyAttributeDontEnum)
	}
}

// HasSourceAppender returns true if this error has a source appender.
func (e *ErrorInstance) HasSourceAppender() bool {
	return e.sourceAppender != nil
}

// SourceAppender returns the source appender function.
func (e *ErrorInstance) SourceAppender() SourceAppender {
	return e.sourceAppender
}

// SetSourceAppender sets the source appender function.
func (e *ErrorInstance) SetSourceAppender(appender SourceAppender) {
	e.sourceAppender = appender
}

// ClearSourceAppender clears the source appender.
func (e *ErrorInstance) ClearSourceAppender() {
	e.sourceAppender = nil
}

// SetRuntimeTypeForCause sets the runtime type for the cause.
func (e *ErrorInstance) SetRuntimeTypeForCause(typ RuntimeType) {
	e.runtimeTypeForCause = typ
}

// RuntimeTypeForCause returns the runtime type for the cause.
func (e *ErrorInstance) RuntimeTypeForCause() RuntimeType {
	return e.runtimeTypeForCause
}

// ClearRuntimeTypeForCause clears the runtime type for the cause.
func (e *ErrorInstance) ClearRuntimeTypeForCause() {
	e.runtimeTypeForCause = TypeNothing
}

// ErrorType returns the error type.
func (e *ErrorInstance) ErrorType() ErrorType {
	return e.errorType
}

// SetStackOverflowError marks this error as a stack overflow.
func (e *ErrorInstance) SetStackOverflowError() {
	e.catchableFromWasm = false
	e.stackOverflowError = true
}

// IsStackOverflowError returns true if this is a stack overflow error.
func (e *ErrorInstance) IsStackOverflowError() bool {
	return e.stackOverflowError
}

// SetOutOfMemoryError marks this error as an OOM error.
func (e *ErrorInstance) SetOutOfMemoryError() {
	e.outOfMemoryError = true
}

// IsOutOfMemoryError returns true if this is an OOM error.
func (e *ErrorInstance) IsOutOfMemoryError() bool {
	return e.outOfMemoryError
}

// SetNativeGetterTypeError marks this as a native getter type error.
func (e *ErrorInstance) SetNativeGetterTypeError() {
	e.nativeGetterTypeError = true
}

// IsNativeGetterTypeError returns true if this is a native getter type error.
func (e *ErrorInstance) IsNativeGetterTypeError() bool {
	return e.nativeGetterTypeError
}

// LineColumn corresponds to JSC::LineColumn.
type LineColumn struct {
	Line   uint32
	Column uint32
}

// RuntimeType corresponds to JSC::RuntimeType.
type RuntimeType uint8

const (
	TypeNothing RuntimeType = 0
	TypeBoolean RuntimeType = 1
	TypeNumber  RuntimeType = 2
	TypeString  RuntimeType = 4
	TypeObject  RuntimeType = 8
	TypeSymbol  RuntimeType = 16
	TypeBigInt  RuntimeType = 32
)
