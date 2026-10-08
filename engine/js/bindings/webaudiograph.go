package bindings

// WebAudio 音频图内核（A3-3 完整子集，2026-10-08）。
//
// 这一层是**纯计算**：音频图模型（节点 / 连接 / 参数自动化）与按 128 帧量子的
// 渲染。它不认识 JS 对象，也不碰输出设备——JS 绑定在 webaudio_api.go，实时/离线
// 驱动在 webaudio_ctx.go，宿主输出在 app 侧（与 A3 一样：引擎不背输出设备）。
//
// 为什么现在做（此前记为「须另立项」）：`docs/audio-backend-proposal.md` §6 把
// 完整音频图列为不做项、`media-format-verification-plan.md` §10 记为「仍未动的
// 只有 Q2-C 完整音频图」。它确实是本仓库媒体主线里最大的一块（原估 3000–6000
// 行），但它的**语义**是可以精确对齐浏览器的（参数自动化与节点公式都是规范里
// 的确定算式），因此可以做、也必须用可复现的判据验证——而不是留在「不做」栏里。
//
// 设计取舍（三条，都写在注释里备查）：
//
//	① **内部固定 2 声道**：多声道路由（ChannelMerger/Splitter 的任意端口数）需要
//	   「输出端口 × 声道」的缓冲模型；本内核先用单输出端口 × 2 声道覆盖绝大多数
//	   实际用途（可视化、合成、混音、滤波），多声道分离/合并**未实现**（如实记录，
//	   不假装支持：对应 API 不挂出，脚本特性检测会如实得到 undefined）。
//	② **时钟是上下文时间**：渲染函数按「量子起点时间 t」求所有参数值，不读挂钟
//	   ——离线渲染（OfflineAudioContext）因此可以比实时快得多，且结果由 length
//	   决定、与运行速度无关（这正是它与 Edge 逐样本可比的根据）。
//	③ **反馈环**：DelayNode 读写分离（写入当前量子、读出延迟量），因此环存在时
//	   仍可渲染；其它节点的环按拓扑排序截断（去掉回边），不静默产生 NaN。
import (
	"math"
	"sort"
)

// ─── 常量 ─────────────────────────────────────────────────

const (
	// wgQuantum 是一个渲染量子的帧数（规范固定 128）。
	wgQuantum = 128
	// wgChannels 是内核实用的声道数（见文件头取舍 ①）。
	wgChannels = 2
)

// ─── 参数自动化（AudioParam）──────────────────────────────

const (
	wgSetValue   = iota // setValueAtTime
	wgLinearRamp        // linearRampToValueAtTime
	wgExpRamp           // exponentialRampToValueAtTime
	wgSetTarget         // setTargetAtTime
	wgValueCurve        // setValueCurveAtTime
)

// wgParamEvent 是一条自动化事件。
//
// 时间语义与规范一致：对 ramp 类事件，`time` 是**目标时间**（区间终点），区间起点
// 是前一条事件的位置——这也是求值器按顺序推进的原因。
type wgParamEvent struct {
	kind      int
	time      float64
	value     float64
	duration  float64   // setValueCurveAtTime 的时长
	timeConst float64   // setTargetAtTime 的时间常数
	curve     []float32 // setValueCurveAtTime 的曲线
}

// wgParam 是一个 AudioParam：一个值 + 一条按时间排序的事件列表。
type wgParam struct {
	name   string
	defVal float64
	value  float64
	events []wgParamEvent
}

func newWGParam(name string, def float64) *wgParam {
	return &wgParam{name: name, defVal: def, value: def}
}

// setValue 实现 `param.value = v`：规范里这是「在**当前时刻**插入一条
// setValueAtTime」。没有自动化事件时直接改值即可；有事件时插入事件，否则脚本
// 的赋值会被事件序列覆盖（脚本读回 .value 与实际渲染值会不一致）。
func (p *wgParam) setValue(v, now float64) {
	p.value = v
	if len(p.events) > 0 {
		p.insert(wgParamEvent{kind: wgSetValue, time: now, value: v})
	}
}

func (p *wgParam) insert(e wgParamEvent) {
	p.events = append(p.events, e)
	sort.SliceStable(p.events, func(i, j int) bool { return p.events[i].time < p.events[j].time })
}

