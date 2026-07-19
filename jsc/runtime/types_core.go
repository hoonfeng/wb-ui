// Forward declarations for JSC types without dedicated files.
// Types with own files (VM, JSValue, JSCell, JSObject, JSFunction, JSArray,
// JSString, JSGlobalObject, Structure, etc.) are NOT duplicated here.

package runtime




// Heap manages GC memory.
type Heap struct{}

// ClassInfo holds metadata about a JS class.
type ClassInfo struct{}

// JSNonFinalObject corresponds to JSC::JSNonFinalObject.
// It is JSObject without finalization support — the common base for prototype objects.
type JSNonFinalObject struct{ JSObject }

// --- Value types (no own file yet) ---

type JSBigInt struct{ JSCell }
type Symbol struct{ JSCell }
type GetterSetter struct{ JSCell }
type CustomGetterSetter struct{ JSCell }
type NativeExecutable struct{ JSCell }
type CodeBlock struct{ JSCell }

// --- Object types (no own file yet) ---

type JSFinalObject struct{ JSObject }

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

// finishCreation completes InternalFunction initialization.
func (f *InternalFunction) finishCreation(vm *VM, length int, name string) {
	f.JSNonFinalObject.finishCreation(vm)
	f.originalName = name
	// Set length and name properties
	f.putDirectWithoutTransition(vm, NewPropertyName("length"), jsNumber(float64(length)), PropertyAttributeReadOnly|PropertyAttributeDontEnum)
	f.putDirectWithoutTransition(vm, NewPropertyName("name"), NewJSValueString(name), PropertyAttributeReadOnly|PropertyAttributeDontEnum)
}
// It is the base class for built-in constructors like Object, Array, etc.
type InternalFunction struct {
	JSNonFinalObject
	functionForCall      func(globalObject *JSGlobalObject, callFrame *ExecState) JSValue
	functionForConstruct func(globalObject *JSGlobalObject, callFrame *ExecState) JSValue
	globalObject         *JSGlobalObject
	originalName         string
}
type NullSetterFunction struct{ InternalFunction }
type JSArrayIterator struct{ JSObject }
type JSArrayBuffer struct{ JSObject }
type JSArrayBufferView struct{ JSObject }
type JSPromise struct{ JSObject }
type JSMap struct{ JSObject }
type JSSet struct{ JSObject }
type JSWeakMap struct{ JSObject }
type JSWeakSet struct{ JSObject }
type JSGlobalProxy struct{ JSObject }
type ProxyObject struct{ JSObject }
type RegExpObject struct{ JSObject }
type ErrorInstance struct{ JSObject }
type NumberObject struct{ JSObject }
type BooleanObject struct{ JSObject }
type StringObject struct{ JSObject }
type DateInstance struct{ JSObject }
type JSCallee struct{ JSObject }
type DirectArguments struct{ JSObject }
type ScopedArguments struct{ JSObject }
type ClonedArguments struct{ JSObject }
type JSGenerator struct{ JSObject }
type JSAsyncGenerator struct{ JSObject }
type JSMapIterator struct{ JSObject }
type JSSetIterator struct{ JSObject }
type JSStringIterator struct{ JSObject }

// --- Scope types ---

type JSScope struct{ JSObject }
type JSLexicalEnvironment struct{ JSScope }
type JSModuleEnvironment struct{ JSLexicalEnvironment }
type JSGlobalLexicalEnvironment struct{ JSScope }
type StrictEvalActivation struct{ JSScope }
type WithScope struct{ JSScope }

// --- Executable types ---

type FunctionExecutable struct{ JSCell }
type ProgramExecutable struct{ JSCell }
type EvalExecutable struct{ JSCell }
type ModuleProgramExecutable struct{ JSCell }
type UnlinkedFunctionExecutable struct{ JSCell }
// --- PropertyOffset ---

// PropertyOffset corresponds to JSC::PropertyOffset.
type PropertyOffset uint8

const (
	PropertyOffsetInvalid PropertyOffset = 0xff
)

