// 视频帧注入（实现路径主线 A1：「宿主注入帧流」路线，见 docs/implementation-path.md §2）。
//
// 分工（与 A0 的 libs.MediaMetadataResolver 同构）：**解码全在宿主**（ffmpeg /
// 平台解码器），引擎只负责「把宿主给的帧画进元素的内容盒」。时间线由媒体绑定层
// 推进（engine/js/bindings/media_element.go 的播放时钟与 seek），本文件维护：
//
//	元素 → 播放画面状态（SetElementVideoState，由绑定层写入）
//	(url, 时间点) → 已解码帧（videoFrameForElement，由 painter 读取）
//
// 为什么不把帧源定义在 bindings：渲染层不能依赖 JS 绑定层（`rendering` 被
// `bindings` 依赖，反向成环），而 painter 需要帧。因此接口定义在渲染层：
// bindings 写入「当前显示状态」，宿主注入「怎么取帧」。
//
// 与 poster 的关系（HTML 的 poster frame 定义 + show poster flag）：
//
//	poster frame = poster 属性存在且图可用 & flag 置位 → 那张图；
//	               否则 → **当前播放位置的帧**；再没有 → 什么都不画。
//
// 也就是说 flag 置位并不等于「不许画帧」：没有 poster 的 <video> 加载完就该看到
// 第一帧。带 poster 的 <video> 则先显示 poster，直到 play() 或 currentTime 被设为
// 非 0（flag 清除）才切到真实帧。判断入口是 videoShowsPoster（painter 用它决定走
// poster 图还是走帧），本文件只管按 (url, 时间点) 取帧。
//
// 取帧有两条路（A2：播放流畅性）：
//
//	静止态（初始帧 / seek / 暂停）：**同步**取帧——用户就在等这一帧，几十毫秒
//	  换来「一定画得出来」，而且静态场景没有后续重绘可以弥补；
//	播放推进中（VideoElementState.Playing 为真）：**异步**取帧 + 预取——渲染
//	  线程绝不因解码等待，帧未交付时先显示上一帧（记 StaleFrames）。
//
// 宿主因此可以注册两个源：同步源（VideoFrameSource，须自带缓存，否则每帧都要
// 拉一次解码器）与异步源（VideoAsyncFrameSource，可为 nil = 全部走同步）。
// 缓存策略见 app/mediaprobe.go（按「文件 + 时间点」只抽一次）。
package rendering

import (
	"math"
	"strings"
	"sync"
	"time"

	"wb-ui/engine/dom"
)

// VideoFrameSource 由宿主注入：返回 url 指向的媒体在 atSeconds 处的帧数据
// （PNG/JPEG 等引擎可解码的位图字节）。ok=false 表示该帧取不到（本机没有
// ffmpeg、文件不存在、该时间点没有帧）——引擎记为负缓存并停止追问该帧。
type VideoFrameSource func(url string, atSeconds float64) ([]byte, bool)

// VideoAsyncFrameSource 由宿主注入：**异步**取帧（A2 播放流畅性）。调用后必须
// 立刻返回，帧数据（或失败）经 deliver 交付；deliver 可能在**任意 goroutine**
// 上被调用，实现须保证每个请求恰好交付一次。宿主侧通常实现为「有界队列 +
// 少量 worker 跑解码器」，见 app/mediaprobe.go 的 MediaProbe.FramePump。
type VideoAsyncFrameSource func(url string, atSeconds float64, deliver func(data []byte, ok bool))

// VideoElementState 是媒体绑定层声明的「元素当前显示什么」。
type VideoElementState struct {
	// URL 是元素的有效媒体源（<video src> 或 <source src> 的原始值；解析成本地
	// 文件是宿主的事）。空 = 没有可显示的画面（未加载 / 切源 / 出错）。
	URL string
	// Time 是应当显示的媒体时间点（秒）。加载完成时是 0（首帧）。
	Time float64
	// ShowPoster 是 HTML 的 show poster flag：true 表示此阶段显示 poster 替代
	// 画面（资源就绪但还没开始播放），false 表示显示 Time 处的真实帧。
	ShowPoster bool
	// Playing 表示元素的播放时钟正在推进（绑定层填）。为真时 painter 取不到
	// Time 处的帧也**不同步等**解码：改走异步交付 + 先显示上一帧（A2）。
	Playing bool
}

