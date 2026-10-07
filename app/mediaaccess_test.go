// 媒体资源门禁（宿主侧）的单测：安全默认（未装配判定 = 拒绝）、策略驱动、
// 判定缓存按策略换键、引擎侧接线。
//
// 真渲染的端到端判定由 cmd/psai -media 的四配置矩阵负责（dev/media/）。

package app

import (
	"os"
	"path/filepath"
	"testing"

	"wb-ui/engine/js/bindings"
	"wb-ui/webkit"
)

// testGateAllowAll 装配「允许一切媒体引用」的判定（等价宿主显式声明 AllowAll）。
// 供只关心「src → 路径映射/链路」的既有用例复用——它们必须先让引用获得授权，
// 否则会撞上「未装配 = 拒绝」的安全默认（见 mediaaccess.go）。
func testGateAllowAll(t *testing.T) {
	t.Helper()
	SetMediaRefGate(func(string) bool { return true },
		func() webkit.ResourcePolicy { return webkit.AllowAll })
	t.Cleanup(func() { SetMediaRefGate(nil, nil) })
}

// testSampleFileURL 返回 dev/media/samples 下样本的 file:// URL（真实文件）。
func testSampleFileURL(t *testing.T, name string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "dev", "media", "samples", name))
	if err != nil {
		t.Fatalf("解析样本路径失败：%v", err)
	}
	return "file:///" + filepath.ToSlash(abs)
}

// wireGate 把 gate 接到真实 WebView 的策略判定上（与 Install* 的接线方式相同）。
func wireGate(t *testing.T, wv *webkit.WebView) {
	t.Helper()
	t.Cleanup(func() {
		SetMediaRefGate(nil, nil)
		if wv != nil {
			wv.Destroy()
		}
	})
	SetMediaRefGate(mediaRefGateFor(wv), mediaPolicyOf(wv))
}

func TestMediaGateDefaultsToDeny(t *testing.T) {
	SetMediaRefGate(nil, nil) // 显式清空：模拟「宿主没装配判定」
	t.Cleanup(func() { SetMediaRefGate(nil, nil) })
	if p, ok := mediaSrcToPath("file:///F:/p/clip.mp4", ""); ok {
		t.Fatalf("未装配判定时不得返回路径（安全默认：一律拒绝），got %q", p)
	}
	if p, ok := mediaSrcToPath("clip.mp4", "file:///F:/p/index.html"); ok {
		t.Fatalf("未装配判定时相对引用也不得返回路径，got %q", p)
	}
	if p, ok := mediaSrcToPath("sine-440-1s.wav", ""); ok {
		t.Fatalf("未装配判定时裸名不得返回路径，got %q", p)
	}
	// ★ data: 自带内容、不经外部通道——与资源策略解耦，未装配判定时同样放行
	//（与引擎侧 MediaResourceAllowed 的判定一致）。
	p, ok := mediaSrcToPath("data:audio/wav;base64,UklGRiQAAABXQVZF", "")
	if !ok {
		t.Fatal("data: 恒放行（与策略解耦），未装配判定时也必须落盘")
	}
	if st, err := os.Stat(p); err != nil || st.Size() == 0 {
		t.Fatalf("data: 落盘路径不可用：%q（%v）", p, err)
	}
	CleanupMediaDataURLFiles()
}

func TestMediaGateFollowsResourcePolicy(t *testing.T) {
	wv := webkit.NewWebView() // ModeBrowser → AllowAll
	wireGate(t, wv)
	fileURL := testSampleFileURL(t, "sine-440-1s.wav")

	// ① AllowAll（ModeBrowser 默认）：本地媒体放行——路径真的来自这个文件。
	got, ok := mediaSrcToPath(fileURL, "")
	if !ok {
		t.Fatal("AllowAll 下本地媒体应放行")
	}
	if st, err := os.Stat(got); err != nil || st.IsDir() {
		t.Fatalf("返回路径应是可读文件：%q（%v）", got, err)
	}
	// ② DenyExternal：**同一个引用**（文件确实存在）被拒 —— 证明拦的是策略，
	//    不是「文件不存在」。三条宿主链路（元数据/抽帧/PCM）都从本函数拿路径，
	//    因此它们在未授权时都读不到盘。
	wv.SetResourcePolicy(webkit.DenyExternal)
	if p, ok := mediaSrcToPath(fileURL, ""); ok {
		t.Fatalf("DenyExternal 下本地媒体应被拒（不得读盘），got %q", p)
	}
	if p, ok := mediaSrcToPath("sine-440-1s.wav", "file:///F:/samples/index.html"); ok {
		t.Fatalf("DenyExternal 下相对引用应被拒，got %q", p)
	}
	// ③ 运行中切回 AllowAll：立即生效（判定缓存按策略换键，不读旧档结果）。
	wv.SetResourcePolicy(webkit.AllowAll)
	if _, ok := mediaSrcToPath(fileURL, ""); !ok {
		t.Fatal("切回 AllowAll 后应立即放行（缓存不得穿透新策略）")
	}
}

func TestMediaGateAllowHostResolvedNeedsResolver(t *testing.T) {
	wv := webkit.NewWebViewWithMode(webkit.ModeToolkit)
	wireGate(t, wv)
	wv.SetResourcePolicy(webkit.AllowHostResolved)
	fileURL := testSampleFileURL(t, "sine-440-1s.wav")

	// ① resolver 未提供该引用 → 拒（AllowHostResolved 的语义）。
	if p, ok := mediaSrcToPath(fileURL, ""); ok {
		t.Fatalf("AllowHostResolved 且 resolver 未命中 → 应拒，got %q", p)
	}
	// ② 宿主显式提供该引用 → 放行。
	wv.SetResourceResolver(func(ref string) (string, bool) {
		if ref == fileURL {
			return "<wav bytes>", true
		}
		return "", false
	})
	ClearMediaRefCache() // ★ 契约：运行中替换 resolver 后必须清判定缓存
	if _, ok := mediaSrcToPath(fileURL, ""); !ok {
		t.Fatal("resolver 命中后应放行（AllowHostResolved）")
	}
	// ③ resolver 未命中的另一个引用仍拒。
	other := testSampleFileURL(t, "testsrc-1s.mp4")
	if p, ok := mediaSrcToPath(other, ""); ok {
		t.Fatalf("resolver 未命中的引用应拒，got %q", p)
	}
}

func TestInstallMediaMetadataResolverWiresEngineGate(t *testing.T) {
	wv := webkit.NewWebView()
	t.Cleanup(func() {
		bindings.MediaSrcAllowed = nil
		bindings.MediaMetadataResolver = nil
		SetMediaRefGate(nil, nil)
		wv.Destroy()
	})
	InstallMediaMetadataResolver(wv, "")
	if bindings.MediaSrcAllowed == nil {
		t.Fatal("装配后引擎侧资源选择门禁必须已接线（否则媒体通道绕过资源策略）")
	}
	// 同一判定：DenyExternal 下引擎侧的资源选择也拒（数据: 仍放行）。
	wv.SetResourcePolicy(webkit.DenyExternal)
	if bindings.MediaSrcAllowed("file:///C:/tmp/clip.mp4") {
		t.Fatal("DenyExternal 下引擎侧媒体引用应被拒")
	}
	if !bindings.MediaSrcAllowed("data:audio/wav;base64,UklGRiQAAABXQVZF") {
		t.Fatal("data: 与策略解耦：引擎侧同样恒放行")
	}
}