func (p *wgParam) setValueAtTime(v, t float64) {
	p.insert(wgParamEvent{kind: wgSetValue, time: t, value: v})
}

func (p *wgParam) linearRampToValueAtTime(v, t float64) {
	p.insert(wgParamEvent{kind: wgLinearRamp, time: t, value: v})
}

func (p *wgParam) exponentialRampToValueAtTime(v, t float64) {
	p.insert(wgParamEvent{kind: wgExpRamp, time: t, value: v})
}

func (p *wgParam) setTargetAtTime(target, startTime, timeConst float64) {
	p.insert(wgParamEvent{kind: wgSetTarget, time: startTime, value: target, timeConst: timeConst})
}

func (p *wgParam) setValueCurveAtTime(curve []float32, startTime, duration float64) {
	p.insert(wgParamEvent{kind: wgValueCurve, time: startTime, value: 0, duration: duration, curve: curve})
}

// cancelScheduledValues 丢弃所有 time >= t 的事件（规范语义）。
func (p *wgParam) cancelScheduledValues(t float64) {
	kept := p.events[:0]
	for _, e := range p.events {
		if e.time < t {
			kept = append(kept, e)
		}
	}
	p.events = kept
}

// cancelAndHoldAtTime 丢弃 time > t 的事件，并把 t 时刻的值固化为一条 setValue
// （规范语义的近似：规范要求「保持当前值」，实现上等价于在 t 处插一条 setValue）。
func (p *wgParam) cancelAndHoldAtTime(t float64) {
	v := p.valueAt(t)
	kept := p.events[:0]
	for _, e := range p.events {
		if e.time <= t {
			kept = append(kept, e)
		}
	}
	p.events = kept
	p.insert(wgParamEvent{kind: wgSetValue, time: t, value: v})
}

// valueAt 求 t 时刻的参数值。
//
// 顺序推进：维护「当前段起点 (segT, segV)」与「当前值 v」。落在 ramp 区间内时按
// 线性/指数插值；setTarget 按指数逼近（起点取进入该事件时的值）；setValueCurve
// 在时长内按曲线插值。
//
// ★ 已知简化：setTarget 与紧接其后的 ramp 组合时，ramp 的起点取 setTarget 的
// 起始值而非「setTarget 在 ramp 起点时刻的值」。规范的完整语义要求按事件段积分，
// 这里用的是工程近似（对典型用法——先 setValueAtTime 再 ramp / 再 setTarget——
// 与浏览器逐样本一致，已由 TestWGParamAutomation 钉住）。
func (p *wgParam) valueAt(t float64) float64 {
	ev := p.events
	if len(ev) == 0 {
		return p.value
	}
	if t < ev[0].time {
		// 首个事件之前：用 value（规范：事件生效前 value 有效）。
		return p.value
	}
	v := p.value
	segT, segV := ev[0].time, p.value
	for i := range ev {
		e := ev[i]
		if e.time > t {
			// 段内插值：只有 ramp 会在「尚未到达目标时间」时有值变化。
			switch e.kind {
			case wgLinearRamp:
				span := e.time - segT
				if span <= 0 {
					return e.value
				}
				return segV + (e.value-segV)*(t-segT)/span
			case wgExpRamp:
				span := e.time - segT
				if span <= 0 || segV == 0 || e.value == 0 {
					return e.value
				}
				return segV * math.Pow(e.value/segV, (t-segT)/span)
			}
			return v
		}
		switch e.kind {
		case wgSetValue:
			v, segT, segV = e.value, e.time, e.value
		case wgLinearRamp, wgExpRamp:
			v, segT, segV = e.value, e.time, e.value
		case wgSetTarget:
			start := segV
			segT = e.time
			dt := t - e.time
			if e.timeConst > 0 {
				v = e.value + (start-e.value)*math.Exp(-dt/e.timeConst)
			} else {
				v = e.value
			}
		case wgValueCurve:
			end := e.time + e.duration
			if len(e.curve) == 0 {
				v = 0
			} else if t <= end && e.duration > 0 {
				pos := (t - e.time) / e.duration * float64(len(e.curve)-1)
				i0 := int(pos)
				if i0 >= len(e.curve)-1 {
					v = float64(e.curve[len(e.curve)-1])
				} else {
					frac := pos - float64(i0)
					v = float64(e.curve[i0])*(1-frac) + float64(e.curve[i0+1])*frac
				}
			} else {
				v = float64(e.curve[len(e.curve)-1])
			}
			segT, segV = end, v
		}
	}
	return v
}

