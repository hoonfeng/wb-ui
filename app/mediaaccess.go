package app

// 媒体资源门禁（宿主侧）：把 `<video>/<audio>` 的 src 判定接到与引擎
// `<img>`/`<script>`/`<link>` **同一条**资源策略上（webkit.WebView.MediaResourceAllowed）。
//
// 为什么必须有这一环：媒体的解码全在宿主（ffmpeg 探测元数据 / 抽帧 / 解 PCM），
// 三条链路都从 mediaSrcToPath 拿本地路径。判定若不落在这一步，
// Toolkit+DenyExternal 下媒体照样被读盘解码——与 `<img>` 同来源的 L0 形成实打实
// 的不一致（根因与修法见 docs/media-format-verification-plan.md §9.9）。
//
// 分工（「一份判定、两处执行」）：
//   - 判定逻辑：webkit.MediaResourceAllowed（与 loadExternalResource 同源——
//     resolver → data: → 策略门禁；本层不另立任何媒体专属规则）；
//   - 装配与执行：本文件（宿主侧 mediaSrcToPath 的前置门禁）+
//     bindings.MediaSrcAllowed（引擎侧资源选择的前置判定）。
//
// ★ 安全默认：**未装配判定 = 一律拒绝**。宿主必须显式装配
//（InstallMediaMetadataResolver / InstallMediaAudio 负责接线，走 WebView 的当前
// 策略）；需要读本地媒体的宿主还须显式声明策略（SetResourcePolicy(AllowHostResolved)
// 或 AllowAll）——`data:` 与 resolver 命中的引用不受影响。

import (
	"strings"
	"sync"

	"wb-ui/webkit"
)

// mediaGateCacheLimit 是判定缓存上限：超过即整体清空（媒体引用以每元素少数几条
// 为主，简单策略足够，不做 LRU）。
const mediaGateCacheLimit = 512

var (
	mediaGateMu     sync.RWMutex
	mediaGateFn     func(ref string) bool
	mediaGatePolicy func() webkit.ResourcePolicy
	mediaGateCache  map[mediaGateKey]bool
)

// mediaGateKey 是判定缓存的键：**引用 + 当时的资源策略**。策略进键，宿主中途
// SetResourcePolicy 切档就不会读到旧档位的判定（TC-M-907 的切换语义在媒体链路上
// 同样成立）。
//
// 缓存的目的只有一个：判定要问宿主 ResourceResolver（AllowHostResolved 档正是靠
// resolver 命中放行），而 resolver 接口是「返回内容」——探针的 hostSamplesResolver
// 会真读文件，抽帧路径每帧问一次就等于每帧读一遍整个媒体文件。缓存把这份开销压成
// 「每个引用每种策略一次」。
//
// ★ 失效契约：SetMediaRefGate（装配）时清空；策略变化自动换键；宿主**运行中替换
// ResourceResolver** 后须调用 ClearMediaRefCache()（本层感知不到 resolver 替换）。
type mediaGateKey struct {
	policy string
	ref    string
}

// SetMediaRefGate 装配媒体引用的访问判定（传 nil 清除 → 恢复「一律拒绝」）。
// policy 是取当前资源策略的闭包（用于缓存键；可为 nil）。
//
// ★ 只应由本包的 InstallMediaMetadataResolver / InstallMediaAudio 调用——判定
// 逻辑在 webkit（MediaResourceAllowed），这一层只做「问一次 + 记住」。
func SetMediaRefGate(fn func(ref string) bool, policy func() webkit.ResourcePolicy) {
	mediaGateMu.Lock()
	defer mediaGateMu.Unlock()
	mediaGateFn = fn
	mediaGatePolicy = policy
	mediaGateCache = nil // 换判定 = 旧判定全部作废
}

// ClearMediaRefCache 清空媒体判定缓存。宿主在**运行中替换 ResourceResolver** 之后
// 必须调用它（缓存按「引用 + 策略」建键，感知不到 resolver 的替换）。
func ClearMediaRefCache() {
	mediaGateMu.Lock()
	defer mediaGateMu.Unlock()
	mediaGateCache = nil
}

// allowMediaRef 报告媒体引用是否可被宿主导入（未装配 = 拒绝）。
func allowMediaRef(ref string) bool {
	s := strings.TrimSpace(ref)
	if s == "" {
		return false
	}
	// ★ data: 与策略解耦、恒放行——用引擎的同一份判定（webkit.IsPolicyFreeResourceRef），
	//   因此「未装配门禁」也不影响 data:（自带内容，不经外部通道，无授权问题）。
	if webkit.IsPolicyFreeResourceRef(s) {
		return true
	}
	mediaGateMu.RLock()
	fn := mediaGateFn
	pol := mediaGatePolicy
	key := mediaGateKey{ref: s}
	if pol != nil {
		key.policy = pol().String()
	}
	hit, ok := mediaGateCache[key]
	mediaGateMu.RUnlock()
	if ok {
		return hit
	}
	if fn == nil {
		return false // 未装配判定：安全默认——拿不到路径
	}
	// 判定在锁外做（内部可能读文件；它不回调本层，因此不会自锁）。
	allowed := fn(s)
	mediaGateMu.Lock()
	if len(mediaGateCache) >= mediaGateCacheLimit {
		mediaGateCache = nil
	}
	if mediaGateCache == nil {
		mediaGateCache = map[mediaGateKey]bool{}
	}
	mediaGateCache[key] = allowed
	mediaGateMu.Unlock()
	return allowed
}

// mediaRefGateFor 返回「该 WebView 是否允许导入这个媒体引用」的判定函数。
// 判定逻辑在 webkit.MediaResourceAllowed——本层不复制任何规则；wv 为 nil 时一律
// 拒绝（方法自带 nil 检查）。
func mediaRefGateFor(wv *webkit.WebView) func(ref string) bool {
	return func(ref string) bool { return wv.MediaResourceAllowed(ref) }
}

// mediaPolicyOf 返回取该 WebView 当前资源策略的闭包（判定缓存的键）。
func mediaPolicyOf(wv *webkit.WebView) func() webkit.ResourcePolicy {
	return func() webkit.ResourcePolicy {
		if wv == nil {
			return webkit.DenyExternal // 无 WebView：按最严档位标记
		}
		return wv.ResourcePolicy()
	}
}
