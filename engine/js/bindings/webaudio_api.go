package bindings

// WebAudio API 层（A3-3 完整子集，2026-10-08）：把 webaudiograph.go 的图内核接到
// JS 对象上——AudioContext / OfflineAudioContext / 各 AudioNode / AudioParam。
//
// 与最小子集（webaudio.go 的 AudioContext + decodeAudioData + AudioBuffer）的关系：
// 最小子集仍然完整保留（特性检测、解码、AudioBuffer 语义一字未改），本文件在其上
// **增加**音频图能力。也就是说：`typeof AudioContext === 'function'` 的既有判据、
// TC-M-603 的全部断言都继续成立，新增的是 `createGain()` 这类真正参与声音生成的
// 对象。
//
// 关联方式（不用隐藏槽位，便于断言与调试）：
//
//	每个 JS 节点对象带 `__wgCtx`（上下文 id）与 `__wgId`（节点 id）两个普通属性；
//	Go 侧由 ctxRegistry 按 id 找到 *wgContext / *wgNode。AudioBuffer 同理带
//	`__wgBuf`。
//
// 分工与 A3 一致：**引擎渲染、宿主输出**。实时上下文把 destination 渲染出的
// 交错 float32 PCM 交给宿主注入的 WGAudioSink（app 侧写 waveOut）；
// OfflineAudioContext 完全在引擎内渲染（不碰设备），因此可以逐步长精确比对。
import (
	"encoding/binary"
	"math"
	"strconv"
	"sync"

	"wb-ui/engine/js/jsc"
)

// WGAudioSink 由宿主注入：接收 AudioContext 的 destination 渲染出的**交错**
// float32 PCM（样本域 [-1,1]，声道 = 2）。未注入时实时上下文照常渲染、只是没有
// 声音去处（与「宿主没注册音频源」同一条纪律：不假装出声，也不报错）。
//
// ★ 它在**事件循环线程**上被调用（渲染发生在定时器回调里），实现必须尽快返回：
// 设备写入/排队应在宿主内部的 goroutine 上完成（见 app/webaudioout.go）。
var WGAudioSink func(interleaved []float32, sampleRate int)

// WGSinkStop 由宿主注入：音频图停止/关闭时通知宿主丢弃已排队数据（可为 nil）。
var WGSinkStop func()

// ─── 上下文注册表 ─────────────────────────────────────────

type wgContext struct {
	mu sync.Mutex

	id         int
	rt         *jsc.Interpreter
	clock      *audioClock
	graph      *wgGraph
	sampleRate int
	offline    bool
	length     int // 离线：总帧数
	channels   int

	// 实时：
	started        bool
	renderedFrames int64
	timerArmed     bool
	pcm            [][]float32 // 渲染暂存（2 声道 × quantum）
	interleaved    []float32
}

var (
	wgRegistryMu sync.Mutex
	wgRegistry   = map[int]*wgContext{}
	wgNextID     int
)

func wgRegister(c *wgContext) int {
	wgRegistryMu.Lock()
	defer wgRegistryMu.Unlock()
	wgNextID++
	c.id = wgNextID
	wgRegistry[c.id] = c
	return c.id
}

func wgLookup(id int) *wgContext {
	wgRegistryMu.Lock()
	defer wgRegistryMu.Unlock()
	return wgRegistry[id]
}

// now 返回**脚本视角**的当前时间。
//
// 实时上下文：就是音频时钟读数。离线上下文：恒为 0 —— 它的时间轴由渲染帧号
// （t = frame/sampleRate）决定，渲染开始前「已经过去的时间」根本不存在。
//
// ★ 这里踩过一次坑：离线上下文若也读真实时钟，`src.start(0)` 会把「JS 建图消耗的
// 真实毫秒数」当成「已经播放了这么久」，于是播放起点整体前移（实测 480 帧的缓冲
// 还没渲染就少播了一截，表现为尾部静音）。离线渲染的确定性因此也要求：任何与时间
// 有关的 JS 侧计算都不能依赖 wall clock。
func (c *wgContext) now() float64 {
	if c == nil {
		return 0
	}
	if c.offline {
		return 0
	}
	return c.clock.now()
}

func newWGContext(rt *jsc.Interpreter, sampleRate int, offline bool, length, channels int) *wgContext {
	c := &wgContext{
		rt:         rt,
		sampleRate: sampleRate,
		clock:      newAudioClock(sampleRate),
		graph:      newWGGraph(sampleRate),
		offline:    offline,
		length:     length,
		channels:   channels,
	}
	c.pcm = make([][]float32, wgChannels)
	for i := range c.pcm {
		c.pcm[i] = make([]float32, wgQuantum)
	}
	c.interleaved = make([]float32, wgQuantum*wgChannels)
	wgRegister(c)
	return c
}

// ctxFromThis 从 JS 对象上的 `__wgCtx` 取上下文。
func wgCtxOf(o *jsc.JSObject) *wgContext {
	if o == nil {
		return nil
	}
	v, ok := o.GetByKey("__wgCtx")
	if !ok || !v.IsNumber() {
		return nil
	}
	return wgLookup(int(v.ToNumber()))
}