// --- PropertyDescriptor (JSC::PropertyDescriptor) ---
// --- PropertyDescriptor (JSC::PropertyDescriptor) ---

// PropertyDescriptor corresponds to JSC::PropertyDescriptor.
// Used by Object.defineProperty, Object.getOwnPropertyDescriptor, etc.
type PropertyDescriptor struct {
	m_value           JSValue
	m_getter          JSValue
	m_setter          JSValue
	m_attributes      uint8 // PropertyAttribute flags
	m_seenValue       bool
	m_seenWritable    bool
	m_seenGetter      bool
	m_seenSetter      bool
	m_seenEnumerable  bool
	m_seenConfigurable bool
}

// NewPropertyDescriptor creates an empty PropertyDescriptor.
func NewPropertyDescriptor() PropertyDescriptor {
	return PropertyDescriptor{}
}

func (d *PropertyDescriptor) Value() JSValue           { return d.m_value }
func (d *PropertyDescriptor) SetValue(v JSValue)       { d.m_value = v; d.m_seenValue = true }
func (d *PropertyDescriptor) Getter() JSValue           { return d.m_getter }
func (d *PropertyDescriptor) SetGetter(g JSValue)       { d.m_getter = g; d.m_seenGetter = true }
func (d *PropertyDescriptor) Setter() JSValue           { return d.m_setter }
func (d *PropertyDescriptor) SetSetter(s JSValue)       { d.m_setter = s; d.m_seenSetter = true }
func (d *PropertyDescriptor) Writable() bool            { return d.m_attributes&PropertyAttributeReadOnly == 0 }
func (d *PropertyDescriptor) SetWritable(b bool)        { d.m_seenWritable = true; if !b { d.m_attributes |= PropertyAttributeReadOnly } else { d.m_attributes &^= PropertyAttributeReadOnly } }
func (d *PropertyDescriptor) Enumerable() bool          { return d.m_attributes&PropertyAttributeDontEnum == 0 }
func (d *PropertyDescriptor) SetEnumerable(b bool)      { d.m_seenEnumerable = true; if !b { d.m_attributes |= PropertyAttributeDontEnum } else { d.m_attributes &^= PropertyAttributeDontEnum } }
func (d *PropertyDescriptor) Configurable() bool        { return d.m_attributes&PropertyAttributeDontDelete == 0 }
func (d *PropertyDescriptor) SetConfigurable(b bool)    { d.m_seenConfigurable = true; if !b { d.m_attributes |= PropertyAttributeDontDelete } else { d.m_attributes &^= PropertyAttributeDontDelete } }

func (d *PropertyDescriptor) IsAccessorDescriptor() bool { return d.m_seenGetter || d.m_seenSetter }
func (d *PropertyDescriptor) IsDataDescriptor() bool     { return d.m_seenValue || d.m_seenWritable }
func (d *PropertyDescriptor) IsGenericDescriptor() bool  { return !d.IsAccessorDescriptor() && !d.IsDataDescriptor() }
func (d *PropertyDescriptor) Attributes() uint8          { return d.m_attributes }
func (d *PropertyDescriptor) SetAttributes(a uint8)      { d.m_attributes = a }
func (d *PropertyDescriptor) EnumerablePresent() bool    { return d.m_seenEnumerable }
func (d *PropertyDescriptor) ConfigurablePresent() bool  { return d.m_seenConfigurable }
func (d *PropertyDescriptor) WritablePresent() bool      { return d.m_seenWritable }
func (d *PropertyDescriptor) GetterPresent() bool        { return d.m_seenGetter }
func (d *PropertyDescriptor) SetterPresent() bool        { return d.m_seenSetter }

// --- Identifier (simplified) ---

type Identifier struct{ m_impl string }

func NewIdentifier(s string) Identifier { return Identifier{m_impl: s} }
func (id Identifier) String() string    { return id.m_impl }
func (id Identifier) Impl() string      { return id.m_impl }
func (id Identifier) IsSymbol() bool    { return false }
func (id Identifier) IsEmpty() bool     { return id.m_impl == "" }