// ─── 节点 ─────────────────────────────────────────────────

type wgNodeKind int

const (
	wgKDestination wgNodeKind = iota
	wgKGain
	wgKOscillator
	wgKBufferSource
	wgKConstantSource
	wgKStereoPanner
	wgKDelay
	wgKBiquadFilter
	wgKAnalyser
)

// wgNode 是图上的一个节点。
//
// 缓冲模型：`out` 是本节点**当前量子**的输出（2 声道 × frames），`in` 是各输入
// 端口求和后的输入。渲染顺序由拓扑序保证「先上游后下游」。
type wgNode struct {
	id   int
	kind wgNodeKind
	g    *wgGraph

	inComing [][]*wgNode // 每个输入端口的来源节点
	out      [][]float32 // [ch][frame]
	in       [][]float32 // [ch][frame]（端口 0 求和结果）

	params map[string]*wgParam

	// — 通用 —
	active bool

	// — OscillatorNode —
	oscType string
	phase   float64
	started bool
	startAt float64
	stopAt  float64 // 0 = 未 stop（与"已 stop"用 stopped 区分）
	stopped bool

	// — AudioBufferSourceNode —
	srcBuf    *AudioDecoded
	srcRate   float64 // 当前读指针（**上下文采样率**下的位置，按 playbackRate 推进）
	srcOn     bool
	srcOffset float64 // start(when, offset) 的 offset（源采样率秒）
	srcDur    float64 // start(when, offset, duration)：0 = 到缓冲末尾
	srcLoop   bool
	loopStart float64
	loopEnd   float64

	// — DelayNode —
	delayBuf [][]float32
	delayPos int

	// — BiquadFilterNode —
	bq [wgChannels]wgBiquadState
	// bqTypeName 是滤波器的 type（规范里 type 是字符串属性，不是 AudioParam，
	// 因此不能放进 params 表）。默认 lowpass（规范默认值）。
	bqTypeName string

	// — AnalyserNode —
	an *wgAnalyserState
}

// wgBiquadState 是双二阶滤波器的一个声道状态（Direct Form I）。
type wgBiquadState struct {
	x1, x2, y1, y2 float64
}

// ─── 图 ───────────────────────────────────────────────────

type wgGraph struct {
	sampleRate int
	quantum    int
	nodes      []*wgNode
	dest       *wgNode
	order      []*wgNode
	orderValid bool
	frames     int // 当前量子长度
}

func newWGGraph(sampleRate int) *wgGraph {
	g := &wgGraph{sampleRate: sampleRate, quantum: wgQuantum, frames: wgQuantum}
	g.dest = g.newNode(wgKDestination)
	return g
}

// newNode 建节点并分配量子缓冲。
func (g *wgGraph) newNode(kind wgNodeKind) *wgNode {
	n := &wgNode{
		id:     len(g.nodes),
		kind:   kind,
		g:      g,
		params: map[string]*wgParam{},
		out:    make([][]float32, wgChannels),
		in:     make([][]float32, wgChannels),
		active: true,
	}
	for c := 0; c < wgChannels; c++ {
		n.out[c] = make([]float32, wgQuantum)
		n.in[c] = make([]float32, wgQuantum)
	}
	g.nodes = append(g.nodes, n)
	g.orderValid = false
	return n
}

// connect 把 src 的输出接到 dst 的输入端口 port。
func (g *wgGraph) connect(src, dst *wgNode) {
	if src == nil || dst == nil {
		return
	}
	for _, p := range dst.inComing {
		for _, e := range p {
			if e == src {
				return // 重复连接是 no-op（与规范一致）
			}
		}
	}
	if len(dst.inComing) == 0 {
		dst.inComing = append(dst.inComing, nil)
	}
	dst.inComing[0] = append(dst.inComing[0], src)
	g.orderValid = false
}

