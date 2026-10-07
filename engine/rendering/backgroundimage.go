// Background-image painting: url(...) images with background-size and
// background-position. Complements the gradient painting in painter.go.
// Mirrors Source/WebCore/rendering/BackgroundPainter.cpp's image path
// (BackgroundImageGeometry).

package rendering

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"wb-ui/engine/dom"
	"wb-ui/engine/html"
	"wb-ui/engine/platform/graphics"
)

// parseBackgroundURL extracts the URL inside a url(...) token.
// Returns ("", false) when s is not a url(...) value.
func parseBackgroundURL(s string) (string, bool) {
	s = strings.TrimSpace(s)
	low := strings.ToLower(s)
	if !strings.HasPrefix(low, "url(") {
		return "", false
	}
	inner := s[4:]
	// ★ 取**第一个** url() 的内容，不是最后一个 ")"：多层背景
	// `background-image: url(a.png), url(b.png)` 是合法且常见的写法，用
	// LastIndex 会把整串当成 URL（`a.png), url(b.png`），连第一层都画不出来。
	// （本引擎只绘制第一层背景图——多层叠加未实现——但第一层的解析必须正确。）
	inner = strings.TrimSpace(inner)
	// 带引号形式按引号配对截断，避免引号内的 ")" 提前结束。
	if len(inner) > 0 && (inner[0] == '"' || inner[0] == '\'') {
		if j := strings.IndexByte(inner[1:], inner[0]); j >= 0 {
			return outerTrim(inner[1 : 1+j]), true
		}
		return outerTrim(inner[1:]), true
	}
	if i := strings.IndexByte(inner, ')'); i >= 0 {
		inner = inner[:i]
	}
	return outerTrim(inner), true
}

// outerTrim 去掉一层成对的引号（tokenizer 已解引，这里兜底手写字符串）。
func outerTrim(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// httpGet fetches a URL's body with a short timeout. Returns nil on error.
func httpGet(url string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 16<<20)) // 16 MiB cap
}

// svgBackgroundCache caches parsed SVG background documents by URL string.
var svgBackgroundCache = struct {
	mu   sync.Mutex
	docs map[string]*svgDocument
}{docs: map[string]*svgDocument{}}

// loadBackgroundSVG resolves and parses an SVG image reference
// (data:image/svg+xml URIs, file:// URLs or document-relative paths). Returns
// nil when the reference is not an SVG or cannot be parsed. Results are cached
// by (resolved) URL.
func loadBackgroundSVG(url string) *svgDocument {
	return loadBackgroundSVGWith(url, currentImageLoaderForDraw())
}