// --- PropertyNameArrayBuilder (simplified) ---

type PropertyNameArrayBuilder struct {
	names []Identifier
}

func NewPropertyNameArrayBuilder(vm *VM, mode PropertyNameMode, privateSymbolMode PrivateSymbolMode) PropertyNameArrayBuilder {
	_ = vm; _ = privateSymbolMode; _ = mode
	return PropertyNameArrayBuilder{}
}

func (b *PropertyNameArrayBuilder) Add(name Identifier) { b.names = append(b.names, name) }
func (b *PropertyNameArrayBuilder) Size() int            { return len(b.names) }
func (b *PropertyNameArrayBuilder) Get(i int) Identifier { return b.names[i] }
func (b *PropertyNameArrayBuilder) Names() []Identifier  { return b.names }

// --- Misc ---

type JSModuleRecord struct{ JSCell }
type JSModuleNamespaceObject struct{ JSObject }
type JSModuleLoader struct{ JSObject }
type ShadowRealmObject struct{ JSObject }

// PreferredPrimitiveType hints for ToPrimitive.
type PreferredPrimitiveType uint8

const (
	NoPreference PreferredPrimitiveType = iota
	PreferNumber
	PreferString
)

// CallType enum.
type CallType uint8

const (
	CallTypeNone CallType = iota
	CallTypeHost
	CallTypeJS
	CallTypeDOM
)

// ConstructType enum.
type ConstructType uint8

const (
	ConstructTypeNone ConstructType = iota
	ConstructTypeHost
	ConstructTypeJS
	ConstructTypeDOM
)

// CallData corresponds to JSC::CallData.
type CallData struct{ Type CallType }

// ConstructData corresponds to JSC::ConstructData.
type ConstructData struct{ Type ConstructType }

// PropertyName (simplified).
type PropertyName struct{ name string }

func NewPropertyName(name string) PropertyName { return PropertyName{name: name} }
func (pn PropertyName) String() string         { return pn.name }

// PropertySlot (simplified).
type PropertySlot struct{ Value JSValue }

// PutPropertySlot (simplified).
type PutPropertySlot struct{}

// DeletePropertySlot (simplified).
type DeletePropertySlot struct{}

// PropertyAttribute flags (from JSC PropertyAttribute.h).
const (
	PropertyAttributeReadOnly            uint8 = 1 << 0
	PropertyAttributeDontEnum            uint8 = 1 << 1
	PropertyAttributeDontDelete          uint8 = 1 << 2
	PropertyAttributeAccessor            uint8 = 1 << 3
	PropertyAttributeCustomAccessor      uint8 = 1 << 4
	PropertyAttributeBuiltinOrFunction   uint8 = 1 << 5
	PropertyAttributeBuiltinOrFunctionOrAccessorOrCustomAccessor uint8 = PropertyAttributeBuiltinOrFunction | PropertyAttributeAccessor | PropertyAttributeCustomAccessor
	PropertyAttributeFunction            uint8 = PropertyAttributeBuiltinOrFunction
	PropertyAttributeBuiltin             uint8 = PropertyAttributeBuiltinOrFunction
	PropertyAttributeAccessorOrCustomAccessor uint8 = PropertyAttributeAccessor | PropertyAttributeCustomAccessor
)

// PropertyNameMode.
type PropertyNameMode uint8

const (
	PropertyNameModeStrings          PropertyNameMode = iota
	PropertyNameModeSymbols
	PropertyNameModeStringsAndSymbols
)

// PrivateSymbolMode.
type PrivateSymbolMode uint8

const (
	PrivateSymbolModeExclude PrivateSymbolMode = iota
	PrivateSymbolModeInclude
)

// ImplementationVisibility.
type ImplementationVisibility uint8

const (
	ImplementationVisibilityPublic ImplementationVisibility = iota
	ImplementationVisibilityPrivate
)

// Intrinsic (simplified).
type Intrinsic uint8

