// Package jsc — EventLoop: JavaScript 事件循环实现
//
// 对标浏览器的事件循环模型：
//   1. 微任务队列（microtasks）：Promise.then/catch/finally、queueMicrotask
//      每个宏任务执行完后立即清空微任务队列
//   2. 宏任务队列（macrotasks）：setTimeout、setInterval
//      按到期时间排序，逐个执行，每执行一个后清空微任务
//   3. 动画帧队列（animation frames）：requestAnimationFrame
//      在 ProcessTasks 的结尾、渲染之前执行
//
// 驱动方式：宿主（app.Host / webkit.WebView）在渲染循环中调用
// EventLoop.ProcessTasks(elapsedMs) 来驱动事件循环。

package jsc

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"wb-ui/goja"
	"wb-ui/dom"
)

// ─── ScheduledTask ──────────────────────────────────────

// scheduledTask 表示一个已调度的回调任务。
type scheduledTask struct {
	ID       int
	Callback JSValue // JS 函数回调
	DueTime  int64   // 到期时间（单调毫秒）
	Delay    int64   // 原始延迟（用于 setInterval 重复调度）
	Repeat   bool    // true = setInterval，到期后重新调度
}

func (t *scheduledTask) IsDue(nowMs int64) bool {
	return nowMs >= t.DueTime
}

// ─── Microtask ──────────────────────────────────────────

// microtask 表示一个微任务（Promise.then 回调或 queueMicrotask）。
type microtask struct {
	Callback JSValue
}

// ─── EventLoop ──────────────────────────────────────────

// EventLoop 管理 JavaScript 事件循环。
// 每个 JS 运行时（Interpreter）应关联一个 EventLoop。
// 宿主通过 ProcessTasks(elapsedMs) 驱动事件循环。
type EventLoop struct {
	interp *Interpreter

	mu sync.Mutex

	// macrotasks 按 DueTime 排序的宏任务队列（setTimeout / setInterval）。
	macrotasks []*scheduledTask

	// microtasks 微任务队列（Promise.then / queueMicrotask）。
	microtasks []*microtask

	// animFrames 动画帧回调队列（requestAnimationFrame）。
	animFrames []*scheduledTask

	// timerIDSeq 自增定时器 ID。
	timerIDSeq int32

	// animIDSeq 自增动画帧 ID。
	animIDSeq int32

	// startTime 事件循环启动时的单调时间（毫秒）。
	startTime int64

	// running 标记事件循环是否在运行。
	running bool
}

// NewEventLoop 创建一个绑定到给定 Interpreter 的事件循环。
func NewEventLoop(interp *Interpreter) *EventLoop {
	el := &EventLoop{
		interp:      interp,
		startTime:   time.Now().UnixMilli(),
		macrotasks:  make([]*scheduledTask, 0),
		microtasks:  make([]*microtask, 0),
		animFrames:  make([]*scheduledTask, 0),
	}
	interp.eventLoop = el
	return el
}

// GetEventLoop 返回关联到此 Interpreter 的 EventLoop（可能为 nil）。
func (r *Interpreter) GetEventLoop() *EventLoop {
	return r.eventLoop
}

// SetEventLoop 设置关联到此 Interpreter 的 EventLoop。
func (r *Interpreter) SetEventLoop(el *EventLoop) {
	r.eventLoop = el
}

// EnsureEventLoop 确保 Interpreter 有关联的 EventLoop，没有则创建一个。
func (r *Interpreter) EnsureEventLoop() *EventLoop {
	if r.eventLoop == nil {
		NewEventLoop(r)
	}
	return r.eventLoop
}

// ─── 公共 API（由 bindings 层调用）──────────────────────

// SetTimeout 调度一个宏任务在 delayMs 毫秒后执行一次。
// 返回定时器 ID，可用于 clearTimeout。
func (el *EventLoop) SetTimeout(callback JSValue, delayMs int64) int {
	if !callback.IsCallable() {
		return 0
	}
	id := int(atomic.AddInt32(&el.timerIDSeq, 1))
	el.mu.Lock()
	defer el.mu.Unlock()
	nowMs := el.nowMs()
	task := &scheduledTask{
		ID:       id,
		Callback: callback,
		DueTime:  nowMs + delayMs,
		Delay:    delayMs,
		Repeat:   false,
	}
	el.insertMacrotask(task)
	return id
}

