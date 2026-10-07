// Package jsc 提供基于 goja 的 JavaScript 引擎适配层。
package jsc

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/maphash"
	"math"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ─── BufferLogger ───────────────────────────────────────

// LogEntry 是一条**带级别**的控制台记录。CDP 的 Log.entryAdded 与
// Runtime.consoleAPICalled 需要级别（error/warn 在 DevTools 里是红/黄），而引擎早
// 期的日志管道只存纯文本——Lines 因此保留（既有消费者不受影响），级别走 Entries。
type LogEntry struct {
	Level string // log/info/warn/error/debug
	Text  string
}

type BufferLogger struct {
	mu      sync.Mutex
	Lines   []string
	Entries []LogEntry
}

func (l *BufferLogger) Write(p []byte) (int, error) {
	return l.WriteLevel("log", p)
}

// WriteLevel 写入一条带级别的记录（console.warn/error/... 按各自级别落账）。
func (l *BufferLogger) WriteLevel(level string, p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	text := string(p)
	l.Lines = append(l.Lines, text)
	l.Entries = append(l.Entries, LogEntry{Level: level, Text: text})
	return len(p), nil
}

// ConsoleEntries 返回「自 since 起」的带级别条目与新的游标（增量语义：宿主适配层
// 记游标，只把新条目转成 CDP 事件）。
func (l *BufferLogger) ConsoleEntries(since int) ([]LogEntry, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if since < 0 || since > len(l.Entries) {
		since = len(l.Entries)
	}
	out := make([]LogEntry, len(l.Entries)-since)
	copy(out, l.Entries[since:])
	return out, len(l.Entries)
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
	vm        *beRuntime
	eventLoop *EventLoop

	// 编译缓存：大脚本（如前端 bundle）重复执行时跳过 parse+compile。
	// goja 的 *Program 与 runtime 解耦，可跨 runtime 复用。
	progMu    sync.Mutex
	progCache map[uint64]cachedProgram
	progBytes int
}

func NewInterpreter() *Interpreter {
	vm := beNew()
	return &Interpreter{vm: vm}
}

func (r *Interpreter) SetupGlobal(logger *BufferLogger) {
	consoleObj := r.vm.NewObject()
	// 按级别落账（level 透传给 BufferLogger.Entries，供 CDP 的 Log.entryAdded /
	// Runtime.consoleAPICalled 保真上报）。此前五个方法共用一个不分级的函数，
	// 前端在 DevTools 里看不到 error/warn 的红黄区分。
	logAt := func(level string) func(call beFunctionCall) beValue {
		return func(call beFunctionCall) beValue {
			parts := make([]string, len(call.Arguments))
			for i, arg := range call.Arguments {
				parts[i] = formatConsoleArg(arg)
			}
			line := strings.Join(parts, " ")
			if logger != nil {
				_, _ = logger.WriteLevel(level, []byte(line+"\n"))
			} else {
				fmt.Println(line)
			}
			return beUndefined()
		}
	}
	consoleObj.Set("log", logAt("log"))
	consoleObj.Set("error", logAt("error"))
	consoleObj.Set("warn", logAt("warn"))
	consoleObj.Set("info", logAt("info"))
	consoleObj.Set("debug", logAt("debug"))
	r.vm.Set("console", consoleObj)
}

