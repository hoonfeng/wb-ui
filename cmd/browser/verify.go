// wb-ui Browser —— 无头能力验证集。
//
// 逐项加载独立页面（每例一个 WebView），验证浏览器核心能力：
// HTML 结构 / CSS 布局与样式 / JS DOM / 事件 / fetch 两层桥 / 定时器 /
// 表单 / SVG / console / 滚动几何。页面内 JS 自我断言，结果写入
// window.__result；Go 侧汇总输出 PASS/FAIL 报告（含每例渲染耗时）。
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"wb-ui/bridge"
	"wb-ui/page"
	"wb-ui/webkit"
)

type verifyCase struct {
	name  string
	html  string
	setup func(wv *webkit.WebView) // 可选：注册 bridge 路由等
	wait  time.Duration            // 可选：加载后等待（定时器用例）
	check func(wv *webkit.WebView) (bool, string) // 可选：Go 侧额外验证（iframe 子 Frame 等 JS 不可见状态）
}

// runVerify 无头运行全部能力用例并输出报告。
func runVerify() {
	// 全局 echo 路由：验证两层桥（fetch → wb-ui bridge → Go handler）。
	bridge.RegisterHTTP("POST", "/_browser/echo", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(map[string]any{"echo": string(b), "ok": true})
	})

	passed, failed := 0, 0
	fmt.Println("=== wb-ui Browser capability verification ===")
	for _, c := range verifyCases {
		status, detail := runVerifyCase(c)
		if status == "PASS" {
			passed++
		} else {
			failed++
		}
		fmt.Printf("[%s] %-18s %s\n", status, c.name, detail)
	}
	fmt.Printf("=== %d passed, %d failed ===\n", passed, failed)
}

func runVerifyCase(c verifyCase) (string, string) {
	wv := webkit.NewWebView()
	if c.setup != nil {
		c.setup(wv)
	}
	start := time.Now()
	if err := wv.LoadHTML(c.html); err != nil {
		return "FAIL", "load: " + err.Error()
	}
	// 驱动事件循环：setTimeout / Promise.then（fetch 两层桥）需要
	// ProcessTasks + RunJobs 推进。
	if c.wait > 0 {
		time.Sleep(c.wait)
	}
	if el := wv.JSInterpreter().GetEventLoop(); el != nil {
		el.ProcessTasks(0)
	}
	wv.JSInterpreter().RunJobs()
	wv.EnsureLayout()

	// 渲染管线验证：任何页面都必须能产出像素（不崩）。
	pix, rerr := wv.Render()
	renderOK := rerr == nil && len(pix) > 0
	renderNote := ""
	if !renderOK {
		renderNote = fmt.Sprintf(" render=FAIL(%v)", rerr)
	}

	res, err := wv.EvalJS("window.__result || 'NO-RESULT'")
	jsResult := ""
	if err == nil {
		jsResult = res.ToString()
	}

	// Go 侧额外验证（iframe 子 Frame 等 JS 不可见状态）。
	if c.check != nil {
		if ok, note := c.check(wv); !ok {
			return "FAIL", "go-check: " + note + " | js: " + jsResult
		}
	}

	elapsed := time.Since(start).Round(time.Millisecond).String()

	status, detail := "FAIL", jsResult
	if strings.HasPrefix(jsResult, "PASS") && renderOK {
		status = "PASS"
	} else if strings.HasPrefix(jsResult, "PASS") {
		status = "FAIL"
	}
	if status == "PASS" {
		detail = fmt.Sprintf("(%s, render ok)", elapsed)
	} else {
		detail = fmt.Sprintf("%s%s (%s)", jsResult, renderNote, elapsed)
	}
	return status, detail
}