// loadBackgroundSVGWith 是 loadBackgroundSVG 的实现体。
//
// ★ U5（缺陷 D3）：此前只认 `data:image/svg+xml` 与「不含冒号的裸路径」，
// 因此 `file://`（含冒号）与文档相对路径**一律返回 nil**——文件引用的 SVG
// 图标/贴图全都不显示（探针实测 C 组第三列空白）。修复方式：非 data: 的
// 引用交给宿主 loader，走与栅格图**完全同一条链**（宿主 ResourceResolver
// 优先 → 按文档基准绝对化 → 逐 URL 策略门禁 → 取字节），于是
//   - ModeBrowser / AllowAll：`file://` 与相对路径都可渲染（= Edge 行为）；
//   - Toolkit + DenyExternal（默认）：仍拒（安全默认不变，TC-M-902）；
//   - Toolkit + AllowHostResolved：仅放行宿主 resolver 提供的引用。
//
// 无 loader（独立渲染测试、dev 探针、纯 rendering 用法）时保留既有的
// 「本地路径直接读」行为。
//
// 网络引用（http(s)/协议相对 //）不在本路径同步取字节：同步网络请求会
// 阻塞渲染线程，栅格图同样把它们交给异步加载（见 loadBackgroundImageWith
// 的 loader 分支）。本项（D3）只涉及 file:// 与相对路径。
func loadBackgroundSVGWith(url string, loader ImageResourceLoader) *svgDocument {
	svgBackgroundCache.mu.Lock()
	defer svgBackgroundCache.mu.Unlock()
	// ★ 缓存查询必须放在**资源策略门禁之后**（见 default 分支）：解析结果缓存是
	//   包级全局的、跨 WebView 与配置共享。若在门禁之前命中缓存，一个宽松配置
	//   （ModeBrowser）解析过的 SVG 会让后续严格配置（Toolkit+DenyExternal）
	//   绕过门禁拿到它——实测探针里 `DenyExternal × rel` 的 SVG 因此呈现
	//   「加载/几何/契约 ✅ 而绘制 ❌」的自相矛盾状态（绘制走门禁路径被拒，
	//   而 IDL/事件走本函数的缓存路径拿到了文档）。
	//   `data:` 与策略无关（自包含、无条件放行），在它自己的分支里查缓存。
	var text string
	// fallback 是 QueryEscape 风格的兼容解码结果（见下方回退说明），仅当首选
	// 解码结果解析不出 SVG 时才会被使用。
	var fallback string
	cacheKey := url
	low := strings.ToLower(url)
	switch {
	case strings.HasPrefix(low, "data:image/svg+xml"):
		// data: 自带内容、不经外部通道 → 与资源策略解耦，两种模式都放行。
		if d, ok := svgBackgroundCache.docs[url]; ok {
			return d
		}
		if strings.Contains(low, ";base64,") {
			if b, ok := decodeDataURI(url); ok {
				text = string(b)
			}
		} else if i := strings.Index(url, ","); i >= 0 {
			// ★ RFC 2397：data URI 的载荷是 **percent-编码文本**，`+` 保持字面，
			//   **不是**空格（那是 form-encoding 的约定）。此前用 QueryUnescape，
			//   SVG 内容里字面出现的 `+`（如 transform="translate(+1,2)"、
			//   path 数据里的 `M0+0`）会被吃成空格 → 图形画错或整段解析失败。
			//   首选按 URL 路径规则解码（与布局侧 readResource 一致）；仅当结果
			//   解析不出 SVG 时，才回退 query 规则，兼容按 QueryEscape 生成的
			//   URI（既有测试与历史页面正是这种写法）。
			raw := url[i+1:]
			text = decodeDataURIText(raw)
			fallback = decodeQueryURIText(raw)
		}
	case strings.HasPrefix(low, "http://"), strings.HasPrefix(low, "https://"),
		strings.HasPrefix(low, "//"):
		return nil
	default:
		ref := url
		if loader != nil {
			if abs := loader.ResolveURL(ref); abs != "" {
				ref = abs
			}
			// ★ 同步取字节只对「本地 + SVG 扩展名」开放：本函数在 paint
			//   线程上被调用（每个 `<img src>` 与 background-image 都会做一次
			//   SVG 探测），对栅格图或远端引用同步取字节会**阻塞渲染线程**
			//   并与栅格图的异步加载重复发请求——实测 `<img src="slow.png">`
			//   （相对路径先被绝对化成本页 http URL）在这里同步发起请求，
			//   Render 被扣住整个 HTTP 超时（30s），回归用例
			//   TestAsyncImageLoadMarksFrameDirty 因此由 PASS(0.12s) 变 FAIL。
			//   栅格图、data: 之外的远端引用一律交给异步通道
			//   （loadBackgroundImageWith 的 loader 分支）。
			if !isSVGReference(ref) || isRemoteReference(ref) {
				return nil
			}
			if !loader.AllowsURL(ref) {
				return nil
			}
			// 缓存按**解析后**的 URL 索引：同页同引用共享，跨页同相对路径
			//（各自绝对化到不同文件）不会互相串台。
			cacheKey = ref
			if d, ok := svgBackgroundCache.docs[ref]; ok {
				return d
			}
			b, err := loader.Load(ref)
			if err != nil {
				return nil
			}
			text = string(b)
		} else {
			// 无 loader（独立渲染测试 / 纯 rendering 用法）：沿用「本地路径直接读」，
			// 缓存键就是原样引用。
			if d, ok := svgBackgroundCache.docs[cacheKey]; ok {
				return d
			}
			if b, err := os.ReadFile(localFilePath(ref)); err == nil {
				text = string(b)
			}
		}
	}
	if text == "" {
		return nil
	}
	doc := parseSVGText(text)
	if doc == nil && fallback != "" && fallback != text {
		doc = parseSVGText(fallback)
	}
	if doc == nil {
		return nil
	}
	svgBackgroundCache.docs[cacheKey] = doc
	return doc
}

// decodeDataURIText 按 RFC 2397 解码非 base64 的 data URI 载荷：载荷是
// **percent-编码文本**，`+` 保持字面（不是空格——那是 form-encoding 的约定，
// 见 decodeQueryURIText）。解码失败时原样返回（载荷本就是明文的容错路径）。
func decodeDataURIText(raw string) string {
	if dec, err := neturl.PathUnescape(raw); err == nil {
		return dec
	}
	return raw
}

// decodeQueryURIText 按 form-encoding 规则（`+` = 空格）解码 data URI 载荷，
// 只用于「percent 解码结果解析不出 SVG」时的兼容回退：返回空串表示无可用回退
// （解码失败或与原文本相同）。
func decodeQueryURIText(raw string) string {
	dec, err := neturl.QueryUnescape(raw)
	if err != nil || dec == raw {
		return ""
	}
	return dec
}

