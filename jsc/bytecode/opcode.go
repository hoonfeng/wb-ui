// Copyright (C) 2008-2021 Apple Inc. All rights reserved.
// Use of this source code is governed by a BSD-style license.
//
// Translated to Go from WebKit Source/JavaScriptCore/bytecode/Opcode.h

package bytecode

import "fmt"

// OpcodeID represents a bytecode opcode identifier.
type OpcodeID uint8

const (
	// Wide prefix opcodes
	OpWide16 OpcodeID = 0
	OpWide32 OpcodeID = 1

	// Basic flow control
	OpEnter        OpcodeID = 2
	OpRet          OpcodeID = 3
	OpJmp          OpcodeID = 4
	OpJtrue        OpcodeID = 5
	OpJfalse       OpcodeID = 6
	OpJeqNull      OpcodeID = 7
	OpJneqNull     OpcodeID = 8
	OpJeq          OpcodeID = 9
	OpJneq         OpcodeID = 10
	OpJstricteq    OpcodeID = 11
	OpJnstricteq   OpcodeID = 12
	OpJless        OpcodeID = 13
	OpJlesseq      OpcodeID = 14
	OpJgreater     OpcodeID = 15
	OpJgreatereq   OpcodeID = 16
	OpJnless       OpcodeID = 17
	OpJnlesseq     OpcodeID = 18
	OpSwitchImm    OpcodeID = 19
	OpSwitchChar   OpcodeID = 20
	OpSwitchString OpcodeID = 21

	// Object operations
	OpNewObject    OpcodeID = 22
	OpNewArray     OpcodeID = 23
	OpNewArrayWithSize OpcodeID = 24
	OpNewArrayBuffer   OpcodeID = 25
	OpNewArrayWithSpecies OpcodeID = 26
	OpNewRegexp    OpcodeID = 27
	OpNewFunc      OpcodeID = 28
	OpNewFuncExp   OpcodeID = 29
	OpNewGenerator OpcodeID = 30
	OpNewAsyncFunc OpcodeID = 31
	OpNewAsyncGenerator OpcodeID = 32

	// Variable access
	OpMov          OpcodeID = 33
	OpGetFromScope OpcodeID = 34
	OpPutToScope   OpcodeID = 35
	OpGetById      OpcodeID = 36
	OpGetByIdWithThis OpcodeID = 37
	OpPutById      OpcodeID = 38
	OpPutByIdWithThis OpcodeID = 39
	OpGetByVal     OpcodeID = 40
	OpPutByVal     OpcodeID = 41
	OpGetByValWithThis OpcodeID = 42
	OpPutByValWithThis OpcodeID = 43
	OpGetPrivateName OpcodeID = 44
	OpSetPrivateName  OpcodeID = 45
	OpGetGlobalVar OpcodeID = 46
	OpPutGlobalVar OpcodeID = 47
	OpGetClosureVar  OpcodeID = 48
	OpPutClosureVar  OpcodeID = 49
	OpGetInternalField    OpcodeID = 50
	OpPutInternalField    OpcodeID = 51
	OpGetArgument  OpcodeID = 52
	OpGetRestLength OpcodeID = 53
	OpToObject     OpcodeID = 54
	OpToThis       OpcodeID = 55
	OpResolveScope OpcodeID = 56
	OpResolveScopeForHoistingFuncDeclInEval OpcodeID = 57

	// Arithmetic & type conversion
	OpAdd          OpcodeID = 58
	OpSub          OpcodeID = 59
	OpMul          OpcodeID = 60
	OpDiv          OpcodeID = 61
	OpMod          OpcodeID = 62
	OpPow          OpcodeID = 63
	OpNegate       OpcodeID = 64
	OpInc          OpcodeID = 65
	OpDec          OpcodeID = 66
	OpBitand       OpcodeID = 67
	OpBitor        OpcodeID = 68
	OpBitxor       OpcodeID = 69
	OpLshift       OpcodeID = 70
	OpRshift       OpcodeID = 71
	OpUrsa         OpcodeID = 72 // Unsigned right shift
	OpUnsigned     OpcodeID = 73
	OpNot          OpcodeID = 74
	OpBitnot       OpcodeID = 75
	OpToPrimitive  OpcodeID = 76
	OpToNumber     OpcodeID = 77
	OpToNumeric    OpcodeID = 78
	OpToString     OpcodeID = 79
	OpTypeof       OpcodeID = 80
	OpIsUndefined  OpcodeID = 81
	OpIsBoolean    OpcodeID = 82
	OpIsNumber     OpcodeID = 83
	OpIsString     OpcodeID = 84
	OpIsObject     OpcodeID = 85
	OpIsCallable   OpcodeID = 86
	OpIsConstructor OpcodeID = 87
	OpHasIndexedProperty OpcodeID = 88
	OpHasGenericProperty  OpcodeID = 89

	// Function calls
	OpCall                 OpcodeID = 90
	OpCallIgnoreResult     OpcodeID = 91
	OpCallDirectEval       OpcodeID = 92
	OpTailCall             OpcodeID = 93
	OpConstruct            OpcodeID = 94
	OpCallVarargs          OpcodeID = 95
	OpTailCallVarargs      OpcodeID = 96
	OpConstructVarargs     OpcodeID = 97

	// Exception handling
	OpThrow                 OpcodeID = 98
	OpThrowStaticError      OpcodeID = 99
	OpCatch                 OpcodeID = 100
	OpPushTryCatch          OpcodeID = 101
	OpPushTryFinally        OpcodeID = 102
	OpPopTry                OpcodeID = 103
	OpUnreachable           OpcodeID = 104

	// Scope & environment
	OpCreateLexicalEnvironment    OpcodeID = 105
	OpCreatePrivateEnvironment    OpcodeID = 106
	OpCreateGlobalLexicalEnvironment OpcodeID = 107
	OpCreateArguments             OpcodeID = 108
	OpCreateDirectArguments       OpcodeID = 109
	OpCreateScopedArguments       OpcodeID = 110
	OpCreateClonedArguments       OpcodeID = 111
	OpCreateRest                  OpcodeID = 112
	OpCreateThis                  OpcodeID = 113
	OpCreatePromiseCallback       OpcodeID = 114

	// Property enumeration
	OpEnumeratorNext              OpcodeID = 115
	OpEnumeratorGetByVal          OpcodeID = 116
	OpEnumeratorInByVal           OpcodeID = 117
	OpEnumeratorHasOwnProperty    OpcodeID = 118
	OpEnumeratorPutByVal          OpcodeID = 119
	OpGetPropertyEnumerator       OpcodeID = 120
	OpToIndexString               OpcodeID = 121

	// Super & class
	OpSuperConstruct              OpcodeID = 122
	OpSuperConstructVarargs       OpcodeID = 123
	OpGetPrototypeOf              OpcodeID = 124
	OpSetPrototypeOf              OpcodeID = 125
	OpCheckTraits                 OpcodeID = 126
	OpCheckPrivateBrand           OpcodeID = 127
	OpSetPrivateBrand             OpcodeID = 128
	OpInstanceOf                  OpcodeID = 129
	OpIsEmpty                     OpcodeID = 130
	OpInByVal                     OpcodeID = 131
	OpInById                      OpcodeID = 132
	OpHasPrivateName              OpcodeID = 133
	OpPushWithScope               OpcodeID = 134

	// Object destructuring & spread
	OpObjectDestruct             OpcodeID = 135
	OpObjectDestructInit         OpcodeID = 136
	OpSpread                     OpcodeID = 137
	OpRestParameter              OpcodeID = 138

	// Module
	OpImport                     OpcodeID = 139
	OpImportAssertion            OpcodeID = 140
	OpImportDefaultSpecifier     OpcodeID = 141
	OpImportNamespace            OpcodeID = 142
	OpExportBySetter             OpcodeID = 143
	OpExportDefault              OpcodeID = 144
	OpExportNamed                OpcodeID = 145
	OpResolveExport              OpcodeID = 146
	OpModuleMetadata             OpcodeID = 147

	// Template literals
	OpStringify                  OpcodeID = 148
	OpCallStringConcat           OpcodeID = 149

	// Yield & await
	OpYield                      OpcodeID = 150
	OpYieldStar                  OpcodeID = 151
	OpAsyncYield                 OpcodeID = 152
	OpAwait                      OpcodeID = 153

	// Debug
	OpDebug                      OpcodeID = 154
	OpDebugHook                  OpcodeID = 155

	// Helper opcodes
	OpNop                        OpcodeID = 156
	OpIdentity                   OpcodeID = 157

	// Iterator
	OpIteratorOpen               OpcodeID = 158
	OpIteratorNext               OpcodeID = 159

	// Define properties
	OpDefineDataProperty         OpcodeID = 160
	OpDefineAccessorProperty     OpcodeID = 161

	// Log / misc
	OpLog                        OpcodeID = 162
	OpProfileType                OpcodeID = 163
	OpProfileControlFlow         OpcodeID = 164

	// LLInt helper opcodes (added after regular bytecodes)
	NumBytecodeIDs OpcodeID = 165
)

