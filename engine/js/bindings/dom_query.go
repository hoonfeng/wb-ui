// Package bindings — CSS 选择器桥接层（dom ↔ css 互操作）
//
// dom 包不能导入 css 包（循环依赖），因此 Matches / Closest / QuerySelector /
// QuerySelectorAll 作为 bindings 包的 free function 实现。
package bindings

import (
	"strings"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
)

// parseSelectorList 解析 CSS 选择器字符串，失败返回 nil。
func parseSelectorList(selector string) *css.SelectorList {
	p := css.NewParser(selector)
	return p.ParseSelectorList()
}

// ElementMatches 检查元素是否匹配给定的 CSS 选择器。
// 镜像 Element::matches(selectors)。
func ElementMatches(el *dom.Element, selector string) bool {
	selList := parseSelectorList(selector)
	if selList == nil || len(selList.Selectors) == 0 {
		return false
	}
	checker := css.NewSelectorChecker()
	for _, sel := range selList.Selectors {
		if checker.Match(sel, el) {
			return true
		}
	}
	return false
}

// ElementClosest 沿祖先链向上查找第一个匹配选择器的元素。
// 镜像 Element::closest(selectors)。
func ElementClosest(el *dom.Element, selector string) *dom.Element {
	selList := parseSelectorList(selector)
	if selList == nil || len(selList.Selectors) == 0 {
		return nil
	}
	checker := css.NewSelectorChecker()
	for cur := el; cur != nil; cur = cur.ParentElement() {
		for _, sel := range selList.Selectors {
			if checker.Match(sel, cur) {
				return cur
			}
		}
	}
	return nil
}

// ElementQuerySelector 在元素子树中查找第一个匹配选择器的后代元素。
// 镜像 Element::querySelector(selectors)。
func ElementQuerySelector(el *dom.Element, selector string) *dom.Element {
	selList := parseSelectorList(selector)
	if selList == nil || len(selList.Selectors) == 0 {
		return nil
	}
	// 简单选择器快路径（.class / #id / tag / *）：与通用链路等价，但省掉每节点的
	// Match → matchComplex → matchCompound → matchSimple 四层调用与按值传递。
	// 实测依据见文件末「快路径」注释块。
	if kind, v := classifySimpleQuery(selList); kind != simpleQueryNone {
		var fast *dom.Element
		el.WalkDescendantElements(func(e *dom.Element) bool {
			if matchSimpleQuery(kind, v, e) {
				fast = e
				return false
			}
			return true
		})
		return fast
	}
	checker := css.NewSelectorChecker()
	var result *dom.Element
	el.WalkDescendantElements(func(e *dom.Element) bool {
		for _, sel := range selList.Selectors {
			if checker.Match(sel, e) {
				result = e
				return false // stop
			}
		}
		return true
	})
	return result
}

// ElementQuerySelectorAll 在元素子树中查找所有匹配选择器的后代元素。
// 镜像 Element::querySelectorAll(selectors)。
func ElementQuerySelectorAll(el *dom.Element, selector string) []*dom.Element {
	selList := parseSelectorList(selector)
	if selList == nil || len(selList.Selectors) == 0 {
		return nil
	}
	// 简单选择器快路径（同 ElementQuerySelector）。
	if kind, v := classifySimpleQuery(selList); kind != simpleQueryNone {
		var fast []*dom.Element
		el.WalkDescendantElements(func(e *dom.Element) bool {
			if matchSimpleQuery(kind, v, e) {
				fast = append(fast, e)
			}
			return true
		})
		return fast
	}
	checker := css.NewSelectorChecker()
	var result []*dom.Element
	el.WalkDescendantElements(func(e *dom.Element) bool {
		for _, sel := range selList.Selectors {
			if checker.Match(sel, e) {
				result = append(result, e)
				break
			}
		}
		return true
	})
	return result
}

// DocumentQuerySelectorAll 在文档中查找所有匹配选择器的元素。
func DocumentQuerySelectorAll(doc *dom.Document, selector string) []*dom.Element {
	root := doc.DocumentElement()
	if root == nil {
		return nil
	}
	return ElementQuerySelectorAll(root, selector)
}

// DocumentQuerySelector 在文档中查找第一个匹配选择器的元素。
func DocumentQuerySelector(doc *dom.Document, selector string) *dom.Element {
	root := doc.DocumentElement()
	if root == nil {
		return nil
	}
	return ElementQuerySelector(root, selector)
}

// ── 简单选择器快路径 ─────────────────────────────────────────────────────────
//
// 实测依据（engine/js/bindings/query_perf_test.go，编辑器形 DOM 1614 节点，
// querySelectorAll×3000 + -cpuprofile）：改掉 strings.Fields 之后，剩余样本里
// 通用匹配链路仍占 ~43%（css.(*SelectorChecker).matchCompound flat 21.4% /
// cum 42.9%）——它对本树的**每个节点**都要跑一遍
// Match → matchComplex → matchCompound → matchSimple，且 ComplexSelector /
// CompoundSelector 按值传递。而 querySelector / querySelectorAll 的真实调用里
// 绝大多数是「单 compound + 单 simple selector」（`.cm-line`、`#app`、`div`）。
//
// 快路径对这三类形态直接判定，语义与通用链路**逐条等价**：
//   - MatchClass → Element::hasClassName（同一个零分配 token 匹配）；
//   - MatchID    → 解析器只接受一个 id 简单选择器，与 element.id 全等比较；
//   - MatchTag   → 与 LocalName 做 ASCII 大小写无关比较（HTML 元素名，
//     与 matchSimple 的 strings.EqualFold 同规则）；`*` 匹配任意元素。
// 只要选择器里出现组合器、多 compound、多 simple、属性/伪类/伪元素，就返回
// simpleQueryNone 回落到通用链路，不做任何猜测式改写。

type simpleQueryKind uint8

const (
	simpleQueryNone simpleQueryKind = iota
	simpleQueryClass
	simpleQueryID
	simpleQueryTag
	simpleQueryUniversal
)

// classifySimpleQuery 报告选择器是否属于「单 compound + 单 simple selector」的
// 常见形态；是则返回其类别与值，否则 simpleQueryNone。
func classifySimpleQuery(list *css.SelectorList) (simpleQueryKind, string) {
	if list == nil || len(list.Selectors) != 1 {
		return simpleQueryNone, ""
	}
	sel := list.Selectors[0]
	if len(sel.Compounds) != 1 {
		return simpleQueryNone, ""
	}
	comp := sel.Compounds[0]
	if len(comp.Selectors) != 1 {
		return simpleQueryNone, ""
	}
	s := comp.Selectors[0]
	switch s.Match {
	case css.MatchClass:
		return simpleQueryClass, s.Value
	case css.MatchID:
		return simpleQueryID, s.Value
	case css.MatchTag:
		if s.Value == "*" {
			return simpleQueryUniversal, ""
		}
		return simpleQueryTag, s.Value
	}
	return simpleQueryNone, ""
}

// matchSimpleQuery 判定单个元素是否命中快路径选择器。
func matchSimpleQuery(kind simpleQueryKind, v string, e *dom.Element) bool {
	switch kind {
	case simpleQueryClass:
		return e.HasClassName(v)
	case simpleQueryID:
		return e.GetId() == v
	case simpleQueryTag:
		return strings.EqualFold(e.LocalName(), v)
	}
	return true // simpleQueryUniversal：`*` 匹配任意元素
}
