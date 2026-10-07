package app

// 宿主侧 WebAudio 解码器（TC-M-603 的宿主半边）：把 `decodeAudioData` 收到的
// 内存字节交给 ffmpeg 解成 float32 PCM。
//
// 分工与 A3 音频链路完全一致（路线 (a)：引擎不背解码器）：
//
//	内存字节 → 临时文件（ffmpeg 只认路径/管道）→ ffmpeg -vn -f f32le -
//	  → stderr 解析源流的采样率与声道数 → 交错 float32 拆成各声道 → AudioDecoded
//
// 为什么与播放链路不同、**保留源采样率与声道**（不统一转 48kHz/2ch）：decodeAudioData
// 的语义是「解出这段数据的音频内容」，AudioBuffer.sampleRate 必须是源采样率才与
// 浏览器一致（Chromium 即如此）；重采样属于播放期（AudioContext 输出）的职责，
// 不属于解码。播放链路的 48kHz/2ch 是为了喂输出设备，两者目的不同。
import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"wb-ui/engine/js/bindings"
)

// audioDecodeTimeout 是单次 WebAudio 解码的超时。比元数据探测（mediaProbeTimeout）
// 宽松：解码要真读完整段数据。超时即按解码失败处理（返回 false → JS 侧 reject），
// 不把 JS 线程挂死在一次坏输入上。
const audioDecodeTimeout = 30 * time.Second

// InstallWebAudio 把宿主解码器接到引擎的 WebAudio 通道上（即 dom 装配段挂出的
// AudioContext 由此获得解码能力）。ffmpegPath 为空时从 PATH 查找。
//
// ffmpeg 不可用时**仍然装配**：每次解码返回 false ⇒ decodeAudioData 以 EncodingError
// reject —— 如实失败，不假装解码成功（与媒体链路「没有 ffmpeg 就保持时长未知」
// 同一纪律）。
func InstallWebAudio(ffmpegPath string) *MediaAudio {
	m := NewMediaAudio(ffmpegPath)
	bindings.AudioDecoder = func(data []byte) (bindings.AudioDecoded, bool) {
		return decodeAudioBytes(m.bin, data)
	}
	return m
}

// ─── 临时文件（按内容摘要命名）────────────────────────────

var (
	audioDecodeMu    sync.Mutex
	audioDecodePaths = map[[32]byte]string{}   // 内容摘要 → 临时文件路径
	audioDecodeFiles = map[string]struct{}{}   // 本进程写下的文件（供收尾清理）
)

// audioDecodeTempFile 把字节落盘并返回路径。命名 = 内容摘要（sha256 前 8 字节）：
// 同一段数据只落盘一次、**跨进程**也命中（连续两次跑同一用例不会各写一份），
// 与 data: 链路（mediadataurl.go）同一套取舍。刻意不带扩展名——ffmpeg 靠内容嗅探
// 识别容器，扩展名反而可能误导。
func audioDecodeTempFile(data []byte) (string, bool) {
	sum := sha256.Sum256(data)
	audioDecodeMu.Lock()
	if hit, ok := audioDecodePaths[sum]; ok {
		audioDecodeMu.Unlock()
		return hit, true
	}
	audioDecodeMu.Unlock()

	path := filepath.Join(os.TempDir(), "wbui-webaudio-"+hex.EncodeToString(sum[:8]))
	if _, err := os.Stat(path); err != nil {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return "", false
		}
		audioDecodeMu.Lock()
		audioDecodeFiles[path] = struct{}{}
		audioDecodeMu.Unlock()
	}
	audioDecodeMu.Lock()
	audioDecodePaths[sum] = path
	audioDecodeMu.Unlock()
	return path, true
}

