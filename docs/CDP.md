# CDP 调试协议（wb-ui 引擎内置服务端）

> 状态：**已实装**（2026-10-07，对应实现路径主线 B 的 B0/B1 与 **B2 / S2**——
> Runtime `objectId` 句柄表、DOM 编辑与查询扩展、CSS 域、console 分级）。
> 规划与分期见 [docs/implementation-path.md](implementation-path.md) §3；本文件是**使用说明**。

wb-ui 引擎内置了 Chrome DevTools Protocol（CDP）服务端：外部工具（自家测试工装、
Chrome DevTools 前端、Puppeteer 子集）可以用**统一协议**驱动引擎——求值、截图、
派发输入、读 DOM 树。它替代了此前散落的一次性 API 调用（`cmd/psai -verify`、
`__devtools` 注入对象）。

---

## 1. 快速开始

```bash
# 无头自检 + 打开调试端口（默认关闭；端口 0 = 不监听、零开销）
go run ./cmd/psai -verify -png "" -remote-debugging-port 9222

# 窗口模式同样支持（同一开关）
go run ./cmd/psai -remote-debugging-port 9222

# 外部工具随后可以：
curl http://127.0.0.1:9222/json/version
curl http://127.0.0.1:9222/json/list
```

启动时 stderr 会打印与 Chrome 同格式的一行，外部脚本据此发现入口：

```
DevTools listening on ws://127.0.0.1:9222/devtools/browser/<host>-<pid>
```

> ★ **只绑 `127.0.0.1`**：调试协议等于把「执行任意 JS + 读页面内容」暴露给连接方，
> 绝不能监听 `0.0.0.0`。服务端还会拒绝非回环来源的连接（双重保险）。
>
> ★ Windows 上端口若处于 `TIME_WAIT`（刚关过一次宿主），立即重绑会失败并给出提示
> ——换端口即可。

---

## 2. 端点与入口

| 端点 | 说明 |
|---|---|
| `GET /json/version` | 浏览器级信息：`Browser`、`Protocol-Version`、`User-Agent`、`webSocketDebuggerUrl` |
| `GET /json/list`（含 `/json`） | 页面目标列表：`id`/`type`/`title`/`url`/`webSocketDebuggerUrl` |
| `GET /json/protocol` | **本实现**的支持面描述（不是 Chrome 全量协议清单）——据此判断可用域 |
| `PUT /json/new` | 明确回 `501`：引擎是单页面宿主（一个 WebView = 一个页面），没有「新建标签页」语义 |
| `WS /devtools/browser/<id>` | 浏览器级入口：先 `Target.getTargets` → `Target.attachToTarget{flatten:true}` 拿 `sessionId` |
| `WS /devtools/page/<targetId>` | 页面级入口：**不带 `sessionId` 的消息直接落到该页面**（S1 工装的最短路径） |

消息格式就是 Chrome 的 wire 格式：

```jsonc
// 客户端 → 服务端
{"id":1,"method":"Runtime.evaluate","params":{"expression":"1+1","returnByValue":true},"sessionId":"…"}
// 服务端 → 客户端（成功）
{"id":1,"result":{"result":{"type":"number","value":2}}}
// 服务端 → 客户端（失败）
{"id":1,"error":{"code":-32601,"message":"cdp: 未实现的方法 Network.enable"}}
// 服务端 → 客户端（事件，无 id）
{"method":"Runtime.consoleAPICalled","params":{…},"sessionId":"…"}
```

---

## 3. 支持的域与方法

