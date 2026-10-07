// Package bindings — HTMLMediaElement 媒体元素模型（HTML §4.8.6 / §4.8.8）。
//
// 为什么需要：<video>/<audio> 此前只暴露 textTracks，没有媒体属性与方法。
// 框架（React/Vue 的 <video> 组件、video.js 一类库）挂载后立刻读 duration /
// readyState / paused，并在首个用户手势时调用 play()——属性读到 undefined、
// play 抛 TypeError（整段脚本中断，页面停在服务端 HTML），比"读不到时长"严重
// 得多。本文件把可观察的媒体契约补齐：
//
//   - 属性：src/currentSrc/duration/currentTime/paused/ended/seeking/volume/
//     muted/defaultMuted/playbackRate/defaultPlaybackRate/readyState/
//     networkState/error/buffered/played/seekable/videoWidth/videoHeight/
//     autoplay/loop/controls/preload/crossOrigin
//   - 方法：load()/play()（返回 Promise）/pause()/canPlayType()/fastSeek()/
//     addTextTrack()/captureStream()（无 MediaStream 实现 → null）
//   - 事件：loadstart/durationchange/loadedmetadata/loadeddata/canplay/
//     canplaythrough/play/playing/timeupdate/pause/ended/volumechange/
//     ratechange/seeking/seeked/error
//   - on* 事件处理器属性（onplay = fn 等，与 addEventListener 等价）
//
// 分工：解码与画面由宿主负责（直播挂件助手的 vcam/ffmpeg 帧注入），本层只维护
// 规范的状态机与事件。时长等元数据经 MediaMetadataResolver 由宿主注入；未注入
// 时 duration 为 NaN（readyState=HAVE_METADATA），此时播放时钟不推进 currentTime
// ——宁可让脚本走「时长未知」分支，也不编造进度。
package bindings

import (
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"wb-ui/engine/dom"
	"wb-ui/engine/js/jsc"
	"wb-ui/engine/rendering"
)

// ─── 宿主注入 ────────────────────────────────────────────

// MediaMetadata 是宿主探测到的媒体元数据（HTML §4.8.6：时长与画面尺寸都在
// 元数据阶段可得，对应 L1 的 duration/videoWidth/videoHeight）。
type MediaMetadata struct {
	// Duration 是媒体时长（秒）；NaN 表示时长未知（不编造播放进度）。
	Duration float64
	// Width/Height 是视频轨道的像素尺寸；0 表示未知（纯音频资源合法）。
	Width  int
	Height int
	// FPS 是视频轨道的帧率（帧/秒）；0 表示未知（纯音频资源，或容器没给帧率）。
	// 引擎侧用它把「帧」的概念带进播放时钟与预取（media_element.go 的
	// prefetchNextFrame 按帧预取；宿主按同一位移量化取帧时刻，见
	// app/mediaprobe.go 的 FrameTimeQuantizer）。
	FPS float64
}

// MediaMetadataResolver 由宿主设置：探测 src 指向的媒体元数据。宿主可用
// `ffmpeg -i <src>` 解析 stderr 的 Duration / 视频尺寸——返回 ok=false 表示
// 探测失败（回退到「时长未知」）。未设置时所有资源都按「时长未知」处理。
var MediaMetadataResolver func(src string) (MediaMetadata, bool)

// ─── 规范常量 ────────────────────────────────────────────

const (
	mediaNetworkEmpty    = 0 // NETWORK_EMPTY
	mediaNetworkIdle     = 1 // NETWORK_IDLE
	mediaNetworkLoading  = 2 // NETWORK_LOADING
	mediaNetworkNoSource = 3 // NETWORK_NO_SOURCE

	mediaHaveNothing     = 0 // HAVE_NOTHING
	mediaHaveMetadata    = 1 // HAVE_METADATA
	mediaHaveCurrentData = 2 // HAVE_CURRENT_DATA
	mediaHaveFutureData  = 3 // HAVE_FUTURE_DATA
	mediaHaveEnoughData  = 4 // HAVE_ENOUGH_DATA

	mediaErrAborted         = 1 // MEDIA_ERR_ABORTED
	mediaErrNetwork         = 2 // MEDIA_ERR_NETWORK
	mediaErrDecode          = 3 // MEDIA_ERR_DECODE
	mediaErrSrcNotSupported = 4 // MEDIA_ERR_SRC_NOT_SUPPORTED
)

// mediaTimeUpdateMs 是播放时钟步长（timeupdate 的规范建议频率量级）。
const mediaTimeUpdateMs = 250

// mediaPrefetchFrames 是知道帧率时每次 tick 预取的帧数（A2：更长的预取窗口）。
// 窗口宽度 = 帧数 ÷ 帧率（10fps 下 3 帧 = 300ms，与一个 timeupdate 步长同量级），
// 保证「下一次 timeupdate 要显示的帧」必定已在路上。渲染层按帧折叠键并对在飞请求
// 去重（engine/rendering/videoframe.go 的 quantize + inflight），所以一次预取几个
// 时刻不会变成几次解码：同一帧只算一次，已缓存/已在飞的时刻直接跳过。
const mediaPrefetchFrames = 3

// ─── 状态 ────────────────────────────────────────────────

// mediaHandler 是一个 on* 事件处理器属性：既是 JS 函数值（读取时返回原函数），
// 也是注册在元素上的 DOM 监听器（派发时由 jsListener 调用）。
type mediaHandler struct {
	fn jsc.JSValue
	l  *jsListener
}

// mediaElementState 是 <video>/<audio> 的媒体状态（每元素一份）。
type mediaElementState struct {
	interp *jsc.Interpreter
	el     *dom.Element

	networkState int
	readyState   int
	paused       bool
	ended        bool
	seeking      bool
	currentTime  float64
	duration     float64 // NaN = 未知（未注入元数据）
	videoW       int     // videoWidth：宿主探测到的视频宽（0 = 未知/纯音频）
	videoH       int     // videoHeight
	fps          float64 // 视频帧率（0 = 未知）：预取按帧推进时用它，见 prefetchNextFrame
	volume       float64
	muted        bool
	playbackRate float64
	defaultRate  float64

	// hasError/errorObj：MediaError（code + message），无错误时为 null。
	hasError bool
	errorObj *jsc.JSObject

	loadedSrc string
	clock     bool
	handlers  map[string]*mediaHandler

	// showPoster 是 HTML 的 show poster flag（§4.8.8）：资源加载后、play() 或
	// currentTime 设为非 0 之前，<video> 显示的是 poster 替代画面，而不是视频帧。
	showPoster bool

	// rvfc / rvfcNext / presented / lastFrameNo 是 requestVideoFrameCallback 的
	// 状态（A2-③）：回调表（handle → 函数，回调触发后即失效）、下一个句柄、已呈现
	// 帧数（规范的 presentedFrames，从 1 开始）、上一次呈现的帧号（-1 = 尚未呈现）。
	rvfc        map[int]jsc.JSValue
	rvfcNext    int
	presented   uint64
	lastFrameNo int64
}

