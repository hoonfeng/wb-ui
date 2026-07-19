// Translation of: Source/JavaScriptCore/runtime/ProxyObject.h
//                  Source/JavaScriptCore/runtime/ProxyObject.cpp
//
// ProxyObject implements the ES6 Proxy exotic object with all 13 traps.

package runtime

import (
	"fmt"
	"unsafe"
)

func proxyFromObject(obj *JSObject) *ProxyObject {
	return (*ProxyObject)(unsafe.Pointer(obj))
}

type ProxyObject struct {
	JSObject
	m_target          JSValue
	m_handler         JSValue
	m_isCallable      bool
	m_isConstructible bool
}

const ProxyObjectStructureFlags uint32 = JSObjectStructureFlags | OverridesGetOwnPropertySlot | OverridesGetOwnPropertyNames | OverridesGetPrototype | OverridesGetCallData | OverridesPut | OverridesIsExtensible | InterceptsGetOwnPropertySlotByIndexEvenWhenLengthIsNotZero | ProhibitsPropertyCaching

const sProxyAlreadyRevokedErrorMessage = "Proxy has already been revoked. No more operations are allowed to be performed on it"

func NewProxyObject(globalObject *JSGlobalObject, target, handler JSValue) *ProxyObject {
	vm := globalObject.VM()
	structure := structureForProxyTarget(globalObject, target)
	proxy := &ProxyObject{m_target: target, m_handler: handler}
	proxy.structureID = structure.structureID
	proxy.typ = ProxyObjectType
	proxy.cellState = DefinitelyWhite
	proxy.properties = make(map[string]JSValue)
	proxy.finishCreation(vm, globalObject, target, handler)
	return proxy
}

func (p *ProxyObject) finishCreation(vm *VM, globalObject *JSGlobalObject, target, handler JSValue) {
	if !target.IsObject() {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: vm}, "A Proxy's 'target' should be an Object")
		return
	}
	if !handler.IsObject() {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: vm}, "A Proxy's 'handler' should be an Object")
		return
	}
	p.m_isCallable = target.IsCallable()
	p.m_isConstructible = IsConstructor(target)
	p.m_target = target
	p.m_handler = handler
}

func (p *ProxyObject) Target() JSValue  { return p.m_target }
func (p *ProxyObject) Handler() JSValue { return p.m_handler }

func structureForProxyTarget(globalObject *JSGlobalObject, target JSValue) *Structure {
	if target.IsCallable() {
		return globalObject.callableProxyObjectStructure()
	}
	return globalObject.proxyObjectStructure()
}

func (p *ProxyObject) getHandlerTrap(globalObject *JSGlobalObject, handler *JSObject, trapName string) JSValue {
	vm := globalObject.VM()
	propName := NewPropertyName(trapName)
	val := handler.getIfPropertyExists(globalObject, propName)
	if val.IsUndefinedOrNull() {
		return JSValueUndefined
	}
	if !val.IsCallable() {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: vm},
			fmt.Sprintf("'%s' property of a Proxy's handler should be callable", trapName))
		return JSValueUndefined
	}
	return val
}

func callProxyHandler(globalObject *JSGlobalObject, fn JSValue, receiver *JSObject, args []JSValue) JSValue {
	callData := getCallDataInline(fn)
	if callData.Type == CallTypeNone {
		return JSValueUndefined
	}
	return call(globalObject, fn, callData, NewJSValueObject(receiver), args)
}

// ==================== [[GetPrototypeOf]] (ES 9.5.1) ====================