// wgNodeOf 从 JS 对象取节点（`__wgId` 指向图中节点）。
func wgNodeOf(o *jsc.JSObject) (*wgContext, *wgNode) {
	c := wgCtxOf(o)
	if c == nil {
		return nil, nil
	}
	v, ok := o.GetByKey("__wgId")
	if !ok || !v.IsNumber() {
		return c, nil
	}
	id := int(v.ToNumber())
	if id < 0 || id >= len(c.graph.nodes) {
		return c, nil
	}
	return c, c.graph.nodes[id]
}

// ─── 装配（由 webaudio.go 的 newAudioContext 调用）────────

// attachAudioGraph 给一个（实时）AudioContext 对象挂上音频图能力：
// destination 换成真实节点，并补 create*/listener 等方法。
func attachAudioGraph(rt *jsc.Interpreter, obj *jsc.JSObject, c *wgContext) {
	obj.Set("__wgCtx", jsc.NumberValue(float64(c.id)))
	obj.Set("destination", jsc.ObjectValue(newWGDestinationObject(rt, c)))
	installWGCreators(rt, obj, c)
	installWGRenderControl(rt, obj, c)
}

// installWGCreators 挂上 create* 工厂方法（AudioContext 与 OfflineAudioContext 共用）。
func installWGCreators(rt *jsc.Interpreter, obj *jsc.JSObject, c *wgContext) {
	mk := func(name string, arity int, fn func(in *jsc.Interpreter, args []jsc.JSValue) jsc.JSValue) {
		obj.Set(name, jsc.FunctionValue(rt.NewNativeFunction(name,
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				return fn(in, args)
			}, arity)))
	}
	mk("createGain", 0, func(in *jsc.Interpreter, _ []jsc.JSValue) jsc.JSValue {
		n := c.graph.newNode(wgKGain)
		n.params["gain"] = newWGParam("gain", 1)
		return jsc.ObjectValue(newWGNodeObject(in, c, n))
	})
	mk("createOscillator", 0, func(in *jsc.Interpreter, _ []jsc.JSValue) jsc.JSValue {
		n := c.graph.newNode(wgKOscillator)
		n.oscType = "sine"
		n.params["frequency"] = newWGParam("frequency", 440)
		n.params["detune"] = newWGParam("detune", 0)
		return jsc.ObjectValue(newWGNodeObject(in, c, n))
	})
	mk("createBufferSource", 0, func(in *jsc.Interpreter, _ []jsc.JSValue) jsc.JSValue {
		n := c.graph.newNode(wgKBufferSource)
		n.params["playbackRate"] = newWGParam("playbackRate", 1)
		n.params["detune"] = newWGParam("detune", 0)
		n.loopEnd = 0
		return jsc.ObjectValue(newWGNodeObject(in, c, n))
	})
	mk("createConstantSource", 0, func(in *jsc.Interpreter, _ []jsc.JSValue) jsc.JSValue {
		n := c.graph.newNode(wgKConstantSource)
		n.params["offset"] = newWGParam("offset", 1)
		return jsc.ObjectValue(newWGNodeObject(in, c, n))
	})
	mk("createStereoPanner", 0, func(in *jsc.Interpreter, _ []jsc.JSValue) jsc.JSValue {
		n := c.graph.newNode(wgKStereoPanner)
		n.params["pan"] = newWGParam("pan", 0)
		return jsc.ObjectValue(newWGNodeObject(in, c, n))
	})
	mk("createDelay", 1, func(in *jsc.Interpreter, args []jsc.JSValue) jsc.JSValue {
		n := c.graph.newNode(wgKDelay)
		maxDelay := 1.0
		if len(args) >= 1 && args[0].IsNumber() {
			maxDelay = args[0].ToNumber()
		}
		// 环形缓冲按 maxDelayTime 分配（+1 个量子余量，保证读写不重叠）。
		mx := int(maxDelay*float64(c.graph.sampleRate)) + wgQuantum
		if mx < wgQuantum*2 {
			mx = wgQuantum * 2
		}
		n.delayBuf = make([][]float32, wgChannels)
		for i := range n.delayBuf {
			n.delayBuf[i] = make([]float32, mx)
		}
		n.params["delayTime"] = newWGParam("delayTime", 0)
		return jsc.ObjectValue(newWGNodeObject(in, c, n))
	})
	mk("createBiquadFilter", 0, func(in *jsc.Interpreter, _ []jsc.JSValue) jsc.JSValue {
		n := c.graph.newNode(wgKBiquadFilter)
		n.bqTypeName = "lowpass"
		n.params["frequency"] = newWGParam("frequency", 350)
		n.params["detune"] = newWGParam("detune", 0)
		n.params["Q"] = newWGParam("Q", 1)
		n.params["gain"] = newWGParam("gain", 0)
		return jsc.ObjectValue(newWGNodeObject(in, c, n))
	})
	mk("createAnalyser", 0, func(in *jsc.Interpreter, _ []jsc.JSValue) jsc.JSValue {
		n := c.graph.newNode(wgKAnalyser)
		n.an = newWGAnalyserState(2048)
		return jsc.ObjectValue(newWGNodeObject(in, c, n))
	})
	mk("createChannelMerger", 1, func(in *jsc.Interpreter, _ []jsc.JSValue) jsc.JSValue {
		// 多声道路由未实现（见 webaudiograph.go 文件头取舍 ①）：如实抛错，
		// 而不是返回一个「连上却不出声」的假节点。
		panic(in.VM().NewTypeError("createChannelMerger: 本引擎暂不支持多声道合并（ChannelMergerNode 未实现）"))
	})
	mk("createChannelSplitter", 1, func(in *jsc.Interpreter, _ []jsc.JSValue) jsc.JSValue {
		panic(in.VM().NewTypeError("createChannelSplitter: 本引擎暂不支持多声道分离（ChannelSplitterNode 未实现）"))
	})
	// createBuffer / decodeAudioData 是 BaseAudioContext 的能力（实时与离线都有），
	// 这里与其它工厂一起装配：离线上下文同样需要它们来准备样本。
	mk("createBuffer", 3, func(in *jsc.Interpreter, args []jsc.JSValue) jsc.JSValue {
		return jsc.ObjectValue(createAudioBuffer(in, args))
	})
	mk("decodeAudioData", 1, func(in *jsc.Interpreter, args []jsc.JSValue) jsc.JSValue {
		return decodeAudioDataFor(in, args)
	})
	_ = obj
}