| 域 | 方法 | 映射到引擎 |
|---|---|---|
| **Browser** | `getVersion`、`getBrowserCommandLine` | 静态信息 |
| **Target** | `getTargets`、`getTargetInfo`、`attachToTarget`(flatten)、`detachFromTarget`、`setDiscoverTargets`、`activateTarget` | Target 注册表 |
| **Runtime** | `enable`、`evaluate`、`callFunctionOn`、`getProperties`、`releaseObject`、`releaseObjectGroup` | `WebView.EvalJS` + 页面侧 `objectId` 句柄表（`window.__cdpHandles`） |
| **Page** | `enable`、`navigate`、`reload`、`captureScreenshot`、`getLayoutMetrics` | `LoadURL` / `Render()` / `Resize` |
| **DOM** | `enable`、`getDocument`、`describeNode`、`querySelector`、`querySelectorAll`、`getAttributes`、`getOuterHTML`、`getBoxModel`、`resolveNode`、`setAttributeValue`、`removeAttribute`、`requestChildNodes` | 页面侧 DOM 快照 + `getBoundingClientRect` + `setAttribute/removeAttribute` |
| **CSS** | `enable`、`disable`、`getComputedStyleForNode`、`getInlineStylesForNode`、`getMatchedStylesForNode` | `getComputedStyle` / `el.style` / 遍历 `document.styleSheets` 匹配 |
| **Input** | `dispatchMouseEvent`、`dispatchKeyEvent` | `HandleMouseButton/Move/Wheel`、`FormFocus.KeyInput/CharInput` |
| **Emulation** | `setDeviceMetricsOverride`、`clearDeviceMetricsOverride` | `WebView.Resize` |
| **Log** | `enable`、`disable`、`clear` | 控制台增量 |
| 事件 | `Runtime.consoleAPICalled`、`Log.entryAdded`、`Page.loadEventFired` | 宿主增量缓冲轮询（150ms） |

未列出的方法一律回 `-32601`（方法不存在）——客户端能明确区分「这台引擎没实现」与
「调用出错」，而不是收到 500 或空结果。

### 语义细节（与 Chrome 的差异，都是已知且有意的）

| 项 | 本实现 | 原因 |
|---|---|---|
| `Target.attachToTarget` | 只支持 `flatten:true` | 非 flatten 要为每个会话开独立隧道 |
| `Runtime.evaluate` 的 `awaitPromise` | 忽略（同步求值） | 引擎求值同步，Promise 由事件循环推进 |
| `objectId` 句柄 | **提供**（形如 `cdp:3`），存在**页面侧** `window.__cdpHandles` | 句柄与 JS 值同生命周期；随页面导航失效（见 TECH_DEBT） |
| `Runtime.getProperties` | 只回**自身**属性（无原型链，`internalProperties` 恒空） | 引擎侧不做对象反射导出 |
| `CSS.getMatchedStylesForNode` | 有选择器与属性，**无 specificity、无源码行号** | 在页面上遍历样式表 + `querySelectorAll` 判定命中（见 TECH_DEBT） |
| `CSS` 的样式编辑（`setStyleSheetText` 等） | 未实现（回 `-32601`） | 属性编辑走 `DOM.setAttributeValue` 可用 |
| `Overlay.highlightNode` | 未实现（回 `-32601`） | 引擎没有「调试高亮」这层绘制 |
| 脚本抛错 | **成功的响应 + `exceptionDetails`** | CDP 语义：普通脚本错误不是协议故障 |
| `Page.captureScreenshot` 的 `format` | 只支持 `png`（其他值明确报错） | 引擎只有 PNG 编码路径 |
| `Emulation…deviceScaleFactor` | 只接受 `1`（其他值报错） | 引擎无 DSF/移动端仿真 |
| 控制台级别 | **按级别保真**（`error`/`warn`/`info`/`debug`/`log`） | 引擎日志缓冲带级别（`jsc.BufferLogger.Entries`）；时间戳是事件泵取走的时刻、不带堆栈 |
| `Input` 中键点击 | 明确报错 | 引擎鼠标管线只处理左键与右键 |
| `DOM.getDocument` | 节点上限 4000（防大页面把整棵树拉进内存） | 工具场景够用 |

---

## 4. 外部客户端示例

### curl（判据 1）

```bash
curl -s http://127.0.0.1:9222/json/version
curl -s http://127.0.0.1:9222/json/list
```

### Python（标准库手写 WS，判据 2/3/4）

仓库里有一份只依赖 Python 标准库的探针：`_temp/cdp_probe.py`（本机验证产物，
不入库）。它完成握手（含 `Sec-WebSocket-Accept` 校验）、掩码文本帧收发，然后：

```
[py] Runtime.evaluate 1+1 -> {"result": {"type": "number", "value": 2}}
[py] Runtime.evaluate document.title -> {"result": {"type": "string", "value": "…"}}
[py] DOM.querySelector('.d-ai-prompt') -> 1
[py] DOM.getBoxModel -> width=280 height=36
[py] Page.captureScreenshot -> base64 长度 70908（PNG 头 89504e470d0a1a0a）
```

### 自家工装（Go，S1 场景样板）

