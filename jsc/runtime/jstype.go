// Translation of: Source/JavaScriptCore/runtime/JSType.h
// Source/JavaScriptCore/runtime/JSTypeInfo.h
//
// JSType enumerates every JSCell variant. Go port uses uint8 directly.

package runtime

// JSType enumerates every JSCell variant. Original: JSC::JSType
type JSType uint8

//go:generate stringer -type=JSType

// All JSType values. Derived from FOR_EACH_JS_TYPE macro in JSType.h.
const (
	// CellType must come before any JSType that is a JSCell.
	CellType                         JSType = iota // 0
	StructureType                                   // 1
	StringType                                      // 2
	HeapBigIntType                                  // 3
	SymbolType                                      // 4
	GetterSetterType                                // 5
	CustomGetterSetterType                          // 6
	APIValueWrapperType                             // 7
	NativeExecutableType                            // 8
	ProgramExecutableType                           // 9
	ModuleProgramExecutableType                     // 10
	EvalExecutableType                              // 11
	FunctionExecutableType                          // 12
	UnlinkedFunctionExecutableType                  // 13
	UnlinkedProgramCodeBlockType                    // 14
	UnlinkedModuleProgramCodeBlockType              // 15
	UnlinkedEvalCodeBlockType                       // 16
	UnlinkedFunctionCodeBlockType                   // 17
	CodeBlockType                                   // 18
	JSCellButterflyType                             // 19
	JSSourceCodeType                                // 20
	JSSlimPromiseReactionType                       // 21
	JSFullPromiseReactionType                       // 22
	JSPromiseCombinatorsContextType                 // 23
	JSPromiseCombinatorsGlobalContextType            // 24
	JSWebAssemblyStreamingContextType               // 25
	JSMicrotaskDispatcherType                       // 26
	ModuleRegistryEntryType                         // 27
	ModuleLoadingContextType                        // 28
	ModuleLoaderPayloadType                         // 29
	ModuleGraphLoadingStateType                     // 30
	JSModuleLoaderType                              // 31
	SentinelType                                    // 32
	ObjectType                                      // 33
	FinalObjectType                                 // 34
	JSCalleeType                                    // 35
	JSFunctionType                                  // 36
	InternalFunctionType                            // 37
	NullSetterFunctionType                          // 38
	BooleanObjectType                               // 39
	NumberObjectType                                // 40
	ErrorInstanceType                               // 41
	GlobalProxyType                                 // 42
	DirectArgumentsType                             // 43
	ScopedArgumentsType                             // 44
	ClonedArgumentsType                             // 45
	ArrayType                                       // 46
	DerivedArrayType                                // 47
	ArrayBufferType                                 // 48
	Int8ArrayType                                   // 49
	Uint8ArrayType                                  // 50
	Uint8ClampedArrayType                           // 51
	Int16ArrayType                                  // 52
	Uint16ArrayType                                 // 53
	Int32ArrayType                                  // 54
	Uint32ArrayType                                 // 55
	Float16ArrayType                                // 56
	Float32ArrayType                                // 57
	Float64ArrayType                                // 58
	BigInt64ArrayType                               // 59
	BigUint64ArrayType                              // 60
	DataViewType                                    // 61
	GlobalObjectType                                // 62
	GlobalLexicalEnvironmentType                    // 63
	LexicalEnvironmentType                          // 64
	ModuleEnvironmentType                           // 65
	StrictEvalActivationType                        // 66
	WithScopeType                                   // 67
	AsyncDisposableStackType                        // 68
	DisposableStackType                             // 69
	ModuleNamespaceObjectType                       // 70
	ShadowRealmType                                 // 71
	RegExpObjectType                                // 72
	JSDateType                                      // 73
	ProxyObjectType                                 // 74
	JSGeneratorType                                 // 75
	JSAsyncFunctionGeneratorType                    // 76
	JSAsyncGeneratorType                            // 77
	JSArrayIteratorType                             // 78
	JSIteratorType                                  // 79
	JSIteratorHelperType                            // 80
	JSMapIteratorType                               // 81
	JSSetIteratorType                               // 82
	JSStringIteratorType                            // 83
	JSWrapForValidIteratorType                      // 84
	JSRegExpStringIteratorType                      // 85
	JSAsyncFromSyncIteratorType                     // 86
	JSPromiseType                                   // 87
	JSMapType                                       // 88
	JSSetType                                       // 89
	JSWeakMapType                                   // 90
	JSWeakSetType                                   // 91
	WebAssemblyModuleType                           // 92
	WebAssemblyInstanceType                         // 93
	WebAssemblyGCObjectType                         // 94
	StringObjectType                                // 95
	DerivedStringObjectType                         // 96
	// LastJSCObjectType = DerivedStringObjectType
)

const (
	// MaxJSType is the maximum JSType value (0b11111111 = 255)
	MaxJSType JSType = 0b11111111

	// EmbedderArrayLikeType is a type reserved for embedders
	EmbedderArrayLikeType uint8 = 0b11101101

	// LastValueCompareCellType is the last type requiring value comparison
	LastValueCompareCellType = HeapBigIntType

	// Typed array range constants
	FirstTypedArrayType                     = Int8ArrayType
	LastTypedArrayType                      = DataViewType
	LastTypedArrayTypeExcludingDataView     = LastTypedArrayType - 1

	// Object type range
	FirstObjectType = ObjectType
	LastObjectType  = MaxJSType

	// Scope type range
	FirstScopeType = GlobalObjectType
	LastScopeType  = WithScopeType

	NumberOfTypedArrayTypes                     = LastTypedArrayType - FirstTypedArrayType + 1
	NumberOfTypedArrayTypesExcludingDataView    = NumberOfTypedArrayTypes - 1
	NumberOfTypedArrayTypesExcludingBigIntArraysAndDataView = NumberOfTypedArrayTypes - 3
)

// IsTypedArrayType returns true if the type is a typed array (excluding DataView).
func IsTypedArrayType(t JSType) bool {
	return uint32(t)-uint32(FirstTypedArrayType) < uint32(NumberOfTypedArrayTypesExcludingDataView)
}

// IsTypedArrayTypeIncludingDataView returns true if the type is a typed array or DataView.
func IsTypedArrayTypeIncludingDataView(t JSType) bool {
	return uint32(t)-uint32(FirstTypedArrayType) < uint32(NumberOfTypedArrayTypes)
}

// IsObjectType returns true if the type is >= ObjectType.
func IsObjectType(t JSType) bool { return t >= ObjectType }

// IsArgumentsType returns true if the type is one of the arguments types.
func IsArgumentsType(t JSType) bool {
	return t == DirectArgumentsType ||
		t == ScopedArgumentsType ||
		t == ClonedArgumentsType
}