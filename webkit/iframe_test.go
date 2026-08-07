package webkit

import (
	"strings"
	"testing"

	"wb-ui/dom"
	"wb-ui/page"
)

// TestIFrameLoadAndRender 验证 iframe 子文档加载：
//   1. 主文档含 <iframe src="data:..."> → 子 Frame 创建并解析子文档
//   2. EnsureLayout 后子 Frame 视口同步为 iframe 内容框尺寸（width/height 属性）
//   3. 渲染完整跑通（子文档经 PaintIFrame 画进 iframe 内容框）
func TestIFrameLoadAndRender(t *testing.T) {
	wv := NewWebView()
	child := `<html><body><p id='c1'>child document</p><div id='blue' style='width:80px;height:40px;background:#3366ff'></div></body></html>`
	// data URI：< > 需 URL 编码，否则 data: 段内出现 <> 会被当作 HTML 实体起点
	dataURI := "data:text/html," + strings.ReplaceAll(strings.ReplaceAll(child, "<", "%3C"), ">", "%3E")
	src := `<html><body><div id="p1">parent content</div><iframe id="f1" src="` + dataURI + `" width="200" height="100"></iframe></body></html>`

	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	wv.Resize(400, 300)
	wv.RebuildRenderTree()
	wv.EnsureLayout()

	// 1. 主文档找到 iframe 元素 → 子 Frame 已注册
	doc := wv.MainFrame().Document()
	if doc == nil {
		t.Fatal("no document")
	}
	iframeEl := doc.GetElementById("f1")
	if iframeEl == nil {
		t.Fatal("iframe#f1 not found in parent document")
	}
	if page.IFrameCount() != 1 {
		t.Fatalf("IFrameCount=%d, want 1", page.IFrameCount())
	}
	sub := page.IFrameFrame(iframeEl)
	if sub == nil {
		t.Fatal("iframe 子 Frame 未注册")
	}

	// 2. 子文档已解析（含 <p id=c1>）
	subDoc := sub.Document()
	if subDoc == nil {
		t.Fatal("子文档 nil")
	}
	t.Logf("子文档 text=%q", subDoc.TextContent())
	if p := subDoc.GetElementById("c1"); p == nil {
		t.Fatal("子文档缺少 #c1")
	} else if got := p.TextContent(); !strings.Contains(got, "child document") {
		t.Fatalf("子文档文本=%q", got)
	}

	// 3. 布局同步：iframe 尺寸 200x100（width/height 属性）→ 子 Frame 视口
	wv.EnsureLayout()
	sub.LayoutNow()
	if sub.ViewportWidth() != 200 || sub.ViewportHeight() != 100 {
		t.Fatalf("子视口=%dx%d, want 200x100", sub.ViewportWidth(), sub.ViewportHeight())
	}

	// 4. 完整渲染（PaintIFrame 走通，不 panic）
	img, err := wv.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(img) == 0 {
		t.Fatal("Render 返回空")
	}

	// 5. 子 Frame 渲染树包含蓝色块（布局几何正确）
	blue := subDoc.GetElementById("blue")
	rv := sub.RenderView()
	if rv == nil {
		t.Fatal("子 RenderView nil")
	}
	if box := rv.FindRenderBoxForNode(blue); box == nil {
		t.Fatal("子文档 #blue 无 RenderBox")
	}
}

// TestIFramePruneOnReload 验证 LoadHTML 加载新主文档后，旧文档的 iframe
// 子 Frame 被清理（不泄漏、不残留）。
func TestIFramePruneOnReload(t *testing.T) {
	wv := NewWebView()
	dataURI := "data:text/html,%3Chtml%3E%3Cbody%3Eold%3C/body%3E%3C/html%3E"
	src1 := `<html><body><iframe id="f1" src="` + dataURI + `" width="100" height="50"></iframe></body></html>`
	src2 := `<html><body>no iframe</body></html>`
	if err := wv.LoadHTML(src1); err != nil {
		t.Fatalf("LoadHTML#1: %v", err)
	}
	if page.IFrameCount() != 1 {
		t.Fatalf("IFrameCount=%d after load1, want 1", page.IFrameCount())
	}
	if err := wv.LoadHTML(src2); err != nil {
		t.Fatalf("LoadHTML#2: %v", err)
	}
	if page.IFrameCount() != 0 {
		t.Fatalf("IFrameCount=%d after reload, want 0 (pruned)", page.IFrameCount())
	}
}

// TestIFrameNoSrc 验证无 src 的 iframe 不创建子 Frame。
func TestIFrameNoSrc(t *testing.T) {
	wv := NewWebView()
	src := `<html><body><iframe id="f1"></iframe></body></html>`
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	if page.IFrameCount() != 0 {
		t.Fatalf("IFrameCount=%d, want 0 (no src)", page.IFrameCount())
	}
	if el := wv.MainFrame().Document().GetElementById("f1"); el != nil {
		if got := el.LocalName(); got != "iframe" {
			t.Fatalf("LocalName=%q", got)
		}
	}
}

// TestIFramePixels 像素级验证：子文档内容（蓝色块 #33aaff）真的被
// PaintIFrame 画进了父文档的 iframe 内容框（而非空白/黑块）。
func TestIFramePixels(t *testing.T) {
	wv := NewWebView()
	child := `<html><body><div id='blue' style='width:60px;height:30px;background:#33aaff'></div></body></html>`
	dataURI := "data:text/html," + strings.ReplaceAll(strings.ReplaceAll(child, "<", "%3C"), ">", "%3E")
	src := `<html><body style="margin:0"><iframe id="f1" src="` + dataURI + `" width="200" height="100" style="border:0"></iframe></body></html>`
	if err := wv.LoadHTML(src); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	wv.Resize(240, 140)
	wv.RebuildRenderTree()
	wv.EnsureLayout()
	img, err := wv.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// RGBA 缓冲：wv.width × wv.height × 4
	w, h := 240, 140
	found := false
	for y := 0; y < h && !found; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			if i+3 >= len(img) {
				continue
			}
			r, g, b, a := img[i], img[i+1], img[i+2], img[i+3]
			// #33aaff = rgb(51,170,255)；允许 ±8 容差（抗锯齿）
			if a > 200 && absDiff(int(r), 51) <= 8 && absDiff(int(g), 170) <= 8 && absDiff(int(b), 255) <= 8 {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("渲染图中未找到 iframe 子文档的蓝色块 #33aaff")
	}
}

func absDiff(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

var _ dom.Node // 保持 dom import（Element 断言辅助）
