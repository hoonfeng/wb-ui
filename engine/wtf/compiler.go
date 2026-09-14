// Translation of: Source/WTF/wtf/Compiler.h
// Completeness: 70%
// Simplifications:
//   - Go has no preprocessor; attribute macros map to no-op functions / build tags
//   - ALWAYS_INLINE / NEVER_INLINE are placeholders (Go inlining is compiler-controlled)
//   - compiler detection is reduced to a single CompilerGo constant

package wtf

// Compiler.h provides compiler-detection macros (COMPILER(CLANG), COMPILER(GCC),
// COMPILER(MSVC)...) and attribute helpers (ALWAYS_INLINE, NEVER_INLINE, NO_RETURN,
// WTF_ALLOW_UNSAFE_BUFFER_USAGE_BEGIN, ...). In Go there is no preprocessor and the
// only compiler is the gc toolchain, so the detection collapses to a single value and
// the attribute macros become no-op markers used purely to keep the translation
// 1:1-readable with upstream WebKit code.

// Compiler identifies the compiler producing the build. WebKit distinguishes Clang,
// GCC, MSVC and others; the Go port is always compiled by the gc toolchain, so the
// only emitted value is CompilerGo.
type Compiler int

const (
	// CompilerGCC mirrors WebKit's COMPILER(GCC). Always false in this port; kept as
	// a named constant so upstream guard expressions stay readable.
	CompilerGCC Compiler = iota
	// CompilerClang mirrors COMPILER(CLANG). Always false in this port.
	CompilerClang
	// CompilerMSVC mirrors COMPILER(MSVC). Always false in this port.
	CompilerMSVC
	// CompilerGo is the only compiler used by wb-ui (the Go gc toolchain).
	CompilerGo
)

// CurrentCompiler returns the compiler building this package. It always returns
// CompilerGo, mirroring the role of the COMPILER(...) family in Compiler.h.
func CurrentCompiler() Compiler { return CompilerGo }

// AlwaysInline is a placeholder for the C++ ALWAYS_INLINE attribute. Go has no inline
// modifier; inlining is decided by the gc compiler's own cost model. The function is
// retained so translated call sites that wrote ALWAYS_INLINE can keep the marker
// without affecting behaviour.
func AlwaysInline() {}

// NeverInline is a placeholder for the C++ NEVER_INLINE attribute. Like
// AlwaysInline it has no effect in Go and exists only to preserve the 1:1 mapping.
func NeverInline() {}

// NoReturn is a placeholder for the C++ NO_RETURN attribute. In Go a function that
// never returns (e.g. one that always calls panic or runtime.Goexit) is inferred by
// the compiler; this marker is a no-op kept for translation fidelity.
func NoReturn() {}

// SuppressASanUnsafeBufferUsage mirrors WTF_ALLOW_UNSAFE_BUFFER_USAGE_BEGIN /
// WTF_ALLOW_UNSAFE_BUFFER_USAGE_END. Go has no address sanitizer integration and no
// raw pointers, so these are no-ops.
func SuppressASanUnsafeBufferUsage() {}