var (
	mediaElMu    sync.Mutex
	mediaElCache = map[*dom.Element]*mediaElementState{}
)

// mediaStateFor 返回元素的媒体状态（首次访问时创建并启动资源选择算法）。
func mediaStateFor(in *jsc.Interpreter, el *dom.Element) *mediaElementState {
	if el == nil {
		return nil
	}
	mediaElMu.Lock()
	st, ok := mediaElCache[el]
	created := false
	if !ok {
		st = &mediaElementState{
			interp:       in,
			el:           el,
			networkState: mediaNetworkEmpty,
			readyState:   mediaHaveNothing,
			paused:       true,
			duration:     math.NaN(),
			volume:       1,
			playbackRate: 1,
			defaultRate:  1,
			// muted IDL 属性的初始值来自 muted content attribute（HTML
			// §4.8.6）；此后 muted 是播放器状态，不再反射 attribute。
			muted:      el.HasAttribute("muted"),
			handlers:    map[string]*mediaHandler{},
			showPoster:  true,
			rvfc:        map[int]jsc.JSValue{},
			lastFrameNo: -1,
		}
		mediaElCache[el] = st
		created = true
	}
	mediaElMu.Unlock()
	// src 已在 HTML 里写好的元素：首次访问媒体属性时启动加载（规范是元素插入
	// 文档时启动；惰性属性访问把这一刻推迟，事件仍异步派发）。★ 在锁外排任务：
	// mediaRunLater 会创建 JS 函数并进事件循环，不该在缓存锁里做。
	if created {
		if src := st.effectiveSrc(); src != "" {
			mediaRunLater(in, func() { st.startLoadFor(src) })
		}
	}
	return st
}

// StartMediaElementLoads 为文档中已连接的 <video>/<audio> 元素启动资源选择算法
// （HTML §4.8.8），返回本次启动的元素数。
//
// 规范要求「src 属性已设置且元素在文档中」即启动加载，而本引擎的媒体状态是
// 「首次访问媒体属性时惰性创建」（见 mediaStateFor）：用 HTML 属性写好的
// `<video src="…">` 在脚本读属性之前根本不加载——readyState 停在 HAVE_NOTHING、
// duration 为 NaN、首帧不绘制（实测静止态 rs=0，只有 play() 之后才补到 rs=4）。
// 宿主在文档装配完成后调用本函数补齐规范时机（见 webkit/webview.go 的
// loadHTMLFrom）。
//
// 幂等：已有状态（脚本访问过媒体属性，或 src 变化走过 setSrc）的元素不重复启动；
// 没有 src 的元素不启动（资源选择本该停在 NETWORK_EMPTY）。
func StartMediaElementLoads(in *jsc.Interpreter, doc *dom.Document) int {
	if in == nil || doc == nil {
		return 0
	}
	started := 0
	for _, tag := range []string{"video", "audio"} {
		for _, el := range doc.GetElementsByTagName(tag) {
			if el == nil || !el.IsConnected() {
				continue
			}
			if strings.TrimSpace(el.GetAttribute("src")) == "" {
				continue
			}
			mediaElMu.Lock()
			_, exists := mediaElCache[el]
			mediaElMu.Unlock()
			if exists {
				continue
			}
			// mediaStateFor 首次创建时会把 startLoadFor 排进事件循环。
			if mediaStateFor(in, el) != nil {
				started++
			}
		}
	}
	return started
}

// clearMediaElementCacheFor 清理媒体元素缓存（clearMediaCachesFor 调用）。
func clearMediaElementCacheFor(doc *dom.Document) {
	mediaElMu.Lock()
	for el := range mediaElCache {
		if doc == nil || el.OwnerDocument() == doc {
			delete(mediaElCache, el)
		}
	}
	mediaElMu.Unlock()
}

// ─── 任务 / 事件 ─────────────────────────────────────────

// mediaRunLater 把 fn 排入事件循环的宏任务；没有事件循环时同步执行（单测与
// 无循环宿主也能观察到结果）。媒体事件全部经此异步派发（规范：媒体事件在
// 任务中排入）。
func mediaRunLater(in *jsc.Interpreter, fn func()) {
	if in == nil {
		fn()
		return
	}
	if loop := in.EnsureEventLoop(); loop != nil {
		cb := in.NewNativeFunction("media_task", func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
			fn()
			return jsc.Undefined()
		}, 0)
		_ = loop.SetTimeout(jsc.FunctionValue(cb), 0)
		return
	}
	fn()
}

func (st *mediaElementState) fireEvent(evType string) {
	if st.el == nil {
		return
	}
	st.el.DispatchEvent(dom.NewEvent(evType, false, false, false))
}

func (st *mediaElementState) fireEventLater(evType string) {
	mediaRunLater(st.interp, func() { st.fireEvent(evType) })
}

// setEventHandler 安装/移除 on* 处理器（等价于 addEventListener/removeEventListener，
// 再次赋值替换旧处理器——与浏览器「同一事件只能有一个 IDL 处理器」一致）。
func (st *mediaElementState) setEventHandler(rt *jsc.Interpreter, evType string, v jsc.JSValue) {
	if old := st.handlers[evType]; old != nil {
		if old.l != nil {
			st.el.RemoveEventListener(evType, old.l, false)
		}
		delete(st.handlers, evType)
	}
	if v.IsFunction() {
		l := &jsListener{interp: rt, fn: v}
		st.el.AddEventListener(evType, l, false)
		st.handlers[evType] = &mediaHandler{fn: v, l: l}
	}
}

// ─── 资源选择与加载 ──────────────────────────────────────

// effectiveSrc 返回媒体元素的当前资源：src attribute 优先，其次第一个
// <source> 子元素的 src（HTML §4.8.6 资源选择算法的最小子集）。
func (st *mediaElementState) effectiveSrc() string {
	if st.el == nil {
		return ""
	}
	if s := strings.TrimSpace(st.el.GetAttribute("src")); s != "" {
		return s
	}
	for _, child := range st.el.ChildNodes() {
		e, ok := child.(*dom.Element)
		if !ok || e.LocalName() != "source" {
			continue
		}
		if s := strings.TrimSpace(e.GetAttribute("src")); s != "" {
			return s
		}
	}
	return ""
}

