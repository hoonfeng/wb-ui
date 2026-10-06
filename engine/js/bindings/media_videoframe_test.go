package bindings

import (
	"math"
	"testing"

	"wb-ui/engine/rendering"
)

// 主线 A1（宿主注入帧流）的绑定层契约：媒体元素只负责把「当前显示什么」（源、
// 时间点、show poster flag）告知渲染层，取帧与绘制由宿主 + painter 完成。
// 这里断言这条通道的时序：元数据就绪给首帧、seek 改时间点、播放时钟推进、
// play() 清 poster flag、切源/出错清状态。

// useVideoFrameSource 装一个空帧源（只为让渲染层接受帧请求；本组测试断言的是
// 状态通道，不关心取帧结果）。
func useVideoFrameSource(t *testing.T) {
	t.Helper()
	rendering.SetVideoFrameSource(func(string, float64) ([]byte, bool) { return nil, false })
	t.Cleanup(func() { rendering.SetVideoFrameSource(nil) })
}

// TestMediaElementRequestsFrameOnMetadata：资源加载成功（HAVE_METADATA 或
// HAVE_ENOUGH_DATA）后，元素应声明 currentTime=0 的帧状态——否则 <video> 明明
// 加载好了却永远是空白（poster 之外什么都不画）。此时 show poster flag 仍置位
// （还没播放），渲染层显示 poster。
func TestMediaElementRequestsFrameOnMetadata(t *testing.T) {
	useVideoFrameSource(t)
	rt, doc, _ := newRuntimeWithDoc(t)
	video := newVideoFixture(doc)
	MediaMetadataResolver = func(src string) (MediaMetadata, bool) {
		if src == "probe.mp4" {
			return MediaMetadata{Duration: 4, Width: 120, Height: 80}, true
		}
		return MediaMetadata{}, false
	}
	defer func() { MediaMetadataResolver = nil }()

	// 未加载：没有状态记录（避免对空 src 追问宿主）。
	if _, ok := rendering.ElementVideoState(video); ok {
		t.Fatalf("未加载时不应有视频状态记录")
	}
	mustRun(t, rt, `document.getElementById("player").src = "probe.mp4";`)
	drainEventLoop(rt)

	st, ok := rendering.ElementVideoState(video)
	if !ok || st.URL != "probe.mp4" || st.Time != 0 || !st.ShowPoster {
		t.Fatalf("视频状态 = (%+v, %v)，want {probe.mp4 0 true}", st, ok)
	}
}

// TestMediaElementClearsPosterFlagOnPlay：play() 清除 show poster flag（HTML
// §4.8.8）——否则播放起来也一直显示 poster、永远看不到画面。
func TestMediaElementClearsPosterFlagOnPlay(t *testing.T) {
	useVideoFrameSource(t)
	rt, doc, _ := newRuntimeWithDoc(t)
	video := newVideoFixture(doc)
	MediaMetadataResolver = func(src string) (MediaMetadata, bool) {
		return MediaMetadata{Duration: 4, Width: 120, Height: 80}, src == "probe.mp4"
	}
	defer func() { MediaMetadataResolver = nil }()

	mustRun(t, rt, `
		{
		const v = document.getElementById("player");
		v.setAttribute("poster", "poster.png");
		v.src = "probe.mp4";
		}
	`)
	drainEventLoop(rt)
	if st, _ := rendering.ElementVideoState(video); !st.ShowPoster {
		t.Fatalf("play() 之前 show poster flag 应为 true，got %+v", st)
	}

	mustRun(t, rt, `document.getElementById("player").play();`)
	st, ok := rendering.ElementVideoState(video)
	if !ok || st.URL != "probe.mp4" || st.ShowPoster {
		t.Fatalf("play() 后状态 = (%+v, %v)，want 同一源且 ShowPoster=false", st, ok)
	}
}

// TestMediaElementRequestsFrameOnSeek：currentTime 赋值后请求该时刻的帧，并清除
// show poster flag（seek 到中间就该看到画面）。
func TestMediaElementRequestsFrameOnSeek(t *testing.T) {
	useVideoFrameSource(t)
	rt, doc, _ := newRuntimeWithDoc(t)
	video := newVideoFixture(doc)
	MediaMetadataResolver = func(src string) (MediaMetadata, bool) {
		return MediaMetadata{Duration: 4, Width: 120, Height: 80}, src == "probe.mp4"
	}
	defer func() { MediaMetadataResolver = nil }()

	mustRun(t, rt, `document.getElementById("player").src = "probe.mp4";`)
	drainEventLoop(rt)

	mustRun(t, rt, `document.getElementById("player").currentTime = 2.5;`)
	st, ok := rendering.ElementVideoState(video)
	if !ok || st.URL != "probe.mp4" || math.Abs(st.Time-2.5) > 1e-9 || st.ShowPoster {
		t.Fatalf("seek 后状态 = (%+v, %v)，want {probe.mp4 2.5 false}", st, ok)
	}
}

