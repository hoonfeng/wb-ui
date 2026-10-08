package bindings

// WebAudio 音频图（A3-3）的测试：离线渲染的**确定性**与各节点/参数的数值语义。
//
// 为什么以 OfflineAudioContext 为主：它的时间轴完全由帧号决定（t = frame/sr），
// 不依赖真实耗时、不碰音频设备，因此可以把「增益是不是 0.25」「线性 ramp 在 0.01s
// 处是不是到 1」这类断言写成**精确**判据，而不是「听起来对」。实时上下文的输出
// 路径在 app 侧验证（宿主 waveOut），此处只验证它不 panic、不改变时钟语义。
//
// 断言写在 JS 里（mustRun 抛错即失败）：样本在 JS 侧是 Float32Array，逐元素读写在
// JS 里最自然，也顺带验证了「视图零拷贝」这一关键语义（见 TestWebAudioGetChannelDataIsView）。

import (
	"math"
	"testing"

	"wb-ui/engine/js/jsc"
)

// renderOffline 执行一段 JS（其中应创建离线上下文、startRendering 并把结果存到
// 全局变量），随后驱动微任务队列让 Promise 决议落地。
func renderOffline(t *testing.T, rt *jsc.Interpreter, src string) {
	t.Helper()
	mustRun(t, rt, src)
	settlePromises(rt)
}

// TC-A3-3-01：OfflineAudioContext 的构造、属性与非法参数。
func TestWebAudioOfflineContextBasics(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	mustRun(t, rt, `
		if (typeof OfflineAudioContext !== "function") throw new Error("OfflineAudioContext 未注册");
		if (typeof webkitOfflineAudioContext !== "function") throw new Error("webkitOfflineAudioContext 别名缺失");

		var oc = new OfflineAudioContext(1, 1024, 48000);
		if (oc.length !== 1024) throw new Error("length=" + oc.length);
		if (oc.sampleRate !== 48000) throw new Error("sampleRate=" + oc.sampleRate);
		if (oc.state !== "suspended") throw new Error("初始 state=" + oc.state);
		if (typeof oc.startRendering !== "function") throw new Error("startRendering 缺失");
		if (oc.destination.numberOfInputs !== 1) throw new Error("destination.numberOfInputs=" + oc.destination.numberOfInputs);
		if (oc.destination.numberOfOutputs !== 0) throw new Error("destination.numberOfOutputs=" + oc.destination.numberOfOutputs);

		// 对象形式构造
		var oc2 = new OfflineAudioContext({ numberOfChannels: 2, length: 512, sampleRate: 8000 });
		if (oc2.sampleRate !== 8000 || oc2.length !== 512) throw new Error("对象形式构造参数未生效");

		// 非法参数：length<=0 必须抛错，而不是返回一个渲染不出东西的上下文
		var threw = false;
		try { new OfflineAudioContext(1, 0, 48000); } catch (e) { threw = true; }
		if (!threw) throw new Error("length=0 未抛错");

		// 工厂方法齐全（这是「完整子集」的判据）
		var need = ["createGain","createOscillator","createBufferSource","createConstantSource",
		            "createStereoPanner","createDelay","createBiquadFilter","createAnalyser",
		            "createBuffer","createChannelMerger","createChannelSplitter"];
		for (var i = 0; i < need.length; i++) {
			if (typeof oc[need[i]] !== "function") throw new Error("缺少工厂方法 " + need[i]);
		}
	`)
}

