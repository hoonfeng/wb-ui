package layout

// 礼物卡片（.gitem 内 img + b 文字）垂直对齐回归测试。
// 缺陷：flex item 内的 inline 文本（b.gd）未按 half-leading 居中，且
// line-height < 字体度量高时负半行距被静默归零（textHeight < cssLH
// 限制）——文字中心比图标中心低 (textHeight-cssLH)/2（21px 字体、
// line-height:1.2 时实测 1.26px，「礼物图标与 +N分钟 文字不齐」根因）。
// 修复：flex item（及父是 flex item 的匿名内容层）排除 plain-inline
// 分支 + 允许负半行距。

import (
	"math"
	"testing"

	"wb-ui/dom"
	"wb-ui/style"
)

func findBoxByClass(t *testing.T, root Box, class string) *ElementBox {
	t.Helper()
	var found *ElementBox
	var walk func(b Box)
	walk = func(b Box) {
		if eb, ok := b.(*ElementBox); ok {
			if eb.Element() != nil && eb.Element().GetAttribute("class") == class {
				if found == nil {
					found = eb
				}
			}
			for _, c := range eb.Children() {
				walk(c)
			}
		}
	}
	walk(root)
	return found
}

func TestGiftItemIconTextVerticalAlign(t *testing.T) {
	restore := setupSkiaMeasure()
	defer restore()

	doc := dom.NewDocument()
	host := doc.CreateElement("div")
	host.SetAttribute("style", "width:500px;font-family:sans-serif")
	gitem := doc.CreateElement("span")
	gitem.SetAttribute("class", "gitem")
	gitem.SetAttribute("style", "display:inline-flex;align-items:center;font-size:21px;line-height:1.2")
	img := doc.CreateElement("img")
	img.SetAttribute("style", "width:18px;height:18px")
	b := doc.CreateElement("b")
	b.SetAttribute("style", "font-weight:700")
	b.AppendChild(doc.CreateTextNode("+1分钟"))
	gitem.AppendChild(img)
	gitem.AppendChild(b)
	host.AppendChild(gitem)
	doc.AppendChild(host)

	resolver := style.NewResolver()
	root := BuildLayoutTree(host, resolver)
	if root == nil {
		t.Fatal("root nil")
	}
	state := Layout(root.(*ElementBox), 500, 600)
	if state == nil {
		t.Fatal("state nil")
	}

	gBox := findBoxByClass(t, root, "gitem")
	if gBox == nil {
		t.Fatal("gitem box not found")
	}
	var imgBox *ElementBox
	var imgCenterY float64
	var textCenterY float64
	var textFound bool
	var findImgAndText func(b Box)
	findImgAndText = func(b Box) {
		if eb, ok := b.(*ElementBox); ok {
			if eb.Element() != nil && eb.Element().LocalName() == "img" {
				imgBox = eb
				g := state.GeometryForBox(eb)
				imgCenterY = g.Top() + g.BorderBoxHeight()/2
			}
			for _, c := range eb.Children() {
				findImgAndText(c)
			}
		}
		if tb, ok := b.(*InlineTextBox); ok && len(tb.TextSegments) > 0 {
			textFound = true
			textCenterY = tb.TextSegments[0].Y + tb.TextSegments[0].Height/2
		}
	}
	findImgAndText(gBox)

	if imgBox == nil {
		t.Fatal("img box not found")
	}
	if !textFound {
		t.Fatal("text segments not found")
	}
	t.Logf("img centerY=%.2f text centerY=%.2f (want equal)", imgCenterY, textCenterY)
	// 装配差异（无 UA 时行盒 25.0 vs cssLH 25.2 的 0.2 取整）实测 0.36px；
	// 真实缺陷（半行距未分配）为 1.26px——0.6 阈值可稳定抓回归。
	if d := math.Abs(imgCenterY - textCenterY); d > 0.6 {
		t.Fatalf("icon/text center mismatch: %.3f px (>0.6) — 礼物图标与文字不垂直对齐缺陷回归", d)
	}

	// 半行距语义：cssLH(25.2) < textHeight(字体度量) 时文字中心应等于
	// 盒中心（允许负半行距），不得停留在盒顶。
	_ = style.DisplayInline
}