// localFilePath 把引用转成本地文件路径：`file:///F:/dir/x.svg` → `F:/dir/x.svg`
// （Windows 盘符形式，URL 的 Path 带前导斜杠），其余（相对/绝对路径）原样
// 返回。与 webkit.FileURLPath 同一套规范化，但渲染层不能反向依赖 webkit。
func localFilePath(ref string) string {
	if !strings.HasPrefix(strings.ToLower(ref), "file://") {
		return ref
	}
	p := ref[len("file://"):]
	p = strings.TrimPrefix(p, "localhost")
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		// Windows 盘符路径：/F:/dir/file → F:/dir/file
		p = p[1:]
	}
	return p
}

// isSVGReference 报告引用是否**看起来**是 SVG（按路径扩展名判定，忽略
// query/fragment 与大小写）。用于在 paint 线程上决定「要不要同步取字节」：
// 同步通道只服务本地 SVG 文件，栅格图与远端引用一律走异步加载通道。
func isSVGReference(ref string) bool {
	p := ref
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	low := strings.ToLower(p)
	return strings.HasSuffix(low, ".svg") || strings.HasSuffix(low, ".svgz")
}

// isRemoteReference 报告引用是否指向远端（http(s) 或协议相对 //）——这类
// 引用在 paint 线程上绝不同步取字节（HTTP 超时会扣住整个渲染线程）。
func isRemoteReference(ref string) bool {
	low := strings.ToLower(ref)
	return strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") ||
		strings.HasPrefix(low, "//")
}