// syncVideoState 把「元素当前显示什么」告知渲染层（主线 A1：宿主注入帧流）。
// <video> 的画面由宿主解码，经 rendering.VideoFrameSource 通道按 (url, 时间点)
// 取回并绘制；<audio> 没有画面，不参与该通道。
//
// 何时调用：元数据就绪（给出首帧）、seek、播放时钟推进、play/pause 改变 poster
// flag。没到 HAVE_METADATA、无源、或出错时清掉记录——渲染层随之回退 poster。
func (st *mediaElementState) syncVideoState() {
	if st.el == nil || st.el.LocalName() != "video" {
		return
	}
	src := st.effectiveSrc()
	if st.hasError || strings.TrimSpace(src) == "" || st.readyState < mediaHaveMetadata {
		st.clearVideoState()
		return
	}
	rendering.SetElementVideoState(st.el, rendering.VideoElementState{
		URL:        src,
		Time:       st.currentTime,
		ShowPoster: st.showPoster,
		// Playing 决定 painter 取帧的方式（A2）：时钟在走就不能同步等解码，
		// 否则每一帧都要在渲染线程上等宿主跑一次 ffmpeg。
		Playing: st.clock && !st.paused && !st.ended,
	})
}

// clearVideoState 清除元素的显示状态（切源 / 出错 / 卸载）：渲染层不再为它向
// 宿主要帧，元素回到 poster 替代画面（HTML §4.8.8）。
func (st *mediaElementState) clearVideoState() {
	if st.el == nil {
		return
	}
	rendering.SetElementVideoState(st.el, rendering.VideoElementState{})
}

// setSrc 写入 src（IDL 属性）：反射 attribute 并重新执行资源选择算法。
func (st *mediaElementState) setSrc(src string) {
	st.el.SetAttribute("src", src)
	// <video> 的 poster 之外没有解码图，但 src 变化同样要让既有的解码缓存失效。
	if st.el.LocalName() == "video" && OnImageSrcChanged != nil {
		OnImageSrcChanged(st.el)
	}
	st.startLoadFor(src)
}

// load 实现 load()：中止当前资源、复位状态、重新选择资源。
func (st *mediaElementState) load() {
	st.clock = false
	st.currentTime = 0
	st.ended = false
	st.seeking = false
	st.duration = math.NaN()
	st.readyState = mediaHaveNothing
	st.networkState = mediaNetworkEmpty
	st.hasError = false
	st.errorObj = nil
	// emptied 与随后的 loadstart 同任务顺序派发（见 startLoadFor 的说明）。
	mediaRunLater(st.interp, func() {
		st.fireEvent("emptied")
		st.startLoadFor(st.effectiveSrc())
	})
}

// startLoadFor 启动指定资源的加载流程（loadstart → 元数据/错误）。
func (st *mediaElementState) startLoadFor(src string) {
	if st.el == nil {
		return
	}
	st.loadedSrc = src
	if strings.TrimSpace(src) == "" {
		st.networkState = mediaNetworkEmpty
		st.readyState = mediaHaveNothing
		return
	}
	st.networkState = mediaNetworkLoading
	st.readyState = mediaHaveNothing
	st.duration = math.NaN()
	st.videoW, st.videoH = 0, 0
	st.currentTime = 0
	st.ended = false
	st.hasError = false
	st.errorObj = nil
	// 新的资源还没有画面：清掉旧状态（否则切换 src 后会继续显示上一个视频的帧），
	// 并复位 show poster flag——带 poster 的 <video> 加载后先显示 poster。
	st.showPoster = true
	st.clearVideoState()
	// ★ 整个流程在同一个任务里按序同步派发：EventLoop 对相同 deadline 的任务
	// 不保证 FIFO（堆序），把 loadstart 与 loadedmetadata 拆成两个任务会让事件
	// 顺序抖动（实测出现过 loadedmetadata 早于 loadstart）。
	mediaRunLater(st.interp, func() {
		st.fireEvent("loadstart")
		st.finishLoad()
	})
}

// finishLoad 完成资源选择：可取得元数据的资源进入 HAVE_METADATA 并派发
// loadedmetadata；确定取不到的（http/https：本引擎无网络栈，宿主也没接
// resolver）派发 error——脚本的失败分支能走到，而不是永远等 canplay。
func (st *mediaElementState) finishLoad() {
	if st.el == nil || st.loadedSrc != st.effectiveSrc() {
		return // 资源已变化：这次加载作废
	}
	src := st.loadedSrc
	if MediaMetadataResolver != nil {
		if meta, ok := MediaMetadataResolver(src); ok {
			st.applyMetadata(meta)
			return
		}
	}
	if isUnreachableMediaSrc(src) {
		st.networkState = mediaNetworkNoSource
		st.readyState = mediaHaveNothing
		st.setError(mediaErrSrcNotSupported, "The media resource could not be loaded")
		st.clearVideoState()
		st.fireEvent("error")
		return
	}
	// 本地/相对资源：本层不解码（宿主可经 MediaMetadataResolver 补齐时长）。
	st.duration = math.NaN()
	st.networkState = mediaNetworkIdle
	st.readyState = mediaHaveMetadata
	st.fireEvent("durationchange")
	st.fireEvent("loadedmetadata")
	// HAVE_METADATA 时 currentTime=0：让宿主给出首帧，脚本立即读到的画面与
	// videoWidth/Height 一致（时长未知只影响播放时钟，不影响首帧可取）。
	st.syncVideoState()
}

// applyMetadata 应用宿主探测到的元数据。时长已知时推进到 HAVE_ENOUGH_DATA 并
// 派发 loadeddata/canplay/canplaythrough；时长未知（NaN）时停在 HAVE_METADATA
// ——尺寸等元数据是可信的，但播放时钟没有依据，派发 canplay 会误导脚本以为
// 可以开始播放。视频尺寸在此生效（videoWidth/videoHeight）。
func (st *mediaElementState) applyMetadata(meta MediaMetadata) {
	st.duration = meta.Duration
	if meta.Width > 0 {
		st.videoW = meta.Width
	}
	if meta.Height > 0 {
		st.videoH = meta.Height
	}
	if meta.FPS > 0 {
		st.fps = meta.FPS
	}
	st.networkState = mediaNetworkIdle
	st.readyState = mediaHaveMetadata
	st.fireEvent("durationchange")
	st.fireEvent("loadedmetadata")
	if math.IsNaN(st.duration) {
		st.syncVideoState()
		return
	}
	st.readyState = mediaHaveEnoughData
	st.fireEvent("loadeddata")
	st.fireEvent("canplay")
	st.fireEvent("canplaythrough")
	st.syncVideoState()
}

