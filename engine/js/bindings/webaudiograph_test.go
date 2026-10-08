package bindings

// 音频图内核的**直连**测试：不经 JS 绑定层，直接建图、连接、渲染量子，检查样本。
//
// 为什么要这一层：JS 绑定层的失败（样本为 0）可能是「绑定没接上」也可能是「内核
// 没产出」，只从 JS 侧看是同一个现象。直连测试把两者分开——内核错了先在内核修，
// 绑定错了再用 webaudio_graph_test.go 定位。

import (
	"math"
	"testing"
)

// newTestCtx 建一个只用于内核测试的离线上下文（无 JS 运行时）。
func newTestCtx(sampleRate, length, channels int) *wgContext {
	return newWGContext(nil, sampleRate, true, length, channels)
}

// renderAll 渲染 frames 帧，返回声道 0 的样本。
func renderAll(c *wgContext, frames int) []float32 {
	out := make([]float32, 0, frames)
	pos := 0
	for pos < frames {
		n := wgQuantum
		if frames-pos < n {
			n = frames - pos
		}
		c.graph.renderQuantum(float64(pos)/float64(c.graph.sampleRate), n, c.pcm)
		out = append(out, c.pcm[0][:n]...)
		pos += n
	}
	return out
}

// TC-A3-3-K1：ConstantSource → Gain → destination 的直流增益。
func TestWGKernelConstantGainDirect(t *testing.T) {
	c := newTestCtx(48000, 256, 1)
	cs := c.graph.newNode(wgKConstantSource)
	cs.params["offset"] = newWGParam("offset", 1)
	g := c.graph.newNode(wgKGain)
	g.params["gain"] = newWGParam("gain", 0.25)
	c.graph.connect(cs, g)
	c.graph.connect(g, c.graph.dest)

	out := renderAll(c, 256)
	t.Logf("cs.out[0]=%v g.out[0]=%v dest.in[0]=%v out[0]=%v",
		cs.out[0][0], g.out[0][0], c.graph.dest.in[0][0], out[0])
	if math.Abs(float64(out[0])-0.25) > 1e-6 {
		t.Fatalf("输出样本 = %v（期望 0.25）", out[0])
	}
}

// TC-A3-3-K2：AudioParam 的线性 ramp 在内核层的时间精度。
func TestWGKernelParamLinearRamp(t *testing.T) {
	p := newWGParam("gain", 1)
	p.setValueAtTime(0, 0)
	p.linearRampToValueAtTime(1, 0.01)
	if v := p.valueAt(0); math.Abs(v) > 1e-9 {
		t.Fatalf("t=0 → %v（期望 0）", v)
	}
	if v := p.valueAt(0.005); math.Abs(v-0.5) > 1e-9 {
		t.Fatalf("t=0.005 → %v（期望 0.5）", v)
	}
	if v := p.valueAt(0.02); math.Abs(v-1) > 1e-9 {
		t.Fatalf("t=0.02 → %v（期望 1）", v)
	}
}

// TC-A3-3-K3：OscillatorNode 的频率精度（过零率必须与设置频率一致）。
//
// 这条独立于绑定层，用来判定「过零率偏低」是内核相位推进的问题还是绑定层的问题。
func TestWGKernelOscillatorFrequency(t *testing.T) {
	c := newTestCtx(48000, 4800, 1)
	osc := c.graph.newNode(wgKOscillator)
	osc.oscType = "sine"
	osc.params["frequency"] = newWGParam("frequency", 440)
	osc.params["detune"] = newWGParam("detune", 0)
	osc.started, osc.startAt = true, 0
	c.graph.connect(osc, c.graph.dest)

	out := renderAll(c, 4800)
	peak, zc := 0.0, 0
	for i, v := range out {
		if a := math.Abs(float64(v)); a > peak {
			peak = a
		}
		if i > 0 && ((out[i-1] < 0 && v >= 0) || (out[i-1] >= 0 && v < 0)) {
			zc++
		}
	}
	t.Logf("峰值=%v 过零=%d（440Hz×0.1s 期望 88）", peak, zc)
	if zc < 86 || zc > 90 {
		t.Fatalf("过零 %d 次（期望 86..90）", zc)
	}
	if peak < 0.99 {
		t.Fatalf("峰值 %v（期望 ≈1）", peak)
	}
}

// TC-A3-3-K4：AudioBufferSourceNode 播放已填样本的缓冲区。
func TestWGKernelBufferSourceDirect(t *testing.T) {
	c := newTestCtx(48000, 960, 1)
	dec := AudioDecoded{SampleRate: 48000, Channels: [][]float32{make([]float32, 480)}}
	for i := range dec.Channels[0] {
		dec.Channels[0][i] = 1
	}
	bs := c.graph.newNode(wgKBufferSource)
	bs.params["playbackRate"] = newWGParam("playbackRate", 1)
	bs.params["detune"] = newWGParam("detune", 0)
	bs.srcBuf = &dec
	bs.srcOn = true
	bs.startAt = 0
	c.graph.connect(bs, c.graph.dest)

	out := renderAll(c, 960)
	t.Logf("out[0]=%v out[479]=%v out[480]=%v", out[0], out[479], out[480])
	if math.Abs(float64(out[0])-1) > 1e-6 {
		t.Fatalf("out[0] = %v（期望 1）", out[0])
	}
	if math.Abs(float64(out[480])) > 1e-6 {
		t.Fatalf("out[480] = %v（期望 0，缓冲区已播完）", out[480])
	}
}
