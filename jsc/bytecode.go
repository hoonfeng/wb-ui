// Translation of: Source/JavaScriptCore/bytecode/Opcode.h
//                  Source/JavaScriptCore/bytecode/Instruction.h
//                  Source/JavaScriptCore/bytecompiler/BytecodeGenerator.cpp
//                  Source/JavaScriptCore/bytecompiler/BytecodeGenerator.h
// Completeness: 55%
// Simplifications:
//   - stack-based VM with named variables (no register allocation, no temporaries).
//   - jump targets are absolute instruction indices patched after generation.
//   - each function body is compiled into a FunctionBody (instructions slice); closures
//     capture the runtime Environment at creation time, not a snapshot of values.
//   - no bytecode size variants (no wide/extra-wide); a single Instruction struct.

package jsc

import "strconv"

// Opcode enumerates the bytecode operations, mirroring the OpcodeID enum in
// bytecode/Opcode.h. The Go port uses a flat enumeration; operands are carried in the
// Instruction fields rather than packed into a tagged union.
type Opcode int

const (
	// OpLoadConst pushes a constant JSValue onto the stack.
	OpLoadConst Opcode = iota
	// OpLoadVar pushes the value of a named variable.
	OpLoadVar
	// OpStoreVar pops a value and stores it into a named variable.
	OpStoreVar
	// OpLoadThis pushes the current 'this' binding.
	OpLoadThis
	// OpLoadUndefined pushes the undefined value.
	OpLoadUndefined
	// OpLoadNull pushes the null value.
	OpLoadNull
	// OpLoadProp pops an object and pushes obj.Name.
	OpLoadProp
	// OpLoadIndex pops an object and a key and pushes obj[key].
	OpLoadIndex
	// OpStoreProp pops a value and an object and sets obj.Name = value.
	OpStoreProp
	// OpStoreIndex pops a value, a key, and an object and sets obj[key] = value.
	OpStoreIndex
	// OpBinOp pops b and a and pushes a op b.
	OpBinOp
	// OpUnOp pops a and pushes op a.
	OpUnOp
	// OpJump jumps to Target unconditionally.
	OpJump
	// OpJumpIfTrue pops a value and jumps if it is truthy.
	OpJumpIfTrue
	// OpJumpIfFalse pops a value and jumps if it is falsy.
	OpJumpIfFalse
	// OpJumpIfNull pops a value and jumps if it is null/undefined (for ??).
	OpJumpIfNullish
	// OpCall pops argc arguments and a callee and pushes the result.
	OpCall
	// OpCallMethod pops argc arguments, a property name, and an object; calls the method.
	OpCallMethod
	// OpNew pops argc arguments and a callee and pushes a new instance.
	OpNew
	// OpNewClosure pushes a JSFunction capturing the current environment.
	OpNewClosure
	// OpReturn pops a value and returns from the current function.
	OpReturn
	// OpReturnUndefined returns undefined from the current function.
	OpReturnUndefined
	// OpPop discards the top of the stack.
	OpPop
	// OpDup duplicates the top of the stack.
	OpDup
	// OpDup2 duplicates the top two stack values (a b -> a b a b).
	OpDup2
	// OpLoadArray pops Count elements and pushes an array object.
	OpLoadArray
	// OpLoadObject pops Count key/value pairs and pushes an object.
	OpLoadObject
	// OpThrow pops a value and throws it.
	OpThrow
	// OpEnterScope pushes a new lexical scope.
	OpEnterScope
	// OpLeaveScope pops the top lexical scope.
	OpLeaveScope
	// OpDeclareVar declares a variable in the current scope.
	OpDeclareVar
	// OpDeclareConst declares a const binding in the current scope (pops the value).
	OpDeclareConst
	// OpNop is a no-op (used as a placeholder).
	OpNop
	// OpTemplateJoin concatenates Count+1 quasis around Count expressions already on
	// the stack, pushing the resulting string.
	OpTemplateJoin
	// OpBeginForIn prepares iteration over an object; pushes a hidden iterator.
	OpBeginForIn
	// OpForInNext advances the iterator and jumps to Target when exhausted.
	OpForInNext
	// OpEndForIn cleans up the for-in iterator.
	OpEndForIn
	// OpEnterTry sets up an exception handler covering the next instructions.
	OpEnterTry
	// OpLeaveTry pops the current exception handler.
	OpLeaveTry
	// OpCatch stores the caught exception value into Name.
	OpCatch
	// OpImport loads a value from a module by name.
	OpImport
	// OpExport stores a value into the current module's export table.
	OpExport
	// OpAwait pops a value from the stack and unwraps a Promise if the value is a
	// Promise. If the Promise is already settled, the resolved/rejected value is
	// pushed back. If the Promise is pending, the interpreter must be running inside
	// an async function; the current function returns its Promise immediately
	// (microtask scheduling not yet implemented).
	OpAwait
	// OpYield yields a value from a generator (simplified: acts as return).
	OpYield
	// OpSetAccessor sets a getter/setter accessor on an object. Pops fn and obj,
	// sets obj[Name] as an accessor with the given function as getter (IntArg=0)
	// or setter (IntArg=1).
	OpSetAccessor
)

// Instruction is a single bytecode instruction. It mirrors the packed Instruction
// class in bytecode/Instruction.h, but uses a flat struct with optional operand fields
// so the interpreter can switch on Op without unpacking.
type Instruction struct {
	Op      Opcode
	IntArg  int           // generic integer operand (argc, count, jump target)
	IntArg2 int           // second integer operand
	Name    string        // variable/property name
	StrArg  string        // auxiliary string operand
	Value   JSValue       // constant payload (OpLoadConst)
	OpTok   TokenKind     // operator payload (OpBinOp/OpUnOp)
	Body    *FunctionBody // function literal payload (OpNewClosure)
	Quasis  []string      // template literal parts (OpTemplateJoin)
}

