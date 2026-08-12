package style

import (
	"strings"
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
)

func newSheet(t *testing.T, input string) *css.CSSStyleSheet {
	t.Helper()
	sheet := css.NewCSSStyleSheet()
	p := css.NewParser(input)
	p.ParseStyleSheetInto(sheet)
	return sheet
}

func TestResolver_BasicStyleRule(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { color: red; }"))
	cs := r.ResolveElement(el)
	if cs.Color.R != 255 || cs.Color.G != 0 || cs.Color.B != 0 {
		t.Fatalf("color=%v want red", cs.Color)
	}
}

func TestResolver_SpecificityCascade(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	el.SetId("main")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { color: red; } #main { color: blue; }"))
	cs := r.ResolveElement(el)
	if cs.Color.B != 255 || cs.Color.R != 0 {
		t.Fatalf("color=%v want blue (id specificity wins)", cs.Color)
	}
}

func TestResolver_SourceOrder(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { color: red; } p { color: blue; }"))
	cs := r.ResolveElement(el)
	if cs.Color.B != 255 {
		t.Fatalf("color=%v want blue (later rule wins)", cs.Color)
	}
}

func TestResolver_Inheritance(t *testing.T) {
	doc := dom.NewDocument()
	parent := dom.NewElement(doc, "div")
	child := dom.NewElement(doc, "p")
	if err := parent.AppendChild(child); err != nil {
		t.Fatal(err)
	}
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "div { color: green; }"))
	cs := r.ResolveElement(child)
	if cs.Color.G != 255 {
		t.Fatalf("color=%v want green inherited from div", cs.Color)
	}
}

func TestResolver_ImportantOverride(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { color: blue !important; } p { color: red; }"))
	cs := r.ResolveElement(el)
	if cs.Color.B != 255 {
		t.Fatalf("color=%v want blue (important wins)", cs.Color)
	}
}

func TestResolver_InlineStyle(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	el.SetAttribute("style", "color: blue;")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { color: red; } #main { color: green; }"))
	cs := r.ResolveElement(el)
	if cs.Color.B != 255 {
		t.Fatalf("color=%v want blue (inline wins)", cs.Color)
	}
}

func TestResolver_NonMatchingSelector(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { color: red; }"))
	cs := r.ResolveElement(el)
	if cs.Color.R != 0 || cs.Color.G != 0 || cs.Color.B != 0 {
		t.Fatalf("color=%v want default black (no match)", cs.Color)
	}
}

func TestResolver_CustomPropertyVarResolution(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { --main: red; --accent: var(--main); }"))
	cs := r.ResolveElement(el)
	main := cs.GetCustomProperty("--main")
	if len(main) == 0 || main[0].Value != "red" {
		t.Fatalf("--main=%v want red", main)
	}
	accent := cs.GetCustomProperty("--accent")
	if len(accent) == 0 || accent[0].Value != "red" {
		t.Fatalf("--accent=%v want resolved red", accent)
	}
}

func TestResolver_DisplayProperty(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "div { display: flex; }"))
	cs := r.ResolveElement(el)
	if cs.Display != DisplayFlex {
		t.Fatalf("display=%v want flex", cs.Display)
	}
}

func TestResolver_BorderShorthand(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "div { border: 1px solid #e5e7eb; }"))
	cs := r.ResolveElement(el)
	if cs.BorderTopStyle != "solid" {
		t.Fatalf("BorderTopStyle=%q want solid", cs.BorderTopStyle)
	}
	if cs.BorderTopWidth.Value != 1 || cs.BorderTopWidth.Unit != "px" {
		t.Fatalf("BorderTopWidth=%+v want 1px", cs.BorderTopWidth)
	}
	if cs.BorderBottomStyle != "solid" {
		t.Fatalf("BorderBottomStyle=%q want solid", cs.BorderBottomStyle)
	}
}

func TestResolver_BorderBottomShorthand(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "div { border-bottom: 2px solid #e5e7eb; }"))
	cs := r.ResolveElement(el)
	if cs.BorderBottomStyle != "solid" {
		t.Fatalf("BorderBottomStyle=%q want solid", cs.BorderBottomStyle)
	}
	if cs.BorderBottomWidth.Value != 2 || cs.BorderBottomWidth.Unit != "px" {
		t.Fatalf("BorderBottomWidth=%+v want 2px", cs.BorderBottomWidth)
	}
}

