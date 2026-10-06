// Command psai —— AI-PS 测试宿主：用 wb-ui **直接加载前端编译产物**（不走 HTTP），
// 由 Go 本地拦截器接管资源请求与页面事件，未实现能力先用桩实现（见 stub.go）。
//
// 用法：
//
//	go run ./cmd/psai            # 打开窗口（1440×900）
//	go run ./cmd/psai -verify    # 无头自检：加载产物 → 推进 → 端到端触发一次桩请求 → 出 PNG
//
// 链路（全程无 HTTP 服务、无网络）：
//
//	web/ai-ps/dist/index.html
//	  └─ LoadHTMLWithBaseURL(html, file:///…/dist/index.html)
//	       ├─ <link>/<script src> 相对引用 → 绝对化 file:// → Interceptor.Resolve
//	       │   （ModeToolkit 下 resolver 是唯一的外部资源通道；内容由本进程从产物目录读出）
//	       ├─ 页面 fetch('/api/…') → bridge SDK → bridge 路由 → stub.go 的 Go handler
//	       └─ 页面事件（工具/图层点击、回车提交、按钮 emit）→ POST /api/events → Go 记录 + 桩响应
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
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

func main() {
	dist := flag.String("dist", filepath.Join("web", "ai-ps", "dist"), "前端编译产物目录")
	verify := flag.Bool("verify", false, "无头自检（不开窗口）")
	pngOut := flag.String("png", filepath.Join("_temp", "ai-ps-host.png"), "无头自检截图输出（空串=不写）")
	width := flag.Int("w", 1440, "视口宽")
	height := flag.Int("h", 900, "视口高")
	audit := flag.Bool("audit", false, "布局审计（无头）：量化视口/文档尺寸与各容器几何，列出越界与内容溢出")
	debugPort := flag.Int("remote-debugging-port", 0, "CDP 调试端口（0=关闭；只绑 127.0.0.1）")
	hold := flag.Duration("hold", 0, "自检后保持运行（供外部工具连接调试，如 15s）")
	// 媒体验证工装（文档 §6.2 第 2 条 / §8.2 阶段 0）：与 AI-PS 产物加载无关，
	// 因此放在读取 dist 之前分流。
	media := flag.Bool("media", false, "媒体验证：四配置矩阵 + L0–L4 判定 + Markdown 报告（本机按需，不入 CI 门禁）")
	mediaSamples := flag.String("media-samples", filepath.Join("dev", "media", "samples"), "媒体验证：样本目录（gen_samples.py 的产物）")
	mediaOut := flag.String("media-out", filepath.Join("dev", "media", "out"), "媒体验证：报告/截图输出目录")
	mediaBaseline := flag.String("media-baseline", filepath.Join("dev", "media", "baseline.json"), "媒体验证：基线期望表（等级比对）")
	mediaEdge := flag.Bool("media-edge", false, "媒体验证：启用 Edge 双端对照（无 Edge 时标 SKIP(no-edge)）")
	mediaUpdate := flag.Bool("media-update-baseline", false, "媒体验证：把本次结果写回基线期望表")
	mediaOnly := flag.String("media-only", "", "媒体验证：只跑名字含该子串的样本（调试用）")
	flag.Parse()

	if *media {
		os.Exit(runMediaProbe(mediaOpts{
			Samples:   *mediaSamples,
			Out:       *mediaOut,
			Baseline:  *mediaBaseline,
			Edge:      *mediaEdge,
			Update:    *mediaUpdate,
			Only:      *mediaOnly,
			ViewportW: *width,
		}))
	}

	distAbs, err := filepath.Abs(*dist)
	if err != nil {
		log.Fatalf("psai: 解析产物目录失败: %v", err)
	}
	indexPath := filepath.Join(distAbs, "index.html")
	html, err := os.ReadFile(indexPath)
	if err != nil {
		log.Fatalf("psai: 读取产物失败: %v\n  提示：先在 web/ai-ps 执行 npm install && npm run build", err)
	}

	// ① UI 库模式：不联网、不导航；外部资源只能由宿主 resolver 提供。
	wv := webkit.NewWebViewWithMode(webkit.ModeToolkit)
	defer wv.Destroy()
	wv.Resize(*width, *height)
	console := &jsc.BufferLogger{}
	wv.SetConsoleLogger(console)

	// ②′ 媒体元数据（实现路径主线 A0）：宿主侧用 ffmpeg 探测 <video>/<audio>
	// 的时长与画面尺寸。放在加载产物之前——页面里的媒体元素一旦读属性就会
	// 触发加载流程。本机没有 ffmpeg 时保持「时长未知」（duration=NaN）。
	app.InstallMediaMetadataResolver(wv, "")

	// ②″ 动图（实现路径主线 A4）：宿主用 goskia 的 SkCodec 解 GIF/WebP 多帧，渲染层
	// 按帧时长选帧（无头自检要验证「连续帧差异」，见 mediaframe_selftest.go 的 A4-1）。
	app.InstallAnimatedImageSource()

	// ② 接线：资源拦截器 + 引擎观测 + 请求/事件桩路由。
	ic := newInterceptor(distAbs)
	ic.attach(wv)
	wv.SetResourceResolver(ic.Resolve)          // ← 资源请求拦截点
	wv.SetOnResourceLoaded(ic.OnResourceLoaded) // ← 引擎加载观测
	registerStubs(ic)                           // ← fetch/事件 拦截（bridge 路由）

	// ③ 直接加载产物：文档基准 = 产物 index.html 的 file:// URL（相对引用由此绝对化）。
	base := fileURLOf(indexPath)
	if err := wv.LoadHTMLWithBaseURL(string(html), base); err != nil {
		log.Fatalf("psai: 装配失败: %v", err)
	}
	ic.settle(wv, 14)

	log.Printf("psai: 模式=%s 基准=%s", wv.Mode(), base)
	log.Printf("psai: 产物目录=%s", distAbs)
	ic.Report()

	// ④ CDP 调试服务（实现路径主线 B0/B1）：默认关闭（端口 0 = 不监听、零开销）；
	// 开启时只绑 127.0.0.1，并打印 "DevTools listening on ws://…" 供外部工具发现。
	dt, err := app.StartDevTools(wv, *debugPort)
	if err != nil {
		log.Printf("psai: 启动 CDP 调试服务失败: %v", err)
	}
	if dt != nil {
		defer func() { _ = dt.Close() }()
	}

	if *verify {
		runVerify(wv, ic, *pngOut, console, dt)
		// 外部工具验证用：自检结束后保持进程与服务存活一段时间（curl/netstat/
		// 外部 WS 客户端在此期间连本机 127.0.0.1:<port>）。
		if *hold > 0 && dt != nil {
			log.Printf("psai: 保持运行 %s 供外部工具连接（%s）", *hold, dt.Server.Addr())
			time.Sleep(*hold)
		}
		return
	}
	if *audit {
		runAudit(wv, ic, *pngOut)
		return
	}
	runWindow(wv, ic, *width, *height)
}