// Opcode lengths - simplified for interpreter
// Each entry: [opcode] = number of operands in 1-byte units
var opcodeLengths = [NumBytecodeIDs]uint8{}

func init() {
	// Initialize opcode lengths
	// Width: 1 (opcode byte) + operands in bytes
	for i := range opcodeLengths {
		opcodeLengths[i] = 1 // minimum: just opcode
	}

	// These have 1 operand (dst)
	opcodeLengths[OpRet] = 2
	opcodeLengths[OpEnd] = 2
	opcodeLengths[OpThrow] = 2
	opcodeLengths[OpYield] = 2
	opcodeLengths[OpAwait] = 2

	// These have 2 operands (dst, src)
	opcodeLengths[OpMov] = 3
	opcodeLengths[OpNot] = 3
	opcodeLengths[OpNegate] = 3
	opcodeLengths[OpToNumber] = 3
	opcodeLengths[OpToString] = 3
	opcodeLengths[OpTypeof] = 3
	opcodeLengths[OpInc] = 3
	opcodeLengths[OpDec] = 3
	opcodeLengths[OpBitnot] = 3
	opcodeLengths[OpUnsigned] = 3

	// These have 3 operands (dst, lhs, rhs)
	opcodeLengths[OpAdd] = 4
	opcodeLengths[OpSub] = 4
	opcodeLengths[OpMul] = 4
	opcodeLengths[OpDiv] = 4
	opcodeLengths[OpMod] = 4
	opcodeLengths[OpPow] = 4
	opcodeLengths[OpBitand] = 4
	opcodeLengths[OpBitor] = 4
	opcodeLengths[OpBitxor] = 4
	opcodeLengths[OpLshift] = 4
	opcodeLengths[OpRshift] = 4
	opcodeLengths[OpUrsa] = 4

	// Branch opcodes: dst, target(offset)
	opcodeLengths[OpJmp] = 3
	opcodeLengths[OpJtrue] = 4
	opcodeLengths[OpJfalse] = 4
	opcodeLengths[OpJeqNull] = 4
	opcodeLengths[OpJneqNull] = 4
	opcodeLengths[OpJeq] = 5
	opcodeLengths[OpJneq] = 5
	opcodeLengths[OpJstricteq] = 5
	opcodeLengths[OpJnstricteq] = 5
	opcodeLengths[OpJless] = 5
	opcodeLengths[OpJlesseq] = 5

	// Call: result, callee, argCount, firstArg
	opcodeLengths[OpCall] = 5
	opcodeLengths[OpCallDirectEval] = 5
	opcodeLengths[OpConstruct] = 5
	opcodeLengths[OpTailCall] = 5

	// Property access
	opcodeLengths[OpGetById] = 5
	opcodeLengths[OpPutById] = 5
	opcodeLengths[OpGetByVal] = 5
	opcodeLengths[OpPutByVal] = 5

	// New object
	opcodeLengths[OpNewObject] = 3
	opcodeLengths[OpNewArray] = 3
	opcodeLengths[OpNewArrayWithSize] = 3

	// Scope
	opcodeLengths[OpGetFromScope] = 5
	opcodeLengths[OpPutToScope] = 5
	opcodeLengths[OpResolveScope] = 3
}

