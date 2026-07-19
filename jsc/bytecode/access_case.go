// JIT-only: AccessCase.h - Inline cache access case for JIT.
// In the interpreter, property access is handled directly.
package bytecode

// AccessCase represents an inline cache access case (JIT only).
type AccessCase struct {
	// Stub for interpreter - property access done directly
}

type AccessCaseSnippetParams struct{}

type GetterSetterAccessCase struct{ AccessCase }
type InstanceOfAccessCase struct{ AccessCase }
type IntrinsicGetterAccessCase struct{ AccessCase }
type ModuleNamespaceAccessCase struct{ AccessCase }
type ProxyableAccessCase struct{ AccessCase }
