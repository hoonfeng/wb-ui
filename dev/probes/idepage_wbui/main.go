// Command idepage_wbui 是判别实验的「自研引擎侧」探针。
//
// 它加载与 Rust 壳**完全相同**的 URL（http://127.0.0.1:8099/ ，IDE 形态 React 压测页），
// 用**逐字相同**的注入脚本驱动同一套基准，因此两侧数字可直接对比。
//
// 用法（仓库根，CGO 环境）：
//
//	cgo_env.bat run ./dev/probes/idepage_wbui -url http://127.0.0.1:8099/
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"wb-ui/engine/js/jsc"
	"wb-ui/engine/platform/graphics"
	"wb-ui/webkit"
)

// ---------- 与 Rust 侧逐字一致的注入脚本 ----------
const jsFatal = `(window.__bench && window.__bench.fatal) || ''`

// jsStartSync 生成同步基准注入脚本。默认次数（300 重排 / 300 样式重算 /
// 300 滚动 / 24 React 更新）与 Rust 侧逐字一致，保证两侧可比；次数可用 flag
// 缩放 —— 全量基准在自研引擎上需数分钟，定位阶段先用小值快速迭代。
func jsStartSync(nReflow, nStyle, nScroll, nReact int) string {
	return `(function(){try{` +
		`window.__countDom();` +
		`window.__runJs();` +
		`window.__runReflow(` + strconv.Itoa(nReflow) + `);` +
		`window.__runStyleRecalc(` + strconv.Itoa(nStyle) + `);` +
		`window.__runScroll(` + strconv.Itoa(nScroll) + `);` +
		`window.__runReactUpdate(` + strconv.Itoa(nReact) + `);` +
		`return 'started';` +
		`}catch(e){window.__bench.fatal=String((e&&e.stack)||e);return 'err';}})()`
}

const jsReactDone = `(window.__bench.reactUpdate ? 1 : 0)`

const jsFull = `(function(){var b=window.__bench;` +
	`b.domCount=document.getElementsByTagName('*').length;` +
	`b.mem=(window.performance&&performance.memory)?performance.memory.usedJSHeapSize:0;` +
	`b.title=document.title;` +
	`return JSON.stringify(b);})()`

func evalStr(wv *webkit.WebView, script string) (string, error) {
	v, err := wv.EvalJS(script)
	if err != nil {
		return "", err
	}
	return v.ToString(), nil
}

// pumpFrame 驱动一帧：任务队列 → 微任务 → 布局 → 渲染。
// 注意：自研引擎没有自己的合成线程，帧必须由宿主(此循环)驱动 —— 这本身就是
// 与「系统 WebView 自带合成器」的结构性差异，会体现在首屏与更新吞吐上。
func pumpFrame(wv *webkit.WebView) {
	if el := wv.JSInterpreter().GetEventLoop(); el != nil {
		el.ProcessTasks(0)
	}
	wv.JSInterpreter().RunJobs()
	wv.EnsureLayout()
	_, _ = wv.Render()
}

// sampleMem 与 Rust 侧同口径：只统计**本进程树**（自身 + 递归子进程）的工作集，
// 避免把其它应用的 msedgewebview2 算进来；另附 Go 运行时堆统计供细粒度解释。
func sampleMem() map[string]any {
	ps := fmt.Sprintf(
		"$root=%d; $all=Get-CimInstance Win32_Process; "+
			"$ids=New-Object 'System.Collections.Generic.HashSet[int]'; [void]$ids.Add($root); "+
			"$c=$true; while($c){ $c=$false; foreach($p in $all){ "+
			"if($ids.Contains([int]$p.ParentProcessId)){ if($ids.Add([int]$p.ProcessId)){ $c=$true } } } }; "+
			"$s=0; $n=0; $w=0; $wc=0; "+
			"foreach($x in $ids){ $pr=Get-Process -Id $x -ErrorAction SilentlyContinue; "+
			"if($pr){ $s+=$pr.WorkingSet64; $n++; "+
			"if($pr.ProcessName -eq 'msedgewebview2'){ $w+=$pr.WorkingSet64; $wc++ } } }; "+
			"\"$s;$n;$w;$wc\"", os.Getpid())
	treeBytes, treeProcs, wv2Bytes, wv2Procs := 0.0, 0.0, 0.0, 0.0
	if o, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps).Output(); err == nil {
		f := strings.Split(strings.TrimSpace(string(o)), ";")
		get := func(i int) float64 {
			if i < len(f) {
				if v, e := strconv.ParseFloat(strings.TrimSpace(f[i]), 64); e == nil {
					return v
				}
			}
			return 0
		}
		treeBytes, treeProcs, wv2Bytes, wv2Procs = get(0), get(1), get(2), get(3)
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return map[string]any{
		"tree_bytes":       treeBytes,
		"tree_procs":       treeProcs,
		"webview2_bytes":   wv2Bytes,
		"webview2_procs":   wv2Procs,
		"go_heap_alloc":    ms.HeapAlloc,
		"go_heap_sys":      ms.HeapSys,
		"go_total_alloc":   ms.TotalAlloc,
		"go_sys":           ms.Sys,
		"go_num_gc":        ms.NumGC,
		"go_pause_total_ns": ms.PauseTotalNs,
	}
}

