package rendering

// A2：异步帧源与预取（播放推进中渲染线程不因解码阻塞）。
//
// 测试用「手工控制的异步源」把交付时机握在手里：deliver 回调交到 channel 上，
// 由测试决定什么时候交付——这样能分开验证「未交付时怎么画」与「交付后怎么画」。

import (
	"math"
	"sync"
	"testing"
	"time"
)

// asyncHarness 是一套「同步源 + 手工异步源」的帧通道测试夹具。
//
// 计数带锁：异步源跑在渲染层起的 goroutine 上（`go async(...)`），而断言在测试
// goroutine 里读——裸 int 会被竞态检测器抓到（那记的是测试的账，不是实现的）。
type asyncHarness struct {
	mu         sync.Mutex
	syncCalls  int
	asyncCalls int
	delivers   chan func([]byte, bool)
}

// calls 返回 (同步取帧次数, 异步请求次数) 的快照。
func (h *asyncHarness) calls() (int, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.syncCalls, h.asyncCalls
}

// newAsyncHarness 安装夹具并把帧通道复位到干净状态（SetVideo*FrameSource 会
// 清缓存与统计）。
func newAsyncHarness(t *testing.T) *asyncHarness {
	t.Helper()
	h := &asyncHarness{delivers: make(chan func([]byte, bool), 8)}
	SetVideoFrameSource(func(url string, at float64) ([]byte, bool) {
		h.mu.Lock()
		h.syncCalls++
		h.mu.Unlock()
		return solidPNG(t, greenFrame, 40, 40), true
	})
	SetVideoAsyncFrameSource(func(url string, at float64, deliver func([]byte, bool)) {
		h.mu.Lock()
		h.asyncCalls++
		h.mu.Unlock()
		h.delivers <- deliver
	})
	t.Cleanup(func() {
		SetVideoFrameSource(nil)
		SetVideoAsyncFrameSource(nil)
	})
	return h
}

// takeDeliver 取出一次异步请求的交付回调（异步源每次调用都会放一个进来）。
func (h *asyncHarness) takeDeliver(t *testing.T) func([]byte, bool) {
	t.Helper()
	select {
	case d := <-h.delivers:
		return d
	case <-time.After(2 * time.Second):
		t.Fatal("异步源未被调用（没有待交付的请求）")
		return nil
	}
}

// waitFor 轮询等待条件成立（异步交付发生在别的 goroutine 上）。
func waitFor(t *testing.T, what string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("等待超时：%s", what)
}

// TestPlayingFrameGoesAsyncWithoutBlocking：播放推进中未命中的帧不得同步取
// （同步源一次都不能被调用），而是提交异步请求并立即返回（首帧还没有 → nil）。
func TestPlayingFrameGoesAsyncWithoutBlocking(t *testing.T) {
	h := newAsyncHarness(t)
	el := newVideoElement(t, nil)
	SetElementVideoState(el, VideoElementState{URL: "clip.mp4", Time: 0.5, Playing: true})

	if img := videoFrameForElement(el); img != nil {
		t.Fatalf("目标帧未交付时应返回 nil（无上一帧可顶），得到 %v", img)
	}
	if syncCalls, _ := h.calls(); syncCalls != 0 {
		t.Fatalf("播放中不得走同步路径，syncCalls=%d", syncCalls)
	}
	if got := VideoFrameStatsSnapshot(); got.AsyncRequests != 1 || got.StaleFrames != 1 {
		t.Fatalf("统计应为 AsyncRequests=1 StaleFrames=1，得到 %+v", got)
	}

	h.takeDeliver(t)(solidPNG(t, blueFrame, 40, 40), true)
	waitFor(t, "异步帧入缓存", func() bool { return VideoFrameStatsSnapshot().AsyncFrames == 1 })

	img := videoFrameForElement(el)
	if img == nil || !img.Loaded() {
		t.Fatalf("交付后绘制应命中缓存，得到 %v", img)
	}
	if syncCalls, _ := h.calls(); syncCalls != 0 {
		t.Fatalf("交付后仍不得走同步路径，syncCalls=%d", syncCalls)
	}
}

