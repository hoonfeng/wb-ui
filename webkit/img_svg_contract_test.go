package webkit

import (
	"net/url"
	"testing"
)

// TestImgSVGContract 钉死 SVG（矢量）资源在 `<img>` 上的**契约与固有尺寸**：
// complete / naturalWidth / naturalHeight / onload 必须与位图一致。
//
// 此前 SVG 的固有尺寸只从**解码位图**取（bindings.imgPixelDim），而 SVG 是矢量
// 资源、根本不进位图解码缓存：naturalWidth/naturalHeight 因此恒 0、complete
// 恒 false；事件侧 flushImageEvents 又把它当「未就绪」→ RequestImageLoad →
// Skia 按位图解码失败 → 派发 **error**——而同一个元素其实画得出来（探针里
// 绘制列 ✅）。浏览器同页面下 complete=true、naturalWidth=24、派发 load。
func TestImgSVGContract(t *testing.T) {
	withWH := `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24">` +
		`<rect width="24" height="24" fill="#008000"/></svg>`
	// 仅 viewBox：固有尺寸走 viewBox（40×20），不是 0。
	vbOnly := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 20">` +
		`<rect width="40" height="20" fill="#0000ff"/></svg>`

	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(400, 300)
	html := `<!DOCTYPE html><html><body style="margin:0">` +
		`<img id="a" src="data:image/svg+xml,` + url.PathEscape(withWH) + `" ` +
		`onload="window.__loads=(window.__loads||0)+1" onerror="window.__errs=(window.__errs||0)+1">` +
		`<img id="vb" src="data:image/svg+xml,` + url.PathEscape(vbOnly) + `">` +
		`</body></html>`
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
  var a=document.getElementById("a"), vb=document.getElementById("vb");
  return "a.complete="+a.complete+" a.nw="+a.naturalWidth+" a.nh="+a.naturalHeight+
         " | vb.nw="+vb.naturalWidth+" vb.nh="+vb.naturalHeight+
         " | loads="+(window.__loads||0)+" errs="+(window.__errs||0);
})()`)
	// ① 有 width/height 的 SVG：固有尺寸就是它声明的尺寸，且 complete/load 齐备。
	for _, want := range []string{"a.complete=true", "a.nw=24", "a.nh=24"} {
		if !contains(got, want) {
			t.Fatalf("SVG <img> 契约缺 %q：%s", want, got)
		}
	}
	// ② 仅 viewBox 的 SVG：固有尺寸 = viewBox 尺寸（不是 0）。
	if !contains(got, "vb.nw=40") || !contains(got, "vb.nh=20") {
		t.Fatalf("仅 viewBox 的 SVG 固有尺寸应为 40x20：%s", got)
	}
	// ③ 事件：派发 load，**不**派发 error（能渲染即加载成功）。
	if !contains(got, "loads=1") {
		t.Fatalf("内联 SVG 应派发 1 次 load：%s", got)
	}
	if !contains(got, "errs=0") {
		t.Fatalf("内联 SVG 不应派发 error：%s", got)
	}
}
