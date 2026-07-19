// Copyright (C) 2021, 2026 Apple Inc. All rights reserved.
// Translated to Go.
package heap

type RootMarkReason uint8

const (
	RootMarkReasonNone                                  RootMarkReason = iota
	RootMarkReasonConservativeScan
	RootMarkReasonExecutableToCodeBlockEdges
	RootMarkReasonExternalRememberedSet
	RootMarkReasonStrongReferences
	RootMarkReasonProtectedValues
	RootMarkReasonMarkListSet
	RootMarkReasonVMExceptions
	RootMarkReasonStrongHandles
	RootMarkReasonDebugger
	RootMarkReasonJITStubRoutines
	RootMarkReasonWeakMapSpace
	RootMarkReasonWeakSets
	RootMarkReasonOutput
	RootMarkReasonJITWorkList
	RootMarkReasonCodeBlocks
	RootMarkReasonDOMGCOutput
	RootMarkReasonPinballCompletionConservativeRoots
)

func RootMarkReasonDescription(reason RootMarkReason) string {
	switch reason {
	case RootMarkReasonNone:
		return "None"
	case RootMarkReasonConservativeScan:
		return "Conservative scan"
	case RootMarkReasonExecutableToCodeBlockEdges:
		return "Executable to CodeBlock edges"
	case RootMarkReasonExternalRememberedSet:
		return "External remembered set"
	case RootMarkReasonStrongReferences:
		return "Strong references"
	case RootMarkReasonProtectedValues:
		return "Protected values"
	case RootMarkReasonMarkListSet:
		return "Mark list set"
	case RootMarkReasonVMExceptions:
		return "VM exceptions"
	case RootMarkReasonStrongHandles:
		return "Strong handles"
	case RootMarkReasonDebugger:
		return "Debugger"
	case RootMarkReasonJITStubRoutines:
		return "JIT stub routines"
	case RootMarkReasonWeakMapSpace:
		return "Weak map space"
	case RootMarkReasonWeakSets:
		return "Weak sets"
	case RootMarkReasonOutput:
		return "Output"
	case RootMarkReasonJITWorkList:
		return "JIT work list"
	case RootMarkReasonCodeBlocks:
		return "Code blocks"
	case RootMarkReasonDOMGCOutput:
		return "DOM GC output"
	case RootMarkReasonPinballCompletionConservativeRoots:
		return "Pinball completion conservative roots"
	default:
		return "Unknown"
	}
}
