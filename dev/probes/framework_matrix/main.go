// Command framework_matrix 在同一个自研引擎上跑「React 19 vs Vue 3.5」的
// 同规模 IDE 形态压测页，产出可比的框架性能矩阵。
//
// 可比性设计：
//
//	· 两页规模同源：tree=320 行、lines=1800 行（Vue 页 dev/fixtures/perf/vue-page.js
//	  逐字复用 React 页的 tokenize/makeCodeLine 与 DOM 结构）
//	· 两页共用同一份 <style>（本工具从 dev/probes/idepage/index.html 现取，
//	  避免样式漂移导致布局成本不可比）
//	· 基准接口同名：__runJs / __runReflow / __runStyleRecalc / __runScroll /
//	  __countDom 完全一致；更新吞吐入口 React 用 __runReactUpdate、Vue 用
//	  __runVueUpdate（Vue 的 flush 是 nextTick 微任务异步，React 侧为同步 flush，
//	  该语义差异在报告中标注）
//	· 加载方式差异：React 页经 HTTP（-react-url，默认 127.0.0.1:8099），Vue 页由
//	  本工具内联装配（Vue 产物 + 页脚本 + 样式，不依赖外部 server）
//
// 用法（wb-ui 仓库根，CGO 环境）：
//
//	go run ./dev/probes/framework_matrix
//	go run ./dev/probes/framework_matrix -update 24 -reflow 300 -out out/framework-matrix.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"time"

	"wb-ui/engine/js/jsc"
	"wb-ui/webkit"
)

// ─── 页面侧脚本注入 ─────────────────────────────────────────────────────

// jsStartSync 触发基准（与 idepage_wbui 的调用序列一致）。
//
// skipJS 跳过 __runJs（纯 JS 计算基准：两轮负载相同、与框架无关，但在引擎上极慢：
// fib(27) + 5400 次正则分词 + 20000 项 localeCompare 排序）。
// stage == "style" 时**只**跑样式重算 —— 用于聚焦 CPU 采样，避免其它阶段稀释样本。
func jsStartSync(cfg stageCfg, isVue bool) string {
	var b strings.Builder
	b.WriteString(`(function(){try{window.__countDom();`)
	if !cfg.skipJS {
		b.WriteString(`window.__runJs();`)
	}
	if cfg.stage == "style" {
		b.WriteString(`window.__runStyleRecalc(` + itoa(cfg.nStyle) + `);`)
	} else {
		upd := "__runReactUpdate"
		if isVue {
			upd = "__runVueUpdate"
		}
		b.WriteString(`window.__runReflow(` + itoa(cfg.nReflow) + `);`)
		b.WriteString(`window.__runStyleRecalc(` + itoa(cfg.nStyle) + `);`)
		b.WriteString(`window.__runScroll(` + itoa(cfg.nScroll) + `);`)
		b.WriteString(`window.` + upd + `(` + itoa(cfg.nUpdate) + `);`)
	}
	b.WriteString(`return 'ok';}catch(e){window.__bench.fatal=String((e&&e.stack)||e);return 'err';}})()`)
	return b.String()
}

const jsReady = `(window.__bench && window.__bench.ready) ? 1 : 0`
const jsUpdateDone = `(window.__bench && (window.__bench.reactUpdate || window.__bench.vueUpdate)) ? 1 : 0`
const jsBench = `JSON.stringify(window.__bench)`
const jsFatal = `(window.__bench && window.__bench.fatal) || ''`

func itoa(n int) string { return fmt.Sprintf("%d", n) }

// ─── WebView 驱动 ───────────────────────────────────────────────────────

func evalStr(wv *webkit.WebView, js string) (string, error) {
	v, err := wv.EvalJS(js)
	if err != nil {
		return "", err
	}
	return v.ToString(), nil
}

func pumpFrame(wv *webkit.WebView) {
	if el := wv.JSInterpreter().GetEventLoop(); el != nil {
		el.ProcessTasks(0)
	}
	wv.JSInterpreter().RunJobs()
	wv.EnsureLayout()
	_, _ = wv.Render()
}

