// engine/js/bindings/imecomposition.go — contenteditable（CodeMirror 6）IME 组合输入
// 的 DOM 层支持。
//
// 浏览器语义：输入法组合期间，浏览器把组合文本写入 DOM（光标处的
// composition range），并对每次更新派发 compositionstart/compositionupdate/
// input(insertCompositionText) 事件；提交时把 range 替换为最终文本并派发
// compositionend + input。CM6 的 readDOMChange 依赖「DOM 已包含组合文本」
// 来同步其编辑器状态（applyDOMChangeInner 在 composing>=0 时把 diff 应用为
// input.type.compose 事务）。
//
// wb-ui 宿主层（app.Host.applyIMEEvents）此前对 contenteditable 完全不写
// DOM（只派发 insertText），组合预览不显示、提交文本插到预览之后（「文字
// 插入异常」根因）。本文件提供三个原语：
//
//   - TextOffsetOfSelection：DOM Selection 锚点在 root 文本内容中的 rune 偏移
//   - ReplaceTextRange：替换 root 文本内容 [from, from+length) 为 insert
//     （拆分/移除/插入文本节点，产生 MutationObserver 记录），返回新末尾偏移
//   - runeCount：UTF-8 字符串 rune 数
//
// 偏移跟踪用「root 文本内容的 rune 偏移」而非节点引用：CM6 每次 input 后
// readDOMChange 会重建 .cm-line（旧节点被替换/分离），但组合文本在文档中的
// 位置保持不变——rune 偏移跨重建稳定，节点引用不稳定。
package bindings

import (
	"strings"

	"wb-ui/engine/dom"
)

func runeCount(s string) int { return len([]rune(s)) }

// textItem 是 root 文本内容中的一个文本节点片段（按文档序）。
type textItem struct {
	node   *dom.Text
	start  int // 该节点首字符在 root 文本内容中的 rune 偏移
	length int
}

// textItemsOf 收集 root 下所有后代文本节点的 (节点, 起始偏移, 长度)。
// <br> 等非文本节点不贡献文本（CM6 空行 .cm-line 只有 <br>，rune 偏移跳过）。
func textItemsOf(root dom.Node) []textItem {
	var items []textItem
	pos := 0
	var walk func(n dom.Node)
	walk = func(n dom.Node) {
		if t, ok := n.(*dom.Text); ok {
			if l := t.Length(); l > 0 {
				items = append(items, textItem{node: t, start: pos, length: l})
			}
			pos += t.Length()
			return
		}
		if el, ok := n.(*dom.Element); ok {
			for c := el.FirstChild(); c != nil; c = c.NextSibling() {
				walk(c)
			}
		}
	}
	if root != nil {
		walk(root)
	}
	return items
}

// TextOffsetOfSelection 返回 DOM Selection 锚点在 root 文本内容中的 rune
// 偏移。锚点是文本节点时按节点内偏移计；锚点是元素时按「前 off 个子节点
// 子树文本」计（空行 .cm-line 的 <br> 光标）。失败返回 ok=false。
func TextOffsetOfSelection(root dom.Node) (int, bool) {
	if root == nil || len(sstate.ranges) == 0 {
		return 0, false
	}
	r := sstate.ranges[0]
	if r == nil {
		return 0, false
	}
	sc := r.GetStr("startContainer")
	if sc.IsNull() || sc.IsUndefined() {
		return 0, false
	}
	node := unwrapNode(sc)
	if node == nil {
		return 0, false
	}
	off := int(r.GetStr("startOffset").ToNumber())
	if off < 0 {
		off = 0
	}
	return TextOffsetAt(root, node, off)
}

// TextOffsetAt 计算 (node, off) 位置在 root 文本内容中的 rune 偏移。
func TextOffsetAt(root, node dom.Node, off int) (int, bool) {
	pos := 0
	found := false
	var walkSub func(n dom.Node)
	var walk func(n dom.Node)
	walkSub = func(n dom.Node) {
		if t, ok := n.(*dom.Text); ok {
			pos += t.Length()
			return
		}
		if el, ok := n.(*dom.Element); ok {
			for c := el.FirstChild(); c != nil; c = c.NextSibling() {
				walk(c)
				if found {
					return
				}
			}
		}
	}
	walk = func(n dom.Node) {
		if n == nil {
			return
		}
		if n == node {
			found = true
			if t, ok := n.(*dom.Text); ok {
				if off > t.Length() {
					off = t.Length()
				}
				pos += off
				return
			}
			// 元素锚点：只计前 off 个子节点的子树文本。
			i := 0
			for c := n.FirstChild(); c != nil && i < off; c = c.NextSibling() {
				walkSub(c)
				i++
			}
			return
		}
		walkSub(n)
	}
	walk(root)
	return pos, found
}

