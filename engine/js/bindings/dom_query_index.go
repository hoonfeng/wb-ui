// Package bindings — querySelector 族的选择器解析缓存与结构索引。
//
// 两块优化（对应 PERF_DISPATCH_BREAKDOWN §7 的「选择器匹配索引化」）：
//
//  1. **选择器解析缓存**：缓存「选择器字符串 → *css.SelectorList」。该映射是
//     纯函数（与 DOM 状态无关），所以**不需要失效协议**；缓存中的 AST 只被只读
//     访问（SelectorChecker.Match / classifySimpleQuery），无共享可变状态。
//
//  2. **文档级结构索引**（tag / class / id → 元素），**按需填充**：某个 key
//     第一次被查询时才扫描一次文档收集它，之后同一 key（且在同一个 DOM 变更
//     序号内）O(1) 取用。按需填充保证「不亏」——一次全新 key 的查询最多就是
//     一次全文档扫描（与遍历实现同量级，且每节点只做一个判定），而重复查询
//     同一选择器（框架/编辑器组件的典型模式，如每帧 querySelector('.cm-line')）
//     直接命中缓存。
//
// 失效协议：dom.DOMChangeSeq()。任何结构或属性变更都会递增它（挂点审计见
// engine/dom/change_seq.go），序号变化即整体丢弃索引、下次查询重新按需填充。
// 序号是一次 O(1) 读取，且只增不减 ⇒ 宁可多失效，绝不返回陈旧结果。
package bindings

import (
	"strings"
	"sync"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
)

// DisableQueryIndex 关闭结构索引（诊断与等价性测试用：索引路径与遍历路径
// 的结果必须逐项一致，见 dom_query_index_test.go）。生产路径保持 false。
var DisableQueryIndex = false

// ── 选择器解析缓存 ──────────────────────────────────────────────────────────

const parsedSelectorCacheLimit = 512

var (
	parsedSelectorMu    sync.Mutex
	parsedSelectorCache = map[string]*css.SelectorList{}
)

// cachedParseSelectorList 返回选择器的解析结果（带缓存）。
// ★ 返回的 *css.SelectorList 视为**只读**：调用方不得修改其内容
//（缓存对象被后续所有同选择器调用共享）。
func cachedParseSelectorList(selector string) *css.SelectorList {
	parsedSelectorMu.Lock()
	if l, ok := parsedSelectorCache[selector]; ok {
		parsedSelectorMu.Unlock()
		return l
	}
	parsedSelectorMu.Unlock()

	// 解析在锁外进行（解析可能较慢，且是纯计算）。
	l := parseSelectorList(selector)

	parsedSelectorMu.Lock()
	// 有界退化：满则整体清空。真实页面里活跃的选择器字符串只有几十个，
	// 清空后立即重建的代价远小于维护 LRU 的复杂度与额外内存。
	if len(parsedSelectorCache) >= parsedSelectorCacheLimit {
		parsedSelectorCache = map[string]*css.SelectorList{}
	}
	parsedSelectorCache[selector] = l
	parsedSelectorMu.Unlock()
	return l
}

// ── 文档级结构索引 ──────────────────────────────────────────────────────────

// docQueryIndex 是文档级的 tag/class/id 索引。字段按需填充（nil = 该 key
// 尚未被查询过）；整个对象绑定一个 DOM 变更序号（seq），序号变化即丢弃。
//
// 索引覆盖「文档根的后代」（不含根自身）——与 querySelector 的后代语义一致，
// 因此文档级查询不需要任何额外过滤。子树查询（root ≠ 文档根）不走索引。
type docQueryIndex struct {
	doc  *dom.Document
	root *dom.Element
	seq  uint64

	byTag   map[string][]*dom.Element
	byID    map[string]*dom.Element
	byClass map[string][]*dom.Element
	all     []*dom.Element
}

var (
	docIndexMu  sync.Mutex
	docIndexCur *docQueryIndex
)

// queryIndexFor 返回 root 当前有效的查询索引；不可用时返回 nil（调用方回退
// 遍历）。仅当 root 是该文档的 DocumentElement（真正的文档级查询）时可用：
// 子树/游离元素查询没有可复用的索引，且过滤成本会抵消收益。
func queryIndexFor(root *dom.Element) *docQueryIndex {
	if DisableQueryIndex || root == nil {
		return nil
	}
	doc := root.OwnerDocument()
	if doc == nil || doc.DocumentElement() != root {
		return nil
	}
	seq := dom.DOMChangeSeq()

	docIndexMu.Lock()
	defer docIndexMu.Unlock()
	if ix := docIndexCur; ix != nil && ix.doc == doc && ix.root == root && ix.seq == seq {
		return ix
	}
	ix := &docQueryIndex{doc: doc, root: root, seq: seq}
	docIndexCur = ix
	return ix
}