// driveUntil 反复推进帧直到探针返回真值，或超时（返回 false）。
func driveUntil(wv *webkit.WebView, probe, label string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s, err := evalStr(wv, probe); err == nil {
			t := strings.TrimSpace(s)
			if t == "1" || t == "true" {
				return true
			}
		}
		pumpFrame(wv)
	}
	fmt.Printf("[WARN] %s：等待超时（%v）\n", label, timeout)
	return false
}

// ─── 页面装配 ───────────────────────────────────────────────────────────

// extractStyle 从 React 页 HTML 中取出 <style> 段（两页共用同一份样式）。
func extractStyle(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s := string(b)
	i := strings.Index(s, "<style>")
	j := strings.Index(s, "</style>")
	if i < 0 || j <= i {
		return "", fmt.Errorf("%s 未找到 <style> 段", path)
	}
	return s[i+len("<style>") : j], nil
}

// buildVueHTML 内联装配 Vue 压测页（Vue 产物 + 页脚本 + 共用样式）。
func buildVueHTML(css, vueJS, pageJS string) string {
	esc := func(s string) string { return strings.ReplaceAll(s, "</script", "<\\/script") }
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\">")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">")
	b.WriteString("<title>vue-idepage</title><style>")
	b.WriteString(css)
	b.WriteString("</style></head><body><div id=\"root\"></div><script>")
	b.WriteString(esc(vueJS))
	b.WriteString("</script><script>")
	b.WriteString(esc(pageJS))
	b.WriteString("</script></body></html>")
	return b.String()
}

// vueCandidates 返回 vue.global(.prod).js 的候选路径（按优先级）。
func vueCandidates(prod bool) []string {
	name := "vue.global.js"
	if prod {
		name = "vue.global.prod.js"
	}
	tail := []string{"node_modules", "vue", "dist", name}
	rel := append([]string{"..", "gou-ide", "cmd", "companion", "web-ui"}, tail...)
	rel2 := append([]string{"cmd", "companion", "web-ui"}, tail...)
	abs := append([]string{"/f/syproject/gou-ide/cmd/companion/web-ui"}, tail...)
	return []string{
		filepath.Join(rel...),
		filepath.Join(rel2...),
		filepath.Join(abs...),
	}
}

func loadFile(explicit string, cands []string) (string, []byte, error) {
	var tried []string
	if explicit != "" {
		if b, err := os.ReadFile(explicit); err == nil {
			return explicit, b, nil
		}
		tried = append(tried, explicit)
	}
	for _, p := range cands {
		if b, err := os.ReadFile(p); err == nil {
			return p, b, nil
		}
		tried = append(tried, p)
	}
	return "", nil, fmt.Errorf("未找到文件，尝试过：%s", strings.Join(tried, " | "))
}

// ─── 阶段执行 ───────────────────────────────────────────────────────────

type stageCfg struct {
	reactURL     string
	vueHTML      string
	nReflow      int
	nStyle       int
	nScroll      int
	nUpdate      int
	skipJS       bool
	cpuProfile   string
	stage        string
	readyTimeout time.Duration
	updTimeout   time.Duration
	w, h         int
}

type runResult struct {
	Framework string         `json:"framework"`
	LoadMs    float64        `json:"loadMs"`
	ReadyMs   float64        `json:"readyMs"`
	Bench     map[string]any `json:"bench"`
	Console   string         `json:"console,omitempty"`
}