// insertTextAtOffset 在 root 文本内容的 rune 偏移 from 处插入文本节点
// （不删除任何内容）。返回新末尾偏移。
func insertTextAtOffset(root dom.Node, from int, insert string) (int, bool) {
	items := textItemsOf(root)
	doc := domDocumentOf(root)
	if doc == nil {
		return 0, false
	}
	ins := dom.NewText(doc, insert)
	// 无任何文本（空编辑器：.cm-line 只有 <br>）：插入到第一个 .cm-line
	// 元素的首个子节点之前（即 <br> 之前），保证 CM6 的 readDOMChange 能
	// 把文本映射到文档第 1 行。
	if len(items) == 0 {
		host := root
		var depth func(n dom.Node) dom.Node
		depth = func(n dom.Node) dom.Node {
			if el, ok := n.(*dom.Element); ok {
				for c := el.FirstChild(); c != nil; c = c.NextSibling() {
					if ce, ok := c.(*dom.Element); ok {
						// CM6 行元素：<div class="cm-line">（也可能是自定义标签）。
						if ce.LocalName() == "cm-line" ||
							strings.Contains(" "+ce.GetAttribute("class")+" ", " cm-line ") {
							return ce
						}
						if d := depth(ce); d != nil {
							return d
						}
					}
				}
			}
			return nil
		}
		if line := depth(root); line != nil {
			host = line
		}
		if he, ok := host.(*dom.Element); ok {
			first := he.FirstChild()
			if first != nil {
				if err := he.InsertBefore(ins, first); err != nil {
					return 0, false
				}
				sstate.updateRangeForInsert(ins, runeCount(insert))
				return runeCount(insert), true
			}
			if err := he.AppendChild(ins); err != nil {
				return 0, false
			}
			sstate.updateRangeForInsert(ins, runeCount(insert))
			return runeCount(insert), true
		}
		return 0, false
	}
	total := 0
	for _, it := range items {
		total = it.start + it.length
	}
	if from >= total {
		// 末尾插入：追加到最后一个文本节点的父级。
		last := items[len(items)-1].node
		if p := last.ParentNode(); p != nil {
			if err := p.InsertBefore(ins, last.NextSibling()); err != nil {
				return 0, false
			}
			sstate.updateRangeForInsert(ins, runeCount(insert))
			return from + runeCount(insert), true
		}
		return 0, false
	}
	for _, it := range items {
		if from > it.start && from < it.start+it.length {
			// 节点内部：拆分后插入。
			if tail, err := it.node.SplitText(from - it.start); err == nil {
				if p := it.node.ParentNode(); p != nil {
					if err := p.InsertBefore(ins, tail); err != nil {
						return 0, false
					}
					sstate.updateRangeForInsert(ins, runeCount(insert))
					return from + runeCount(insert), true
				}
			}
			return 0, false
		}
		if from == it.start {
			// 节点边界前插入。
			if p := it.node.ParentNode(); p != nil {
				if err := p.InsertBefore(ins, it.node); err != nil {
					return 0, false
				}
				sstate.updateRangeForInsert(ins, runeCount(insert))
				return from + runeCount(insert), true
			}
			return 0, false
		}
	}
	return 0, false
}

func domDocumentOf(n dom.Node) *dom.Document {
	switch v := n.(type) {
	case *dom.Element:
		return v.OwnerDocument()
	case *dom.Text:
		return v.OwnerDocument()
	}
	return nil
}

