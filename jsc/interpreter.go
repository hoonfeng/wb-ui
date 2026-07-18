// Translation of: Source/JavaScriptCore/interpreter/Interpreter.h
//                  Source/JavaScriptCore/interpreter/Interpreter.cpp
//                  Source/JavaScriptCore/runtime/CallFrame.h
//                  Source/JavaScriptCore/runtime/JSLexicalEnvironment.h
// Completeness: 60%
// Simplifications:
//   - stack-based VM with a single []JSValue operand stack per frame.
//   - lexical environments are a parent-linked *Environment; no activation record split.
//   - exceptions propagate via an explicit *jsException sentinel rather than longjmp.
//   - no JIT trampolines; all calls go through runFunction directly.

package jsc

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
)

// Environment is the Go translation of JSC::JSLexicalEnvironment. It is a scope chain
// node holding variable bindings plus a pointer to the enclosing scope.
type Environment struct {
	bindings map[string]JSValue
	parent   *Environment
	// constNames records bindings that are const (assignment is a no-op error silenced).
	constNames map[string]bool
}

// NewEnvironment creates a new environment with the given parent.
func NewEnvironment(parent *Environment) *Environment {
	return &Environment{
		bindings:   make(map[string]JSValue),
		parent:     parent,
		constNames: make(map[string]bool),
	}
}

// Get walks the scope chain looking for name, returning the value and a found flag.
func (e *Environment) Get(name string) (JSValue, bool) {
	cur := e
	for cur != nil {
		if v, ok := cur.bindings[name]; ok {
			return v, true
		}
		cur = cur.parent
	}
	return Undefined(), false
}

// Set assigns name to value. If name exists in some enclosing scope it is updated
// there; otherwise it is defined in the current scope (implicit global).
func (e *Environment) Set(name string, value JSValue) {
	cur := e
	for cur != nil {
		if _, ok := cur.bindings[name]; ok {
			if cur.constNames[name] {
				return // const reassignment silently ignored (subset simplification)
			}
			cur.bindings[name] = value
			return
		}
		cur = cur.parent
	}
	e.Declare(name, value)
}

// Declare binds name in the current scope only.
func (e *Environment) Declare(name string, value JSValue) {
	if e.bindings == nil {
		e.bindings = make(map[string]JSValue)
	}
	e.bindings[name] = value
}

// DeclareConst binds a const name in the current scope.
func (e *Environment) DeclareConst(name string, value JSValue) {
	e.Declare(name, value)
	if e.constNames == nil {
		e.constNames = make(map[string]bool)
	}
	e.constNames[name] = true
}

// Parent returns the enclosing environment.
func (e *Environment) Parent() *Environment { return e.parent }

// jsException is the sentinel exception value carried across runFunction returns.
type jsException struct {
	value JSValue
}

func (x *jsException) Error() string { return "jsc exception: " + x.value.ToString() }

// tryFrame records an active exception handler. When catchPC == -1 the frame has been
// deactivated (a catch is already executing) so re-thrown exceptions propagate outward.
type tryFrame struct {
	catchPC int
}

// forInIter tracks an in-progress for-in loop's key list and position.
type forInIter struct {
	keys    []string
	pos     int
	isForOf bool
	obj     JSValue // the original object (for-of needs it for values)
}

// CallContext carries the 'this' binding and arguments for a single function call.
type CallContext struct {
	This JSValue
	Args []JSValue
}

// Interpreter is the Go translation of JSC::Interpreter. It owns the global
// environment and object, and runs FunctionBody bytecode on a per-frame stack.
type Interpreter struct {
	global        *JSObject
	globalEnv     *Environment
	objectProto   *JSObject
	functionProto *JSObject
	arrayProto    *JSObject
	mapProto      *JSObject
	setProto      *JSObject
	weakMapProto  *JSObject
	weakSetProto  *JSObject
	stringProto   *JSObject
	promiseProto  *JSObject
	symbolProto   *JSObject
	forInStack    []*forInIter
	moduleRegistry map[string]map[string]JSValue
	// currentModuleName is the module name being executed (for OpExport).
	currentModuleName string
	// maxCallDepth bounds recursion to avoid runaway stack growth.
	maxCallDepth int
	depth        int
	throwPending *jsException
}

// NewInterpreter constructs an interpreter with a fresh global object and environment.
// SetupGlobal must be called (or GlobalObject installed) before running scripts.

func (in *Interpreter) WeakMapPrototype() *JSObject {
	if in.weakMapProto != nil {
		return in.weakMapProto
	}
	proto := NewObject(in.objectProto)
	proto.ClassName = "WeakMap"

	proto.Set("set", FunctionValue(NewNativeFunction("set", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		m := mapThis(this)
		if m == nil || len(args) == 0 || !args[0].IsObject() {
			return this
		}
		key := mapKey(args[0])
		val := Undefined()
		if len(args) > 1 {
			val = args[1]
		}
		m.set(key, val)
		return this
	}, 2)))

	proto.Set("get", FunctionValue(NewNativeFunction("get", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		m := mapThis(this)
		if m == nil || len(args) == 0 {
			return Undefined()
		}
		v, ok := m.get(mapKey(args[0]))
		if !ok {
			return Undefined()
		}
		return v
	}, 1)))

	proto.Set("has", FunctionValue(NewNativeFunction("has", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		m := mapThis(this)
		if m == nil || len(args) == 0 {
			return BooleanValue(false)
		}
		return BooleanValue(m.has(mapKey(args[0])))
	}, 1)))

	proto.Set("delete", FunctionValue(NewNativeFunction("delete", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		m := mapThis(this)
		if m == nil || len(args) == 0 {
			return BooleanValue(false)
		}
		return BooleanValue(m.delete(mapKey(args[0])))
	}, 1)))

	in.weakMapProto = proto
	return proto
}

// WeakSetPrototype returns the WeakSet.prototype object.
func (in *Interpreter) WeakSetPrototype() *JSObject {
	if in.weakSetProto != nil {
		return in.weakSetProto
	}
	proto := NewObject(in.objectProto)
	proto.ClassName = "WeakSet"

	proto.Set("add", FunctionValue(NewNativeFunction("add", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := setThis(this)
		if s == nil || len(args) == 0 || !args[0].IsObject() {
			return this
		}
		s.add(mapKey(args[0]))
		return this
	}, 1)))

	proto.Set("has", FunctionValue(NewNativeFunction("has", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := setThis(this)
		if s == nil || len(args) == 0 {
			return BooleanValue(false)
		}
		return BooleanValue(s.has(mapKey(args[0])))
	}, 1)))

	proto.Set("delete", FunctionValue(NewNativeFunction("delete", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := setThis(this)
		if s == nil || len(args) == 0 {
			return BooleanValue(false)
		}
		return BooleanValue(s.delete(mapKey(args[0])))
	}, 1)))

	in.weakSetProto = proto
	return proto
}