// CompileProgram compiles a top-level Program AST into a FunctionBody ready to execute.
// Hoisted function declarations are emitted first so they are visible before any
// statement runs, mirroring JSC's hoisting pass.
func CompileProgram(prog *Program) *FunctionBody {
	g := newGenerator()
	// Hoisting pass: function declarations become bindings in the current scope.
	for _, st := range prog.Body {
		if fd, ok := st.(*FunctionDeclaration); ok {
			g.declareVar(fd.Name)
		}
	}
	// Declare vars/lets with hoisted names (TDZ is not enforced).
	for _, st := range prog.Body {
		if vd, ok := st.(*VariableDeclaration); ok && vd.Kind == "var" {
			for _, d := range vd.Declarators {
				g.declareVar(d.Name)
			}
		}
	}
	// Emit function declarations first (hoisted).
	for _, st := range prog.Body {
		if fd, ok := st.(*FunctionDeclaration); ok {
			g.emitFunctionHoisted(fd)
		}
	}
	// Emit remaining statements in order.
	for _, st := range prog.Body {
		g.emitStmt(st)
	}
	g.emit(Instruction{Op: OpReturnUndefined})
	body := &FunctionBody{Name: "<main>", Instructions: g.code, NumLocals: len(g.scopeLocals), IsTopLevel: true}
	return body
}

// compileFunction compiles a function expression/declaration body into a FunctionBody.
// restParam is the name of the rest parameter (e.g., "nums" for ...nums), empty if none.
func compileFunction(name string, params []string, defaults []Expr, body []Stmt, isArrow bool, isAsync bool, isGenerator bool, destructs []DestructInfo, restParam ...string) *FunctionBody {
	var rp string
	if len(restParam) > 0 {
		rp = restParam[0]
	}
	g := newGenerator()
	g.params = params
	g.params = params
	g.isArrow = isArrow
	// Parameters become local variables.
	for _, p := range params {
		g.declareVar(p)
	}
	// Emit destructuring code for array/object params.
	for _, d := range destructs {
		if d.IsArray {
			for j, nm := range d.Names {
				// nm = params[d.ParamIdx][j]
				g.emit(Instruction{Op: OpLoadVar, Name: params[d.ParamIdx]})
				g.emit(Instruction{Op: OpLoadConst, Value: NumberValue(float64(j))})
				g.emit(Instruction{Op: OpLoadIndex})
				g.emit(Instruction{Op: OpDeclareVar, Name: nm})
				g.emit(Instruction{Op: OpStoreVar, Name: nm})
				g.emit(Instruction{Op: OpPop})
			}
		} else {
			for _, nm := range d.Names {
				// nm = params[d.ParamIdx][nm]
				g.emit(Instruction{Op: OpLoadVar, Name: params[d.ParamIdx]})
				g.emit(Instruction{Op: OpLoadConst, Value: StringValue(nm)})
				g.emit(Instruction{Op: OpLoadIndex})
				g.emit(Instruction{Op: OpDeclareVar, Name: nm})
				g.emit(Instruction{Op: OpStoreVar, Name: nm})
				g.emit(Instruction{Op: OpPop})
			}
		}
	}
	// Emit default parameter checks: if param === undefined, assign default.
	for i, def := range defaults {
		if def == nil {
			continue
		}
		// Generate: if (param === undefined) { param = default; }
		g.emit(Instruction{Op: OpLoadVar, Name: params[i]})
		g.emit(Instruction{Op: OpLoadUndefined})
		g.emit(Instruction{Op: OpBinOp, OpTok: TokenStrictEqual})
		skipJump := g.emitJump(OpJumpIfFalse)
		g.emitExpr(def)
		g.emit(Instruction{Op: OpStoreVar, Name: params[i]})
		g.emit(Instruction{Op: OpPop})
		g.patchJump(skipJump)
	}
	// Hoist inner function declarations.
	for _, st := range body {
		if fd, ok := st.(*FunctionDeclaration); ok {
			g.declareVar(fd.Name)
		}
	}
	for _, st := range body {
		if fd, ok := st.(*FunctionDeclaration); ok {
			g.emitFunctionHoisted(fd)
		}
	}
	for _, st := range body {
		g.emitStmt(st)
	}
	g.emit(Instruction{Op: OpReturnUndefined})
	return &FunctionBody{
		Name:         name,
		Params:       params,
		Instructions: g.code,
		NumLocals:    len(g.scopeLocals),
		IsArrow:      isArrow,
		IsAsync:      isAsync,
		IsGenerator:  isGenerator,
		RestParam:    rp,
	}
}

// BytecodeGenerator is the Go translation of JSC::BytecodeGenerator. It walks an AST and
// appends Instructions to an internal slice, patching jump targets after generation.
type BytecodeGenerator struct {
	code        []Instruction
	params      []string
	isArrow     bool
	scopeLocals map[string]bool
	loopStack   []*loopFrame
	switchStack []int   // break target indices for switch statements
	tryDepth    int
}

// loopFrame records jump targets for break/continue within a loop.
type loopFrame struct {
	breakTargets    []int
	continueTargets []int
	startPC         int
}

func newGenerator() *BytecodeGenerator {
	return &BytecodeGenerator{scopeLocals: make(map[string]bool)}
}

// declareVar records a local variable name.
func (g *BytecodeGenerator) declareVar(name string) {
	if g.scopeLocals == nil {
		g.scopeLocals = make(map[string]bool)
	}
	g.scopeLocals[name] = true
}

// emit appends an instruction and returns its index.
func (g *BytecodeGenerator) emit(inst Instruction) int {
	g.code = append(g.code, inst)
	return len(g.code) - 1
}

// emitJump emits a jump instruction with a placeholder target, returning the index for
// later patching.
func (g *BytecodeGenerator) emitJump(op Opcode) int {
	return g.emit(Instruction{Op: op, IntArg: -1})
}

// patchJump sets the target of the jump instruction at idx to the current code length.
func (g *BytecodeGenerator) patchJump(idx int) {
	g.code[idx].IntArg = len(g.code)
}

// here returns the index of the next instruction to be emitted.
func (g *BytecodeGenerator) here() int { return len(g.code) }

// emitFunctionHoisted emits a NewClosure and StoreVar for a hoisted function declaration.
func (g *BytecodeGenerator) emitFunctionHoisted(fd *FunctionDeclaration) {
	body := compileFunction(fd.Name, fd.Params, fd.Defaults, fd.Body, false, fd.IsAsync, fd.IsGenerator, nil, fd.RestParam)
	g.emit(Instruction{Op: OpNewClosure, Body: body, Name: fd.Name})
	g.emit(Instruction{Op: OpStoreVar, Name: fd.Name})
	g.emit(Instruction{Op: OpPop})
}

