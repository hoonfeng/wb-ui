package layout

// AboutModal 的 logo 垂直定位回归测试（行内替换元素的行盒基线）。
//
// 缺陷：`vertical-align:baseline`（默认）的替换元素，其基线 = 自身 margin box
// 底边（CSS 2.1 §10.8.1 没错），但**行盒基线**必须是全体行内盒要求的最大值
// ——替换元素要求「行盒顶→基线」至少 marginTop + borderBoxH。原实现固定取
// 文本度量（baseLine = 行盒顶 + halfLeading + 字体 ascent），于是比文本行高的
// 图片底边坐上文本基线后，顶边整体浮出行盒上方。
//
// 实测（1280x800 真实窗口，.about-logo 容器 y=296、高 64、内含 64x64 img）：
//   .about-logo-img 渲染在 y=246 —— 上移 50px，压到弹窗标题栏下沿，
//   logo 与「PairCode IDE」标题之间出现 50px 空洞（浏览器里 logo 贴容器顶）。
//
// 修复：行盒基线取 max(文本基线, marginTop + borderBoxH)，并把本元素登记进
// baselineBoxes 参与同一条行内基线（与表单控件机制共用）。

import (
	"math"
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/style"
)

// TestInlineReplacedTallerThanTextStaysInsideLineBox 钉死：比文本行高得多的
// inline 替换元素（64px img / 13px 文本）必须整体落在其父行盒内，顶边与父内容
// 顶对齐（浏览器行为），而不是上浮到父容器之外。
func TestInlineReplacedTallerThanTextStaysInsideLineBox(t *testing.T) {
	restore := setupSkiaMeasure()
	defer restore()

	doc := dom.NewDocument()
	host := doc.CreateElement("div")
	host.SetAttribute("style", "width:386px;font-family:sans-serif;font-size:13px;line-height:1.5")
	wrap := doc.CreateElement("div")
	wrap.SetAttribute("class", "logo-wrap")
	img := doc.CreateElement("img")
	// 与 AboutModal 的 .about-logo-img 同尺寸（无 margin）。
	img.SetAttribute("style", "width:64px;height:64px")
	wrap.AppendChild(img)
	host.AppendChild(wrap)
	doc.AppendChild(host)

	resolver := style.NewResolver()
	root := BuildLayoutTree(host, resolver)
	if root == nil {
		t.Fatal("root nil")
	}
	state := Layout(root.(*ElementBox), 386, 600)
	if state == nil {
		t.Fatal("state nil")
	}

	wrapBox := findBoxByClass(t, root, "logo-wrap")
	if wrapBox == nil {
		t.Fatal("logo-wrap box not found")
	}
	var imgBox *ElementBox
	var walk func(b Box)
	walk = func(b Box) {
		if eb, ok := b.(*ElementBox); ok {
			if eb.Element() != nil && eb.Element().LocalName() == "img" && imgBox == nil {
				imgBox = eb
			}
			for _, c := range eb.Children() {
				walk(c)
			}
		}
	}
	walk(root)
	if imgBox == nil {
		t.Fatal("img box not found")
	}

	wg := state.GeometryForBox(wrapBox)
	ig := state.GeometryForBox(imgBox)

	// 行盒高度必须容纳替换元素（这条修复前后都成立，作为场景前提钉住）。
	if math.Abs(wg.BorderBoxHeight()-64) > 0.5 {
		t.Fatalf("wrap height = %.2f, want 64（行盒未计入替换元素高度）", wg.BorderBoxHeight())
	}
	// ★ 核心断言：图片顶边相对父内容顶的偏移必须为 0。
	//   修复前 = -50（图片底边坐文本基线 → 整体上浮 50px）。
	if off := ig.Top() - wg.BorderTop(); math.Abs(off) > 0.5 {
		t.Fatalf("img top offset inside wrap = %.2f, want 0（负值 = 图片浮出容器上方，AboutModal logo 上移 50px 的根因）", off)
	}
}

// TestInlineReplacedSmallerThanTextKeepsTextBaseline 钉住：矮于文本行的替换
// 元素（8px svg 图标）必须仍按「文本基线」定位 —— 修复不得改变这类既有正确
// 行为（.qexec-caret 的 8px svg 曾在 1280x800 下校正到 y=15.88 ≈ 浏览器 16.0）。
func TestInlineReplacedSmallerThanTextKeepsTextBaseline(t *testing.T) {
	restore := setupSkiaMeasure()
	defer restore()

	doc := dom.NewDocument()
	host := doc.CreateElement("div")
	host.SetAttribute("style", "width:400px;font-family:sans-serif;font-size:15px;line-height:15px")
	wrap := doc.CreateElement("div")
	wrap.SetAttribute("class", "icon-wrap")
	svg := doc.CreateElement("svg")
	svg.SetAttribute("style", "width:8px;height:8px")
	wrap.AppendChild(svg)
	host.AppendChild(wrap)
	doc.AppendChild(host)

	resolver := style.NewResolver()
	root := BuildLayoutTree(host, resolver)
	if root == nil {
		t.Fatal("root nil")
	}
	state := Layout(root.(*ElementBox), 400, 600)
	if state == nil {
		t.Fatal("state nil")
	}

	wrapBox := findBoxByClass(t, root, "icon-wrap")
	if wrapBox == nil {
		t.Fatal("icon-wrap box not found")
	}
	var svgBox *ElementBox
	var walk func(b Box)
	walk = func(b Box) {
		if eb, ok := b.(*ElementBox); ok {
			if eb.Element() != nil && eb.Element().LocalName() == "svg" && svgBox == nil {
				svgBox = eb
			}
			for _, c := range eb.Children() {
				walk(c)
			}
		}
	}
	walk(root)
	if svgBox == nil {
		t.Fatal("svg box not found")
	}

	wg := state.GeometryForBox(wrapBox)
	sg := state.GeometryForBox(svgBox)

	// 8px 图标矮于文本行（15px），必须被文本基线兜住：顶边下移量在 (0, 15) 内
	// 且不为 0（= 贴行盒顶的旧缺陷）。
	if off := sg.Top() - wg.BorderTop(); off <= 0.5 || off >= 15 {
		t.Fatalf("small svg top offset = %.2f, want (0,15) —— 应按文本基线定位，不受高替换元素修复影响", off)
	}
}