// SetInterval 调度一个宏任务每 intervalMs 毫秒重复执行。
// 返回定时器 ID，可用于 clearInterval。
func (el *EventLoop) SetInterval(callback JSValue, intervalMs int64) int {
	if !callback.IsCallable() {
		return 0
	}
	id := int(atomic.AddInt32(&el.timerIDSeq, 1))
	el.mu.Lock()
	defer el.mu.Unlock()
	nowMs := el.nowMs()
	task := &scheduledTask{
		ID:       id,
		Callback: callback,
		DueTime:  nowMs + intervalMs,
		Delay:    intervalMs,
		Repeat:   true,
	}
	el.insertMacrotask(task)
	return id
}

// ClearTimeout 取消指定 ID 的 setTimeout 任务。
func (el *EventLoop) ClearTimeout(id int) {
	el.removeMacrotask(id)
}

// ClearInterval 取消指定 ID 的 setInterval 任务。
func (el *EventLoop) ClearInterval(id int) {
	el.removeMacrotask(id)
}

// RequestAnimationFrame 注册一个动画帧回调，在下次 ProcessTasks 结尾执行。
// 返回动画帧 ID，可用于 CancelAnimationFrame。
func (el *EventLoop) RequestAnimationFrame(callback JSValue) int {
	if !callback.IsCallable() {
		return 0
	}
	id := int(atomic.AddInt32(&el.animIDSeq, 1))
	el.mu.Lock()
	defer el.mu.Unlock()
	el.animFrames = append(el.animFrames, &scheduledTask{
		ID:       id,
		Callback: callback,
	})
	return id
}

// CancelAnimationFrame 取消指定 ID 的动画帧回调。
func (el *EventLoop) CancelAnimationFrame(id int) {
	el.mu.Lock()
	defer el.mu.Unlock()
	for i, t := range el.animFrames {
		if t.ID == id {
			el.animFrames = append(el.animFrames[:i], el.animFrames[i+1:]...)
			return
		}
	}
}

// QueueMicrotask 将一个回调加入微任务队列。
func (el *EventLoop) QueueMicrotask(callback JSValue) {
	if !callback.IsCallable() {
		return
	}
	el.mu.Lock()
	defer el.mu.Unlock()
	el.microtasks = append(el.microtasks, &microtask{Callback: callback})
}

// ─── 驱动 API（由宿主调用）──────────────────────────────

// ProcessTasks 处理所有到期的任务。
// elapsedMs 是自事件循环启动以来经过的毫秒数。
// 处理顺序：宏任务 → 微任务（清空） → 重复直到无宏任务 → 动画帧回调。
// 宿主应在渲染循环中定期调用此方法。
func (el *EventLoop) ProcessTasks(_ int64) {
	el.running = true

	// ★ 统一时间基准：宏任务的 DueTime 由 SetTimeout 用 el.nowMs()（EventLoop
	//   自身 startTime 相对时间）计算；ProcessTasks 必须用同一时钟判断到期。
	//   宿主传入的 elapsedMs（host.animStart 基准）与 EventLoop.startTime 不同
	//   步，会导致所有 setTimeout/setInterval 永不触发（到期时间永远达不到）。
	now := el.nowMs()

	// 处理宏任务 + 微任务，直到宏任务队列为空。
	// ★ 帧预算（浏览器模型）：一帧只处理有限个宏任务，然后进入
	// 微任务 + 动画帧阶段——保证 requestAnimationFrame 回调每帧必执行。
	// 之前无预算时，Vue 渲染/Bridge 轮询持续产生 setTimeout，宏任务循环
	// 永不退出，flushAnimationFrames 永远到不了 → rAF 回调（CodeMirror 6
	// 的 measure/行高探测）从不执行 → HeightOracle 停留默认 lineHeight=14
	// → 行号栏按 14px/行步进而内容 18.2px，逐行错位。剩余宏任务由宿主
	// 下一帧的 ProcessTasks 继续处理。
	budget := 500
	for budget > 0 {
		task := el.popNextMacrotask(now)
		if task == nil {
			break
		}
		budget--
		// 执行宏任务回调。
		el.executeCallback(task.Callback)

		// 如果是 setInterval，重新调度（在当前时间 + 间隔）。
		if task.Repeat {
			el.mu.Lock()
			newTask := &scheduledTask{
				ID:       task.ID,
				Callback: task.Callback,
				DueTime:  el.nowMs() + task.Delay,
				Delay:    task.Delay,
				Repeat:   true,
			}
			el.insertMacrotask(newTask)
			el.mu.Unlock()
		}

		// 每个宏任务执行完后，清空所有微任务。
		el.flushMicrotasks()

		// 宏任务可能调度新的宏任务（setTimeout 内再 setTimeout）——刷新 now。
		now = el.nowMs()
	}

	// 即使没有宏任务，也清空微任务（处理 Promise 回调）。
	el.flushMicrotasks()

	// 执行动画帧回调。
	el.flushAnimationFrames()

	el.running = false
}