// emitStmt dispatches statement compilation by concrete type.
func (g *BytecodeGenerator) emitStmt(st Stmt) {
	if st == nil {
		return
	}
	switch n := st.(type) {
	case *ExpressionStatement:
		if n.Expr != nil {
			g.emitExpr(n.Expr)
			g.emit(Instruction{Op: OpPop})
		}
	case *VariableDeclaration:
		for _, d := range n.Declarators {
			g.declareVar(d.Name)
			if n.Kind == "const" {
				// const bindings are declared with their init value and marked
				// read-only in the environment so later assignments are no-ops.
				if d.Init != nil {
					g.emitExpr(d.Init)
				} else {
					g.emit(Instruction{Op: OpLoadUndefined})
				}
				g.emit(Instruction{Op: OpDeclareConst, Name: d.Name})
			} else if n.Kind == "let" {
				// let declarations must be declared in the current (block) scope,
				// otherwise OpStoreVar will find an outer binding and overwrite it.
				if d.Init != nil {
					g.emitExpr(d.Init)
				} else {
					g.emit(Instruction{Op: OpLoadUndefined})
				}
				g.emit(Instruction{Op: OpDeclareVar, Name: d.Name})
				g.emit(Instruction{Op: OpStoreVar, Name: d.Name})
				g.emit(Instruction{Op: OpPop})
			} else {
				// var: hoisted declaration, init only at point of definition.
				if d.Init != nil {
					g.emitExpr(d.Init)
					g.emit(Instruction{Op: OpStoreVar, Name: d.Name})
					g.emit(Instruction{Op: OpPop})
				}
			}
		}
	case *BlockStatement:
		g.emit(Instruction{Op: OpEnterScope})
		for _, s := range n.Body {
			g.emitStmt(s)
		}
		g.emit(Instruction{Op: OpLeaveScope})
	case *IfStatement:
		g.emitIf(n)
	case *ForStatement:
		g.emitFor(n)
	case *ForInStatement:
		g.emitForIn(n)
	case *WhileStatement:
		g.emitWhile(n)
	case *DoWhileStatement:
		g.emitDoWhile(n)
	case *SwitchStatement:
		g.emitSwitch(n)
	case *ReturnStatement:
		if n.Argument != nil {
			g.emitExpr(n.Argument)
		} else {
			g.emit(Instruction{Op: OpLoadUndefined})
		}
		g.emit(Instruction{Op: OpReturn})
	case *BreakStatement:
		if len(g.loopStack) > 0 {
			frame := g.loopStack[len(g.loopStack)-1]
			idx := g.emitJump(OpJump)
			frame.breakTargets = append(frame.breakTargets, idx)
		} else if len(g.switchStack) > 0 {
			idx := g.emitJump(OpJump)
			g.switchStack = append(g.switchStack, idx)
		}
		// If neither loop nor switch context, break is a no-op (outer context).
	case *ContinueStatement:
		if len(g.loopStack) == 0 {
			return
		}
		frame := g.loopStack[len(g.loopStack)-1]
		idx := g.emitJump(OpJump)
		frame.continueTargets = append(frame.continueTargets, idx)
	case *FunctionDeclaration:
		// Emit function declaration inline for block-scoped declarations.
		// Top-level declarations are also hoisted by CompileProgram, resulting in
		// a duplicate emission. The first (hoisted) StoreVar succeeds; the second
		// (inline) StoreVar overwrites the same binding with the same value, so
		// the duplicate is harmless.
		g.emitFunctionHoisted(n)
	case *ThrowStatement:
		g.emitExpr(n.Argument)
		g.emit(Instruction{Op: OpThrow})
	case *TryStatement:
		g.emitTry(n)
	case *ClassDeclaration:
		g.emitClass(n)
	case *ImportDeclaration:
		// import default from "mod": load module and store default binding.
		// Note: default import always loads the "default" export, not the local name.
		if n.DefaultName != "" {
			g.emit(Instruction{Op: OpImport, Name: n.Module, StrArg: "default"})
			g.emit(Instruction{Op: OpStoreVar, Name: n.DefaultName})
			g.emit(Instruction{Op: OpPop})
		}
		// import { a, b } from "mod": load each named export.
		for _, name := range n.NamedNames {
			g.emit(Instruction{Op: OpImport, Name: n.Module, StrArg: name})
			g.emit(Instruction{Op: OpStoreVar, Name: name})
			g.emit(Instruction{Op: OpPop})
		}
	case *ExportDeclaration:
		// export { a, b }: export named values.
		for _, name := range n.NamedNames {
			g.emit(Instruction{Op: OpLoadVar, Name: name})
			g.emit(Instruction{Op: OpExport, Name: name})
		}
		// export default expr: evaluate expr and export as "default".
		if n.DefaultExpr != nil {
			g.emitExpr(n.DefaultExpr)
			g.emit(Instruction{Op: OpExport, Name: "default"})
		}
	default:
		// Unknown statement kind: ignore.
	}
}

// emitIf compiles an if/else.
func (g *BytecodeGenerator) emitIf(n *IfStatement) {
	g.emitExpr(n.Test)
	jumpFalse := g.emitJump(OpJumpIfFalse)
	g.emitStmt(n.Consequent)
	if n.Alternate != nil {
		jumpEnd := g.emitJump(OpJump)
		g.patchJump(jumpFalse)
		g.emitStmt(n.Alternate)
		g.patchJump(jumpEnd)
	} else {
		g.patchJump(jumpFalse)
	}
}

// emitFor compiles a C-style for loop.
func (g *BytecodeGenerator) emitFor(n *ForStatement) {
	frame := &loopFrame{}
	g.loopStack = append(g.loopStack, frame)
	if n.Init != nil {
		g.emitStmt(n.Init)
	}
	loopStart := g.here()
	frame.startPC = loopStart
	frame.continueTargets = append(frame.continueTargets, loopStart)
	var exitJump int
	if n.Test != nil {
		g.emitExpr(n.Test)
		exitJump = g.emitJump(OpJumpIfFalse)
	} else {
		exitJump = -1
	}
	if n.Body != nil {
		g.emitStmt(n.Body)
	}
	if n.Update != nil {
		g.emitExpr(n.Update)
		g.emit(Instruction{Op: OpPop})
	}
	g.emitJumpTo(loopStart)
	if exitJump >= 0 {
		g.patchJump(exitJump)
	}
	for _, idx := range frame.breakTargets {
		g.patchJump(idx)
	}
	g.loopStack = g.loopStack[:len(g.loopStack)-1]
}

