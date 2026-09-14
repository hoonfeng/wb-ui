// Tests for the :fullscreen pseudo-class match (Fullscreen spec §5.1): it must
// match the element whose fullscreen flag is set — i.e. the document's
// fullscreen element — and nothing else.

package css

import (
	"testing"

	"wb-ui/engine/dom"
)

// mustParseOneSelector 解析只含一个选择器的字符串（测试用）。
func mustParseOneSelector(t *testing.T, src string) ComplexSelector {
	t.Helper()
	list := NewParser(src).ParseSelectorList()
	if list == nil || len(list.Selectors) != 1 {
		t.Fatalf("解析 %q 失败", src)
	}
	return list.Selectors[0]
}

func TestSelector_FullscreenPseudoClass(t *testing.T) {
	doc := dom.NewDocument()
	html := dom.NewElement(doc, "html")
	doc.AppendChild(html)
	body := dom.NewElement(doc, "body")
	html.AppendChild(body)
	box := dom.NewElement(doc, "div")
	body.AppendChild(box)
	sib := dom.NewElement(doc, "div")
	body.AppendChild(sib)

	c := NewSelectorChecker()
	fs := mustParseOneSelector(t, ":fullscreen")
	if got := fs.String(); got != ":fullscreen" {
		t.Fatalf("ComplexSelector.String() = %q，want \":fullscreen\"", got)
	}

	// 没有全屏元素时，谁都不匹配。
	if c.Match(fs, box) {
		t.Fatal("未全屏时 :fullscreen 不应匹配")
	}
	if c.Match(fs, html) {
		t.Fatal("未全屏时 :fullscreen 不应匹配根元素")
	}

	// 进入全屏：只有全屏元素本身匹配（规范要求的 fullscreen flag 语义）。
	doc.SetFullscreenElement(box)
	if !c.Match(fs, box) {
		t.Fatal("全屏元素应匹配 :fullscreen")
	}
	if c.Match(fs, sib) {
		t.Fatal("非全屏的同级元素不应匹配 :fullscreen")
	}
	if c.Match(fs, body) || c.Match(fs, html) {
		t.Fatal("全屏元素的祖先不应匹配 :fullscreen")
	}

	// 退出全屏后不再匹配。
	doc.SetFullscreenElement(nil)
	if c.Match(fs, box) {
		t.Fatal("退出全屏后 :fullscreen 不应再匹配")
	}

	// UA 规则使用的形式 :fullscreen:not(:root)：根元素全屏时被排除
	// （根元素本来就铺满视口，再钉成 fixed 会破坏文档滚动）。
	ua := mustParseOneSelector(t, ":fullscreen:not(:root)")
	doc.SetFullscreenElement(box)
	if !c.Match(ua, box) {
		t.Fatal("div 全屏时应匹配 :fullscreen:not(:root)")
	}
	doc.SetFullscreenElement(html)
	if !c.Match(fs, html) {
		t.Fatal("根元素全屏时应匹配 :fullscreen")
	}
	if c.Match(ua, html) {
		t.Fatal("根元素全屏时不应匹配 :fullscreen:not(:root)")
	}
}
