// 前置 hook：必须在 React bundle 之前加载（React/scheduler 在模块初始化时就把
// setTimeout 等异步原语捕获进局部变量，之后 hook 无效）。
//
// 用途一（默认）：给异步原语计数 + 给缺 MessageChannel 的引擎装 polyfill，
//                用于判定「静默挂起」断在哪条异步链。
// 用途二：加 ?nopolyfill=1 访问时不装 polyfill，观察**原始**调度路径。
(function () {
  var H = window.__hooks = {
    setTimeout: 0, raf: 0, micro: 0, mcCtor: 0, mcPost: 0, mcDeliver: 0,
    mcPolyfilled: false, hookOwnSetTimeout: false
  };
  var _st = window.setTimeout;
  window.setTimeout = function () { H.setTimeout++; return _st.apply(window, arguments); };

  var _qm = window.queueMicrotask;
  if (typeof _qm === 'function') {
    window.queueMicrotask = function (fn) { H.micro++; return _qm.call(window, fn); };
  }
  var _raf = window.requestAnimationFrame;
  if (typeof _raf === 'function') {
    window.requestAnimationFrame = function (fn) { H.raf++; return _raf.call(window, fn); };
  }

  var wantPolyfill = location.search.indexOf('nopolyfill') === -1;
  if (wantPolyfill && typeof window.MessageChannel === 'undefined') {
    H.mcPolyfilled = true;
    function Port() { this.onmessage = null; this._other = null; this._closed = false; this._listeners = []; }
    Port.prototype.postMessage = function (data) {
      var other = this._other;
      H.mcPost++;
      if (!other || other._closed) return;
      _st.call(window, function () {
        H.mcDeliver++;
        var ev = { data: data, target: other, currentTarget: other };
        try {
          if (typeof other.onmessage === 'function') other.onmessage(ev);
          for (var i = 0; i < other._listeners.length; i++) other._listeners[i](ev);
        } catch (e) {
          window.__polyErr = String((e && e.stack) || e);
        }
      }, 0);
    };
    Port.prototype.start = function () {};
    Port.prototype.close = function () { this._closed = true; };
    Port.prototype.addEventListener = function (t, l) { if (t === 'message' && l) this._listeners.push(l); };
    Port.prototype.removeEventListener = function () {};
    window.MessageChannel = function () {
      H.mcCtor++;
      var a = new Port(), b = new Port();
      a._other = b; b._other = a;
      return { port1: a, port2: b };
    };
    window.MessagePort = Port;
  }
})();