// videoFrameKey 标识「某源的某个时间点」。时间量化为毫秒：浮点抖动（0.30000001
// vs 0.3）不应产生两个缓存条目，且抽帧精度本来也到不了毫秒。
type videoFrameKey struct {
	url string
	ms  int64
}

func videoKeyFor(url string, at float64) videoFrameKey {
	return videoFrameKey{url: url, ms: int64(at*1000 + 0.5)}
}

// quantizeLocked 用宿主注册的时刻量化器折叠「同一帧的不同时刻」（A2）。锁内调用
// （量化器只读宿主的已探测元数据，见 SetVideoFrameTimeQuantizer 的锁序说明）。
func (videoFrames *videoFrameState) quantizeLocked(url string, at float64) float64 {
	if videoFrames.quantize == nil {
		return at
	}
	q := videoFrames.quantize(url, at)
	if math.IsNaN(q) || math.IsInf(q, 0) {
		return at // 量化器出错时退回原值：绝不把无效键写进缓存
	}
	return q
}

// keyLocked 计算「显示/预取某时刻」的缓存键：先量化（帧率已知时把同一帧的时刻
// 折叠成一个键），再按毫秒量化（浮点抖动不应产生两个条目）。
func (videoFrames *videoFrameState) keyLocked(url string, at float64) videoFrameKey {
	return videoKeyFor(url, videoFrames.quantizeLocked(url, at))
}

// videoFrameCacheLimit 是已解码帧的缓存上限（FIFO 淘汰）。单帧约几百 KB（120x80
// RGBA 仅 38KB，1080p 是 8MB），64 条对调试/短播放足够——播放是**前向**推进，
// FIFO 淘汰掉的恰好是已经播过的旧时刻（预取只提交未来时刻，不会挤掉当前帧）。
// 长播放的超前预取窗口见 docs/TECH_DEBT.md。
const videoFrameCacheLimit = 64

// inflightMaxAge 是「已提交、尚未交付」的失效时长。宿主实现约定每个请求恰好
// 交付一次（见 VideoAsyncFrameSource），但漏交会时**永远**卡住那个时刻的帧
// （画面停在上一帧）；超过它的键因此允许重新请求。
const inflightMaxAge = 10 * time.Second

// VideoFrameStats 是帧通道的计数（诊断与自检用，见 cmd/psai 判据 A2-4）。
type VideoFrameStats struct {
	// SyncRequests 是渲染线程上**同步**向宿主要帧的次数——每次都可能在渲染
	// 线程上等数十毫秒（宿主要跑解码器）。播放推进期间它不增长，就说明渲染
	// 没有被解码拖住。
	SyncRequests int64
	// AsyncRequests 是播放推进中因未命中缓存而提交的异步请求数。
	AsyncRequests int64
	// PrefetchRequests 是 PrefetchVideoFrame 提交的预取请求数。
	PrefetchRequests int64
	// AsyncFrames 是异步交付并成功入缓存的帧数。
	AsyncFrames int64
	// StaleFrames 是绘制时目标时刻的帧还没到、退回显示上一帧（或空）的次数。
	StaleFrames int64
}