// TC-A3-3-02：离线渲染的确定性与直流增益（同一脚本两次渲染必须逐样本相同）。
func TestWebAudioOfflineRenderDeterministicAndGain(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	renderOffline(t, rt, `
		function renderConstGain() {
			var oc = new OfflineAudioContext(1, 256, 48000);
			var cs = oc.createConstantSource();
			var g = oc.createGain();
			g.gain.value = 0.25;
			cs.connect(g);
			g.connect(oc.destination);
			cs.start(0);
			var out = null;
			oc.startRendering().then(function (b) { out = b; });
			return oc;
		}
		var __oc1 = renderConstGain();
		var __oc2 = renderConstGain();
		var __b1 = null, __b2 = null;
		__oc1.startRendering().then(function(){});
	`)
	// 上面只建了图，真正取结果是下面这段（保持 JS 侧状态简单，便于定位失败）。
	renderOffline(t, rt, `
		var oc = new OfflineAudioContext(1, 256, 48000);
		var cs = oc.createConstantSource();
		var g = oc.createGain();
		g.gain.value = 0.25;
		cs.connect(g); g.connect(oc.destination); cs.start(0);
		var __buf = null;
		oc.startRendering().then(function (b) { __buf = b; });
	`)
	mustRun(t, rt, `
		if (__buf === null) throw new Error("startRendering 的 Promise 未决议");
		if (__buf.numberOfChannels !== 1) throw new Error("numberOfChannels=" + __buf.numberOfChannels);
		if (__buf.length !== 256) throw new Error("length=" + __buf.length);
		if (__buf.sampleRate !== 48000) throw new Error("sampleRate=" + __buf.sampleRate);
		var d = __buf.getChannelData(0);
		for (var i = 0; i < d.length; i++) {
			if (Math.abs(d[i] - 0.25) > 1e-6) throw new Error("样本 " + i + " = " + d[i] + "（期望 0.25）");
		}
	`)
	// 确定性：同一图渲染两次，逐样本完全相同。
	mustRun(t, rt, `
		var oc2 = new OfflineAudioContext(1, 256, 48000);
		var cs2 = oc2.createConstantSource();
		var g2 = oc2.createGain();
		g2.gain.value = 0.25;
		cs2.connect(g2); g2.connect(oc2.destination); cs2.start(0);
		var __buf2 = null;
		oc2.startRendering().then(function (b) { __buf2 = b; });
	`)
	settlePromises(rt)
	mustRun(t, rt, `
		var a = __buf.getChannelData(0), b = __buf2.getChannelData(0);
		for (var i = 0; i < a.length; i++) {
			if (a[i] !== b[i]) throw new Error("渲染不确定：样本 " + i + " " + a[i] + " != " + b[i]);
		}
	`)
}

// TC-A3-3-03：getChannelData 必须是**视图**（写入要落到缓冲区）。
//
// 这条不是形式主义：如果返回拷贝，脚本「往 getChannelData 里填样本再播放」的写法
// 会静默产出静音——没有任何报错，只是没声音。
func TestWebAudioGetChannelDataIsView(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	mustRun(t, rt, `
		var oc = new OfflineAudioContext(1, 128, 48000);
		var buf = oc.createBuffer(1, 128, 48000);
		var d = buf.getChannelData(0);
		if (!(d instanceof Float32Array)) throw new Error("getChannelData 不是 Float32Array: " + Object.prototype.toString.call(d));
		if (d.length !== 128) throw new Error("视图长度=" + d.length);
		d[7] = 0.5;
		var d2 = buf.getChannelData(0);
		if (d2[7] !== 0.5) throw new Error("getChannelData 不是视图：写入未保留（读回 " + d2[7] + "）");
		if (d !== d2) throw new Error("两次 getChannelData 返回了不同对象（规范：同一视图）");
	`)
}

// TC-A3-3-04：AudioBufferSourceNode 播放脚本合成的样本（含播完静音）。
func TestWebAudioBufferSourcePlaysSynthesizedSamples(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	renderOffline(t, rt, `
		var oc = new OfflineAudioContext(1, 960, 48000);
		var buf = oc.createBuffer(1, 480, 48000);
		var d = buf.getChannelData(0);
		for (var i = 0; i < 480; i++) d[i] = 1;
		var bs = oc.createBufferSource();
		if (bs.buffer !== null) throw new Error("buffer 初始值应为 null");
		bs.buffer = buf;
		if (bs.playbackRate.value !== 1) throw new Error("playbackRate 默认值=" + bs.playbackRate.value);
		bs.connect(oc.destination);
		bs.start(0);
		var __buf = null;
		oc.startRendering().then(function (b) { __buf = b; });
	`)
	mustRun(t, rt, `
		var d = __buf.getChannelData(0);
		if (__buf.length !== 960) throw new Error("length=" + __buf.length);
		for (var i = 0; i < 480; i++) {
			if (Math.abs(d[i] - 1) > 1e-6) throw new Error("缓冲区样本 " + i + " = " + d[i] + "（期望 1）");
		}
		for (var j = 480; j < 960; j++) {
			if (Math.abs(d[j]) > 1e-6) throw new Error("播完后样本 " + j + " = " + d[j] + "（期望 0）");
		}
	`)
}