// parseSVGText parses an SVG document string and builds the svgDocument for
// painting. Uses the HTML parser (which folds SVG foreign content).
func parseSVGText(text string) *svgDocument {
	d, err := html.Parse(text)
	if err != nil {
		return nil
	}
	var svgEl *dom.Element
	var walk func(n dom.Node)
	walk = func(n dom.Node) {
		if svgEl != nil {
			return
		}
		if el, ok := n.(*dom.Element); ok && el.LocalName() == "svg" {
			svgEl = el
			return
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	for c := d.FirstChild(); c != nil; c = c.NextSibling() {
		walk(c)
	}
	if svgEl == nil {
		return nil
	}
	return buildSVGDocument(svgEl)
}

// paintSVGScaled paints an svgDocument into the destination rect (x,y,w,h).
// It routes through paintSVGTo with the rect as viewport, so viewBox and
// preserveAspectRatio resolve exactly like inline <svg> elements — and the
// shared cached svgDocument's mutable viewport fields are never touched.
func paintSVGScaled(canvas *graphics.Canvas, svg *svgDocument, x, y, w, h float64) {
	if svg == nil || canvas == nil {
		return
	}
	paintSVGTo(canvas, svg, x, y, w, h, graphics.Color{})
}

// paintBackgroundImageTiled draws a decoded background image into the box
// honoring background-repeat. The image's destination (dx,dy,dw,dh) is the
// first tile; repeat (default) tiles in both axes, repeat-x/repeat-y tile in
// one axis, no-repeat draws a single tile. Tiles that start before the box
// edge start at the tile's own offset so the pattern stays aligned with the
// position origin (matching CSS: the position defines the first tile's
// location).
func paintBackgroundImageTiled(canvas *graphics.Canvas, img *DecodedImage,
	x, y, w, h, dx, dy, dw, dh float64, repeat string) {
	if canvas == nil || img == nil || !img.Loaded() || dw <= 0 || dh <= 0 {
		return
	}
	rep := strings.ToLower(strings.TrimSpace(repeat))
	repX := rep != "no-repeat" && rep != "repeat-y"
	repY := rep != "no-repeat" && rep != "repeat-x"
	startX := dx
	startY := dy
	for ty := startY; ty < y+h; ty += dh {
		for tx := startX; tx < x+w; tx += dw {
			if repX && tx+dw < x {
				continue
			}
			if repY && ty+dh < y {
				continue
			}
			img.Draw(canvas, tx, ty, dw, dh)
			if !repX {
				break
			}
		}
		if !repY {
			break
		}
	}
}

// decodeDataURI decodes a data: URI (data:image/png;base64,XXXX) to bytes.
// Returns (data, true) for base64 data URIs; (nil, false) otherwise.
func decodeDataURI(uri string) ([]byte, bool) {
	low := strings.ToLower(uri)
	if !strings.HasPrefix(low, "data:") {
		return nil, false
	}
	if !strings.Contains(low, ";base64,") {
		return nil, false
	}
	_, raw, _ := strings.Cut(uri, ";base64,")
	raw = strings.TrimSpace(raw)
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, false
	}
	return data, true
}

// backgroundImageCache caches decoded background images by URL string.
//
// ⚠️ Single-WebView global: see package doc.
var backgroundImageCache = struct {
	mu      sync.Mutex
	imgs    map[string]*DecodedImage
	loading map[string]bool // http(s) URLs currently being fetched
	// retryAt：加载失败后的重试退避截止时刻。★ 没有它时，一个必然失败的
	// URL（被宿主 loader 拒绝、404、返回空数据）会让**每一次 paint** 都
	// 重新发起异步加载、每次失败都触发"已加载"通知 → 宿主
	// MarkRenderTreeDirty() → 下一帧全量重建渲染树。实测真实 IDE 页面上
	// 形成逐帧循环：每帧重建树 38.3MB / 76822 allocs（≈80ms），是
	// 「帧耗时 300ms+、GC 占 35%」的最大单一来源
	// （见 webkit/diag_frame_rebuild_test.go 的调用栈证据）。
	retryAt map[string]time.Time
}{imgs: map[string]*DecodedImage{}, loading: map[string]bool{}, retryAt: map[string]time.Time{}}

// bgImageRetryBackoff 是背景图加载失败后的重试退避时长。既保留"下次绘制
// 会重试"的既有语义（临时故障最终能自愈），又把重试频率从每帧降到每秒
// 一次，避免"每帧失败 → 每帧标脏 → 每帧全量重建树"的逐帧风暴。
const bgImageRetryBackoff = time.Second

// bgImageBackingOff 报告 url 是否处于失败退避窗口内。调用方须持有
// backgroundImageCache.mu。
func bgImageBackingOff(url string) bool {
	t, bad := backgroundImageCache.retryAt[url]
	return bad && time.Now().Before(t)
}

// bgImageLoadedCallback, when set, is invoked after an async http(s)
// background image finishes loading (success or failure). Hosts use it to
// schedule a repaint so the image appears without waiting for the next
// frame-driven paint.
var bgImageLoadedCallback func(url string)

// SetBackgroundImageLoadedCallback registers the callback fired after an
// async background image load completes. Pass nil to clear.
func SetBackgroundImageLoadedCallback(cb func(url string)) {
	backgroundImageCache.mu.Lock()
	bgImageLoadedCallback = cb
	backgroundImageCache.mu.Unlock()
}

// bgImageLoadedListeners 是「图片加载完成」的多监听器集合：单回调
// （bgImageLoadedCallback）给纯 rendering 用法（测试/探针），监听器给宿主
// 接线——多个 WebView 各自关心自己文档里的图片，单回调会被后注册者覆盖。
var bgImageLoadedListeners = struct {
	mu   sync.Mutex
	next int
	fns  map[int]func(string, bool)
}{fns: map[int]func(string, bool){}}

// AddBackgroundImageLoadedListener 注册图片（异步）加载完成监听器，返回
// 幂等的注销函数。
//
// 为什么需要它：浏览器里资源到位就会 invalidate 重绘，而本引擎的按需渲染
// （app.Host.Run 里 `rv.IsDirty()` 为假即跳过 Paint）不会自己发现「缓存里
// 多了一张图」——`<img>` / background-image 的字节在后台 goroutine 取回后
// 没有任何人置脏，图片就**永远不画出来**。宿主（webkit.WebView）用本接口
// 接线：图片到位 → 标记渲染树脏 + MarkAllDirty。
//
// ★ ok 表示本次**是否解码成功**：宿主据此为文档里的 `<img>` 派发
// load / error（HTML 规范的事件语义）。此前只有 url，宿主无法区分成功与
// 失败——`<img onerror>` 永远收不到通知，图片失败重试/占位逻辑失效。
func AddBackgroundImageLoadedListener(fn func(url string, ok bool)) func() {
	if fn == nil {
		return func() {}
	}
	bgImageLoadedListeners.mu.Lock()
	bgImageLoadedListeners.next++
	id := bgImageLoadedListeners.next
	bgImageLoadedListeners.fns[id] = fn
	bgImageLoadedListeners.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			bgImageLoadedListeners.mu.Lock()
			delete(bgImageLoadedListeners.fns, id)
			bgImageLoadedListeners.mu.Unlock()
		})
	}
}

// notifyBackgroundImageLoaded 触发「图片加载完成」（成功与失败都触发，与
// 既有单回调语义一致；ok 区分二者，供宿主派发 load / error）。在取字节的
// goroutine 里调用：先取快照再回调，不持锁调用外部代码。
func notifyBackgroundImageLoaded(url string, ok bool) {
	backgroundImageCache.mu.Lock()
	cb := bgImageLoadedCallback
	backgroundImageCache.mu.Unlock()
	if cb != nil {
		cb(url)
	}
	bgImageLoadedListeners.mu.Lock()
	fns := make([]func(string, bool), 0, len(bgImageLoadedListeners.fns))
	for _, fn := range bgImageLoadedListeners.fns {
		fns = append(fns, fn)
	}
	bgImageLoadedListeners.mu.Unlock()
	for _, fn := range fns {
		fn(url, ok)
	}
}

