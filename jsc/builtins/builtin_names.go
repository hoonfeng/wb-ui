// Translation of: Source/JavaScriptCore/builtins/BuiltinNames.h
//                  Source/JavaScriptCore/builtins/BuiltinNames.cpp
//
// BuiltinNames manages the well-known identifiers and private names used
// by builtin functions (e.g. @add, @callFunction, @push, @values, etc.).

package builtins

import (
	"wb-ui/jsc/runtime"
)

// BuiltinNames corresponds to JSC::BuiltinNames.
// It holds the canonical identifiers for all builtin function names,
// well-known symbols, and private names used by JSC builtins.
type BuiltinNames struct {
	vm                 *runtime.VM
	builtinIdentifiers map[string]BuiltinNamePair
	wellKnownSymbols   map[string]*runtime.Identifier
	privateNames       map[string]bool
}

// BuiltinNamePair holds the public and private identifiers for a builtin name.
type BuiltinNamePair struct {
	Public  runtime.Identifier
	Private runtime.Identifier
}

// NewBuiltinNames creates a new BuiltinNames for the given VM.
func NewBuiltinNames(vm *runtime.VM) *BuiltinNames {
	bn := &BuiltinNames{
		vm:                 vm,
		builtinIdentifiers: make(map[string]BuiltinNamePair),
		wellKnownSymbols:   make(map[string]*runtime.Identifier),
		privateNames:       make(map[string]bool),
	}
	bn.initializeBuiltinNames()
	return bn
}

// initializeBuiltinNames sets up all builtin name identifiers.
func (bn *BuiltinNames) initializeBuiltinNames() {
	// Well-known symbols
	bn.registerWellKnownSymbol("iterator")
	bn.registerWellKnownSymbol("toStringTag")
	bn.registerWellKnownSymbol("toPrimitive")
	bn.registerWellKnownSymbol("hasInstance")
	bn.registerWellKnownSymbol("isConcatSpreadable")
	bn.registerWellKnownSymbol("match")
	bn.registerWellKnownSymbol("matchAll")
	bn.registerWellKnownSymbol("replace")
	bn.registerWellKnownSymbol("search")
	bn.registerWellKnownSymbol("species")
	bn.registerWellKnownSymbol("split")
	bn.registerWellKnownSymbol("unscopables")
	bn.registerWellKnownSymbol("asyncIterator")
	bn.registerWellKnownSymbol("dispose")
	bn.registerWellKnownSymbol("asyncDispose")

	// Builtin function names
	builtinFuncNames := []string{
		"abs", "add", "applyFunction", "assert", "callFunction", "charCodeAt",
		"executor", "isView", "iteratedObject", "iteratedString", "promise",
		"Object", "Number", "Array", "ArrayBuffer", "ShadowRealm", "RegExp",
		"Iterator", "min", "create", "defineProperty", "defaultPromiseThen",
		"Set", "Map", "throwTypeErrorFunction", "typedArrayLength",
		"BuiltinLog", "BuiltinDescribe", "homeObject",
		"resolvePromise", "rejectPromise", "fulfillPromise", "markPromiseAsHandled",
		"isPromiseStatePending", "resolvePromiseWithFirstResolvingFunctionCallCheck",
		"rejectPromiseWithFirstResolvingFunctionCallCheck",
		"fulfillPromiseWithFirstResolvingFunctionCallCheck",
		"newResolvedPromise", "newRejectedPromise",
		"resolveWithInternalMicrotaskForAsyncAwait",
		"asyncGeneratorNextQueueEnqueue", "asyncGeneratorCompleteAndDrain",
		"asyncGeneratorSuspend", "driveAsyncFunction",
		"newHandledRejectedPromise", "promiseReturnUndefinedOnFulfilled",
		"promiseResolve", "promiseReject", "performPromiseThen",
		"push", "repeatCharacter", "starDefault", "starNamespace",
		"then", "keys", "values", "set", "clear", "defer", "delete", "size",
		"shift", "staticInitializerBlock",
		"Int8Array", "Int16Array", "Int32Array", "Uint8Array",
		"Uint8ClampedArray", "Uint16Array", "Uint32Array",
		"Float16Array", "Float32Array", "Float64Array",
		"BigInt64Array", "BigUint64Array",
		"exec", "generator", "generatorNext", "generatorState",
		"generatorFrame", "generatorValue", "generatorThis", "generatorResumeMode",
		"this", "toIntegerOrInfinity", "toLength",
		"importInRealm", "evalFunction", "evalInRealm", "moveFunctionToRealm",
		"newTargetLocal", "derivedConstructor",
		"isTypedArrayView", "isSharedTypedArrayView",
		"isResizableOrGrowableSharedTypedArrayView", "isDetached",
		"typedArrayFromFast", "instanceOf", "isArray", "sameValue",
		"regExpCreate", "isRegExp", "isFinite", "makeTypeError",
		"AggregateError",
		"mapStorage", "mapIterationNext", "mapIterationEntry",
		"mapIterationEntryKey", "mapIterationEntryValue",
		"mapIteratorNext", "mapIteratorKey", "mapIteratorValue",
		"setStorage", "setIterationNext", "setIterationEntry",
		"setIterationEntryKey", "setIteratorNext", "setIteratorKey",
		"setPrototypeDirect", "setPrototypeDirectOrThrow",
		"regExpBuiltinExec", "regExpProtoFlagsGetter",
		"regExpProtoHasIndicesGetter", "regExpProtoGlobalGetter",
		"regExpProtoIgnoreCaseGetter", "regExpProtoMultilineGetter",
		"regExpProtoSourceGetter", "regExpProtoStickyGetter",
		"regExpProtoDotAllGetter", "regExpProtoUnicodeGetter",
		"regExpProtoUnicodeSetsGetter",
		"regExpPrototypeSymbolMatch", "regExpPrototypeSymbolMatchAll",
		"regExpPrototypeSymbolReplace", "regExpSearchFast",
		"stringIncludesInternal", "stringIndexOfInternal", "stringSubstring",
		"handleNegativeProxyHasTrapResult", "handlePositiveProxySetTrapResult",
		"handleProxyGetTrapResult",
		"importModule", "moduleFetchFailureKind", "moduleFailureModuleRecord",
		"moduleFailureModuleKey", "moduleFailureModuleType", "moduleFailureKind",
		"copyDataProperties", "cloneObject", "meta",
		"instanceFieldInitializer", "privateBrand", "privateClassBrand",
		"hasOwnPropertyFunction", "createPrivateSymbol",
		"entries", "emptyPropertyNameEnumerator", "sentinelString",
		"createRemoteFunction", "isRemoteFunction",
		"arrayFromFastWithoutMapFn",
		"jsonParse", "jsonStringify", "String",
		"substr", "endsWith",
		"getOwnPropertyDescriptor", "getOwnPropertyNames", "getOwnPropertySymbols",
		"hasOwn", "indexOf", "pop",
		"wrapForValidIteratorCreate", "asyncFromSyncIteratorCreate",
		"regExpStringIteratorCreate", "iteratorHelperCreate",
		"syncIterator", "includes",
		"ReferenceError", "SuppressedError", "DisposableStack", "AsyncDisposableStack",
	}

	for _, name := range builtinFuncNames {
		bn.registerBuiltinName(name)
	}
}

