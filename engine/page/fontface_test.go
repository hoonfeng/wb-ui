package page

import (
	"fmt"
	"testing"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
	"wb-ui/engine/style"
)

func parseFontFaceSheet(t *testing.T, href, text string) *css.CSSStyleSheet {
	t.Helper()
	sheet := css.NewCSSStyleSheetWithOwner(nil, href)
	p := css.NewParser(text)
	p.ParseStyleSheetInto(sheet)
	return sheet
}

// TestRegisterFontFacesResolvesSrcAgainstSheet 覆盖 @font-face 发现的三个
// 关键语义：family 去引号、src 的相对 URL 按**样式表自身**基准解析
// （CSS Values 3 §4.4）、src 候选列表按声明顺序全部交给宿主（woff2 →
// woff 回退靠宿主逐个尝试）。
func TestRegisterFontFacesResolvesSrcAgainstSheet(t *testing.T) {
	f := &Frame{}
	type call struct {
		family string
		srcs   []string
	}
	var got []call
	f.FontFaceLoader = func(family string, srcs []string) {
		got = append(got, call{family: family, srcs: append([]string(nil), srcs...)})
	}

	sheet := parseFontFaceSheet(t, "https://example.com/css/main.css", `
@font-face {
  font-family: "Icon Font";
  src: url(./fonts/icon.woff2) format("woff2"), url(../fonts/icon.woff) format("woff");
}
@media (min-width: 100px) {
  @font-face { font-family: Body; src: url(https://cdn.example.com/body.ttf); }
}
`)
	f.registerFontFaces(sheet)

	if len(got) != 2 {
		t.Fatalf("loader calls = %d, want 2: %+v", len(got), got)
	}
	if got[0].family != "Icon Font" {
		t.Errorf("family = %q, want %q", got[0].family, "Icon Font")
	}
	wantSrcs := []string{
		"https://example.com/css/fonts/icon.woff2",
		"https://example.com/fonts/icon.woff",
	}
	if len(got[0].srcs) != len(wantSrcs) {
		t.Fatalf("srcs = %v, want %v", got[0].srcs, wantSrcs)
	}
	for i := range wantSrcs {
		if got[0].srcs[i] != wantSrcs[i] {
			t.Errorf("srcs[%d] = %q, want %q", i, got[0].srcs[i], wantSrcs[i])
		}
	}
	if got[1].family != "Body" {
		t.Errorf("@media 内 family = %q, want %q", got[1].family, "Body")
	}
	if len(got[1].srcs) != 1 || got[1].srcs[0] != "https://cdn.example.com/body.ttf" {
		t.Errorf("@media 内 srcs = %v，绝对 URL 应原样保留", got[1].srcs)
	}
}

// TestRegisterFontFacesFallsBackToDocumentBase：内联 <style> 的样式表没有
// 自身 URL，相对 src 必须按文档基准解析。
func TestRegisterFontFacesFallsBackToDocumentBase(t *testing.T) {
	doc := dom.NewDocument()
	doc.SetURL("https://example.com/app/index.html")
	f := &Frame{document: doc}
	var gotSrc string
	f.FontFaceLoader = func(_ string, srcs []string) {
		if len(srcs) > 0 {
			gotSrc = srcs[0]
		}
	}

	f.registerFontFaces(parseFontFaceSheet(t, "", `@font-face{font-family:X;src:url(fonts/x.ttf)}`))
	if gotSrc != "https://example.com/app/fonts/x.ttf" {
		t.Errorf("src = %q, want document-relative resolution", gotSrc)
	}
}

// TestRegisterFontFacesWithoutLoaderIsNoop：没有宿主接线时不得 panic
// （page 层可独立使用，字体加载是可选的宿主能力）。
func TestRegisterFontFacesWithoutLoaderIsNoop(t *testing.T) {
	f := &Frame{}
	f.registerFontFaces(parseFontFaceSheet(t, "", `@font-face{font-family:X;src:url(a.ttf)}`))
}

// TestImportedSheetFontFaceIsDiscovered：@import 展开出来的导入表不进
// document.styleSheets，只能由解析器回调发现——Vite/Rollup 产物把字体定义
// 放在被 @import 的 CSS 里，缺了这条路径整站字体静默缺失。
func TestImportedSheetFontFaceIsDiscovered(t *testing.T) {
	f := &Frame{}
	var families []string
	f.FontFaceLoader = func(family string, _ []string) { families = append(families, family) }

	imported := map[string]string{
		"https://example.com/css/theme.css": `@font-face{font-family:ImportedFont;src:url(theme.woff2)}`,
	}
	r := style.NewResolver()
	r.StyleSheetLoader = func(href string) (string, error) {
		if text, ok := imported[href]; ok {
			return text, nil
		}
		return "", fmt.Errorf("not found: %s", href)
	}
	r.OnImportedStyleSheet = f.registerFontFaces
	r.AddStyleSheet(parseFontFaceSheet(t, "https://example.com/css/main.css", `@import url("theme.css");`))

	if len(families) != 1 || families[0] != "ImportedFont" {
		t.Errorf("families = %v, want [ImportedFont]", families)
	}
}
