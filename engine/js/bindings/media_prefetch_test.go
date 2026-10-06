package bindings

// A2：播放推进中的预取契约（绑定层只负责「告诉宿主下一步显示哪一帧」，取帧与
// 解码全在宿主）。

import (
	"math"
	"sort"
	"sync"
	"testing"
	"time"

	"wb-ui/engine/rendering"
)

// prefetchCall 是一次预取请求（宿主异步帧源收到的参数）。
type prefetchCall struct {
	url string
	at  float64
}

// prefetchLog 记录预取请求：预取从渲染层起的 goroutine 上转交宿主，必须带锁。
type prefetchLog struct {
	mu    sync.Mutex
	calls []prefetchCall
}

func (l *prefetchLog) snapshot() []prefetchCall {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]prefetchCall, len(l.calls))
	copy(out, l.calls)
	return out
}

// usePrefetchRecorder 装一个「只记录、不交付」的异步帧源。
func usePrefetchRecorder(t *testing.T) *prefetchLog {
	t.Helper()
	l := &prefetchLog{}
	rendering.SetVideoAsyncFrameSource(func(url string, at float64, _ func([]byte, bool)) {
		l.mu.Lock()
		l.calls = append(l.calls, prefetchCall{url: url, at: at})
		l.mu.Unlock()
	})
	t.Cleanup(func() { rendering.SetVideoAsyncFrameSource(nil) })
	return l
}

// TestVideoStatePlayingFlagFollowsClock：Playing 只描述「时钟是否在走」——它是
// painter 决定「同步等帧」还是「异步 + 回退上一帧」的唯一依据，必须在 play()
// 返回时就已经为真（否则 play 后的头一个 tick 之前还要白等一次解码）。
func TestVideoStatePlayingFlagFollowsClock(t *testing.T) {
	useVideoFrameSource(t)
	rt, doc, _ := newRuntimeWithDoc(t)
	video := newVideoFixture(doc)
	MediaMetadataResolver = func(src string) (MediaMetadata, bool) {
		return MediaMetadata{Duration: 1, Width: 120, Height: 80}, src == "probe.mp4"
	}
	defer func() { MediaMetadataResolver = nil }()

	mustRun(t, rt, `document.getElementById("player").src = "probe.mp4";`)
	drainEventLoop(rt)
	if st, ok := rendering.ElementVideoState(video); !ok || st.Playing {
		t.Fatalf("加载完成（还没播放）时 Playing 应为 false，got (%+v, %v)", st, ok)
	}

	mustRun(t, rt, `document.getElementById("player").play();`)
	if st, ok := rendering.ElementVideoState(video); !ok || !st.Playing {
		t.Fatalf("play() 后 Playing 应为 true，got (%+v, %v)", st, ok)
	}

	mustRun(t, rt, `document.getElementById("player").pause();`)
	if st, ok := rendering.ElementVideoState(video); !ok || st.Playing {
		t.Fatalf("pause() 后 Playing 应为 false，got (%+v, %v)", st, ok)
	}
}

// TestPrefetchesNextFrameOnTick：每推进一个时钟步长就预取「下一次会显示的时刻」
// ——预取让 painter 到达时已命中缓存（判据 A2-4：播放中同步取帧 = 0 次）。
func TestPrefetchesNextFrameOnTick(t *testing.T) {
	useVideoFrameSource(t)
	log := usePrefetchRecorder(t)
	rt, doc, _ := newRuntimeWithDoc(t)
	video := newVideoFixture(doc)
	st0 := mediaStateFor(rt, video)
	st0.loadedSrc = "probe.mp4"
	mustRun(t, rt, `document.getElementById("player").setAttribute("src", "probe.mp4");`)
	st0.readyState = mediaHaveEnoughData
	st0.duration = 1
	st0.paused = false
	st0.clock = true

	st0.tick() // currentTime: 0 → 0.25，预取 0.5
	waitPrefetch(t, log, 1)
	got := log.snapshot()[0]
	if got.url != "probe.mp4" || math.Abs(got.at-0.5) > 1e-9 {
		t.Fatalf("第一个 tick 的预取请求 = %+v，want {probe.mp4 0.5}", got)
	}
}