const (
	NoIntrinsic                        Intrinsic = 0
	HasOwnPropertyIntrinsic            Intrinsic = 1
	ObjectGetPrototypeOfIntrinsic      Intrinsic = 2
	ObjectGetOwnPropertyNamesIntrinsic Intrinsic = 3
	ObjectGetOwnPropertySymbolsIntrinsic Intrinsic = 4
	ObjectKeysIntrinsic                Intrinsic = 5
	ObjectCreateIntrinsic              Intrinsic = 6
	ObjectDefinePropertyIntrinsic      Intrinsic = 7
	ObjectAssignIntrinsic              Intrinsic = 8
	ObjectIsIntrinsic                  Intrinsic = 9
	ObjectHasOwnIntrinsic              Intrinsic = 10
)

// DontEnumPropertiesMode.
type DontEnumPropertiesMode uint8

const (
	IncludeDontEnumProperties DontEnumPropertiesMode = iota
	ExcludeDontEnumProperties
)

// ThrowScope (simplified).
type ThrowScope struct{ vm *VM }

func DeclareThrowScope(vm *VM) ThrowScope { return ThrowScope{vm: vm} }

func ReturnIfException(scope ThrowScope, returnValue interface{}) interface{} {
	if scope.vm != nil && scope.vm.Exception != nil {
		return returnValue
	}
	return nil
}

// --- ASSERT / RELEASE_ASSERT macros (simplified) ---

func ASSERT(cond bool) {
	if !cond {
		panic("ASSERTION FAILED")
	}
}

func RELEASE_ASSERT(cond bool) {
	if !cond {
		panic("RELEASE_ASSERT FAILED")
	}
}

func RELEASE_ASSERT_NOT_REACHED() {
	panic("RELEASE_ASSERT_NOT_REACHED")
}

func EXCEPTION_ASSERT(cond bool) {
	if !cond {
		// non-fatal in debug
	}
}

// --- JSValue helper functions ---

// jsBoolean creates a boolean JSValue.
func jsBoolean(b bool) JSValue {
	return NewJSValueBool(b)
}

// jsUndefined returns the undefined value.
func jsUndefined() JSValue {
	return JSValueUndefined
}

// jsNull returns the null value.
func jsNull() JSValue {
	return JSValueNull
}

// JSValueEncode encodes a JSValue into an EncodedJSValue (simplified — just returns the JSValue).
func JSValueEncode(v JSValue) JSValue { return v }

// encodedJSValue returns JSValueEncode(jsUndefined()).
func encodedJSValue() JSValue { return JSValueUndefined }

// JSValueDecode decodes an EncodedJSValue (simplified).
func JSValueDecode(v JSValue) JSValue { return v }

// --- Exception helpers ---

// throwVMTypeError throws a TypeError in the VM.
func throwVMTypeError(globalObject *JSGlobalObject, scope ThrowScope, msg ...string) JSValue {
	vm := globalObject.VM()
	errMsg := "TypeError"
	if len(msg) > 0 {
		errMsg = msg[0]
	}
	vm.ThrowException(globalObject, errMsg)
	return JSValueUndefined
}

// throwTypeError throws a TypeError without VM.
func throwTypeError(globalObject *JSGlobalObject, scope ThrowScope, msg string) {
	_ = scope
	globalObject.VM().ThrowException(globalObject, msg)
}

// throwOutOfMemoryError throws an OOM error.
func throwOutOfMemoryError(globalObject *JSGlobalObject, scope ThrowScope) {
	_ = scope
	globalObject.VM().ThrowException(globalObject, "Out of memory")
}

// getVM returns the VM from a JSGlobalObject.
func getVM(globalObject *JSGlobalObject) *VM {
	return globalObject.VM()
}

// --- ECAMMode ---

type ECMAMode uint8

const (
	ECMAModeStrict ECMAMode = iota
	ECMAModeSloppy
)

func (m ECMAMode) Strict() ECMAMode { return ECMAModeStrict }

// --- Slot type for PropertySlot ---

type InternalMethodType uint8

