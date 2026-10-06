// 图片契约（文档 §5 G8 / U2）的测试：`<img>` 的 load / error 事件派发。
//
// 此前引擎只在图片加载完成后做「内部置脏重绘」，从不向 DOM 派发事件——
// 页面侧 `img.onload` / `addEventListener('load')` / `onerror` 永不触发，
// 懒加载、骨架屏、失败重试、占位图逻辑全部失效（媒体验证探针实测
// window.__mediaEvents 为空）。修复分两条路：
//   - 异步（file:// / http(s) / 宿主 resolver）：取字节的 goroutine 排队，
//     主线程在 Render 之后派发（DispatchEvent 调 JS，不能跨线程）；
//   - 同步（data: 内联）：绘制路径解码完成后，由 flushImageEvents 补派发。

package webkit

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const tinyPNGDataURI = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFAAH/q842iQAAAABJRU5ErkJggg=="

func fileURLOfPath(p string) string {
	p = filepath.ToSlash(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return "file://" + p
}

func renderRounds(wv *WebView, n int) error {
	for i := 0; i < n; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			return err
		}
	}
	return nil
}

// TestImgLoadEventDispatchedForDataURI：同一 data: URL 的两个 <img> 各收到
// 一次 load（HTML 规范：同一 URL 的每个元素都要收到——文档 TC-M-805），
// 且重复绘制不重复派发。
func TestImgLoadEventDispatchedForDataURI(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(200, 200)
	html := `<!DOCTYPE html><html><body style="margin:0">` +
		`<img id="a" src="` + tinyPNGDataURI + `" onload="window.__loads=(window.__loads||0)+1">` +
		`<img id="b" src="` + tinyPNGDataURI + `" onload="window.__loads=(window.__loads||0)+1">` +
		`</body></html>`
	if err := wv.LoadHTML(html); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	if err := renderRounds(wv, 5); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := dynEval(t, wv, `String(window.__loads)`); got != "2" {
		t.Fatalf("两个 <img> 应各收到一次 load（同一 URL），got %q", got)
	}
	// 重复绘制：load 只派发一次。
	if err := renderRounds(wv, 3); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := dynEval(t, wv, `String(window.__loads)`); got != "2" {
		t.Fatalf("load 不得重复派发，got %q", got)
	}
}

// TestImgLoadEventFiresAfterDynamicSrcChange：换 src 后新图片要再次派发 load
// （标记按 src 记账，而不是「一辈子只派发一次」）。
func TestImgLoadEventFiresAfterDynamicSrcChange(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(200, 200)
	html := `<!DOCTYPE html><html><body style="margin:0">` +
		`<img id="a" onload="window.__n=(window.__n||0)+1">` +
		`</body></html>`
	if err := wv.LoadHTML(html); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	if err := renderRounds(wv, 2); err != nil {
		t.Fatalf("Render: %v", err)
	}
	dynEval(t, wv, `(function(){ document.getElementById('a').src = `+jsQuoteForTest(tinyPNGDataURI)+`; return 'ok'; })()`)
	if err := renderRounds(wv, 4); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := dynEval(t, wv, `String(window.__n)`); got != "1" {
		t.Fatalf("设置 src 后应派发一次 load，got %q", got)
	}
}

// TestImgErrorEventDispatchedForMissingFile：文件不存在（ModeBrowser 允许
// file:// 通道，但读盘失败）→ 必须派发 error（而不是静默失败）。
func TestImgErrorEventDispatchedForMissingFile(t *testing.T) {
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(200, 200)
	missing := fileURLOfPath(filepath.Join(t.TempDir(), "missing", "nope.png"))
	html := `<!DOCTYPE html><html><body style="margin:0">` +
		`<img id="a" src="` + missing + `"` +
		` onerror="window.__errs=(window.__errs||0)+1" onload="window.__loads=(window.__loads||0)+1">` +
		`</body></html>`
	if err := wv.LoadHTML(html); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	// 异步取字节失败 → 排队 → 主线程派发；真实时间流逝才会完成。
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if err := renderRounds(wv, 1); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if got := dynEval(t, wv, `String(window.__errs)`); got == "1" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := dynEval(t, wv, `String(window.__errs)`); got != "1" {
		t.Fatalf("加载失败的 <img> 应派发一次 error，got %q", got)
	}
	if got := dynEval(t, wv, `String(window.__loads)`); got == "1" {
		t.Fatal("加载失败的 <img> 不应派发 load")
	}
}

func jsQuoteForTest(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