// TestPrefetchClampsToDuration：末尾把预取时刻收敛到 duration（越界时刻没有帧，
// 宿主会把 duration 映射到最后一帧）——这样播放结束的那一帧也是提前备好的。
func TestPrefetchClampsToDuration(t *testing.T) {
	useVideoFrameSource(t)
	log := usePrefetchRecorder(t)
	rt, doc, _ := newRuntimeWithDoc(t)
	video := newVideoFixture(doc)
	st0 := mediaStateFor(rt, video)
	st0.loadedSrc = "probe.mp4"
	mustRun(t, rt, `document.getElementById("player").setAttribute("src", "probe.mp4");`)
	st0.readyState = mediaHaveEnoughData
	st0.duration = 1
	st0.currentTime = 0.75
	st0.paused = false
	st0.clock = true

	st0.tick() // 0.75 → 1.0 ≥ duration：走结束分支，不预取
	if len(log.snapshot()) != 0 {
		t.Fatalf("结束分支不应预取（没有下一帧了），got %+v", log.snapshot())
	}

	// 0.5 → 0.75：下一次是 1.0 > duration，收敛到 duration（=1.0，其实就是它）。
	st0.ended = false
	st0.paused = false
	st0.clock = true
	st0.currentTime = 0.5
	st0.tick()
	waitPrefetch(t, log, 1)
	if got := log.snapshot()[0]; got.at != 1 {
		t.Fatalf("末尾预取应收敛到 duration=1，got %+v", got)
	}
}

// TestNoPrefetchForAudioElement：<audio> 没有画面，不参与帧通道（也不预取）。
func TestNoPrefetchForAudioElement(t *testing.T) {
	useVideoFrameSource(t)
	log := usePrefetchRecorder(t)
	rt, doc, _ := newRuntimeWithDoc(t)
	audio := doc.CreateElement("audio")
	audio.SetId("sound")
	doc.AppendChild(audio)
	st := mediaStateFor(rt, audio)
	st.loadedSrc = "sound.mp3"
	mustRun(t, rt, `document.getElementById("sound").setAttribute("src", "sound.mp3");`)
	st.readyState = mediaHaveEnoughData
	st.duration = 1
	st.paused = false
	st.clock = true

	st.tick()
	st.tick()
	if calls := log.snapshot(); len(calls) != 0 {
		t.Fatalf("audio 不应预取帧，got %+v", calls)
	}
}

// waitPrefetch 等预取请求到达记录器（预取经渲染层的 goroutine 转交）。
func waitPrefetch(t *testing.T, log *prefetchLog, want int) {
	t.Helper()
	for i := 0; i < 2000; i++ {
		if len(log.snapshot()) >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("等待预取请求超时（want ≥ %d，got %+v）", want, log.snapshot())
}

// TestPrefetchWindowFollowsFrameRate：知道帧率时按**帧**预取一个窗口（A2-② 的
// 「更长的预取窗口」）：10fps 下预取接下来 mediaPrefetchFrames 帧（+1/fps、
// +2/fps、+3/fps），而不是只备下一个时钟时刻——播放中帧到达的总有一帧半到三帧
// 的余量，抖动不会让绘制落空。
func TestPrefetchWindowFollowsFrameRate(t *testing.T) {
	useVideoFrameSource(t)
	log := usePrefetchRecorder(t)
	rt, doc, _ := newRuntimeWithDoc(t)
	video := newVideoFixture(doc)
	st0 := mediaStateFor(rt, video)
	st0.loadedSrc = "probe.mp4"
	mustRun(t, rt, `document.getElementById("player").setAttribute("src", "probe.mp4");`)
	st0.readyState = mediaHaveEnoughData
	st0.duration = 1
	st0.fps = 10
	st0.paused = false
	st0.clock = true

	st0.tick() // 0 → 0.25：预取 0.35 / 0.45 / 0.55
	waitPrefetch(t, log, mediaPrefetchFrames)
	got := log.snapshot()
	if len(got) != mediaPrefetchFrames {
		t.Fatalf("预取帧数 = %d，want %d（%+v）", len(got), mediaPrefetchFrames, got)
	}
	// ★ 三个请求各自经渲染层的 goroutine 转交，**到达顺序不定**：按值排序后再比，
	// 否则测的是调度时序（首版实现就栽在这里：0.55 先到被判成「第 1 个」）。
	ats := make([]float64, 0, len(got))
	for _, c := range got {
		if c.url != "probe.mp4" {
			t.Errorf("预取 src = %q，want probe.mp4", c.url)
		}
		ats = append(ats, c.at)
	}
	sort.Float64s(ats)
	for i, at := range ats {
		want := st0.currentTime + float64(i+1)/st0.fps
		if math.Abs(at-want) > 1e-9 {
			t.Errorf("第 %d 个预取时刻 = %v，want %v", i+1, at, want)
		}
	}
	// 每秒播放 10 帧、每次 tick 预取 3 帧：窗口覆盖 300ms，比时钟步长（250ms）略宽
	// ——这正是「更长的预取窗口」的含义（帧率的函数，不是拍脑袋的常数）。
	if span := ats[len(ats)-1] - ats[0]; math.Abs(span-2.0/10) > 1e-9 {
		t.Errorf("预取窗口跨度 = %v，want %v", span, 2.0/10)
	}
}