// formatConsoleArg 将 console.log 参数格式化为可读文本。
// 对象/数组尝试 JSON 序列化（避免 Go map 的 %v 打印出 map[]），
// 失败时回退到 Go %v。
func formatConsoleArg(v beValue) string {
	if v == nil {
		return "<nil>"
	}
	if obj, ok := v.(*beObject); ok {
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
	// 大脚本走编译缓存：重复加载同一 bundle 时省去 parse+compile
	// （10MB 量级的 bundle 单次 compile 约 600ms）。
	if len(code) >= progCacheMinLen {
		val, err := r.runCached(code)
		if err != nil {
			return JSValue{}, err
		}
		return JSValue{v: val, interp: r}, nil
	}
	val, err := r.vm.RunString(code)
	if err != nil {
		return JSValue{}, err
	}
	return JSValue{v: val, interp: r}, nil
}

// progCacheMinLen 是启用编译缓存的脚本长度阈值：小于此长度 parse 开销小，
// 直接 RunString（避免 hash 与缓存管理开销）。
const progCacheMinLen = 64 * 1024

// progCacheMaxBytes 是缓存脚本源码的字节数上限（约 3 个 10MB bundle）。
// Program 编译产物约为源码的 5~7 倍，超限时清空缓存避免内存膨胀。
const progCacheMaxBytes = 32 * 1024 * 1024

type cachedProgram struct {
	srcLen int
	prog   *beProgram
}

var progHashSeed = maphash.MakeSeed()

func progHash(s string) uint64 {
	return maphash.String(progHashSeed, s)
}

// runCached 编译大脚本并缓存其 *beProgram，命中缓存时直接 RunProgram。
// 用 hash + srcLen 双重校验避免 hash 碰撞误命中；编译失败不缓存；
// 缓存总源码字节超限时整体清空。
func (r *Interpreter) runCached(code string) (beValue, error) {
	key := progHash(code)

	r.progMu.Lock()
	if cp, ok := r.progCache[key]; ok && cp.srcLen == len(code) {
		r.progMu.Unlock()
		if os.Getenv("WB_JS_TIMING") != "" {
			t0 := time.Now()
			ret, err := r.vm.RunProgram(cp.prog)
			fmt.Printf("[JS-TIMING] cached run=%v srcLen=%d err=%v\n", time.Since(t0), len(code), err)
			return ret, err
		}
		return r.vm.RunProgram(cp.prog)
	}
	r.progMu.Unlock()

	t0 := time.Now()
	prog, err := beCompile("cached.js", code, false)
	if err != nil {
		return nil, err
	}
	compileDur := time.Since(t0)

	r.progMu.Lock()
	if r.progCache == nil {
		r.progCache = make(map[uint64]cachedProgram)
	}
	if r.progBytes+len(code) > progCacheMaxBytes {
		// 超限：整体清空（简单策略，避免缓存多个大 bundle 导致内存膨胀）。
		r.progCache = make(map[uint64]cachedProgram)
		r.progBytes = 0
	}
	r.progCache[key] = cachedProgram{srcLen: len(code), prog: prog}
	r.progBytes += len(code)
	r.progMu.Unlock()

	t1 := time.Now()
	var ms0, ms1 runtime.MemStats
	runtime.ReadMemStats(&ms0)
	ret, err := r.vm.RunProgram(prog)
	runtime.ReadMemStats(&ms1)
	if os.Getenv("WB_JS_TIMING") != "" {
		fmt.Printf("[JS-TIMING] compile=%v run=%v gcCount=%d gcPause=%v heap=%dMB->%dMB totalAlloc=%dMB srcLen=%d err=%v\n",
			compileDur, time.Since(t1), ms1.NumGC-ms0.NumGC,
			time.Duration(ms1.PauseTotalNs-ms0.PauseTotalNs),
			ms0.HeapAlloc>>20, ms1.HeapAlloc>>20, ms1.TotalAlloc>>20, len(code), err)
	}
	return ret, err
}

func (r *Interpreter) Evaluate(code string) (interface{}, error) {
	return r.Run(code)
}

func (r *Interpreter) Call(fn JSValue, this JSValue, args []JSValue) (JSValue, error) {
	gojaArgs := make([]beValue, len(args))
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
	case []byte:
		// []byte → Uint8Array（live2d 模型字节 / 通用二进制透传）。
		// ★ 用 vm.New 而不是 vm.Call：typed array 构造器要求 **new 调用**，
		//   `vm.Call(ctor, undefined, ab)` 会被拒绝 → 静默退化为下面的 ArrayBuffer
		//   兜底（实测见 engine/js/jsc/bytes_value_test.go：改前 ctor=ArrayBuffer 且
		//   **没有** .buffer 属性，与注释承诺不符，调用方按 Uint8Array 读会踩空）。
		vm := r.vm
		abVal := vm.ToValue(vm.NewArrayBuffer(val))
		if ctor := vm.Get("Uint8Array"); ctor != nil {
			if arr, err := vm.New(ctor, abVal); err == nil {
				return JSValue{v: arr, interp: r}
			}
		}
		return JSValue{v: abVal, interp: r}
	}
	return Undefined()
}