// CleanupWebAudioTempFiles 删除**本进程**写下的 WebAudio 解码临时文件，返回删除
// 个数。刻意不自动调用（与 CleanupMediaDataURLFiles 同一理由：文件按内容命名、
// 跨进程复用，进程一启动就清只会白白重写一遍）。
func CleanupWebAudioTempFiles() int {
	audioDecodeMu.Lock()
	paths := make([]string, 0, len(audioDecodeFiles))
	for p := range audioDecodeFiles {
		paths = append(paths, p)
	}
	audioDecodeFiles = map[string]struct{}{}
	audioDecodePaths = map[[32]byte]string{}
	audioDecodeMu.Unlock()

	removed := 0
	for _, p := range paths {
		if err := os.Remove(p); err == nil {
			removed++
		}
	}
	return removed
}

// ─── ffmpeg 解码 ──────────────────────────────────────────

var (
	// `44100 Hz`
	ffHzRe = regexp.MustCompile(`(\d+)\s*Hz`)
	// 非 mono/stereo 的显式声道数写法：`4 channels`
	ffChannelCountRe = regexp.MustCompile(`(\d+)\s*channels`)
)

// parseFFmpegAudioStream 从 `ffmpeg -i` 的 stderr 里取**第一个音频流**的采样率与
// 声道数。音频流行形如：
//
//	Stream #0:0: Audio: pcm_s16le ([1][0][0][0] / 0x0001), 44100 Hz, mono, s16, 705 kb/s
//
// 只在含 `Audio:` 的行上匹配——与 parseFFmpegProbe 同一纪律（容器/视频行里也有
// 相似的数字，混着匹配会取到错的量）。
func parseFFmpegAudioStream(out string) (sampleRate, channels int) {
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "Audio:") {
			continue
		}
		if m := ffHzRe.FindStringSubmatch(line); m != nil {
			sampleRate, _ = strconv.Atoi(m[1])
		}
		switch {
		case strings.Contains(line, "mono"):
			channels = 1
		case strings.Contains(line, "stereo"):
			channels = 2
		default:
			if m := ffChannelCountRe.FindStringSubmatch(line); m != nil {
				channels, _ = strconv.Atoi(m[1])
			}
		}
		if sampleRate > 0 && channels > 0 {
			return sampleRate, channels
		}
	}
	return sampleRate, channels
}

// decodeAudioBytes 用 ffmpeg 把内存字节解成 float32 PCM（各声道分开）。
// bin 为空、字节为空、ffmpeg 失败、输出为空、或源流信息解析不出采样率/声道 → false。
func decodeAudioBytes(bin string, data []byte) (bindings.AudioDecoded, bool) {
	if strings.TrimSpace(bin) == "" || len(data) == 0 {
		return bindings.AudioDecoded{}, false
	}
	path, ok := audioDecodeTempFile(data)
	if !ok {
		return bindings.AudioDecoded{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), audioDecodeTimeout)
	defer cancel()
	// `-vn`：只要音频（视频文件也能取到它的音轨；纯音频文件本就不受影响）。
	// `-f f32le -`：交错 32-bit float 小端样本写到 stdout——这正是 AudioBuffer
	// 的样本量纲，后面只需按声道拆分，不做任何转换。
	cmd := exec.CommandContext(ctx, bin, "-hide_banner", "-nostdin", "-i", path, "-vn", "-f", "f32le", "-")
	var out, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	if err := cmd.Run(); err != nil || out.Len() == 0 {
		return bindings.AudioDecoded{}, false
	}
	rate, channels := parseFFmpegAudioStream(errBuf.String())
	if rate <= 0 || channels <= 0 || channels > 32 {
		return bindings.AudioDecoded{}, false
	}
	raw := out.Bytes()
	frames := len(raw) / 4 / channels
	if frames <= 0 {
		return bindings.AudioDecoded{}, false
	}
	chans := make([][]float32, channels)
	for c := range chans {
		chans[c] = make([]float32, frames)
	}
	for f := 0; f < frames; f++ {
		for c := 0; c < channels; c++ {
			i := (f*channels + c) * 4
			chans[c][f] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i:]))
		}
	}
	return bindings.AudioDecoded{SampleRate: rate, Channels: chans}, true
}