// IsRunning 返回事件循环是否正在处理任务。
func (el *EventLoop) IsRunning() bool {
	return el.running
}

// PendingTasks 返回待处理任务总数（用于判断是否需要继续渲染）。
func (el *EventLoop) PendingTasks() int {
	el.mu.Lock()
	defer el.mu.Unlock()
	return len(el.macrotasks) + len(el.microtasks) + len(el.animFrames)
}

// ─── 内部方法 ──────────────────────────────────────────

func (el *EventLoop) nowMs() int64 {
	return time.Now().UnixMilli() - el.startTime
}

// insertMacrotask 按 DueTime 有序插入宏任务（调用方需持有锁）。
func (el *EventLoop) insertMacrotask(task *scheduledTask) {
	idx := sort.Search(len(el.macrotasks), func(i int) bool {
		return el.macrotasks[i].DueTime >= task.DueTime
	})
	el.macrotasks = append(el.macrotasks, nil)
	copy(el.macrotasks[idx+1:], el.macrotasks[idx:])
	el.macrotasks[idx] = task
}

// popNextMacrotask 弹出并返回下一个到期的宏任务（线程安全）。
func (el *EventLoop) popNextMacrotask(nowMs int64) *scheduledTask {
	el.mu.Lock()
	defer el.mu.Unlock()
	if len(el.macrotasks) == 0 {
		return nil
	}
	if !el.macrotasks[0].IsDue(nowMs) {
		return nil
	}
	task := el.macrotasks[0]
	el.macrotasks = el.macrotasks[1:]
	return task
}

// removeMacrotask 按 ID 移除宏任务（用于 clearTimeout/clearInterval）。
func (el *EventLoop) removeMacrotask(id int) {
	el.mu.Lock()
	defer el.mu.Unlock()
	for i, t := range el.macrotasks {
		if t.ID == id {
			el.macrotasks = append(el.macrotasks[:i], el.macrotasks[i+1:]...)
			return
		}
	}
}

// flushMicrotasks 清空所有微任务（包括 MutationObserver 记录）。
func (el *EventLoop) flushMicrotasks() {
	for {
		// 先投递 MutationObserver 记录
		dom.FlushMutationObservers()

		el.mu.Lock()
		if len(el.microtasks) == 0 {
			el.mu.Unlock()
			// goja 的原生 Promise 微任务（Vue scheduler）也可能已排队
			//（宏任务回调里 Promise.then）——一并 flush。
			el.interp.RunJobs()
			return
		}
		// 取出当前所有微任务（快照），清空队列。
		tasks := el.microtasks
		el.microtasks = nil
		el.mu.Unlock()

		for _, t := range tasks {
			el.executeCallback(t.Callback)
		}
		// 执行微任务后可能产生新的 MutationObserver 记录，继续循环
	}
}

// flushAnimationFrames 清空所有动画帧回调。
func (el *EventLoop) flushAnimationFrames() {
	el.mu.Lock()
	frames := el.animFrames
	el.animFrames = nil
	el.mu.Unlock()

	for _, f := range frames {
		el.executeCallback(f.Callback)
	}
}

// executeCallback 在 JS 运行时中安全执行一个回调。
func (el *EventLoop) executeCallback(cb JSValue) {
	if !cb.IsCallable() {
		return
	}
	// 使用 Interpreter.Call 执行回调。
	_, _ = el.interp.Call(cb, Undefined(), nil)
	// ★ Flush goja 原生 Promise 微任务（Vue 响应式 scheduler 走
	// Promise.resolve().then）——Runtime.Call 不会自动跑 jobQueue。
	el.interp.RunJobs()
}

// ─── Runtime 池化（内存优化）─────────────────────────

var runtimePool = sync.Pool{
	New: func() interface{} {
		return NewInterpreter()
	},
}

// GetPooledInterpreter 从池中获取一个 Interpreter（重置后返回）。
func GetPooledInterpreter() *Interpreter {
	rt := runtimePool.Get().(*Interpreter)
	// 重置全局状态（goja Runtime 内部状态通过新建 VM 重置）
	rt.vm = goja.New()
	rt.eventLoop = nil
	return rt
}

// PutPooledInterpreter 将 Interpreter 归还池中。
func PutPooledInterpreter(rt *Interpreter) {
	if rt == nil {
		return
	}
	runtimePool.Put(rt)
}
