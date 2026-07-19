// CallData corresponds to JSC::CallData (runtime/CallData.h)
package runtime

// CallType corresponds to JSC::CallData::Type.
type CallType uint8

const (
	CallTypeNone   CallType = iota
	CallTypeNative
	CallTypeJS
)

// ConstructType corresponds to JSC::ConstructData::Type.
type ConstructType uint8

const (
	ConstructTypeNone   ConstructType = iota
	ConstructTypeNative
	ConstructTypeJS
)

// CallData corresponds to JSC::CallData.
type CallData struct {
	Type             CallType
	NativeFunction   TaggedNativeFunction
	IsBoundFunction  bool
	IsWasm           bool
	FunctionExecutable *FunctionExecutable
	Scope            *JSScope
}

// NewCallData creates a new CallData.
func NewCallData() CallData {
	return CallData{Type: CallTypeNone}
}

// ConstructData corresponds to JSC::ConstructData.
type ConstructData struct {
	Type                ConstructType
	NativeFunction      TaggedNativeFunction
	FunctionExecutable  *FunctionExecutable
	Scope               *JSScope
}

// NewConstructData creates a new ConstructData.
func NewConstructData() ConstructData {
	return ConstructData{Type: ConstructTypeNone}
}

// TaggedNativeFunction is a native function pointer for CallData.
type TaggedNativeFunction func(globalObject *JSGlobalObject, callFrame *ExecState) JSValue