// IsImageReady 报告某个 URL 的图片是否已**解码就绪**（命中解码缓存，或已
// 登记为动图帧序列）。
//
// 用途：`data:` URL 走 loadBackgroundImageWith 的同步分支解码，不产生异步
// 完成通知——宿主无法通过监听器得知「这张内联图已经好了」。宿主在绘制后
// 用它补派发 `<img>` 的 load 事件（见 webkit 的 flushImageEvents）。
func IsImageReady(url string) bool {
	backgroundImageCache.mu.Lock()
	_, ok := backgroundImageCache.imgs[url]
	backgroundImageCache.mu.Unlock()
	if ok {
		return true
	}
	return IsAnimatedImageURL(url)
}

// RequestImageLoad 主动为某个 URL 发起一次异步加载（若尚未在加载、尚未
// 就绪，且不在失败退避窗口内）。重复调用是廉价的（受 loading/退避保护）。
//
// 为什么需要它：引擎的图片加载此前只发生在**绘制**路径（loadBackgroundImage
// → loader），而替换元素的盒尺寸来自资源固有尺寸——**加载失败的资源量不出
// 尺寸、盒塌成 0×0、于是根本不绘制**，加载也就永远不发起，最终「图片失败」
// 既不显示也不报错（`<img onerror>` 永远收不到通知）。浏览器语义是「src
// 生效即开始加载」，与是否绘制无关；宿主（webkit）用本接口把这条语义补上。
func RequestImageLoad(url string, loader ImageResourceLoader) {
	if url == "" || loader == nil {
		return
	}
	if !loader.AllowsURL(url) {
		return // 策略拒绝：既不加载也不通知（安全默认）
	}
	backgroundImageCache.mu.Lock()
	defer backgroundImageCache.mu.Unlock()
	if _, ok := backgroundImageCache.imgs[url]; ok {
		return
	}
	if backgroundImageCache.loading[url] || bgImageBackingOff(url) {
		return
	}
	backgroundImageCache.loading[url] = true
	go fetchImageViaLoaderAsync(url, loader)
}

// defaultSVGWidth/Height 是 SVG 固有尺寸缺失时的默认值（CSS Images §3 的
// 替换元素默认尺寸 300×150，比例 2:1）。
const (
	defaultSVGWidth  = 300
	defaultSVGHeight = 150
)

// SVGReferenceIntrinsicSize 返回 SVG 引用的**固有尺寸**（对应
// HTMLImageElement 的 naturalWidth/naturalHeight）：width/height 属性优先，
// 缺失时用 viewBox；两者都没有时按 CSS 默认尺寸算法取 300×150（只有一边
// 有效时用 2:1 比例补齐另一边）。
//
// ok=false 表示「这个引用不是可渲染的 SVG」：栅格图、远端引用（走异步通道）、
// 被策略拒绝、解析失败。宿主据此把 SVG 与位图两条路分开——SVG 是矢量资源，
// 不进位图解码缓存，所以它的固有尺寸只能这样回答。
func SVGReferenceIntrinsicSize(url string, loader ImageResourceLoader) (float64, float64, bool) {
	if !isSVGReferenceURL(url) {
		return 0, 0, false
	}
	doc := loadBackgroundSVGWith(url, loader)
	if doc == nil {
		return 0, 0, false
	}
	w, h := svgIntrinsicSize(doc)
	switch {
	case w <= 0 && h <= 0:
		w, h = defaultSVGWidth, defaultSVGHeight
	case w <= 0:
		w = h * defaultSVGWidth / defaultSVGHeight
	case h <= 0:
		h = w * defaultSVGHeight / defaultSVGWidth
	}
	return w, h, true
}

// IsSVGReferenceReady 报告某个引用是否为**已经可以渲染**的 SVG 矢量资源。
//
// 宿主（webkit）用它补派发 `<img>` 的 load：SVG 不进位图解码缓存，
// IsImageReady 对它恒为假——照位图逻辑走会变成「Skia 解不出位图 → error」，
// 而它其实画得出来。浏览器语义是「能渲染即加载成功」。
func IsSVGReferenceReady(url string, loader ImageResourceLoader) bool {
	_, _, ok := SVGReferenceIntrinsicSize(url, loader)
	return ok
}

// isSVGReferenceURL 报告引用是否为「本地可判定的 SVG」：`data:image/svg+xml`
// 内联，或本地/相对路径的 `.svg`/`.svgz`。远端引用（http(s)/协议相对）恒
// false——它们由异步取字节通道负责，不能被同步判定（见 loadBackgroundSVGWith
// 对 paint 线程阻塞的说明）。
func isSVGReferenceURL(url string) bool {
	low := strings.ToLower(url)
	if strings.HasPrefix(low, "data:") {
		return strings.HasPrefix(low, "data:image/svg+xml")
	}
	if isRemoteReference(url) {
		return false
	}
	return isSVGReference(url)
}

