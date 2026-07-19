// Translation of: Source/JavaScriptCore/runtime/ObjectConstructor.h
//                  Source/JavaScriptCore/runtime/ObjectConstructor.cpp
//
// ObjectConstructor is the Object() constructor and its static methods.

package runtime

// ObjectConstructor corresponds to JSC::ObjectConstructor.
type ObjectConstructor struct {
	InternalFunction
}

// StructureFlags for ObjectConstructor.
const ObjectConstructorStructureFlags uint32 = InternalFunctionStructureFlags | HasStaticPropertyTable

// NewObjectConstructor creates a new ObjectConstructor.
func NewObjectConstructor(vm *VM, globalObject *JSGlobalObject, structure *Structure, objectPrototype *ObjectPrototype) *ObjectConstructor {
	// In C++: InternalFunction(vm, structure, callObjectConstructor, constructWithObjectConstructor)
	obj := &ObjectConstructor{}
	obj.InternalFunction = InternalFunction{
		JSNonFinalObject: JSNonFinalObject{},
		functionForCall:      callObjectConstructor,
		functionForConstruct: constructWithObjectConstructor,
		globalObject:         globalObject,
	}
	obj.structureID = structure.structureID
	obj.typ = InternalFunctionType
	obj.cellState = DefinitelyWhite
	obj.properties = make(map[string]JSValue)
	obj.finishCreation(vm, globalObject, objectPrototype)
	return obj
}

// finishCreation sets up the ObjectConstructor with its static methods and properties.
func (c *ObjectConstructor) finishCreation(vm *VM, globalObject *JSGlobalObject, objectPrototype *ObjectPrototype) {
	// Base::finishCreation(vm, 1, "Object", WithoutStructureTransition)
	c.InternalFunction.finishCreation(vm, 1, "Object")

	// putDirectWithoutTransition prototype
	c.putDirectWithoutTransition(vm, NewPropertyName("prototype"),
		NewJSValueObject(&objectPrototype.JSNonFinalObject.JSObject),
		PropertyAttributeDontEnum|PropertyAttributeDontDelete|PropertyAttributeReadOnly)

	// Public native functions (simplified — properties set in global object init)
	_ = globalObject
}

// ===== Call/Construct =====

// constructObjectWithNewTarget implements ES 19.1.1.1 Object([value])
func constructObjectWithNewTarget(globalObject *JSGlobalObject, callFrame *ExecState, newTarget JSValue) *JSObject {
	vm := globalObject.VM()
	objectConstructor := uncheckedDowncast[ObjectConstructor](callFrame.Callee())

	// 1. If NewTarget is neither undefined nor the active function
	if !newTarget.IsUndefined() && newTarget != NewJSValueObject(&objectConstructor.JSObject) {
		functionGlobalObject := getFunctionRealm(globalObject, newTarget.GetObject())
		baseStructure := functionGlobalObject.objectStructureForObjectConstructor()
		objectStructure := InternalFunctionCreateSubclassStructure(globalObject, newTarget.GetObject(), baseStructure)
		if objectStructure == nil {
			return nil
		}
		return constructEmptyObject(vm, objectStructure)
	}

	// 2. If value is null, undefined or not supplied, return ObjectCreate(%ObjectPrototype%)
	argument := callFrame.Argument(0)
	if argument.IsUndefinedOrNull() {
		return constructEmptyObject(vm, globalObject.objectStructureForObjectConstructor())
	}

	// 3. Return ToObject(value)
	return argument.ToObject(globalObject)
}

// constructWithObjectConstructor is the [[Construct]] for Object constructor.
func constructWithObjectConstructor(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	return JSValueEncode(NewJSValueObject(constructObjectWithNewTarget(globalObject, callFrame, callFrame.NewTarget())))
}

// callObjectConstructor is the [[Call]] for Object constructor.
func callObjectConstructor(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	return JSValueEncode(NewJSValueObject(constructObjectWithNewTarget(globalObject, callFrame, JSValueUndefined)))
}

// ===== Static methods =====

// objectConstructorGetPrototypeOf implements Object.getPrototypeOf()
func objectConstructorGetPrototypeOf(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	return JSValueEncode(callFrame.Argument(0).GetPrototype(globalObject))
}

// objectConstructorSetPrototypeOf implements Object.setPrototypeOf()
func objectConstructorSetPrototypeOf(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	_ = vm

	objectValue := callFrame.Argument(0)
	if objectValue.IsUndefinedOrNull() {
		return throwVMTypeError(globalObject, ThrowScope{vm: vm}, "Cannot set prototype of undefined or null")
	}

	protoValue := callFrame.Argument(1)
	if !protoValue.IsObject() && !protoValue.IsNull() {
		return throwVMTypeError(globalObject, ThrowScope{vm: vm}, "Prototype value can only be an Object or null")
	}

	object := objectValue.ToObject(globalObject)
	if object == nil {
		return encodedJSValue()
	}

	shouldThrowIfCantSet := true
	object.SetPrototype(globalObject, protoValue, shouldThrowIfCantSet)
	return JSValueEncode(objectValue)
}

