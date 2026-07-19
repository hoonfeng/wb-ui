// Copyright (C) 2014-2023 Apple Inc. All rights reserved.
// Translated to Go by the jsc-translator.

package runtime

// JSCallee corresponds to JSC::JSCallee.
// Base class for call-frame callee objects, holding the scope chain.
type JSCallee struct {
	JSNonFinalObject
	m_scope *JSScope
}

const JSCalleeStructureFlags uint32 = JSNonFinalObjectStructureFlags | ImplementsHasInstance | ImplementsDefaultHasInstance

func NewJSCallee(vm *VM, globalObject *JSGlobalObject, scope *JSScope) *JSCallee {
	callee := &JSCallee{
		m_scope: scope,
	}
	callee.structureID = globalObject.calleeStructure().structureID
	callee.typ = JSCalleeType
	callee.cellState = DefinitelyWhite
	callee.properties = make(map[string]JSValue)
	return callee
}

func (c *JSCallee) Scope() *JSScope          { return c.m_scope }
func (c *JSCallee) SetScope(vm *VM, s *JSScope) {
	c.m_scope = s
}