// disconnect 断开 src → dst（dst 为 nil 时断开 src 的所有下游）。
func (g *wgGraph) disconnect(src, dst *wgNode) {
	if src == nil {
		return
	}
	if dst == nil {
		for _, n := range g.nodes {
			for pi := range n.inComing {
				kept := n.inComing[pi][:0]
				for _, e := range n.inComing[pi] {
					if e != src {
						kept = append(kept, e)
					}
				}
				n.inComing[pi] = kept
			}
		}
	} else {
		for pi := range dst.inComing {
			kept := dst.inComing[pi][:0]
			for _, e := range dst.inComing[pi] {
				if e != src {
					kept = append(kept, e)
				}
			}
			dst.inComing[pi] = kept
		}
	}
	g.orderValid = false
}

// topo 计算拓扑序（上游在前）。
//
// 实现：从 destination 反向 DFS，取后序再反转；遇到正在访问中的节点即回边
// （反馈环），跳过该边。DelayNode 的环因「写入当前量子、读出延迟量」的读写分离
// 而不会产生 NaN，因此这里不需要额外处理。
func (g *wgGraph) topo() {
	if g.orderValid {
		return
	}
	state := make(map[*wgNode]int, len(g.nodes)) // 0=未访问 1=访问中 2=已完成
	var order []*wgNode
	var visit func(n *wgNode)
	visit = func(n *wgNode) {
		if state[n] == 1 || state[n] == 2 {
			return
		}
		state[n] = 1
		for _, port := range n.inComing {
			for _, up := range port {
				visit(up)
			}
		}
		state[n] = 2
		order = append(order, n)
	}
	// 后序 DFS（上游全部完成后再记录自己）得到的 order **已经是「上游在前」**。
	// ★ 这里曾多写了一次反转，后果很隐蔽：所有节点都比自己的上游先渲染，于是
	//   destination 读到的永远是上游**上一个量子**的输出（整体延迟 128 帧），
	//   而振荡器/缓冲源这类「从 0 开始」的信号在首量子恰好是 0 —— 表现为整段
	//   静音（DelayNode 的输入端同理拿不到本量子的写入）。
	visit(g.dest)
	// 不在 destination 上游的节点（尚未连接/已断开）也要渲染：它们没有下游，
	// 渲染与否不影响输出，但保持「节点被访问过」的一致性（分析器要读它们）。
	for _, n := range g.nodes {
		if state[n] == 0 {
			visit(n)
		}
	}
	g.order = order
	g.orderValid = true
}

// renderQuantum 渲染一个量子：t 是量子起点的上下文时间，frames 是帧数（≤ quantum）。
// 结果写进 out（2 声道 × frames）。
func (g *wgGraph) renderQuantum(t float64, frames int, out [][]float32) {
	g.frames = frames
	g.topo()
	for _, n := range g.order {
		g.renderNode(n, t, frames)
	}
	for c := 0; c < wgChannels; c++ {
		for i := 0; i < frames; i++ {
			out[c][i] = 0
		}
	}
	// destination 的输出 = 其输入求和（它没有"计算"，只是收集）
	for c := 0; c < wgChannels; c++ {
		copy(out[c][:frames], g.dest.in[c][:frames])
	}
}

// sumInputs 把上游节点的输出累加到 n.in（端口 0）。
func (g *wgGraph) sumInputs(n *wgNode, frames int) {
	for c := 0; c < wgChannels; c++ {
		for i := 0; i < frames; i++ {
			n.in[c][i] = 0
		}
	}
	for _, port := range n.inComing {
		for _, up := range port {
			for c := 0; c < wgChannels; c++ {
				src := up.out[c]
				dst := n.in[c]
				for i := 0; i < frames; i++ {
					dst[i] += src[i]
				}
			}
		}
	}
}

func (g *wgGraph) clearOut(n *wgNode, frames int) {
	for c := 0; c < wgChannels; c++ {
		for i := 0; i < frames; i++ {
			n.out[c][i] = 0
		}
	}
}