// ── 布局审计：自适应缺陷的量化定位 ────────────────────────────────────

// auditBox 是一个元素的几何快照（页面侧收集，Go 侧判定）。
type auditBox struct {
	Sel string `json:"c"`
	L   int    `json:"l"`
	T   int    `json:"t"`
	W   int    `json:"w"`
	H   int    `json:"h"`
	SW  int    `json:"sw"`
	SH  int    `json:"sh"`
}

// auditSnapshot 是一屏的布局全貌。
type auditSnapshot struct {
	VW    int        `json:"vw"`
	VH    int        `json:"vh"`
	DW    int        `json:"dw"`
	DH    int        `json:"dh"`
	BW    int        `json:"bw"`
	BH    int        `json:"bh"`
	Count int        `json:"count"`
	Els   []auditBox `json:"els"`
}

// runAudit 打印一屏的布局审计报告：根级溢出/留白、越界元素、内容超出自身盒的元素，
// 以及关键容器的几何表——用于在多个视口尺寸下对比同一份布局实现。
func runAudit(wv *webkit.WebView, ic *Interceptor, pngPath string) {
	const script = `(function(){
		function num(v){ return (typeof v === 'number' && isFinite(v)) ? Math.round(v) : -1 }
		var out = []
		var els = document.querySelectorAll('[class]')
		for (var i = 0; i < els.length; i++) {
			var el = els[i], cls = String(el.getAttribute('class') || '')
			if (cls.indexOf('d-') < 0 && cls.indexOf('screen-') < 0) continue
			var r = el.getBoundingClientRect()
			out.push({
				c: cls.split(' ')[0],
				l: num(r.left), t: num(r.top), w: num(r.width), h: num(r.height),
				sw: num(el.scrollWidth), sh: num(el.scrollHeight)
			})
		}
		var de = document.documentElement, bd = document.body
		return JSON.stringify({
			vw: num(window.innerWidth), vh: num(window.innerHeight),
			dw: num(de.scrollWidth), dh: num(de.scrollHeight),
			bw: num(bd ? bd.scrollWidth : -1), bh: num(bd ? bd.scrollHeight : -1),
			count: out.length, els: out
		})
	})()`

	v, err := wv.EvalJS(script)
	if err != nil {
		fmt.Printf("布局审计失败（EvalJS）: %v\n", err)
		return
	}
	var snap auditSnapshot
	if err := json.Unmarshal([]byte(v.ToString()), &snap); err != nil {
		fmt.Printf("布局审计：解析页面快照失败: %v\n  原始: %s\n", err, truncate(v.ToString(), 200))
		return
	}

	fmt.Printf("\n=== 布局审计 @ %dx%d ===\n", snap.VW, snap.VH)
	fmt.Printf("文档 scroll = %dx%d   body = %dx%d   采样节点 %d\n", snap.DW, snap.DH, snap.BW, snap.BH, snap.Count)

	// 根级：溢出（内容比视口大 → 出现滚动/裁切）与留白（内容比视口小 → 未铺满）。
	if snap.DH > snap.VH {
		fmt.Printf("★ 根级纵向溢出：文档高 %d > 视口高 %d（多出 %d）\n", snap.DH, snap.VH, snap.DH-snap.VH)
	}
	if snap.DW > snap.VW {
		fmt.Printf("★ 根级横向溢出：文档宽 %d > 视口宽 %d（多出 %d）\n", snap.DW, snap.VW, snap.DW-snap.VW)
	}

	var outside, inner []string
	for _, b := range snap.Els {
		if b.W <= 0 || b.H <= 0 {
			continue
		}
		if b.L < -1 || b.T < -1 || b.L+b.W > snap.VW+1 || b.T+b.H > snap.VH+1 {
			outside = append(outside, fmt.Sprintf("  %-20s l=%-5d t=%-5d w=%-5d h=%-5d → 右=%d 底=%d",
				b.Sel, b.L, b.T, b.W, b.H, b.L+b.W, b.T+b.H))
		}
		if (b.SW > 0 && b.SW > b.W+1) || (b.SH > 0 && b.SH > b.H+1) {
			inner = append(inner, fmt.Sprintf("  %-20s 盒=%dx%d  内容=%dx%d", b.Sel, b.W, b.H, b.SW, b.SH))
		}
	}
	fmt.Printf("越界元素 %d 个（超出视口边界）：\n", len(outside))
	for _, s := range outside {
		fmt.Println(s)
	}
	fmt.Printf("内容超出自身盒 %d 个（存在裁切/内部滚动）：\n", len(inner))
	for _, s := range inner {
		fmt.Println(s)
	}

	// 关键容器几何表：多尺寸对比时最直观的一览。
	// 注意：Sel 取自 class 属性首段（不带点），与这里保持一致。
	keys := []string{"screen-main", "d-main_app", "d-main_titlebar", "d-main_body",
		"d-main_toolstrip", "d-main_center", "d-docbar", "d-canvas-stage", "d-artboard",
		"d-ab-photo", "d-ab-meta", "d-statusbar", "d-main_right", "d-ai-panel",
		"d-ai-actions", "d-ai-results", "d-layers-panel"}
	fmt.Println("关键容器：")
	for _, k := range keys {
		for _, b := range snap.Els {
			if b.Sel == k {
				fmt.Printf("  %-22s l=%-5d t=%-5d w=%-5d h=%-5d\n", b.Sel, b.L, b.T, b.W, b.H)
				break
			}
		}
	}

	if pngPath != "" {
		if err := renderPNG(wv, pngPath); err != nil {
			fmt.Printf("  写 PNG 失败: %v\n", err)
		}
	}
	ic.Report()
}

