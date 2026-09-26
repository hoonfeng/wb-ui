package webkit

import "testing"

// TestGetComputedStyleImportantSameRule 钉死：同一条规则里**混有 `!important`
// 声明**时（gou-ide 真实规则原样），普通声明的 longhand 必须照常可读。
//
// 真实规则（RightPanel.vue:3031-3035）：
//
//	.input-wrapper .chat-input {
//	  border: 0 !important; background: transparent !important;
//	  height: 40px; min-height: 40px; font-size: 14px;
//	  padding: 8px 0; box-sizing: border-box;
//	}
//
// 真实页面引擎侧实测：`paddingTop/Bottom = 0`（应 8px），而同规则的
// `height(=40px)`/`box-sizing` 却能读到 —— 疑 `!important` 处理路径干扰同规则内
// 其它声明（此前「类规则未匹配」的推断与「同规则 height 可读」自相矛盾）。
func TestGetComputedStyleImportantSameRule(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(600, 300)
	html := `<!DOCTYPE html><html><head><style>
.input-wrapper .chat-input {
  border: 0 !important; background: transparent !important;
  height: 40px; min-height: 40px; font-size: 14px;
  padding: 8px 0; box-sizing: border-box;
}
</style></head><body style="margin:0">
<div class="input-wrapper"><div class="chat-input" id="ci" style="height:40px"></div></div>
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
          "minH="+c.minHeight, "fs="+c.fontSize, "h="+c.height, "bs="+c.boxSizing,
          "bt="+c.borderTopWidth].join(" ");
})()`)
	t.Logf("computed = %s", got)
	if !contains(got, "pt=8px") || !contains(got, "pb=8px") {
		t.Errorf("同规则内普通声明 padding 被 !important 兄弟声明干扰：%s（期望 pt=8px pb=8px）", got)
	}
	if contains(got, "minH=undefined") || contains(got, "fs=undefined") {
		t.Errorf("同规则内 min-height/font-size 读不到：%s", got)
	}
}
