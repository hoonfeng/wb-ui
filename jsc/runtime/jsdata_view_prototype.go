// Translation of: Source/JavaScriptCore/runtime/JSDataViewPrototype.h/.cpp
package runtime

// JSDataViewPrototype corresponds to JSC::JSDataViewPrototype.
type JSDataViewPrototype struct {
	JSNonFinalObject
}

const JSDataViewPrototypeStructureFlags uint32 = JSNonFinalObjectStructureFlags

func NewJSDataViewPrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *JSDataViewPrototype {
	p := &JSDataViewPrototype{}
	p.structureID = structure.structureID
	p.typ = structure.Type()
	p.cellState = 1
	p.properties = make(map[string]JSValue)
	return p
}

func (p *JSDataViewPrototype) FinishCreation(vm *VM) {
	// Register prototype properties (methods registered in globalObject init)
	p.properties["constructor"] = JSValueUndefined // set by caller
	p.properties["buffer"] = JSValueUndefined       // getter
	p.properties["byteLength"] = JSValueUndefined   // getter
	p.properties["byteOffset"] = JSValueUndefined   // getter
}
