package app

// 媒体元数据探测（实现路径主线 A0）。
//
// 为什么在宿主侧做：引擎不背解码器（体积/许可/跨平台成本），主线 A 的
// 「宿主注入帧流」路线要求宿主负责解码侧的一切；元数据是其中成本最低的一环。
// 不探测时 <video>/<audio> 的 duration 恒为 NaN——脚本读不到进度，时间轴、
// 进度条、倍速菜单全部退化（框架挂载后立刻读 duration）。
//
// 探测手段：`ffmpeg -i <file>` 把容器头打到 stderr（`Duration: 00:00:01.00`、
// 视频行的 `120x80`），不真正解码。本机没有 ffmpeg 时 resolver 返回 ok=false，
// 引擎保持「时长未知」（宁可让脚本走未知分支，也不编造进度）。

import (
	"bytes"
	"context"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"wb-ui/engine/dom"
	"wb-ui/engine/js/bindings"
	"wb-ui/engine/rendering"
	"wb-ui/webkit"
)

// mediaProbeTimeout 是单次探测上限：ffmpeg 读容器头是毫秒级，超时说明文件
// 异常或 ffmpeg 卡住——放弃探测，不拖住媒体元素的加载流程。
const mediaProbeTimeout = 3 * time.Second

// mediaFrameTimeout 是单次抽帧上限：`ffmpeg -ss <t> -frames:v 1` 在毫秒级完成，
// 超时说明文件异常或关键帧极远——放弃该帧（渲染层记为负缓存，不会反复重试）。
const mediaFrameTimeout = 5 * time.Second

// mediaFrameCacheLimit 是抽帧结果（PNG 字节）的缓存上限，FIFO 淘汰。解码结果在
// 渲染层另有一层缓存（engine/rendering/videoframe.go），两层都按 (文件, 时刻) 键。
const mediaFrameCacheLimit = 64

// mediaFrameFallbackStep 是「末尾收敛余量 / 抽帧失败回退步长」（秒）：最后一帧与
// duration 之间总有一小段空隙（1s/10fps 的视频末帧在 0.9s），因此播放到结束时
// 的 currentTime == duration 这个精确时刻**没有帧**——直接抽会空手而归、画面消失。
// 规范要求播放结束后保留最后一帧，所以越界请求收敛到 `duration - Step`。
//
// 实测（本机 ffmpeg）：`-ss 1.0`（越界）与 `-sseof -0.05` 都产出 0 字节，`-ss 0.9`
// 有帧——所以兜底走「按已知时长收敛 + 退一步重试」，不用 `-sseof`。
const mediaFrameFallbackStep = 0.1

// MediaProbe 是带缓存的 ffmpeg 元数据探测器（同一路径只探测一次，可并发）。
type MediaProbe struct {
	bin string // ffmpeg 可执行文件路径；空 = 本机没有

	mu     sync.Mutex
	cache  map[string]probeEntry
	warned bool // 「没找到 ffmpeg」只提示一次，避免每条媒体刷日志

	// frames 是抽帧结果缓存：key = "绝对路径@毫秒"。值为 nil 表示该帧取不到
	// （负缓存——painter 每次绘制都会来问，不能反复跑 ffmpeg）。
	frames      map[string][]byte
	frameKeys   []string
	frameWarned map[string]bool // 同一文件的抽帧失败只打一次日志

	// pump 是异步抽帧执行器（A2）：渲染层提交请求后立刻返回，帧由 worker
	// 交付。worker 惰性启动、空闲自退，因此没有需要释放的生命周期。
	pump *framePump
}

type probeEntry struct {
	meta bindings.MediaMetadata
	ok   bool
}

// NewMediaProbe 建探测器。ffmpegPath 为空时从 PATH 查找 ffmpeg。
func NewMediaProbe(ffmpegPath string) *MediaProbe {
	bin := ffmpegPath
	if strings.TrimSpace(bin) == "" {
		bin, _ = exec.LookPath("ffmpeg")
	}
	p := &MediaProbe{
		bin:         bin,
		cache:       map[string]probeEntry{},
		frames:      map[string][]byte{},
		frameWarned: map[string]bool{},
	}
	p.pump = newFramePump(p) // 只是建队列：worker 在第一次提交时才启动
	return p
}

