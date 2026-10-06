// 图片资源的宿主接线（ImageResourceLoader）：`<img src>`、background-image、
// mask-image、SVG `<image href>` 的 URL 解析与取字节统一交给宿主。
//
// 为什么需要它：rendering 包只负责「画一张已解码的图」，不应当自己决定
// 「这个 URL 能不能联网、该从哪读」。只有宿主（webkit.WebView）知道运行
// 模式（ModeBrowser/ModeToolkit）、文档 URL 与宿主 ResourceResolver。
// 此前渲染层内置 httpGet 直接联网，代价是三条：
//   - UI 库模式（ModeToolkit）下 `<img src="http://…">` 会**真的发请求**，
//     违反「零网络」承诺；
//   - 相对引用不按文档 URL 解析（`<img src="logo.png">` 被当成宿主进程
//     工作目录下的文件）——真实页面里的图片必然加载失败；
//   - 宿主 ResourceResolver（内存/内嵌资源通道）对图片无效。
//
// 接线链：page.Frame.ImageLoader → RenderTreeBuilder.SetImageLoader →
// RenderView.imageLoader → Paint 入口设为「当前 loader」→
// loadBackgroundImage 用它解析 URL 并取字节（异步，不阻塞渲染线程）。
// 未接线（nil：独立渲染测试、dev 探针、纯 rendering 用法）时保持既有
// 内置行为（data:/本地文件同步，http 异步），行为不变。

package rendering

import "sync"

// ImageResourceLoader 是图片资源的宿主接线。
type ImageResourceLoader interface {
	// ResolveURL 把页面里写下的图片引用（可能为相对路径）按文档基准解析
	// 为绝对 URL。返回空串表示「无法解析 / 无需解析」，调用方保留原引用
	// 并继续按既有规则处理。
	ResolveURL(ref string) string

	// Load 取图片字节。宿主在这里决定策略：宿主 ResourceResolver 优先、
	// http(s) 是否允许（模式门禁）、本地文件读取。返回 error 表示加载
	// 失败（例如 UI 库模式拒绝了网络引用）——调用方不缓存失败结果。
	Load(absURL string) ([]byte, error)

	// AllowsURL 报告**已按文档基准解析为绝对 URL** 的引用是否允许加载。
	// false 时渲染层在**查询缓存之前**就拒绝该引用：图片缓存
	//（backgroundImageCache）是进程级全局的，若别的 WebView 已经加载过同一
	// URL，只看缓存会让被策略拒绝的图片照样显示出来——策略承诺因此被缓存
	// 旁路（探针实测过：ModeToolkit 里的 `<img src="http://…/pic.png">`
	// 显示出了浏览器模式刚取回的图）。
	//
	// 为什么逐 URL 而不是「模式是否允许外部资源」：资源策略
	// AllowHostResolved 只放行宿主 ResourceResolver **明确提供**的引用，
	// 粗粒度判定无法表达「这个 URL 宿主给了、那个没给」。data: URL 自带
	// 内容、不经外部通道，恒放行（与策略解耦，两种模式都允许）。
	AllowsURL(url string) bool
}

// currentImageLoader 是本线程当前绘制上下文的图片 loader。由 Paint 入口
// 从 RenderView 取出并设置（见 renderpipeline.go），因为 loadBackgroundImage
// 的调用点遍布 painter/mask/svg 各处，逐处传参会让签名大面积膨胀。
//
// 多 WebView：每个 WebView 有自己的 RenderView → loader，Paint 期间
// save/restore（嵌套 Paint：iframe 子 Frame 绘制）保证策略始终来自当前
// 正在绘制的那个文档。
var currentImageLoader = struct {
	mu sync.RWMutex
	l  ImageResourceLoader
}{}

// swapCurrentImageResourceLoader 设置当前图片 loader 并返回旧值：
// Paint 入口用 `defer swapCurrentImageResourceLoader(prev)` 恢复。
func swapCurrentImageResourceLoader(l ImageResourceLoader) ImageResourceLoader {
	currentImageLoader.mu.Lock()
	prev := currentImageLoader.l
	currentImageLoader.l = l
	currentImageLoader.mu.Unlock()
	return prev
}

// currentImageLoaderForDraw 返回当前绘制上下文的图片 loader（可能为 nil）。
func currentImageLoaderForDraw() ImageResourceLoader {
	currentImageLoader.mu.RLock()
	l := currentImageLoader.l
	currentImageLoader.mu.RUnlock()
	return l
}