// isUnreachableMediaSrc 判定本引擎肯定取不到的源（http/https 需要网络栈）。
func isUnreachableMediaSrc(src string) bool {
	l := strings.ToLower(strings.TrimSpace(src))
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://")
}

func (st *mediaElementState) setError(code int, msg string) {
	st.hasError = true
	o := jsc.NewObject(mediaProtoFromGlobal(st.interp, "MediaError"))
	st.errorObj = o
	st.errorObj.SetClassName("MediaError")
	st.errorObj.Set("code", jsc.NumberValue(float64(code)))
	st.errorObj.Set("message", jsc.StringValue(msg))
}

// mediaErrorFor 把 code 映射为规范的 MEDIA_ERR_* 常量名（错误对象的 message）。
func mediaErrorName(code int) string {
	switch code {
	case mediaErrAborted:
		return "MEDIA_ERR_ABORTED"
	case mediaErrNetwork:
		return "MEDIA_ERR_NETWORK"
	case mediaErrDecode:
		return "MEDIA_ERR_DECODE"
	case mediaErrSrcNotSupported:
		return "MEDIA_ERR_SRC_NOT_SUPPORTED"
	}
	return "MEDIA_ERR_UNKNOWN"
}

// ─── 播放 / 时钟 ─────────────────────────────────────────

// play 实现 play()：返回 Promise（规范语义——脚本会 await 它并在 catch 里
// 处理自动播放被拒）。无可用资源时 reject NotSupportedError。
func (st *mediaElementState) play() jsc.JSValue {
	in := st.interp
	if st.effectiveSrc() == "" {
		return in.RejectPromise(newDOMExceptionValue(in, "NotSupportedError",
			"The element has no supported sources."))
	}
	if st.hasError {
		return in.RejectPromise(newDOMExceptionValue(in, "NotSupportedError",
			"The element has no supported sources."))
	}
	if !st.paused {
		return in.ResolvePromise(jsc.Undefined())
	}
	st.paused = false
	st.ended = false
	if st.readyState == mediaHaveNothing && st.networkState != mediaNetworkLoading {
		st.startLoadFor(st.effectiveSrc())
	}
	// 开始播放：show poster flag 清除（放在 startLoadFor 之后——加载算法会把
	// flag 复位，先清会被它盖掉）。此后 <video> 显示的是真实帧，不是 poster。
	st.showPoster = false
	// ★ 先起时钟再同步显示状态：Playing 由「时钟是否在走」推出，反过来写会让
	// play() 后的头 250ms（首个 tick 之前）仍然声称 Playing=false——painter
	// 就会去同步取帧，白白在渲染线程上等一次解码。
	st.startClock()
	st.syncVideoState()
	// play/playing 同任务顺序派发（同上：跨任务的顺序不可依赖）。
	mediaRunLater(in, func() {
		st.fireEvent("play")
		st.fireEvent("playing")
	})
	return in.ResolvePromise(jsc.Undefined())
}

// pause 实现 pause()：已暂停或无资源时是 no-op（不派发重复事件）。
func (st *mediaElementState) pause() {
	if st.paused || st.effectiveSrc() == "" {
		return
	}
	st.paused = true
	st.clock = false
	// 暂停要上报状态（Playing 随之转 false）：此后绘制会在静止态同步取当前
	// 时刻的帧——暂停瞬间看到的画面必须精确，而不是继续异步凑合。
	st.syncVideoState()
	mediaRunLater(st.interp, func() { st.fireEvent("pause") })
}

func (st *mediaElementState) startClock() {
	if st.clock {
		return
	}
	st.clock = true
	st.scheduleTick()
}

func (st *mediaElementState) scheduleTick() {
	if st.interp == nil {
		return
	}
	loop := st.interp.EnsureEventLoop()
	if loop == nil {
		return
	}
	cb := st.interp.NewNativeFunction("media_timeupdate", func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
		st.tick()
		return jsc.Undefined()
	}, 0)
	_ = loop.SetTimeout(jsc.FunctionValue(cb), mediaTimeUpdateMs)
}

// tick 是播放时钟的一步：时长已知时推进 currentTime 并按需派发 timeupdate/
// ended；时长未知（NaN）时不推进进度，但保持时钟（pause/ended 语义仍正确）。
func (st *mediaElementState) tick() {
	if !st.clock || st.paused || st.ended {
		st.clock = false
		return
	}
	if math.IsNaN(st.duration) || st.duration <= 0 {
		st.scheduleTick()
		return
	}
	st.currentTime += float64(mediaTimeUpdateMs) / 1000 * st.playbackRate
	if st.currentTime >= st.duration {
		st.currentTime = st.duration
		st.ended = true
		st.paused = true
		st.clock = false
		st.syncVideoState()
		st.maybePresentVideoFrame() // 末尾那一帧也是「新呈现的一帧」
		st.fireEvent("timeupdate")
		st.fireEvent("ended")
		return
	}
	st.syncVideoState()
	st.maybePresentVideoFrame()
	st.prefetchNextFrame()
	st.fireEvent("timeupdate")
	st.scheduleTick()
}

// prefetchNextFrame 请求宿主**异步**预取「下一次时间更新会显示的帧」（A2）：
// 播放中渲染线程不等解码，靠的就是帧总比绘制早一步到（app 侧的 worker 池跑
// 解码器，见 MediaProbe.FramePump）。
//
// 末尾把请求收敛到 duration：越界时刻没有帧，宿主会把 duration 对应到最后一帧
// （见 app/mediaprobe.go 的 clampFrameTime），所以播放结束时那一帧也是预取来的。
func (st *mediaElementState) prefetchNextFrame() {
	if st.el == nil || st.el.LocalName() != "video" {
		return
	}
	src := st.effectiveSrc()
	if strings.TrimSpace(src) == "" {
		return
	}
	// 步长：知道帧率就按**帧**推进（预取到的就是接下来要显示的那几帧本身），否则
	// 退回「一个时钟步长」。窗口宽度见 mediaPrefetchFrames。
	step := float64(mediaTimeUpdateMs) / 1000 * st.playbackRate
	count := 1
	if st.fps > 0 {
		step = st.playbackRate / st.fps
		count = mediaPrefetchFrames
	}
	for i := 1; i <= count; i++ {
		next := st.currentTime + step*float64(i)
		if !math.IsNaN(st.duration) && st.duration > 0 && next > st.duration {
			next = st.duration
		}
		rendering.PrefetchVideoFrame(src, next)
	}
}