func TestResolver_ResolveDocument(t *testing.T) {
	doc := dom.NewDocument()
	root := dom.NewElement(doc, "html")
	body := dom.NewElement(doc, "body")
	p := dom.NewElement(doc, "p")
	if err := root.AppendChild(body); err != nil {
		t.Fatal(err)
	}
	if err := body.AppendChild(p); err != nil {
		t.Fatal(err)
	}
	if err := doc.AppendChild(root); err != nil {
		t.Fatal(err)
	}
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "body { color: red; }"))
	out := r.ResolveDocument(doc)
	if len(out) == 0 {
		t.Fatalf("ResolveDocument returned empty map")
	}
	if _, ok := out[body]; !ok {
		t.Fatalf("body not in resolved map")
	}
	if out[body].Color.R != 255 {
		t.Fatalf("body color=%v want red", out[body].Color)
	}
}

func TestResolver_FontSize(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { font-size: 16px; }"))
	cs := r.ResolveElement(el)
	if cs.FontSize.Value != 16 || cs.FontSize.Unit != "px" {
		t.Fatalf("font-size=%+v want 16px", cs.FontSize)
	}
}

func TestResolver_FontFamilyAndWeight(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { font-family: Arial, sans-serif; font-weight: bold; }"))
	cs := r.ResolveElement(el)
	// The resolver stores the raw value string which may contain spacing variations
	if !strings.Contains(cs.FontFamily, "Arial") || !strings.Contains(cs.FontFamily, "sans-serif") {
		t.Fatalf("font-family=%q should contain Arial and sans-serif", cs.FontFamily)
	}
	if cs.FontWeight != "bold" {
		t.Fatalf("font-weight=%q want bold", cs.FontWeight)
	}
}

func TestResolver_LineHeightAndLetterSpacing(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { line-height: 1.5; letter-spacing: 0.5px; }"))
	cs := r.ResolveElement(el)
	if cs.LineHeight.Value != 1.5 || cs.LineHeight.Unit != "" {
		t.Fatalf("line-height=%+v want 1.5", cs.LineHeight)
	}
	if cs.LetterSpacing.Value != 0.5 || cs.LetterSpacing.Unit != "px" {
		t.Fatalf("letter-spacing=%+v want 0.5px", cs.LetterSpacing)
	}
}

// TestResolver_LineHeightNormal: line-height: normal 解析为 Unit="normal"
//（字体度量语义，浏览器标准），而非落默认 1.2 倍——xterm 的
// .xterm-rows/.xterm-char-measure-element 用 normal，错了行高差 1px。
func TestResolver_LineHeightNormal(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { line-height: normal; }"))
	cs := r.ResolveElement(el)
	if cs.LineHeight.Unit != "normal" {
		t.Fatalf("line-height=%+v want Unit=normal（字体度量语义）", cs.LineHeight)
	}
}

func TestResolver_TextAlignAndTransform(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { text-align: center; text-transform: uppercase; }"))
	cs := r.ResolveElement(el)
	if cs.TextAlign != TextAlignCenter {
		t.Fatalf("text-align=%v want center", cs.TextAlign)
	}
	if cs.TextTransform != "uppercase" {
		t.Fatalf("text-transform=%q want uppercase", cs.TextTransform)
	}
}

func TestResolver_TextDecoration(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { text-decoration: underline; }"))
	cs := r.ResolveElement(el)
	if cs.TextDecoration != "underline" {
		t.Fatalf("text-decoration=%q want underline", cs.TextDecoration)
	}
}

func TestResolver_WhiteSpace(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "p { white-space: pre-wrap; }"))
	cs := r.ResolveElement(el)
	if cs.WhiteSpace != WhiteSpacePreWrap {
		t.Fatalf("white-space=%v want pre-wrap", cs.WhiteSpace)
	}
}

func TestResolver_MarginShorthand4(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "div { margin: 10px 20px 5px 15px; }"))
	cs := r.ResolveElement(el)
	checkLen(t, "margin-top", cs.MarginTop, 10, "px")
	checkLen(t, "margin-right", cs.MarginRight, 20, "px")
	checkLen(t, "margin-bottom", cs.MarginBottom, 5, "px")
	checkLen(t, "margin-left", cs.MarginLeft, 15, "px")
}

