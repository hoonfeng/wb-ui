// Command aboutmodal_engine_probe 在 **wb-ui 引擎内**驱动真实 9090 页面（真实后端 +
// 真实产物）做两类验证：
//
//	① 渲染验证：走「帮助菜单 → 关于 PairCode IDE」打开弹窗，读回高度约束
//	   （min(680px,88vh)）、技术栈是否被裁、底部按钮是否单行，并落盘引擎渲染截图。
//	② 交互验证（用户报告的两条 bug）：
//	     路径 A：点关于弹窗内「查看帮助文档」(.about-modal .btn-primary)
//	             → 期望：关于关闭、帮助在最前
//	               （overlays==1、.help-modal 存在、无 .about-modal、
//	                elementFromPoint(原关于中心) 落在帮助之上）
//	     路径 B：点关于弹窗内「×」(.about-modal .modal-close) 与底部「关闭」(.btn-secondary)
//	             → 期望：overlays==0
//
// ★ 交互验证一律用**引擎真实鼠标事件**（HandleMouseMove + HandleMouseButton，用法见
// webkit/interact_test.go 与 app/menu_hover_stay_open_test.go），而不是 DOM
// element.click()：前者经过引擎自己的命中测试，能暴露「元素在 DOM 里但引擎不派发
// 事件」这类缺陷，后者会绕过它。若真实鼠标点击后状态未变，探针会再用 DOM click()
// 补点一次作为**对照**，用来区分「引擎不派发事件」（命中测试问题）与「事件已派发但
// 页面状态/节点卸载未生效」（Vue diff 或引擎节点移除问题）。
//
// 每条路径都从干净状态独立出发：重载页面 → 点「帮助」菜单 → 点「关于」菜单项。
// 点击前后各打印一次状态（overlays 数 / about、help 存在性 / overlay DOM 顺序与
// z-index / elementFromPoint 命中归属）并落盘截图，作为 before/after 证据。
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

