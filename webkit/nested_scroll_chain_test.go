package webkit

import (
	"strconv"
	"strings"
	"testing"
)

// TestNestedWheelChainsToOuterScrollable 钉死 (f) 嵌套可滚容器滚动链：滚轮
// 落在**内层不可滚**（声明 overflow-y:auto 但内容不超出）的容器上时，必须
// 上溯到最近的**真正可滚**祖先并滚动它（浏览器 scroll chaining）。
// 断言可量化：外层 #outer 的 scrollTop 数值 + 内层 #inner 保持 0。
func TestNestedWheelChainsToOuterScrollable(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(300, 300)
	html := `<!DOCTYPE html><html><body style="margin:0">
<div id="outer" style="width:150px;height:100px;overflow-y:auto">
  <div id="inner" style="width:100px;height:30px;overflow-y:auto">
    <div style="height:20px">i</div>
  </div>
  <div style="height:300px">tall</div>
</div></body></html>`
	if err := wv.LoadHTML(html); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	for i := 0; i < 10; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	read := func() (outer, inner float64, raw string) {
		got, err := wv.EvalJS(`(function(){
  var o = document.getElementById('outer'), i = document.getElementById('inner');
  return o.scrollTop + ',' + i.scrollTop + ' | outerScrollHeight=' + o.scrollHeight +
         ' outerClientHeight=' + o.clientHeight + ' innerScrollHeight=' + i.scrollHeight +
         ' innerClientHeight=' + i.clientHeight;
})()`)
		if err != nil {
			t.Fatalf("EvalJS: %v", err)
		}
		raw = got.ToString()
		parts := strings.SplitN(strings.SplitN(raw, " | ", 2)[0], ",", 2)
		if len(parts) == 2 {
			outer, _ = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
			inner, _ = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		}
		return
	}
	o0, i0, raw0 := read()
	t.Logf("滚动前 = outer.scrollTop=%v inner.scrollTop=%v | %s", o0, i0, raw0)
	if o0 != 0 || i0 != 0 {
		t.Fatalf("初始 scrollTop 非 0：outer=%v inner=%v", o0, i0)
	}
	// 光标先移到内层 (#inner 位于 outer 左上，(0,0)-(100,30)，取 (50,15))，再滚轮。
	// ★ deltaY 符号：Interaction.Wheel 的注释与实现均为「正=向上滚」（Win32 语义，
	//   step = -deltaY/120*48）→ 向下滚必须传**负值**，否则在顶部只会被 clamp 回 0。
	wv.HandleMouseMove(50, 15)
	wv.HandleWheel(-120)
	for i := 0; i < 6; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	o1, i1, raw1 := read()
	t.Logf("滚轮 -120（向下）后 = outer.scrollTop=%v inner.scrollTop=%v | %s", o1, i1, raw1)
	if i1 != 0 {
		t.Fatalf("内层不可滚却发生滚动：inner.scrollTop=%v（%s）", i1, raw1)
	}
	if o1 <= 0 {
		t.Fatalf("滚动链失效：滚轮落在内层不可滚容器上时外层未上溯滚动（outer.scrollTop=%v，期望 120）\n  %s", o1, raw1)
	}
}