func (p *ProxyObject) performGetPrototype(globalObject *JSGlobalObject) JSValue {
	vm := globalObject.VM()
	if p.m_handler.IsNull() {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: vm}, sProxyAlreadyRevokedErrorMessage)
		return JSValueUndefined
	}
	handler := asObject(p.m_handler)
	trapFn := p.getHandlerTrap(globalObject, handler, "getPrototypeOf")
	if trapFn.IsUndefined() {
		return p.m_target.GetPrototype(globalObject)
	}
	args := []JSValue{p.m_target}
	trapResult := callProxyHandler(globalObject, trapFn, handler, args)
	if !trapResult.IsObject() && !trapResult.IsNull() {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: vm},
			"Proxy handler's 'getPrototypeOf' trap should either return an object or null")
		return JSValueUndefined
	}
	targetObj := asObject(p.m_target)
	if !targetObj.IsExtensible(globalObject) {
		targetProto := targetObj.GetPrototype(globalObject)
		if !sameValue(globalObject, targetProto, trapResult) {
			_ = throwVMTypeError(globalObject, ThrowScope{vm: vm},
				"Proxy's 'getPrototypeOf' trap for a non-extensible target should return the same value")
		}
	}
	return trapResult
}

func getPrototypeProxy(obj *JSObject, globalObject *JSGlobalObject) JSValue {
	return proxyFromObject(obj).performGetPrototype(globalObject)
}

// ==================== [[SetPrototypeOf]] (ES 9.5.2) ====================

func (p *ProxyObject) performSetPrototype(globalObject *JSGlobalObject, prototype JSValue, shouldThrowIfCantSet bool) bool {
	vm := globalObject.VM()
	if p.m_handler.IsNull() {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: vm}, sProxyAlreadyRevokedErrorMessage)
		return false
	}
	handler := asObject(p.m_handler)
	trapFn := p.getHandlerTrap(globalObject, handler, "setPrototypeOf")
	if trapFn.IsUndefined() {
		return asObject(p.m_target).SetPrototype(globalObject, prototype, shouldThrowIfCantSet)
	}
	args := []JSValue{p.m_target, prototype}
	trapResult := callProxyHandler(globalObject, trapFn, handler, args)
	if !trapResult.ToBoolean() {
		if shouldThrowIfCantSet {
			_ = throwVMTypeError(globalObject, ThrowScope{vm: vm},
				"Proxy 'setPrototypeOf' returned false indicating it could not set the prototype value")
		}
		return false
	}
	if !asObject(p.m_target).IsExtensible(globalObject) {
		targetPrototype := asObject(p.m_target).GetPrototype(globalObject)
		if !sameValue(globalObject, prototype, targetPrototype) {
			_ = throwVMTypeError(globalObject, ThrowScope{vm: vm},
				"Proxy 'setPrototypeOf' trap returned true when its target is non-extensible")
			return false
		}
	}
	return true
}

func setPrototypeProxy(obj *JSObject, globalObject *JSGlobalObject, prototype JSValue, shouldThrowIfCantSet bool) bool {
	return proxyFromObject(obj).performSetPrototype(globalObject, prototype, shouldThrowIfCantSet)
}

// ==================== [[IsExtensible]] (ES 9.5.3) ====================

func (p *ProxyObject) performIsExtensible(globalObject *JSGlobalObject) bool {
	vm := globalObject.VM()
	if p.m_handler.IsNull() {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: vm}, sProxyAlreadyRevokedErrorMessage)
		return false
	}
	handler := asObject(p.m_handler)
	trapFn := p.getHandlerTrap(globalObject, handler, "isExtensible")
	if trapFn.IsUndefined() {
		return asObject(p.m_target).IsExtensible(globalObject)
	}
	args := []JSValue{p.m_target}
	trapResult := callProxyHandler(globalObject, trapFn, handler, args)
	result := trapResult.ToBoolean()
	targetIsExtensible := asObject(p.m_target).IsExtensible(globalObject)
	if result != targetIsExtensible {
		if targetIsExtensible {
			_ = throwVMTypeError(globalObject, ThrowScope{vm: vm},
				"Proxy object's 'isExtensible' trap returned false when the target is extensible")
		} else {
			_ = throwVMTypeError(globalObject, ThrowScope{vm: vm},
				"Proxy object's 'isExtensible' trap returned true when the target is non-extensible")
		}
	}
	return result
}

func isExtensibleProxy(obj *JSObject, globalObject *JSGlobalObject) bool {
	return proxyFromObject(obj).performIsExtensible(globalObject)
}

