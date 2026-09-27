// Command nowrap_probe 验证引擎的「按钮文字误折行竖排」缺陷是否仍存在。
//
// 存在动机（技术债验收）：gou-ide 前端 AboutModal 的 `.btn-secondary` 上挂着
// `white-space: nowrap`，注释理由是「wb-ui 引擎中 content 恰等于文字宽时会误
// 折行竖排——AboutModal 关闭按钮变两行的根因」（引入于 6ab5ff36，2026-08-10）。
//
// 本探针**故意不加 nowrap**，并复刻 AboutModal 的 fixed overlay → flex 列 →
// modal-footer flex 行结构；除固定场景外，还对「关闭」文本做**精确宽度扫描**
// （floor/ceil/±0.1/±0.5/±1px），因为原缺陷的触发条件是 content 宽**恰等于**
// 文字宽——只在单一宽度上取样会漏判。
//
// 判定：任一宽度下 textTopBands > 1 → 缺陷真实存在（应修引擎 IFC 阈值，
// 并保留 nowrap 作为临时绕过）；全部为 1 → nowrap 属不必要绕过，应移除。
//
// 用法（wb-ui 仓库根，CGO 环境）：
//
//	go run ./dev/probes/nowrap_probe
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"

	"wb-ui/engine/js/jsc"
	"wb-ui/webkit"
)

// 复刻 AboutModal 结构；★ 按钮上**没有** white-space: nowrap。
const page = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><style>
  html, body { margin: 0; padding: 0; }
  #overlay { position: fixed; left: 0; top: 0; right: 0; bottom: 0;
             background: rgba(0,0,0,0.5); display: flex;
             align-items: center; justify-content: center; }
  #modal { background: #222; border-radius: 8px; width: 880px; height: 580px;
           display: flex; flex-direction: column; overflow: hidden; }
  #mbody { flex: 1; padding: 20px; overflow: hidden; }
  #mfoot { display: flex; align-items: center; justify-content: flex-end;
           gap: 8px; padding: 12px 20px; border-top: 1px solid #444; flex-shrink: 0; }
  button { font-size: 13px; font-weight: 500; padding: 7px 16px; border-radius: 4px;
           border: 1px solid #555; background: #333; color: #eee; cursor: pointer;
           display: flex; align-items: center; gap: 6px; }
  /* 扫描靶：纯 inline-block，宽度由脚本逐个设定（含小数） */
  #scan { display: inline-block; font-size: 13px; padding: 0; border: 0;
          background: #333; color: #eee; }
</style></head>
<body>
  <div id="overlay"><div id="modal">
    <div id="mbody">内容区</div>
    <div id="mfoot">
      <button id="p1">查看帮助文档</button>
      <button id="s1">关闭</button>
    </div>
  </div></div>
  <div id="scan">关闭</div>
