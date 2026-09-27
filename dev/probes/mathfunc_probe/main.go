// Command mathfunc_probe 验证引擎对「裸 CSS 数学函数」作为**长度属性值**的支持：
// max-height 分别写 vh / min() / clamp() / calc()，读回真实布局高度与计算样式。
//
// 存在动机（技术债验收）：gou-ide 前端 AboutModal 曾把
// `max-height: min(680px, 88vh)` 降级成单一值 `88vh`，理由据称是「引擎未实现
// 裸 min()，声明被整体丢弃」。若本探针显示裸 min() 已生效，则该降级属
// **替代性绕过**，应立即还原为标准写法（见 docs/TECH_DEBT.md）。
//
// 用法（wb-ui 仓库根，CGO 环境）：
//
//	go run ./dev/probes/mathfunc_probe
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"wb-ui/engine/js/jsc"
	"wb-ui/webkit"
)

const page = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><style>
  html, body { margin: 0; padding: 0; }
  .box { height: 4000px; background: #333; }
  #vh    { max-height: 88vh; }
  #mn    { max-height: min(680px, 88vh); }
  #mx    { max-height: max(100px, 20vh); }
  #cl    { max-height: clamp(100px, 50vh, 400px); }
  #ca    { max-height: calc(100vh - 100px); }
</style></head>
<body>
  <div class="box" id="vh"></div>
  <div class="box" id="mn"></div>
  <div class="box" id="mx"></div>
  <div class="box" id="cl"></div>
  <div class="box" id="ca"></div>
</body></html>`

const jsRead = `(function () {
  var ids = ['vh', 'mn', 'mx', 'cl', 'ca'];
  var out = { viewport: { w: window.innerWidth, h: window.innerHeight }, rows: [] };
  for (var i = 0; i < ids.length; i++) {
    var e = document.getElementById(ids[i]);
    if (!e) { out.rows.push({ id: ids[i], missing: true }); continue; }
    var cs = getComputedStyle(e);
    out.rows.push({
      id: ids[i],
      offsetHeight: e.offsetHeight,
      computedMaxHeight: cs ? cs.maxHeight : '(nil)',
    });
  }
  return JSON.stringify(out);
})()`

func pump(wv *webkit.WebView, n int) {
	for i := 0; i < n; i++ {
		if el := wv.JSInterpreter().GetEventLoop(); el != nil {
			el.ProcessTasks(0)
		}
		wv.JSInterpreter().RunJobs()
		wv.EnsureLayout()
		_, _ = wv.Render()
	}
}

func main() {
	wv := webkit.NewWebView()
	defer wv.Destroy()
	wv.Resize(1440, 900)
	wv.SetConsoleLogger(&jsc.BufferLogger{})

	if err := wv.LoadHTML(page); err != nil {
		fmt.Printf("[FAIL] LoadHTML: %v\n", err)
		os.Exit(1)
	}
	pump(wv, 20)

	v, err := wv.EvalJS(jsRead)
	if err != nil {
		fmt.Printf("[FAIL] EvalJS: %v\n", err)
		os.Exit(1)
	}
	var res struct {
		Viewport struct{ W, H int } `json:"viewport"`
		Rows     []struct {
			ID               string `json:"id"`
			Missing          bool   `json:"missing"`
			OffsetHeight     int    `json:"offsetHeight"`
			ComputedMaxH     string `json:"computedMaxHeight"`
		} `json:"rows"`
	}
	if err := json.Unmarshal([]byte(v.ToString()), &res); err != nil {
		fmt.Printf("[FAIL] JSON: %v（原始 %s）\n", err, v.ToString())
		os.Exit(1)
	}

	fmt.Printf("viewport = %dx%d  (88vh=%.0fpx, 50vh=%.0fpx, 20vh=%.0fpx)\n",
		res.Viewport.W, res.Viewport.H,
		float64(res.Viewport.H)*0.88, float64(res.Viewport.H)*0.50, float64(res.Viewport.H)*0.20)
	fmt.Printf("%-6s %-14s %-16s %s\n", "id", "offsetHeight", "computedMaxH", "期望")
	expect := map[string]string{
		"vh": "792 (88vh)",
		"mn": "680 (min(680px,88vh))",
		"mx": "180 (max(100px,20vh))",
		"cl": "400 (clamp(100px,50vh,400px))",
		"ca": "800 (calc(100vh - 100px))",
	}
	for _, r := range res.Rows {
		if r.Missing {
			fmt.Printf("%-6s MISSING\n", r.ID)
			continue
		}
		fmt.Printf("%-6s %-14d %-16s %s\n", r.ID, r.OffsetHeight, r.ComputedMaxH, expect[r.ID])
	}
}
