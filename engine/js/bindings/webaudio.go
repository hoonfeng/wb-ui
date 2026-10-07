// Package bindings — WebAudio 最小子集（TC-M-603）：AudioContext /
// decodeAudioData / AudioBuffer。
//
// 范围（docs/audio-backend-proposal.md §6「不做」+ §7 的 A3-3 期）：只交付
// **最小可用面**，**不承诺**完整音频图——
//
//   - 做：全局 `AudioContext` 构造器（`typeof AudioContext !== 'undefined'` 特性检测
//     可过；含旧前缀名 `webkitAudioContext`）、`sampleRate` / `currentTime` / `state` /
//     `destination`、`resume()` / `suspend()` / `close()`、`createBuffer()`、
//     `decodeAudioData()`（Promise + 旧式回调双形态）、`AudioBuffer`
//     （sampleRate / length / duration / numberOfChannels / getChannelData）；
//   - 不做：AudioNode 图（GainNode / OscillatorNode / AnalyserNode / …）、AudioParam
//     自动化、AudioWorklet、OfflineAudioContext、MediaElementAudioSourceNode ——
//     它们要求「音频图执行引擎 + 实时线程调度」，是另立项的高风险项
//     （量级 ≈3000–6000 行，见 §9.6 侦查结论）。
//
// 分工与 A3 音频链路同一条路线 (a)：**引擎不背解码器**——宿主注入 `AudioDecoder`
// （app/webaudio.go 用 ffmpeg 解 f32le PCM），本层只做「对象模型 + 参数校验 +
// Promise 形态」。
//
// ★ 诚实边界：宿主未装配解码器时 `decodeAudioData` **reject**（EncodingError），
// 不返回编造的空 buffer——「API 存在」不等于「功能假装可用」（docs 的证据纪律）。
package bindings

import (
	"fmt"
	"sync"
	"time"

	"wb-ui/engine/js/jsc"
)

// ─── 宿主注入点 ───────────────────────────────────────────

// AudioDecoded 是宿主解码出的 PCM（即 AudioBuffer 的内容）：每声道一份 float32
// 样本，取值域 [-1, 1]（与 AudioBuffer 的规范量纲一致）。
type AudioDecoded struct {
	// SampleRate 是解码后音频的采样率。取**源**采样率而非 context.sampleRate
	// ——与 Chromium 的 decodeAudioData 行为一致：重采样是播放期（AudioContext
	// 输出）的职责，不属于解码。
	SampleRate int
	// Channels 是各声道的样本（长度 = 声道数，至少 1；各声道等长）。
	Channels [][]float32
}

// AudioDecoder 由宿主设置：把内存中的媒体字节解码为 PCM。ok=false 表示解码失败
// （不可识别的容器/编码、ffmpeg 不可用等）——decodeAudioData 的 Promise 因此以
// EncodingError reject，与浏览器对不可解码数据的行为一致。
//
// 未设置（nil）时 decodeAudioData 一律 reject：引擎侧没有解码器，不编造数据。
var AudioDecoder func(data []byte) (AudioDecoded, bool)

// defaultAudioContextSampleRate 是 AudioContext 未显式指定 sampleRate 时的默认值。
// 48kHz 与宿主 PCM 通道（app/mediaaudio.go 的 audioPCMSampleRate）同档，也是多数
// 声卡的输出档位。
const defaultAudioContextSampleRate = 48000

// ─── 安装 ─────────────────────────────────────────────────

// installWebAudio 把 AudioContext 挂到全局（幂等）。由 dom.go 的装配段在 DOM 接口
// 构造器族注册完成后调用——与 Audio/Option 同一时机（见 registerExtraElementCtors）。
func installWebAudio(rt *jsc.Interpreter, g *jsc.JSObject) {
	if rt == nil || g == nil {
		return
	}
	if _, ok := g.GetByKey("AudioContext"); ok {
		return
	}
	ctor := rt.NewConstructor("AudioContext", func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
		return newAudioContext(in, args)
	})
	g.Set("AudioContext", jsc.FunctionValue(ctor))
	// webkitAudioContext 是旧版 Safari 前缀名，部分音视频/可视化库仍做兼容探测。
	if _, ok := g.GetByKey("webkitAudioContext"); !ok {
		g.Set("webkitAudioContext", jsc.FunctionValue(ctor))
	}
}

// ─── AudioContext ─────────────────────────────────────────

// audioClock 是 AudioContext 的时钟与生命周期状态（规范：currentTime 只增不减；
// suspend 期间冻结；close 后停止）。
type audioClock struct {
	mu sync.Mutex

	sampleRate   int
	closed       bool
	suspended    bool
	accumulated  time.Duration
	runningSince time.Time
}

func newAudioClock(sampleRate int) *audioClock {
	return &audioClock{sampleRate: sampleRate, runningSince: time.Now()}
}

// now 返回上下文时钟（秒）：运行中按挂钟推进，suspend/close 后冻结在累计值。
func (c *audioClock) now() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.accumulated
	if !c.suspended && !c.closed && !c.runningSince.IsZero() {
		d += time.Since(c.runningSince)
	}
	return d.Seconds()
}