// emitJumpTo emits an unconditional jump to target.
func (g *BytecodeGenerator) emitJumpTo(target int) {
	g.emit(Instruction{Op: OpJump, IntArg: target})
}

// emitWhile compiles a while loop.
func (g *BytecodeGenerator) emitWhile(n *WhileStatement) {
	frame := &loopFrame{}
	g.loopStack = append(g.loopStack, frame)
	loopStart := g.here()
	frame.startPC = loopStart
	frame.continueTargets = append(frame.continueTargets, loopStart)
	g.emitExpr(n.Test)
	exitJump := g.emitJump(OpJumpIfFalse)
	if n.Body != nil {
		g.emitStmt(n.Body)
	}
	g.emitJumpTo(loopStart)
	g.patchJump(exitJump)
	for _, idx := range frame.breakTargets {
		g.patchJump(idx)
	}
	g.loopStack = g.loopStack[:len(g.loopStack)-1]
}

// emitSwitch compiles a switch statement as a chained if-else comparison.
// Each test+body pair is emitted sequentially with proper jump targets,
// matching JS engine behavior (fallthrough not yet supported — each body
// jumps to end after completion).
func (g *BytecodeGenerator) emitSwitch(n *SwitchStatement) {
	switchBreakIdx := len(g.switchStack)
	g.switchStack = append(g.switchStack, -1) // marker

	// ── Phase 1: stash discriminant ──
	g.emitExpr(n.Discriminant)
	discTemp := "__sw" + strconv.Itoa(switchBreakIdx)
	g.emit(Instruction{Op: OpDeclareVar, Name: discTemp})
	g.emit(Instruction{Op: OpStoreVar, Name: discTemp})
	g.emit(Instruction{Op: OpPop})

	// ── Phase 2: emit case test+body pairs (if-else chain) ──
	// Record the index of each case's first instruction (test) for patching fail jumps.
	caseStarts := make([]int, len(n.Cases))
	caseBodyEndJumps := make([]int, len(n.Cases)) // OpJump → end after each body
	failJumps := make([]int, len(n.Cases))        // OpJumpIfFalse after each test
	for i := range n.Cases {
		caseStarts[i] = -1
		caseBodyEndJumps[i] = -1
		failJumps[i] = -1
	}

	for i, c := range n.Cases {
		caseStarts[i] = g.here()

		if c.Test != nil {
			// Emit test: temp === testExpr
			g.emit(Instruction{Op: OpLoadVar, Name: discTemp})
			g.emitExpr(c.Test)
			g.emit(Instruction{Op: OpBinOp, OpTok: TokenStrictEqual})
			// If false (no match), skip this body → jump to next case
			failJumps[i] = g.emitJump(OpJumpIfFalse)
		}

		// Emit case body
		for _, st := range c.Body {
			g.emitStmt(st)
		}

		// After body, jump to end (prevents fallthrough into next case)
		caseBodyEndJumps[i] = len(g.code)
		g.emit(Instruction{Op: OpJump, IntArg: -1}) // patched to end below
	}

	// ── Phase 3: patch jumps ──
	endPos := g.here()

	// Patch each failJump to the start of the next case (skip this body)
	for i, c := range n.Cases {
		if c.Test == nil || failJumps[i] < 0 {
			continue
		}
		// Find the next case to jump to on test failure
		target := endPos
		for j := i + 1; j < len(n.Cases); j++ {
			if caseStarts[j] >= 0 {
				target = caseStarts[j]
				break
			}
		}
		g.code[failJumps[i]].IntArg = target
	}

	// Patch each body-end jump to endPos
	for _, idx := range caseBodyEndJumps {
		if idx >= 0 {
			g.code[idx].IntArg = endPos
		}
	}

	// Patch break statements inside this switch
	for _, idx := range g.switchStack[switchBreakIdx+1:] {
		if idx >= 0 && idx < len(g.code) {
			g.code[idx].IntArg = endPos
		}
	}
	g.switchStack = g.switchStack[:switchBreakIdx]
}

// emitDoWhile compiles a do-while loop.
func (g *BytecodeGenerator) emitDoWhile(n *DoWhileStatement) {
	frame := &loopFrame{}
	g.loopStack = append(g.loopStack, frame)
	loopStart := g.here()
	frame.startPC = loopStart
	frame.continueTargets = append(frame.continueTargets, loopStart)
	if n.Body != nil {
		g.emitStmt(n.Body)
	}
	g.emitExpr(n.Test)
	exitJump := g.emitJump(OpJumpIfFalse)
	g.emitJumpTo(loopStart)
	g.patchJump(exitJump)
	for _, idx := range frame.breakTargets {
		g.patchJump(idx)
	}
	g.loopStack = g.loopStack[:len(g.loopStack)-1]
}

// emitForIn compiles a for-in/for-of loop (string-key iteration only).
func (g *BytecodeGenerator) emitForIn(n *ForInStatement) {
	frame := &loopFrame{}
	g.loopStack = append(g.loopStack, frame)
	g.emitExpr(n.Right)
	isForOf := 0
	if n.IsOf {
		isForOf = 1
	}
	g.emit(Instruction{Op: OpBeginForIn, IntArg: isForOf})
	loopStart := g.here()
	frame.continueTargets = append(frame.continueTargets, loopStart)
	exhaustedJump := g.emitJump(OpForInNext)
	// Assign the current key to the loop variable.
	if len(n.Declarators) > 0 && n.IsOf {
		// for-of with destructuring: iterate value is array element
		if n.IsArrayDestruct {
			// Array destructuring [a,b]: a = iterValue[0], b = iterValue[1]
			for j, d := range n.Declarators {
				g.emit(Instruction{Op: OpDup})
				g.emit(Instruction{Op: OpLoadConst, Value: NumberValue(float64(j))})
				g.emit(Instruction{Op: OpLoadIndex})
				g.emit(Instruction{Op: OpStoreVar, Name: d.Name})
				g.emit(Instruction{Op: OpPop})
			}
		} else {
			// Object destructuring {a,b}: a = iterValue.a, b = iterValue.b
			for _, d := range n.Declarators {
				g.emit(Instruction{Op: OpDup})
				g.emit(Instruction{Op: OpLoadProp, Name: d.Name})
				g.emit(Instruction{Op: OpStoreVar, Name: d.Name})
				g.emit(Instruction{Op: OpPop})
			}
		}
		g.emit(Instruction{Op: OpPop}) // consume original iteration value
	} else if id, ok := n.Left.(*Identifier); ok {
		g.emit(Instruction{Op: OpStoreVar, Name: id.Name})
	} else if me, ok := n.Left.(*MemberExpression); ok {
		// member assignment handled via StoreProp/StoreIndex
		g.emitExpr(me.Object)
		g.emit(Instruction{Op: OpDup})
		g.emit(Instruction{Op: OpLoadConst, Value: StringValue(me.Name)})
		g.emit(Instruction{Op: OpStoreIndex})
	}
	if n.Body != nil {
		g.emitStmt(n.Body)
	}
	g.emitJumpTo(loopStart)
	g.patchJump(exhaustedJump)
	g.emit(Instruction{Op: OpEndForIn})
	for _, idx := range frame.breakTargets {
		g.patchJump(idx)
	}
	g.loopStack = g.loopStack[:len(g.loopStack)-1]
}

