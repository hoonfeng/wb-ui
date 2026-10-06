package webkit

// 状态属性（checked）变更的样式失效回归（2026-09）：
//
// 三条路径都必须让 `:checked` 的匹配结果抵达 computed style 与画面：
//
//	① 引擎内部 HTMLInputElement.SetChecked（真实点击走这里）；
//	② JS `el.checked = true`（lazyelement 的 IDL setter）；
//	③ JS `el.setAttribute('checked', ...)`（既有链路，作回归保护）。
//
// 且失效范围必须覆盖**兄弟/后继组合器**：`input:checked + .track::after`
// （开关滑块）这类邻近元素受影响时，只清 el 子树的缓存会让它停在旧样式。
//
// 实测基准（2026-09-27 headless；修复前 → 修复后）：
//
//	引擎 SetChecked(true) 自身 #c：rgb(1, 2, 3) → rgb(9, 9, 9)
//	                     兄弟 #s：rgb(7, 7, 7) → rgb(5, 5, 5)（此前恒为旧值）
//	JS el.checked=true  自身 #c：rgb(1, 2, 3)（完全没失效）→ rgb(9, 9, 9)

import (
	"testing"

	"wb-ui/engine/html5"
)

// checkedStateFixture：自身样式（#c:checked）、兄弟相邻样式（#c:checked + #s）
// 与像素可读色块（#c:checked + #s + #blk）三路判据。
const checkedStateFixture = `<style>
#c{color:rgb(1,2,3)}
#s{color:rgb(7,7,7)}
#blk{width:60px;height:30px;background:rgb(7,7,7)}
#c:checked{color:rgb(9,9,9)}
#c:checked + #s{color:rgb(5,5,5)}
#c:checked + #s + #blk{background:rgb(5,5,5)}
</style><input type="checkbox" id="c"><span id="s">x</span><div id="blk"></div>`

func evalComputedColor(t *testing.T, wv *WebView, id string) string {
	t.Helper()
	v, err := wv.EvalJS(`getComputedStyle(document.getElementById('` + id + `')).color`)
	if err != nil {
		t.Fatalf("getComputedStyle(#%s): %v", id, err)
	}
	return v.ToString()
}

func mustEval(t *testing.T, wv *WebView, src string) {
	t.Helper()
	if _, err := wv.EvalJS(src); err != nil {
		t.Fatalf("EvalJS(%s): %v", src, err)
	}
}

// TestCheckedStateInvalidatesStyle：三条路径 × (自身, 兄弟) 都要更新。
func TestCheckedStateInvalidatesStyle(t *testing.T) {
	wv := interactTestWebView(t, checkedStateFixture)
	doc := wv.Document()
	cb := doc.GetElementById("c")
	if cb == nil {
		t.Fatal("checkbox #c 缺失")
	}
	in, ok := html5.ToInputElement(cb)
	if !ok {
		t.Fatal("#c 不是 input")
	}
	if got, want := evalComputedColor(t, wv, "c"), "rgb(1, 2, 3)"; got != want {
		t.Fatalf("初始 #c color=%q，want %q（夹具本身没生效，后续断言无意义）", got, want)
	}

	// ① 引擎内部路径（真实点击 ToggleCheckboxRadio 调的就是它）
	in.SetChecked(true)
	if !in.Checked() {
		t.Fatal("SetChecked(true) 后 Checked()=false")
	}
	if got, want := evalComputedColor(t, wv, "c"), "rgb(9, 9, 9)"; got != want {
		t.Errorf("引擎 SetChecked(true) 后 #c color=%q，want %q（引擎内部改状态属性未失效样式）", got, want)
	}
	if got, want := evalComputedColor(t, wv, "s"), "rgb(5, 5, 5)"; got != want {
		t.Errorf("引擎 SetChecked(true) 后兄弟 #s color=%q，want %q（失效范围漏兄弟组合器）", got, want)
	}
	in.SetChecked(false)
	if got, want := evalComputedColor(t, wv, "c"), "rgb(1, 2, 3)"; got != want {
		t.Errorf("SetChecked(false) 后 #c color=%q，want %q", got, want)
	}
	if got, want := evalComputedColor(t, wv, "s"), "rgb(7, 7, 7)"; got != want {
		t.Errorf("SetChecked(false) 后 #s color=%q，want %q", got, want)
	}

	// ② JS IDL setter
	mustEval(t, wv, `document.getElementById('c').checked = true`)
	if got, want := evalComputedColor(t, wv, "c"), "rgb(9, 9, 9)"; got != want {
		t.Errorf("JS el.checked=true 后 #c color=%q，want %q（IDL setter 未失效）", got, want)
	}
	if got, want := evalComputedColor(t, wv, "s"), "rgb(5, 5, 5)"; got != want {
		t.Errorf("JS el.checked=true 后 #s color=%q，want %q", got, want)
	}
	mustEval(t, wv, `document.getElementById('c').checked = false`)
	if got, want := evalComputedColor(t, wv, "s"), "rgb(7, 7, 7)"; got != want {
		t.Errorf("JS el.checked=false 后 #s color=%q，want %q", got, want)
	}

	// ③ JS setAttribute（既有失效链路）
	mustEval(t, wv, `document.getElementById('c').setAttribute('checked','checked')`)
	if got, want := evalComputedColor(t, wv, "s"), "rgb(5, 5, 5)"; got != want {
		t.Errorf("JS setAttribute('checked') 后 #s color=%q，want %q", got, want)
	}
}

// TestCheckedStateRepaintsPixels：渲染路径也要跟着动（不只 JS 侧读数）——
// 勾选后相邻色块必须真的按 :checked 规则重绘。
func TestCheckedStateRepaintsPixels(t *testing.T) {
	wv := interactTestWebView(t, checkedStateFixture)
	doc := wv.Document()
	blk := doc.GetElementById("blk")
	if blk == nil {
		t.Fatal("#blk 缺失")
	}
	box := wv.RenderView().FindRenderBoxForNode(blk)
	if box == nil {
		t.Fatal("#blk 无 render box")
	}
	px, py := int(box.AbsoluteX()+box.Width()/2), int(box.AbsoluteY()+box.Height()/2)
	r, g, b, _ := fragmentPixelAt(t, wv, px, py)
	if r != 7 || g != 7 || b != 7 {
		t.Fatalf("初始像素=(%d,%d,%d)，want (7,7,7)", r, g, b)
	}
	if !ToggleCheckboxRadio(doc.GetElementById("c")) {
		t.Fatal("ToggleCheckboxRadio(#c)=false")
	}
	r, g, b, _ = fragmentPixelAt(t, wv, px, py)
	if r != 5 || g != 5 || b != 5 {
		t.Fatalf("勾选后像素=(%d,%d,%d)，want (5,5,5)——相邻色块未按 :checked 重绘", r, g, b)
	}
}
