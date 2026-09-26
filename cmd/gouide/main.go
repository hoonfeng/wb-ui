// Command gouide 是 PairCode IDE 的桌面端外壳：wb-ui 渲染引擎 + 原生窗口。
//
// 形态：本程序承载原生窗口（wb-ui WebKit + Skia 渲染）；gou-ide 内核后端以
// 子进程方式拉起（pair.exe / companion.exe），监听本机独立端口（默认 9098，
// 绝不占用默认 9090）。双击本程序即可得到完整的 IDE 桌面窗口；关闭窗口时由
// 本程序拉起的后端进程一并退出（-keep-alive 可保留服务）。
//
// 用法：
//
//	gouide.exe                          # 自动发现后端 + 拉起 + 开窗口（双击体验）
//	gouide.exe -port 9098 -width 1600 -height 1000
//	gouide.exe -attach                  # 只连接已在运行的后端（不拉起新进程）
//	gouide.exe -backend D:/x/pair.exe   # 显式指定后端可执行文件
//	gouide.exe -headless -out x.png     # 无窗口自检：加载页面、打印摘要、可存 PNG
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"wb-ui/app"
	"wb-ui/engine/rendering"
	"wb-ui/webkit"
)

// defaultPort 桌面端专用端口。★ 绝不动默认 9090（9090 属 Web 模式与用户
// 正在运行的实例；桌面壳必须错开，测试一律非默认端口）。
const defaultPort = 9098

// logf 全局日志函数，由 main 替换为「文件 + stdout」落盘实现。
var logf = func(format string, a ...any) { fmt.Printf(format, a...); fmt.Println() }

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

func main() {
	port := flag.Int("port", defaultPort, "后端端口（作为 WEB_PORT 传给子进程；-attach 时连接该端口）")
	backend := flag.String("backend", "", "后端可执行文件（pair.exe/companion.exe）；缺省自动发现")
	attach := flag.Bool("attach", false, "只连接已在运行的后端，不拉起新进程")
	width := flag.Int("width", 1600, "窗口宽（CSS px）")
	height := flag.Int("height", 1000, "窗口高（CSS px）")
	title := flag.String("title", "PairCode IDE", "窗口标题")
	headless := flag.Bool("headless", false, "无窗口自检模式：加载页面、打印摘要、退出")
	frames := flag.Int("frames", 60, "headless 模式的渲染/事件循环轮数")
	out := flag.String("out", "", "headless 模式额外写出 PNG 截图（如 dev/output/gouide.png）")
	eval := flag.String("eval", "", "headless 模式额外执行该 JS 并打印结果（诊断用）")
	eval2 := flag.String("eval2", "", "headless 模式：-eval 之后再跑 *frames 轮，然后执行该 JS 并打印（观察事件触发后的异步结果）")
	dumpText := flag.Bool("dump-text", false, "headless 模式 dump 渲染树中的文本对象（诊断文字污染用）")
	keepAlive := flag.Bool("keep-alive", false, "窗口关闭后保留后端服务（默认一并退出）")
	console := flag.Bool("console", false, "打印页面 console 输出（调试用）")
	flag.Parse()

	logf = newLogger()
	base := fmt.Sprintf("http://127.0.0.1:%d", *port)
	logf("PairCode IDE 桌面端启动：端口 %d，视口 %dx%d", *port, *width, *height)

	// 1) 后端编排：已在运行则直接连接；否则拉起子进程并等待就绪。
	var proc *exec.Cmd
	switch {
	case probeReady(base, 1500*time.Millisecond):
		logf("检测到后端已在运行（%s），直接连接", base)
	case *attach:
		logf("后端未就绪，但指定了 -attach：仍尝试连接（页面可能加载失败）")
	default:
		exe, err := discoverBackend(*backend)
		if err != nil {
			logf("后端发现失败：%v", err)
			os.Exit(2)
		}
		logPath := filepath.Join(executableDir(), "logs", "gouide-backend.log")
		proc, err = startBackend(exe, *port, logPath)
		if err != nil {
			logf("后端启动失败（%s）：%v", exe, err)
			os.Exit(2)
		}
		logf("后端已拉起：%s（PID %d，日志 %s）", exe, proc.Process.Pid, logPath)
		if waitReady(base, 120*time.Second) {
			logf("后端就绪：%s", base)
		} else {
			logf("等待后端就绪超时（120s），请查看 %s", logPath)
		}
	}

	// 2) 渲染引擎：WebView 加载前端。
	wv := webkit.NewWebView()
	defer wv.Destroy()
	wv.Resize(*width, *height)
	url := base + "/"
	if err := wv.LoadURL(url); err != nil {
		logf("页面加载失败（%s）：%v", url, err)
	} else {
		logf("页面已加载：%s", url)
	}
	// -eval 在窗口模式下也执行一次（诊断/标记用：如注入可见角标以便
	// 在窗口截图里确认「抓到的是本窗口而非同名浏览器标签」）。
	if *eval != "" {
		if _, err := wv.EvalJS(*eval); err != nil {
			logf("-eval 执行失败：%v", err)
		} else {
			logf("-eval 已执行")
		}
	}

	// 3) 无窗口自检模式（自动化验证用，不创建窗口）。
	if *headless {
		pump(wv, *frames)
		fmt.Println("=== 页面摘要 ===")
		fmt.Println(evalStr(wv, jsSummary))
		if *console {
			fmt.Println("=== console 输出 ===")
			fmt.Println(wv.ConsoleOutput())
		}
		if *eval != "" {
			fmt.Println("=== eval ===")
			fmt.Println(evalStr(wv, *eval))
		}
		if *eval2 != "" {
			// ★ 再推进 *frames 轮事件循环：让 -eval 里触发的事件（dispatchEvent /
			//   fetch 回调 / Vue 渲染）真正落地后再取结果。用于复现「一次交互是否
			//   被处理两次」这类必须跨帧观察的问题（如 toast 双弹）。
			pump(wv, *frames)
			fmt.Println("=== eval2 ===")
			fmt.Println(evalStr(wv, *eval2))
		}
		if *dumpText {
			fmt.Println("=== 渲染树文本对象 ===")
			fmt.Println(dumpRenderTexts(wv))
		}
		if *out != "" {
			if err := writePNG(wv, *out, *width, *height); err != nil {
				logf("PNG 写出失败：%v", err)
			} else {
				logf("PNG 已写出：%s", *out)
			}
		}
		stopBackend(proc)
		logf("自检完成，退出")
		return
	}

	// 4) 原生窗口模式。
	host, err := app.NewHost(wv, *width, *height, *title)
	if err != nil {
		logf("窗口创建失败：%v", err)
		stopBackend(proc)
		os.Exit(1)
	}
	logf("窗口已打开（标题 %q）；关闭窗口即退出", *title)
	// 置顶并显示：双击启动后窗口必须在前台（GLFW 默认只创建窗口，
	// 可能被其它窗口遮挡，用户会以为「没启动」）。
	if win := host.Window(); win != nil {
		win.Focus()
	}
	host.Run()
	logf("窗口已关闭")

	if !*keepAlive {
		stopBackend(proc)
	}
	logf("退出")
}