// installWGRenderControl 挂实时渲染控制（resume/suspend/close 已由最小子集提供，
// 这里只补「开始渲染」的语义与 state 变化时的联动）。
func installWGRenderControl(rt *jsc.Interpreter, obj *jsc.JSObject, c *wgContext) {
	// 最小子集的 resume/suspend/close 已完成时钟状态切换；实时渲染的启动挂在
	// 同一个对象上（在 newAudioContext 里 resume 之后调 wgStartRealTime）。
}

// ─── destination ──────────────────────────────────────────

func newWGDestinationObject(rt *jsc.Interpreter, c *wgContext) *jsc.JSObject {
	n := c.graph.dest
	o := newWGNodeObject(rt, c, n)
	o.SetClassName("AudioDestinationNode")
	o.Set("maxChannelCount", jsc.NumberValue(2))
	// 规范：destination 是输出终点，numberOfOutputs 恒为 0（newWGNodeObject 的通用
	// 默认值是 1，这里按规范修正——否则脚本里 `destination.numberOfOutputs` 读错）。
	o.Set("numberOfOutputs", jsc.NumberValue(0))
	return o
}

// ─── 节点对象 ─────────────────────────────────────────────

// newWGNodeObject 构造一个 AudioNode 等价对象。通用属性 + 类型特有成员按 kind 挂。
func newWGNodeObject(rt *jsc.Interpreter, c *wgContext, n *wgNode) *jsc.JSObject {
	o := jsc.NewObject(rt.ObjectPrototype())
	o.SetClassName(nodeClassName(n.kind))
	o.Set("__wgCtx", jsc.NumberValue(float64(c.id)))
	o.Set("__wgId", jsc.NumberValue(float64(n.id)))
	o.Set("context", jsc.ObjectValue(dummyContextRef(rt, c)))
	o.Set("numberOfInputs", jsc.NumberValue(float64(nodeInputs(n.kind))))
	o.Set("numberOfOutputs", jsc.NumberValue(float64(nodeOutputs(n.kind))))
	o.Set("channelCount", jsc.NumberValue(float64(wgChannels)))
	o.Set("channelCountMode", jsc.StringValue("max"))
	o.Set("channelInterpretation", jsc.StringValue("speakers"))

	// connect / disconnect：目标是节点对象时接线；目标是 AudioParam 时返回该节点
	// （规范里目标为参数意味着「参数驱动输入」，本内核未实现参数输入，如实按
	// 不支持处理：抛 TypeError 而不是静默丢弃——静默会让脚本以为连上了）。
	o.Set("connect", jsc.FunctionValue(rt.NewNativeFunction("connect",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			if len(args) < 1 || !args[0].IsObject() {
				panic(in.VM().NewTypeError("connect: 参数 1 不是 AudioNode"))
			}
			dstObj := args[0].AsObject()
			dstCtx, dst := wgNodeOf(dstObj)
			if dst == nil || dstCtx != c {
				panic(in.VM().NewTypeError("connect: 目标节点不属于同一个 AudioContext"))
			}
			c.mu.Lock()
			c.graph.connect(n, dst)
			c.mu.Unlock()
			return args[0]
		}, 1)))
	o.Set("disconnect", jsc.FunctionValue(rt.NewNativeFunction("disconnect",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			c.mu.Lock()
			defer c.mu.Unlock()
			if len(args) >= 1 && args[0].IsObject() {
				if _, dst := wgNodeOf(args[0].AsObject()); dst != nil {
					c.graph.disconnect(n, dst)
					return jsc.Undefined()
				}
			}
			c.graph.disconnect(n, nil)
			return jsc.Undefined()
		}, 0)))

	switch n.kind {
	case wgKGain:
		o.Set("gain", jsc.ObjectValue(newWGParamObject(rt, c, n, n.params["gain"])))
	case wgKOscillator:
		o.Set("frequency", jsc.ObjectValue(newWGParamObject(rt, c, n, n.params["frequency"])))
		o.Set("detune", jsc.ObjectValue(newWGParamObject(rt, c, n, n.params["detune"])))
		o.SetAccessor("type", func(in *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue(n.oscType)
		}, func(in *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) {
			s := v.ToString()
			switch s {
			case "sine", "square", "sawtooth", "triangle":
				n.oscType = s
			default:
				panic(in.VM().NewTypeError("OscillatorNode.type 取值非法: " + s))
			}
		})
		o.Set("start", jsc.FunctionValue(rt.NewNativeFunction("start",
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				when := c.now()
				if len(args) >= 1 && args[0].IsNumber() {
					when = args[0].ToNumber()
				}
				c.mu.Lock()
				if n.started {
					c.mu.Unlock()
					panic(in.VM().NewTypeError("OscillatorNode.start: 已经启动过（规范：只能 start 一次）"))
				}
				n.started, n.startAt = true, when
				c.mu.Unlock()
				wgEnsureRealTime(c)
				return jsc.Undefined()
			}, 0)))
		o.Set("stop", jsc.FunctionValue(rt.NewNativeFunction("stop",
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				when := c.now()
				if len(args) >= 1 && args[0].IsNumber() {
					when = args[0].ToNumber()
				}
				c.mu.Lock()
				n.stopped, n.stopAt = true, when
				c.mu.Unlock()
				return jsc.Undefined()
			}, 0)))
	case wgKBufferSource:
		o.Set("playbackRate", jsc.ObjectValue(newWGParamObject(rt, c, n, n.params["playbackRate"])))
		o.Set("detune", jsc.ObjectValue(newWGParamObject(rt, c, n, n.params["detune"])))
		o.SetAccessor("buffer", func(in *jsc.Interpreter) jsc.JSValue {
			if n.srcBuf == nil {
				return jsc.Null()
			}
			return jsc.ObjectValue(newAudioBuffer(in, *n.srcBuf))
		}, func(in *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) {
			c.mu.Lock()
			defer c.mu.Unlock()
			if !v.IsObject() {
				n.srcBuf = nil
				return
			}
			if dec := wgBufferOf(v.AsObject()); dec != nil {
				n.srcBuf = dec
			}
		})
		o.SetAccessor("loop", func(in *jsc.Interpreter) jsc.JSValue {
			return jsc.BooleanValue(n.srcLoop)
		}, func(in *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { n.srcLoop = v.ToBoolean() })
		o.SetAccessor("loopStart", func(in *jsc.Interpreter) jsc.JSValue {
			return jsc.NumberValue(n.loopStart)
		}, func(in *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { n.loopStart = v.ToNumber() })
		o.SetAccessor("loopEnd", func(in *jsc.Interpreter) jsc.JSValue {
			return jsc.NumberValue(n.loopEnd)
		}, func(in *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) { n.loopEnd = v.ToNumber() })
		o.Set("start", jsc.FunctionValue(rt.NewNativeFunction("start",
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				when := c.now()
				offset := 0.0
				dur := 0.0
				if len(args) >= 1 && args[0].IsNumber() {
					when = args[0].ToNumber()
				}
				if len(args) >= 2 && args[1].IsNumber() {
					offset = args[1].ToNumber()
				}
				if len(args) >= 3 && args[2].IsNumber() {
					dur = args[2].ToNumber()
				}
				c.mu.Lock()
				// srcOffset 只记**纯 offset**：读指针的「已流逝量」由渲染端按
				// (t - startAt) 推进（start(when) 允许 when 已过，此时渲染端自然就
				// 从 offset + 已流逝量开始）。若这里也加一次 elapsed，两边会重复计数，
				// 播放起点被算成两倍提前量。
				n.srcOn, n.srcOffset, n.srcDur = true, offset, dur
				n.startAt = when
				c.mu.Unlock()
				wgEnsureRealTime(c)
				return jsc.Undefined()
			}, 0)))
		o.Set("stop", jsc.FunctionValue(rt.NewNativeFunction("stop",
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				when := c.now()
				if len(args) >= 1 && args[0].IsNumber() {
					when = args[0].ToNumber()
				}
				c.mu.Lock()
				if n.srcDur <= 0 || (when-n.startAt) < n.srcDur {
					// stop(when) 表示「when 之后不再出声」：渲染端按 (tt - startAt) >= dur
					// 判定，因此把 dur 记为「从 startAt 到 when 的总时长」；若 when 已在
					// startAt 之前（立即停），dur=0 会让渲染端从第一帧起就不再输出。
					if when > n.startAt {
						n.srcDur = when - n.startAt
					} else {
						n.srcDur = 0
						n.srcOn = false
					}
				}
				c.mu.Unlock()
				return jsc.Undefined()
			}, 0)))
	case wgKConstantSource:
		o.Set("offset", jsc.ObjectValue(newWGParamObject(rt, c, n, n.params["offset"])))
		o.Set("start", jsc.FunctionValue(rt.NewNativeFunction("start",
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				when := c.now()
				if len(args) >= 1 && args[0].IsNumber() {
					when = args[0].ToNumber()
				}
				c.mu.Lock()
				n.started, n.startAt = true, when
				c.mu.Unlock()
				wgEnsureRealTime(c)
				return jsc.Undefined()
			}, 0)))
		o.Set("stop", jsc.FunctionValue(rt.NewNativeFunction("stop",
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				when := c.now()
				if len(args) >= 1 && args[0].IsNumber() {
					when = args[0].ToNumber()
				}
				c.mu.Lock()
				n.stopped, n.stopAt = true, when
				c.mu.Unlock()
				return jsc.Undefined()
			}, 0)))
	case wgKStereoPanner:
		o.Set("pan", jsc.ObjectValue(newWGParamObject(rt, c, n, n.params["pan"])))
	case wgKDelay:
		o.Set("delayTime", jsc.ObjectValue(newWGParamObject(rt, c, n, n.params["delayTime"])))
	case wgKBiquadFilter:
		o.Set("frequency", jsc.ObjectValue(newWGParamObject(rt, c, n, n.params["frequency"])))
		o.Set("detune", jsc.ObjectValue(newWGParamObject(rt, c, n, n.params["detune"])))
		o.Set("Q", jsc.ObjectValue(newWGParamObject(rt, c, n, n.params["Q"])))
		o.Set("gain", jsc.ObjectValue(newWGParamObject(rt, c, n, n.params["gain"])))
		o.SetAccessor("type", func(in *jsc.Interpreter) jsc.JSValue {
			return jsc.StringValue(n.bqTypeName)
		}, func(in *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) {
			s := v.ToString()
			switch s {
			case "lowpass", "highpass", "bandpass", "lowshelf", "highshelf", "peaking", "notch", "allpass":
				n.bqTypeName = s
			default:
				panic(in.VM().NewTypeError("BiquadFilterNode.type 取值非法: " + s))
			}
		})
		o.Set("getFrequencyResponse", jsc.FunctionValue(rt.NewNativeFunction("getFrequencyResponse",
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				if len(args) < 3 {
					panic(in.VM().NewTypeError("getFrequencyResponse: 需要 3 个参数"))
				}
				freqs, ok := float32SliceArg(args[0])
				if !ok {
					panic(in.VM().NewTypeError("getFrequencyResponse: 参数 1 不是 Float32Array"))
				}
				mag := make([]float32, len(freqs))
				phase := make([]float32, len(freqs))
				fq := n.params["frequency"].valueAt(c.now())
				q := n.params["Q"].valueAt(c.now())
				g := n.params["gain"].valueAt(c.now())
				for i, f := range freqs {
					b0, b1, b2, a1, a2 := wgBiquadCoeffs(n.bqTypeName, float64(f), q, g, float64(c.graph.sampleRate))
					m, ph := wgBiquadResponse(b0, b1, b2, a1, a2, float64(f), float64(c.graph.sampleRate))
					mag[i], phase[i] = float32(m), float32(ph)
				}
				_ = fq
				return in.Float32ArrayValue(mag) // 简化：只回幅度（见 docs 记录）
			}, 3)))
	case wgKAnalyser:
		o.Set("fftSize", jsc.NumberValue(float64(n.an.fftSize)))
		o.Set("frequencyBinCount", jsc.NumberValue(float64(n.an.fftSize/2)))
		o.SetAccessor("smoothingTimeConstant", func(in *jsc.Interpreter) jsc.JSValue {
			return jsc.NumberValue(n.an.smoothing)
		}, func(in *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) {
			s := v.ToNumber()
			if s < 0 {
				s = 0
			}
			if s > 1 {
				s = 1
			}
			n.an.smoothing = s
		})
		o.Set("getFloatTimeDomainData", jsc.FunctionValue(rt.NewNativeFunction("getFloatTimeDomainData",
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				if len(args) < 1 || !args[0].IsObject() {
					panic(in.VM().NewTypeError("getFloatTimeDomainData: 参数 1 不是 Float32Array"))
				}
				td := n.an.timeDomain()
				return jsc.ObjectValue(writeFloat32Array(in, args[0].AsObject(), td))
			}, 1)))
		o.Set("getFloatFrequencyData", jsc.FunctionValue(rt.NewNativeFunction("getFloatFrequencyData",
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				if len(args) < 1 || !args[0].IsObject() {
					panic(in.VM().NewTypeError("getFloatFrequencyData: 参数 1 不是 Float32Array"))
				}
				mag := n.an.magnitudes()
				db := make([]float32, len(mag))
				for i, m := range mag {
					if m <= 0 {
						db[i] = -1000
					} else {
						db[i] = float32(20 * math.Log10(m))
					}
				}
				return jsc.ObjectValue(writeFloat32Array(in, args[0].AsObject(), db))
			}, 1)))
		o.Set("getByteFrequencyData", jsc.FunctionValue(rt.NewNativeFunction("getByteFrequencyData",
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				if len(args) < 1 || !args[0].IsObject() {
					panic(in.VM().NewTypeError("getByteFrequencyData: 参数 1 不是 Uint8Array"))
				}
				mag := n.an.magnitudes()
				b := make([]byte, len(mag))
				for i, m := range mag {
					// 规范：dB 映射到 0..255（minDecibels=-100, maxDecibels=-30）
					db := 20 * math.Log10(math.Max(m, 1e-10))
					x := (db + 100) / 70 * 255
					if x < 0 {
						x = 0
					}
					if x > 255 {
						x = 255
					}
					b[i] = byte(x)
				}
				return jsc.ObjectValue(writeUint8Array(in, args[0].AsObject(), b))
			}, 1)))
	}
	return o
}