// objectConstructorGetOwnPropertyDescriptor is the helper that both public and private paths use.
func objectConstructorGetOwnPropertyDescriptorFunc(globalObject *JSGlobalObject, object *JSObject, propertyName Identifier) JSValue {
	vm := globalObject.VM()
	_ = vm
	descriptor, ok := object.getOwnPropertyDescriptor(globalObject, NewPropertyName(propertyName.String()))
	if !ok {
		return jsUndefined()
	}
	result := constructObjectFromPropertyDescriptor(globalObject, descriptor)
	ASSERT(result != nil)
	return NewJSValueObject(result)
}

// objectConstructorGetOwnPropertyDescriptors returns all own property descriptors.
func objectConstructorGetOwnPropertyDescriptorsFunc(globalObject *JSGlobalObject, object *JSObject) JSValue {
	vm := globalObject.VM()


	properties := object.GetPropertyNames(globalObject, IncludeDontEnumProperties)
	descriptors := constructEmptyObject(vm, nil)
	for _, name := range properties {
		propName := NewPropertyName(name)
		desc, ok := object.getOwnPropertyDescriptor(globalObject, propName)
		if !ok {
			continue
		}
		fromDesc := constructObjectFromPropertyDescriptor(globalObject, desc)
		descriptors.putOwnDataPropertyMayBeIndex(globalObject, propName, NewJSValueObject(fromDesc), PutPropertySlot{})
	}
	return NewJSValueObject(descriptors)
}


func objectConstructorDefineProperty(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	_ = vm

	if !callFrame.Argument(0).IsObject() {
		return throwVMTypeError(globalObject, ThrowScope{vm: vm}, "Properties can only be defined on Objects.")
	}
	obj := asObject(callFrame.Argument(0))
	propertyName, _ := callFrame.Argument(1).ToPropertyKey(globalObject)

	descriptor, _ := toPropertyDescriptor(globalObject, callFrame.Argument(2))
	obj.defineOwnProperty(globalObject, NewPropertyName(propertyName.String()), descriptor, true)
	return JSValueEncode(NewJSValueObject(obj))
}

// objectConstructorCreate implements Object.create()
func objectConstructorCreate(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	_ = vm

	proto := callFrame.Argument(0)
	if !proto.IsObject() && !proto.IsNull() {
		return throwVMTypeError(globalObject, ThrowScope{vm: vm}, "Object prototype may only be an Object or null.")
	}
	var newObject *JSObject
	if proto.IsObject() {
		newObject = constructEmptyObjectWithGlobal(globalObject, asObject(proto), 0)
	} else {
		newObject = constructEmptyObject(vm, nil)
	}
	if callFrame.Argument(1).IsUndefined() {
		return JSValueEncode(NewJSValueObject(newObject))
	}

	properties := callFrame.Argument(1).ToObject(globalObject)
	defineProperties(globalObject, newObject, properties)
	return JSValueEncode(NewJSValueObject(newObject))
}

// objectConstructorKeys implements Object.keys()
func objectConstructorKeys(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	_ = vm
	object := callFrame.Argument(0).ToObject(globalObject)
	if object == nil {
		return encodedJSValue()
	}
	result := ownPropertyKeys(globalObject, object, PropertyNameModeStrings, ExcludeDontEnumProperties)
	return JSValueEncode(result)
}

// objectConstructorGetOwnPropertyNames implements Object.getOwnPropertyNames()
func objectConstructorGetOwnPropertyNames(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	_ = vm
	object := callFrame.Argument(0).ToObject(globalObject)
	if object == nil {
		return encodedJSValue()
	}
	result := ownPropertyKeys(globalObject, object, PropertyNameModeStrings, IncludeDontEnumProperties)
	return JSValueEncode(result)
}

// objectConstructorGetOwnPropertySymbols implements Object.getOwnPropertySymbols()
func objectConstructorGetOwnPropertySymbols(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	_ = vm
	object := callFrame.Argument(0).ToObject(globalObject)
	if object == nil {
		return encodedJSValue()
	}
	result := ownPropertyKeys(globalObject, object, PropertyNameModeSymbols, IncludeDontEnumProperties)
	return JSValueEncode(result)
}