// runVerify 无头自检：断言产物已装配、验证拦截链路端到端可用，并出 PNG。
func runVerify(wv *webkit.WebView, ic *Interceptor, pngPath string, console *jsc.BufferLogger, dt *app.DevTools) {
	fmt.Println("\n=== 无头自检：产物装配 ===")
	probe(wv, "document.title")
	probe(wv, "document.querySelectorAll('.d-ai-prompt').length")
	probe(wv, "document.querySelectorAll('.d-main_toolstrip > *').length")
	probe(wv, "document.querySelectorAll('.d-layers-list > *').length")
	probe(wv, "document.querySelectorAll('.d-ai-results > *').length")
	probe(wv, "document.querySelector('.d-ai-status') ? document.querySelector('.d-ai-status').textContent : '(missing)'")

	fmt.Println("\n=== 无头自检：模式能力面（Toolkit 应无 XHR/Worker）===")
	for _, p := range []string{"typeof fetch", "typeof XMLHttpRequest", "typeof Worker", "typeof WebSocket"} {
		probe(wv, p)
	}

	fmt.Println("\n=== 无头自检：端到端请求（页面 fetch → bridge → Go 桩 → 回 JS）===")
	if _, err := wv.EvalJS(`(function(){
		window.__probe = { state: 'pending' };
		fetch('/api/ai/generate', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ prompt: '无头自检：赛博朋克城市夜景，霓虹招牌' })
		}).then(function(r){ return r.json() }).then(function(d){
			window.__probe = { state: 'ok', jobId: d.jobId, message: d.message, colors: d.previewColors };
		}).catch(function(e){
			window.__probe = { state: 'error', error: String(e) };
		});
	})()`); err != nil {
		fmt.Printf("  EvalJS 触发请求失败: %v\n", err)
	}
	ic.settle(wv, 18)
	probe(wv, "JSON.stringify(window.__probe)")

	fmt.Println("\n=== 无头自检：端到端事件（页面事件 → Go 拦截器 → 桩响应）===")
	if _, err := wv.EvalJS(`(function(){
		window.__evt = { state: 'pending' };
		fetch('/api/events', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ name: 'verify.probe', payload: { from: 'headless' } })
		}).then(function(r){ return r.json() }).then(function(d){
			window.__evt = { state: 'ok', count: d.count, message: d.message };
		}).catch(function(e){
			window.__evt = { state: 'error', error: String(e) };
		});
	})()`); err != nil {
		fmt.Printf("  EvalJS 触发事件上报失败: %v\n", err)
	}
	ic.settle(wv, 14)
	probe(wv, "JSON.stringify(window.__evt)")

	fmt.Println("\n=== 无头自检：真实交互（模拟点击 → 事件/请求拦截 → 界面回填）===")
	// (a) 点击左侧工具条第 1 个图标：页面 click 委托 → POST /api/events（tool.select）
	clickSelector(wv, ".d-main_toolstrip > *")
	ic.settle(wv, 10)
	// (b) 提示词框输入 + 点「生成图像」：Vue emit → App.onAiGenerate
	//     → postEvent('ai.generate') + fetch('/api/ai/generate') → Go 桩 → 回填状态与缩略图
	setInputValue(wv, ".d-ai-prompt", "赛博朋克城市夜景，霓虹招牌")
	clickSelector(wv, ".d-ai-actions > *")
	ic.settle(wv, 20)
	probe(wv, "document.querySelector('.d-ai-status') ? document.querySelector('.d-ai-status').textContent : '(missing)'")
	probe(wv, "window.__aips ? window.__aips.events.length : -1")
	probe(wv, "window.__aips ? window.__aips.events.map(function(e){return e.name}).join(',') : '(no state)'")
	probe(wv, "window.__aips ? window.__aips.requests.length : -1")
	probe(wv, "document.querySelector('.d-ai-results > *') ? document.querySelector('.d-ai-results > *').style.background : '(missing)'")

	if pngPath != "" {
		if err := renderPNG(wv, pngPath); err != nil {
			fmt.Printf("  写 PNG 失败: %v\n", err)
		}
	}

	mediaSelfCheck(wv, ic)

	// 主线 A1：视频出画面（宿主注入帧流）——判据 A1-1/2/3。
	mediaFrameSelfCheck(wv, ic)

	// 主线 A2 起步：播放时画面随时间推进——判据 A2-1/2/3。
	mediaPlaybackSelfCheck(wv, ic)

	// 主线 A2 续做：精确到帧的 seek 与连续帧采样——判据 A2-5/6
	// （A2-② 的更长预取窗口由 A2-4 的统计与这两个判据共同覆盖）。
	mediaExactSeekSelfCheck(wv, ic)
	mediaContinuousFrameSelfCheck(wv, ic)
	mediaFrameCallbackSelfCheck(wv, ic)

	devtoolsSelfCheck(dt, wv)

	// 主线 A4：动图帧推进（GIF 连续帧差异）。
	// ★ 放在 CDP 判据**之后**：这条判据要连续采样 6 次（约 2 秒），页面状态在等待期间
	// 会推进——先跑它会让 CDP 判据的目标元素（工具条/输入框）不再处于初始状态（判据 5
	// 与键盘项直接变成「跳过」）。自检插桩的规矩：不改变其它判据的前置状态。
	mediaAnimatedImageSelfCheck(wv, ic)

	// 主线 B 的 S3 场景（browser-level 连接 + 扁平会话 + Page.navigate + 设备仿真）。
	// ★ 必须排在最后：判据 14 会把页面导航走（探针页），末尾再导航回原文档——
	// 任何依赖当前页面 DOM/状态的判据都不能排在它后面。
	devtoolsS3SelfCheck(dt, wv)

	ic.Report()
	fmt.Println("\n=== 页面控制台输出 ===")
	fmt.Print(console.String())
	fmt.Println("=== 无头自检结束 ===")
}

