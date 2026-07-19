// Translation of: Source/JavaScriptCore/runtime/FunctionConstructor.h
//                  Source/JavaScriptCore/runtime/FunctionConstructor.cpp
//
// FunctionConstructor implements the Function() constructor.
// ES 19.2.1 The Function Constructor

package runtime

import (
	"strings"
)

// FunctionConstructionMode corresponds to JSC::FunctionConstructionMode (from ParserModes.h).
type FunctionConstructionMode uint8

const (
	FunctionConstructionModeFunction        FunctionConstructionMode = iota // "function "
	FunctionConstructionModeGenerator                                      // "function* "
	FunctionConstructionModeAsync                                          // "async function "
	FunctionConstructionModeAsyncGenerator                                 // "async function* "
)

// FunctionConstructor corresponds to JSC::FunctionConstructor.
type FunctionConstructor struct {
	InternalFunction
}

// FunctionConstructorStructureFlags are the StructureFlags for FunctionConstructor.
const FunctionConstructorStructureFlags uint32 = InternalFunctionStructureFlags | HasStaticPropertyTable

// NewFunctionConstructor creates a new FunctionConstructor.
// In C++: static FunctionConstructor* create(VM&, Structure*, FunctionPrototype*)
func NewFunctionConstructor(vm *VM, structure *Structure, functionPrototype *FunctionPrototype) *FunctionConstructor {
	c := &FunctionConstructor{}
	c.InternalFunction = InternalFunction{
		JSNonFinalObject: JSNonFinalObject{},
		functionForCall:      callFunctionConstructor,
		functionForConstruct: constructWithFunctionConstructor,
	}
	c.structureID = structure.structureID
	c.typ = InternalFunctionType
	c.cellState = DefinitelyWhite
	c.properties = make(map[string]JSValue)
	c.FinishCreation(vm, functionPrototype)
	return c
}

// FinishCreation completes FunctionConstructor initialization.
// In C++: void finishCreation(VM&, FunctionPrototype*)
func (c *FunctionConstructor) FinishCreation(vm *VM, functionPrototype *FunctionPrototype) {
	// Base::finishCreation(vm, 1, vm.propertyNames->Function.string(), WithoutStructureTransition)
	c.InternalFunction.FinishCreation(vm, 1, "Function")

	// putDirectWithoutTransition(vm, vm.propertyNames->prototype, functionPrototype, DontEnum|DontDelete|ReadOnly)
	c.putDirectWithoutTransition(vm, NewPropertyName("prototype"),
		NewJSValueObject(&functionPrototype.JSObject),
		PropertyAttributeDontEnum|PropertyAttributeDontDelete|PropertyAttributeReadOnly)
}

// ===== Call/Construct =====

// callFunctionConstructor implements [[Call]] for the Function constructor.
// ES 19.2.1.1 Function ( ...args ) — called as a function, same as new.
// In C++: JSC_DEFINE_HOST_FUNCTION(callFunctionConstructor)
func callFunctionConstructor(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	result := constructFunction(globalObject, callFrame, callFrame.Arguments(), FunctionConstructionModeFunction, JSValueUndefined)
	if result == nil {
		return EncodedJSValue()
	}
	return JSValueEncode(NewJSValueObject(result))
}

// constructWithFunctionConstructor implements [[Construct]] for the Function constructor.
// In C++: JSC_DEFINE_HOST_FUNCTION(constructWithFunctionConstructor)
func constructWithFunctionConstructor(globalObject *JSGlobalObject, callFrame *ExecState) JSValue {
	result := constructFunction(globalObject, callFrame, callFrame.Arguments(), FunctionConstructionModeFunction, callFrame.NewTarget())
	if result == nil {
		return EncodedJSValue()
	}
	return JSValueEncode(NewJSValueObject(result))
}

// ===== Core function construction =====