</body></html>`

// jsMeasure 量「关闭」在 13px 下的精确文本宽（nowrap 的临时 span，量完即删）。
const jsMeasure = `(function () {
  var s = document.createElement('span');
  s.style.cssText = 'position:absolute;visibility:hidden;white-space:nowrap;font-size:13px;padding:0;border:0';
  s.textContent = '关闭';
  document.body.appendChild(s);
  var w = s.getBoundingClientRect().width;
  document.body.removeChild(s);
  return String(w);
})()`

// jsSetScanWidth 设定扫描靶宽度。
func jsSetScanWidth(w float64) string {
	return fmt.Sprintf(`(function(){var e=document.getElementById('scan');e.style.width='%gpx';return e.style.width;})()`, w)
}

// jsScanRow 读扫描靶的行盒信息。
const jsScanRow = `(function () {
  var e = document.getElementById('scan');
  if (!e) return 'null';
  var cs = getComputedStyle(e);
  var tops = [];
  var rects = 0;
  try {
    var r = document.createRange();
    r.selectNodeContents(e);
    var list = r.getClientRects();
    rects = list.length;
    for (var k = 0; k < list.length; k++) {
      var t = Math.round(list[k].top);
      if (tops.indexOf(t) < 0) tops.push(t);
    }
  } catch (err) {}
  return JSON.stringify({
    offsetW: e.offsetWidth, offsetH: e.offsetHeight,
    scrollW: e.scrollWidth, scrollH: e.scrollHeight,
    textRects: rects, textTopBands: tops.length,
    whiteSpace: cs ? cs.whiteSpace : '(nil)',
    lineHeight: cs ? cs.lineHeight : '(nil)'
  });
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

func evalStr(wv *webkit.WebView, js string) (string, error) {
	v, err := wv.EvalJS(js)
	if err != nil {
		return "", err
	}
	return v.ToString(), nil
}

type scanRow struct {
	OffsetW      int    `json:"offsetW"`
	OffsetH      int    `json:"offsetH"`
	ScrollW      int    `json:"scrollW"`
	ScrollH      int    `json:"scrollH"`
	TextRects    int    `json:"textRects"`
	TextTopBands int    `json:"textTopBands"`
	WhiteSpace   string `json:"whiteSpace"`
	LineHeight   string `json:"lineHeight"`
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

	bad := []string{}

	// ── 阶段 1：AboutModal 真实结构的固定场景 ──────────────────────────────
	fmt.Println("=== 阶段 1：AboutModal 结构（flex 行内按钮，无 nowrap）===")
	fixed := `(function () {
	  var out = { rows: [] };
	  var ids = ['p1', 's1'];
	  for (var i = 0; i < ids.length; i++) {
	    var e = document.getElementById(ids[i]);
	    var cs = getComputedStyle(e);
	    var tops = [], rects = 0;
	    try {
	      var r = document.createRange(); r.selectNodeContents(e);
	      var list = r.getClientRects(); rects = list.length;
	      for (var k = 0; k < list.length; k++) {
	        var t = Math.round(list[k].top);
	        if (tops.indexOf(t) < 0) tops.push(t);
	      }
	    } catch (err) {}
	    out.rows.push({ id: ids[i], offsetW: e.offsetWidth, offsetH: e.offsetHeight,
	      scrollW: e.scrollWidth, scrollH: e.scrollHeight,
	      textRects: rects, textTopBands: tops.length,
	      whiteSpace: cs ? cs.whiteSpace : '(nil)' });
	  }
	  return JSON.stringify(out);
	})()`
	if s, err := evalStr(wv, fixed); err == nil {
		var r struct {
			Rows []struct {
				ID           string `json:"id"`
				OffsetW      int    `json:"offsetW"`
				OffsetH      int    `json:"offsetH"`
				ScrollW      int    `json:"scrollW"`
				ScrollH      int    `json:"scrollH"`
				TextRects    int    `json:"textRects"`
				TextTopBands int    `json:"textTopBands"`
				WhiteSpace   string `json:"whiteSpace"`
			} `json:"rows"`
		}
		if json.Unmarshal([]byte(s), &r) == nil {
			fmt.Printf("%-5s %-9s %-9s %-9s %-9s %-10s %-9s %s\n",
				"id", "offsetW", "offsetH", "scrollW", "scrollH", "textRects", "topBands", "whiteSpace")
			for _, row := range r.Rows {
				fmt.Printf("%-5s %-9d %-9d %-9d %-9d %-10d %-9d %s\n",
					row.ID, row.OffsetW, row.OffsetH, row.ScrollW, row.ScrollH,
					row.TextRects, row.TextTopBands, row.WhiteSpace)
				if row.TextTopBands > 1 {
					bad = append(bad, row.ID)
				}
			}
		} else {
			fmt.Printf("  [WARN] 解析失败：%s\n", s)
		}
	} else {
		fmt.Printf("  [WARN] EvalJS: %v\n", err)
	}

	// ── 阶段 2：「恰等于文字宽」精确扫描 ───────────────────────────────────
	wraw, err := evalStr(wv, jsMeasure)
	if err != nil {
		fmt.Printf("[FAIL] 测文本宽：%v\n", err)
		os.Exit(1)
	}
	var tw float64
	if _, err := fmt.Sscanf(wraw, "%g", &tw); err != nil {
		fmt.Printf("[FAIL] 文本宽解析 %q：%v\n", wraw, err)
		os.Exit(1)
	}
	fmt.Printf("\n=== 阶段 2：「关闭」文本宽 = %.4fpx → 宽度扫描（判据：topBands>1 即折行）===\n", tw)

	widths := []float64{
		math.Floor(tw) - 2, math.Floor(tw) - 1, math.Floor(tw) - 0.5, math.Floor(tw) - 0.1,
		math.Floor(tw), math.Floor(tw) + 0.1, math.Floor(tw) + 0.5, math.Floor(tw) + 1,
		math.Ceil(tw), math.Ceil(tw) + 1, tw, tw - 0.01, tw + 0.01, tw + 1, tw + 5,
	}
	fmt.Printf("%-10s %-9s %-9s %-9s %-10s %-9s %s\n",
		"setW", "offsetW", "offsetH", "scrollW", "textRects", "topBands", "行高")
	for _, w := range widths {
		if w <= 0 {
			continue
		}
		if _, err := evalStr(wv, jsSetScanWidth(w)); err != nil {
			fmt.Printf("%-10.2f [WARN] 设宽失败：%v\n", w, err)
			continue
		}
		pump(wv, 5)
		s, err := evalStr(wv, jsScanRow)
		if err != nil {
			fmt.Printf("%-10.2f [WARN] 读取失败：%v\n", w, err)
			continue
		}
		var row scanRow
		if json.Unmarshal([]byte(s), &row) != nil {
			fmt.Printf("%-10.2f [WARN] 解析失败：%s\n", w, s)
			continue
		}
		mark := ""
		if row.TextTopBands > 1 {
			mark = "  ← 折行!"
			bad = append(bad, fmt.Sprintf("scan@%.2fpx", w))
		}
		fmt.Printf("%-10.2f %-9d %-9d %-9d %-10d %-9d %s%s\n",
			w, row.OffsetW, row.OffsetH, row.ScrollW, row.TextRects, row.TextTopBands, row.LineHeight, mark)
	}

	fmt.Println()
	if len(bad) == 0 {
		fmt.Println("[结果] 未复现折行（固定场景 + 15 档宽度扫描全部单行）")
		fmt.Println("       → nowrap 属不必要绕过：应移除，回归标准写法")
	} else {
		fmt.Printf("[结果] 折行复现于：%v\n", bad)
		fmt.Println("       → nowrap 仍有必要：应在引擎侧修 IFC 折行阈值，并保留绕过 + 登记")
	}
}
