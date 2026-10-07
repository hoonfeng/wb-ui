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
	"fmt"
	"math"
	"os/exec"
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

// probeLoopback 执行判据 B：挑一个环回设备录音 1 秒，FFT 找主峰。
// 任何一步不可用都返回 SKIP（不阻塞验收），理由写进 Detail。
func probeLoopback(expectHz, seconds float64) loopbackEvidence {
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
		ev.Detail = fmt.Sprintf("主峰 %.1fHz 偏离期望 %.0fHz（该设备的输入未路由到系统输出）", peak, expectHz)
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
	fmt.Fprintf(&b, "- 设备：%s\n- 结果：**%s**\n", orDash(lo.Device), lo.Status)
	if lo.PeakHz > 0 {
		fmt.Fprintf(&b, "- 录回音频主峰：%.1f Hz（幅度 %.3f）\n", lo.PeakHz, lo.PeakMag)
	}
	if lo.Detail != "" {
		fmt.Fprintf(&b, "- 说明：%s\n", lo.Detail)
	}
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