// runStage 在一个新 WebView 中加载页面、跑完全套基准并回读 window.__bench。
func runStage(name string, isVue bool, cfg stageCfg) *runResult {
	wv := webkit.NewWebView()
	defer wv.Destroy()
	wv.Resize(cfg.w, cfg.h)
	wv.SetConsoleLogger(&jsc.BufferLogger{})

	t0 := time.Now()
	if isVue {
		if err := wv.LoadHTMLWithBaseURL(cfg.vueHTML, "http://127.0.0.1/vue-idepage"); err != nil {
			fmt.Printf("[FAIL] %s LoadHTML: %v\n", name, err)
			return nil
		}
	} else {
		if err := wv.LoadURL(cfg.reactURL); err != nil {
			fmt.Printf("[FAIL] %s LoadURL(%s): %v\n", name, cfg.reactURL, err)
			return nil
		}
	}
	loadMs := float64(time.Since(t0).Microseconds()) / 1000
	fmt.Printf("[OK] %s 装载 %.1fms\n", name, loadMs)

	if !driveUntil(wv, jsReady, name+" 首屏就绪", cfg.readyTimeout) {
		if f, _ := evalStr(wv, jsFatal); strings.TrimSpace(f) != "" {
			fmt.Printf("[FAIL] %s 页面 fatal：%s\n", name, f)
		}
		return nil
	}
	readyMs := float64(time.Since(t0).Microseconds()) / 1000
	fmt.Printf("[OK] %s 首屏 %.1fms\n", name, readyMs)

	// 可选：只对基准阶段采 Go 侧 CPU profile
	if cfg.cpuProfile != "" {
		if f, ferr := os.Create(cfg.cpuProfile); ferr == nil {
			if perr := pprof.StartCPUProfile(f); perr == nil {
				fmt.Printf("[profile] 采样 → %s\n", cfg.cpuProfile)
				defer func() {
					pprof.StopCPUProfile()
					_ = f.Close()
					fmt.Printf("[profile] 采样完成 → %s\n", cfg.cpuProfile)
				}()
			}
		} else {
			fmt.Printf("[WARN] profile 文件创建失败：%v\n", ferr)
		}
	}

	if _, err := evalStr(wv, jsStartSync(cfg, isVue)); err != nil {
		fmt.Printf("[FAIL] %s 基准启动：%v\n", name, err)
		return nil
	}
	for i := 0; i < 8; i++ {
		pumpFrame(wv)
	}
	// stage=style 不触发框架更新（__runReactUpdate/__runVueUpdate 未调用），
	// 该条件永不为真 —— 不跳过就会白等满 updTimeout。
	if cfg.stage != "style" {
		driveUntil(wv, jsUpdateDone, name+" 更新吞吐完成", cfg.updTimeout)
	}
	// 收尾推进若干帧，确保统计写回
	for i := 0; i < 3; i++ {
		pumpFrame(wv)
	}

	raw, err := evalStr(wv, jsBench)
	if err != nil {
		fmt.Printf("[FAIL] %s 回读 __bench：%v\n", name, err)
		return nil
	}
	var bench map[string]any
	if err := json.Unmarshal([]byte(raw), &bench); err != nil {
		fmt.Printf("[FAIL] %s __bench JSON 解析：%v\n", name, err)
		return nil
	}
	return &runResult{
		Framework: name,
		LoadMs:    loadMs,
		ReadyMs:   readyMs,
		Bench:     bench,
		Console:   strings.TrimSpace(wv.ConsoleOutput()),
	}
}

// metric 读取 bench 中的数值指标（路径形如 "reflow.perOpUs"）。
func metric(r *runResult, path string) (float64, bool) {
	if r == nil {
		return 0, false
	}
	cur := any(r.Bench)
	for _, seg := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return 0, false
		}
		cur, ok = m[seg]
		if !ok {
			return 0, false
		}
	}
	f, ok := cur.(float64)
	return f, ok
}

func compareMetric(react, vue *runResult, path, unit string) map[string]any {
	rv, rok := metric(react, path)
	vv, vok := metric(vue, path)
	out := map[string]any{"unit": unit, "react": rv, "vue": vv, "reactOk": rok, "vueOk": vok}
	if rok && vok && rv != 0 {
		out["vueOverReact"] = +(vv / rv * 100) / 100
	}
	return out
}

