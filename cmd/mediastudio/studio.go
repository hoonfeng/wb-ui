package main

// MediaStudio 宿主逻辑 —— 录屏 / 录音 / 合成 / 剪辑 / 导出 / 回放。
//
// 为什么这个应用本身就是媒体验证：它的每一步都压在本仓媒体链路上——
//
//	舞台画面 ← 引擎渲染（布局 + 绘制 + CSS 动画 + 图片解码 + 宿主抽帧的 <video>）
//	录制抓帧 ← WebView.Render()（引擎像素出口，预乘 RGBA）
//	系统声音 ← WASAPI 环回采集（app.CaptureDefaultRenderLoopback）+ 输出设备播放（PlayTestTone）
//	视频合成 ← ffmpeg（宿主编码侧；引擎不背编解码器，见 app/mediaprobe.go 包注释）
//	剪辑     ← 应用自持帧序列的区间选取（不依赖容器 seek）
//	回放     ← 引擎 <video> + 宿主抽帧通道（app.InstallMediaMetadataResolver）
//
// 因此「应用能录出一段正确的视频、并能把它播回来」= 媒体链路端到端可用；
// 录制产物本身（mp4 + wav + 帧 PNG）就是验证证据。

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"wb-ui/app"
	"wb-ui/engine/rendering"
	"wb-ui/webkit"
)

const (
	// framePattern 是帧序列命名（ffmpeg image2 demuxer 用同一模式串 + -start_number 1）。
	framePattern = "f%04d.png"
	// testToneHz 是录音期间的已知测试音频率（判据 B 的同一手法）。
	testToneHz = 440.0
)

// assetInfo 是产物清单的一条（页面「产物」面板与报告共用）。
type assetInfo struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	Detail string `json:"detail,omitempty"`
}

// take 是一次录制（或一次剪辑产物）的完整结果。
type take struct {
	dir       string          // 帧目录（f0001.png …）
	frames    int             // 帧数
	fps       int             // 帧率
	seconds   float64         // 名义时长
	stage     image.Rectangle // 舞台在视口里的裁切矩形
	wav       string          // 音轨（环回录音的 WAV，可为空）
	audioFmt  rendering.AudioFormat
	pcmBytes  int
	mp4       string
	mp4Bytes  int64
	frameDiff float64 // 首末帧平均像素差（>0 证明录制期间画面真的在变）
}

// replayRec 是回放验证的观测结果。
type replayRec struct {
	Duration    float64
	ReadyState  float64
	VideoW      int
	VideoH      int
	CurrentTime float64
	Events      string
	PreviewRect image.Rectangle
	Shot        string  // 回放截图（out/replay.png）
	MidFrame    string  // 对比用录制中间帧
	SigDiff     float64 // 网格签名平均差（0-255 尺度；越小越像）
}

// studio 是应用宿主运行态。
type studio struct {
	wv     *webkit.WebView
	ffmpeg string
	cfg    options
	webDir string

	mu       sync.Mutex
	busy     bool
	state    string
	last     *take
	clipTk   *take
	assets   []assetInfo
	shotURL  string
	videoURL string
	logs     []string

	// animStart 是本应用充当 embedder 时的动画时钟起点（见 advanceFrame）。
	animStart time.Time
}

func newStudio(wv *webkit.WebView, ffmpeg string, cfg options) *studio {
	webDir, err := filepath.Abs(filepath.Join("cmd", "mediastudio", "web"))
	if err != nil {
		webDir = filepath.Join("cmd", "mediastudio", "web")
	}
	samples, _ := filepath.Abs(cfg.samples)
	s := &studio{wv: wv, ffmpeg: ffmpeg, cfg: cfg, webDir: webDir, state: "ready", animStart: time.Now()}
	shot := filepath.Join(samples, "quad.png")
	video := filepath.Join(samples, "testsrc-1s.mp4")
	if _, err := os.Stat(shot); err != nil {
		shot = filepath.Join(samples, "quad.jpg")
	}
	s.shotURL = fileURLOf(shot)
	s.videoURL = fileURLOf(video)
	return s
}

// indexPath 是应用页面路径。
func (s *studio) indexPath() string { return filepath.Join(s.webDir, "index.html") }