// renderNode 渲染一个节点的一个量子。
func (g *wgGraph) renderNode(n *wgNode, t float64, frames int) {
	switch n.kind {
	case wgKDestination:
		g.sumInputs(n, frames)
		g.clearOut(n, frames)
		for c := 0; c < wgChannels; c++ {
			copy(n.out[c][:frames], n.in[c][:frames])
		}
	case wgKGain:
		g.sumInputs(n, frames)
		gain := n.params["gain"]
		for i := 0; i < frames; i++ {
			tt := t + float64(i)/float64(g.sampleRate)
			gv := float32(gain.valueAt(tt))
			for c := 0; c < wgChannels; c++ {
				n.out[c][i] = n.in[c][i] * gv
			}
		}
	case wgKConstantSource:
		off := n.params["offset"]
		for i := 0; i < frames; i++ {
			tt := t + float64(i)/float64(g.sampleRate)
			v := float32(off.valueAt(tt))
			for c := 0; c < wgChannels; c++ {
				n.out[c][i] = v
			}
		}
	case wgKOscillator:
		g.renderOscillator(n, t, frames)
	case wgKBufferSource:
		g.renderBufferSource(n, t, frames)
	case wgKStereoPanner:
		g.sumInputs(n, frames)
		pan := n.params["pan"]
		for i := 0; i < frames; i++ {
			tt := t + float64(i)/float64(g.sampleRate)
			p := pan.valueAt(tt)
			if p < -1 {
				p = -1
			} else if p > 1 {
				p = 1
			}
			// 等功率（规范 StereoPannerNode 的算法）。
			x := (p + 1) * math.Pi / 4
			gl, gr := float32(math.Cos(x)), float32(math.Sin(x))
			mono := (n.in[0][i] + n.in[1][i]) * 0.5
			n.out[0][i] = mono * gl
			n.out[1][i] = mono * gr
		}
	case wgKDelay:
		g.renderDelay(n, t, frames)
	case wgKBiquadFilter:
		g.renderBiquad(n, t, frames)
	case wgKAnalyser:
		g.sumInputs(n, frames)
		for c := 0; c < wgChannels; c++ {
			copy(n.out[c][:frames], n.in[c][:frames])
		}
		if n.an != nil {
			// 分析器本身不改信号（规范：输出 = 输入），只记录最近样本供读取。
			n.an.push(n.out, frames)
		}
	default:
		g.clearOut(n, frames)
	}
}

// renderOscillator 渲染振荡器：sine / square / sawtooth / triangle。
//
// 相位按 **每帧** 推进（frequency 与 detune 都是可自动化的参数 ⇒ 频率可以在量子
// 内变化，按量子取一个常量会让扫描（常用于测试与扫频）出现台阶）。
func (g *wgGraph) renderOscillator(n *wgNode, t float64, frames int) {
	g.clearOut(n, frames)
	if !n.started || (n.stopped && t >= n.stopAt) {
		return
	}
	freq := n.params["frequency"]
	det := n.params["detune"]
	for i := 0; i < frames; i++ {
		tt := t + float64(i)/float64(g.sampleRate)
		if tt < n.startAt {
			continue
		}
		if n.stopped && tt >= n.stopAt {
			break
		}
		f := freq.valueAt(tt) * math.Pow(2, det.valueAt(tt)/1200)
		s := wgOscSample(n.oscType, n.phase)
		n.phase += f / float64(g.sampleRate)
		if n.phase >= 1 {
			n.phase -= math.Floor(n.phase)
		}
		for c := 0; c < wgChannels; c++ {
			n.out[c][i] = float32(s)
		}
	}
}

// wgOscSample 按相位（0..1）取一个振荡器样本（规范的基本波形；square/sawtooth 用
// 非带限版本，与浏览器在无 AudioWorklet 时的实现同档——差异在高次谐波，判据比对
// 用基频与包络，不要求逐样本相同）。
func wgOscSample(kind string, phase float64) float64 {
	switch kind {
	case "square":
		if phase < 0.5 {
			return 1
		}
		return -1
	case "sawtooth":
		return 2*phase - 1
	case "triangle":
		if phase < 0.25 {
			return 4 * phase
		}
		if phase < 0.75 {
			return 2 - 4*phase
		}
		return 4*phase - 4
	default: // sine
		return math.Sin(2 * math.Pi * phase)
	}
}