// jsOpenHelp 在下拉里点开帮助文档项。注意菜单里没有「帮助文档」这一项：帮助菜单
// 下是「常见问题 / 快速开始 / 文档中心 / 功能介绍 / API 文档 / 工具文档 / 快捷键参考」
// 七条文档入口（任一点开都会打开帮助弹窗），最后一条才是「关于 PairCode IDE」。
const jsOpenHelp = `(function () {
  var items = document.querySelectorAll('.menu-item');
  var want = ['快速开始', '文档中心', '常见问题', '功能介绍', 'API 文档', '工具文档', '快捷键参考'];
  for (var w = 0; w < want.length; w++) {
    for (var i = 0; i < items.length; i++) {
      var t = (items[i].textContent || '');
      if (t.indexOf(want[w]) >= 0) { items[i].click(); return 'clicked:' + t.trim(); }
    }
  }
  return 'NO_HELP_ITEM(' + items.length + ')';
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

// jsStateAll 读**交互路径**关心的全量状态：overlay 数、about/help 存在性、overlay
// 在 DOM 中的堆叠顺序（含 z-index，用于判断谁在上层）、elementFromPoint 命中归属、
// 关于内三个按钮的可用矩形。
const jsStateAll = `(function () {
  function cls(el) { return (el && el.getAttribute) ? (el.getAttribute('class') || '') : ''; }
  function inTree(el, sel) { return !!(el && el.closest && el.closest(sel)); }
  var ovs = document.querySelectorAll('.modal-overlay');
  var about = document.querySelector('.modal-content.about-modal');
  var help = document.querySelector('.modal-content.help-modal');
  var out = { overlays: ovs.length, hasAbout: !!about, hasHelp: !!help, stack: [] };
  for (var i = 0; i < ovs.length; i++) {
    var ov = ovs[i];
    var c = ov.querySelector('.modal-content');
    var cs = getComputedStyle(ov);
    out.stack.push({
      idx: i,
      content: c ? cls(c) : '(no .modal-content)',
      zIndex: cs ? cs.zIndex : '(nil)',
      display: cs ? cs.display : '(nil)'
    });
  }
  function probeAt(label, el) {
    if (!el) return null;
    var r = el.getBoundingClientRect();
    var x = Math.round(r.x + r.width / 2), y = Math.round(r.y + r.height / 2);
    var hit = null;
    try { hit = document.elementFromPoint(x, y); } catch (e) {}
    var hc = (hit && hit.closest) ? hit.closest('.modal-content') : null;
    return {
      label: label, point: [x, y],
      hit: hit ? (hit.tagName + '#' + (hit.id || '') + '.' + cls(hit)) : null,
      hitContent: hc ? cls(hc) : null,
      inAbout: inTree(hit, '.about-modal'),
      inHelp: inTree(hit, '.help-modal')
    };
  }
  out.hitAtAbout = probeAt('about-center', about);
  out.hitAtHelp = probeAt('help-center', help);
  // 样式健康度：判断该弹窗的 scoped 样式是否真的应用。未应用会让元素退化为
  // 文档流堆叠（弹窗不居中、logo 尺寸失控、按钮落到文档流底部），此时测到的
  // 「按钮位置异常」是环境问题而非引擎布局缺陷，必须先排除。
  out.style = {};
  (function () {
    var ov = document.querySelector('.modal-overlay');
    if (ov) {
      var c = getComputedStyle(ov);
      out.style.overlay = { position: c.position, display: c.display,
                            justifyContent: c.justifyContent, alignItems: c.alignItems };
    }
    var mc = document.querySelector('.modal-content.about-modal');
    if (mc) {
      var c2 = getComputedStyle(mc);
      out.style.content = { width: c2.width, display: c2.display,
                            flexDirection: c2.flexDirection, maxHeight: c2.maxHeight };
    }
    var ft = document.querySelector('.about-modal .modal-footer');
    if (ft) {
      var c3 = getComputedStyle(ft);
      out.style.footer = { display: c3.display, height: c3.height,
                           justifyContent: c3.justifyContent };
    }
    var img = document.querySelector('.about-logo-img');
    if (img) {
      var c4 = getComputedStyle(img);
      out.style.logoImg = { width: c4.width, height: c4.height };
    }
    out.style.sheetCount = document.styleSheets ? document.styleSheets.length : -1;
    out.style.linkCount = document.querySelectorAll('link[rel=stylesheet]').length;
  })();
  var map = {
    primary: '.about-modal .btn-primary',
    closeX: '.about-modal .modal-close',
    secondary: '.about-modal .btn-secondary'
  };
  out.aboutButtons = {};
  for (var k in map) {
    var b = document.querySelector(map[k]);
    if (!b) { out.aboutButtons[k] = null; continue; }
    var br = b.getBoundingClientRect();
    out.aboutButtons[k] = {
      text: (b.textContent || '').trim().slice(0, 16),
      rect: { x: Math.round(br.x), y: Math.round(br.y),
              w: Math.round(br.width), h: Math.round(br.height) }
    };
  }
  return JSON.stringify(out);
})()`

// jsElCenterFmt 取选择器命中元素的中心点（客户区 CSS 像素）；不存在或不可见返回空串。
const jsElCenterFmt = `(function () {
  var e = document.querySelector(%s);
  if (!e) return '';
  var r = e.getBoundingClientRect();
  if (r.width <= 0 || r.height <= 0) return '';
  return JSON.stringify({ x: r.x + r.width / 2, y: r.y + r.height / 2,
                          w: r.width, h: r.height });
})()`

// jsDOMClickFmt 对照用：DOM 层直接 element.click()（不经过引擎命中测试）。
const jsDOMClickFmt = `(function () {
  var e = document.querySelector(%s);
  if (!e) return 'NO_EL';
  e.click();
  return 'clicked:' + ((e.textContent || '').trim().slice(0, 16));
})()`

// jsHitScanFmt 在视口内按步长网格采样 document.elementFromPoint，找出目标元素
// （选择器命中）**在视口内实际可被命中的点集**，并与 getBoundingClientRect()、
// offsetLeft/Top/Width/Height 三个几何来源并列输出。用于回答：按钮在视口内到底
// 能不能被点中、可命中矩形在哪。
const jsHitScanFmt = `(function () {
  var sel = %s;
  var step = %d;
  var t = document.querySelector(sel);
  if (!t) return JSON.stringify({ sel: sel, exists: false });
  var vw = window.innerWidth || 0, vh = window.innerHeight || 0;
  var n = 0, minX = 1e9, minY = 1e9, maxX = -1e9, maxY = -1e9;
  for (var y = 0; y < vh; y += step) {
    for (var x = 0; x < vw; x += step) {
      var e = null;
      try { e = document.elementFromPoint(x, y); } catch (err) { e = null; }
      if (!e) continue;
      var isTarget = (e === t) || (t.contains && t.contains(e)) || (e.contains && e.contains(t));
      if (isTarget) {
        n++;
        if (x < minX) minX = x;
        if (y < minY) minY = y;
        if (x > maxX) maxX = x;
        if (y > maxY) maxY = y;
      }
    }
  }
  var r = t.getBoundingClientRect();
  return JSON.stringify({
    sel: sel, exists: true, hitCount: n,
    hitBox: n ? [minX, minY, maxX, maxY] : null,
    rect: [Math.round(r.x), Math.round(r.y), Math.round(r.width), Math.round(r.height)],
    offsetBox: [t.offsetLeft, t.offsetTop, t.offsetWidth, t.offsetHeight],
    vw: vw, vh: vh
  });
})()`

// ------------------------------------------------------------------ 引擎驱动

// pump 推进引擎若干帧：跑事件循环任务、微任务、布局并渲染。
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

// waitReady 等 SPA 挂载完成（#app 有内容且 .menu-btn 出现）。
func waitReady(wv *webkit.WebView, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pump(wv, 3)
		s, err := evalStr(wv, `(function(){var a=document.getElementById('app');var b=document.querySelector('.menu-btn');return (a&&a.children.length>0&&b)?'READY':'WAIT';})()`)
		if err == nil && strings.TrimSpace(s) == "READY" {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	if v, err := evalStr(wv, `(function(){var a=document.getElementById('app');var b=document.querySelector('.menu-btn');return JSON.stringify({app:!!a,appChildren:a?a.children.length:-1,menuBtn:!!b,bodyLen:document.body?document.body.innerHTML.length:-1});})()`); err == nil {
		fmt.Printf("      [诊断] %s\n", v)
	}
	return false
}

// savePNG 落盘引擎渲染结果（Render 返回原始像素缓冲 → PNG）。
func savePNG(wv *webkit.WebView, path string, w, h int) bool {
	pix, err := wv.Render()
	if err != nil || len(pix) == 0 {
		fmt.Printf("      [WARN] Render: %v\n", err)
		return false
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if len(pix) >= len(img.Pix) {
		copy(img.Pix, pix[:len(img.Pix)])
	}
	f, cerr := os.Create(path)
	if cerr != nil {
		fmt.Printf("      [WARN] 建文件 %s: %v\n", path, cerr)
		return false
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		fmt.Printf("      [WARN] 编码 PNG: %v\n", err)
		return false
	}
	fmt.Printf("      截图 → %s（%dx%d）\n", path, w, h)
	return true
}

// readState 读交互状态并打印（返回解析后的 map）。
func readState(wv *webkit.WebView, label string) (map[string]any, bool) {
	raw, err := evalStr(wv, jsStateAll)
	if err != nil {
		fmt.Printf("      [%s] 读状态失败: %v\n", label, err)
		return nil, false
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &m); err != nil {
		fmt.Printf("      [%s] JSON 解析失败: %v（原始 %s）\n", label, err, raw)
		return nil, false
	}
	pretty, _ := json.MarshalIndent(m, "", "  ")
	fmt.Printf("      [%s] 状态：\n%s\n", label, indentLines(string(pretty), "        "))
	return m, true
}

func indentLines(s, pad string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = pad + lines[i]
	}
	return strings.Join(lines, "\n")
}

// elCenter 取选择器命中元素中心的客户区坐标。
func elCenter(wv *webkit.WebView, sel string) (x, y, w, h float64, ok bool) {
	raw, err := evalStr(wv, fmt.Sprintf(jsElCenterFmt, fmt.Sprintf("%q", sel)))
	if err != nil {
		return 0, 0, 0, 0, false
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0, 0, 0, false
	}
	var p struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		W float64 `json:"w"`
		H float64 `json:"h"`
	}
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return 0, 0, 0, 0, false
	}
	return p.X, p.Y, p.W, p.H, true
}

// clickRealMouse 用**引擎真实鼠标事件**点击选择器命中元素的中心：
// mouseMove（设置 hover 与命中位置）→ mouseDown → mouseUp（触发 click）。
// 这条路径经过引擎自己的命中测试，等价于用户真实点击。
func clickRealMouse(wv *webkit.WebView, sel string) (string, bool) {
	x, y, w, h, ok := elCenter(wv, sel)
	if !ok {
		return fmt.Sprintf("元素不存在或尺寸为 0: %s", sel), false
	}
	wv.EnsureLayout()
	wv.EnsureHitTestReady()
	wv.HandleMouseMove(x, y)
	pump(wv, 3)
	wv.HandleMouseButton(x, y, 0, 0) // 左键按下
	pump(wv, 3)
	wv.HandleMouseButton(x, y, 0, 1) // 左键释放 → click
	pump(wv, 15)
	return fmt.Sprintf("真实鼠标点击 (%d,%d) 元素盒 %dx%d [%s]", int(x), int(y), int(w), int(h), sel), true
}

// clickDOM 对照用：DOM 层 element.click()（绕过引擎命中测试）。
func clickDOM(wv *webkit.WebView, sel string) (string, bool) {
	s, err := evalStr(wv, fmt.Sprintf(jsDOMClickFmt, fmt.Sprintf("%q", sel)))
	if err != nil {
		return err.Error(), false
	}
	pump(wv, 15)
	s = strings.TrimSpace(s)
	return s, s != "NO_EL"
}

// openAboutNow 在当前已就绪页面上走菜单路径打开关于弹窗（点「帮助」→「关于」）。
func openAboutNow(wv *webkit.WebView) {
	if s, err := evalStr(wv, jsClickMenuBtn); err == nil {
		fmt.Printf("      点「帮助」菜单: %s\n", strings.TrimSpace(s))
	} else {
		fmt.Printf("      [WARN] 点「帮助」菜单: %v\n", err)
	}
	pump(wv, 10)
	if s, err := evalStr(wv, jsOpenAbout); err == nil {
		fmt.Printf("      点「关于」菜单项: %s\n", strings.TrimSpace(s))
	} else {
		fmt.Printf("      [WARN] 点「关于」菜单项: %v\n", err)
	}
	pump(wv, 25)
}

// openAboutViaMenu 从干净状态打开关于：重载页面 → 等就绪 → 菜单路径打开关于。
// 每条用户路径都独立重载，避免上一条路径的残留状态干扰。
func openAboutViaMenu(wv *webkit.WebView, url string) bool {
	if err := wv.LoadURL(url); err != nil {
		fmt.Printf("      [FAIL] LoadURL: %v\n", err)
		return false
	}
	if !waitReady(wv, 60*time.Second) {
		fmt.Printf("      [FAIL] 页面未就绪\n")
		return false
	}
	openAboutNow(wv)
	return true
}

// singleCase 为 true 时（-case 指定单条路径）每条路径都用「首次加载」状态运行：
// 页面只加载一次、不重载 —— 与用户真实场景一致，避免重载本身引入的差异
// （重载后样式表/布局可能未就绪，会让元素退化为文档流堆叠，造成假复现）。
var singleCase bool

// openAboutForPath 为一条用户路径准备起点。all 模式重载页面后再走菜单打开关于；
// 单用例模式复用当前已就绪的**首次加载**页面，只走菜单打开关于。
func openAboutForPath(wv *webkit.WebView, url string) bool {
	if singleCase {
		openAboutNow(wv)
		return true
	}
	return openAboutViaMenu(wv, url)
}

// openHelpForPath 打开帮助弹窗作为起点（单用例模式复用首次加载页面，不重载）。
func openHelpForPath(wv *webkit.WebView, url string) bool {
	if !singleCase {
		if err := wv.LoadURL(url); err != nil {
			fmt.Printf("      [FAIL] LoadURL: %v\n", err)
			return false
		}
		if !waitReady(wv, 60*time.Second) {
			fmt.Printf("      [FAIL] 页面未就绪\n")
			return false
		}
	}
	if s, err := evalStr(wv, jsClickMenuBtn); err == nil {
		fmt.Printf("      点「帮助」菜单: %s\n", strings.TrimSpace(s))
	} else {
		fmt.Printf("      [WARN] 点「帮助」菜单: %v\n", err)
	}
	pump(wv, 10)
	if s, err := evalStr(wv, jsOpenHelp); err == nil {
		fmt.Printf("      点「帮助文档」菜单项: %s\n", strings.TrimSpace(s))
	} else {
		fmt.Printf("      [WARN] 点帮助项: %v\n", err)
	}
	pump(wv, 25)
	return true
}

// ------------------------------------------------------------------ 断言辅助

func f64At(m map[string]any, key string) float64 {
	if m == nil {
		return -1
	}
	if v, ok := m[key].(float64); ok {
		return v
	}
	return -1
}

func boolAt(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func subBool(m map[string]any, key, sub string) bool {
	if m == nil {
		return false
	}
	if o, ok := m[key].(map[string]any); ok {
		if v, ok := o[sub].(bool); ok {
			return v
		}
	}
	return false
}

func subStr(m map[string]any, key, sub string) string {
	if m == nil {
		return ""
	}
	if o, ok := m[key].(map[string]any); ok {
		if v, ok := o[sub].(string); ok {
			return v
		}
	}
	return ""
}

func check(cond bool, format string, args ...any) bool {
	mark := "FAIL"
	if cond {
		mark = "PASS"
	}
	fmt.Printf("      [%s] %s\n", mark, fmt.Sprintf(format, args...))
	return cond
}

// sameStack 判两次状态是否「弹窗堆叠未变化」。
func sameStack(a, b map[string]any) bool {
	return f64At(a, "overlays") == f64At(b, "overlays") &&
		boolAt(a, "hasAbout") == boolAt(b, "hasAbout") &&
		boolAt(a, "hasHelp") == boolAt(b, "hasHelp")
}

// toF 把 JSON 解出的数值转 float64（非数值返回 0）。
func toF(v any) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

// hitScan 在视口内网格采样 elementFromPoint，返回目标元素的可命中点集与几何对照。
func hitScan(wv *webkit.WebView, sel string, step int) map[string]any {
	raw, err := evalStr(wv, fmt.Sprintf(jsHitScanFmt, fmt.Sprintf("%q", sel), step))
	if err != nil {
		fmt.Printf("      [WARN] hitScan %s: %v\n", sel, err)
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &m); err != nil {
		fmt.Printf("      [WARN] hitScan JSON %s: %v（原始 %s）\n", sel, err, raw)
		return nil
	}
	pretty, _ := json.MarshalIndent(m, "", "  ")
	fmt.Printf("      [hitScan] %s\n%s\n", sel, indentLines(string(pretty), "        "))
	return m
}

// diagnoseHitGeometry 诊断「关于弹窗内按钮在视口内的可命中区域」：
//
//	① 对三个按钮做 elementFromPoint 网格扫描 → 各自可命中包围盒，并与
//	   getBoundingClientRect()/offset* 并列对照（几何对不对、能不能点中、点在哪）；
//	② 取 .btn-secondary 可命中包围盒中心做**真实鼠标点击** → 看是否真能关闭弹窗
//	   （鼠标事件命中测试与 elementFromPoint 是否同一套几何）。
func diagnoseHitGeometry(wv *webkit.WebView, url string, w, h int) bool {
	fmt.Printf("\n########## ③ 命中几何诊断（按钮在视口内能否被命中、命中的是哪套几何）##########\n")
	if !openAboutForPath(wv, url) {
		fmt.Printf("      [FAIL] 诊断起点：打开关于失败\n")
		return false
	}
	hitScan(wv, ".about-modal .modal-close", 6)
	hitScan(wv, ".about-modal .btn-primary", 6)
	sec := hitScan(wv, ".about-modal .btn-secondary", 6)
	savePNG(wv, "screenshots/aboutmodal_diag_before.png", w, h)
	if sec == nil {
		return false
	}
	hb, _ := sec["hitBox"].([]any)
	if len(hb) != 4 {
		fmt.Printf("      [结论] .btn-secondary 在整个视口内**不可被 elementFromPoint 命中**（hitCount=%v）→ 该按钮在视口内无任何可命中点\n", sec["hitCount"])
		return false
	}
	x0, y0 := toF(hb[0]), toF(hb[1])
	x1, y1 := toF(hb[2]), toF(hb[3])
	cx, cy := (x0+x1)/2, (y0+y1)/2
	fmt.Printf("      [实验] 用可命中盒中心 (%.0f,%.0f) 做真实鼠标点击（盒 [%.0f,%.0f,%.0f,%.0f]）\n",
		cx, cy, x0, y0, x1, y1)
	wv.EnsureLayout()
	wv.EnsureHitTestReady()
	wv.HandleMouseMove(cx, cy)
	pump(wv, 3)
	wv.HandleMouseButton(cx, cy, 0, 0)
	pump(wv, 3)
	wv.HandleMouseButton(cx, cy, 0, 1)
	pump(wv, 15)
	after, ok := readState(wv, "diag/after-click-at-hitbox")
	if !ok {
		return false
	}
	savePNG(wv, "screenshots/aboutmodal_diag_after.png", w, h)
	return check(f64At(after, "overlays") == 0,
		"真实鼠标点在 elementFromPoint 认定的按钮位置 → 弹窗关闭（实得 overlays=%v；若否：鼠标事件命中测试与 elementFromPoint 不是同一套几何）",
		f64At(after, "overlays"))
}

// ------------------------------------------------------------------ 用户路径

// pathA 路径 A：关于弹窗内点「查看帮助文档」。
func pathA(wv *webkit.WebView, url string, w, h int) bool {
	fmt.Printf("\n===== 路径 A：关于内点「查看帮助文档」(.about-modal .btn-primary) =====\n")
	if !openAboutForPath(wv, url) {
		return check(false, "路径 A 起点：打开关于弹窗")
	}
	before, ok := readState(wv, "A/before")
	if !ok {
		return false
	}
	savePNG(wv, "screenshots/aboutmodal_A_before.png", w, h)
	if !check(f64At(before, "overlays") == 1 && boolAt(before, "hasAbout"),
		"起点：overlays==1 且 .about-modal 存在（实得 overlays=%v hasAbout=%v hasHelp=%v）",
		f64At(before, "overlays"), boolAt(before, "hasAbout"), boolAt(before, "hasHelp")) {
		return false
	}

	desc, clicked := clickRealMouse(wv, ".about-modal .btn-primary")
	fmt.Printf("      点击：%s\n", desc)
	if !check(clicked, "「查看帮助文档」按钮可被真实鼠标命中并点击") {
		return false
	}

	after, ok := readState(wv, "A/after")
	if !ok {
		return false
	}
	savePNG(wv, "screenshots/aboutmodal_A_after.png", w, h)

	pass := true
	if !check(f64At(after, "overlays") == 1, "点击后 .modal-overlay 数 == 1（实得 %v）", f64At(after, "overlays")) {
		pass = false
	}
	if !check(boolAt(after, "hasHelp"), "点击后 .help-modal 存在（帮助已打开）") {
		pass = false
	}
	if !check(!boolAt(after, "hasAbout"), "点击后 .about-modal 已消失（关于已关闭）") {
		pass = false
	}
	// 谁在最前：取「原关于弹窗中心」坐标做命中测试，应落在帮助之上。
	hitAbout := subStr(after, "hitAtAbout", "hitContent")
	if !check(!subBool(after, "hitAtAbout", "inAbout"),
		"elementFromPoint(原关于中心) 未命中 about 子树（实得 hitContent=%q inAbout=%v inHelp=%v）",
		hitAbout, subBool(after, "hitAtAbout", "inAbout"), subBool(after, "hitAtAbout", "inHelp")) {
		pass = false
	}
	if !check(subBool(after, "hitAtHelp", "inHelp"),
		"elementFromPoint(帮助中心) 命中 help 子树（帮助在最前）") {
		pass = false
	}

	// 对照：真实鼠标点击后若状态未变，用 DOM click() 补点，用于区分
	// 「引擎不派发事件」（命中测试问题）与「事件已派发但状态/卸载未生效」。
	if sameStack(before, after) {
		fmt.Printf("      [NOTE] 真实鼠标点击后状态未变 → 追加 DOM click() 对照\n")
		if d, ok2 := clickDOM(wv, ".about-modal .btn-primary"); ok2 {
			fmt.Printf("      [NOTE] DOM click(): %s\n", d)
			if after2, ok3 := readState(wv, "A/after-DOMclick"); ok3 {
				savePNG(wv, "screenshots/aboutmodal_A_after_domclick.png", w, h)
				if !check(!sameStack(after, after2),
					"DOM click() 改变了状态（若否：事件处理链路本身不生效，与命中测试无关）") {
					pass = false
				}
			}
		}
	}
	return pass
}

// pathA2 「帮助 → 关于 → 查看帮助文档」：先经帮助弹窗进入关于（组件挂载顺序与
// 从菜单直接打开关于不同），再点「查看帮助文档」。用于验证用户报告的
// 「帮助弹在关于后方且关于不关闭」是否只在特定进入路径下出现。
func pathA2(wv *webkit.WebView, url string, w, h int) bool {
	fmt.Printf("\n===== 路径 A2：帮助 → 关于 → 查看帮助文档（用户报告的进入路径）=====\n")
	if !openHelpForPath(wv, url) {
		return check(false, "路径 A2 起点：打开帮助弹窗")
	}
	s1, ok := readState(wv, "A2/afterOpenHelp")
	if !ok {
		return false
	}
	savePNG(wv, "screenshots/aboutmodal_A2_help_open.png", w, h)
	if !check(f64At(s1, "overlays") == 1 && boolAt(s1, "hasHelp"),
		"起点：overlays==1 且 .help-modal 存在（实得 overlays=%v hasHelp=%v）",
		f64At(s1, "overlays"), boolAt(s1, "hasHelp")) {
		return false
	}

	desc, clicked := clickRealMouse(wv, ".help-modal .btn-about")
	fmt.Printf("      点击：%s\n", desc)
	if !check(clicked, "帮助弹窗内「关于」按钮可被真实鼠标命中并点击") {
		return false
	}
	s2, ok := readState(wv, "A2/afterOpenAboutFromHelp")
	if !ok {
		return false
	}
	savePNG(wv, "screenshots/aboutmodal_A2_about_from_help.png", w, h)
	pass := true
	if !check(f64At(s2, "overlays") == 1, "从帮助打开关于后 overlays==1（实得 %v；2 = 两弹窗叠加）", f64At(s2, "overlays")) {
		pass = false
	}
	if !check(boolAt(s2, "hasAbout"), "从帮助打开关于后 .about-modal 存在") {
		pass = false
	}
	if !check(!boolAt(s2, "hasHelp"), "从帮助打开关于后 .help-modal 已消失") {
		pass = false
	}

	desc2, clicked2 := clickRealMouse(wv, ".about-modal .btn-primary")
	fmt.Printf("      点击：%s\n", desc2)
	if !check(clicked2, "关于弹窗内「查看帮助文档」按钮可被真实鼠标命中并点击") {
		return false
	}
	s3, ok := readState(wv, "A2/afterViewHelpDoc")
	if !ok {
		return false
	}
	savePNG(wv, "screenshots/aboutmodal_A2_view_help_after.png", w, h)
	if !check(f64At(s3, "overlays") == 1, "点「查看帮助文档」后 overlays==1（实得 %v；2 = 帮助与关于叠加）", f64At(s3, "overlays")) {
		pass = false
	}
	if !check(boolAt(s3, "hasHelp"), "点「查看帮助文档」后 .help-modal 存在") {
		pass = false
	}
	if !check(!boolAt(s3, "hasAbout"), "点「查看帮助文档」后 .about-modal 已关闭") {
		pass = false
	}
	if !check(subBool(s3, "hitAtHelp", "inHelp"), "elementFromPoint(帮助中心) 命中 help（帮助在最前，未被关于压住）") {
		pass = false
	}
	return pass
}

// pathX 叠加复现路径：帮助弹窗打开后，从**菜单栏**再打开关于。
//
// MenuBar.vue 的 action 分发只单向开窗、不做互斥（`help-*` 只置 showHelp、
// `about` 只置 showAbout），而 UiModals.vue 里的 onAboutOpenHelp / onHelpOpenAbout
// 才是互斥的。于是菜单入口会留下两个同时打开的 overlay；又因模板中 HelpModal
// 排在 AboutModal 之前、两者 z-index 同为 2000，DOM 靠后的关于压在上层 ——
// 视觉表现正是用户报告的「帮助弹在关于后方、关于在前面挡着」。
//
// 返回值：true = 复现成功（overlays==2 且关于压住帮助）。
func pathX(wv *webkit.WebView, url string, w, h int) bool {
	fmt.Printf("\n===== 路径 X：帮助打开后从菜单栏打开关于（叠加复现路径）=====\n")
	if !openHelpForPath(wv, url) {
		return check(false, "路径 X 起点：打开帮助弹窗")
	}
	s1, ok := readState(wv, "X/afterOpenHelp")
	if !ok {
		return false
	}
	savePNG(wv, "screenshots/aboutmodal_X_help_open.png", w, h)
	if !check(f64At(s1, "overlays") == 1 && boolAt(s1, "hasHelp"),
		"起点：overlays==1 且 .help-modal 存在（实得 overlays=%v hasHelp=%v）",
		f64At(s1, "overlays"), boolAt(s1, "hasHelp")) {
		return false
	}

	// 真实用户操作：帮助弹窗开着时，从菜单栏再点「关于 PairCode IDE」。
	openAboutNow(wv)
	s2, ok := readState(wv, "X/afterOpenAboutFromMenu")
	if !ok {
		return false
	}
	savePNG(wv, "screenshots/aboutmodal_X_overlap.png", w, h)

	repro := true
	if !check(f64At(s2, "overlays") == 2 && boolAt(s2, "hasAbout") && boolAt(s2, "hasHelp"),
		"★ 复现判定：帮助已打开时从菜单打开关于 → 两个 overlay 同时存在（实得 overlays=%v hasAbout=%v hasHelp=%v）",
		f64At(s2, "overlays"), boolAt(s2, "hasAbout"), boolAt(s2, "hasHelp")) {
		repro = false
	}
	if !check(subBool(s2, "hitAtHelp", "inAbout"),
		"★ 复现判定：elementFromPoint(帮助中心) 命中 about 子树 → 关于压在帮助上方（帮助在后方）") {
		repro = false
	}

	// 再点关于内「关闭」：关于确实被卸载，但下方帮助立刻显形 → 用户感知为「关于关不掉」。
	desc, clicked := clickRealMouse(wv, ".about-modal .modal-close")
	fmt.Printf("      点击：%s\n", desc)
	if clicked {
		if s3, ok3 := readState(wv, "X/afterCloseAbout"); ok3 {
			savePNG(wv, "screenshots/aboutmodal_X_after_close_about.png", w, h)
			check(f64At(s3, "overlays") == 1 && boolAt(s3, "hasHelp"),
				"点关于「关闭」后只剩帮助仍在（overlays=%v hasHelp=%v）→ 用户容易感知为「关于没关掉」",
				f64At(s3, "overlays"), boolAt(s3, "hasHelp"))
		}
	}
	return repro
}

// pathB 路径 B：关于弹窗内点关闭（× 或底部「关闭」按钮）。
func pathB(wv *webkit.WebView, url, sel, slug, name string, w, h int) bool {
	fmt.Printf("\n===== 路径 B：关于内点「%s」(%s) =====\n", name, sel)
	if !openAboutForPath(wv, url) {
		return check(false, "路径 B[%s] 起点：打开关于弹窗", name)
	}
	before, ok := readState(wv, "B/"+slug+"/before")
	if !ok {
		return false
	}
	savePNG(wv, "screenshots/aboutmodal_B_"+slug+"_before.png", w, h)
	if !check(f64At(before, "overlays") == 1 && boolAt(before, "hasAbout"),
		"起点：overlays==1 且 .about-modal 存在（实得 overlays=%v hasAbout=%v）",
		f64At(before, "overlays"), boolAt(before, "hasAbout")) {
		return false
	}

	desc, clicked := clickRealMouse(wv, sel)
	fmt.Printf("      点击：%s\n", desc)
	if !check(clicked, "「%s」按钮可被真实鼠标命中并点击", name) {
		return false
	}

	after, ok := readState(wv, "B/"+slug+"/after")
	if !ok {
		return false
	}
	savePNG(wv, "screenshots/aboutmodal_B_"+slug+"_after.png", w, h)

	pass := true
	if !check(f64At(after, "overlays") == 0, "点击后 .modal-overlay 数 == 0（实得 %v）", f64At(after, "overlays")) {
		pass = false
	}
	if !check(!boolAt(after, "hasAbout"), "点击后 .about-modal 已从 DOM 移除") {
		pass = false
	}

	if sameStack(before, after) {
		fmt.Printf("      [NOTE] 真实鼠标点击后状态未变 → 追加 DOM click() 对照\n")
		if d, ok2 := clickDOM(wv, sel); ok2 {
			fmt.Printf("      [NOTE] DOM click(): %s\n", d)
			if after2, ok3 := readState(wv, "B/"+slug+"/after-DOMclick"); ok3 {
				savePNG(wv, "screenshots/aboutmodal_B_"+slug+"_after_domclick.png", w, h)
				if !check(!sameStack(after, after2),
					"DOM click() 改变了状态（若否：事件处理链路本身不生效，与命中测试无关）") {
					pass = false
				}
			}
		}
	}
	return pass
}

// ------------------------------------------------------------------ main

func main() {
	url := flag.String("url", "http://127.0.0.1:9090/", "目标页面（真实后端）")
	w := flag.Int("w", 1550, "视口宽")
	h := flag.Int("h", 651, "视口高")
	pngOut := flag.String("png", "screenshots/aboutmodal_engine.png", "渲染验证截图输出")
	caseName := flag.String("case", "all", "只跑指定用例：all | A | B1 | B2 | diag（非 all 时页面只加载一次、不重载 = 首次加载状态）")
	flag.Parse()
	singleCase = *caseName != "all"
	switch *caseName {
	case "all", "A", "A2", "X", "B1", "B2", "diag":
	default:
		fmt.Printf("[FAIL] 未知 -case %q（可用：all | A | A2 | X | B1 | B2 | diag）\n", *caseName)
		os.Exit(1)
	}

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
	ready := waitReady(wv, 60*time.Second)
	fmt.Printf("[2] 页面就绪（#app 有内容 + .menu-btn 存在）: %v\n", ready)
	if !ready {
		os.Exit(1)
	}

	// ---------------- ① 渲染验证（帮助菜单 → 关于） ----------------
	if !singleCase {
		fmt.Printf("\n########## ① 渲染验证（帮助菜单 → 关于 PairCode IDE）##########\n")
		openAboutNow(wv)
		if raw, err := evalStr(wv, jsReadAbout); err == nil {
			var out map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &out); err != nil {
				fmt.Printf("      [WARN] JSON: %v（原始 %s）\n", err, raw)
			} else {
				pretty, _ := json.MarshalIndent(out, "", "  ")
				fmt.Printf("      弹窗状态：\n%s\n", indentLines(string(pretty), "        "))
			}
		} else {
			fmt.Printf("      [WARN] 读弹窗状态: %v\n", err)
		}
		savePNG(wv, *pngOut, *w, *h)
	}

	// ---------------- ② 交互验证（用户报告的两条路径） ----------------
	fmt.Printf("\n########## ② 交互验证（用户报告的两条路径）##########\n")
	type pathRes struct {
		name string
		ok   bool
	}
	var results []pathRes
	if *caseName == "all" || *caseName == "A" {
		results = append(results, pathRes{"A 关于 → 查看帮助文档（期望 overlays==1 且只剩 .help-modal、帮助在最前）", pathA(wv, *url, *w, *h)})
	}
	if *caseName == "all" || *caseName == "A2" {
		results = append(results, pathRes{"A2 帮助 → 关于 → 查看帮助文档（期望 overlays==1、只剩 .help-modal）", pathA2(wv, *url, *w, *h)})
	}
	reproMark := ""
	if *caseName == "all" || *caseName == "X" {
		m := "FAIL"
		if pathX(wv, *url, *w, *h) {
			m = "PASS"
		}
		reproMark = m
	}
	if *caseName == "all" || *caseName == "B1" {
		results = append(results, pathRes{"B1 关于 → × (.modal-close)（期望 overlays==0）", pathB(wv, *url, ".about-modal .modal-close", "close_x", "关闭 ×", *w, *h)})
	}
	if *caseName == "all" || *caseName == "B2" {
		results = append(results, pathRes{"B2 关于 → 关闭 (.btn-secondary)（期望 overlays==0）", pathB(wv, *url, ".about-modal .btn-secondary", "close_btn", "关闭", *w, *h)})
	}

	// ---------------- ③ 命中几何诊断（按钮在视口内能否被命中） ----------------
	diagOK := true
	if *caseName == "all" || *caseName == "diag" {
		diagOK = diagnoseHitGeometry(wv, *url, *w, *h)
	}

	fmt.Printf("\n########## 汇总 ##########\n")
	allPass := true
	for _, r := range results {
		mark := "PASS"
		if !r.ok {
			mark = "FAIL"
			allPass = false
		}
		fmt.Printf("  [%s] %s\n", mark, r.name)
	}
	diagMark := "FAIL"
	if diagOK {
		diagMark = "PASS"
	}
	fmt.Printf("  [%s] ③ 命中几何诊断（可命中盒 vs getBoundingClientRect + 按可命中盒真实点击）\n", diagMark)
	if reproMark != "" {
		fmt.Printf("  [%s] ★ 路径 X 叠加复现（[PASS]=复现了「菜单入口不互斥 → 双 overlay 叠加」，[FAIL]=未复现）\n", reproMark)
	}
	if allPass {
		fmt.Printf("\n结论：路径 A/B 全部通过（交互无缺陷）。\n")
	} else {
		fmt.Printf("\n结论：存在失败路径 → 复现用户报告的缺陷，需在引擎侧定位根因。\n")
		os.Exit(2)
	}
}