// resolver 是宿主资源通道：只提供三个根内的文件——应用目录（web/）、素材样本目录、
// 以及**应用自己的产物目录**（录出来的 mp4/wav 要能被引擎 <video>/<audio> 读回去，
// 回放才成立；这与真实应用「媒体库能预览自己的产物」是同一条需求）。
// 与媒体门禁共用同一条判定（webkit.MediaResourceAllowed → resolver 命中放行）。
func (s *studio) resolver(ref string) (string, bool) {
	p := ref
	switch {
	case strings.HasPrefix(ref, "file://"):
		p = webkit.FileURLPath(ref)
	case strings.HasPrefix(ref, "http://"), strings.HasPrefix(ref, "https://"):
		return "", false
	}
	abs, err := filepath.Abs(filepath.FromSlash(p))
	if err != nil {
		return "", false
	}
	samplesAbs, _ := filepath.Abs(s.cfg.samples)
	outAbs, _ := filepath.Abs(s.cfg.out)
	tmpAbs, _ := filepath.Abs(s.cfg.tmp)
	for _, root := range []string{s.webDir, samplesAbs, outAbs, tmpAbs} {
		rel, err := filepath.Rel(root, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		b, err := os.ReadFile(abs)
		if err != nil {
			return "", false
		}
		return string(b), true
	}
	return "", false
}

// note 记录一条应用日志（打印 + 报告）。
func (s *studio) note(format string, a ...any) string {
	msg := fmt.Sprintf(format, a...)
	s.mu.Lock()
	s.logs = append(s.logs, msg)
	s.mu.Unlock()
	fmt.Println("[mediastudio] " + msg)
	return msg
}

// addAsset 登记/更新一条产物。
func (s *studio) addAsset(name, path, detail string) {
	var size int64
	if st, err := os.Stat(path); err == nil {
		size = st.Size()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.assets {
		if s.assets[i].Name == name {
			s.assets[i] = assetInfo{Name: name, Path: path, Bytes: size, Detail: detail}
			return
		}
	}
	s.assets = append(s.assets, assetInfo{Name: name, Path: path, Bytes: size, Detail: detail})
}

// settle 推进引擎事件循环并渲染若干轮（页面脚本/异步加载/Promise 回调落地）。
func (s *studio) settle(rounds int) {
	start := time.Now()
	for i := 0; i < rounds; i++ {
		s.advanceFrame(time.Since(start).Milliseconds(), true)
		if _, err := s.wv.Render(); err != nil {
			return
		}
		time.Sleep(3 * time.Millisecond)
	}
}

// advanceFrame 推进一帧：事件循环 → 微任务 → 动画 → 布局（与 app.Host 的帧循环同序，
// 同 dev/probes/webshot 的 pumpFrames）。
//
// ★ 为什么无头宿主必须自己驱动动画：引擎把 @keyframes / transition 的补间放在**绘制期**
// 按 rendering.AnimationTime（全局秒）计算，且只有宿主每帧调用 rendering.ApplyAnimations(rv)
// 才生效（app/host.go 的帧循环）。缺了这两步，所有 CSS 动画永久停在 0%——实测：
// 录制 30 帧画面纹丝不动（首末帧像素差只有时间戳文字在变）。那是宿主侧缺失，不是引擎缺陷。
func (s *studio) advanceFrame(elapsedMs int64, driveLoop bool) {
	if driveLoop {
		driveEventLoop(s.wv, elapsedMs)
	}
	rendering.AnimationTime = time.Since(s.animStart).Seconds()
	if rv := s.wv.RenderView(); rv != nil {
		if rendering.ApplyAnimations(rv) && rendering.AnimationsAffectLayout(rv) {
			if mf := s.wv.MainFrame(); mf != nil {
				if fr := mf.Frame(); fr != nil {
					fr.SetNeedsLayout(true)
				}
			}
		}
	}
	s.wv.EnsureLayout()
	rendering.AnimationTime = time.Since(s.animStart).Seconds()
}

// driveEventLoop 处理到期的宏任务与微任务 —— 与 app.Host 每帧做的同一件事。
func driveEventLoop(wv *webkit.WebView, elapsedMs int64) {
	interp := wv.JSInterpreter()
	if interp == nil {
		return
	}
	if el := interp.GetEventLoop(); el != nil {
		el.ProcessTasks(elapsedMs)
	}
	interp.RunJobs()
}

// waitFor 推进事件循环直到 pred 成立或超时。
func (s *studio) waitFor(pred func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if pred() {
			return true
		}
		if !time.Now().Before(deadline) {
			return pred()
		}
		s.advanceFrame(16, true)
		_, _ = s.wv.Render()
		time.Sleep(16 * time.Millisecond)
	}
}

// click 在元素中心模拟「按下 + 释放」：走引擎命中测试 → 页面 DOM 事件 →
// 页面处理器（fetch → bridge 路由）。无头模式下点击的真实路径。
func (s *studio) click(sel string) bool {
	script := "(function(){var el=document.querySelector(" + jsString(sel) + ");" +
		"if(!el)return '';var r=el.getBoundingClientRect();" +
		"return (r.left+r.width/2)+','+(r.top+r.height/2);})()"
	v, err := s.wv.EvalJS(script)
	if err != nil {
		s.note("点击 %s 失败：%v", sel, err)
		return false
	}
	var x, y float64
	if _, err := fmt.Sscanf(v.ToString(), "%f,%f", &x, &y); err != nil {
		s.note("点击 %s 失败：取不到元素中心（%q）", sel, v.ToString())
		return false
	}
	s.wv.HandleMouseMove(x, y)
	s.wv.HandleMouseButton(x, y, 0, 0)
	s.wv.HandleMouseButton(x, y, 0, 1)
	s.note("点击 %s @ (%.1f, %.1f)", sel, x, y)
	return true
}

// stageRect 读舞台元素在视口里的矩形（录制裁切区域）。
func (s *studio) stageRect() (image.Rectangle, error) {
	v, err := s.wv.EvalJS(`(function(){
		var el = document.getElementById('stage');
		if (!el) return '';
		var r = el.getBoundingClientRect();
		return JSON.stringify({x: Math.round(r.left), y: Math.round(r.top),
			w: Math.round(r.width), h: Math.round(r.height)});
	})()`)
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("读舞台矩形失败：%w", err)
	}
	raw := unquoteJSON(strings.TrimSpace(v.ToString()))
	if raw == "" || raw == "undefined" || raw == "null" {
		return image.Rectangle{}, fmt.Errorf("舞台元素不存在（页面未就绪？）")
	}
	var g struct {
		X int `json:"x"`
		Y int `json:"y"`
		W int `json:"w"`
		H int `json:"h"`
	}
	if err := json.Unmarshal([]byte(raw), &g); err != nil {
		return image.Rectangle{}, fmt.Errorf("解析舞台矩形失败（%q）：%w", raw, err)
	}
	return image.Rect(g.X, g.Y, g.X+g.W, g.Y+g.H), nil
}

