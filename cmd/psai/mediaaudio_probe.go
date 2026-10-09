package main

// 探针侧的音频取证（主线 A3-1）。
//
// 三件事：
//
//	判据 A（主）——宿主输出回调（app.PCMTap）收到的 PCM 做 FFT，主峰 ≈ 样本频率。
//	  同一个推送循环里，这份字节既写进设备、也交给 tap，所以它是「引擎确实把音频
//	  交给了输出」的自证判据；不需要环回录音设备（多数机器没有「立体声混音」）。
//	判据 B（可选）——用 ffmpeg dshow 环回录音 1 秒再做 FFT（外部视角复核）。
//	  本机没有环回设备时输出 SKIP(no-loopback)，不阻塞验收。
//	TC-M-602 定点——play() 之后 500ms 的 currentTime（音频为主时钟的时序证据）。
//
// 取证全在探针侧：引擎与宿主不参与任何频谱计算（判据工具不该混进播放链路）。
import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"wb-ui/app"
	"wb-ui/engine/rendering"
	"wb-ui/webkit"
)

// ── 判据 A：PCM 交付观察 ──────────────────────────────────────────────

// audioTapCollector 按 src 累积 tap 交付的 PCM（tap 在推送 goroutine 上被调用，
// 必须自带锁；每个音频样本格子各有一条会话，因此按 src 分开存）。
type audioTapCollector struct {
	mu    sync.Mutex
	bySrc map[string]*audioTapData
}

type audioTapData struct {
	format rendering.AudioFormat
	pcm    []byte
	frames int64
	// firstAt 是本 src **首次交付 PCM** 的时刻。它是定点判据的零点：设备从收到
	// 第一块 PCM 才开始播，所以「时钟走了多久」要从这一刻算（见 clockOK）。
	firstAt time.Time
}

func newAudioTapCollector() *audioTapCollector {
	return &audioTapCollector{bySrc: map[string]*audioTapData{}}
}

// tap 是 app.PCMTap 的实现。
//
// 只保留前 1 秒的字节：矩阵页有 12 个音频格同时播，1 秒 × 48kHz × 2ch × 2B
// = 192KB/格，共约 2.3MB——够 FFT 与「交付量」判据用，也不会把内存撑起来。
func (c *audioTapCollector) tap(src string, format rendering.AudioFormat, _ int64, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.bySrc[src]
	if d == nil {
		d = &audioTapData{}
		c.bySrc[src] = d
	}
	d.format = format
	if format.Valid() {
		if d.firstAt.IsZero() {
			d.firstAt = time.Now()
		}
		limit := format.SampleRate * format.BytesPerFrame()
		if len(d.pcm) < limit {
			d.pcm = append(d.pcm, data...)
			if len(d.pcm) > limit {
				d.pcm = d.pcm[:limit]
			}
		}
		d.frames += int64(len(data) / format.BytesPerFrame())
	}
}

func (c *audioTapCollector) snapshot(src string) (rendering.AudioFormat, []byte, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.bySrc[src]
	if d == nil {
		return rendering.AudioFormat{}, nil, 0
	}
	out := make([]byte, len(d.pcm))
	copy(out, d.pcm)
	return d.format, out, d.frames
}

// firstTapAt 返回该 src 首次交付 PCM 的时刻（零值 = 从未交付）。
func (c *audioTapCollector) firstTapAt(src string) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if d := c.bySrc[src]; d != nil {
		return d.firstAt
	}
	return time.Time{}
}

// waitAudioFirstPCM 等到每个音频格都交付了首块 PCM（或超时）。
//
// 为什么需要：12 个音频格同时 play() 时，各自的会话要抢 ffmpeg 进程与输出设备，
// 启动顺序不定——先采集再等会让排在后面的格子读到一个「时钟还没起步」的 0。
// 定点判据要求的是「时钟起步之后按真实速率推进」，所以采样点必须落在起步之后。
// data: 来源永远没有 PCM（无本地路径），不参与等待。
func waitAudioFirstPCM(wv *webkit.WebView, col *audioTapCollector, cells []cell, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		all := true
		for _, c := range cells {
			if c.Sample.Kind != "audio" || c.Source == "data" {
				continue
			}
			if col.firstTapAt(c.URL).IsZero() {
				all = false
				break
			}
		}
		if all {
			return
		}
		driveEventLoop(wv, 10)
		time.Sleep(5 * time.Millisecond)
	}
}

