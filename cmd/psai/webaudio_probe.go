package main

// TC-M-603 的探针自检：在**引擎真实 JS 环境**里验证 WebAudio 最小子集
//（`AudioContext` / `decodeAudioData`）。
//
// 为什么放在探针里（而不是只留单测）：TC-M-603 的判据是「API 存在且最小可用」，
// 而单测跑的是 Go 侧直接构造的解释器；探针跑的是**真实 WebView + 真实页面环境 +
// 真实宿主 ffmpeg**——它证明的是「页面脚本在 psai 宿主里真的拿得到 AudioContext、
// 真的能把一段音频字节解成 AudioBuffer」，与 `<img>`/`<video>` 的取证同一层次。
//
// 实现要点：
//   - 样本字节经 `Interpreter.ValueOf([]byte)` 注入全局（jsc 的 []byte → Uint8Array
//     转换），不拼 JS 源码字面量（44KB 的数组字面量既慢又易错）；
//   - 解码走**回调形态**（`decodeAudioData(buf, ok, err)`）：本引擎的回调是同步
//     调用的（没有独立音频线程），因此结果无需驱动微任务队列即可读回；Promise 形态
//     由单测覆盖（那里用 RunJobs 驱动）；
//   - 样本量纲的验证不只看类型：把 `getChannelData(0)` 的样本带回 Go 侧做 FFT
//     （复用判据 A 的同一套工具），主峰必须是样本频率——否则「解出了一堆 0」也会
//     被判成通过。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wb-ui/app"
	"wb-ui/webkit"
)

// webAudioEvidence 是一次 TC-M-603 自检的取证结果（全部为实测值，失败即如实记录）。
type webAudioEvidence struct {
	Available  bool    `json:"available"`   // typeof AudioContext === "function"
	AliasOK    bool    `json:"alias_ok"`    // webkitAudioContext 别名存在
	Sample     string  `json:"sample"`      // 自检用的样本文件名
	SampleRate float64 `json:"sample_rate"` // AudioContext.sampleRate
	State      string  `json:"state"`
	Bytes      float64 `json:"bytes"` // 交给 decodeAudioData 的字节数
	Decoded    bool    `json:"decoded"`
	BufRate    float64 `json:"buf_rate"`    // AudioBuffer.sampleRate
	BufLength  float64 `json:"buf_length"`  // AudioBuffer.length（帧）
	BufChans   float64 `json:"buf_chans"`   // AudioBuffer.numberOfChannels
	BufDur     float64 `json:"buf_duration"`
	Float32    bool    `json:"float32"` // getChannelData(0) instanceof Float32Array
	Peak       float64 `json:"peak"`    // 样本峰值（JS 侧扫描）
	DominantHz float64 `json:"dominant_hz"`
	DominantAmp float64 `json:"dominant_amp"`
	Err        string  `json:"err,omitempty"`
}

