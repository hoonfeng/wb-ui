package bindings

// WebAudio 最小子集（TC-M-603）的测试。
//
// 验收口径（docs/audio-backend-proposal.md §9.6 侦查给出的「最小面」定义）：
//  ① 特性检测过：全局有 AudioContext 构造器；
//  ② 最小可用：new AudioContext() → decodeAudioData(arrayBuffer) → then 拿到
//     AudioBuffer，且 sampleRate / length / numberOfChannels / duration /
//     getChannelData(0) 可用（后者必须是真 Float32Array）。
//
// 另覆盖失败路径（**不编造数据**：未装配解码器/解码失败/参数非法一律失败）与
// createBuffer、越界、时钟单调性等边界。真 ffmpeg 解码的端到端取证在 app 侧
// （app/webaudio.go）与探针侧，本文件用假解码器把「引擎把宿主 PCM 正确包成
// AudioBuffer」变成可精确断言的事。
//
// Promise 的决议在 Go 侧靠 rt.RunJobs() 驱动微任务队列（jsc 的 job 队列），
// 因此 `then` 的结果能同步读到。

import "testing"

// installFakeAudioDecoder 装配假解码器（并记录收到的字节），测试结束自动还原。
// 返回「最后一次收到的字节」的取值函数。
func installFakeAudioDecoder(t *testing.T, dec AudioDecoded, ok bool) func() []byte {
	t.Helper()
	prev := AudioDecoder
	var got []byte
	AudioDecoder = func(data []byte) (AudioDecoded, bool) {
		got = append([]byte(nil), data...)
		return dec, ok
	}
	t.Cleanup(func() { AudioDecoder = prev })
	return func() []byte { return got }
}

// settlePromises 驱动微任务队列，让此前排入的 Promise 回调执行。
func settlePromises(rt interface{ RunJobs() }) { rt.RunJobs() }

func TestWebAudioAudioContextFeatureDetection(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	// 特性检测是 TC-M-603 的第一条判据：「全局对象上有该构造器」即可。
	mustRun(t, rt, `
		if (typeof AudioContext !== "function") throw new Error("AudioContext 未注册: " + typeof AudioContext);
		if ("AudioContext" in this === false) throw new Error("AudioContext 不在全局对象上");
		if (typeof webkitAudioContext !== "function") throw new Error("webkitAudioContext 别名缺失");
	`)
}

func TestWebAudioAudioContextProperties(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	mustRun(t, rt, `
		var ctx = new AudioContext();
		if (ctx.sampleRate !== 48000) throw new Error("默认 sampleRate=" + ctx.sampleRate);
		if (ctx.state !== "running") throw new Error("初始 state=" + ctx.state);
		if (typeof ctx.currentTime !== "number") throw new Error("currentTime 不是数字");
		if (ctx.currentTime < 0) throw new Error("currentTime 为负: " + ctx.currentTime);
		if (!ctx.destination || ctx.destination.numberOfInputs !== 1) throw new Error("destination 形态不对");

		// 显式 sampleRate 生效。
		var ctx2 = new AudioContext({ sampleRate: 22050 });
		if (ctx2.sampleRate !== 22050) throw new Error("显式 sampleRate=" + ctx2.sampleRate);
	`)
}

func TestWebAudioCurrentTimeMonotonicAndSuspend(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	mustRun(t, rt, `
		var ctx = new AudioContext();
		ctx.suspend().then(function(){ ctx.__suspended = true; });
		ctx.__t0 = ctx.currentTime;
		if (ctx.state !== "suspended") throw new Error("suspend 后 state=" + ctx.state);
		if (ctx.currentTime < ctx.__t0) throw new Error("currentTime 回退");
		ctx.resume();
		if (ctx.state !== "running") throw new Error("resume 后 state=" + ctx.state);
		ctx.close();
		if (ctx.state !== "closed") throw new Error("close 后 state=" + ctx.state);
		// close 之后 resume 不该把状态拉回 running（规范：closed 是终态）。
		ctx.resume();
		if (ctx.state !== "closed") throw new Error("close 后 resume 复活了上下文");
	`)
	settlePromises(rt)
	mustRun(t, rt, `
		if (!ctx.__suspended) throw new Error("suspend() 的 Promise 未决议");
	`)
}

