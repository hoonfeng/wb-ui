// BoxContentSize 内容尺寸缓存的失效语义回归测试（见 renderview.go 的
// boxContentSizeCache / InvalidateContentSizeCache）。
//
// 背景：滚动性能修复给 BoxContentSize（原先每次调用都做 O(子树) 递归）加了
// 缓存，代价是必须保证任何会改变「子树几何 / 文本段」的操作都让缓存作废。
// 失效点两处：syncGeometry()（每次布局后的几何刷新）与 ApplyTextChange()
// （文本段增量更新）。任一漏掉，overflow:auto 容器在内容增长后都会读到**陈旧
// 的内容尺寸** → 滚动范围塌成 0 / 滚动条不画（DOM 侧 scrollHeight 与渲染层
// ScrollRange 两套口径不一致）。
//
// 本测试用真实路径钉死这一点：解析 HTML → 建渲染树 → 布局 → 读内容尺寸 →
// 增长子内容 → 重排（syncGeometry）→ 再读，必须反映新几何。
package rendering

import (
	"strings"
	"testing"

	"wb-ui/engine/dom"
)

// contentSizeCacheHTML：overflow:auto 滚动容器（200×100）+ 子内容 #content
// （宽 100px，高度由内容撑开）。初始文本一行；测试把它替换成长文本，令其在
// 100px 宽内换行成多行，从而把 #scroller 的内容高度从 ~1 行推到数百行。
const contentSizeCacheHTML = `<!doctype html>
<html><head><style>
  html, body { margin: 0; padding: 0; }
  #scroller { width: 200px; height: 100px; overflow: auto; }
  #content { width: 100px; }
</style></head>
<body>
  <div id="scroller"><div id="content">short</div></div>
</body></html>`

func TestContentSizeCacheInvalidation(t *testing.T) {
	rv := buildDocHTML(t, 800, 600, contentSizeCacheHTML)

	doc := rv.Document()
	scrollerEl := doc.GetElementById("scroller")
	if scrollerEl == nil {
		t.Fatal("fixture #scroller missing")
	}
	contentEl := doc.GetElementById("content")
	if contentEl == nil {
		t.Fatal("fixture #content missing")
	}
	textNode, ok := contentEl.FirstChild().(*dom.Text)
	if !ok {
		t.Fatalf("fixture #content first child = %T, want *dom.Text", contentEl.FirstChild())
	}
	scrollerBox := rv.FindRenderBoxForNode(scrollerEl)
	if scrollerBox == nil {
		t.Fatal("no render box for #scroller")
	}

	// ① 首次读取（缓存未命中，走全子树递归）。
	w1, h1 := rv.BoxContentSize(scrollerBox)
	if w1 <= 0 || h1 <= 0 {
		t.Fatalf("BoxContentSize(#scroller) = (%.1f, %.1f); want positive content size for the initial fixture", w1, h1)
	}
	// ② 同一几何版本内重复读取必须命中缓存且返回同一值（缓存读路径正确，
	//    也是后面「新值 ≠ 旧值」断言的前提：说明比较的是缓存内容而非随机）。
	w1b, h1b := rv.BoxContentSize(scrollerBox)
	if w1b != w1 || h1b != h1 {
		t.Fatalf("cached re-read = (%.1f, %.1f), first read = (%.1f, %.1f); 缓存命中路径必须返回同一值",
			w1b, h1b, w1, h1)
	}
	t.Logf("初始内容尺寸 = (%.1f, %.1f)", w1, h1)

	// ③ 增长子内容（文本节点）→ ApplyTextChange（失效点之一）→ 重排
	//    （Layout 内调 syncGeometry，失效点之二）→ 读到的必须是新几何。
	textNode.SetData(strings.Repeat("word ", 400))
	if !rv.ApplyTextChange(textNode) {
		t.Fatal("ApplyTextChange 返回 false：文本节点未关联渲染对象（夹具失效）")
	}
	rv.Layout(nil)

	w2, h2 := rv.BoxContentSize(scrollerBox)
	t.Logf("文本增长并重排后内容尺寸 = (%.1f, %.1f)", w2, h2)
	if h2 <= h1 {
		t.Fatalf("内容尺寸未随子内容增长更新：before=(%.1f, %.1f) after=(%.1f, %.1f) —— 缓存陈旧或失效点缺失",
			w1, h1, w2, h2)
	}
	// ③-b 重读一次：新值同样必须进入缓存（否则每次读取都重算，性能回归）。
	w2b, h2b := rv.BoxContentSize(scrollerBox)
	if w2b != w2 || h2b != h2 {
		t.Fatalf("增长后 cached re-read = (%.1f, %.1f), first read = (%.1f, %.1f); 新几何必须被缓存",
			w2b, h2b, w2, h2)
	}

	// ④ 端到端语义：内容高度已远超容器可视高（100px）→ 容器必须可滚动。
	//    若缓存陈旧（内容尺寸停在旧值），这里 canY 会退化为 false。
	maxX, maxY, _, canY := ScrollRange(rv, scrollerBox)
	t.Logf("ScrollRange(%.0f × %.0f 可视区) = (maxX=%.1f, maxY=%.1f, canX=%v, canY=%v)",
		scrollerBox.FrameRect().Width, scrollerBox.FrameRect().Height, maxX, maxY, true, canY)
	if !canY || maxY <= 0 {
		t.Fatalf("canY=%v maxY=%.1f; 内容增长后容器应可滚动（缓存陈旧会让内容尺寸停在旧值）", canY, maxY)
	}
	_ = maxX
}

