// Translation of: Source/JavaScriptCore/runtime/JSGlobalObject.h
//                  Source/JavaScriptCore/runtime/JSGlobalObject.cpp
//
// JSGlobalObject is the global object for a JavaScript context (window/globalThis).

package runtime

// JSGlobalObject corresponds to JSC::JSGlobalObject. It is the top-level
// execution context for JavaScript code, holding the global scope, prototype
// chain roots, and built-in constructor/ prototype objects.
type JSGlobalObject struct {
	JSObject
	// VM reference
	vm *VM

	// Global scope
	globalScope *JSScope

	// eval cache
	evalEnabled bool
}

// StructureFlags for JSGlobalObject.
const JSGlobalObjectStructureFlags uint32 = JSObjectStructureFlags

// NewJSGlobalObject creates a new global object.
func NewJSGlobalObject(vm *VM, structure *Structure) *JSGlobalObject {
	global := &JSGlobalObject{
		vm: vm,
	}
	global.structureID = structure.structureID
	global.typ = GlobalObjectType
	global.cellState = DefinitelyWhite
	global.properties = make(map[string]JSValue)
	global.evalEnabled = true

	_ = structure
	return global
}

// VM returns the associated VM.
func (g *JSGlobalObject) VM() *VM { return g.vm }

// SetVM sets the VM reference.
func (g *JSGlobalObject) SetVM(vm *VM) { g.vm = vm }

// GlobalScope returns the global scope.
func (g *JSGlobalObject) GlobalScope() *JSScope { return g.globalScope }

// SetGlobalScope sets the global scope.
func (g *JSGlobalObject) SetGlobalScope(scope *JSScope) { g.globalScope = scope }

// EvalEnabled returns whether eval() is enabled.
func (g *JSGlobalObject) EvalEnabled() bool { return g.evalEnabled }

// SetEvalEnabled enables or disables eval().
func (g *JSGlobalObject) SetEvalEnabled(enabled bool) { g.evalEnabled = enabled }

// GetMethod retrieves a method (callable property) by name.
func (g *JSGlobalObject) GetMethod(globalObject *JSGlobalObject, name PropertyName) JSValue {
	val := g.Get(globalObject, name)
	if val.IsFunction() {
		return val
	}
	return JSValueUndefined
}

// Type returns the JSType.
func (g *JSGlobalObject) Type() JSType { return GlobalObjectType }

// ToString returns "[object global]".
func (g *JSGlobalObject) ToString() string {
	return "[object global]"
}