// videoFrameState 是帧通道的全部可变状态（进程内单例 videoFrames）：帧源、
// 元素显示状态、已解码帧缓存、在飞请求与统计。
type videoFrameState struct {
	mu    sync.Mutex
	src   VideoFrameSource
	async VideoAsyncFrameSource
	// quantize 是宿主注册的时刻量化器（A2-①：精确到帧的 seek）。显示与预取都用
	// 它折叠键：10fps 下 0.51/0.55/0.59 是同一帧，折叠后只解码一次、只缓存一份，
	// 预取 0.55 也就直接命中绘制 0.59。nil = 不折叠（帧率未知的默认行为）。
	quantize func(url string, atSeconds float64) float64
	times map[*dom.Element]VideoElementState
	imgs  map[videoFrameKey]*DecodedImage // 值为 nil = 负缓存（问过，取不到）
	order []videoFrameKey
	// inflight 是「已提交、尚未交付」的键：painter 每帧都会来问、预取也会重复
	// 请求同一时刻，去重后一次解码只跑一遍。
	inflight map[videoFrameKey]time.Time
	// last 是每个元素**最近一次成功显示**的帧键：播放中目标帧未交付时用它顶上
	// （显示旧一帧远好于空窗或阻塞渲染线程）。
	last  map[*dom.Element]videoFrameKey
	stats VideoFrameStats
}

var videoFrames = videoFrameState{
	times:    map[*dom.Element]VideoElementState{},
	imgs:     map[videoFrameKey]*DecodedImage{},
	inflight: map[videoFrameKey]time.Time{},
	last:     map[*dom.Element]videoFrameKey{},
}

// SetVideoFrameSource 注册（传 nil 清除）宿主的**同步**帧源。注册时丢弃既有帧
// 缓存：帧源换了，旧帧不再可信（可能来自另一个解码器/另一份文件）。
func SetVideoFrameSource(src VideoFrameSource) {
	videoFrames.mu.Lock()
	videoFrames.src = src
	videoFrames.resetLocked()
	videoFrames.mu.Unlock()
}

// SetVideoAsyncFrameSource 注册（传 nil 清除）宿主的**异步**帧源（A2）。注册后：
//   - 播放推进中未命中缓存的帧不再同步取，而是提交异步请求 + 先显示上一帧；
//   - PrefetchVideoFrame 的请求交给它执行。
//
// 未注册时一切照旧走同步路径（引擎独立使用、单测的默认行为）。
func SetVideoAsyncFrameSource(src VideoAsyncFrameSource) {
	videoFrames.mu.Lock()
	videoFrames.async = src
	videoFrames.resetLocked()
	videoFrames.mu.Unlock()
}

// SetVideoFrameTimeQuantizer 注册（传 nil 清除）宿主的**时刻量化器**（A2-①）。
//
// 显示与预取会就「同一帧」问很多个不同时刻——currentTime 是连续浮点数而帧是
// 离散的（10fps 下 0.51 / 0.55 / 0.59 都是第 5 帧）。量化器把这些时刻折叠到同一个
// 键上，于是同帧只解码一次、只缓存一份，预取也更容易被绘制命中。
//
// 宿主实现必须与它**自己的抽帧规则**一致（app/mediaprobe.go 的 alignFrameTime：
// 用已探测的帧率把时刻对齐到目标帧区间的取样点）；帧率未知时返回原值（不折叠）。
//
// 锁序：量化器在 videoFrames.mu 内被调用，实现只读自己的元数据缓存（宿主侧是
// MediaProbe.mu）；MediaProbe 的抽帧与交付路径都不持 videoFrames.mu，因此两级锁
// 只有「videoFrames.mu → probe.mu」一种获取顺序，不会死锁。
//
// 注册时不清空帧缓存：量化只改变键的折叠方式，已解码的帧数据仍然有效。
func SetVideoFrameTimeQuantizer(fn func(url string, atSeconds float64) float64) {
	videoFrames.mu.Lock()
	videoFrames.quantize = fn
	videoFrames.mu.Unlock()
}

// resetLocked 丢弃全部帧状态与缓存（换帧源时旧帧不再可信）。统计一并归零：
// 计数描述的是「当前这套帧源」的行为。
func (videoFrames *videoFrameState) resetLocked() {
	videoFrames.times = map[*dom.Element]VideoElementState{}
	videoFrames.imgs = map[videoFrameKey]*DecodedImage{}
	videoFrames.order = nil
	videoFrames.inflight = map[videoFrameKey]time.Time{}
	videoFrames.last = map[*dom.Element]videoFrameKey{}
	videoFrames.stats = VideoFrameStats{}
}

