package api

// JSClassDefinition defines a JavaScript class.
type JSClassDefinition struct {
	Version            uint32
	Attributes         JSClassAttributes
	ClassName          string
	ParentClass        JSClassRef
	StaticValues       *JSStaticValue
	StaticFunctions    *JSStaticFunction
	Initialize         func(ctx JSContextRef, object JSObjectRef)
	Finalize           func(object JSObjectRef)
	HasProperty        func(ctx JSContextRef, object JSObjectRef, propertyName JSStringRef) bool
	GetProperty        func(ctx JSContextRef, object JSObjectRef, propertyName JSStringRef, exception *JSValueRef) JSValueRef
	SetProperty        func(ctx JSContextRef, object JSObjectRef, propertyName JSStringRef, value JSValueRef, exception *JSValueRef) bool
	DeleteProperty     func(ctx JSContextRef, object JSObjectRef, propertyName JSStringRef, exception *JSValueRef) bool
	GetPropertyNames   func(ctx JSContextRef, object JSObjectRef, accumulator JSPropertyNameAccumulatorRef)
	CallAsFunction     func(ctx JSContextRef, function JSObjectRef, thisObject JSObjectRef, argumentCount uintptr, arguments *JSValueRef, exception *JSValueRef) JSValueRef
	CallAsConstructor  func(ctx JSContextRef, constructor JSObjectRef, argumentCount uintptr, arguments *JSValueRef, exception *JSValueRef) JSObjectRef
	HasInstance        func(ctx JSContextRef, constructor JSObjectRef, possibleInstance JSValueRef, exception *JSValueRef) bool
	ConvertToType      func(ctx JSContextRef, value JSValueRef, typ JSType, exception *JSValueRef) JSValueRef
}

// JSClassAttributes defines attributes for a JavaScript class.
type JSClassAttributes uint32

const (
	JSClassAttributeNone JSClassAttributes = 0
)

// JSStaticValue defines a static value in a JavaScript class.
type JSStaticValue struct {
	Name       string
	GetProperty func(ctx JSContextRef, object JSObjectRef, propertyName JSStringRef, exception *JSValueRef) JSValueRef
	SetProperty func(ctx JSContextRef, object JSObjectRef, propertyName JSStringRef, value JSValueRef, exception *JSValueRef) bool
	Attributes JSObjectPropertyAttributes
}

// JSStaticFunction defines a static function in a JavaScript class.
type JSStaticFunction struct {
	Name       string
	CallAsFunction func(ctx JSContextRef, function JSObjectRef, thisObject JSObjectRef, argumentCount uintptr, arguments *JSValueRef, exception *JSValueRef) JSValueRef
	Attributes JSObjectPropertyAttributes
}

// JSClassCreate creates a JavaScript class from a definition.
func JSClassCreate(def *JSClassDefinition) JSClassRef {
	_ = def
	return 0
}

// JSClassRetain retains a JavaScript class.
func JSClassRetain(jsClass JSClassRef) JSClassRef {
	return jsClass
}

// JSClassRelease releases a JavaScript class.
func JSClassRelease(jsClass JSClassRef) {
	_ = jsClass
}
