// Command aboutmodal_engine_probe 在 **wb-ui 引擎内**驱动真实 9090 页面（真实后端 +
// 真实产物），按真实用户路径打开「帮助 → 关于 PairCode IDE」弹窗，读回 DOM 状态并
// 输出引擎渲染截图。
//
// 存在动机：AboutModal 曾为绕开「引擎未实现 min()」而降级为 max-height:88vh。
// mathfunc_probe 已在最小用例上证明引擎支持 min()，本探针进一步在**真实页面 +
// 真实产物**上确认弹窗渲染无回归（高度受 min(680px,88vh) 约束、技术栈不被裁切、
// 按钮单行），并把引擎真实渲染结果落盘为 PNG 供视觉复核。
//
// 用法（wb-ui 仓库根，CGO 环境；需 9090 实例在跑）：
//
//	go run ./dev/probes/aboutmodal_engine_probe
//	go run ./dev/probes/aboutmodal_engine_probe -url http://127.0.0.1:9090/
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"
	"time"

	"wb-ui/engine/js/jsc"
	"wb-ui/webkit"
)

// jsClickMenuBtn 走真实用户路径第 1 步：点开「帮助」菜单按钮。
const jsClickMenuBtn = `(function () {
  var b = document.querySelector('.menu-btn');
  if (!b) return 'NO_MENU_BTN';
  b.click();
  return 'clicked:' + (b.textContent || '').trim();
})()`

// jsOpenAbout 走真实用户路径第 2 步：在下拉里点「关于 PairCode IDE」。
const jsOpenAbout = `(function () {
  var items = document.querySelectorAll('.menu-item');
  for (var i = 0; i < items.length; i++) {
    var t = (items[i].textContent || '');
    if (t.indexOf('关于') >= 0) { items[i].click(); return 'clicked:' + t.trim(); }
  }
  return 'NO_ABOUT_ITEM(' + items.length + ')';
})()`

