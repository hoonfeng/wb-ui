package rendering

// 动图帧推进（实现路径主线 A4）：宿主注入「多帧解码」，引擎按帧时长选帧。
//
// 分工与 A1 的视频帧一致——**解码在宿主**（goskia 的 SkCodec / 平台解码器），引擎只
// 维护「url → 帧序列 + 起始时刻」并按经过时间选帧。这样渲染层不依赖具体解码库，
// 宿主（app）也不必把帧序列塞进图片缓存（那是单帧路径）。
//
// 为什么需要 HasAnimatedImages()：引擎按需渲染（app.Host 里 `rv.IsDirty()` 为假就跳过
// Paint），动图必须让宿主知道「还有动画在跑」，否则 GIF 会停在第一帧不动。

import (
	"sync"
	"time"
)

// AnimatedFrame 是动图的一帧（解码后的位图 + 停留时长）。
type AnimatedFrame struct {
	Image *DecodedImage
	// DurationMS 是该帧停留时长（毫秒）；<=0 表示容器没给，按 animatedDefaultFrameMS 兜底。
	DurationMS int
}

// AnimatedImageSource 由宿主注入：把 (url, 原始编码字节) 解成多帧动图。
// ok=false 表示不是动图（静态图、解码失败、宿主没有该能力）——引擎继续走单帧路径。
// loops 用 Skia 语义：-1 = 无限循环，0 = 只播一次，n>0 = 循环 n 次。
type AnimatedImageSource func(url string, data []byte) (frames []AnimatedFrame, loops int, ok bool)

// animatedDefaultFrameMS 是容器没给帧时长时的兜底（浏览器对无延时 GIF 的常见处理）。
const animatedDefaultFrameMS = 100

// animatedEntry 是一个 url 的动图状态（帧序列 + 起始时刻）。
type animatedEntry struct {
	frames  []AnimatedFrame
	loops   int
	totalMS int
	started time.Duration
}

var animatedImages = struct {
	mu    sync.Mutex
	src   AnimatedImageSource
	clock func() time.Duration
	cache map[string]*animatedEntry
}{
	cache: map[string]*animatedEntry{},
	// 时钟默认取进程单调时钟：动图推进不能受系统时间调整影响。
	clock: func() time.Duration { return time.Duration(time.Now().UnixNano()) },
}

// SetAnimatedImageSource 注册（传 nil 清除）宿主的动图解码器。注册时清空帧缓存：
// 解码器换了解码结果不再可信。
func SetAnimatedImageSource(src AnimatedImageSource) {
	animatedImages.mu.Lock()
	animatedImages.src = src
	animatedImages.cache = map[string]*animatedEntry{}
	animatedImages.mu.Unlock()
}

// SetAnimatedImageClock 替换内部时钟（测试用：动图推进依赖时间，测试不能真等）。
func SetAnimatedImageClock(fn func() time.Duration) {
	if fn == nil {
		return
	}
	animatedImages.mu.Lock()
	animatedImages.clock = fn
	animatedImages.cache = map[string]*animatedEntry{}
	animatedImages.mu.Unlock()
}

// HasAnimatedImages 报告当前是否有动图在播放（宿主据此决定「没有别的脏源也要重绘」）。
func HasAnimatedImages() bool {
	animatedImages.mu.Lock()
	defer animatedImages.mu.Unlock()
	return len(animatedImages.cache) > 0
}

// ResetAnimatedImages 清空动图缓存与注册（测试/页面卸载用）。
func ResetAnimatedImages() {
	animatedImages.mu.Lock()
	animatedImages.cache = map[string]*animatedEntry{}
	animatedImages.mu.Unlock()
}