// webAudioProbeScript 在引擎里跑的自检脚本：返回一个纯数据对象（无函数），
// 由 Go 侧逐字段读回。
const webAudioProbeScript = `(function(){
  var out = { available: false, aliasOK: false };
  if (typeof AudioContext !== "function") return out;
  out.available = true;
  out.aliasOK = (typeof webkitAudioContext === "function");
  try {
    var ctx = new AudioContext();
    out.sampleRate = ctx.sampleRate;
    out.state = ctx.state;
    // 取注入的样本字节：优先读全局（Go 侧挂在解释器的全局对象上），window 前缀只作
    // 兜底——引擎里 window 是 bindings 注册的身份对象，未必就是全局对象本身。
    // 形态两种都接受：Uint8Array（有 .buffer）或 ArrayBuffer 本身——jsc 的
    // []byte → JS 转换在构造器调用失败时会退化为 ArrayBuffer（见
    // engine/js/jsc/bytes_value_test.go 钉住的实测形态），而 decodeAudioData 本就
    // 两者都收。
    var bytes = (typeof __wbAudioBytes !== "undefined") ? __wbAudioBytes : null;
    if (!bytes && typeof window !== "undefined" && window) { bytes = window.__wbAudioBytes; }
    var ab = null;
    if (bytes) {
      if (bytes.buffer) { ab = bytes.buffer; }
      else if (typeof bytes.byteLength === "number") { ab = bytes; }
    }
    if (!ab || !ab.byteLength) {
      out.err = "样本字节不可用（diag: type=" + (typeof bytes) + ", ctor=" +
        (bytes && bytes.constructor && bytes.constructor.name) + ", byteLength=" +
        (bytes ? bytes.byteLength : "n/a") + "）";
      return out;
    }
    out.bytes = ab.byteLength;
    var buf = null, err = null;
    ctx.decodeAudioData(ab, function(b){ buf = b; }, function(e){ err = e; });
    if (err) { out.err = (err.name || "Error") + ": " + (err.message || ""); return out; }
    if (!buf) { out.err = "decodeAudioData 的回调既未成功也未失败"; return out; }
    out.decoded = true;
    out.bufRate = buf.sampleRate;
    out.bufLength = buf.length;
    out.bufChans = buf.numberOfChannels;
    out.bufDuration = buf.duration;
    var ch = buf.getChannelData(0);
    out.float32 = (ch instanceof Float32Array);
    if (!ch) { out.err = "getChannelData(0) 返回空"; return out; }
    var peak = 0;
    for (var i = 0; i < ch.length; i++) {
      var a = ch[i] < 0 ? -ch[i] : ch[i];
      if (a > peak) peak = a;
    }
    out.peak = peak;
    out.samples = Array.prototype.slice.call(ch);
  } catch (e) {
    out.err = String(e && e.message ? e.message : e);
  }
  return out;
})()`

// probeWebAudio 在给定 WebView 的引擎里执行 TC-M-603 自检。wavPath 是样本文件
// （真音频字节：宿主 ffmpeg 解码路径必须用真数据才成立）。
func probeWebAudio(wv *webkit.WebView, wavPath string) webAudioEvidence {
	ev := webAudioEvidence{Sample: filepath.Base(wavPath)}
	interp := wv.JSInterpreter()
	if interp == nil {
		ev.Err = "无 JS 解释器"
		return ev
	}
	data, err := os.ReadFile(wavPath)
	if err != nil {
		ev.Err = fmt.Sprintf("读样本失败: %v", err)
		return ev
	}
	// 样本字节注入：走 jsc 的 []byte → Uint8Array 转换（不拼数组字面量）。
	interp.GlobalObject().Set("__wbAudioBytes", interp.ValueOf(data))
	res, err := interp.RunJS(webAudioProbeScript)
	if err != nil {
		ev.Err = fmt.Sprintf("自检脚本执行失败: %v", err)
		return ev
	}
	o := res.AsObject()
	if o == nil {
		ev.Err = "自检脚本未返回对象"
		return ev
	}
	num := func(k string) float64 {
		if v, ok := o.GetByKey(k); ok {
			return v.ToNumber()
		}
		return 0
	}
	boolean := func(k string) bool {
		if v, ok := o.GetByKey(k); ok {
			return v.ToBoolean()
		}
		return false
	}
	str := func(k string) string {
		if v, ok := o.GetByKey(k); ok {
			return v.ToString()
		}
		return ""
	}
	ev.Available = boolean("available")
	ev.AliasOK = boolean("aliasOK")
	ev.SampleRate = num("sampleRate")
	ev.State = str("state")
	ev.Bytes = num("bytes")
	ev.Decoded = boolean("decoded")
	ev.BufRate = num("bufRate")
	ev.BufLength = num("bufLength")
	ev.BufChans = num("bufChans")
	ev.BufDur = num("bufDuration")
	ev.Float32 = boolean("float32")
	ev.Peak = num("peak")
	ev.Err = str("err")

	// 样本带回来做 FFT：证明解出的**确实是那段音频**（而不只是「类型正确」）。
	if ev.Decoded && ev.Float32 {
		if sv, ok := o.GetByKey("samples"); ok {
			if arr, ok := sv.Export().([]interface{}); ok && len(arr) > 0 {
				mono := make([]float64, 0, len(arr))
				for _, item := range arr {
					if f, ok := item.(float64); ok {
						mono = append(mono, f)
					} else {
						mono = append(mono, 0)
					}
				}
				ev.DominantHz, ev.DominantAmp = app.DominantFrequency(mono, int(ev.BufRate))
			}
		}
	}
	return ev
}

