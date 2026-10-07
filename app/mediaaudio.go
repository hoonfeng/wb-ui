package app

// 宿主侧音频会话（主线 A3-1）：把「一个 src 的 PCM 从哪来、到哪去、播到哪了」
// 三件事实现成 rendering.AudioSession。
//
// 链路（与视频的 A1/A2 帧流对称）：
//
//	ffmpeg -ss <t> -i <file> -f s16le -ar 48000 -ac 2 -   →  PCM 块
//	  → tap（可选：交付观察者——判据 A 的取证口 / 非 Windows 的自备输出口）
//	  → audioDevice.write（Windows waveOut；无设备则只走 tap，降级模式）
//	  → dev.playedFrames() 就是 Position()，也就是引擎的 currentTime（A3 的时钟真相）
//
// 为什么统一成 48kHz/2ch：设备打开率高（几乎所有 waveOut 设备都支持），引擎通道
// 上因此不必带重采样/位深转换；判据（FFT 主峰）与时长换算也只需要一种格式。
// 音频样本本身是 44.1kHz mono，由 ffmpeg 这一行参数完成转换——解码侧的复杂性
// 全在宿主，这正是路线 (a) 的分工（引擎不背解码器）。
import (
	"io"
	"log"
	"math"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"wb-ui/engine/js/bindings"
	"wb-ui/engine/rendering"
	"wb-ui/webkit"
)

// 宿主统一交付的 PCM 格式（s16le 交错）。
const (
	audioPCMSampleRate = 48000
	audioPCMChannels   = 2
)

// audioChunkFrames 是推送循环每次读写的采样帧数：2400 帧 = 50ms @48kHz。
// 取 50ms 是因为设备缓冲是 100ms（app/audioout.go）：一个读块必定放得进一个
// 缓冲，write 的等待也因此最多跨一块，暂停/seek 的响应保持在百毫秒内。
const audioChunkFrames = 2400

// PCMTap 是 PCM 交付观察者：宿主把**即将写入输出设备的同一份** PCM 交给它。
//
// 两个用途：
//   - 取证（判据 A）：探针对它做 FFT，主峰应是样本的 440Hz——这是「引擎确实把
//     音频交给了输出」的自证，不需要环回录音设备；
//   - 降级输出（非 Windows / 本机无设备）：宿主用这个回调自备输出（自己接
//     ALSA/CoreAudio/PulseAudio）。
//
// 实现必须快速返回（在推送 goroutine 上被调用，阻塞会拖慢播放）。
type PCMTap func(src string, format rendering.AudioFormat, startFrame int64, data []byte)

// MediaAudio 是音频会话工厂（宿主注入 rendering.AudioSessionSource）。
type MediaAudio struct {
	bin string

	mu  sync.Mutex
	tap PCMTap
}

// NewMediaAudio 建工厂。ffmpegPath 为空时从 PATH 查找 ffmpeg。
func NewMediaAudio(ffmpegPath string) *MediaAudio {
	bin := ffmpegPath
	if strings.TrimSpace(bin) == "" {
		bin, _ = exec.LookPath("ffmpeg")
	}
	return &MediaAudio{bin: bin}
}

// SetTap 注册（传 nil 清除）PCM 交付观察者。
func (m *MediaAudio) SetTap(tap PCMTap) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.tap = tap
	m.mu.Unlock()
}

// InstallMediaAudio 把音频会话源接到引擎上：<audio>/<video> 调用 play() 时，引擎
// 按 src 打开一个会话（见 engine/js/bindings/mediaaudio.go）。返回工厂供调用方
// 注册 tap（判据 A）或复用。
func InstallMediaAudio(wv *webkit.WebView, ffmpegPath string) *MediaAudio {
	m := NewMediaAudio(ffmpegPath)
	base := func() string {
		if wv == nil {
			return ""
		}
		return wv.DocumentBaseURL()
	}
	// ★ 资源策略门禁（与引擎 `<img>`/`<script>`/`<link>` 同一条判定，见
	//   mediaaccess.go）：音频 PCM 链路与元数据/抽帧共用同一个判定函数；同时把
	//   引擎侧的资源选择门禁一起接上（两个 Install 都接，谁先谁后都不留缺口）。
	gate := mediaRefGateFor(wv)
	SetMediaRefGate(gate, mediaPolicyOf(wv))
	bindings.MediaSrcAllowed = gate
	rendering.SetAudioSessionSource(m.SessionSource(base))
	return m
}