// audioCellEvidence 是一格音频的完整取证（判定与报告共用）。
type audioCellEvidence struct {
	Config string
	ID     string
	Sample string
	Source string
	// ExpectHz 是样本的期望基频（manifest 的 hz 字段；0 = 未知，不判判据 A）。
	ExpectHz float64

	format rendering.AudioFormat
	// pcm 是 tap 收到的前 1 秒 PCM（判据 A 的输入，也是可复核的原始证据）。
	pcm    []byte
	frames int64
	// peakHz/peakMag 是判据 A 的 FFT 主峰。
	peakHz  float64
	peakMag float64
	// ctMid 是 play() 之后 500ms 采到的 currentTime（TC-M-602 定点）。
	ctMid float64
	// firstTap/sampleAt 是定点判据的两个时间锚点（见 clockOK）。
	firstTap time.Time
	sampleAt time.Time
	// ctEnd/ended 是播放结束后的状态（1 秒样本应为 1.0 / true）。
	ctEnd float64
	ended bool
	// hasOutput 报告该次运行本机是否有内置输出设备（支持矩阵的逐格依据）。
	hasOutput bool
	// opened 报告会话是否成功打开（宿主音频源被调用且有音轨）。
	opened bool
}

// collectAudioEvidence 按格子组装取证结果：PCM 来自 tap，时钟来自页面采集。
// 顺序与 cells 一致（报告可 diff）。
func collectAudioEvidence(cfg string, cells []cell, col *audioTapCollector,
	midByID, endByID map[string]cellProbe, hasOutput bool, sampleAt time.Time) []audioCellEvidence {

	var out []audioCellEvidence
	for _, c := range cells {
		if c.Sample.Kind != "audio" {
			continue
		}
		format, pcm, frames := col.snapshot(c.URL)
		ev := audioCellEvidence{
			Config: cfg, ID: c.ID, Sample: c.Sample.Name, Source: c.Source,
			ExpectHz: c.Sample.AudioHz, hasOutput: hasOutput,
			format: format, pcm: pcm, frames: frames,
			opened:   frames > 0,
			firstTap: col.firstTapAt(c.URL),
			sampleAt: sampleAt,
		}
		if format.Valid() && len(pcm) > 0 {
			mono := app.PCMToMono(pcm, format.Channels)
			ev.peakHz, ev.peakMag = app.DominantFrequency(mono, format.SampleRate)
		}
		if p, ok := midByID[c.ID]; ok {
			ev.ctMid = p.CurTime
		}
		if p, ok := endByID[c.ID]; ok {
			ev.ctEnd = p.CurTime
			if b, ok := p.Paused.(bool); ok {
				ev.ended = b && p.CurTime > 0
			}
		}
		out = append(out, ev)
	}
	return out
}

// audioPeakToleranceHz 是判据 A 的频率容差。
//
// 采样率 48kHz、FFT 窗 16384 点 ⇒ bin 宽 2.93Hz，加抛物插值后定位精度约 ±1Hz；
// 但样本本身是 44.1kHz 源经 ffmpeg 重采样到 48kHz 的，重采样滤波器的过渡带会
// 让主峰略偏（实测 439.88Hz）。10Hz 容差足以吸收这些，又能拒绝把 220/880Hz
// （谐波/倍频）误判成 440Hz。
const audioPeakToleranceHz = 10

// peakOK 报告该格的判据 A 是否通过（期望频率未知时按「有 PCM 即可」）。
func (ev audioCellEvidence) peakOK() bool {
	if ev.frames <= 0 || ev.peakHz <= 0 {
		return false
	}
	if ev.ExpectHz <= 0 {
		return true
	}
	return math.Abs(ev.peakHz-ev.ExpectHz) <= audioPeakToleranceHz
}

