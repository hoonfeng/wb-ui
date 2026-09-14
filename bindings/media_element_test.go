package bindings

import (
	"math"
	"testing"

	"wb-ui/dom"
	"wb-ui/jsc"
)

// newVideoFixture 建一个 <video id="player"> 挂到文档上。
func newVideoFixture(doc *dom.Document) *dom.Element {
	video := doc.CreateElement("video")
	video.SetId("player")
	doc.AppendChild(video)
	return video
}

// drainEventLoop 推进事件循环直到没有待处理任务（宿主每帧做的事情）；媒体
// 事件是宏任务，且一次派发可能排出下一批任务，所以要循环到收敛。
func drainEventLoop(rt *jsc.Interpreter) {
	for i := 0; i < 40; i++ {
		loop := rt.GetEventLoop()
		if loop == nil {
			return
		}
		if loop.PendingTasks() == 0 {
			rt.RunJobs()
			if loop.PendingTasks() == 0 {
				return
			}
			continue
		}
		loop.ProcessTasks(0)
		rt.RunJobs()
	}
}

// TestMediaElementProperties 覆盖 HTMLMediaElement 的初始属性契约：框架挂载
// <video> 后立刻读这些属性，读到 undefined 就会把整段脚本带崩。
func TestMediaElementProperties(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	newVideoFixture(doc)

	mustRun(t, rt, `
		{
		const v = document.getElementById("player");
		if (typeof HTMLMediaElement !== "function") throw new Error("HTMLMediaElement 缺失");
		if (typeof HTMLVideoElement !== "function") throw new Error("HTMLVideoElement 缺失");
		if (typeof HTMLAudioElement !== "function") throw new Error("HTMLAudioElement 缺失");
		if (typeof MediaError !== "function") throw new Error("MediaError 缺失");
		if (typeof TimeRanges !== "function") throw new Error("TimeRanges 缺失");
		if (!(v instanceof HTMLMediaElement)) throw new Error("video 不是 HTMLMediaElement");
		if (!(v instanceof HTMLVideoElement)) throw new Error("video 不是 HTMLVideoElement");
		if (!(v instanceof HTMLElement)) throw new Error("video 不是 HTMLElement");
		if (!("play" in v)) throw new Error("'play' in video 应为 true");
		if (typeof v.play !== "function" || typeof v.pause !== "function") throw new Error("play/pause 缺失");
		if (typeof v.load !== "function" || typeof v.canPlayType !== "function") throw new Error("load/canPlayType 缺失");
		if (v.paused !== true) throw new Error("初始应为 paused");
		if (v.ended !== false) throw new Error("初始 ended 应为 false");
		if (v.seeking !== false) throw new Error("初始 seeking 应为 false");
		if (!Number.isNaN(v.duration)) throw new Error("未加载时 duration 应为 NaN，实际 " + v.duration);
		if (v.currentTime !== 0) throw new Error("初始 currentTime 应为 0");
		if (v.readyState !== 0) throw new Error("初始 readyState 应为 0（HAVE_NOTHING）");
		if (v.networkState !== 0) throw new Error("初始 networkState 应为 0（NETWORK_EMPTY）");
		if (v.volume !== 1) throw new Error("初始 volume 应为 1");
		if (v.playbackRate !== 1) throw new Error("初始 playbackRate 应为 1");
		if (v.muted !== false) throw new Error("初始 muted 应为 false");
		if (v.error !== null) throw new Error("初始 error 应为 null");
		if (v.buffered.length !== 0) throw new Error("未加载时 buffered 应为空");
		if (v.seekable.length !== 0) throw new Error("未加载时 seekable 应为空");
		if (v.loop !== false || v.autoplay !== false || v.controls !== false) {
			throw new Error("布尔属性初始应为 false");
		}
		if (v.preload !== "metadata") throw new Error("preload 缺省应为 metadata，实际 " + v.preload);
		if (v.crossOrigin !== null) throw new Error("未设置时 crossOrigin 应为 null");
		if (v.currentSrc !== "") throw new Error("currentSrc 初始应为空");
		}
	`)
}