// SetElementVideoState 由媒体绑定层调用：声明元素的显示状态。URL 为空表示该
// 元素当前没有可显示的画面（未加载 / 切源 / 出错）——记录随之清除，painter 回退
// poster（若元素有 poster 属性）。
func SetElementVideoState(el *dom.Element, st VideoElementState) {
	if el == nil {
		return
	}
	videoFrames.mu.Lock()
	if st.URL == "" {
		delete(videoFrames.times, el)
		delete(videoFrames.last, el)
	} else {
		if old, ok := videoFrames.times[el]; ok && old == st {
			videoFrames.mu.Unlock()
			return // 状态未变：不制造无谓的写（painter 每次绘制都会读）
		}
		videoFrames.times[el] = st
	}
	videoFrames.mu.Unlock()
}

// ElementVideoState 返回元素当前声明的显示状态（只读，调试与测试用）。
// ok=false 表示该元素当前没有可显示的画面。
func ElementVideoState(el *dom.Element) (VideoElementState, bool) {
	if el == nil {
		return VideoElementState{}, false
	}
	videoFrames.mu.Lock()
	defer videoFrames.mu.Unlock()
	st, tracked := videoFrames.times[el]
	return st, tracked
}

// videoFrameForElement 返回元素当前应显示的帧（nil = 无帧，painter 应回退
// poster / 不画）。命中缓存（含负缓存）时零开销；未命中时分两种：
//
//	静止态（!Playing）→ **同步**取帧：这次绘制就等这一帧（初始帧 / seek /
//	  暂停），宿主实现必须自带缓存；
//	播放推进中（Playing 且有异步源）→ 提交异步请求后**立即**返回该元素上一次
//	  显示的帧：渲染线程不因解码阻塞（A2；未交付的帧由下一次绘制命中）。
//
// 同步取帧在锁外执行（宿主要跑解码器，几十毫秒级，持锁会阻塞其它元素的绘制与
// 绑定层写入）。
func videoFrameForElement(el *dom.Element) *DecodedImage {
	if el == nil {
		return nil
	}
	videoFrames.mu.Lock()
	st, tracked := videoFrames.times[el]
	src := videoFrames.src
	async := videoFrames.async
	if !tracked || (src == nil && async == nil) {
		videoFrames.mu.Unlock()
		return nil
	}
	key := videoFrames.keyLocked(st.URL, st.Time)
	if img, hit := videoFrames.imgs[key]; hit {
		if img != nil {
			videoFrames.last[el] = key // 只有真帧才算「最近显示」
		}
		videoFrames.mu.Unlock()
		return img // 命中（nil 亦为命中：负缓存）
	}
	if st.Playing && async != nil {
		if !videoFrames.inflightLocked(key) {
			videoFrames.inflight[key] = time.Now()
			videoFrames.stats.AsyncRequests++
			url, at := st.URL, st.Time
			go async(url, at, func(data []byte, ok bool) { deliverVideoFrame(key, data, ok) })
		}
		// 目标帧还没交付：先用上一次成功显示的帧顶上（显示旧一帧好过空窗，
		// 更远好过让渲染线程去等 ffmpeg）。
		fallback := videoFrames.lastFrameLocked(el, st.URL)
		videoFrames.stats.StaleFrames++
		videoFrames.mu.Unlock()
		return fallback
	}
	videoFrames.mu.Unlock()
	if src == nil {
		return nil
	}

	// ★ 取帧在锁外：宿主要跑 ffmpeg，几十毫秒级，绝不能持锁（会阻塞其它元素的
	// 绘制与绑定层的写入）。
	videoFrames.mu.Lock()
	videoFrames.stats.SyncRequests++
	videoFrames.mu.Unlock()
	data, ok := src(st.URL, st.Time)
	var img *DecodedImage
	if ok && len(data) > 0 {
		img = NewDecodedImage(data)
	}
	videoFrames.mu.Lock()
	if _, exists := videoFrames.imgs[key]; !exists {
		videoFrames.imgs[key] = img
		videoFrames.order = append(videoFrames.order, key)
		videoFrames.evictLocked()
	}
	if img != nil {
		videoFrames.last[el] = key
	}
	videoFrames.mu.Unlock()
	return img
}