// ==================== [[PreventExtensions]] (ES 9.5.4) ====================

func (p *ProxyObject) performPreventExtensions(globalObject *JSGlobalObject) bool {
	vm := globalObject.VM()
	if p.m_handler.IsNull() {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: vm}, sProxyAlreadyRevokedErrorMessage)
		return false
	}
	handler := asObject(p.m_handler)
	trapFn := p.getHandlerTrap(globalObject, handler, "preventExtensions")
	if trapFn.IsUndefined() {
		return asObject(p.m_target).PreventExtensions(globalObject)
	}
	args := []JSValue{p.m_target}
	trapResult := callProxyHandler(globalObject, trapFn, handler, args)
	result := trapResult.ToBoolean()
	if result && asObject(p.m_target).IsExtensible(globalObject) {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: vm},
			"Proxy's 'preventExtensions' trap returned true even though its target is extensible")
		return false
	}
	return result
}

func preventExtensionsProxy(obj *JSObject, globalObject *JSGlobalObject) bool {
	return proxyFromObject(obj).performPreventExtensions(globalObject)
}

// ==================== [[GetOwnProperty]] (ES 9.5.5) ====================

func (p *ProxyObject) performGetOwnPropertyDescriptor(globalObject *JSGlobalObject, propertyName PropertyName) (PropertyDescriptor, bool) {
	vm := globalObject.VM()
	if p.m_handler.IsNull() {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: vm}, sProxyAlreadyRevokedErrorMessage)
		return PropertyDescriptor{}, false
	}
	handler := asObject(p.m_handler)
	trapFn := p.getHandlerTrap(globalObject, handler, "getOwnPropertyDescriptor")
	if trapFn.IsUndefined() {
		return asObject(p.m_target).getOwnPropertyDescriptor(globalObject, propertyName)
	}
	args := []JSValue{p.m_target, NewJSValueString(propertyName.String())}
	trapResult := callProxyHandler(globalObject, trapFn, handler, args)
	if !trapResult.IsObject() && !trapResult.IsUndefined() {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: vm},
			"result of 'getOwnPropertyDescriptor' call should either be an Object or undefined")
		return PropertyDescriptor{}, false
	}
	if trapResult.IsUndefined() {
		targetObj := asObject(p.m_target)
		_, ok := targetObj.getOwnPropertyDescriptor(globalObject, propertyName)
		if ok {
			if !targetObj.IsExtensible(globalObject) {
				_ = throwVMTypeError(globalObject, ThrowScope{vm: vm},
					"When 'getOwnPropertyDescriptor' returns undefined, the 'target' of a Proxy should be extensible")
			}
		}
		return PropertyDescriptor{}, false
	}
	targetObj := asObject(p.m_target)
	targetIsExtensible := targetObj.IsExtensible(globalObject)
	trapDesc, _ := toPropertyDescriptor(globalObject, trapResult)
	completePropertyDescriptor(&trapDesc)
	targetDesc, targetHasDesc := targetObj.getOwnPropertyDescriptor(globalObject, propertyName)
	trapDesc.Configurable()
	targetDesc.Configurable()
	_ = targetIsExtensible
	_ = targetHasDesc
	return trapDesc, true
}

func completePropertyDescriptor(desc *PropertyDescriptor) {
	if desc.IsAccessorDescriptor() {
		if !desc.GetterPresent() {
			desc.SetGetter(JSValueUndefined)
		}
		if !desc.SetterPresent() {
			desc.SetSetter(JSValueUndefined)
		}
	} else {
		if !desc.Value().IsValid() {
			desc.SetValue(JSValueUndefined)
		}
		if !desc.WritablePresent() {
			desc.SetWritable(false)
		}
	}
	if !desc.EnumerablePresent() {
		desc.SetEnumerable(false)
	}
	if !desc.ConfigurablePresent() {
		desc.SetConfigurable(false)
	}
}

// ==================== [[DefineOwnProperty]] (ES 9.5.6) ====================