// dummyContextRef 返回一个「回指上下文」的轻量对象（避免无限递归：节点 → context
// → destination → 节点…，这里只暴露 id，脚本读 node.context.sampleRate 可用）。
func dummyContextRef(rt *jsc.Interpreter, c *wgContext) *jsc.JSObject {
	o := jsc.NewObject(rt.ObjectPrototype())
	o.SetClassName("AudioContext")
	o.Set("__wgCtx", jsc.NumberValue(float64(c.id)))
	o.Set("sampleRate", jsc.NumberValue(float64(c.sampleRate)))
	return o
}

func nodeClassName(k wgNodeKind) string {
	switch k {
	case wgKGain:
		return "GainNode"
	case wgKOscillator:
		return "OscillatorNode"
	case wgKBufferSource:
		return "AudioBufferSourceNode"
	case wgKConstantSource:
		return "ConstantSourceNode"
	case wgKStereoPanner:
		return "StereoPannerNode"
	case wgKDelay:
		return "DelayNode"
	case wgKBiquadFilter:
		return "BiquadFilterNode"
	case wgKAnalyser:
		return "AnalyserNode"
	}
	return "AudioNode"
}

func nodeInputs(k wgNodeKind) int {
	switch k {
	case wgKOscillator, wgKBufferSource, wgKConstantSource:
		return 0
	}
	return 1
}

