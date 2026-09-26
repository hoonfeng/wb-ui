package webkit

import (
	"bytes"
	"testing"
)

// renderPagePixels 加载 HTML、推进若干帧、返回渲染像素（对比用）。
func renderPagePixels(t *testing.T, html string, w, h int) []byte {
	t.Helper()
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(w, h)
	if err := wv.LoadHTML(html); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	for i := 0; i < 20; i++ {
		if el := wv.JSInterpreter().GetEventLoop(); el != nil {
			el.ProcessTasks(0)
		}
		wv.JSInterpreter().RunJobs()
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	pix, err := wv.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(pix) < w*h*4 {
		t.Fatalf("像素不足: %d < %d", len(pix), w*h*4)
	}
	return pix[:w*h*4]
}

func pixelsEqual(a, b []byte) bool { return bytes.Equal(a, b) }

// TestCommentTextNotRendered：静态 HTML 注释不得渲染（DOM 规范：注释不产生渲染内容）。
func TestCommentTextNotRendered(t *testing.T) {
	page := func(mid string) string {
		return `<!DOCTYPE html><html><body style="margin:0;background:#ffffff">
<div style="width:300px;height:60px;background:#ffffff">AB` + mid + `CD</div>
</body></html>`
	}
	a := renderPagePixels(t, page(`<!-- HIDDENCOMMENT -->`), 320, 80)
	b := renderPagePixels(t, page(``), 320, 80)
	if !pixelsEqual(a, b) {
		t.Fatalf("静态 HTML 注释文本被渲染：带注释与不带注释的像素不一致")
	}
}

// TestCommentInsideSVGNotRendered：静态 SVG 内部注释不得渲染
// （gou-ide 的 SvgIcon.vue 每个图标分支前都有 <!-- Folder --> 等注释）。
func TestCommentInsideSVGNotRendered(t *testing.T) {
	page := func(mid string) string {
		return `<!DOCTYPE html><html><body style="margin:0;background:#ffffff">
<svg class="svg-icon" width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="#000000" stroke-width="2">` + mid + `<path d="M6 9 12 15 18 9"/></svg>
</body></html>`
	}
	a := renderPagePixels(t, page(`<!-- Chevron Down (Rotated chevron-right) -->`), 120, 80)
	b := renderPagePixels(t, page(``), 120, 80)
	if !pixelsEqual(a, b) {
		t.Fatalf("静态 SVG 内注释文本被渲染：像素不一致")
	}
}

// TestDynamicCommentNotRendered：运行时 innerHTML/svg 插入的注释不得渲染
// （Vue 运行时路径：图标与插件 UI 全部经此插入）。
func TestDynamicCommentNotRendered(t *testing.T) {
	page := func(inner string) string {
		return `<!DOCTYPE html><html><body style="margin:0;background:#ffffff">
<div id="host"></div>
<script>
document.getElementById('host').innerHTML = '<svg class="svg-icon" width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="#000000">` + inner + `<path d="M6 9 12 15 18 9"/></svg>';
</script>
</body></html>`
	}
	a := renderPagePixels(t, page(`<!-- Folder -->`), 120, 80)
	b := renderPagePixels(t, page(``), 120, 80)
	if !pixelsEqual(a, b) {
		t.Fatalf("运行时插入的注释文本被渲染：像素不一致")
	}
}