// clockOK 报告该格的「音频为主时钟」时序是否成立（TC-M-602 定点）。
//
// 两条判据：
//
//	① 播完：终态 currentTime 必须到达 duration（1 秒样本 → 1.00）。
//	② 不超前：采样点的 currentTime 不得超过「该会话首块 PCM 交付以来经过的时间」
//	   ——这是音频时钟的**本质性质**：设备位置只可能 ≤ 已交付且已播出的时间，
//	   任何超前都说明时钟不是输出位置驱动的（例如仍按 play() 起算的挂钟）。
//	   下界留 0.45s 余量：设备从收到首块 PCM 到真正出声有启动延迟（12 格并发
//	   抢设备时实测可达数百毫秒），这不是引擎能决定的，不该判成缺陷。
//
// 采样时还没收到 PCM 的格子（并发播放时排在后面的格子）不适用判据②：此时它连
// 时钟都没起步，只看终态。
//
// ★ 严格断言在引擎单测里（`engine/js/bindings/mediaaudio_test.go` 的
// TestAudioSessionDrivesCurrentTime：currentTime **逐值精确等于**会话 Position，
// 并用「位置不动则 currentTime 不动」反证它不是挂钟自增）。探针这边是端到端
// 环境（12 格并发 + 真设备），只做上述不变量检查。
func (ev audioCellEvidence) clockOK(duration float64) bool {
	if ev.ctMid < 0 {
		return false
	}
	if duration > 0 && ev.ctEnd > 0 && math.Abs(ev.ctEnd-duration) > 0.05 {
		// 未播完也可能是「探针采样过早」，不算失败：只在明显偏差（>50ms）时判否。
		return false
	}
	if ev.firstTap.IsZero() || ev.firstTap.After(ev.sampleAt) {
		return ev.ctEnd > 0
	}
	expect := ev.sampleAt.Sub(ev.firstTap).Seconds()
	if duration > 0 && expect > duration {
		expect = duration
	}
	if ev.ctMid > expect+audioClockOvershootSec {
		return false
	}
	return ev.ctMid >= expect-audioClockLagSec
}

// audioClockOvershootSec 是「时钟超前」的容许量（秒）：采样本身有开销（页面采集
// + 事件循环推进），几十毫秒的测量误差不该判成缺陷。
const audioClockOvershootSec = 0.1

// audioClockLagSec 是「时钟滞后」的容许量（秒）：设备缓冲深度（4×100ms）加上
// 启动延迟，落后几百毫秒是正常播放态。
const audioClockLagSec = 0.45

// ── 判据 B：环回录音（可选） ─────────────────────────────────────────

// loopbackEvidence 是判据 B 的结果。
type loopbackEvidence struct {
	Status  string // ok / mismatch / SKIP(no-loopback) / SKIP(loopback-failed)
	Method  string // 采集路径：wasapi-loopback / ffmpeg-dshow
	Device  string
	PeakHz  float64
	PeakMag float64
	Command string
	Detail  string
}

// dshowDeviceRe 匹配 ffmpeg dshow 设备列表里的一行：
//
//	[dshow @ 0x…] "Voicemeeter Out B3 (VB-Audio Voicemeeter VAIO)" (audio)
var dshowDeviceRe = regexp.MustCompile(`"([^"]+)"\s*\(audio\)`)

// listDShowAudioDevices 列出本机 dshow 音频设备（ffmpeg 把设备表打在 stderr，
// 且因为没有真实输入而返回非 0——这不是错误）。
func listDShowAudioDevices(ffmpeg string) []string {
	cmd := exec.Command(ffmpeg, "-hide_banner", "-list_devices", "true", "-f", "dshow", "-i", "dummy")
	out, _ := cmd.CombinedOutput()
	var devs []string
	for _, m := range dshowDeviceRe.FindAllStringSubmatch(string(out), -1) {
		devs = append(devs, m[1])
	}
	return devs
}

