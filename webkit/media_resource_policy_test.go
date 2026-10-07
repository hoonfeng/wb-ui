// 媒体资源门禁（PurposeMedia）的单测：`<video>/<audio>` 的判定语义必须与
// `<img>`/`<script>`/`<link>` 完全一致（语义收口，文档 §9.9）。
//
// 纯逻辑（零值 WebView + 假 resolver）：真渲染的端到端判定（四配置 × 矩阵页
// 截图与像素比对）由 cmd/psai -media 负责，见 dev/media/。

package webkit

import "testing"

func TestMediaResourceAllowedDenyExternal(t *testing.T) {
	wv := &WebView{mode: ModeToolkit} // 默认 DenyExternal
	if wv.MediaResourceAllowed("file:///C:/tmp/clip.mp4") {
		t.Fatal("DenyExternal 下媒体的 file:// 应被拒（与 <img> 一致）")
	}
	if wv.MediaResourceAllowed("clip.mp4") {
		t.Fatal("DenyExternal 下媒体的相对路径应被拒（与 <img> 一致）")
	}
	if wv.MediaResourceAllowed("") {
		t.Fatal("空引用应被拒")
	}
	if !wv.MediaResourceAllowed("data:video/mp4;base64,AAAAIGZ0eXA") {
		t.Fatal("data: 与策略解耦：DenyExternal 下也必须放行")
	}
}

func TestMediaResourceAllowedHttpAndBlobAlwaysDenied(t *testing.T) {
	// 媒体通道没有网络栈（引擎不代宿主联网，宿主也没有网络媒体解码通道）：
	// http(s)/blob 在任何策略下都不放行——这是**能力边界**，与 <img> 的
	// fetchResource 通道不同，见 media_resource_policy.go 的文件头注释。
	for _, pol := range []ResourcePolicy{DenyExternal, AllowHostResolved, AllowAll} {
		wv := &WebView{}
		wv.SetResourcePolicy(pol)
		for _, ref := range []string{
			"http://example.com/a.mp4",
			"https://example.com/a.mp4",
			"blob:null/1234",
		} {
			if wv.MediaResourceAllowed(ref) {
				t.Fatalf("%s 下 %q 应恒拒（媒体通道无网络栈）", pol, ref)
			}
		}
		if !wv.MediaResourceAllowed("data:audio/wav;base64,UklGRiQAAABXQVZF") {
			t.Fatalf("%s 下 data: 必须放行（与策略解耦）", pol)
		}
	}
}

func TestMediaResourceAllowedAllowHostResolved(t *testing.T) {
	const fileURL = "file:///C:/tmp/clip.mp4"
	wv := &WebView{mode: ModeToolkit}
	wv.SetResourcePolicy(AllowHostResolved)
	if wv.MediaResourceAllowed(fileURL) {
		t.Fatal("AllowHostResolved 且 resolver 未提供该引用 → 应拒")
	}
	wv.SetResourceResolver(func(ref string) (string, bool) {
		if ref == fileURL {
			return "<mp4 bytes>", true
		}
		return "", false
	})
	if !wv.MediaResourceAllowed(fileURL) {
		t.Fatal("AllowHostResolved 且 resolver 命中 → 应放行")
	}
	if wv.MediaResourceAllowed("file:///C:/tmp/other.mp4") {
		t.Fatal("AllowHostResolved 下 resolver 未命中的引用应拒")
	}
	if www := "http://example.com/a.mp4"; wv.MediaResourceAllowed(www) {
		t.Fatal("AllowHostResolved 不代宿主联网：http(s) 仍拒")
	}
	if !wv.MediaResourceAllowed("data:audio/wav;base64,UklGRiQAAABXQVZF") {
		t.Fatal("data: 与策略解耦：AllowHostResolved 下同样放行")
	}
}

func TestMediaResourceAllowedResolvesRelativeRefBeforeResolver(t *testing.T) {
	// 相对引用：resolver 先按原样问一次，再按**文档基准绝对化**后问一次
	//（与 loadExternalResource 的两轮询问一致）——探针的 AllowHostResolved 配置
	// 正是靠绝对化这一轮命中 samples 目录内的媒体（hostSamplesResolver 只认
	// 目录内路径，裸文件名不在其内）。
	const (
		base    = "file:///F:/samples/_matrix.html"
		rel     = "testsrc-1s.mp4"
		absRef  = "file:///F:/samples/testsrc-1s.mp4"
		otherWA = "file:///F:/samples/other.mp4"
	)
	wv := &WebView{mode: ModeToolkit}
	wv.SetResourcePolicy(AllowHostResolved)
	wv.currentURL = base
	if wv.MediaResourceAllowed(rel) {
		t.Fatal("resolver 尚未提供时应拒")
	}
	asked := map[string]bool{}
	wv.SetResourceResolver(func(ref string) (string, bool) {
		asked[ref] = true
		if ref == absRef {
			return "<mp4 bytes>", true
		}
		return "", false
	})
	if !wv.MediaResourceAllowed(rel) {
		t.Fatal("resolver 命中绝对化后的引用 → 应放行（探针 hostSamplesResolver 的路径）")
	}
	if !asked[rel] || !asked[absRef] {
		t.Fatalf("resolver 应被问两轮（原样 %q + 绝对化 %q），实际 %v", rel, absRef, asked)
	}
	if wv.MediaResourceAllowed(otherWA) {
		t.Fatal("目录外引用应拒")
	}
}

func TestMediaResourceAllowedAllowAllAndLiveSwitching(t *testing.T) {
	const fileURL = "file:///C:/tmp/clip.mp4"
	wv := &WebView{} // ModeBrowser → AllowAll
	if !wv.MediaResourceAllowed(fileURL) {
		t.Fatal("AllowAll 下媒体的 file:// 应放行")
	}
	if !wv.MediaResourceAllowed("clip.mp4") {
		t.Fatal("AllowAll 下媒体的相对路径应放行")
	}
	// 运行时切换策略立即生效（TC-M-907 的切换语义在媒体链路上同样成立）。
	wv.SetResourcePolicy(DenyExternal)
	if wv.MediaResourceAllowed(fileURL) {
		t.Fatal("切换为 DenyExternal 后媒体的 file:// 应立即被拒")
	}
	if !wv.MediaResourceAllowed("data:video/mp4;base64,AAAAIGZ0eXA") {
		t.Fatal("切换策略不影响 data:（与策略解耦）")
	}
}