// record 执行一次录屏 + 录音 + 合成。drive 表示录制循环内是否推进事件循环
//（无头/宿主顶层调用为 true；嵌套在引擎事件循环内调用时为 false，避免重入）。
func (s *studio) record(seconds float64, fps int, withAudio, drive bool) (*take, error) {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return nil, fmt.Errorf("已有录制在进行中")
	}
	s.busy = true
	s.state = "recording"
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.busy = false
		s.state = "ready"
		s.mu.Unlock()
	}()

	rect, err := s.stageRect()
	if err != nil {
		return nil, err
	}
	rect = evenRect(rect, s.wv.Width(), s.wv.Height())
	if rect.Dx() < 16 || rect.Dy() < 16 {
		return nil, fmt.Errorf("舞台矩形异常：%v", rect)
	}

	framesDir := filepath.Join(s.cfg.tmp, "frames")
	if err := os.RemoveAll(framesDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(framesDir, 0o755); err != nil {
		return nil, err
	}

	fps = clampInt(fps, 2, 30)
	if seconds < 0.5 {
		seconds = 0.5
	}
	// 动画时钟从录制起点归零：录出来的片段总是从 0% 相位开始（可复现）。
	s.animStart = time.Now()
	tk := &take{dir: framesDir, fps: fps, seconds: seconds, stage: rect}
	s.note("开始录制：%s（%.1fs @ %dfps，音轨=%v）", rect, seconds, fps, withAudio)

	// ── 音频：先起录音、再起播放（WASAPI loopback 的已知语义：端点无音频在播时
	//    不产生静音包，见 app/loopback_windows.go）。
	var wg sync.WaitGroup
	var pcm []byte
	var afmt rendering.AudioFormat
	var aerr error
	recording := withAudio && app.LoopbackCaptureAvailable() && app.AudioOutputAvailable()
	if recording {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pcm, afmt, aerr = app.CaptureDefaultRenderLoopback(seconds + 0.4)
		}()
		time.Sleep(150 * time.Millisecond)
		go func() {
			_ = app.PlayTestTone(testToneHz, seconds+0.15, rendering.AudioFormat{SampleRate: 48000, Channels: 2})
		}()
	} else if withAudio {
		s.note("录音不可用（环回采集或输出设备缺失）→ 本次只有视频轨")
	}

	// ── 主循环：按帧率对齐抓帧
	count := int(math.Round(seconds * float64(fps)))
	if count < 1 {
		count = 1
	}
	step := time.Second / time.Duration(fps)
	prev := time.Now()
	var first, last []byte
	for i := 1; i <= count; i++ {
		now := time.Now()
		s.advanceFrame(now.Sub(prev).Milliseconds(), drive)
		prev = now
		px, err := s.wv.Render()
		if err != nil {
			return nil, fmt.Errorf("第 %d 帧渲染失败：%w", i, err)
		}
		img, err := s.writeFrame(filepath.Join(framesDir, fmt.Sprintf(framePattern, i)), px, rect)
		if err != nil {
			return nil, err
		}
		// 画面变化量按**舞台区域**算（录制内容本身）：整幅视口里舞台只占一部分，
		// 拿整幅平均会把动画位移稀释掉。
		if i == 1 {
			first = append([]byte(nil), img.Pix...)
		}
		last = append(last[:0], img.Pix...)
		if sleep := step - time.Since(now); sleep > 0 {
			time.Sleep(sleep)
		}
	}
	tk.frames = count
	if len(first) > 0 && len(first) == len(last) {
		tk.frameDiff = changedPixelRatio(first, last)
	}
	s.note("抓帧完成：%d 帧，首末帧变化像素占比 %.2f%%（舞台区域）", count, tk.frameDiff)

	wg.Wait()
	if aerr != nil {
		s.note("录音失败（本次不带音轨）：%v", aerr)
	} else if len(pcm) > 0 {
		wav := filepath.Join(s.cfg.tmp, "rec.wav")
		if err := writeWAV(wav, pcm, afmt); err != nil {
			return nil, err
		}
		tk.wav, tk.audioFmt, tk.pcmBytes = wav, afmt, len(pcm)
		s.note("环回录音：%d 字节 PCM（%dHz %dch，%.2fs）", len(pcm), afmt.SampleRate, afmt.Channels,
			float64(len(pcm))/float64(afmt.BytesPerFrame()*afmt.SampleRate+1))
	}

	// 录制产物直接落输出目录：它既是回放素材（引擎要读回去），也是本轮验证证据。
	out := filepath.Join(s.cfg.out, "record.mp4")
	if err := os.MkdirAll(s.cfg.out, 0o755); err != nil {
		return nil, err
	}
	if err := s.composeMP4(framesDir, fps, tk.wav, -1, -1, out); err != nil {
		return nil, err
	}
	tk.mp4 = out
	if st, err := os.Stat(out); err == nil {
		tk.mp4Bytes = st.Size()
	}

	s.mu.Lock()
	s.last = tk
	s.mu.Unlock()
	s.addAsset("record.mp4", out, fmt.Sprintf("%d 帧 @%dfps · %.1fs", count, fps, seconds))
	if tk.wav != "" {
		s.addAsset("rec.wav", tk.wav, fmt.Sprintf("%dHz %dch · %.1fs", tk.audioFmt.SampleRate, tk.audioFmt.Channels, tk.seconds))
	}
	mid := filepath.Join(framesDir, fmt.Sprintf(framePattern, maxInt(1, count/2)))
	s.addAsset("frame-mid.png", mid, fmt.Sprintf("%dx%d", rect.Dx(), rect.Dy()))
	s.note("合成完成：record.mp4（%d 字节）", tk.mp4Bytes)
	return tk, nil
}