// TestMediaElementCanPlayType 覆盖 canPlayType：已知容器 "maybe"，未知容器空串
// （脚本据此在多个 <source> 中选择——HLS 不被本引擎支持，必须诚实报空）。
func TestMediaElementCanPlayType(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	newVideoFixture(doc)

	mustRun(t, rt, `
		{
		const v = document.getElementById("player");
		if (v.canPlayType("video/mp4") !== "maybe") throw new Error("mp4 = " + v.canPlayType("video/mp4"));
		if (v.canPlayType("video/mp4; codecs=avc1") !== "maybe") throw new Error("带 codecs 的 mp4 失败");
		if (v.canPlayType("audio/mpeg") !== "maybe") throw new Error("mp3 失败");
		if (v.canPlayType("application/vnd.apple.mpegurl") !== "") throw new Error("HLS 应报空串");
		if (v.canPlayType("video/nonsense") !== "") throw new Error("未知类型应报空串");
		if (v.canPlayType("") !== "") throw new Error("空类型应报空串");
		}
	`)
}

// TestMediaElementPlayRejectsWithoutSource 覆盖无资源时 play() 的拒绝语义：
// 返回 rejected Promise（name=NotSupportedError），而不是抛同步异常。
func TestMediaElementPlayRejectsWithoutSource(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	newVideoFixture(doc)

	mustRun(t, rt, `
		{
		globalThis.__state = "pending";
		const v = document.getElementById("player");
		const p = v.play();
		if (!(p instanceof Promise)) throw new Error("play() 必须返回 Promise");
		p.then(() => { __state = "resolved"; }, (e) => { __state = "rejected:" + e.name; });
		}
	`)
	drainEventLoop(rt)
	if v, _ := rt.RunJS(`globalThis.__state`); v.ToString() != "rejected:NotSupportedError" {
		t.Fatalf("play() 无源时状态 = %v，want rejected:NotSupportedError", v)
	}
	if v, _ := rt.RunJS(`document.getElementById("player").paused`); !v.ToBoolean() {
		t.Fatal("拒绝后仍应是 paused")
	}
}

// TestMediaElementLoadPipeline 覆盖资源选择流程：src 变化 → loadstart →
// durationchange/loadedmetadata，readyState 从 HAVE_NOTHING 推进到
// HAVE_METADATA（时长未知，因为宿主没注入元数据）。
func TestMediaElementLoadPipeline(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	newVideoFixture(doc)

	mustRun(t, rt, `
		{
		globalThis.__events = [];
		const v = document.getElementById("player");
		["loadstart", "durationchange", "loadedmetadata", "canplay", "error"].forEach((t) => {
			v.addEventListener(t, () => { __events.push(t); });
		});
		v.src = "movie.mp4";
		}
	`)
	// 同步阶段：已进入 NETWORK_LOADING / HAVE_NOTHING，事件还没派发。
	if v, _ := rt.RunJS(`document.getElementById("player").networkState`); v.ToNumber() != 2 {
		t.Fatalf("设置 src 后 networkState = %v，want 2（LOADING）", v)
	}
	if v, _ := rt.RunJS(`globalThis.__events.length`); v.ToNumber() != 0 {
		t.Fatalf("同步阶段不应派发事件，got %v", v)
	}

	drainEventLoop(rt)
	mustRun(t, rt, `
		{
		const v = document.getElementById("player");
		if (v.readyState !== 1) throw new Error("readyState = " + v.readyState + "，want 1（HAVE_METADATA）");
		if (v.networkState !== 1) throw new Error("networkState = " + v.networkState + "，want 1（IDLE）");
		if (!Number.isNaN(v.duration)) throw new Error("未注入元数据时 duration 应为 NaN");
		const got = __events.join(",");
		if (got !== "loadstart,durationchange,loadedmetadata") {
			throw new Error("事件序列 = " + got);
		}
		if (v.currentSrc !== "movie.mp4") throw new Error("currentSrc = " + v.currentSrc);
		}
	`)
}

