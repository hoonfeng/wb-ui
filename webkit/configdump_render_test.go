package webkit

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// pixToImage 把 RGBA 字节缓冲转为 image.NRGBA。
func pixToImage(pix []byte, w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(pix) && i < w*h*4; i++ {
		img.Pix[i] = pix[i]
	}
	return img
}

// TestRenderConfigDump 渲染真实配置器 HTML（由主项目 TestDumpConfigHTML
// 生成），保存截图用于检查欢迎语换行 / 按钮文字居中。
func TestRenderConfigDump(t *testing.T) {
	wd, _ := os.Getwd()
	t.Logf("cwd = %s", wd)
	htmlPath := filepath.Join("..", "..", "直播挂件助手", "dev", "cfgdump", "config_dump.html")
	if abs, err := filepath.Abs(htmlPath); err == nil {
		t.Logf("trying: %s", abs)
	}
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Skipf("config_dump.html not generated: %v", err)
	}
	wv := NewWebView()
	wv.Resize(900, 640)
	if err := wv.LoadHTML(string(data)); err != nil {
		t.Fatalf("load: %v", err)
	}
	if mf := wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.RebuildRenderTree()
			fr.SetNeedsLayout(true)
		}
	}
	wv.EnsureLayout()
	pix, err := wv.Render()
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	out := filepath.Join("..", "..", "直播挂件助手", "dev", "cfgdump", "config_wbui.png")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img := pixToImage(pix, 900, 640)
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("saved %s", out)
}

// TestRenderConfigDumpProp 渲染配置器并打开 text 挂件属性面板（openProp），
// 检查欢迎语内容预览（.txt-view）是否单行、按钮文字是否居中。
func TestRenderConfigDumpProp(t *testing.T) {
	htmlPath := filepath.Join("..", "..", "直播挂件助手", "dev", "cfgdump", "config_dump.html")
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Skipf("config_dump.html not generated: %v", err)
	}
	wv := NewWebView()
	wv.Resize(900, 640)
	if err := wv.LoadHTML(string(data)); err != nil {
		t.Fatalf("load: %v", err)
	}
	if mf := wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.RebuildRenderTree()
			fr.SetNeedsLayout(true)
		}
	}
	wv.EnsureLayout()
	// 模拟用户双击画布上的欢迎语挂件 → openProp('text-1')
	// （wbox 通过 renderStage 生成，先检查是否存在）
	if _, err := wv.EvalJS(`window.openProp && openProp('text-1') || 'no-openProp'`); err != nil {
		t.Fatalf("openProp: %v", err)
	}
	// openProp 修改 DOM（innerHTML）→ 渲染树需重建（EnsureLayout 只 relayout）
	if mf := wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.RebuildRenderTree()
			fr.SetNeedsLayout(true)
		}
	}
	wv.EnsureLayout()
	// 检查 txt-view 几何
	v, err := wv.EvalJS(`JSON.stringify((function(){
		var e = document.querySelector('.txt-view');
		if (!e) return 'no-txt-view';
		var r = e.getBoundingClientRect();
		var d = document.querySelector('.txt-view').textContent;
		return {w: Math.round(r.width), h: Math.round(r.height), text: d};
	})())`)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	t.Logf("txt-view = %s", v.ToString())
	pix, err := wv.Render()
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	out := filepath.Join("..", "..", "直播挂件助手", "dev", "cfgdump", "config_prop_wbui.png")
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img := pixToImage(pix, 900, 640)
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("saved %s", out)
}