// SessionSource 返回可直接交给 rendering.SetAudioSessionSource 的会话源。
// baseURL 是取文档基准的闭包（页面会导航，基准随之变化，不能在装配时固化）。
func (m *MediaAudio) SessionSource(baseURL func() string) rendering.AudioSessionSource {
	return func(src string, startSeconds float64) (rendering.AudioSession, bool) {
		if m == nil || strings.TrimSpace(m.bin) == "" {
			return nil, false
		}
		base := ""
		if baseURL != nil {
			base = baseURL()
		}
		path, ok := mediaSrcToPath(src, base)
		if !ok {
			return nil, false
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		// 先问元数据：没有音轨的资源（纯视频、图片格式的文件）不该开会话——否则
		// 引擎会切到一条永远不推进的「音频时钟」上。探测是同包已有的 ffmpeg 通道。
		meta, ok := probeWithFFmpeg(m.bin, abs)
		if !ok || !meta.HasAudio {
			return nil, false
		}
		return newAudioSession(m, src, abs, startSeconds, meta.Duration), true
	}
}

// audioSession 是一次播放会话（renderering.AudioSession 的宿主实现）。
type audioSession struct {
	m      *MediaAudio
	src    string
	path   string
	format rendering.AudioFormat
	// totalFrames 是总采样帧数（由探测到的时长换算；0 = 未知）。
	totalFrames int64

	dev audioDevice // nil = 降级模式（PCM 只走 tap）

	mu         sync.Mutex
	closed     bool
	failed     bool
	drained    bool // 解码流已到 EOF（所有数据都已交付）
	gotPCM     bool // 至少交付过一块 PCM（false + drained = 没有音频数据）
	paused     bool
	gen        int64
	baseSeconds float64 // 本代音频的起点（媒体时间）
	written     int64   // 本代已交付的采样帧数
	wallStart   time.Time
	wallPlayed  time.Duration
	cmd         *exec.Cmd
	out         io.ReadCloser

	pumpWG sync.WaitGroup
}

// newAudioSession 建会话并启动解码/推送。设备打开失败不是错误：会话降级为
// 「PCM 只走 tap」，时钟改由挂钟与已交付帧数推算（见 Position）。
func newAudioSession(m *MediaAudio, src, path string, startSeconds float64, duration float64) *audioSession {
	format := rendering.AudioFormat{SampleRate: audioPCMSampleRate, Channels: audioPCMChannels}
	s := &audioSession{
		m:           m,
		src:         src,
		path:        path,
		format:      format,
		baseSeconds: startSeconds,
	}
	if duration > 0 && !math.IsNaN(duration) {
		s.totalFrames = int64(math.Round(duration * float64(format.SampleRate)))
	}
	if dev, ok := openAudioDevice(format); ok {
		s.dev = dev
	}
	// ★ 世代号必须先自增再启动：startPump 用「s.gen != gen」判断这一代是否已作废，
	//   若传进去的 gen 与当前 s.gen 相等，启动即被判为旧世代而自我终止（实测症状：
	//   会话打开成功但一个 PCM 块都不交付，tap 永远收不到数据）。
	s.gen = 1
	s.startPump(startSeconds, s.gen)
	return s
}

func (s *audioSession) Format() rendering.AudioFormat { return s.format }

func (s *audioSession) TotalFrames() int64 { return s.totalFrames }

func (s *audioSession) HasOutput() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dev != nil
}

// Failed 报告「这个源确实没有可播的音频」（无音轨 / 解码器起不来）。引擎据此
// 退回挂钟时钟（见 bindings 的 tickAudio）——不能让它挂在一个永不推进的位置上。
func (s *audioSession) Failed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failed || (s.drained && !s.gotPCM)
}

// Position 返回输出侧已播放的媒体时间（秒）。
//
//	有设备：媒体起点 + 设备已播帧数 / 采样率——**设备位置**是唯一真相；
//	降级：媒体起点 + min(已交付帧数, 挂钟×采样率)/采样率——按实时速率模拟推进。
func (s *audioSession) Position() float64 {
	s.mu.Lock()
	dev := s.dev
	base := s.baseSeconds
	written := s.written
	rate := float64(s.format.SampleRate)
	elapsed := s.elapsedLocked()
	s.mu.Unlock()

	var frames int64
	if dev != nil {
		frames = dev.playedFrames()
	} else {
		frames = written
		if real := int64(elapsed.Seconds() * rate); real < frames {
			frames = real
		}
	}
	if frames < 0 {
		frames = 0
	}
	return base + float64(frames)/rate
}

// elapsedLocked 返回「本代实际播放过的时间」（暂停期间不计）。
func (s *audioSession) elapsedLocked() time.Duration {
	d := s.wallPlayed
	if !s.wallStart.IsZero() {
		d += time.Since(s.wallStart)
	}
	return d
}

func (s *audioSession) Pause() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	if !s.paused {
		s.paused = true
		if !s.wallStart.IsZero() {
			s.wallPlayed += time.Since(s.wallStart)
			s.wallStart = time.Time{}
		}
	}
	dev := s.dev
	s.mu.Unlock()
	if dev != nil {
		_ = dev.pause()
	}
	return nil
}

func (s *audioSession) Resume() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	if s.paused {
		s.paused = false
		s.wallStart = time.Now()
	}
	dev := s.dev
	s.mu.Unlock()
	if dev != nil {
		_ = dev.resume()
	}
	return nil
}

