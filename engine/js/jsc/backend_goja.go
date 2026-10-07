package jsc

// goja 后端的**唯一**落点（C-P2 后端接口抽象）。
//
// 本文件是全包唯一 import `wb-ui/engine/js/goja` 的地方：它把 goja 的类型与构造器
// 收敛成一组后端无关的名字（`be*`），其余文件（goja_adapter / streams / webapi /
// env / lazy / eventloop）只用 `be*`。于是「换后端」的落点变成**提供另一份同名定义**
// （backend_v8.go，C-P3 的接入点），而不必改动绑定层与流实现。
//
// 命名：`be` = backend（后端）。`beValue` 是后端值句柄（goja 侧等价 goja.Value），
// 对上层表现为不透明类型——除本文件外没有任何代码知道它背后是 goja。

import (
	"sync"

	"wb-ui/engine/js/goja"
)

// ─── 类型（后端句柄，对上层不透明） ───────────────────────
type (
	// beValue 是后端值句柄。
	beValue = goja.Value
	// beObject 是后端对象句柄。
	beObject = goja.Object
	// beRuntime 是后端运行时。
	beRuntime = goja.Runtime
	// beFunctionCall 是函数调用上下文（this + arguments）。
	beFunctionCall = goja.FunctionCall
	// beConstructorCall 是构造调用上下文（new 的 this + arguments）。
	beConstructorCall = goja.ConstructorCall
	// beProgram 是已编译脚本（可跨 runtime 复用，见 Interpreter 的编译缓存）。
	beProgram = goja.Program
	// beDynamicObject 是「Go 值伪装成 JS 对象」的接口（styleProxy 等用它）。
	beDynamicObject = goja.DynamicObject
	// beCallable 是可调用值（本仓库 vendored goja 的签名是
	// `func(this Value, args ...Value) (Value, error)`，调用方按 `fn(this, args...)` 用）。
	beCallable = goja.Callable
)

// ─── 基础值构造 ──────────────────────────────────────────

// beUndefined / beNull 返回后端的基础单例值。
func beUndefined() beValue { return goja.Undefined() }
func beNull() beValue      { return goja.Null() }

// beIsUndefined / beIsNull 判定基础单例值（跨后端统一比较入口）。
func beIsUndefined(v beValue) bool { return goja.IsUndefined(v) }
func beIsNull(v beValue) bool      { return goja.IsNull(v) }

// beNew 建一个后端运行时。
func beNew() *beRuntime { return goja.New() }

// beNewString / beNewFloat / beNewBoolean 造标量值。本仓库 vendored goja 的这三个是
// **包级**构造器（不接 runtime——值本身与 runtime 无关，见 engine/js/goja/value.go）。
func beNewString(s string) beValue { return goja.NewString(s) }
func beNewFloat(f float64) beValue { return goja.NewFloat(f) }
func beNewBoolean(b bool) beValue  { return goja.NewBoolean(b) }

// beCompile 编译脚本（name 只用于错误信息与栈帧）。
func beCompile(name, src string, strict bool) (*beProgram, error) {
	return goja.Compile(name, src, strict)
}

// beAssertFunction 把值断言成可调用对象（不可调用时返回 ok=false）。
// ★ 本仓库 vendored goja 的 Callable 签名是 `func(this Value, args ...Value) (Value, error)`
// （不是上游的 `func(FunctionCall) Value`），调用方按 `fn(this, args...)` 用。
func beAssertFunction(v beValue) (beCallable, bool) {
	return goja.AssertFunction(v)
}

// beArrayBufferBytes 取底层 ArrayBuffer 的字节（非 ArrayBuffer 时 ok=false）。
// 与其它 be* 句柄一样对上层不透明：上层经 Interpreter.ArrayBufferBytes 使用，
// 「换后端」时这里换一份同名实现即可。
func beArrayBufferBytes(v beValue) ([]byte, bool) {
	if v == nil {
		return nil, false
	}
	if ab, ok := v.Export().(goja.ArrayBuffer); ok {
		return ab.Bytes(), true
	}
	return nil, false
}

// ─── 常量 / 特殊符号 ─────────────────────────────────────

const (
	beFLAGTRUE  = goja.FLAG_TRUE
	beFLAGFALSE = goja.FLAG_FALSE
)

// beSymIterator 是后端的 Symbol.iterator（streams 用它与 Go 迭代器互通）。
var beSymIterator = goja.SymIterator

// ─── 后端注册（C-P3 的接入点） ───────────────────────────

// backendRegistry 记录可用后端；当前只有 goja（默认）。
var backendRegistry = struct {
	mu       sync.Mutex
	backends map[string]Backend
	active   string
}{backends: map[string]Backend{}}

// RegisterBackend 注册一个后端实现（按名字）。重复注册同名后端会覆盖——
// C-P3 接入 V8 时在这里注册 "v8"。
func RegisterBackend(b Backend) {
	if b == nil || b.Name() == "" {
		return
	}
	backendRegistry.mu.Lock()
	defer backendRegistry.mu.Unlock()
	backendRegistry.backends[b.Name()] = b
	if backendRegistry.active == "" {
		backendRegistry.active = b.Name()
	}
}

// SetActiveBackend 选择默认后端（名字未注册时返回 false，保持原选择）。
func SetActiveBackend(name string) bool {
	backendRegistry.mu.Lock()
	defer backendRegistry.mu.Unlock()
	if _, ok := backendRegistry.backends[name]; !ok {
		return false
	}
	backendRegistry.active = name
	return true
}

// ActiveBackend 返回当前默认后端名与实现。
func ActiveBackend() (string, Backend) {
	backendRegistry.mu.Lock()
	defer backendRegistry.mu.Unlock()
	return backendRegistry.active, backendRegistry.backends[backendRegistry.active]
}

// BackendNames 返回已注册的后端名（诊断/探针用）。
func BackendNames() []string {
	backendRegistry.mu.Lock()
	defer backendRegistry.mu.Unlock()
	out := make([]string, 0, len(backendRegistry.backends))
	for name := range backendRegistry.backends {
		out = append(out, name)
	}
	return out
}

// gojaBackend 是 goja 的后端实现（契约见 backend.go）。
type gojaBackend struct{}

func (gojaBackend) Name() string { return "goja" }

func (gojaBackend) NewRuntime() RuntimeHandle {
	return &gojaRuntimeHandle{rt: beNew()}
}

// gojaRuntimeHandle 把 goja runtime 包成后端无关句柄。
type gojaRuntimeHandle struct{ rt *beRuntime }

func (h *gojaRuntimeHandle) RunString(src string) (any, error) {
	v, err := h.rt.RunString(src)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (h *gojaRuntimeHandle) GlobalObject() any { return h.rt.GlobalObject() }

func init() { RegisterBackend(gojaBackend{}) }