// objectConstructorSeal implements Object.seal()
func objectConstructorSealFunc(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	_ = vm
	obj := callFrame.Argument(0)
	if !obj.IsObject() {
		return JSValueEncode(obj)
	}
	object := asObject(obj)
	object.seal(vm)
	return JSValueEncode(NewJSValueObject(object))
}

// objectConstructorFreeze implements Object.freeze()
func objectConstructorFreezeFunc(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	_ = vm
	obj := callFrame.Argument(0)
	if !obj.IsObject() {
		return JSValueEncode(obj)
	}
	object := asObject(obj)
	object.freeze(vm)
	return JSValueEncode(NewJSValueObject(object))
}

// objectConstructorPreventExtensions implements Object.preventExtensions()
func objectConstructorPreventExtensions(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	_ = vm
	argument := callFrame.Argument(0)
	if !argument.IsObject() {
		return JSValueEncode(argument)
	}
	object := asObject(argument)
	object.PreventExtensions(globalObject)
	return JSValueEncode(NewJSValueObject(object))
}

// objectConstructorIsSealed implements Object.isSealed()
func objectConstructorIsSealed(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	obj := callFrame.Argument(0)
	if !obj.IsObject() {
		return JSValueEncode(jsBoolean(true))
	}
	object := asObject(obj)
	return JSValueEncode(jsBoolean(object.isSealed(globalObject.VM())))
}

// objectConstructorIsFrozen implements Object.isFrozen()
func objectConstructorIsFrozen(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	obj := callFrame.Argument(0)
	if !obj.IsObject() {
		return JSValueEncode(jsBoolean(true))
	}
	object := asObject(obj)
	return JSValueEncode(jsBoolean(object.isFrozen(globalObject.VM())))
}

// objectConstructorIsExtensible implements Object.isExtensible()
func objectConstructorIsExtensible(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	obj := callFrame.Argument(0)
	if !obj.IsObject() {
		return JSValueEncode(jsBoolean(false))
	}
	object := asObject(obj)
	return JSValueEncode(jsBoolean(object.IsExtensible(globalObject)))
}

// objectConstructorIs implements Object.is()
func objectConstructorIs(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	return JSValueEncode(jsBoolean(sameValue(globalObject, callFrame.Argument(0), callFrame.Argument(1))))
}

// objectConstructorHasOwn implements Object.hasOwn()
func objectConstructorHasOwn(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	base := callFrame.Argument(0).ToObject(globalObject)
	if base == nil {
		return encodedJSValue()
	}
	propertyName, _ := callFrame.Argument(1).ToPropertyKey(globalObject)
	result := objectPrototypeHasOwnProperty(globalObject, base, NewPropertyName(propertyName.String()))
	return JSValueEncode(jsBoolean(result))
}

// ===== Helper functions =====

// ownPropertyKeys enumerates own property keys based on mode.
func ownPropertyKeys(globalObject *JSGlobalObject, object *JSObject, propertyNameMode PropertyNameMode, dontEnumMode DontEnumPropertiesMode) JSValue {
	vm := globalObject.VM()
	names := object.GetPropertyNames(globalObject, dontEnumMode)
	keys := make([]JSValue, 0, len(names))
	for _, name := range names {
		if propertyNameMode == PropertyNameModeSymbols {
			continue
		}
		keys = append(keys, jsOwnedString(vm, name))
	}
	array := NewJSArray(vm, globalObject)
	for _, k := range keys {
		array.elements = append(array.elements, k)
	}
	return NewJSValueObject(&array.JSObject)
}



// defineProperties implements Object.defineProperties().
func defineProperties(globalObject *JSGlobalObject, object *JSObject, properties *JSObject) JSValue {
	_ = globalObject
	// Simplified: copy all properties
	for name, val := range properties.enumerateProperties() {
		object.properties[name] = val
	}
	return NewJSValueObject(object)
}

