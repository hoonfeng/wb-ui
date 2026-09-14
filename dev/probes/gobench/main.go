// Command gobench: 剖析 goja 编译大 bundle 的耗时，支持 GC_PERCENT 环境
// 变量调优 GC 频率（启动编译期小对象扫描是主要开销）。
package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"wb-ui/engine/js/goja"
)

func main() {
	path := ""
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	if path == "" {
		fmt.Fprintln(os.Stderr, "usage: gobench <bundle.js>")
		fmt.Fprintln(os.Stderr, "  env: GC_PERCENT=<n>  GOMAXPROCS=<n>")
		os.Exit(2)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("read err:", err)
		return
	}
	src := string(data)
	fmt.Printf("bundle: %d bytes\n", len(data))

	if v := os.Getenv("GC_PERCENT"); v != "" {
		var pct int
		fmt.Sscanf(v, "%d", &pct)
		old := debug.SetGCPercent(pct)
		fmt.Printf("GCPercent: %d (was %d)\n", pct, old)
	}
	if v := os.Getenv("GOMAXPROCS"); v != "" {
		var n int
		fmt.Sscanf(v, "%d", &n)
		runtime.GOMAXPROCS(n)
		fmt.Printf("GOMAXPROCS: %d\n", n)
	}

	// warmup
	goja.New().RunString("1+1")

	var times []time.Duration
	for i := 0; i < 3; i++ {
		t0 := time.Now()
		_, err = goja.Compile("bundle.js", src, false)
		if err != nil {
			fmt.Println("compile err:", err)
			return
		}
		times = append(times, time.Since(t0))
	}
	for i, t := range times {
		fmt.Printf("compile#%d: %v\n", i, t)
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("HeapAlloc: %d MB, NumGC: %d\n", m.HeapAlloc>>20, m.NumGC)
}