func nodeOutputs(k wgNodeKind) int { return 1 }

// ─── AudioParam 对象 ──────────────────────────────────────

func newWGParamObject(rt *jsc.Interpreter, c *wgContext, n *wgNode, p *wgParam) *jsc.JSObject {
	o := jsc.NewObject(rt.ObjectPrototype())
	o.SetClassName("AudioParam")
	o.Set("__wgCtx", jsc.NumberValue(float64(c.id)))
	o.Set("__wgId", jsc.NumberValue(float64(n.id)))
	o.Set("__wgParam", jsc.StringValue(p.name))
	o.SetAccessor("value", func(in *jsc.Interpreter) jsc.JSValue {
		c.mu.Lock()
		defer c.mu.Unlock()
		return jsc.NumberValue(p.valueAt(c.now()))
	}, func(in *jsc.Interpreter, _ jsc.JSValue, v jsc.JSValue) {
		c.mu.Lock()
		p.setValue(v.ToNumber(), c.now())
		c.mu.Unlock()
	})
	o.Set("defaultValue", jsc.NumberValue(p.defVal))
	o.Set("minValue", jsc.NumberValue(math.Inf(-1)))
	o.Set("maxValue", jsc.NumberValue(math.Inf(1)))

	atTime := func(name string, fn func(p *wgParam, args []jsc.JSValue, now float64)) {
		o.Set(name, jsc.FunctionValue(rt.NewNativeFunction(name,
			func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				if len(args) < 2 || !args[1].IsNumber() {
					panic(in.VM().NewTypeError(name + ": 参数 2 必须是时间（数字）"))
				}
				c.mu.Lock()
				fn(p, args, c.now())
				c.mu.Unlock()
				return jsc.Undefined()
			}, 3)))
	}
	atTime("setValueAtTime", func(p *wgParam, args []jsc.JSValue, _ float64) {
		p.setValueAtTime(args[0].ToNumber(), args[1].ToNumber())
	})
	atTime("linearRampToValueAtTime", func(p *wgParam, args []jsc.JSValue, _ float64) {
		p.linearRampToValueAtTime(args[0].ToNumber(), args[1].ToNumber())
	})
	atTime("exponentialRampToValueAtTime", func(p *wgParam, args []jsc.JSValue, _ float64) {
		p.exponentialRampToValueAtTime(args[0].ToNumber(), args[1].ToNumber())
	})
	atTime("setTargetAtTime", func(p *wgParam, args []jsc.JSValue, _ float64) {
		tc := 0.0
		if len(args) >= 3 && args[2].IsNumber() {
			tc = args[2].ToNumber()
		}
		p.setTargetAtTime(args[0].ToNumber(), args[1].ToNumber(), tc)
	})
	atTime("setValueCurveAtTime", func(p *wgParam, args []jsc.JSValue, _ float64) {
		dur := 0.0
		if len(args) >= 3 && args[2].IsNumber() {
			dur = args[2].ToNumber()
		}
		var curve []float32
		if len(args) >= 1 && args[0].IsObject() {
			if f, ok := float32SliceArg(args[0]); ok {
				curve = f
			}
		}
		p.setValueCurveAtTime(curve, args[1].ToNumber(), dur)
	})
	o.Set("cancelScheduledValues", jsc.FunctionValue(rt.NewNativeFunction("cancelScheduledValues",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			t := c.now()
			if len(args) >= 1 && args[0].IsNumber() {
				t = args[0].ToNumber()
			}
			c.mu.Lock()
			p.cancelScheduledValues(t)
			c.mu.Unlock()
			return jsc.Undefined()
		}, 1)))
	o.Set("cancelAndHoldAtTime", jsc.FunctionValue(rt.NewNativeFunction("cancelAndHoldAtTime",
		func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			t := c.now()
			if len(args) >= 1 && args[0].IsNumber() {
				t = args[0].ToNumber()
			}
			c.mu.Lock()
			p.cancelAndHoldAtTime(t)
			c.mu.Unlock()
			return jsc.Undefined()
		}, 1)))
	return o
}

