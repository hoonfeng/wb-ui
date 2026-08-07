// Package jsc 提供基于 goja 的 JavaScript 引擎适配层。
package jsc

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"wb-ui.com/goja"
)

// ─── BufferLogger ───────────────────────────────────────

type BufferLogger struct {
	mu    sync.Mutex
	Lines []string
}

func (l *BufferLogger) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	text := string(p)
	l.Lines = append(l.Lines, text)
	return len(p), nil
}

func (l *BufferLogger) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.Lines, "")
}

// ─── NativeFunc ─────────────────────────────────────────

type NativeFunc func(in *Interpreter, this JSValue, args []JSValue) JSValue

// ─── Interpreter ────────────────────────────────────────

type Interpreter struct {
	vm        *goja.Runtime
	eventLoop *EventLoop
}

func NewInterpreter() *Interpreter {
	vm := goja.New()
	return &Interpreter{vm: vm}
}

func (r *Interpreter) SetupGlobal(logger *BufferLogger) {
	consoleObj := r.vm.NewObject()
	logFunc := func(call goja.FunctionCall) goja.Value {
		parts := make([]string, len(call.Arguments))
		for i, arg := range call.Arguments {
			parts[i] = formatConsoleArg(arg)
		}
		line := strings.Join(parts, " ")
		if logger != nil {
			fmt.Fprintln(logger, line)
		} else {
			fmt.Println(line)
		}
		return goja.Undefined()
	}
	consoleObj.Set("log", logFunc)
	consoleObj.Set("error", logFunc)
	consoleObj.Set("warn", logFunc)
	consoleObj.Set("info", logFunc)
	consoleObj.Set("debug", logFunc)
	r.vm.Set("console", consoleObj)
}

// formatConsoleArg 将 console.log 参数格式化为可读文本。
// 对象/数组尝试 JSON 序列化（避免 Go map 的 %v 打印出 map[]），
// 失败时回退到 Go %v。
func formatConsoleArg(v goja.Value) string {
	if v == nil {
		return "<nil>"
	}
	if obj, ok := v.(*goja.Object); ok {
		if b, err := json.Marshal(obj.Export()); err == nil {
			return string(b)
		}
	}
	return fmt.Sprintf("%v", v.Export())
}

func (r *Interpreter) GlobalObject() *JSObject {
	return &JSObject{obj: r.vm.GlobalObject(), interp: r}
}

func (r *Interpreter) ObjectPrototype() *JSObject {
	return &JSObject{obj: r.vm.NewObject(), interp: r}
}

func (r *Interpreter) Run(code string) (interface{}, error) {
	val, err := r.vm.RunString(code)
	if err != nil {
		return nil, err
	}
	return val.Export(), nil
}

// RunJS 执行代码并返回 JSValue。
func (r *Interpreter) RunJS(code string) (JSValue, error) {
	val, err := r.vm.RunString(code)
	if err != nil {
		return JSValue{}, err
	}
	return JSValue{v: val, interp: r}, nil
}

func (r *Interpreter) Evaluate(code string) (interface{}, error) {
	return r.Run(code)
}

func (r *Interpreter) Call(fn JSValue, this JSValue, args []JSValue) (JSValue, error) {
	gojaArgs := make([]goja.Value, len(args))
	for i, a := range args {
		gojaArgs[i] = a.val(r.vm)
	}
	result, err := r.vm.Call(fn.val(r.vm), this.val(r.vm), gojaArgs...)
	if err != nil {
		return JSValue{}, err
	}
	return JSValue{v: result, interp: r}, nil
}

// RunJobs flushes goja's native Promise microtask queue. goja runs these
// implicitly when RunProgram returns, but Runtime.Call does NOT — so after
// invoking a JS callback via Call (DOM event dispatch → Vue @click handler),
// embedders must call RunJobs or Vue's reactive scheduler (Promise.then
// based) never runs and the DOM never updates.
func (r *Interpreter) RunJobs() {
	r.vm.RunJobs()
}