func TestResolver_MarginShorthand2(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "div { margin: 20px 10px; }"))
	cs := r.ResolveElement(el)
	checkLen(t, "margin-top", cs.MarginTop, 20, "px")
	checkLen(t, "margin-bottom", cs.MarginBottom, 20, "px")
	checkLen(t, "margin-right", cs.MarginRight, 10, "px")
	checkLen(t, "margin-left", cs.MarginLeft, 10, "px")
}

func TestResolver_PaddingShorthand3(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "div { padding: 5px 10px 15px; }"))
	cs := r.ResolveElement(el)
	checkLen(t, "padding-top", cs.PaddingTop, 5, "px")
	checkLen(t, "padding-right", cs.PaddingRight, 10, "px")
	checkLen(t, "padding-bottom", cs.PaddingBottom, 15, "px")
	checkLen(t, "padding-left", cs.PaddingLeft, 10, "px")
}

func TestResolver_PaddingShorthand1(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "div { padding: 8px; }"))
	cs := r.ResolveElement(el)
	checkLen(t, "padding-top", cs.PaddingTop, 8, "px")
	checkLen(t, "padding-right", cs.PaddingRight, 8, "px")
	checkLen(t, "padding-bottom", cs.PaddingBottom, 8, "px")
	checkLen(t, "padding-left", cs.PaddingLeft, 8, "px")
}

func TestResolver_WidthAndHeight(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `div { width: 200px; height: 100px; }`))
	cs := r.ResolveElement(el)
	checkLen(t, "width", cs.Width, 200, "px")
	checkLen(t, "height", cs.Height, 100, "px")
}

func TestResolver_MinMaxWidth(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `div { min-width: 50px; max-width: 500px; }`))
	cs := r.ResolveElement(el)
	checkLen(t, "min-width", cs.MinWidth, 50, "px")
	checkLen(t, "max-width", cs.MaxWidth, 500, "px")
}

func TestResolver_BorderRadius(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `div { border-radius: 8px; }`))
	cs := r.ResolveElement(el)
	checkLen(t, "border-radius", cs.BorderRadius, 8, "px")
}

func TestResolver_BoxSizing(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `div { box-sizing: border-box; }`))
	cs := r.ResolveElement(el)
	if cs.BoxSizing != "border-box" {
		t.Fatalf("box-sizing=%q want border-box", cs.BoxSizing)
	}
}

func TestResolver_OpacityAndVisibility(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `div { opacity: 0.5; visibility: hidden; }`))
	cs := r.ResolveElement(el)
	if cs.Opacity != 0.5 {
		t.Fatalf("opacity=%v want 0.5", cs.Opacity)
	}
	if cs.Visibility != "hidden" {
		t.Fatalf("visibility=%q want hidden", cs.Visibility)
	}
}

func TestResolver_ZIndex(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `div { z-index: 10; position: absolute; }`))
	cs := r.ResolveElement(el)
	if cs.ZIndex != 10 {
		t.Fatalf("z-index=%d want 10", cs.ZIndex)
	}
	if cs.Position != PositionAbsolute {
		t.Fatalf("position=%v want absolute", cs.Position)
	}
}

func TestResolver_FlexProperties(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `div { display: flex; flex-direction: column; flex-wrap: wrap; gap: 10px; }`))
	cs := r.ResolveElement(el)
	if cs.Display != DisplayFlex {
		t.Fatalf("display=%v want flex", cs.Display)
	}
	if cs.FlexDirection != "column" {
		t.Fatalf("flex-direction=%q want column", cs.FlexDirection)
	}
	if cs.FlexWrap != "wrap" {
		t.Fatalf("flex-wrap=%q want wrap", cs.FlexWrap)
	}
	checkLen(t, "gap", cs.Gap, 10, "px")
}

func TestResolver_FlexItemProperties(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `div { flex: 1 0 auto; flex-grow: 2; flex-shrink: 1; flex-basis: 100px; order: 3; }`))
	cs := r.ResolveElement(el)
	if cs.FlexGrow != 2 {
		t.Fatalf("flex-grow=%v want 2", cs.FlexGrow)
	}
	if cs.FlexShrink != 1 {
		t.Fatalf("flex-shrink=%v want 1", cs.FlexShrink)
	}
	checkLen(t, "flex-basis", cs.FlexBasis, 100, "px")
	if cs.Order != 3 {
		t.Fatalf("order=%d want 3", cs.Order)
	}
}