// LoadImageSync 是 loadBackgroundImage 的导出包装（供 webkit 桥按 <img>
// 元素的 src 主动解码：canvas 2D drawImage 的图片源）。
func LoadImageSync(url string) *DecodedImage {
	return loadBackgroundImage(url, "")
}

// LoadImageWithLoader 用宿主提供的 loader 加载并解码图片（canvas 2D
// drawImage 的图片源）：与 paint 路径同一条策略链——URL 解析、宿主
// ResourceResolver 优先、模式门禁都由 loader 决定。loader 为 nil 时等价于
// LoadImageSync（内置行为）。
//
// 注意这是**同步**加载（JS 的 drawImage 需要立即拿到图）；宿主接线时
// 引用通常已在 paint 路径取回过（命中缓存）。
func LoadImageWithLoader(url string, loader ImageResourceLoader) *DecodedImage {
	return loadBackgroundImageWith(url, "", loader)
}

// loadBackgroundImage resolves and decodes a background-image URL.
// data: URIs and file paths decode synchronously (local, fast). http(s)
// URLs fetch asynchronously: the first call spawns a goroutine and returns
// nil; subsequent paints pick the image from the cache once loaded. This
// keeps the render thread unblocked by network latency.
func loadBackgroundImage(url, baseDir string) *DecodedImage {
	return loadBackgroundImageWith(url, baseDir, currentImageLoaderForDraw())
}

// loadBackgroundImageWith 是 loadBackgroundImage 的实现体。loader 非 nil
// （宿主接线，见 image_resource.go）时：先把引用按文档基准解析为绝对 URL，
// data: 之外的引用一律交给 loader 取字节（宿主决定 ResourceResolver 优先、
// http(s) 是否允许、本地文件读取）——UI 库模式下网络引用因此被拒绝，而不是
// 由渲染层静默联网。loader 为 nil 时保持既有内置行为（独立渲染/探针场景
// 逐字节不变）。
func loadBackgroundImageWith(url, baseDir string, loader ImageResourceLoader) *DecodedImage {
	// ★ data: URI 自带内容、不涉及任何外部资源——必须在策略门禁**之前**放行
	//（契约见 image_resource.go 的 AllowsURL：「data: URL 恒放行，与资源策略
	// 解耦」，两种模式都允许）。此前门禁在前，UI 库模式（ModeToolkit）下连
	// data: 图都被拒：`<video poster="data:…">`、内嵌 data: 图标、内联 SVG 的
	// `background-image: url(data:image/png;base64,…)` 全部画不出来。
	if b, ok := decodeDataURI(url); ok {
		// 动图（A4）：data: URI 里的 GIF/WebP 动画同样按当前时刻取帧——一次性登记帧
		// 序列，之后每次绘制都重新选帧。
		if img, isAnim := animatedFrameForData(url, b); isAnim {
			return img
		}
		backgroundImageCache.mu.Lock()
		defer backgroundImageCache.mu.Unlock()
		if img, hit := backgroundImageCache.imgs[url]; hit {
			return img
		}
		img := NewDecodedImage(b)
		if img == nil {
			return nil
		}
		backgroundImageCache.imgs[url] = img
		return img
	}
	if loader != nil && url != "" {
		// ★ 策略门禁先于缓存查询：backgroundImageCache 是进程级全局的，
		//   若另一个 WebView（浏览器模式）已经加载过同一 URL，缓存命中会
		//   让 UI 库模式下被拒绝的图片照样显示——门禁被缓存旁路（探针实测：
		//   ModeToolkit 里的 `<img src="http://…/pic.png">` 显示出了浏览器
		//   模式刚取回的图）。
		// ★ 顺序：先按文档基准解析为绝对 URL，再**逐 URL** 判定策略——
		//   AllowHostResolved 的放行对象是「宿主明确提供的那个引用」，
		//   必须先拿到解析后的引用才判得准（TC-M-903/904）。
		if abs := loader.ResolveURL(url); abs != "" {
			url = abs
		}
		if !loader.AllowsURL(url) {
			return nil
		}
	}
	// 动图（A4）：已登记的 url **每次绘制**都按当前时刻取帧——不能走下面的单帧缓存
	// （一个 url 只存一帧，一旦命中缓存 GIF 会永远停在登记那一刻的帧上）。
	if img, ok := animatedFrameForURL(url); ok {
		return img
	}
	backgroundImageCache.mu.Lock()
	defer backgroundImageCache.mu.Unlock()
	if img, ok := backgroundImageCache.imgs[url]; ok {
		return img
	}
	var data []byte
	if loader != nil {
		// 宿主接线：data: 之外的引用（http(s)/file/相对）交给宿主。首次
		// 调用异步启动并返回 nil，goroutine 填充缓存 + 触发已加载回调，
		// 之后的 paint 命中缓存即画出。
		if !backgroundImageCache.loading[url] && !bgImageBackingOff(url) {
			backgroundImageCache.loading[url] = true
			go fetchImageViaLoaderAsync(url, loader)
		}
		return nil
	} else if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		// Remote image: async fetch once per URL. Return nil now; the
		// goroutine fills the cache and fires the loaded callback.
		if !backgroundImageCache.loading[url] && !bgImageBackingOff(url) {
			backgroundImageCache.loading[url] = true
			go fetchBackgroundImageAsync(url)
		}
		return nil
	} else if strings.HasPrefix(url, "file://") {
		// file:///F:/path/to.png（模板/贴图资源引用）：剥前缀读文件。
		p := strings.TrimPrefix(url, "file://")
		p = strings.TrimPrefix(p, "/")
		b, err := os.ReadFile(p)
		if err == nil {
			data = b
		}
	} else if !strings.Contains(url, ":") { // not a scheme, treat as file
		p := url
		if baseDir != "" && !strings.HasPrefix(url, "/") && !strings.HasPrefix(url, "\\") {
			p = baseDir + string(os.PathSeparator) + url
		}
		b, err := os.ReadFile(p)
		if err == nil {
			data = b
		}
	}
	if len(data) == 0 {
		return nil
	}
	// 动图（A4）：宿主注册了解码器且识别成功时，这里返回的是**当前应显示的帧**，
	// 且该 url 不再进单帧缓存（否则后续绘制会拿到登记那一刻的固定帧）。
	img := decodeImageOrAnimated(url, data)
	if img == nil {
		return nil
	}
	if !IsAnimatedImageURL(url) {
		backgroundImageCache.imgs[url] = img
	}
	return img
}

