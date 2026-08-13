package webkit

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// cm6LikeHTML 生成一个模拟 CM6 编辑器的 DOM：cm-content 下 N 行，每行 M 个
// 语法高亮 span（token）。用于量化「打字触发的全量 RebuildRenderTree」耗时。
func cm6LikeHTML(lines, spansPerLine int) string {
	var b strings.Builder
	b.WriteString(`<html><head><style>.cm-editor{font-family:monospace;font-size:14px}.cm-line{white-space:pre}.cm-tag{color:#a00}.cm-var{color:#00a}</style></head><body><div class="cm-editor"><div class="cm-scroller"><div class="cm-content">`)
	for i := 0; i < lines; i++ {
		b.WriteString(`<div class="cm-line">`)
		for j := 0; j < spansPerLine; j++ {
			switch j % 3 {
			case 0:
				b.WriteString(`<span class="cm-tag">const</span>`)
			case 1:
				b.WriteString(`<span class="cm-var">foo</span>`)
			default:
				b.WriteString(`<span>=</span>`)
			}
		}
		b.WriteString("</div>")
	}
	b.WriteString(`</div></div></div></body></html>`)
	return b.String()
}

// BenchmarkRebuildRenderTree 量化「打字触发全量 RebuildRenderTree」的耗时。
// 100 行 × 10 span ≈ 1000+ 渲染节点，接近真实 CM6 编辑器的规模。
func BenchmarkRebuildRenderTree(b *testing.B) {
	wv := NewWebView()
	if err := wv.LoadHTML(cm6LikeHTML(100, 10)); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wv.RebuildRenderTree()
	}
}

// BenchmarkRebuildRenderTree_Small 小 DOM（10 行 × 5 span）对照。
func BenchmarkRebuildRenderTree_Small(b *testing.B) {
	wv := NewWebView()
	if err := wv.LoadHTML(cm6LikeHTML(10, 5)); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wv.RebuildRenderTree()
	}
}

// TestRebuildRenderTree_Scale 打印不同规模下 RebuildRenderTree 的耗时，
// 观察是「线性」还是「超线性」（超线性说明 linkLayoutBoxes 的 O(n²) 是热点）。
func TestRebuildRenderTree_Scale(t *testing.T) {
	for _, size := range []struct{ lines, spans int }{
		{10, 5}, {50, 5}, {100, 10}, {200, 10}, {400, 10},
	} {
		wv := NewWebView()
		src := cm6LikeHTML(size.lines, size.spans)
		if err := wv.LoadHTML(src); err != nil {
			t.Fatal(err)
		}
		// 预热一次
		wv.RebuildRenderTree()
		const n = 5
		var total time.Duration
		for i := 0; i < n; i++ {
			start := time.Now()
			wv.RebuildRenderTree()
			total += time.Since(start)
		}
		avg := total / n
		fmt.Printf("rebuild scale: lines=%d spans=%d nodes≈%d avg=%v\n",
			size.lines, size.spans, size.lines*size.spans+size.lines, avg)
	}
}