// PrefetchVideoFrame 请求宿主**异步**预取 (url, atSeconds) 的帧（A2）。绑定层在
// 播放时钟推进时按自己的步长调用它（下一次 tick 会显示的时刻），使 painter 到达
// 时通常已经命中缓存——「播放中同步取帧 = 0 次」靠的就是它。
//
// 未注册异步源、url 为空、该键已缓存（含负缓存）或已在飞时，本函数是 no-op。
func PrefetchVideoFrame(url string, atSeconds float64) {
	if strings.TrimSpace(url) == "" {
		return
	}
	if atSeconds < 0 {
		atSeconds = 0
	}
	videoFrames.mu.Lock()
	async := videoFrames.async
	if async == nil {
		videoFrames.mu.Unlock()
		return
	}
	key := videoFrames.keyLocked(url, atSeconds)
	if _, hit := videoFrames.imgs[key]; hit || videoFrames.inflightLocked(key) {
		videoFrames.mu.Unlock()
		return // 已有（或已问过、取不到）：不重复解码
	}
	videoFrames.inflight[key] = time.Now()
	videoFrames.stats.PrefetchRequests++
	videoFrames.mu.Unlock()
	go async(url, atSeconds, func(data []byte, ok bool) { deliverVideoFrame(key, data, ok) })
}

// deliverVideoFrame 交付异步取回的一帧（宿主 worker 在任意 goroutine 上调用）：
// 写入缓存供后续绘制/预取命中，负缓存同样记录（不再反复追问取不到的时刻）。
//
// 它**不**直接改「元素最近显示的帧」：哪次绘制命中了这个键，由那次绘制自己记。
func deliverVideoFrame(key videoFrameKey, data []byte, ok bool) {
	var img *DecodedImage
	if ok && len(data) > 0 {
		img = NewDecodedImage(data)
	}
	added := false
	videoFrames.mu.Lock()
	delete(videoFrames.inflight, key)
	if _, exists := videoFrames.imgs[key]; !exists {
		videoFrames.imgs[key] = img
		videoFrames.order = append(videoFrames.order, key)
		videoFrames.evictLocked()
		if img != nil {
			videoFrames.stats.AsyncFrames++
			added = true
		}
	}
	videoFrames.mu.Unlock()
	if added {
		// 新帧入缓存 → 通知宿主「可以重绘了」（A2-③）。锁外调用：监听器可能直接
		// 触发宿主的主循环。url 与时刻由键还原（键已按帧折叠过）。
		notifyVideoFrameReady(key.url, float64(key.ms)/1000)
	}
}

// videoFrameReadyListeners 是「异步帧交付成功」的监听器集合（A2-③：帧就绪重绘）。
// 与图片加载完成通知（backgroundimage.go 的 bgImageLoadedListeners）同构：多个
// WebView 各自只关心自己文档里的帧，单个全局回调会被后注册者覆盖。
var videoFrameReadyListeners = struct {
	mu   sync.Mutex
	next int
	fns  map[int]func(url string, atSeconds float64)
}{fns: map[int]func(string, float64){}}