// fetchImageViaLoaderAsync 通过宿主 loader 取图片（可能真的走网络、命中宿主
// ResourceResolver，或被 UI 库模式拒绝），解码后写入缓存并触发已加载回调。
// 失败不写缓存（与既有 http 路径一致：下一次 paint 会重试）。
func fetchImageViaLoaderAsync(url string, loader ImageResourceLoader) {
	data, err := loader.Load(url)
	backgroundImageCache.mu.Lock()
	delete(backgroundImageCache.loading, url)
	loaded := false
	if err == nil && len(data) > 0 {
		if img := decodeImageOrAnimated(url, data); img != nil {
			if !IsAnimatedImageURL(url) { // 动图不进单帧缓存（见 loadBackgroundImageWith）
				backgroundImageCache.imgs[url] = img
			}
			loaded = true
		}
	}
	if loaded {
		delete(backgroundImageCache.retryAt, url)
	} else {
		// ★ 失败：退避一段时间再重试。否则下一次 paint 立刻重新发起 →
		// 再次失败 → 再次通知 → 宿主每帧 MarkRenderTreeDirty（见 retryAt
		// 字段注释）。
		backgroundImageCache.retryAt[url] = time.Now().Add(bgImageRetryBackoff)
	}
	backgroundImageCache.mu.Unlock()
	notifyBackgroundImageLoaded(url, loaded)
}

// fetchBackgroundImageAsync downloads an http(s) image off-thread and stores
// the decoded result in the cache, then fires the loaded callback.
func fetchBackgroundImageAsync(url string) {
	data, err := httpGet(url)
	backgroundImageCache.mu.Lock()
	delete(backgroundImageCache.loading, url)
	loaded := false
	if err == nil && len(data) > 0 {
		if img := decodeImageOrAnimated(url, data); img != nil {
			if !IsAnimatedImageURL(url) { // 动图不进单帧缓存（见 loadBackgroundImageWith）
				backgroundImageCache.imgs[url] = img
			}
			loaded = true
		}
	}
	if loaded {
		delete(backgroundImageCache.retryAt, url)
	} else {
		backgroundImageCache.retryAt[url] = time.Now().Add(bgImageRetryBackoff)
	}
	backgroundImageCache.mu.Unlock()
	notifyBackgroundImageLoaded(url, loaded)
}

// bgSizeMode describes how background-size scales the image.
type bgSizeMode int

const (
	bgSizeAuto     bgSizeMode = iota // original pixel size
	bgSizeCover                      // scale to cover the box (crop overflow)
	bgSizeContain                    // scale to fit inside the box (letterbox)
	bgSizeExplicit                   // width/height lengths or percentages
)