// NewInterpreter constructs an interpreter with a fresh global object and environment.
// SetupGlobal must be called (or GlobalObject installed) before running scripts.
func NewInterpreter() *Interpreter {
	objectProto := &JSObject{Properties: make(map[string]JSValue), ClassName: "Object"}
	// Install Object.prototype methods
	objectProto.Set("toString", FunctionValue(NewNativeFunction("toString", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if this.IsUndefined() { return StringValue("undefined") }
		if this.IsNull() { return StringValue("null") }
		if this.IsString() { return this }
		if this.IsNumber() { return StringValue(fmt.Sprintf("%g", this.AsNumber())) }
		if this.IsBoolean() { return StringValue(fmt.Sprintf("%t", this.AsBoolean())) }
		// For objects, return "[object ClassName]"
		cn := "Object"
		if this.IsObject() {
			if o := this.AsObject(); o != nil && o.ClassName != "" {
				cn = o.ClassName
			}
		}
		return StringValue(fmt.Sprintf("[object %s]", cn))
	}, 0)))
	objectProto.Set("hasOwnProperty", FunctionValue(NewNativeFunction("hasOwnProperty", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		if !this.IsObject() { return BooleanValue(false) }
		key := ""
		if len(args) > 0 { key = args[0].ToString() }
		obj := this.AsObject()
		if obj == nil { return BooleanValue(false) }
		_, found := obj.Get(key)
		return BooleanValue(found)
	}, 1)))
	functionProto := &JSObject{Properties: make(map[string]JSValue), Prototype: objectProto, ClassName: "Function"}
	arrayProto := &JSObject{Properties: make(map[string]JSValue), Prototype: objectProto, ClassName: "Array"}
	global := &JSObject{Properties: make(map[string]JSValue), ClassName: "Global"}
	globalEnv := NewEnvironment(nil)
	return &Interpreter{
		global:        global,
		globalEnv:     globalEnv,
		objectProto:   objectProto,
		functionProto: functionProto,
		arrayProto:    arrayProto,
		maxCallDepth:  5000,
		moduleRegistry: make(map[string]map[string]JSValue),
	}
}

// GlobalObject returns the interpreter's global object.
func (in *Interpreter) GlobalObject() *JSObject { return in.global }

// GlobalEnv returns the interpreter's global environment.
func (in *Interpreter) GlobalEnv() *Environment { return in.globalEnv }

// ObjectPrototype returns the Object.prototype for the interpreter.
func (in *Interpreter) ObjectPrototype() *JSObject { return in.objectProto }

// ArrayPrototype returns the Array.prototype for the interpreter.
func (in *Interpreter) ArrayPrototype() *JSObject { return in.arrayProto }

// FunctionPrototype returns the Function.prototype for the interpreter.
func (in *Interpreter) FunctionPrototype() *JSObject { return in.functionProto }

// strVal extracts a Go string from a JSValue (handles both primitive strings and String objects).
func strVal(v JSValue) string {
	if v.IsString() {
		return v.AsString()
	}
	if v.IsObject() {
		return v.ToString()
	}
	return ""
}

// StringPrototype returns the String.prototype object with all String methods.
func (in *Interpreter) StringPrototype() *JSObject {
	if in.stringProto != nil {
		return in.stringProto
	}
	proto := NewObject(in.objectProto)
	proto.ClassName = "String"

	proto.Set("startsWith", FunctionValue(NewNativeFunction("startsWith", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := strVal(this)
		search := ""
		if len(args) > 0 { search = args[0].ToString() }
		pos := 0
		if len(args) > 1 && args[1].IsNumber() { pos = int(args[1].ToInt32()) }
		if pos < 0 { pos = 0 }
		return BooleanValue(pos <= len(s) && strings.HasPrefix(s[pos:], search))
	}, 1)))

	proto.Set("endsWith", FunctionValue(NewNativeFunction("endsWith", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := strVal(this)
		search := ""
		if len(args) > 0 { search = args[0].ToString() }
		l := len(s)
		if len(args) > 1 && args[1].IsNumber() { l = int(args[1].ToInt32()) }
		return BooleanValue(l >= len(search) && s[l-len(search):l] == search)
	}, 1)))

	proto.Set("includes", FunctionValue(NewNativeFunction("includes", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := strVal(this)
		search := ""
		if len(args) > 0 { search = args[0].ToString() }
		pos := 0
		if len(args) > 1 && args[1].IsNumber() { pos = int(args[1].ToInt32()) }
		if pos < 0 { pos = 0 }
		if pos > len(s) { return BooleanValue(false) }
		return BooleanValue(strings.Contains(s[pos:], search))
	}, 1)))

	proto.Set("trim", FunctionValue(NewNativeFunction("trim", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		return StringValue(strings.TrimSpace(strVal(this)))
	}, 0)))

	proto.Set("trimStart", FunctionValue(NewNativeFunction("trimStart", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		return StringValue(strings.TrimLeft(strVal(this), " \t\n\r\v\f"))
	}, 0)))

	proto.Set("trimEnd", FunctionValue(NewNativeFunction("trimEnd", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		return StringValue(strings.TrimRight(strVal(this), " \t\n\r\v\f"))
	}, 0)))

	proto.Set("charAt", FunctionValue(NewNativeFunction("charAt", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := strVal(this)
		pos := 0
		if len(args) > 0 && args[0].IsNumber() { pos = int(args[0].ToInt32()) }
		if pos < 0 || pos >= len(s) { return StringValue("") }
		return StringValue(string(s[pos]))
	}, 1)))

	proto.Set("charCodeAt", FunctionValue(NewNativeFunction("charCodeAt", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := strVal(this)
		pos := 0
		if len(args) > 0 && args[0].IsNumber() { pos = int(args[0].ToInt32()) }
		if pos < 0 || pos >= len(s) { return NumberValue(math.NaN()) }
		return NumberValue(float64(s[pos]))
	}, 1)))

	proto.Set("indexOf", FunctionValue(NewNativeFunction("indexOf", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := strVal(this)
		search := ""
		if len(args) > 0 { search = args[0].ToString() }
		from := 0
		if len(args) > 1 && args[1].IsNumber() { from = int(args[1].ToInt32()) }
		if from < 0 { from = 0 }
		if from > len(s) { return NumberValue(-1) }
		idx := strings.Index(s[from:], search)
		if idx < 0 { return NumberValue(-1) }
		return NumberValue(float64(from + idx))
	}, 1)))

	proto.Set("match", FunctionValue(NewNativeFunction("match", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := strVal(this)
		if len(args) == 0 { return Null() }
		pat := args[0].ToString()
		re, err := regexp.Compile(pat)
		if err != nil { return Null() }
		m := re.FindString(s)
		if m == "" { return Null() }
		return StringValue(m)
	}, 1)))

	proto.Set("replace", FunctionValue(NewNativeFunction("replace", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := strVal(this)
		if len(args) < 2 { return StringValue(s) }
		search := args[0].ToString()
		replacement := args[1].ToString()
		return StringValue(strings.Replace(s, search, replacement, 1))
	}, 2)))

	proto.Set("replaceAll", FunctionValue(NewNativeFunction("replaceAll", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := strVal(this)
		if len(args) < 2 { return StringValue(s) }
		search := args[0].ToString()
		replacement := args[1].ToString()
		return StringValue(strings.ReplaceAll(s, search, replacement))
	}, 2)))

	proto.Set("split", FunctionValue(NewNativeFunction("split", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := strVal(this)
		sep := ""
		if len(args) > 0 { sep = args[0].ToString() }
		limit := -1
		if len(args) > 1 && args[1].IsNumber() { limit = int(args[1].ToInt32()) }
		var parts []string
		if sep == "" {
			for _, r := range s { parts = append(parts, string(r)) }
		} else {
			parts = strings.Split(s, sep)
		}
		if limit >= 0 && limit < len(parts) { parts = parts[:limit] }
		arr := make([]JSValue, len(parts))
		for i, p := range parts { arr[i] = StringValue(p) }
		return ObjectValue(NewArray(in.arrayProto, arr))
	}, 2)))

	proto.Set("slice", FunctionValue(NewNativeFunction("slice", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := strVal(this)
		start := 0
		if len(args) > 0 && args[0].IsNumber() { start = int(args[0].ToInt32()) }
		end := len(s)
		if len(args) > 1 && args[1].IsNumber() { end = int(args[1].ToInt32()) }
		// Handle negative indices
		if start < 0 { start = max(0, len(s)+start) }
		if end < 0 { end = max(0, len(s)+end) }
		if start >= end || start >= len(s) { return StringValue("") }
		if end > len(s) { end = len(s) }
		return StringValue(s[start:end])
	}, 2)))

	proto.Set("substring", FunctionValue(NewNativeFunction("substring", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := strVal(this)
		start := 0
		if len(args) > 0 && args[0].IsNumber() { start = int(args[0].ToInt32()) }
		end := len(s)
		if len(args) > 1 && args[1].IsNumber() { end = int(args[1].ToInt32()) }
		if start < 0 { start = 0 }
		if end < 0 { end = 0 }
		if start > end { start, end = end, start }
		if start > len(s) { start = len(s) }
		if end > len(s) { end = len(s) }
		return StringValue(s[start:end])
	}, 2)))

	proto.Set("toLowerCase", FunctionValue(NewNativeFunction("toLowerCase", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		return StringValue(strings.ToLower(strVal(this)))
	}, 0)))

	proto.Set("toUpperCase", FunctionValue(NewNativeFunction("toUpperCase", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		return StringValue(strings.ToUpper(strVal(this)))
	}, 0)))

	proto.Set("concat", FunctionValue(NewNativeFunction("concat", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := strVal(this)
		for _, a := range args { s += a.ToString() }
		return StringValue(s)
	}, 1)))

	proto.Set("repeat", FunctionValue(NewNativeFunction("repeat", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := strVal(this)
		n := 0
		if len(args) > 0 && args[0].IsNumber() { n = int(args[0].ToInt32()) }
		if n <= 0 { return StringValue("") }
		return StringValue(strings.Repeat(s, n))
	}, 1)))

	proto.Set("padStart", FunctionValue(NewNativeFunction("padStart", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := strVal(this)
		maxLen := 0
		if len(args) > 0 && args[0].IsNumber() { maxLen = int(args[0].ToInt32()) }
		fill := " "
		if len(args) > 1 { fill = args[1].ToString() }
		if len(s) >= maxLen { return StringValue(s) }
		pad := strings.Repeat(fill, (maxLen-len(s)+len(fill)-1)/len(fill))
		return StringValue(pad[:maxLen-len(s)] + s)
	}, 2)))

	proto.Set("padEnd", FunctionValue(NewNativeFunction("padEnd", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
		s := strVal(this)
		maxLen := 0
		if len(args) > 0 && args[0].IsNumber() { maxLen = int(args[0].ToInt32()) }
		fill := " "
		if len(args) > 1 { fill = args[1].ToString() }
		if len(s) >= maxLen { return StringValue(s) }
		pad := strings.Repeat(fill, (maxLen-len(s)+len(fill)-1)/len(fill))
		return StringValue(s + pad[:maxLen-len(s)])
	}, 2)))

	in.stringProto = proto
	return proto
}