func main() {
	reactURL := flag.String("react-url", "http://127.0.0.1:8099/", "React 压测页地址（需先起静态服务）")
	reactHTML := flag.String("react-html", "dev/probes/idepage/index.html", "React 页 HTML（取其 <style> 供两页共用）")
	vuePath := flag.String("vue", "", "vue.global.js 路径（缺省自动探测）")
	vueProd := flag.Bool("vue-prod", false, "Vue 用 prod 构建（vue.global.prod.js）")
	vuePage := flag.String("vue-page", "dev/fixtures/perf/vue-page.js", "Vue 压测页脚本")
	w := flag.Int("w", 1280, "视口宽（与 Rust 壳/idepage_wbui 一致）")
	h := flag.Int("h", 860, "视口高")
	nReflow := flag.Int("reflow", 20, "强制同步重排次数（每次都是全量 layout，约 0.44s/次，300 次要数分钟）")
	nStyle := flag.Int("style", 20, "样式重算+重排次数")
	nScroll := flag.Int("scroll", 20, "滚动路径次数")
	nUpdate := flag.Int("update", 8, "框架更新批次数")
	skipJS := flag.Bool("skip-js", false, "跳过 __runJs（纯 JS 计算基准：两轮负载相同、与框架无关，但引擎上可能极慢）")
	cpuProfile := flag.String("cpuprofile", "", "CPU profile 输出路径（Go 侧采样，覆盖基准阶段；配 -stage=style 可聚焦）")
	stage := flag.String("stage", "all", "基准阶段：all（全套）| style（仅样式重算，采样聚焦用）")
	readyTimeout := flag.Duration("ready-timeout", 60*time.Second, "首屏等待上限")
	updTimeout := flag.Duration("update-timeout", 120*time.Second, "更新吞吐等待上限")
	only := flag.String("only", "", "只跑一侧：react | vue（缺省两侧都跑）")
	out := flag.String("out", "out/framework-matrix.json", "报告输出路径")
	flag.Parse()

	cfg := stageCfg{
		reactURL: *reactURL, nReflow: *nReflow, nStyle: *nStyle, nScroll: *nScroll,
		nUpdate: *nUpdate, skipJS: *skipJS, cpuProfile: *cpuProfile, stage: *stage,
		readyTimeout: *readyTimeout, updTimeout: *updTimeout, w: *w, h: *h,
	}

	// Vue 页装配材料（样式与 React 页同源）
	css, err := extractStyle(*reactHTML)
	if err != nil {
		fmt.Printf("[FAIL] 取样式失败：%v\n", err)
		os.Exit(1)
	}
	vp, vueJS, err := loadFile(*vuePath, vueCandidates(*vueProd))
	if err != nil {
		fmt.Printf("[FAIL] Vue 产物：%v\n", err)
		os.Exit(1)
	}
	pp, pageJS, err := loadFile(*vuePage, nil)
	if err != nil {
		fmt.Printf("[FAIL] Vue 压测页脚本：%v\n", err)
		os.Exit(1)
	}
	cfg.vueHTML = buildVueHTML(css, string(vueJS), string(pageJS))
	fmt.Printf("[OK] Vue 产物 %s（%.0fKB，%s）｜页脚本 %s（%.0fKB）｜样式取自 %s\n",
		vp, float64(len(vueJS))/1024, map[bool]string{true: "prod", false: "dev"}[*vueProd],
		pp, float64(len(pageJS))/1024, *reactHTML)

	var reactRes, vueRes *runResult
	if *only != "vue" {
		fmt.Println("=== React 19 ===")
		reactRes = runStage("react", false, cfg)
		writeRun(*out, "react", reactRes)
	}
	if *only != "react" {
		fmt.Println("=== Vue 3.5 ===")
		vueRes = runStage("vue", true, cfg)
		writeRun(*out, "vue", vueRes)
	}

	report := map[string]any{
		"generatedAt": time.Now().Format(time.RFC3339),
		"viewport":    map[string]int{"w": *w, "h": *h},
		"scale":       map[string]int{"tree": 320, "lines": 1800},
		"params":      map[string]int{"reflow": *nReflow, "style": *nStyle, "scroll": *nScroll, "update": *nUpdate},
		"notes": []string{
			"两页 DOM 结构与样式同源（tree=320/lines=1800）；Vue 页复用 React 页的 tokenize/makeCodeLine",
			"更新吞吐：React 侧 __runReactUpdate 为同步 flush；Vue 侧 __runVueUpdate 以 nextTick 为批界（微任务异步 flush）",
			"loadMs 不可比：React 经 HTTP 加载 229KB bundle，Vue 由本工具内联装配 572KB(dev)/161KB(prod)，两者加载路径不同，勿横向比较",
			"reactMountCallMs=0 非缺失：React 19 并发模式下 root.render() 立即返回、真正渲染在调度器中完成；Vue 的 mount 是同步渲染，故其 mountCallMs（1316~1740ms）量纲不同",
			"可比指标：domCount / reflow.perOpUs / styleRecalc.perOpUs / scroll.perOpUs / 更新 perBatchMs",
		},
		"react": reactRes,
		"vue":   vueRes,
	}

	if reactRes != nil && vueRes != nil {
		report["comparison"] = map[string]any{
			"domCount":          compareMetric(reactRes, vueRes, "domCount", "elements"),
			"jsTotalMs":         compareMetric(reactRes, vueRes, "js.totalMs", "ms"),
			"reflowPerOpUs":     compareMetric(reactRes, vueRes, "reflow.perOpUs", "us/op"),
			"stylePerOpUs":      compareMetric(reactRes, vueRes, "styleRecalc.perOpUs", "us/op"),
			"scrollPerOpUs":     compareMetric(reactRes, vueRes, "scroll.perOpUs", "us/op"),
			"updatePerBatchMs":  compareMetric(reactRes, vueRes, "reactUpdate.perBatchMs", "ms/batch"),
			"updatePerBatchMsV": compareMetric(reactRes, vueRes, "vueUpdate.perBatchMs", "ms/batch"),
		}
		fmt.Println("=== 对比（React → Vue）===")
		for _, k := range []string{"domCount", "jsTotalMs", "reflowPerOpUs", "stylePerOpUs", "scrollPerOpUs"} {
			if c, ok := report["comparison"].(map[string]any)[k].(map[string]any); ok {
				fmt.Printf("  %-18s react=%-10v vue=%-10v ratio=%v\n", k, c["react"], c["vue"], c["vueOverReact"])
			}
		}
		fmt.Printf("  %-18s react=%-10v（每批 ms）\n", "update(react)", mustMetric(reactRes, "reactUpdate.perBatchMs"))
		fmt.Printf("  %-18s vue=%-10v（每批 ms）\n", "update(vue)", mustMetric(vueRes, "vueUpdate.perBatchMs"))
	}

	if *out != "" {
		if dir := filepath.Dir(*out); dir != "" {
			_ = os.MkdirAll(dir, 0o755)
		}
		b, _ := json.MarshalIndent(report, "", "  ")
		if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
			fmt.Printf("[WARN] 写报告失败：%v\n", err)
		} else {
			fmt.Printf("[OK] 报告已写入 %s\n", *out)
		}
	}

	if reactRes == nil && *only != "vue" {
		os.Exit(2)
	}
	if vueRes == nil && *only != "react" {
		os.Exit(3)
	}
}

func mustMetric(r *runResult, path string) any {
	if v, ok := metric(r, path); ok {
		return v
	}
	return "n/a"
}

// writeRun 把单轮结果立即落盘（<out 去扩展名>-<framework>.json）：一轮失败时
// 另一轮的数据不至于一起丢失。
func writeRun(out, name string, res *runResult) {
	if res == nil {
		return
	}
	base := strings.TrimSuffix(out, filepath.Ext(out))
	p := base + "-" + name + ".json"
	b, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		fmt.Printf("[WARN] 序列化 %s 失败：%v\n", name, err)
		return
	}
	if err := os.WriteFile(p, append(b, '\n'), 0o644); err != nil {
		fmt.Printf("[WARN] 写 %s 失败：%v\n", p, err)
		return
	}
	fmt.Printf("[OK] 单轮结果 %s\n", p)
}
