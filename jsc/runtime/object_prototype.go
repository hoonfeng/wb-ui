// Translation of: Source/JavaScriptCore/runtime/ObjectPrototype.h
//                  Source/JavaScriptCore/runtime/ObjectPrototype.cpp
//
// ObjectPrototype is the prototype for all JavaScript objects (Object.prototype).

package runtime

// ObjectPrototype corresponds to JSC::ObjectPrototype.
// Implements methods inherited by all JS objects.
type ObjectPrototype struct {
	JSNonFinalObject
}

// StructureFlags for ObjectPrototype.
const ObjectPrototypeStructureFlags uint32 = JSNonFinalObjectStructureFlags | IsImmutablePrototypeExoticObject

// NewObjectPrototype creates a new ObjectPrototype.
func NewObjectPrototype(vm *VM, globalObject *JSGlobalObject, structure *Structure) *ObjectPrototype {
	proto := &ObjectPrototype{}
	proto.finishCreation(vm, globalObject)
	return proto
}

// finishCreation registers all Object.prototype methods.
func (o *ObjectPrototype) finishCreation(vm *VM, globalObject *JSGlobalObject) {
	o.JSNonFinalObject.finishCreation(vm)
	ASSERT(o.inherits(JSObjectClassInfo()))

	o.putDirectWithoutTransition(vm, NewPropertyName("toString"), globalObject.objectProtoToStringFunction(), PropertyAttributeDontEnum)
	// TODO: add JSC_NATIVE_FUNCTION equivalents for:
	// toLocaleString, valueOf, hasOwnProperty, propertyIsEnumerable,
	// isPrototypeOf, __defineGetter__, __defineSetter__, __lookupGetter__, __lookupSetter__
	_ = vm
}

// CreateObjectPrototypeStructure creates the Structure for ObjectPrototype.
func CreateObjectPrototypeStructure(vm *VM, globalObject *JSGlobalObject, prototype JSValue) *Structure {
	typeInfo := NewTypeInfo(ObjectType, ObjectPrototypeStructureFlags)
	classInfo := JSObjectClassInfo()
	return NewStructure(vm, globalObject, prototype, typeInfo, classInfo)
}

// ===== Host functions =====

// objectProtoFuncValueOf implements Object.prototype.valueOf()
func objectProtoFuncValueOf(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	valueObj := thisValue.ToObject(globalObject)
	if valueObj == nil {
	return EncodedJSValue()
	}
	valueObj.auditStructureID()
	return JSValueEncode(NewJSValueObject(valueObj))
}

// objectPrototypeHasOwnProperty checks if the object has an own property.
func objectPrototypeHasOwnProperty(globalObject *JSGlobalObject, thisObject *JSObject, propertyName PropertyName) bool {
	vm := globalObject.VM()
	_ = vm
	// Simplified: check own properties
	return thisObject.hasOwnProperty(globalObject, propertyName)
}

// objectProtoFuncHasOwnProperty implements Object.prototype.hasOwnProperty()
func objectProtoFuncHasOwnProperty(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	_ = vm

	base := callFrame.ThisValue()
	subscript := callFrame.Argument(0)

	// Convert subscript to property key
	propertyKey, err := subscript.ToPropertyKey(globalObject)
	if err != nil {
		return JSValueEncode(jsBoolean(false))
	}

	thisObject := base.ToThis(globalObject, ECMAModeStrict).ToObject(globalObject)
	if thisObject == nil {
	return EncodedJSValue()
	}

	result := objectPrototypeHasOwnProperty(globalObject, thisObject, NewPropertyName(propertyKey.String()))
	return JSValueEncode(jsBoolean(result))
}

// objectProtoFuncIsPrototypeOf implements Object.prototype.isPrototypeOf()
func objectProtoFuncIsPrototypeOf(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	_ = vm

	if !callFrame.Argument(0).IsObject() {
		return JSValueEncode(jsBoolean(false))
	}

	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	thisObj := thisValue.ToObject(globalObject)
	if thisObj == nil {
	return EncodedJSValue()
	}

	v := callFrame.Argument(0).GetObject().GetPrototype(globalObject)

	for {
		if !v.IsObject() {
			return JSValueEncode(jsBoolean(false))
		}
		if v == NewJSValueObject(thisObj) {
			return JSValueEncode(jsBoolean(true))
		}
		v = v.GetObject().GetPrototype(globalObject)
	}
}

