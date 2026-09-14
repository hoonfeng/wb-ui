// 运行模式（Mode）：wb-ui 既可以作为「嵌入浏览器」使用，也可以作为
// 「UI 库」使用。两种形态共享同一套渲染/布局/DOM/CSS/事件管线与壳内
// 交互，差别只在**装配阶段接入了哪些浏览器专属能力**（外部网络、子
// 框架、并发脚本、导航、外部资源）。详见 docs/MODES.md。
//
// 设计约束：
//   - ModeBrowser 是默认值，行为与历史实现逐字节一致（回归安全）。
//   - 模式在装配（LoadHTML）时锁定：装配要按模式决定 5 处注入
//     （fetch 版本、XHR、浏览器全局、子框架、外部资源通道），中途切换
//     会留下「页面脚本已 feature-detect 过旧能力」的不一致状态，因此
//     SetMode 在装配后只允许同值调用（否则 ErrModeLocked）。
//   - 多 WebView 各持自己的模式（引擎既有的 per-WebView 分派架构），
//     配置面板可以是 Browser 模式、纯 Go 构建的界面可以是 Toolkit 模式。

package webkit

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"wb-ui/dom"
)

// Mode 是 WebView 的运行模式。
type Mode int

const (
	// ModeBrowser：嵌入浏览器（默认）。完整浏览器语义——外部资源
	// （http(s)/file/data）加载、iframe 子文档、XMLHttpRequest、Worker、
	// WebSocket、LoadURL 导航、fetch 的真实网络回退。
	ModeBrowser Mode = iota

	// ModeToolkit：UI 库。保留渲染/布局/DOM/CSS/事件与宿主桥（bridge
	// 路由，UI 用它取数据），裁剪掉浏览器专属的「外部输入」：
	//   - fetch 只命中宿主注册的桥路由；无匹配路由时 reject（不发请求）
	//   - 不暴露 XMLHttpRequest / Worker / WebSocket
	//   - 不加载 <iframe> 子文档（元素仍参与布局/绘制）
	//   - LoadURL（导航）返回 ErrModeNotSupported
	//   - <link>/<script src>/@import 只允许 data: URL 或宿主
	//     ResourceResolver 提供的资源（SetResourceResolver）
	ModeToolkit
)

// String 返回模式的稳定名称（"browser"/"toolkit"），用于日志与文档。
func (m Mode) String() string {
	switch m {
	case ModeBrowser:
		return "browser"
	case ModeToolkit:
		return "toolkit"
	}
	return fmt.Sprintf("Mode(%d)", int(m))
}

// allowsNetwork 报告该模式是否允许 fetch 在未命中桥路由时发起真实
// 网络/文件请求。
func (m Mode) allowsNetwork() bool { return m != ModeToolkit }

// allowsSubframes 报告该模式是否装配 <iframe> 子文档。
func (m Mode) allowsSubframes() bool { return m != ModeToolkit }

// allowsExternalURLs 报告该模式是否允许 http(s)/file 外部资源引用。
func (m Mode) allowsExternalURLs() bool { return m != ModeToolkit }

// allowsNavigation 报告该模式是否允许导航（LoadURL / 顶层文档换源）。
func (m Mode) allowsNavigation() bool { return m != ModeToolkit }

// hidesThreadGlobals 报告该模式是否隐藏浏览器并发/长连接全局
// （Worker/WebSocket）。
func (m Mode) hidesThreadGlobals() bool { return m == ModeToolkit }

// ResourceResolver 把外部资源引用（<link href>、<script src>、@import 等）
// 解析为内容。它是 UI 库模式下唯一的外部资源通道：宿主把资源以内存/
// 内嵌形式（基础方式）提供，替代网络或文件系统（web 方式）。
//
// ok=false 表示「该引用不由我提供」，引擎将继续按模式策略处理（UI 库
// 模式下即拒绝加载）。
type ResourceResolver func(ref string) (content string, ok bool)

var (
	// ErrModeLocked 表示 WebView 已按当前模式装配（LoadHTML 之后），
	// 不能再切换到另一个模式。宿主如需另一模式，请新建 WebView。
	ErrModeLocked = errors.New("webkit: mode is locked after LoadHTML")

	// ErrModeNotSupported 表示该操作在当前模式下不可用（例如 UI 库
	// 模式下的 LoadURL 导航）。
	ErrModeNotSupported = errors.New("webkit: not supported in this mode")

	// ErrExternalResourceBlocked 表示外部资源引用被当前模式拒绝
	// （UI 库模式且宿主未提供 ResourceResolver）。
	ErrExternalResourceBlocked = errors.New("webkit: external resource blocked in this mode")
)

