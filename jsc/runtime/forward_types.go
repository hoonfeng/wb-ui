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

// JSArrayIterator corresponds to JSC::JSArrayIterator.
type JSArrayIterator struct{ JSObject }

// JSArrayBuffer corresponds to JSC::JSArrayBuffer.
type JSArrayBuffer struct{ JSObject }

// JSArrayBufferView corresponds to JSC::JSArrayBufferView.
type JSArrayBufferView struct{ JSObject }

// JSPromise corresponds to JSC::JSPromise.
type JSPromise struct{ JSObject }

// JSMap corresponds to JSC::JSMap.
type JSMap struct{ JSObject; data map[string]JSValue }

// JSSet corresponds to JSC::JSSet.
type JSSet struct{ JSObject; data map[string]bool }

// JSWeakMap corresponds to JSC::JSWeakMap.
type JSWeakMap struct{ JSObject; data map[string]JSValue }

// JSWeakSet corresponds to JSC::JSWeakSet.
type JSWeakSet struct{ JSObject; data map[string]bool }

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

// JSMapIterator corresponds to JSC::JSMapIterator.
type JSMapIterator struct{ JSObject }

// JSSetIterator corresponds to JSC::JSSetIterator.
type JSSetIterator struct{ JSObject }

// JSStringIterator corresponds to JSC::JSStringIterator.
type JSStringIterator struct{ JSObject }

// JSBoundFunction corresponds to JSC::JSBoundFunction.
type JSBoundFunction struct{ JSObject }

// JSProxy corresponds to JSC::JSProxy (future use).
type JSProxy struct{ JSObject }

// Symbol corresponds to JSC::Symbol.
type Symbol struct{ JSCell }
