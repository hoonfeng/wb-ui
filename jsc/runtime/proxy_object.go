// Copyright (C) 2016-2022 Apple Inc. All rights reserved.
// Translated to Go by the jsc-translator.

package runtime

// ProxyObject corresponds to JSC::ProxyObject.
// Implements the ES6 Proxy exotic object.
type ProxyObject struct {
	JSObject
	m_target  JSValue
	m_handler JSValue
}

const ProxyObjectStructureFlags uint32 = JSObjectStructureFlags | OverridesGetOwnPropertySlot | OverridesGetOwnPropertyNames | OverridesGetPrototype | OverridesGetCallData | OverridesPut | OverridesIsExtensible | InterceptsGetOwnPropertySlotByIndexEvenWhenLengthIsNotZero | ProhibitsPropertyCaching

func NewProxyObject(globalObject *JSGlobalObject, target JSValue, handler JSValue) *ProxyObject {
	vm := globalObject.VM()
	proxy := &ProxyObject{
		m_target:  target,
		m_handler: handler,
	}
	proxy.structureID = 0
	proxy.typ = ProxyObjectType
	proxy.cellState = DefinitelyWhite
	proxy.properties = make(map[string]JSValue)
	_ = vm
	return proxy
}

func (p *ProxyObject) Target() JSValue  { return p.m_target }
func (p *ProxyObject) Handler() JSValue { return p.m_handler }
