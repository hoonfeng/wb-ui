package page

import (
	"strings"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
)

// FontFaceLoader 是宿主处理 `@font-face` 的回调：family 是去掉引号的 CSS
// family 名，srcs 是按声明顺序排列的 `url()` 候选（已按所属样式表基准
// 解析成绝对 URL）。宿主按顺序取第一个能解码的候选注册到渲染字体管理器。
//
// 为什么要传整个候选列表：`src: url(x.woff2) format("woff2"),
// url(x.woff) format("woff")` 是真实站点的标准写法，现代产物第一个候选
// 通常是 woff2；而 Skia 的 SFNT 解码器不支持 woff2（只认 TTF/OTF/WOFF）。
// 只取第一个会让整站自定义字体静默缺失（图标字体退化成方块），必须像
// 浏览器一样逐个回退。
type FontFaceLoader func(family string, srcs []string)

// registerFontFaces 把一张样式表里的 @font-face 规则交给宿主的
// FontFaceLoader。四条样式表接入路径都会调用它：内联 <style>、装配期
// <link>、运行时动态 <link>（AddStyleSheetFromText）以及解析器展开的
// @import 导入表（Resolver.OnImportedStyleSheet）。
//
// 浏览器同样在样式表「接入」时产生 CSSFontFaceRule，而不是等首次用到
// 该 family 才扫描样式表——这样字体会与 CSS 并行开始下载，首屏不闪烁。
func (f *Frame) registerFontFaces(sheet *css.CSSStyleSheet) {
	if f == nil || f.FontFaceLoader == nil || sheet == nil {
		return
	}
	// 相对 url() 相对**样式表自身**的 URL 解析（CSS Values 3 §4.4），
	// 不是文档 URL：/css/main.css 里的 `url(fonts/x.woff2)` 指向
	// /css/fonts/x.woff2。没有样式表 URL 时才回退到文档基准。
	base := sheet.BaseURL()
	if base == "" {
		base = sheet.Href()
	}
	if base == "" && f.document != nil {
		base = f.document.BaseURL()
	}
	f.walkFontFaceRules(sheet.Rules(), base)
}

func (f *Frame) walkFontFaceRules(rules []css.Rule, base string) {
	for _, r := range rules {
		switch v := r.(type) {
		case *css.FontFaceRule:
			family, srcs := fontFaceDescriptors(v)
			if family == "" || len(srcs) == 0 {
				continue
			}
			if base != "" {
				for i, src := range srcs {
					srcs[i] = dom.ResolveURL(base, src)
				}
			}
			f.FontFaceLoader(family, srcs)
		case *css.MediaRule:
			f.walkFontFaceRules(v.Rules, base)
		case *css.SupportsRule:
			f.walkFontFaceRules(v.Rules, base)
		}
	}
}

// fontFaceDescriptors 从 @font-face 声明里取 font-family 与 src 的 url()
// 候选列表。值取自 token 流而不是重新解析文本：tokenizer 已经处理了引号、
// 转义与注释（`url("./a b.woff2")` 这类含空格路径不会在文本切分时被截断）。
func fontFaceDescriptors(rule *css.FontFaceRule) (family string, srcs []string) {
	for _, d := range rule.Declarations {
		switch strings.ToLower(strings.TrimSpace(d.Name)) {
		case "font-family":
			family = strings.Trim(strings.TrimSpace(d.ValueString()), `"'`)
		case "src":
			for _, t := range d.Value {
				if t.Type != css.TokenURL {
					continue
				}
				if u := strings.TrimSpace(t.Value); u != "" {
					srcs = append(srcs, u)
				}
			}
		}
	}
	return family, srcs
}
