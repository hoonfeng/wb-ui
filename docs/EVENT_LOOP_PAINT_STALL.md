# 事件派发后画面停止更新（drain 死等超时）

> 现象：**触发事件（点击 / 右键菜单 / 鼠标释放）后，画面就不再更新了**——表现为界面在每次
> 交互后"卡住"百余毫秒，连续操作时几乎冻结。
>
> 结论：宿主「派发事件后同步驱动事件循环直到排空」的循环，用 `EventLoop.PendingTasks() == 0`
> 当退出条件；而 `PendingTasks()` **把尚未到期的定时器也算作 pending**。页面上几乎总有一个
> 未到期定时器（Vue 的 `nextTick` 兜底 `setTimeout`、框架轮询/心跳、CM6/xterm 的自调度），
> 判定因此**恒为 false**，循环只能靠超时退出——每次事件白等满 120~180ms，这段时间里主循环
> **既不布局也不绘制**。
>
> 修复：`engine/js/jsc/eventloop.go` 新增 `EventLoop.HasReadyTasks()`（只算**可立即执行**的任务：
> 到期宏任务 / 微任务 / rAF / setImmediate）；`app/host.go` 抽出 `Host.drainEventLoop(timeout)`
> 并用它替换 4 处判定；`engine/js/worker/worker.go` 的同源判定同步修正。

## 1. 现象与定位口

宿主侧（`app/host.go`）在派发事件后要**同步**驱动事件循环，让事件处理器里的 async 链在本帧
内落地（`@click → await fetch → Vue 更新`）。历史上这是 4 段各自内联的循环：

| 位置（修复前行号） | 触发时机 | 超时 |
|---|---|---|
| `app/host.go:4409` | 鼠标**释放**（编辑器光标同步：CM6 的 `requestMeasure → rAF` 链） | 180ms |
| `app/host.go:4977` | **contextmenu**（Vue 右键菜单 visible=true 的响应式更新） | 180ms |
| `app/host.go:5139` | **click**（目标带 `onclick` 属性分支） | 180ms |
| `app/host.go:5207` | **click**（DOM 事件分支） | 120ms |

四段结构完全一致（局部变量名略异）：

```go
deadline := time.Now().Add(180 * time.Millisecond)
for {
    interp.RunJobs()
    h.processEventLoop()                 // → EventLoop.ProcessTasks
    el := interp.GetEventLoop()
    if el == nil || el.PendingTasks() == 0 {   // ← 退出条件：这里错了
        break
    }
    if time.Now().After(deadline) { break }
    time.Sleep(5 * time.Millisecond)
}
```

## 2. 根因：`PendingTasks` 的语义包含**未到期**定时器

`engine/js/jsc/eventloop.go`：

```go
func (el *EventLoop) PendingTasks() int {
    return len(el.macrotasks) + len(el.microtasks) + len(el.animFrames) + len(el.immediates)
}
```

`SetTimeout`/`SetInterval` 把任务直接 `insertMacrotask`（**按 DueTime 排队等待**），只有
`popNextMacrotask(now)` 在**到期**时才弹出；也就是说 `macrotasks` 里长期躺着未到期的任务，
`PendingTasks()` 把它们全算成 pending。

于是：页面只要注册过任何一个**未到期**的定时器（`setTimeout(fn, 30000)`、`setInterval` 轮询…），
`PendingTasks()` 就恒 > 0 ⇒ 上述循环永远走不到 `break`，只能靠 `deadline` 退出 ⇒
**每次都白等满 120~180ms**。

这段时间里宿主主循环停在这个循环内，帧循环的 `EnsureLayout()` / `Paint()` 都不会执行
（在 `app.Host.Run` 的帧序里，这些 drain 发生在帧首的事件处理阶段，事件处理不返回 ⇒
本帧永远走不到绘制）——用户看到的就是「一触发事件，画面就停止更新」。

> 为什么历史上的注释会写"普通点击无异步链，PendingTasks 立即排空"？
> 那句话描述的是**理想语义**（"没有待执行的活儿就退出"）。它恰好说明作者想要的是
> **就绪**语义，而 `PendingTasks()` 给的是**排队总量**语义 —— 二者在"存在未到期定时器"时
> 分道扬镳，这正是缺陷所在。

## 3. 修复

### 3.1 引擎侧：新增就绪判定

```go
// engine/js/jsc/eventloop.go
func (el *EventLoop) HasReadyTasks() bool {
    if el == nil { return false }
    el.mu.Lock(); defer el.mu.Unlock()
    if len(el.microtasks) > 0 || len(el.animFrames) > 0 || len(el.immediates) > 0 {
        return true
    }
    if len(el.macrotasks) == 0 { return false }
    return el.macrotasks[0].IsDue(el.nowMs())   // 队列按 DueTime 升序，只看队首
}
```

### 3.2 宿主侧：统一为一个 helper

