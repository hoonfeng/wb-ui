// JS 侧 WebSocket shim：把浏览器 API 面补齐，页面代码零改动。
//
// 注入时机：读入 index.html 后插到 <head> 最前面（见 local.go），因此先于
// 任何页面脚本（含 Vue bundle）执行。Go 侧的 __wbwsOpen/__wbwsSend/__wbwsClose
// 由 wsbridge.go 在装配前注册到全局。
//
// 事件模型与浏览器一致：
//
//	ws.onopen / onmessage / onerror / onclose + addEventListener
//	readyState 0=CONNECTING 1=OPEN 2=CLOSING 3=CLOSED
//	ev.data 为字符串（终端 PTY 输出由宿主按字符串推送；前端已按此处理）
package main

// wsShimJS 注入到页面 <head> 最前的脚本。
const wsShimJS = `(function () {
  if (typeof window === 'undefined') return;
  var CONNECTING = 0, OPEN = 1, CLOSING = 2, CLOSED = 3;
  var registry = {};
  var seq = 0;
  // 宿主函数挂在 engine 的 "go" 命名空间（bindings.RegisterGoFunction 的约定），
  // 不是全局裸名；保留 window 兜底便于将来改动。
  function hostFn(name) {
    if (typeof go !== 'undefined' && go && typeof go[name] === 'function') return go[name];
    if (typeof window[name] === 'function') return window[name];
    return null;
  }

  function dispatchTo(ws, type, ev) {
    var h = ws['on' + type];
    if (typeof h === 'function') {
      try { h.call(ws, ev); } catch (e) { /* 回调异常不影响其他监听器 */ }
    }
    var list = ws._listeners[type] || [];
    for (var i = 0; i < list.length; i++) {
      try { list[i].call(ws, ev); } catch (e2) { /* 同上 */ }
    }
  }

  function WSBridgeSocket(url) {
    if (!(this instanceof WSBridgeSocket)) {
      throw new TypeError("Failed to construct 'WebSocket': use new");
    }
    this.url = String(url);
    this.protocol = '';
    this.extensions = '';
    this.bufferedAmount = 0;
    this.binaryType = 'blob';
    this.readyState = CONNECTING;
    this.onopen = null;
    this.onmessage = null;
    this.onerror = null;
    this.onclose = null;
    this._listeners = {};
    this._id = ++seq;
    registry[this._id] = this;
    try {
      var o = hostFn('__wbwsOpen');
      if (!o) throw new Error('宿主未注册 __wbwsOpen');
      o(this._id, this.url);
    } catch (e) {
      this.readyState = CLOSED;
      dispatchTo(this, 'error', { type: 'error', message: String(e && e.message || e), target: this });
    }
  }

  WSBridgeSocket.prototype.send = function (data) {
    if (this.readyState !== OPEN) return;
    try {
      var s = hostFn('__wbwsSend');
      if (s) s(this._id, (typeof data === 'string') ? data : String(data));
    } catch (e) { /* 发送失败按浏览器语义静默丢弃 */ }
  };

  WSBridgeSocket.prototype.close = function (code, reason) {
    if (this.readyState === CLOSED || this.readyState === CLOSING) return;
    this.readyState = CLOSING;
    try {
      var c = hostFn('__wbwsClose');
      if (c) c(this._id);
    } catch (e) { /* 忽略 */ }
  };

  WSBridgeSocket.prototype.addEventListener = function (type, fn) {
    if (typeof fn !== 'function') return;
    if (!this._listeners[type]) this._listeners[type] = [];
    this._listeners[type].push(fn);
  };

  WSBridgeSocket.prototype.removeEventListener = function (type, fn) {
    var list = this._listeners[type];
    if (!list) return;
    var out = [];
    for (var i = 0; i < list.length; i++) {
      if (list[i] !== fn) out.push(list[i]);
    }
    this._listeners[type] = out;
  };

  WSBridgeSocket.CONNECTING = CONNECTING;
  WSBridgeSocket.OPEN = OPEN;
  WSBridgeSocket.CLOSING = CLOSING;
  WSBridgeSocket.CLOSED = CLOSED;
  WSBridgeSocket.prototype.CONNECTING = CONNECTING;
  WSBridgeSocket.prototype.OPEN = OPEN;
  WSBridgeSocket.prototype.CLOSING = CLOSING;
  WSBridgeSocket.prototype.CLOSED = CLOSED;

  // 宿主派发入口（Go 侧 EvalJS 调用）：open / message / error / close。
  window.__wbwsDispatch = function (id, type, payload) {
    var ws = registry[id];
    if (!ws) return;
    if (type === 'open') {
      ws.readyState = OPEN;
      dispatchTo(ws, 'open', { type: 'open', target: ws });
      return;
    }
    if (type === 'message') {
      dispatchTo(ws, 'message', { type: 'message', data: payload, target: ws });
      return;
    }
    if (type === 'error') {
      dispatchTo(ws, 'error', { type: 'error', message: payload, target: ws });
      return;
    }
    if (type === 'close') {
      if (ws.readyState === CLOSED) return;
      ws.readyState = CLOSED;
      delete registry[id];
      dispatchTo(ws, 'close', { type: 'close', code: 1000, reason: '', wasClean: true, target: ws });
    }
  };

  window.WebSocket = WSBridgeSocket;
})();`