// TestPlayingFallsBackToLastShownFrame：目标帧未交付时用**上一次成功显示**的帧
// 顶上（显示旧一帧好过空窗或阻塞渲染线程）。
func TestPlayingFallsBackToLastShownFrame(t *testing.T) {
	h := newAsyncHarness(t)
	el := newVideoElement(t, nil)
	SetElementVideoState(el, VideoElementState{URL: "clip.mp4", Time: 0}) // 静止态：同步取
	first := videoFrameForElement(el)
	if first == nil || !first.Loaded() {
		t.Fatalf("静止态首帧应同步取到，得到 %v", first)
	}
	if syncCalls, _ := h.calls(); syncCalls != 1 {
		t.Fatalf("静止态应同步取一次，syncCalls=%d", syncCalls)
	}

	SetElementVideoState(el, VideoElementState{URL: "clip.mp4", Time: 0.5, Playing: true})
	got := videoFrameForElement(el)
	if got != first {
		t.Fatalf("未交付时应回退到上一帧（同一张），得到 %v 想要 %v", got, first)
	}
	if syncCalls, _ := h.calls(); syncCalls != 1 {
		t.Fatalf("回退不得触发同步取帧，syncCalls=%d", syncCalls)
	}
}

// TestPrefetchDedupesAndFeedsPainter：预取同一时刻只提交一次；交付后 painter
// 直接命中——播放中「同步取帧 = 0」正是靠这条路径。
func TestPrefetchDedupesAndFeedsPainter(t *testing.T) {
	h := newAsyncHarness(t)
	PrefetchVideoFrame("clip.mp4", 1.0)
	PrefetchVideoFrame("clip.mp4", 1.0) // 同一键：去重
	// 预取提交发生在渲染层起的 goroutine 上：等它真的调用宿主。
	waitFor(t, "预取请求提交", func() bool { _, asyncCalls := h.calls(); return asyncCalls == 1 })
	if _, asyncCalls := h.calls(); asyncCalls != 1 {
		t.Fatalf("同一时刻的预取应去重，asyncCalls=%d", asyncCalls)
	}
	if got := VideoFrameStatsSnapshot(); got.PrefetchRequests != 1 {
		t.Fatalf("PrefetchRequests 应为 1，得到 %+v", got)
	}

	h.takeDeliver(t)(solidPNG(t, redFrame, 40, 40), true)
	waitFor(t, "预取帧入缓存", func() bool { return VideoFrameStatsSnapshot().AsyncFrames == 1 })

	el := newVideoElement(t, nil)
	SetElementVideoState(el, VideoElementState{URL: "clip.mp4", Time: 1.0, Playing: true})
	img := videoFrameForElement(el)
	if img == nil || !img.Loaded() {
		t.Fatalf("预取已覆盖的时刻应命中缓存（异步请求不增加），得到 %v", img)
	}
	if syncCalls, _ := h.calls(); syncCalls != 0 {
		t.Fatalf("命中缓存不该产生同步取帧，syncCalls=%d", syncCalls)
	}
	if got := VideoFrameStatsSnapshot(); got.AsyncRequests != 0 || got.StaleFrames != 0 {
		t.Fatalf("命中缓存不该产生取帧行为，统计 %+v", got)
	}
}

// TestPrefetchSkipsCachedAndNegative：已缓存（含负缓存）的时刻不重复请求解码器。
func TestPrefetchSkipsCachedAndNegative(t *testing.T) {
	h := newAsyncHarness(t)
	PrefetchVideoFrame("clip.mp4", 2.0)
	h.takeDeliver(t)(nil, false) // 取不到：记负缓存
	waitFor(t, "负缓存写入", func() bool { _, asyncCalls := h.calls(); return asyncCalls == 1 })

	PrefetchVideoFrame("clip.mp4", 2.0)
	if _, asyncCalls := h.calls(); asyncCalls != 1 {
		t.Fatalf("负缓存命中不应再问宿主要帧，asyncCalls=%d", asyncCalls)
	}
	el := newVideoElement(t, nil)
	SetElementVideoState(el, VideoElementState{URL: "clip.mp4", Time: 2.0, Playing: true})
	if img := videoFrameForElement(el); img != nil {
		t.Fatalf("负缓存命中应返回 nil，得到 %v", img)
	}
	if syncCalls, _ := h.calls(); syncCalls != 0 {
		t.Fatalf("负缓存命中也不得回退同步取帧，syncCalls=%d", syncCalls)
	}
}