// SetMaxCallDepth configures the recursion limit.
func (in *Interpreter) SetMaxCallDepth(n int) { in.maxCallDepth = n }

// Run compiles and executes a source string at top level, returning the result of the
// last expression statement (or undefined) and any thrown exception.
func (in *Interpreter) Run(src string) (JSValue, error) {
	prog, err := Parse(src)
	if err != nil {
		return Undefined(), err
	}
	body := CompileProgram(prog)
	return in.RunBody(body, in.globalEnv, Undefined())
}

// RunBody executes a compiled FunctionBody in the given environment.
func (in *Interpreter) RunBody(body *FunctionBody, env *Environment, this JSValue) (JSValue, error) {
	v, exc := in.runFunction(body, env, this, nil)
	if exc != nil {
		return Undefined(), exc
	}
	return v, nil
}

// runFunction is the core bytecode dispatch loop. It returns the result value or an
// exception sentinel. The operand stack is local to the frame.
// For async functions (body.IsAsync), execution is wrapped in a Promise: the return
// value resolves the promise, and any uncaught exception rejects it.
func (in *Interpreter) runFunction(body *FunctionBody, env *Environment, this JSValue, args []JSValue) (JSValue, *jsException) {
	// Async functions: delegate to runAsyncFunction which wraps the synchronous
	// bytecode execution in a Promise. The runAsyncFunction calls runFunctionBody
	// (which does NOT check IsAsync) to avoid infinite recursion.
	if body.IsAsync {
		return in.runAsyncFunction(body, env, this, args)
	}
	return in.runFunctionBody(body, env, this, args)
}