// Mode 返回当前运行模式（默认 ModeBrowser）。
func (wv *WebView) Mode() Mode {
	if wv == nil {
		return ModeBrowser
	}
	return wv.mode
}

// SetMode 设置运行模式。必须在 LoadHTML 之前调用：装配阶段要按模式
// 决定 fetch 版本、浏览器全局、子框架与外部资源通道的接线，装配完成
// 后模式即锁定（同值调用是 no-op，切换返回 ErrModeLocked）。
func (wv *WebView) SetMode(m Mode) error {
	if wv == nil || wv.destroyed {
		return ErrDestroyed
	}
	if wv.modeLocked && m != wv.mode {
		return ErrModeLocked
	}
	wv.mode = m
	return nil
}

// SetResourceResolver 设置宿主资源解析器（见 ResourceResolver）。两种
// 模式都先经它；UI 库模式下它是外部引用的唯一通道，因此通常在装配前
// 设置（运行时替换也生效——解析器在每次资源引用时调用）。
//
// 每次外部引用最多问两轮：先按页面写下的**原样**引用（宿主常用逻辑名
// `app://theme.css`），未命中且该引用可被文档 URL 绝对化时再问一次
// **绝对 URL** 形式（宿主也可只认其中一种）。
func (wv *WebView) SetResourceResolver(fn ResourceResolver) {
	if wv == nil {
		return
	}
	wv.resourceResolver = fn
	// 换 resolver = 取内容的语义变了：清空资源缓存，否则旧 resolver 提供的
	// 内容会继续被使用（缓存见 resource_cache.go）。
	wv.ClearResourceCache()
}

