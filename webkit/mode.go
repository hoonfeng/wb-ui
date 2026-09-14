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
func (wv *WebView) SetResourceResolver(fn ResourceResolver) {
	if wv == nil {
		return
	}
	wv.resourceResolver = fn
}

// loadExternalResource 把外部资源引用解析为内容，是 <link rel=stylesheet>、
// <script src>、@import 等所有外部资源引用的**统一接线点**。
//
// 顺序：
//  1. 宿主 ResourceResolver（两种模式一致，可用它覆盖网络/文件系统）
//  2. data: URL（内联内容，非外部输入，两种模式都允许）
//  3. http(s) / file://（仅 ModeBrowser；UI 库模式返回
//     ErrExternalResourceBlocked）
func (wv *WebView) loadExternalResource(ref string) (string, error) {
	if ref == "" {
		return "", errors.New("webkit: empty resource reference")
	}
	if wv.resourceResolver != nil {
		if content, ok := wv.resourceResolver(ref); ok {
			return content, nil
		}
	}
	if strings.HasPrefix(ref, "data:") {
		return fetchURL(ref)
	}
	if !wv.mode.allowsExternalURLs() {
		return "", fmt.Errorf("%w: %q（UI 库模式请用 SetResourceResolver 提供，或改用 data: URL）",
			ErrExternalResourceBlocked, ref)
	}
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return fetchURL(ref)
	}
	fp := fileURLPath(ref)
	d, err := os.ReadFile(fp)
	if err != nil {
		return "", fmt.Errorf("load resource %q: %w", ref, err)
	}
	return string(d), nil
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