// toPropertyDescriptor converts a JSValue to a PropertyDescriptor.
func toPropertyDescriptor(globalObject *JSGlobalObject, in JSValue) (PropertyDescriptor, bool) {
	desc := NewPropertyDescriptor()

	if !in.IsObject() {
		return desc, false
	}
	description := asObject(in)

	// Read standard property descriptor fields
	if val := description.getIfPropertyExists(globalObject, NewPropertyName("enumerable")); val.IsNotUndefined() {
		desc.SetEnumerable(val.ToBoolean())
	}
	if val := description.getIfPropertyExists(globalObject, NewPropertyName("configurable")); val.IsNotUndefined() {
		desc.SetConfigurable(val.ToBoolean())
	}
	if val := description.getIfPropertyExists(globalObject, NewPropertyName("value")); val.IsNotUndefined() {
		desc.SetValue(val)
	}
	if val := description.getIfPropertyExists(globalObject, NewPropertyName("writable")); val.IsNotUndefined() {
		desc.SetWritable(val.ToBoolean())
	}
	if val := description.getIfPropertyExists(globalObject, NewPropertyName("get")); val.IsNotUndefined() {
		if !val.IsUndefined() && !val.IsCallable() {
			return desc, false
		}
		desc.SetGetter(val)
	}
	if val := description.getIfPropertyExists(globalObject, NewPropertyName("set")); val.IsNotUndefined() {
		if !val.IsUndefined() && !val.IsCallable() {
			return desc, false
		}
		desc.SetSetter(val)
	}

	// Validate accessor descriptor
	if desc.IsAccessorDescriptor() {
		if desc.Value().IsNotUndefined() {
			return desc, false
		}
		if desc.WritablePresent() {
			return desc, false
		}
	}

	return desc, true
}
// constructObjectFromPropertyDescriptor creates a JSObject from a PropertyDescriptor.
func constructObjectFromPropertyDescriptor(globalObject *JSGlobalObject, descriptor PropertyDescriptor) *JSObject {
	vm := getVM(globalObject)
	result := constructEmptyObject(vm, nil)

	if descriptor.Value().IsNotUndefined() {
		result.PutDirect(vm, NewPropertyName("value"), descriptor.Value(), 0)
	}
	if descriptor.WritablePresent() {
		result.PutDirect(vm, NewPropertyName("writable"), jsBoolean(descriptor.Writable()), 0)
	}
	if descriptor.GetterPresent() {
		result.PutDirect(vm, NewPropertyName("get"), descriptor.Getter(), 0)
	}
	if descriptor.SetterPresent() {
		result.PutDirect(vm, NewPropertyName("set"), descriptor.Setter(), 0)
	}
	if descriptor.EnumerablePresent() {
		result.PutDirect(vm, NewPropertyName("enumerable"), jsBoolean(descriptor.Enumerable()), 0)
	}
	if descriptor.ConfigurablePresent() {
		result.PutDirect(vm, NewPropertyName("configurable"), jsBoolean(descriptor.Configurable()), 0)
	}
	return result
}


// objectAssignGeneric implements the core Object.assign() logic.



// objectAssignGeneric implements the core Object.assign() logic.
func objectAssignGeneric(globalObject *JSGlobalObject, vm *VM, target *JSObject, source *JSObject) {
	_ = vm
	_ = globalObject
	// Simplified: copy all enumerable own properties
	for name, val := range source.enumerateProperties() {
		target.properties[name] = val
	}
}

// getFunctionRealm returns the realm (global object) of a function.
func getFunctionRealm(globalObject *JSGlobalObject, obj *JSObject) *JSGlobalObject {
	_ = globalObject
	_ = obj
	// Simplified: return the globalObject
	return globalObject
}

// InternalFunctionCreateSubclassStructure creates a subclass structure.
func InternalFunctionCreateSubclassStructure(globalObject *JSGlobalObject, newTarget *JSObject, baseStructure *Structure) *Structure {
	_ = globalObject
	_ = newTarget
	return baseStructure
}

// constructEmptyObject creates an empty JSObject with a given structure or default.
func constructEmptyObject(vm *VM, structure *Structure) *JSObject {
	if structure == nil {
		typeInfo := NewTypeInfo(ObjectType, 0)
		structure = NewStructure(vm, nil, JSValueNull, typeInfo, &ClassInfo{})
	}
	return NewJSObject(vm, structure)
}

// constructEmptyObjectWithGlobal creates an empty object using a global object.
func constructEmptyObjectWithGlobal(globalObject *JSGlobalObject, prototype *JSObject, inlineCapacity uint32) *JSObject {
	_ = inlineCapacity
	vm := getVM(globalObject)
	typeInfo := NewTypeInfo(ObjectType, 0)
	var proto JSValue
	if prototype != nil {
		proto = NewJSValueObject(prototype)
	} else {
		proto = JSValueNull
	}
	structure := NewStructure(vm, globalObject, proto, typeInfo, &ClassInfo{})
	return NewJSObject(vm, structure)
}

// constructEmptyArray creates an empty array.
func constructEmptyArray(globalObject *JSGlobalObject, prototype *JSObject) *JSArray {
	_ = prototype
	vm := getVM(globalObject)
	array := NewJSArray(vm, globalObject)
	typeInfo := NewTypeInfo(ArrayType, 0)
	structure := NewStructure(vm, globalObject, JSValueNull, typeInfo, &ClassInfo{})
	array.structureID = structure.structureID
	return array
}
// jsNumber creates a number JSValue.
func jsNumber(n float64) JSValue {
	return NewJSValueNumber(n)
}
