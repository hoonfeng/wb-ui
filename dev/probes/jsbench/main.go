// dev/probes/jsbench —— 用 goja 跑一份纯计算 JS 基准，与 node(V8) 对照。
//
// 用法（仓库根目录）：
//
//	go run ./dev/probes/jsbench dev/output/jsbench.js
//
// 目的：量化「goja（纯 Go 解释器，无 JIT）vs V8/JSC（JIT）」的执行差距，
// 为「JS 性能差能否靠换引擎解决」提供数字。
package main

import (
	"fmt"
	"os"
	"time"

	"wb-ui/engine/js/goja"
)

func main() {
	path := "dev/output/jsbench.js"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}

	vm := goja.New()
	vm.Set("print", func(s string) { fmt.Println(s) })

	t0 := time.Now()
	prog, err := goja.Compile(path, string(src), false)
	if err != nil {
		fmt.Fprintln(os.Stderr, "compile:", err)
		os.Exit(1)
	}
	compileMS := float64(time.Since(t0).Microseconds()) / 1000

	t1 := time.Now()
	if _, err := vm.RunProgram(prog); err != nil {
		fmt.Fprintln(os.Stderr, "run:", err)
		os.Exit(1)
	}
	runMS := float64(time.Since(t1).Microseconds()) / 1000

	fmt.Printf("[goja] compile=%.1fms run=%.1fms total=%.1fms\n", compileMS, runMS, compileMS+runMS)
}
