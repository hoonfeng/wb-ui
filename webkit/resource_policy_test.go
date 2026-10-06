// 资源策略（ResourcePolicy）的轻量单测：默认值按模式推导、显式设置覆盖
// 模式、逐 URL 判定语义（决策 1 / 文档 §5 G9 的 TC-M-901..907 纯逻辑部分）。
//
// 真渲染的端到端判定（四配置 × 矩阵页截图与像素比对）由媒体验证工装
//（cmd/psai -media，见 dev/media/）负责；这里只钉住策略判定的语义，
// 保证「修 data: 放行」不会把安全默认放宽。

package webkit

import "testing"

func TestResourcePolicyDefaultsPerMode(t *testing.T) {
	browser := &WebView{}
	if got := browser.ResourcePolicy(); got != AllowAll {
		t.Fatalf("ModeBrowser 默认策略 = %v，want AllowAll（历史行为不变）", got)
	}
	toolkit := &WebView{mode: ModeToolkit}
	if got := toolkit.ResourcePolicy(); got != DenyExternal {
		t.Fatalf("ModeToolkit 默认策略 = %v，want DenyExternal（安全默认不变）", got)
	}
}

func TestResourcePolicyExplicitOverridesMode(t *testing.T) {
	// Toolkit 显式声明 AllowHostResolved：外部通道仍拒（放行发生在 resolver
	// 通道，见 loadExternalResource）。
	tk := &WebView{mode: ModeToolkit}
	tk.SetResourcePolicy(AllowHostResolved)
	if got := tk.ResourcePolicy(); got != AllowHostResolved {
		t.Fatalf("ResourcePolicy() = %v，want AllowHostResolved", got)
	}
	if tk.allowsExternalURLs() {
		t.Fatal("AllowHostResolved 不应放行外部通道（不代宿主联网/读盘）")
	}
	// Browser 显式声明 DenyExternal：外部通道应被拒（显式设置优先于模式）。
	br := &WebView{}
	br.SetResourcePolicy(DenyExternal)
	if br.allowsExternalURLs() {
		t.Fatal("显式 DenyExternal 在 ModeBrowser 下也应拒绝外部通道")
	}
	if got := br.ResourcePolicy(); got != DenyExternal {
		t.Fatalf("ResourcePolicy() = %v，want DenyExternal", got)
	}
}

func TestImageLoaderAllowsURLByPolicy(t *testing.T) {
	const (
		fileURL = "file:///C:/tmp/pic.png"
		httpURL = "http://example.com/pic.png"
		dataURL = "data:image/png;base64,iVBORw0KGgo="
	)

	// ① Toolkit 默认（DenyExternal）：data: 放行，file/http 拒（TC-M-901/902）。
	tk := &WebView{mode: ModeToolkit}
	lt := &webViewImageLoader{wv: tk}
	if !lt.AllowsURL(dataURL) {
		t.Fatal("DenyExternal 下 data: 必须放行（自带内容，与策略解耦）")
	}
	if lt.AllowsURL(fileURL) {
		t.Fatal("DenyExternal 下 file:// 应被拒")
	}
	if lt.AllowsURL(httpURL) {
		t.Fatal("DenyExternal 下 http(s) 应被拒")
	}

	// ② Browser 默认（AllowAll）：全放行（历史行为）。
	ltb := &webViewImageLoader{wv: &WebView{}}
	if !ltb.AllowsURL(fileURL) || !ltb.AllowsURL(httpURL) || !ltb.AllowsURL(dataURL) {
		t.Fatal("AllowAll 下 data:/file/http 都应放行")
	}

	// ③ AllowHostResolved：只放行宿主 resolver 明确提供的引用（TC-M-903/904）。
	tk.SetResourcePolicy(AllowHostResolved)
	if lt.AllowsURL(fileURL) {
		t.Fatal("AllowHostResolved 且 resolver 未提供该引用 → 应拒")
	}
	tk.SetResourceResolver(func(ref string) (string, bool) {
		if ref == fileURL {
			return "<png bytes>", true
		}
		return "", false
	})
	if !lt.AllowsURL(fileURL) {
		t.Fatal("AllowHostResolved 且 resolver 命中 → 应放行")
	}
	if lt.AllowsURL("file:///C:/tmp/other.png") {
		t.Fatal("AllowHostResolved 下 resolver 未命中的引用应拒")
	}
	if lt.AllowsURL(httpURL) {
		t.Fatal("AllowHostResolved 不代宿主联网：http(s) 仍拒")
	}
	if !lt.AllowsURL(dataURL) {
		t.Fatal("data: 与策略解耦：AllowHostResolved 下同样放行")
	}

	// ④ 运行时切换策略立即生效（TC-M-907）。
	tk.SetResourcePolicy(AllowAll)
	if !lt.AllowsURL(httpURL) {
		t.Fatal("切换为 AllowAll 后 http(s) 应放行")
	}
}

func TestResourcePolicyInvalidatedCache(t *testing.T) {
	// 策略切换必须清资源缓存：否则上一策略下取回的内容会穿透新策略
	//（与 SetResourceResolver 同一条规矩）。
	wv := &WebView{mode: ModeToolkit}
	wv.resourceCacheFor().put("http://example.com/a.png", cachedResource{content: "old"})
	if _, ok := wv.resourceCacheFor().get("http://example.com/a.png"); !ok {
		t.Fatal("前置条件失败：缓存未写入")
	}
	wv.SetResourcePolicy(AllowHostResolved)
	if _, ok := wv.resourceCacheFor().get("http://example.com/a.png"); ok {
		t.Fatal("SetResourcePolicy 后缓存应已清空")
	}
}
