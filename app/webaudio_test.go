package app

// WebAudio 宿主解码器的测试（TC-M-603 的宿主半边）。
//
// 引擎侧的 AudioContext/AudioBuffer 对象模型由 engine/js/bindings/webaudio_test.go
// 用假解码器精确断言；这里补的是**真 ffmpeg** 那一半：decodeAudioBytes 把内存字节
// 解成 float32 PCM，采样率/声道/样本正确性必须实测，不能只看编译通过。
//
// 输入是测试自己合成的 WAV（不依赖 dev/media 的样本——样本不入库，且这里要的是
// 「已知答案」的信号：44100Hz 单声道 440Hz 正弦）。本机没有 ffmpeg 时 skip
// （与媒体链路的处理一致：没有外部解码器就不该假装通过）。

import (
	"bytes"
	"encoding/binary"
	"math"
	"os/exec"
	"testing"
)

// synthSineWAV 造一段 s16le 单声道 WAV：sampleRate、freq Hz、dur 秒的满幅正弦。
func synthSineWAV(sampleRate int, freq, dur float64) []byte {
	frames := int(float64(sampleRate) * dur)
	dataSize := frames * 2
	buf := bytes.NewBuffer(make([]byte, 0, 44+dataSize))
	w := func(v any) { _ = binary.Write(buf, binary.LittleEndian, v) }
	buf.WriteString("RIFF")
	w(uint32(36 + dataSize))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	w(uint32(16))
	w(uint16(1))                  // PCM
	w(uint16(1))                  // 单声道
	w(uint32(sampleRate))         // 采样率
	w(uint32(sampleRate * 2))     // 字节率
	w(uint16(2))                  // 块对齐
	w(uint16(16))                 // 位深
	buf.WriteString("data")
	w(uint32(dataSize))
	for i := 0; i < frames; i++ {
		s := math.Sin(2 * math.Pi * freq * float64(i) / float64(sampleRate))
		w(int16(s * 32767))
	}
	return buf.Bytes()
}

func TestDecodeAudioBytesWithRealFFmpeg(t *testing.T) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("本机无 ffmpeg：跳过真实解码用例（与媒体链路同一处理）")
	}
	const (
		rate  = 44100
		freq  = 440.0
		dur   = 0.25
		wantF = int(rate * dur) // 11025
	)
	dec, ok := decodeAudioBytes(bin, synthSineWAV(rate, freq, dur))
	if !ok {
		t.Fatal("真 ffmpeg 解码失败（synthSineWAV 是合法 PCM WAV）")
	}
	// ★ 源采样率与声道必须**原样保留**（decodeAudioData 的语义）：若这里变成了
	//   48k/2ch，说明解码命令误用了播放链路的统一格式参数。
	if dec.SampleRate != rate {
		t.Fatalf("sampleRate = %d，期望 %d（源采样率必须保留，不重采样）", dec.SampleRate, rate)
	}
	if len(dec.Channels) != 1 {
		t.Fatalf("声道数 = %d，期望 1", len(dec.Channels))
	}
	got := len(dec.Channels[0])
	if got < wantF-8 || got > wantF+8 {
		t.Fatalf("样本帧数 = %d，期望 ≈%d", got, wantF)
	}
	// 样本内容：满幅正弦 ⇒ 峰值 ≈1，且主峰频率 ≈440Hz（复用判据 A 的同一套 FFT
	// 工具——这是「解出来的确实是那段音频」而不是「解出了一堆 0」的证据）。
	peak := 0.0
	mono := make([]float64, got)
	for i, s := range dec.Channels[0] {
		f := float64(s)
		mono[i] = f
		if a := math.Abs(f); a > peak {
			peak = a
		}
	}
	if peak < 0.95 {
		t.Fatalf("样本峰值 = %.4f，期望 ≈1（解出的不是满幅正弦）", peak)
	}
	gotFreq, amp := DominantFrequency(mono, dec.SampleRate)
	if math.Abs(gotFreq-freq) > 5 {
		t.Fatalf("频谱主峰 = %.2fHz，期望 %.0fHz", gotFreq, freq)
	}
	if amp < 0.5 {
		t.Fatalf("主峰归一化幅度 = %.3f（样本过弱，不像是真解出了正弦）", amp)
	}
}

func TestDecodeAudioBytesFailures(t *testing.T) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("本机无 ffmpeg：跳过（失败路径同样要真跑才成立）")
	}
	// ① 不是媒体数据：必须失败（不编造 AudioBuffer）。
	if _, ok := decodeAudioBytes(bin, []byte("this is definitely not audio data")); ok {
		t.Fatal("非媒体字节不应解码成功")
	}
	// ② 空字节 / 无 ffmpeg：直接失败。
	if _, ok := decodeAudioBytes(bin, nil); ok {
		t.Fatal("空字节不应解码成功")
	}
	if _, ok := decodeAudioBytes("", synthSineWAV(8000, 440, 0.05)); ok {
		t.Fatal("ffmpeg 路径为空时不应解码成功")
	}
}