// ValueOf 把 Go 值转换为绑定到本运行时的 JSValue，供宿主主动调用 JS
// 函数（CallFunction / Call）传参使用。支持 string / bool / int / int64 /
// float64 / float32 / nil / []any / map[string]any / JSValue；其他类型回退
// 为 undefined。
func (r *Interpreter) ValueOf(v any) JSValue {
	if v == nil {
		return Null()
	}
	switch val := v.(type) {
	case JSValue:
		return val
	case string:
		return StringValue(val)
	case bool:
		return BooleanValue(val)
	case int:
		return NumberValue(float64(val))
	case int64:
		return NumberValue(float64(val))
	case float64:
		return NumberValue(val)
	case float32:
		return NumberValue(float64(val))
	case []any:
		items := make([]interface{}, len(val))
		for i, it := range val {
			items[i] = r.ValueOf(it).val(r.vm)
		}
		return JSValue{v: r.vm.NewArray(items...), interp: r}
	case map[string]any:
		obj := r.vm.NewObject()
		for k, mv := range val {
			obj.Set(k, r.ValueOf(mv).val(r.vm))
		}
		return JSValue{v: obj, interp: r}
	}
	return Undefined()
}

// NewNativeFunction 在正确运行时创建原生函数（推荐用法）。
func (r *Interpreter) NewNativeFunction(name string, fn NativeFunc, _ int) *JSFunction {
	fv := r.wrapNativeFunc(fn, r)
	return &JSFunction{
		v:       fv,
		id:      fmt.Sprintf("nf:%s:%p", name, fn),
		wrapped: true,
	}
}

// NewConstructor 创建一个可 new 调用的构造函数。fn 接收 (interpreter, this, args)，
// 返回新创建的 JSObject（作为 new 表达式的结果）。
func (r *Interpreter) NewConstructor(name string, fn func(in *Interpreter, this JSValue, args []JSValue) *JSObject) *JSFunction {
	constVal := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		interp := r
		this := JSValue{v: call.This, interp: interp}
		args := make([]JSValue, len(call.Arguments))
		for i, a := range call.Arguments {
			args[i] = JSValue{v: a, interp: interp}
		}
		result := fn(interp, this, args)
		if result == nil || result.obj == nil {
			return nil
		}
		return result.obj
	})
	return &JSFunction{
		v:       constVal,
		id:      fmt.Sprintf("ctor:%s:%p", name, fn),
		wrapped: true,
	}
}

// wrapNativeFunc 使用指定 goja.Runtime 包装 NativeFunc。
func (r *Interpreter) wrapNativeFunc(fn NativeFunc, interp *Interpreter) goja.Value {
	return r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		this := JSValue{v: call.This, interp: interp}
		args := make([]JSValue, len(call.Arguments))
		for i, a := range call.Arguments {
			args[i] = JSValue{v: a, interp: interp}
		}
		result := fn(interp, this, args)
		if result.v == nil {
			return goja.Undefined()
		}
		return result.v
	})
}

func (r *Interpreter) ResolvePromise(val JSValue) JSValue {
	p, resolve, _ := r.vm.NewPromise()
	_ = resolve(val.Export())
	return JSValue{v: p.PromiseObj(), interp: r}
}

func (r *Interpreter) RejectPromise(errStr JSValue) JSValue {
	p, _, reject := r.vm.NewPromise()
	_ = reject(errStr.Export())
	return JSValue{v: p.PromiseObj(), interp: r}
}

func (r *Interpreter) VM() *goja.Runtime { return r.vm }

// ─── JSValue ────────────────────────────────────────────

type JSValue struct {
	v        goja.Value
	nativeFn NativeFunc // 未绑定运行时的原生函数（包级 NewNativeFunction 使用）
	interp   *Interpreter
}