func (bn *BuiltinNames) registerWellKnownSymbol(name string) {
	bn.wellKnownSymbols[name] = nil
}

func (bn *BuiltinNames) registerBuiltinName(name string) {
	pub := runtime.NewIdentifier(name)
	priv := runtime.NewIdentifier("@" + name)
	bn.builtinIdentifiers[name] = BuiltinNamePair{Public: pub, Private: priv}
	bn.privateNames[priv.String()] = true
}

// LookUpPrivateName looks up a private name by identifier.
func (bn *BuiltinNames) LookUpPrivateName(ident *runtime.Identifier) bool {
	if ident == nil {
		return false
	}
	return bn.privateNames[ident.String()]
}

// LookUpPrivateNameByString looks up a private name by string.
func (bn *BuiltinNames) LookUpPrivateNameByString(s string) bool {
	return bn.privateNames[s]
}

// LookUpWellKnownSymbol looks up a well-known symbol by identifier.
func (bn *BuiltinNames) LookUpWellKnownSymbol(ident *runtime.Identifier) *runtime.Identifier {
	if ident == nil {
		return nil
	}
	if sym, ok := bn.wellKnownSymbols[ident.String()]; ok {
		return sym
	}
	return nil
}

// GetPublicName returns the public identifier for a builtin function name.
func (bn *BuiltinNames) GetPublicName(name string) runtime.Identifier {
	if pair, ok := bn.builtinIdentifiers[name]; ok {
		return pair.Public
	}
	return runtime.Identifier{}
}

// GetPrivateName returns the private identifier for a builtin function name.
func (bn *BuiltinNames) GetPrivateName(name string) runtime.Identifier {
	if pair, ok := bn.builtinIdentifiers[name]; ok {
		return pair.Private
	}
	return runtime.Identifier{}
}
