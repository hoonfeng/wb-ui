package webkit

import (
	"strings"
	"testing"
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