// val 返回底层 goja.Value。对于未绑定的原生函数，使用指定运行时创建包装。
func (v JSValue) val(rt *goja.Runtime) goja.Value {
	if v.v != nil {
		return v.v
	}
	if v.nativeFn != nil && rt != nil {
		interp := v.interp
		if interp == nil {
			interp = &Interpreter{vm: rt}
		}
		// 在目标运行时创建包装
		return interp.wrapNativeFunc(v.nativeFn, interp)
	}
	return goja.Undefined()
}

func (v JSValue) IsUndefined() bool { return v.v == nil && v.nativeFn == nil }
func (v JSValue) IsNull() bool      { return v.v != nil && goja.IsNull(v.v) }
func (v JSValue) IsBoolean() bool {
	if v.v == nil {
		return false
	}
	if _, isObj := v.v.(*goja.Object); isObj {
		return false
	}
	_, ok := v.v.Export().(bool)
	return ok
}
func (v JSValue) IsNumber() bool {
	if v.v == nil {
		return false
	}
	if _, isObj := v.v.(*goja.Object); isObj {
		return false
	}
	switch v.v.Export().(type) {
	case int64, float64:
		return true
	}
	return false
}
func (v JSValue) IsString() bool {
	if v.v == nil {
		return false
	}
	if _, isObj := v.v.(*goja.Object); isObj {
		return false
	}
	_, ok := v.v.Export().(string)
	return ok
}
func (v JSValue) IsCallable() bool {
	return v.nativeFn != nil || (v.v != nil && v.v.ToBoolean() && v.AsFunction() != nil)
}
func (v JSValue) IsObject() bool {
	if v.v == nil {
		return v.nativeFn != nil
	}
	_, isObj := v.v.(*goja.Object)
	return isObj || v.nativeFn != nil
}
func (v JSValue) IsFunction() bool {
	return v.nativeFn != nil || (v.v != nil && v.v.ToBoolean() && v.AsFunction() != nil)
}

func (v JSValue) SameAs(other JSValue) bool {
	if v.nativeFn != nil || other.nativeFn != nil {
		if v.nativeFn == nil || other.nativeFn == nil {
			return false
		}
		return reflect.ValueOf(v.nativeFn).Pointer() == reflect.ValueOf(other.nativeFn).Pointer()
	}
	// goja.Value 接口比较：对象/函数为指针比较（同一 JS 对象 → true），
	// 原始值（string/number/bool）按值比较。
	return v.v == other.v
}

func (v JSValue) ToString() string {
	if v.v == nil || goja.IsUndefined(v.v) || goja.IsNull(v.v) {
		return ""
	}
	return v.v.String()
}

func (v JSValue) ToBoolean() bool {
	if v.v == nil {
		return false
	}
	return v.v.ToBoolean()
}

func (v JSValue) ToNumber() float64 {
	if v.v == nil {
		return 0
	}
	return v.v.ToFloat()
}

func (v JSValue) AsBoolean() bool   { return v.ToBoolean() }
func (v JSValue) AsNumber() float64 { return v.ToNumber() }
func (v JSValue) AsString() string  { return v.ToString() }

func (v JSValue) AsObject() *JSObject {
	if v.v == nil {
		return nil
	}
	obj, ok := v.v.(*goja.Object)
	if !ok {
		return nil
	}
	interp := v.interp
	if interp == nil {
		interp = &Interpreter{vm: obj.Runtime()}
	}
	return &JSObject{obj: obj, interp: interp}
}

func (v JSValue) AsFunction() *JSFunction {
	if v.v != nil {
		return &JSFunction{v: v.v, id: fmt.Sprintf("js:%p", v.v), wrapped: true}
	}
	if v.nativeFn != nil {
		return &JSFunction{nativeFn: v.nativeFn, id: fmt.Sprintf("nf:%p", v.nativeFn)}
	}
	return nil
}

func (v JSValue) Export() interface{} {
	if v.v == nil {
		return nil
	}
	return v.v.Export()
}