func main() {
	url := flag.String("url", "http://127.0.0.1:8099/", "压测页地址（与 Rust 壳同一 URL）")
	out := flag.String("out", "out/wbui-side.json", "JSON 报告输出路径")
	pngOut := flag.String("png", "out/wbui-side.png", "渲染截图输出路径")
	w := flag.Int("w", 1280, "视口宽（与 Rust 窗口一致）")
	h := flag.Int("h", 860, "视口高")
	maxReadySec := flag.Float64("ready-timeout", 60, "首屏等待上限（秒）")
	maxReactSec := flag.Float64("react-timeout", 120, "React 更新基准等待上限（秒）")
	nReflow := flag.Int("reflow", 300, "强制同步重排次数（默认 300，与 Rust 侧保持一致）")
	nStyle := flag.Int("style", 300, "样式重算+重排次数（默认 300）")
	nScroll := flag.Int("scroll", 300, "滚动路径次数（默认 300）")
	nReact := flag.Int("react", 24, "React 更新次数（默认 24）")
	cpuProfile := flag.String("cpuprofile", "", "CPU profile 输出路径（在阶段 1 注入基准前开始采样）")
	flag.Parse()

	// CPU profile 的采样起点设在「阶段 1 注入基准」之前：本探针要优化的是
	// 重排 / 样式重算热路径。若从进程启动就采样，首屏构建（HTML 解析 + 首帧
	// 光栅化）会稀释样本，把热点指向与重排无关的函数（实测：全程采样时
	// runtime.cgocall 占 23.7%，而它来自首屏绘制，不是重排）。
	profileStop := func() {}
	defer func() { profileStop() }()

	t0 := time.Now()

	wv := webkit.NewWebView()
	defer wv.Destroy()
	wv.Resize(*w, *h)
	logger := &jsc.BufferLogger{}
	wv.SetConsoleLogger(logger)

	loadStart := time.Now()
	loadErr := wv.LoadURL(*url)
	loadMs := float64(time.Since(loadStart).Microseconds()) / 1000.0
	if loadErr != nil {
		fmt.Printf("[FAIL] LoadURL(%s): %v\n", *url, loadErr)
		os.Exit(1)
	}
	fmt.Printf("[OK] LoadURL(%s) %.1fms（视口 %dx%d）\n", *url, loadMs, *w, *h)

	// ---- 阶段 0：驱动到页面 __bench.readyAt 出现（= 首屏）----
	readyWallMs, pageReadyAt := 0.0, 0.0
	readyOK := false
	readyDeadline := time.Now().Add(time.Duration(*maxReadySec * float64(time.Second)))
	frames := 0
	for time.Now().Before(readyDeadline) {
		pumpFrame(wv)
		frames++
		// 进度输出：4 万节点页面的首屏驱动可能持续数十秒甚至数分钟，
		// 没有进度就无法区分「慢」与「死」。
		if frames%20 == 0 {
			fmt.Printf("[phase0] frames=%d elapsed=%.1fs\n", frames, time.Since(t0).Seconds())
		}
		if v, err := evalStr(wv, `(window.__bench && window.__bench.readyAt) || 0`); err == nil {
			if n, perr := strconv.ParseFloat(strings.TrimSpace(v), 64); perr == nil && n > 0 {
				pageReadyAt = n
				readyWallMs = float64(time.Since(t0).Microseconds()) / 1000.0
				readyOK = true
				break
			}
		}
	}
	fmt.Printf("[phase0] ready=%v wall=%.1fms pageReadyAt=%.1fms frames=%d\n",
		readyOK, readyWallMs, pageReadyAt, frames)

	if *cpuProfile != "" {
		if pf, perr := os.Create(*cpuProfile); perr == nil {
			if serr := pprof.StartCPUProfile(pf); serr == nil {
				profileStop = func() {
					pprof.StopCPUProfile()
					_ = pf.Close()
					fmt.Printf("[cpuprofile] 已写入 %s\n", *cpuProfile)
				}
			}
		}
	}

	fatal := ""
	if v, err := evalStr(wv, jsFatal); err == nil {
		fatal = strings.TrimSpace(v)
	}

	// ---- 阶段 1：注入同步基准 ----
	startRes := ""
	if readyOK {
		if v, err := evalStr(wv, jsStartSync(*nReflow, *nStyle, *nScroll, *nReact)); err == nil {
			startRes = strings.TrimSpace(v)
		}
		// 让同步基准落地（runJs 是重计算，需要时间）
		for i := 0; i < 5; i++ {
			pumpFrame(wv)
		}
	}
	fmt.Printf("[phase1] jsStart=%q\n", startRes)

	// 强制把 JS 计算阶段收口（同步脚本已执行完，但 React 更新是异步的）
	reactDone := false
	if readyOK && startRes == "started" {
		reactDeadline := time.Now().Add(time.Duration(*maxReactSec * float64(time.Second)))
		for time.Now().Before(reactDeadline) {
			pumpFrame(wv)
			if v, err := evalStr(wv, jsReactDone); err == nil && strings.TrimSpace(v) == "1" {
				reactDone = true
				break
			}
		}
	}
	fmt.Printf("[phase2] reactDone=%v\n", reactDone)

	// ---- 阶段 3：抓完整结果 + 内存 + 截图 ----
	full := ""
	if v, err := evalStr(wv, jsFull); err == nil {
		full = strings.TrimSpace(v)
	}
	totalMs := float64(time.Since(t0).Microseconds()) / 1000.0

	mem := sampleMem()

	// 截图（视觉验证：确认 React 页面真的画出来了）
	pix, rerr := wv.Render()
	if rerr == nil && len(pix) > 0 {
		img := image.NewRGBA(image.Rect(0, 0, *w, *h))
		if len(pix) >= len(img.Pix) {
			copy(img.Pix, pix[:len(img.Pix)])
		}
		if err := os.MkdirAll(filepath.Dir(*pngOut), 0o755); err == nil {
			if f, err := os.Create(*pngOut); err == nil {
				_ = png.Encode(f, img)
				_ = f.Close()
				fmt.Printf("[png] %s\n", *pngOut)
			}
		}
	}

	consoleOut := strings.TrimSpace(wv.ConsoleOutput())
	if len(consoleOut) > 4000 {
		consoleOut = consoleOut[:4000] + "\n...(截断)"
	}

	// 文本宽度缓存统计：判断缓存是否被「反复填满 → 全清重建」
	//（工作集 > globalWidthCacheMax 时，测量反复回落 Skia 光栅器）。
	wcHits, wcMisses, wcSize := graphics.WidthCacheStats()
	report := map[string]any{
		"side":                    "wbui-engine",
		"url":                     *url,
		"viewport":                fmt.Sprintf("%dx%d", *w, *h),
		"loadUrlMs":               loadMs,
		"coldStartToFirstPaintMs": readyWallMs,
		"pageReadyAtMs":           pageReadyAt,
		"readyOK":                 readyOK,
		"driveFrames":             frames,
		"reactDone":               reactDone,
		"totalProbeMs":            totalMs,
		"fatal":                   fatal,
		"console":                 consoleOut,
		"mem":                     mem,
		"widthCache": map[string]any{
			"hits":   wcHits,
			"misses": wcMisses,
			"size":   wcSize,
		},
		"bench":                   json.RawMessage(validJSONOrNull(full)),
	}
	blob, _ := json.MarshalIndent(report, "", "  ")
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err == nil {
		_ = os.WriteFile(*out, blob, 0o644)
	}
	fmt.Printf("\n===== WBUI SIDE REPORT (%s) =====\n%s\n", *out, string(blob))
}

// validJSONOrNull 保证写进报告的 bench 字段是合法 JSON（否则置 null）。
func validJSONOrNull(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || !json.Valid([]byte(s)) {
		return "null"
	}
	return s
}
