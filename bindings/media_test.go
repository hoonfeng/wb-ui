package bindings

import (
	"strings"
	"testing"

	"wb-ui/dom"
)

// newMediaFixtureDoc 构建 dev/suites/cssprobe/fixtures/media-text-track.html 的 DOM
// 结构：<video> + 一个 data: URL 的 <track> 字幕，外加结果容器。
func newMediaFixtureDoc(doc *dom.Document) (*dom.Element, *dom.Element) {
	video := doc.CreateElement("video")
	track := doc.CreateElement("track")
	track.SetId("captions")
	track.SetAttribute("kind", "captions")
	track.SetAttribute("srclang", "en")
	track.SetAttribute("default", "")
	// 与夹具逐字相同的 data: URL（percent 编码的 WEBVTT）
	track.SetAttribute("src", "data:text/vtt,WEBVTT%0A%0A00%3A00%3A01.000%20--%3E%2000%3A00%3A03.000%0AHello")
	result := doc.CreateElement("div")
	result.SetId("result")
	doc.AppendChild(video)
	video.AppendChild(track)
	doc.AppendChild(result)
	return video, track
}

// TestMediaTextTrackFixture 复刻 media-text-track 夹具的完整契约：
// element.track.cues[0] 可读可写、video.textTracks[0] 与 element.track 是
// 同一对象、cue.text 为解析出的正文。夹具在同步阶段调用一次 complete()，
// 因此首次访问 .track 就必须能读到 cues（data: URL 视为本地即时资源）。
func TestMediaTextTrackFixture(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	newMediaFixtureDoc(doc)

	mustRun(t, rt, `
		{
		const element = document.getElementById("captions");
		function complete() {
			const cue = element.track.cues && element.track.cues[0];
			if (!cue) return;
			cue.line = -2;
			cue.size = 80;
			const videoTrack = document.querySelector("video").textTracks[0];
			if (videoTrack === element.track && cue.text === "Hello") {
				const result = document.getElementById("result");
				result.textContent = "media text track completed";
				result.className = "passed";
			}
		}
		complete();
		element.addEventListener("load", complete, { once: true });
		}
	`)

	mustRun(t, rt, `
		{
		const element = document.getElementById("captions");
		if (element.track.cues.length !== 1) {
			throw new Error("cues.length = " + element.track.cues.length);
		}
		const cue = element.track.cues[0];
		if (cue.text !== "Hello") throw new Error("cue.text = " + cue.text);
		if (cue.startTime !== 1 || cue.endTime !== 3) {
			throw new Error("cue timing = " + cue.startTime + ".." + cue.endTime);
		}
		if (cue.line !== -2 || cue.size !== 80) throw new Error("cue 属性不可写");
		if (element.track.kind !== "captions") throw new Error("kind = " + element.track.kind);
		if (element.track.language !== "en") throw new Error("language = " + element.track.language);
		if (element.track.readyState !== 2) throw new Error("readyState = " + element.track.readyState);
		if (element.track.cues !== element.track.cues) throw new Error("cues 必须同一实例");
		const list = document.querySelector("video").textTracks;
		if (list.length !== 1) throw new Error("textTracks.length = " + list.length);
		if (list[0] !== element.track) throw new Error("textTracks[0] 与 element.track 必须是同一对象");
		if (!(list instanceof TextTrackList)) throw new Error("textTracks 不是 TextTrackList");
		if (!(element.track instanceof TextTrack)) throw new Error("track 不是 TextTrack");
		if (list.getTrackById("captions") !== element.track) throw new Error("getTrackById 失败");
		if (element.track.cues.getCueById("nope") !== null) throw new Error("getCueById 应返回 null");
		const result = document.getElementById("result");
		if (result.className !== "passed") throw new Error("结果未标记通过: " + result.className);
		}
	`)
}