`cmd/psai/devtools_selftest.go` 是「用 CDP 驱动引擎」的参考实现：起服务后，用真实
HTTP + WebSocket 客户端连自己，跑完 7 条验收判据。

---

## 5. 验收判据与实测结果（2026-10-07）

| # | 判据 | 实测 |
|---|---|---|
| 1 | `/json/version` 含 `Browser`/`webSocketDebuggerUrl`；`/json/list` 含页面 target | ✓（curl 与自检双证） |
| 2 | WS 上 `Runtime.evaluate` `1+1` → `result.value == 2` | ✓（Go 自检 + 外部 Python 客户端） |
| 3 | `Runtime.evaluate` 与 `wv.EvalJS` 结果一致 | ✓（`document.title` 一致） |
| 4 | `Page.captureScreenshot` 解码 PNG 与 `wv.Render()` 逐像素一致 | ✓（1296000 个不透明像素全等） |
| 5 | `Input.dispatchMouseEvent` 派发后页面命中对象一致 | ✓（等价形式：点击 → 目标元素 click 计数 = 1，页面向宿主发出了 `tool.select`） |
| 6 | `DOM.getDocument` 节点数与页面侧计数一致 | ✓（155 = 155）；`getBoxModel` body 1440×900 |
| 7 | 目标端口默认关闭；开启时只监听 loopback | ✓（`netstat -ano`：`TCP 127.0.0.1:9345 0.0.0.0:0 LISTENING`） |
| 8 | Runtime 句柄表：对象求值回 `objectId`；`getProperties` 列出属性；`callFunctionOn(objectId)` 以该对象为 `this`；`releaseObject` 不报错 | ✓（`objectId=cdp:0`；列出 `a`/`b`；`this.b` → 「两」） |
| 9 | DOM 扩展：`querySelectorAll`/`getAttributes`/`getOuterHTML`/`resolveNode` 可用 | ✓（`div` → 36 个节点；首节点 attributes 4 项、outerHTML 11937 字节、resolveNode 回 objectId） |
| 10 | DOM 编辑：`setAttributeValue` 落到页面、`removeAttribute` 让属性消失 | ✓（页面 `getAttribute` 读到 `42`；删除后不再读到） |
| 11 | CSS 域：computed / inline / matched 三样都能取 | ✓（computedStyle 26 项、inlineStyle 2 项、matchedCSSRules 1 条 = 注入的规则） |
| 12 | console 分级：`console.error/warn` 以 `error`/`warn` 推 `Log.entryAdded` | ✓（先推平 enable 后的历史存量，再按探针文本等事件） |
| 附 | 未实现方法回 `-32601` | ✓ |
| 附 | `Input.dispatchKeyEvent` 输入 3 个字符 → 输入框 value = `"cdp"` | ✓ |

复现：

```bash
go build -o _temp/psai.exe ./cmd/psai
./_temp/psai.exe -verify -png "" -remote-debugging-port 9355   # 13 项 ✓ / 0 项 ✗
```

---

## 6. 实现结构

```
engine/devtools/cdp/          协议服务端（纯 Go，不依赖宿主）
  ├── ws/                     RFC6455 子集：握手、掩码文本帧、分片、ping/pong/close
  ├── adapter.go              Adapter 接口（宿主实现）+ 空实现 + 数据结构
  ├── protocol.go             wire 结构、JSON-RPC 错误码
  ├── server.go               HTTP 端点、WS 升级、目标/会话注册表、事件泵
  └── domains.go              各域方法分派与实现
app/cdp.go                    适配层：域方法 → webkit.WebView（唯一出现宿主依赖的地方）
app/mediaprobe.go             媒体元数据探测（主线 A0）
cmd/psai/devtools_selftest.go 端到端自检（S1 样板）
```

分层要点：`engine/` 不反向依赖 `webkit/`、`app/` —— 协议层只认 `Adapter` 接口，
宿主依赖集中在 `app/cdp.go`。

---

## 7. 不承诺的范围

- **Playwright `connect_over_cdp`**：需要 `Network`/`Page`/`Runtime` 全套与会话语义
  细节，成本远超收益（docs/implementation-path.md §3.1 已列为「不承诺」）。
- **Chrome DevTools 前端完整可用**：Elements 面板依赖 `CSS.*` 与 `objectId` 句柄，
  属 S2 范围（P2 期）。
- **多标签页 / Worker target**：当前一个 WebView 一个 target（Worker target 是后续项）。
