// Translation of: Source/JavaScriptCore/runtime/JSCJSValue.h
//                  Source/JavaScriptCore/runtime/JSCJSValue.cpp
//                  Source/JavaScriptCore/runtime/JSObject.h
//                  Source/JavaScriptCore/runtime/JSObject.cpp
//                  Source/JavaScriptCore/runtime/JSFunction.h
//                  Source/JavaScriptCore/runtime/JSFunction.cpp
// Completeness: 60%
// Simplifications:
//   - JSValue is a tagged Go struct (tag + payload) instead of a NaN-boxed 64-bit
//     payload; the encoding/decoding helper functions of the C++ version are dropped.
//   - no GC cell / structure butterfly; the property map is a plain Go map.
//   - no prototype chain caching; lookups walk the Prototype pointer directly.
//   - JSFunction unifies native Go callbacks and script-defined closures in one type.

package jsc

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// JSValueTag enumerates the runtime tag for a JSValue, mirroring the JSType enum in
// JSCJSValue.h. The Go port uses an explicit tag rather than NaN-boxing.
type JSValueTag int

const (
	// TagUndefined is the tag for the undefined value.
	TagUndefined JSValueTag = iota
	// TagNull is the tag for the null value.
	TagNull
	// TagBoolean is the tag for boolean values.
	TagBoolean
	// TagNumber is the tag for number values (IEEE 754 float64).
	TagNumber
	// TagString is the tag for string values.
	TagString
	// TagSymbol is the tag for symbol values.
	TagSymbol
	// TagObject is the tag for object values (including arrays/functions).
	TagObject
	// TagFunction is the tag for callable values.
	TagFunction
)

// JSValue is the Go translation of JSC::JSValue. WebKit encodes values via NaN-boxing
// into a single 64-bit payload; the Go port uses an explicit tag plus payload fields.
// The zero value is Undefined.
type JSValue struct {
	tag     JSValueTag
	boolean bool
	number  float64
	str     string
	symbol  string
	object  *JSObject
	fn      *JSFunction
}

// Undefined returns the undefined value.
func Undefined() JSValue { return JSValue{tag: TagUndefined} }

// Null returns the null value.
func Null() JSValue { return JSValue{tag: TagNull} }

// BooleanValue wraps a bool.
func BooleanValue(b bool) JSValue { return JSValue{tag: TagBoolean, boolean: b} }

// NumberValue wraps a float64.
func NumberValue(n float64) JSValue { return JSValue{tag: TagNumber, number: n} }

// IntValue wraps an int as a number.
func IntValue(n int) JSValue { return JSValue{tag: TagNumber, number: float64(n)} }

// StringValue wraps a Go string.
func StringValue(s string) JSValue { return JSValue{tag: TagString, str: s} }

// SymbolValue wraps a description string as a Symbol.
func SymbolValue(desc string) JSValue { return JSValue{tag: TagSymbol, symbol: desc} }

// ObjectValue wraps a *JSObject.
func ObjectValue(o *JSObject) JSValue {
	if o == nil {
		return Null()
	}
	if o.IsArray {
		return JSValue{tag: TagObject, object: o}
	}
	return JSValue{tag: TagObject, object: o}
}

// FunctionValue wraps a *JSFunction.
func FunctionValue(f *JSFunction) JSValue {
	if f == nil {
		return Undefined()
	}
	return JSValue{tag: TagFunction, fn: f}
}

// IsUndefined reports whether the value is undefined.
func (v JSValue) IsUndefined() bool { return v.tag == TagUndefined }

// IsNull reports whether the value is null.
func (v JSValue) IsNull() bool { return v.tag == TagNull }

// IsBoolean reports whether the value is a boolean.
func (v JSValue) IsBoolean() bool { return v.tag == TagBoolean }

// IsNumber reports whether the value is a number.
func (v JSValue) IsNumber() bool { return v.tag == TagNumber }

// IsString reports whether the value is a string.
func (v JSValue) IsString() bool { return v.tag == TagString }

// IsSymbol reports whether the value is a symbol.
func (v JSValue) IsSymbol() bool { return v.tag == TagSymbol }