// TestMediaTextTrackLoadEventAsync 覆盖 load 事件：按规范异步派发（经
// EventLoop 宏任务），且监听器在同步阶段注册后仍能收到。
func TestMediaTextTrackLoadEventAsync(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	newMediaFixtureDoc(doc)

	mustRun(t, rt, `
		{
		globalThis.__loads = 0;
		const element = document.getElementById("captions");
		// 先触发一次加载（同步解析），再注册监听器：异步派发必须仍能到达。
		void element.track;
		element.addEventListener("load", () => { globalThis.__loads++; }, { once: true });
		}
	`)
	if v, _ := rt.RunJS(`globalThis.__loads`); v.ToNumber() != 0 {
		t.Fatalf("load 事件不应在同步阶段派发，got %v", v.ToNumber())
	}
	// 驱动事件循环（宿主每帧做的事情）
	if loop := rt.GetEventLoop(); loop != nil {
		loop.ProcessTasks(0)
	}
	rt.RunJobs()
	if v, _ := rt.RunJS(`globalThis.__loads`); v.ToNumber() != 1 {
		t.Fatalf("load 事件次数 = %v, want 1", v.ToNumber())
	}
}

// TestMediaTextTrackErrorForUnsupportedScheme 覆盖非 data: 资源：本引擎没有
// 网络栈，按规范异步派发 error（脚本的失败分支能走到，而不是静默挂起）。
func TestMediaTextTrackErrorForUnsupportedScheme(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	video := doc.CreateElement("video")
	track := doc.CreateElement("track")
	track.SetId("remote")
	track.SetAttribute("src", "https://example.com/captions.vtt")
	doc.AppendChild(video)
	video.AppendChild(track)

	mustRun(t, rt, `
		{
		globalThis.__errors = 0;
		const element = document.getElementById("remote");
		element.addEventListener("error", () => { globalThis.__errors++; }, { once: true });
		void element.track;
		}
	`)
	if loop := rt.GetEventLoop(); loop != nil {
		loop.ProcessTasks(0)
	}
	rt.RunJobs()
	if v, _ := rt.RunJS(`globalThis.__errors`); v.ToNumber() != 1 {
		t.Fatalf("error 事件次数 = %v, want 1", v.ToNumber())
	}
	mustRun(t, rt, `
		{
		const element = document.getElementById("remote");
		if (element.track.cues.length !== 0) throw new Error("失败轨道不应有 cue");
		if (element.track.readyState !== 3) throw new Error("readyState 应为 3（ERROR），实际 " + element.track.readyState);
		}
	`)
}

// TestMediaTextTrackListIsLive 覆盖 TextTrackList 的 live 语义：新增/移除
// <track> 元素后 length 与索引随树变化（框架动态挂字幕依赖）。
func TestMediaTextTrackListIsLive(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	video, _ := newMediaFixtureDoc(doc)
	extra := doc.CreateElement("track")
	extra.SetId("second")
	extra.SetAttribute("kind", "descriptions")
	extra.SetAttribute("src", "data:text/vtt,WEBVTT%0A%0A00%3A00%3A05.000%20--%3E%2000%3A00%3A06.000%0AWorld")

	mustRun(t, rt, `
		{
		const video = document.querySelector("video");
		if (video.textTracks.length !== 1) throw new Error("初始 length = " + video.textTracks.length);
		}
	`)
	video.AppendChild(extra)
	mustRun(t, rt, `
		{
		const video = document.querySelector("video");
		if (video.textTracks.length !== 2) throw new Error("追加后 length = " + video.textTracks.length);
		if (video.textTracks[1].id !== "second") throw new Error("索引未按树序刷新");
		if (video.textTracks[1].cues[0].text !== "World") throw new Error("第二个轨道未解析");
		if (video.textTracks[0].id !== "captions" || video.textTracks[0].cues[0].text !== "Hello") {
			throw new Error("第一个轨道被污染");
		}
		}
	`)
	video.RemoveChild(extra)
	mustRun(t, rt, `
		{
		const video = document.querySelector("video");
		if (video.textTracks.length !== 1) throw new Error("移除后 length = " + video.textTracks.length);
		if (video.textTracks[1] !== undefined) throw new Error("多余索引未清理");
		}
	`)
}