```go
// app/host.go
func (h *Host) drainEventLoop(timeout time.Duration) {
    interp := h.wv.JSInterpreter()
    if interp == nil { return }
    el := interp.GetEventLoop()
    if el == nil { return }
    deadline := time.Now().Add(timeout)
    for {
        interp.RunJobs()
        h.processEventLoop()
        if !el.HasReadyTasks() { return }          // ← 就绪判据（关键）
        if time.Now().After(deadline) { return }   // 上限保护：自调度链
        time.Sleep(2 * time.Millisecond)
    }
}
```

四处调用点分别保留原有超时（180/180/180/120ms）与位置，sleep 由 5ms 收到 2ms（只在该推进
时才让出，链推进更快）。

### 3.3 语义边界（为什么不破坏原有意图）

- **rAF 链**（CM6 的 measure 链：rAF 回调里再注册 rAF）：每轮 `ProcessTasks` 都会
  `flushAnimationFrames`，回调里新注册的 rAF 又让 `HasReadyTasks()==true` ⇒ 循环继续推进，
  直到链结束或超时——**原「同步推进到排空」的意图完整保留**。
- **微任务/Promise**：`RunJobs()` 每次清空 goja job 队列，新产生的微任务同样让循环继续。
- **短定时器**（`setTimeout(fn, 0)`、几 ms）：到期即计入就绪，照旧在本帧内推进。
- **未到期的远定时器**：不再阻塞本帧——这与浏览器一致（定时器不阻塞当前帧），宿主的帧循环
  每帧都会 `ProcessTasks`，定时器到点照常执行。
- **自调度链**（rAF 递归 / `setTimeout(0)` 循环 / 长 async 链）：仍由 `timeout` 兜底。

### 3.4 同源修正

`engine/js/worker/worker.go` 的定时器驱动（每 4ms ticker）原本用 `PendingTasks() > 0` 决定
是否 `ProcessTasks` —— 同样会因未到期定时器而**每 4ms 空转一圈**。改为 `HasReadyTasks()`。

> 未改动：`dev/suites/cssprobe/main.go:513` 的同类写法（`PendingTasks() > 0` 作循环条件）属
> dev 套件工具，且有 `eventLoopTurns` 上限保护，不影响产品行为，本轮不动以保持套件基线。

## 4. 验证

| 检查 | 结果 |
|---|---|
| `go test ./engine/js/jsc/ -run "HasReadyTasks\|DrainReady" -v` | **PASS**（4 个用例，含子用例） |
| `go test ./app/ -run "Drain\|EventDispatchKeepsDirty" -v` | **PASS**（2 个用例） |
| `go test ./app/... ./webkit/... -count=1` | **ok**（app 8.6s / webkit 3.1s，无回归） |
| `go test ./engine/... -count=1` | **25 包 ok** |

测试要点（两个层面）：

1. **jsc 层语义**（`engine/js/jsc/eventloop_ready_test.go`）
   - `TestHasReadyTasksIgnoresFutureTimers`：未到期的 `setTimeout` ⇒ `HasReadyTasks()==false`
     但 `PendingTasks()>0` —— 一条用例同时锁住「新判据不阻塞」与「旧判据必然等待」的机制。
   - `TestHasReadyTasksImmediateKinds`：`queueMicrotask` / `requestAnimationFrame` /
     `setImmediate` / `setTimeout(0)` 入队即就绪。
   - `TestDrainReadySemanticsKeepsShortChainAndSkipsFarTimer`：同一条链上**对照**两种判据——
     旧判据（`PendingTasks`）必然等满超时，新判据（`HasReadyTasks`）立即返回，
     且未到期的远定时器**没有**被执行。
2. **宿主层行为**（`app/eventloop_drain_test.go`，用 `NewHostForTest` + 真实 WebView + 真实页面）
   - `TestDrainEventLoopDoesNotBlockOnFutureTimers`：页面带 30s 未到期 `setTimeout` +
     `setInterval` ⇒ `h.drainEventLoop(180ms)` 必须 **< 60ms** 返回，且远定时器未执行。
   - `TestEventDispatchKeepsDirtyAfterDrain`：回归的另一半——**真实鼠标路径**（`HandleMouseMove` +
     按下/释放）派发 click，处理器改背景色；drain 之后 `RenderView.IsDirty()` 必须仍为 true，
     且随后一次 `Render()` 必须真的画出 `#ff0000`（既证明事件没被 drain 吞掉，也证明事件后的
     绘制没有停止）。

## 5. 口径提醒（写宿主驱动循环时）

- **「有没有活儿可干」用 `HasReadyTasks()`**；`PendingTasks()` 只适合"队列里还有多少东西"
  这类统计/诊断用途，**不要**拿它当循环退出条件。
- 宿主的帧循环每帧都应无条件调用 `ProcessTasks`（本仓库各处宿主都是这么做的），
  未到期定时器由时间推进——不需要、也不应该在事件后同步等它们。
- 事件后同步 drain 想达到的目的（"本帧内把能推进的推进完"）与"等未来定时器"是两件事，
  后者必然表现为**画面冻结**。
