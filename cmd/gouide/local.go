// 本地加载（local load）：IDE 前端与插件资产直接从磁盘读取，页面本身不经过
// HTTP 服务器。这是「GUI 版本」的第一半；第二半是请求拦截（见 intercept.go）。
//
// 关键点（与 cmd/companion/web_server.go 的服务端路由一一对齐）：
//
//	index.html 及其相对引用（./assets/*、./favicon.svg）  → <webDir>/*
//	/plugins-assets/<name>/<file>                       → <pluginsDir>/<name>/<file>
//	/api/*                                              → 拦截转发到后端（intercept.go）
//
// WebView 用 LoadHTMLWithBaseURL(html, "app://ide/") 装配：文档获得稳定的来源
// URL，页面里的相对引用因此能被引擎解析成 app://ide/... 后交给 ResourceResolver。
// 引用在引擎里最多问两种形态（原样 + 绝对化），Resolve 两种都接受。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wb-ui/webkit"
)

// docBaseURL 是本地页面的来源 URL。用自定义 scheme 而不是 file://，原因：
//   - 绝对路径引用 /plugins-assets/... 与相对引用 ./assets/... 都会归一化到
//     同一个名字空间，由 Resolve 统一判定，不依赖宿主当前工作目录；
//   - file:// 在 Windows 上还要处理盘符/大小写，容易在「读不到」与「读到
//     别处」之间摇摆。
const docBaseURL = "app://ide/"

// localAssets 承载本地加载所需的两个根目录。
type localAssets struct {
	webDir     string // 前端壳产物（index.html + assets/）
	pluginsDir string // 插件包根（<name>/client.js、theme-*.css、assets/*.js）
}

// Resolve 是 webkit.WebView 的 ResourceResolver：把文档引用解析为磁盘内容。
//
// 接受的引用形态（引擎会按原样与绝对化两种形态各问一次）：
//
//	"./assets/index-xxx.js"、"assets/index-xxx.js"、"app://ide/assets/..." 
//	"/plugins-assets/ui-appearance/theme-midnight.css"、"app://ide/plugins-assets/..."
//
// ok=false 表示「不由本地提供」，引擎继续走自己的策略（本地模式下没有网络，
// 通常就是加载失败并打日志）。
func (a *localAssets) Resolve(ref string) (string, bool) {
	rel, ok := normalizeRef(ref)
	if !ok {
		return "", false
	}
	base := a.webDir
	if strings.HasPrefix(rel, "plugins-assets/") {
		base = a.pluginsDir
		rel = strings.TrimPrefix(rel, "plugins-assets/")
		// 插件包名不能越界：第一段必须是单层目录名。
		if rel == "" || strings.Contains(rel, "..") {
			return "", false
		}
	}
	if rel == "" || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
		return "", false
	}
	full := filepath.Join(base, filepath.FromSlash(rel))
	if !within(base, full) {
		return "", false
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return "", false
	}
	return string(b), true
}

// normalizeRef 把各种引用形态归一化为「相对仓库根」的斜杠路径。
func normalizeRef(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false
	}
	// 只处理无 scheme 或本程序的 app://ide/；http(s)/data: 等交给引擎。
	if i := strings.Index(ref, "://"); i >= 0 {
		if !strings.HasPrefix(ref, "app://ide/") {
			return "", false
		}
		ref = strings.TrimPrefix(ref, "app://ide/")
	}
	if q := strings.IndexAny(ref, "?#"); q >= 0 {
		ref = ref[:q]
	}
	ref = strings.TrimPrefix(ref, "/")
	for strings.HasPrefix(ref, "./") {
		ref = strings.TrimPrefix(ref, "./")
	}
	if ref == "" || strings.Contains(ref, "..") {
		return "", false
	}
	return ref, true
}

// within 兜底确认拼出来的路径仍在根目录内（防 ../ 逃逸）。
func within(base, full string) bool {
	absBase, err1 := filepath.Abs(base)
	absFull, err2 := filepath.Abs(full)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(absBase, absFull)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

// LoadPage 读取本地 index.html 并以 docBaseURL 为来源 URL 装配主文档。
// 调用方必须在此之前注册 API 拦截与 WebSocket 桥（两者都要早于装配：页面
// 脚本一执行就可能 fetch / new WebSocket）。
//
// 装配前把 WebSocket shim 插到 <head> 最前：它先于任何页面脚本执行，从而
// 让 Vue bundle 里的 new WebSocket 拿到桥接实现而不是引擎的 stub。
func (a *localAssets) LoadPage(wv *webkit.WebView) error {
	indexPath := filepath.Join(a.webDir, "index.html")
	b, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("读取本地 index.html 失败（%s）：%w", indexPath, err)
	}
	html := injectHeadScript(string(b), wsShimJS)
	wv.SetResourceResolver(a.Resolve)
	if err := wv.LoadHTMLWithBaseURL(html, docBaseURL); err != nil {
		return fmt.Errorf("本地装配页面失败：%w", err)
	}
	return nil
}