// IsObject reports whether the value is an object.
func (v JSValue) IsObject() bool { return v.tag == TagObject }

// IsFunction reports whether the value is callable (function object).
func (v JSValue) IsFunction() bool { return v.tag == TagFunction }

// IsCallable reports whether the value can be called as a function.
func (v JSValue) IsCallable() bool { return v.tag == TagFunction }

// Tag returns the runtime tag of the value.
func (v JSValue) Tag() JSValueTag { return v.tag }

// AsBoolean returns the boolean payload. It panics if the value is not a boolean.
func (v JSValue) AsBoolean() bool {
	if v.tag != TagBoolean {
		panic("jsc: AsBoolean on non-boolean value")
	}
	return v.boolean
}

// AsNumber returns the number payload. It panics if the value is not a number.
func (v JSValue) AsNumber() float64 {
	if v.tag != TagNumber {
		panic("jsc: AsNumber on non-number value")
	}
	return v.number
}

// AsString returns the string payload. It panics if the value is not a string.
func (v JSValue) AsString() string {
	if v.tag != TagString {
		panic("jsc: AsString on non-string value")
	}
	return v.str
}

// AsObject returns the object payload. It panics if the value is not an object.
func (v JSValue) AsObject() *JSObject {
	if v.tag != TagObject {
		panic("jsc: AsObject on non-object value")
	}
	return v.object
}

// AsFunction returns the function payload. It panics if the value is not a function.
func (v JSValue) AsFunction() *JSFunction {
	if v.tag != TagFunction {
		panic("jsc: AsFunction on non-function value")
	}
	return v.fn
}

// AsObjectOrPanic returns the object or function's underlying object. Functions carry
// an object too (their properties). Returns nil for non-objects/non-functions.
func (v JSValue) AsObjectOrNil() *JSObject {
	switch v.tag {
	case TagObject:
		return v.object
	case TagFunction:
		return v.fn.properties
	}
	return nil
}

// ToBoolean follows the ECMAScript ToBoolean abstract operation.
func (v JSValue) ToBoolean() bool {
	switch v.tag {
	case TagUndefined, TagNull:
		return false
	case TagBoolean:
		return v.boolean
	case TagNumber:
		return !(v.number == 0 || math.IsNaN(v.number))
	case TagString:
		return len(v.str) > 0
	case TagSymbol:
		return true
	case TagObject, TagFunction:
		return true
	}
	return false
}

// ToNumber follows the ECMAScript ToNumber abstract operation (subset).
func (v JSValue) ToNumber() float64 {
	switch v.tag {
	case TagUndefined:
		return math.NaN()
	case TagNull:
		return 0
	case TagBoolean:
		if v.boolean {
			return 1
		}
		return 0
	case TagNumber:
		return v.number
	case TagString:
		s := strings.TrimSpace(v.str)
		if s == "" {
			return 0
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return math.NaN()
		}
		return n
	case TagObject, TagFunction:
		return math.NaN()
	}
	return math.NaN()
}

// ToInt32 follows the ECMAScript ToInt32 abstract operation (used for bitwise ops).
func (v JSValue) ToInt32() int32 {
	n := v.ToNumber()
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0
	}
	posInt := math.Trunc(n)
	int32bit := math.Mod(posInt, 4294967296) // 2^32
	if int32bit < 0 {
		int32bit += 4294967296
	}
	if int32bit >= 2147483648 { // 2^31
		int32bit -= 4294967296
	}
	return int32(int32bit)
}

// ToUint32 follows the ECMAScript ToUint32 abstract operation.
func (v JSValue) ToUint32() uint32 {
	return uint32(v.ToInt32())
}

// ToString follows the ECMAScript ToString abstract operation (subset).
func (v JSValue) ToString() string {
	switch v.tag {
	case TagUndefined:
		return "undefined"
	case TagNull:
		return "null"
	case TagBoolean:
		if v.boolean {
			return "true"
		}
		return "false"
	case TagNumber:
		return formatNumber(v.number)
	case TagString:
		return v.str
	case TagSymbol:
		return "Symbol(" + v.symbol + ")"
	case TagObject:
		if v.object != nil && v.object.IsArray {
			return v.arrayToString()
		}
		return "[object Object]"
	case TagFunction:
		return "function () { [native code] }"
	}
	return ""
}

