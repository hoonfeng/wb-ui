//go:build ignore

// WebView Destroy 内存探针：创建→LoadHTML（JS 定时器/事件监听/DOM 构建）
// →Render→Destroy，多轮后对比 GC 内存——验证 Destroy 后 DOM 树/渲染树/
// JS 解释器整棵树可回收（挂件重建泄漏修复的回归验证）。
package main

import (
	"fmt"
	"runtime"
	"time"

	"wb-ui/webkit"
)

func heap() uint64 {
	runtime.GC()
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

const page = `<!doctype html><html><head><style>body{background:#111;font-family:sans-serif}</style></head><body>
<div id="x" style="color:#0f8;font-size:20px">hello</div>
<script>
  var n = 0;
  document.getElementById('x').addEventListener('click', function(){ n++; });
  window.addEventListener('resize', function(){ n++; });
  setInterval(function(){ n++; document.getElementById('x').textContent = 't' + n; }, 50);
  for (var i = 0; i < 60; i++) { var d = document.createElement('div'); d.textContent = 'item ' + i; document.body.appendChild(d); }
</script></body></html>`

func round() {
	wv := webkit.NewWebView()
	wv.Resize(320, 120)
	if err := wv.LoadHTML(page); err != nil {
		panic(err)
	}
	if _, err := wv.Render(); err != nil {
		panic(err)
	}
	wv.Destroy()
}

func main() {
	round() // 预热（字体/一次性质资源）
	base := heap()

	for i := 0; i < 10; i++ {
		round()
	}
	after10 := heap()

	for i := 0; i < 50; i++ {
		round()
	}
	after60 := heap()

	fmt.Printf("base=%d KiB  after10=%d KiB  delta10=%d KiB  after60=%d KiB  delta60=%d KiB  (per-round=%d B)",
		base/1024, after10/1024, int64(after10)-int64(base), after60/1024, int64(after60)-int64(base), (int64(after60)-int64(base))/60)
}
