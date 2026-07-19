// InlineCallFrame - describes an inlined function call (DFG JIT)
package bytecode

type InlineCallFrame struct {
	codeBlock        *CodeBlock
	arguments        []VirtualRegister
	directCall       bool
	kind             CallMode
	argumentCount    int
}