func TestResolver_Overflow(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `div { overflow: hidden; }`))
	cs := r.ResolveElement(el)
	if cs.OverflowX != OverflowHidden || cs.OverflowY != OverflowHidden {
		t.Fatalf("overflow=%v/%v want hidden/hidden", cs.OverflowX, cs.OverflowY)
	}
}

func TestResolver_OverflowScroll(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `div { overflow-x: scroll; overflow-y: auto; }`))
	cs := r.ResolveElement(el)
	if cs.OverflowX != OverflowScroll {
		t.Fatalf("overflow-x=%v want scroll", cs.OverflowX)
	}
	if cs.OverflowY != OverflowAuto {
		t.Fatalf("overflow-y=%v want auto", cs.OverflowY)
	}
}

func TestResolver_BackgroundColor(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `div { background-color: #ff0; }`))
	cs := r.ResolveElement(el)
	if cs.BackgroundColor.R != 0xFF || cs.BackgroundColor.G != 0xFF || cs.BackgroundColor.B != 0x00 {
		t.Fatalf("background-color=%v want #ffff00", cs.BackgroundColor)
	}
}

func TestResolver_Direction(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `div { direction: rtl; }`))
	cs := r.ResolveElement(el)
	if cs.Direction != "rtl" {
		t.Fatalf("direction=%q want rtl", cs.Direction)
	}
}

func TestResolver_ClassSelectorMatch(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	el.SetAttribute("class", "highlight")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `.highlight { background-color: yellow; }`))
	cs := r.ResolveElement(el)
	if cs.BackgroundColor.R != 0xFF || cs.BackgroundColor.G != 0xFF || cs.BackgroundColor.B != 0x00 {
		t.Fatalf("background-color=%v want yellow", cs.BackgroundColor)
	}
}

func TestResolver_AttributeSelector(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "input")
	el.SetAttribute("type", "text")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `input[type="text"] { border: 1px solid black; }`))
	cs := r.ResolveElement(el)
	if cs.BorderTopWidth.Value != 1 {
		t.Fatalf("border-top-width=%+v want 1px", cs.BorderTopWidth)
	}
	if cs.BorderTopStyle != "solid" {
		t.Fatalf("border-top-style=%q want solid", cs.BorderTopStyle)
	}
}

func TestResolver_ChildCombinator(t *testing.T) {
	doc := dom.NewDocument()
	parent := dom.NewElement(doc, "div")
	child := dom.NewElement(doc, "p")
	_ = parent.AppendChild(child)
	_ = doc.AppendChild(parent)
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `div > p { color: red; }`))
	cs := r.ResolveElement(child)
	if cs.Color.R != 255 || cs.Color.G != 0 || cs.Color.B != 0 {
		t.Fatalf("color=%v want red (child combinator)", cs.Color)
	}
}

func TestResolver_DescendantCombinator(t *testing.T) {
	doc := dom.NewDocument()
	grandparent := dom.NewElement(doc, "div")
	parent := dom.NewElement(doc, "section")
	child := dom.NewElement(doc, "p")
	_ = parent.AppendChild(child)
	_ = grandparent.AppendChild(parent)
	_ = doc.AppendChild(grandparent)
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `div p { color: blue; }`))
	cs := r.ResolveElement(child)
	if cs.Color.B != 255 {
		t.Fatalf("color=%v want blue (descendant combinator)", cs.Color)
	}
}

func TestResolver_MultipleStyleSheets(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `p { color: red; }`))
	r.AddStyleSheet(newSheet(t, `p { font-size: 14px; }`))
	cs := r.ResolveElement(el)
	if cs.Color.R != 255 {
		t.Fatalf("color=%v want red", cs.Color)
	}
	checkLen(t, "font-size", cs.FontSize, 14, "px")
}

func TestResolver_DisplayNone(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `div { display: none; }`))
	cs := r.ResolveElement(el)
	if cs.Display != DisplayNone {
		t.Fatalf("display=%v want none", cs.Display)
	}
}