// emitTry compiles a try/catch/finally. The interpreter's handleThrow deactivates the
// try frame and pushes the caught value when jumping to the catch pad; OpLeaveTry pops
// the (deactivated) frame on both the normal and exception paths.
func (g *BytecodeGenerator) emitTry(n *TryStatement) {
	handlerIdx := g.emit(Instruction{Op: OpEnterTry, IntArg: -1})
	if n.Block != nil {
		g.emitStmt(n.Block)
	}
	// Normal completion: clear the handler and skip the catch pad.
	g.emit(Instruction{Op: OpLeaveTry})
	skipCatch := g.emitJump(OpJump)
	// Catch landing pad: the thrown value is already on the stack.
	if n.Handler != nil {
		g.code[handlerIdx].IntArg = g.here()
		if n.Handler.Param != "" {
			// StoreVar leaves the value on the stack; Pop removes the duplicate.
			g.emit(Instruction{Op: OpStoreVar, Name: n.Handler.Param})
			g.emit(Instruction{Op: OpPop})
		} else {
			g.emit(Instruction{Op: OpPop}) // discard caught value
		}
		if n.Handler.Body != nil {
			g.emitStmt(n.Handler.Body)
		}
		g.emit(Instruction{Op: OpLeaveTry}) // pop deactivated frame
	}
	// finally runs on the normal completion path (exception re-throw skips it).
	g.patchJump(skipCatch)
	if n.Finalizer != nil {
		g.emitStmt(n.Finalizer)
	}
}

func (g *BytecodeGenerator) emitClass(n *ClassDeclaration) {
	// Safety check
	if n == nil || n.Body == nil { return }
	if n.Name == "" { return }

	// Step 1: Load superclass if present (makes "super" available in scope)
	if n.SuperClass != nil {
		g.emitExpr(n.SuperClass)
		g.emit(Instruction{Op: OpDeclareVar, Name: "super"})
		g.emit(Instruction{Op: OpStoreVar, Name: "super"})
		g.emit(Instruction{Op: OpPop})
	}

	// Step 2: Find or generate constructor
	var ctorParams []string
	var ctorBody []Stmt
	var ctorRest string
	hasExplicitCtor := false
	for _, m := range n.Body.Methods {
		if m.Name == "constructor" {
			ctorParams = m.Params
			ctorBody = m.Body
			hasExplicitCtor = true
			break
		}
	}

	if n.SuperClass != nil && !hasExplicitCtor {
		// Default constructor: constructor(...args) { super(...args); }
		// Generate bytecode: push this, load super, load args[0], call via OpCallMethod
		g2 := newGenerator()
		g2.declareVar("args")
		// OpCallMethod expects: [this, methodFn, args...]
		g2.emit(Instruction{Op: OpLoadThis})       // push this (receiver)
		g2.emit(Instruction{Op: OpLoadVar, Name: "super"})  // push super function
		// Push args[0] (first argument)
		g2.emit(Instruction{Op: OpLoadVar, Name: "args"})
		g2.emit(Instruction{Op: OpLoadConst, Value: NumberValue(0)})
		g2.emit(Instruction{Op: OpLoadIndex})
		g2.emit(Instruction{Op: OpCallMethod, IntArg: 1, Name: "super"})
		g2.emit(Instruction{Op: OpPop})
		g2.emit(Instruction{Op: OpReturnUndefined})
		g2.emit(Instruction{Op: OpReturnUndefined})
		defBody := &FunctionBody{
			Name:         n.Name,
			Params:       []string{"args"},
			Instructions: g2.code,
			NumLocals:    1,
			RestParam:    "args",
		}
		g.emit(Instruction{Op: OpNewClosure, Body: defBody, Name: n.Name})
	} else {
		// Use explicit constructor or empty constructor for non-extending classes
		body := compileFunction(n.Name, ctorParams, nil, ctorBody, false, false, false, nil, ctorRest)
		g.emit(Instruction{Op: OpNewClosure, Body: body, Name: n.Name})
	}

	// Step 3: Store constructor as class variable
	g.declareVar(n.Name)
	g.emit(Instruction{Op: OpStoreVar, Name: n.Name})
	g.emit(Instruction{Op: OpPop})

	// Step 4: Attach prototype methods (non-static) to Constructor.prototype
	hasProtoMethods := false
	for _, m := range n.Body.Methods {
		if m.Name != "constructor" && !m.Static { hasProtoMethods = true; break }
	}
	if hasProtoMethods {
		g.emit(Instruction{Op: OpLoadVar, Name: n.Name})
		g.emit(Instruction{Op: OpLoadProp, Name: "prototype"})
		for _, m := range n.Body.Methods {
			if m.Name == "constructor" || m.Static { continue }
			// Reload prototype before each method to keep stack consistent regardless
			// of prior OpSetAccessor or OpStoreProp behavior.
			if m.Kind != "get" && m.Kind != "set" {
				// For regular methods, reload prototype each time (OpStoreProp consumes it).
				g.emit(Instruction{Op: OpLoadVar, Name: n.Name})
				g.emit(Instruction{Op: OpLoadProp, Name: "prototype"})
			}
			mbody := compileFunction(m.Name, m.Params, m.Defaults, m.Body, false, false, false, nil, "")
			g.emit(Instruction{Op: OpNewClosure, Body: mbody, Name: m.Name})
			if m.Kind == "get" || m.Kind == "set" {
				isSetter := 0
				if m.Kind == "set" { isSetter = 1 }
				g.emit(Instruction{Op: OpSetAccessor, Name: m.Name, IntArg: isSetter})
			} else {
				g.emit(Instruction{Op: OpStoreProp, Name: m.Name})
				g.emit(Instruction{Op: OpPop})
			}
		}
		g.emit(Instruction{Op: OpPop}) // pop prototype reference
	}

	// Step 5: Attach static methods to the constructor function itself
	for _, m := range n.Body.Methods {
		if m.Name == "constructor" || !m.Static { continue }
		g.emit(Instruction{Op: OpLoadVar, Name: n.Name})
		mbody := compileFunction(m.Name, m.Params, m.Defaults, m.Body, false, false, false, nil, "")
		g.emit(Instruction{Op: OpNewClosure, Body: mbody, Name: m.Name})
		g.emit(Instruction{Op: OpNewClosure, Body: mbody, Name: m.Name})
		g.emit(Instruction{Op: OpStoreVar, Name: "__static_" + m.Name})
		g.emit(Instruction{Op: OpPop}) // pop class
		g.emit(Instruction{Op: OpLoadVar, Name: n.Name})
		g.emit(Instruction{Op: OpLoadVar, Name: "__static_" + m.Name})
		g.emit(Instruction{Op: OpStoreProp, Name: m.Name})
		g.emit(Instruction{Op: OpPop}) // pop class
	}
}

