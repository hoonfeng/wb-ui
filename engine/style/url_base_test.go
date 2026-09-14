package style

// 样式表内相对 url() 的基准，以及 WebKit 前缀属性别名。
//
// CSS Values 3 §4.4 / CSS Syntax §5.4：样式表里的 url() 在**解析时**即相对
// **样式表自身 URL** 解析，与文档 URL 无关；而写在元素 style 属性 / 内联
// <style> 里的相对 url() 相对**文档** URL（内联样式的 base 就是文档的 base）。
//
// 引擎把这件事落在两处：收集声明时带上来源样式表的 base（collectedDecl.
// sheetBase，由 collect* 链路逐层传递），应用前统一绝对化
// （absolutizeCollectedURLs）。用 token 层重写而不是逐属性处理，因此
// background-image / mask-image / content / list-style-image / 简写 /
// 自定义属性 / @keyframes 这些**所有**读 URL 的通道一次性覆盖。

import (
	"testing"

	"wb-ui/engine/css"
	"wb-ui/engine/dom"
)

// newSheetWithBase 建一张外部样式表：base = 样式表自身的 URL。
func newSheetWithBase(t *testing.T, base, input string) *css.CSSStyleSheet {
	t.Helper()
	sheet := css.NewCSSStyleSheetWithOwner(nil, base)
	p := css.NewParser(input)
	p.ParseStyleSheetInto(sheet)
	return sheet
}

// TestStyleSheetURLBaseForRelativeURLs：外部样式表里的相对 url() 按样式表
// URL 绝对化——逐属性走一遍**所有**读 URL 的通道。
func TestStyleSheetURLBaseForRelativeURLs(t *testing.T) {
	const base = "http://example.com/css/site.css"
	doc := dom.NewDocument()
	r := NewResolver()
	r.AddStyleSheet(newSheetWithBase(t, base, `
		div {
			background-image: url(img/bg.png);
			mask-image: url(../masks/m.png);
			content: url(q.png);
			list-style-image: url(bullet.png);
			border-image-source: url(border.png);
			cursor: url(cur.png), auto;
			--asset: url(theme/x.svg);
		}
	`))
	el := dom.NewElement(doc, "div")
	cs := r.ResolveElement(el)

	for _, tc := range []struct {
		prop string
		want string
	}{
		{"background-image", "url(http://example.com/css/img/bg.png)"},
		{"mask-image", "url(http://example.com/masks/m.png)"},
		{"content", "url(http://example.com/css/q.png)"},
		{"list-style-image", "url(http://example.com/css/bullet.png)"},
		{"border-image-source", "url(http://example.com/css/border.png)"},
		{"cursor", "url(http://example.com/css/cur.png), auto"},
	} {
		if got := cs.GetProperty(tc.prop); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.prop, got, tc.want)
		}
	}
	// 自定义属性存在 CustomProperties（token 切片），单独查。
	toks := cs.GetCustomProperty("--asset")
	if len(toks) == 0 {
		t.Errorf("--asset 未保存（自定义属性通道）")
	} else if toks[0].Type != css.TokenURL || toks[0].Value != "http://example.com/css/theme/x.svg" {
		t.Errorf("--asset tokens[0] = type %v value %q, want url token 的绝对 URL", toks[0].Type, toks[0].Value)
	}
	// 结构体字段同样生效（渲染层读字段，不是读 Properties）。
	if cs.BackgroundImage != "url(http://example.com/css/img/bg.png)" {
		t.Errorf("cs.BackgroundImage = %q", cs.BackgroundImage)
	}
}

// TestStyleSheetURLBaseForBackgroundShorthand：简写 `background` 与
// `background-image` 的多层写法都要按样式表基准绝对化（简写展开走的是同一条
// token 流，这里锁定展开后没有把绝对 URL 丢掉）。
func TestStyleSheetURLBaseForBackgroundShorthand(t *testing.T) {
	doc := dom.NewDocument()
	r := NewResolver()
	r.AddStyleSheet(newSheetWithBase(t, "http://example.com/a/b/site.css", `
		#s { background: url(bg.png) no-repeat; }
		#m { background-image: url(one.png), url(two.png); }
	`))
	s := dom.NewElement(doc, "div")
	s.SetId("s")
	if got := r.ResolveElement(s).BackgroundImage; got != "url(http://example.com/a/b/bg.png)" {
		t.Errorf("简写 background 的 url 未按样式表基准解析：%q", got)
	}
	m := dom.NewElement(doc, "div")
	m.SetId("m")
	if got := r.ResolveElement(m).BackgroundImage; got != "url(http://example.com/a/b/one.png), url(http://example.com/a/b/two.png)" {
		t.Errorf("多层 background-image 未逐层绝对化：%q", got)
	}
}

