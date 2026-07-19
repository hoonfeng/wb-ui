// Translation of: Source/JavaScriptCore/runtime/VM.h
//                  Source/JavaScriptCore/runtime/VM.cpp
//
// VM (Virtual Machine) holds the state for a single JavaScript execution context.

package runtime

// VM corresponds to JSC::VM. It owns the heap, global object, call stack,
// and all execution state for a JavaScript context.
type VM struct {
	// Exception holds the currently pending exception (nil if none).
	Exception *JSValue

	// The global object for this VM.
	GlobalObject *JSGlobalObject

	// Top call frame (execution stack).
	TopCallFrame *ExecState

	// SmallStrings caches common single-character strings.
	SmallStrings *SmallStrings

	// Heap memory manager.
	heap *Heap

	// Whether the VM is currently executing (used for re-entry checks).
	isExecuting bool

	// Whether the VM has been terminated.
	isTerminated bool

	// Structure for Symbol cells. Initialized during VM bootstrap.
	symbolStructure *Structure
}

// SmallStrings caches common JSString values (single-char strings, common keywords).
type SmallStrings struct {
	strings map[string]*JSString
}

// NewSmallStrings creates a new SmallStrings cache.
func NewSmallStrings() *SmallStrings {
	return &SmallStrings{strings: make(map[string]*JSString)}
}

// NewVM creates a new JavaScript VM.
func NewVM() *VM {
	vm := &VM{
		SmallStrings: NewSmallStrings(),
		heap:         &Heap{},
	}
	return vm
}

// NewExecState creates a new execution state (call frame).
func NewExecState(vm *VM) *ExecState {
	return &ExecState{vm: vm}
}

// ExecState corresponds to JSC::ExecState (CallFrame*).
// It holds the execution context for a running function.
type ExecState struct {
	vm            *VM
	callerFrame   *ExecState
	thisValue     JSValue
	argumentCount int
	arguments     []JSValue
	codeBlock     *CodeBlock
	callee        *JSObject
	scope         *JSScope
}

// VM returns the VM associated with this ExecState.
func (e *ExecState) VM() *VM { return e.vm }

// CallerFrame returns the caller's frame.
func (e *ExecState) CallerFrame() *ExecState { return e.callerFrame }

// SetCallerFrame sets the caller frame.
func (e *ExecState) SetCallerFrame(caller *ExecState) { e.callerFrame = caller }

// ThisValue returns the 'this' value.
func (e *ExecState) ThisValue() JSValue { return e.thisValue }

// SetThisValue sets the 'this' value.
func (e *ExecState) SetThisValue(v JSValue) { e.thisValue = v }

// ArgumentCount returns the number of arguments.
func (e *ExecState) ArgumentCount() int { return e.argumentCount }

// SetArgumentCount sets the argument count.
func (e *ExecState) SetArgumentCount(n int) { e.argumentCount = n }

// Argument returns the argument at the given index.
func (e *ExecState) Argument(i int) JSValue {
	if i < 0 || i >= len(e.arguments) {
		return JSValueUndefined
	}
	return e.arguments[i]
}

// Arguments returns all arguments.
func (e *ExecState) Arguments() []JSValue { return e.arguments }

// SetArguments sets the argument list.
func (e *ExecState) SetArguments(args []JSValue) {
	e.arguments = args
	e.argumentCount = len(args)
}

// Callee returns the callee object.
func (e *ExecState) Callee() *JSObject { return e.callee }

// SetCallee sets the callee object.
func (e *ExecState) SetCallee(c *JSObject) { e.callee = c }

// NewTarget returns the new.target value.
func (e *ExecState) NewTarget() JSValue {
	// Simplified: return undefined (no new.target support)
	return JSValueUndefined
}

// jsCallee returns the callee as *JSObject (C++ compat).
func (e *ExecState) jsCallee() *JSObject { return e.callee }

// Scope returns the current scope.
func (e *ExecState) Scope() *JSScope { return e.scope }

// SetScope sets the current scope.
func (e *ExecState) SetScope(s *JSScope) { e.scope = s }

// CodeBlock returns the current CodeBlock.
func (e *ExecState) CodeBlock() *CodeBlock { return e.codeBlock }

// SetCodeBlock sets the current CodeBlock.
func (e *ExecState) SetCodeBlock(cb *CodeBlock) { e.codeBlock = cb }

// --- VM helpers ---

// SetException sets the pending exception.
func (vm *VM) SetException(val JSValue) {
	vm.Exception = &val
}

// ClearException clears the pending exception.
func (vm *VM) ClearException() {
	vm.Exception = nil
}

// HasException returns true if there's a pending exception.
func (vm *VM) HasException() bool {
	return vm.Exception != nil
}

// ThrowException creates an error and sets it as the pending exception.
func (vm *VM) ThrowException(globalObject *JSGlobalObject, msg string) JSValue {
	_ = globalObject
	err := JSValue{tag: TagObject, payload: nil}
	vm.SetException(err)
	return err
}

// IsExecuting returns whether the VM is currently executing code.
func (vm *VM) IsExecuting() bool { return vm.isExecuting }

// SetExecuting sets the executing flag.
func (vm *VM) SetExecuting(executing bool) { vm.isExecuting = executing }

// IsTerminated returns whether the VM has been terminated.
func (vm *VM) IsTerminated() bool { return vm.isTerminated }

// Terminate terminates the VM.
func (vm *VM) Terminate() { vm.isTerminated = true }