// jsReadAbout 读弹窗状态：overlay 数、弹窗盒、body 是否溢出、技术栈是否被裁、按钮行数。
const jsReadAbout = `(function () {
  var overlays = document.querySelectorAll('.modal-overlay').length;
  var m = document.querySelector('.modal-content.about-modal');
  var out = { overlays: overlays, hasModal: !!m };
  if (!m) { return JSON.stringify(out); }
  var cs = getComputedStyle(m);
  var r = m.getBoundingClientRect();
  out.modal = { x: Math.round(r.x), y: Math.round(r.y),
                w: Math.round(r.width), h: Math.round(r.height) };
  out.maxHeight = cs ? cs.maxHeight : '(nil)';
  out.height = cs ? cs.height : '(nil)';
  out.cssText = (cs && cs.cssText) ? cs.cssText : '';
  var body = m.querySelector('.modal-body');
  if (body) {
    var bcs = getComputedStyle(body);
    out.body = { clientH: body.clientHeight, scrollH: body.scrollHeight,
                 overflowY: bcs ? bcs.overflowY : '(nil)',
                 minHeight: bcs ? bcs.minHeight : '(nil)' };
  }
  var ts = m.querySelector('.tech-stack');
  if (ts) {
    var tr = ts.getBoundingClientRect();
    var br = body ? body.getBoundingClientRect() : null;
    out.techStack = { top: Math.round(tr.top), bottom: Math.round(tr.bottom),
                      bodyBottom: br ? Math.round(br.bottom) : -1,
                      clipped: br ? (tr.bottom > br.bottom + 0.5) : null };
  }
  var btns = m.querySelectorAll('.modal-footer button');
  out.buttons = [];
  for (var i = 0; i < btns.length; i++) {
    var b = btns[i];
    var tops = [];
    var rects = 0;
    try {
      var rg = document.createRange(); rg.selectNodeContents(b);
      var list = rg.getClientRects(); rects = list.length;
      for (var k = 0; k < list.length; k++) {
        var t = Math.round(list[k].top);
        if (tops.indexOf(t) < 0) tops.push(t);
      }
    } catch (e) {}
    out.buttons.push({ text: (b.textContent || '').trim(),
                       offsetH: b.offsetHeight, offsetW: b.offsetWidth,
                       textRects: rects, topBands: tops.length });
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

func evalStr(wv *webkit.WebView, js string) (string, error) {
	v, err := wv.EvalJS(js)
	if err != nil {
		return "", err
	}
	return v.ToString(), nil
}

func main() {
	url := flag.String("url", "http://127.0.0.1:9090/", "目标页面（真实后端）")
	w := flag.Int("w", 1550, "视口宽")
	h := flag.Int("h", 651, "视口高")
	pngOut := flag.String("png", "screenshots/aboutmodal_engine.png", "渲染截图输出")
	flag.Parse()

	if err := os.MkdirAll("screenshots", 0o755); err != nil {
		fmt.Printf("[WARN] mkdir screenshots: %v\n", err)
	}

	wv := webkit.NewWebView()
	defer wv.Destroy()
	wv.Resize(*w, *h)
	wv.SetConsoleLogger(&jsc.BufferLogger{})

	fmt.Printf("[1] LoadURL %s\n", *url)
	if err := wv.LoadURL(*url); err != nil {
		fmt.Printf("[FAIL] LoadURL: %v\n", err)
		os.Exit(1)
	}
	// 等挂载 + 区域插件（ui-titlebar / ui-modals）装载
	deadline := time.Now().Add(40 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		pump(wv, 3)
		if s, err := evalStr(wv, `(function(){var a=document.getElementById('app');var b=document.querySelector('.menu-btn');return (a&&a.children.length>0&&b)?'READY':'WAIT';})()`); err == nil && strings.TrimSpace(s) == "READY" {
			ready = true
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
	fmt.Printf("[2] 页面就绪（#app 有内容 + .menu-btn 存在）: %v\n", ready)
	if !ready {
		if v, err := evalStr(wv, `(function(){var a=document.getElementById('app');var b=document.querySelector('.menu-btn');return JSON.stringify({app:!!a,appChildren:a?a.children.length:-1,menuBtn:!!b,bodyLen:document.body?document.body.innerHTML.length:-1});})()`); err == nil {
			fmt.Printf("     诊断: %s\n", v)
		}
	}

	// 步骤 3：真实用户路径 —— 点「帮助」菜单
	if s, err := evalStr(wv, jsClickMenuBtn); err == nil {
		fmt.Printf("[3] 点「帮助」菜单: %s\n", strings.TrimSpace(s))
	} else {
		fmt.Printf("[3] [WARN] %v\n", err)
	}
	pump(wv, 10)

	// 步骤 4：点「关于 PairCode IDE」
	if s, err := evalStr(wv, jsOpenAbout); err == nil {
		fmt.Printf("[4] 点「关于」菜单项: %s\n", strings.TrimSpace(s))
	} else {
		fmt.Printf("[4] [WARN] %v\n", err)
	}
	pump(wv, 25)

	// 步骤 5：读弹窗状态
	raw, err := evalStr(wv, jsReadAbout)
	if err != nil {
		fmt.Printf("[FAIL] 读弹窗状态: %v\n", err)
		os.Exit(1)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		fmt.Printf("[FAIL] JSON: %v（原始 %s）\n", err, raw)
		os.Exit(1)
	}
	pretty, _ := json.MarshalIndent(out, "", "  ")
	fmt.Printf("[5] 弹窗状态：\n%s\n", string(pretty))

	// 步骤 6：导出引擎渲染截图（Render 返回原始像素缓冲，需编码为 PNG）
	if pix, rerr := wv.Render(); rerr == nil && len(pix) > 0 {
		img := image.NewRGBA(image.Rect(0, 0, *w, *h))
		if len(pix) >= len(img.Pix) {
			copy(img.Pix, pix[:len(img.Pix)])
		}
		if f, cerr := os.Create(*pngOut); cerr == nil {
			_ = png.Encode(f, img)
			_ = f.Close()
			fmt.Printf("[6] 引擎渲染截图 → %s（%dx%d）\n", *pngOut, *w, *h)
		} else {
			fmt.Printf("[6] [WARN] 建文件：%v\n", cerr)
		}
	} else {
		fmt.Printf("[6] [WARN] Render：%v\n", rerr)
	}
}