// writeFrame 把视口像素里 rect 区域裁出来写 PNG（引擎像素是预乘 RGBA），
// 并返回裁切后的图像（录制循环用它算「画面是否随时间变化」）。
func (s *studio) writeFrame(path string, px []byte, rect image.Rectangle) (*image.NRGBA, error) {
	img := nrgbaFromPremul(px, s.wv.Width(), s.wv.Height(), rect)
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return nil, err
	}
	return img, nil
}

// fullPNG 渲染整幅视口并写 PNG（界面截图/回放截图）。
func (s *studio) fullPNG(path string) error {
	s.wv.EnsureLayout()
	px, err := s.wv.Render()
	if err != nil {
		return err
	}
	vw, vh := s.wv.Width(), s.wv.Height()
	if len(px) < vw*vh*4 {
		return fmt.Errorf("像素缓冲不足：%d < %d", len(px), vw*vh*4)
	}
	img := nrgbaFromPremul(px, vw, vh, image.Rect(0, 0, vw, vh))
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// exportFrame 导出当前画面（整幅视口）为 PNG 或 JPEG —— 图片编码链路的应用侧出口。
func (s *studio) exportFrame(format string) (string, error) {
	s.wv.EnsureLayout()
	px, err := s.wv.Render()
	if err != nil {
		return "", err
	}
	vw, vh := s.wv.Width(), s.wv.Height()
	if len(px) < vw*vh*4 {
		return "", fmt.Errorf("像素缓冲不足：%d < %d", len(px), vw*vh*4)
	}
	img := nrgbaFromPremul(px, vw, vh, image.Rect(0, 0, vw, vh))
	name, enc := "frame-now.png", func(w io.Writer, m image.Image) error { return png.Encode(w, m) }
	if strings.EqualFold(format, "jpeg") || strings.EqualFold(format, "jpg") {
		name = "frame-now.jpg"
		enc = func(w io.Writer, m image.Image) error { return jpeg.Encode(w, m, &jpeg.Options{Quality: 92}) }
	}
	path := filepath.Join(s.cfg.out, name)
	if err := os.MkdirAll(s.cfg.out, 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := enc(f, img); err != nil {
		return "", err
	}
	s.addAsset(name, path, fmt.Sprintf("%dx%d", vw, vh))
	s.note("导出当前帧：%s", path)
	return path, nil
}

// exportClip 按 in/out（秒）从最近一次录制的帧序列里剪出区间并重新合成。
// 剪辑作用在应用自持的帧序列上（不是容器 seek），因此它验证的是「帧轨 + 音频轨的
// 独立重组」，而不是 ffmpeg 的 seek 能力。
func (s *studio) exportClip(inSec, outSec float64) (*take, error) {
	s.mu.Lock()
	src := s.last
	s.mu.Unlock()
	if src == nil {
		return nil, fmt.Errorf("尚无录制结果可剪辑")
	}
	if inSec < 0 {
		inSec = 0
	}
	if outSec > src.seconds {
		outSec = src.seconds
	}
	if outSec-inSec < 0.1 {
		return nil, fmt.Errorf("剪辑区间过短（%.2fs）", outSec-inSec)
	}
	i0 := int(math.Floor(inSec*float64(src.fps))) + 1
	i1 := int(math.Round(outSec * float64(src.fps)))
	i0 = clampInt(i0, 1, src.frames)
	i1 = clampInt(i1, i0, src.frames)

	dir := filepath.Join(s.cfg.tmp, "clip")
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	n := 0
	for i := i0; i <= i1; i++ {
		b, err := os.ReadFile(filepath.Join(src.dir, fmt.Sprintf(framePattern, i)))
		if err != nil {
			return nil, err
		}
		n++
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf(framePattern, n)), b, 0o644); err != nil {
			return nil, err
		}
	}
	out := filepath.Join(s.cfg.out, "clip.mp4")
	if err := os.MkdirAll(s.cfg.out, 0o755); err != nil {
		return nil, err
	}
	if err := s.composeMP4(dir, src.fps, src.wav, inSec, outSec-inSec, out); err != nil {
		return nil, err
	}
	ct := &take{dir: dir, frames: n, fps: src.fps, seconds: outSec - inSec, stage: src.stage, wav: src.wav, mp4: out}
	if st, err := os.Stat(out); err == nil {
		ct.mp4Bytes = st.Size()
	}
	s.mu.Lock()
	s.clipTk = ct
	s.mu.Unlock()
	s.addAsset("clip.mp4", out, fmt.Sprintf("%d 帧 · %.1fs（%.1f→%.1fs）", n, ct.seconds, inSec, outSec))
	s.note("剪辑完成：clip.mp4（%d 帧，%.1fs→%.1fs，%d 字节）", n, inSec, outSec, ct.mp4Bytes)
	return ct, nil
}

