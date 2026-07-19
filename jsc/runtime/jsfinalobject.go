// JSFinalObject corresponds to JSC::JSFinalObject (runtime/JSFinalObject.h)
package runtime

// JSFinalObject corresponds to JSC::JSFinalObject.
// It is a JSObject with a fixed inline capacity for properties.
type JSFinalObject struct {
	JSObject
}

// NewJSFinalObject creates a new JSFinalObject.
func NewJSFinalObject(vm *VM, structure *Structure) *JSFinalObject {
	obj := &JSFinalObject{}
	obj.structureID = structure.structureID
	obj.typ = FinalObjectType
	obj.cellState = DefinitelyWhite
	obj.properties = make(map[string]JSValue)
	_ = vm
	return obj
}