func (p *ProxyObject) performDefineOwnProperty(globalObject *JSGlobalObject, propertyName PropertyName, descriptor PropertyDescriptor, shouldThrow bool) bool {
	vm := globalObject.VM()
	if p.m_handler.IsNull() {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: vm}, sProxyAlreadyRevokedErrorMessage)
		return false
	}
	handler := asObject(p.m_handler)
	trapFn := p.getHandlerTrap(globalObject, handler, "defineProperty")
	if trapFn.IsUndefined() {
		return asObject(p.m_target).defineOwnProperty(globalObject, propertyName, descriptor, shouldThrow)
	}
	descObj := constructObjectFromPropertyDescriptor(globalObject, descriptor)
	args := []JSValue{p.m_target, NewJSValueString(propertyName.String()), NewJSValueObject(descObj)}
	trapResult := callProxyHandler(globalObject, trapFn, handler, args)
	if !trapResult.ToBoolean() {
		if shouldThrow {
			_ = throwVMTypeError(globalObject, ThrowScope{vm: vm},
				fmt.Sprintf("Proxy's 'defineProperty' trap returned falsy value for property '%s'", propertyName.String()))
		}
		return false
	}
	_ = descObj
	return true
}

// ==================== [[HasProperty]] (ES 9.5.7) ====================

func (p *ProxyObject) performHasProperty(globalObject *JSGlobalObject, propertyName PropertyName) bool {
	vm := globalObject.VM()
	if p.m_handler.IsNull() {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: vm}, sProxyAlreadyRevokedErrorMessage)
		return false
	}
	handler := asObject(p.m_handler)
	trapFn := p.getHandlerTrap(globalObject, handler, "has")
	if trapFn.IsUndefined() {
		return asObject(p.m_target).HasProperty(globalObject, propertyName)
	}
	args := []JSValue{p.m_target, NewJSValueString(propertyName.String())}
	trapResult := callProxyHandler(globalObject, trapFn, handler, args)
	if trapResult.ToBoolean() {
		return true
	}
	validateNegativeHasTrapResult(globalObject, asObject(p.m_target), propertyName.String())
	return false
}

func verifyHasPropertyProxy(obj *JSObject, globalObject *JSGlobalObject, propertyName *PropertyName) bool {
	return proxyFromObject(obj).performHasProperty(globalObject, *propertyName)
}

func validateNegativeHasTrapResult(globalObject *JSGlobalObject, target *JSObject, propertyName string) {
	desc, ok := target.getOwnPropertyDescriptor(globalObject, NewPropertyName(propertyName))
	if ok {
		if !desc.Configurable() {
			_ = throwVMTypeError(globalObject, ThrowScope{vm: globalObject.VM()},
				"Proxy 'has' must return 'true' for non-configurable properties")
		} else if !target.IsExtensible(globalObject) {
			_ = throwVMTypeError(globalObject, ThrowScope{vm: globalObject.VM()},
				"Proxy 'has' must return 'true' for a non-extensible 'target' object")
		}
	}
}

// ==================== [[Get]] (ES 9.5.8) ====================

func (p *ProxyObject) performGet(globalObject *JSGlobalObject, propertyName PropertyName, receiver JSValue) JSValue {
	vm := globalObject.VM()
	if p.m_handler.IsNull() {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: vm}, sProxyAlreadyRevokedErrorMessage)
		return JSValueUndefined
	}
	handler := asObject(p.m_handler)
	trapFn := p.getHandlerTrap(globalObject, handler, "get")
	if trapFn.IsUndefined() {
		targetObj := asObject(p.m_target)
		var slot PropertySlot
		if targetObj.getPropertySlot(globalObject, propertyName, &slot) {
			return slot.Value
		}
		return JSValueUndefined
	}
	args := []JSValue{p.m_target, NewJSValueString(propertyName.String()), receiver}
	trapResult := callProxyHandler(globalObject, trapFn, handler, args)
	return trapResult
}

func getOwnPropertySlotProxy(obj *JSObject, globalObject *JSGlobalObject, propertyName *PropertyName, slot *PropertySlot) bool {
	proxy := proxyFromObject(obj)
	result := proxy.performGet(globalObject, *propertyName, NewJSValueObject(obj))
	slot.Value = result
	return result.IsValid()
}