// Convenience aliases
const (
	OpEnd     OpcodeID = OpRet  // end == ret
)

// Opcode name strings
var opcodeNames = [NumBytecodeIDs]string{
	OpWide16:  "op_wide16",
	OpWide32:  "op_wide32",
	OpEnter:   "op_enter",
	OpRet:     "op_ret",
	OpJmp:     "op_jmp",
	OpJtrue:   "op_jtrue",
	OpJfalse:  "op_jfalse",
	OpCall:    "op_call",
	OpConstruct: "op_construct",
	OpGetById: "op_get_by_id",
	OpPutById: "op_put_by_id",
	OpAdd:     "op_add",
	OpSub:     "op_sub",
	OpMul:     "op_mul",
	OpDiv:     "op_div",
	OpNewObject: "op_new_object",
	OpNewArray: "op_new_array",
	OpMov:     "op_mov",
	OpThrow:   "op_throw",
	OpCatch:   "op_catch",
	OpNop:     "op_nop",
}

func init() {
	for i := range opcodeNames {
		if opcodeNames[i] == "" {
			opcodeNames[i] = fmt.Sprintf("op_%d", i)
		}
	}
}

// IsBranch returns true if the opcode is a branch instruction.
func IsBranch(opcodeID OpcodeID) bool {
	switch opcodeID {
	case OpJmp, OpJtrue, OpJfalse, OpJeqNull, OpJneqNull,
		OpJeq, OpJneq, OpJstricteq, OpJnstricteq,
		OpJless, OpJlesseq, OpJgreater, OpJgreatereq,
		OpSwitchImm, OpSwitchChar, OpSwitchString:
		return true
	}
	return false
}

// IsUnconditionalBranch returns true if the opcode always branches.
func IsUnconditionalBranch(opcodeID OpcodeID) bool {
	return opcodeID == OpJmp
}

// IsTerminal returns true if the opcode terminates execution.
func IsTerminal(opcodeID OpcodeID) bool {
	return opcodeID == OpRet || opcodeID == OpUnreachable
}

// IsThrow returns true if the opcode throws an exception.
func IsThrow(opcodeID OpcodeID) bool {
	return opcodeID == OpThrow || opcodeID == OpThrowStaticError
}

func OpcodeName(id OpcodeID) string {
	return opcodeNames[id]
}