// ─── requestVideoFrameCallback（A2-③）────────────────────

// requestVideoFrameCallback 实现 video.requestVideoFrameCallback(cb)：注册「新一帧
// 呈现」回调并返回句柄（规范里是 callback id，非 0；参数不可调用时返回 0 = 无效句柄）。
//
// 时机：本引擎的帧由宿主按需注入，没有真实合成器，因此「呈现」定义为**播放时钟推进
// 到新的帧号**（tick 里判定，见 maybePresentVideoFrame）。回调触发一次后即失效——
// 与规范一致（要持续观察就得在回调里再注册一次，这也是 rVFC 的常规用法）。
func (st *mediaElementState) requestVideoFrameCallback(args []jsc.JSValue) int {
	if len(args) == 0 || !args[0].IsCallable() {
		return 0
	}
	if st.rvfc == nil {
		st.rvfc = map[int]jsc.JSValue{}
	}
	st.rvfcNext++
	handle := st.rvfcNext
	st.rvfc[handle] = args[0]
	return handle
}

// cancelVideoFrameCallback 取消一个尚未触发的帧呈现回调（重复取消是 no-op，与
// addEventListener 的 handle 语义一致：失效的 handle 不该报错）。
func (st *mediaElementState) cancelVideoFrameCallback(handle int) {
	delete(st.rvfc, handle)
}

// frameNo 返回 currentTime 所属的帧号（HTML 里「该帧的媒体时间戳所属帧」）。
// 帧率未知时退化为毫秒粒度：粒度只影响回调频率，不会丢帧。
func (st *mediaElementState) frameNo() int64 {
	if st.fps > 0 {
		return int64(math.Floor(st.currentTime*st.fps + 1e-6))
	}
	return int64(st.currentTime * 1000)
}

// maybePresentVideoFrame 在时钟推进后判定「是否呈现了新一帧」并触发回调（A2-③）。
// 帧号没变就什么都不做——这正是 rVFC 与 timeupdate 的区别：timeupdate 按固定频率报
// 时钟，rVFC 只在**真的有新帧**时回调。
func (st *mediaElementState) maybePresentVideoFrame() {
	// ★ 帧号跟踪**不能**受「当前有没有注册回调」影响：否则注册回调那一刻的
	// lastFrameNo 会停留在很久以前的帧号上，注册后第一次时钟推进就误报一次「新帧」
	// （规范要的是「下一次新帧呈现」，不是「立刻回调」）。
	n := st.frameNo()
	changed := n != st.lastFrameNo
	st.lastFrameNo = n
	if !changed || len(st.rvfc) == 0 {
		return
	}
	st.fireVideoFrameCallbacks()
}

// fireVideoFrameCallbacks 调用已注册的帧呈现回调，并按规范填 VideoFrameCallbackMetadata
// （presentedFrames 从 1 开始、mediaTime 是该帧的**起点**而不是 currentTime）。
//
// 已知差异（记录在 docs/TECH_DEBT.md）：回调里的 `this` 传 undefined 而不是元素本身；
// 没有真实合成器，presentationTime/expectedDisplayTime 是宿主 wall clock 的近似值
// （单调、可用于测帧间隔），processingDuration 恒为 0。
func (st *mediaElementState) fireVideoFrameCallbacks() {
	handlers := st.rvfc
	if len(handlers) == 0 {
		return
	}
	st.rvfc = map[int]jsc.JSValue{} // 一次性：触发过的句柄即失效
	st.presented++

	now := float64(time.Now().UnixNano()) / 1e6
	mediaTime := st.currentTime
	if st.fps > 0 {
		mediaTime = math.Floor(st.currentTime*st.fps+1e-6) / st.fps
	}
	interp := st.interp
	presented := st.presented
	for _, fn := range handlers {
		f := fn
		mediaRunLater(interp, func() {
			if interp == nil || !f.IsFunction() {
				return
			}
			meta := jsc.NewObject(nil)
			meta.Set("presentationTime", jsc.NumberValue(now))
			meta.Set("expectedDisplayTime", jsc.NumberValue(now+16.7)) // 下一个 60Hz 帧
			meta.Set("width", jsc.NumberValue(float64(st.videoW)))
			meta.Set("height", jsc.NumberValue(float64(st.videoH)))
			meta.Set("mediaTime", jsc.NumberValue(mediaTime))
			meta.Set("presentedFrames", jsc.NumberValue(float64(presented)))
			meta.Set("processingDuration", jsc.NumberValue(0))
			_, _ = interp.Call(f, jsc.Undefined(), []jsc.JSValue{
				jsc.NumberValue(now), jsc.ObjectValue(meta),
			})
		})
	}
}

// setCurrentTime 实现 currentTime 的 setter（含 seeking/seeked 事件）。
func (st *mediaElementState) setCurrentTime(t float64) {
	if math.IsNaN(t) {
		return
	}
	if t < 0 {
		t = 0
	}
	if !math.IsNaN(st.duration) && st.duration > 0 && t > st.duration {
		t = st.duration
	}
	if t == st.currentTime {
		return
	}
	st.currentTime = t
	st.seeking = true
	if t > 0 {
		// show poster flag 在 currentTime 被设为非 0 时清除（HTML §4.8.8）：
		// seek 到中间就该看到画面，而不是继续看 poster。
		st.showPoster = false
	}
	st.syncVideoState()
	mediaRunLater(st.interp, func() {
		st.fireEvent("seeking")
		st.seeking = false
		st.fireEvent("seeked")
		// seek 的落点是一帧新的画面 → 也是一次「呈现」（规范里 rVFC 在 seek 后同样
		// 会触发；暂停态没有 tick，不在这里报就永远不会报）。
		st.maybePresentVideoFrame()
	})
}

// ─── TimeRanges ──────────────────────────────────────────

