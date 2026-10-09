package app

// 判据 B 的「播放侧」：用本平台内置输出后端播一段正弦，供 WASAPI loopback 采回。
//
// 为什么与引擎会话分开：判据 A 已经证明「引擎要写进输出设备的字节正确」（同一份
// 待输出 PCM 的 FFT）；判据 B 要补的是**端到端的另一半**——「这些字节真的从输出
// 端点出来了、并且能被采回」。那一步只需要「与引擎相同的输出后端 + 一段已知频率
// 的测试音」，不需要页面会话；用宿主输出后端直接播比为此拉一个 WebView 更轻、也
// 更容易与录音窗口对齐（先起录音、再起播放）。报告里如实标注测试音的来源。
import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	"wb-ui/engine/rendering"
)

// testToneAmplitude 是测试音的幅度（相对满幅）。取 0.8 而不是 1.0：满幅正弦在
// float→s16 的取整上容易进入削波区，一旦削波就会在频谱里出现谐波，污染「主峰是
// 不是期望频率」这条判据。0.8 与判据 A 样本的幅度同量级，不影响幅度对照的结论。
const testToneAmplitude = 0.8

// errNoAudioOutput 表示本平台/本机没有可用的内置输出设备。
var errNoAudioOutput = errors.New("audio: 本机没有可用的内置输出设备")

// PlayTestTone 用内置输出后端播放 hz 频率的正弦 seconds 秒，**阻塞到设备真的把
// 它播完**（不只是写完缓冲）。格式非法时用 48kHz/2ch（与 A3-1 的宿主默认一致）。
func PlayTestTone(hz, seconds float64, format rendering.AudioFormat) error {
	if !format.Valid() {
		format = rendering.AudioFormat{SampleRate: 48000, Channels: 2}
	}
	if hz <= 0 || seconds <= 0 {
		return fmt.Errorf("audio: 非法测试音参数（%.1fHz %.2fs）", hz, seconds)
	}
	dev, ok := openAudioDevice(format)
	if !ok {
		return errNoAudioOutput
	}
	pcm := sinePCM(hz, seconds, format)
	if err := dev.write(pcm); err != nil {
		_ = dev.close()
		return err
	}
	// write 只保证「进了设备缓冲」；判据 B 的录音窗口要覆盖整段，所以再等设备把
	// 已提交的帧播完（playedFrames 是设备位置，不是提交量）。
	total := int64(float64(format.SampleRate) * seconds)
	deadline := time.Now().Add(3 * time.Second)
	for dev.playedFrames() < total && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	return dev.close()
}

// sinePCM 生成 s16le 交错的正弦 PCM（幅度 testToneAmplitude）。
func sinePCM(hz, seconds float64, format rendering.AudioFormat) []byte {
	frames := int(seconds * float64(format.SampleRate))
	buf := make([]byte, frames*format.Channels*2)
	for i := 0; i < frames; i++ {
		v := math.Sin(2*math.Pi*hz*float64(i)/float64(format.SampleRate)) * testToneAmplitude
		s := int16(math.Round(v * 32767))
		for c := 0; c < format.Channels; c++ {
			off := (i*format.Channels + c) * 2
			binary.LittleEndian.PutUint16(buf[off:], uint16(s))
		}
	}
	return buf
}