func TestWebAudioDecodeAudioDataPromise(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	installFakeAudioDecoder(t, AudioDecoded{
		SampleRate: 44100,
		Channels:   [][]float32{{0, 0.5, -1, 1}, {1, -1, 0.25, 0}},
	}, true)
	mustRun(t, rt, `
		var __ctx = new AudioContext();
		var __bytes = new Uint8Array([82, 73, 70, 70]);
		var __buf = null, __err = null;
		__ctx.decodeAudioData(__bytes.buffer).then(function(b){ __buf = b; }, function(e){ __err = e; });
	`)
	settlePromises(rt)
	mustRun(t, rt, `
		if (__err) throw new Error("不应失败: " + __err.name + ": " + __err.message);
		if (!__buf) throw new Error("Promise 未决议出 AudioBuffer");
		if (__buf.sampleRate !== 44100) throw new Error("sampleRate=" + __buf.sampleRate);
		if (__buf.length !== 4) throw new Error("length=" + __buf.length);
		if (__buf.numberOfChannels !== 2) throw new Error("numberOfChannels=" + __buf.numberOfChannels);
		if (Math.abs(__buf.duration - 4/44100) > 1e-9) throw new Error("duration=" + __buf.duration);
		var ch0 = __buf.getChannelData(0);
		if (!(ch0 instanceof Float32Array)) throw new Error("getChannelData 返回的不是 Float32Array");
		if (ch0.length !== 4) throw new Error("ch0.length=" + ch0.length);
		if (Math.abs(ch0[1] - 0.5) > 1e-6) throw new Error("ch0[1]=" + ch0[1]);
		if (Math.abs(ch0[2] + 1) > 1e-6) throw new Error("ch0[2]=" + ch0[2]);
		var ch1 = __buf.getChannelData(1);
		if (!(ch1 instanceof Float32Array)) throw new Error("ch1 不是 Float32Array");
		if (Math.abs(ch1[3]) > 1e-6) throw new Error("ch1[3]=" + ch1[3]);
		if (__buf.constructor.name !== "AudioBuffer") throw new Error("constructor.name=" + __buf.constructor.name);
	`)
}

func TestWebAudioDecodeAudioDataReadsTypedArrayView(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	got := installFakeAudioDecoder(t, AudioDecoded{SampleRate: 8000, Channels: [][]float32{{0, 0}}}, true)
	// subarray 视图：byteOffset=2、byteLength=4 ⇒ 引擎必须按视图切片，而不是把
	// 整个底层 buffer 交出去（规范：decodeAudioData 接受 TypedArray）。
	mustRun(t, rt, `
		var ctx = new AudioContext();
		var whole = new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8]);
		var view = whole.subarray(2, 6);
		ctx.decodeAudioData(view);
	`)
	gotBytes := got()
	if len(gotBytes) != 4 {
		t.Fatalf("引擎应收到视图的 4 字节，实际 %d 字节：%v", len(gotBytes), gotBytes)
	}
	for i, want := range []byte{3, 4, 5, 6} {
		if gotBytes[i] != want {
			t.Fatalf("视图字节[%d] = %d，期望 %d（整段 %v）", i, gotBytes[i], want, gotBytes)
		}
	}
}

func TestWebAudioDecodeAudioDataRejectsWithoutDecoder(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	prev := AudioDecoder
	AudioDecoder = nil
	t.Cleanup(func() { AudioDecoder = prev })
	mustRun(t, rt, `
		var ctx = new AudioContext();
		var __err = null;
		var ok = false;
		ctx.decodeAudioData(new Uint8Array([1, 2, 3]).buffer).then(function(){ ok = true; },
			function(e){ __err = e; });
	`)
	settlePromises(rt)
	mustRun(t, rt, `
		if (ok) throw new Error("未装配解码器时不应成功");
		if (!__err) throw new Error("应当 reject");
		if (__err.name !== "EncodingError") throw new Error("name=" + __err.name);
	`)
}

