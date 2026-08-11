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

// BumpStyleVersion 使全部 computed style 缓存失效（样式表变更时调用）。
func BumpStyleVersion() {
	atomic.AddUint64(&styleVer, 1)
	cssCacheMu.Lock()
	cssCache = map[interface{}]*cssCacheEntry{}
	cssCacheMu.Unlock()
}

// InvalidateComputedStyle 清除 el 及其后代元素的缓存（class/style/属性
// 变更或元素增删时调用；文本节点变更不需要）。
func InvalidateComputedStyle(el *dom.Element) {
	cssCacheMu.Lock()
	delSubtree(el, cssCache)
	cssCacheMu.Unlock()
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