// InstallMediaMetadataResolver 把探测器接到引擎的媒体元数据通道上：页面里
// <video>/<audio> 的 src 会按文档基准解析成本地文件路径后交给 ffmpeg。返回
// 探测器供调用方复用（抽帧 worker 惰性启动、空闲自退，不需要释放）。
func InstallMediaMetadataResolver(wv *webkit.WebView, ffmpegPath string) *MediaProbe {
	p := NewMediaProbe(ffmpegPath)
	base := func() string {
		if wv == nil {
			return ""
		}
		return wv.DocumentBaseURL()
	}
	bindings.MediaMetadataResolver = p.ResolverFor(base)
	// 同一条宿主注入链的第二环（主线 A1）：<video> 的当前帧也由宿主解。引擎侧只
	// 维护「元素 → 时间点」（绑定层写）与「(url, 时刻) → 已解码帧」（渲染层缓存）。
	// 第二环再分两条路（A2）：静止态走同步源（加载首帧 / seek / 暂停），播放推进
	// 走异步源（worker 池抽帧，渲染线程不等解码）。
	rendering.SetVideoFrameSource(p.FrameSourceFor(base))
	rendering.SetVideoAsyncFrameSource(p.AsyncFrameSourceFor(base))
	// 第三环（A2-①）：把「时刻」折叠成「帧」。帧率由同一份已探测元数据提供，
	// 因此这三环用的是同一套「某个时刻属于哪一帧」的判断。
	rendering.SetVideoFrameTimeQuantizer(p.FrameTimeQuantizer(base))
	return p
}

// ResolverFor 返回可直接赋给 bindings.MediaMetadataResolver 的解析函数。
// baseURL 是取文档基准的闭包（页面会导航，基准随之变化，不能在装配时固化）。
func (p *MediaProbe) ResolverFor(baseURL func() string) func(string) (bindings.MediaMetadata, bool) {
	return func(src string) (bindings.MediaMetadata, bool) {
		if p == nil {
			return bindings.MediaMetadata{}, false
		}
		base := ""
		if baseURL != nil {
			base = baseURL()
		}
		path, ok := mediaSrcToPath(src, base)
		if !ok {
			return bindings.MediaMetadata{}, false
		}
		return p.Resolve(path)
	}
}

