// Package bindings — CSS 选择器桥接层（dom ↔ css 互操作）
//
// dom 包不能导入 css 包（循环依赖），因此 Matches / Closest / QuerySelector /
// QuerySelectorAll 作为 bindings 包的 free function 实现。
package bindings

import (
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
	checker := css.NewSelectorChecker()
	var result *dom.Element
	el.WalkDescendants(func(n dom.Node) bool {
		e, ok := n.(*dom.Element)
		if !ok {
			return true // continue
		}
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
	checker := css.NewSelectorChecker()
	var result []*dom.Element
	el.WalkDescendants(func(n dom.Node) bool {
		e, ok := n.(*dom.Element)
		if !ok {
			return true
		}
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