// pump 驱动事件循环 + 微任务 + 一帧渲染，让 Promise/定时器/图片推进。
func pump(wv *webkit.WebView, rounds int) {
	for i := 0; i < rounds; i++ {
		if el := wv.JSInterpreter().GetEventLoop(); el != nil {
			el.ProcessTasks(0)
		}
		wv.JSInterpreter().RunJobs()
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

// writePNG 把当前渲染结果写成 PNG（headless 自检留证用）。
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

// executableDir 本程序所在目录（日志/相对路径的基准）。
func executableDir() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

// dumpRenderTexts 遍历渲染树，列出所有非空 RenderText 的内容及来源 DOM 节点类型。
// 用于诊断「画面上出现不该有的文字」（如 svg 内注释被画成文字污染图标区域）。
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

// newLogger 返回「stdout + <exeDir>/logs/gouide-desktop.log」双写日志函数。
// 日志超过 2MB 时重建，避免无限增长。
func newLogger() func(string, ...any) {
	logPath := filepath.Join(executableDir(), "logs", "gouide-desktop.log")
	if st, err := os.Stat(logPath); err == nil && st.Size() > 2<<20 {
		_ = os.Remove(logPath)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err == nil {
		if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			return func(format string, a ...any) {
				msg := time.Now().Format("15:04:05.000") + " " + fmt.Sprintf(format, a...)
				fmt.Println(msg)
				_, _ = fmt.Fprintln(f, msg)
			}
		}
	}
	return func(format string, a ...any) {
		fmt.Println(time.Now().Format("15:04:05.000") + " " + fmt.Sprintf(format, a...))
	}
}