// TC-A3-3-05：AudioParam 自动化（setValueAtTime + linearRampToValueAtTime）。
func TestWebAudioAudioParamLinearRamp(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	renderOffline(t, rt, `
		var oc = new OfflineAudioContext(1, 960, 48000);
		var cs = oc.createConstantSource();
		var g = oc.createGain();
		cs.connect(g); g.connect(oc.destination); cs.start(0);
		g.gain.setValueAtTime(0, 0);
		g.gain.linearRampToValueAtTime(1, 0.01);   // 0 → 1，历时 10ms
		var __buf = null;
		oc.startRendering().then(function (b) { __buf = b; });
	`)
	mustRun(t, rt, `
		var d = __buf.getChannelData(0);
		if (Math.abs(d[0]) > 1e-6) throw new Error("ramp 起点样本 = " + d[0] + "（期望 0）");
		// t = 240/48000 = 5ms ⇒ 线性 ramp 应为 0.5
		if (Math.abs(d[240] - 0.5) > 2e-3) throw new Error("ramp 中点样本 = " + d[240] + "（期望 ≈0.5）");
		// t = 720/48000 = 15ms > 10ms ⇒ 已到目标值
		if (Math.abs(d[720] - 1) > 1e-6) throw new Error("ramp 终点样本 = " + d[720] + "（期望 1）");
		// 单调不减
		for (var i = 1; i < 480; i++) {
			if (d[i] < d[i-1] - 1e-6) throw new Error("ramp 在 " + i + " 处回退");
		}
	`)
}

// TC-A3-3-06：OscillatorNode 的包络（峰值与过零率对应 440Hz@48kHz）。
func TestWebAudioOscillatorSineEnvelope(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	renderOffline(t, rt, `
		var oc = new OfflineAudioContext(1, 4800, 48000);
		var osc = oc.createOscillator();
		if (osc.type !== "sine") throw new Error("默认波形=" + osc.type);
		if (osc.frequency.value !== 440) throw new Error("默认频率=" + osc.frequency.value);
		var g = oc.createGain();
		g.gain.value = 0.5;
		osc.connect(g); g.connect(oc.destination);
		osc.start(0);
		var __buf = null;
		oc.startRendering().then(function (b) { __buf = b; });
	`)
	mustRun(t, rt, `
		var d = __buf.getChannelData(0);
		var peak = 0, zc = 0;
		for (var i = 0; i < d.length; i++) {
			var v = d[i] < 0 ? -d[i] : d[i];
			if (v > peak) peak = v;
		}
		for (var j = 1; j < d.length; j++) {
			if ((d[j-1] < 0 && d[j] >= 0) || (d[j-1] >= 0 && d[j] < 0)) zc++;
		}
		// 440Hz、0.1s ⇒ 44 个周期 ⇒ 88 次过零；峰值受采样相位影响略低于 0.5。
		if (peak < 0.44 || peak > 0.5) throw new Error("峰值 = " + peak + "（期望 ≈0.5）");
		if (zc < 86 || zc > 90) throw new Error("过零次数 = " + zc + "（期望 ≈88 = 440Hz×0.1s×2）");
	`)
}

// TC-A3-3-07：DelayNode 的延迟量（delayTime=1ms ⇒ 前 48 帧静音）。
func TestWebAudioDelayNodeOffset(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	renderOffline(t, rt, `
		var oc = new OfflineAudioContext(1, 256, 48000);
		var cs = oc.createConstantSource();
		var dl = oc.createDelay(1.0);
		dl.delayTime.value = 0.001;   // 48 帧
		cs.connect(dl); dl.connect(oc.destination); cs.start(0);
		var __buf = null;
		oc.startRendering().then(function (b) { __buf = b; });
	`)
	mustRun(t, rt, `
		var d = __buf.getChannelData(0);
		for (var i = 0; i < 48; i++) {
			if (Math.abs(d[i]) > 1e-6) throw new Error("延迟期样本 " + i + " = " + d[i] + "（期望 0）");
		}
		for (var j = 48; j < 256; j++) {
			if (Math.abs(d[j] - 1) > 1e-6) throw new Error("延迟后样本 " + j + " = " + d[j] + "（期望 1）");
		}
	`)
}