// mediaSelfCheck 是主线 A0（媒体元数据注入）的无头验收：宿主用 ffmpeg 探测出
// 时长/尺寸 → 页面里新建的 <video> 应拿到 duration≈1、videoWidth/Height=120x80，
// 且事件按 loadstart→durationchange→loadedmetadata→loadeddata→canplay 派发。
// 本机没有 ffmpeg 时打印「跳过」——宿主缺依赖时引擎保持 duration=NaN（不编造）。
func mediaSelfCheck(wv *webkit.WebView, ic *Interceptor) {
	fmt.Println("\n=== 无头自检：媒体元数据（主线 A0 · L1）===")
	clip, err := ensureProbeClip()
	if err != nil {
		fmt.Printf("  跳过：%v\n", err)
		return
	}
	fmt.Printf("  探测样本：%s\n", clip)
	if _, err := wv.EvalJS(fmt.Sprintf(`(function(){
		window.__media = { events: [] };
		var v = document.createElement('video');
		v.id = 'media-probe';
		['loadstart','durationchange','loadedmetadata','loadeddata','canplay'].forEach(function(t){
			v.addEventListener(t, function(){ window.__media.events.push(t); });
		});
		v.src = %s;
		document.body.appendChild(v);
		window.__media.readyState0 = v.readyState;
	})()`, jsString(fileURLOf(clip)))); err != nil {
		fmt.Printf("  建 <video> 失败: %v\n", err)
		return
	}
	// 媒体事件是宏任务（loadstart 与元数据派发都在任务里），推进事件循环。
	ic.settle(wv, 8)
	probe(wv, "document.getElementById('media-probe').readyState")
	probe(wv, "document.getElementById('media-probe').duration")
	probe(wv, "document.getElementById('media-probe').videoWidth + 'x' + document.getElementById('media-probe').videoHeight")
	probe(wv, "JSON.stringify(window.__media.events)")
}

