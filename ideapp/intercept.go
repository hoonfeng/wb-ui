// 请求拦截（request interception）：页面里的 fetch 不经过网络栈，由宿主
// 直接答复。这是「GUI 版本」的第二半（第一半是 local.go 的本地加载）。
//
// 拦截面：
//
//	/api/*              → 转发到本机后端（http://127.0.0.1:<port>），内核逻辑
//	                      与 Web 模式完全同一套，桌面端不重写业务。
//	/plugins-assets/*   → 直接从本地插件目录读文件（client.js / theme-*.css），
//	                      与 web_server.go 的同名路由一一对齐。
//
// 为什么 /api 仍转发到后端而不是进程内直调：gou-ide 的后端是独立可执行文件
// （内核、插件、PTY、模型调用都在里面），桌面壳只负责窗口与渲染。改为进程内
// 直调需要把 webServer 抽成库，属于另一层重构；本轮的「拦截」目标是让页面
// 不依赖 HTTP 服务器提供静态资源、且请求路径统一由宿主答复，转发层对前端
// 完全透明（前端代码零改动）。
package ideapp

import (
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"wb-ui/bridge"
)

// apiProxyTimeout 单次 API 转发的上限。wb-ui 的 fetch 拦截是同步的（bridge
// 在 JS 线程里同步调用 Go handler），请求挂太久会卡住整个渲染线程，因此给一个
// 明确的超时并把失败如实返回给页面（前端本来就有超时/重试逻辑）。
const apiProxyTimeout = 120 * time.Second

// installIntercepts 注册全部拦截路由。必须在 WebView 装配（LoadHTML*）之前
// 调用：bridge 路由表在装配时被注入 JS 解释器。
func installIntercepts(backendBase string, assets *localAssets) {
	installAPIRoutes(backendBase)
	installPluginAssetRoutes(assets)
}

// installAPIRoutes 把 /api/* 转发到后端。
//
// 说明：bridge.RegisterHTTP 的空 method 表示「任意方法」。前缀通配 "/api/*"
// 覆盖 /api/ui-boot、/api/plugins、/api/system/info 等全部 REST 接口。
func installAPIRoutes(backendBase string) {
	base := strings.TrimSuffix(backendBase, "/")
	client := &http.Client{Timeout: apiProxyTimeout}
	bridge.RegisterHTTP("", "/api/*", func(w http.ResponseWriter, r *http.Request) {
		// 只带 path + query：bridge 已经把 origin 剥掉，这里补回后端地址。
		target := base + r.URL.Path
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		var body io.Reader
		if r.Body != nil {
			body = r.Body
		}
		req, err := http.NewRequest(r.Method, target, body)
		if err != nil {
			http.Error(w, "gouide: 构造后端请求失败: "+err.Error(), http.StatusBadGateway)
			return
		}
		// 透传页面带来的头（Content-Type 决定后端如何解析 body）。
		for k, vs := range r.Header {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		if req.Header.Get("Origin") == "" {
			req.Header.Set("Origin", base)
		}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[intercept] %s %s 转发失败：%v", r.Method, target, err)
			http.Error(w, "gouide: 后端不可达: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		// bridge 的虚拟 ResponseWriter 会丢响应头，但会保留状态码与 body：
		// 前端拿到的 Response 因此有正确 status，body 原样返回。
		w.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(w, resp.Body); err != nil {
			log.Printf("[intercept] %s %s 读取响应失败：%v", r.Method, target, err)
		}
	})
}

// installPluginAssetRoutes 让页面 fetch('/plugins-assets/<name>/<file>') 直接
// 命中本地文件（插件 client 半源码、主题样式等）。
func installPluginAssetRoutes(assets *localAssets) {
	root := assets.pluginsDir
	bridge.RegisterHTTP("", "/plugins-assets/*", func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, "/plugins-assets/")
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" || strings.Contains(rel, "..") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		parts := strings.SplitN(rel, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		full := filepath.Join(root, parts[0], filepath.FromSlash(parts[1]))
		if !within(filepath.Join(root, parts[0]), full) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		b, err := os.ReadFile(full)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", contentTypeOf(full))
		_, _ = w.Write(b)
	})
}

// contentTypeOf 给插件资产一个最小 MIME 映射（bridge 不转发头，但保留这段
// 逻辑给未来直传头用；同时便于排查）。
func contentTypeOf(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".js", ".mjs":
		return "application/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".html":
		return "text/html; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".woff2":
		return "font/woff2"
	}
	return "application/octet-stream"
}