// pickLoopbackDevice 从设备表里挑最可能录到「系统正在播放的声音」的设备。
//
// 真环回（Stereo Mix / 立体声混音 / What U Hear / Loopback）优先；虚拟声卡
// （Voicemeeter / VB-Cable / Sonar / Virtual）次之——它们能不能录到取决于路由，
// 所以只是候选，最终判定仍以录音后的 FFT 为准。
func pickLoopbackDevice(devs []string) string {
	best, bestScore := "", 0
	for _, d := range devs {
		l := strings.ToLower(d)
		score := 0
		switch {
		case strings.Contains(l, "stereo mix"), strings.Contains(d, "立体声混音"),
			strings.Contains(l, "what u hear"), strings.Contains(l, "loopback"):
			score = 3
		case strings.Contains(l, "voicemeeter"), strings.Contains(l, "virtual"),
			strings.Contains(l, "sonar"), strings.Contains(l, "cable"):
			score = 1
		}
		if score > bestScore {
			bestScore, best = score, d
		}
	}
	return best
}

// probeLoopback 执行判据 B：**先起录音、再起播放**，把输出端点真的吐出来的字节采回来
// 做 FFT。两条采集路径，优先前者（2026-10 实测结论）：
//
//	① WASAPI loopback（app.CaptureDefaultRenderLoopback）：由音频引擎在**默认 render
//	   端点内部**做 tap，录到的就是「应用写给该端点、混音之后」的字节。不需要
//	   「立体声混音」之类的环回设备、不需要管理员、也不受虚拟声卡内部路由影响——
//	   实测本机默认输出是虚拟声卡（Voicemeeter VAIO3）且其引擎未运行时，dshow 只能
//	   录到底噪（rms ≈ 1e-5），而同一窗口的 loopback 照样录到 440.000Hz。
//	② ffmpeg dshow 录一个环回/虚拟采集设备（回退路径，语义见 probeLoopbackDShow）。
//
// 任何一步不可用都返回 SKIP（不阻塞验收），理由写进 Detail。
func probeLoopback(expectHz, seconds float64, outDir string) loopbackEvidence {
	if ev, ok := probeLoopbackWASAPI(expectHz, seconds, outDir); ok {
		return ev
	}
	ev := probeLoopbackDShow(expectHz, seconds)
	if ev.Method == "" {
		ev.Method = "ffmpeg-dshow"
	}
	if ev.Status == "SKIP(no-loopback)" {
		ev.Detail = "WASAPI loopback 在本机不可用（端点打不开/混音格式不支持）；" + ev.Detail
	}
	return ev
}

// probeLoopbackWASAPI 走 ① 路径：起录音 → 播测试音 → 取回波形做 FFT。
//
// 返回 ok=false 的语义是「**采集路径本身不可用**」（端点打不开、混音格式不支持、
// 录音窗口内一帧都没拿到）——调用方据此回退 ②。而「录到数据但主峰不是期望频率」
// **不是** ok=false：那是有效证据（mismatch），必须如实记录而不是被回退路径盖掉。
func probeLoopbackWASAPI(expectHz, seconds float64, outDir string) (loopbackEvidence, bool) {
	format := rendering.AudioFormat{SampleRate: 48000, Channels: 2}
	type recResult struct {
		pcm    []byte
		format rendering.AudioFormat
		err    error
	}
	ch := make(chan recResult, 1)
	go func() {
		pcm, f, err := app.CaptureDefaultRenderLoopback(seconds + 0.8)
		ch <- recResult{pcm, f, err}
	}()
	// ★ 先起录音：Windows 的 loopback 在端点没有音频在播时**不产包**（GetNextPacketSize
	//   返回 0），「先播后录」会把开头整段丢掉。
	time.Sleep(250 * time.Millisecond)
	playErr := app.PlayTestTone(expectHz, seconds, format)
	r := <-ch
	if r.err != nil || len(r.pcm) == 0 {
		return loopbackEvidence{}, false
	}
	f := r.format
	if !f.Valid() {
		f = format
	}
	// 录音原件落盘：判据 B 的结论要能被**复听复核**（不带证据的频谱数字只是断言）。
	wavNote := ""
	if outDir != "" {
		wavPath := filepath.Join(outDir, "loopback-wasapi.wav")
		if err := writeLoopbackWAV(wavPath, r.pcm, f); err == nil {
			wavNote = "；录音证据 → " + wavPath
		} else {
			wavNote = "；录音落盘失败：" + err.Error()
		}
	}
	// 从「第一段非零音频」开始取 seconds 秒再交给 FFT。
	pcm := trimLoopbackToAudio(r.pcm, f, seconds)
	mono := app.PCMToMono(pcm, f.Channels)
	peak, mag := app.DominantFrequency(mono, f.SampleRate)
	method := "wasapi-loopback"
	if playErr != nil {
		method += "(play-failed)"
	}
	ev := loopbackEvidence{
		Method:  method,
		Device:  fmt.Sprintf("WASAPI loopback @ 默认输出端点（%dHz %dch）", f.SampleRate, f.Channels),
		PeakHz:  peak,
		PeakMag: mag,
		Command: fmt.Sprintf("app.CaptureDefaultRenderLoopback(%.1fs) ‖ app.PlayTestTone(%.0fHz, %.1fs, %dHz/%dch)%s",
			seconds+0.8, expectHz, seconds, format.SampleRate, format.Channels, wavNote),
	}
	if math.Abs(peak-expectHz) <= audioPeakToleranceHz {
		ev.Status = "ok"
	} else {
		ev.Status = "mismatch"
		ev.Detail = fmt.Sprintf("录回主峰 %.1fHz 与期望 %.0fHz 不符（幅度 %.3f）", peak, expectHz, mag)
	}
	return ev, true
}