func getOwnPropertySlotByIndexProxy(obj *JSObject, globalObject *JSGlobalObject, index uint32, slot *PropertySlot) bool {
	proxy := proxyFromObject(obj)
	propName := NewPropertyName(fmt.Sprintf("%d", index))
	result := proxy.performGet(globalObject, propName, NewJSValueObject(obj))
	slot.Value = result
	return result.IsValid()
}

// ==================== [[Set]] (ES 9.5.9) ====================

func (p *ProxyObject) performPut(globalObject *JSGlobalObject, putValue, thisValue JSValue, propertyName PropertyName, shouldThrow bool) bool {
	vm := globalObject.VM()
	if p.m_handler.IsNull() {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: vm}, sProxyAlreadyRevokedErrorMessage)
		return false
	}
	handler := asObject(p.m_handler)
	trapFn := p.getHandlerTrap(globalObject, handler, "set")
	if trapFn.IsUndefined() {
		targetObj := asObject(p.m_target)
		cell := (*JSCell)(unsafe.Pointer(targetObj))
		var ps PutPropertySlot
		return targetObj.Put(cell, globalObject, propertyName, putValue, &ps)
	}
	args := []JSValue{p.m_target, NewJSValueString(propertyName.String()), putValue, thisValue}
	trapResult := callProxyHandler(globalObject, trapFn, handler, args)
	if !trapResult.ToBoolean() {
		if shouldThrow {
			_ = throwVMTypeError(globalObject, ThrowScope{vm: vm},
				fmt.Sprintf("Proxy object's 'set' trap returned falsy value for property '%s'", propertyName.String()))
		}
		return false
	}
	validatePositiveSetTrapResult(globalObject, asObject(p.m_target), propertyName.String(), putValue)
	return true
}

func validatePositiveSetTrapResult(globalObject *JSGlobalObject, target *JSObject, propertyName string, putValue JSValue) {
	desc, ok := target.getOwnPropertyDescriptor(globalObject, NewPropertyName(propertyName))
	if ok && !desc.Configurable() {
		if desc.IsDataDescriptor() && !desc.Writable() {
			if !sameValue(globalObject, desc.Value(), putValue) {
				_ = throwVMTypeError(globalObject, ThrowScope{vm: globalObject.VM()},
					"Proxy handler's 'set' on a non-configurable and non-writable property on 'target' should either return false or be the same value already on the 'target'")
			}
		} else if desc.IsAccessorDescriptor() && desc.Setter().IsUndefined() {
			_ = throwVMTypeError(globalObject, ThrowScope{vm: globalObject.VM()},
				"Proxy handler's 'set' method on a non-configurable accessor property without a setter should return false")
		}
	}
}

func putProxy(cell *JSCell, globalObject *JSGlobalObject, propertyName *PropertyName, value JSValue, slot *PutPropertySlot) bool {
	proxy := proxyFromObject(AsObject(cell))
	return proxy.performPut(globalObject, value, NewJSValueObject(AsObject(cell)), *propertyName, slot != nil)
}

func putByIndexProxy(cell *JSCell, globalObject *JSGlobalObject, propertyName uint32, value JSValue, shouldThrow bool) bool {
	proxy := proxyFromObject(AsObject(cell))
	ident := NewPropertyName(fmt.Sprintf("%d", propertyName))
	return proxy.performPut(globalObject, value, NewJSValueObject(AsObject(cell)), ident, shouldThrow)
}

// ==================== [[Delete]] (ES 9.5.10) ====================