// TestVTTCueConstruction 覆盖 new VTTCue(start, end, text) 与 instanceof
// （脚本自建提示的路径，也验证原型链而非仅数据属性）。
func TestVTTCueConstruction(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	_ = doc

	mustRun(t, rt, `
		{
		const cue = new VTTCue(0, 2.5, "hello");
		if (!(cue instanceof VTTCue)) throw new Error("instanceof VTTCue 失败");
		if (cue.startTime !== 0 || cue.endTime !== 2.5 || cue.text !== "hello") {
			throw new Error("VTTCue 构造参数未生效");
		}
		const track = new TextTrack();
		track.addCue(cue);
		if (track.cues.length !== 1 || track.cues[0] !== cue) throw new Error("addCue 失败");
		track.removeCue(cue);
		if (track.cues.length !== 0 || track.cues[0] !== undefined) throw new Error("removeCue 失败");
		}
	`)
}

// TestWebVTTParsing 直接覆盖解析器：多 cue、标识行、settings、时间戳形式
// （HH:MM:SS.mmm 与 MM:SS.mmm、"," 小数分隔符）。
func TestWebVTTParsing(t *testing.T) {
	src := strings.Join([]string{
		"WEBVTT - 测试文件",
		"",
		"intro",
		"00:00:01.000 --> 00:00:03.000 align:start line:5%",
		"第一行",
		"第二行",
		"",
		"01:02:03,500 --> 01:02:05,000",
		"第三个",
		"",
		"NOTE 这不是 cue",
		"",
		"00:10.000 --> 00:12.000",
		"短时",
	}, "\n")
	cues := parseWebVTT(src)
	if len(cues) != 3 {
		t.Fatalf("解析出 %d 条 cue，want 3", len(cues))
	}
	if cues[0].ID != "intro" || cues[0].Text != "第一行\n第二行" {
		t.Fatalf("cue0 = %+v", cues[0])
	}
	if cues[0].Start != 1 || cues[0].End != 3 {
		t.Fatalf("cue0 timing = %v..%v", cues[0].Start, cues[0].End)
	}
	if cues[0].Settings["align"] != "start" || cues[0].Settings["line"] != "5%" {
		t.Fatalf("cue0 settings = %v", cues[0].Settings)
	}
	if cues[1].Start != 3723.5 || cues[1].End != 3725 {
		t.Fatalf("cue1 timing = %v..%v", cues[1].Start, cues[1].End)
	}
	if cues[2].Start != 10 || cues[2].End != 12 {
		t.Fatalf("cue2 timing = %v..%v", cues[2].Start, cues[2].End)
	}
}

// TestDecodeDataURL 覆盖 data: URL 的两种载荷形式与百分号边界。
func TestDecodeDataURL(t *testing.T) {
	if mime, body, ok := decodeDataURL("data:text/vtt,WEBVTT%0A%0AHello"); !ok || mime != "text/vtt" || body != "WEBVTT\n\nHello" {
		t.Fatalf("percent 载荷解析失败: %q %q %v", mime, body, ok)
	}
	if mime, body, ok := decodeDataURL("data:text/vtt;base64,V0VCVlRUCkhp"); !ok || mime != "text/vtt" || body != "WEBVTT\nHi" {
		t.Fatalf("base64 载荷解析失败: %q %q %v", mime, body, ok)
	}
	if _, _, ok := decodeDataURL("https://example.com/a.vtt"); ok {
		t.Fatal("非 data: URL 不应被解码")
	}
	// "+" 在 data URL 中保持原义（不是空格）
	if _, body, ok := decodeDataURL("data:text/plain,a+b"); !ok || body != "a+b" {
		t.Fatalf("加号语义错误: %q", body)
	}
}