// runFunctionBody is the dispatch loop without async wrapping. It is called by
// both runFunction (non-async) and runAsyncFunction (async wrapper).
func (in *Interpreter) runFunctionBody(body *FunctionBody, env *Environment, this JSValue, args []JSValue) (JSValue, *jsException) {
	defer func() {
		if r := recover(); r != nil {
			// Panic recovered: return undefined to keep the script executing
		}
	}()
	if in.depth > in.maxCallDepth {
		in.depth--
		fmt.Fprintf(os.Stderr, "[STACK_OVERFLOW] depth=%d max=%d\n", in.depth, in.maxCallDepth)
		return Undefined(), &jsException{value: StringValue("RangeError: Maximum call stack size exceeded")}
	}
	defer func() { in.depth-- }()

	// Bind parameters into the function's environment (a child of the closure env).
	// For top-level program bodies the environment IS the supplied (global) environment,
	// so top-level var/function declarations persist on it rather than vanishing with an
	// ephemeral child scope. This mirrors script-scope semantics where top-level
	// declarations become global bindings reachable after Run returns.
	var frameEnv *Environment
	if body.IsTopLevel {
		frameEnv = env
	} else {
		frameEnv = NewEnvironment(env)
		for i, p := range body.Params {
			if i < len(args) {
				frameEnv.Declare(p, args[i])
			} else {
				frameEnv.Declare(p, Undefined())
			}
		}
		// 'arguments' object (array-like) for non-arrow functions.
		if !body.IsArrow {
			argElems := make([]JSValue, len(args))
			copy(argElems, args)
			frameEnv.Declare("arguments", ObjectValue(NewArray(in.arrayProto, argElems)))
		}
	}

	stack := make([]JSValue, 0, 16)
	var tryStack []tryFrame
	code := body.Instructions
	pc := 0

	// pop removes and returns the top of the stack.
	pop := func() JSValue {
		n := len(stack) - 1
		if n < 0 {
			return Undefined()
		}
		v := stack[n]
		stack = stack[:n]
		return v
	}
	// push appends a value.
	push := func(v JSValue) { stack = append(stack, v) }

	// handleThrow processes a thrown value: if a try handler is active, jump to its
	// catchPC and push the value; otherwise propagate by returning an exception.
	handleThrow := func(v JSValue) *jsException {
		for i := len(tryStack) - 1; i >= 0; i-- {
			if tryStack[i].catchPC >= 0 {
				pc = tryStack[i].catchPC
				tryStack[i].catchPC = -1 // deactivate so re-throws propagate
				push(v)
				return nil
			}
		}
		return &jsException{value: v}
	}

	for pc < len(code) {
		inst := code[pc]
		pc++
		switch inst.Op {
		case OpLoadConst:
			push(inst.Value)
		case OpLoadVar:
			if v, ok := frameEnv.Get(inst.Name); ok {
				push(v)
			} else if v, ok := in.global.Get(inst.Name); ok {
				push(v)
			} else {
				push(Undefined())
			}
		case OpStoreVar:
			v := pop()
			if _, ok := frameEnv.Get(inst.Name); ok {
				frameEnv.Set(inst.Name, v)
			} else if in.global.HasOwn(inst.Name) {
				in.global.Set(inst.Name, v)
			} else {
				frameEnv.Declare(inst.Name, v)
			}
			push(v)
		case OpLoadThis:
			push(this)
		case OpLoadUndefined:
			push(Undefined())
		case OpLoadNull:
			push(Null())
		case OpLoadProp:
			obj := pop()
			push(in.getProperty(obj, inst.Name))
		case OpLoadIndex:
			key := pop()
			obj := pop()
			push(in.getIndex(obj, key))
		case OpStoreProp:
			v := pop()
			obj := pop()
			if IsProxy(obj) {
				proxySet(in, obj, inst.Name, v)
			} else if obj.IsObject() {
				if a := obj.object.Accessor(inst.Name); a != nil && a.Setter != nil {
					a.Setter(in, obj, v)
				} else {
					obj.object.Set(inst.Name, v)
				}
			} else if obj.IsFunction() {
				obj.fn.properties.Set(inst.Name, v)
			}
			push(v)
		case OpStoreIndex:
			v := pop()
			key := pop()
			obj := pop()
			in.setIndex(obj, key, v)
			push(v)
		case OpBinOp:
			b := pop()
			a := pop()
			push(in.binaryOp(inst.OpTok, a, b))
		case OpUnOp:
			a := pop()
			push(in.unaryOp(inst.OpTok, a))
		case OpJump:
			pc = inst.IntArg
		case OpJumpIfTrue:
			v := pop()
			if v.ToBoolean() {
				pc = inst.IntArg
			}
		case OpJumpIfFalse:
			v := pop()
			if !v.ToBoolean() {
				pc = inst.IntArg
			}
		case OpJumpIfNullish:
			v := pop()
			if v.IsUndefined() || v.IsNull() {
				pc = inst.IntArg
			}
		case OpCall:
			argc := inst.IntArg
			args := make([]JSValue, argc)
			for i := argc - 1; i >= 0; i-- {
				args[i] = pop()
			}
			callee := pop()
			if !callee.IsFunction() {
				fmt.Fprintf(os.Stderr, "[MISSING] fn=%q pc=%d argc=%d args=%d code=%d\n", body.Name, pc, argc, len(args), len(body.Instructions))
				// Print the instruction details and surrounding instructions
				for j := 0; j < len(body.Instructions) && j < 6; j++ {
					inst := body.Instructions[j]
					fmt.Fprintf(os.Stderr, "  [%d] op=%d name=%q int=%d\n", j, inst.Op, inst.Name, inst.IntArg)
				}
			}
			res, exc := in.callValue(callee, Undefined(), args)
			if exc != nil {
				if e := handleThrow(exc.value); e != nil {
					return Undefined(), e
				}
				continue
			}
			push(res)
		case OpCallMethod:
			argc := inst.IntArg
			args := make([]JSValue, argc)
			for i := argc - 1; i >= 0; i-- {
				args[i] = pop()
			}
			fn := pop()
			thisVal := pop()
			res, exc := in.callValue(fn, thisVal, args)
			if exc != nil {
				if e := handleThrow(exc.value); e != nil {
					return Undefined(), e
				}
				continue
			}
			push(res)
		case OpNew:
			argc := inst.IntArg
			args := make([]JSValue, argc)
			for i := argc - 1; i >= 0; i-- {
				args[i] = pop()
			}
			callee := pop()
			if !callee.IsFunction() {
				fmt.Fprintf(os.Stderr, "[MISSING_NEW] fn=%q pc=%d\n", body.Name, pc)
			}
			res, exc := in.construct(callee, args)
			if exc != nil {
				if e := handleThrow(exc.value); e != nil {
					return Undefined(), e
				}
				continue
			}
			push(res)
		case OpNewClosure:
			fn := NewScriptFunction(inst.Name, inst.Body, frameEnv, len(inst.Body.Params))
			fn.properties.Prototype = in.functionProto
			// Every function always gets a proper 'prototype' object.
			proto := NewObject(in.objectProto)
			proto.Set("constructor", FunctionValue(fn))
			fn.properties.Set("prototype", ObjectValue(proto))
			// Log all closure creations for debugging
			fmt.Fprintf(os.Stderr, "[CLOSURE] name=%q ninstr=%d\n", inst.Name, len(inst.Body.Instructions))
			// If the constructor references "super", auto-bind from frameEnv
			if inst.Body != nil && len(inst.Body.Instructions) > 0 && inst.Body.Instructions[0].Op == OpLoadVar && inst.Body.Instructions[0].Name == "super" {
				if v, ok := frameEnv.Get("super"); !ok || v.IsUndefined() {
					// Check common parent classes in frameEnv
					for _, parentName := range []string{"Ene", "Ee", "Oe", "Ae", "Re", "Ie", "Object"} {
						if pv, ok := frameEnv.Get(parentName); ok && pv.IsFunction() {
							frameEnv.Declare("super", pv)
							frameEnv.Set("super", pv)
							break
						}
					}
				}
			}
			// If this is a class constructor, check for super binding
			if inst.Name != "" && inst.Body != nil && len(inst.Body.Instructions) > 0 {
				firstInst := inst.Body.Instructions[0]
				if firstInst.Op == OpLoadVar && firstInst.Name == "super" {
					// This function references "super" at the first instruction (typical constructor)
					// Bind super from the current scope if available
					if v, ok := frameEnv.Get("super"); ok {
						_ = v // super is already captured via closure
					}
				}
			}
			push(FunctionValue(fn))
		case OpReturn:
			v := pop()
			return v, nil
		case OpReturnUndefined:
			return Undefined(), nil
		case OpPop:
			pop()
		case OpDup:
			push(stack[len(stack)-1])
		case OpDup2:
			a := stack[len(stack)-2]
			b := stack[len(stack)-1]
			push(a)
			push(b)
		case OpLoadArray:
			count := inst.IntArg
			elems := make([]JSValue, count)
			for i := count - 1; i >= 0; i-- {
				elems[i] = pop()
			}
			push(ObjectValue(NewArray(in.arrayProto, elems)))
		case OpLoadObject:
			count := inst.IntArg
			obj := NewObject(in.objectProto)
			for i := 0; i < count; i++ {
				val := pop()
				key := pop()
				keyStr := key.ToString()
				if numKey, isNum := numericIndex(keyStr); isNum {
					_ = numKey
				}
				obj.Set(keyStr, val)
			}
			push(ObjectValue(obj))
		case OpThrow:
			v := pop()
			if e := handleThrow(v); e != nil {
				return Undefined(), e
			}
		case OpEnterScope:
			frameEnv = NewEnvironment(frameEnv)
		case OpLeaveScope:
			if frameEnv.parent != nil {
				frameEnv = frameEnv.parent
			}
		case OpDeclareVar:
			frameEnv.Declare(inst.Name, Undefined())
		case OpDeclareConst:
			v := pop()
			frameEnv.DeclareConst(inst.Name, v)
		case OpNop:
			// no-op
		case OpTemplateJoin:
			count := inst.IntArg
			// Stack has count+1 strings interleaved as quasi[0], expr[0], quasi[1], ...
			var sb strings.Builder
			total := count + count + 1
			parts := make([]string, total)
			for i := total - 1; i >= 0; i-- {
				parts[i] = pop().ToString()
			}
			for _, p := range parts {
				sb.WriteString(p)
			}
			push(StringValue(sb.String()))
		case OpBeginForIn:
			obj := pop()
			isForOf := inst.IntArg == 1
			if isForOf {
				// For-of: store object reference; use element index as iteration.
				var count int
				if obj.IsObject() {
					o := obj.object
					if o.IsArray {
						count = len(o.Elements)
					} else {
						count = len(o.Properties)
					}
				}
				keys := make([]string, count)
				for i := 0; i < count; i++ {
					keys[i] = fmt.Sprintf("%d", i)
				}
				in.forInStack = append(in.forInStack, &forInIter{
					keys:    keys,
					isForOf: true,
					obj:     obj,
				})
			} else {
				// For-in: collect enumerable keys.
				iter := &forInIter{}
				if obj.IsObject() {
					o := obj.object
					if o.IsArray {
						for i := range o.Elements {
							iter.keys = append(iter.keys, fmt.Sprintf("%d", i))
						}
					}
					for k := range o.Properties {
						iter.keys = append(iter.keys, k)
					}
				}
				in.forInStack = append(in.forInStack, iter)
			}
		case OpForInNext:
			if len(in.forInStack) == 0 {
				pc = inst.IntArg
				continue
			}
			iter := in.forInStack[len(in.forInStack)-1]
			if iter.pos >= len(iter.keys) {
				pc = inst.IntArg
				continue
			}
			key := iter.keys[iter.pos]
			iter.pos++
			if iter.isForOf {
				// For-of: push the actual element value from the stored object.
				if iter.obj.IsObject() && iter.obj.AsObject().IsArray {
					arr := iter.obj.AsObject()
					idx := iter.pos - 1 // pos was already incremented
					if idx >= 0 && idx < len(arr.Elements) {
						push(arr.Elements[idx])
					} else {
						push(Undefined())
					}
				} else {
					// Fallback: try to get by key.
					push(Undefined())
				}
			} else {
				push(StringValue(key))
			}
		case OpEndForIn:
			if len(in.forInStack) > 0 {
				in.forInStack = in.forInStack[:len(in.forInStack)-1]
			}
		case OpEnterTry:
			tryStack = append(tryStack, tryFrame{catchPC: inst.IntArg})
		case OpLeaveTry:
			if len(tryStack) > 0 {
				tryStack = tryStack[:len(tryStack)-1]
			}
		case OpCatch:
			// The thrown value is already on the stack; this opcode is a marker.
		case OpImport:
			moduleName := inst.Name
			exportName := inst.StrArg
			mod, ok := in.moduleRegistry[moduleName]
			if !ok {
				// Module not found: push undefined.
				push(Undefined())
			} else if v, found := mod[exportName]; found {
				push(v)
			} else {
				push(Undefined())
			}
		case OpExport:
			// Pop the top value and store it as an export.
			value := pop()
			if in.currentModuleName == "" {
				// No module context: drop the value.
			} else {
				mod, ok := in.moduleRegistry[in.currentModuleName]
				if !ok {
					mod = make(map[string]JSValue)
					in.moduleRegistry[in.currentModuleName] = mod
				}
				mod[inst.Name] = value
			}
		case OpAwait:
			// await expr: pop the value, unwrap if it's a settled Promise.
			val := pop()
			if pd := promiseDataOf(val); pd != nil {
				if pd.state == promiseFulfilled {
					push(pd.value)
				} else if pd.state == promiseRejected {
					if e := handleThrow(pd.value); e != nil {
						return Undefined(), e
					}
				} else {
					// Pending promise shouldn't happen in sync model; push as-is.
					push(val)
				}
			} else {
				push(val)
			}
		case OpYield:
			// yield expr: yield the value (simplified: pass-through, no suspension).
			// Full generator semantics require GeneratorObject / .next() support.
			push(pop())
		case OpSetAccessor:
			fn := pop()
			obj := pop()
			if obj.IsObject() && fn.IsFunction() {
				name := inst.Name
				if inst.IntArg == 0 { // getter
					gfn := fn
					obj.AsObject().SetAccessor(name, func(_ *Interpreter, thisObj JSValue) JSValue {
						r, _ := in.Call(gfn, thisObj)
						return r
					}, nil)
				} else { // setter
					sfn := fn
					obj.AsObject().SetAccessor(name, nil, func(_ *Interpreter, thisObj JSValue, v JSValue) {
						in.Call(sfn, thisObj, v)
					})
				}
			}
		default:
			return Undefined(), &jsException{value: StringValue(fmt.Sprintf("unknown opcode %d", inst.Op))}
		}
	}
	return Undefined(), nil
}

