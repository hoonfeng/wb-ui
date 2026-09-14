// Package worker 实现 Web Worker 的**并发脚本执行**（引擎的并发能力补全）。
//
// 设计要点：
//
//   - 每个 Worker 拥有**独立的 goja 运行时**（jsc.NewInterpreter）与独立的
//     事件循环，跑在**自己的 goroutine** 上。worker 里的死循环、大计算不会
//     阻塞页面主线程——这正是 Worker 存在的理由（真并发，不是"伪异步"）。
//   - 两个运行时之间**只传 JSON 文本**（结构化克隆的近似实现）。goja 非线程
//     安全，任何共享对象都会引入数据竞争；用文本传递换取零共享、零锁。
//   - worker → 主线程的消息交给宿主注入的 OnMessage 回调（bindings 层负责把
//     它排进主线程事件循环，**绝不在 worker goroutine 上执行主运行时的 JS**）。
//
// 已实现（classic worker 的核心面）：self / name / postMessage / onmessage /
// addEventListener / removeEventListener / close / importScripts /
// setTimeout / setInterval / clearTimeout / clearInterval / console。
//
// 未实现（有意保留，见 docs/CALIB.md）：module worker、SharedWorker /
// ServiceWorker、transferable 对象与 SharedArrayBuffer、MessageChannel /
// MessagePort、Blob URL 脚本（引擎无 Blob）、location / navigator 子集。
package worker

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"wb-ui/engine/js/jsc"
)

// ScriptFetcher 加载非 data: 的 worker 脚本（http(s)、file:、相对 URL）。
// 引擎默认不联网：只有宿主注入 Fetcher 后这些 URL 才可用。
type ScriptFetcher func(scriptURL string) (string, error)

// Options 描述一个 worker 的创建参数（对应 new Worker(url, options)）。
type Options struct {
	// URL 是脚本地址。data: URL 自包含（无需宿主）；其余交给 Fetcher。
	URL string

	// Script 是内联源码（引擎扩展，优先于 URL）：宿主或测试直接给脚本文本。
	Script string

	// Name 是 Worker 名字（worker 内的 self.name）。
	Name string

	// Logger 接收 worker 内 console.* 输出（nil 时打到 stdout）。
	Logger *jsc.BufferLogger

	// Fetcher 加载 URL 的脚本源码；nil 时非 data: URL 报错（NetworkError）。
	Fetcher ScriptFetcher

	// OnMessage 在 **worker 线程**被调用，参数是 JSON 文本。
	//
	// ★ 实现必须线程安全，且不得在此回调里执行主运行时的 JS——主运行时的 JS
	// 只能在主线程 tick 内执行（goja 非线程安全）。bindings 的 Worker 层做法是
	// 入队 + 在主事件循环上排一个宏任务。
	OnMessage func(jsonData string)

	// OnError 在 **worker 线程**被调用（脚本加载 / 执行错误）。约束同上。
	OnError func(name, message string)
}

// Worker 是一个运行中的 worker（Go 侧句柄）。
type Worker struct {
	opts Options

	// toWorker 主线程 → worker 的消息（JSON 文本）。带缓冲：postMessage 不阻塞
	// 主线程；队列满时丢弃（浏览器同样不做背压）。
	toWorker chan string

	quit chan struct{}
	done chan struct{}

	startOnce sync.Once
	stopOnce  sync.Once

	// ↓ 以下字段只被 worker goroutine 访问（无需加锁）。
	in        *jsc.Interpreter
	el        *jsc.EventLoop
	listeners map[string][]jsc.JSValue
	closed    bool
}

// New 创建一个 worker（尚未启动；调用 Start 后开始执行脚本）。
func New(opts Options) *Worker {
	return &Worker{
		opts:      opts,
		toWorker:  make(chan string, 128),
		quit:      make(chan struct{}),
		done:      make(chan struct{}),
		listeners: map[string][]jsc.JSValue{},
	}
}

// Start 启动 worker 的 goroutine（重复调用无副作用）。
func (w *Worker) Start() {
	w.startOnce.Do(func() { go w.run() })
}