func TestWebAudioDecodeAudioDataRejectsOnDecodeFailure(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	installFakeAudioDecoder(t, AudioDecoded{}, false)
	mustRun(t, rt, `
		var ctx = new AudioContext();
		var __err = null;
		ctx.decodeAudioData(new Uint8Array([9, 9, 9]).buffer).then(function(){ throw new Error("不应成功"); },
			function(e){ __err = e; });
	`)
	settlePromises(rt)
	mustRun(t, rt, `
		if (!__err) throw new Error("解码失败应 reject");
		if (__err.name !== "EncodingError") throw new Error("name=" + __err.name);
	`)
}

func TestWebAudioDecodeAudioDataTypeErrorOnBadArgument(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	installFakeAudioDecoder(t, AudioDecoded{SampleRate: 8000, Channels: [][]float32{{0}}}, true)
	mustRun(t, rt, `
		var ctx = new AudioContext();
		var __names = [];
		ctx.decodeAudioData().then(function(){ throw new Error("缺参数不该成功"); },
			function(e){ __names.push(e.name); });
		ctx.decodeAudioData("not-a-buffer").then(function(){ throw new Error("字符串不该成功"); },
			function(e){ __names.push(e.name); });
		ctx.decodeAudioData(new Uint8Array(0).buffer).then(function(){ throw new Error("空数据不该成功"); },
			function(e){ __names.push(e.name); });
	`)
	settlePromises(rt)
	mustRun(t, rt, `
		if (__names.length !== 3) throw new Error("只有一个 reject 生效: " + JSON.stringify(__names));
		if (__names[0] !== "TypeError" || __names[1] !== "TypeError") throw new Error("缺参数/非数组应为 TypeError: " + JSON.stringify(__names));
		if (__names[2] !== "EncodingError") throw new Error("空数据应为 EncodingError: " + __names[2]);
	`)
}

func TestWebAudioDecodeAudioDataCallbackForm(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	installFakeAudioDecoder(t, AudioDecoded{SampleRate: 16000, Channels: [][]float32{{0.25, 0.5}}}, true)
	// 旧式回调形态（规范早期版本，音视频库仍普遍使用）：成功回调收到 AudioBuffer。
	mustRun(t, rt, `
		var ctx = new AudioContext();
		var __cb = null;
		ctx.decodeAudioData(new Uint8Array([1, 2]).buffer, function(b){ __cb = b; });
		if (!__cb) throw new Error("成功回调未被调用");
		if (__cb.sampleRate !== 16000) throw new Error("回调收到 sampleRate=" + __cb.sampleRate);
		if (__cb.length !== 2) throw new Error("回调收到 length=" + __cb.length);
	`)
}

func TestWebAudioGetChannelDataOutOfRange(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	mustRun(t, rt, `
		var ctx = new AudioContext();
		var buf = ctx.createBuffer(1, 8, 8000);
		var threw = false;
		try { buf.getChannelData(1); } catch (e) { threw = true; }
		if (!threw) throw new Error("越界声道索引应当抛错");
		var threwNeg = false;
		try { buf.getChannelData(-1); } catch (e) { threwNeg = true; }
		if (!threwNeg) throw new Error("负索引应当抛错");
	`)
}

func TestWebAudioCreateBuffer(t *testing.T) {
	rt, _, _ := newRuntimeWithDoc(t)
	mustRun(t, rt, `
		var ctx = new AudioContext();
		var buf = ctx.createBuffer(2, 100, 8000);
		if (buf.numberOfChannels !== 2) throw new Error("numberOfChannels=" + buf.numberOfChannels);
		if (buf.length !== 100) throw new Error("length=" + buf.length);
		if (buf.sampleRate !== 8000) throw new Error("sampleRate=" + buf.sampleRate);
		if (Math.abs(buf.duration - 100/8000) > 1e-9) throw new Error("duration=" + buf.duration);
		var ch = buf.getChannelData(1);
		if (!(ch instanceof Float32Array)) throw new Error("createBuffer 的声道不是 Float32Array");
		if (ch.length !== 100) throw new Error("声道长度=" + ch.length);
		// 新建 buffer 是静音（全 0），与规范一致。
		for (var i = 0; i < ch.length; i++) {
			if (ch[i] !== 0) throw new Error("样本[" + i + "] = " + ch[i] + "（应为 0）");
		}
	`)
}
