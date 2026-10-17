package webkit

import (
	"errors"
	"strings"
	"testing"

	"wb-ui/engine/js/jsc"
)

// TestToolkitModeFrontendFeaturesGraceful：ModeToolkit（UI 库）下新增的
// 前端项目能力必须优雅降级——
//   - @font-face 的外部字体被资源策略拒绝：只记日志，不 panic、不影响渲染
//   - URL.createObjectURL / fetch(blob:) 是纯内存能力，照常可用
//   - <script type="module"> 内联模块照常执行
//
// 这三条保证「任意前端项目」在两种运行模式下行为可预期，而不是在
// Toolkit 模式里因为一条被拒绝的字体请求就中断装配。
func TestToolkitModeFrontendFeaturesGraceful(t *testing.T) {
	wv := NewWebViewWithMode(ModeToolkit)
	t.Cleanup(wv.Destroy)

	html := `<!DOCTYPE html><html><head><style>
@font-face { font-family: "ExternalFont"; src: url("/fonts/never-loaded.woff2"); }
body { font-family: "ExternalFont", sans-serif; }
</style></head><body><div id="out">boot</div>
<script type="module">
  window.__toolkit = { hasCreate: typeof URL.createObjectURL === "function" };
  var u = URL.createObjectURL(new Blob(["toolkit-blob"], { type: "text/plain" }));
  window.__toolkit.url = u;
  fetch(u).then(function (r) { return r.text(); }).then(function (txt) {
    window.__toolkit.text = txt;
  }, function (e) { window.__toolkit.err = String(e); });
</script>
</body></html>`
	if err := wv.LoadHTML(html); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	esmPump(wv, 10)

	got := esmEval(t, wv, "JSON.stringify(window.__toolkit)")
	if !strings.Contains(got, `"hasCreate":true`) {
		t.Errorf("URL.createObjectURL missing in toolkit mode: %s", got)
	}
	if !strings.Contains(got, `"text":"toolkit-blob"`) {
		t.Errorf("fetch(blob:) failed in toolkit mode: %s", got)
	}
	if strings.Contains(got, "err") {
		t.Errorf("unexpected blob error in toolkit mode: %s", got)
	}
	// 页面仍渲染（被拒绝的 @font-face 不能让装配失败）。
	if out := esmEval(t, wv, "document.getElementById('out').textContent"); out != "boot" {
		t.Errorf("out = %q, want boot", out)
	}
}

// TestToolkitModeBlobResourcePolicy：blob: URL 是页面用 createObjectURL
// 自产的内存内容（同源、不经网络/文件系统），ModeToolkit + DenyExternal
// 下仍必须能被资源通道读取（`<img src=blob:>`、`@font-face url(blob:)`、
// `import(blob:)` 都依赖它）——这是「堵漏」：既不能因为策略把页面自己的
// 内容也拦掉，也不能因此放宽 http(s)/file 的拒绝。
func TestToolkitModeBlobResourcePolicy(t *testing.T) {
	wv := NewWebViewWithMode(ModeToolkit)
	t.Cleanup(wv.Destroy)

	// 登记一个 blob URL（与页面 URL.createObjectURL 共用同一张存储）。
	u := jsc.RegisterBlobURL("null", "blob-image-bytes", "image/png")
	if u == "" {
		t.Fatal("RegisterBlobURL returned empty URL")
	}

	got, err := wv.loadExternalResource(u, PurposeImage)
	if err != nil || got != "blob-image-bytes" {
		t.Fatalf("toolkit 模式读取已登记 blob 失败：content=%q err=%v", got, err)
	}
	// 图片 loader 的逐 URL 判定同样放行（渲染层在缓存查询之前调用它）。
	loader := &webViewImageLoader{wv: wv}
	if !loader.AllowsURL(u) {
		t.Fatal("image loader 应放行已登记 blob URL")
	}

	// 未登记的 blob URL：按浏览器语义失败（不联网、不读盘）。
	if _, err := wv.loadExternalResource("blob:null/never-registered", PurposeImage); err == nil {
		t.Fatal("未登记的 blob URL 应失败")
	}
	if loader.AllowsURL("blob:null/never-registered") {
		t.Fatal("image loader 不应放行未登记的 blob URL")
	}

	// revoke 之后失效。
	jsc.RevokeBlobURL(u)
	if _, err := wv.loadExternalResource(u, PurposeImage); err == nil {
		t.Fatal("revoke 后的 blob URL 应失败")
	}
	if loader.AllowsURL(u) {
		t.Fatal("image loader 不应放行 revoke 后的 blob URL")
	}

	// 策略承诺不被放宽：http(s)/file/相对路径仍然被 DenyExternal 拒绝。
	for _, ref := range []string{
		"http://127.0.0.1:1/never.css",
		"https://127.0.0.1:1/never.css",
		"file:///C:/never-exists/wbui.css",
	} {
		if _, err := wv.loadExternalResource(ref, PurposeStylesheet); !errors.Is(err, ErrExternalResourceBlocked) {
			t.Errorf("%q 应被 DenyExternal 拒绝，得到 err=%v", ref, err)
		}
	}
}
