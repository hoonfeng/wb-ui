// IDE 形态的 React 压测页（判别实验用）。
//
// 设计约束：
//  1. 同一份产物要能在「Rust 壳 + 系统 WebView2」与「wb-ui 自研引擎」两侧加载，
//     因此不依赖任何一侧特有的 API（不用 window.ipc），结果一律写回 window.__bench，
//     由壳去 eval/轮询。
//  2. 指标必须能判别「引擎架构」而非「vsync」：rAF 在 WebView2 上被合成器按
//     60Hz 节流、在自研引擎上可能不受限，直接比帧率没有意义。所以主动测量
//     强制同步布局(reflow)、样式重算(style recalc)、React 更新吞吐与纯 JS 计算。
//  3. 页面形态刻意贴近真实 IDE（左文件树 / 中代码视图 / 右面板 / 底状态栏），
//     代码视图按 CodeMirror 的大文档 DOM 形态构造（每行一个 div + 若干 token span）。

import React, { useEffect, useMemo, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';

// ---------- 规模参数（可被 URL query 覆盖，便于扫规模）----------
const qs = new URLSearchParams(location.search);
const num = (k, d) => {
  const v = parseInt(qs.get(k) || '', 10);
  return Number.isFinite(v) && v > 0 ? v : d;
};
const TREE_ROWS = num('tree', 320);
const CODE_LINES = num('lines', 1800);

// ---------- 文本分词（同时作为 JS 计算负载）----------
const KEYWORDS = new Set(['const', 'let', 'var', 'function', 'return', 'if', 'else', 'for', 'while',
  'class', 'new', 'await', 'async', 'import', 'export', 'from', 'try', 'catch', 'this',
  'null', 'undefined', 'true', 'false', 'typeof', 'instanceof']);
const RE_TOKEN = /(\/\/[^\n]*)|('[^']*'|"[^"]*"|`[^`]*`)|\b(\d+(?:\.\d+)?)\b|\b([A-Za-z_$][\w$]*)\b|([{}()[\];,.:=+\-*/<>!&|?]+)|(\s+)/g;

function tokenize(src) {
  const out = [];
  let m;
  RE_TOKEN.lastIndex = 0;
  while ((m = RE_TOKEN.exec(src)) !== null) {
    let k = 'punct', v = m[0];
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
  const n = i % 7;
  switch (n) {
    case 0: return `const handler${i} = async (event) => { /* 行 ${i} */`;
    case 1: return `  const value${i} = await fetch('/api/node/${i}').then((r) => r.json());`;
    case 2: return `  if (value${i} && value${i}.kind === 'widget') return value${i}.render(${i});`;
    case 3: return `  for (let j = 0; j < ${i % 40}; j++) { total += j * ${i % 13}; }`;
    case 4: return `  return { id: ${i}, label: '节点-${i}', depth: ${i % 9}, collapsed: false };`;
    case 5: return `}` ;
    default: return ``;
  }
}

const CODE_LINES_DATA = Array.from({ length: CODE_LINES }, (_, i) => makeCodeLine(i));
const TOKENS_DATA = CODE_LINES_DATA.map(tokenize);
const TREE_DATA = Array.from({ length: TREE_ROWS }, (_, i) => ({
  id: i,
  name: `node_${i}.ts${i % 3 === 0 ? 'x' : ''}`,
  depth: i % 6,
  dir: i % 4 === 0,
}));

// ---------- 组件 ----------
function FileTree() {
  return React.createElement('div', { className: 'tree', id: 'tree' },
    React.createElement('div', { className: 'tree-head' }, '资源管理器'),
    ...TREE_DATA.map((n) => React.createElement('div', {
      key: n.id,
      className: 'tree-row' + (n.dir ? ' dir' : ''),
      style: { paddingLeft: (8 + n.depth * 14) + 'px' },
    }, React.createElement('span', { className: 'ico' }, n.dir ? '▸' : '·'), n.name)));
}

function CodeView() {
  return React.createElement('div', { className: 'code', id: 'code' },
    ...TOKENS_DATA.map((toks, i) => React.createElement('div', { className: 'line', key: i },
      React.createElement('span', { className: 'ln' }, String(i + 1)),
      ...toks.map((t, j) => React.createElement('span', { key: j, className: 'tok t-' + t[0] }, t[1])))));
}

function Panel({ tick }) {
  return React.createElement('div', { className: 'panel', id: 'panel' },
    React.createElement('div', { className: 'panel-head' }, '属性'),
    React.createElement('div', { className: 'card' },
      React.createElement('div', { className: 'card-title' }, '构建状态'),
      React.createElement('div', { className: 'bar-outer' },
        React.createElement('div', { className: 'bar-inner', id: 'bar', style: { width: ((tick * 7) % 100) + '%' } }))),
    React.createElement('div', { className: 'card grad' },
      React.createElement('div', { className: 'card-title' }, '渐变 / 圆角 / 阴影'),
      React.createElement('div', { className: 'chip-row' },
        ...['A', 'B', 'C', 'D'].map((c) => React.createElement('span', { key: c, className: 'chip' }, c)))),
    React.createElement('div', { className: 'card' },
      React.createElement('div', { className: 'card-title' }, '计数 ' + tick)));
}

function App() {
  const [tick, setTick] = useState(0);
  const rafRef = useRef(0);
  const rafCount = useRef(0);

  // rAF 存活 + 频率观测（仅作参考信息，不参与结论）
  useEffect(() => {
    let stop = false;
    const step = () => {
      if (stop) return;
      rafCount.current++;
      rafRef.current = requestAnimationFrame(step);
    };
    rafRef.current = requestAnimationFrame(step);
    return () => { stop = true; cancelAnimationFrame(rafRef.current); };
  }, []);

  // 首屏：commit 后等两帧（确保已绘制）再记录
  useEffect(() => {
    const raf = window.requestAnimationFrame || ((cb) => setTimeout(cb, 16));
    raf(() => raf(() => {
      window.__bench.rafAvailable = !!window.requestAnimationFrame;
      window.__bench.readyAt = Math.round(performance.now());
      document.title = 'ready:' + window.__bench.readyAt;
      window.__bench.ready = true;
    }));
  }, []);

  useEffect(() => {
    window.__bench.rafTicks = rafCount.current;
  });

  return React.createElement('div', { className: 'app', id: 'app' },
    React.createElement('div', { className: 'titlebar' }, 'IDE 形态压测页 · React 19（tree=' +
      TREE_ROWS + ' lines=' + CODE_LINES + '）'),
    React.createElement('div', { className: 'body' },
      React.createElement(FileTree),
      React.createElement(CodeView),
      React.createElement(Panel, { tick }),
    ),
    React.createElement('div', { className: 'statusbar' },
      React.createElement('span', null, '就绪'),
      React.createElement('span', null, 'rAF ' + rafCount.current),
      React.createElement('span', { className: 'spacer' }),
      React.createElement('button', {
        className: 'btn', id: 'btn',
        onClick: () => setTick((t) => t + 1),
      }, '状态更新 ' + tick),
    ));
}

// ---------- 测量接口（壳侧 eval 调用，结果写回 window.__bench）----------
window.__bench = {
  ready: false,
  readyAt: 0,
  rafAvailable: false,
  ua: navigator.userAgent,
  js: null,
  reflow: null,
  styleRecalc: null,
  reactUpdate: null,
  scroll: null,
  domCount: 0,
};

const now = () => performance.now();

// 1) 纯 JS 计算：递归 fib + 正则分词 + 对象数组操作（与语言能力相关）
window.__runJs = function () {
  const t0 = now();
  // fib
  const fib = (n) => (n < 2 ? n : fib(n - 1) + fib(n - 2));
  const f = fib(27);
  const t1 = now();
  // 正则分词：把全部代码行分词 3 遍
  let toks = 0;
  const t2 = now();
  for (let r = 0; r < 3; r++) {
    for (let i = 0; i < CODE_LINES_DATA.length; i++) toks += tokenize(CODE_LINES_DATA[i]).length;
  }
  const t3 = now();
  // 对象/数组：构建 20000 项并按字段排序
  const arr = [];
  for (let i = 0; i < 20000; i++) arr.push({ i, k: (i * 2654435761) % 100000, s: 'v' + (i % 997) });
  arr.sort((a, b) => a.k - b.k || a.s.localeCompare(b.s));
  const t4 = now();
  window.__bench.js = {
    fibMs: +(t1 - t0).toFixed(2), fibResult: f,
    tokenizeMs: +(t3 - t2).toFixed(2), tokenCount: toks,
    arrayMs: +(t4 - t3).toFixed(2),
    totalMs: +(t4 - t0).toFixed(2),
  };
};

// 2) 强制同步布局：写样式后读 offsetWidth，逼引擎每次重新 layout（架构敏感）
window.__runReflow = function (n) {
  const el = document.getElementById('panel');
  const t0 = now();
  let acc = 0;
  for (let i = 0; i < n; i++) {
    el.style.paddingLeft = (i % 7) + 'px';
    el.style.width = (200 + (i % 50)) + 'px';
    acc += el.offsetWidth + el.offsetHeight;
  }
  const t1 = now();
  window.__bench.reflow = { n, totalMs: +(t1 - t0).toFixed(2), perOpUs: +(((t1 - t0) * 1000) / n).toFixed(2), acc };
};

// 3) 样式重算 + 绘制失效：改 class 触发 recalc（+ 强制重排）
window.__runStyleRecalc = function (n) {
  const el = document.getElementById('bar');
  const t0 = now();
  for (let i = 0; i < n; i++) {
    el.className = 'bar-inner ' + (i % 2 ? 'on' : 'off');
    el.style.left = (i % 11) + 'px';
    void el.offsetWidth;
  }
  const t1 = now();
  window.__bench.styleRecalc = { n, totalMs: +(t1 - t0).toFixed(2), perOpUs: +(((t1 - t0) * 1000) / n).toFixed(2) };
};

// 4) 滚动容器：设置 scrollTop 后强制布局（模拟滚动路径成本）
window.__runScroll = function (n) {
  const el = document.getElementById('code');
  const t0 = now();
  let acc = 0;
  for (let i = 0; i < n; i++) {
    el.scrollTop = (i * 13) % 2000;
    acc += el.scrollTop;
  }
  const t1 = now();
  window.__bench.scroll = { n, totalMs: +(t1 - t0).toFixed(2), perOpUs: +(((t1 - t0) * 1000) / n).toFixed(2), acc };
};

// 5) React 更新吞吐：连续 setState 更新 200 个树行文本 + 每个批次强制 reflow
function RunReactUpdate(batches, cb) {
  const host = document.createElement('div');
  host.id = 'react-probe';
  document.body.appendChild(host);
  const root = createRoot(host);
  let done = 0;
  const t0 = now();
  const render = (i) => React.createElement('div', { className: 'rprobe' },
    ...Array.from({ length: 200 }, (_, k) => React.createElement('div', { key: k, className: 'rrow' },
      'batch' + i + '/' + k + '-' + (i * k % 997))));
  const step = () => {
    root.render(render(done));
    // 强制读一次布局，逼同步 commit 落地
    void host.offsetHeight;
    if (++done < batches) return step();
    const t1 = now();
    window.__bench.reactUpdate = { batches, totalMs: +(t1 - t0).toFixed(2), perBatchMs: +((t1 - t0) / batches).toFixed(2) };
    root.unmount();
    host.remove();
    if (cb) cb();
  };
  step();
}
window.__runReactUpdate = function (batches) {
  // 用 rAF 让 React 的并发调度有机会落地，随后同步推进
  const raf = window.requestAnimationFrame || ((cb) => setTimeout(cb, 16));
  raf(() => RunReactUpdate(batches || 20));
};

// 6) DOM 规模（用于解释两侧差异）
window.__countDom = function () {
  window.__bench.domCount = document.getElementsByTagName('*').length;
};

// ---------- 启动 ----------
try {
  const t0 = performance.now();
  const root = createRoot(document.getElementById('root'));
  root.render(React.createElement(App));
  window.__bench.reactMountCallMs = +(performance.now() - t0).toFixed(2);
} catch (e) {
  window.__bench.fatal = String((e && e.stack) || e);
  document.title = 'fatal';
}
