# gouide 页面加载模式（IDE 的 GUI 版本）

`cmd/gouide` 是 PairCode IDE 的桌面壳：wb-ui 提供原生窗口与渲染，gou-ide 内核
以后端子进程运行（默认端口 9098，测试用 9097/9099，**绝不占用 9090**）。

## 0. 结论（2026-10 排查：模式该选哪个）

| 能力 | `hybrid`（默认） | `local`（纯本地） | `http`（LoadURL） |
|---|---|---|---|
| 页面装配（15 实例 / 10 槽 / ~720 元素） | ✅ | ✅ | ✅ |
| `/api/*` REST | ✅ 真实 HTTP | ✅ 拦截转发 | ✅ 真实 HTTP |
| `/plugins-assets/*` | ✅ 真实 HTTP | ✅ 磁盘直读 | ✅ 真实 HTTP |
| `location.origin` | ✅ `http://127.0.0.1:<port>` | ⚠️ 空字符串 | ✅ 正确 |
| `document.cookie` | ⚠️ 不可写（引擎无 cookie 存储） | ⚠️ 同 | ⚠️ 同 |
| `localStorage` | ✅ | ✅ | ✅ |
| WebSocket `/ws`、`/api/terminal/ws` | ✅ 宿主桥接，真实连接 | ✅ 宿主桥接 | ❌ 引擎 stub（假 open、send 空操作、收不到消息） |
| 资源型 URL（`<img src="/api/...">`） | ⚠️ 引擎未实现子资源加载 | ⚠️ 同（且不走拦截） | ⚠️ 同 |

三条硬结论：

1. **本地加载没有让功能变少**。装配结果与 HTTP 模式一致（`instances=15 slots=10`，
   元素数差 5 个 = 本地多注入的 shim 节点）；`/api`、插件资产、XHR、fetch 全部可用。
2. **纯 HTTP 模式反而更缺**：wb-ui 引擎的 `WebSocket` 是 stub（`bindings/dom.go`
   异步派发假 `open`，`send` 空操作），终端与全局事件流在 `-mode http` 下完全不可用。
3. `window.open` 不存在、`Response.headers.get` 不存在、`<img>` 子资源不加载 ——
   这三项是**引擎级缺失，两种模式表现完全一致**，与「是否装配」无关，需要另开
   引擎任务补齐，不能靠换加载模式解决。

因此默认模式改为 **hybrid**：页面走 HTTP（origin 正确、任何后端路由都不需要桌面壳
再补拦截），WebSocket 仍由宿主桥接（终端/事件流可用）。`local` 作为实验分支保留。

## 1. 三种模式

| 模式 | 页面来源 | 静态资源 | `/api/*` | WebSocket |
|---|---|---|---|---|
| `-mode hybrid`（默认） | 后端 `GET /` 取 HTML，灌到 `LoadHTMLWithBaseURL(html, "http://127.0.0.1:<port>/")` | 引擎网络栈（真实 HTTP） | 真实 HTTP | 宿主 RFC6455 客户端 + JS shim |
| `-mode local` | 磁盘 `index.html`，来源 URL `app://ide/` | 宿主 `ResourceResolver` 读磁盘 | `bridge.RegisterHTTP` 拦截转发 | 宿主 RFC6455 客户端 + JS shim |
| `-mode http` | `LoadURL(http://127.0.0.1:<port>/)` | 真实 HTTP | 真实 HTTP | 引擎 stub（不可用） |

兼容旧参数：`-local` ≡ `-mode local`，`-http` ≡ `-mode http`。

## 2. 文件组成

| 文件 | 职责 |
|---|---|
| `httpmode.go` | hybrid：拉取后端 `index.html`、注入 WS shim、以 HTTP 为文档来源装配 |
| `local.go` | local：目录发现（`-webdir` / `-plugins` / 环境变量 / exe 旁 / 仓库向上查找）、`ResourceResolver` 引用解析、`LoadHTMLWithBaseURL(html, "app://ide/")` |
| `intercept.go` | local：`bridge.RegisterHTTP("", "/api/*", …)` 转发后端；`/plugins-assets/*` 直读插件目录 |
| `wsclient.go` | 最小 RFC6455 客户端（握手、文本/二进制帧、ping/pong、close） |
| `wsbridge.go` | 连接注册表、读循环、主线程派发队列（`Flush`）、URL 映射（按 path 转发到后端） |
| `wsshim.go` | 注入到 `<head>` 最前的 JS shim：覆盖 `window.WebSocket`，实现构造器/事件/`send`/`close` 与常量 |