func TestResolver_EmptyStyleSheet(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	r := NewResolver()
	// No stylesheets added; should return default computed style
	cs := r.ResolveElement(el)
	if cs.Color.R != 0 || cs.Color.G != 0 || cs.Color.B != 0 {
		t.Fatalf("color=%v want default black (0,0,0)", cs.Color)
	}
	if cs.Display != DisplayBlock {
		t.Fatalf("display=%v want DisplayBlock (p is a block element by default)", cs.Display)
	}
}

func TestResolver_PseudoClassHover(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "a")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `a:hover { color: red; }`))
	// :hover is a dynamic pseudo-class; resolver should match it when the element
	// has the corresponding state set
	el.SetHovered(true)
	cs := r.ResolveElement(el)
	if cs.Color.R != 255 {
		t.Fatalf("a:hover color=%v want red", cs.Color)
	}
}

// TestResolver_HoverClearedOnCursorLeave 验证「鼠标移出窗口清除 hover」的
// 修复语义（t3）：SetHovered(false) 后 resolver 不再把 :hover 样式应用到
// 元素——agent 输出触发渲染树重建时，若 hover 残留未清，:hover 会被错误
// 应用（无交互画面却出现 hover 高亮/按钮变色）。清除后重建必须恢复正常。
func TestResolver_HoverClearedOnCursorLeave(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "a")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `a:hover { color: red; }`))

	// 悬停：:hover 生效
	el.SetHovered(true)
	if cs := r.ResolveElement(el); cs.Color.R != 255 {
		t.Fatalf("hovered 时 color=%v want red", cs.Color)
	}
	// 模拟鼠标移出窗口（EventCursorLeave → SetHovered(false)）：
	// 重建后 :hover 必须不再应用（"无操作时渲染被影响"的根因）。
	el.SetHovered(false)
	if cs := r.ResolveElement(el); cs.Color.R != 0 {
		t.Fatalf("移出窗口后 color=%v want 默认黑色（:hover 残留未清除）", cs.Color)
	}

	// 嵌套：父元素 hover 残留清除后，子元素 :hover（祖先链冒泡）也应失效。
	parent := dom.NewElement(doc, "div")
	parent.AppendChild(el)
	r2 := NewResolver()
	r2.AddStyleSheet(newSheet(t, `div:hover a { color: blue; }`))
	parent.SetHovered(true)
	if cs := r2.ResolveElement(el); cs.Color.B != 255 {
		t.Fatalf("父 hovered 冒泡：a color=%v want blue", cs.Color)
	}
	parent.SetHovered(false)
	if cs := r2.ResolveElement(el); cs.Color.B != 0 {
		t.Fatalf("父 hover 清除后：a color=%v want 默认（祖先链 :hover 残留）", cs.Color)
	}
}

func TestResolver_BackgroundShorthandPosSize(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `div { background: linear-gradient(to right,#ff0000,#ff0000) 0 0/70px 70px no-repeat; }`))
	cs := r.ResolveElement(el)
	if !strings.HasPrefix(cs.BackgroundImage, "linear-gradient(") {
		t.Fatalf("BackgroundImage=%q", cs.BackgroundImage)
	}
	if cs.BackgroundSize != "70px 70px" {
		t.Fatalf("BackgroundSize=%q, want 70px 70px", cs.BackgroundSize)
	}
	if cs.BackgroundPosition != "0 0" {
		t.Fatalf("BackgroundPosition=%q, want 0 0", cs.BackgroundPosition)
	}
}

func TestResolver_BackgroundShorthandNoSize(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `div { background: linear-gradient(to right,#ff0000,#ff0000); }`))
	cs := r.ResolveElement(el)
	if cs.BackgroundSize != "" {
		t.Fatalf("BackgroundSize=%q, want empty", cs.BackgroundSize)
	}
}

func TestResolver_ScrollbarProps(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `div { scrollbar-width: thin; scrollbar-color: #ff0000 #0000ff; }`))
	cs := r.ResolveElement(el)
	if cs.GetProperty("scrollbar-width") != "thin" {
		t.Fatalf("scrollbar-width=%q, want thin", cs.GetProperty("scrollbar-width"))
	}
	// Token serialization joins with spaces; compare the two colors by
	// splitting (the renderer uses strings.Fields anyway).
	sc := strings.Fields(cs.GetProperty("scrollbar-color"))
	if len(sc) != 2 || sc[0] != "#ff0000" || sc[1] != "#0000ff" {
		t.Fatalf("scrollbar-color=%q, want [#ff0000 #0000ff]", cs.GetProperty("scrollbar-color"))
	}
}

