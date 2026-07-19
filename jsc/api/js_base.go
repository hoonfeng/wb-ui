package api

// JSBase defines the fundamental types for the JavaScriptCore C API.

// JSContextGroupRef represents a group of JavaScript contexts.
type JSContextGroupRef uintptr

// JSContextRef represents a JavaScript execution context.
type JSContextRef uintptr

// JSGlobalContextRef represents a global JavaScript execution context.
type JSGlobalContextRef uintptr

// JSStringRef represents a string in the JavaScriptCore API.
type JSStringRef uintptr

// JSClassRef represents a JavaScript class definition.
type JSClassRef uintptr

// JSPropertyNameArrayRef represents an array of property names.
type JSPropertyNameArrayRef uintptr

// JSPropertyNameAccumulatorRef represents an accumulator for property names.
type JSPropertyNameAccumulatorRef uintptr

// JSValueRef represents a JavaScript value.
type JSValueRef uintptr

// JSObjectRef represents a JavaScript object.
type JSObjectRef uintptr

// JSObjectPropertyAttributes describes the attributes of a property.
type JSObjectPropertyAttributes uint32

const (
	JSObjectPropertyAttributeNone         JSObjectPropertyAttributes = 0
	JSObjectPropertyAttributeReadOnly     JSObjectPropertyAttributes = 1 << 1
	JSObjectPropertyAttributeDontEnum     JSObjectPropertyAttributes = 1 << 2
	JSObjectPropertyAttributeDontDelete   JSObjectPropertyAttributes = 1 << 3
)

// JSType describes the type of a JavaScript value.
type JSType uint32

const (
	kJSTypeUndefined JSType = 0
	kJSTypeNull      JSType = 1
	kJSTypeBoolean   JSType = 2
	kJSTypeNumber    JSType = 3
	kJSTypeString    JSType = 4
	kJSTypeObject    JSType = 5
	kJSTypeSymbol    JSType = 6
	kJSTypeBigInt    JSType = 7
)
