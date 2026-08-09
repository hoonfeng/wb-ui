package webkit

import (
	"fmt"
	"strings"
	"testing"

	"wb-ui/layout"
	"wb-ui/rendering"
)

// TestFoldedSummaryRealCSS: 引擎级对照——真实 folded-summary 结构 + 真实 CSS
// + 500px 视口 + 长 desc（60 字），验证与 Edge 浏览器标准一致。
// Edge 参照（edge_folded_ref，bubble 422px）：title=完成摘要@(62,6 21x77)
// 每字一行（flex-shrink 把标题压到单字宽，desc 长时浏览器标准行为）、
// desc 358x58（2 行）。
// 回归：用户反馈「完成摘要这四个字一行一个了」——经 Edge 像素对照确认
// 这是浏览器 flex-shrink 的标准行为（desc 占满空间后标题被压缩），
// wb-ui 必须与 Edge 对齐（21x77），而非强行保持标题一行。
func TestFoldedSummaryRealCSS(t *testing.T) {
	html := `<html><head><style>
		body { margin: 0; font-family: sans-serif; }
		.msg-item { display: flex; gap: 8px; align-items: flex-start; }
		.msg-avatar { width: 24px; height: 24px; flex-shrink: 0; background: #ccc; }
		.msg-bubble { flex: 1; min-width: 0; max-width: 85%; font-size: 13px; line-height: 1.6; word-break: break-word; overflow-wrap: break-word; }
		.folded-summary { display: flex; align-items: center; gap: 5px; padding: 5px 10px; background: #fff; border: 1px solid #ccc; font-size: 12px; cursor: pointer; }
		.folded-title { color: #111; font-weight: 500; }
		.folded-desc { color: #888; }
	</style></head><body>
	<div class="msg-item">
		<div class="msg-avatar"></div>
		<div class="msg-bubble">
			<div class="folded-summary">
				<svg class="folded-chevron" viewBox="0 0 8 8" width="9" height="9"><path d="M2.6 1.2 L6.8 4 L2.6 6.8 Z"/></svg>
				<svg class="svg-icon" width="11" height="11" viewBox="0 0 24 24"><path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01"/></svg>
				<span class="folded-title">完成摘要</span>
				<span class="folded-desc">已为你完成全部请求并生成了完整摘要，共修改 12 个文件，包含样式修复、布局修复、事件派发、渲染树重建等多项改进，所有测试均已通过并完成提交。</span>
			</div>
		</div>
	</div>
	</body></html>`

	wv := NewWebView()
	wv.Resize(500, 600)
	if err := wv.LoadHTML(html); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	if _, err := wv.Render(); err != nil {
		t.Fatalf("Render: %v", err)
	}

	doc := wv.MainFrame().Document()
	title := doc.GetElementsByClassName("folded-title")
	if len(title) == 0 {
		t.Fatal("no .folded-title element")
	}
	rv := wv.RenderView()
	if rv == nil {
		t.Fatal("RenderView nil")
	}

	// 收集标题文字段（RenderText.Segments，递归）
	var segs []rendering.InlineTextBox
	var collect func(ro rendering.RenderObject)
	collect = func(ro rendering.RenderObject) {
		if ro == nil {
			return
		}
		if rt, ok := ro.(*rendering.RenderText); ok {
			segs = append(segs, rt.Segments()...)
		}
		for c := ro.FirstChild(); c != nil; c = c.NextSibling() {
			collect(c)
		}
	}
	lb := rv.FindRenderBoxForNode(title[0])
	if lb == nil {
		t.Fatal("no render box for .folded-title")
	}
	collect(lb)
	if len(segs) == 0 {
		t.Fatal("no text segments for .folded-title")
	}

	// 断言：与 Edge 一致——4 字逐行（每字一行，4 个不同 Y）
	ys := map[int]bool{}
	for _, s := range segs {
		ys[int(s.Y)] = true
	}
	t.Logf("folded-title segments=%d, distinct Y=%d", len(segs), len(ys))
	for _, s := range segs {
		t.Logf("  seg %d-%d @(%.0f,%.0f)", s.Start, s.Start+s.Len, s.X, s.Y)
	}
	// 长 desc 时浏览器标准：flex-shrink 把标题压到单字宽 → 4 行（4 个 Y）
	if len(ys) < 4 {
		t.Fatalf("长 desc 下标题应每字一行（Edge 4 行 21x77），实际 %d 行", len(ys))
	}

	// box 尺寸对照 Edge（21x77）
	b := lb
	if g := rv.LayoutState().GeometryForBox(b.LayoutBox()); g != nil {
		t.Logf("title box: w=%.1f h=%.1f（Edge 参照 21x77）", g.BorderBoxWidth(), g.BorderBoxHeight())
		if g.BorderBoxWidth() < 15 || g.BorderBoxWidth() > 30 {
			t.Errorf("title 宽度 %.1f 偏离 Edge 参照（21px）", g.BorderBoxWidth())
		}
	}

	// 打印测量值（对照参考）
	w1 := layout.MeasureTextFunc("sans-serif", 12, 400, "normal", "完成摘要")
	t.Logf("measure 完成摘要=%.1f", w1)
	_ = fmt.Sprintf
	_ = strings.Join
}
