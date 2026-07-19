// Translation of: Source/JavaScriptCore/runtime/IterationKind.h
package runtime

// IterationKind corresponds to JSC::IterationKind.
// Specifies which iteration type an iterator produces: keys, values, or entries.
type IterationKind uint32

const (
	IterationKindKeys    IterationKind = 0
	IterationKindValues                 = 1
	IterationKindEntries                = 2
)
