package webkit

import "testing"

// TestGetComputedStyleDeclaredLonghands 钉死：**已声明**的 longhand 必须能被
// getComputedStyle 读出（声明白名单含这些属性、computedStyleFor 已实现简写展开）。
//
// 复现 gou-ide 真实场景：`.input-wrapper .chat-input` 声明了
// `padding:8px 0; box-sizing:border-box; height/min-height:40px; font-size`
// （RightPanel.vue:3031-3035），而引擎侧读到的却是
// `paddingTop=0`、`paddingRight/Left/borderTopWidth/fontSize/lineHeight/minHeight = undefined`
// —— 一切依赖 computed 的 JS 自适应/动态高度逻辑都会因此失效。
func TestGetComputedStyleDeclaredLonghands(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(400, 300)
	html := `<!DOCTYPE html><html><head><style>
.ib .ci { padding: 8px 0; box-sizing: border-box; font-size: 14px; line-height: 1.6; min-height: 40px; border: 1px solid #333333; }
</style></head><body style="margin:0">
<div class="ib"><div class="ci" id="ci" style="height:40px"></div></div>
</body></html>`
	if err := wv.LoadHTML(html); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	for i := 0; i < 6; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	got := dynEval(t, wv, `(function(){
  var c = getComputedStyle(document.getElementById("ci"));
  return ["pt="+c.paddingTop, "pr="+c.paddingRight, "pb="+c.paddingBottom, "pl="+c.paddingLeft,
          "bt="+c.borderTopWidth, "fs="+c.fontSize, "lh="+c.lineHeight, "minH="+c.minHeight,
          "bs="+c.boxSizing].join(" ");
})()`)
	t.Logf("computed = %s", got)
	for _, want := range []string{"pt=8px", "pb=8px", "bt=1px", "fs=14px", "bs=border-box"} {
		if !contains(got, want) {
			t.Errorf("getComputedStyle 读不出已声明属性：缺少 %q；实际 %s", want, got)
		}
	}
	for _, bad := range []string{"pr=undefined", "pl=undefined", "lh=undefined", "minH=undefined"} {
		if contains(got, bad) {
			t.Errorf("已声明 longhand 返回 undefined（%s）：%s", bad, got)
		}
	}
}
