// AI-PS 入口：引入设计令牌 CSS、补齐 goja 缺失的内置方法、装上「请求/事件」拦截层，
// 再把设计生成的界面挂到 #app。
import '../tokens.css'
import { createApp } from 'vue'
import App from './App.vue'
// 自适应覆盖层：必须排在设计产物（App.vue → vue/Main.vue 的 scoped 样式）之后，
// 使其成为同一份 CSS 里的最后一段（双保险：顺序 + #app 提特异性）。
import './responsive.css'
import { installFetchSpy, postEvent, logEvent, pageState } from './bridge.js'

// ── goja 兼容补齐（与 examples/vue_load_test 的实测经验一致：引擎缺这些内置方法）──
if (!Object.getOwnPropertyNames) {
  Object.getOwnPropertyNames = function (o) { var k = []; for (var n in o) k.push(n); return k }
}
if (!Object.fromEntries) {
  Object.fromEntries = function (e) {
    var r = {}
    for (var i = 0; e && i < e.length; i++) { if (e[i]) r[e[i][0]] = e[i][1] }
    return r
  }
}
if (!Object.isExtensible) {
  Object.isExtensible = function () { return true }
}
if (!Array.prototype.flatMap) {
  Array.prototype.flatMap = function (f) {
    var r = []
    for (var i = 0; i < this.length; i++) {
      var v = f(this[i], i, this)
      if (v && typeof v.length === 'number') { for (var j = 0; j < v.length; j++) r.push(v[j]) } else { r.push(v) }
    }
    return r
  }
}
if (!Array.prototype.at) {
  Array.prototype.at = function (i) {
    var n = Number(i); if (isNaN(n)) n = 0
    var l = this.length; n = n >= 0 ? n : l + n
    return (n < 0 || n >= l) ? undefined : this[n]
  }
}
if (!String.prototype.padStart) {
  String.prototype.padStart = function (len, pad) {
    var s = String(this); pad = pad === undefined ? ' ' : String(pad)
    while (s.length < len && pad.length) s = pad + s
    return s.length > len ? s.slice(s.length - len) : s
  }
}

// ── 请求拦截层（观测）：记录每一次 fetch ──────────────────────────────
installFetchSpy()

// ── 事件拦截层（页面侧）：界面事件 → 上报宿主 Go（POST /api/events）──
function childIndex(el) {
  var i = 0
  while (el && el.previousElementSibling) { el = el.previousElementSibling; i++ }
  return i
}

document.addEventListener('click', function (ev) {
  var t = ev.target
  if (!t || !t.closest) return
  var tool = t.closest('.d-main_toolstrip > *')
  if (tool) {
    postEvent('tool.select', { index: childIndex(tool) })
    return
  }
  var layer = t.closest('.d-layers-list > *')
  if (layer) {
    postEvent('layer.select', { index: childIndex(layer) })
  }
}, true)

document.addEventListener('keydown', function (ev) {
  var t = ev.target
  if (!t || ev.key !== 'Enter') return
  if (String(t.className || '').indexOf('d-ai-prompt') >= 0) {
    postEvent('ai.prompt.submit', { prompt: String(t.value || '') })
  }
})

// ── 挂载 ────────────────────────────────────────────────────────────
var app = createApp(App)
app.mount('#app')

pageState().ready = true
// 只调 postEvent（其内部已 logEvent）——重复调用会在事件流里留下两条 page.ready
postEvent('page.ready', { mounted: true })
