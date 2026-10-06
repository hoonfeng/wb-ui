package jsc

// JS 引擎后端的契约（实现路径主线 C 的 C-P2：后端接口抽象）。
//
// 背景（docs/implementation-path.md §4.3）：主线 C 的做法不是「goja 或 V8 二选一」，
// 而是把 jsc 适配层做成**可替换后端**——默认纯 Go 的 goja，需要 JIT/完整 ES 时切 V8。
//
// 本文件给的是**后端契约**（Backend / RuntimeHandle），backend_goja.go 是 goja 的
// 落地（也是全包唯一 import goja 的地方），backend_v8.go 是 C-P3 的接入点。
//
// 契约刻意保持**最小**：只列「任何后端都必须提供」的能力——建运行时、执行脚本、
// 取全局对象。其余能力（streams / Web API / 事件循环）都建立在这三样之上，已由
// jsc 的后端无关代码实现（那些代码只用 be* 名字，见 backend_goja.go 的说明）。
// 不在契约里预先堆接口方法：等 V8 后端真正接进来时，按**实际编译错误**补齐，
// 而不是凭想象设计一套没人用的抽象。

// Backend 是一个 JS 引擎后端。
type Backend interface {
	// Name 是后端名（"goja" / "v8"）；用于诊断、探针与 SetActiveBackend 选择。
	Name() string
	// NewRuntime 建一个独立的运行时（每个 Worker 一个）。
	NewRuntime() RuntimeHandle
}

// RuntimeHandle 是后端运行时的抽象句柄：宿主/worker 只需要「执行脚本」与
// 「取全局对象」两件事，其余交互都通过 be* 值句柄进行。
type RuntimeHandle interface {
	// RunString 编译并执行一段脚本，返回结果值（后端句柄）。
	RunString(src string) (any, error)
	// GlobalObject 取全局对象（后端句柄）。
	GlobalObject() any
}