func TestResolver_SelectionColors(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "p")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `::selection { background-color: #ffee00; color: #000000; }`))
	bg, fg, ok := r.SelectionColors()
	if !ok {
		t.Fatalf("SelectionColors ok=false, want true")
	}
	if bg.R != 0xFF || bg.G != 0xEE || bg.B != 0 {
		t.Fatalf("bg=%+v, want #ffee00", bg)
	}
	if fg.R != 0 || fg.G != 0 || fg.B != 0 {
		t.Fatalf("fg=%+v, want black", fg)
	}
	// No ::selection rule → ok=false.
	r2 := NewResolver()
	r2.AddStyleSheet(newSheet(t, `p { color: red; }`))
	if _, _, ok := r2.SelectionColors(); ok {
		t.Fatalf("no ::selection rule should report ok=false")
	}
	_ = el
}

func TestResolver_NthChildSelector(t *testing.T) {
	doc := dom.NewDocument()
	container := dom.NewElement(doc, "div")
	for i := 0; i < 3; i++ {
		item := dom.NewElement(doc, "p")
		_ = container.AppendChild(item)
	}
	_ = doc.AppendChild(container)
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `p:nth-child(2) { color: red; }`))
	children := container.ChildNodes()
	if len(children) < 2 {
		t.Fatalf("need at least 2 children")
	}
	secondChild := children[1].(*dom.Element)
	cs := r.ResolveElement(secondChild)
	if cs.Color.R != 255 {
		t.Fatalf("p:nth-child(2) color=%v want red", cs.Color)
	}
}

func TestResolver_GridProperties(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `div { display: grid; grid-template-columns: 1fr 1fr; grid-gap: 10px; }`))
	cs := r.ResolveElement(el)
	if cs.Display != DisplayGrid {
		t.Fatalf("display=%v want grid", cs.Display)
	}
	// Raw value may have spacing variations; check it contains the key parts
	if !strings.Contains(cs.GridTemplateColumns, "1fr") {
		t.Fatalf("grid-template-columns=%q should contain 1fr", cs.GridTemplateColumns)
	}
}

func TestResolver_NotSelector(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "span")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `:not(p) { color: red; } p { color: blue; }`))
	cs := r.ResolveElement(el)
	if cs.Color.R != 255 {
		t.Fatalf(":not(p) span color=%v want red", cs.Color)
	}
}

func checkLen(t *testing.T, name string, l Length, wantVal float64, wantUnit string) {
	t.Helper()
	if l.Value != wantVal {
		t.Errorf("%s.Value=%v want %v", name, l.Value, wantVal)
	}
	if l.Unit != wantUnit {
		t.Errorf("%s.Unit=%q want %q", name, l.Unit, wantUnit)
	}
}

// shadowSheet 构造一个 owner 位于 shadow tree 内 <style> 元素的 author 样式表，
// 模拟 extractAndAddStyles 对 shadow 内 <style> 的提取（owner 决定 scoping root）。
func shadowSheet(t *testing.T, sr *dom.ShadowRoot, cssText string) *css.CSSStyleSheet {
	t.Helper()
	host := sr.Host()
	styleEl := dom.NewElement(host.OwnerDocument(), "style")
	_ = sr.AppendChild(styleEl)
	sheet := css.NewCSSStyleSheetWithOwner(styleEl, "")
	p := css.NewParser(cssText)
	p.ParseStyleSheetInto(sheet)
	return sheet
}

func TestResolver_ShadowInheritanceFromHost(t *testing.T) {
	doc := dom.NewDocument()
	host := dom.NewElement(doc, "div")
	sr, err := host.AttachShadow("open")
	if err != nil {
		t.Fatal(err)
	}
	shadowChild := dom.NewElement(doc, "span")
	_ = sr.AppendChild(shadowChild)

	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "div { color: green; }"))
	cs := r.ResolveElement(shadowChild)
	// shadow tree 顶层元素应通过 shadow root 继承 host 的 color。
	if cs.Color.G != 255 {
		t.Fatalf("shadow child color=%v want green inherited from shadow host", cs.Color)
	}
}