// renderBufferSource 渲染 AudioBufferSourceNode：按 playbackRate/detune 推进读指针
// （以**源采样帧**计），支持 loop 与 start(when, offset, duration)/stop(when) 的
// 上下文时间语义。
//
// ★ 定位方式：量子起点的读位置由 `offset + (t - startAt) × rate` 解析给出，量子内
//
//	再逐帧推进。这样同一段音频无论被拆成多少个量子渲染（实时 vs 离线的量子边界
//	不同），结果都由**上下文时间**决定，不依赖渲染调用序列。
func (g *wgGraph) renderBufferSource(n *wgNode, t float64, frames int) {
	g.clearOut(n, frames)
	if !n.srcOn || n.srcBuf == nil || len(n.srcBuf.Channels) == 0 {
		return
	}
	srcCh := n.srcBuf.Channels
	srcLen := len(srcCh[0])
	srcRate := float64(n.srcBuf.SampleRate)
	if srcRate <= 0 || srcLen == 0 {
		return
	}
	rate := n.params["playbackRate"]
	det := n.params["detune"]
	stepAt := func(tt float64) float64 {
		return rate.valueAt(tt) * math.Pow(2, det.valueAt(tt)/1200) // 源帧 / 输出帧
	}
	pos := n.srcOffset * srcRate
	if t > n.startAt {
		pos += (t - n.startAt) * srcRate * stepAt(t)
	}
	for i := 0; i < frames; i++ {
		tt := t + float64(i)/float64(g.sampleRate)
		if tt < n.startAt {
			continue // 尚未开始：输出保持 0（清空过）
		}
		if n.srcDur > 0 && (tt-n.startAt) >= n.srcDur {
			break
		}
		if pos >= float64(srcLen) {
			if n.srcLoop && n.loopEnd > n.loopStart {
				l0 := n.loopStart * srcRate
				l1 := n.loopEnd * srcRate
				if l1 <= l0 || l1 > float64(srcLen) {
					l1 = float64(srcLen)
				}
				span := l1 - l0
				if span > 0 {
					pos = l0 + math.Mod(pos-l0, span)
				} else {
					pos = l0
				}
			} else {
				break // 播完：其后样本为 0（规范：缓冲区耗尽后输出静音）
			}
		}
		i0 := int(pos)
		if i0 < 0 {
			i0 = 0
		}
		if i0 >= srcLen {
			break
		}
		i1 := i0 + 1
		if i1 >= srcLen {
			i1 = srcLen - 1
		}
		frac := pos - float64(i0)
		for c := 0; c < wgChannels; c++ {
			ch := srcCh[c%len(srcCh)]
			v := float64(ch[i0])*(1-frac) + float64(ch[i1])*frac
			n.out[c][i] = float32(v)
		}
		pos += stepAt(tt)
	}
}

// renderDelay 渲染 DelayNode：环形缓冲，写入当前量子、读出 delayTime 对应的样本
// （读写分离 ⇒ 反馈环不会产生 NaN）。delayTime 可自动化，读取位置按每帧线性插值。
func (g *wgGraph) renderDelay(n *wgNode, t float64, frames int) {
	g.sumInputs(n, frames)
	if n.delayBuf == nil {
		mx := 8 * g.sampleRate // 8s 上限（规范默认 maxDelayTime 1s；这里给足余量）
		n.delayBuf = make([][]float32, wgChannels)
		for c := range n.delayBuf {
			n.delayBuf[c] = make([]float32, mx+wgQuantum)
		}
	}
	bufLen := len(n.delayBuf[0])
	dt := n.params["delayTime"]
	base := n.delayPos
	// 先写入
	for c := 0; c < wgChannels; c++ {
		copy(n.delayBuf[c][base:base+frames], n.in[c][:frames])
	}
	for i := 0; i < frames; i++ {
		tt := t + float64(i)/float64(g.sampleRate)
		d := dt.valueAt(tt)
		if d < 0 {
			d = 0
		}
		delayFrames := d * float64(g.sampleRate)
		if delayFrames > float64(bufLen-wgQuantum-1) {
			delayFrames = float64(bufLen - wgQuantum - 1)
		}
		readPos := float64(base+i) - delayFrames
		for readPos < 0 {
			readPos += float64(bufLen)
		}
		r0 := int(readPos)
		r1 := (r0 + 1) % bufLen
		frac := float32(readPos - float64(r0))
		for c := 0; c < wgChannels; c++ {
			a, b := n.delayBuf[c][r0], n.delayBuf[c][r1]
			n.out[c][i] = a*(1-frac) + b*frac
		}
	}
	n.delayPos = (base + frames) % bufLen
}

