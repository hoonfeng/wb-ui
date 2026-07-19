package api

// JSObjectRef API provides functions for working with JavaScript objects.

// JSObjectMake creates a new JavaScript object.
func JSObjectMake(ctx JSContextRef, jsClass JSClassRef, data uintptr) JSObjectRef {
	_ = ctx
	_ = jsClass
	_ = data
	return 0
}

// JSObjectMakeFunctionWithCallback creates a function with a native callback.
func JSObjectMakeFunctionWithCallback(ctx JSContextRef, name JSStringRef, callAsFunction func(ctx JSContextRef, function JSObjectRef, thisObject JSObjectRef, argumentCount uintptr, arguments *JSValueRef, exception *JSValueRef) JSValueRef) JSObjectRef {
	_ = ctx
	_ = name
	_ = callAsFunction
	return 0
}

// JSObjectMakeConstructor creates a constructor function.
func JSObjectMakeConstructor(ctx JSContextRef, jsClass JSClassRef, callAsConstructor func(ctx JSContextRef, constructor JSObjectRef, argumentCount uintptr, arguments *JSValueRef, exception *JSValueRef) JSObjectRef) JSObjectRef {
	_ = ctx
	_ = jsClass
	_ = callAsConstructor
	return 0
}

// JSObjectMakeArray creates a JavaScript array.
func JSObjectMakeArray(ctx JSContextRef, argumentCount uintptr, arguments *JSValueRef, exception *JSValueRef) JSObjectRef {
	_ = ctx
	_ = argumentCount
	_ = arguments
	_ = exception
	return 0
}

// JSObjectMakeDate creates a JavaScript Date.
func JSObjectMakeDate(ctx JSContextRef, argumentCount uintptr, arguments *JSValueRef, exception *JSValueRef) JSObjectRef {
	_ = ctx
	_ = argumentCount
	_ = arguments
	_ = exception
	return 0
}

// JSObjectMakeError creates a JavaScript Error.
func JSObjectMakeError(ctx JSContextRef, argumentCount uintptr, arguments *JSValueRef, exception *JSValueRef) JSObjectRef {
	_ = ctx
	_ = argumentCount
	_ = arguments
	_ = exception
	return 0
}

// JSObjectMakeRegExp creates a JavaScript RegExp.
func JSObjectMakeRegExp(ctx JSContextRef, argumentCount uintptr, arguments *JSValueRef, exception *JSValueRef) JSObjectRef {
	_ = ctx
	_ = argumentCount
	_ = arguments
	_ = exception
	return 0
}

// JSObjectGetPrototype returns the prototype of an object.
func JSObjectGetPrototype(ctx JSContextRef, object JSObjectRef) JSValueRef {
	_ = ctx
	_ = object
	return 0
}

// JSObjectSetPrototype sets the prototype of an object.
func JSObjectSetPrototype(ctx JSContextRef, object JSObjectRef, value JSValueRef) {
	_ = ctx
	_ = object
	_ = value
}

// JSObjectHasProperty checks if an object has a property.
func JSObjectHasProperty(ctx JSContextRef, object JSObjectRef, propertyName JSStringRef) bool {
	_ = ctx
	_ = object
	_ = propertyName
	return false
}

// JSObjectGetProperty gets a property value.
func JSObjectGetProperty(ctx JSContextRef, object JSObjectRef, propertyName JSStringRef, exception *JSValueRef) JSValueRef {
	_ = ctx
	_ = object
	_ = propertyName
	_ = exception
	return 0
}

// JSObjectSetProperty sets a property value.
func JSObjectSetProperty(ctx JSContextRef, object JSObjectRef, propertyName JSStringRef, value JSValueRef, attributes JSObjectPropertyAttributes, exception *JSValueRef) bool {
	_ = ctx
	_ = object
	_ = propertyName
	_ = value
	_ = attributes
	_ = exception
	return true
}

// JSObjectDeleteProperty deletes a property.
func JSObjectDeleteProperty(ctx JSContextRef, object JSObjectRef, propertyName JSStringRef, exception *JSValueRef) bool {
	_ = ctx
	_ = object
	_ = propertyName
	_ = exception
	return true
}

// JSObjectGetPropertyAtIndex gets a property at a numeric index.
func JSObjectGetPropertyAtIndex(ctx JSContextRef, object JSObjectRef, propertyIndex uint32, exception *JSValueRef) JSValueRef {
	_ = ctx
	_ = object
	_ = propertyIndex
	_ = exception
	return 0
}

// JSObjectSetPropertyAtIndex sets a property at a numeric index.
func JSObjectSetPropertyAtIndex(ctx JSContextRef, object JSObjectRef, propertyIndex uint32, value JSValueRef, exception *JSValueRef) {
	_ = ctx
	_ = object
	_ = propertyIndex
	_ = value
	_ = exception
}

// JSObjectCopyPropertyNames returns the property names of an object.
func JSObjectCopyPropertyNames(ctx JSContextRef, object JSObjectRef) JSPropertyNameArrayRef {
	_ = ctx
	_ = object
	return 0
}

// JSObjectIsFunction checks if an object is a function.
func JSObjectIsFunction(ctx JSContextRef, object JSObjectRef) bool {
	_ = ctx
	_ = object
	return false
}

// JSObjectCallAsFunction calls an object as a function.
func JSObjectCallAsFunction(ctx JSContextRef, object JSObjectRef, thisObject JSObjectRef, argumentCount uintptr, arguments *JSValueRef, exception *JSValueRef) JSValueRef {
	_ = ctx
	_ = object
	_ = thisObject
	_ = argumentCount
	_ = arguments
	_ = exception
	return 0
}

// JSObjectIsConstructor checks if an object is a constructor.
func JSObjectIsConstructor(ctx JSContextRef, object JSObjectRef) bool {
	_ = ctx
	_ = object
	return false
}

// JSObjectCallAsConstructor calls an object as a constructor.
func JSObjectCallAsConstructor(ctx JSContextRef, object JSObjectRef, argumentCount uintptr, arguments *JSValueRef, exception *JSValueRef) JSObjectRef {
	_ = ctx
	_ = object
	_ = argumentCount
	_ = arguments
	_ = exception
	return 0
}

// JSObjectCopyPropertyNamesArray gets the number of property names.
func JSPropertyNameArrayGetCount(array JSPropertyNameArrayRef) uintptr {
	_ = array
	return 0
}

// JSPropertyNameArrayGetNameAtIndex gets a property name at an index.
func JSPropertyNameArrayGetNameAtIndex(array JSPropertyNameArrayRef, index uintptr) JSStringRef {
	_ = array
	_ = index
	return 0
}

// JSPropertyNameArrayRelease releases a property name array.
func JSPropertyNameArrayRelease(array JSPropertyNameArrayRef) {
	_ = array
}