func (p *ProxyObject) performDelete(globalObject *JSGlobalObject, propertyName PropertyName) bool {
	vm := globalObject.VM()
	if p.m_handler.IsNull() {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: vm}, sProxyAlreadyRevokedErrorMessage)
		return false
	}
	handler := asObject(p.m_handler)
	trapFn := p.getHandlerTrap(globalObject, handler, "deleteProperty")
	if trapFn.IsUndefined() {
		targetObj := asObject(p.m_target)
		cell := (*JSCell)(unsafe.Pointer(targetObj))
		var ds DeletePropertySlot
		return targetObj.DeleteProperty(cell, globalObject, propertyName, &ds)
	}
	args := []JSValue{p.m_target, NewJSValueString(propertyName.String())}
	trapResult := callProxyHandler(globalObject, trapFn, handler, args)
	if !trapResult.ToBoolean() {
		return false
	}
	return true
}

func deletePropertyProxy(cell *JSCell, globalObject *JSGlobalObject, propertyName *PropertyName) bool {
	return proxyFromObject(AsObject(cell)).performDelete(globalObject, *propertyName)
}

func deletePropertyByIndexProxy(cell *JSCell, globalObject *JSGlobalObject, propertyName uint32) bool {
	proxy := proxyFromObject(AsObject(cell))
	ident := NewPropertyName(fmt.Sprintf("%d", propertyName))
	return proxy.performDelete(globalObject, ident)
}

// ==================== [[OwnPropertyKeys]] (ES 9.5.11) ====================

func (p *ProxyObject) performOwnKeys(globalObject *JSGlobalObject) []string {
	vm := globalObject.VM()
	if p.m_handler.IsNull() {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: vm}, sProxyAlreadyRevokedErrorMessage)
		return nil
	}
	handler := asObject(p.m_handler)
	trapFn := p.getHandlerTrap(globalObject, handler, "ownKeys")
	if trapFn.IsUndefined() {
		return asObject(p.m_target).GetPropertyNames(globalObject, IncludeDontEnumProperties)
	}
	args := []JSValue{p.m_target}
	trapResult := callProxyHandler(globalObject, trapFn, handler, args)
	if !trapResult.IsObject() {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: vm},
			"Proxy handler's 'ownKeys' method must return an object")
		return nil
	}
	resultObj := trapResult.ToObject(globalObject)
	keys := make([]string, 0)
	seen := make(map[string]bool)
	for k := range resultObj.properties {
		if seen[k] {
			_ = throwVMTypeError(globalObject, ThrowScope{vm: vm},
				"Proxy handler's 'ownKeys' trap result must not contain any duplicate names")
			return nil
		}
		seen[k] = true
		keys = append(keys, k)
	}
	_ = vm
	return keys
}

func getOwnPropertyNamesProxy(obj *JSObject, globalObject *JSGlobalObject) []string {
	return proxyFromObject(obj).performOwnKeys(globalObject)
}

// ==================== [[Call]] ====================

func performProxyCall(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	proxy := proxyFromObject(callFrame.Callee())
	if proxy.m_handler.IsNull() {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: globalObject.VM()}, sProxyAlreadyRevokedErrorMessage)
		return JSValueUndefined
	}
	handler := asObject(proxy.m_handler)
	trapFn := proxy.getHandlerTrap(globalObject, handler, "apply")
	if trapFn.IsUndefined() {
		result, _ := proxy.m_target.ToObject(globalObject).Call(globalObject, callFrame.ThisValue(), callFrame.Arguments())
		return result
	}
	argArray := callFrame.Arguments()
	argsArr := NewJSArray(globalObject.VM(), globalObject)
	for _, a := range argArray {
		argsArr.elements = append(argsArr.elements, a)
	}
	callArgs := []JSValue{proxy.m_target, callFrame.ThisValue(), NewJSValueObject(&argsArr.JSObject)}
	return callProxyHandler(globalObject, trapFn, handler, callArgs)
}

func getCallDataProxy(cell *JSCell) CallData {
	proxy := proxyFromObject(AsObject(cell))
	if proxy.m_isCallable {
		return CallData{Type: CallTypeNative, NativeFunction: performProxyCall}
	}
	return CallData{Type: CallTypeNone}
}

// ==================== [[Construct]] ====================

