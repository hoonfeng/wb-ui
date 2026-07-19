// JSNonFinalObject corresponds to JSC::JSNonFinalObject (runtime/JSNonFinalObject.h)
package runtime

// JSNonFinalObject corresponds to JSC::JSNonFinalObject.
// It is JSObject without finalization support — the common base for prototype objects.
type JSNonFinalObject struct {
	JSObject
}

// NewJSNonFinalObject creates a new JSNonFinalObject.
func NewJSNonFinalObject(vm *VM, structure *Structure) *JSNonFinalObject {
	obj := &JSNonFinalObject{}
	obj.structureID = structure.structureID
	obj.typ = ObjectType
	obj.cellState = DefinitelyWhite
	obj.properties = make(map[string]JSValue)
	_ = vm
	return obj
}

// FinishCreation completes the initialization of a JSNonFinalObject.
func (o *JSNonFinalObject) FinishCreation(vm *VM) {
	_ = vm
	// In JSC, this sets up the object's structure and finalization
	// In Go translation, this is a no-op for JSNonFinalObject
}
