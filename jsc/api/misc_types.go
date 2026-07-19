package api

// JavaScriptCore umbrella header placeholder.
const JavaScriptCoreVersion = "7606.1.40"

// JavaScript umbrella header for frameworks that include JavaScriptCore.

// JSAPIGlobalObject is the API-level global object.
type JSAPIGlobalObject struct {
	VM uintptr
}

// JSAPIValueWrapper wraps a JSValue for API usage.
type JSAPIValueWrapper struct{}

// JSAPIWrapperObject wraps an Objective-C object for API usage.
type JSAPIWrapperObject struct{}

// JSCallbackConstructor is a constructor backed by a C callback.
type JSCallbackConstructor struct {
	Callback        func(ctx JSContextRef, constructor JSObjectRef, argumentCount uintptr, arguments *JSValueRef, exception *JSValueRef) JSObjectRef
	ClassRef        JSClassRef
}

// JSCallbackFunction is a function backed by a C callback.
type JSCallbackFunction struct {
	Callback func(ctx JSContextRef, function JSObjectRef, thisObject JSObjectRef, argumentCount uintptr, arguments *JSValueRef, exception *JSValueRef) JSValueRef
	Name     string
}

// JSCallbackObject is an object backed by a C callback.
type JSCallbackObject struct {
	ClassRef        JSClassRef
	PrivateData     uintptr
}

// JSRetainPtr provides retain/release semantics for JS objects.
type JSRetainPtr struct {
	Ptr uintptr
}

// JSScript represents a compiled script.
type JSScript struct {
	Source   string
	URL      string
	SourceID SourceID
}

// JSScriptSourceProvider provides source for JSScript.
type JSScriptSourceProvider struct {
	Script *JSScript
}

// JSVirtualMachine represents a JavaScript virtual machine.
type JSVirtualMachine struct {
	ContextGroup JSContextGroupRef
}

// JSManagedValue wraps a JSValue with managed memory semantics.
type JSManagedValue struct {
	Value JSValueRef
}

// JSWeakValue represents a weak JS value reference.
type JSWeakValue struct {
	Value JSValueRef
}

// JSWeakObjectMapRefInternal is the internal representation of a weak object map.
type JSWeakObjectMapRefInternal struct {
	Map map[JSObjectRef]JSValueRef
}

// JSWrapperMap maps between native and JavaScript objects.
type JSWrapperMap struct{}

// ObjCCallbackFunction is an Objective-C callback function.
type ObjCCallbackFunction struct {
	Function interface{}
}

// APICast provides casting between API types.
type APICast struct{}

// APIUtils provides utility functions for the API.
type APIUtils struct{}

// APIIntegrityPrivate provides integrity checking for the API.
type APIIntegrityPrivate struct{}

// WorkAround173516139 is a workaround for a specific API issue.
type WorkAround173516139 struct{}

// WebKitAvailability provides availability macros for WebKit.
type WebKitAvailability struct{}

// JSCTestRunnerUtils provides test runner utilities.
type JSCTestRunnerUtils struct{}

// JSHeapFinalizerPrivate provides heap finalizer support.
type JSHeapFinalizerPrivate struct{}

// JSLockRefPrivate provides VM lock reference support.
type JSLockRefPrivate struct{}

// JSMarkingConstraintPrivate provides marking constraint support for custom GC roots.
type JSMarkingConstraintPrivate struct{}

// JSRemoteInspector provides remote inspection support.
type JSRemoteInspector struct{}

// JSRemoteInspectorServer provides the remote inspector server.
type JSRemoteInspectorServer struct{}

// MARReportCrashPrivate provides crash reporting support.
type MARReportCrashPrivate struct{}

// PASReportCrashPrivate provides crash reporting support for PAS.
type PASReportCrashPrivate struct{}

// ExtraSymbolsForTAPI provides extra symbols for TAPI.
type ExtraSymbolsForTAPI struct{}

// OpaqueJSString is the opaque type for JSString.
type OpaqueJSString struct {
	Chars string
}

// JSBaseInternal provides internal base functions.
type JSBaseInternal struct{}

// JSBasePrivate provides private base functions.
type JSBasePrivate struct{}

// JSContextInternal provides internal context functions.
type JSContextInternal struct{}

// JSContextPrivate provides private context functions.
type JSContextPrivate struct{}

// JSContextRefInspectorSupport provides inspector support for contexts.
type JSContextRefInspectorSupport struct{}

// JSContextRefInternal provides internal context ref functions.
type JSContextRefInternal struct{}

// JSContextRefPrivate provides private context ref functions.
type JSContextRefPrivate struct{}

// JSExport provides export support for JavaScriptCore.
type JSExport struct{}

// JSValueInternal provides internal JSValue functions.
type JSValueInternal struct{}

// JSValuePrivate provides private JSValue functions.
type JSValuePrivate struct{}

// JSWeakPrivate provides private weak reference functions.
type JSWeakPrivate struct{}

// JSManagedValueInternal provides internal managed value functions.
type JSManagedValueInternal struct{}

// JSVirtualMachineInternal provides internal VM functions.
type JSVirtualMachineInternal struct{}

// JSVirtualMachinePrivate provides private VM functions.
type JSVirtualMachinePrivate struct{}

// JSScriptInternal provides internal script functions.
type JSScriptInternal struct{}

// JSScriptRefPrivate provides private script ref functions.
type JSScriptRefPrivate struct{}

// JSScriptSourceProvider provides source for scripts.
// Already defined above.

// JSStringRefBSTR provides BSTR string support (Windows).
type JSStringRefBSTR struct{}

// JSStringRefCF provides CFString string support (macOS).
type JSStringRefCF struct{}

// JSStringRefPrivate provides private string ref functions.
type JSStringRefPrivate struct{}

// JSObjectRefPrivate provides private object ref functions.
type JSObjectRefPrivate struct{}

// APICallbackFunction provides callback function support for the API.
type APICallbackFunction struct{}

// SourceID is a type alias for source identifier.
type SourceID = uint64

// JSValue.h type - the high-level JSValue wrapper.
type JSValueAPI struct {
	Value JSValueRef
	Context JSContextRef
}

// ObjcRuntimeExtras provides Objective-C runtime extras.
type ObjcRuntimeExtras struct{}
