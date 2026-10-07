package app

// 宿主侧音频链路测试（主线 A3-1）：
//
//	ffmpeg 解码 → PCM 块 → tap（判据 A 的取证口）→ 输出设备 → 设备位置
//
// 用**真的** ffmpeg 与**真的**输出设备（有就测、没有就跳过）：这一层的价值恰恰
// 在于「外部进程 + 平台 API」这两处无法用假替身证明的接缝。判据 A（PCM 的 FFT
// 主峰 = 样本频率）就是在这里落地成可复现断言的。

import (
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"wb-ui/engine/rendering"
)

// sineSample 返回 440Hz 正弦样本的绝对路径与 file:// URL。
// 样本由 dev/media/gen_samples.py 生成、不入库，缺失时跳过（不是失败）。
func sineSample(t *testing.T) (path string, url string) {
	t.Helper()
	rel := filepath.Join("..", "dev", "media", "samples", "sine-440-1s.wav")
	if _, err := os.Stat(rel); err != nil {
		t.Skipf("缺少音频样本 %s（先跑 python dev/media/gen_samples.py）：%v", rel, err)
	}
	abs, err := filepath.Abs(rel)
	if err != nil {
		t.Fatalf("取样本绝对路径失败：%v", err)
	}
	return abs, "file:///" + filepath.ToSlash(abs)
}

// pcmCollector 是 tap 的测试实现：按 src 累积 PCM 与交付帧数。
type pcmCollector struct {
	mu     sync.Mutex
	data   []byte
	format rendering.AudioFormat
	frames int64
}

func (c *pcmCollector) tap(_ string, format rendering.AudioFormat, _ int64, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.format != format {
		c.format = format
	}
	c.data = append(c.data, data...)
	c.frames += int64(len(data) / format.BytesPerFrame())
}

func (c *pcmCollector) snapshot() ([]byte, rendering.AudioFormat, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]byte, len(c.data))
	copy(out, c.data)
	return out, c.format, c.frames
}

// TestMediaAudioSessionDelivers440Hz 是判据 A 的宿主侧落地：会话把 440Hz 正弦
// 解成 PCM 交给输出（tap 取到的就是写进设备的那一份），FFT 主峰必须是 440Hz。
func TestMediaAudioSessionDelivers440Hz(t *testing.T) {
	_, url := sineSample(t)
	m := NewMediaAudio("")
	if m.bin == "" {
		t.Skip("本机没有 ffmpeg（宿主解码链路无法测试）")
	}
	col := &pcmCollector{}
	m.SetTap(col.tap)

	src := m.SessionSource(nil)
	sess, ok := src(url, 0)
	if !ok {
		t.Fatalf("会话应打开成功（%s 是含音轨的 wav）", url)
	}
	defer sess.Close()

	// 等到攒够一整个 FFT 窗口（16384 帧 ≈ 341ms）。
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, format, frames := col.snapshot()
		if format.Valid() && frames >= spectrumMaxSamples {
			break
		}
		if time.Now().After(deadline) {
			_, format, frames := col.snapshot()
			t.Fatalf("5 秒内应交付 ≥%d 帧 PCM，实际 %d 帧（format=%+v）",
				spectrumMaxSamples, frames, format)
		}
		time.Sleep(10 * time.Millisecond)
	}

	data, format, frames := col.snapshot()
	if format.SampleRate != audioPCMSampleRate || format.Channels != audioPCMChannels {
		t.Fatalf("交付格式应为 %dhz/%dch，实际 %+v",
			audioPCMSampleRate, audioPCMChannels, format)
	}
	mono := PCMToMono(data, format.Channels)
	freq, mag := DominantFrequency(mono, format.SampleRate)
	if math.Abs(freq-440) > 10 {
		t.Fatalf("PCM 频谱主峰应为 440Hz±10，实际 %.1fHz（幅度 %.3f）", freq, mag)
	}
	if mag < 0.05 {
		t.Fatalf("主峰幅度过小（%.3f），FFT 结果不可信", mag)
	}
	t.Logf("判据 A 宿主侧：交付 %d 帧、主峰 %.2fHz（幅度 %.3f）、输出设备=%v",
		frames, freq, mag, sess.HasOutput())

	// 播放时钟与交付量一致：已交付帧数换算的时长不应落后播放位置太多
	// （设备缓冲 4×100ms，加上解码超前，落差应在 1 秒内）。
	before := sess.Position()
	time.Sleep(300 * time.Millisecond)
	after := sess.Position()
	if after <= before {
		t.Fatalf("播放位置应推进：%v → %v", before, after)
	}
	_, _, frames2 := col.snapshot()
	delivered := float64(frames2) / float64(format.SampleRate)
	if delivered-after > 1.0 {
		t.Fatalf("交付量（%.2fs）不该比播放位置（%.2fs）超前太多（>1s 说明节流失效）",
			delivered, after)
	}
}

// TestOpenAudioDeviceReportsPosition 覆盖 Windows 输出后端本身：打开设备、
// 写静音、设备位置必须真的推进（这是「音频为主时钟」的硬件依据）。
func TestOpenAudioDeviceReportsPosition(t *testing.T) {
	format := rendering.AudioFormat{SampleRate: audioPCMSampleRate, Channels: audioPCMChannels}
	dev, ok := openAudioDevice(format)
	if !ok {
		t.Skip("本机/本平台没有可用的内置输出设备（降级模式：PCM 只交给 tap）")
	}
	defer dev.close()

	// 写 200ms 静音（16 位 0 = 静音，不会打扰环境）。
	chunkFrames := format.SampleRate / 5
	buf := make([]byte, chunkFrames*format.BytesPerFrame())
	for i := 0; i < 4; i++ {
		if err := dev.write(buf); err != nil {
			t.Fatalf("write 失败：%v", err)
		}
	}
	before := dev.playedFrames()
	deadline := time.Now().Add(3 * time.Second)
	for dev.playedFrames() == before {
		if time.Now().After(deadline) {
			t.Fatalf("3 秒内设备位置应推进（起始 %d 帧）", before)
		}
		time.Sleep(10 * time.Millisecond)
	}
	after := dev.playedFrames()
	if after < before {
		t.Fatalf("设备位置不应回退：%d → %d", before, after)
	}
	// flush 后位置归零（seek 依赖这一语义）。
	if err := dev.flush(); err != nil {
		t.Fatalf("flush 失败：%v", err)
	}
	if p := dev.playedFrames(); p != 0 {
		t.Fatalf("flush 后设备位置应归零，实际 %d", p)
	}
	t.Logf("waveOut：写 4×200ms，位置推进到 %d 帧后 flush 归零", after)
}
