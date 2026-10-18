// Package ideapp 承载 PairCode IDE 的桌面窗口装配：wb-ui 渲染引擎 + 原生窗口。
//
// 页面加载有三种模式（-mode）：
//
//	hybrid（默认）  HTTP 装配 + WebSocket 桥：从后端取 index.html，以
//	                http://127.0.0.1:<port>/ 为文档来源装配。origin/host/protocol、
//	                脚本/样式/图片/fetch 全部与浏览器一致（httpmode.go）；引擎的
//	                WebSocket 是 stub，故仍注入 shim 由宿主桥接（wsbridge.go）。
//	local          纯本地装配 + 请求拦截：index.html 与插件资产读磁盘（local.go），
//	                /api/* 由宿主拦截转发（intercept.go）。
//	http           LoadURL 旧形态：整站真实 HTTP，但没有 WS 桥，终端/事件流不可用。
//
// 它同时服务两种进程形态：
//
//   - 一体化（推荐）：gou-ide 的 pair.exe 在 `-tags gui` 下启动后端后，同进程
//     调用 Run——窗口与后端同生命周期，不拉起任何子进程；
//   - 分离（兼容）：wb-ui 的 cmd/gouide 拉起外部 pair.exe 子进程后调用 Run。
package ideapp

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"wb-ui/app"
	"wb-ui/engine/rendering"
	"wb-ui/webkit"
)

// Options 控制一次 GUI 运行。
type Options struct {
	BaseURL    string // 后端基址，如 http://127.0.0.1:9090
	Mode       string // hybrid（默认）/ local / http
	WebDir     string // local 模式的前端产物目录（空=自动发现）
	PluginsDir string // local 模式的插件包根（空=自动发现）

	Width  int
	Height int
	Title  string

	// 自检（headless）选项
	Headless bool
	Frames   int
	Out      string
	Eval     string
	Eval2    string
	DumpText bool
	Console  bool

	// Logf 日志函数；缺省写 stdout。
	Logf func(format string, a ...any)
}

// logf 包级日志函数，Run/Headless 入口按 Options.Logf 覆盖。
var logf = func(format string, a ...any) { fmt.Printf(format+"\n", a...) }

// normalize 补默认值并校验。
func (o *Options) normalize() error {
	o.BaseURL = strings.TrimSuffix(strings.TrimSpace(o.BaseURL), "/")
	if o.BaseURL == "" {
		return fmt.Errorf("ideapp: BaseURL 为空")
	}
	mode := strings.ToLower(strings.TrimSpace(o.Mode))
	if mode == "" {
		mode = "hybrid"
	}
	if mode != "hybrid" && mode != "local" && mode != "http" {
		return fmt.Errorf("ideapp: 未知加载模式 %q（可用：hybrid/local/http）", o.Mode)
	}
	o.Mode = mode
	if o.Width <= 0 {
		o.Width = 1600
	}
	if o.Height <= 0 {
		o.Height = 1000
	}
	if o.Title == "" {
		o.Title = "PairCode IDE"
	}
	if o.Frames <= 0 {
		o.Frames = 60
	}
	if o.Logf != nil {
		logf = o.Logf
	}
	return nil
}

// Run 打开原生窗口并运行事件循环，阻塞直到窗口关闭。
func Run(opts Options) error {
	if err := opts.normalize(); err != nil {
		return err
	}
	logf("PairCode IDE 桌面窗口：%s（模式 %s，视口 %dx%d）", opts.BaseURL, opts.Mode, opts.Width, opts.Height)

	wv := webkit.NewWebView()
	defer wv.Destroy()
	wv.Resize(opts.Width, opts.Height)

	wsb, err := prepare(wv, opts)
	if err != nil {
		return err
	}
	if wsb != nil {
		defer wsb.Shutdown()
	}
	if opts.Eval != "" {
		if _, err := wv.EvalJS(opts.Eval); err != nil {
			logf("-eval 执行失败：%v", err)
		}
	}

	host, err := app.NewHost(wv, opts.Width, opts.Height, opts.Title)
	if err != nil {
		return fmt.Errorf("ideapp: 窗口创建失败：%w", err)
	}
	// ★ 每帧在主线程消费 WS 桥队列（goja 非线程安全，读循环只入队）。
	if wsb != nil {
		host.OnFrame = wsb.Flush
	}
	// 置顶并显示：双击启动后窗口必须在前台。
	if win := host.Window(); win != nil {
		win.Focus()
	}
	logf("窗口已打开（标题 %q）；关闭窗口即退出", opts.Title)
	host.Run()
	logf("窗口已关闭")
	return nil
}

