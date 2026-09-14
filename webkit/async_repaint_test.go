package webkit

// 异步资源到位 → 引擎自动置脏重绘（浏览器语义）。
//
// 为什么必须锁定：app.Host.Run 是**按需渲染**——`rv.IsDirty()` 为假就跳过
// Clear+Paint+Present。图片字节由后台 goroutine 取回后，如果没有任何人置脏，
// 真实按需渲染的宿主就**永远看不到这张图**（缓存里明明有，画面不变）。渲染层
// 的 SetBackgroundImageLoadedCallback 此前只有测试在用，主链路从未接线。

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"wb-ui/engine/rendering"
)

// TestAsyncImageLoadMarksFrameDirty：图片到达时必须由**引擎自己**置脏。
// 反向验证：注释掉 onAsyncImageLoaded 里的 rv.MarkAllDirty（或整条监听器
// 接线）后，本测试的「图片到位后视图未置脏」会立刻失败。
func TestAsyncImageLoadMarksFrameDirty(t *testing.T) {
	png := redPNG4x4(t)
	requested := make(chan struct{}, 4)
	release := make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("/slow.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><style>#pic{width:24px;height:24px}</style>`+
			`</head><body><img id="pic" src="slow.png"></body></html>`)
	})
	mux.HandleFunc("/slow.png", func(w http.ResponseWriter, r *http.Request) {
		select {
		case requested <- struct{}{}:
		default:
		}
		<-release // 扣住响应：让测试能观察到「图片尚未到达」的中间状态
		w.Header().Set("Content-Type", "image/png")
		w.Write(png)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	wv := modeWebView(t, ModeBrowser)
	loaded := make(chan string, 8)
	wv.SetOnResourceLoaded(func(u string) {
		select {
		case loaded <- u:
		default:
		}
	})
	if err := wv.LoadURL(srv.URL + "/slow.html"); err != nil {
		t.Fatalf("LoadURL: %v", err)
	}
	// 渲染一帧即触发图片取回（图片 loader 在 Paint 路径上）。
	if _, err := wv.Render(); err != nil {
		t.Fatalf("Render: %v", err)
	}
	select {
	case <-requested:
	case <-time.After(5 * time.Second):
		t.Fatal("服务器未收到 /slow.png（图片请求未发出）")
	}
	// 稳定一帧 + 清零脏标记：否则「图片到位后置脏」可能来自其它脏源（布局/
	// 样式重扫），断言就抓不住本回归。
	if _, err := wv.Render(); err != nil {
		t.Fatalf("Render: %v", err)
	}
	rv := wv.RenderView()
	if rv == nil {
		t.Fatal("RenderView 缺失")
	}
	rv.ClearDirty()
	if rv.IsDirty() {
		t.Fatal("前置条件失败：清零脏标记后视图仍为脏（有其它脏源在持续置脏）")
	}

	close(release) // 放行图片响应
	select {
	case u := <-loaded:
		if want := srv.URL + "/slow.png"; u != want {
			t.Errorf("资源到位通知 URL = %q, want %q", u, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("图片加载完成后没有收到资源到位通知（SetOnResourceLoaded 未触发）")
	}

	// ★ 核心断言：引擎自己置脏——宿主什么都没做。按需渲染的宿主靠它决定是否
	// 执行本帧的 Paint；不置脏 = 图片永远不会画出来。
	if !rv.IsDirty() {
		t.Errorf("图片到位后视图未置脏：按需渲染的宿主会永远跳过 Paint（图片不显示）")
	}

	// 像素级确认：这一帧真的把异步图片画了出来。
	if _, err := wv.Render(); err != nil {
		t.Fatalf("Render: %v", err)
	}
	doc := wv.Document()
	if doc == nil {
		t.Fatal("文档缺失")
	}
	el := doc.GetElementById("pic")
	var box *rendering.RenderBox
	if rv2 := wv.RenderView(); rv2 != nil && el != nil {
		box = rv2.FindRenderBoxForNode(el)
	}
	if box == nil {
		t.Fatal("#pic 渲染盒缺失")
	}
	pix, err := wv.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	cx := int(box.AbsoluteX() + box.Width()/2)
	cy := int(box.AbsoluteY() + box.Height()/2)
	r, g, b, a := pixelAt(pix, wv.Width(), cx, cy)
	if !(r > 200 && g < 80 && b < 80 && a > 200) {
		t.Errorf("异步加载的图片未画出：(%d,%d) = rgba(%d,%d,%d,%d), want 红", cx, cy, r, g, b, a)
	}
}