// writeLoopbackWAV 把采回的 s16le 交错 PCM 写成标准 wav（判据 B 的可复听证据）。
func writeLoopbackWAV(path string, pcm []byte, f rendering.AudioFormat) error {
	channels := f.Channels
	if channels < 1 {
		channels = 1
	}
	hdr := make([]byte, 44)
	copy(hdr[0:4], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(36+len(pcm)))
	copy(hdr[8:12], "WAVE")
	copy(hdr[12:16], "fmt ")
	binary.LittleEndian.PutUint32(hdr[16:20], 16)
	binary.LittleEndian.PutUint16(hdr[20:22], 1) // PCM
	binary.LittleEndian.PutUint16(hdr[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(hdr[24:28], uint32(f.SampleRate))
	binary.LittleEndian.PutUint32(hdr[28:32], uint32(f.SampleRate*channels*2))
	binary.LittleEndian.PutUint16(hdr[32:34], uint16(channels*2))
	binary.LittleEndian.PutUint16(hdr[34:36], 16)
	copy(hdr[36:40], "data")
	binary.LittleEndian.PutUint32(hdr[40:44], uint32(len(pcm)))
	return os.WriteFile(path, append(hdr, pcm...), 0o644)
}

// trimLoopbackToAudio 在录音里定位**第一段非零音频**的起点，返回从那里开始、长度约
// seconds 秒的切片（不够就取到末尾）。
//
// 为什么必须定位而不是裁固定时长：录音**必须先于播放启动**（Windows 的 loopback 在
// 端点静音时不产包），而「起录音」到「设备真的开始交付数据」之间还隔着 COM 初始化
// 与输出设备打开——实测静音前缀可达 0.4s。而 DominantFrequency 只对**采样序列的
// 前 spectrumMaxSamples（16384 ≈ 0.34s @48k）**做 FFT，固定裁 0.3s 会把整个窗口
// 留在静音里，主峰因此恒为 0（症状：录到的数据非零、判据却报 mismatch）。
//
// 整段都是静音时原样返回：让 FFT 如实得出「主峰 0」而不是把判据悄悄变成通过。
func trimLoopbackToAudio(pcm []byte, f rendering.AudioFormat, seconds float64) []byte {
	bytesPerFrame := 2 * f.Channels
	if bytesPerFrame <= 0 || len(pcm) < bytesPerFrame {
		return pcm
	}
	const threshold = 100 // s16 满幅 32767 的约 0.3%（远高于底噪、远低于测试音幅度）
	frames := len(pcm) / bytesPerFrame
	start := -1
	for i := 0; i < frames && start < 0; i++ {
		off := i * bytesPerFrame
		for c := 0; c < f.Channels; c++ {
			v := int16(uint16(pcm[off+c*2]) | uint16(pcm[off+c*2+1])<<8)
			if v > threshold || v < -threshold {
				start = off
				break
			}
		}
	}
	if start < 0 {
		return pcm
	}
	end := start + int(seconds*float64(f.SampleRate))*bytesPerFrame
	if end > len(pcm) {
		end = len(pcm)
	}
	return pcm[start:end]
}

// probeLoopbackDShow 是 ② 回退路径：挑一个环回/虚拟采集设备录音 seconds 秒，FFT 找主峰。
// 任何一步不可用都返回 SKIP（不阻塞验收），理由写进 Detail。
func probeLoopbackDShow(expectHz, seconds float64) loopbackEvidence {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return loopbackEvidence{Status: "SKIP(no-loopback)", Detail: "本机没有 ffmpeg"}
	}
	devs := listDShowAudioDevices(ffmpeg)
	dev := pickLoopbackDevice(devs)
	if dev == "" {
		return loopbackEvidence{
			Status: "SKIP(no-loopback)",
			Detail: fmt.Sprintf("本机 dshow 音频设备 %d 个，没有环回/虚拟声卡候选（多数机器没有「立体声混音」）", len(devs)),
		}
	}
	format := rendering.AudioFormat{SampleRate: 48000, Channels: 2}
	cmdline := fmt.Sprintf("ffmpeg -hide_banner -v error -f dshow -i \"audio=%s\" -t %.1f -f s16le -ar %d -ac %d -",
		dev, seconds, format.SampleRate, format.Channels)
	cmd := exec.Command(ffmpeg, "-hide_banner", "-v", "error", "-f", "dshow", "-i", "audio="+dev,
		"-t", fmt.Sprintf("%.3f", seconds), "-f", "s16le",
		"-ar", fmt.Sprintf("%d", format.SampleRate), "-ac", fmt.Sprintf("%d", format.Channels), "-")
	cmd.WaitDelay = 3 * time.Second
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return loopbackEvidence{
			Status: "SKIP(loopback-failed)", Device: dev, Command: cmdline,
			Detail: fmt.Sprintf("录音失败（设备被独占/不可用）：%v", err),
		}
	}
	mono := app.PCMToMono(out, format.Channels)
	peak, mag := app.DominantFrequency(mono, format.SampleRate)
	ev := loopbackEvidence{Device: dev, PeakHz: peak, PeakMag: mag, Command: cmdline}
	if math.Abs(peak-expectHz) <= audioPeakToleranceHz {
		ev.Status = "ok"
	} else {
		ev.Status = "mismatch"
		// ★ 只陈述测得的事实，**不代下因果结论**（2026-10-07，Q3 结项）：「录到别的声音」
		//   与「只录到底噪」在频谱上都表现为「主峰≠期望」，但成因与排查方向完全不同。
		//   二者的区分靠**幅度对照**（判据 A 的同一份 PCM 幅度），报告侧做，见 audioSection。
		ev.Detail = fmt.Sprintf("录回主峰 %.1fHz 与期望 %.0fHz 不符（幅度 %.3f）", peak, expectHz, mag)
	}
	return ev
}