// mediaProtoFromGlobal 取全局构造器 ctorName 的 prototype，让本包创建的对象
// 通过 instanceof 检查（找不到时返回 nil → 对象走普通 Object 原型）。每次从
// 全局查而不是缓存到包级变量：原型是 per-interpreter 的，缓存会造成跨 runtime
// 共享 goja 对象。
func mediaProtoFromGlobal(in *jsc.Interpreter, ctorName string) *jsc.JSObject {
	if in == nil {
		return nil
	}
	g := in.GlobalObject()
	if g == nil {
		return nil
	}
	ctor := g.GetStr(ctorName)
	if ctor.IsUndefined() {
		return nil
	}
	o := ctor.AsObject()
	if o == nil {
		return nil
	}
	p := o.GetStr("prototype")
	if p.IsUndefined() {
		return nil
	}
	return p.AsObject()
}

// newTimeRangesValue 构造 TimeRanges 对象（buffered/played/seekable）。
func newTimeRangesValue(proto *jsc.JSObject, ranges [][2]float64) jsc.JSValue {
	o := jsc.NewObject(proto)
	o.SetClassName("TimeRanges")
	o.Set("length", jsc.NumberValue(float64(len(ranges))))
	idx := func(args []jsc.JSValue) int {
		if len(args) == 0 {
			return -1
		}
		return int(args[0].ToNumber())
	}
	o.Set("start", jsc.FunctionValue(jsc.NewNativeFunction("start",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			i := idx(args)
			if i < 0 || i >= len(ranges) {
				return jsc.NumberValue(math.NaN()) // 规范是 IndexSizeError
			}
			return jsc.NumberValue(ranges[i][0])
		}, 1)))
	o.Set("end", jsc.FunctionValue(jsc.NewNativeFunction("end",
		func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
			i := idx(args)
			if i < 0 || i >= len(ranges) {
				return jsc.NumberValue(math.NaN())
			}
			return jsc.NumberValue(ranges[i][1])
		}, 1)))
	return jsc.ObjectValue(o)
}

// ranges 返回该元素的 TimeRanges（时长已知时为 [0,duration] 单段，否则空）。
func (st *mediaElementState) ranges() jsc.JSValue {
	proto := mediaProtoFromGlobal(st.interp, "TimeRanges")
	if math.IsNaN(st.duration) || st.duration <= 0 {
		return newTimeRangesValue(proto, nil)
	}
	return newTimeRangesValue(proto, [][2]float64{{0, st.duration}})
}

// newDOMExceptionValue 构造带 name/message 的 DOMException 等价对象（本引擎
// 没有 DOMException 构造器；脚本只依赖 name/message 两个字段）。
func newDOMExceptionValue(in *jsc.Interpreter, name, msg string) jsc.JSValue {
	o := jsc.NewObject(nil)
	o.SetClassName("DOMException")
	o.Set("name", jsc.StringValue(name))
	o.Set("message", jsc.StringValue(msg))
	return jsc.ObjectValue(o)
}

// ─── canPlayType ─────────────────────────────────────────

// canPlayType 按容器类型给「可能支持」（HTML §4.8.6：本层不探测具体编解码器，
// 未知容器返回空串）。脚本据此在多 <source> 中选择——把不支持的容器诚实报为
// 空串（如 HLS 的 application/vnd.apple.mpegurl），比一律 "maybe" 更有用。
func canPlayType(t string) string {
	l := strings.ToLower(strings.TrimSpace(t))
	if l == "" {
		return ""
	}
	if i := strings.IndexByte(l, ';'); i >= 0 {
		l = strings.TrimSpace(l[:i])
	}
	switch l {
	case "video/mp4", "audio/mp4", "audio/mpeg", "audio/mp3",
		"video/webm", "audio/webm",
		"video/ogg", "audio/ogg", "application/ogg",
		"audio/wav", "audio/x-wav", "audio/wave",
		"video/quicktime", "video/x-matroska":
		return "maybe"
	}
	return ""
}

// ─── 属性安装 ────────────────────────────────────────────

// mediaElementPropNames 是 HTMLMediaElement 的 IDL 属性/方法名。
var mediaElementPropNames = map[string]bool{
	"src": true, "currentSrc": true, "crossOrigin": true, "preload": true,
	"autoplay": true, "loop": true, "controls": true, "defaultMuted": true,
	"muted": true, "volume": true, "playbackRate": true, "defaultPlaybackRate": true,
	"currentTime": true, "duration": true, "paused": true, "ended": true,
	"seeking": true, "readyState": true, "networkState": true, "error": true,
	"buffered": true, "played": true, "seekable": true,
	"videoWidth": true, "videoHeight": true,
	"canPlayType": true, "load": true, "play": true, "pause": true,
	"fastSeek": true, "addTextTrack": true, "captureStream": true,
	// A2-③：帧呈现回调（HTML §4.8.7 的 requestVideoFrameCallback）。
	"requestVideoFrameCallback": true, "cancelVideoFrameCallback": true,
}

// mediaEventNames 是媒体元素会派发的事件类型（on* 处理器属性按此集合识别）。
var mediaEventNames = []string{
	"loadstart", "progress", "suspend", "abort", "error", "emptied", "stalled",
	"loadedmetadata", "loadeddata", "canplay", "canplaythrough", "playing",
	"waiting", "seeking", "seeked", "ended", "durationchange", "timeupdate",
	"play", "pause", "ratechange", "resize", "volumechange",
}

var mediaEventHandlerProps = map[string]bool{}

// mediaElementPropList 是媒体属性名的稳定顺序列表（Object.keys 用；map 迭代
// 无序会让 Keys 的结果抖动）。
var mediaElementPropList []string

func init() {
	for _, n := range mediaEventNames {
		mediaEventHandlerProps["on"+n] = true
	}
	for k := range mediaElementPropNames {
		mediaElementPropList = append(mediaElementPropList, k)
	}
	for k := range mediaEventHandlerProps {
		mediaElementPropList = append(mediaElementPropList, k)
	}
	sort.Strings(mediaElementPropList)
}

// isMediaElementTag 判定媒体元素标签（<video>/<audio>）。
func isMediaElementTag(tag string) bool {
	return tag == "video" || tag == "audio"
}

// hasMediaElementProp 判定「元素是媒体元素、且 key 是它的属性」。元素包装器的
// Has/Live/Keys 用它按标签判定——不把媒体属性名塞进全局 elemKnownProps，否则
// `'paused' in div` 会变成 true、`Object.keys(div)` 会多出 30 多个假属性。
func hasMediaElementProp(el *dom.Element, key string) bool {
	if el == nil {
		return false
	}
	switch el.LocalName() {
	case "video", "audio":
		return isMediaElementProp(key)
	}
	return false
}