// TestMediaElementSourceChild 覆盖 <source> 资源选择：没有 src attribute 时
// 取第一个 <source> 子元素的 src（而不是把元素当成无源）。
func TestMediaElementSourceChild(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	video := newVideoFixture(doc)
	source := doc.CreateElement("source")
	source.SetAttribute("src", "clip.webm")
	source.SetAttribute("type", "video/webm")
	video.AppendChild(source)

	mustRun(t, rt, `
		{
		globalThis.__state = "pending";
		const v = document.getElementById("player");
		if (v.currentSrc !== "clip.webm") throw new Error("currentSrc = " + v.currentSrc);
		const p = v.play();
		p.then(() => { __state = "resolved"; }, (e) => { __state = "rejected:" + e.name; });
		}
	`)
	drainEventLoop(rt)
	if v, _ := rt.RunJS(`globalThis.__state`); v.ToString() != "resolved" {
		t.Fatalf("有 <source> 时 play() 应 resolve，实际 %v", v)
	}
	if v, _ := rt.RunJS(`document.getElementById("player").paused`); v.ToBoolean() {
		t.Fatal("play() 后 paused 应为 false")
	}
}

// TestMediaElementMetadataResolver 覆盖宿主注入元数据的路径（挂件宿主可用
// ffmpeg 探测时长）：duration 生效、readyState 推进到 HAVE_ENOUGH_DATA、
// canplay/canplaythrough 派发、buffered 变成 [0,duration]。
func TestMediaElementMetadataResolver(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	newVideoFixture(doc)
	MediaMetadataResolver = func(src string) (float64, bool) {
		if src == "probe.mp4" {
			return 12.5, true
		}
		return 0, false
	}
	defer func() { MediaMetadataResolver = nil }()

	mustRun(t, rt, `
		{
		globalThis.__events = [];
		const v = document.getElementById("player");
		["loadstart", "durationchange", "loadedmetadata", "loadeddata", "canplay", "canplaythrough"].forEach((t) => {
			v.addEventListener(t, () => { __events.push(t); });
		});
		v.src = "probe.mp4";
		}
	`)
	drainEventLoop(rt)
	mustRun(t, rt, `
		{
		const v = document.getElementById("player");
		if (v.duration !== 12.5) throw new Error("duration = " + v.duration);
		if (v.readyState !== 4) throw new Error("readyState = " + v.readyState + "，want 4");
		if (v.networkState !== 1) throw new Error("networkState = " + v.networkState);
		if (v.buffered.length !== 1 || v.buffered.start(0) !== 0 || v.buffered.end(0) !== 12.5) {
			throw new Error("buffered 不符");
		}
		if (!(v.buffered instanceof TimeRanges)) throw new Error("buffered 应为 TimeRanges 实例");
		const got = __events.join(",");
		if (got !== "loadstart,durationchange,loadedmetadata,loadeddata,canplay,canplaythrough") {
			throw new Error("事件序列 = " + got);
		}
		}
	`)
}

// TestMediaElementErrorForRemoteSource 覆盖不可达资源（http/https，本引擎无
// 网络栈）：派发 error + MediaError(code=4)，脚本的失败分支能走到。
func TestMediaElementErrorForRemoteSource(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	newVideoFixture(doc)

	mustRun(t, rt, `
		{
		globalThis.__errs = 0;
		const v = document.getElementById("player");
		v.addEventListener("error", () => { __errs++; });
		v.src = "https://example.com/movie.mp4";
		}
	`)
	drainEventLoop(rt)
	mustRun(t, rt, `
		{
		const v = document.getElementById("player");
		if (__errs !== 1) throw new Error("error 事件次数 = " + __errs);
		if (v.error === null) throw new Error("error 对象缺失");
		if (v.error.code !== 4) throw new Error("error.code = " + v.error.code);
		if (typeof v.error.message !== "string" || v.error.message.length === 0) {
			throw new Error("error.message 应为非空描述: " + v.error.message);
		}
		if (!(v.error instanceof MediaError)) throw new Error("error 应为 MediaError 实例");
		if (v.readyState !== 0) throw new Error("失败后 readyState = " + v.readyState);
		if (v.networkState !== 3) throw new Error("失败后 networkState = " + v.networkState + "，want 3（NO_SOURCE）");
		if (!Number.isNaN(v.duration)) throw new Error("失败后 duration 应为 NaN");
		}
	`)
}