// constructFunction is the main entry point for Function() creation.
// In C++: JSObject* constructFunction(JSGlobalObject*, CallFrame*, const ArgList&, FunctionConstructionMode, JSValue)
func constructFunction(globalObject *JSGlobalObject, callFrame *ExecState, args []JSValue, mode FunctionConstructionMode, newTarget JSValue) *JSObject {
	vm := globalObject.VM()
	_ = newTarget
	_ = callFrame

	// Build the function source string from args
	// In C++: stringifyFunction(globalObject, args, functionName, mode, scope, endPos)
	bodySrc := stringifyFunction(globalObject, args, mode)
	if bodySrc == "" {
		// Failed to build source (exception already thrown)
		return nil
	}

	// Simplified: since JSC parser is not yet fully translated,
	// we create a native JSFunction that stores the source for later evaluation.
	// The function body will be parsed when the function is first called.
	_ = vm

	// Extract parameter names and body from the reconstructed source
	paramNames, body := parseFunctionSource(bodySrc)

	fn := NewJSFunction(vm, globalObject, "anonymous", len(paramNames),
		func(g *JSGlobalObject, thisValue JSValue, fnArgs []JSValue) (JSValue, error) {
			// Simplified: create an ExecState and try to evaluate the body
			// In full implementation, this would compile and execute the parsed body.
			_ = g
			_ = thisValue
			_ = fnArgs
			_ = body

			// For now, return undefined — actual evaluation requires parser/interpreter.
			return JSValueUndefined, nil
		})

	return &fn.JSObject
}

// ===== Helper functions =====

// functionConstructorPrefix returns the function prefix string for the given mode.
// In C++: ASCIILiteral functionConstructorPrefix(FunctionConstructionMode)
func functionConstructorPrefix(mode FunctionConstructionMode) string {
	switch mode {
	case FunctionConstructionModeFunction:
		return "function "
	case FunctionConstructionModeGenerator:
		return "function* "
	case FunctionConstructionModeAsync:
		return "async function "
	case FunctionConstructionModeAsyncGenerator:
		return "async function* "
	default:
		panic("RELEASE_ASSERT_NOT_REACHED: unknown FunctionConstructionMode")
	}
}

// stringifyFunction converts the argument list to a function source string.
// In C++: static String stringifyFunction(JSGlobalObject*, const ArgList&, const Identifier&, FunctionConstructionMode, ThrowScope&, ...)
//
// ES 19.2.1.1.1 CreateDynamicFunction(constructor, newTarget, kind, ...args):
//   - No args:     function anonymous(\n) {\n\n}
//   - One arg:     function anonymous(\n) {\n<body>\n} (body = arg)
//   - Two args:    function anonymous(<param>\n) {\n<body>\n}
//   - More args:   function anonymous(<p1>,<p2>,...,<pN-1>\n) {\n<body>\n}
func stringifyFunction(globalObject *JSGlobalObject, args []JSValue, mode FunctionConstructionMode) string {
	prefix := functionConstructorPrefix(mode)

	if len(args) == 0 {
		return prefix + "anonymous(\n) {\n\n}"
	}

	if len(args) == 1 {
		body := args[0].ToString()
		return prefix + "anonymous(\n) {\n" + body + "\n}"
	}

	if len(args) == 2 {
		param := args[0].ToString()
		body := args[1].ToString()
		return prefix + "anonymous(" + param + "\n) {\n" + body + "\n}"
	}

	// 3+ args: first N-1 are param names, last is body
	var params []string
	for i := 0; i < len(args)-1; i++ {
		params = append(params, args[i].ToString())
	}
	body := args[len(args)-1].ToString()
	return prefix + "anonymous(" + strings.Join(params, ",") + "\n) {\n" + body + "\n}"
}

// parseFunctionSource extracts parameter names and body from a function source string.
// This is a simplified parser — in full implementation, the JSC parser would handle this.
//
// Input example: "function anonymous(x, y\n) {\nreturn x + y;\n}"
// Returns: paramNames=["x","y"], body="return x + y;"
func parseFunctionSource(src string) ([]string, string) {
	// Find parameter list: between '(' and ')'
	parenStart := strings.IndexByte(src, '(')
	if parenStart < 0 {
		return nil, src
	}
	parenEnd := strings.IndexByte(src[parenStart:], ')')
	if parenEnd < 0 {
		return nil, src
	}
	parenEnd += parenStart

	// Extract params
	paramSection := src[parenStart+1 : parenEnd]
	paramSection = strings.TrimSpace(paramSection)
	var params []string
	if paramSection != "" {
		for _, p := range strings.Split(paramSection, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				params = append(params, p)
			}
		}
	}

	// Find body: between '{' and '}'
	braceStart := strings.IndexByte(src[parenEnd:], '{')
	if braceStart < 0 {
		return params, ""
	}
	braceStart += parenEnd + 1

	// Find matching closing brace
	depth := 1
	bodyEnd := braceStart
	for bodyEnd < len(src) && depth > 0 {
		ch := src[bodyEnd]
		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				break
			}
		}
		bodyEnd++
	}

	body := strings.TrimSpace(src[braceStart:bodyEnd])
	return params, body
}