// isMediaElementProp 判定属性名是否属于 HTMLMediaElement（含 on* 处理器）。
func isMediaElementProp(key string) bool {
	return mediaElementPropNames[key] || mediaEventHandlerProps[key]
}

// installMediaElementProperty 物化 <video>/<audio> 的一个媒体属性。
// 返回 ok=false 表示该名字不属于媒体模型（调用方继续按通用元素属性处理）。
func installMediaElementProperty(rt *jsc.Interpreter, el *dom.Element, key string) (jsc.JSValue, *elemAccessor, bool) {
	if !isMediaElementProp(key) {
		return jsc.JSValue{}, nil, false
	}
	st := mediaStateFor(rt, el)
	if st == nil {
		return jsc.JSValue{}, nil, false
	}

	// on* 事件处理器属性。
	if mediaEventHandlerProps[key] {
		ev := key[2:]
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue {
				if h := st.handlers[ev]; h != nil {
					return h.fn
				}
				return jsc.Null()
			},
			set: func(v jsc.JSValue) { st.setEventHandler(rt, ev, v) },
		}, true
	}

	boolAttr := func(name string) (jsc.JSValue, *elemAccessor, bool) {
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.BooleanValue(el.HasAttribute(name)) },
			set: func(v jsc.JSValue) {
				if v.ToBoolean() {
					el.SetAttribute(name, "")
				} else {
					el.RemoveAttribute(name)
				}
			},
		}, true
	}

	switch key {
	case "src":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.StringValue(el.GetAttribute("src")) },
			set: func(v jsc.JSValue) { st.setSrc(v.ToString()) },
		}, true
	case "currentSrc":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			if st.loadedSrc != "" {
				return jsc.StringValue(st.loadedSrc)
			}
			return jsc.StringValue(st.effectiveSrc())
		}}, true
	case "crossOrigin":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue {
				if !el.HasAttribute("crossorigin") {
					return jsc.Null()
				}
				return jsc.StringValue(el.GetAttribute("crossorigin"))
			},
			set: func(v jsc.JSValue) {
				if v.IsNull() || v.IsUndefined() {
					el.RemoveAttribute("crossorigin")
					return
				}
				el.SetAttribute("crossorigin", v.ToString())
			},
		}, true
	case "preload":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue {
				if !el.HasAttribute("preload") {
					return jsc.StringValue("metadata") // 规范缺省值
				}
				return jsc.StringValue(el.GetAttribute("preload"))
			},
			set: func(v jsc.JSValue) { el.SetAttribute("preload", v.ToString()) },
		}, true
	case "autoplay":
		return boolAttr("autoplay")
	case "loop":
		v, acc, ok := boolAttr("loop")
		return v, acc, ok
	case "controls":
		return boolAttr("controls")
	case "defaultMuted":
		return boolAttr("muted")
	case "muted":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.BooleanValue(st.muted) },
			set: func(v jsc.JSValue) {
				next := v.ToBoolean()
				if next == st.muted {
					return
				}
				st.muted = next
				st.fireEvent("volumechange")
			},
		}, true
	case "volume":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue {
				if st.muted {
					return jsc.NumberValue(0)
				}
				return jsc.NumberValue(st.volume)
			},
			set: func(v jsc.JSValue) {
				vol := v.ToNumber()
				if math.IsNaN(vol) {
					return
				}
				// 规范：越界抛 IndexSizeError；这里夹取（脚本几乎不会依赖抛错）。
				if vol < 0 {
					vol = 0
				}
				if vol > 1 {
					vol = 1
				}
				if vol == st.volume {
					return
				}
				st.volume = vol
				st.fireEvent("volumechange")
			},
		}, true
	case "playbackRate":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.NumberValue(st.playbackRate) },
			set: func(v jsc.JSValue) {
				r := v.ToNumber()
				if math.IsNaN(r) || r <= 0 {
					return
				}
				if r == st.playbackRate {
					return
				}
				st.playbackRate = r
				st.fireEvent("ratechange")
			},
		}, true
	case "defaultPlaybackRate":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.NumberValue(st.defaultRate) },
			set: func(v jsc.JSValue) {
				r := v.ToNumber()
				if math.IsNaN(r) || r <= 0 {
					return
				}
				// 规范：defaultPlaybackRate 是「资源开始播放时的初始速率」，
				// 改它不会改变当前 playbackRate，也不派发 ratechange。
				st.defaultRate = r
			},
		}, true
	case "currentTime":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.NumberValue(st.currentTime) },
			set: func(v jsc.JSValue) { st.setCurrentTime(v.ToNumber()) },
		}, true
	case "duration":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.NumberValue(st.duration) },
		}, true
	case "paused":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.BooleanValue(st.paused) },
		}, true
	case "ended":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.BooleanValue(st.ended) },
		}, true
	case "seeking":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.BooleanValue(st.seeking) },
		}, true
	case "readyState":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.NumberValue(float64(st.readyState)) },
		}, true
	case "networkState":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.NumberValue(float64(st.networkState)) },
		}, true
	case "error":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue {
			if st.hasError && st.errorObj != nil {
				return jsc.ObjectValue(st.errorObj)
			}
			return jsc.Null()
		}}, true
	case "buffered", "played", "seekable":
		return jsc.JSValue{}, &elemAccessor{get: func() jsc.JSValue { return st.ranges() }}, true
	case "videoWidth":
		// 画面尺寸来自宿主探测的元数据（本层不持有位图）；未知时为 0。
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.NumberValue(float64(st.videoW)) },
		}, true
	case "videoHeight":
		return jsc.JSValue{}, &elemAccessor{
			get: func() jsc.JSValue { return jsc.NumberValue(float64(st.videoH)) },
		}, true
	case "canPlayType":
		return funcVal(rt.NewNativeFunction("canPlayType",
			func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				if len(args) == 0 {
					return jsc.StringValue("")
				}
				return jsc.StringValue(canPlayType(args[0].ToString()))
			}, 1)), nil, true
	case "load":
		return funcVal(rt.NewNativeFunction("load",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				st.load()
				return jsc.Undefined()
			}, 0)), nil, true
	case "play":
		return funcVal(rt.NewNativeFunction("play",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				return st.play()
			}, 0)), nil, true
	case "pause":
		return funcVal(rt.NewNativeFunction("pause",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				st.pause()
				return jsc.Undefined()
			}, 0)), nil, true
	case "fastSeek":
		return funcVal(rt.NewNativeFunction("fastSeek",
			func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				if len(args) > 0 {
					st.setCurrentTime(args[0].ToNumber())
				}
				return jsc.Undefined()
			}, 1)), nil, true
	case "addTextTrack":
		return funcVal(rt.NewNativeFunction("addTextTrack",
			func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				return st.addTextTrack(args)
			}, 3)), nil, true
	case "captureStream":
		// 没有 MediaStream 实现：返回 null（脚本的 `if (stream)` 分支走空，
		// 而不是拿到一个再也用不了的对象）。
		return funcVal(rt.NewNativeFunction("captureStream",
			func(_ *jsc.Interpreter, _ jsc.JSValue, _ []jsc.JSValue) jsc.JSValue {
				return jsc.Null()
			}, 0)), nil, true
	case "requestVideoFrameCallback":
		return funcVal(rt.NewNativeFunction("requestVideoFrameCallback",
			func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				return jsc.NumberValue(float64(st.requestVideoFrameCallback(args)))
			}, 1)), nil, true
	case "cancelVideoFrameCallback":
		return funcVal(rt.NewNativeFunction("cancelVideoFrameCallback",
			func(_ *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
				if len(args) > 0 {
					st.cancelVideoFrameCallback(int(args[0].ToNumber()))
				}
				return jsc.Undefined()
			}, 1)), nil, true
	}
	return jsc.JSValue{}, nil, false
}