// TestMediaElementPlayPauseEvents 覆盖 play/pause 的状态机与事件（含 on* 处理器
// 属性与 addEventListener 两种注册方式、重复 pause 不派发事件）。
func TestMediaElementPlayPauseEvents(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	newVideoFixture(doc)

	mustRun(t, rt, `
		{
		globalThis.__events = [];
		const v = document.getElementById("player");
		v.src = "movie.mp4";
		["play", "playing", "pause"].forEach((t) => {
			v.addEventListener(t, () => { __events.push(t); });
		});
		// on* 处理器属性（等价于 addEventListener，再次赋值替换旧处理器）
		globalThis.__onplays = 0;
		v.onplay = () => { __onplays++; };
		v.play();
		}
	`)
	drainEventLoop(rt)
	mustRun(t, rt, `
		{
		const v = document.getElementById("player");
		if (v.paused !== false) throw new Error("play() 后 paused 应为 false");
		if (__onplays !== 1) throw new Error("onplay 处理器调用次数 = " + __onplays);
		if (__events.join(",") !== "play,playing") throw new Error("事件序列 = " + __events.join(","));
		if (typeof v.onplay !== "function") throw new Error("onplay 应返回原处理器");
		}
	`)
	mustRun(t, rt, `
		{
		const v = document.getElementById("player");
		v.pause();
		if (v.paused !== true) throw new Error("pause() 后应为 paused");
		v.pause();
		}
	`)
	drainEventLoop(rt)
	if v, _ := rt.RunJS(`globalThis.__events.join(",")`); v.ToString() != "play,playing,pause" {
		t.Fatalf("事件序列 = %v（重复 pause 不应再派发）", v)
	}
}

// TestMediaElementHandlerReplacement 覆盖 on* 处理器属性的替换语义：重新赋值
// 后旧处理器不再被调用（IDL 事件处理器属性每个事件只有一个）。
func TestMediaElementHandlerReplacement(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	newVideoFixture(doc)

	mustRun(t, rt, `
		{
		globalThis.__first = 0;
		globalThis.__second = 0;
		const v = document.getElementById("player");
		v.src = "movie.mp4";
		v.onvolumechange = () => { __first++; };
		v.volume = 0.5;
		if (__first !== 1) throw new Error("第一次 volumechange 未到达: " + __first);
		v.onvolumechange = () => { __second++; };
		if (v.volume !== 0.5) throw new Error("volume = " + v.volume);
		v.volume = 0.25;
		if (__first !== 1 || __second !== 1) throw new Error("替换失败: " + __first + "/" + __second);
		v.onvolumechange = null;
		v.volume = 0.75;
		if (__first !== 1 || __second !== 1) throw new Error("移除失败: " + __first + "/" + __second);
		}
	`)
}

// TestMediaElementMutedAndVolume 覆盖 muted/volume/defaultMuted 的反射与状态语义。
func TestMediaElementMutedAndVolume(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	newVideoFixture(doc)

	mustRun(t, rt, `
		{
		const v = document.getElementById("player");
		// muted IDL 属性的初始值来自 muted content attribute
		v.setAttribute("muted", "");
		if (v.muted !== true) throw new Error("muted attribute 未生效");
		if (v.defaultMuted !== true) throw new Error("defaultMuted 应反射 muted attribute");
		v.defaultMuted = false;
		if (v.hasAttribute("muted")) throw new Error("defaultMuted=false 应移除 attribute");
		if (v.muted !== true) throw new Error("muted 是状态，不该被 defaultMuted 改动");
		v.muted = false;
		if (v.muted !== false) throw new Error("muted 不可写");
		// volume 夹取到 [0,1]
		v.volume = 1.5;
		if (v.volume !== 1) throw new Error("volume 未夹取: " + v.volume);
		v.volume = -1;
		if (v.volume !== 0) throw new Error("volume 未夹取: " + v.volume);
		}
	`)
}

