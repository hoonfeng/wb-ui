// AI-PS 宿主的「Go 本地拦截器」。
//
// 它是页面与宿主之间唯一的数据通道，接管两类输入：
//  1. 资源请求：<link>/<script src>/@import/img 等外部引用 → webkit.ResourceResolver
//     （ModeToolkit 下这是**唯一**的外部资源通道：引擎不会走网络/文件系统，
//     一切内容都由本拦截器从编译产物目录读出并留痕）
//  2. 页面请求与事件：fetch('/api/...') → bridge 路由 → Go 桩 handler（见 stub.go）
//     事件（工具选择/图层选择/回车提交/按钮 emit）经 POST /api/events 上报到此。
//
// 本项目是「先跑通链路」的测试工程：所有能力都是桩实现，但每一次交互都真实经过
// Go 拦截器，记录可直接用于核对链路完整性。
package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"wb-ui/engine/dom"
	"wb-ui/webkit"
)

// ResourceHit 一次资源请求的拦截记录。
type ResourceHit struct {
	At      string `json:"at"`
	Ref     string `json:"ref"`     // 页面写下的引用（原样 / 绝对化后）
	Path    string `json:"path"`    // 实际读取的产物文件路径（命中时）
	Bytes   int    `json:"bytes"`   // 提供的内容大小
	Hit     bool   `json:"hit"`     // 是否由本拦截器提供（false = 未命中，交回引擎）
	ErrText string `json:"errText"` // 未命中原因
}

// RequestHit 一次桥请求（页面 fetch → Go 桩 handler）。
type RequestHit struct {
	At     string `json:"at"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   string `json:"body"`
}

// EventHit 一次页面事件（POST /api/events 上报）。
type EventHit struct {
	At      string `json:"at"`
	Name    string `json:"name"`
	Payload string `json:"payload"`
}

// ClickHit 一次宿主层点击（app.Host 的 ClickHandler：非 js: 的 onclick 命中）。
type ClickHit struct {
	At      string  `json:"at"`
	Onclick string  `json:"onclick"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
}

// Interceptor 拦截器状态。
type Interceptor struct {
	distDir string // 编译产物根目录（dist）

	mu        sync.Mutex
	resources []ResourceHit
	requests  []RequestHit
	events    []EventHit
	clicks    []ClickHit
	loaded    []string // SetOnResourceLoaded 观测到的资源加载

	wv *webkit.WebView
}

func newInterceptor(distDir string) *Interceptor {
	return &Interceptor{distDir: distDir}
}

func (ic *Interceptor) attach(wv *webkit.WebView) { ic.wv = wv }

func nowStamp() string { return time.Now().Format("15:04:05.000") }

// Resolve 实现 webkit.ResourceResolver：把外部引用映射为编译产物里的文件内容。
//
// 支持的引用形态（先问原样引用、再问绝对 URL，见 webkit/mode.go）：
//   - 相对路径："./assets/index-xxx.js"、"assets/style.css"
//   - 逻辑命名空间："app://ai-ps/assets/index-xxx.js"
//   - 绝对 file URL："file:///F:/.../web/ai-ps/dist/assets/index-xxx.js"
//
// 只提供产物目录**之内**的文件（防目录穿越）；目录外一律不接管（ok=false）。
func (ic *Interceptor) Resolve(ref string) (string, bool) {
	if strings.TrimSpace(ref) == "" {
		return "", false
	}
	rel, ok := ic.relPath(ref)
	if !ok {
		ic.recordResource(ref, "", 0, false, "引用不在产物目录内，未接管")
		return "", false
	}
	abs := filepath.Join(ic.distDir, filepath.FromSlash(rel))
	data, err := os.ReadFile(abs)
	if err != nil {
		ic.recordResource(ref, abs, 0, false, fmt.Sprintf("读取失败：%v", err))
		return "", false
	}
	ic.recordResource(ref, abs, len(data), true, "")
	return string(data), true
}

// relPath 把引用归一化为「相对产物目录」的斜杠路径。
func (ic *Interceptor) relPath(ref string) (string, bool) {
	s := strings.TrimSpace(ref)
	// 去掉 query / fragment（资源缓存按完整 URL 索引，这里只关心文件）
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	switch {
	case strings.HasPrefix(s, "app://"):
		rest := strings.TrimPrefix(s, "app://")
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			return cleanRel(rest[i+1:]), true
		}
		return "", false
	case strings.HasPrefix(s, "file://"):
		p := fileURLToPath(s)
		if p == "" {
			return "", false
		}
		rel, err := filepath.Rel(ic.distDir, p)
		if err != nil || strings.HasPrefix(rel, "..") {
			return "", false
		}
		return cleanRel(filepath.ToSlash(rel)), true
	case strings.HasPrefix(s, "http://"), strings.HasPrefix(s, "https://"), strings.HasPrefix(s, "data:"):
		return "", false // 产物自包含，不该出现；出现即不接管（Toolkit 模式会拒绝）
	default:
		// 相对引用（原样引用路径）："./assets/x.js" / "assets/x.js" / "/assets/x.js"
		s = strings.TrimPrefix(s, "./")
		s = strings.TrimPrefix(s, "/")
		return cleanRel(s), true
	}
}