// Resolve 探测本地媒体文件的元数据（带缓存）。ok=false 表示不是本地文件、
// 文件不存在、或探测失败。
func (p *MediaProbe) Resolve(path string) (bindings.MediaMetadata, bool) {
	if p == nil || strings.TrimSpace(path) == "" {
		return bindings.MediaMetadata{}, false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	p.mu.Lock()
	if e, hit := p.cache[abs]; hit {
		p.mu.Unlock()
		return e.meta, e.ok
	}
	bin := p.bin
	p.mu.Unlock()
	if bin == "" {
		p.mu.Lock()
		if !p.warned {
			p.warned = true
			log.Printf("[media] 未找到 ffmpeg：<video>/<audio> 只维护状态机，duration 保持 NaN")
		}
		p.mu.Unlock()
		return bindings.MediaMetadata{}, false
	}
	meta, ok := probeWithFFmpeg(bin, abs)
	p.mu.Lock()
	p.cache[abs] = probeEntry{meta: meta, ok: ok}
	p.mu.Unlock()
	return meta, ok
}

// FrameSourceFor 返回可直接交给 rendering.SetVideoFrameSource 的帧源。
// baseURL 是取文档基准的闭包（页面会导航，基准随之变化，不能在装配时固化）。
func (p *MediaProbe) FrameSourceFor(baseURL func() string) rendering.VideoFrameSource {
	return func(src string, atSeconds float64) ([]byte, bool) {
		if p == nil {
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
		return p.FrameAt(path, atSeconds)
	}
}

// FrameAt 抽取本地媒体在 atSeconds 处的帧（PNG 字节，带缓存）。ok=false 表示
// 本机没有 ffmpeg、文件不存在、或该时刻抽不到帧。
func (p *MediaProbe) FrameAt(path string, atSeconds float64) ([]byte, bool) {
	if p == nil || strings.TrimSpace(path) == "" {
		return nil, false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if atSeconds < 0 {
		atSeconds = 0
	}
	// 先按时长收敛（末尾帧）、再按帧率对齐（精确到帧）。★ 必须在算缓存键**之前**
	// 做：宿主与渲染层的折叠规则一致，同一帧的不同请求才会对上同一个键（否则
	// 同一帧会被反复解码）。顺序也不能反——收敛把越界时刻拉回「有帧」的区间，
	// 对齐再把它挪到目标帧的取样点。
	atSeconds = p.clampFrameTime(abs, atSeconds)
	atSeconds = p.frameAlign(abs, atSeconds)
	key := abs + "@" + strconv.FormatInt(int64(atSeconds*1000+0.5), 10)
	p.mu.Lock()
	if p.frames == nil {
		// 手工构造的 MediaProbe（测试/嵌入式用法）可能没有初始化缓存 map。
		p.frames = map[string][]byte{}
	}
	if p.frameWarned == nil {
		p.frameWarned = map[string]bool{}
	}
	if b, hit := p.frames[key]; hit {
		p.mu.Unlock()
		return b, b != nil
	}
	bin := p.bin
	p.mu.Unlock()
	if bin == "" {
		// 「没找到 ffmpeg」的提示已由 Resolve 打过（同一次装配），这里不重复。
		return nil, false
	}
	data, ok := extractFrameWithFFmpeg(bin, abs, atSeconds)
	// 兜底重试的步长：已知帧率就退**一帧**，否则退 mediaFrameFallbackStep（见常量
	// 注释）。★ 重试值不再做帧对齐：ffmpeg 的 ceil 语义会把「目标帧取样点减一帧」
	// 落在上一帧上，正是想要的（再对齐一次反而会退两帧）。
	step := mediaFrameFallbackStep
	if fps, hasFPS := p.FPS(abs); hasFPS {
		step = 1 / fps
	}
	if (!ok || len(data) == 0) && atSeconds >= step {
		// 兜底：容器时长不准或请求正好落在边界上时，退一帧重试一次。
		if retry, rok := extractFrameWithFFmpeg(bin, abs, atSeconds-step); rok && len(retry) > 0 {
			data, ok = retry, true
		}
	}
	var out []byte
	if ok && len(data) > 0 {
		out = data
	}
	p.mu.Lock()
	if _, exists := p.frames[key]; !exists {
		p.frames[key] = out
		p.frameKeys = append(p.frameKeys, key)
		for len(p.frameKeys) > mediaFrameCacheLimit {
			oldest := p.frameKeys[0]
			p.frameKeys = p.frameKeys[1:]
			delete(p.frames, oldest)
		}
	}
	p.mu.Unlock()
	if out == nil {
		p.warnFrameFailure(abs, atSeconds)
	}
	return out, out != nil
}

// clampFrameTime 把请求时刻收敛到「有帧可抽」的范围：已知时长时，落在
// `[duration-Step, ∞)` 的请求回退到 `duration-Step`（见常量注释：末帧与 duration
// 之间总有空隙）。时长未知时原样返回，由抽帧结果说话。
func (p *MediaProbe) clampFrameTime(abs string, at float64) float64 {
	if at <= 0 {
		return 0
	}
	dur, ok := p.Duration(abs)
	if !ok {
		return at
	}
	if at >= dur-mediaFrameFallbackStep {
		tail := dur - mediaFrameFallbackStep
		if tail < 0 {
			tail = 0
		}
		return tail
	}
	return at
}

// alignFrameTime 把请求时刻对齐到「该时刻**所属**的那一帧」的取样点（A2-①：
// 精确到帧的 seek）。
//
// 实测事实（本机 ffmpeg + 每帧一色的 10fps 样本，逐帧比对 select=eq(n,k) 的
// 参照帧）：`-ss t` 取的是**时间戳 ≥ t 的第一帧**，即 ceil 语义；而 HTML 的
// currentTime 语义是「显示 t 所属的那一帧」（floor 语义）。两者差一帧，直接拿
// currentTime 抽帧会整体提前一帧——实测请求 0.05s 得到第 1 帧、请求 0.01s 直接
// 跳过首帧。
//
// 修正：把请求值左移到目标帧区间的左半边。帧 k 的时间戳是 k/fps，目标帧区间是
// [k/fps, (k+1)/fps)，落在其中且早于 (k+1)/fps 的取样点是 (k-0.5)/fps。实测该
// 规则下「输入侧 -ss」「输出侧 -ss」「输入侧预跳 + 输出侧精确」三种调用方式都
// 取到第 k 帧（含 k=0 与末帧边界）。
//
// fps <= 0（帧率未知）时原样返回：无法判断帧边界时保持既有行为（ceil 语义，偏差
// ≤ 一帧），不猜。
func alignFrameTime(at, fps float64) float64 {
	if fps <= 0 || math.IsNaN(at) || at <= 0 {
		return at
	}
	k := math.Floor(at*fps + 1e-6)
	if k < 0.5 {
		return 0 // 第 0 帧的取样点就是 0（负值没有帧）
	}
	return (k - 0.5) / fps
}

// frameAlign 按**已缓存**的帧率对齐请求时刻（不触发探测）。
func (p *MediaProbe) frameAlign(abs string, at float64) float64 {
	fps, ok := p.FPS(abs)
	if !ok {
		return at
	}
	return alignFrameTime(at, fps)
}

// FPS 返回**已缓存**的视频帧率（不触发探测：探测由元数据阶段完成）。ok=false
// 表示该文件还没探测过、探测失败、或容器没有帧率信息（纯音频 / 部分流式容器）。
func (p *MediaProbe) FPS(abs string) (float64, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, hit := p.cache[abs]
	if !hit || !e.ok || e.meta.FPS <= 0 || math.IsNaN(e.meta.FPS) {
		return 0, false
	}
	return e.meta.FPS, true
}

// FrameTimeQuantizer 返回可直接交给 rendering.SetVideoFrameTimeQuantizer 的时刻
// 量化器（A2-①）：把「同一帧的不同时刻」折叠成一个键。
//
// 规则与 frameAlign 完全一致（也就是抽帧实际使用的时刻），因此折叠后的键与其
// 对应画面严格一致：渲染层的缓存与宿主抽帧缓存都按帧共享条目，预取与绘制因此
// 更容易互相命中。帧率未知（或 src 不是本地文件）时返回原值——不折叠。
func (p *MediaProbe) FrameTimeQuantizer(baseURL func() string) func(string, float64) float64 {
	return func(src string, atSeconds float64) float64 {
		if p == nil {
			return atSeconds
		}
		base := ""
		if baseURL != nil {
			base = baseURL()
		}
		path, ok := mediaSrcToPath(src, base)
		if !ok {
			return atSeconds
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		return p.frameAlign(abs, atSeconds)
	}
}

// Duration 返回**已缓存**的媒体时长（不触发探测：探测由元数据阶段完成，见
// Resolve）。ok=false 表示该文件还没探测过、探测失败、或时长未知（流式输入）。
func (p *MediaProbe) Duration(abs string) (float64, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, hit := p.cache[abs]
	if !hit || !e.ok || math.IsNaN(e.meta.Duration) || e.meta.Duration <= 0 {
		return 0, false
	}
	return e.meta.Duration, true
}

// warnFrameFailure 对同一文件的抽帧失败只打一次日志（负缓存已避免重复抽帧，
// 但 A2 的长播放会问很多时间点——坏文件不该把日志刷爆）。
func (p *MediaProbe) warnFrameFailure(abs string, at float64) {
	p.mu.Lock()
	if p.frameWarned[abs] {
		p.mu.Unlock()
		return
	}
	p.frameWarned[abs] = true
	p.mu.Unlock()
	log.Printf("[media] 抽帧失败（该文件后续失败不再重复提示）：%s @%.3fs", filepath.Base(abs), at)
}

// extractFrameWithFFmpeg 用 ffmpeg 抽一帧并输出 PNG 字节（image2pipe → stdout）。
//
// -ss 放在 -i 之前 = **快速（关键帧）定位**：抽帧的用途是「显示这一时刻附近的
// 画面」，不需要精确到帧；精确抽帧（-ss 放 -i 后）要解码到该时刻的全部帧，成本
// 随时刻线性增长。代价是画面可能落在该时刻之前的关键帧上——A2（精确 seek）需要
// 重新评估，见 docs/TECH_DEBT.md。
func extractFrameWithFFmpeg(bin, path string, at float64) ([]byte, bool) {
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), mediaFrameTimeout)
	defer cancel()
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error"}
	if at > 0 {
		args = append(args, "-ss", strconv.FormatFloat(at, 'f', 3, 64))
	}
	args = append(args, "-i", path, "-frames:v", "1", "-f", "image2pipe", "-vcodec", "png", "-")
	cmd := exec.CommandContext(ctx, bin, args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, false
	}
	if err != nil {
		log.Printf("[media] ffmpeg 抽帧出错 %s @%.3fs: %v（%s）", filepath.Base(path), at, err, strings.TrimSpace(errBuf.String()))
		return nil, false
	}
	if out.Len() == 0 {
		return nil, false
	}
	return out.Bytes(), true
}

// probeWithFFmpeg 跑一次 `ffmpeg -i` 并解析输出。
func probeWithFFmpeg(bin, path string) (bindings.MediaMetadata, bool) {
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		return bindings.MediaMetadata{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), mediaProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-hide_banner", "-nostdin", "-i", path)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	// 没有输出文件 → ffmpeg 退出码非 0，但容器信息已经打到 stderr：只看输出。
	_ = cmd.Run()
	if ctx.Err() != nil {
		return bindings.MediaMetadata{}, false
	}
	return parseFFmpegProbe(buf.String())
}

var (
	// Duration: 00:00:01.00 / Duration: 00:01:23.45
	ffDurationRe = regexp.MustCompile(`Duration:\s*(\d+):(\d{2}):(\d{2}(?:\.\d+)?)`)
	// 含音频流的那一行（`Stream #0:1: Audio: aac (LC) …`）。A3：宿主据此告诉
	// 引擎「这个资源有没有音频轨」——没有音轨的资源不该打开音频会话（否则
	// play() 之后引擎会挂到一条永不推进的音频时钟上）。
	ffAudioStreamRe = regexp.MustCompile(`\bAudio:`)
	// 视频行的像素尺寸（120x80、1920x1080）。首位非 0 才能躲开编解码器串里的
	// `0x31637661` 一类十六进制。
	ffVideoSizeRe = regexp.MustCompile(`([1-9]\d{0,4})x([1-9]\d{0,4})`)
	// 视频行的帧率（`10 fps`、`29.97 fps`）。只在含 `Video:` 的行上匹配，避免
	// 命中音频/字幕行里的奇怪串。
	ffFPSRe = regexp.MustCompile(`([1-9]\d*(?:\.\d+)?)\s*fps`)
)

// parseFFmpegProbe 解析 `ffmpeg -i` 的 stderr：Duration 行给时长，含 `Video:`
// 的行给像素尺寸。时长缺失（`Duration: N/A`、流式输入）时 Duration 保持 NaN
// ——引擎据此停在 HAVE_METADATA，不声称能播放。两者都没解析到 → ok=false。
func parseFFmpegProbe(out string) (bindings.MediaMetadata, bool) {
	meta := bindings.MediaMetadata{Duration: math.NaN()}
	found := false
	for _, line := range strings.Split(out, "\n") {
		if m := ffDurationRe.FindStringSubmatch(line); m != nil {
			h, _ := strconv.Atoi(m[1])
			mi, _ := strconv.Atoi(m[2])
			sec, _ := strconv.ParseFloat(m[3], 64)
			meta.Duration = float64(h*3600+mi*60) + sec
			found = true
			continue
		}
		if ffAudioStreamRe.MatchString(line) {
			// 有音轨：只置 HasAudio，**不**置 found——「有音频流」本身不是引擎
			// 能用上的元数据（时长/尺寸才是），与帧率同一处理。
			meta.HasAudio = true
			continue
		}
		if strings.Contains(line, "Video:") {
			if meta.Width == 0 {
				if m := ffVideoSizeRe.FindStringSubmatch(line); m != nil {
					w, _ := strconv.Atoi(m[1])
					h, _ := strconv.Atoi(m[2])
					meta.Width, meta.Height = w, h
					found = true
				}
			}
			// 帧率是附加信息：单独命中它**不**算「探测到元数据」（found 只由时长
			// 与尺寸置位）——只有帧率而没有时长/尺寸的容器对引擎没有意义。
			if meta.FPS == 0 {
				if m := ffFPSRe.FindStringSubmatch(line); m != nil {
					if v, err := strconv.ParseFloat(m[1], 64); err == nil && v > 0 {
						meta.FPS = v
					}
				}
			}
		}
	}
	return meta, found
}

// mediaSrcToPath 把媒体元素的 src 映射成本地文件路径。http(s)/blob 一律返回
// false（引擎无网络栈，属另一条主线）；`data:` 先把内联字节落盘再当本地文件
// （见 mediadataurl.go——这条链路只认路径，ffmpeg 读不了「没有路径的字节」）；
// 相对引用按文档基准解析；没有基准时按宿主工作目录（与 LoadHTML 直出内容下的
// 图片读取行为一致）。
func mediaSrcToPath(src, base string) (string, bool) {
	s := strings.TrimSpace(src)
	if s == "" {
		return "", false
	}
	low := strings.ToLower(s)
	switch {
	case strings.HasPrefix(low, "data:"):
		// 落盘失败（编码非法 / 内容为空）时与 http(s)/blob 同一条路：拿不到路径。
		return mediaDataURLToFile(s)
	case strings.HasPrefix(low, "blob:"):
		return "", false
	case strings.HasPrefix(low, "http://"), strings.HasPrefix(low, "https://"):
		return "", false
	case strings.HasPrefix(low, "file://"):
		return webkit.FileURLPath(s), true
	}
	if base != "" {
		abs := dom.ResolveURL(base, s)
		if abs == "" {
			return "", false
		}
		if strings.HasPrefix(strings.ToLower(abs), "file://") {
			return webkit.FileURLPath(abs), true
		}
		if strings.Contains(abs, "://") {
			return "", false // app://、http:// 基准下的相对引用不是本地文件
		}
		return abs, true
	}
	return s, true
}