func (c *audioClock) suspend() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.suspended {
		return
	}
	c.accumulated += time.Since(c.runningSince)
	c.runningSince = time.Time{}
	c.suspended = true
}

func (c *audioClock) resume() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || !c.suspended {
		return
	}
	c.suspended = false
	c.runningSince = time.Now()
}

func (c *audioClock) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if !c.suspended && !c.runningSince.IsZero() {
		c.accumulated += time.Since(c.runningSince)
	}
	c.runningSince = time.Time{}
	c.closed = true
}

// stateName 返回规范的三态之一（running / suspended / closed）。
func (c *audioClock) stateName() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case c.closed:
		return "closed"
	case c.suspended:
		return "suspended"
	}
	return "running"
}

// newAudioContext 实现 `new AudioContext(options)`。
func newAudioContext(rt *jsc.Interpreter, args []jsc.JSValue) *jsc.JSObject {
	if rt == nil {
		return nil
	}
	sampleRate := defaultAudioContextSampleRate
	if len(args) >= 1 && args[0].IsObject() {
		if o := args[0].AsObject(); o != nil {
			if v, ok := o.GetByKey("sampleRate"); ok && v.IsNumber() {
				if r := int(v.ToNumber()); r > 0 {
					sampleRate = r
				}
			}
		}
	}
	clock := newAudioClock(sampleRate)
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.SetClassName("AudioContext")
	obj.Set("sampleRate", jsc.NumberValue(float64(sampleRate)))
	// currentTime / state 必须是访问器：每次读都反映当下状态（不是快照字段）。
	obj.SetAccessor("currentTime", func(in *jsc.Interpreter) jsc.JSValue {
		return jsc.NumberValue(clock.now())
	}, nil)
	obj.SetAccessor("state", func(in *jsc.Interpreter) jsc.JSValue {
		return jsc.StringValue(clock.stateName())
	}, nil)
	// ★ 最小子集没有音频图，也就没有真实的输出处理路径：两个延迟如实报 0，
	//   不编造设备参数（规范里它们是设备相关量）。脚本普遍只把它们当数值读。
	obj.Set("baseLatency", jsc.NumberValue(0))
	obj.Set("outputLatency", jsc.NumberValue(0))
	obj.Set("destination", jsc.ObjectValue(newAudioDestination(rt)))

	obj.Set("resume", jsc.FunctionValue(rt.NewNativeFunction("resume",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			clock.resume()
			return in.ResolvePromise(jsc.Undefined())
		}, 0)))
	obj.Set("suspend", jsc.FunctionValue(rt.NewNativeFunction("suspend",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			clock.suspend()
			return in.ResolvePromise(jsc.Undefined())
		}, 0)))
	obj.Set("close", jsc.FunctionValue(rt.NewNativeFunction("close",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			clock.close()
			return in.ResolvePromise(jsc.Undefined())
		}, 0)))
	obj.Set("createBuffer", jsc.FunctionValue(rt.NewNativeFunction("createBuffer",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			return jsc.ObjectValue(createAudioBuffer(in, args))
		}, 3)))
	obj.Set("decodeAudioData", jsc.FunctionValue(rt.NewNativeFunction("decodeAudioData",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			return decodeAudioDataFor(in, args)
		}, 1)))
	return obj
}

// newAudioDestination 返回 destination 的最小等价对象：它不是音频图节点（最小子集
// 不建图），只承载「上下文有输出端」的存在性与常见的只读属性。
func newAudioDestination(rt *jsc.Interpreter) *jsc.JSObject {
	o := jsc.NewObject(rt.ObjectPrototype())
	o.SetClassName("AudioDestinationNode")
	o.Set("maxChannelCount", jsc.NumberValue(2))
	o.Set("channelCount", jsc.NumberValue(2))
	o.Set("numberOfInputs", jsc.NumberValue(1))
	o.Set("numberOfOutputs", jsc.NumberValue(0))
	return o
}

// ─── decodeAudioData ──────────────────────────────────────