var verifyCases = []verifyCase{
	{
		name: "html-structure",
		html: `<!DOCTYPE html><html><head><title>struct</title></head><body>
			<h1 id="t">标题</h1><ul id="list"><li>a</li><li>b</li><li>c</li></ul>
			<table id="tab"><tr><td>1</td></tr><tr><td>2</td></tr></table>
			<a id="lnk" href="page2.html">link</a><p id="p">hello</p>
			<script>
				var ok = document.getElementById('t').textContent === '标题';
				ok = ok && document.querySelectorAll('#list li').length === 3;
				ok = ok && document.querySelectorAll('#tab tr').length === 2;
				ok = ok && document.getElementById('lnk').getAttribute('href') === 'page2.html';
				ok = ok && document.body.textContent.indexOf('hello') >= 0;
				window.__result = ok ? 'PASS' : 'FAIL: html structure mismatch';
			</script></body></html>`,
	},
	{
		name: "css-layout",
		html: `<!DOCTYPE html><html><head><style>
			#flex { display:flex; width:300px; height:40px; gap:0; }
			#flex .it { flex:1; }
			#abs { position:absolute; left:10px; top:20px; width:50px; height:30px; }
		</style></head><body>
			<div id="flex"><div class="it"></div><div class="it"></div><div class="it"></div></div>
			<div id="abs"></div>
			<script>
				var ok = true;
				var flex = document.getElementById('flex');
				var its = flex.children;
				ok = ok && flex.offsetWidth === 300 && flex.offsetHeight === 40;
				// flex:1 三等分：每个子项 ≈100px
				ok = ok && its.length === 3 && Math.abs(its[0].offsetWidth - 100) <= 3;
				ok = ok && Math.abs(its[2].offsetWidth - 100) <= 3;
				var abs = document.getElementById('abs');
				var r = abs.getBoundingClientRect();
				ok = ok && Math.abs(r.left - 10) <= 1 && Math.abs(r.top - 20) <= 1 && r.width === 50 && r.height === 30;
				window.__result = ok ? 'PASS' : 'FAIL: layout mismatch flex=' + its[0].offsetWidth + ' abs=' + JSON.stringify({l:r.left,t:r.top,w:r.width,h:r.height});
			</script></body></html>`,
	},
	{
		name: "css-style",
		html: `<!DOCTYPE html><html><head><style>
			#box { color: #ff0000; background: rgb(0,128,0); font-size: 20px; font-weight: bold; }
		</style></head><body><div id="box">text</div>
			<script>
				try {
				var s = getComputedStyle(document.getElementById('box'));
				var ok = (typeof s.color === 'string') && (s.color === '#ff0000' || s.color.indexOf('255') >= 0);
				ok = ok && (s.fontSize === '20px');
				ok = ok && (s.fontWeight === 'bold' || s.fontWeight === '700');
				// 颜色序列化与浏览器一致：rgb(0, 128, 0)（逗号后一空格，无多余空格）
				var bg = s.backgroundColor || '';
				ok = ok && bg === 'rgb(0, 128, 0)';
				window.__result = ok ? 'PASS' : 'FAIL: color=' + s.color + ' fs=' + s.fontSize + ' fw=' + s.fontWeight + ' bg=[' + bg + ']';
				} catch(e) { window.__result = 'FAIL: ' + (e && e.message || e); }
			</script>
		</script></body></html>`,
	},
	{
		name: "js-dom",
		html: `<!DOCTYPE html><html><body><div id="root"></div>
			<script>
				var root = document.getElementById('root');
				var el = document.createElement('span');
				el.textContent = 'x';
				el.className = 'c1';
				el.setAttribute('data-k', 'v');
				root.appendChild(el);
				var ok = root.children.length === 1 && el.textContent === 'x';
				ok = ok && el.getAttribute('data-k') === 'v' && el.classList.contains('c1');
				el.classList.add('c2'); ok = ok && el.classList.contains('c2');
				root.innerHTML = '<b>bold</b><i>it</i>';
				ok = ok && root.children.length === 2 && root.children[0].tagName.toLowerCase() === 'b';
				root.removeChild(root.children[1]);
				ok = ok && root.children.length === 1;
				root.textContent = 'plain';
				ok = ok && root.children.length === 0 && root.textContent === 'plain';
				window.__result = ok ? 'PASS' : 'FAIL: dom ops';
			</script></body></html>`,
	},
	{
		name: "js-event",
		html: `<!DOCTYPE html><html><body>
			<button id="b">go</button><input id="i">
			<script>
				var clicks = 0, inputs = 0;
				document.getElementById('b').addEventListener('click', function(){ clicks++; });
				document.getElementById('i').addEventListener('input', function(){ inputs++; });
				document.getElementById('b').dispatchEvent(new Event('click'));
				document.getElementById('b').dispatchEvent(new Event('click'));
				document.getElementById('i').dispatchEvent(new Event('input'));
				var ok = clicks === 2 && inputs === 1;
				window.__result = ok ? 'PASS' : 'FAIL: clicks=' + clicks + ' inputs=' + inputs;
			</script></body></html>`,
	},
	{
		name: "js-fetch-bridge",
		html: `<!DOCTYPE html><html><body>
			<script>
				fetch('/_browser/echo', { method:'POST', body: JSON.stringify({x:1}) })
				.then(function(r){ return r.json(); })
				.then(function(d){
					var ok = d.ok === true && d.echo === '{"x":1}';
					window.__result = ok ? 'PASS' : 'FAIL: echo=' + JSON.stringify(d);
				})
				.catch(function(e){ window.__result = 'FAIL: ' + e; });
			</script></body></html>`,
		wait: 100 * time.Millisecond,
	},
	{
		name: "js-timer",
		html: `<!DOCTYPE html><html><body>
			<script>
				window.__timerHit = false;
				setTimeout(function(){ window.__timerHit = true; window.__result = 'PASS'; }, 30);
			</script></body></html>`,
		wait: 120 * time.Millisecond,
	},
	{
		name: "form",
		html: `<!DOCTYPE html><html><body>
			<input id="txt" value="a"><textarea id="ta">t</textarea>
			<input id="cb" type="checkbox"><input id="rb1" type="radio" name="g"><input id="rb2" type="radio" name="g">
			<select id="sel"><option>o1</option><option>o2</option></select>
			<script>
				try {
				var ok = true, fail = '';
				var txt = document.getElementById('txt');
				txt.value = 'hello'; ok = ok && txt.value === 'hello'; if (txt.value !== 'hello') fail += ' txt=' + txt.value;
				var cb = document.getElementById('cb'); cb.checked = true; ok = ok && cb.checked; if (!cb.checked) fail += ' cb';
				document.getElementById('rb2').checked = true;
				var sel = document.getElementById('sel'); sel.selectedIndex = 1;
				ok = ok && sel.value === 'o2'; if (sel.value !== 'o2') fail += ' sel.value=' + sel.value;
				ok = ok && sel.options.length === 2; if (sel.options.length !== 2) fail += ' sel.options=' + (sel.options && sel.options.length);
				ok = ok && document.getElementById('ta').value === 't'; if (document.getElementById('ta').value !== 't') fail += ' ta';
				window.__result = ok ? 'PASS' : 'FAIL: form' + fail;
				} catch(e) { window.__result = 'FAIL: ' + (e && e.message || e); }
			</script></body></html>`,


	},
	{
		name: "svg-render",
		html: `<!DOCTYPE html><html><body>
			<svg id="s" width="100" height="50">
				<rect x="1" y="1" width="40" height="20" fill="red"/>
				<circle cx="70" cy="15" r="10" fill="blue"/>
				<text x="5" y="40">svg-text</text>
			</svg>
			<script>
				try {
				var s = document.getElementById('s');
				var names = [];
				for (var i = 0; i < s.childNodes.length; i++) names.push(String(s.childNodes[i].nodeName).toLowerCase());
				var ok = names.indexOf('rect') >= 0 && names.indexOf('circle') >= 0 && names.indexOf('text') >= 0;
				window.__result = ok ? 'PASS' : 'FAIL: svg children=' + names.join(',');
				} catch(e) { window.__result = 'FAIL: ' + (e && e.message || e); }
			</script></body></html>`,
	},
	{
		name: "console",
		html: `<!DOCTYPE html><html><body>
			<script>
				console.log('hello from page', 42);
				console.error('err line');
				window.__result = 'PASS';
			</script></body></html>`,
	},
	{
		name: "scroll-geom",
		html: `<!DOCTYPE html><html><head><style>
			#sc { overflow:auto; width:100px; height:50px; }
			#sc .inner { height:200px; }
		</style></head><body><div id="sc"><div class="inner"></div></div>
			<script>
				var sc = document.getElementById('sc');
				var ok = sc.scrollHeight >= 200 && sc.clientHeight <= 60 && sc.clientHeight >= 40;
				sc.scrollTop = 100;
				ok = ok && sc.scrollTop >= 95;
				window.__result = ok ? 'PASS' : 'FAIL: sh=' + sc.scrollHeight + ' ch=' + sc.clientHeight + ' st=' + sc.scrollTop;
			</script></body></html>`,
	},
	{
		name: "attr-class",
		html: `<!DOCTYPE html><html><body><div id="d" class="a b" title="tt"></div>
			<script>
				try {
				var d = document.getElementById('d');
				var ok = d.className === 'a b' && d.title === 'tt';
				if (d.className !== 'a b') window.__result = 'FAIL: className=' + d.className;
				d.setAttribute('data-x', '1'); ok = ok && d.getAttribute('data-x') === '1';
				d.removeAttribute('title'); ok = ok && d.title === '';
				d.style.color = 'red'; ok = ok && d.style.color === 'red';
				ok = ok && d.id === 'd' && d.tagName.toLowerCase() === 'div';
				window.__result = ok ? 'PASS' : 'FAIL: attrs color=' + d.style.color + ' title=' + d.title + ' tag=' + d.tagName;
				} catch(e) { window.__result = 'FAIL: ' + (e && e.message || e); }
			</script></body></html>`,
	},
	{
		// iframe 子文档：子 Frame 加载 data: 子文档，内容在 iframe 内容框内
		// 渲染（RenderIFrame）。JS 断言 iframe 元素几何；Go 侧 check 断言
		// 子 Frame 已创建、子文档已解析、视口尺寸与内容框一致。
		name: "iframe",
		html: `<!DOCTYPE html><html><head><style>
			#box { width:220px; padding:10px; }
			#f1 { border:1px solid #999; }
		</style></head><body>
			<div id="box"><iframe id="f1" width="200" height="100" src="data:text/html,%3Chtml%3E%3Cbody%3E%3Cp%20id='c1'%3Echild%20page%3C/p%3E%3Cdiv%20id='blue'%20style='width:60px;height:30px;background:%2333aaff'%3E%3C/div%3E%3C/body%3E%3C/html%3E"></iframe></div>
			<script>
				var f = document.getElementById('f1');
				var ok = !!f && f.tagName.toLowerCase() === 'iframe';
				var r = f.getBoundingClientRect();
				ok = ok && Math.abs(r.width - 200) <= 2 && Math.abs(r.height - 100) <= 2;
				window.__result = ok ? 'PASS' : 'FAIL: iframe rect=' + r.width + 'x' + r.height;
			</script></body></html>`,
		check: func(wv *webkit.WebView) (bool, string) {
			// Go 侧：iframe 子 Frame 已创建、子文档含 #c1、视口 200x100
			doc := wv.MainFrame().Document()
			if doc == nil {
				return false, "no document"
			}
			f := doc.GetElementById("f1")
			if f == nil {
				return false, "no #f1 element"
			}
			sub := page.IFrameFrame(f)
			if sub == nil {
				return false, "iframe 子 Frame 未创建 (page.IFrameFrame nil)"
			}
			subDoc := sub.Document()
			if subDoc == nil {
				return false, "子文档 nil"
			}
			if p := subDoc.GetElementById("c1"); p == nil {
				return false, "子文档缺少 #c1"
			}
			wv.EnsureLayout()
			sub.LayoutNow()
			if sub.ViewportWidth() != 200 || sub.ViewportHeight() != 100 {
				return false, fmt.Sprintf("子视口=%dx%d, want 200x100", sub.ViewportWidth(), sub.ViewportHeight())
			}
			return true, ""
		},
	},
}