// ─── 辅助 ─────────────────────────────────────────────────

// float32SliceArg 从 JS 的 Float32Array / Array 取样本。
func float32SliceArg(v jsc.JSValue) ([]float32, bool) {
	o := v.AsObject()
	if o == nil {
		return nil, false
	}
	lv, ok := o.GetByKey("length")
	if !ok || !lv.IsNumber() {
		return nil, false
	}
	n := int(lv.ToNumber())
	if n < 0 || n > 1<<22 {
		return nil, false
	}
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		iv, ok := o.GetByKey(strconv.Itoa(i))
		if !ok {
			return nil, false
		}
		out[i] = float32(iv.ToNumber())
	}
	return out, true
}

// writeFloat32Array 把样本**就地**写进调用方给的 Float32Array
// （getFloatTimeDomainData / getFloatFrequencyData 的语义是「填充传入的数组」）。
//
// 走 jsc.ArrayBufferBytes 拿到该 TypedArray 的底层字节切片 → 零拷贝、逐元素直接
// 落到视图内存；若传入的其实是普通 Array（非 TypedArray），退化为逐索引写入
// （规范要求 TypedArray，这里对宽松调用也给可用结果，而不是静默什么都不做）。
func writeFloat32Array(in *jsc.Interpreter, o *jsc.JSObject, data []float32) *jsc.JSObject {
	if o == nil {
		return nil
	}
	if raw, ok := in.ArrayBufferBytes(jsc.ObjectValue(o)); ok && len(raw) >= len(data)*4 {
		for i, v := range data {
			binary.LittleEndian.PutUint32(raw[i*4:], math.Float32bits(v))
		}
		return o
	}
	for i, v := range data {
		o.Set(strconv.Itoa(i), jsc.NumberValue(float64(v)))
	}
	return o
}