// composeMP4 用 ffmpeg 把帧序列（+ 可选音轨）合成 mp4。ss/dur > 0 时对音轨做输入级裁剪。
// 先试 libx264，不可用则回退 mpeg4（本机 ffmpeg 构建差异的兜底）。
func (s *studio) composeMP4(framesDir string, fps int, wav string, ss, dur float64, out string) error {
	if fps <= 0 {
		fps = 10
	}
	head := []string{"-y", "-hide_banner", "-loglevel", "error",
		"-framerate", strconv.Itoa(fps), "-start_number", "1",
		"-i", filepath.ToSlash(filepath.Join(framesDir, "f%04d.png"))}
	if wav != "" {
		if ss > 0 {
			head = append(head, "-ss", fmt.Sprintf("%.3f", ss))
		}
		if dur > 0 {
			head = append(head, "-t", fmt.Sprintf("%.3f", dur))
		}
		head = append(head, "-i", filepath.ToSlash(wav))
	}
	encs := [][]string{
		{"-c:v", "libx264", "-pix_fmt", "yuv420p", "-preset", "veryfast", "-crf", "20"},
		{"-c:v", "mpeg4", "-pix_fmt", "yuv420p", "-q:v", "3"},
	}
	var lastErr error
	for _, enc := range encs {
		args := append(append([]string{}, head...), enc...)
		if wav != "" {
			args = append(args, "-c:a", "aac", "-b:a", "128k", "-shortest")
		}
		args = append(args, filepath.ToSlash(out))
		cmd := exec.Command(s.ffmpeg, args...)
		b, err := cmd.CombinedOutput()
		if err != nil {
			lastErr = fmt.Errorf("ffmpeg 合成失败（%s）：%v %s", strings.Join(enc, " "), err, strings.TrimSpace(string(b)))
			continue
		}
		return nil
	}
	return lastErr
}