// ensureProbeClip 准备媒体自检用的样本：1 秒 / 120x80 的 testsrc 视频，由本机
// ffmpeg 生成到 _temp/mediaverify/（产物不入库，与 _temp 里其它证据同规矩）。
// 本机没有 ffmpeg 时返回 error，调用方打印「跳过」。
func ensureProbeClip() (string, error) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("本机没有 ffmpeg：<video>/<audio> 无法探测元数据")
	}
	dir := filepath.Join("_temp", "mediaverify")
	out := filepath.Join(dir, "probe-120x80-1s.mp4")
	// 一律返回绝对路径：调用方把它转成 file:// URL 交给引擎，相对路径会丢盘符。
	abs, err := filepath.Abs(out)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(abs); err == nil && !st.IsDir() {
		return abs, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=120x80:rate=10",
		"-pix_fmt", "yuv420p", abs)
	if outp, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("生成探测样本失败: %v（%s）", err, strings.TrimSpace(string(outp)))
	}
	return abs, nil
}

// runWindow 打开窗口（真实交互：鼠标/键盘事件进入引擎 → 页面 → Go 拦截器）。
func runWindow(wv *webkit.WebView, ic *Interceptor, w, h int) {
	host, err := app.NewHost(wv, w, h, "AI-PS · wb-ui 直载产物（本地桩）")
	if err != nil {
		log.Fatalf("psai: 窗口创建失败: %v", err)
	}
	host.SetClickHandler(ic.OnClick)
	go stdinLoop(ic)
	host.Run()
}

