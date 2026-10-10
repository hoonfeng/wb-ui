// Command mediastudio —— 「MediaStudio 录屏剪辑台」：一个用 wb-ui 媒体能力搭起来的
// 真实应用，用来端到端验证本仓的媒体链路（而不是停在矩阵/探针口径）。
//
// 应用做什么：
//
//	录屏（抓引擎渲染的舞台区域 → 帧序列）、录音（WASAPI 环回采系统输出）、
//	合成（ffmpeg 出 mp4）、剪辑（按 in/out 剪帧序列 + 音轨）、导出（PNG/JPEG 帧）、
//	回放（把录出来的 mp4 交回引擎 <video> 播放 + seek + 截图比对）。
//
// 为什么它能验证媒体能力：上列每一步都是一条真实链路的端到端使用——
//
//	舞台画面   ← 引擎布局/绘制/CSS 动画/图片解码/宿主抽帧
//	抓帧       ← webkit.WebView.Render()
//	声音       ← app.CaptureDefaultRenderLoopback + app.PlayTestTone
//	合成/剪辑  ← ffmpeg（宿主编码侧）
//	回放       ← app.InstallMediaMetadataResolver 的元数据 + 抽帧通道
//
// 用法：
//
//	go run ./cmd/mediastudio            # 开窗口，交互使用（点「开始录制」）
//	go run ./cmd/mediastudio -run        # 无头自动演示：录制→剪辑→导出→回放→报告
//	go run ./cmd/mediastudio -run -sec 2 -fps 12 -no-audio
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"wb-ui/app"
	"wb-ui/engine/js/jsc"
	"wb-ui/webkit"
)

// options 是应用启动配置。
type options struct {
	run     bool
	out     string
	tmp     string
	samples string
	w       int
	h       int
	sec     float64
	fps     int
	noAudio bool
	clipIn  float64
	clipOut float64
	hold    time.Duration
}

func main() {
	var cfg options
	flag.BoolVar(&cfg.run, "run", false, "无头自动演示：录制 → 剪辑 → 导出帧 → 回放验证 → 报告")
	flag.StringVar(&cfg.out, "out", filepath.Join("dev", "media", "out", "studio"), "产物与报告输出目录")
	flag.StringVar(&cfg.tmp, "tmp", filepath.Join("_temp", "mediastudio"), "中间帧目录（不入库）")
	flag.StringVar(&cfg.samples, "samples", filepath.Join("dev", "media", "samples"), "素材样本目录")
	flag.IntVar(&cfg.w, "w", 1280, "视口宽")
	flag.IntVar(&cfg.h, "h", 860, "视口高")
	flag.Float64Var(&cfg.sec, "sec", 3, "录制时长（秒）")
	flag.IntVar(&cfg.fps, "fps", 10, "录制帧率")
	flag.BoolVar(&cfg.noAudio, "no-audio", false, "不录系统声音（跳过环回采集）")
	flag.Float64Var(&cfg.clipIn, "clip-in", 0.5, "自动模式的剪辑入点（秒）")
	flag.Float64Var(&cfg.clipOut, "clip-out", 2.5, "自动模式的剪辑出点（秒）")
	flag.DurationVar(&cfg.hold, "hold", 0, "结束后保持进程存活（供外部截图/调试）")
	flag.Parse()

	for _, dir := range []string{cfg.out, cfg.tmp} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("mediastudio: 建目录失败 %s: %v", dir, err)
		}
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		log.Fatalf("mediastudio: 未找到 ffmpeg（探测/抽帧/编码都要它）: %v", err)
	}

	// ① UI 库模式：不联网、不导航；外部资源只由宿主 resolver 提供。
	wv := webkit.NewWebViewWithMode(webkit.ModeToolkit)
	defer wv.Destroy()
	wv.Resize(cfg.w, cfg.h)
	console := &jsc.BufferLogger{}
	wv.SetConsoleLogger(console)

	// ② 媒体接线（三条宿主通道）：
	//   - 元数据 + 抽帧（<video> 的 duration/尺寸与当前帧）；
	//   - 动图源（GIF/WebP 多帧）；
	//   - 音频会话（<audio> 的 PCM 输出与时钟）。
	app.InstallMediaMetadataResolver(wv, ffmpeg)
	app.InstallAnimatedImageSource()
	app.InstallMediaAudio(wv, ffmpeg)

	s := newStudio(wv, ffmpeg, cfg)
	// ③ 资源策略：放行宿主 resolver 能解析的引用（舞台图片/视频在本机样本目录里）。
	wv.SetResourcePolicy(webkit.AllowHostResolved)
	wv.SetResourceResolver(s.resolver)
	s.registerRoutes()

	html, err := os.ReadFile(s.indexPath())
	if err != nil {
		log.Fatalf("mediastudio: 读应用页面失败: %v", err)
	}
	if err := wv.LoadHTMLWithBaseURL(string(html), fileURLOf(s.indexPath())); err != nil {
		log.Fatalf("mediastudio: 装配失败: %v", err)
	}

	if cfg.run {
		code := s.runAuto()
		if out := strings.TrimSpace(wv.ConsoleOutput()); out != "" {
			fmt.Println("[mediastudio] 页面控制台：")
			fmt.Println(out)
		}
		if cfg.hold > 0 {
			time.Sleep(cfg.hold)
		}
		os.Exit(code)
	}

	host, err := app.NewHost(wv, cfg.w, cfg.h, "MediaStudio · 录屏剪辑台（wb-ui 媒体验证应用）")
	if err != nil {
		log.Fatalf("mediastudio: 窗口创建失败: %v", err)
	}
	log.Printf("mediastudio: 窗口已开（%dx%d）——点「开始录制」录屏+录音，再导出剪辑", cfg.w, cfg.h)
	host.Run()
}