// renderBiquad 渲染 BiquadFilterNode（RBJ cookbook 系数，Direct Form I）。
// type: lowpass | highpass | bandpass | lowshelf | highshelf | peaking | notch | allpass
func (g *wgGraph) renderBiquad(n *wgNode, t float64, frames int) {
	g.sumInputs(n, frames)
	fq := n.params["frequency"]
	q := n.params["Q"]
	gain := n.params["gain"]
	kind := n.bqTypeName
	if kind == "" {
		kind = "lowpass"
	}
	for i := 0; i < frames; i++ {
		tt := t + float64(i)/float64(g.sampleRate)
		b0, b1, b2, a1, a2 := wgBiquadCoeffs(kind, fq.valueAt(tt), q.valueAt(tt), gain.valueAt(tt), float64(g.sampleRate))
		for c := 0; c < wgChannels; c++ {
			x := float64(n.in[c][i])
			y := b0*x + b1*n.bq[c].x1 + b2*n.bq[c].x2 - a1*n.bq[c].y1 - a2*n.bq[c].y2
			n.bq[c].x2, n.bq[c].x1 = n.bq[c].x1, x
			n.bq[c].y2, n.bq[c].y1 = n.bq[c].y1, y
			n.out[c][i] = float32(y)
		}
	}
}

// wgBiquadCoeffs 按 RBJ audio EQ cookbook 计算双二阶系数（已归一化为 a0 = 1）。
func wgBiquadCoeffs(kind string, freq, q, gainDB, sampleRate float64) (b0, b1, b2, a1, a2 float64) {
	if freq < 0 {
		freq = 0
	}
	nyq := sampleRate / 2
	if freq > nyq {
		freq = nyq
	}
	w0 := 2 * math.Pi * freq / sampleRate
	cw, sw := math.Cos(w0), math.Sin(w0)
	if q <= 0 {
		q = 0.0001
	}
	alpha := sw / (2 * q)
	A := math.Pow(10, gainDB/40)
	switch kind {
	case "highpass":
		b0, b1, b2 = (1+cw)/2, -(1 + cw), (1+cw)/2
		a0 := 1 + alpha
		a1, a2 = -2*cw/a0, (1-alpha)/a0
		return b0 / a0, b1 / a0, b2 / a0, a1, a2
	case "bandpass":
		b0, b1, b2 = alpha, 0, -alpha
		a0 := 1 + alpha
		a1, a2 = -2*cw/a0, (1-alpha)/a0
		return b0 / a0, b1 / a0, b2 / a0, a1, a2
	case "notch":
		b0, b1, b2 = 1, -2*cw, 1
		a0 := 1 + alpha
		a1, a2 = -2*cw/a0, (1-alpha)/a0
		return b0 / a0, b1 / a0, b2 / a0, a1, a2
	case "allpass":
		b0, b1, b2 = 1-alpha, -2*cw, 1+alpha
		a0 := 1 + alpha
		a1, a2 = -2*cw/a0, (1-alpha)/a0
		return b0 / a0, b1 / a0, b2 / a0, a1, a2
	case "peaking":
		b0 = 1 + alpha*A
		b1 = -2 * cw
		b2 = 1 - alpha*A
		a0 := 1 + alpha/A
		a1, a2 = -2*cw/a0, (1-alpha/A)/a0
		return b0 / a0, b1 / a0, b2 / a0, a1, a2
	case "lowshelf":
		s := 2 * math.Sqrt(A) * alpha
		b0 = A * ((A + 1) - (A-1)*cw + s)
		b1 = 2 * A * ((A - 1) - (A+1)*cw)
		b2 = A * ((A + 1) - (A-1)*cw - s)
		a0 := (A + 1) + (A-1)*cw + s
		a1 = -2 * ((A - 1) + (A+1)*cw)
		a2 = (A + 1) + (A-1)*cw - s
		return b0 / a0, b1 / a0, b2 / a0, a1 / a0, a2 / a0
	case "highshelf":
		s := 2 * math.Sqrt(A) * alpha
		b0 = A * ((A + 1) + (A-1)*cw + s)
		b1 = -2 * A * ((A - 1) + (A+1)*cw)
		b2 = A * ((A + 1) + (A-1)*cw - s)
		a0 := (A + 1) - (A-1)*cw + s
		a1 = 2 * ((A - 1) - (A+1)*cw)
		a2 = (A + 1) - (A-1)*cw - s
		return b0 / a0, b1 / a0, b2 / a0, a1 / a0, a2 / a0
	default: // lowpass
		b0, b1, b2 = (1-cw)/2, 1-cw, (1-cw)/2
		a0 := 1 + alpha
		a1, a2 = -2*cw/a0, (1-alpha)/a0
		return b0 / a0, b1 / a0, b2 / a0, a1, a2
	}
}