// decodeAudioDataFor 实现 `ctx.decodeAudioData(audioData[, successCallback[, errorCallback]])`。
//
// 参数校验与失败语义按规范：参数不是 ArrayBuffer/TypedArray → TypeError；数据无法
// 解码或为空 → EncodingError。两种失败都同时驱动 Promise 与（若给了）errorCallback。
//
// ★ 回调是**同步**调用的：本引擎没有独立音频线程，解码本身同步完成（宿主 ffmpeg
// 一次调用），因此「先决议 Promise、再调回调」即可，不额外排宏任务队列。
func decodeAudioDataFor(in *jsc.Interpreter, args []jsc.JSValue) jsc.JSValue {
	var onOk, onErr jsc.JSValue
	if len(args) >= 2 && args[1].IsFunction() {
		onOk = args[1]
	}
	if len(args) >= 3 && args[2].IsFunction() {
		onErr = args[2]
	}
	fail := func(name, msg string) jsc.JSValue {
		exc := newDOMExceptionValue(in, name, msg)
		if !onErr.IsUndefined() {
			_, _ = in.Call(onErr, jsc.Undefined(), []jsc.JSValue{exc})
		}
		return in.RejectPromise(exc)
	}

	if len(args) == 0 || !args[0].IsObject() {
		return fail("TypeError", "Failed to execute 'decodeAudioData' on "+
			"'BaseAudioContext': parameter 1 is not of type 'ArrayBuffer'.")
	}
	data, ok := in.ArrayBufferBytes(args[0])
	if !ok {
		return fail("TypeError", "Failed to execute 'decodeAudioData' on "+
			"'BaseAudioContext': 参数 1 既不是 ArrayBuffer 也不是 TypedArray。")
	}
	if len(data) == 0 {
		return fail("EncodingError", "Failed to execute 'decodeAudioData' on "+
			"'BaseAudioContext': 音频数据为空。")
	}
	if AudioDecoder == nil {
		return fail("EncodingError", "Failed to execute 'decodeAudioData' on "+
			"'BaseAudioContext': 宿主未装配 WebAudio 解码器（app.InstallWebAudio）。")
	}
	dec, ok := AudioDecoder(data)
	if !ok || len(dec.Channels) == 0 || len(dec.Channels[0]) == 0 {
		return fail("EncodingError", "Failed to execute 'decodeAudioData' on "+
			"'BaseAudioContext': 无法解码为可用的音频数据。")
	}
	buf := newAudioBuffer(in, dec)
	val := jsc.ObjectValue(buf)
	if !onOk.IsUndefined() {
		_, _ = in.Call(onOk, jsc.Undefined(), []jsc.JSValue{val})
	}
	return in.ResolvePromise(val)
}

// ─── AudioBuffer ──────────────────────────────────────────

// createAudioBuffer 实现 `ctx.createBuffer(numberOfChannels, length, sampleRate)`：
// 返回一段**静音**（全 0 样本）的 AudioBuffer，供脚本先分配再填充。
func createAudioBuffer(in *jsc.Interpreter, args []jsc.JSValue) *jsc.JSObject {
	if len(args) < 3 {
		panic(in.VM().NewTypeError("Failed to execute 'createBuffer' on " +
			"'BaseAudioContext': 需要 3 个参数（numberOfChannels, length, sampleRate）。"))
	}
	ch := int(args[0].ToNumber())
	length := int(args[1].ToNumber())
	rate := int(args[2].ToNumber())
	if ch < 1 {
		panic(in.VM().NewTypeError(fmt.Sprintf(
			"Failed to execute 'createBuffer' on 'BaseAudioContext': numberOfChannels 必须 ≥ 1（收到 %d）。", ch)))
	}
	if length < 1 {
		panic(in.VM().NewTypeError(fmt.Sprintf(
			"Failed to execute 'createBuffer' on 'BaseAudioContext': length 必须 ≥ 1（收到 %d）。", length)))
	}
	if rate < 1 {
		panic(in.VM().NewTypeError(fmt.Sprintf(
			"Failed to execute 'createBuffer' on 'BaseAudioContext': sampleRate 必须 ≥ 1（收到 %d）。", rate)))
	}
	dec := AudioDecoded{SampleRate: rate, Channels: make([][]float32, ch)}
	for i := range dec.Channels {
		dec.Channels[i] = make([]float32, length)
	}
	return newAudioBuffer(in, dec)
}

// newAudioBuffer 构造 AudioBuffer 等价对象（decodeAudioData 与 createBuffer 共用）。
func newAudioBuffer(rt *jsc.Interpreter, dec AudioDecoded) *jsc.JSObject {
	channels := dec.Channels
	rate := dec.SampleRate
	if rate <= 0 {
		rate = defaultAudioContextSampleRate
	}
	length := 0
	if len(channels) > 0 {
		length = len(channels[0])
	}
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.SetClassName("AudioBuffer")
	obj.Set("sampleRate", jsc.NumberValue(float64(rate)))
	obj.Set("length", jsc.NumberValue(float64(length)))
	obj.Set("numberOfChannels", jsc.NumberValue(float64(len(channels))))
	obj.Set("duration", jsc.NumberValue(float64(length)/float64(rate)))
	obj.Set("getChannelData", jsc.FunctionValue(rt.NewNativeFunction("getChannelData",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			idx := 0
			if len(args) >= 1 {
				idx = int(args[0].ToNumber())
			}
			if idx < 0 || idx >= len(channels) {
				// 规范：越界索引抛 IndexSizeError（本引擎没有 DOMException 构造器，
				// 与其它绑定一致用 TypeError 承载，消息里说明实际语义）。
				panic(in.VM().NewTypeError(fmt.Sprintf(
					"Failed to execute 'getChannelData' on 'AudioBuffer': 声道索引 %d 越界（numberOfChannels=%d）。",
					idx, len(channels))))
			}
			return in.Float32ArrayValue(channels[idx])
		}, 1)))
	return obj
}
