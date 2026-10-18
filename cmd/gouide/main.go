// Command gouide 是 PairCode IDE 的「分离模式」桌面壳：wb-ui 渲染引擎 + 原生窗口，
// 后端以内置子进程方式拉起（pair.exe / companion.exe）。
//
// ★ 一体化模式（推荐）：gou-ide 的 pair.exe 在 `-tags gui` 下已内嵌同一套窗口
// 装配（ideapp.Run），后端与窗口同进程、无需外部 exe。本程序保留为「分离壳」，
// 用于：连接已在运行的后端（-attach）、调试 GUI 与后端解耦、或后端不在同目录时。
//
// 用法：
//
//	gouide.exe                          # 自动发现后端 + 拉起 + 开窗口（双击体验）
//	gouide.exe -port 9098 -width 1600 -height 1000
//	gouide.exe -attach                  # 只连接已在运行的后端（不拉起新进程）
//	gouide.exe -backend D:/x/pair.exe   # 显式指定后端可执行文件
//	gouide.exe -mode local -webdir D:/x/web -plugins D:/x/.pair/plugins
//	gouide.exe -headless -out x.png     # 无窗口自检：加载页面、打印摘要、可存 PNG
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"wb-ui/ideapp"
)

// defaultPort 桌面端专用端口。★ 绝不动默认 9090（9090 属 Web 模式与用户
// 正在运行的实例；桌面壳必须错开，测试一律非默认端口）。
const defaultPort = 9098

// logf 全局日志函数，main 中替换为「文件 + stdout」落盘实现。
var logf = func(format string, a ...any) { fmt.Printf(format, a...); fmt.Println() }

func main() {
	port := flag.Int("port", defaultPort, "后端端口（作为 WEB_PORT 传给子进程；-attach 时连接该端口）")
	backend := flag.String("backend", "", "后端可执行文件（pair.exe/companion.exe）；缺省自动发现")
	attach := flag.Bool("attach", false, "只连接已在运行的后端，不拉起新进程")
	mode := flag.String("mode", "hybrid", "页面加载模式：hybrid（HTTP 装配 + WS 桥，默认）/ local（纯本地 + 拦截）/ http（LoadURL，无 WS 桥）")
	local := flag.Bool("local", false, "等价 -mode=local（兼容旧参数）")
	httpMode := flag.Bool("http", false, "等价 -mode=http（兼容旧参数）")
	webDirFlag := flag.String("webdir", "", "前端产物目录（含 index.html）；缺省自动发现")
	pluginsFlag := flag.String("plugins", "", "插件包根目录；缺省自动发现 .pair/plugins")
	width := flag.Int("width", 1600, "窗口宽（CSS px）")
	height := flag.Int("height", 1000, "窗口高（CSS px）")
	title := flag.String("title", "PairCode IDE", "窗口标题")
	headless := flag.Bool("headless", false, "无窗口自检模式：加载页面、打印摘要、退出")
	frames := flag.Int("frames", 60, "headless 模式的渲染/事件循环轮数")
	out := flag.String("out", "", "headless 模式额外写出 PNG 截图（如 dev/output/gouide.png）")
	eval := flag.String("eval", "", "headless 模式额外执行该 JS 并打印结果（诊断用）")
	eval2 := flag.String("eval2", "", "headless 模式：-eval 之后再跑 *frames 轮，然后执行该 JS 并打印")
	dumpText := flag.Bool("dump-text", false, "headless 模式 dump 渲染树中的文本对象（诊断文字污染用）")
	keepAlive := flag.Bool("keep-alive", false, "窗口关闭后保留后端服务（默认一并退出）")
	console := flag.Bool("console", false, "打印页面 console 输出（调试用）")
	flag.Parse()

	logf = newLogger()
	base := fmt.Sprintf("http://127.0.0.1:%d", *port)
	logf("PairCode IDE 桌面端（分离壳）启动：端口 %d，视口 %dx%d", *port, *width, *height)

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

	// 2) 窗口装配交给 ideapp（与一体化 pair.exe 共用同一套代码）。
	loadMode := strings.ToLower(strings.TrimSpace(*mode))
	if *local {
		loadMode = "local"
	}
	if *httpMode {
		loadMode = "http"
	}
	opts := ideapp.Options{
		BaseURL:    base,
		Mode:       loadMode,
		WebDir:     *webDirFlag,
		PluginsDir: *pluginsFlag,
		Width:      *width,
		Height:     *height,
		Title:      *title,
		Headless:   *headless,
		Frames:     *frames,
		Out:        *out,
		Eval:       *eval,
		Eval2:      *eval2,
		DumpText:   *dumpText,
		Console:    *console,
		Logf:       logf,
	}

	if *headless {
		err := ideapp.Headless(opts)
		if err != nil {
			logf("自检失败：%v", err)
		}
		stopBackend(proc)
		logf("自检完成，退出")
		if err != nil {
			os.Exit(1)
		}
		return
	}

	if err := ideapp.Run(opts); err != nil {
		logf("窗口运行失败：%v", err)
		stopBackend(proc)
		os.Exit(1)
	}
	if !*keepAlive {
		stopBackend(proc)
	}
	logf("退出")
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