// Name 返回 worker 名字。
func (w *Worker) Name() string { return w.opts.Name }

// Done 在 worker 线程退出后关闭（terminate / close / 宿主停止）。
func (w *Worker) Done() <-chan struct{} { return w.done }

// PostMessage 把 JSON 文本投递给 worker（主线程调用，非阻塞）。
func (w *Worker) PostMessage(jsonData string) {
	select {
	case <-w.quit:
	case w.toWorker <- jsonData:
	default:
		// 队列满：丢弃（与浏览器一致，不做背压）
	}
}

// Terminate 结束 worker：停掉 goroutine、丢弃待处理消息（主线程调用，幂等）。
func (w *Worker) Terminate() {
	w.stopOnce.Do(func() { close(w.quit) })
}

// ─── worker goroutine ───────────────────────────────────

func (w *Worker) run() {
	defer close(w.done)

	in := jsc.NewInterpreter()
	w.in = in
	// worker 全局只有 console（没有 DOM / window）。
	in.SetupGlobal(w.opts.Logger)
	w.el = jsc.NewEventLoop(in)

	w.installGlobals()
	w.runScript()

	// 定时器驱动：worker 自己的事件循环由本 goroutine 推进（setTimeout /
	// setInterval / Promise 微任务都在这里落地）。间隔取 4ms，与浏览器宏任务
	// 粒度同量级，且不影响主线程。
	ticker := time.NewTicker(4 * time.Millisecond)
	defer ticker.Stop()

	for {
		if w.closed {
			return
		}
		select {
		case data := <-w.toWorker:
			w.dispatchMessage(data)
		case <-ticker.C:
			if w.el.PendingTasks() > 0 {
				w.el.ProcessTasks(0)
			}
		case <-w.quit:
			return
		}
	}
}

// runScript 加载并执行 worker 脚本。加载/编译失败按规范派发 error 事件
// （worker 仍然存活，只是没有消息处理器）。
func (w *Worker) runScript() {
	src, err := w.scriptSource(w.opts.URL, w.opts.Script)
	if err != nil {
		w.emitError("NetworkError", err.Error())
		return
	}
	if _, err := w.in.RunJS(src); err != nil {
		w.emitError("Error", err.Error())
		return
	}
	w.in.RunJobs()
}

// scriptSource 解析脚本正文：内联源码优先，其次 data: URL，最后交给 Fetcher。
func (w *Worker) scriptSource(scriptURL, inline string) (string, error) {
	if inline != "" {
		return inline, nil
	}
	if scriptURL == "" {
		return "", errors.New("Failed to execute 'Worker': a script URL is required")
	}
	if strings.HasPrefix(strings.ToLower(scriptURL), "data:") {
		return decodeDataURL(scriptURL)
	}
	if w.opts.Fetcher != nil {
		return w.opts.Fetcher(scriptURL)
	}
	return "", fmt.Errorf("Failed to load worker script %q: no fetcher registered with the engine", scriptURL)
}

// decodeDataURL 解析 data: URL 的脚本正文（支持 base64 与百分号编码）。
func decodeDataURL(u string) (string, error) {
	comma := strings.IndexByte(u, ',')
	if comma < 0 {
		return "", errors.New("invalid data: URL for worker script")
	}
	meta, payload := u[len("data:"):comma], u[comma+1:]
	if strings.Contains(strings.ToLower(meta), ";base64") {
		clean := strings.TrimSpace(payload)
		if b, err := base64.StdEncoding.DecodeString(clean); err == nil {
			return string(b), nil
		}
		if b, err := base64.RawStdEncoding.DecodeString(clean); err == nil {
			return string(b), nil
		}
		return "", errors.New("invalid base64 payload in data: URL for worker script")
	}
	if s, err := url.PathUnescape(payload); err == nil {
		return s, nil
	}
	return payload, nil
}