// TestFallbackRejectsOtherSource：换 src 后旧帧不能顶上来（那是另一个视频的画面）。
func TestFallbackRejectsOtherSource(t *testing.T) {
	h := newAsyncHarness(t)
	el := newVideoElement(t, nil)
	SetElementVideoState(el, VideoElementState{URL: "a.mp4", Time: 0})
	if img := videoFrameForElement(el); img == nil {
		t.Fatal("a.mp4 首帧应同步取到")
	}
	SetElementVideoState(el, VideoElementState{URL: "b.mp4", Time: 0.5, Playing: true})
	if img := videoFrameForElement(el); img != nil {
		t.Fatalf("换源后不得用旧源的帧顶上，得到 %v", img)
	}
	if syncCalls, _ := h.calls(); syncCalls != 1 {
		t.Fatalf("换源回退不得触发同步取帧，syncCalls=%d", syncCalls)
	}
}

// TestAsyncOnlySourceLeavesStillFramesUnpainted：只注册异步源、静止态求帧时返回
// nil（不阻塞渲染线程）——静止态要求宿主提供同步源，这是刻意的分工。
func TestAsyncOnlySourceLeavesStillFramesUnpainted(t *testing.T) {
	h := newAsyncHarness(t)
	SetVideoFrameSource(nil) // 只留异步源
	el := newVideoElement(t, nil)
	SetElementVideoState(el, VideoElementState{URL: "clip.mp4", Time: 0})
	if img := videoFrameForElement(el); img != nil {
		t.Fatalf("没有同步源时静止态应返回 nil，得到 %v", img)
	}
	if _, asyncCalls := h.calls(); asyncCalls != 0 {
		t.Fatalf("静止态不得走异步源，asyncCalls=%d", asyncCalls)
	}
}

// TestTimeQuantizerFoldsSameFrameToOneRequest：宿主注册的时刻量化器把「同一帧的
// 不同时刻」折叠成一个键（A2-①）。播放中 currentTime 连续变化（0.51→0.55→0.59
// 都是 10fps 的第 5 帧），但解码只该发生一次：请求去重按帧、缓存也按帧共享。
func TestTimeQuantizerFoldsSameFrameToOneRequest(t *testing.T) {
	h := newAsyncHarness(t)
	// 与宿主 app/mediaprobe.go 的 alignFrameTime 同一规则（10fps）。
	SetVideoFrameTimeQuantizer(func(_ string, at float64) float64 {
		k := math.Floor(at*10 + 1e-6)
		if k < 0.5 {
			return 0
		}
		return (k - 0.5) / 10
	})
	t.Cleanup(func() { SetVideoFrameTimeQuantizer(nil) })

	el := newVideoElement(t, nil)
	SetElementVideoState(el, VideoElementState{URL: "clip.mp4", Time: 0.51, Playing: true})
	if img := videoFrameForElement(el); img != nil {
		t.Fatalf("首帧未交付时应返回 nil，得到 %v", img)
	}
	// 同一帧内的另外两个时刻：不得再提交请求（同键 + 已在飞）。
	SetElementVideoState(el, VideoElementState{URL: "clip.mp4", Time: 0.55, Playing: true})
	videoFrameForElement(el)
	SetElementVideoState(el, VideoElementState{URL: "clip.mp4", Time: 0.59, Playing: true})
	videoFrameForElement(el)
	// 异步源跑在渲染层起的 goroutine 上：断言前必须等它跑起来（同样的等待在下面
	// 交付后也要用，否则测的是调度时序而不是去重逻辑）。
	waitFor(t, "第 5 帧的异步请求", func() bool {
		_, asyncCalls := h.calls()
		return asyncCalls >= 1
	})
	if _, asyncCalls := h.calls(); asyncCalls != 1 {
		t.Fatalf("同一帧的三个时刻应只提交一次异步请求，asyncCalls=%d", asyncCalls)
	}

	h.takeDeliver(t)(solidPNG(t, blueFrame, 40, 40), true)
	waitFor(t, "折叠后的键入缓存", func() bool { return VideoFrameStatsSnapshot().AsyncFrames == 1 })

	// 交付后：同帧的任意时刻都命中这条缓存，且不走同步路径。
	if img := videoFrameForElement(el); img == nil || !img.Loaded() {
		t.Fatalf("同帧时刻应命中折叠后的缓存，得到 %v", img)
	}
	if syncCalls, _ := h.calls(); syncCalls != 0 {
		t.Fatalf("命中折叠缓存时不得走同步路径，syncCalls=%d", syncCalls)
	}

	// 跨到下一帧（0.61 → 第 6 帧）：折叠后是另一个键 → 提交新请求。
	SetElementVideoState(el, VideoElementState{URL: "clip.mp4", Time: 0.61, Playing: true})
	_ = videoFrameForElement(el)
	waitFor(t, "下一帧提交新请求", func() bool {
		_, asyncCalls := h.calls()
		return asyncCalls == 2
	})
}

