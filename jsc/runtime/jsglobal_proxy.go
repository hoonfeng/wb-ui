// Copyright (C) 2011-2023 Apple Inc. All rights reserved.
// Translated to Go by the jsc-translator.

package runtime

// JSGlobalProxy corresponds to JSC::JSGlobalProxy.
// Proxies access to the global object (used for cross-window access).
type JSGlobalProxy struct {
	JSNonFinalObject
	m_target *JSGlobalObject
}

const GlobalProxyStructureFlags uint32 = JSNonFinalObjectStructureFlags | OverridesGetOwnPropertySlot | OverridesGetOwnPropertyNames | OverridesPut | OverridesGetPrototype | OverridesIsExtensible | InterceptsGetOwnPropertySlotByIndexEvenWhenLengthIsNotZero

func NewJSGlobalProxy(vm *VM, structure *Structure, target *JSGlobalObject) *JSGlobalProxy {
	proxy := &JSGlobalProxy{
		m_target: target,
	}
	proxy.structureID = structure.structureID
	proxy.typ = GlobalProxyType
	proxy.cellState = DefinitelyWhite
	proxy.properties = make(map[string]JSValue)
	return proxy
}

func (p *JSGlobalProxy) Target() *JSGlobalObject { return p.m_target }
func (p *JSGlobalProxy) SetTarget(vm *VM, target *JSGlobalObject) {
	p.m_target = target
}
