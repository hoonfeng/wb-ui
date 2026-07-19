// Executables corresponds to JSC executable types (FunctionExecutable, etc.)
package runtime

// FunctionExecutable corresponds to JSC::FunctionExecutable.
type FunctionExecutable struct {
	JSCell
}

// ProgramExecutable corresponds to JSC::ProgramExecutable.
type ProgramExecutable struct {
	JSCell
}

// EvalExecutable corresponds to JSC::EvalExecutable.
type EvalExecutable struct {
	JSCell
}

// ModuleProgramExecutable corresponds to JSC::ModuleProgramExecutable.
type ModuleProgramExecutable struct {
	JSCell
}

// UnlinkedFunctionExecutable corresponds to JSC::UnlinkedFunctionExecutable.
type UnlinkedFunctionExecutable struct {
	JSCell
}

// NativeExecutable corresponds to JSC::NativeExecutable.
type NativeExecutable struct {
	JSCell
}

// CodeBlock corresponds to JSC::CodeBlock.
type CodeBlock struct {
	JSCell
}
