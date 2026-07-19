// Translation of: Source/JavaScriptCore/runtime/JSArrayBufferPrototype.h/.cpp
package runtime

// JSArrayBufferPrototype corresponds to JSC::JSArrayBufferPrototype.
type JSArrayBufferPrototype struct {
	JSNonFinalObject
}

const JSArrayBufferPrototypeStructureFlags uint32 = JSNonFinalObjectStructureFlags

func NewJSArrayBufferPrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *JSArrayBufferPrototype {
	p := &JSArrayBufferPrototype{}
	p.structureID = structure.structureID
	p.typ = structure.Type()
	p.cellState = 1
	p.properties = make(map[string]JSValue)
	return p
}

func (p *JSArrayBufferPrototype) FinishCreation(vm *VM) {
	p.properties["constructor"] = JSValueUndefined
	p.properties["byteLength"] = JSValueUndefined
}

// ArrayBufferProtoFuncByteLength implements get ArrayBuffer.prototype.byteLength.
func ArrayBufferProtoFuncByteLength(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	return JSValueUndefined, nil
}

// ArrayBufferProtoFuncSlice implements ArrayBuffer.prototype.slice(begin, end).
func ArrayBufferProtoFuncSlice(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	return JSValueUndefined, nil
}

// ArrayBufferProtoFuncIsView implements ArrayBuffer.isView(obj).
func ArrayBufferProtoFuncIsView(globalObject *JSGlobalObject, callFrame *ExecState) (JSValue, error) {
	return NewJSValueBool(false), nil
}