// objectProtoFuncDefineGetter implements Object.prototype.__defineGetter__()
func objectProtoFuncDefineGetter(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	_ = vm

	thisObject := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict).ToObject(globalObject)
	if thisObject == nil {
	return EncodedJSValue()
	}

	get := callFrame.Argument(1)
	if !get.IsCallable() {
		return throwVMTypeError(globalObject, ThrowScope{vm: vm}, "invalid getter usage")
	}

	propertyName, err := callFrame.Argument(0).ToPropertyKey(globalObject)
	if err != nil {
	return EncodedJSValue()
	}

	descriptor := NewPropertyDescriptor()
	descriptor.SetGetter(get)
	descriptor.SetEnumerable(true)
	descriptor.SetConfigurable(true)

	shouldThrow := true
	thisObject.defineOwnProperty(globalObject, NewPropertyName(propertyName.String()), descriptor, shouldThrow)

	return JSValueEncode(jsUndefined())
}

// objectProtoFuncDefineSetter implements Object.prototype.__defineSetter__()
func objectProtoFuncDefineSetter(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	_ = vm

	thisObject := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict).ToObject(globalObject)
	if thisObject == nil {
	return EncodedJSValue()
	}

	set := callFrame.Argument(1)
	if !set.IsCallable() {
		return throwVMTypeError(globalObject, ThrowScope{vm: vm}, "invalid setter usage")
	}

	propertyName, err := callFrame.Argument(0).ToPropertyKey(globalObject)
	if err != nil {
	return EncodedJSValue()
	}

	descriptor := NewPropertyDescriptor()
	descriptor.SetSetter(set)
	descriptor.SetEnumerable(true)
	descriptor.SetConfigurable(true)

	shouldThrow := true
	thisObject.defineOwnProperty(globalObject, NewPropertyName(propertyName.String()), descriptor, shouldThrow)

	return JSValueEncode(jsUndefined())
}

// objectProtoFuncPropertyIsEnumerable implements Object.prototype.propertyIsEnumerable()
func objectProtoFuncPropertyIsEnumerable(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	_ = vm

	propertyName, err := callFrame.Argument(0).ToPropertyKey(globalObject)
	if err != nil {
	return EncodedJSValue()
	}

	thisObject := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict).ToObject(globalObject)
	if thisObject == nil {
	return EncodedJSValue()
	}

	descriptor, ok := thisObject.getOwnPropertyDescriptor(globalObject, NewPropertyName(propertyName.String()))
	if !ok {
		return JSValueEncode(jsBoolean(false))
	}
	return JSValueEncode(jsBoolean(descriptor.Enumerable()))
}

// objectProtoFuncToLocaleString implements Object.prototype.toLocaleString()
func objectProtoFuncToLocaleString(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	vm := globalObject.VM()
	_ = vm

	// 1. Let V be the this value.
	thisValue := callFrame.ThisValue()

	// 2. Let O be ToObject(V).
	object := thisValue.ToThis(globalObject, ECMAModeStrict).ToObject(globalObject)
	if object == nil {
	return EncodedJSValue()
	}

	// 3. Let toString be O.[[Get]]("toString", V)
	toString := object.Get(globalObject, NewPropertyName("toString"))

	// 4. If IsCallable(toString) is false, throw TypeError.
	if !toString.IsCallable() {
		return throwVMTypeError(globalObject, ThrowScope{vm: vm})
	}

	// 5. Return Call(toString, V).
	callData := getCallDataInline(toString)
	return RELEASE_AND_RETURN(ThrowScope{vm: vm}, call(globalObject, toString, callData, thisValue, nil))
}

// objectProtoFuncToString implements Object.prototype.toString()
func objectProtoFuncToString(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	thisValue := callFrame.ThisValue().ToThis(globalObject, ECMAModeStrict)
	return JSValueEncode(objectPrototypeToString(globalObject, thisValue))
}

// objectPrototypeToString is the core [[ToString]] implementation for Object.prototype.toString.
func objectPrototypeToString(globalObject *JSGlobalObject, thisValue JSValue) JSValue {
	_ = globalObject
	if thisValue.IsUndefined() {
		return NewJSValueString("[object Undefined]")
	}
	if thisValue.IsNull() {
		return NewJSValueString("[object Null]")
	}
	if thisValue.IsObject() {
		obj := thisValue.GetObject()
		className := "[object "
		if obj != nil {
			className += obj.ClassName(globalObject)
		} else {
			className += "Object"
		}
		className += "]"
		return NewJSValueString(className)
	}
	// For primitives, wrap and get class name
	obj := thisValue.ToObject(globalObject)
	if obj == nil {
		return NewJSValueString("[object Object]")
	}
	return NewJSValueString("[object " + obj.ClassName(globalObject) + "]")
}