// ReplaceTextRange 把 root 文本内容的 [from, from+length)（rune 区间）替换为
// insert（单个文本节点），并同步 window.getSelection() 到插入文本之后。
// 返回插入后的新末尾 rune 偏移。length==0 时为纯插入。
// 所有修改经 SplitText/RemoveChild/InsertBefore 完成，保证 MutationObserver
// 记录正常生成（CM6 readDOMChange 依赖）。
func ReplaceTextRange(root dom.Node, from, length int, insert string) (int, bool) {
	if root == nil {
		return 0, false
	}
	if from < 0 {
		from = 0
	}
	if length < 0 {
		length = 0
	}
	endPos := from + length
	if length == 0 {
		return insertTextAtOffset(root, from, insert)
	}
	items := textItemsOf(root)
	// 定位与 [from, endPos) 相交的首/末文本项。
	i0, i1 := -1, -1
	for i, it := range items {
		s, e := it.start, it.start+it.length
		if e <= from || s >= endPos {
			continue
		}
		if i0 < 0 {
			i0 = i
		}
		i1 = i
	}
	if i0 < 0 {
		// 无文本覆盖：退化为纯插入。
		return insertTextAtOffset(root, from, insert)
	}
	first, last := items[i0], items[i1]

	if i0 == i1 {
		n := first.node
		subFrom := from - first.start
		subTo := endPos - first.start
		if subFrom < 0 {
			subFrom = 0
		}
		if subTo > first.length {
			subTo = first.length
		}
		var err error
		if subTo < first.length {
			if _, err = n.SplitText(subTo); err != nil {
				return 0, false
			}
		}
		var mid dom.Node = n
		if subFrom > 0 {
			if mid, err = n.SplitText(subFrom); err != nil {
				return 0, false
			}
		}
		parent := mid.ParentNode()
		if parent == nil {
			return 0, false
		}
		ref := mid.NextSibling()
		if err := parent.RemoveChild(mid); err != nil {
			return 0, false
		}
		doc := domDocumentOf(root)
		if doc == nil {
			return 0, false
		}
		ins := dom.NewText(doc, insert)
		if ref != nil {
			if err := parent.InsertBefore(ins, ref); err != nil {
				return 0, false
			}
		} else if err := parent.AppendChild(ins); err != nil {
			return 0, false
		}
		sstate.updateRangeForInsert(ins, runeCount(insert))
		return from + runeCount(insert), true
	}

	// 多节点：先切尾节点（幸存尾部 = 插入参照），再切首节点，移除中间链。
	var lastKeep *dom.Text
	subToLast := endPos - last.start
	if subToLast > last.length {
		subToLast = last.length
	}
	if subToLast < last.length {
		if tail, err := last.node.SplitText(subToLast); err == nil {
			lastKeep = tail
		}
	}
	var firstDel dom.Node
	subFromFirst := from - first.start
	if subFromFirst < 0 {
		subFromFirst = 0
	}
	if subFromFirst > 0 {
		if tail, err := first.node.SplitText(subFromFirst); err == nil {
			firstDel = tail
		} else {
			return 0, false
		}
	} else {
		firstDel = first.node
	}
	// 移除 firstDel 起、直到 lastKeep（不含）为止的节点链。
	parent := firstDel.ParentNode()
	if parent == nil {
		return 0, false
	}
	// 幸存参照：last 节点完全覆盖时，插入位置 = last 节点之后的兄弟
	// （必须在移除前捕获——RemoveChild 可能清空被移除节点的兄弟链接）。
	var refAfter *dom.Node
	if lastKeep == nil {
		if sn := last.node.NextSibling(); sn != nil {
			refAfter = &sn
		}
	}
	n := firstDel
	for n != nil && n != lastKeep {
		next := n.NextSibling()
		p := n.ParentNode()
		if p != nil {
			_ = p.RemoveChild(n)
		}
		n = next
	}
	doc := domDocumentOf(root)
	if doc == nil {
		return 0, false
	}
	ins := dom.NewText(doc, insert)
	if lastKeep != nil {
		// 用 lastKeep 自己的父级插入（跨嵌套 span 时与 firstDel 父级不同）。
		lp := lastKeep.ParentNode()
		if lp == nil {
			lp = parent
		}
		if err := lp.InsertBefore(ins, lastKeep); err != nil {
			return 0, false
		}
	} else if refAfter != nil {
		if err := parent.InsertBefore(ins, *refAfter); err != nil {
			return 0, false
		}
	} else if err := parent.AppendChild(ins); err != nil {
		return 0, false
	}
	sstate.updateRangeForInsert(ins, runeCount(insert))
	return from + runeCount(insert), true
}