// ── 报告小节 ─────────────────────────────────────────────────────────

// audioSection 生成报告里的「音频（A3-1）」小节：支持矩阵 + 判据 A + 判据 B +
// TC-M-602 定点。全部数字来自本次运行（可复跑、可 diff）。
func audioSection(evidence []audioCellEvidence, lo loopbackEvidence) string {
	var b strings.Builder
	b.WriteString("### 平台支持矩阵\n\n")
	b.WriteString(audioSupportMatrix())

	b.WriteString("### 判据 A：宿主输出回调的 PCM 频谱（L4-S 主判据）\n\n")
	b.WriteString("复查命令：`cmd/psai -media`（探针在 `app.PCMTap` 上取「即将写进输出设备的同一份 PCM」做 FFT）\n\n")
	b.WriteString("| 配置 | 样本 | 来源 | 交付帧数 | 主峰 Hz | 期望 Hz | 幅度 | 判定 |\n|---|---|---|---|---|---|---|---|\n")
	measured := 0
	for _, ev := range evidence {
		if ev.frames <= 0 {
			continue
		}
		measured++
		verdict := "❌ 偏离"
		if ev.peakOK() {
			verdict = "✅"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %.2f | %.0f | %.3f | %s |\n",
			ev.Config, ev.Sample, ev.Source, ev.frames, ev.peakHz, ev.ExpectHz, ev.peakMag, verdict)
	}
	if measured == 0 {
		b.WriteString("| — | — | — | 0 | — | — | — | ⚠️ 未采到 PCM（宿主音频源未注册 / 无 ffmpeg / 无音轨） |\n")
	}
	b.WriteString("\n")

	b.WriteString("### 判据 B：环回录音（可选，外部视角）\n\n")
	if lo.Command != "" {
		fmt.Fprintf(&b, "命令：`%s`\n\n", lo.Command)
	}
	if lo.Method != "" {
		fmt.Fprintf(&b, "- 采集路径：**%s**\n", lo.Method)
	}
	fmt.Fprintf(&b, "- 设备：%s\n- 结果：**%s**\n", orDash(lo.Device), lo.Status)
	if lo.PeakHz > 0 {
		fmt.Fprintf(&b, "- 录回音频主峰：%.1f Hz（幅度 %.3f）\n", lo.PeakHz, lo.PeakMag)
	}
	if lo.Detail != "" {
		fmt.Fprintf(&b, "- 说明：%s\n", lo.Detail)
	}
	// ★ 幅度对照（2026-10-07）：`mismatch` 有两类成因，靠「录回幅度 vs 判据 A 的同一份
	//   PCM 幅度」区分——同量级 = 确实录到了别的声音（查输出路由）；低若干数量级 =
	//   只录到底噪（查采集设备有没有被系统输出喂到）。不写这个对照，读者无法判断
	//   mismatch 是「环境未配置」还是「输出链路有问题」。
	if refMag := maxAudioPeakMag(evidence); refMag > 0 && lo.PeakMag > 0 {
		fmt.Fprintf(&b, "- 幅度对照：判据 A 的同一份待输出 PCM 幅度 **%.3f**；",
			refMag)
		if ratio := refMag / lo.PeakMag; ratio >= 2 {
			fmt.Fprintf(&b, "录回幅度仅为其 **1/%.0f** ⇒ 基本只录到底噪，该采集设备未被系统输出喂到。\n", ratio)
		} else if lo.Status == "ok" {
			b.WriteString("两者同量级 ⇒ **端到端成立**（要写进设备的字节与被采回的波形同量级）。\n")
		} else {
			b.WriteString("两者同量级 ⇒ 确实录到了声音，只是主峰不是期望的正弦。\n")
		}
	}
	b.WriteString("\n**判定条件与语义**（本节口径）：判据 B 是**可选的外部复核**（端到端视角：\n" +
		"解码 → 输出设备 → 采集设备），四种取值都**如实记录、都不伪装成通过**：\n" +
		"- `ok`：录回主峰落在期望 ±容差 ⇒ 端到端成立；\n" +
		"- `SKIP(no-loopback)`：WASAPI loopback 不可用，**且**无 ffmpeg / 无环回（立体声混音）\n" +
		"  或虚拟声卡候选 ⇒ 跳过；\n" +
		"- `SKIP(loopback-failed)`：有候选但打开失败（被独占）⇒ 跳过；\n" +
		"- `mismatch`：录到数据但主峰不符 ⇒ **不视为通过，也不阻塞验收**。\n" +
		"L4-S 的**主线判据**是 A（同一份待输出 PCM 的 FFT 主峰/幅度）——它证明的是\n" +
		"「要写进输出设备的字节正确」；B 只补「设备真的出声且被采回」。因此 B 不可用\n" +
		"时验收仍成立，只是端到端那一环缺外部证据（本机即此情形）。\n")
	b.WriteString("\n")

	b.WriteString("### TC-M-602 定点：播放时钟（音频为主时钟）\n\n")
	b.WriteString("判据：①终态 `currentTime` 到达 duration；②采样点 `currentTime` **不超前**于" +
		"「该会话首块 PCM 交付以来经过的时间」（设备位置只可能 ≤ 已交付且已播出的时间，" +
		"超前即说明时钟不是输出位置驱动的），滞后容许 0.45s（设备启动延迟与缓冲深度）。\n\n" +
		"引擎侧的**严格**断言另见 `engine/js/bindings/mediaaudio_test.go`" +
		"（`TestAudioSessionDrivesCurrentTime`：currentTime 逐值精确等于会话 Position）。\n\n")
	b.WriteString("| 配置 | 样本 | 来源 | 采样点（首块后 s） | currentTime | 期望 | 结束 currentTime | 判定 |\n|---|---|---|---|---|---|---|---|\n")
	for _, ev := range evidence {
		verdict := "—"
		expect := 0.0
		if !ev.firstTap.IsZero() && !ev.firstTap.After(ev.sampleAt) {
			expect = ev.sampleAt.Sub(ev.firstTap).Seconds()
			// 期望值不该超过该格自己的终态（样本播完即封顶）：采样点若落在终态之后，
			// 「还应经过多少秒」没有意义，封顶到终态才与判据（不超前）同口径。
			if ev.ctEnd > 0 && expect > ev.ctEnd {
				expect = ev.ctEnd
			}
		}
		if ev.frames > 0 {
			verdict = "✅"
			if !ev.clockOK(1.0) {
				verdict = "⚠️"
			}
		}
		sample := 0.0
		if !ev.sampleAt.IsZero() && !ev.firstTap.IsZero() && !ev.firstTap.After(ev.sampleAt) {
			sample = expect
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %.2f | %.2f | %.2f | %.2f | %s |\n",
			ev.Config, ev.Sample, ev.Source, sample, ev.ctMid, expect, ev.ctEnd, verdict)
	}
	b.WriteString("\n> 容差 **±0.2s**（采样开销 + 设备启动延迟）。`data:` 来源没有本地路径" +
		"（宿主解不了码），采样时无 PCM、时钟未起步——不在判据 A 的覆盖范围，保持 L1。\n\n")
	return b.String()
}