// TestPrefetchUsesQuantizedKey：预取与绘制共用同一套折叠规则——预取第 5 帧的取样点
// 之后，绘制同帧的**另一个**时刻直接命中，不必再等一次解码（A2-②：更长的预取窗口
// 之所以有效，前提就是「预取的键 == 绘制的键」）。
func TestPrefetchUsesQuantizedKey(t *testing.T) {
	h := newAsyncHarness(t)
	SetVideoFrameTimeQuantizer(func(_ string, at float64) float64 {
		k := math.Floor(at*10 + 1e-6)
		if k < 0.5 {
			return 0
		}
		return (k - 0.5) / 10
	})
	t.Cleanup(func() { SetVideoFrameTimeQuantizer(nil) })

	PrefetchVideoFrame("clip.mp4", 0.55) // 宿主实际抽的是 0.45（第 5 帧取样点）
	waitFor(t, "预取提交异步请求", func() bool {
		_, asyncCalls := h.calls()
		return asyncCalls >= 1
	})
	if _, asyncCalls := h.calls(); asyncCalls != 1 {
		t.Fatalf("预取应提交一次异步请求，asyncCalls=%d", asyncCalls)
	}
	h.takeDeliver(t)(solidPNG(t, blueFrame, 40, 40), true)
	waitFor(t, "预取帧入缓存", func() bool { return VideoFrameStatsSnapshot().AsyncFrames == 1 })

	el := newVideoElement(t, nil)
	SetElementVideoState(el, VideoElementState{URL: "clip.mp4", Time: 0.59, Playing: true})
	img := videoFrameForElement(el)
	if img == nil || !img.Loaded() {
		t.Fatalf("绘制同帧的另一时刻应命中预取的帧，得到 %v", img)
	}
	if syncCalls, asyncCalls := h.calls(); syncCalls != 0 || asyncCalls != 1 {
		t.Fatalf("命中预取缓存后不该再有请求，syncCalls=%d asyncCalls=%d", syncCalls, asyncCalls)
	}
}

// TestVideoFrameReadyNotifiesOnDelivery：异步帧交付成功后通知监听器（A2-③）——宿主
// （webkit.WebView）靠它把「帧到位」变成一次置脏重绘，绑定层的
// requestVideoFrameCallback 也由此拿到呈现时机。
//
// 两条边界一起锁定：**失败交付不通知**（没有新帧可画，通知只会让宿主白重绘一次）、
// **注销函数幂等**（多次调用不该 panic，也不该把别人的监听器摘掉）。
func TestVideoFrameReadyNotifiesOnDelivery(t *testing.T) {
	h := newAsyncHarness(t)
	var mu sync.Mutex
	var got []float64
	off := AddVideoFrameReadyListener(func(_ string, at float64) {
		mu.Lock()
		got = append(got, at)
		mu.Unlock()
	})
	defer off()
	defer off() // 幂等：第二次调用必须是 no-op

	PrefetchVideoFrame("clip.mp4", 1.0)
	waitFor(t, "预取提交异步请求", func() bool {
		_, asyncCalls := h.calls()
		return asyncCalls == 1
	})
	h.takeDeliver(t)(solidPNG(t, blueFrame, 40, 40), true)
	waitFor(t, "交付后通知监听器", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == 1
	})
	mu.Lock()
	if len(got) != 1 || math.Abs(got[0]-1.0) > 1e-9 {
		t.Errorf("通知的时刻 = %v，want [1]", got)
	}
	mu.Unlock()

	// 失败交付（抽帧失败 / 队列满被丢）不通知。
	PrefetchVideoFrame("clip.mp4", 2.0)
	waitFor(t, "第二个预取请求", func() bool {
		_, asyncCalls := h.calls()
		return asyncCalls == 2
	})
	h.takeDeliver(t)(nil, false)
	waitFor(t, "失败交付写入负缓存", func() bool {
		return VideoFrameStatsSnapshot().AsyncFrames == 1
	})
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Errorf("失败交付不应通知（没有新帧可画），got %v", got)
	}
}