// emitExpr dispatches expression compilation by concrete type.
func (g *BytecodeGenerator) emitExpr(e Expr) {
	if e == nil {
		g.emit(Instruction{Op: OpLoadUndefined})
		return
	}
	switch n := e.(type) {
	case *Literal:
		g.emit(Instruction{Op: OpLoadConst, Value: n.Value})
	case *Identifier:
		g.emit(Instruction{Op: OpLoadVar, Name: n.Name})
	case *ThisExpression:
		g.emit(Instruction{Op: OpLoadThis})
	case *BinaryExpression:
		g.emitExpr(n.Left)
		g.emitExpr(n.Right)
		g.emit(Instruction{Op: OpBinOp, OpTok: n.Op})
	case *LogicalExpression:
		g.emitLogical(n)
	case *UnaryExpression:
		g.emitExpr(n.Argument)
		g.emit(Instruction{Op: OpUnOp, OpTok: n.Op})
	case *UpdateExpression:
		g.emitUpdate(n)
	case *AssignmentExpression:
		g.emitAssignment(n)
	case *ConditionalExpression:
		g.emitExpr(n.Test)
		jf := g.emitJump(OpJumpIfFalse)
		g.emitExpr(n.Consequent)
		jend := g.emitJump(OpJump)
		g.patchJump(jf)
		g.emitExpr(n.Alternate)
		g.patchJump(jend)
	case *MemberExpression:
		g.emitExpr(n.Object)
		if n.Computed {
			g.emitExpr(n.Property)
			g.emit(Instruction{Op: OpLoadIndex})
		} else {
			g.emit(Instruction{Op: OpLoadProp, Name: n.Name})
		}
	case *CallExpression:
		g.emitCall(n)
	case *NewExpression:
		g.emitExpr(n.Callee)
		for _, a := range n.Arguments {
			g.emitExpr(a)
		}
		g.emit(Instruction{Op: OpNew, IntArg: len(n.Arguments)})
	case *ArrayExpression:
		for _, el := range n.Elements {
			if el == nil {
				g.emit(Instruction{Op: OpLoadUndefined})
			} else {
				g.emitExpr(el)
			}
		}
		g.emit(Instruction{Op: OpLoadArray, IntArg: len(n.Elements)})
	case *ObjectExpression:
		for _, prop := range n.Properties {
			if prop.Kind == "spread" {
				// Spread properties: evaluate the expression but ignore result for now
				g.emitExpr(prop.Spread)
				g.emit(Instruction{Op: OpPop})
				continue
			}
			// Property keys are names, not variable references. Identifier keys become
			// string constants; literal (string/number) keys pass through unchanged.
			if id, ok := prop.Key.(*Identifier); ok && !prop.Computed {
				g.emit(Instruction{Op: OpLoadConst, Value: StringValue(id.Name)})
			} else {
				g.emitExpr(prop.Key)
			}
			g.emitExpr(prop.Value)
		}
		g.emit(Instruction{Op: OpLoadObject, IntArg: len(n.Properties)})
	case *FunctionExpression:
		body := compileFunction(n.Name, n.Params, n.Defaults, n.Body, false, n.IsAsync, n.IsGenerator, nil)
		g.emit(Instruction{Op: OpNewClosure, Body: body, Name: n.Name})
	case *ArrowFunction:
		body := compileFunction("", n.Params, n.Defaults, stmtsFromNode(n.Body), true, n.IsAsync, n.IsGenerator, n.Destructuring)
		if n.IsExpr {
			// Wrap a concise-body expression so the function returns it.
			body = compileArrowExpr(n.Params, n.Body.(Expr), n.IsAsync, n.IsGenerator, n.Destructuring)
		}
		g.emit(Instruction{Op: OpNewClosure, Body: body})
	case *TemplateLiteral:
		g.emitTemplate(n)
	case *TaggedTemplateExpression:
		// Tagged template: emit tag function, then template args, then call.
		g.emitExpr(n.Tag)
		// Push template strings (quasis) and expressions as arguments.
		for _, q := range n.Template.Quasis {
			g.emit(Instruction{Op: OpLoadConst, Value: StringValue(q)})
		}
		for _, e := range n.Template.Expressions {
			g.emitExpr(e)
		}
		argCount := len(n.Template.Quasis) + len(n.Template.Expressions)
		g.emit(Instruction{Op: OpCall, IntArg: argCount})
	case *RegexLiteral:
		// Regex literal evaluates to a placeholder object: the source string.
		g.emit(Instruction{Op: OpLoadConst, Value: StringValue("/" + n.Pattern + "/" + n.Flags)})
	case *SequenceExpression:
		for i, ex := range n.Expressions {
			g.emitExpr(ex)
			if i < len(n.Expressions)-1 {
				g.emit(Instruction{Op: OpPop})
			}
		}
	case *SpreadExpression:
		g.emitExpr(n.Argument)
	case *AwaitExpression:
		g.emitExpr(n.Argument)
		g.emit(Instruction{Op: OpAwait})
	case *YieldExpression:
		if n.Argument != nil {
			g.emitExpr(n.Argument)
		} else {
			g.emit(Instruction{Op: OpLoadUndefined})
		}
		g.emit(Instruction{Op: OpYield})
	case *ClassDeclaration:
		// Class expressions: compile similarly to class declarations.
		g.emitClass(n)
	default:
		g.emit(Instruction{Op: OpLoadUndefined})
	}
}

