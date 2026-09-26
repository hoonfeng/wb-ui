// Command vuesupport 诊断 Vue 3 在 wb-ui 引擎上的支持度、渲染正确性与调度时序。
//
// 背景：gouide 的真实前端（plugins-src/ui-app 与各 UI 区域插件，Vue 3.5）最终要跑在
// 自研引擎上，因此「Vue 能挂载、能正确更新」是 gouide 迁移的前置条件。本探针与
// dev/probes/reactsupport 对齐，但验证的是 Vue 的依赖面与渲染路径：
//
//	① Vue 3.5 源码真实依赖的 Web API 面（计数取自 vue.global.js：WeakMap 19 处、
//	   Object.defineProperty 18 处、Reflect 13 处、new Proxy 8 处、Symbol.iterator
//	   6 处、requestIdleCallback 3 处、new Function 2 处、MutationObserver 2 处……）
//	② 模板编译路径（依赖 new Function）与渲染函数路径是否都通
//	③ 响应式更新：computed/v-for/v-if/v-model/@click/组件 emit 是否驱动 DOM
//	④ nextTick / Promise / setTimeout 的调度时序
//
// 页面由探针内联装配（不依赖外部 server）：把 vue.global.js 与诊断脚本拼进 HTML，
// 经 LoadHTMLWithBaseURL 交给引擎，再用 pumpFrame 驱动事件循环与渲染帧。
//
// 用法（wb-ui 仓库根，CGO 环境）：
//
//	go run ./dev/probes/vuesupport
//	go run ./dev/probes/vuesupport -prod -rounds 90 -out out/vuesupport-prod.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"wb-ui/engine/js/jsc"
	"wb-ui/webkit"
)

