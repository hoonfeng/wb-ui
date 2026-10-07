package webkit

// 验证 CM6 文本测量路径（Range.getClientRects → measureTextWidth → Skia）
// 返回的宽度与 Skia advance 一致——「文本宽度没有使用标准 Skia」的回归测试。
import (
	"encoding/json"
	"math"
	"testing"

	"wb-ui/engine/js/bindings"
	"wb-ui/engine/dom"
	"wb-ui/engine/platform/graphics"
)

func TestCM6RangeMeasurementMatchesSkia(t *testing.T) {
	if graphics.GetFontManager() == nil {
		_ = graphics.InitFontManager("")
		graphics.GetFontManager().LoadSystemFonts()
	}
	wv := NewWebView()
	wv.Resize(800, 600)
	html := `<!DOCTYPE html><html><head><style>
		.cm-content { font-family: ui-monospace, 'Cascadia Mono', 'Segoe UI Mono', Consolas, monospace; font-size: 13px; line-height: normal; white-space: pre; }
	</style></head><body><div id="c" class="cm-content">aaaaaaaaaaaaaaaaaaaaaaaaaaa</div></body></html>`
	if err := wv.LoadHTML(html); err != nil {
		t.Fatal(err)
	}
	// 与 app.Host 一致地接线 computed font（生产环境由 Host 注入）。
	bindings.GetElementComputedFont = func(el *dom.Element) (string, float64, int, string) {
		fr := wv.MainFrame().Frame()
		if fr == nil || fr.Resolver() == nil {
			return "sans-serif", 14, 400, "normal"
		}
		cs := fr.Resolver().ResolveElement(el)
		if cs == nil {
			return "sans-serif", 14, 400, "normal"
		}
		size := cs.FontSize.Value
		if size <= 0 {
			size = 14
		}
		w := 400
		switch cs.FontWeight {
		case "bold", "bolder", "600", "700", "800", "900":
			w = 700
		}
		st := "normal"
		if cs.FontStyle == "italic" || cs.FontStyle == "oblique" {
			st = cs.FontStyle
		}
		fam := cs.FontFamily
		if fam == "" {
			fam = "sans-serif"
		}
		return fam, size, w, st
	}
	defer func() { bindings.GetElementComputedFont = nil }()

	// 取 .cm-content 的 computed font（JS 侧量什么，Go 侧就用什么量）。
	el := wv.MainFrame().Document().GetElementById("c")
	if el == nil {
		t.Fatal("element #c not found")
	}
	fam, size, weight, stl := bindings.GetElementComputedFont(el)
	t.Logf("computed font: %q %.2fpx w=%d style=%q", fam, size, weight, stl)

	// JS 侧：CM6 的 measureTextSize 用 27 字符 text node + getClientRects。
	v, err := wv.EvalJS(`(function(){
		var el = document.getElementById('c');
		var rng = document.createRange();
		rng.selectNodeContents(el.firstChild);
		var r = rng.getClientRects()[0];
		return JSON.stringify({w: r.width, h: r.height});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	js := v.ToString()
	var jr struct {
		W float64 `json:"w"`
		H float64 `json:"h"`
	}
	if err := json.Unmarshal([]byte(js), &jr); err != nil {
		t.Fatalf("parse %q: %v", js, err)
	}
	jw, jh := jr.W, jr.H
	t.Logf("JS getClientRects: width=%.4f height=%.4f", jw, jh)

	// Go 侧：Skia 测量同一字符串。
	const text = "aaaaaaaaaaaaaaaaaaaaaaaaaaa"
	skiaW := graphics.MeasureText(graphics.Font{Family: fam, Size: size, Weight: weight, Style: stl}, text)
	if math.Abs(jw-skiaW) > 0.05 {
		t.Fatalf("getClientRects width %.4f != Skia %.4f (diff %.4f)", jw, skiaW, jw-skiaW)
	}
	// 高度：line-height normal → 行盒高度 = ascent + descent + **lineGap**。
	//
	// ★ 断言口径修正（2026-10-07）：原断言只取 ascent+descent（**不含** lineGap），
	//   于是一直把引擎的正确行为判成失败——实测 14.8281 / 13.0000 = **1.1406**，正是
	//   该字体 normal 行距系数，也就是缺失的 lineGap。浏览器 `Range.getClientRects()`
	//   对 inline 内容返回的是**行盒**矩形，而 `line-height: normal` 的行盒高度按字体
	//   度量算作 ascent+descent+lineGap（引擎侧同一口径见
	//   `engine/layout/layoututil.go` 的 `fontLineGap`，注释里有 Chrome 实测对照）。
	a, d, _, gap := graphics.GlobalFontMetrics(graphics.Font{Family: fam, Size: size, Weight: weight, Style: stl})
	if math.Abs(jh-(a+d+gap)) > 0.5 {
		t.Fatalf("getClientRects height %.4f != Skia ascent+descent+lineGap %.4f (ascent=%.4f descent=%.4f lineGap=%.4f)",
			jh, a+d+gap, a, d, gap)
	}
}