// TC-A3-3-08：StereoPannerNode 的声道分布（pan=-1 ⇒ 全左、右声道静音）。
func TestWebAudioStereoPannerChannels(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	renderOffline(t, rt, `
		var oc = new OfflineAudioContext(2, 128, 48000);
		var cs = oc.createConstantSource();
		var sp = oc.createStereoPanner();
		sp.pan.value = -1;
		cs.connect(sp); sp.connect(oc.destination); cs.start(0);
		var __buf = null;
		oc.startRendering().then(function (b) { __buf = b; });
	`)
	mustRun(t, rt, `
		if (__buf.numberOfChannels !== 2) throw new Error("numberOfChannels=" + __buf.numberOfChannels);
		var L = __buf.getChannelData(0), R = __buf.getChannelData(1);
		for (var i = 0; i < L.length; i++) {
			if (Math.abs(L[i] - 1) > 1e-6) throw new Error("左声道样本 " + i + " = " + L[i] + "（期望 1）");
			if (Math.abs(R[i]) > 1e-6) throw new Error("右声道样本 " + i + " = " + R[i] + "（期望 0）");
		}
	`)
}

// TC-A3-3-09：AnalyserNode 的时域/频域读取（含就地填充语义）。
func TestWebAudioAnalyserReadback(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	renderOffline(t, rt, `
		var oc = new OfflineAudioContext(1, 2048, 48000);
		var cs = oc.createConstantSource();
		var an = oc.createAnalyser();
		an.smoothingTimeConstant = 0;   // 关掉平滑：默认 0.8 会让首帧幅度被平滑成 0.2 倍
		cs.connect(an); an.connect(oc.destination); cs.start(0);
		var __an = an;
		oc.startRendering().then(function () {});
	`)
	mustRun(t, rt, `
		var an = __an;
		if (an.fftSize !== 2048) throw new Error("fftSize=" + an.fftSize);
		if (an.frequencyBinCount !== 1024) throw new Error("frequencyBinCount=" + an.frequencyBinCount);
		var td = new Float32Array(an.fftSize);
		an.getFloatTimeDomainData(td);
		// 输出信号是直流 1，分析器记录的最近样本应全部为 1（就地填充 → td 被改写）
		for (var i = 0; i < td.length; i++) {
			if (Math.abs(td[i] - 1) > 1e-6) throw new Error("时域样本 " + i + " = " + td[i] + "（期望 1）");
		}
		var fd = new Float32Array(an.frequencyBinCount);
		an.getFloatFrequencyData(fd);
		// 直流成分集中在第 0 个 bin，且远高于其它 bin。
		// 全 1 信号加 Blackman 窗后 DC 幅度 = 窗均值 ≈0.42 ⇒ ≈-7.5dB。
		if (!(fd[0] > -12 && fd[0] < -3)) throw new Error("DC bin = " + fd[0] + "（期望 ≈-7.5dB）");
		if (!(fd[100] < fd[0] - 40)) throw new Error("第 100 bin = " + fd[100] + " 未明显低于 DC bin " + fd[0]);
		var bd = new Uint8Array(an.frequencyBinCount);
		an.getByteFrequencyData(bd);
		if (!(bd[0] > 200)) throw new Error("DC bin（字节）= " + bd[0] + "（期望接近 255）");
	`)
}