// jsDiag 启动探测：① API 面 ② 动态代码 ③ Proxy/Reflect ④ 依赖收集基础设施
// ⑤ DOM 面 ⑥ Vue 装载 ⑦ 模板编译 + 最小应用挂载（含渲染函数对照路径）。
const jsDiag = `(function () {
  var v = { api: {}, dyn: {}, proxy: {}, reflect: {}, infra: {}, dom: {}, vue: {}, mount: {}, render: {}, errors: [] };
  window.__v = v;
  function E(name, e) { v.errors.push(name + ': ' + String((e && e.stack) || e)); }

  // ① API 面（左列 = Vue 3.5 源码实际引用；带下划线的是对照项）
  var names = ['Proxy', 'Reflect', 'WeakMap', 'WeakSet', 'WeakRef', 'Symbol', 'Promise', 'Map', 'Set',
    'MutationObserver', 'IntersectionObserver', 'ResizeObserver', 'requestIdleCallback',
    'requestAnimationFrame', 'cancelAnimationFrame', 'getComputedStyle', 'matchMedia', 'CustomEvent',
    'Event', 'EventTarget', 'Node', 'Element', 'HTMLElement', 'SVGElement', 'Text', 'Comment',
    'DocumentFragment', 'queueMicrotask', 'setImmediate', 'MessageChannel', 'performance',
    'structuredClone', 'Intl', 'customElements', 'CSS', 'localStorage', 'navigator', 'location', 'history',
    'XMLHttpRequest', 'fetch', 'Image', 'URL', 'TextEncoder', 'reportError'];
  for (var i = 0; i < names.length; i++) { v.api[names[i]] = typeof window[names[i]]; }
  v.api._document = typeof document;
  v.api._createElement = typeof (document && document.createElement);
  v.api._createElementNS = typeof (document && document.createElementNS);
  v.api._createTreeWalker = typeof (document && document.createTreeWalker);
  v.api._createComment = typeof (document && document.createComment);
  v.api._createDocumentFragment = typeof (document && document.createDocumentFragment);
  v.api._addEventListener = typeof (window && window.addEventListener);
  v.api._objectKeys = typeof Object.keys;
  v.api._getOwnPropertyDescriptor = typeof Object.getOwnPropertyDescriptor;
  v.api._defineProperty = typeof Object.defineProperty;
  v.api._arrayIsArray = typeof Array.isArray;
  v.api._nodeType = typeof Node;
  v.api._nodePrototype = typeof (window.Node && window.Node.prototype);
  v.api._nodeProtoMethods = (window.Node && window.Node.prototype) ? Object.getOwnPropertyNames(window.Node.prototype).join(',') : '';

  // ② 动态代码（模板编译依赖 new Function）
  try {
    var f = new Function('a', 'b', 'return a + b;');
    var got = f(20, 22);
    v.dyn.newFunction = (got === 42) ? true : ('wrong:' + got);
  } catch (e) { v.dyn.newFunction = 'ERR: ' + String((e && e.message) || e); }
  try {
    var f2 = new Function('with (this) { return count + 1; }');
    v.dyn.newFunctionWithThis = f2.call({ count: 41 });
  } catch (e) { v.dyn.newFunctionWithThis = 'ERR: ' + String((e && e.message) || e); }
  try { v.dyn.eval = (eval('6*7') === 42) ? true : 'wrong'; } catch (e) { v.dyn.eval = 'ERR: ' + String((e && e.message) || e); }

  // ③ Proxy trap 行为（reactive 核心；每个 trap 是否真被调用都要看到）
  try {
    var t = { a: 1 }, log = [];
    var p = new Proxy(t, {
      get: function (o, k, r) { log.push('get:' + String(k)); return o[k]; },
      set: function (o, k, val, r) { log.push('set:' + String(k)); o[k] = val; return true; },
      has: function (o, k) { log.push('has:' + String(k)); return k in o; },
      deleteProperty: function (o, k) { log.push('del:' + String(k)); delete o[k]; return true; },
      ownKeys: function (o) { log.push('ownKeys'); return Reflect.ownKeys(o); },
      getOwnPropertyDescriptor: function (o, k) { log.push('gopd:' + String(k)); return Reflect.getOwnPropertyDescriptor(o, k); }
    });
    p.b = 2;
    var readA = p.a, afterSet = t.b, hasIn = ('a' in p), keys = Object.keys(p).join(',');
    delete p.b;
    var afterDel = ('b' in t);
    v.proxy.readA = readA;
    v.proxy.targetAfterSet = afterSet;
    v.proxy.hasIn = hasIn;
    v.proxy.keys = keys;
    v.proxy.afterDelete = afterDel;
    v.proxy.log = log.join(',');
    v.proxy.ok = (readA === 1 && afterSet === 2 && hasIn === true && keys === 'a,b' && afterDel === false);
  } catch (e) { v.proxy.err = String((e && e.stack) || e); }

  // ④ Reflect + 依赖收集基础设施（WeakMap / defineProperty / Symbol.iterator / 数组方法）
  try {
    var ro = {}; Reflect.set(ro, 'x', 5);
    v.reflect = {
      set: ro.x, get: Reflect.get({ y: 9 }, 'y'), has: Reflect.has({ z: 1 }, 'z'),
      ownKeys: Reflect.ownKeys({ a: 1 }).join(','), deleteOk: Reflect.deleteProperty(ro, 'x')
    };
  } catch (e) { v.reflect.err = String((e && e.stack) || e); }
  try {
    var wm = new WeakMap(), wmk = {};
    wm.set(wmk, 7);
    var it = { a: 1, b: 2 }, acc = [];
    for (var kk in it) { acc.push(kk); }
    var arr = [3, 1, 2].map(function (x) { return x * 2; }).filter(function (x) { return x > 2; });
    v.infra = {
      weakMap: wm.get(wmk),
      symbolIterator: (typeof Symbol.iterator) + ':' + (typeof it[Symbol.iterator]),
      hasIterator: (typeof Symbol !== 'undefined') && (typeof Symbol.iterator !== 'undefined'),
      defineProperty: (function () {
        var o = {}, calls = 0;
        Object.defineProperty(o, 'p', {
          get: function () { calls++; return 'got'; },
          set: function (x) { calls++; },
          enumerable: true, configurable: true
        });
        o.p = 1;
        var g = o.p;
        return { v: g, calls: calls };
      })(),
      arrayMap: arr.join(','),
      forIn: acc.join(','),
      getOwnPropertyNames: Object.getOwnPropertyNames({ a: 1, b: 2 }).join(','),
      getOwnPropertyDescriptor: JSON.stringify(Object.getOwnPropertyDescriptor({ a: 1 }, 'a') || null)
    };
  } catch (e) { v.infra.err = String((e && e.stack) || e); }

  // ⑤ DOM 面
  try {
    var nsEl = null;
    try { nsEl = document.createElementNS('http://www.w3.org/2000/svg', 'svg'); } catch (e) { v.dom.createElementNSErr = String((e && e.message) || e); }
    v.dom.createElementNS = nsEl ? String(nsEl.tagName || '') : 'null';
    var cl = document.createElement('div');
    cl.appendChild(document.createElement('span'));
    v.dom.cloneNode = cl.cloneNode(true).children.length;
    v.dom.createTreeWalker = typeof document.createTreeWalker;
    v.dom.createComment = (function () {
      var c = document.createComment('x');
      document.body.appendChild(c);
      var ok = (document.body.lastChild === c);
      c.parentNode.removeChild(c);
      return ok ? 'ok' : 'not-attached';
    })();
    v.dom.fragment = (function () {
      var fr = document.createDocumentFragment();
      fr.appendChild(document.createElement('i'));
      fr.appendChild(document.createElement('b'));
      return fr.childNodes.length;
    })();
    if (typeof window.matchMedia === 'function') {
      var mq = window.matchMedia('(min-width: 100px)');
      v.dom.matchMediaRet = String(mq && mq.matches);
    } else { v.dom.matchMediaRet = 'no-matchMedia'; }
    if (typeof window.getComputedStyle === 'function') {
      var cd = document.createElement('div');
      cd.style.width = '10px';
      document.body.appendChild(cd);
      var cs = window.getComputedStyle(cd);
      v.dom.getComputedStyle = cs ? (typeof cs.width) + ':' + String(cs.width) : 'null';
      document.body.removeChild(cd);
    } else { v.dom.getComputedStyle = 'no-getComputedStyle'; }
    var evEl = document.createElement('div'), hits = 0;
    evEl.addEventListener('ping', function () { hits++; });
    var ce = null;
    try { ce = new CustomEvent('ping', { detail: { n: 1 } }); } catch (e) { v.dom.customEventErr = String((e && e.message) || e); }
    evEl.dispatchEvent(ce || { type: 'ping' });
    v.dom.customEventHits = hits;
  } catch (e) { v.dom.err = String((e && e.stack) || e); }

  // ⑥ Vue 装载
  v.vue.type = typeof window.Vue;
  if (window.Vue) {
    v.vue.version = window.Vue.version || '';
    v.vue.exportKeys = Object.keys(window.Vue).join(',');
    if (typeof window.Vue.compile === 'function') {
      try {
        var rf = window.Vue.compile('<div class="c">{{ msg }}</div>');
        v.vue.compileReturned = typeof rf;
      } catch (e) { v.vue.compileErr = String((e && e.stack) || e); }
    } else { v.vue.compileReturned = 'no-compile-export'; }
  }

  // ⑦ 渲染函数路径（绕开模板编译器：若它通而模板不通 → 缺的是 new Function）
  if (window.Vue && typeof window.Vue.createApp === 'function') {
    try {
      var RF = {
        render: function () {
          var h = window.Vue.h;
          return h('div', { id: 'rf' }, [
            h('span', { class: 's1' }, 'rf-ok'),
            h('p', { id: 'rf-cnt' }, 'n=' + this.n)
          ]);
        },
        data: function () { return { n: 5 }; }
      };
      var app2 = window.Vue.createApp(RF);
      app2.mount('#app2');
      window.__vm2 = app2._instance && app2._instance.proxy;
      v.render.mounted = true;
    } catch (e) { E('renderFn.mount', e); }
  }

  // ⑧ 模板路径：最小但覆盖真实用法（插值/computed/v-for/v-if/v-model/@click/组件 emit/生命周期）
  if (window.Vue && typeof window.Vue.createApp === 'function') {
    try {
      var Child = {
        props: ['label'],
        emits: ['ping'],
        template: '<span class="child" @click="$emit(\'ping\', label)">{{ label }}!</span>'
      };
      var App = {
        components: { Child: Child },
        template: [
          '<div class="wrap" id="wrap">',
          '<h1 id="title">{{ title }}</h1>',
          '<p id="cnt">count={{ count }} doubled={{ doubled }}</p>',
          '<ul id="list"><li v-for="(it, i) in items" :key="it.id" class="item" :data-i="i">{{ it.name }}</li></ul>',
          '<input id="inp" v-model="title">',
          '<button id="btn" @click="inc">+1</button>',
          '<child id="ch" :label="title" @ping="onPing"></child>',
          '<p v-if="count > 0" id="vi">visible</p>',
          '</div>'
        ].join(''),
        data: function () { return { title: 'vue-ok', count: 0, items: [{ id: 1, name: 'a' }, { id: 2, name: 'b' }, { id: 3, name: 'c' }] }; },
        computed: { doubled: function () { return this.count * 2; } },
        methods: {
          inc: function () { this.count++; },
          onPing: function (x) { v.mount.pinged = String(x); }
        },
        mounted: function () { v.mount.mounted = true; },
        updated: function () { v.mount.updates = (v.mount.updates || 0) + 1; }
      };
      var app = window.Vue.createApp(App);
      window.__app = app;
      var vm = app.mount('#app');
      window.__vm = vm;
      v.mount.returnedVM = !!vm;
      v.mount.ok = true;
    } catch (e) { E('createApp.mount', e); }
  }

  // ⑨ 调度时序
  v.sched = {};
  try { window.Vue.nextTick(function () { v.sched.nextTick = true; }); } catch (e) { E('nextTick', e); }
  try { Promise.resolve().then(function () { v.sched.promise = true; }); } catch (e) { E('promise', e); }
  try { queueMicrotask(function () { v.sched.queueMicrotask = true; }); } catch (e) { E('queueMicrotask', e); }
  try { setTimeout(function () { v.sched.timeout = true; }, 0); } catch (e) { E('timeout', e); }
  try { requestAnimationFrame(function () { v.sched.raf = true; }); } catch (e) { E('raf', e); }
  try { requestIdleCallback(function () { v.sched.idle = true; }); } catch (e) { E('idle', e); }

  return 'started';
})()`

