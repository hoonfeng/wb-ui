// wb-ui 内存探针：循环 LoadHTML / Render 测量堆增长，定位泄漏或频繁分配。
// 用法：go run ./dev/probes/mem_probe
package main

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"wb-ui/webkit"
)

func heap() (allocMB float64, objects int64) {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.HeapAlloc) / (1 << 20), int64(m.HeapObjects)
}

const bigHTML = `<!DOCTYPE html><html><head><style>
	.card { border: 1px solid #ccc; padding: 8px; margin: 4px; font-size: 14px; }
	.row { display: flex; gap: 6px; }
	.badge { background: #e33; color: #fff; border-radius: 3px; padding: 1px 6px; }
</style></head><body>
<script>
	var items = [];
	for (var i = 0; i < 500; i++) {
		var d = document.createElement('div');
		d.className = 'card';
		d.innerHTML = '<div class="row"><span class="badge">#' + i + '</span><b>item ' + i + '</b></div>';
		document.body.appendChild(d);
		items.push(d);
	}
	setTimeout(function(){ window.__done = true; }, 10);
</script></body></html>`

func main() {
	wv := webkit.NewWebView()
	if err := wv.LoadHTML(bigHTML); err != nil {
		fmt.Println("load err:", err)
		os.Exit(1)
	}
	// 预热（jsc 全局、字体、样式表缓存等一次性初始化）。
	for i := 0; i < 3; i++ {
		_ = wv.LoadHTML(bigHTML)
	}
	base, baseObj := heap()
	fmt.Printf("baseline after warmup: HeapAlloc=%.1fMB objects=%d\n", base, baseObj)

	// 循环加载（模拟浏览器反复导航）：DOM/渲染树/JS 全部重建，旧对象应被 GC。
	for i := 0; i < 60; i++ {
		_ = wv.LoadHTML(bigHTML)
		if (i+1)%10 == 0 {
			m, o := heap()
			fmt.Printf("load iter=%2d HeapAlloc=%.1fMB objects=%d delta=%.1fMB\n", i+1, m, o, m-base)
		}
	}
	m1, o1 := heap()
	fmt.Printf("AFTER 60x LoadHTML: HeapAlloc=%.1fMB objects=%d delta=%.1fMB\n", m1, o1, m1-base)

	// 渲染分配：Render() 每次创建新 canvas 并光栅化。
	rbase, _ := heap()
	start := time.Now()
	for i := 0; i < 200; i++ {
		if _, err := wv.Render(); err != nil {
			fmt.Println("render err:", err)
			break
		}
	}
	m2, o2 := heap()
	fmt.Printf("AFTER 200x Render: HeapAlloc=%.1fMB objects=%d (render+%.1fMB, %.0fms total)\n",
		m2, o2, m2-rbase, float64(time.Since(start).Milliseconds()))
}
