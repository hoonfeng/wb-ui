package style

import (
	"testing"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
)

// TestScrollbarColorInheritance：`scrollbar-color` / `scrollbar-width` 是
// **可继承**属性（CSS Scrollbars Styling §2.2 / §3.1），但它们在 wb-ui 里存在
// ComputedStyle.Properties map 中（不在 InheritedData 结构体里），ResolveElement
// 必须显式下发，否则子元素永远读到空值。
//
// 这条链正是 gou-ide 的真实路径：index.html 给 html 设了
// `scrollbar-color: var(--scrollbar-thumb, #6e7681) transparent`，Chromium 把它
// 下发到全页每个滚动容器。漏掉继承会导致：
//   - 拿不到 thumb/track 配色（滚动条回落到写死的浅灰）；
//   - 压不住 ::-webkit-scrollbar 自定义宽度 → 滚动条被画成 9px 细条无箭头，
//     而浏览器是 17px 经典样式（带上下箭头、thumb 宽 14 居中）。
//   实测（2026-09-26）：修复前 .pp-list 的 diag 是
//   `scrollW=9 webkitSB=true sbc=""`，修复后为
//   `scrollW=17 webkitSB=false sbc="#414b64 transparent"`。
func TestScrollbarColorInheritance(t *testing.T) {
	sheet := css.NewCSSStyleSheet()
	p := css.NewParser(`
		html { scrollbar-color: #414b64 transparent; scrollbar-width: thin; }
		.opt-out { scrollbar-color: auto; }
		.thin-website::-webkit-scrollbar { width: 4px; }
	`)
	p.ParseStyleSheetInto(sheet)

	r := NewResolver()
	r.AddStyleSheet(sheet)

	doc := dom.NewDocument()
	htmlEl := dom.NewElement(doc, "html")
	body := dom.NewElement(doc, "body")
	htmlEl.AppendChild(body)

	plain := dom.NewElement(doc, "div")
	plain.SetAttribute("class", "thin-website")
	body.AppendChild(plain)

	optOut := dom.NewElement(doc, "div")
	optOut.SetAttribute("class", "opt-out")
	body.AppendChild(optOut)

	htmlCS := r.ResolveElement(htmlEl)
	if got := htmlCS.GetProperty("scrollbar-color"); got != "#414b64 transparent" {
		t.Fatalf("html scrollbar-color = %q, want %q", got, "#414b64 transparent")
	}

	// ① 继承：普通后代应拿到 html 的值。
	plainCS := r.ResolveElement(plain)
	if got := plainCS.GetProperty("scrollbar-color"); got != "#414b64 transparent" {
		t.Fatalf("继承失效：后代 scrollbar-color = %q, want %q", got, "#414b64 transparent")
	}
	if got := plainCS.GetProperty("scrollbar-width"); got != "thin" {
		t.Fatalf("继承失效：后代 scrollbar-width = %q, want thin", got)
	}
	if !HasCustomScrollbarColor(plainCS) {
		t.Fatalf("HasCustomScrollbarColor = false，但后代继承到了非 auto 的 scrollbar-color")
	}

	// ② 继承 + ::-webkit-scrollbar{width:4px}：Chromium 会**忽略** webkit 规则，
	//    宽度回退平台经典 17px；scrollbar-width:thin 优先于 webkit 声明 → 11px。
	if got := ScrollbarWidth(plainCS); got != ThinScrollbarWidth {
		t.Fatalf("ScrollbarWidth = %v, want %v（thin 优先于被压制的 ::-webkit-scrollbar）", got, ThinScrollbarWidth)
	}

	// ③ 显式 scrollbar-color:auto 时，::-webkit-scrollbar{width:4px} 才生效。
	optCS := r.ResolveElement(optOut)
	if got := optCS.GetProperty("scrollbar-color"); got != "auto" {
		t.Fatalf("显式 auto 未生效：%q", got)
	}
	if HasCustomScrollbarColor(optCS) {
		t.Fatalf("scrollbar-color:auto 不应被当作自定义配色")
	}
	// 注意：它同时也继承了 html 的 scrollbar-width:thin，而标准属性优先于
	// ::-webkit-scrollbar 声明（Chrome 同序）→ 仍是 11，而不是 webkit 的 4。
	if got := ScrollbarWidth(optCS); got != ThinScrollbarWidth {
		t.Fatalf("scrollbar-color:auto + 继承 thin + ::-webkit-scrollbar{4px} → %v, want %v", got, ThinScrollbarWidth)
	}

	// ④ 元素自身的声明必须覆盖继承值（InheritFrom 在应用自身声明之前调用）。
	if got := r.ResolveElement(optOut).GetProperty("scrollbar-color"); got == "#414b64 transparent" {
		t.Fatalf("元素自身声明被继承值覆盖（InheritFrom 调用顺序错误）")
	}

	// ⑤ 干净场景（父链上没有 scrollbar-color / scrollbar-width）：
	//    显式 scrollbar-color:auto 时 ::-webkit-scrollbar{width:4px} 生效 → 4px。
	sheet2 := css.NewCSSStyleSheet()
	p2 := css.NewParser(`
		html { }
		.thin-4::-webkit-scrollbar { width: 4px; }
	`)
	p2.ParseStyleSheetInto(sheet2)
	r2 := NewResolver()
	r2.AddStyleSheet(sheet2)
	doc2 := dom.NewDocument()
	html2 := dom.NewElement(doc2, "html")
	body2 := dom.NewElement(doc2, "body")
	html2.AppendChild(body2)
	el4 := dom.NewElement(doc2, "div")
	el4.SetAttribute("class", "thin-4")
	body2.AppendChild(el4)
	cs4 := r2.ResolveElement(el4)
	if HasCustomScrollbarColor(cs4) {
		t.Fatalf("无 scrollbar-color 声明时不应判定为自定义配色：%q", cs4.GetProperty("scrollbar-color"))
	}
	if got := ScrollbarWidth(cs4); got != 4 {
		t.Fatalf("::-webkit-scrollbar{width:4px} → %v, want 4", got)
	}
}

// TestScrollbarWidthDefaults 对齐 Chromium/Windows 的实测宽度三态。
func TestScrollbarWidthDefaults(t *testing.T) {
	cases := []struct {
		name  string
		props map[string]string
		want  float64
	}{
		{"默认（scrollbar-color 自绘）", nil, 15},
		{"thin", map[string]string{"scrollbar-width": "thin"}, 10},
		{"none", map[string]string{"scrollbar-width": "none"}, 0},
		{"webkit 自定义（无 scrollbar-color）", map[string]string{"-webkit-scrollbar-width": "8px"}, 8},
		{"非 auto scrollbar-color 压制 webkit", map[string]string{
			"scrollbar-color": "#414b64 transparent", "-webkit-scrollbar-width": "8px"}, 15},
		{"scrollbar-color:auto 放行 webkit", map[string]string{
			"scrollbar-color": "auto", "-webkit-scrollbar-width": "8px"}, 8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs := &ComputedStyle{}
			if tc.props != nil {
				cs.Properties = map[string]string{}
				for k, v := range tc.props {
					cs.Properties[k] = v
				}
			}
			if got := ScrollbarWidth(cs); got != tc.want {
				t.Fatalf("ScrollbarWidth = %v, want %v", got, tc.want)
			}
		})
	}
}
