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
	// 高度：line-height normal → Skia ascent+descent（15.22 之类）。
	a := graphics.GlobalFontAscent(graphics.Font{Family: fam, Size: size, Weight: weight, Style: stl})
	d := graphics.GlobalFontDescent(graphics.Font{Family: fam, Size: size, Weight: weight, Style: stl})
	if math.Abs(jh-(a+d)) > 0.5 {
		t.Fatalf("getClientRects height %.4f != Skia ascent+descent %.4f", jh, a+d)
	}
}