// firstWAVSample 取自检用的 WAV 样本：优先用 manifest 里的确定性样本名
// （`sine-440-1s.wav`，440Hz 满幅正弦 ⇒ 主峰可判），缺失时退化为目录里第一个 .wav。
func firstWAVSample(samplesDir string) (string, bool) {
	p := filepath.Join(samplesDir, "sine-440-1s.wav")
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p, true
	}
	ents, err := os.ReadDir(samplesDir)
	if err != nil {
		return "", false
	}
	for _, e := range ents {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".wav") {
			return filepath.Join(samplesDir, e.Name()), true
		}
	}
	return "", false
}

// webAudioSection 生成报告小节。所有数字都来自实测；失败时只陈述测得的事实。
func webAudioSection(ev webAudioEvidence, sampleName string) string {
	var b strings.Builder
	b.WriteString("### TC-M-603：WebAudio 最小子集（`AudioContext` / `decodeAudioData`）\n\n")
	b.WriteString("> 范围：按 `audio-backend-proposal.md` §6/§7 的 **A3-3（最小子集）**——" +
		"**不含** AudioNode 图（GainNode/OscillatorNode/AnalyserNode…）、AudioParam 自动化、" +
		"AudioWorklet、OfflineAudioContext；那些要新建「音频图执行引擎 + 实时线程调度」" +
		"（≈3000–6000 行），属另立项的高风险项。\n\n")

	b.WriteString("| 项 | 实测 |\n|---|---|\n")
	fmt.Fprintf(&b, "| 特性检测 `typeof AudioContext === \"function\"` | %s |\n", mark(ev.Available))
	fmt.Fprintf(&b, "| 别名 `webkitAudioContext` | %s |\n", mark(ev.AliasOK))
	if ev.SampleRate > 0 {
		fmt.Fprintf(&b, "| `AudioContext.sampleRate` / `state` | %g / `%s` |\n", ev.SampleRate, ev.State)
	}
	if ev.Err != "" {
		fmt.Fprintf(&b, "| 端到端自检（样本 `%s`） | ❌ %s |\n", sampleName, ev.Err)
		b.WriteString("\n")
		b.WriteString("> ⚠️ 自检失败原因如实记录，未按通过处理。\n\n")
		return b.String()
	}
	fmt.Fprintf(&b, "| 输入字节（`%s`） | %g 字节 |\n", sampleName, ev.Bytes)
	fmt.Fprintf(&b, "| `decodeAudioData` → AudioBuffer | %s sampleRate=%g、声道=%g、length=%g 帧、duration=%.3fs |\n",
		mark(ev.Decoded), ev.BufRate, ev.BufChans, ev.BufLength, ev.BufDur)
	fmt.Fprintf(&b, "| `getChannelData(0) instanceof Float32Array` | %s |\n", mark(ev.Float32))
	fmt.Fprintf(&b, "| 样本峰值 | %.4f（样本自身幅度，非满幅） |\n", ev.Peak)
	fmt.Fprintf(&b, "| 频谱主峰（Go 侧 FFT，判据 A 同一套工具） | %.2f Hz（幅度 %.3f） |\n",
		ev.DominantHz, ev.DominantAmp)
	b.WriteString("\n")

	// 判定不看**绝对**幅度：样本 `sine-440-1s.wav` 本身就是低幅度正弦（实测峰值
	// 0.125 ≈ -18dBFS），要求「接近 1」是错的。看两件真正说明问题的事：有信号
	//（峰值明显 > 0）、且频谱主峰落在样本频率的容差内（FFT 分辨率 ≈2.7Hz）。
	ok := ev.Available && ev.Decoded && ev.Float32 && ev.Peak > 0.001 &&
		ev.DominantHz > 435 && ev.DominantHz < 445
	if ok {
		b.WriteString("**判定：✅ 达成**——页面脚本在真实宿主环境里拿到 `AudioContext`，" +
			"并把一段真音频字节解成 `AudioBuffer`（样本量纲经 FFT 复核为 440Hz 正弦）。\n\n")
	} else {
		b.WriteString("**判定：❌ 未达成**——见上表实测值（不按通过处理）。\n\n")
	}
	return b.String()
}