## 3. local 模式的引用映射（实验分支）

| 页面引用 | 实际文件 |
|---|---|
| `./assets/index-xxx.js`、`app://ide/assets/index-xxx.js` | `<webDir>/assets/index-xxx.js` |
| `./favicon.svg` | `<webDir>/favicon.svg` |
| `/plugins-assets/ui-appearance/theme-midnight.css` | `<pluginsDir>/ui-appearance/theme-midnight.css` |
| `fetch('/api/...')` | 拦截转发到 `http://127.0.0.1:<port>/api/...` |
| `new WebSocket('ws://ide/ws')` | `ws://127.0.0.1:<port>/ws` |

越界防护：引用里出现 `..` 一律拒绝；拼接后的路径必须仍在根目录内。

## 4. 运行

```bat
:: 1) 后端（示例端口；测试一律非默认端口）
set WEB_PORT=9099
companion.exe

:: 2) 桌面壳（wb-ui 仓库根，CGO 环境）
set CGO_ENABLED=1
set PATH=F:\syproject\goskia\bin;%PATH%
set GOWORK=off
go build -o _temp\gouide.exe .\cmd\gouide
_temp\gouide.exe -attach -port 9099
```

local 模式需要能定位前端产物与插件目录（缺省自动发现，也可显式指定）：

```bat
gouide.exe -attach -port 9099 -mode local ^
  -webdir F:\syproject\gou-ide\.pair\assets\runtime\web ^
  -plugins F:\syproject\gou-ide\.pair\plugins
```

启动脚本 `scripts/run-gouide-local.bat` 已封装构建 + 启动。

常用自检 flag：

```bat
gouide.exe -headless -attach -port 9099 -frames 400 -out out\shot.png
gouide.exe -headless -eval "..." -eval2 "..."      :: 跨帧观察异步结果
```

## 5. 已验证（2026-10-18，后端 9099 + 新构建 companion）

hybrid 模式（headless，400 帧 + eval2 跨帧探针）：

```
装配    ：instances=15 slots=10，elements=722，titlebar/activity-bar/sidebar 几何正常
origin  ：http://127.0.0.1:9099
fetch   ：/api/health → status 200，body 142 字节
XHR     ：readyState=4，status=200，142 字节
WS      ：ws.readyState=open，收到 4 条消息；日志「已连接 /ws、/api/terminal/ws」
截图    ：screenshots/gouide_hybrid_headless.png（与网页版布局一致）
```

local 模式（同后端，400 帧）：

```
装配    ：instances=15 slots=10，elements=722
fetch   ：200 / 142 字节；XHR 200 / 142 字节
WS      ：open，收到 3 条消息（ws://ide/ws → 后端 /ws）
origin  ：空字符串（前端 apiURL 用 new URL 解析出相对路径，仍命中拦截）
```

`-mode http`（同后端）：装配一致，但 `ws` 只有 stub 的假 open，收不到任何消息。

## 6. 已知限制

1. **WS 消息按字符串派发**：引擎没有可用 `Blob`，终端二进制帧在宿主转成字符串交给
   xterm；非 UTF-8 字节会被替换。
2. **派发线程**：读循环只往队列放脚本，真正执行由主线程 `Host.OnFrame` / headless
   `pump` 触发；headless 必须保留足够帧数才能观察到异步结果。
3. **引擎级缺失（与加载模式无关）**：`window.open` 未实现（工具集导出按钮点了无效）、
   `Response.headers` 无 `get`（无法读 Content-Length 等）、`<img>`/CSS 子资源不加载
   （图片预览面板显示失败）。这三项要靠 wb-ui 引擎补齐。
4. **local 模式的额外缺口**：文档 origin 为空串、cookie 不可写、资源型 URL 不会走
   `/api` 拦截 —— 这是默认改回 HTTP 装配的直接原因。
5. **后端端口**：桌面壳默认 9098，测试用 9097/9099；默认 9090 属 Web 模式与用户
   运行中的实例，任何情况下都不占用。