// stdinLoop 终端驱动（自动化/调试）：report / status / quit。
func stdinLoop(ic *Interceptor) {
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		switch strings.TrimSpace(sc.Text()) {
		case "quit", "exit":
			fmt.Println("[psai] quit")
			os.Exit(0)
		case "report":
			ic.Report()
		case "status":
			res, hits, reqs, evts, clicks, loaded := ic.Counts()
			fmt.Printf("[psai] 资源请求=%d 命中=%d 桥请求=%d 事件=%d 点击=%d 引擎加载=%d\n",
				res, hits, reqs, evts, clicks, loaded)
		default:
			fmt.Println("[psai] 命令：report | status | quit")
		}
	}
}

// probe 执行一段 JS 并打印结果（自检输出用）。
func probe(wv *webkit.WebView, script string) {
	v, err := wv.EvalJS(script)
	if err != nil {
		fmt.Printf("  %-58s → ERR %v\n", script, err)
		return
	}
	fmt.Printf("  %-58s → %s\n", script, v.ToString())
}

// renderPNG 渲染当前视口并写 PNG（引擎像素是预乘 RGBA，PNG 需非预乘）。
func renderPNG(wv *webkit.WebView, path string) error {
	pixels, err := wv.Render()
	if err != nil {
		return err
	}
	w, h := wv.Width(), wv.Height()
	if len(pixels) < w*h*4 {
		return fmt.Errorf("像素缓冲 %d 字节 < %dx%d", len(pixels), w, h)
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, pixels[:w*h*4])
	for i := 0; i < len(img.Pix); i += 4 {
		a := img.Pix[i+3]
		if a == 0 || a == 255 {
			continue
		}
		for c := 0; c < 3; c++ {
			v := int(img.Pix[i+c]) * 255 / int(a)
			if v > 255 {
				v = 255
			}
			img.Pix[i+c] = uint8(v)
		}
	}
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
	if err := png.Encode(f, img); err != nil {
		return err
	}
	fmt.Printf("  已写出 %s（%dx%d）\n", path, w, h)
	return nil
}

// fileURLOf 把本地路径转成 file:// URL（Windows 盘符路径 → file:///F:/…）。
func fileURLOf(p string) string {
	// 相对路径先绝对化：否则 "file://" + "/_temp/x.mp4" 产出 file:///_temp/x.mp4
	// （缺盘符）——引擎与宿主都解析不到这个文件。
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	sl := filepath.ToSlash(p)
	if !strings.HasPrefix(sl, "/") {
		sl = "/" + sl
	}
	return "file://" + sl
}

// clickSelector 在匹配元素中心模拟一次「按下 + 释放」：走引擎命中测试 → 页面 DOM 事件
// →（页面侧委托）→ Go 拦截器。这是「事件拦截」在无头模式下的真实路径。
func clickSelector(wv *webkit.WebView, sel string) {
	x, y, ok := elementCenter(wv, sel)
	if !ok {
		fmt.Printf("  点击跳过（未找到元素）: %s\n", sel)
		return
	}
	wv.HandleMouseMove(x, y)
	wv.HandleMouseButton(x, y, 0, 0)
	wv.HandleMouseButton(x, y, 0, 1)
	fmt.Printf("  点击 %s @ (%.1f, %.1f)\n", sel, x, y)
}

// elementCenter 取元素中心坐标（CSS 像素）。
func elementCenter(wv *webkit.WebView, sel string) (float64, float64, bool) {
	script := "(function(){var el=document.querySelector(" + jsString(sel) + ");" +
		"if(!el)return '';var r=el.getBoundingClientRect();" +
		"return (r.left+r.width/2)+','+(r.top+r.height/2);})()"
	v, err := wv.EvalJS(script)
	if err != nil {
		return 0, 0, false
	}
	var x, y float64
	if _, err := fmt.Sscanf(v.ToString(), "%f,%f", &x, &y); err != nil {
		return 0, 0, false
	}
	return x, y, true
}

// setInputValue 给输入框赋值并派发 input 事件（模拟用户输入）。
func setInputValue(wv *webkit.WebView, sel, text string) {
	script := "(function(){var el=document.querySelector(" + jsString(sel) + ");" +
		"if(!el)return 'missing';el.value=" + jsString(text) + ";" +
		"el.dispatchEvent(new Event('input',{bubbles:true}));return el.value;})()"
	v, err := wv.EvalJS(script)
	if err != nil {
		fmt.Printf("  输入失败 %s: %v\n", sel, err)
		return
	}
	fmt.Printf("  输入 %s = %q\n", sel, v.ToString())
}

// jsString 生成 JS 字符串字面量（转义引号、反斜杠与换行）。
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
