// 最小 React 复现：只挂一个 div，用于判定「引擎能否跑 React 19」本身。
// 暴露尽量多的中间状态，便于在引擎侧读回定位卡点。
import React from 'react';
import { createRoot } from 'react-dom/client';

window.__min = { phase: 'boot', logs: [] };

// 捕获异步异常（React 的调度回调若抛错，会被引擎吞掉而 console 无输出）
window.__minErrors = [];
window.addEventListener('error', function (e) {
  window.__minErrors.push('error-event: ' + String((e && (e.message || e.error)) || e));
});
window.onerror = function (m, s, l, c, err) {
  window.__minErrors.push('onerror: ' + String(m) + ' @' + l + ':' + c +
    (err && err.stack ? ' | ' + String(err.stack).slice(0, 400) : ''));
  return false;
};

try {
  const el = document.getElementById('root');
  window.__min.rootElFound = !!el;
  const root = createRoot(el);
  window.__min.root = root;
  window.__min.hasInternalRoot = !!(root && (root._internalRoot || root.__internalRoot));
  window.__min.phase = 'createRoot-ok';

  root.render(React.createElement('div', { id: 'hello', className: 'box' }, 'hello-react'));
  window.__min.phase = 'render-called';

  // 兜底观测：直接读 DOM
  window.__minCheck = function () {
    const h = document.getElementById('hello');
    return JSON.stringify({
      phase: window.__min.phase,
      rootElFound: window.__min.rootElFound,
      hasInternalRoot: window.__min.hasInternalRoot,
      helloExists: !!h,
      helloText: h ? h.textContent : null,
      rootHTML: (document.getElementById('root') || {}).innerHTML || '',
      errors: window.__minErrors,
      rootKeys: (function () {
        try {
          return Object.getOwnPropertyNames(document.getElementById('root')).filter(function (k) {
            return k.indexOf('__react') === 0;
          });
        } catch (e) { return 'ERR:' + e; }
      })(),
    });
  };
} catch (e) {
  window.__min.phase = 'throw';
  window.__min.err = String((e && e.stack) || e);
}
