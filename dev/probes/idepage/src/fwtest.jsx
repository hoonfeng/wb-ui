// React 端到端夹具入口：与 vue-app.html 同构的 UI（同一套 class/尺寸），
// 用于对比「Vue 3.5 / React 19」在 wb-ui 引擎上的挂载、更新与样式渲染。
// 由 dev/fixtures/framework/build-react.mjs 用 esbuild 打包成 iife。
import React, { useState, useMemo } from 'react';
import { createRoot } from 'react-dom/client';

const ITEMS0 = [
  { id: 1, text: 'alpha 项目', tag: 'new', on: false, color: '#e53935' },
  { id: 2, text: 'beta 项目', tag: '', on: true, color: '#1e88e5' },
  { id: 3, text: 'gamma 项目', tag: 'wip', on: false, color: '#43a047' },
];

function Child({ label }) {
  return <div className="child">child: {label}</div>;
}

function App() {
  const [items, setItems] = useState(ITEMS0);
  const [open, setOpen] = useState(false);
  const upper = useMemo(() => 'React 应用 / 共 ' + items.length + ' 项', [items]);

  window.__setOpen = setOpen;
  window.__addItem = function () {
    setItems(function (prev) {
      const n = prev.length + 1;
      return prev.concat([{ id: n, text: 'delta ' + n, tag: '', on: false, color: '#8e24aa' }]);
    });
  };
  window.__state = function () {
    const rows = document.querySelectorAll('.row');
    const d = document.querySelector('.drop');
    return JSON.stringify({
      rows: rows.length,
      dropVisible: d ? (d.getBoundingClientRect().width > 0 && d.getBoundingClientRect().height > 0) : false,
      box: (document.querySelector('.box') || {}).textContent || '',
      child: (document.querySelector('.child') || {}).textContent || '',
      lastRow: rows.length ? rows[rows.length - 1].textContent : '',
    });
  };

  return (
    <div className="app">
      <div className="bar">
        <span className="title">React 应用</span>
        <span className="count">{items.length}</span>
      </div>
      <ul className="list">
        {items.map(function (it, i) {
          return (
            <li key={it.id} className={'row' + (it.on ? ' active' : '')} style={{ borderLeftColor: it.color }}>
              <span className="idx">{i + 1}</span>
              <span className="txt">{it.text}</span>
              {it.tag ? <span className="tag">{it.tag}</span> : null}
            </li>
          );
        })}
      </ul>
      <div className="panel">panel z-index:1</div>
      {open ? (
        <div className="drop">
          <div className="mi">menu-1</div>
          <div className="mi">menu-2</div>
          <div className="mi">menu-3</div>
        </div>
      ) : null}
      <div className="btn" onClick={function () { setOpen(function (v) { return !v; }); }}>toggle</div>
      <div className="box">{upper}</div>
      <Child label="React 应用" />
    </div>
  );
}

window.__errors = [];
window.addEventListener('error', function (e) {
  window.__errors.push(String((e && (e.message || e.error)) || e));
});

const el = document.getElementById('root');
window.__rootFound = !!el;
createRoot(el).render(<App />);
window.__renderCalled = true;
