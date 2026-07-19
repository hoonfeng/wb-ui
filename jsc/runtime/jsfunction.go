// Translation of: Source/JavaScriptCore/runtime/JSFunction.h
//                  Source/JavaScriptCore/runtime/JSFunction.cpp
//
// JSFunction is the JSC function type — wraps native Go callbacks or
// script-defined closures.

package runtime

// JSFunction corresponds to JSC::JSFunction.
// It stores either a native (Go) function or a script FunctionExecutable.
type JSFunction struct {
	JSObject
	// executable stores the FunctionExecutable (script) or nil for native.
	executable *FunctionExecutable
	// nativeFunc stores the Go callback for native functions.
	nativeFunc func(globalObject *JSGlobalObject, thisValue JSValue, args []JSValue) (JSValue, error)
	// constructor for native constructors.
	nativeConstructor func(globalObject *JSGlobalObject, args []JSValue) (JSValue, error)
	// functionLength is the .length property value.
	functionLength int
	// functionName is the .name property value.
	functionName string
}

// StructureFlags for JSFunction.
const JSFunctionStructureFlags uint32 = JSObjectStructureFlags | OverridesGetCallData

// InternalFunctionStructureFlags is the StructureFlags for InternalFunction.
const InternalFunctionStructureFlags uint32 = JSObjectStructureFlags | OverridesGetCallData | ImplementsHasInstance | ImplementsDefaultHasInstance

// NewJSFunction creates a JSFunction with a native Go callback.
func NewJSFunction(vm *VM, globalObject *JSGlobalObject, name string, length int, fn func(*JSGlobalObject, JSValue, []JSValue) (JSValue, error)) *JSFunction {
	_ = vm
	_ = globalObject
	f := &JSFunction{
		functionLength: length,
		functionName:   name,
		nativeFunc:     fn,
	}
	f.typ = JSFunctionType
	f.cellState = DefinitelyWhite
	f.properties = make(map[string]JSValue)
	return f
}

// NewJSFunctionWithExecutable creates a JSFunction backed by a FunctionExecutable.
func NewJSFunctionWithExecutable(vm *VM, globalObject *JSGlobalObject, executable *FunctionExecutable) *JSFunction {
	_ = vm
	_ = globalObject
	f := &JSFunction{
		executable: executable,
	}
	f.typ = JSFunctionType
	f.cellState = DefinitelyWhite
	f.properties = make(map[string]JSValue)
	return f
}

// --- Accessors ---

// Executable returns the FunctionExecutable (nil for native functions).
func (f *JSFunction) Executable() *FunctionExecutable { return f.executable }

// NativeFunc returns the native Go callback.
func (f *JSFunction) NativeFunc() func(*JSGlobalObject, JSValue, []JSValue) (JSValue, error) {
	return f.nativeFunc
}

// SetNativeFunc sets the native Go callback.
func (f *JSFunction) SetNativeFunc(fn func(*JSGlobalObject, JSValue, []JSValue) (JSValue, error)) {
	f.nativeFunc = fn
}

// FunctionLength returns the .length property.
func (f *JSFunction) FunctionLength() int { return f.functionLength }

// FunctionName returns the .name property.
func (f *JSFunction) FunctionName() string { return f.functionName }

// SetFunctionName sets the function name.
func (f *JSFunction) SetFunctionName(name string) { f.functionName = name }

// --- Callable ---

// Call implements [[Call]] for JSFunction.
func (f *JSFunction) Call(globalObject *JSGlobalObject, thisValue JSValue, args []JSValue) (JSValue, error) {
	if f.nativeFunc != nil {
		return f.nativeFunc(globalObject, thisValue, args)
	}
	// TODO: script-defined function call
	_ = thisValue
	return JSValueUndefined, nil
}

// Construct implements [[Construct]] for JSFunction.
func (f *JSFunction) Construct(globalObject *JSGlobalObject, args []JSValue) (JSValue, error) {
	if f.nativeConstructor != nil {
		return f.nativeConstructor(globalObject, args)
	}
	// Default: create a new object
	obj := CreateEmptyJSObject(nil, globalObject)
	return JSValue{tag: TagObject, payload: obj}, nil
}

// IsHostFunction returns true if this is a native function.
func (f *JSFunction) IsHostFunction() bool {
	return f.nativeFunc != nil
}

// IsClassConstructorFunction returns true if this is a class constructor.
func (f *JSFunction) IsClassConstructorFunction() bool {
	return false // simplified
}

// --- GetCallData/GetConstructData (global functions) ---

func init() {
	// Register JSFunction as callable in GetCallData
}