// animatedFrameForData 是图片加载路径的入口：给定 (url, 编码字节)，若宿主把它识别为
// 动图，则登记帧序列并返回**当前应显示的帧**。
//
// 时间基准是该 url **首次**被请求的时刻（不是每帧重置）——否则每次重绘都会从第 0 帧
// 开始，动图永远停在第一帧。
func animatedFrameForData(url string, data []byte) (*DecodedImage, bool) {
	if url == "" || len(data) == 0 {
		return nil, false
	}
	animatedImages.mu.Lock()
	src := animatedImages.src
	now := animatedImages.clock()
	if e, hit := animatedImages.cache[url]; hit {
		img := e.frameAt(now)
		animatedImages.mu.Unlock()
		return img, img != nil
	}
	animatedImages.mu.Unlock()
	if src == nil {
		return nil, false
	}

	// 解码在锁外（宿主可能跑 SkCodec，几十毫秒级）。
	frames, loops, ok := src(url, data)
	if !ok || len(frames) == 0 {
		return nil, false
	}
	entry := &animatedEntry{frames: frames, loops: loops, started: now}
	for _, f := range frames {
		d := f.DurationMS
		if d <= 0 {
			d = animatedDefaultFrameMS
		}
		entry.totalMS += d
	}
	animatedImages.mu.Lock()
	if _, exists := animatedImages.cache[url]; !exists {
		animatedImages.cache[url] = entry
	}
	e := animatedImages.cache[url]
	img := e.frameAt(animatedImages.clock())
	animatedImages.mu.Unlock()
	return img, img != nil
}

// frameAt 返回 now 时刻应显示的帧。loops == 0（只播一次）时停在末帧；否则按总时长取模。
func (e *animatedEntry) frameAt(now time.Duration) *DecodedImage {
	if e == nil || len(e.frames) == 0 {
		return nil
	}
	elapsed := int((now - e.started).Milliseconds())
	if elapsed < 0 {
		elapsed = 0
	}
	if e.loops == 0 && elapsed >= e.totalMS {
		return e.frames[len(e.frames)-1].Image // 只播一次：停在末帧
	}
	t := 0
	if e.totalMS > 0 {
		t = elapsed % e.totalMS
	}
	acc := 0
	for _, f := range e.frames {
		d := f.DurationMS
		if d <= 0 {
			d = animatedDefaultFrameMS
		}
		acc += d
		if t < acc {
			return f.Image
		}
	}
	return e.frames[len(e.frames)-1].Image
}

// animatedFrameForURL 查**已登记**的动图并按当前时刻取帧（不触发解码）。
func animatedFrameForURL(url string) (*DecodedImage, bool) {
	if url == "" {
		return nil, false
	}
	animatedImages.mu.Lock()
	defer animatedImages.mu.Unlock()
	e, hit := animatedImages.cache[url]
	if !hit {
		return nil, false
	}
	img := e.frameAt(animatedImages.clock())
	return img, img != nil
}

// IsAnimatedImageURL 报告 url 是否已被登记为动图。
// ★ 动图**不能**进单帧图片缓存（backgroundImageCache.imgs）：那里一个 url 只存一帧，
// 缓存命中会让 GIF 永远停在登记那一刻的帧上。
func IsAnimatedImageURL(url string) bool {
	animatedImages.mu.Lock()
	defer animatedImages.mu.Unlock()
	_, hit := animatedImages.cache[url]
	return hit
}

// decodeImageOrAnimated 解码图片字节：若宿主注册了动图解码器且识别为动图，返回**当前
// 应显示的帧**并登记帧序列；否则返回普通单帧解码结果（既有行为逐字节不变）。
func decodeImageOrAnimated(url string, data []byte) *DecodedImage {
	if img, ok := animatedFrameForData(url, data); ok {
		return img
	}
	return NewDecodedImage(data)
}

// animatedFrameForSrc 是**绘制路径**取「当前帧」的入口：按图片加载时用的同一套 URL
// 解析（宿主的 loader 可能把相对引用解析成绝对 URL）去找已登记的动图。
//
// 为什么绘制路径需要它：`RenderBox` 上缓存的 DecodedImage 是「某次绘制取到的帧」的
// 快照，直接复用会让 GIF/WebP 动画永远停在那一帧（首帧加载后被 SetDecodedImage 固化）。
// 动图的帧由引擎按帧时长选，所以每次绘制都要重新问一次「现在该显示哪一帧」。
func animatedFrameForSrc(src string) (*DecodedImage, bool) {
	if src == "" {
		return nil, false
	}
	if img, ok := animatedFrameForURL(src); ok {
		return img, true
	}
	if loader := currentImageLoaderForDraw(); loader != nil {
		if abs := loader.ResolveURL(src); abs != "" && abs != src {
			if img, ok := animatedFrameForURL(abs); ok {
				return img, true
			}
		}
	}
	return nil, false
}