// loadLocal 执行完整的本地装配：定位目录 → 注册 /api 拦截与插件资产拦截 →
// 安装 WebSocket 桥 → 读磁盘 index.html 装配。返回的桥由调用方负责 Shutdown。
//
// 失败时调用方应回退到 hybrid（HTTP 装配）：本地形态是实验分支，不因目录
// 缺失/磁盘产物过期把用户挡在门外。
func loadLocal(wv *webkit.WebView, base, webDirFlag, pluginsFlag string) (*wsBridge, error) {
	webDir, err := discoverWebDir(webDirFlag)
	if err != nil {
		return nil, err
	}
	pluginsDir, err := discoverPluginsDir(pluginsFlag)
	if err != nil {
		return nil, err
	}
	assets := &localAssets{webDir: webDir, pluginsDir: pluginsDir}
	logf("本地加载：web=%s plugins=%s", webDir, pluginsDir)
	// 拦截与 WS 桥必须先于装配注册（bridge 路由与 Go 全局函数都在 LoadHTML
	// 期间被注入解释器；页面脚本一执行就可能 fetch / new WebSocket）。
	installIntercepts(base, assets)
	wsb := newWSBridge(base)
	wsb.Install(wv)
	if err := assets.LoadPage(wv); err != nil {
		wsb.Shutdown()
		return nil, err
	}
	logf("页面已本地装配：%s（拦截 /api 与 WebSocket → %s）", docBaseURL, base)
	return wsb, nil
}

// injectHeadScript 把脚本插到 <head> 之后（大小写不敏感）；没有 <head> 时插到
// 文档最前。script 内容来自本包常量，不需要转义。
func injectHeadScript(html, script string) string {
	tag := "<script>\n" + script + "\n</script>"
	lower := strings.ToLower(html)
	if i := strings.Index(lower, "<head>"); i >= 0 {
		pos := i + len("<head>")
		return html[:pos] + "\n" + tag + html[pos:]
	}
	return tag + "\n" + html
}

// discoverWebDir 按优先级定位前端壳产物目录：
//  1. -webdir 显式指定 / PAIRCODE_WEB_DIR 环境变量
//  2. 程序目录旁 .pair/assets/runtime/web（安装形态；与 companion 一致）
//  3. 程序目录旁 web/
//  4. 从程序目录与当前工作目录向上逐级找 .pair/assets/runtime/web（开发形态）
//  5. 同级 gou-ide 仓库（父目录下的 gou-ide/.pair/assets/runtime/web）
func discoverWebDir(explicit string) (string, error) {
	var cands []string
	if explicit != "" {
		cands = append(cands, explicit)
	}
	if env := strings.TrimSpace(os.Getenv("PAIRCODE_WEB_DIR")); env != "" {
		cands = append(cands, env)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		cands = append(cands,
			filepath.Join(dir, ".pair", "assets", "runtime", "web"),
			filepath.Join(dir, "web"),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		cands = append(cands, walkUpFor(wd, filepath.Join(".pair", "assets", "runtime", "web"))...)
		cands = append(cands, siblingGouide(wd))
	}
	if exe, err := os.Executable(); err == nil {
		cands = append(cands, siblingGouide(filepath.Dir(exe)))
	}
	for _, c := range cands {
		if c == "" {
			continue
		}
		if st, err := os.Stat(filepath.Join(c, "index.html")); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("未找到前端产物目录（需要含 index.html）：请用 -webdir 指定，" +
		"或把 .pair/assets/runtime/web 放到程序同目录")
}

// discoverPluginsDir 定位插件包根目录（<name>/client.js 与 theme-*.css）。
func discoverPluginsDir(explicit string) (string, error) {
	var cands []string
	if explicit != "" {
		cands = append(cands, explicit)
	}
	if env := strings.TrimSpace(os.Getenv("PAIRCODE_PLUGINS_DIR")); env != "" {
		cands = append(cands, env)
	}
	if exe, err := os.Executable(); err == nil {
		cands = append(cands, filepath.Join(filepath.Dir(exe), ".pair", "plugins"))
	}
	if wd, err := os.Getwd(); err == nil {
		cands = append(cands, walkUpFor(wd, filepath.Join(".pair", "plugins"))...)
		cands = append(cands, siblingGouidePlugins(wd))
	}
	if exe, err := os.Executable(); err == nil {
		cands = append(cands, siblingGouidePlugins(filepath.Dir(exe)))
	}
	for _, c := range cands {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("未找到插件目录：请用 -plugins 指定（默认找 .pair/plugins）")
}

// walkUpFor 从 start 逐级向上，返回每级拼上 suffix 后存在的目录（按由近到远）。
func walkUpFor(start, suffix string) []string {
	var out []string
	dir, err := filepath.Abs(start)
	if err != nil {
		return out
	}
	for i := 0; i < 6; i++ {
		out = append(out, filepath.Join(dir, suffix))
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return out
}

// siblingGouide 在 start 及其上级目录里找 gou-ide 仓库的前端产物
// （开发期 wb-ui 与 gou-ide 同级，方便直接跑源码构建出来的 exe）。
func siblingGouide(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for i := 0; i < 4; i++ {
		c := filepath.Join(dir, "gou-ide", ".pair", "assets", "runtime", "web")
		if st, err := os.Stat(filepath.Join(c, "index.html")); err == nil && !st.IsDir() {
			return c
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// siblingGouidePlugins 同 siblingGouide，但定位 .pair/plugins。
func siblingGouidePlugins(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for i := 0; i < 4; i++ {
		c := filepath.Join(dir, "gou-ide", ".pair", "plugins")
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
