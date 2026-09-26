// Command jsboundary 测 goja 的 JS↔Go 边界成本，用于与 V8 绑定（v8go）对照。
//
// 与 _temp/v8spike 的基准项一一对应：
//   JS→Go 空回调 / JS→Go 读参+返回数 / Go→JS 调用 / Go 侧 obj.Get、obj.Set /
//   纯 JS 属性读写 / 纯 JS 函数调用（后两项为引擎内对照）
//
// 用法：go run ./dev/probes/jsboundary [N]
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"wb-ui/engine/js/goja"
)

func main() {
	n := 1000000
	if len(os.Args) > 1 {
		if v, err := strconv.Atoi(os.Args[1]); err == nil {
			n = v
		}
	}

	vm := goja.New()
	vm.Set("goNoop", func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})
	vm.Set("goInc", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(call.Argument(0).ToInteger() + 1)
	})

	report := func(name string, d time.Duration) {
		fmt.Printf("%-34s %9.2f ms  %9.1f ns/op\n",
			name, float64(d.Nanoseconds())/1e6, float64(d.Nanoseconds())/float64(n))
	}

	// 1) JS → Go 空回调
	src := fmt.Sprintf("(function(){for(var i=0;i<%d;i++)goNoop(i);return 0;})()", n)
	t0 := time.Now()
	if _, err := vm.RunString(src); err != nil {
		fmt.Println("js1:", err)
		return
	}
	report("goja JS→Go 空回调", time.Since(t0))

	// 2) JS → Go 读参+返回数
	src = fmt.Sprintf("(function(){var s=0;for(var i=0;i<%d;i++)s=goInc(s);return s;})()", n)
	t0 = time.Now()
	if _, err := vm.RunString(src); err != nil {
		fmt.Println("js2:", err)
		return
	}
	report("goja JS→Go 读参+返回数", time.Since(t0))

	// 3) Go → JS 调用（每次新建参数值）
	val, err := vm.RunString("(function(a){return a+1;})")
	if err != nil {
		fmt.Println("def:", err)
		return
	}
	jf, ok := goja.AssertFunction(val)
	if !ok {
		fmt.Println("assertfunction failed")
		return
	}
	t0 = time.Now()
	for i := 0; i < n; i++ {
		if _, err := jf(goja.Undefined(), vm.ToValue(i)); err != nil {
			fmt.Println("call:", err)
			break
		}
	}
	report("goja Go→JS 调用(每次新值)", time.Since(t0))

	// 4) Go 侧对象属性访问
	obj := vm.NewObject()
	obj.Set("x", 1)
	t0 = time.Now()
	for i := 0; i < n; i++ {
		obj.Get("x")
	}
	report("goja Go 侧 obj.Get", time.Since(t0))
	t0 = time.Now()
	for i := 0; i < n; i++ {
		obj.Set("x", i)
	}
	report("goja Go 侧 obj.Set", time.Since(t0))

	// 5) 纯 JS 对照
	src = fmt.Sprintf("(function(){var o={x:1};var s=0;for(var i=0;i<%d;i++){o.x=i;s+=o.x;}return s;})()", n)
	t0 = time.Now()
	if _, err := vm.RunString(src); err != nil {
		fmt.Println("js5:", err)
		return
	}
	report("goja 纯 JS 属性读写(对照)", time.Since(t0))

	src = fmt.Sprintf("(function(){function f(a){return a+1;}var s=0;for(var i=0;i<%d;i++)s=f(s);return s;})()", n)
	t0 = time.Now()
	if _, err := vm.RunString(src); err != nil {
		fmt.Println("js6:", err)
		return
	}
	report("goja 纯 JS 函数调用(对照)", time.Since(t0))
}
