package webkit

import "testing"

// TestAppendedCommentNotRendered：运行时 appendChild(comment) 到**已存在**的
// <svg> 内不得渲染出文字。
// 背景：Vue patch 会逐个插入注释节点（SvgIcon.vue 的 <!-- Folder --> 等），
// 若增量路径把注释当文本渲染，图标位置会出现注释原文并与图标重叠。
func TestAppendedCommentNotRendered(t *testing.T) {
	page := func(js string) string {
		return `<!DOCTYPE html><html><body style="margin:0;background:#ffffff">
<svg class="svg-icon" width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="#000000" stroke-width="2"><path d="M6 9 12 15 18 9"/></svg>
<script>` + js + `</script>
</body></html>`
	}
	base := renderPagePixels(t, page(``), 120, 80)
	withC := renderPagePixels(t, page(`document.querySelector("svg").appendChild(document.createComment(" HIDDENCOMMENTTEXT "));`), 120, 80)
	if !pixelsEqual(base, withC) {
		t.Fatalf("运行时 appendChild 的注释被渲染（Vue patch 路径）")
	}
}

// TestAppendedCommentInDivNotRendered：div 内 appendChild(comment) 同样不得渲染。
func TestAppendedCommentInDivNotRendered(t *testing.T) {
	page := func(js string) string {
		return `<!DOCTYPE html><html><body style="margin:0;background:#ffffff">
<div id="host" style="width:300px;height:60px">AB</div>
<script>` + js + `</script>
</body></html>`
	}
	base := renderPagePixels(t, page(``), 320, 80)
	withC := renderPagePixels(t, page(`document.getElementById("host").appendChild(document.createComment(" HIDDENCOMMENTTEXT "));`), 320, 80)
	if !pixelsEqual(base, withC) {
		t.Fatalf("div 内运行时 appendChild 的注释被渲染")
	}
}

// TestVueStyleCommentInsertionNotRendered：模拟 Vue 的插入顺序
// （先建 svg 子节点，再逐个 insertBefore 注释到既有子节点前）。
func TestVueStyleCommentInsertionNotRendered(t *testing.T) {
	page := func(js string) string {
		return `<!DOCTYPE html><html><body style="margin:0;background:#ffffff">
<svg class="svg-icon" width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="#000000" stroke-width="2"><path d="M6 9 12 15 18 9"/></svg>
<script>
var s = document.querySelector("svg");
var first = s.firstChild;
` + js + `
</script>
</body></html>`
	}
	base := renderPagePixels(t, page(``), 120, 80)
	withC := renderPagePixels(t, page(`s.insertBefore(document.createComment(" Folder "), first);`), 120, 80)
	if !pixelsEqual(base, withC) {
		t.Fatalf("insertBefore 插入的注释被渲染")
	}
}