// allElements 返回文档根的全部后代元素（文档序，首次调用时构建）。
func (ix *docQueryIndex) allElements() []*dom.Element {
	if ix.all == nil {
		var out []*dom.Element
		ix.root.WalkDescendantElements(func(e *dom.Element) bool {
			out = append(out, e)
			return true
		})
		ix.all = out
	}
	return ix.all
}

// elementsByTag 按 **已小写化** 的 tag 收集元素（文档序）。
// 与 matchSimpleQuery 的 EqualFold(LocalName, v) 等价：LocalName 恒为小写。
func (ix *docQueryIndex) elementsByTag(tag string) []*dom.Element {
	if c, ok := ix.byTag[tag]; ok {
		return c
	}
	var out []*dom.Element
	ix.root.WalkDescendantElements(func(e *dom.Element) bool {
		if e.LocalName() == tag {
			out = append(out, e)
		}
		return true
	})
	if ix.byTag == nil {
		ix.byTag = map[string][]*dom.Element{}
	}
	ix.byTag[tag] = out
	return out
}

// elementByID 返回文档序里第一个 id 匹配的元素（无可缓存时缓存 nil）。
func (ix *docQueryIndex) elementByID(id string) *dom.Element {
	if e, ok := ix.byID[id]; ok {
		return e
	}
	var found *dom.Element
	ix.root.WalkDescendantElements(func(e *dom.Element) bool {
		if e.GetId() == id {
			found = e
			return false
		}
		return true
	})
	if ix.byID == nil {
		ix.byID = map[string]*dom.Element{}
	}
	ix.byID[id] = found
	return found
}

// elementsByClass 按 class token 收集元素（文档序）。
// 用 HasClassName（与快路径 / 通用链路的 class 匹配同源，零分配 token 匹配）。
func (ix *docQueryIndex) elementsByClass(token string) []*dom.Element {
	if c, ok := ix.byClass[token]; ok {
		return c
	}
	var out []*dom.Element
	ix.root.WalkDescendantElements(func(e *dom.Element) bool {
		if e.HasClassName(token) {
			out = append(out, e)
		}
		return true
	})
	if ix.byClass == nil {
		ix.byClass = map[string][]*dom.Element{}
	}
	ix.byClass[token] = out
	return out
}

// indexedFirstMatch 用结构索引取「首个匹配」（文档序）；ok=false 表示索引
// 不可用，调用方必须回退遍历。
func indexedFirstMatch(root *dom.Element, kind simpleQueryKind, value string) (*dom.Element, bool) {
	ix := queryIndexFor(root)
	if ix == nil {
		return nil, false
	}
	switch kind {
	case simpleQueryClass:
		if c := ix.elementsByClass(value); len(c) > 0 {
			return c[0], true
		}
		return nil, true
	case simpleQueryID:
		return ix.elementByID(value), true
	case simpleQueryTag:
		if c := ix.elementsByTag(strings.ToLower(value)); len(c) > 0 {
			return c[0], true
		}
		return nil, true
	case simpleQueryUniversal:
		if c := ix.allElements(); len(c) > 0 {
			return c[0], true
		}
		return nil, true
	}
	return nil, false
}

// indexedAllMatches 用结构索引取「全部匹配（文档序）」；ok=false 表示索引
// 不可用，调用方必须回退遍历。
//
// ★ 返回**副本**：索引里缓存的切片是跨调用共享的，直接把缓存切片交出去会让
// 调用方（或其后端）的 append 改写索引内容。复制成本是 O(结果数)，仍远低于
// 一次全文档遍历。
func indexedAllMatches(root *dom.Element, kind simpleQueryKind, value string) ([]*dom.Element, bool) {
	ix := queryIndexFor(root)
	if ix == nil {
		return nil, false
	}
	switch kind {
	case simpleQueryClass:
		return cloneElements(ix.elementsByClass(value)), true
	case simpleQueryID:
		if e := ix.elementByID(value); e != nil {
			return []*dom.Element{e}, true
		}
		return nil, true
	case simpleQueryTag:
		return cloneElements(ix.elementsByTag(strings.ToLower(value))), true
	case simpleQueryUniversal:
		return cloneElements(ix.allElements()), true
	}
	return nil, false
}

// cloneElements 复制元素切片（保持文档序、与源切片不共享底层数组）。
func cloneElements(src []*dom.Element) []*dom.Element {
	if len(src) == 0 {
		return nil
	}
	out := make([]*dom.Element, len(src))
	copy(out, src)
	return out
}