// stmtsFromNode coerces an arrow body (Expr or Stmt) into a []Stmt.
func stmtsFromNode(body Node) []Stmt {
	if body == nil {
		return nil
	}
	if s, ok := body.(Stmt); ok {
		return []Stmt{s}
	}
	if e, ok := body.(Expr); ok {
		return []Stmt{&ReturnStatement{Argument: e}}
	}
	return nil
}

// compileArrowExpr compiles an arrow function with a concise expression body.
func compileArrowExpr(params []string, body Expr, isAsync bool, isGenerator bool, destructs []DestructInfo) *FunctionBody {
	g := newGenerator()
	g.params = params
	g.isArrow = true
	for _, p := range params {
		g.declareVar(p)
	}
	// Emit destructuring code for array/object params.
	for _, d := range destructs {
		if d.IsArray {
			for j, nm := range d.Names {
				g.emit(Instruction{Op: OpLoadVar, Name: params[d.ParamIdx]})
				g.emit(Instruction{Op: OpLoadConst, Value: NumberValue(float64(j))})
				g.emit(Instruction{Op: OpLoadIndex})
				g.emit(Instruction{Op: OpDeclareVar, Name: nm})
				g.emit(Instruction{Op: OpStoreVar, Name: nm})
				g.emit(Instruction{Op: OpPop})
			}
		} else {
			for _, nm := range d.Names {
				g.emit(Instruction{Op: OpLoadVar, Name: params[d.ParamIdx]})
				g.emit(Instruction{Op: OpLoadConst, Value: StringValue(nm)})
				g.emit(Instruction{Op: OpLoadIndex})
				g.emit(Instruction{Op: OpDeclareVar, Name: nm})
				g.emit(Instruction{Op: OpStoreVar, Name: nm})
				g.emit(Instruction{Op: OpPop})
			}
		}
	}
	g.emitExpr(body)
	g.emit(Instruction{Op: OpReturn})
	return &FunctionBody{Name: "<arrow>", Params: params, Instructions: g.code, NumLocals: len(g.scopeLocals), IsArrow: true, IsAsync: isAsync, IsGenerator: isGenerator}
}

// emitLogical compiles && || ?? with short-circuit jumps.
func (g *BytecodeGenerator) emitLogical(n *LogicalExpression) {
	g.emitExpr(n.Left)
	g.emit(Instruction{Op: OpDup})
	if n.Op == TokenCoalesce {
		// '??': if left is nullish, discard it and use right; else keep left.
		discard := g.emitJump(OpJumpIfNullish)
		skip := g.emitJump(OpJump)
		g.patchJump(discard)
		g.emit(Instruction{Op: OpPop})
		g.emitExpr(n.Right)
		g.patchJump(skip)
		return
	}
	// '&&'/'||': jump to end (keep left) when the short-circuit fires.
	var op Opcode
	switch n.Op {
	case TokenAnd:
		op = OpJumpIfFalse
	case TokenOr:
		op = OpJumpIfTrue
	}
	jump := g.emitJump(op)
	g.emit(Instruction{Op: OpPop})
	g.emitExpr(n.Right)
	g.patchJump(jump)
}

// emitUpdate compiles ++/-- for identifier and member targets. The store operations
// leave the assigned value on the stack; for postfix the OLD value is left instead.
func (g *BytecodeGenerator) emitUpdate(n *UpdateExpression) {
	binTok := TokenPlus
	if n.Op == TokenDecrement {
		binTok = TokenMinus
	}
	switch tgt := n.Argument.(type) {
	case *Identifier:
		if n.Prefix {
			// ++x: load x, +1, store. StoreVar leaves the new value on the stack.
			g.emit(Instruction{Op: OpLoadVar, Name: tgt.Name})
			g.emit(Instruction{Op: OpLoadConst, Value: NumberValue(1)})
			g.emit(Instruction{Op: OpBinOp, OpTok: binTok})
			g.emit(Instruction{Op: OpStoreVar, Name: tgt.Name})
		} else {
			// x++: load x, dup, +1, store, pop. Leaves old value.
			g.emit(Instruction{Op: OpLoadVar, Name: tgt.Name})
			g.emit(Instruction{Op: OpDup})
			g.emit(Instruction{Op: OpLoadConst, Value: NumberValue(1)})
			g.emit(Instruction{Op: OpBinOp, OpTok: binTok})
			g.emit(Instruction{Op: OpStoreVar, Name: tgt.Name})
			g.emit(Instruction{Op: OpPop})
		}
	case *MemberExpression:
		if tgt.Computed {
			// Computed member: obj, key, dup2, load, +1, store.
			g.emitExpr(tgt.Object)
			g.emitExpr(tgt.Property)
			if n.Prefix {
				g.emit(Instruction{Op: OpDup2})
				g.emit(Instruction{Op: OpLoadIndex})
				g.emit(Instruction{Op: OpLoadConst, Value: NumberValue(1)})
				g.emit(Instruction{Op: OpBinOp, OpTok: binTok})
				g.emit(Instruction{Op: OpStoreIndex})
			} else {
				g.emit(Instruction{Op: OpDup2})
				g.emit(Instruction{Op: OpLoadIndex})
				g.emit(Instruction{Op: OpLoadConst, Value: NumberValue(1)})
				g.emit(Instruction{Op: OpBinOp, OpTok: binTok})
				g.emit(Instruction{Op: OpStoreIndex})
				g.emit(Instruction{Op: OpPop})
			}
			return
		}
		// Non-computed: obj, dup, load prop, +1, store prop.
		g.emitExpr(tgt.Object)
		if n.Prefix {
			g.emit(Instruction{Op: OpDup})
			g.emit(Instruction{Op: OpLoadProp, Name: tgt.Name})
			g.emit(Instruction{Op: OpLoadConst, Value: NumberValue(1)})
			g.emit(Instruction{Op: OpBinOp, OpTok: binTok})
			g.emit(Instruction{Op: OpStoreProp, Name: tgt.Name})
		} else {
			g.emit(Instruction{Op: OpDup})
			g.emit(Instruction{Op: OpLoadProp, Name: tgt.Name})
			g.emit(Instruction{Op: OpLoadConst, Value: NumberValue(1)})
			g.emit(Instruction{Op: OpBinOp, OpTok: binTok})
			g.emit(Instruction{Op: OpStoreProp, Name: tgt.Name})
			g.emit(Instruction{Op: OpPop})
		}
	default:
		g.emitExpr(n.Argument)
	}
}