// TestMediaElementPlaybackRate 覆盖 playbackRate/defaultPlaybackRate 与 ratechange。
func TestMediaElementPlaybackRate(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	newVideoFixture(doc)

	mustRun(t, rt, `
		{
		globalThis.__rates = 0;
		const v = document.getElementById("player");
		v.addEventListener("ratechange", () => { __rates++; });
		v.playbackRate = 2;
		if (v.playbackRate !== 2) throw new Error("playbackRate = " + v.playbackRate);
		v.playbackRate = 2;
		if (__rates !== 1) throw new Error("重复赋值不应派发 ratechange: " + __rates);
		v.playbackRate = -1;
		if (v.playbackRate !== 2) throw new Error("非法速率应被忽略: " + v.playbackRate);
		v.defaultPlaybackRate = 0.5;
		if (v.defaultPlaybackRate !== 0.5) throw new Error("defaultPlaybackRate = " + v.defaultPlaybackRate);
		// 规范：defaultPlaybackRate 是「开始播放时的初始速率」，不改当前 playbackRate
		if (v.playbackRate !== 2) {
			throw new Error("defaultPlaybackRate 不应改变当前 playbackRate: " + v.playbackRate);
		}
		if (__rates !== 1) throw new Error("ratechange 次数 = " + __rates);
		}
	`)
}

// TestMediaElementSeek 覆盖 currentTime 写入 / fastSeek 的夹取与 seeking/seeked。
func TestMediaElementSeek(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	video := newVideoFixture(doc)
	st := mediaStateFor(rt, video)

	st.loadedSrc = "probe.mp4"
	st.duration = 10

	mustRun(t, rt, `
		{
		globalThis.__events = [];
		const v = document.getElementById("player");
		["seeking", "seeked"].forEach((t) => { v.addEventListener(t, () => { __events.push(t); }); });
		v.currentTime = 4;
		if (v.currentTime !== 4) throw new Error("currentTime = " + v.currentTime);
		v.fastSeek(99);
		if (v.currentTime !== 10) throw new Error("超过时长应夹取到 duration: " + v.currentTime);
		v.fastSeek(-5);
		if (v.currentTime !== 0) throw new Error("负值应夹取到 0: " + v.currentTime);
		}
	`)
	drainEventLoop(rt)
	if v, _ := rt.RunJS(`globalThis.__events.join(",")`); v.ToString() != "seeking,seeked,seeking,seeked,seeking,seeked" {
		t.Fatalf("seek 事件序列 = %s", v.ToString())
	}
}

// TestMediaElementPlayClock 覆盖播放时钟（白盒）：时长已知时按步长推进
// currentTime，到达终点置 ended/paused 并派发 timeupdate + ended；时长未知时
// 不推进（不编造进度）。
func TestMediaElementPlayClock(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	video := newVideoFixture(doc)
	st := mediaStateFor(rt, video)

	mustRun(t, rt, `
		{
		globalThis.__events = [];
		const v = document.getElementById("player");
		["timeupdate", "ended"].forEach((t) => { v.addEventListener(t, () => { __events.push(t); }); });
		}
	`)

	st.duration = 1
	st.paused = false
	st.clock = true
	for i := 0; i < 3; i++ {
		st.tick()
	}
	if math.Abs(st.currentTime-0.75) > 1e-9 {
		t.Fatalf("三次 tick 后 currentTime = %v，want 0.75", st.currentTime)
	}
	st.tick() // 0.75 → 1.0：到达终点
	if !st.ended || !st.paused {
		t.Fatalf("到达终点后 ended=%v paused=%v，want true/true", st.ended, st.paused)
	}
	if math.Abs(st.currentTime-1) > 1e-9 {
		t.Fatalf("终点 currentTime = %v，want 1", st.currentTime)
	}
	if v, _ := rt.RunJS(`globalThis.__events.join(",")`); v.ToString() != "timeupdate,timeupdate,timeupdate,timeupdate,ended" {
		t.Fatalf("时钟事件序列 = %v", v)
	}
	// 停止后不再推进
	st.tick()
	if math.Abs(st.currentTime-1) > 1e-9 {
		t.Fatalf("停止后 currentTime 仍在变化: %v", st.currentTime)
	}

	// 时长未知：不推进 currentTime
	st2 := &mediaElementState{interp: rt, el: video, duration: math.NaN(), paused: false, clock: true, volume: 1, playbackRate: 1}
	st2.tick()
	if st2.currentTime != 0 {
		t.Fatalf("时长未知时不应推进 currentTime，got %v", st2.currentTime)
	}
}