func TestResolver_ShadowStyleScopedInside(t *testing.T) {
	doc := dom.NewDocument()
	host := dom.NewElement(doc, "div")
	sr, _ := host.AttachShadow("open")
	span := dom.NewElement(doc, "span")
	_ = sr.AppendChild(span)

	sheet := shadowSheet(t, sr, "span { color: red; }")
	r := NewResolver()
	r.AddStyleSheet(sheet)
	cs := r.ResolveElement(span)
	if cs.Color.R != 255 {
		t.Fatalf("shadow-internal span color=%v want red", cs.Color)
	}
}

func TestResolver_ShadowStyleDoesNotLeak(t *testing.T) {
	doc := dom.NewDocument()
	host := dom.NewElement(doc, "div")
	sr, _ := host.AttachShadow("open")
	sheet := shadowSheet(t, sr, "span { color: red; }")

	lightSpan := dom.NewElement(doc, "span") // 文档 light DOM，不在 shadow 内
	r := NewResolver()
	r.AddStyleSheet(sheet)
	cs := r.ResolveElement(lightSpan)
	if cs.Color.R != 0 || cs.Color.G != 0 || cs.Color.B != 0 {
		t.Fatalf("shadow style leaked to document: color=%v want default black", cs.Color)
	}
}

func TestResolver_DocumentStyleDoesNotPenetrate(t *testing.T) {
	doc := dom.NewDocument()
	host := dom.NewElement(doc, "div")
	sr, _ := host.AttachShadow("open")
	span := dom.NewElement(doc, "span")
	_ = sr.AppendChild(span)

	r := NewResolver()
	r.AddStyleSheet(newSheet(t, "span { color: red; }"))
	cs := r.ResolveElement(span)
	if cs.Color.R != 0 || cs.Color.G != 0 || cs.Color.B != 0 {
		t.Fatalf("document style penetrated shadow tree: color=%v want default black", cs.Color)
	}
}

// ─── CSS Scoping: :host / :host-context / ::slotted / ::part ───

func TestResolver_HostStyleFromShadow(t *testing.T) {
	doc := dom.NewDocument()
	host := dom.NewElement(doc, "div")
	sr, _ := host.AttachShadow("open")

	sheet := shadowSheet(t, sr, ":host { color: red; }")
	r := NewResolver()
	r.AddStyleSheet(sheet)
	cs := r.ResolveElement(host)
	if cs.Color.R != 255 || cs.Color.G != 0 || cs.Color.B != 0 {
		t.Fatalf(":host color=%v want red", cs.Color)
	}
}

func TestResolver_HostWithSelector(t *testing.T) {
	doc := dom.NewDocument()
	host := dom.NewElement(doc, "div")
	host.SetAttribute("class", "foo")
	sr, _ := host.AttachShadow("open")

	sheet := shadowSheet(t, sr, ":host(.foo) { color: red; } :host(.bar) { color: blue; }")
	r := NewResolver()
	r.AddStyleSheet(sheet)
	cs := r.ResolveElement(host)
	if cs.Color.R != 255 || cs.Color.G != 0 || cs.Color.B != 0 {
		t.Fatalf(":host(.foo) color=%v want red", cs.Color)
	}
}

func TestResolver_HostContext(t *testing.T) {
	doc := dom.NewDocument()
	wrapper := dom.NewElement(doc, "div")
	wrapper.SetAttribute("class", "dark")
	host := dom.NewElement(doc, "div")
	_ = wrapper.AppendChild(host)
	sr, _ := host.AttachShadow("open")

	sheet := shadowSheet(t, sr, ":host-context(.dark) { color: red; }")
	r := NewResolver()
	r.AddStyleSheet(sheet)
	cs := r.ResolveElement(host)
	if cs.Color.R != 255 || cs.Color.G != 0 || cs.Color.B != 0 {
		t.Fatalf(":host-context(.dark) color=%v want red", cs.Color)
	}
}

func TestResolver_HostBeatsDocumentRule(t *testing.T) {
	doc := dom.NewDocument()
	host := dom.NewElement(doc, "div")
	host.SetAttribute("class", "host")
	sr, _ := host.AttachShadow("open")

	docSheet := newSheet(t, ".host { color: blue; }")
	shadowSheet := shadowSheet(t, sr, ":host { color: red; }")
	r := NewResolver()
	r.AddStyleSheet(docSheet)
	r.AddStyleSheet(shadowSheet)
	cs := r.ResolveElement(host)
	// :host (scope=1) outranks the document .host rule (scope=0) for normal decls.
	if cs.Color.R != 255 || cs.Color.G != 0 || cs.Color.B != 0 {
		t.Fatalf(":host should beat document rule: color=%v want red", cs.Color)
	}
}