// ArrayBufferBytes 取 JS 值的字节：ArrayBuffer 直接取；TypedArray / DataView 取其
// buffer 并按 byteOffset / byteLength 切片（规范上 decodeAudioData 同时接受这两类）。
// 其他类型返回 ok=false。
//
// ★ 为什么要有它：be* 句柄对 bindings 层不透明，「读页面交来的 ArrayBuffer」这种
// 能力必须由适配层提供（WebAudio 的 decodeAudioData(arrayBuffer) 正是第一处消费者）。
func (r *Interpreter) ArrayBufferBytes(v JSValue) ([]byte, bool) {
	if r == nil || r.vm == nil || v.v == nil {
		return nil, false
	}
	if b, ok := beArrayBufferBytes(v.v); ok {
		return b, true
	}
	obj, ok := v.v.(*beObject)
	if !ok {
		return nil, false
	}
	buf := obj.Get("buffer")
	if buf == nil || beIsUndefined(buf) || beIsNull(buf) {
		return nil, false
	}
	b, ok := beArrayBufferBytes(buf)
	if !ok {
		return nil, false
	}
	off, n := 0, len(b)
	if ov := obj.Get("byteOffset"); ov != nil && !beIsUndefined(ov) {
		off = int(ov.ToInteger())
	}
	if nv := obj.Get("byteLength"); nv != nil && !beIsUndefined(nv) {
		n = int(nv.ToInteger())
	}
	if off < 0 || off > len(b) {
		return nil, false
	}
	if n < 0 || off+n > len(b) {
		n = len(b) - off
	}
	return b[off : off+n], true
}

// Float32ArrayValue 把样本切片转成 JS 的 Float32Array。
//
// ★ 为什么补在适配层：jsc 此前只有内部的 []byte → Uint8Array 转换，没有「创建
// Float32Array」的公开能力；WebAudio 的 AudioBuffer.getChannelData(i) 按规范必须
// 返回 Float32Array（库普遍按 length / 下标 / instanceof 使用）。
//
// 实现：样本先写进 ArrayBuffer（一次拷贝），再用 Float32Array 视图包装——避免逐
// 元素 Set（10 万级样本下差异明显）。
func (r *Interpreter) Float32ArrayValue(data []float32) JSValue {
	if r == nil || r.vm == nil {
		return Undefined()
	}
	raw := make([]byte, len(data)*4)
	for i, f := range data {
		binary.LittleEndian.PutUint32(raw[i*4:], math.Float32bits(f))
	}
	abVal := r.vm.ToValue(r.vm.NewArrayBuffer(raw))
	if ctor := r.vm.Get("Float32Array"); ctor != nil {
		if arr, err := r.vm.New(ctor, abVal); err == nil {
			return JSValue{v: arr, interp: r}
		}
	}
	// 退化路径（理论上不可达：Float32Array 是 goja 内建）：返回普通数组——宁可
	// 慢，也不返回一个类型不对的值让调用方在 instanceof 上踩空。
	items := make([]interface{}, len(data))
	for i, f := range data {
		items[i] = float64(f)
	}
	return JSValue{v: r.vm.NewArray(items...), interp: r}
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
	constVal := r.vm.ToValue(func(call beConstructorCall) *beObject {
		interp := r
		this := JSValue{v: call.This, interp: interp}
		n := len(call.Arguments)
		var buf [8]JSValue
		var args []JSValue
		if n <= len(buf) {
			args = buf[:n]
		} else {
			args = make([]JSValue, n)
		}
		for i, a := range call.Arguments {
			args[i] = JSValue{v: a, interp: interp}
		}
		result := fn(interp, this, args)
		if result == nil || result.obj == nil {
			return nil
		}
		return result.obj
	})
	// ★ 让 fn.name 反映构造器名（浏览器语义：HTMLDivElement.name ===
	// "HTMLDivElement"）。goja 对 Go 函数默认取「Go 函数全名」
	//（如 wb-ui/engine/js/jsc.(*Interpreter).NewConstructor.func1），而脚本普遍读
	// ctor.name / el.constructor.name 做类型分派（React、Vue、Lit、各类
	// isElement 辅助函数）。函数 name 在 goja 里是 configurable 的，
	// 故用 DefineDataProperty 覆盖（普通赋值会被 writable:false 静默忽略）。
	if obj, ok := constVal.(*beObject); ok {
		_ = obj.DefineDataProperty("name", r.vm.ToValue(name),
			beFLAGFALSE, beFLAGTRUE, beFLAGFALSE)
	}
	return &JSFunction{
		v:       constVal,
		id:      fmt.Sprintf("ctor:%s:%p", name, fn),
		wrapped: true,
	}
}