// jsUpdate 第二阶段：改响应式状态、DOM 事件、v-model，检查更新是否真的落到 DOM。
const jsUpdate = `(function () {
  var v = window.__v, vm = window.__vm, out = { steps: [] };
  v.update = out;
  if (!vm) { out.err = 'no vm (mount 失败)'; return 'no-vm'; }

  // ① 纯响应式赋值（computed + 插值 + v-if 应随之更新）
  try {
    out.beforeCnt = (document.getElementById('cnt') || {}).textContent || '';
    vm.count = 3;
    out.afterSetCount = vm.count;
    out.computedNow = vm.doubled;
    out.steps.push('set-count');
  } catch (e) { out.pushErr1 = String((e && e.stack) || e); }

  // ② 数组变更（v-for 应新增一项）
  try {
    vm.items.push({ id: 4, name: 'd' });
    out.itemsNow = vm.items.length;
    out.steps.push('push-item');
  } catch (e) { out.pushErr2 = String((e && e.stack) || e); }

  // ③ 方法调用（@click 的处理函数路径）
  try {
    vm.inc();
    out.afterInc = vm.count;
    out.steps.push('call-inc');
  } catch (e) { out.pushErr3 = String((e && e.stack) || e); }

  // ④ 真实 DOM 事件（事件系统路径：btn.click → @click → inc）
  try {
    var btn = document.getElementById('btn');
    out.btnExists = !!btn;
    if (btn) {
      if (typeof btn.click === 'function') { btn.click(); } else if (typeof btn.dispatchEvent === 'function') { btn.dispatchEvent(new Event('click')); }
      out.afterDomClick = vm.count;
    }
    out.steps.push('dom-click');
  } catch (e) { out.domClickErr = String((e && e.stack) || e); }

  // ⑤ v-model：写 value + input 事件应回写 title
  try {
    var inp = document.getElementById('inp');
    out.inpExists = !!inp;
    if (inp) {
      inp.value = 'from-input';
      inp.dispatchEvent(new Event('input'));
      out.titleAfterInput = vm.title;
    }
    out.steps.push('v-model');
  } catch (e) { out.inputErr = String((e && e.stack) || e); }

  // ⑥ 组件 emit：点击子组件 → 父组件 @ping
  try {
    var ch = document.getElementById('ch');
    out.childExists = !!ch;
    if (ch) {
      if (typeof ch.click === 'function') { ch.click(); } else if (typeof ch.dispatchEvent === 'function') { ch.dispatchEvent(new Event('click')); }
    }
    out.steps.push('child-emit');
  } catch (e) { out.childErr = String((e && e.stack) || e); }

  // ⑦ 渲染函数路径的响应式更新
  try {
    if (window.__vm2) { window.__vm2.n = 6; out.rfUpdated = window.__vm2.n; }
  } catch (e) { out.rfErr = String((e && e.stack) || e); }

  return 'updated';
})()`