// ─── JSObject ───────────────────────────────────────────

type JSObject struct {
	obj    *goja.Object
	interp *Interpreter
}

func (o *JSObject) Internal() interface{} {
	if o == nil || o.obj == nil {
		return nil
	}
	return o.obj.Internal
}

func (o *JSObject) SetInternal(v interface{}) {
	if o == nil || o.obj == nil {
		return
	}
	o.obj.Internal = v
}

func (o *JSObject) Set(key string, val JSValue) {
	if o == nil || o.obj == nil {
		return
	}
	targetRt := o.obj.Runtime()
	// ★ 关键：未绑定的 nativeFn（包级 NewNativeFunction + FunctionValue）没有
	//   interp。必须用 o.interp（宿主创建的真实 Interpreter）包装，否则
	//   val() 会临时 new 一个 &Interpreter{vm: rt}——它的 eventLoop 是独立的
	//   新实例！bindings 里 setTimeout/setInterval 等经 g.Set 注册的全局函数
	//   会把任务挂到那个孤儿 EventLoop，而 host.ProcessTasks 驱动的是真实
	//   interpreter 的 EventLoop → 所有前端定时器永不触发。
	if val.nativeFn != nil && val.interp == nil && o.interp != nil {
		val.interp = o.interp
	}
	o.obj.Set(key, val.val(targetRt))
}

// SetIterator 把对象的 Symbol.iterator 设为 fn（每次迭代调用 fn 返回
// 迭代器对象，支持 Array.from / for...of / 展开运算符）。
func (o *JSObject) SetIterator(fn NativeFunc) {
	if o == nil || o.obj == nil || o.interp == nil {
		return
	}
	_ = o.obj.SetSymbol(goja.SymIterator, o.interp.wrapNativeFunc(fn, o.interp))
}

func (o *JSObject) GetStr(key string) JSValue {
	if o == nil || o.obj == nil {
		return JSValue{}
	}
	v := o.obj.Get(key)
	if v == nil {
		return JSValue{v: goja.Undefined()}
	}
	return JSValue{v: v, interp: o.interp}
}

func (o *JSObject) GetByKey(key string) (JSValue, bool) {
	if o == nil || o.obj == nil {
		return JSValue{}, false
	}
	v := o.obj.Get(key)
	if v == nil {
		return JSValue{}, false
	}
	return JSValue{v: v, interp: o.interp}, true
}

func (o *JSObject) GetOrZero(key string) JSValue {
	v, _ := o.GetByKey(key)
	return v
}