// wrapNativeFunc 使用指定 beRuntime 包装 NativeFunc。
// ★ 性能：每次调用 make([]JSValue, n) 分配——Vue mount 百万级 DOM API
// 调用（createElement/setAttribute/style 等）都走此边界。改为栈上小数组
// （≤8 参数零分配；>8 才 make）。fn 若存储 args 逃逸时编译器自动转堆，
// 语义不变。
func (r *Interpreter) wrapNativeFunc(fn NativeFunc, interp *Interpreter) beValue {
	return r.vm.ToValue(func(call beFunctionCall) beValue {
		this := JSValue{v: call.This, interp: interp}
		n := len(call.Arguments)
		var buf [8]JSValue
		var args []JSValue
		if n <= len(buf) {
			args = buf[:n]
		} else {
			args = make([]JSValue, n)
		}
		for i, a := range call.Arguments {
			args[i] = JSValue{v: a, interp: interp}
		}
		result := fn(interp, this, args)
		if result.v == nil {
			return beUndefined()
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

func (r *Interpreter) VM() *beRuntime { return r.vm }

// ─── JSValue ────────────────────────────────────────────

type JSValue struct {
	v        beValue
	nativeFn NativeFunc // 未绑定运行时的原生函数（包级 NewNativeFunction 使用）
	interp   *Interpreter
}

// val 返回底层 beValue。对于未绑定的原生函数，使用指定运行时创建包装。
func (v JSValue) val(rt *beRuntime) beValue {
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
	return beUndefined()
}

func (v JSValue) IsUndefined() bool { return v.v == nil && v.nativeFn == nil }
func (v JSValue) IsNull() bool      { return v.v != nil && beIsNull(v.v) }
func (v JSValue) IsBoolean() bool {
	if v.v == nil {
		return false
	}
	if _, isObj := v.v.(*beObject); isObj {
		return false
	}
	_, ok := v.v.Export().(bool)
	return ok
}
func (v JSValue) IsNumber() bool {
	if v.v == nil {
		return false
	}
	if _, isObj := v.v.(*beObject); isObj {
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
	if _, isObj := v.v.(*beObject); isObj {
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
	_, isObj := v.v.(*beObject)
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
	// beValue 接口比较：对象/函数为指针比较（同一 JS 对象 → true），
	// 原始值（string/number/bool）按值比较。
	return v.v == other.v
}

// Interp 返回该值所属的 JS 解释器（nil = 未绑定原生函数/运行时）。
// 用于跨解释器注册表（bindings 事件监听 side-table）按解释器过滤清理。
func (v JSValue) Interp() *Interpreter { return v.interp }

func (v JSValue) ToString() string {
	if v.v == nil || beIsUndefined(v.v) || beIsNull(v.v) {
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
	obj, ok := v.v.(*beObject)
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
	obj    *beObject
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

// Delete 删除对象的一个自有属性（等价 JS 的 `delete obj.key`）。属性不存在
// 也算成功（JS 语义）；属性不可配置时返回 false。
//
// 用途：bindings 隐藏浏览器全局（HideGlobal）——把 `Worker`/`WebSocket` 从
// global 上**真正删掉**，`"Worker" in window` 因此是 false。置 undefined 只能
// 让 `typeof` 判定正确，`in` 仍为 true，靠 `in` 做 feature detect 的库会误判。
func (o *JSObject) Delete(key string) bool {
	if o == nil || o.obj == nil || key == "" {
		return false
	}
	return o.obj.Delete(key) == nil
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
	// API 级插桩（perfapi.go）：名单内的方法在**注册时**包一层计时 wrapper
	// （wrapper 内部自带开关判断，默认关闭时原样返回 fn，运行路径零开销）。
	// 放在这里而不是逐个 binding 调用点，是为了让所有经 Set 注册的方法
	// （含 fn1/fn2 这类通用包装产生的 "fn" 名）都能按注册名被统计。
	if val.nativeFn != nil {
		val.nativeFn = perfWrapNative(key, val.nativeFn)
	}
	o.obj.Set(key, val.val(targetRt))
}

// SetIterator 把对象的 Symbol.iterator 设为 fn（每次迭代调用 fn 返回
// 迭代器对象，支持 Array.from / for...of / 展开运算符）。
func (o *JSObject) SetIterator(fn NativeFunc) {
	if o == nil || o.obj == nil || o.interp == nil {
		return
	}
	_ = o.obj.SetSymbol(beSymIterator, o.interp.wrapNativeFunc(fn, o.interp))
}

func (o *JSObject) GetStr(key string) JSValue {
	if o == nil || o.obj == nil {
		return JSValue{}
	}
	v := o.obj.Get(key)
	if v == nil {
		return JSValue{v: beUndefined()}
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

	var gfn beValue
	switch g := getter.(type) {
	case func(*Interpreter) JSValue:
		g = perfWrapGet1(prop, g)
		gfn = rt.ToValue(func(call beFunctionCall) beValue {
			val := g(interp)
			return val.val(rt)
		})
	case func(*Interpreter, JSValue) JSValue:
		g = perfWrapGet2(prop, g)
		gfn = rt.ToValue(func(call beFunctionCall) beValue {
			val := g(interp, JSValue{})
			return val.val(rt)
		})
	}

	var sfn beValue = beUndefined()
	if setter != nil {
		switch s := setter.(type) {
		case func(*Interpreter, JSValue, JSValue):
			s = perfWrapSet(prop, s)
			sfn = rt.ToValue(func(call beFunctionCall) beValue {
				v := JSValue{v: call.Argument(0), interp: interp}
				s(interp, JSValue{}, v)
				return beUndefined()
			})
		}
	}
	if gfn != nil {
		_ = o.obj.DefineAccessorProperty(prop, gfn, sfn, beFLAGTRUE, beFLAGTRUE)
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
	v        beValue
	id       string
	nativeFn NativeFunc // 未绑定的原生函数
	wrapped  bool       // true 表示已绑定到运行时
}

func (f *JSFunction) String() string { return f.id }

// ─── 工厂函数 ───────────────────────────────────────────

func StringValue(s string) JSValue  { return JSValue{v: beNewString(s)} }
func NumberValue(f float64) JSValue { return JSValue{v: beNewFloat(f)} }
func BooleanValue(b bool) JSValue   { return JSValue{v: beNewBoolean(b)} }
func Null() JSValue                 { return JSValue{v: beNull()} }
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
		interp = &Interpreter{vm: beNew()}
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

// WrapObject wraps an existing beObject (e.g. from NewDynamicObject) as a JSObject.
// The object must belong to the same runtime as the interpreter.
func WrapObject(obj *beObject, interp *Interpreter) *JSObject {
	return &JSObject{obj: obj, interp: interp}
}

// SetObjectPrototype 把 obj 的 [[Prototype]] 设为 proto（等价于 JS 侧
// Object.setPrototypeOf(obj, proto)），成功返回 true。
//
// ★ 为什么需要这个 API：普通对象可以走 `obj.Set("__proto__", …)`（Object.prototype
// 上 `__proto__` 是访问器属性，赋值即触发 setter），但 **dynamic object**
// （NewDynamicObject 包装的 handler，如 bindings/styleProxy）会拦截 Set ——
// 赋 "__proto__" 会被 handler 当成业务属性处理（styleProxy 会把它当 CSS 属性写进
// style 属性）。goja 的文档亦明确：dynamic object 的原型只能通过
// Object.SetPrototype()（Go）或 Object.setPrototypeOf()（JS）修改。
//
// proto 为 nil、或 proto 属于另一个 runtime 时返回 false（调用方保持原原型）——
// 跨 runtime 复用对象会被 goja 拒绝（"Illegal runtime transition of an Object"）。
func SetObjectPrototype(obj *JSObject, proto *JSObject) bool {
	if obj == nil || obj.obj == nil || proto == nil || proto.obj == nil {
		return false
	}
	// ★ 不要在这里比较 *Interpreter 指针：同一个 runtime 可能有多个 Interpreter
	//   包装（AsObject 等路径），指针不等会被误判成「跨 runtime」而静默失败
	//   （第 20 轮实测：el.style 的原型因此未设上）。跨 runtime 传入 proto 由
	//   goja 自身校验（SetPrototype → runtime.try 捕获，返回 error → 这里返回 false）。
	return obj.obj.SetPrototype(proto.obj) == nil
}

func NewArray(proto *JSObject, items []JSValue) *JSObject {
	var interp *Interpreter
	if proto != nil && proto.interp != nil {
		interp = proto.interp
	} else {
		// 无 proto 时复用 items 中第一个已绑定 runtime 的对象，避免把
		// 属于其他 runtime 的对象塞进新建 runtime 的数组 → goja 抛
		// "Illegal runtime transition of an Object"（ResizeObserver /
		// MutationObserver / IntersectionObserver 回调、NodeList 等
		// 元素/记录数组都走这里）。
		for _, item := range items {
			if item.interp != nil && item.interp.vm != nil {
				interp = item.interp
				break
			}
		}
		if interp == nil {
			interp = &Interpreter{vm: beNew()}
		}
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
