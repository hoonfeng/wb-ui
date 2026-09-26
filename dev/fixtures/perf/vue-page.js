// Vue 3 版 IDE 形态压测页 —— 与 React 版（dev/probes/idepage）同规模、同布局、
// 同基准接口，用于「框架 × 引擎」对比：
//
//   · 规模参数同源：tree=320 行、lines=1800 行（可被 window.__VUE_PAGE_CONFIG 覆盖）
//   · 数据生成逐字复用 React 版的 tokenize/makeCodeLine（保证 DOM 形状与 token 数一致）
//   · window.__bench 字段与 __runJs / __runReflow / __runStyleRecalc / __runScroll /
//     __countDom 逐个对齐；只有更新吞吐入口不同（React 用 __runReactUpdate，本页用
//     __runVueUpdate）
//   · 渲染走 Vue 的 h() 渲染函数路径（与 React 的 createElement 命令式路径对齐，
//     也贴近 SFC 编译产物），不使用模板编译
//
// 由 dev/probes/framework_matrix 内联加载（Vue 产物 + 本文件 + 与 React 页完全相同的
// <style>），页面自身不引用任何外部资源。
(function () {
  'use strict';

  var h = Vue.h;
  var cfg = window.__VUE_PAGE_CONFIG || {};
  var TREE_ROWS = cfg.tree || 320;
  var CODE_LINES = cfg.lines || 1800;

  // ---------- 文本分词（逐字复用 React 版；同时作为 JS 计算负载）----------
  var KEYWORDS = new Set(['const', 'let', 'var', 'function', 'return', 'if', 'else', 'for', 'while',
    'class', 'new', 'await', 'async', 'import', 'export', 'from', 'try', 'catch', 'this',
    'null', 'undefined', 'true', 'false', 'typeof', 'instanceof']);
  var RE_TOKEN = /(\/\/[^\n]*)|('[^']*'|"[^"]*"|`[^`]*`)|\b(\d+(?:\.\d+)?)\b|\b([A-Za-z_$][\w$]*)\b|([{}()[\];,.:=+\-*/<>!&|?]+)|(\s+)/g;

  function tokenize(src) {
    var out = [];
    var m;
    RE_TOKEN.lastIndex = 0;
    while ((m = RE_TOKEN.exec(src)) !== null) {
      var k = 'punct', v = m[0];
      if (m[1] !== undefined) k = 'comment';
      else if (m[2] !== undefined) k = 'string';
      else if (m[3] !== undefined) k = 'number';
      else if (m[4] !== undefined) k = KEYWORDS.has(m[4]) ? 'kw' : 'ident';
      else if (m[6] !== undefined) k = 'ws';
      out.push([k, v]);
    }
    return out;
  }

  function makeCodeLine(i) {
    var n = i % 7;
    switch (n) {
      case 0: return 'const handler' + i + ' = async (event) => { /* 行 ' + i + ' */';
      case 1: return '  const value' + i + " = await fetch('/api/node/" + i + "').then((r) => r.json());";
      case 2: return '  if (value' + i + " && value" + i + ".kind === 'widget') return value" + i + '.render(' + i + ');';
      case 3: return '  for (let j = 0; j < ' + (i % 40) + '; j++) { total += j * ' + (i % 13) + '; }';
      case 4: return "  return { id: " + i + ", label: '节点-" + i + "', depth: " + (i % 9) + ', collapsed: false };';
      case 5: return '}';
      default: return '';
    }
  }

  var CODE_LINES_DATA = [];
  for (var ci = 0; ci < CODE_LINES; ci++) { CODE_LINES_DATA.push(makeCodeLine(ci)); }
  var TOKENS_DATA = CODE_LINES_DATA.map(tokenize);
  var TREE_DATA = [];
  for (var ti = 0; ti < TREE_ROWS; ti++) {
    TREE_DATA.push({
      id: ti,
      name: 'node_' + ti + '.ts' + (ti % 3 === 0 ? 'x' : ''),
      depth: ti % 6,
      dir: ti % 4 === 0,
    });
  }

  // ---------- 视图（渲染函数路径）----------
  function fileTree() {
    var rows = [h('div', { class: 'tree-head' }, '资源管理器')];
    for (var i = 0; i < TREE_DATA.length; i++) {
      var n = TREE_DATA[i];
      rows.push(h('div', {
        key: n.id,
        class: 'tree-row' + (n.dir ? ' dir' : ''),
        style: { paddingLeft: (8 + n.depth * 14) + 'px' },
      }, [h('span', { class: 'ico' }, n.dir ? '▸' : '·'), n.name]));
    }
    return h('div', { class: 'tree', id: 'tree' }, rows);
  }

  function codeView() {
    var lines = [];
    for (var i = 0; i < TOKENS_DATA.length; i++) {
      var toks = TOKENS_DATA[i];
      var spans = [h('span', { class: 'ln' }, String(i + 1))];
      for (var j = 0; j < toks.length; j++) {
        spans.push(h('span', { key: j, class: 'tok t-' + toks[j][0] }, toks[j][1]));
      }
      lines.push(h('div', { class: 'line', key: i }, spans));
    }
    return h('div', { class: 'code', id: 'code' }, lines);
  }

  function panel(tick) {
    var chips = [];
    var names = ['A', 'B', 'C', 'D'];
    for (var i = 0; i < names.length; i++) { chips.push(h('span', { key: names[i], class: 'chip' }, names[i])); }
    return h('div', { class: 'panel', id: 'panel' }, [
      h('div', { class: 'panel-head' }, '属性'),
      h('div', { class: 'card' }, [
        h('div', { class: 'card-title' }, '构建状态'),
        h('div', { class: 'bar-outer' }, [
          h('div', { class: 'bar-inner', id: 'bar', style: { width: ((tick * 7) % 100) + '%' } }),
        ]),
      ]),
      h('div', { class: 'card grad' }, [
        h('div', { class: 'card-title' }, '渐变 / 圆角 / 阴影'),
        h('div', { class: 'chip-row' }, chips),
      ]),
      h('div', { class: 'card' }, [h('div', { class: 'card-title' }, '计数 ' + tick)]),
    ]);
  }

  // ---------- 状态与挂载 ----------
  var state = Vue.reactive({ tick: 0, rafCount: 0 });

  var app = Vue.createApp({
    render: function () {
      return h('div', { class: 'app', id: 'app' }, [
        h('div', { class: 'titlebar' }, 'IDE 形态压测页 · Vue ' + Vue.version +
          '（tree=' + TREE_ROWS + ' lines=' + CODE_LINES + '）'),
        h('div', { class: 'body' }, [fileTree(), codeView(), panel(state.tick)]),
        h('div', { class: 'statusbar' }, [
          h('span', null, '就绪'),
          h('span', null, 'rAF ' + state.rafCount),
          h('span', { class: 'spacer' }),
          h('button', { class: 'btn', id: 'btn', onClick: function () { state.tick++; } }, '状态更新 ' + state.tick),
        ]),
      ]);
    },
  });

  // ---------- 测量接口（与 React 版同名，壳侧 eval 调用）----------
  window.__bench = {
    ready: false,
    readyAt: 0,
    rafAvailable: false,
    ua: navigator.userAgent,
    js: null,
    reflow: null,
    styleRecalc: null,
    vueUpdate: null,
    scroll: null,
    domCount: 0,
    fatal: '',
  };

  var now = function () { return performance.now(); };

  // 1) 纯 JS 计算（逐字复用 React 版）
  window.__runJs = function () {
    var t0 = now();
    var fib = function (n) { return n < 2 ? n : fib(n - 1) + fib(n - 2); };
    var f = fib(27);
    var t1 = now();
    var toks = 0;
    var t2 = now();
    for (var r = 0; r < 3; r++) {
      for (var i = 0; i < CODE_LINES_DATA.length; i++) { toks += tokenize(CODE_LINES_DATA[i]).length; }
    }
    var t3 = now();
    var arr = [];
    for (var k = 0; k < 20000; k++) { arr.push({ i: k, k: (k * 2654435761) % 100000, s: 'v' + (k % 997) }); }
    arr.sort(function (a, b) { return a.k - b.k || a.s.localeCompare(b.s); });
    var t4 = now();
    window.__bench.js = {
      fibMs: +(t1 - t0).toFixed(2), fibResult: f,
      tokenizeMs: +(t3 - t2).toFixed(2), tokenCount: toks,
      arrayMs: +(t4 - t3).toFixed(2),
      totalMs: +(t4 - t0).toFixed(2),
    };
  };

  // 2) 强制同步布局（逐字复用 React 版）
  window.__runReflow = function (n) {
    var el = document.getElementById('panel');
    var t0 = now();
    var acc = 0;
    for (var i = 0; i < n; i++) {
      el.style.paddingLeft = (i % 7) + 'px';
      el.style.width = (200 + (i % 50)) + 'px';
      acc += el.offsetWidth + el.offsetHeight;
    }
    var t1 = now();
    window.__bench.reflow = { n: n, totalMs: +(t1 - t0).toFixed(2), perOpUs: +(((t1 - t0) * 1000) / n).toFixed(2), acc: acc };
  };

  // 3) 样式重算 + 强制重排（逐字复用 React 版）
  window.__runStyleRecalc = function (n) {
    var el = document.getElementById('bar');
    var t0 = now();
    for (var i = 0; i < n; i++) {
      el.className = 'bar-inner ' + (i % 2 ? 'on' : 'off');
      el.style.left = (i % 11) + 'px';
      void el.offsetWidth;
    }
    var t1 = now();
    window.__bench.styleRecalc = { n: n, totalMs: +(t1 - t0).toFixed(2), perOpUs: +(((t1 - t0) * 1000) / n).toFixed(2) };
  };

  // 4) 滚动路径（逐字复用 React 版）
  window.__runScroll = function (n) {
    var el = document.getElementById('code');
    var t0 = now();
    var acc = 0;
    for (var i = 0; i < n; i++) {
      el.scrollTop = (i * 13) % 2000;
      acc += el.scrollTop;
    }
    var t1 = now();
    window.__bench.scroll = { n: n, totalMs: +(t1 - t0).toFixed(2), perOpUs: +(((t1 - t0) * 1000) / n).toFixed(2), acc: acc };
  };

  // 5) 更新吞吐（Vue 路径）：每批重写 200 行文本 + 更新落地后强制同步布局。
  //    与 React 版 __runReactUpdate 的批次/行数一致；差异在于 Vue 的 flush 是
  //    微任务异步的（nextTick），因此每批以 nextTick 为界，React 侧是同步 flush。
  window.__runVueUpdate = function (batches) {
    var raf = window.requestAnimationFrame || function (cb) { setTimeout(cb, 16); };
    raf(function () {
      var host = document.createElement('div');
      host.id = 'vue-probe';
      document.body.appendChild(host);
      var st = Vue.reactive({ items: [] });
      var probe = Vue.createApp({
        render: function () {
          var rows = [];
          for (var i = 0; i < st.items.length; i++) {
            rows.push(h('div', { key: i, class: 'rrow' }, st.items[i]));
          }
          return h('div', { class: 'rprobe' }, rows);
        },
      });
      probe.mount(host);
      var total = batches || 20;
      var t0 = now();
      var i = 0;
      var batch = function () {
        var cur = i;
        var next = [];
        for (var k = 0; k < 200; k++) { next.push('batch' + cur + '/' + k + '-' + (cur * k % 997)); }
        st.items = next;
        Vue.nextTick(function () {
          void host.offsetHeight;
          if (++i < total) { return batch(); }
          var ms = now() - t0;
          window.__bench.vueUpdate = { batches: total, totalMs: +ms.toFixed(2), perBatchMs: +(ms / total).toFixed(2) };
          probe.unmount();
          host.remove();
        });
      };
      batch();
    });
  };

  // 6) DOM 规模（逐字复用 React 版）
  window.__countDom = function () {
    window.__bench.domCount = document.getElementsByTagName('*').length;
  };

  // ---------- 启动 ----------
  try {
    var t0 = now();
    var vm = app.mount('#root');
    window.__bench.vueMountCallMs = +(now() - t0).toFixed(2);
    window.__vm = vm;

    // rAF 存活 + 频率观测（仅作参考，不参与结论）
    var step = function () {
      state.rafCount++;
      requestAnimationFrame(step);
    };
    if (window.requestAnimationFrame) { requestAnimationFrame(step); }

    // 首屏：等两帧再记录（同 React 版）
    var raf2 = window.requestAnimationFrame || function (cb) { setTimeout(cb, 16); };
    raf2(function () {
      raf2(function () {
        window.__bench.rafAvailable = !!window.requestAnimationFrame;
        window.__bench.readyAt = Math.round(performance.now());
        document.title = 'ready:' + window.__bench.readyAt;
        window.__bench.ready = true;
      });
    });
  } catch (e) {
    window.__bench.fatal = String((e && e.stack) || e);
    document.title = 'fatal';
  }
})();