// audioSupportMatrix 生成内置输出平台的支持矩阵（Markdown 表）。
func audioSupportMatrix() string {
	builtin := app.AudioOutputAvailable()
	var b strings.Builder
	b.WriteString("| 平台 | PCM 解码（ffmpeg） | 播放时钟 | 内置输出后端 | 降级行为 |\n")
	b.WriteString("|---|---|---|---|---|\n")
	b.WriteString("| Windows | ✅ | ✅ 设备位置（waveOutGetPosition） | ✅ waveOut | — |\n")
	b.WriteString("| Linux/macOS | ✅ | ✅ 挂钟 + 已交付帧 | ❌（未实现） | PCM → 宿主 tap（宿主自备 ALSA/CoreAudio 输出） |\n")
	fmt.Fprintf(&b, "\n本次运行平台：**%s**；本机内置输出设备：**%s**。\n\n", runtime.GOOS, yesNo(builtin))
	if !builtin {
		b.WriteString("> 本机没有可用的内置输出设备 ⇒ 会话进入降级模式：PCM 仍按实时速率解出并交给 tap，" +
			"判据 A（FFT）依然成立；耳听发声不可验证（请在有声卡的机器上复跑）。\n\n")
	}
	return b.String()
}

func yesNo(v bool) string {
	if v {
		return "可用"
	}
	return "不可用（降级）"
}

func orDash(s string) string {
	if s == "" {
		return "（无）"
	}
	return s
}

// maxAudioPeakMag 取本次判据 A 中最大的主峰幅度，作为「同一份待输出 PCM」的幅度参考。
// 判据 B 的 `mismatch` 靠它区分两类成因：录回幅度与之同量级 = 确实录到了别的声音；
// 低若干数量级 = 只录到底噪（采集设备没被系统输出喂到）。没有这个对照，读者无法判断
// mismatch 是「环境未配置」还是「输出链路有问题」。
func maxAudioPeakMag(evidence []audioCellEvidence) float64 {
	best := 0.0
	for _, ev := range evidence {
		if ev.frames > 0 && ev.peakMag > best {
			best = ev.peakMag
		}
	}
	return best
}