// Headless 无窗口自检：装配页面、推进事件循环、打印摘要（可存 PNG / 执行 JS）。
func Headless(opts Options) error {
	if err := opts.normalize(); err != nil {
		return err
	}
	wv := webkit.NewWebView()
	defer wv.Destroy()
	wv.Resize(opts.Width, opts.Height)

	wsb, err := prepare(wv, opts)
	if err != nil {
		return err
	}
	if wsb != nil {
		defer wsb.Shutdown()
	}
	if opts.Eval != "" {
		if _, err := wv.EvalJS(opts.Eval); err != nil {
			logf("-eval 执行失败：%v", err)
		}
	}

	pump(wv, wsb, opts.Frames)
	fmt.Println("=== 页面摘要 ===")
	fmt.Println(evalStr(wv, jsSummary))
	if opts.Console {
		fmt.Println("=== console 输出 ===")
		fmt.Println(wv.ConsoleOutput())
	}
	if opts.Eval != "" {
		fmt.Println("=== eval ===")
		fmt.Println(evalStr(wv, opts.Eval))
	}
	if opts.Eval2 != "" {
		pump(wv, wsb, opts.Frames)
		fmt.Println("=== eval2 ===")
		fmt.Println(evalStr(wv, opts.Eval2))
	}
	if opts.DumpText {
		fmt.Println("=== 渲染树文本对象 ===")
		fmt.Println(dumpRenderTexts(wv))
	}
	if opts.Out != "" {
		if err := writePNG(wv, opts.Out, opts.Width, opts.Height); err != nil {
			logf("PNG 写出失败：%v", err)
		} else {
			logf("PNG 已写出：%s", opts.Out)
		}
	}
	return nil
}

// prepare 按模式装配页面，返回需要随窗口生命周期的 WebSocket 桥（可能为 nil）。
//
// 调用顺序很重要：WS 桥 / bridge 拦截路由必须在 LoadHTML* 之前安装——页面
// 脚本一执行就可能 new WebSocket 或 fetch。
func prepare(wv *webkit.WebView, opts Options) (*wsBridge, error) {
	switch opts.Mode {
	case "hybrid":
		wsb := newWSBridge(opts.BaseURL)
		wsb.Install(wv)
		if err := loadHTTPPage(wv, opts.BaseURL); err != nil {
			logf("HTTP 装配失败：%v；回退 LoadURL（无 WS 桥）", err)
			if err2 := wv.LoadURL(opts.BaseURL + "/"); err2 != nil {
				wsb.Shutdown()
				return nil, fmt.Errorf("ideapp: 页面加载失败（%s/）：%w", opts.BaseURL, err2)
			}
			wsb.Shutdown()
			return nil, nil
		}
		logf("页面已 HTTP 装配：%s/（子资源走真实 HTTP，WebSocket 由宿主桥接）", opts.BaseURL)
		return wsb, nil

	case "local":
		wsb, err := loadLocal(wv, opts.BaseURL, opts.WebDir, opts.PluginsDir)
		if err == nil {
			logf("页面已本地装配：%s（拦截 /api 与 WebSocket）", docBaseURL)
			return wsb, nil
		}
		logf("本地装配失败：%v；回退 HTTP 装配", err)
		return prepare(wv, Options{
			BaseURL: opts.BaseURL, Mode: "hybrid",
			Width: opts.Width, Height: opts.Height, Title: opts.Title, Logf: opts.Logf,
		})

	case "http":
		if err := wv.LoadURL(opts.BaseURL + "/"); err != nil {
			return nil, fmt.Errorf("ideapp: 页面加载失败（%s/）：%w", opts.BaseURL, err)
		}
		logf("页面已加载：%s/（无 WebSocket 桥）", opts.BaseURL)
		return nil, nil
	}
	return nil, fmt.Errorf("ideapp: 未知加载模式 %q", opts.Mode)
}

