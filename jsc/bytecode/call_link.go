// CallEdge / CallLinkInfo / CallLinkStatus (JIT IC types)
package bytecode

type CallEdge struct {
	callee uint64 // JSCell*
	count  uint32
}

type CallLinkInfo struct {
	callType        uint8
	hasBeenSeen     bool
	hasSeenLateRepatch bool
}

type CallLinkInfoBase struct{}
type CallLinkStatus struct {
	variants []CallVariant
	isProved bool
}
type CallVariant struct {
	executable uint64 // ExecutableBase*
	isClosure  bool
}
type CallVariantInlines struct{}