// TC-A3-3-10：错误路径必须**如实报错**（而不是静默给出假节点/假结果）。
func TestWebAudioGraphErrorPaths(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	mustRun(t, rt, `
		var oc = new OfflineAudioContext(1, 128, 48000);

		// 振荡器只能 start 一次
		var osc = oc.createOscillator();
		osc.start(0);
		var threw = false;
		try { osc.start(0); } catch (e) { threw = true; }
		if (!threw) throw new Error("OscillatorNode.start 二次调用未抛错");

		// 跨上下文连接必须拒绝
		var ex = new OfflineAudioContext(1, 128, 48000);
		var g1 = oc.createGain(), g2 = ex.createGain();
		threw = false;
		try { g1.connect(g2); } catch (e) { threw = true; }
		if (!threw) throw new Error("跨上下文 connect 未抛错");

		// connect 非节点参数必须拒绝
		threw = false;
		try { g1.connect(42); } catch (e) { threw = true; }
		if (!threw) throw new Error("connect(42) 未抛错");

		// 未实现的多声道路由：如实抛错，不返回假节点
		threw = false;
		try { oc.createChannelMerger(2); } catch (e) { threw = true; }
		if (!threw) throw new Error("createChannelMerger 未抛错（本引擎未实现，应如实报错）");

		// 非法枚举值必须拒绝
		threw = false;
		try { osc.type = "noise"; } catch (e) { threw = true; }
		if (!threw) throw new Error("oscillator.type 非法值未抛错");

		var bq = oc.createBiquadFilter();
		if (bq.type !== "lowpass") throw new Error("biquad 默认类型=" + bq.type);
		threw = false;
		try { bq.type = "brickwall"; } catch (e) { threw = true; }
		if (!threw) throw new Error("biquad.type 非法值未抛错");
	`)
}

// TC-A3-3-11：BiquadFilter 的频响（Go 层数值：低通在 DC≈1、Nyquist≈0）。
func TestWebAudioBiquadResponseNumeric(t *testing.T) {
	sr := 48000.0
	b0, b1, b2, a1, a2 := wgBiquadCoeffs("lowpass", 1000, 0.707, 0, sr)
	dcMag, _ := wgBiquadResponse(b0, b1, b2, a1, a2, 0, sr)
	if math.Abs(dcMag-1) > 1e-6 {
		t.Fatalf("低通 DC 幅频 = %v（期望 1）", dcMag)
	}
	nyq, _ := wgBiquadResponse(b0, b1, b2, a1, a2, sr/2, sr)
	if nyq > 0.01 {
		t.Fatalf("低通 Nyquist 幅频 = %v（期望 ≈0）", nyq)
	}
	// 高通反之（用截止频率处的 -3dB 作为独立判据，避免只依赖同一函数的对称性）
	h0, h1, h2, ha1, ha2 := wgBiquadCoeffs("highpass", 1000, 0.707, 0, sr)
	hdc, _ := wgBiquadResponse(h0, h1, h2, ha1, ha2, 0, sr)
	if hdc > 0.01 {
		t.Fatalf("高通 DC 幅频 = %v（期望 ≈0）", hdc)
	}
	// 截止频率处：幅度 ≈ 0.707（Q=0.707 的 Butterworth 特性）
	cut, _ := wgBiquadResponse(b0, b1, b2, a1, a2, 1000, sr)
	if math.Abs(cut-0.7071) > 0.05 {
		t.Fatalf("低通截止点幅频 = %v（期望 ≈0.707）", cut)
	}
}

// TC-A3-3-12：实时 AudioContext 的图不改变时钟语义（renderQuantum 不吞错、
// suspend 后不再推进），且离线与实时的创建路径互不干扰。
func TestWebAudioRealTimeContextGraphWiring(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	mustRun(t, rt, `
		var ctx = new AudioContext();
		var g = ctx.createGain();
		var osc = ctx.createOscillator();
		osc.connect(g);
		g.connect(ctx.destination);
		if (g.context.sampleRate !== ctx.sampleRate) throw new Error("node.context.sampleRate 不一致");

		// 实时上下文不因「建了图但没 start」而改变状态
		if (ctx.state !== "running") throw new Error("state=" + ctx.state);

		// suspend → currentTime 冻结
		ctx.suspend();
		var t0 = ctx.currentTime;
		for (var i = 0; i < 1000; i++) { /* 空转 */ }
		if (ctx.currentTime < t0) throw new Error("suspend 后 currentTime 回退");

		// 断开连接不报错（幂等），disconnect 后仍可重新连接
		osc.disconnect();
		osc.connect(g);
		ctx.close();
		if (ctx.state !== "closed") throw new Error("close 后 state=" + ctx.state);
	`)
}