// pump 驱动事件循环 + 微任务 + 一帧渲染，让 Promise/定时器/图片推进。
func pump(wv *webkit.WebView, wsb *wsBridge, rounds int) {
	for i := 0; i < rounds; i++ {
		if el := wv.JSInterpreter().GetEventLoop(); el != nil {
			el.ProcessTasks(0)
		}
		wv.JSInterpreter().RunJobs()
		if wsb != nil {
			wsb.Flush()
		}
		wv.EnsureLayout()
		_, _ = wv.Render()
		time.Sleep(5 * time.Millisecond)
	}
}

// evalStr 执行页面 JS 并取字符串结果（失败时返回错误文本，不中断流程）。
func evalStr(wv *webkit.WebView, script string) string {
	v, err := wv.EvalJS(script)
	if err != nil {
		return "<eval error: " + err.Error() + ">"
	}
	return v.ToString()
}

// writePNG 把当前渲染结果写成 PNG。
func writePNG(wv *webkit.WebView, out string, w, h int) error {
	pix, err := wv.Render()
	if err != nil {
		return err
	}
	if len(pix) == 0 {
		return fmt.Errorf("渲染无像素输出")
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if len(pix) >= len(img.Pix) {
		copy(img.Pix, pix[:len(img.Pix)])
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// dumpRenderTexts 遍历渲染树，列出所有非空 RenderText 的内容及来源 DOM 节点类型。
func dumpRenderTexts(wv *webkit.WebView) string {
	rv := wv.RenderView()
	if rv == nil {
		return "(RenderView 为空)"
	}
	var sb strings.Builder
	total, shown := 0, 0
	var walk func(o rendering.RenderObject)
	walk = func(o rendering.RenderObject) {
		if o == nil {
			return
		}
		if rt, ok := o.(*rendering.RenderText); ok {
			if s := strings.TrimSpace(rt.Text()); s != "" {
				total++
				if shown < 80 {
					nodeType, nodeName := "?", "?"
					if n := o.Node(); n != nil {
						nodeType = fmt.Sprintf("%d", n.NodeType())
						nodeName = n.NodeName()
					}
					sb.WriteString(fmt.Sprintf("  [%d] nodeType=%s name=%s %q\n", total, nodeType, nodeName, truncateRunes(s, 60)))
					shown++
				}
			}
		}
		for c := o.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(rv)
	sb.WriteString(fmt.Sprintf("  (共 %d 个非空白 RenderText，显示前 %d 个)\n", total, shown))
	return sb.String()
}

// truncateRunes 按 rune 截断，避免切断多字节字符。
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// jsSummary 页面摘要（headless 自检用）：标题/元素数/关键区域几何/UI 运行时。
// 任意一项失败都不影响其余项（逐项 try/catch）。
const jsSummary = `(function () {
  function safe(fn, dflt) { try { var v = fn(); return (v === undefined || v === null) ? dflt : v; } catch (e) { return 'ERR:' + String(e && e.message || e); } }
  var o = {};
  o.title = safe(function () { return document.title; }, '');
  o.elements = safe(function () { return document.getElementsByTagName('*').length; }, -1);
  o.appChildren = safe(function () { var a = document.getElementById('app'); return a ? a.children.length : -1; }, -1);
  o.geom = {};
  ['#app', '.titlebar', '.activity-bar', '.sidebar', '.main-area'].forEach(function (s) {
    o.geom[s] = safe(function () {
      var e = document.querySelector(s);
      if (!e) return 'missing';
      var b = e.getBoundingClientRect();
      return [Math.round(b.x), Math.round(b.y), Math.round(b.width), Math.round(b.height)].join(',');
    }, 'missing');
  });
  o.runtime = safe(function () {
    var pr = window.__pluginRuntime;
    if (!pr) return 'no-runtime';
    return 'instances=' + pr.instances().length + ' slots=' + pr.clientSlots().length;
  }, '');
  return JSON.stringify(o, null, 2);
})()`
