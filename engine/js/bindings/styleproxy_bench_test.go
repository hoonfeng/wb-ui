package bindings

import (
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/js/jsc"
)

// C-P4 跨界基准（style 路径）：`el.style` 的句柄创建与逐属性读写是 JS↔Go
// 边界上最热的路径之一（Vue patchStyle、CM6 的 gutter spacer 都靠它）。
// 优化前后用同一组基准对比（见 docs/implementation-path.md §C-P4）。

func newBenchRuntime(b *testing.B) (*jsc.Interpreter, *dom.Document) {
	b.Helper()
	rt := jsc.NewInterpreter()
	rt.SetupGlobal(&jsc.BufferLogger{})
	doc := dom.NewDocument()
	RegisterDOMBindings(rt, doc)
	el := doc.CreateElement("div")
	el.SetId("bench")
	doc.AppendChild(el)
	return rt, doc
}

func runJS(b *testing.B, rt *jsc.Interpreter, src string) {
	b.Helper()
	if _, err := rt.Run(src); err != nil {
		b.Fatalf("Run 失败: %v", err)
	}
}

// BenchmarkStyleHandleAccess 测「取 el.style 句柄」的成本（T5 句柄缓存）。
// 每次迭代取 100 次句柄，不做其它操作。
func BenchmarkStyleHandleAccess(b *testing.B) {
	rt, _ := newBenchRuntime(b)
	runJS(b, rt, `
	  var el = document.getElementById("bench");
	  el.setAttribute("style", "color: rgb(1,2,3); width: 10px");
	  function run(n) { for (var i = 0; i < n; i++) { var s = el.style; if (s === null) throw new Error("null"); } }
	`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runJS(b, rt, "run(100)")
	}
}

// BenchmarkStyleSetPropertyDotted 测逐属性写（`el.style.width = "10px"`）：
// 每次写都要解析 style 属性文本 → 改一条 → 序列化写回 + 失效缓存（T3）。
func BenchmarkStyleSetPropertyDotted(b *testing.B) {
	rt, _ := newBenchRuntime(b)
	runJS(b, rt, `
	  var el = document.getElementById("bench");
	  el.setAttribute("style", "color: rgb(1,2,3); width: 10px; height: 20px");
	  function run(n) {
	    for (var i = 0; i < n; i++) {
	      el.style.width = "10px";
	      el.style.height = "20px";
	      el.style.color = "rgb(1,2,3)";
	    }
	  }
	`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runJS(b, rt, "run(100)")
	}
}

// BenchmarkStyleSetPropertyNoop 测「写入同值」：值未变化时理论上不需要
// 改属性文本、更不需要失效样式（T3 的无变更快速路径）。
func BenchmarkStyleSetPropertyNoop(b *testing.B) {
	rt, _ := newBenchRuntime(b)
	runJS(b, rt, `
	  var el = document.getElementById("bench");
	  el.setAttribute("style", "color: rgb(1,2,3); width: 10px; height: 20px");
	  function run(n) { for (var i = 0; i < n; i++) { el.style.width = "10px"; } }
	`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runJS(b, rt, "run(200)")
	}
}

// BenchmarkStyleGetPropertyValue 测批量读（`getPropertyValue`）：每次读都
// 重新解析 style 属性文本（T3 解析缓存）。
func BenchmarkStyleGetPropertyValue(b *testing.B) {
	rt, _ := newBenchRuntime(b)
	runJS(b, rt, `
	  var el = document.getElementById("bench");
	  el.setAttribute("style", "color: rgb(1,2,3); width: 10px; height: 20px; padding: 4px 6px");
	  function run(n) {
	    for (var i = 0; i < n; i++) {
	      if (el.style.getPropertyValue("width") !== "10px") throw new Error("bad width");
	      if (el.style.getPropertyValue("color") === "") throw new Error("bad color");
	    }
	  }
	`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runJS(b, rt, "run(200)")
	}
}

// BenchmarkStyleCssTextRead 测 cssText 读（序列化路径）。
func BenchmarkStyleCssTextRead(b *testing.B) {
	rt, _ := newBenchRuntime(b)
	runJS(b, rt, `
	  var el = document.getElementById("bench");
	  el.setAttribute("style", "color: rgb(1,2,3); width: 10px; height: 20px");
	  function run(n) { for (var i = 0; i < n; i++) { var s = el.style.cssText; if (s === "") throw new Error("empty"); } }
	`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runJS(b, rt, "run(200)")
	}
}

// BenchmarkStyleSetPropertyMethod 测 `setProperty()` 方法形态（CSSOM 路径）。
func BenchmarkStyleSetPropertyMethod(b *testing.B) {
	rt, _ := newBenchRuntime(b)
	runJS(b, rt, `
	  var el = document.getElementById("bench");
	  el.setAttribute("style", "color: rgb(1,2,3)");
	  function run(n) { for (var i = 0; i < n; i++) { el.style.setProperty("width", "10px"); } }
	`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runJS(b, rt, "run(200)")
	}
}
