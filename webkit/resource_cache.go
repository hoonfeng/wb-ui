package webkit

// 外部资源的内存缓存与「按用途的 MIME 检查」。
//
// 两件事都属于「与浏览器行为对齐」：
//
//  1. **内存缓存**：浏览器对同一 URL 的外部资源只取一次（memory cache），页面
//     里重复引用（多个 `<link>` 指向同一 CSS、样式重扫、@import 重复）不会反复
//     打网络/宿主。引擎此前每次引用都重新取——UI 库模式下的表现尤其刺眼：
//     宿主 ResourceResolver 被样式重扫反复调用，大文件被重复读入。
//
//  2. **MIME 检查**：浏览器对 `<link rel=stylesheet>` / `<script src>` 在响应带
//     `X-Content-Type-Options: nosniff` 时按类型拒绝（text/plain 的 CSS 不生效、
//     image/png 的脚本不执行），没有 nosniff 时宽松接受（仅控制台提示）；图片
//     不看 MIME（解码成功即采用）。引擎此前只记录日志，从不拒绝。
//
// 缓存范围是**每个 WebView**：宿主 ResourceResolver 是每个 WebView 自己的
// （模板内容随宿主状态而变），缓存必须与「谁在取内容」一致；换 resolver 时
// 整体清空（见 WebView.SetResourceResolver）。图片解码结果的共享是另一回事
// （rendering.backgroundImageCache 是进程级的）。

import (
	"strings"
	"sync"
)

// ResourcePurpose 是资源引用的用途：决定 MIME 检查强度。
type ResourcePurpose int

const (
	// PurposeStylesheet：`<link rel=stylesheet>` 与 CSS `@import`。
	PurposeStylesheet ResourcePurpose = iota
	// PurposeScript：`<script src>`。
	PurposeScript
	// PurposeImage：`<img src>` / background-image / mask-image / SVG <image>。
	PurposeImage
	// PurposeDocument：文档（宿主 LoadURL 走自己的取内容通道，此值用于日志）。
	PurposeDocument
)

func (p ResourcePurpose) String() string {
	switch p {
	case PurposeStylesheet:
		return "stylesheet"
	case PurposeScript:
		return "script"
	case PurposeImage:
		return "image"
	default:
		return "document"
	}
}

// 缓存上限（浏览器同样有内存缓存上限）：条数与总字节数任一超限即按插入顺序
// 淘汰最旧的条目。
const (
	maxResourceCacheEntries = 128
	maxResourceCacheBytes   = 4 << 20 // 4 MB
)

type cachedResource struct {
	content     string
	contentType string
}

// resourceCache 见文件头注释。
type resourceCache struct {
	mu      sync.Mutex
	entries map[string]cachedResource
	order   []string
	bytes   int
}

func newResourceCache() *resourceCache {
	return &resourceCache{entries: map[string]cachedResource{}}
}

func (c *resourceCache) get(key string) (cachedResource, bool) {
	if c == nil {
		return cachedResource{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.entries[key]
	return r, ok
}

func (c *resourceCache) put(key string, r cachedResource) {
	if c == nil || key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; exists {
		return
	}
	c.entries[key] = r
	c.order = append(c.order, key)
	c.bytes += len(r.content)
	for (len(c.order) > maxResourceCacheEntries || c.bytes > maxResourceCacheBytes) && len(c.order) > 0 {
		old := c.order[0]
		c.order = c.order[1:]
		if e, ok := c.entries[old]; ok {
			c.bytes -= len(e.content)
			delete(c.entries, old)
		}
	}
}

func (c *resourceCache) clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.entries = map[string]cachedResource{}
	c.order = nil
	c.bytes = 0
	c.mu.Unlock()
}

// isJavascriptMIMEType 报告媒体类型是否是可执行的 JS 类型（HTML 规范的
// "JavaScript MIME type" 常用子集）。
func isJavascriptMIMEType(media string) bool {
	if media == "" {
		return true // 无类型：浏览器按脚本处理
	}
	if i := strings.IndexByte(media, ';'); i >= 0 {
		media = strings.TrimSpace(media[:i])
	}
	switch media {
	case "application/ecmascript", "application/javascript", "application/x-ecmascript",
		"application/x-javascript", "text/ecmascript", "text/javascript",
		"text/javascript1.0", "text/javascript1.1", "text/javascript1.2",
		"text/javascript1.3", "text/javascript1.4", "text/javascript1.5",
		"text/jscript", "text/livescript", "text/x-ecmascript", "text/x-javascript":
		return true
	}
	return false
}

// mimeAllowed 按用途做浏览器语义的 MIME 检查（false = 该资源按规范不应被采用）。
//
// 浏览器行为对照：
//   - `<link rel=stylesheet>` / `<script src>`：带 `X-Content-Type-Options:
//     nosniff` 时**严格**按 MIME 拒绝；没有 nosniff 时宽松接受（仅提示）。
//   - 图片不看 MIME：解码成功即采用（浏览器也是「解码失败才算失败」）。
//   - 无 Content-Type（file://、部分服务器）：不因缺类型而拒绝。
func mimeAllowed(purpose ResourcePurpose, contentType string, nosniff bool) bool {
	if contentType == "" {
		return true
	}
	media := parseMediaType(contentType)
	switch purpose {
	case PurposeStylesheet:
		if media == "text/css" {
			return true
		}
		return !nosniff
	case PurposeScript:
		if isJavascriptMIMEType(media) {
			return true
		}
		return !nosniff
	default:
		return true
	}
}