// ── 无头自动演示 ──────────────────────────────────────────────────────

// runAuto 走一遍完整的应用流程：界面渲染 → 录制 → 剪辑 → 帧导出 → 回放验证 → 报告。
// 录制/剪辑/导出都通过「模拟点击页面按钮」触发，即走真实交互路径
//（命中测试 → DOM 事件 → 页面 onclick → fetch → bridge 路由 → 宿主动作）。
func (s *studio) runAuto() int {
	fail := 0
	s.note("无头自动演示：视口 %dx%d，录制 %.1fs @%dfps，音轨=%v",
		s.cfg.w, s.cfg.h, s.cfg.sec, s.cfg.fps, !s.cfg.noAudio)

	// ① 页面就绪 + 界面截图（界面本身就是引擎渲染能力的证据）
	s.settle(30)
	uiPNG := filepath.Join(s.cfg.out, "ui.png")
	if err := s.fullPNG(uiPNG); err != nil {
		s.note("界面截图失败：%v", err)
		fail++
	} else {
		s.note("① 界面渲染截图：%s", uiPNG)
	}

	// ② 录制
	var tk *take
	if s.click("#btn-record") {
		got := s.waitFor(func() bool {
			s.mu.Lock()
			defer s.mu.Unlock()
			return s.last != nil
		}, 120*time.Second)
		tk = s.snapshot()
		if !got || tk == nil {
			s.note("② 录制失败：未拿到录制结果")
			fail++
		} else {
			s.note("② 录制完成：%d 帧 / %d 字节 / 音轨 %d 字节 / 首末帧差 %.2f",
				tk.frames, tk.mp4Bytes, tk.pcmBytes, tk.frameDiff)
		}
	} else {
		fail++
	}

	// ③ 剪辑
	var clip *take
	if tk != nil {
		s.settle(4)
		if s.click("#btn-export-clip") {
			s.waitFor(func() bool {
				s.mu.Lock()
				defer s.mu.Unlock()
				return s.clipTk != nil
			}, 60*time.Second)
			clip = s.snapshotClip()
		}
		if clip == nil {
			s.note("③ 剪辑失败：未拿到剪辑产物")
			fail++
		} else {
			s.note("③ 剪辑完成：%d 帧 · %.1fs · %d 字节", clip.frames, clip.seconds, clip.mp4Bytes)
		}
	}

	// ④ 帧导出（PNG / JPEG 两条图片编码出口）
	s.settle(3)
	pngShot := filepath.Join(s.cfg.out, "frame-now.png")
	jpgShot := filepath.Join(s.cfg.out, "frame-now.jpg")
	if s.click("#btn-frames") {
		s.waitFor(func() bool { return fileExists(pngShot) }, 20*time.Second)
	}
	if s.click("#btn-frame-jpg") {
		s.waitFor(func() bool { return fileExists(jpgShot) }, 20*time.Second)
	}
	if !fileExists(pngShot) || !fileExists(jpgShot) {
		s.note("④ 帧导出不完整：png=%v jpg=%v", fileExists(pngShot), fileExists(jpgShot))
		fail++
	} else {
		s.note("④ 帧导出：%s / %s", pngShot, jpgShot)
	}

	// ⑤ 回放验证（引擎 <video> + 宿主抽帧通道）
	var rec *replayRec
	if tk != nil {
		s.settle(4)
		s.click("#btn-play")
		s.settle(6)
		r, err := s.replaySeekAndShot(tk.mp4)
		if err != nil {
			s.note("⑤ 回放验证失败：%v", err)
			fail++
		} else {
			rec = r
		}
	}

	// ⑥ 报告
	rep := s.writeReport(tk, clip, rec, uiPNG)
	s.note("⑥ 报告已写出：%s", rep)

	if fail > 0 {
		fmt.Printf("[mediastudio] 自动演示结束：%d 项失败\n", fail)
		return 1
	}
	fmt.Println("[mediastudio] 自动演示全部完成")
	return 0
}

// snapshot 取最近一次录制结果的快照（只读字段，避免跨线程读活对象）。
func (s *studio) snapshot() *take {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == nil {
		return nil
	}
	t := *s.last
	return &t
}

// snapshotClip 取最近一次剪辑结果的快照。
func (s *studio) snapshotClip() *take {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clipTk == nil {
		return nil
	}
	t := *s.clipTk
	return &t
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir() && st.Size() > 0
}

func fileSize(p string) int64 {
	if st, err := os.Stat(p); err == nil {
		return st.Size()
	}
	return 0
}

func ffmpegVersion(bin string) string {
	b, err := exec.Command(bin, "-version").Output()
	if err != nil {
		return "（取不到版本）"
	}
	line := strings.SplitN(string(b), "\n", 2)[0]
	return strings.TrimSpace(line)
}