func performProxyConstruct(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	proxy := proxyFromObject(callFrame.Callee())
	if proxy.m_handler.IsNull() {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: globalObject.VM()}, sProxyAlreadyRevokedErrorMessage)
		return JSValueUndefined
	}
	handler := asObject(proxy.m_handler)
	trapFn := proxy.getHandlerTrap(globalObject, handler, "construct")
	if trapFn.IsUndefined() {
		result, _ := proxy.m_target.ToObject(globalObject).Construct(globalObject, callFrame.Arguments())
		return result
	}
	argArray := callFrame.Arguments()
	argsArr := NewJSArray(globalObject.VM(), globalObject)
	for _, a := range argArray {
		argsArr.elements = append(argsArr.elements, a)
	}
	callArgs := []JSValue{proxy.m_target, NewJSValueObject(&argsArr.JSObject), callFrame.NewTarget()}
	result := callProxyHandler(globalObject, trapFn, handler, callArgs)
	if !result.IsObject() {
		_ = throwVMTypeError(globalObject, ThrowScope{vm: globalObject.VM()},
			"Result from Proxy handler's 'construct' method should be an object")
		return JSValueUndefined
	}
	return result
}

func getConstructDataProxy(cell *JSCell) CallData {
	proxy := proxyFromObject(AsObject(cell))
	if proxy.m_isConstructible {
		return CallData{Type: CallTypeNative, NativeFunction: performProxyConstruct}
	}
	return CallData{Type: CallTypeNone}
}

// ==================== HasInstance ====================

func hasInstanceProxy(cell *JSCell, globalObject *JSGlobalObject, value JSValue) bool {
	proxy := proxyFromObject(AsObject(cell))
	targetObj := proxy.m_target.ToObject(globalObject)
	return ordinaryHasInstance(globalObject, targetObj, value)
}

func ordinaryHasInstance(globalObject *JSGlobalObject, c *JSObject, o JSValue) bool {
	if !o.IsObject() {
		return false
	}
	oObj := o.ToObject(globalObject)
	proto := c.Get(globalObject, NewPropertyName("prototype"))
	if proto.IsUndefinedOrNull() {
		return false
	}
	current := oObj.GetPrototype(globalObject)
	for current.IsObject() {
		if sameValue(globalObject, current, proto) {
			return true
		}
		obj := current.ToObject(globalObject)
		if obj == nil {
			return false
		}
		current = obj.GetPrototype(globalObject)
	}
	return false
}

func (p *ProxyObject) seal(vm *VM) {
	asObject(p.m_target).seal(vm)
}

func (p *ProxyObject) freeze(vm *VM) {
	asObject(p.m_target).freeze(vm)
}

func (p *ProxyObject) Revoke(vm *VM) {
	p.m_handler = JSValueNull
}

func (p *ProxyObject) IsRevoked() bool {
	return p.m_handler.IsNull()
}

// ==================== Helpers ====================

func validateAndApplyPropertyDescriptor(globalObject *JSGlobalObject, obj *JSObject, propertyName string,
	isExtensible bool, desc PropertyDescriptor, isCurrentDefined bool, current PropertyDescriptor, throwErr bool) bool {
	_ = globalObject
	_ = obj
	_ = propertyName
	if !isCurrentDefined {
		if !isExtensible {
			if throwErr {
				_ = throwVMTypeError(globalObject, ThrowScope{vm: globalObject.VM()},
					"Cannot define property: object is not extensible")
			}
			return false
		}
		return true
	}
	return isCompatiblePropertyDescriptor(desc, current)
}

func isCompatiblePropertyDescriptor(desc, current PropertyDescriptor) bool {
	if !current.Configurable() {
		if desc.ConfigurablePresent() && desc.Configurable() {
			return false
		}
		if desc.EnumerablePresent() && desc.Enumerable() != current.Enumerable() {
			return false
		}
	}
	return true
}

func IsConstructor(val JSValue) bool {
	if !val.IsObject() {
		return false
	}
	obj := val.ToObject(nil)
	if obj == nil {
		return false
	}
	callData := getConstructDataInline(NewJSValueObject(obj))
	return callData.Type != ConstructTypeNone
}
