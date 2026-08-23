//go:build ignore

// 对照组：创建→加载→渲染后**不调用 Destroy**，60 轮看泄漏幅度。
package main

import (
	"fmt"
	"runtime"
	"time"

	"wb-ui/webkit"
)

func heapT() uint64 {
	runtime.GC()
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

func roundNoDestroy() {
	wv := webkit.NewWebView()
	wv.Resize(320, 120)
	wv.LoadHTML(`<!doctype html><html><body><div id="x" style="color:#0f8;font-size:20px">hello</div><script>
var n=0; document.getElementById('x').addEventListener('click',function(){n++;});
window.addEventListener('resize',function(){n++;});
setInterval(function(){n++;},50);
for(var i=0;i<60;i++){var d=document.createElement('div');d.textContent='item '+i;document.body.appendChild(d);}
</script></body></html>`)
	wv.Render()
	// no Destroy — 泄漏路径
}

func main() {
	roundNoDestroy()
	base := heapT()
	for i := 0; i < 30; i++ {
		roundNoDestroy()
	}
	after := heapT()
	fmt.Printf("no-destroy: base=%d KiB after30=%d KiB delta=%d KiB (per-round=%d B)\n",
		base/1024, after/1024, (int64(after)-int64(base))/1024, (int64(after)-int64(base))/30)
}