// TestMediaElementRequestsFrameOnClockTick：播放时钟推进时同步推进帧请求
// （时间点跟着 currentTime 走，painter 下次绘制就是新帧）。
func TestMediaElementRequestsFrameOnClockTick(t *testing.T) {
	useVideoFrameSource(t)
	rt, doc, _ := newRuntimeWithDoc(t)
	video := newVideoFixture(doc)
	st0 := mediaStateFor(rt, video)
	st0.loadedSrc = "probe.mp4"
	mustRun(t, rt, `document.getElementById("player").setAttribute("src", "probe.mp4");`)
	// 时钟只在资源就绪后推进（syncVideoState 也要求 readyState ≥ HAVE_METADATA：
	// 没有元数据就没有画面可言）。
	st0.readyState = mediaHaveEnoughData

	st0.duration = 1
	st0.paused = false
	st0.clock = true
	for i := 0; i < 3; i++ {
		st0.tick()
	}
	if math.Abs(st0.currentTime-0.75) > 1e-9 {
		t.Fatalf("三次 tick 后 currentTime = %v，want 0.75", st0.currentTime)
	}
	st, ok := rendering.ElementVideoState(video)
	if !ok || st.URL != "probe.mp4" || math.Abs(st.Time-0.75) > 1e-9 {
		t.Fatalf("时钟推进后状态 = (%+v, %v)，want {probe.mp4 0.75}", st, ok)
	}
}

// TestMediaElementClearsFrameOnSourceChange：切源先清状态（旧视频的帧不能留在新
// 资源上），新资源加载完成后重新声明。
func TestMediaElementClearsFrameOnSourceChange(t *testing.T) {
	useVideoFrameSource(t)
	rt, doc, _ := newRuntimeWithDoc(t)
	video := newVideoFixture(doc)
	MediaMetadataResolver = func(src string) (MediaMetadata, bool) {
		if src == "a.mp4" || src == "b.mp4" {
			return MediaMetadata{Duration: 3, Width: 120, Height: 80}, true
		}
		return MediaMetadata{}, false
	}
	defer func() { MediaMetadataResolver = nil }()

	mustRun(t, rt, `document.getElementById("player").src = "a.mp4";`)
	drainEventLoop(rt)
	if st, _ := rendering.ElementVideoState(video); st.URL != "a.mp4" {
		t.Fatalf("首源状态 URL = %q，want a.mp4", st.URL)
	}

	mustRun(t, rt, `document.getElementById("player").src = "b.mp4";`)
	if _, ok := rendering.ElementVideoState(video); ok {
		t.Fatalf("切源瞬间应清掉旧状态（新资源还没有画面）")
	}
	drainEventLoop(rt)
	if st, _ := rendering.ElementVideoState(video); st.URL != "b.mp4" {
		t.Fatalf("新源加载后状态 URL = %q，want b.mp4", st.URL)
	}
}

// TestMediaElementClearsFrameOnError：源不可达（本引擎无网络栈）时派发 error 并
// 清状态——不能一边报错一边让宿主继续抽帧。
func TestMediaElementClearsFrameOnError(t *testing.T) {
	useVideoFrameSource(t)
	rt, doc, _ := newRuntimeWithDoc(t)
	video := newVideoFixture(doc)

	mustRun(t, rt, `document.getElementById("player").src = "https://example.com/x.mp4";`)
	drainEventLoop(rt)

	st := mediaStateFor(rt, video)
	if !st.hasError {
		t.Fatalf("http 源应进入错误状态")
	}
	if _, ok := rendering.ElementVideoState(video); ok {
		t.Fatalf("出错后不应保留视频状态记录")
	}
}

// TestAudioElementNeverRequestsFrame：<audio> 没有画面，不参与帧通道。
func TestAudioElementNeverRequestsFrame(t *testing.T) {
	useVideoFrameSource(t)
	rt, doc, _ := newRuntimeWithDoc(t)
	audio := doc.CreateElement("audio")
	audio.SetId("sound")
	doc.AppendChild(audio)
	MediaMetadataResolver = func(src string) (MediaMetadata, bool) {
		return MediaMetadata{Duration: 4}, src == "sound.mp3"
	}
	defer func() { MediaMetadataResolver = nil }()

	mustRun(t, rt, `
		{
		const a = document.getElementById("sound");
		a.src = "sound.mp3";
		a.play();
		}
	`)
	drainEventLoop(rt)

	if st, ok := rendering.ElementVideoState(audio); ok {
		t.Fatalf("audio 不应产生视频状态，got %+v", st)
	}
}