const (
	InternalMethodTypeGet             InternalMethodType = iota
	InternalMethodTypeGetOwnProperty
	InternalMethodTypeHasProperty
)

// --- assert / exception helpers ---

func JS_EXPORT_PRIVATE_ASSERT_STRING(cond bool) {
	if !cond {
		panic("JSC assertion failed")
	}
}

// --- Helper functions ---

// asString returns the JSString from a JSValue (simplified).
func asString(v JSValue) *JSString {
	if v.IsString() {
		return nil // TODO: convert to JSString*
	}
	return nil
}

// asObject returns the JSObject from a JSValue.
func asObject(v JSValue) *JSObject {
	if v.IsObject() {
		return v.GetObject()
	}
	return nil
}

// uncheckedDowncast performs a direct type assertion (no check, like C++ uncheckedDowncast).
func uncheckedDowncast[T any](obj interface{}) *T {
	result, ok := obj.(*T)
	if !ok {
		panic("uncheckedDowncast failed")
	}
	return result
}

// dynamicDowncast performs a safe type assertion with nil on failure.
func dynamicDowncast[T any](obj interface{}) *T {
	result, ok := obj.(*T)
	if !ok {
		return nil
	}
	return result
}

// isJSArray returns true if the given object is a JSArray.
// getCallDataInline returns CallData for a JSValue (bridges to jscell's GetCallData).
func getCallDataInline(value JSValue) CallData {
	if !value.IsCell() {
		return CallData{Type: CallTypeNone}
	}
	if obj := value.GetObject(); obj != nil {
		return GetCallData(&obj.JSCell)
	}
	return CallData{Type: CallTypeNone}
}

// getConstructDataInline returns ConstructData for a JSValue.
func getConstructDataInline(value JSValue) ConstructData {
	if !value.IsCell() {
		return ConstructData{Type: ConstructTypeNone}
	}
	if obj := value.GetObject(); obj != nil {
		return GetConstructData(&obj.JSCell)
	}
	return ConstructData{Type: ConstructTypeNone}
}

// call invokes [[Call]] on a value.
func call(globalObject *JSGlobalObject, value JSValue, callData CallData, thisValue JSValue, args []JSValue) JSValue {
	_ = callData
	if fn := value.AsFunction(); fn != nil {
		result, err := fn.Call(globalObject, thisValue, args)
		if err != nil {
			globalObject.VM().ThrowException(globalObject, err.Error())
			return JSValueUndefined
		}
		return result
	}
	return JSValueUndefined
}

// --- RELEASE_AND_RETURN macro ---

func RELEASE_AND_RETURN(scope ThrowScope, value JSValue) JSValue {
	if scope.vm != nil && scope.vm.Exception != nil {
		return JSValueUndefined
	}
	return value
}

// jsOwnedString creates an owned JS string value.
func jsOwnedString(vm *VM, s string) JSValue {
	_ = vm
	return NewJSValueString(s)
}

// jsOwnedStringFromImpl creates a JSValue from a string implementation.
func jsOwnedStringFromImpl(vm *VM, impl string) JSValue {
	return jsOwnedString(vm, impl)
}

// --- SameValue (Object.is) ---

func sameValue(globalObject *JSGlobalObject, a, b JSValue) bool {
	_ = globalObject
	if a.IsNumber() && b.IsNumber() {
		if a.IsNaN() && b.IsNaN() {
			return true
		}
		return a.ToNumber() == b.ToNumber()
	}
	if a.Tag() != b.Tag() {
		if a.IsNumber() && b.IsNumber() {
			return a.ToNumber() == 0 && b.ToNumber() == 0
		}
		return false
	}
	return a == b
}

// propertyNameFromIndex converts an index to a property name string.
func propertyNameFromIndex(index uint32) string {
	if index == 0 {
		return "0"
	}
	if index == 1 {
		return "1"
	}
	buf := [20]byte{}
	i := len(buf)
	for index > 0 {
		i--
		buf[i] = byte('0' + index%10)
		index /= 10
	}
	return string(buf[i:])
}