// jsCollect 读回最终 DOM / 组件状态（含 Vue 内部挂载标记，用于区分「没挂载」与「挂载了但没更新」）。
const jsCollect = `(function () {
  function safe(fn, dflt) {
    try { var x = fn(); return (x === undefined || x === null) ? dflt : x; }
    catch (e) { return 'ERR:' + String((e && e.message) || e); }
  }
  var v = window.__v || {};
  var r = v.result = v.result || {};
  var appEl = document.getElementById('app');
  r.appExists = !!appEl;
  r.appInnerLen = safe(function () { return appEl.innerHTML.length; }, -1);
  r.appChildCount = safe(function () { return appEl.children.length; }, -1);
  r.totalElements = safe(function () { return document.getElementsByTagName('*').length; }, -1);
  r.bodyTextLen = safe(function () { return (document.body.textContent || '').length; }, -1);
  r.wrapExists = safe(function () { return !!document.getElementById('wrap'); }, false);
  r.titleText = safe(function () { return document.getElementById('title').textContent; }, '');
  r.cntText = safe(function () { return document.getElementById('cnt').textContent; }, '');
  r.listCount = safe(function () { return document.getElementById('list').children.length; }, -1);
  r.listText = safe(function () {
    var out = [], li = document.getElementById('list').children;
    for (var i = 0; i < li.length; i++) { out.push(li[i].textContent); }
    return out.join('|');
  }, '');
  r.viExists = safe(function () { return !!document.getElementById('vi'); }, false);
  r.childText = safe(function () { var c = document.getElementById('ch'); return c ? c.textContent : ''; }, '');
  r.rfText = safe(function () { var c = document.getElementById('rf'); return c ? c.textContent : ''; }, '');
  r.appVueKeys = safe(function () {
    return Object.getOwnPropertyNames(appEl).filter(function (k) { return k.indexOf('__v') === 0; }).join(',');
  }, '');
  var vm = window.__vm;
  r.vmExists = !!vm;
  r.vmTitle = safe(function () { return vm.title; }, '');
  r.vmCount = safe(function () { return vm.count; }, -1);
  r.vmItems = safe(function () { return vm.items.length; }, -1);
  r.vmDoubled = safe(function () { return vm.doubled; }, -1);
  r.appHasVnode = safe(function () { return ('__vue_app__' in appEl) || ('__vnode' in appEl) || ('__vueParentComponent' in appEl); }, false);
  return JSON.stringify(v);
})()`

