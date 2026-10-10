package style

import (
	"strings"
	"testing"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
)

// uaSheet 用 UA origin 解析一段样式表，模拟 engine/html5/defaultcss.go 注入的
// 用户代理样式（origin 更低、源序更靠前 → 先于作者声明应用）。
func uaSheet(t *testing.T, input string) *css.CSSStyleSheet {
	t.Helper()
	sheet := css.NewCSSStyleSheet()
	p := css.NewParserWithOrigin(input, css.OriginUserAgent)
	p.ParseStyleSheetInto(sheet)
	sheet.SetOrigin(css.OriginUserAgent)
	return sheet
}

// TestResolver_FontFamilyInheritOverridesUAControlFont 锁定 MediaStudio
// `#btn-record` 按钮高度差的样式侧根因。实测高度（laydump 1280x860，同树同命令，
// 仅把 resolver.go 切到本修复前的版本）：**修复前 204x34 → 修复后 204x37**（Edge 204x36）。
//
// ★ 本文曾写「wb-ui 38 / Edge 36」：38 是更早的 D6 缺陷（UA 控件缺
// `line-height: normal`，见 docs/render-consistency-fixes.md）时期的数字，**已作废**。
//
//	UA 表（defaultcss.go）有 `input, button, select, textarea { font-family: Arial }`，
//	作者的 `.btn { font-family: inherit }` 排在它**之后**应用。修复前
//	applyDeclaration 对 inherit 只 `break`（注释称"保留当前字段"），而字段此时
//	已被 UA 声明写成 Arial —— 作者声明静默失效，控件用 Arial 的 normal 行高
//	（20px @13.33px）而非父元素 system-ui 的（17px）。
//
// 期望：`font-family: inherit` = 父元素计算值（CSS Cascade §7.3）。
func TestResolver_FontFamilyInheritOverridesUAControlFont(t *testing.T) {
	doc := dom.NewDocument()
	div := dom.NewElement(doc, "div")
	btn := dom.NewElement(doc, "button")
	if err := div.AppendChild(btn); err != nil {
		t.Fatal(err)
	}
	r := NewResolver()
	// UA 规则照抄 defaultcss.go；`font-size: 13.3333px` 一并给出，保证字体族
	// 与字号同处一组 CSS 通配声明（浏览器的 html.css 也是 `font: 400 13.3px Arial`）。
	r.AddStyleSheet(uaSheet(t, "input, button, select, textarea { font-family: Arial; font-size: 13.3333px; }"))
	r.AddStyleSheet(newSheet(t, "div { font-family: monospace; } button { font-family: inherit; }"))

	parent := r.ResolveElement(div)
	if parent.FontFamily != "monospace" {
		t.Fatalf("父元素 font-family=%q want monospace（前置条件失败）", parent.FontFamily)
	}
	if got := r.ResolveElement(btn).FontFamily; got != "monospace" {
		t.Fatalf("button font-family=%q want monospace（应继承父元素，而非 UA 的 Arial）", got)
	}
}

// TestResolver_FontFamilyInheritWithoutUA：没有 UA 控件字体声明时，inherit 同样
// 取父元素计算值（applyDeclaration 写入字面量后由 resolveInheritKeyword 收敛）。
func TestResolver_FontFamilyInheritWithoutUA(t *testing.T) {
	doc := dom.NewDocument()
	div := dom.NewElement(doc, "div")
	btn := dom.NewElement(doc, "button")
	if err := div.AppendChild(btn); err != nil {
		t.Fatal(err)
	}
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "div { font-family: 'Courier New', monospace; } button { font-family: inherit; }"))
	// 与父元素的计算值逐字比较：inherit 必须搬到**父的计算值**，而不是把作者
	// 声明里的字面量搬过来（applyDeclaration 对 font-family 会剥掉首尾引号，
	// 见 case "font-family" 的 strings.Trim —— 那是既有行为，不在本测试范围内，
	// 故不硬编码期望字符串）。
	parentVal := r.ResolveElement(div).FontFamily
	if parentVal == "" || parentVal == "inherit" {
		t.Fatalf("前置条件失败：父元素 font-family=%q", parentVal)
	}
	if !strings.Contains(parentVal, "monospace") {
		t.Fatalf("前置条件失败：父元素 font-family=%q 应含 monospace 回退项", parentVal)
	}
	if got := r.ResolveElement(btn).FontFamily; got != parentVal {
		t.Fatalf("button font-family=%q want 父元素计算值 %q", got, parentVal)
	}
}

// TestResolver_InheritOnRootFallsBackToInitial：根元素上的 `inherit` 无父可继承，
// 落到 CSS 初始值 sans-serif（CSS Fonts 3 §2.1）。
func TestResolver_InheritOnRootFallsBackToInitial(t *testing.T) {
	doc := dom.NewDocument()
	root := dom.NewElement(doc, "html")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "html { font-family: inherit; }"))
	if got := r.ResolveElement(root).FontFamily; got != "sans-serif" {
		t.Fatalf("root font-family=%q want sans-serif（初始值）", got)
	}
}

// TestResolver_FontFamilyInheritChain：多级祖先链上 inherit 逐级上溯到最近祖先的
// 计算值（而非直接父元素的声明文本）。
func TestResolver_FontFamilyInheritChain(t *testing.T) {
	doc := dom.NewDocument()
	a := dom.NewElement(doc, "div")
	b := dom.NewElement(doc, "div")
	c := dom.NewElement(doc, "button")
	if err := a.AppendChild(b); err != nil {
		t.Fatal(err)
	}
	if err := b.AppendChild(c); err != nil {
		t.Fatal(err)
	}
	a.SetId("a")
	r := NewResolver()
	// a 自己显式声明 monospace（id 选择器压过 div 的 inherit），其余两级靠 inherit 上溯。
	r.AddStyleSheet(newSheet(t, "#a { font-family: monospace; } div { font-family: inherit; } button { font-family: inherit; }"))
	if got := r.ResolveElement(c).FontFamily; got != "monospace" {
		t.Fatalf("button font-family=%q want monospace（沿链继承）", got)
	}
}