// TestContentSizeCacheInvalidationOverflowSubtree 覆盖监督者点名的风险面：
// **overflow 子树** 里的缓存陈旧。boxContentSizeUncached 对内层 overflow 盒有
// 「子盒自身可滚 ⇒ 其溢出内容不计入外层滚动区域」的裁剪规则，该判断本身就
// 递归调用 BoxContentSize（带缓存）——若内层内容增长后缓存没失效，外层会按
// **内层旧尺寸** 判断「是否可滚」，从而错误地把内层溢出内容计/不计入外层。
const contentSizeCacheOverflowHTML = `<!doctype html>
<html><head><style>
  html, body { margin: 0; padding: 0; }
  #outer { width: 200px; height: 120px; overflow: auto; }
  #inner { width: 100px; height: 40px; overflow: auto; }
  #innerContent { width: 60px; }
</style></head>
<body>
  <div id="outer">
    <div id="inner"><div id="innerContent">short</div></div>
    <div id="tail" style="width: 200px; height: 60px"></div>
  </div>
</body></html>`

func TestContentSizeCacheInvalidationOverflowSubtree(t *testing.T) {
	rv := buildDocHTML(t, 800, 600, contentSizeCacheOverflowHTML)

	doc := rv.Document()
	outerBox := rv.FindRenderBoxForNode(doc.GetElementById("outer"))
	innerBox := rv.FindRenderBoxForNode(doc.GetElementById("inner"))
	innerContentEl := doc.GetElementById("innerContent")
	if outerBox == nil || innerBox == nil || innerContentEl == nil {
		t.Fatal("fixture boxes missing (#outer / #inner / #innerContent)")
	}
	textNode, ok := innerContentEl.FirstChild().(*dom.Text)
	if !ok {
		t.Fatalf("fixture #innerContent first child = %T, want *dom.Text", innerContentEl.FirstChild())
	}

	ow1, oh1 := rv.BoxContentSize(outerBox)
	iw1, ih1 := rv.BoxContentSize(innerBox)
	t.Logf("初始：outer 内容=(%.1f, %.1f) inner 内容=(%.1f, %.1f)", ow1, oh1, iw1, ih1)
	if ih1 <= 0 {
		t.Fatalf("inner 内容尺寸 = (%.1f, %.1f); want positive height", iw1, ih1)
	}

	// 内层内容大幅增长（内层高固定 40px ⇒ 内层自身可滚 ⇒ 其溢出内容不计入外层）。
	textNode.SetData(strings.Repeat("word ", 400))
	if !rv.ApplyTextChange(textNode) {
		t.Fatal("ApplyTextChange 返回 false：文本节点未关联渲染对象（夹具失效）")
	}
	rv.Layout(nil)

	ow2, oh2 := rv.BoxContentSize(outerBox)
	iw2, ih2 := rv.BoxContentSize(innerBox)
	t.Logf("增长后：outer 内容=(%.1f, %.1f) inner 内容=(%.1f, %.1f)", ow2, oh2, iw2, ih2)

	// (a) 内层内容尺寸必须反映新几何（否则外层会按旧尺寸做裁剪判断）。
	if ih2 <= ih1 {
		t.Fatalf("inner 内容尺寸未更新：before=(%.1f, %.1f) after=(%.1f, %.1f) —— 缓存陈旧",
			iw1, ih1, iw2, ih2)
	}
	// (b) 内层自身可滚 ⇒ 其溢出内容不得计入外层：外层内容高度不得被内层的
	//     数百行内容撑大（外层只有 #inner 的 40px + #tail 的 60px ≈ 100px）。
	if oh2 > oh1+1 {
		t.Fatalf("outer 内容高度 = %.1f（初始 %.1f，内层容纳 40px + tail 60px）：内层可滚容器的溢出内容被计入外层 —— "+
			"失效点缺失导致按内层旧尺寸放行了本应被裁剪的子树", oh2, oh1)
	}
	// (c) 外层仍可滚动（内容 ~100px，可视 120px ⇒ 此处 canY 允许 false，
	//     关键是内层可滚：用内层自身验证可滚性，防止缓存把范围塌成 0）。
	_, innerMaxY, _, innerCanY := ScrollRange(rv, innerBox)
	t.Logf("inner ScrollRange: maxY=%.1f canY=%v", innerMaxY, innerCanY)
	if !innerCanY || innerMaxY <= 0 {
		t.Fatalf("inner canY=%v maxY=%.1f；内层内容增长后必须可滚动（缓存陈旧会让它停在旧值）", innerCanY, innerMaxY)
	}
}