// AddVideoFrameReadyListener 注册「播放帧已交付」监听器，返回幂等的注销函数。
//
// 为什么需要它：按需渲染的宿主（app.Host 空闲帧跳过 Paint）不会自己发现「帧缓存里
// 多了一帧」——异步抽帧的字节是在 worker goroutine 上回来的，没有这一层通知，播放中
// 的帧要等**下一次别的重绘理由**（鼠标移动、脚本改 DOM）才可能露头，画面看起来就是
// 卡的。宿主（webkit.WebView）用它接线：帧到位 → 置脏重绘；绑定层的
// requestVideoFrameCallback 也由此拿到「新帧已呈现」的时机。
//
// 回调在交付 goroutine 上执行（与图片那条通知一致）：实现里不要直接操作 DOM。
func AddVideoFrameReadyListener(fn func(url string, atSeconds float64)) func() {
	if fn == nil {
		return func() {}
	}
	videoFrameReadyListeners.mu.Lock()
	videoFrameReadyListeners.next++
	id := videoFrameReadyListeners.next
	videoFrameReadyListeners.fns[id] = fn
	videoFrameReadyListeners.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			videoFrameReadyListeners.mu.Lock()
			delete(videoFrameReadyListeners.fns, id)
			videoFrameReadyListeners.mu.Unlock()
		})
	}
}

// notifyVideoFrameReady 通知所有监听器（在交付 goroutine 上调用；回调在锁外跑）。
func notifyVideoFrameReady(url string, atSeconds float64) {
	videoFrameReadyListeners.mu.Lock()
	fns := make([]func(string, float64), 0, len(videoFrameReadyListeners.fns))
	for _, fn := range videoFrameReadyListeners.fns {
		fns = append(fns, fn)
	}
	videoFrameReadyListeners.mu.Unlock()
	for _, fn := range fns {
		fn(url, atSeconds)
	}
}

// lastFrameLocked 返回元素上一次成功显示过的帧（同源才有效——换 src 后旧帧
// 属于另一个视频，绝不能顶上来）。帧已被淘汰时返回 nil（painter 不画）。
func (videoFrames *videoFrameState) lastFrameLocked(el *dom.Element, url string) *DecodedImage {
	key, ok := videoFrames.last[el]
	if !ok || key.url != url {
		return nil
	}
	return videoFrames.imgs[key]
}

// inflightLocked 报告该键是否正在等待交付（超过 inflightMaxAge 视为失效）。
func (videoFrames *videoFrameState) inflightLocked(key videoFrameKey) bool {
	at, ok := videoFrames.inflight[key]
	return ok && time.Since(at) < inflightMaxAge
}

// evictLocked 按 FIFO 淘汰超出上限的已解码帧。
func (videoFrames *videoFrameState) evictLocked() {
	for len(videoFrames.order) > videoFrameCacheLimit {
		oldest := videoFrames.order[0]
		videoFrames.order = videoFrames.order[1:]
		delete(videoFrames.imgs, oldest)
	}
}

// VideoFrameStatsSnapshot 返回帧通道的计数快照（诊断 / 自检用）。
func VideoFrameStatsSnapshot() VideoFrameStats {
	videoFrames.mu.Lock()
	defer videoFrames.mu.Unlock()
	return videoFrames.stats
}

// ResetVideoFrameStats 把计数清零（自检在「播放开始前」调用，之后只统计播放
// 期间的行为）。缓存与元素状态不受影响。
func ResetVideoFrameStats() {
	videoFrames.mu.Lock()
	videoFrames.stats = VideoFrameStats{}
	videoFrames.mu.Unlock()
}

// videoShowsPoster 报告 <video> 此刻是否该显示 poster 替代画面（poster frame 的
// 第一种情形）。定义为「元素有 poster 属性 + show poster flag 置位（或元素还没
// 进入加载流程）」——flag 置位但没有 poster 时不算：那时 poster frame 退化为当前
// 帧，必须继续渲染视频画面。
func videoShowsPoster(el *dom.Element) bool {
	if el == nil || strings.TrimSpace(el.GetAttribute("poster")) == "" {
		return false
	}
	st, tracked := ElementVideoState(el)
	return !tracked || st.ShowPoster
}
