// InternalFunction corresponds to JSC::InternalFunction (runtime/InternalFunction.h)
package runtime

// InternalFunction corresponds to JSC::InternalFunction.
// It is the base class for built-in constructors like Object, Array, etc.
type InternalFunction struct {
	JSNonFinalObject
	functionForCall      func(globalObject *JSGlobalObject, callFrame *ExecState) JSValue
	functionForConstruct func(globalObject *JSGlobalObject, callFrame *ExecState) JSValue
	globalObject         *JSGlobalObject
	originalName         string
}

// NewInternalFunction creates a new InternalFunction.
func NewInternalFunction(vm *VM, structure *Structure, functionForCall func(*JSGlobalObject, *ExecState) JSValue, functionForConstruct func(*JSGlobalObject, *ExecState) JSValue) *InternalFunction {
	fn := &InternalFunction{
		functionForCall:      functionForCall,
		functionForConstruct: functionForConstruct,
	}
	fn.structureID = structure.structureID
	fn.typ = InternalFunctionType
	fn.cellState = DefinitelyWhite
	fn.properties = make(map[string]JSValue)
	_ = vm
	return fn
}

// FinishCreation completes InternalFunction initialization.
func (f *InternalFunction) FinishCreation(vm *VM, length int, name string) {
	f.JSNonFinalObject.FinishCreation(vm)
	f.originalName = name
	// Set length and name properties
	f.putDirectWithoutTransition(vm, NewPropertyName("length"), jsNumber(float64(length)), PropertyAttributeReadOnly|PropertyAttributeDontEnum)
	f.putDirectWithoutTransition(vm, NewPropertyName("name"), NewJSValueString(name), PropertyAttributeReadOnly|PropertyAttributeDontEnum)
}

// GetCallData returns CallData for the InternalFunction.
func (f *InternalFunction) GetCallData() CallData {
	if f.functionForCall != nil {
		return CallData{Type: CallTypeNative, NativeFunction: f.functionForCall}
	}
	return CallData{Type: CallTypeNone}
}

// GetConstructData returns ConstructData for the InternalFunction.
func (f *InternalFunction) GetConstructData() ConstructData {
	if f.functionForConstruct != nil {
		return ConstructData{Type: ConstructTypeNative, NativeFunction: f.functionForConstruct}
	}
	return ConstructData{Type: ConstructTypeNone}
}

// Call invokes [[Call]] on this InternalFunction.
func (f *InternalFunction) Call(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	if f.functionForCall != nil {
		result := f.functionForCall(globalObject, callFrame)
		if callFrame.vm.Exception != nil {
			return JSValueUndefined, nil
		}
		return result, nil
	}
	return JSValueUndefined, nil
}

// Construct invokes [[Construct]] on this InternalFunction.
func (f *InternalFunction) Construct(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	if f.functionForConstruct != nil {
		result := f.functionForConstruct(globalObject, callFrame)
		if callFrame.vm.Exception != nil {
			return JSValueUndefined, nil
		}
		return result, nil
	}
	return JSValueUndefined, nil
}

// NullSetterFunction corresponds to JSC::NullSetterFunction.
type NullSetterFunction struct {
	InternalFunction
}