// replaySeekAndShot 驱动页面 <video> 播放录制的 mp4、seek 到中段并截图 —— 回放侧
// 的端到端验证（元数据链路 + 宿主抽帧通道都在这条路上）。
func (s *studio) replaySeekAndShot(mp4 string) (*replayRec, error) {
	url := fileURLOf(mp4)
	script := `(function(){
		var v = document.getElementById('preview');
		window.__replay = [];
		['loadstart','loadedmetadata','loadeddata','canplay','seeked','playing'].forEach(function(t){
			v.addEventListener(t, function(){ window.__replay.push(t); });
		});
		if (!v.getAttribute('src')) { v.src = ` + jsString(url) + `; }
		var p = v.play(); if (p && p.catch) p.catch(function(){});
		return 'ok';
	})()`
	if _, err := s.wv.EvalJS(script); err != nil {
		return nil, fmt.Errorf("驱动回放失败：%w", err)
	}
	s.settle(14)
	if _, err := s.wv.EvalJS(`(function(){
		var v = document.getElementById('preview');
		v.currentTime = Math.min(1.2, (v.duration || 2) * 0.5);
		return v.currentTime;
	})()`); err != nil {
		return nil, err
	}
	s.settle(14)

	raw, err := s.wv.EvalJS(`(function(){
		var v = document.getElementById('preview');
		var r = v.getBoundingClientRect();
		return JSON.stringify({d: v.duration, rs: v.readyState, w: v.videoWidth, h: v.videoHeight,
			ct: v.currentTime, ev: (window.__replay || []).join(','),
			ns: v.networkState, ec: (v.error ? v.error.code : 0),
			x: Math.round(r.left), y: Math.round(r.top), rw: Math.round(r.width), rh: Math.round(r.height),
			src: v.getAttribute('src') || ''});
	})()`)
	if err != nil {
		return nil, err
	}
	var g struct {
		D   float64 `json:"d"`
		RS  float64 `json:"rs"`
		W   int     `json:"w"`
		H   int     `json:"h"`
		CT  float64 `json:"ct"`
		EV  string  `json:"ev"`
		X   int     `json:"x"`
		Y   int     `json:"y"`
		RW  int     `json:"rw"`
		RH  int     `json:"rh"`
		Src string  `json:"src"`
		NS  float64 `json:"ns"`
		EC  float64 `json:"ec"`
	}
	if err := json.Unmarshal([]byte(unquoteJSON(strings.TrimSpace(raw.ToString()))), &g); err != nil {
		return nil, fmt.Errorf("解析回放状态失败：%w", err)
	}
	info := &replayRec{
		Duration: g.D, ReadyState: g.RS, VideoW: g.W, VideoH: g.H, CurrentTime: g.CT, Events: g.EV,
		PreviewRect: image.Rect(g.X, g.Y, g.X+g.RW, g.Y+g.RH),
	}
	shot := filepath.Join(s.cfg.out, "replay.png")
	if err := os.MkdirAll(s.cfg.out, 0o755); err != nil {
		return nil, err
	}
	if err := s.fullPNG(shot); err != nil {
		return nil, err
	}
	info.Shot = shot
	s.addAsset("replay.png", shot, "回放截图")

	// 量化证据：把「回放画面」与「录制中间帧」各自折成 16×16 网格平均色签名再比较。
	s.mu.Lock()
	tk := s.last
	s.mu.Unlock()
	if tk != nil {
		mid := filepath.Join(tk.dir, fmt.Sprintf(framePattern, maxInt(1, tk.frames/2)))
		info.MidFrame = mid
		a, err1 := gridSig(mid, image.Rectangle{}, false)
		b, err2 := gridSig(shot, info.PreviewRect, true)
		if err1 == nil && err2 == nil {
			info.SigDiff = sigDiff(a, b)
		}
	}
	s.note("回放验证：src=%s duration=%.2fs videoWidth=%d readyState=%.0f networkState=%g errorCode=%g currentTime=%.2f 事件=%s 签名差=%.2f",
		g.Src, info.Duration, info.VideoW, info.ReadyState, g.NS, g.EC, info.CurrentTime, info.Events, info.SigDiff)
	return info, nil
}

