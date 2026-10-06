package bindings

// A2-③：requestVideoFrameCallback —— 「新一帧呈现」时回调一次即失效。
//
// 与 timeupdate 的区别是这条判据的全部意义：timeupdate 按固定频率报时钟，rVFC 只在
// **真的有新帧**时回调。所以断言落在「帧号不变就不回调、帧号一变就回调、metadata 的
// mediaTime 是帧起点而不是 currentTime 的随机位置」。

import (
	"encoding/json"
	"math"
	"testing"

	"wb-ui/engine/js/jsc"
)

// frameCallbackRecord 是一次 rVFC 回调记录到的 metadata（kind 是本测试加的标签）。
type frameCallbackRecord struct {
	Kind      string  `json:"kind"`
	Media     float64 `json:"media"`
	Presented float64 `json:"presented"`
	W         float64 `json:"w"`
	H         float64 `json:"h"`
}

// TestRequestVideoFrameCallbackFiresOnNewFrame 覆盖 rVFC 的四条契约：
//
//	① 帧号变化（时钟推进到新帧）时触发一次；
//	② metadata 的 mediaTime 是**该帧起点**（10fps 下 0.25s → 0.2），
//	   presentedFrames 从 1 开始计数；
//	③ 一次性：触发过的 handle 立即失效（再推进时钟不会重复回调）；
//	④ cancelVideoFrameCallback 能在触发前摘掉回调，重新注册后 presentedFrames 递增。
func TestRequestVideoFrameCallbackFiresOnNewFrame(t *testing.T) {
	useVideoFrameSource(t)
	rt, doc, _ := newRuntimeWithDoc(t)
	video := newVideoFixture(doc)
	st0 := mediaStateFor(rt, video)
	st0.loadedSrc = "probe.mp4"
	mustRun(t, rt, `document.getElementById("player").setAttribute("src", "probe.mp4");`)
	st0.readyState = mediaHaveEnoughData
	st0.duration = 1
	st0.fps = 10
	st0.videoW, st0.videoH = 120, 80
	st0.paused = false
	st0.clock = true

	mustRun(t, rt, `window.__rc = [];
		window.__h1 = document.getElementById("player").requestVideoFrameCallback(function(now, md){
			window.__rc.push({kind:"first", media: md.mediaTime, presented: md.presentedFrames, w: md.width, h: md.height});
		});
		window.__h2 = document.getElementById("player").requestVideoFrameCallback(function(){
			window.__rc.push({kind:"canceled"});
		});
		document.getElementById("player").cancelVideoFrameCallback(window.__h2);`)
	if evalInt(t, rt, `(window.__h1 > 0 && window.__h2 > window.__h1) ? 1 : 0`) != 1 {
		t.Fatalf("句柄应为递增的正整数：h1=%s", evalStr(t, rt, `window.__h1 + "," + window.__h2`))
	}

	// 时钟推进一步：0 → 0.25（帧号 0 → 2）→ 呈现新帧。
	st0.tick()
	drainEventLoop(rt)
	recs := rvfcRecords(t, rt)
	if len(recs) != 1 || recs[0].Kind != "first" {
		t.Fatalf("应只触发未被取消的回调一次，got %+v", recs)
	}
	if recs[0].Presented != 1 {
		t.Errorf("presentedFrames = %v，want 1（规范从 1 开始）", recs[0].Presented)
	}
	if math.Abs(recs[0].Media-0.2) > 1e-9 {
		t.Errorf("mediaTime = %v，want 0.2（该帧起点，而不是 currentTime 0.25）", recs[0].Media)
	}
	if recs[0].W != 120 || recs[0].H != 80 {
		t.Errorf("metadata 尺寸 = %vx%v，want 120x80", recs[0].W, recs[0].H)
	}

	// 一次性：同一个 handle 不再触发。
	st0.tick()
	drainEventLoop(rt)
	if again := rvfcRecords(t, rt); len(again) != 1 {
		t.Fatalf("回调触发一次后应失效，got %+v", again)
	}

	// 重新注册 → 下一次呈现再触发，且 presentedFrames 递增、mediaTime 前进。
	mustRun(t, rt, `window.__h3 = document.getElementById("player").requestVideoFrameCallback(function(now, md){
		window.__rc.push({kind:"again", media: md.mediaTime, presented: md.presentedFrames});
	});`)
	st0.tick()
	drainEventLoop(rt)
	recs3 := rvfcRecords(t, rt)
	if len(recs3) != 2 || recs3[1].Kind != "again" {
		t.Fatalf("重新注册后应再次触发，got %+v", recs3)
	}
	if recs3[1].Presented != 2 {
		t.Errorf("presentedFrames = %v，want 2", recs3[1].Presented)
	}
	if recs3[1].Media <= recs3[0].Media {
		t.Errorf("mediaTime 应随帧推进递增：%v → %v", recs3[0].Media, recs3[1].Media)
	}
}

// TestRequestVideoFrameCallbackSameFrameNoFire：同一帧内时钟推进（10fps 下
// 0.55→0.59 仍是第 5 帧）**不**触发回调——这正是「帧呈现」与「时钟走字」的分界。
func TestRequestVideoFrameCallbackSameFrameNoFire(t *testing.T) {
	useVideoFrameSource(t)
	rt, doc, _ := newRuntimeWithDoc(t)
	video := newVideoFixture(doc)
	st0 := mediaStateFor(rt, video)
	st0.loadedSrc = "probe.mp4"
	mustRun(t, rt, `document.getElementById("player").setAttribute("src", "probe.mp4");`)
	st0.readyState = mediaHaveEnoughData
	st0.duration = 4
	st0.fps = 10
	st0.paused = false
	st0.clock = true
	// 先把「上一帧」定在第 5 帧。
	st0.currentTime = 0.51
	st0.maybePresentVideoFrame()
	drainEventLoop(rt)

	mustRun(t, rt, `window.__rc2 = [];
		document.getElementById("player").requestVideoFrameCallback(function(now, md){
			window.__rc2.push({kind:"fired", media: md.mediaTime});
		});`)
	// 同一帧内的另两个时刻：帧号都是 5 → 不回调。
	for _, t0 := range []float64{0.55, 0.59} {
		st0.currentTime = t0
		st0.maybePresentVideoFrame()
		drainEventLoop(rt)
	}
	if recs := rvfcRecordsKey(t, rt, "__rc2"); len(recs) != 0 {
		t.Fatalf("同一帧内的时刻推进不该触发 rVFC，got %+v", recs)
	}
	// 跨到第 6 帧（0.6）→ 触发。
	st0.currentTime = 0.6
	st0.maybePresentVideoFrame()
	drainEventLoop(rt)
	recs := rvfcRecordsKey(t, rt, "__rc2")
	if len(recs) != 1 || math.Abs(recs[0].Media-0.6) > 1e-9 {
		t.Fatalf("跨帧应触发一次且 mediaTime = 0.6，got %+v", recs)
	}
}

// rvfcRecords 读回 __rc 的记录。
func rvfcRecords(t *testing.T, rt *jsc.Interpreter) []frameCallbackRecord {
	t.Helper()
	return rvfcRecordsKey(t, rt, "__rc")
}

// rvfcRecordsKey 读回指定数组名的回调记录。
func rvfcRecordsKey(t *testing.T, rt *jsc.Interpreter, name string) []frameCallbackRecord {
	t.Helper()
	raw := evalStr(t, rt, `JSON.stringify(window.`+name+`)`)
	var out []frameCallbackRecord
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("解析 %s 失败: %v（原始值 %q）", name, err, raw)
	}
	return out
}