func evalStr(wv *webkit.WebView, js string) string {
	v, err := wv.EvalJS(js)
	if err != nil {
		return "<eval error: " + err.Error() + ">"
	}
	return v.ToString()
}

func pumpFrame(wv *webkit.WebView) {
	if el := wv.JSInterpreter().GetEventLoop(); el != nil {
		el.ProcessTasks(0)
	}
	wv.JSInterpreter().RunJobs()
	wv.EnsureLayout()
	_, _ = wv.Render()
}

// vueCandidates 返回 vue.global(.prod).js 的候选路径（按优先级）。
func vueCandidates(prod bool) []string {
	name := "vue.global.js"
	if prod {
		name = "vue.global.prod.js"
	}
	tail := []string{"node_modules", "vue", "dist", name}
	rel := append([]string{"..", "gou-ide", "cmd", "companion", "web-ui"}, tail...)
	rel2 := append([]string{"cmd", "companion", "web-ui"}, tail...)
	abs := append([]string{"/f/syproject/gou-ide/cmd/companion/web-ui"}, tail...)
	return []string{
		filepath.Join(rel...),
		filepath.Join(rel2...),
		filepath.Join(abs...),
	}
}

// loadVue 读入 Vue 产物源码；explicit 非空时优先使用。
func loadVue(explicit string, prod bool) (string, string, error) {
	var cands []string
	if explicit != "" {
		cands = append(cands, explicit)
	}
	cands = append(cands, vueCandidates(prod)...)
	var tried []string
	for _, p := range cands {
		b, err := os.ReadFile(p)
		if err == nil {
			return p, string(b), nil
		}
		tried = append(tried, p)
	}
	return "", "", fmt.Errorf("未找到 Vue 产物，尝试过：%s", strings.Join(tried, " | "))
}

