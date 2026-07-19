// Package bytecompiler provides the bytecode generator.
// This file corresponds to WebKit SuperSamplerBytecodeScope.h.

package bytecompiler

// SuperSamplerBytecodeScope corresponds to the JSC RAII class of the same name.
// In Go, we simplify to a struct that embeds a reference to the BytecodeGenerator.
type SuperSamplerBytecodeScope struct {
	generator *BytecodeGenerator
}

// NewSuperSamplerBytecodeScope creates a new scope and calls emitSuperSamplerBegin.
func NewSuperSamplerBytecodeScope(generator *BytecodeGenerator) *SuperSamplerBytecodeScope {
	s := &SuperSamplerBytecodeScope{generator: generator}
	s.generator.EmitSuperSamplerBegin()
	return s
}

// Close calls emitSuperSamplerEnd.
func (s *SuperSamplerBytecodeScope) Close() {
	s.generator.EmitSuperSamplerEnd()
}
