// Forward declarations for JSC types without dedicated files.
// Types with own files (VM, JSValue, JSCell, JSObject, JSFunction, JSArray,
// JSString, JSGlobalObject, Structure, etc.) are NOT duplicated here.

package runtime

// Heap manages GC memory.
type Heap struct{}

// ClassInfo holds metadata about a JS class.
type ClassInfo struct{}

// --- Value types (no own file yet) ---

type JSBigInt struct{ JSCell }
type Symbol struct{ JSCell }
type GetterSetter struct{ JSCell }
type CustomGetterSetter struct{ JSCell }
type NativeExecutable struct{ JSCell }
type CodeBlock struct{ JSCell }

// --- Object types (no own file yet) ---

type JSFinalObject struct{ JSObject }
type InternalFunction struct{ JSObject }
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
