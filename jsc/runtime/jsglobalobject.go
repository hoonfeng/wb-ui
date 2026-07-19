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

	// Structure for SymbolObject (wrapper for Symbol values).
	symbolObjectStructure *Structure
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
	}
	return JSValueUndefined
}

// objectProtoToStringFunction returns the Object.prototype.toString function.
func (g *JSGlobalObject) objectProtoToStringFunction() JSValue {
	fn := NewJSFunction(g.vm, g, "toString", 0, func(globalObject *JSGlobalObject, thisValue JSValue, args []JSValue) (JSValue, error) {
		callFrame := NewExecState(g.vm)
		callFrame.SetThisValue(thisValue)
		if len(args) > 0 {
			callFrame.SetArguments(args)
		}
		result := objectProtoFuncToString(globalObject, callFrame)
		return result, nil
	})
	return NewJSValueObject(&fn.JSObject)
}
// objectStructureForObjectConstructor returns the Structure used when constructing
// a plain Object via the Object() constructor.
func (g *JSGlobalObject) objectStructureForObjectConstructor() *Structure {
	typeInfo := NewTypeInfo(ObjectType, 0)
	return NewStructure(g.vm, g, NewJSValueObject(&g.JSObject), typeInfo, &ClassInfo{})
}

// arrayStructureForIndexingTypeDuringAllocation returns the array structure for the given indexing type.
func (g *JSGlobalObject) arrayStructureForIndexingTypeDuringAllocation(indexingType IndexingType) *Structure {
	_ = indexingType
	typeInfo := NewTypeInfo(ArrayType, 0)
	return NewStructure(g.vm, g, NewJSValueObject(&g.JSObject), typeInfo, &ClassInfo{})
}

// originalArrayStructureForIndexingType returns the original array structure.
func (g *JSGlobalObject) originalArrayStructureForIndexingType(indexingType IndexingType) *Structure {
	return g.arrayStructureForIndexingTypeDuringAllocation(indexingType)
}

// isHavingABadTime returns whether the VM is having a bad time (Array transition).
func (g *JSGlobalObject) isHavingABadTime() bool {
	return false // simplified
}

// Type returns the JSType.
func (g *JSGlobalObject) Type() JSType { return GlobalObjectType }

// ErrorStructure returns the structure for the given error type.
func (g *JSGlobalObject) ErrorStructure(errType ErrorType) *Structure {
	_ = errType
	// Simplified - returns a basic Structure with ObjectType
	typeInfo := NewTypeInfo(ObjectType, 0)
	return NewStructure(g.vm, g, NewJSValueObject(&g.JSObject), typeInfo, &ClassInfo{})
}

// booleanObjectStructure returns the structure for BooleanObject.
func (g *JSGlobalObject) booleanObjectStructure() *Structure {
	typeInfo := NewTypeInfo(ObjectType, 0)
	return NewStructure(g.vm, g, NewJSValueObject(&g.JSObject), typeInfo, &ClassInfo{})
}

// numberObjectStructure returns the structure for NumberObject.
func (g *JSGlobalObject) numberObjectStructure() *Structure {
	typeInfo := NewTypeInfo(ObjectType, 0)
	return NewStructure(g.vm, g, NewJSValueObject(&g.JSObject), typeInfo, &ClassInfo{})
}

// ToString returns "[object global]".

// ToString returns "[object global]".
func (g *JSGlobalObject) ToString() string {
	return "[object global]"
}