// addTextTrack 实现 addTextTrack(kind, label, language)：插入一个无 src 的
// <track> 子元素并返回其 TextTrack。这样它自动出现在 textTracks（live 集合）
// 里，且「video.textTracks[i] 与 <track>.track 同一对象」的标识语义继续成立。
func (st *mediaElementState) addTextTrack(args []jsc.JSValue) jsc.JSValue {
	doc := st.el.OwnerDocument()
	if doc == nil {
		return jsc.Null()
	}
	kind := "subtitles"
	if len(args) > 0 && args[0].ToString() != "" {
		kind = args[0].ToString()
	}
	trackEl := doc.CreateElement("track")
	trackEl.SetAttribute("kind", kind)
	if len(args) > 1 && args[1].ToString() != "" {
		trackEl.SetAttribute("label", args[1].ToString())
	}
	if len(args) > 2 && args[2].ToString() != "" {
		trackEl.SetAttribute("srclang", args[2].ToString())
	}
	st.el.AppendChild(trackEl)
	if OnNodeInserted != nil && st.el.IsConnected() {
		OnNodeInserted(trackEl)
	}
	obj := textTrackForElement(st.interp, trackEl)
	if obj == nil {
		return jsc.Null()
	}
	return jsc.ObjectValue(obj)
}

// ─── 原型与构造器 ────────────────────────────────────────

var (
	mediaElProtoMu sync.Mutex
	domMediaProto  *jsc.JSObject // HTMLMediaElement.prototype
	domVideoProto  *jsc.JSObject // HTMLVideoElement.prototype
	domAudioProto  *jsc.JSObject // HTMLAudioElement.prototype
)

// registerMediaElementTypes 注册 HTMLMediaElement / HTMLVideoElement /
// HTMLAudioElement / MediaError / TimeRanges 构造器。原型链在 dom.go 的
// 链构建处接到 HTMLElement.prototype 上（video/audio 元素的包装器直接使用
// video/audio 原型，因此 `video instanceof HTMLMediaElement` 成立）。
func registerMediaElementTypes(rt *jsc.Interpreter, g *jsc.JSObject) {
	newElCtor := func(name string, parentSetter func(proto *jsc.JSObject)) *jsc.JSObject {
		ctor := rt.NewConstructor(name, func(in *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) *jsc.JSObject {
			o := this.AsObject()
			if o == nil {
				o = jsc.NewObject(nil)
			}
			return o
		})
		proto := jsc.FunctionValue(ctor).AsObject().GetStr("prototype").AsObject()
		g.Set(name, jsc.FunctionValue(ctor))
		parentSetter(proto)
		return proto
	}
	mediaProto := newElCtor("HTMLMediaElement", func(*jsc.JSObject) {})
	domMediaProto = mediaProto
	domVideoProto = newElCtor("HTMLVideoElement", func(p *jsc.JSObject) {
		p.Set("__proto__", jsc.ObjectValue(mediaProto))
	})
	domAudioProto = newElCtor("HTMLAudioElement", func(p *jsc.JSObject) {
		p.Set("__proto__", jsc.ObjectValue(mediaProto))
	})

	// MediaError：常量 + 可构造对象（脚本可能 `new MediaError()`）。
	errCtor := rt.NewConstructor("MediaError", func(in *jsc.Interpreter, _ jsc.JSValue, args []jsc.JSValue) *jsc.JSObject {
		o := jsc.NewObject(nil)
		o.SetClassName("MediaError")
		code := mediaErrAborted
		if len(args) > 0 {
			code = int(args[0].ToNumber())
		}
		o.Set("code", jsc.NumberValue(float64(code)))
		o.Set("message", jsc.StringValue(mediaErrorName(code)))
		return o
	})
	errProto := jsc.FunctionValue(errCtor).AsObject().GetStr("prototype").AsObject()
	errProto.Set("MEDIA_ERR_ABORTED", jsc.NumberValue(mediaErrAborted))
	errProto.Set("MEDIA_ERR_NETWORK", jsc.NumberValue(mediaErrNetwork))
	errProto.Set("MEDIA_ERR_DECODE", jsc.NumberValue(mediaErrDecode))
	errProto.Set("MEDIA_ERR_SRC_NOT_SUPPORTED", jsc.NumberValue(mediaErrSrcNotSupported))
	g.Set("MediaError", jsc.FunctionValue(errCtor))

	// TimeRanges：不可 new，仅原型链（instanceof 成立）。
	trCtor := rt.NewConstructor("TimeRanges", func(_ *jsc.Interpreter, this jsc.JSValue, _ []jsc.JSValue) *jsc.JSObject {
		o := this.AsObject()
		if o == nil {
			o = jsc.NewObject(nil)
		}
		return o
	})
	g.Set("TimeRanges", jsc.FunctionValue(trCtor))
}

// mediaElementPrototypeFor 返回元素包装器应使用的原型（非媒体元素返回 nil）。
func mediaElementPrototypeFor(el *dom.Element) *jsc.JSObject {
	if el == nil {
		return nil
	}
	mediaElProtoMu.Lock()
	defer mediaElProtoMu.Unlock()
	switch el.LocalName() {
	case "video":
		return domVideoProto
	case "audio":
		return domAudioProto
	}
	return nil
}
