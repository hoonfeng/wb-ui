package bindings

// computedStyleFor 结果缓存（性能优化）：CM6 measure 每次 getClientRects
// 都调 computedStyleFor（全样式表扫描 ~5.7ms/次，每次输入 35+ 次 →
// 200ms+，事件响应慢主因）。缓存 key=元素指针（dom.Node 接口持指针，
// Go map 接口 key 按动态类型+指针比较 ✓）。
//
// 失效策略（精确）：
//   - 样式表变更（<style> 添加/文本变化）→ BumpStyleVersion 全失效
//   - 元素 class/style/属性变更 → InvalidateComputedStyle(el)：只清除
//     该元素及其后代的缓存（选择器匹配依赖 class/style；祖先不变）
//   - DOM 结构变更：元素节点增删时失效（选择器只匹配元素；文本节点
//     变化不影响任何元素样式 → 编辑器打字 insertText 不失效缓存，这是本
//     优化的核心）
import (
	"sync"
	"sync/atomic"

	"wb-ui/dom"
)

type cssCacheEntry struct {
	ver   uint64 // 全局样式版本
	props map[string]string
}

var (
	cssCacheMu sync.Mutex
	cssCache   = map[interface{}]*cssCacheEntry{}
	styleVer   uint64
)

// lineHeightEntry 缓存 line-height 继承链解析结果：rangeRect 里 line-height
// 从文本节点父元素向上逐层 computedStyleFor（CM6 DOM 树 ~17 层祖先），
// 每次输入 29 次 rangeRect × 17 层 = ~495 次 computedStyleFor（38ms），
// 但同一父元素的 line-height 在输入期间不变 → 缓存后只解析一次。
type lineHeightEntry struct {
	ver    uint64
	height float64 // 最终 line-height px 值（found=true 时有效）
	found  bool    // 是否在继承链中找到显式 line-height
}

var (
	lineHeightMu    sync.Mutex
	lineHeightCache = map[interface{}]*lineHeightEntry{}
)

// BumpStyleVersion 使全部 computed style 缓存失效（样式表变更时调用）。
func BumpStyleVersion() {
	atomic.AddUint64(&styleVer, 1)
	cssCacheMu.Lock()
	cssCache = map[interface{}]*cssCacheEntry{}
	cssCacheMu.Unlock()
	lineHeightMu.Lock()
	lineHeightCache = map[interface{}]*lineHeightEntry{}
	lineHeightMu.Unlock()
}

// InvalidateComputedStyle 清除 el 及其后代元素的缓存（class/style/属性
// 变更或元素增删时调用；文本节点变更不需要）。
func InvalidateComputedStyle(el *dom.Element) {
	cssCacheMu.Lock()
	delSubtree(el, cssCache)
	cssCacheMu.Unlock()
	// line-height 继承链缓存 key 是文本节点父元素，其 line-height 依赖
	// 祖先链上的 line-height 声明——内联 style/class 变更可能影响祖先或
	// 后代的 line-height 继承，保守起见全清（缓存通常很小，仅活跃编辑行）。
	lineHeightMu.Lock()
	lineHeightCache = map[interface{}]*lineHeightEntry{}
	lineHeightMu.Unlock()
}

func delSubtree(el *dom.Element, cache map[interface{}]*cssCacheEntry) {
	delete(cache, el)
	for _, c := range el.ChildNodes() {
		if e, ok := c.(*dom.Element); ok {
			delSubtree(e, cache)
		}
	}
}

func cssCacheGet(n interface{}) (map[string]string, bool) {
	cssCacheMu.Lock()
	defer cssCacheMu.Unlock()
	e, ok := cssCache[n]
	if !ok || e.ver != atomic.LoadUint64(&styleVer) {
		return nil, false
	}
	return e.props, true
}

func cssCachePut(n interface{}, props map[string]string) {
	if len(cssCache) > 8192 {
		cssCache = map[interface{}]*cssCacheEntry{}
	}
	cssCacheMu.Lock()
	cssCache[n] = &cssCacheEntry{ver: atomic.LoadUint64(&styleVer), props: props}
	cssCacheMu.Unlock()
}

// lineHeightCacheGet 返回缓存的 line-height 继承链结果：(height, found, ok)。
// ok=false 表示缓存未命中（或版本过期），需要重新遍历继承链解析。
func lineHeightCacheGet(n interface{}) (float64, bool, bool) {
	lineHeightMu.Lock()
	defer lineHeightMu.Unlock()
	e, ok := lineHeightCache[n]
	if !ok || e.ver != atomic.LoadUint64(&styleVer) {
		return 0, false, false
	}
	return e.height, e.found, true
}

func lineHeightCachePut(n interface{}, height float64, found bool) {
	lineHeightMu.Lock()
	lineHeightCache[n] = &lineHeightEntry{ver: atomic.LoadUint64(&styleVer), height: height, found: found}
	lineHeightMu.Unlock()
}

// init 注册动态伪类状态变化回调：:hover/:focus/:active 状态变化不影响
// 样式表/属性（BumpStyleVersion/InvalidateComputedStyle 都不触发），但
// 选择器匹配跨元素（:hover 冒泡、div:hover a、:focus-within、:active 冒泡）
// ——挂起点（如 tree-head 的子元素）与兄弟分支（gdel 依赖共同祖先的
// :hover 冒泡匹配）都可能受状态影响的元素，无法按子树精确失效。因此
// 直接全失效（BumpStyleVersion）：hover/focus/active 切换是低频事件
// （元素边界才触发），下一次 getComputedStyle 重算的代价可忽略。
// 不处理则 getComputedStyle 在鼠标移开/焦点转移后仍返回旧值
// （:hover 视觉不恢复的根因——tb-close 自身恢复但 .tree-head:hover
//  .gdel 兄弟分支不恢复，实测复现）。
func init() {
	dom.DynamicPseudoStateChanged = func(el *dom.Element) {
		BumpStyleVersion()
	}
}