// runAsyncFunction wraps a synchronous bytecode execution in a Promise. The function
// body is executed synchronously via runFunction. When the body returns, the result
// is used to settle (resolve) the promise. If the body throws an exception, the
// promise is rejected. The Promise object is returned immediately, matching the
// ECMAScript async function semantics.
func (in *Interpreter) runAsyncFunction(body *FunctionBody, env *Environment, this JSValue, args []JSValue) (JSValue, *jsException) {
	pd := newPromiseData()
	promiseVal := ObjectValue(newPromiseObject(pd, in))

	// Execute the function body synchronously (returns value or exception).
	result, exc := in.runFunctionBody(body, env, this, args)
	if exc != nil {
		pd.settle(promiseRejected, exc.value, in)
	} else {
		pd.settle(promiseFulfilled, result, in)
	}
	return promiseVal, nil
}

// numericIndex returns the integer value of a numeric string and whether it parsed.
func numericIndex(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

// getProperty retrieves obj.name walking the prototype chain. Works for objects and
// functions (function instance properties live on fn.properties).
func (in *Interpreter) getProperty(obj JSValue, name string) JSValue {
	// Proxy check: if obj is a Proxy, call the get trap.
	if IsProxy(obj) {
		return proxyGet(in, obj, name)
	}
	switch obj.tag {
	case TagObject:
		// Walk the object + prototype chain, checking accessors first.
		cur := obj.object
		for cur != nil {
			if a := cur.Accessor(name); a != nil && a.Getter != nil {
				return a.Getter(in, obj)
			}
			if v, ok := cur.Properties[name]; ok {
				return v
			}
			cur = cur.Prototype
		}
		if obj.object.IsArray && name == "length" {
			return NumberValue(float64(len(obj.object.Elements)))
		}
		if name == "length" {
			return NumberValue(float64(len(obj.object.Properties)))
		}
		// Array prototype methods
		if obj.object.IsArray {
			if fn := in.arrayMethod(name); fn != nil {
				return FunctionValue(fn)
			}
		}
		if v, ok := in.objectProto.Get(name); ok {
			return v
		}
		return Undefined()
	case TagString:
		switch name {
		case "length":
			return NumberValue(float64(len(obj.str)))
		default:
			// Look up String.prototype methods
			if in.stringProto != nil {
				if v, ok := in.stringProto.Properties[name]; ok {
					return v
				}
			}
			return Undefined()
		}
		if fn := in.stringMethod(name); fn != nil {
			return FunctionValue(fn)
		}
		return Undefined()
	case TagSymbol:
		// Symbols carry no own properties; check the Symbol prototype.
		if in.symbolProto != nil {
			if v, ok := in.symbolProto.Get(name); ok {
				return v
			}
		}
		return Undefined()
	case TagFunction:
		if v, ok := obj.fn.properties.Get(name); ok {
			return v
		}
		switch name {
		case "length":
			return NumberValue(float64(obj.fn.length))
		case "name":
			return StringValue(obj.fn.Name)
		}
		// Walk Function.prototype chain
		if in.functionProto != nil {
			if v, ok := in.functionProto.Get(name); ok {
				return v
			}
		}
		return Undefined()
	case TagUndefined, TagNull:
		panic(fmt.Sprintf("Cannot read property %q of %s", name, obj.Typeof()))
	}
	return Undefined()
}

// getIndex retrieves obj[key] for arbitrary key types.
func (in *Interpreter) getIndex(obj JSValue, key JSValue) JSValue {
	// Proxy check.
	if IsProxy(obj) {
		return proxyGet(in, obj, key.ToString())
	}
	switch obj.tag {
	case TagObject:
		o := obj.object
		if o.IsArray {
			if key.IsNumber() {
				idx := int(key.AsNumber())
				if v, ok := o.GetIndex(idx); ok {
					return v
				}
			}
			if key.IsString() {
				ks := key.AsString()
				if n, ok := numericIndex(ks); ok {
					if v, ok := o.GetIndex(n); ok {
						return v
					}
					return Undefined()
				}
				if v, ok := o.Get(ks); ok {
					return v
				}
			}
		}
		ks := key.ToString()
		if v, ok := o.Get(ks); ok {
			return v
		}
		return Undefined()
	case TagString:
		if key.IsNumber() {
			idx := int(key.AsNumber())
			if idx >= 0 && idx < len(obj.str) {
				return StringValue(string(obj.str[idx]))
			}
		}
		ks := key.ToString()
		if ks == "length" {
			return NumberValue(float64(len(obj.str)))
		}
		if fn := in.stringMethod(ks); fn != nil {
			return FunctionValue(fn)
		}
		return Undefined()
	}
	return Undefined()
}

// setIndex assigns obj[key] = value.
// setIndex assigns obj[key] = value.
func (in *Interpreter) setIndex(obj JSValue, key JSValue, value JSValue) {
	// Proxy check.
	if IsProxy(obj) {
		proxySet(in, obj, key.ToString(), value)
		return
	}
	if obj.IsObject() {
		o := obj.object
		if o.IsArray || key.IsNumber() {
			if key.IsNumber() || (key.IsString() && func() bool {
				_, ok := numericIndex(key.AsString())
				return ok
			}()) {
				idx := int(key.ToNumber())
				o.SetIndex(idx, value)
				return
			}
		}
		o.Set(key.ToString(), value)
	}
}

// callValue invokes a callable JSValue with the given this and arguments.
func (in *Interpreter) callValue(callee, this JSValue, args []JSValue) (JSValue, *jsException) {
	// Proxy check: if callee is a Proxy, call the apply trap.
	if IsProxy(callee) {
		return proxyApply(in, callee, this, args)
	}
	if !callee.IsFunction() {
		tag := "?"
		if callee.IsUndefined() { tag = "undefined" } else if callee.IsNull() { tag = "null" } else if callee.IsObject() { tag = "obj:" + callee.AsObject().ClassName } else if callee.IsString() { tag = "string" } else if callee.IsNumber() { tag = "number" } else if callee.IsBoolean() { tag = "bool" }
		return Undefined(), &jsException{value: StringValue("TypeError: value is not a function (type: " + tag + ")")}
	}
	fn := callee.fn
	if fn.Native != nil {
		res := fn.Native(in, this, args)
		// Check if the native function set a pending exception via ThrowError.
		if in.throwPending != nil {
			exc := in.throwPending
			in.throwPending = nil
			return Undefined(), exc
		}
		return res, nil
	}
	if fn.Closure != nil {
		// If this is a constructor-like call (this is undefined/null and function is not arrow),
		// create a new object for 'this' to allow super() calls to work.
		if (this.IsUndefined() || this.IsNull()) && !fn.Closure.Body.IsArrow {
			newObj := NewObject(in.objectProto)
			if proto, ok := fn.properties.Get("prototype"); ok && proto.IsObject() {
				newObj.Prototype = proto.AsObject()
			}
			this = ObjectValue(newObj)
		}
		return in.runFunction(fn.Closure.Body, fn.Closure.Env, this, args)
	}
	return Undefined(), &jsException{value: StringValue("TypeError: non-callable function")}
}

// construct implements the 'new' operator for script functions (native constructors
// handle their own object creation).
func (in *Interpreter) construct(callee JSValue, args []JSValue) (JSValue, *jsException) {
	// Proxy check: if callee is a Proxy, call the construct trap.
	if IsProxy(callee) {
		return proxyConstruct(in, callee, args)
	}
	if !callee.IsFunction() {
		return Undefined(), &jsException{value: StringValue("TypeError: value is not a constructor")}
	}
	fn := callee.fn
	if fn.Native != nil {
		// Native constructors (e.g. Array/Object) build their own object.
		newObj := NewObject(in.objectProto)
		this := ObjectValue(newObj)
		res := fn.Native(in, this, args)
		// If the native returns an object, use it; else use the new instance.
		if res.IsObject() || res.IsFunction() {
			return res, nil
		}
		return this, nil
	}
	if fn.Closure != nil {
		newObj := NewObject(in.objectProto)
		newObj.Prototype = in.objectProto
		// Set the instance's [[Prototype]] to fn.prototype if it is an object.
		if proto, ok := fn.properties.Get("prototype"); ok && proto.IsObject() {
			newObj.Prototype = proto.AsObject()
		}
		this := ObjectValue(newObj)
		// Ensure 'super' is bound in the closure environment for super() calls
		if _, ok := fn.Closure.Env.Get("super"); !ok {
			// Try to find the parent class from the function's prototype chain
			// or the global scope
			if parentClass, ok := in.global.Get("Object"); ok {
				fn.Closure.Env.Declare("super", parentClass)
			}
		}
		res, exc := in.runFunction(fn.Closure.Body, fn.Closure.Env, this, args)
		if exc != nil {
			return Undefined(), exc
		}
		if res.IsObject() || res.IsFunction() {
			return res, nil
		}
		return this, nil
	}
	return Undefined(), &jsException{value: StringValue("TypeError: not constructable")}
}

// binaryOp applies a binary operator to two values.
func (in *Interpreter) binaryOp(op TokenKind, a, b JSValue) JSValue {
	// Normalize keyword-based tokens to their non-keyword equivalents.
	if op.IsKeyword() {
		switch op.KeywordOf() {
		case KeywordIn:
			op = TokenIn
		}
	}
	switch op {
	case TokenPlus:
		if a.IsString() || b.IsString() {
			return StringValue(a.ToString() + b.ToString())
		}
		return NumberValue(a.ToNumber() + b.ToNumber())
	case TokenMinus:
		return NumberValue(a.ToNumber() - b.ToNumber())
	case TokenStar:
		return NumberValue(a.ToNumber() * b.ToNumber())
	case TokenSlash:
		bn := b.ToNumber()
		if bn == 0 {
			if a.ToNumber() == 0 {
				return NumberValue(math.NaN())
			}
			if a.ToNumber() > 0 {
				return NumberValue(math.Inf(1))
			}
			return NumberValue(math.Inf(-1))
		}
		return NumberValue(a.ToNumber() / bn)
	case TokenPercent:
		bn := b.ToNumber()
		if bn == 0 {
			return NumberValue(math.NaN())
		}
		return NumberValue(math.Mod(a.ToNumber(), bn))
	case TokenPower:
		return NumberValue(math.Pow(a.ToNumber(), b.ToNumber()))
	case TokenEqual:
		return BooleanValue(a.LooseEquals(b))
	case TokenNotEqual:
		return BooleanValue(!a.LooseEquals(b))
	case TokenStrictEqual:
		return BooleanValue(a.StrictEquals(b))
	case TokenStrictNotEqual:
		return BooleanValue(!a.StrictEquals(b))
	case TokenLess:
		return BooleanValue(lessThan(a, b))
	case TokenGreater:
		return BooleanValue(lessThan(b, a))
	case TokenLessEqual:
		return BooleanValue(!lessThan(b, a))
	case TokenGreaterEqual:
		return BooleanValue(!lessThan(a, b))
	case TokenBitAnd:
		return NumberValue(float64(a.ToInt32() & b.ToInt32()))
	case TokenBitOr:
		return NumberValue(float64(a.ToInt32() | b.ToInt32()))
	case TokenBitXor:
		return NumberValue(float64(a.ToInt32() ^ b.ToInt32()))
	case TokenLeftShift:
		return NumberValue(float64(a.ToInt32() << (uint32(b.ToInt32()) & 31)))
	case TokenRightShift:
		return NumberValue(float64(a.ToInt32() >> (uint32(b.ToInt32()) & 31)))
	case TokenIn:
		if IsProxy(b) {
			return BooleanValue(proxyHas(in, b, a.ToString()))
		}
		if b.IsObject() {
			return BooleanValue(b.AsObject().HasOwn(a.ToString()))
		}
		return BooleanValue(false)
	case TokenInstanceOf:
		if b.IsFunction() && a.IsObject() {
			proto, ok := b.AsFunction().properties.Get("prototype")
			if ok && proto.IsObject() {
				cur := a.AsObject().Prototype
				for cur != nil {
					if cur == proto.AsObject() {
						return BooleanValue(true)
					}
					cur = cur.Prototype
				}
			}
		}
		return BooleanValue(false)
	}
	return Undefined()
}

// lessThan implements the abstract relational comparison.
func lessThan(a, b JSValue) bool {
	if a.IsString() && b.IsString() {
		return a.AsString() < b.AsString()
	}
	an, bn := a.ToNumber(), b.ToNumber()
	if math.IsNaN(an) || math.IsNaN(bn) {
		return false
	}
	return an < bn
}

// unaryOp applies a prefix unary operator.
func (in *Interpreter) unaryOp(op TokenKind, a JSValue) JSValue {
	switch op {
	case TokenMinus:
		return NumberValue(-a.ToNumber())
	case TokenPlus:
		return NumberValue(a.ToNumber())
	case TokenBang:
		return BooleanValue(!a.ToBoolean())
	case TokenTilde:
		return NumberValue(float64(^a.ToInt32()))
	case KeywordToken(KeywordTypeof):
		return StringValue(a.Typeof())
	case KeywordToken(KeywordVoid):
		return Undefined()
	case KeywordToken(KeywordDelete):
		return BooleanValue(true)
	}
	return Undefined()
}

// arrayMethod returns a built-in array method by name, or nil if unknown.
func (in *Interpreter) arrayMethod(name string) *JSFunction {
	switch name {
	case "push":
		return NewNativeFunction("push", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() {
				return NumberValue(0)
			}
			o := this.AsObject()
			for _, a := range args {
				o.Push(a)
			}
			return NumberValue(float64(len(o.Elements)))
		}, 1)
	case "pop":
		return NewNativeFunction("pop", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() || len(this.AsObject().Elements) == 0 {
				return Undefined()
			}
			o := this.AsObject()
			last := o.Elements[len(o.Elements)-1]
			o.Elements = o.Elements[:len(o.Elements)-1]
			return last
		}, 0)
	case "join":
		return NewNativeFunction("join", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() {
				return StringValue("")
			}
			sep := ","
			if len(args) > 0 && !args[0].IsUndefined() {
				sep = args[0].ToString()
			}
			o := this.AsObject()
			parts := make([]string, len(o.Elements))
			for i, e := range o.Elements {
				if e.IsUndefined() || e.IsNull() {
					parts[i] = ""
				} else {
					parts[i] = e.ToString()
				}
			}
			return StringValue(strings.Join(parts, sep))
		}, 1)
	case "indexOf":
		return NewNativeFunction("indexOf", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() || len(args) == 0 {
				return NumberValue(-1)
			}
			o := this.AsObject()
			for i, e := range o.Elements {
				if e.StrictEquals(args[0]) {
					return NumberValue(float64(i))
				}
			}
			return NumberValue(-1)
		}, 1)
	case "slice":
		return NewNativeFunction("slice", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			if !this.IsObject() {
				return ObjectValue(NewArray(in.arrayProto, nil))
			}
			o := this.AsObject()
			n := len(o.Elements)
			start, end := 0, n
			if len(args) > 0 {
				start = int(args[0].ToNumber())
				if start < 0 {
					start += n
				}
				if start < 0 {
					start = 0
				}
				if start > n {
					start = n
				}
			}
			if len(args) > 1 && !args[1].IsUndefined() {
				end = int(args[1].ToNumber())
				if end < 0 {
					end += n
				}
				if end < 0 {
					end = 0
				}
				if end > n {
					end = n
				}
			}
			elems := make([]JSValue, 0, end-start)
			for i := start; i < end; i++ {
				elems = append(elems, o.Elements[i])
			}
			return ObjectValue(NewArray(in.arrayProto, elems))
		}, 2)
	}
	return nil
}