func (o *JSObject) SetAccessor(prop string, getter, setter interface{}) {
	if o == nil || o.obj == nil || o.interp == nil {
		return
	}
	rt := o.interp.vm
	interp := o.interp

	var gfn goja.Value
	switch g := getter.(type) {
	case func(*Interpreter) JSValue:
		gfn = rt.ToValue(func(call goja.FunctionCall) goja.Value {
			val := g(interp)
			return val.val(rt)
		})
	case func(*Interpreter, JSValue) JSValue:
		gfn = rt.ToValue(func(call goja.FunctionCall) goja.Value {
			val := g(interp, JSValue{})
			return val.val(rt)
		})
	}

	var sfn goja.Value = goja.Undefined()
	if setter != nil {
		switch s := setter.(type) {
		case func(*Interpreter, JSValue, JSValue):
			sfn = rt.ToValue(func(call goja.FunctionCall) goja.Value {
				v := JSValue{v: call.Argument(0), interp: interp}
				s(interp, JSValue{}, v)
				return goja.Undefined()
			})
		}
	}
	if gfn != nil {
		_ = o.obj.DefineAccessorProperty(prop, gfn, sfn, goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
}

func (o *JSObject) Keys() []string {
	if o == nil || o.obj == nil {
		return nil
	}
	return o.obj.Keys()
}

func (o *JSObject) SetClassName(name string) {
	if o == nil || o.obj == nil || o.interp == nil {
		return
	}
	o.obj.Set("constructor", o.interp.vm.ToValue(map[string]interface{}{
		"name": name,
	}))
}

// ─── JSFunction ─────────────────────────────────────────

type JSFunction struct {
	v        goja.Value
	id       string
	nativeFn NativeFunc // 未绑定的原生函数
	wrapped  bool        // true 表示已绑定到运行时
}

func (f *JSFunction) String() string { return f.id }

// ─── 工厂函数 ───────────────────────────────────────────

func StringValue(s string) JSValue  { return JSValue{v: goja.NewString(s)} }
func NumberValue(f float64) JSValue { return JSValue{v: goja.NewFloat(f)} }
func BooleanValue(b bool) JSValue   { return JSValue{v: goja.NewBoolean(b)} }
func Null() JSValue                 { return JSValue{v: goja.Null()} }
func Undefined() JSValue            { return JSValue{} }

func ObjectValue(o *JSObject) JSValue {
	if o == nil || o.obj == nil {
		return Null()
	}
	return JSValue{v: o.obj, interp: o.interp}
}

func FunctionValue(f *JSFunction) JSValue {
	if f == nil {
		return Undefined()
	}
	if f.wrapped && f.v != nil {
		return JSValue{v: f.v}
	}
	if f.nativeFn != nil {
		return JSValue{nativeFn: f.nativeFn}
	}
	return Undefined()
}

func NewObject(proto *JSObject) *JSObject {
	var interp *Interpreter
	if proto != nil && proto.interp != nil {
		interp = proto.interp
	} else {
		interp = &Interpreter{vm: goja.New()}
	}
	obj := &JSObject{obj: interp.vm.NewObject(), interp: interp}
	// proto 参数必须真正生效：设置原型链（vendored goja 无 SetPrototype API，
	// 用标准 __proto__ 赋值触发 setter，与 Object.setPrototypeOf 等价）。
	// 此前忽略 proto 导致 wrapElement 创建的实例原型为 Object.prototype，
	// Element.prototype 上的方法（setAttribute 等）无法被实例继承。
	if proto != nil && proto.obj != nil {
		obj.obj.Set("__proto__", proto.obj)
	}
	return obj
}

// WrapObject wraps an existing goja.Object (e.g. from NewDynamicObject) as a JSObject.
// The object must belong to the same runtime as the interpreter.
func WrapObject(obj *goja.Object, interp *Interpreter) *JSObject {
	return &JSObject{obj: obj, interp: interp}
}

func NewArray(proto *JSObject, items []JSValue) *JSObject {
	var interp *Interpreter
	if proto != nil && proto.interp != nil {
		interp = proto.interp
	} else {
		interp = &Interpreter{vm: goja.New()}
	}
	gojaItems := make([]interface{}, len(items))
	for i, item := range items {
		gojaItems[i] = item.val(interp.vm)
	}
	return &JSObject{obj: interp.vm.NewArray(gojaItems...), interp: interp}
}

// NewArrayForInterp 在指定 Interpreter 的运行时中创建数组，避免跨运行时问题。
func NewArrayForInterp(in *Interpreter, items []JSValue) *JSObject {
	if in == nil || in.vm == nil {
		return NewArray(nil, items)
	}
	gojaItems := make([]interface{}, len(items))
	for i, item := range items {
		gojaItems[i] = item.val(in.vm)
	}
	return &JSObject{obj: in.vm.NewArray(gojaItems...), interp: in}
}

// NewNativeFunction 创建原生 JS 函数（包级函数）。
// 函数值将在设置到 JSObject 时用目标运行时包装。
func NewNativeFunction(name string, fn NativeFunc, _ int) *JSFunction {
	return &JSFunction{
		id:       fmt.Sprintf("nnf:%s:%p", name, fn),
		nativeFn: fn,
	}
}
