// JSScope corresponds to JSC::JSScope (runtime/JSScope.h)
package runtime

// JSScope corresponds to JSC::JSScope — the scope chain object.
type JSScope struct {
	JSObject
}

// NewJSScope creates a new JSScope.
func NewJSScope(vm *VM, structure *Structure) *JSScope {
	scope := &JSScope{}
	scope.structureID = structure.structureID
	scope.typ = ObjectType // will be refined per subclass
	scope.cellState = DefinitelyWhite
	scope.properties = make(map[string]JSValue)
	_ = vm
	return scope
}

// JSLexicalEnvironment corresponds to JSC::JSLexicalEnvironment.
type JSLexicalEnvironment struct {
	JSScope
}

// JSModuleEnvironment corresponds to JSC::JSModuleEnvironment.
type JSModuleEnvironment struct {
	JSLexicalEnvironment
}

// JSGlobalLexicalEnvironment corresponds to JSC::JSGlobalLexicalEnvironment.
type JSGlobalLexicalEnvironment struct {
	JSScope
}

// StrictEvalActivation corresponds to JSC::StrictEvalActivation.
type StrictEvalActivation struct {
	JSScope
}

// WithScope corresponds to JSC::JSWithScope.
type WithScope struct {
	JSScope
}

// JSModuleRecord corresponds to JSC::JSModuleRecord.
type JSModuleRecord struct {
	JSCell
}

// JSModuleNamespaceObject corresponds to JSC::JSModuleNamespaceObject.
type JSModuleNamespaceObject struct {
	JSObject
}

// JSModuleLoader corresponds to JSC::JSModuleLoader.
type JSModuleLoader struct {
	JSObject
}

// ShadowRealmObject corresponds to JSC::ShadowRealmObject.
type ShadowRealmObject struct {
	JSObject
}