// installGlobals 注入 worker 全局（DedicatedWorkerGlobalScope 的核心面）。
func (w *Worker) installGlobals() {
	in := w.in
	g := in.GlobalObject()

	// self 指向 worker 全局本身（脚本常写 self.onmessage = ...）。
	g.Set("self", jsc.ObjectValue(g))
	g.Set("name", jsc.StringValue(w.opts.Name))
	// on<type> 属性默认 null（脚本与库会做特性检测：'onmessage' in self）。
	g.Set("onmessage", jsc.Null())
	g.Set("onerror", jsc.Null())

	g.Set("postMessage", jsc.FunctionValue(in.NewNativeFunction("postMessage",
		func(interp *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			data := jsc.JSValue{}
			if len(args) >= 1 {
				data = args[0]
			}
			text, ok := StringifyJSON(interp, data)
			if !ok {
				panic(interp.VM().NewTypeError(
					"Failed to execute 'postMessage' on 'DedicatedWorkerGlobalScope': The object could not be cloned."))
			}
			w.emit(text)
			return jsc.Undefined()
		}, 1)))

	g.Set("close", jsc.FunctionValue(in.NewNativeFunction("close",
		func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			// close() 结束 worker：run 循环下一轮退出，不再处理消息。
			w.closed = true
			return jsc.Undefined()
		}, 0)))

	g.Set("addEventListener", jsc.FunctionValue(in.NewNativeFunction("addEventListener",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 2 || !args[1].IsCallable() {
				return jsc.Undefined()
			}
			t := args[0].ToString()
			for _, fn := range w.listeners[t] {
				if fn.SameAs(args[1]) {
					return jsc.Undefined() // 重复注册同一回调：忽略
				}
			}
			w.listeners[t] = append(w.listeners[t], args[1])
			return jsc.Undefined()
		}, 2)))

	g.Set("removeEventListener", jsc.FunctionValue(in.NewNativeFunction("removeEventListener",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 2 {
				return jsc.Undefined()
			}
			t := args[0].ToString()
			cur := w.listeners[t]
			out := cur[:0]
			for _, fn := range cur {
				if fn.SameAs(args[1]) {
					continue
				}
				out = append(out, fn)
			}
			w.listeners[t] = out
			return jsc.Undefined()
		}, 2)))

	g.Set("importScripts", jsc.FunctionValue(in.NewNativeFunction("importScripts",
		func(interp *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			for _, arg := range args {
				src, err := w.scriptSource(arg.ToString(), "")
				if err != nil {
					panic(interp.VM().NewGoError(err))
				}
				if _, err := interp.RunJS(src); err != nil {
					panic(interp.VM().NewGoError(err))
				}
			}
			return jsc.Undefined()
		}, 1)))

	// 定时器：挂在 worker 自己的事件循环上（由 run 的 ticker 驱动）。
	g.Set("setTimeout", jsc.FunctionValue(in.NewNativeFunction("setTimeout",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 1 || !args[0].IsCallable() {
				return jsc.NumberValue(0)
			}
			ms := int64(0)
			if len(args) >= 2 {
				ms = int64(args[1].ToNumber())
			}
			if ms < 0 {
				ms = 0
			}
			return jsc.NumberValue(float64(w.el.SetTimeout(args[0], ms)))
		}, 2)))
	g.Set("setInterval", jsc.FunctionValue(in.NewNativeFunction("setInterval",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 1 || !args[0].IsCallable() {
				return jsc.NumberValue(0)
			}
			ms := int64(0)
			if len(args) >= 2 {
				ms = int64(args[1].ToNumber())
			}
			if ms < 1 {
				ms = 1
			}
			return jsc.NumberValue(float64(w.el.SetInterval(args[0], ms)))
		}, 2)))
	g.Set("clearTimeout", jsc.FunctionValue(in.NewNativeFunction("clearTimeout",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) >= 1 {
				w.el.ClearTimeout(int(args[0].ToNumber()))
			}
			return jsc.Undefined()
		}, 1)))
	g.Set("clearInterval", jsc.FunctionValue(in.NewNativeFunction("clearInterval",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) >= 1 {
				w.el.ClearInterval(int(args[0].ToNumber()))
			}
			return jsc.Undefined()
		}, 1)))
}

