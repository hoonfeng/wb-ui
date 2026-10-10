package jsc

import (
	"testing"
	"time"
)

// TestHasReadyTasksIgnoresFutureTimers 是「触发事件后绘制停止」BUG 的**核心**
// 回归断言：
//
//   - PendingTasks 把**尚未到期**的 setTimeout/setInterval 也算作 pending
//     （len(macrotasks) 含未到期项 —— 它们只是留在队列里等时间到）；
//   - 宿主若拿它当「事件循环是否已排空」的判据，页面只要有一个未到期定时器，
//     判据就恒为 false，只能等满超时 —— 期间不布局不绘制（画面冻结）。
//
// 因此这里同时断言两边：未到期定时器 **不算** HasReadyTasks，但 **仍计入**
// PendingTasks（后者是旧判定必然等满超时的根源）。
func TestHasReadyTasksIgnoresFutureTimers(t *testing.T) {
	rt := NewInterpreter()
	el := rt.EnsureEventLoop()
	cb, err := rt.RunJS("(function(){})")
	if err != nil {
		t.Fatalf("RunJS: %v", err)
	}

	if id := el.SetTimeout(cb, 40); id == 0 {
		t.Fatal("SetTimeout 返回 0（回调未排队）")
	}
	if el.HasReadyTasks() {
		t.Fatal("HasReadyTasks = true：未到期的 setTimeout 不该算作「可立即执行」")
	}
	if el.PendingTasks() == 0 {
		t.Fatal("PendingTasks = 0：未到期定时器必须计入（旧判定等满超时的根源）")
	}

	// 宿主每帧的 ProcessTasks 不该把未到期任务驱动掉。
	el.ProcessTasks(0)
	if el.HasReadyTasks() {
		t.Fatal("ProcessTasks 后 HasReadyTasks = true：未到期定时器不该被执行")
	}

	// 到期后变为就绪；执行后队列清空。
	time.Sleep(60 * time.Millisecond)
	if !el.HasReadyTasks() {
		t.Fatal("到期后 HasReadyTasks = false：应可就绪")
	}
	el.ProcessTasks(0)
	if el.HasReadyTasks() {
		t.Fatal("一次性定时器执行完毕后 HasReadyTasks 应为 false")
	}
}

// TestHasReadyTasksImmediateKinds 微任务 / rAF / setImmediate 一旦入队即可
// 立即执行（与时间无关），必须算就绪。
func TestHasReadyTasksImmediateKinds(t *testing.T) {
	cases := []struct {
		name string
		put  func(el *EventLoop, cb JSValue)
	}{
		{"queueMicrotask", func(el *EventLoop, cb JSValue) { el.QueueMicrotask(cb) }},
		{"requestAnimationFrame", func(el *EventLoop, cb JSValue) { el.RequestAnimationFrame(cb) }},
		{"setImmediate", func(el *EventLoop, cb JSValue) { el.SetImmediate(cb) }},
		{"setTimeout0", func(el *EventLoop, cb JSValue) { el.SetTimeout(cb, 0) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := NewInterpreter()
			el := rt.EnsureEventLoop()
			cb, err := rt.RunJS("(function(){})")
			if err != nil {
				t.Fatalf("RunJS: %v", err)
			}
			if el.HasReadyTasks() {
				t.Fatal("空循环 HasReadyTasks = true")
			}
			tc.put(el, cb)
			if !el.HasReadyTasks() {
				t.Fatal("入队后 HasReadyTasks = false：立即执行的任务必须算就绪")
			}
			el.ProcessTasks(0)
			if el.HasReadyTasks() {
				t.Fatal("执行完毕后 HasReadyTasks 应为 false")
			}
		})
	}
}

// TestHasReadyTasksNilLoop nil 接收者安全返回 false（调用方无需判空）。
func TestHasReadyTasksNilLoop(t *testing.T) {
	var el *EventLoop
	if el.HasReadyTasks() {
		t.Fatal("nil EventLoop HasReadyTasks = true")
	}
}

// TestDrainReadySemanticsKeepsShortChainAndSkipsFarTimer 模拟宿主的「事件后
// 同步驱动到排空」循环（判据 = HasReadyTasks）：
//
//   - 未到期的**远**定时器（5s）不得让循环继续等待；
//   - 因而不阻塞当前帧：循环立即返回；
//   - 远定时器始终不执行（由后续帧按时间推进，符合浏览器语义）。
//
// 作为对照，同一条链在**旧判据**（PendingTasks）下会一直等到 deadline 才退出
// —— 这正是「触发事件后每次点击都白等 120~180ms、画面冻结」的机制。
func TestDrainReadySemanticsKeepsShortChainAndSkipsFarTimer(t *testing.T) {
	rt := NewInterpreter()
	el := rt.EnsureEventLoop()
	// 注意：裸 Interpreter 没有 window（只有 DOM 绑定的页面才有），
	// 用 globalThis 挂状态。
	if _, err := rt.RunJS(`globalThis.__log = [];`); err != nil {
		t.Fatalf("RunJS: %v", err)
	}
	near, err := rt.RunJS(`(function(){ globalThis.__log.push('near'); })`)
	if err != nil {
		t.Fatalf("RunJS near: %v", err)
	}
	far, err := rt.RunJS(`(function(){ globalThis.__log.push('far'); })`)
	if err != nil {
		t.Fatalf("RunJS far: %v", err)
	}
	el.SetTimeout(near, 5)     // 短链：本帧内到期
	el.SetTimeout(far, 5000)   // 远定时器：绝不该阻塞本帧

	const timeout = 40 * time.Millisecond
	deadline := time.Now().Add(timeout)

	// ── 旧判据（PendingTasks）：必然等满超时 ──────────────────────────
	start := time.Now()
	for {
		rt.RunJobs()
		el.ProcessTasks(0)
		if el.PendingTasks() == 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if old := time.Since(start); old < timeout/2 {
		t.Fatalf("旧判据（PendingTasks）应在未到期远定时器上等满超时（%v），实际 %v", timeout, old)
	}

	// ── 新判据（HasReadyTasks）：未到期任务不再阻塞 ────────────────────
	log := func() string {
		v, err := rt.RunJS("globalThis.__log.join(',')")
		if err != nil {
			t.Fatalf("RunJS log: %v", err)
		}
		return v.ToString()
	}
	deadline = time.Now().Add(timeout)
	start = time.Now()
	for {
		rt.RunJobs()
		el.ProcessTasks(0)
		if !el.HasReadyTasks() || time.Now().After(deadline) {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if elapsed := time.Since(start); elapsed > timeout/2 {
		t.Fatalf("新判据应立即返回（未到期定时器不阻塞），实际 %v", elapsed)
	}
	if got := log(); got != "near" {
		t.Fatalf("短链定时器应已执行且远定时器不得执行，log = %q，want \"near\"", got)
	}
}