// emitAssignment compiles simple and compound assignments. Store operations consume the
// value and push it back, so the assigned value is left on the stack for the expression
// result.
func (g *BytecodeGenerator) emitAssignment(n *AssignmentExpression) {
	if n.Op == TokenAssign {
		switch tgt := n.Target.(type) {
		case *Identifier:
			g.emitExpr(n.Value)
			g.emit(Instruction{Op: OpStoreVar, Name: tgt.Name})
		case *MemberExpression:
			if tgt.Computed {
				g.emitExpr(tgt.Object)
				g.emitExpr(tgt.Property)
				g.emitExpr(n.Value)
				g.emit(Instruction{Op: OpStoreIndex})
			} else {
				g.emitExpr(tgt.Object)
				g.emitExpr(n.Value)
				g.emit(Instruction{Op: OpStoreProp, Name: tgt.Name})
			}
		default:
			g.emitExpr(n.Value)
			g.emit(Instruction{Op: OpPop})
		}
		return
	}
	// Compound assignment: load current, apply binop, store.
	binOp := compoundBinaryOp(n.Op)
	switch tgt := n.Target.(type) {
	case *Identifier:
		g.emit(Instruction{Op: OpLoadVar, Name: tgt.Name})
		g.emitExpr(n.Value)
		g.emit(Instruction{Op: OpBinOp, OpTok: binOp})
		g.emit(Instruction{Op: OpStoreVar, Name: tgt.Name})
	case *MemberExpression:
		if tgt.Computed {
			g.emitExpr(tgt.Object)
			g.emitExpr(tgt.Property)
			g.emit(Instruction{Op: OpDup2})
			g.emit(Instruction{Op: OpLoadIndex})
			g.emitExpr(n.Value)
			g.emit(Instruction{Op: OpBinOp, OpTok: binOp})
			g.emit(Instruction{Op: OpStoreIndex})
		} else {
			g.emitExpr(tgt.Object)
			g.emit(Instruction{Op: OpDup})
			g.emit(Instruction{Op: OpLoadProp, Name: tgt.Name})
			g.emitExpr(n.Value)
			g.emit(Instruction{Op: OpBinOp, OpTok: binOp})
			g.emit(Instruction{Op: OpStoreProp, Name: tgt.Name})
		}
	default:
		g.emitExpr(n.Value)
		g.emit(Instruction{Op: OpPop})
	}
}

// compoundBinaryOp maps a compound assignment token to its binary equivalent.
func compoundBinaryOp(op TokenKind) TokenKind {
	switch op {
	case TokenPlusAssign:
		return TokenPlus
	case TokenMinusAssign:
		return TokenMinus
	case TokenMultAssign:
		return TokenStar
	case TokenDivAssign:
		return TokenSlash
	case TokenModAssign:
		return TokenPercent
	case TokenPowAssign:
		return TokenPower
	case TokenBitAndAssign:
		return TokenBitAnd
	case TokenBitOrAssign:
		return TokenBitOr
	case TokenBitXorAssign:
		return TokenBitXor
	case TokenLShiftAssign:
		return TokenLeftShift
	case TokenRShiftAssign:
		return TokenRightShift
	case TokenURShiftAssign:
		return TokenUnsignedRightShift
	case TokenCoalesceAssign:
		return TokenCoalesce
	case TokenOrAssign:
		return TokenOr
	case TokenAndAssign:
		return TokenAnd
	}
	return TokenPlus
}

// emitCall compiles a call expression. Method calls (obj.method()) are detected so the
// interpreter can supply the correct 'this' binding.
func (g *BytecodeGenerator) emitCall(n *CallExpression) {
	if me, ok := n.Callee.(*MemberExpression); ok && !me.Optional {
		// Method call: push object, load property, push args, OpCallMethod.
		g.emitExpr(me.Object)
		g.emit(Instruction{Op: OpDup})
		if me.Computed {
			g.emitExpr(me.Property)
			g.emit(Instruction{Op: OpLoadIndex})
		} else {
			g.emit(Instruction{Op: OpLoadProp, Name: me.Name})
		}
		for _, a := range n.Arguments {
			g.emitExpr(a)
		}
		g.emit(Instruction{Op: OpCallMethod, IntArg: len(n.Arguments), Name: me.Name})
		return
	}
	// Regular call. Check for super() which needs to call parent constructor on current this
	if id, ok := n.Callee.(*Identifier); ok && id.Name == "super" {
		// super() in a constructor: need to call parent constructor with current this.
		// Push current this (as receiver), load super function, push args, then use OpCallMethod.
		g.emit(Instruction{Op: OpLoadThis})
		g.emit(Instruction{Op: OpLoadVar, Name: "super"})
		for _, a := range n.Arguments {
			g.emitExpr(a)
		}
		g.emit(Instruction{Op: OpCallMethod, IntArg: len(n.Arguments), Name: "super"})
		return
	}
	g.emitExpr(n.Callee)
	for _, a := range n.Arguments {
		g.emitExpr(a)
	}
	g.emit(Instruction{Op: OpCall, IntArg: len(n.Arguments)})
}

// emitTemplate compiles a template literal by interleaving quasis and expressions.
func (g *BytecodeGenerator) emitTemplate(n *TemplateLiteral) {
	if len(n.Expressions) == 0 {
		if len(n.Quasis) > 0 {
			g.emit(Instruction{Op: OpLoadConst, Value: StringValue(n.Quasis[0])})
		} else {
			g.emit(Instruction{Op: OpLoadConst, Value: StringValue("")})
		}
		return
	}
	// Push quasi[0], expr[0], quasi[1], ... and join.
	for i, ex := range n.Expressions {
		g.emit(Instruction{Op: OpLoadConst, Value: StringValue(n.Quasis[i])})
		g.emitExpr(ex)
	}
	tail := ""
	if len(n.Quasis) > len(n.Expressions) {
		tail = n.Quasis[len(n.Expressions)]
	}
	g.emit(Instruction{Op: OpLoadConst, Value: StringValue(tail)})
	g.emit(Instruction{Op: OpTemplateJoin, IntArg: len(n.Expressions)})
}