// TestInlineSheetsKeepDocumentRelativeURLs：无 base（内联 <style>）时相对
// url() 保持原样——它该由渲染层按**文档** URL 解析，不能被误绝对化。
func TestInlineSheetsKeepDocumentRelativeURLs(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `div { background-image: url(logo.png); }`))
	if got := r.ResolveElement(el).BackgroundImage; got != "url(logo.png)" {
		t.Errorf("内联样式表里的相对 url() 被改动：%q（应保持原样，由渲染层按文档 URL 解析）", got)
	}
}

// TestAbsolutizeOnlyRelativeURLs：绝对引用 / 协议相对 / data: / 自定义
// 逻辑名（宿主 ResourceResolver 用的 app:// 之类）都原样保留。
func TestAbsolutizeOnlyRelativeURLs(t *testing.T) {
	doc := dom.NewDocument()
	r := NewResolver()
	r.AddStyleSheet(newSheetWithBase(t, "http://example.com/css/site.css", `
		#u { background-image: url(https://cdn.example.org/i.png); }
		#p { background-image: url(//cdn.example.org/i.png); }
		#d { background-image: url(data:image/png;base64,AAAA); }
		#a { background-image: url(app://assets/i.png); }
	`))
	for _, tc := range []struct{ id, want string }{
		{"u", "url(https://cdn.example.org/i.png)"},
		{"p", "url(//cdn.example.org/i.png)"},
		{"d", "url(data:image/png;base64,AAAA)"},
		{"a", "url(app://assets/i.png)"},
	} {
		el := dom.NewElement(doc, "div")
		el.SetId(tc.id)
		if got := r.ResolveElement(el).BackgroundImage; got != tc.want {
			t.Errorf("#%s background-image = %q, want %q", tc.id, got, tc.want)
		}
	}
}

// TestKeyframesURLsResolvedAgainstSheetBase：@keyframes 的声明不走级联
// 收集链路，但同样要按样式表基准解析（否则动画里的背景图请求错目录）。
func TestKeyframesURLsResolvedAgainstSheetBase(t *testing.T) {
	r := NewResolver()
	r.AddStyleSheet(newSheetWithBase(t, "http://example.com/css/site.css",
		`@keyframes fade { to { background-image: url(frames/last.png); } }`))
	kf := r.LookupKeyframes("fade")
	if kf == nil || len(kf.Keyframes) == 0 || len(kf.Keyframes[0].Declarations) == 0 {
		t.Fatalf("@keyframes 未注册：%+v", kf)
	}
	if got := kf.Keyframes[0].Declarations[0].ValueString(); got != "url(http://example.com/css/frames/last.png)" {
		t.Errorf("@keyframes 里的 url() = %q, want 相对样式表解析", got)
	}
}

// TestImportedSheetURLBase：@import 进来的样式表用**自己的 URL** 作基准
// （导入链逐级正确），不是导入者也不是文档的 URL。
func TestImportedSheetURLBase(t *testing.T) {
	doc := dom.NewDocument()
	el := dom.NewElement(doc, "div")
	el.SetId("box")
	r := NewResolver()
	r.StyleSheetLoader = func(href string) (string, error) {
		if href != "http://example.com/css/theme.css" {
			t.Errorf("StyleSheetLoader 收到的 href = %q（应按导入者 URL 解析）", href)
		}
		return `#box { background-image: url(sub/bg.png); }`, nil
	}
	r.AddStyleSheet(newSheetWithBase(t, "http://example.com/css/site.css", `@import "theme.css";`))

	if got := r.ResolveElement(el).BackgroundImage; got != "url(http://example.com/css/sub/bg.png)" {
		t.Errorf("导入表里的 url() = %q, want url(http://example.com/css/sub/bg.png)", got)
	}
}

// TestPrefixedPropertyAliases：WebKit 前缀属性与标准属性同义。前缀写法此前
// 会变成一条无人消费的陌生属性（mask 图片通道静默失效）。
func TestPrefixedPropertyAliases(t *testing.T) {
	doc := dom.NewDocument()
	r := NewResolver()
	r.AddStyleSheet(newSheet(t, `
		div {
			-webkit-mask-image: url(mask.png);
			-webkit-mask-size: 100% 100%;
			-webkit-transform: rotate(45deg);
			-webkit-animation: spin 2s linear infinite;
			-webkit-background-clip: text;
		}
	`))
	cs := r.ResolveElement(dom.NewElement(doc, "div"))
	for _, want := range []string{"mask-image", "mask-size", "transform", "animation", "background-clip"} {
		if got := cs.GetProperty(want); got == "" {
			t.Errorf("前缀属性未映射到 %q（-webkit- 写法被当成陌生属性丢弃）", want)
		}
	}
	// 前缀里含**长度**值的属性也要真生效（不是只写进 Properties）。
	if cs.GetProperty("mask-size") != "100% 100%" {
		t.Errorf("mask-size = %q, want 100%% 100%%", cs.GetProperty("mask-size"))
	}
}
