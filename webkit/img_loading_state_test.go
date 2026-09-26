package webkit

import "testing"

// TestImgCompleteAndNaturalDimensions 钉死：`HTMLImageElement` 的
// `complete` / `naturalWidth` / `naturalHeight` 必须在 JS 侧可读。
//
// 此前 lazyelement.go 的 tag=="img" 分支只暴露 width/height，JS 读到的是
// `undefined` —— 依赖图片加载状态的**懒加载 / 占位 / 骨架屏 / 失败重试**逻辑
// 全部走错分支（实测：真实窗口 `imgs=1 ok=0`，而浏览器同页面 `imgs=1 ok=1`）。
func TestImgCompleteAndNaturalDimensions(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(400, 300)
	// 1x1 PNG（data URI）+ 一个无 src 的 img。
	html := `<!DOCTYPE html><html><body style="margin:0">
<img id="a" src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFAAH/q842iQAAAABJRU5ErkJggg==">
<img id="b"></body></html>`
	if err := wv.LoadHTML(html); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	for i := 0; i < 10; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	got := dynEval(t, wv, `(function(){
  var a=document.getElementById("a"), b=document.getElementById("b");
  return "a.complete="+a.complete+"("+typeof a.complete+") a.nw="+a.naturalWidth+" a.nh="+a.naturalHeight+
         " | b.complete="+b.complete+"("+typeof b.complete+")";
})()`)
	// ① 有 src 的 #a：**解码成功** → complete === true 且 naturalWidth > 0
	//   （收紧断言：此前只要求「是 boolean」/「非 undefined」，解码路径回归时不会报警）；
	// ② 无 src 的 #b：按规范 complete === true。
	if !contains(got, "a.complete=true(boolean)") {
		t.Fatalf("有 src 的 img.complete 应为 true（解码后）：%s", got)
	}
	if contains(got, "a.nw=0 ") || contains(got, "a.nw=undefined") {
		t.Fatalf("有 src 的 img.naturalWidth 应 > 0：%s", got)
	}
	if contains(got, "a.nh=0 ") || contains(got, "a.nh=undefined") {
		t.Fatalf("有 src 的 img.naturalHeight 应 > 0：%s", got)
	}
	if !contains(got, "b.complete=true(boolean)") {
		t.Fatalf("无 src 的 img.complete 应为 true：%s", got)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