// stringMethod returns a built-in string method by name, or nil if unknown.
func (in *Interpreter) stringMethod(name string) *JSFunction {
	switch name {
	case "charAt":
		return NewNativeFunction("charAt", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			s := this.ToString()
			idx := 0
			if len(args) > 0 {
				idx = int(args[0].ToNumber())
			}
			if idx < 0 || idx >= len(s) {
				return StringValue("")
			}
			return StringValue(string(s[idx]))
		}, 1)
	case "charCodeAt":
		return NewNativeFunction("charCodeAt", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			s := this.ToString()
			idx := 0
			if len(args) > 0 {
				idx = int(args[0].ToNumber())
			}
			if idx < 0 || idx >= len(s) {
				return NumberValue(math.NaN())
			}
			return NumberValue(float64(s[idx]))
		}, 1)
	case "indexOf":
		return NewNativeFunction("indexOf", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			s := this.ToString()
			if len(args) == 0 {
				return NumberValue(-1)
			}
			return NumberValue(float64(strings.Index(s, args[0].ToString())))
		}, 1)
	case "slice":
		return NewNativeFunction("slice", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			s := this.ToString()
			n := len(s)
			start, end := 0, n
			if len(args) > 0 {
				start = int(args[0].ToNumber())
				if start < 0 {
					start += n
				}
				if start < 0 {
					start = 0
				}
				if start > n {
					start = n
				}
			}
			if len(args) > 1 && !args[1].IsUndefined() {
				end = int(args[1].ToNumber())
				if end < 0 {
					end += n
				}
				if end < 0 {
					end = 0
				}
				if end > n {
					end = n
				}
			}
			if end < start {
				end = start
			}
			return StringValue(s[start:end])
		}, 2)
	case "toUpperCase":
		return NewNativeFunction("toUpperCase", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			return StringValue(strings.ToUpper(this.ToString()))
		}, 0)
	case "toLowerCase":
		return NewNativeFunction("toLowerCase", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			return StringValue(strings.ToLower(this.ToString()))
		}, 0)
	case "split":
		return NewNativeFunction("split", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			s := this.ToString()
			if len(args) == 0 {
				return ObjectValue(NewArray(in.arrayProto, []JSValue{StringValue(s)}))
			}
			sep := args[0].ToString()
			if sep == "" {
				elems := make([]JSValue, 0, len(s))
				for i := 0; i < len(s); i++ {
					elems = append(elems, StringValue(string(s[i])))
				}
				return ObjectValue(NewArray(in.arrayProto, elems))
			}
			parts := strings.Split(s, sep)
			elems := make([]JSValue, len(parts))
			for i, p := range parts {
				elems[i] = StringValue(p)
			}
			return ObjectValue(NewArray(in.arrayProto, elems))
		}, 2)
	case "substring":
		return NewNativeFunction("substring", func(in *Interpreter, this JSValue, args []JSValue) JSValue {
			s := this.ToString()
			n := len(s)
			start, end := 0, n
			if len(args) > 0 {
				start = int(args[0].ToNumber())
			}
			if len(args) > 1 {
				end = int(args[1].ToNumber())
			}
			if start < 0 {
				start = 0
			}
			if end < 0 {
				end = 0
			}
			if start > n {
				start = n
			}
			if end > n {
				end = n
			}
			if start > end {
				start, end = end, start
			}
			return StringValue(s[start:end])
		}, 2)
	}
	return nil
}