// arrayToString formats an array value as a JS-style comma-joined string.
func (v JSValue) arrayToString() string {
	o := v.object
	parts := make([]string, len(o.Elements))
	for i, e := range o.Elements {
		if e.IsUndefined() || e.IsNull() {
			parts[i] = ""
		} else {
			parts[i] = e.ToString()
		}
	}
	return strings.Join(parts, ",")
}

// IsArray reports whether a JSValue is an array object.
func (v JSValue) IsArray() bool {
	return v.tag == TagObject && v.object != nil && v.object.IsArray
}

// formatNumber renders a float64 following ECMAScript Number-to-string rules
// (shortest representation, no trailing .0 for integers).
func formatNumber(n float64) string {
	if math.IsNaN(n) {
		return "NaN"
	}
	if math.IsInf(n, 1) {
		return "Infinity"
	}
	if math.IsInf(n, -1) {
		return "-Infinity"
	}
	if n == 0 {
		return "0"
	}
	// Integer-valued floats render without a decimal point.
	if v := math.Trunc(n); v == n && math.Abs(n) < 1e21 {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return strconv.FormatFloat(n, 'g', -1, 64)
}

// StrictEquals implements the strict equality (===) comparison.
func (v JSValue) StrictEquals(other JSValue) bool {
	if v.tag != other.tag {
		return false
	}
	switch v.tag {
	case TagUndefined, TagNull:
		return true
	case TagBoolean:
		return v.boolean == other.boolean
	case TagNumber:
		if math.IsNaN(v.number) || math.IsNaN(other.number) {
			return false
		}
		return v.number == other.number
	case TagString:
		return v.str == other.str
	case TagSymbol:
		return v.symbol == other.symbol
	case TagObject:
		return v.object == other.object
	case TagFunction:
		return v.fn == other.fn
	}
	return false
}

// LooseEquals implements the abstract (==) comparison (subset).
func (v JSValue) LooseEquals(other JSValue) bool {
	if v.tag == other.tag {
		return v.StrictEquals(other)
	}
	// null == undefined
	if (v.tag == TagNull && other.tag == TagUndefined) ||
		(v.tag == TagUndefined && other.tag == TagNull) {
		return true
	}
	// number/string coercion
	if v.tag == TagNumber && other.tag == TagString {
		return v.number == other.ToNumber()
	}
	if v.tag == TagString && other.tag == TagNumber {
		return v.ToNumber() == other.number
	}
	// boolean coercion
	if v.tag == TagBoolean {
		return BooleanValue(v.boolean).ToNumber() == other.ToNumber() ||
			(v.boolean && other.ToBoolean()) || (!v.boolean && !other.ToBoolean() && v.StrictEquals(other))
	}
	if other.tag == TagBoolean {
		return v.ToNumber() == BooleanValue(other.boolean).ToNumber()
	}
	// object to primitive (very simplified)
	if (v.tag == TagObject || v.tag == TagFunction) && (other.tag == TagNumber || other.tag == TagString) {
		return v.ToNumber() == other.ToNumber()
	}
	if (other.tag == TagObject || other.tag == TagFunction) && (v.tag == TagNumber || v.tag == TagString) {
		return v.ToNumber() == other.ToNumber()
	}
	return false
}

// Typeof mirrors the ECMAScript typeof operator.
func (v JSValue) Typeof() string {
	switch v.tag {
	case TagUndefined:
		return "undefined"
	case TagNull:
		return "object"
	case TagBoolean:
		return "boolean"
	case TagNumber:
		return "number"
	case TagString:
		return "string"
	case TagObject:
		if v.object != nil && v.object.IsArray {
			return "object"
		}
		return "object"
	case TagFunction:
		return "function"
	}
	return "undefined"
}

// String implements fmt.Stringer for debugging.
func (v JSValue) String() string { return v.ToString() }

// JSObject is the Go translation of JSC::JSObject. WebKit stores properties in a
// Butterfly plus a Structure for transition tracking; the Go port uses a single
// map[string]JSValue plus an optional Prototype chain. Arrays use an inline slice
// (Elements) for indexed access.
type JSObject struct {
	// Properties holds named (string-keyed) properties.
	Properties map[string]JSValue
	// Prototype is the [[Prototype]] internal slot.
	Prototype *JSObject
	// IsArray marks the object as an exotic Array.
	IsArray bool
	// Elements holds indexed array elements when IsArray is true.
	Elements []JSValue
	// ClassName is the internal class name ([[Class]]) for diagnostics.
	ClassName string
	// accessors holds named property accessors (getters/setters). When an accessor
	// exists for a name it takes precedence over a plain property entry, mirroring
	// JS accessor properties. Used by host bindings to route property reads/writes
	// back into Go.
	accessors map[string]*Accessor
	// Internal is an opaque host-data slot, mirroring the JSDOMWrapper's wrapped
	// C++ pointer. Bindings store the wrapped Go object (e.g. *dom.Element) here so
	// that methods receiving a JSValue wrapper can recover the Go object.
	Internal any
}

// AccessorGetter is the signature of a JS property getter that runs under the
// interpreter. The 'this' value is the object the property was read from.
type AccessorGetter func(interp *Interpreter, this JSValue) JSValue

// AccessorSetter is the signature of a JS property setter. The 'this' value is the
// object being assigned to and value is the right-hand side.
type AccessorSetter func(interp *Interpreter, this JSValue, value JSValue)

// Accessor pairs a getter and setter for a named JS property.
type Accessor struct {
	Getter AccessorGetter
	Setter AccessorSetter
}

// SetAccessor installs a named accessor property. Either get or set may be nil to
// model a read-only or write-only property. Installing an accessor shadows any plain
// property of the same name.
func (o *JSObject) SetAccessor(name string, get AccessorGetter, set AccessorSetter) {
	if o.accessors == nil {
		o.accessors = make(map[string]*Accessor)
	}
	o.accessors[name] = &Accessor{Getter: get, Setter: set}
}

// Accessor returns the accessor registered for name, or nil when none is present.
func (o *JSObject) Accessor(name string) *Accessor {
	if o.accessors == nil {
		return nil
	}
	return o.accessors[name]
}

// NewObject creates a new plain object with the given prototype.
func NewObject(proto *JSObject) *JSObject {
	return &JSObject{
		Properties: make(map[string]JSValue),
		Prototype:  proto,
		ClassName:  "Object",
	}
}

// NewArray creates a new array object pre-populated with the given elements.
func NewArray(proto *JSObject, elements []JSValue) *JSObject {
	elems := make([]JSValue, len(elements))
	copy(elems, elements)
	return &JSObject{
		Properties: make(map[string]JSValue),
		Prototype:  proto,
		IsArray:    true,
		Elements:   elems,
		ClassName:  "Array",
	}
}

// Get returns the property value for key, walking the prototype chain. The found flag
// reports whether the property exists.
func (o *JSObject) Get(key string) (JSValue, bool) {
	cur := o
	for cur != nil {
		if v, ok := cur.Properties[key]; ok {
			return v, true
		}
		cur = cur.Prototype
	}
	return Undefined(), false
}

// Set assigns a named property.
func (o *JSObject) Set(key string, value JSValue) {
	if o.Properties == nil {
		o.Properties = make(map[string]JSValue)
	}
	o.Properties[key] = value
}

// HasOwn reports whether key is an own property.
func (o *JSObject) HasOwn(key string) bool {
	_, ok := o.Properties[key]
	return ok
}

// Delete removes a named property. It returns true if the property existed.
func (o *JSObject) Delete(key string) bool {
	if _, ok := o.Properties[key]; ok {
		delete(o.Properties, key)
		return true
	}
	return false
}

// GetIndex returns the indexed element for arrays.
func (o *JSObject) GetIndex(i int) (JSValue, bool) {
	if !o.IsArray || i < 0 || i >= len(o.Elements) {
		return Undefined(), false
	}
	return o.Elements[i], true
}

// SetIndex assigns an indexed element, growing the backing slice as needed.
func (o *JSObject) SetIndex(i int, value JSValue) {
	if !o.IsArray {
		o.IsArray = true
		o.ClassName = "Array"
	}
	for i >= len(o.Elements) {
		o.Elements = append(o.Elements, Undefined())
	}
	o.Elements[i] = value
}

// Push appends an element to the end of an array.
func (o *JSObject) Push(value JSValue) {
	if !o.IsArray {
		o.IsArray = true
		o.ClassName = "Array"
	}
	o.Elements = append(o.Elements, value)
}

// Length returns the array length or the function-valued length property.
func (o *JSObject) Length() int {
	if o.IsArray {
		return len(o.Elements)
	}
	if v, ok := o.Properties["length"]; ok {
		return int(v.ToNumber())
	}
	return len(o.Properties)
}

// NativeFunc is the Go signature for a native (host) function callable from JS.
type NativeFunc func(interp *Interpreter, this JSValue, args []JSValue) JSValue

// JSFunction is the Go translation of JSC::JSFunction. It unifies native Go callbacks
// and script-defined closures (script functions are stored as a closure capturing an
// environment plus the compiled function body).
type JSFunction struct {
	// Name is the function's display name.
	Name string
	// Native, when non-nil, is a host function implemented in Go.
	Native NativeFunc
	// Closure, when non-nil, is a script-defined function.
	Closure *Closure
	// Properties holds function-instance properties.
	properties *JSObject
	// length is the formal parameter count (for .length).
	length int
}

// Closure captures the environment and compiled body of a script-defined function.
type Closure struct {
	// Body is the compiled bytecode block.
	Body *FunctionBody
	// Env is the captured lexical environment at definition time.
	Env *Environment
	// Name is the function name.
	Name string
}

// NewNativeFunction constructs a JSFunction backed by a Go callback.
func NewNativeFunction(name string, fn NativeFunc, length int) *JSFunction {
	return &JSFunction{
		Name:       name,
		Native:     fn,
		length:     length,
		properties: NewObject(nil),
	}
}

// NewScriptFunction constructs a JSFunction backed by a closure.
func NewScriptFunction(name string, body *FunctionBody, env *Environment, length int) *JSFunction {
	return &JSFunction{
		Name: name,
		Closure: &Closure{
			Body: body,
			Env:  env,
			Name: name,
		},
		length:     length,
		properties: NewObject(nil),
	}
}

// Properties returns the function-instance property object.
func (f *JSFunction) Properties() *JSObject { return f.properties }

// Length returns the formal parameter count.
func (f *JSFunction) Length() int { return f.length }

// FunctionBody is a forward-declared compiled function body. It is filled in by the
// BytecodeGenerator and consumed by the Interpreter.
type FunctionBody struct {
	// Name is the function name.
	Name string
	// Params is the list of formal parameter names.
	Params []string
	// Instructions is the bytecode instruction slice.
	Instructions []Instruction
	// NumLocals is the number of local variable slots required.
	NumLocals int
	// IsArrow marks arrow functions (no own this binding).
	IsArrow bool
	// IsTopLevel marks the top-level program body. When true the interpreter runs the
	// body directly against the global environment instead of a fresh child scope, so
	// that top-level var/function declarations persist on the global environment and
	// remain reachable after Run returns (mirroring script-scope semantics).
	IsTopLevel bool
	// IsAsync marks async functions. When true the function execution wraps its
	// result in a Promise and the await instruction can be used.
	IsAsync bool
}

// String returns a debug representation of a JSValue's tag.
func (t JSValueTag) String() string {
	switch t {
	case TagUndefined:
		return "undefined"
	case TagNull:
		return "null"
	case TagBoolean:
		return "boolean"
	case TagNumber:
		return "number"
	case TagString:
		return "string"
	case TagObject:
		return "object"
	case TagFunction:
		return "function"
	}
	return fmt.Sprintf("tag(%d)", int(t))
}
