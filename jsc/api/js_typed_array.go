package api

// JSTypedArray API provides functions for working with typed arrays.

// JSTypedArrayType defines the type of a typed array.
type JSTypedArrayType uint32

const (
	kJSTypedArrayTypeInt8                JSTypedArrayType = 0
	kJSTypedArrayTypeInt16               JSTypedArrayType = 1
	kJSTypedArrayTypeInt32               JSTypedArrayType = 2
	kJSTypedArrayTypeUint8               JSTypedArrayType = 3
	kJSTypedArrayTypeUint8Clamped        JSTypedArrayType = 4
	kJSTypedArrayTypeUint16              JSTypedArrayType = 5
	kJSTypedArrayTypeUint32              JSTypedArrayType = 6
	kJSTypedArrayTypeFloat32             JSTypedArrayType = 7
	kJSTypedArrayTypeFloat64             JSTypedArrayType = 8
	kJSTypedArrayTypeBigInt64            JSTypedArrayType = 9
	kJSTypedArrayTypeBigUint64           JSTypedArrayType = 10
	kJSTypedArrayTypeArrayBuffer         JSTypedArrayType = 11
	kJSTypedArrayTypeNone                JSTypedArrayType = 12
)

// JSObjectGetTypedArrayBytes deallocator type.
type JSTypedArrayBytesDeallocatorPtr uintptr

// JSValueMakeTypedArray creates a typed array value.
func JSValueMakeTypedArray(ctx JSContextRef, arrayType JSTypedArrayType, bytes uintptr, length uintptr, deallocator JSTypedArrayBytesDeallocatorPtr, deallocatorContext uintptr, exception *JSValueRef) JSObjectRef {
	_ = ctx
	_ = arrayType
	_ = bytes
	_ = length
	_ = deallocator
	_ = deallocatorContext
	_ = exception
	return 0
}

// JSValueMakeTypedArrayWithBuffer creates a typed array from a buffer.
func JSValueMakeTypedArrayWithBuffer(ctx JSContextRef, arrayType JSTypedArrayType, buffer JSObjectRef, exception *JSValueRef) JSObjectRef {
	_ = ctx
	_ = arrayType
	_ = buffer
	_ = exception
	return 0
}

// JSValueMakeTypedArrayOfType creates an empty typed array of a given type.
func JSValueMakeTypedArrayOfType(ctx JSContextRef, arrayType JSTypedArrayType, length uintptr, exception *JSValueRef) JSObjectRef {
	_ = ctx
	_ = arrayType
	_ = length
	_ = exception
	return 0
}

// JSValueGetTypedArrayType returns the type of a typed array.
func JSValueGetTypedArrayType(ctx JSContextRef, value JSValueRef, exception *JSValueRef) JSTypedArrayType {
	_ = ctx
	_ = value
	_ = exception
	return kJSTypedArrayTypeNone
}

// JSObjectGetTypedArrayBytesPtr returns a pointer to the typed array's data.
func JSObjectGetTypedArrayBytesPtr(ctx JSContextRef, object JSObjectRef, exception *JSValueRef) uintptr {
	_ = ctx
	_ = object
	_ = exception
	return 0
}

// JSObjectGetTypedArrayLength returns the length of a typed array.
func JSObjectGetTypedArrayLength(ctx JSContextRef, object JSObjectRef, exception *JSValueRef) uintptr {
	_ = ctx
	_ = object
	_ = exception
	return 0
}

// JSObjectGetTypedArrayByteLength returns the byte length of a typed array.
func JSObjectGetTypedArrayByteLength(ctx JSContextRef, object JSObjectRef, exception *JSValueRef) uintptr {
	_ = ctx
	_ = object
	_ = exception
	return 0
}

// JSObjectGetTypedArrayByteOffset returns the byte offset of a typed array.
func JSObjectGetTypedArrayByteOffset(ctx JSContextRef, object JSObjectRef, exception *JSValueRef) uintptr {
	_ = ctx
	_ = object
	_ = exception
	return 0
}

// JSObjectGetTypedArrayBuffer returns the underlying array buffer.
func JSObjectGetTypedArrayBuffer(ctx JSContextRef, object JSObjectRef, exception *JSValueRef) JSObjectRef {
	_ = ctx
	_ = object
	_ = exception
	return 0
}

// JSObjectMakeArrayBufferWithBytesNoCopy creates a new ArrayBuffer sharing the given bytes.
func JSObjectMakeArrayBufferWithBytesNoCopy(ctx JSContextRef, bytes uintptr, length uintptr, deallocator JSTypedArrayBytesDeallocatorPtr, deallocatorContext uintptr, exception *JSValueRef) JSObjectRef {
	_ = ctx
	_ = bytes
	_ = length
	_ = deallocator
	_ = deallocatorContext
	_ = exception
	return 0
}

// JSObjectMakeArrayBufferWithBytesNoCopy owns the bytes (no copy).
func JSObjectMakeArrayBufferWithBytesNoCopyOwn(ctx JSContextRef, bytes uintptr, length uintptr, deallocator JSTypedArrayBytesDeallocatorPtr, deallocatorContext uintptr, exception *JSValueRef) JSObjectRef {
	_ = ctx
	_ = bytes
	_ = length
	_ = deallocator
	_ = deallocatorContext
	_ = exception
	return 0
}