func TestResolver_SlottedStyle(t *testing.T) {
	doc := dom.NewDocument()
	host := dom.NewElement(doc, "div")
	sr, _ := host.AttachShadow("open")
	slot := dom.NewElement(doc, "slot")
	_ = sr.AppendChild(slot)
	span := dom.NewElement(doc, "span")
	_ = host.AppendChild(span) // light-DOM child, assigned to the default slot

	sheet := shadowSheet(t, sr, "::slotted(span) { color: red; }")
	r := NewResolver()
	r.AddStyleSheet(sheet)
	cs := r.ResolveElement(span)
	if cs.Color.R != 255 || cs.Color.G != 0 || cs.Color.B != 0 {
		t.Fatalf("::slotted(span) color=%v want red", cs.Color)
	}
}

func TestResolver_PartStyle(t *testing.T) {
	doc := dom.NewDocument()
	host := dom.NewElement(doc, "div")
	sr, _ := host.AttachShadow("open")
	btn := dom.NewElement(doc, "button")
	btn.SetAttribute("part", "btn")
	_ = sr.AppendChild(btn)

	sheet := newSheet(t, "::part(btn) { color: red; }")
	r := NewResolver()
	r.AddStyleSheet(sheet)
	cs := r.ResolveElement(btn)
	if cs.Color.R != 255 || cs.Color.G != 0 || cs.Color.B != 0 {
		t.Fatalf("::part(btn) color=%v want red", cs.Color)
	}
}

func TestResolver_PartLosesToShadowRule(t *testing.T) {
	doc := dom.NewDocument()
	host := dom.NewElement(doc, "div")
	sr, _ := host.AttachShadow("open")
	btn := dom.NewElement(doc, "button")
	btn.SetAttribute("part", "btn")
	_ = sr.AppendChild(btn)

	shadowSheet := shadowSheet(t, sr, "button { color: green; }")
	docSheet := newSheet(t, "::part(btn) { color: red; }")
	r := NewResolver()
	r.AddStyleSheet(shadowSheet)
	r.AddStyleSheet(docSheet)
	cs := r.ResolveElement(btn)
	// shadow-internal button rule (scope=1) outranks the document ::part rule (scope=0).
	if cs.Color.R != 0 || cs.Color.G != 255 || cs.Color.B != 0 {
		t.Fatalf("shadow rule should beat ::part: color=%v want green", cs.Color)
	}
}

func TestResolver_PartWithHostPrefix(t *testing.T) {
	doc := dom.NewDocument()
	host := dom.NewElement(doc, "xwidget")
	sr, _ := host.AttachShadow("open")
	btn := dom.NewElement(doc, "button")
	btn.SetAttribute("part", "btn")
	_ = sr.AppendChild(btn)

	sheet := newSheet(t, "xwidget::part(btn) { color: red; } ywidget::part(btn) { color: blue; }")
	r := NewResolver()
	r.AddStyleSheet(sheet)
	cs := r.ResolveElement(btn)
	if cs.Color.R != 255 || cs.Color.G != 0 || cs.Color.B != 0 {
		t.Fatalf("xwidget::part(btn) color=%v want red", cs.Color)
	}
}

func TestResolver_SlottedWithHostPrefix(t *testing.T) {
	doc := dom.NewDocument()
	host := dom.NewElement(doc, "xwidget")
	sr, _ := host.AttachShadow("open")
	slot := dom.NewElement(doc, "slot")
	_ = sr.AppendChild(slot)
	span := dom.NewElement(doc, "span")
	_ = host.AppendChild(span) // light-DOM child, assigned to the default slot

	sheet := shadowSheet(t, sr, "xwidget::slotted(span) { color: red; } ywidget::slotted(span) { color: blue; }")
	r := NewResolver()
	r.AddStyleSheet(sheet)
	cs := r.ResolveElement(span)
	if cs.Color.R != 255 || cs.Color.G != 0 || cs.Color.B != 0 {
		t.Fatalf("xwidget::slotted(span) color=%v want red", cs.Color)
	}
}