// dispatchMessage 在 worker 线程上派发 main → worker 的 message 事件。
func (w *Worker) dispatchMessage(jsonData string) {
	in := w.in
	g := in.GlobalObject()
	ev := w.newMessageEvent(jsonData)

	if v, ok := g.GetByKey("onmessage"); ok && v.IsCallable() {
		_, _ = in.Call(v, jsc.ObjectValue(g), []jsc.JSValue{jsc.ObjectValue(ev)})
	}
	for _, fn := range w.listeners["message"] {
		_, _ = in.Call(fn, jsc.ObjectValue(g), []jsc.JSValue{jsc.ObjectValue(ev)})
	}
	// worker 里 await / Promise.then 的后续（Vue、库的异步分支）需要跑完微任务。
	in.RunJobs()
}

// newMessageEvent 构造 worker 侧的 MessageEvent（type/data/origin/source/ports）。
func (w *Worker) newMessageEvent(jsonData string) *jsc.JSObject {
	ev := jsc.NewObject(w.in.ObjectPrototype())
	ev.SetClassName("MessageEvent")
	ev.Set("type", jsc.StringValue("message"))
	ev.Set("data", ParseJSON(w.in, jsonData))
	ev.Set("origin", jsc.StringValue(""))
	ev.Set("lastEventId", jsc.StringValue(""))
	ev.Set("source", jsc.Null())
	ev.Set("ports", jsc.ObjectValue(jsc.NewArrayForInterp(w.in, nil)))
	return ev
}

func (w *Worker) emit(jsonData string) {
	if w.opts.OnMessage != nil {
		w.opts.OnMessage(jsonData)
	}
}

func (w *Worker) emitError(name, message string) {
	if w.opts.OnError != nil {
		w.opts.OnError(name, message)
	}
}

// ─── JSON 序列化（结构化克隆的近似实现）─────────────────

// ParseJSON 在指定运行时里把 JSON 文本解析成 JS 值（失败返回 null）。
// bindings 层派发主线程消息时复用同一实现。
func ParseJSON(in *jsc.Interpreter, text string) jsc.JSValue {
	out, err := jsonCall(in, "parse", jsc.StringValue(text))
	if err != nil {
		return jsc.Null()
	}
	return out
}

// StringifyJSON 在指定运行时里把 JS 值序列化为 JSON 文本。
// 返回 false 表示该值**不可克隆**（函数、undefined、循环引用等）。
//
// ★ 判定用 Export()==nil 而不是 IsUndefined()：goja 的 Undefined() 是一个
// 非 nil 的 Value，jsc.JSValue.IsUndefined()（判 v.v == nil）对
// JSON.stringify 返回的 undefined 会误判为"有值"，于是函数/Symbol 会被
// 序列化成字符串 "undefined" 而不是抛 DataCloneError。
func StringifyJSON(in *jsc.Interpreter, v jsc.JSValue) (string, bool) {
	out, err := jsonCall(in, "stringify", v)
	if err != nil || out.Export() == nil {
		return "", false
	}
	return out.ToString(), true
}

// jsonCall 调用运行时的 JSON.<name>（用 JS 自身的实现，保证与浏览器一致：
// 循环引用抛 TypeError、toJSON 生效、undefined 结果即"不可克隆"）。
func jsonCall(in *jsc.Interpreter, name string, args ...jsc.JSValue) (jsc.JSValue, error) {
	jsonVal := in.GlobalObject().GetStr("JSON")
	jsonObj := jsonVal.AsObject()
	if jsonObj == nil {
		return jsc.Undefined(), errors.New("JSON is not available in this runtime")
	}
	fn, ok := jsonObj.GetByKey(name)
	if !ok || !fn.IsCallable() {
		return jsc.Undefined(), errors.New("JSON." + name + " is not callable")
	}
	return in.Call(fn, jsonVal, args)
}