// probeMedia 读产物真实容器信息 —— 报告里的独立校验（不采信应用自己的说法）。
// 优先 ffprobe；本机没有 ffprobe 时兜底用 `ffmpeg -i`（不产出、只读容器头，
// 信息打在 stderr 上，退出码非零是正常现象）。
func (s *studio) probeMedia(path string) string {
	if bin, err := exec.LookPath("ffprobe"); err == nil {
		cmd := exec.Command(bin, "-v", "error",
			"-show_entries", "format=duration,size:stream=codec_type,codec_name,width,height,sample_rate,channels",
			"-of", "default=nw=1", filepath.ToSlash(path))
		if b, err := cmd.CombinedOutput(); err == nil && len(strings.TrimSpace(string(b))) > 0 {
			return strings.TrimSpace(string(b))
		}
	}
	b, _ := exec.Command(s.ffmpeg, "-hide_banner", "-i", filepath.ToSlash(path)).CombinedOutput()
	out := strings.TrimSpace(string(b))
	if out == "" {
		return "（本机取不到容器信息）"
	}
	return out
}

// status 是页面 /api/status 的响应体（也用于录制/剪辑后的回填）。
func (s *studio) status() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := map[string]any{
		"ok":       true,
		"state":    s.state,
		"assets":   s.assets,
		"shotUrl":  s.shotURL,
		"videoUrl": s.videoURL,
	}
	if s.last != nil {
		res["duration"] = s.last.seconds
		res["frames"] = s.last.frames
		res["preview"] = fileURLOf(s.last.mp4)
	}
	if s.clipTk != nil {
		res["clipPreview"] = fileURLOf(s.clipTk.mp4)
	}
	return res
}

// ── 小工具 ────────────────────────────────────────────────────────────

// nrgbaFromPremul 把预乘 RGBA 视口像素里 rect 区域转成 NRGBA 图像（PNG 需要非预乘）。
func nrgbaFromPremul(px []byte, vw, vh int, rect image.Rectangle) *image.NRGBA {
	w, h := rect.Dx(), rect.Dy()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		sy := rect.Min.Y + y
		if sy < 0 || sy >= vh {
			continue
		}
		for x := 0; x < w; x++ {
			sx := rect.Min.X + x
			if sx < 0 || sx >= vw {
				continue
			}
			si := (sy*vw + sx) * 4
			if si+3 >= len(px) {
				continue
			}
			o := y*img.Stride + x*4
			a := px[si+3]
			if a == 0 {
				img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = 0, 0, 0, 0
				continue
			}
			for c := 0; c < 3; c++ {
				v := int(px[si+c]) * 255 / int(a)
				if v > 255 {
					v = 255
				}
				img.Pix[o+c] = uint8(v)
			}
			img.Pix[o+3] = a
		}
	}
	return img
}

// evenRect 把矩形夹到视口内并取偶数宽高（yuv420p 要求偶数）。
func evenRect(r image.Rectangle, vw, vh int) image.Rectangle {
	if r.Min.X < 0 {
		r.Min.X = 0
	}
	if r.Min.Y < 0 {
		r.Min.Y = 0
	}
	if r.Max.X > vw {
		r.Max.X = vw
	}
	if r.Max.Y > vh {
		r.Max.Y = vh
	}
	w := r.Dx() / 2 * 2
	h := r.Dy() / 2 * 2
	return image.Rect(r.Min.X, r.Min.Y, r.Min.X+w, r.Min.Y+h)
}

// writeWAV 写 44 字节头的 s16le 交错 WAV。
func writeWAV(path string, pcm []byte, f rendering.AudioFormat) error {
	if !f.Valid() {
		return fmt.Errorf("音频格式非法（%dch @ %dHz）", f.Channels, f.SampleRate)
	}
	byteRate := f.SampleRate * f.Channels * 2
	buf := make([]byte, 0, 44+len(pcm))
	put16 := func(v uint16) { buf = binary.LittleEndian.AppendUint16(buf, v) }
	put32 := func(v uint32) { buf = binary.LittleEndian.AppendUint32(buf, v) }
	buf = append(buf, "RIFF"...)
	put32(uint32(36 + len(pcm)))
	buf = append(buf, "WAVEfmt "...)
	put32(16)
	put16(1)
	put16(uint16(f.Channels))
	put32(uint32(f.SampleRate))
	put32(uint32(byteRate))
	put16(uint16(f.Channels * 2))
	put16(16)
	buf = append(buf, "data"...)
	put32(uint32(len(pcm)))
	buf = append(buf, pcm...)
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, buf, 0o644)
}