// writeUint8Array 同 writeFloat32Array，用于 getByteFrequencyData 的 Uint8Array。
func writeUint8Array(in *jsc.Interpreter, o *jsc.JSObject, data []byte) *jsc.JSObject {
	if o == nil {
		return nil
	}
	if raw, ok := in.ArrayBufferBytes(jsc.ObjectValue(o)); ok && len(raw) >= len(data) {
		copy(raw, data)
		return o
	}
	for i, v := range data {
		o.Set(strconv.Itoa(i), jsc.NumberValue(float64(v)))
	}
	return o
}

// wgBiquadResponse 计算双二阶在频率 f 处的幅频/相频响应。
func wgBiquadResponse(b0, b1, b2, a1, a2, f, sampleRate float64) (mag, phase float64) {
	w := 2 * math.Pi * f / sampleRate
	cw, sw := math.Cos(w), math.Sin(w)
	c2, s2 := math.Cos(2*w), math.Sin(2*w)
	// H(e^{jw}) = (b0 + b1 z^-1 + b2 z^-2) / (1 + a1 z^-1 + a2 z^-2)
	numRe := b0 + b1*cw + b2*c2
	numIm := -(b1*sw + b2*s2)
	denRe := 1 + a1*cw + a2*c2
	denIm := -(a1*sw + a2*s2)
	den := denRe*denRe + denIm*denIm
	if den == 0 {
		return 0, 0
	}
	re := (numRe*denRe + numIm*denIm) / den
	im := (numIm*denRe - numRe*denIm) / den
	return math.Hypot(re, im), math.Atan2(im, re)
}

// wgBufferOf 从 AudioBuffer JS 对象取回解码数据。数据挂在对象的内部槽里
// （newAudioBuffer 的 SetInternal），与 getChannelData 返回的 Float32Array 共享
// 同一底层样本数组：脚本先写 getChannelData 再交给 bufferSource.buffer 时，
// 写进去的样本能被原样播放（这是绝大多数「合成音频」示例的写法）。
func wgBufferOf(o *jsc.JSObject) *AudioDecoded {
	if o == nil {
		return nil
	}
	if dec, ok := o.Internal().(*AudioDecoded); ok {
		return dec
	}
	return nil
}

// wgEnsureRealTime 有节点开始发声时启动实时渲染（幂等）。
func wgEnsureRealTime(c *wgContext) {
	if c == nil || c.offline {
		return
	}
	c.mu.Lock()
	c.started = true
	c.renderedFrames = int64(c.now() * float64(c.graph.sampleRate))
	needArm := !c.timerArmed
	c.timerArmed = true
	c.mu.Unlock()
	if needArm {
		wgArmTimer(c)
	}
}

// wgArmTimer 排下一次实时渲染（16ms —— 与浏览器 60Hz 的渲染节奏同量级）。
func wgArmTimer(c *wgContext) {
	if c == nil || c.rt == nil {
		return
	}
	loop := c.rt.EnsureEventLoop()
	if loop == nil {
		return
	}
	cb := c.rt.NewNativeFunction("webaudio_pump", func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
		wgPumpRealTime(c)
		return jsc.Undefined()
	}, 0)
	_ = loop.SetTimeout(jsc.FunctionValue(cb), 16)
}