// buildHTML 把 Vue 源码与诊断脚本内联成页面（不依赖外部 server）。
func buildHTML(vueJS string) string {
	// <script> 内出现字面 </script 会提前闭合标签，转义为 <\/script（JS 等价）。
	safe := strings.ReplaceAll(vueJS, "</script", "<\\/script")
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\">")
	b.WriteString("<title>vuesupport</title>")
	b.WriteString("<style>body{margin:0;font:14px sans-serif}.item{display:block}</style>")
	b.WriteString("</head><body>")
	b.WriteString("<div id=\"app\"></div><div id=\"app2\"></div>")
	b.WriteString("<script>")
	b.WriteString(safe)
	b.WriteString("</script>")
	b.WriteString("<script>")
	b.WriteString(jsDiag)
	b.WriteString("</script>")
	b.WriteString("</body></html>")
	return b.String()
}

func main() {
	vuePath := flag.String("vue", "", "vue.global.js 路径（缺省自动探测 gou-ide companion 的 node_modules）")
	prod := flag.Bool("prod", false, "使用 vue.global.prod.js（生产版；dev 版警告更详细）")
	rounds := flag.Int("rounds", 60, "每阶段驱动轮数")
	w := flag.Int("w", 1280, "视口宽")
	h := flag.Int("h", 860, "视口高")
	out := flag.String("out", "out/vuesupport.json", "JSON 报告输出路径")
	noUpdate := flag.Bool("no-update", false, "跳过响应式更新阶段")
	flag.Parse()

	path, vueJS, err := loadVue(*vuePath, *prod)
	if err != nil {
		fmt.Printf("[FAIL] %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[OK] Vue 产物 %s（%.0fKB，%s）\n", path, float64(len(vueJS))/1024, map[bool]string{true: "prod", false: "dev"}[*prod])

	wv := webkit.NewWebView()
	defer wv.Destroy()
	wv.Resize(*w, *h)
	wv.SetConsoleLogger(&jsc.BufferLogger{})

	html := buildHTML(vueJS)
	start := time.Now()
	if err := wv.LoadHTMLWithBaseURL(html, "http://127.0.0.1/"); err != nil {
		fmt.Printf("[FAIL] LoadHTMLWithBaseURL: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[OK] 页面装配（内联 %.0fKB）%.1fms（视口 %dx%d）\n", float64(len(html))/1024, float64(time.Since(start).Microseconds())/1000, *w, *h)

	fmt.Println("[diag] 启动探测:", evalStr(wv, jsDiag))
	for i := 0; i < *rounds; i++ {
		pumpFrame(wv)
	}

	if !*noUpdate {
		fmt.Println("[update] 响应式更新:", evalStr(wv, jsUpdate))
		for i := 0; i < *rounds; i++ {
			pumpFrame(wv)
		}
	}

	raw := evalStr(wv, jsCollect)
	fmt.Println("=== 探测结果 ===")
	if pretty, err := prettyJSON(raw); err == nil {
		fmt.Println(pretty)
	} else {
		fmt.Println(raw)
	}

	if *out != "" {
		if dir := filepath.Dir(*out); dir != "" {
			_ = os.MkdirAll(dir, 0o755)
		}
		if pretty, err := prettyJSON(raw); err == nil {
			if werr := os.WriteFile(*out, []byte(pretty+"\n"), 0o644); werr != nil {
				fmt.Printf("[WARN] 写报告失败: %v\n", werr)
			} else {
				fmt.Printf("[OK] 报告已写入 %s\n", *out)
			}
		}
	}

	fmt.Println("=== console 输出 ===")
	if s := strings.TrimSpace(wv.ConsoleOutput()); s != "" {
		fmt.Println(s)
	} else {
		fmt.Println("(无输出)")
	}
}

func prettyJSON(s string) (string, error) {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	return string(b), err
}
