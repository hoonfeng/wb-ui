// AI-PS 前端「拦截层」（页面侧）—— 把每一次请求与界面事件显式留痕并上报宿主（Go 拦截器）。
//
// 分工（测试链路的核心）：
//   · 请求：wb-ui 宿主装配时已注入 bridge SDK，它把「已注册路由」的 fetch 直接交给 Go
//     handler（不经网络）。本文件在它之上再包一层 **观测**：记录每次请求的方法/URL/状态/耗时。
//   · 事件：界面事件（工具选择、图层选择、回车提交、按钮 emit）先经 postEvent() 上报
//     POST /api/events → Go 拦截器落日志 → 桩响应。
//
// 兼容性：产物要跑在 wb-ui 的 goja 引擎里，这里只用 ES5/ES2015 语法与安全 API。

var _state = (typeof window !== 'undefined')
  ? (window.__aips = window.__aips || { requests: [], events: [], ready: false })
  : { requests: [], events: [], ready: false }

function nowMs() {
  return (typeof performance !== 'undefined' && performance.now) ? Math.round(performance.now()) : Date.now()
}

export function logRequest(entry) {
  _state.requests.push(entry)
  console.log('[ai-ps][req] ' + entry.method + ' ' + entry.url + ' -> ' + entry.status + ' (' + entry.ms + 'ms)')
}

export function logEvent(name, payload) {
  _state.events.push({ name: name, payload: payload || {}, at: nowMs() })
  console.log('[ai-ps][evt] ' + name + ' ' + JSON.stringify(payload || {}))
}

// installFetchSpy 包装 window.fetch：只记录、不改路由（路由仍由宿主 bridge 决定）。
export function installFetchSpy() {
  if (typeof window === 'undefined' || !window.fetch) return
  var orig = window.fetch
  window.fetch = function (url, options) {
    var started = Date.now()
    var method = (options && options.method) ? String(options.method).toUpperCase() : 'GET'
    return orig.call(this, url, options).then(function (res) {
      logRequest({ url: String(url), method: method, status: res.status, ms: Date.now() - started })
      return res
    }, function (err) {
      logRequest({
        url: String(url), method: method, status: 0, ms: Date.now() - started,
        error: String(err && err.message ? err.message : err)
      })
      throw err
    })
  }
}

// postEvent 上报界面事件（Go 拦截器记录 + 桩响应）。
export function postEvent(name, payload) {
  logEvent(name, payload)
  return fetch('/api/events', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: name, payload: payload || {} })
  }).then(function (res) { return res.json() }, function () { return {} })
}

// requestJSON 向桩接口发请求（POST + JSON）。
export function requestJSON(path, body) {
  return fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body || {})
  }).then(function (res) { return res.json() })
}

export function pageState() { return _state }