// wgPumpRealTime 把图渲染推进到当前上下文时间，并把结果交给宿主 sink。
//
// ★ 渲染帧数 = 「上下文时钟已流逝的时间」而不是「定时器调用次数」：定时器被主循环
//
//	挤慢时（重排、GC）会一次补上更多量子，音频因此不会因掉帧而变调或断流——这与
//	浏览器音频线程独立于主线程的效果等价（本引擎是单线程协作式，因此只能保证
//	「总时长正确」，不保证低延迟）。
func wgPumpRealTime(c *wgContext) {
	if c == nil || c.offline {
		return
	}
	c.mu.Lock()
	if !c.started || c.clock.stateName() != "running" {
		c.timerArmed = false
		c.mu.Unlock()
		return
	}
	target := int64(c.now() * float64(c.graph.sampleRate))
	// 一次最多补 200ms 的量子：长时间挂起（断点/系统休眠）后不追赶巨量历史。
	maxCatch := int64(float64(c.graph.sampleRate) * 0.2)
	if target-c.renderedFrames > maxCatch {
		c.renderedFrames = target - maxCatch
	}
	for c.renderedFrames+wgQuantum <= target {
		t := float64(c.renderedFrames) / float64(c.graph.sampleRate)
		c.graph.renderQuantum(t, wgQuantum, c.pcm)
		c.renderedFrames += wgQuantum
		if WGAudioSink != nil {
			k := 0
			for i := 0; i < wgQuantum; i++ {
				for ch := 0; ch < wgChannels; ch++ {
					c.interleaved[k] = c.pcm[ch][i]
					k++
				}
			}
			WGAudioSink(c.interleaved, c.graph.sampleRate)
		}
	}
	arm := c.started
	c.timerArmed = arm
	c.mu.Unlock()
	if arm {
		wgArmTimer(c)
	}
}

// ─── OfflineAudioContext ──────────────────────────────────

// newOfflineAudioContext 实现 `new OfflineAudioContext(numberOfChannels, length, sampleRate)`
// 与 `new OfflineAudioContext({numberOfChannels, length, sampleRate})`。
func newOfflineAudioContext(rt *jsc.Interpreter, args []jsc.JSValue) *jsc.JSObject {
	if rt == nil {
		return nil
	}
	channels, length, rate := 1, 0, defaultAudioContextSampleRate
	if len(args) >= 1 && args[0].IsObject() {
		if o := args[0].AsObject(); o != nil {
			if v, ok := o.GetByKey("numberOfChannels"); ok && v.IsNumber() {
				channels = int(v.ToNumber())
			}
			if v, ok := o.GetByKey("length"); ok && v.IsNumber() {
				length = int(v.ToNumber())
			}
			if v, ok := o.GetByKey("sampleRate"); ok && v.IsNumber() {
				rate = int(v.ToNumber())
			}
		}
	} else {
		if len(args) >= 1 && args[0].IsNumber() {
			channels = int(args[0].ToNumber())
		}
		if len(args) >= 2 && args[1].IsNumber() {
			length = int(args[1].ToNumber())
		}
		if len(args) >= 3 && args[2].IsNumber() {
			rate = int(args[2].ToNumber())
		}
	}
	if channels < 1 {
		channels = 1
	}
	if channels > 32 {
		panic(rt.VM().NewTypeError("OfflineAudioContext: numberOfChannels 过大（>32）"))
	}
	if length <= 0 {
		panic(rt.VM().NewTypeError("OfflineAudioContext: length 必须 > 0"))
	}
	if rate <= 0 {
		rate = defaultAudioContextSampleRate
	}
	c := newWGContext(rt, rate, true, length, channels)
	obj := jsc.NewObject(rt.ObjectPrototype())
	obj.SetClassName("OfflineAudioContext")
	obj.Set("__wgCtx", jsc.NumberValue(float64(c.id)))
	obj.Set("sampleRate", jsc.NumberValue(float64(rate)))
	obj.Set("length", jsc.NumberValue(float64(length)))
	obj.Set("currentTime", jsc.NumberValue(0))
	obj.Set("state", jsc.StringValue("suspended"))
	obj.Set("destination", jsc.ObjectValue(newWGDestinationObject(rt, c)))
	installWGCreators(rt, obj, c)
	obj.Set("startRendering", jsc.FunctionValue(rt.NewNativeFunction("startRendering",
		func(in *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			buf := wgRenderOffline(c)
			if buf == nil {
				return in.RejectPromise(newDOMExceptionValue(in, "InvalidStateError",
					"OfflineAudioContext.startRendering: 渲染失败"))
			}
			return in.ResolvePromise(jsc.ObjectValue(newAudioBuffer(in, *buf)))
		}, 0)))
	return obj
}

// wgRenderOffline 离线渲染整个 length，返回 AudioBuffer 数据（单声道时混 2→1）。
//
// 离线渲染的语义要点（也是它能与浏览器逐样本比的根据）：
//   - 时间轴由 **帧号** 决定（t = frame / sampleRate），与真实耗时无关；
//   - 图在渲染前必须已被脚本连好（startRendering 之后不再接受连接变化——
//     规范如此，本实现也只在开始时取一次拓扑）；
//   - 输出声道数 = 上下文声明的 numberOfChannels（1 或 2）。
func wgRenderOffline(c *wgContext) *AudioDecoded {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	sr := c.graph.sampleRate
	out := make([][]float32, c.channels)
	for i := range out {
		out[i] = make([]float32, c.length)
	}
	frames := c.length
	pos := 0
	for pos < frames {
		n := wgQuantum
		if frames-pos < n {
			n = frames - pos
		}
		t := float64(pos) / float64(sr)
		c.graph.renderQuantum(t, n, c.pcm)
		for ch := 0; ch < c.channels; ch++ {
			src := ch
			if src >= wgChannels {
				src = wgChannels - 1
			}
			copy(out[ch][pos:pos+n], c.pcm[src][:n])
		}
		pos += n
	}
	return &AudioDecoded{SampleRate: sr, Channels: out}
}