// parseBackgroundSize parses a background-size value. Returns the mode and,
// for bgSizeExplicit, the horizontal/vertical sizes (percentages are 0..100
// with px>0, or absolute px values with unit "px").
func parseBackgroundSize(s string) (mode bgSizeMode, wPct, wPx, hPct, hPx float64) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" || s == "auto" {
		return bgSizeAuto, 0, 0, 0, 0
	}
	if s == "cover" {
		return bgSizeCover, 0, 0, 0, 0
	}
	if s == "contain" {
		return bgSizeContain, 0, 0, 0, 0
	}
	// <width> [<height>] — lengths/percentages; single value => auto height.
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return bgSizeAuto, 0, 0, 0, 0
	}
	mode = bgSizeExplicit
	wPct, wPx = parseBgSizePart(parts[0])
	if len(parts) > 1 {
		hPct, hPx = parseBgSizePart(parts[1])
	}
	return
}

func parseBgSizePart(v string) (pct, px float64) {
	if strings.HasSuffix(v, "%") {
		if n, ok := parseFloatS(strings.TrimSuffix(v, "%")); ok {
			return n, 0
		}
		return 0, 0
	}
	// px / em / etc: strip the unit; non-px units are treated as px-scale
	// (simplification: background-size supports px and % in practice).
	v = strings.TrimSuffix(v, "px")
	if n, ok := parseFloatS(v); ok {
		return 0, n
	}
	return 0, 0
}

func parseFloatS(s string) (float64, bool) {
	n, err := strconv.ParseFloat(s, 64)
	return n, err == nil
}

// computeBackgroundDest computes the destination rect for a background image
// inside a (x,y,w,h) box, honoring background-size and background-position.
func computeBackgroundDest(x, y, w, h float64, size, position string, imgW, imgH int) (dx, dy, dw, dh float64) {
	mode, wPct, wPx, hPct, hPx := parseBackgroundSize(size)
	iw, ih := float64(imgW), float64(imgH)
	switch mode {
	case bgSizeCover:
		scale := maxF(w/iw, h/ih)
		dw, dh = iw*scale, ih*scale
	case bgSizeContain:
		scale := minF(w/iw, h/ih)
		dw, dh = iw*scale, ih*scale
	case bgSizeExplicit:
		aw, ah := wPx, hPx
		if wPct > 0 {
			aw = w * wPct / 100
		}
		if hPct > 0 {
			ah = h * hPct / 100
		}
		dw, dh = aw, ah
		// auto height preserves aspect ratio.
		if dw <= 0 && dh > 0 {
			dw = dh * iw / ih
		} else if dh <= 0 && dw > 0 {
			dh = dw * ih / iw
		}
		if dw <= 0 {
			dw = w
		}
		if dh <= 0 {
			dh = h
		}
	default: // auto: original size
		dw, dh = iw, ih
	}
	// Position: default 0% 0% (top-left). Percentages offset by
	// (box - image) so 50% centers, 100% aligns bottom-right.
	px, py := parseBackgroundPosition(position, w-dw, h-dh)
	dx, dy = x+px, y+py
	return
}

// computeGradientDest computes the destination rect for a gradient layer
// inside a box. Gradients have no intrinsic size, so background-size:
// auto/cover/contain all mean "fill the box"; explicit lengths/percentages
// confine the gradient to a sub-rect, offset by background-position.
func computeGradientDest(x, y, w, h float64, size, pos string) (dx, dy, dw, dh float64) {
	mode, wPct, wPx, hPct, hPx := parseBackgroundSize(size)
	dw, dh = w, h
	if mode == bgSizeExplicit {
		if wPct > 0 {
			dw = w * wPct / 100
		} else if wPx > 0 {
			dw = wPx
		}
		if hPct > 0 {
			dh = h * hPct / 100
		} else if hPx > 0 {
			dh = hPx
		}
	}
	px, py := parseBackgroundPosition(pos, w-dw, h-dh)
	dx, dy = x+px, y+py
	return
}

// parseBackgroundPosition parses background-position. Percentages and
// keywords (left/center/right, top/middle/bottom) resolve against the
// (box - image) delta; px values are absolute offsets.
func parseBackgroundPosition(s string, deltaW, deltaH float64) (px, py float64) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, 0
	}
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return 0, 0
	}
	// Two-value form, or a single value that implies the other axis is center.
	xv, yv := parts[0], "center"
	if len(parts) >= 2 {
		yv = parts[1]
	}
	if len(parts) == 1 && (xv == "top" || xv == "bottom") {
		xv, yv = "center", parts[0]
	}
	px = resolveBgPosAxis(xv, deltaW)
	py = resolveBgPosAxis(yv, deltaH)
	return
}

func resolveBgPosAxis(v string, delta float64) float64 {
	switch v {
	case "left", "top":
		return 0
	case "center", "middle":
		return delta / 2
	case "right", "bottom":
		return delta
	}
	if strings.HasSuffix(v, "%") {
		if n, ok := parseFloatS(strings.TrimSuffix(v, "%")); ok {
			return delta * n / 100
		}
		return 0
	}
	if n, ok := parseFloatS(v); ok {
		return n
	}
	return 0
}