func cleanRel(s string) string {
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(s)))
	return strings.TrimPrefix(cleaned, "./")
}

// fileURLToPath 把 file:// URL 转成本地路径（与 webkit 内部 fileURLPath 同口径）。
func fileURLToPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	p := u.Path
	if u.Host != "" && u.Host != "localhost" {
		p = "//" + u.Host + p
	}
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:] // /F:/... → F:/...
	}
	return filepath.FromSlash(p)
}

func (ic *Interceptor) recordResource(ref, path string, n int, hit bool, errText string) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.resources = append(ic.resources, ResourceHit{
		At: nowStamp(), Ref: ref, Path: path, Bytes: n, Hit: hit, ErrText: errText,
	})
}

// OnResourceLoaded 观测引擎的资源加载完成事件（与 Resolve 交叉印证）。
func (ic *Interceptor) OnResourceLoaded(url string) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.loaded = append(ic.loaded, url)
}

// OnClick 宿主层点击回调（app.Host.SetClickHandler）：无 JS 处理器的 onclick 命中。
func (ic *Interceptor) OnClick(el *dom.Element, onclick string, x, y float64) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.clicks = append(ic.clicks, ClickHit{At: nowStamp(), Onclick: onclick, X: x, Y: y})
	fmt.Printf("[intercept][click] onclick=%q at (%.1f, %.1f)\n", onclick, x, y)
}

func (ic *Interceptor) recordRequest(method, path, body string) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.requests = append(ic.requests, RequestHit{At: nowStamp(), Method: method, Path: path, Body: body})
	fmt.Printf("[intercept][req] %s %s body=%s\n", method, path, truncate(body, 120))
}

func (ic *Interceptor) recordEvent(name, payload string) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	ic.events = append(ic.events, EventHit{At: nowStamp(), Name: name, Payload: payload})
	fmt.Printf("[intercept][evt] %s %s\n", name, truncate(payload, 120))
}

func (ic *Interceptor) recordEventRaw(raw string) {
	ic.recordEvent("(raw)", raw)
}

// Counts 统计（供 /api/status 与终端 report 使用）。
func (ic *Interceptor) Counts() (resources, hits, requests, events, clicks, loaded int) {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	resources = len(ic.resources)
	for _, r := range ic.resources {
		if r.Hit {
			hits++
		}
	}
	return resources, hits, len(ic.requests), len(ic.events), len(ic.clicks), len(ic.loaded)
}

// Report 打印拦截摘要（终端可视化链路）。
func (ic *Interceptor) Report() {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	fmt.Println("──────────────── Go 本地拦截器记录 ────────────────")
	fmt.Printf("资源请求 %d 次（命中产物 %d 次）\n", len(ic.resources), countHits(ic.resources))
	for _, r := range ic.resources {
		status := "HIT "
		if !r.Hit {
			status = "MISS"
		}
		rel := r.Path
		if rel == "" {
			rel = "-"
		} else if rp, err := filepath.Rel(ic.distDir, r.Path); err == nil {
			rel = filepath.ToSlash(rp)
		}
		fmt.Printf("  [%s] %-4s %-42s → %-34s %6d B %s\n", r.At, status, truncate(r.Ref, 42), truncate(rel, 34), r.Bytes, r.ErrText)
	}
	fmt.Printf("引擎资源加载观测 %d 次\n", len(ic.loaded))
	for _, u := range ic.loaded {
		fmt.Printf("  [loaded] %s\n", u)
	}
	fmt.Printf("桥请求 %d 次\n", len(ic.requests))
	for _, r := range ic.requests {
		fmt.Printf("  [%s] %-4s %-28s %s\n", r.At, r.Method, r.Path, truncate(r.Body, 60))
	}
	fmt.Printf("页面事件 %d 次\n", len(ic.events))
	for _, e := range ic.events {
		fmt.Printf("  [%s] %-22s %s\n", e.At, e.Name, truncate(e.Payload, 60))
	}
	fmt.Printf("宿主点击 %d 次\n", len(ic.clicks))
	for _, c := range ic.clicks {
		fmt.Printf("  [%s] onclick=%q (%.1f, %.1f)\n", c.At, c.Onclick, c.X, c.Y)
	}
	fmt.Println("──────────────────────────────────────────────────")
}

func countHits(rs []ResourceHit) int {
	n := 0
	for _, r := range rs {
		if r.Hit {
			n++
		}
	}
	return n
}

// settle 推进引擎事件循环：让 <script src> 的异步加载/执行与 Promise 回调跑完
// （与 webkit 测试里 EnsureLayout+Render 循环同款；额外的短睡让异步 goroutine 有机会落地）。
func (ic *Interceptor) settle(wv *webkit.WebView, rounds int) {
	for i := 0; i < rounds; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			return
		}
		time.Sleep(3 * time.Millisecond)
	}
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