// loadExternalResource 把外部资源引用解析为内容，是**所有外部资源**的统一
// 接线点：`<link rel=stylesheet>`、`<script src>`、CSS `@import`
// （Frame.importStyleSheetLoader → StyleSheetLoader → 本方法），以及
// `<img src>`/background-image（rendering.ImageResourceLoader →
// webViewImageLoader.Load → 本方法）。
//
// 顺序：
//  1. 缓存命中（按解析后的绝对 URL 索引，见 resource_cache.go）
//  2. 宿主 ResourceResolver（两种模式一致，可用它覆盖网络/文件系统），
//     先按脚本写下的原样引用问一次（宿主常用逻辑名），绝对化后再问一次
//  3. 相对引用以**文档基准**（document.baseURI = 文档 URL + `<base href>`）
//     解析为绝对 URL（浏览器语义）
//  4. data: URL（内联内容，非外部输入，两种模式都允许）
//  5. 模式门禁：http(s)/file 仅 ModeBrowser（UI 库模式返回
//     ErrExternalResourceBlocked）——★ 门禁在缓存查询**之前**，否则别的
//     WebView（浏览器模式）留下的缓存会让被拒绝的引用穿透模式承诺
//  6. 取内容 + 按用途做 MIME 检查（nosniff 语义）+ 写缓存
func (wv *WebView) loadExternalResource(ref string, purpose ResourcePurpose) (string, error) {
	if ref == "" {
		return "", errors.New("webkit: empty resource reference")
	}
	cache := wv.resourceCacheFor()
	if wv.resourceResolver != nil {
		// resolver 的结果也缓存（键带前缀，与取内容通道区分）：UI 库模式下
		// 样式重扫会重复问同一个引用，宿主不必再自己套一层缓存。
		if r, ok := cache.get("resolver:" + ref); ok {
			return r.content, nil
		}
		if content, ok := wv.resourceResolver(ref); ok {
			cache.put("resolver:"+ref, cachedResource{content: content})
			return content, nil
		}
	}
	// ★ 相对引用以「文档 URL」为基准解析（浏览器语义）：真实页面里
	//   <link href="app.css"> / <script src="/js/x.js"> 都是相对路径，原样
	//   交给下面的分支必然失败（os.ReadFile("app.css") 会去读宿主进程的
	//   当前工作目录）。无文档 URL（LoadHTML 直出内容）时不做解析，保持
	//   既有行为（相对当前目录的文件读取）。
	if abs := dom.ResolveURL(wv.documentBaseURL(), ref); abs != ref {
		if r, ok := cache.get("resolver:" + abs); ok {
			return r.content, nil
		}
		if wv.resourceResolver != nil {
			if content, ok := wv.resourceResolver(abs); ok {
				cache.put("resolver:"+abs, cachedResource{content: content})
				return content, nil
			}
		}
		ref = abs
	}
	if strings.HasPrefix(ref, "data:") {
		if r, ok := cache.get(ref); ok {
			return r.content, nil
		}
		res, err := fetchResource(ref)
		if err != nil {
			return "", err
		}
		if !mimeAllowed(purpose, res.contentType, res.nosniff) {
			return "", fmt.Errorf("webkit: %q: Content-Type %q 不适用于 %s（nosniff）",
				ref, res.contentType, purpose)
		}
		cache.put(ref, cachedResource{content: res.content, contentType: res.contentType})
		return res.content, nil
	}
	if !wv.mode.allowsExternalURLs() {
		return "", fmt.Errorf("%w: %q（UI 库模式请用 SetResourceResolver 提供，或改用 data: URL）",
			ErrExternalResourceBlocked, ref)
	}
	// ★ 缓存查询放在模式门禁之后（见函数注释第 5 条）。
	if r, ok := cache.get(ref); ok {
		return r.content, nil
	}
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		res, err := fetchResource(ref)
		if err != nil {
			return "", err
		}
		if !mimeAllowed(purpose, res.contentType, res.nosniff) {
			// 浏览器语义：nosniff 下类型不符的资源**不采用**（样式不生效、
			// 脚本不执行），控制台给一条错误。
			return "", fmt.Errorf("webkit: %q: Content-Type %q 不适用于 %s（X-Content-Type-Options: nosniff）",
				ref, res.contentType, purpose)
		}
		if !res.noStore {
			cache.put(ref, cachedResource{content: res.content, contentType: res.contentType})
		}
		return res.content, nil
	}
	fp := fileURLPath(ref)
	d, err := os.ReadFile(fp)
	if err != nil {
		return "", fmt.Errorf("load resource %q: %w", ref, err)
	}
	cache.put(ref, cachedResource{content: string(d)})
	return string(d), nil
}

// resourceCacheFor 返回本 WebView 的资源缓存（惰性创建）。
func (wv *WebView) resourceCacheFor() *resourceCache {
	if wv == nil {
		return nil
	}
	wv.resourceCacheMu.Lock()
	defer wv.resourceCacheMu.Unlock()
	if wv.resourceCache == nil {
		wv.resourceCache = newResourceCache()
	}
	return wv.resourceCache
}

// ClearResourceCache 清空资源内存缓存（宿主内容变化、需要强制重新取内容时用；
// SetResourceResolver 会自动清空）。
func (wv *WebView) ClearResourceCache() {
	if wv == nil {
		return
	}
	wv.resourceCacheMu.Lock()
	c := wv.resourceCache
	wv.resourceCacheMu.Unlock()
	c.clear()
}

// fileURLPath 把 file:// 引用转成本地文件路径。
//
// 支持标准 URL 形式 file:///C:/dir/f.css（URL 的 Path 是 /C:/dir/f.css，
// 需要去掉盘符前的斜杠）与 file://C:/dir/f.css、file:///home/u/f.css、
// UNC 形式的 file://host/share/f.css。旧的 `strings.TrimPrefix(ref,
// "file://")` 对标准形式会留下前导斜杠 → os.ReadFile 失败（file:///…
// 一律读不到）——这里统一规范化。
//
// 非 file:// 引用原样返回（调用方已处理 http(s)/data:）。
func fileURLPath(ref string) string {
	if !strings.HasPrefix(ref, "file://") {
		return ref
	}
	if u, err := url.Parse(ref); err == nil && u.Path != "" {
		p := u.Path
		if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
			// Windows 盘符路径：/C:/dir/file → C:/dir/file
			p = p[1:]
		}
		if u.Host != "" && u.Host != "localhost" {
			// UNC：file://host/share → //host/share
			p = "//" + u.Host + p
		}
		return p
	}
	return strings.TrimPrefix(ref, "file://")
}
