// Object types without dedicated files yet - forward declarations.
package runtime

// --- Value types (no own file yet) ---

// JSBigInt corresponds to JSC::JSBigInt.
type JSBigInt struct{ JSCell }

// GetterSetter corresponds to JSC::GetterSetter.
type GetterSetter struct{ JSCell }

// CustomGetterSetter corresponds to JSC::CustomGetterSetter.
type CustomGetterSetter struct{ JSCell }

// --- Object types (no own file yet) ---

// JSArrayBuffer corresponds to JSC::JSArrayBuffer.
type JSArrayBuffer struct{ JSObject }

// JSArrayBufferView corresponds to JSC::JSArrayBufferView.
type JSArrayBufferView struct{ JSObject }

// JSGlobalProxy corresponds to JSC::JSGlobalProxy.
type JSGlobalProxy struct{ JSObject }

// ProxyObject corresponds to JSC::ProxyObject.
type ProxyObject struct{ JSObject }


// StringObject corresponds to JSC::StringObject.
type StringObject struct{ JSObject }

// JSCallee corresponds to JSC::JSCallee.
type JSCallee struct{ JSObject }

// DirectArguments corresponds to JSC::DirectArguments.
type DirectArguments struct{ JSObject }

// ScopedArguments corresponds to JSC::ScopedArguments.
type ScopedArguments struct{ JSObject }

// ClonedArguments corresponds to JSC::ClonedArguments.
type ClonedArguments struct{ JSObject }

// JSGenerator corresponds to JSC::JSGenerator.
type JSGenerator struct{ JSObject }

// JSAsyncGenerator corresponds to JSC::JSAsyncGenerator.
type JSAsyncGenerator struct{ JSObject }

// JSBoundFunction corresponds to JSC::JSBoundFunction.
type JSBoundFunction struct{ JSObject }
