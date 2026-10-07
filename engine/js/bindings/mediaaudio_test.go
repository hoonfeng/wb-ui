package bindings

// 音频会话驱动播放时钟的测试（主线 A3）。
//
// 这些测试覆盖的核心主张是「音频为主时钟」：currentTime 不是引擎自己数的挂钟，
// 而是**输出侧已播位置**（rendering.AudioSession.Position）。用假的会话替身就能
// 精确断言这一点——真设备（waveOut）的端到端取证在探针侧（判据 A）与 app 侧的
// 集成测试里，见 cmd/psai/mediaprobe.go 与 docs/media-format-verification-plan.md。

import (
	"math"
	"testing"

	"wb-ui/engine/rendering"
)

// fakeAudioSession 是 rendering.AudioSession 的测试替身：位置由测试显式设定，
// 因此「currentTime 是否等于会话位置」是可精确断言的。
type fakeAudioSession struct {
	format    rendering.AudioFormat
	total     int64
	pos       float64
	hasOutput bool
	failed    bool
	paused    bool
	closed    bool
	seeks     []float64
}

func (f *fakeAudioSession) Format() rendering.AudioFormat { return f.format }
func (f *fakeAudioSession) TotalFrames() int64            { return f.total }
func (f *fakeAudioSession) Position() float64             { return f.pos }
func (f *fakeAudioSession) HasOutput() bool               { return f.hasOutput }
func (f *fakeAudioSession) Failed() bool                  { return f.failed }
func (f *fakeAudioSession) Pause() error                  { f.paused = true; return nil }
func (f *fakeAudioSession) Resume() error                 { f.paused = false; return nil }
func (f *fakeAudioSession) Seek(s float64) error {
	f.seeks = append(f.seeks, s)
	f.pos = s
	return nil
}
func (f *fakeAudioSession) Close() error { f.closed = true; return nil }

// newAudioFixture 建一个加载完成的 <audio> 并注册假的会话源。
//
// 返回的 session() 每次调用取**当前**会话：会话是 play() 那一刻才创建的，而
// fixture 在此之前就返回了——直接返回变量快照只会永远拿到 nil。
func newAudioFixture(t *testing.T, meta MediaMetadata) (session func() *fakeAudioSession, st *mediaElementState) {
	t.Helper()
	rt, doc, _ := newRuntimeWithDoc(t)
	prevResolver := MediaMetadataResolver
	MediaMetadataResolver = func(string) (MediaMetadata, bool) { return meta, true }
	t.Cleanup(func() {
		rendering.SetAudioSessionSource(nil)
		MediaMetadataResolver = prevResolver
	})

	el := doc.CreateElement("audio")
	el.SetId("track")
	el.SetAttribute("src", "clip.wav")
	doc.AppendChild(el)

	var fake *fakeAudioSession
	rendering.SetAudioSessionSource(func(string, float64) (rendering.AudioSession, bool) {
		fake = &fakeAudioSession{
			format:    rendering.AudioFormat{SampleRate: 48000, Channels: 2},
			total:     48000, // 1 秒
			hasOutput: true,
		}
		return fake, true
	})

	StartMediaElementLoads(rt, doc)
	drainEventLoop(rt)
	return func() *fakeAudioSession { return fake }, mediaStateFor(rt, el)
}

// TestAudioSessionDrivesCurrentTime 断言播放时钟由会话位置驱动：
// play() 之后 currentTime 恰好等于 Position()，而不是按挂钟步长自增。
func TestAudioSessionDrivesCurrentTime(t *testing.T) {
	session, st := newAudioFixture(t, MediaMetadata{Duration: 1, HasAudio: true})
	if st == nil {
		t.Fatal("应有媒体状态")
	}
	if st.readyState < mediaHaveEnoughData {
		t.Fatalf("元数据齐全时应达 HAVE_ENOUGH_DATA，实际 %d", st.readyState)
	}

	st.play()
	fake := session()
	if fake == nil {
		t.Fatalf("play() 应打开音频会话（registered=%v src=%q paused=%v）",
			rendering.AudioSessionSourceRegistered(), st.effectiveSrc(), st.paused)
	}
	if st.currentTime != 0 {
		t.Fatalf("刚播放时 currentTime 应为 0，实际 %v", st.currentTime)
	}

	// 位置推进由输出侧决定：引擎只做「取回」。
	fake.pos = 0.5
	st.tick()
	if math.Abs(st.currentTime-0.5) > 1e-9 {
		t.Fatalf("currentTime 应等于会话位置 0.5，实际 %v", st.currentTime)
	}
	fake.pos = 0.9
	st.tick()
	if math.Abs(st.currentTime-0.9) > 1e-9 {
		t.Fatalf("currentTime 应等于会话位置 0.9，实际 %v", st.currentTime)
	}
	// 关键反证：位置不动时 currentTime 也不动（挂钟模式会继续自增 0.25）。
	prev := st.currentTime
	st.tick()
	if st.currentTime != prev {
		t.Fatalf("会话位置未推进时 currentTime 不应变化：%v → %v", prev, st.currentTime)
	}
}