// ─── 分析器（AnalyserNode）────────────────────────────────

// wgAnalyserState 是 AnalyserNode 的记录状态：最近 fftSize 个时域样本 + 平滑系数。
type wgAnalyserState struct {
	fftSize   int
	ring      [][]float32 // [ch][fftSize]，按 fftSize 循环
	pos       int
	smoothing float64
	lastMag   []float64 // 上一帧幅度谱（smoothing 用）
}

func newWGAnalyserState(fftSize int) *wgAnalyserState {
	if fftSize < 32 || fftSize > 32768 || (fftSize&(fftSize-1)) != 0 {
		fftSize = 2048
	}
	a := &wgAnalyserState{fftSize: fftSize, smoothing: 0.8}
	a.ring = make([][]float32, wgChannels)
	for c := range a.ring {
		a.ring[c] = make([]float32, fftSize)
	}
	a.lastMag = make([]float64, fftSize/2)
	return a
}

func (a *wgAnalyserState) push(out [][]float32, frames int) {
	if a == nil {
		return
	}
	for i := 0; i < frames; i++ {
		for c := 0; c < wgChannels && c < len(out); c++ {
			a.ring[c][a.pos] = out[c][i]
		}
		a.pos = (a.pos + 1) % a.fftSize
	}
}

// timeDomain 返回按时间顺序排列的最近 fftSize 个样本（通道 0）。
func (a *wgAnalyserState) timeDomain() []float32 {
	out := make([]float32, a.fftSize)
	n := a.fftSize
	for i := 0; i < n; i++ {
		out[i] = a.ring[0][(a.pos+i)%n]
	}
	return out
}

// magnitudes 做一次 FFT 并返回幅度谱（长度 fftSize/2），带 smoothing 与黑曼窗。
func (a *wgAnalyserState) magnitudes() []float64 {
	td := a.timeDomain()
	n := len(td)
	re := make([]float64, n)
	im := make([]float64, n)
	for i := 0; i < n; i++ {
		// Blackman 窗（与浏览器 AnalyserNode 的默认窗一致）。
		w := 0.42 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n-1)) + 0.08*math.Cos(4*math.Pi*float64(i)/float64(n-1))
		re[i] = float64(td[i]) * w
	}
	wgFFT(re, im, false)
	half := n / 2
	mag := make([]float64, half)
	s := a.smoothing
	for i := 0; i < half; i++ {
		m := math.Hypot(re[i], im[i]) / float64(n)
		if s > 0 && i < len(a.lastMag) {
			m = s*a.lastMag[i] + (1-s)*m
		}
		mag[i] = m
	}
	a.lastMag = mag
	return mag
}

// wgFFT 是原地 radix-2 Cooley–Tukey FFT（长度必须是 2 的幂）。
func wgFFT(re, im []float64, inverse bool) {
	n := len(re)
	if n <= 1 || n&(n-1) != 0 {
		return
	}
	// 位反转置换
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		ang := 2 * math.Pi / float64(length)
		if !inverse {
			ang = -ang
		}
		wr, wi := math.Cos(ang), math.Sin(ang)
		for i := 0; i < n; i += length {
			cr, ci := 1.0, 0.0
			for j := 0; j < length/2; j++ {
				ur, ui := re[i+j], im[i+j]
				vr, vi := re[i+j+length/2]*cr-im[i+j+length/2]*ci, re[i+j+length/2]*ci+im[i+j+length/2]*cr
				re[i+j], im[i+j] = ur+vr, ui+vi
				re[i+j+length/2], im[i+j+length/2] = ur-vr, ui-vi
				cr, ci = cr*wr-ci*wi, cr*wi+ci*wr
			}
		}
	}
	if inverse {
		for i := range re {
			re[i] /= float64(n)
			im[i] /= float64(n)
		}
	}
}