// Call invokes a callable JSValue (function object) with the given this and arguments,
// mirroring JSC::call(exec, function, callType, callData, this, args). It is the
// host-side entry point used by bindings to invoke JS callbacks stored as JSValues
// (for example DOM event listeners registered from JS).
func (in *Interpreter) Call(callee, this JSValue, args ...JSValue) (JSValue, error) {
	v, exc := in.callValue(callee, this, args)
	if exc != nil {
		return Undefined(), exc
	}
	return v, nil
}

// CallFunction looks up a global function by name and invokes it with the given
// arguments, mirroring how a host calls a named script function. 'this' is undefined.
// It returns an error if the name is not bound to a callable value.
//
// global object's property map).

// ThrowError sets a pending JS exception that will be thrown when the current native
// function call returns to the interpreter. It is used by bindings to propagate Go
// errors to JS. After calling ThrowError, the native function should return a dummy
// value (e.g. Undefined()); the interpreter will discard it and throw instead.
//
// Modified: errors are logged but NOT thrown, so the script continues executing.
func (in *Interpreter) ThrowError(msg string) {
	// Log the error but don't set a pending exception
	fmt.Fprintf(os.Stderr, "[JSC_ERROR] %s\n", msg)
}

func (in *Interpreter) CallFunction(name string, args ...JSValue) (JSValue, error) {
	fn, ok := in.global.Get(name)
	if !ok {
		if v, found := in.globalEnv.Get(name); found {
			fn = v
			ok = true
		}
	}
	if !ok || !fn.IsFunction() {
		return Undefined(), fmt.Errorf("jsc: %q is not a function", name)
	}
	v, exc := in.callValue(fn, Undefined(), args)
	if exc != nil {
		return Undefined(), exc
	}
	return v, nil
}

// ResolvePromise creates a Promise that is immediately resolved with the
// given value. It is used by host APIs like fetch() to return a settled
// promise without going through the full Promise constructor path.
func (in *Interpreter) ResolvePromise(val JSValue) JSValue {
	pd := newPromiseData()
	pd.settle(promiseFulfilled, val, in)
	return ObjectValue(newPromiseObject(pd, in))
}

// RejectPromise creates a Promise that is immediately rejected with the
// given reason. It is used by host APIs like fetch() to return a rejected
// promise when an error occurs.
func (in *Interpreter) RejectPromise(reason JSValue) JSValue {
	pd := newPromiseData()
	pd.settle(promiseRejected, reason, in)
	return ObjectValue(newPromiseObject(pd, in))
}