// TestAudioSessionEndedAtDuration 断言播到末尾：位置到达时长即 ended，
// currentTime 收敛到 duration，且会话被关闭（设备不该在结束后继续占着）。
func TestAudioSessionEndedAtDuration(t *testing.T) {
	session, st := newAudioFixture(t, MediaMetadata{Duration: 1, HasAudio: true})
	st.play()
	fake := session()
	if fake == nil {
		t.Fatal("play() 应打开音频会话")
	}

	fake.pos = 1.0
	st.tick()
	if !st.ended {
		t.Fatalf("位置到达时长后应 ended（currentTime=%v）", st.currentTime)
	}
	if math.Abs(st.currentTime-1.0) > 1e-9 {
		t.Fatalf("ended 时 currentTime 应收敛到 duration=1，实际 %v", st.currentTime)
	}
	if !fake.closed {
		t.Fatal("ended 后应关闭音频会话")
	}
	if !st.paused {
		t.Fatal("ended 后元素应处于暂停态（规范）")
	}
}

// TestAudioSessionSeekFollowsCurrentTime 断言 seek 同时移动输出位置：
// 只改 currentTime 不改会话会先听到旧位置的一段音频。
func TestAudioSessionSeekFollowsCurrentTime(t *testing.T) {
	session, st := newAudioFixture(t, MediaMetadata{Duration: 1, HasAudio: true})
	st.play()
	fake := session()
	if fake == nil {
		t.Fatal("play() 应打开音频会话")
	}

	st.setCurrentTime(0.4)
	if len(fake.seeks) != 1 || math.Abs(fake.seeks[0]-0.4) > 1e-9 {
		t.Fatalf("seek 应把会话位置也移到 0.4，实际 %v", fake.seeks)
	}
}

// TestAudioPauseStopsOutput 断言 pause 同时挂起输出（只停时钟不停输出会让声音
// 继续播，且恢复时位置跳一段）。
func TestAudioPauseStopsOutput(t *testing.T) {
	session, st := newAudioFixture(t, MediaMetadata{Duration: 1, HasAudio: true})
	st.play()
	fake := session()
	if fake == nil {
		t.Fatal("play() 应打开音频会话")
	}
	st.pause()
	if !fake.paused {
		t.Fatal("pause() 应挂起音频输出")
	}
	if fake.closed {
		t.Fatal("pause() 不应关闭会话（恢复后要从原处继续）")
	}
	st.play()
	if fake.paused {
		t.Fatal("再次 play() 应恢复输出（复用同一会话）")
	}
}

// TestNoAudioTrackKeepsWallClock 断言纯视频资源不会打开音频会话：
// 给没有音轨的资源开一条永不推进的音频时钟会让 currentTime 卡死在 0。
func TestNoAudioTrackKeepsWallClock(t *testing.T) {
	session, st := newAudioFixture(t, MediaMetadata{Duration: 1, HasAudio: false})
	if st == nil {
		t.Fatal("应有媒体状态")
	}
	st.play()
	if session() != nil {
		t.Fatal("元数据表明无音轨时不应打开音频会话")
	}
	// 退回挂钟时钟：tick 仍按步长推进（1 秒样本 250ms 一步 → 0.25）。
	st.tick()
	if math.Abs(st.currentTime-0.25) > 1e-9 {
		t.Fatalf("无音频时应走挂钟时钟（0.25），实际 %v", st.currentTime)
	}
}

// TestFailedAudioSessionFallsBack 断言会话失败（无音轨/解码器起不来）时回退
// 挂钟：不能让播放挂在一个永不推进的位置上。
func TestFailedAudioSessionFallsBack(t *testing.T) {
	session, st := newAudioFixture(t, MediaMetadata{Duration: 1, HasAudio: true})
	st.play()
	fake := session()
	if fake == nil {
		t.Fatal("play() 应打开会话")
	}
	fake.failed = true
	fake.pos = 0
	st.tick()
	if st.audio != nil {
		t.Fatal("会话失败后应关闭并从元素上摘掉")
	}
	if !fake.closed {
		t.Fatal("失败会话应被关闭")
	}
	// 回退发生在这次 tick 内，挂钟随即推进第一步（0 → 0.25）；下一步继续推进。
	if math.Abs(st.currentTime-0.25) > 1e-9 {
		t.Fatalf("摘掉失败会话后应同一步走挂钟并推进到 0.25，实际 %v", st.currentTime)
	}
	st.tick()
	if math.Abs(st.currentTime-0.5) > 1e-9 {
		t.Fatalf("挂钟应继续按步长推进到 0.5，实际 %v", st.currentTime)
	}
}
