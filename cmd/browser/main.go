// wb-ui Browser —— 基于 wb-ui 引擎的最小浏览器验证工具。
//
// 用法：
//
//	go run ./cmd/browser                     # 打开浏览器窗口（内置首页）
//	go run ./cmd/browser <url|file.html>     # 直接加载指定页面
//	go run ./cmd/browser -verify             # 无头能力验证（不依赖窗口）
//
// 窗口内导航：内置首页地址栏输入 URL / 本地文件路径（Enter 或「打开」），
// 前进/后退/刷新按钮通过两层 API 桥（fetch → wb-ui bridge → Go handler）
// 驱动引擎导航，天然验证 bridge 拦截链路的完整性与渲染/事件/JS 能力。
//
// stdin 驱动（自动化/调试）：open <url>、back、fwd、reload、home、quit。
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"wb-ui/app"
	"wb-ui/bridge"
	"wb-ui/webkit"

	_ "embed"
)

//go:embed home.html
var homeHTML string

// browserState 维护导航历史栈与当前位置。
type browserState struct {
	history []string // 每项为已解析的完整 URL（空串=浏览器首页）
	pos     int      // 当前历史位置
	current string   // 当前实际加载的 URL（标题显示用）
}

func (s *browserState) canBack() bool { return s.pos > 0 }
func (s *browserState) canFwd() bool  { return s.pos < len(s.history)-1 }

func (s *browserState) push(url string) {
	// 前进后新导航：丢弃当前位置之后的栈（浏览器语义）。
	if s.pos < len(s.history)-1 {
		s.history = s.history[:s.pos+1]
	}
	s.history = append(s.history, url)
	s.pos = len(s.history) - 1
}

func main() {
	log.SetFlags(log.Ltime)
	args := os.Args[1:]

	// 无头能力验证模式（不创建窗口）。
	for _, a := range args {
		if a == "-verify" || a == "--verify" {
			runVerify()
			return
		}
	}

	wv := webkit.NewWebView()
	st := &browserState{pos: -1}
	st.push("") // 首页为历史原点

	// 注册浏览器控制路由（两层桥：页面 fetch → wb-ui bridge → 此处 handler）。
	registerNavRoutes(wv, st)

	// 初始页面：参数 URL 或浏览器首页。
	initial := ""
	if len(args) > 0 {
		initial = args[0]
	}
	if initial != "" {
		loadTarget(wv, st, initial, true)
	} else if err := wv.LoadHTML(homeHTML); err != nil {
		log.Fatalf("browser: load home failed: %v", err)
	}

	host, err := app.NewHost(wv, 1280, 800, "wb-ui Browser")
	if err != nil {
		log.Fatalf("browser: window failed: %v", err)
	}

	// stdin 驱动（自动化）——与页面内导航（fetch → handler）共用 loadTarget。
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, " ", 2)
			switch parts[0] {
			case "quit", "exit":
				fmt.Println("[browser] quit")
				os.Exit(0)
			case "open":
				if len(parts) == 2 {
					loadTarget(wv, st, strings.TrimSpace(parts[1]), true)
					fmt.Println("[browser] opened:", st.current)
				}
			case "back":
				if st.canBack() {
					st.pos--
					loadTarget(wv, st, st.history[st.pos], false)
					fmt.Println("[browser] back ->", st.current)
				}
			case "fwd":
				if st.canFwd() {
					st.pos++
					loadTarget(wv, st, st.history[st.pos], false)
					fmt.Println("[browser] fwd ->", st.current)
				}
			case "reload":
				loadTarget(wv, st, st.current, false)
				fmt.Println("[browser] reload ->", st.current)
			case "home":
				loadTarget(wv, st, "", false)
				fmt.Println("[browser] home")
			case "status":
				fmt.Printf("[browser] url=%s canBack=%v canFwd=%v\n",
					st.current, st.canBack(), st.canFwd())
			default:
				fmt.Println("[browser] commands: open <url> | back | fwd | reload | home | status | quit")
			}
		}
	}()

	host.Run()
	log.Println("[browser] exited")
}

// loadTarget 解析并加载目标（url 可为 http(s)://、data:、本地文件路径、
// 空串=浏览器首页）。record=true 时把目标推入历史栈。
func loadTarget(wv *webkit.WebView, st *browserState, url string, record bool) {
	url = strings.TrimSpace(url)
	if url == "" {
		url = "about:blank"
	}
	resolved := url
	switch {
	case strings.HasPrefix(url, "http://"), strings.HasPrefix(url, "https://"),
		strings.HasPrefix(url, "data:"), strings.HasPrefix(url, "file://"),
		url == "about:blank":
		// 直接交给引擎（fetchURL 支持 http(s)/data/file；about:blank 特殊处理）。
	default:
		// 本地文件路径（无协议前缀）：先确认存在，避免把拼写错误当 URL。
		if _, err := os.Stat(url); err != nil {
			log.Printf("browser: local file %q not found: %v", url, err)
		}
	}
	if record {
		st.push(resolved)
	}
	st.current = resolved
	loadInto(wv, resolved)
}

func loadInto(wv *webkit.WebView, url string) {
	if url == "about:blank" {
		if err := wv.LoadHTML("<!DOCTYPE html><html><body style='background:#fff'></body></html>"); err != nil {
			log.Printf("browser: blank load failed: %v", err)
		}
		return
	}
	if err := wv.LoadURL(url); err != nil {
		log.Printf("browser: load %q failed: %v", url, err)
		return
	}
	if doc := wv.Page().MainFrame().Document(); doc != nil {
		if title := doc.Title(); title != "" {
			log.Printf("browser: loaded %q title=%q", url, title)
		}
	}
}

// registerNavRoutes 把浏览器控制接口注册到 wb-ui 两层桥：
// 页面 fetch('/_browser/...') 被 wb-ui 原生拦截，直接调到这里。
// handler 收到的是真实 *http.Request（两层桥 dispatchHTTP 装配）。
func registerNavRoutes(wv *webkit.WebView, st *browserState) {
	bridge.RegisterHTTP("GET", "/_browser/status", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"url": st.current, "canBack": st.canBack(), "canFwd": st.canFwd(),
		})
	})
	bridge.RegisterHTTP("POST", "/_browser/nav", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			URL string `json:"url"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		loadTarget(wv, st, body.URL, true)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "url": st.current})
	})
	bridge.RegisterHTTP("POST", "/_browser/back", func(w http.ResponseWriter, r *http.Request) {
		if st.canBack() {
			st.pos--
			loadTarget(wv, st, st.history[st.pos], false)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "url": st.current})
	})
	bridge.RegisterHTTP("POST", "/_browser/fwd", func(w http.ResponseWriter, r *http.Request) {
		if st.canFwd() {
			st.pos++
			loadTarget(wv, st, st.history[st.pos], false)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "url": st.current})
	})
	bridge.RegisterHTTP("POST", "/_browser/reload", func(w http.ResponseWriter, r *http.Request) {
		loadTarget(wv, st, st.current, false)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "url": st.current})
	})
	_ = fmt.Sprintf // keep fmt
}