// TestMediaElementAddTextTrack 覆盖 addTextTrack(kind,label,language)：返回的
// 轨道出现在 textTracks 里，且与 <track>.track 是同一对象。
func TestMediaElementAddTextTrack(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	newVideoFixture(doc)

	mustRun(t, rt, `
		{
		const v = document.getElementById("player");
		const track = v.addTextTrack("descriptions", "描述", "zh");
		if (!(track instanceof TextTrack)) throw new Error("返回值不是 TextTrack");
		if (track.kind !== "descriptions") throw new Error("kind = " + track.kind);
		if (track.label !== "描述") throw new Error("label = " + track.label);
		if (track.language !== "zh") throw new Error("language = " + track.language);
		if (v.textTracks.length !== 1) throw new Error("textTracks.length = " + v.textTracks.length);
		if (v.textTracks[0] !== track) throw new Error("textTracks[0] 与返回值不是同一对象");
		const second = v.addTextTrack("captions");
		if (v.textTracks.length !== 2) throw new Error("第二次 addTextTrack 后 length = " + v.textTracks.length);
		if (second.kind !== "captions") throw new Error("缺省 kind 失败");
		}
	`)
}

// TestMediaElementLoadResets 覆盖 load() 的复位语义：重新加载会清掉时长/进度/
// 错误并派发 emptied + loadstart。
func TestMediaElementLoadResets(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	newVideoFixture(doc)

	mustRun(t, rt, `
		{
		globalThis.__events = [];
		const v = document.getElementById("player");
		["emptied", "loadstart", "loadedmetadata"].forEach((t) => {
			v.addEventListener(t, () => { __events.push(t); });
		});
		v.src = "movie.mp4";
		}
	`)
	drainEventLoop(rt)
	mustRun(t, rt, `
		{
		const v = document.getElementById("player");
		v.load();
		if (v.readyState !== 0) throw new Error("load() 后 readyState = " + v.readyState);
		if (v.currentTime !== 0) throw new Error("load() 后 currentTime = " + v.currentTime);
		}
	`)
	drainEventLoop(rt)
	if v, _ := rt.RunJS(`globalThis.__events.join(",")`); v.ToString() != "loadstart,loadedmetadata,emptied,loadstart,loadedmetadata" {
		t.Fatalf("load() 事件序列 = %s", v.ToString())
	}
}

// TestMediaElementInstanceofPrototypeChain 验证视频/音频元素的原型链与音频路径
// （audio 元素同样有媒体属性，此前连 src 都没有）。
func TestMediaElementInstanceofPrototypeChain(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	audio := doc.CreateElement("audio")
	audio.SetId("sound")
	audio.SetAttribute("src", "song.mp3")
	doc.AppendChild(audio)

	mustRun(t, rt, `
		{
		const a = document.getElementById("sound");
		if (!(a instanceof HTMLAudioElement)) throw new Error("audio 不是 HTMLAudioElement");
		if (!(a instanceof HTMLMediaElement)) throw new Error("audio 不是 HTMLMediaElement");
		if (a.src !== "song.mp3") throw new Error("audio.src = " + a.src);
		if (a.paused !== true) throw new Error("audio 初始应为 paused");
		if (a.currentSrc !== "song.mp3") throw new Error("audio.currentSrc = " + a.currentSrc);
		if (typeof a.play !== "function") throw new Error("audio.play 缺失");
		// 非媒体元素不受影响
		const div = document.createElement("div");
		if (div instanceof HTMLMediaElement) throw new Error("div 不该是 HTMLMediaElement");
		if ("paused" in div) throw new Error("div 不该有 paused");
		}
	`)
}