// meanAbsDiff 两幅等长像素缓冲的平均绝对差（按字节）。
func meanAbsDiff(a, b []byte) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n == 0 {
		return 0
	}
	var sum float64
	for i := 0; i < n; i++ {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		sum += float64(d)
	}
	return sum / float64(n)
}

// changedPixelRatio 两幅等长 RGBA 缓冲里「变化明显」的像素占比（百分比 0-100）：
// 任一通道差超过 16 即算该像素变了。
//
// 为什么不用平均差：动画位移只影响局部像素（本应用舞台里运动块约占 1%），
// 平均值会被大片静止区域稀释到 0.5 以下、与「完全静止」难以区分；占比则直接
// 回答「画面有多少在动」，判据稳健。
func changedPixelRatio(a, b []byte) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n < 4 {
		return 0
	}
	changed, total := 0, 0
	absD := func(x int) int {
		if x < 0 {
			return -x
		}
		return x
	}
	for i := 0; i+3 < n; i += 4 {
		total++
		if absD(int(a[i])-int(b[i])) > 16 || absD(int(a[i+1])-int(b[i+1])) > 16 ||
			absD(int(a[i+2])-int(b[i+2])) > 16 || absD(int(a[i+3])-int(b[i+3])) > 16 {
			changed++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(changed) * 100 / float64(total)
}

// gridSig 把图像（可选裁切到 crop、可选去掉纯黑边）折成 16×16 网格平均色签名。
func gridSig(path string, crop image.Rectangle, dropBlack bool) ([]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	if crop.Dx() > 0 && crop.Dy() > 0 {
		if cr := crop.Intersect(b); cr.Dx() > 0 && cr.Dy() > 0 {
			b = cr
		}
	}
	if dropBlack {
		minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, bl, _ := img.At(x, y).RGBA()
				if int(r>>8)+int(g>>8)+int(bl>>8) > 36 {
					if x < minX {
						minX = x
					}
					if y < minY {
						minY = y
					}
					if x > maxX {
						maxX = x
					}
					if y > maxY {
						maxY = y
					}
				}
			}
		}
		if maxX > minX && maxY > minY {
			if cr := image.Rect(minX, minY, maxX+1, maxY+1).Intersect(b); cr.Dx() > 0 && cr.Dy() > 0 {
				b = cr
			}
		}
	}
	const n = 16
	sig := make([]float64, 0, n*n*3)
	for gy := 0; gy < n; gy++ {
		y0 := b.Min.Y + b.Dy()*gy/n
		y1 := b.Min.Y + b.Dy()*(gy+1)/n
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for gx := 0; gx < n; gx++ {
			x0 := b.Min.X + b.Dx()*gx/n
			x1 := b.Min.X + b.Dx()*(gx+1)/n
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var sr, sg, sb, cnt float64
			for y := y0; y < y1 && y < b.Max.Y; y++ {
				for x := x0; x < x1 && x < b.Max.X; x++ {
					r, g, bl, _ := img.At(x, y).RGBA()
					sr += float64(r >> 8)
					sg += float64(g >> 8)
					sb += float64(bl >> 8)
					cnt++
				}
			}
			if cnt == 0 {
				cnt = 1
			}
			sig = append(sig, sr/cnt, sg/cnt, sb/cnt)
		}
	}
	return sig, nil
}

// sigDiff 两个签名的平均绝对差（0-255 尺度）。
func sigDiff(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return -1
	}
	var sum float64
	for i := range a {
		d := a[i] - b[i]
		if d < 0 {
			d = -d
		}
		sum += d
	}
	return sum / float64(len(a))
}

// unquoteJSON 去掉 EvalJS 返回值的引号包裹（引擎可能把字符串结果再包一层）。
func unquoteJSON(s string) string {
	if len(s) >= 2 && strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		var out string
		if err := json.Unmarshal([]byte(s), &out); err == nil {
			return out
		}
	}
	return s
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// fileURLOf 把本地路径转成 file:// URL（Windows 盘符路径 → file:///F:/…）。
func fileURLOf(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	sl := filepath.ToSlash(p)
	if !strings.HasPrefix(sl, "/") {
		sl = "/" + sl
	}
	return "file://" + sl
}

// jsString 生成 JS 字符串字面量。
func jsString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString("\\\"")
		case '\\':
			b.WriteString("\\\\")
		case '\n':
			b.WriteString("\\n")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