// Seek 重启解码流并清空输出缓冲：位置、已交付计数、挂钟一起归零。
func (s *audioSession) Seek(seconds float64) error {
	if seconds < 0 {
		seconds = 0
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.gen++
	gen := s.gen
	cmd, out := s.cmd, s.out
	s.cmd, s.out = nil, nil
	s.written = 0
	s.baseSeconds = seconds
	s.drained = false
	s.gotPCM = false
	s.wallPlayed = 0
	dev := s.dev
	s.mu.Unlock()

	if out != nil {
		_ = out.Close()
	}
	stopFFmpeg(cmd)
	if dev != nil {
		_ = dev.flush() // 丢弃旧位置的数据并把设备位置归零
	}
	s.startPump(seconds, gen)
	return nil
}

func (s *audioSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	cmd, out := s.cmd, s.out
	s.cmd, s.out = nil, nil
	dev := s.dev
	s.dev = nil
	s.mu.Unlock()

	if out != nil {
		_ = out.Close()
	}
	stopFFmpeg(cmd)
	if dev != nil {
		_ = dev.close()
	}
	return nil
}

// startPump 启动一代解码/推送（gen 用于让旧一代自行退出）。
func (s *audioSession) startPump(fromSeconds float64, gen int64) {
	cmd := exec.Command(s.m.bin,
		"-v", "error", "-nostdin",
		"-ss", strconv.FormatFloat(fromSeconds, 'f', 6, 64),
		"-i", s.path,
		"-f", "s16le",
		"-ar", strconv.Itoa(s.format.SampleRate),
		"-ac", strconv.Itoa(s.format.Channels),
		"-",
	)
	out, err := cmd.StdoutPipe()
	if err != nil {
		s.markFailed("ffmpeg stdout 管道创建失败：" + err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		s.markFailed("ffmpeg 启动失败：" + err.Error())
		return
	}
	s.mu.Lock()
	if s.closed || s.gen != gen {
		s.mu.Unlock()
		stopFFmpeg(cmd)
		return
	}
	s.cmd = cmd
	s.out = out
	s.wallStart = time.Now()
	s.mu.Unlock()

	s.pumpWG.Add(1)
	go s.pump(gen, out)
}

// pump 是推送循环：解码流 → tap → 输出设备。EOF 或设备消失即结束。
func (s *audioSession) pump(gen int64, out io.ReadCloser) {
	defer s.pumpWG.Done()
	defer out.Close()

	bpf := s.format.BytesPerFrame()
	buf := make([]byte, audioChunkFrames*bpf)
	for {
		if !s.waitWhilePaused(gen) {
			return
		}
		n, err := io.ReadFull(out, buf)
		if n > 0 {
			data := buf[:n]
			s.mu.Lock()
			if s.closed || s.gen != gen {
				s.mu.Unlock()
				return
			}
			startFrame := s.written
			frames := int64(n / bpf)
			s.written += frames
			s.gotPCM = true
			tap := s.m.tap
			s.mu.Unlock()

			if tap != nil {
				tap(s.src, s.format, startFrame, data)
			}
			if dev := s.currentDevice(); dev != nil {
				if werr := dev.write(data); werr != nil {
					// 设备写失败（被独占、被拔）：降级为「只走 tap」，位置改由
					// 挂钟推算——播放不该因为一个设备故障整体停摆。
					s.dropDevice()
				}
			} else if frames > 0 {
				// 降级模式：按实时速率节流，让 Position 的挂钟推算与之一致
				// （否则整段会被瞬间"播完"，时钟直接跳到末尾）。
				time.Sleep(time.Duration(float64(frames) / float64(s.format.SampleRate) * float64(time.Second)))
			}
		}
		if err != nil {
			break // EOF / 读错：本代结束
		}
	}
	s.mu.Lock()
	if !s.closed && s.gen == gen {
		s.drained = true
	}
	s.mu.Unlock()
}

// waitWhilePaused 在暂停期间等待；会话结束或换代时返回 false。
func (s *audioSession) waitWhilePaused(gen int64) bool {
	for {
		s.mu.Lock()
		closed, paused, stale := s.closed, s.paused, s.gen != gen
		s.mu.Unlock()
		if closed || stale {
			return false
		}
		if !paused {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (s *audioSession) currentDevice() audioDevice {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dev
}

// dropDevice 关闭并丢弃输出设备（写失败时），会话转入降级模式。
func (s *audioSession) dropDevice() {
	s.mu.Lock()
	dev := s.dev
	s.dev = nil
	s.mu.Unlock()
	if dev != nil {
		_ = dev.close()
	}
}

// markFailed 标记会话不可用并留下原因：这条链路横跨「外部进程 + 平台设备」，
// 失败原因（ffmpeg 不在、参数不认、管道建立不了）不记下来就只剩「0 帧」这一个
// 无从查起的现象。
func (s *audioSession) markFailed(reason string) {
	s.mu.Lock()
	s.failed = true
	s.mu.Unlock()
	log.Printf("[audio] 会话不可用：%s（%s）", s.src, reason)
}

// stopFFmpeg 结束一个解码进程（SysProcAttr 未设时 Kill 足够；Wait 回收进程表项）。
func stopFFmpeg(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
}
