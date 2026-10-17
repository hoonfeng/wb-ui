# gouide 本地加载 + 拦截模式（IDE 的 GUI 版本）

`cmd/gouide` 是 PairCode IDE 的桌面壳：wb-ui 提供原生窗口与渲染，gou-ide 内核
以后端子进程运行（默认端口 9098，测试用 9097/9099，**绝不占用 9090**）。

页面加载有两条路，默认是本文描述的本地模式：

| 模式 | 页面来源 | 静态资源 | `/api/*` | WebSocket |
|---|---|---|---|---|
| `-local`（默认） | 磁盘 `index.html`，来源 URL `app://ide/` | 宿主 `ResourceResolver` 读磁盘 | `bridge.RegisterHTTP` 拦截后转发后端 | 宿主 RFC6455 客户端 + JS shim |
| `-http` | `LoadURL(http://127.0.0.1:<port>/)` | 后端 HTTP 服务 | 真实 HTTP | 引擎 stub（不可用） |

## 1. 组成

| 文件 | 职责 |
|---|---|
| `local.go` | 目录发现（`-webdir` / `-plugins` / 环境变量 / exe 旁 / 仓库向上查找）、引用解析（`ResourceResolver`）、`LoadHTMLWithBaseURL(html, "app://ide/")`、注入 WS shim |
| `intercept.go` | `bridge.RegisterHTTP("", "/api/*", …)` 转发后端；`/plugins-assets/*` 直接读插件目录 |
| `wsclient.go` | 最小 RFC6455 客户端（握手、文本/二进制帧、ping/pong、close） |
| `wsbridge.go` | 连接注册表、读循环、主线程派发队列（`Flush`）、URL 映射 `ws://ide/* → ws://127.0.0.1:<port>/*` |
| `wsshim.go` | 注入到 `<head>` 最前的 JS shim：覆盖 `window.WebSocket`，实现构造器/事件/`send`/`close` 与常量 |

## 2. 引用映射

页面里的引用经引擎归一化后交给 `ResourceResolver`，两种形态都接受
（原样引用与绝对化 URL）：

| 页面引用 | 实际文件 |
|---|---|
| `./assets/index-xxx.js`、`app://ide/assets/index-xxx.js` | `<webDir>/assets/index-xxx.js` |
| `./favicon.svg` | `<webDir>/favicon.svg` |
| `/plugins-assets/ui-appearance/theme-midnight.css` | `<pluginsDir>/ui-appearance/theme-midnight.css` |
| `fetch('/api/...')` | 拦截转发到 `http://127.0.0.1:<port>/api/...` |
| `fetch('/plugins-assets/<name>/client.js')` | `<pluginsDir>/<name>/client.js` |
| `new WebSocket('ws://ide/ws')` | `ws://127.0.0.1:<port>/ws` |
| `new WebSocket('ws://ide/api/terminal/ws')` | `ws://127.0.0.1:<port>/api/terminal/ws` |

越界防护：引用里出现 `..` 一律拒绝；拼接后的路径必须仍在根目录内。

## 3. 运行

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

目录缺省会自动发现（程序旁 `.pair/assets/runtime/web`、`.pair/plugins`，开发期
还会向上找仓库与同级 `gou-ide`）。也可以显式指定：

```bat
gouide.exe -attach -port 9099 ^
  -webdir F:\syproject\gou-ide\.pair\assets\runtime\web ^
  -plugins F:\syproject\gou-ide\.pair\plugins
```

常用自检 flag：

```bat
gouide.exe -headless -attach -port 9099 -frames 150 -out out\shot.png
gouide.exe -headless -eval "..." -eval2 "..."      :: 跨帧观察异步结果
```

## 4. 已验证（2026-10，wb-ui 仓库本机）

后端 `companion.exe`（`WEB_PORT=9099`）+ 本地模式 headless：

```
页面摘要：title="PairCode IDE - taskboard-demo-ws" elements=717
          instances=15 slots=10（10 个 UI 区域插件全部装载）
/api     ：fetch('/api/system/info') → {os:windows, version:v1.6.17, workspace:…}
/plugins ：fetch('/plugins-assets/ui-appearance/theme-midnight.css') → 2719 字节
WS       ：[wsbridge] 已连接 /ws、/api/terminal/ws
终端     ：termRecv=6，首帧为真实 shell 输出（"Microsoft Windows [Version 10.0…"）
错误     ：__pageErrors 为空
```

窗口模式（`-attach -port 9099`，1600x1000）实测：窗口正常打开、两条 WS 均连接、
界面与网页版布局一致（截图 `screenshots/gouide_window_local.png`）。

## 5. 已知限制

1. **fetch 拦截是同步的**：`bridge` 在 JS 线程里同步调用 Go handler，单次请求
   上限 120s（`apiProxyTimeout`）。流式接口（SSE）不要走 `/api/*`。
2. **WebSocket 消息按字符串派发**：wb-ui 引擎没有 `Blob`，终端二进制帧在宿主
   转成字符串交给 xterm；非 UTF-8 字节会被替换。
3. **派发线程**：读循环只在队列里放脚本，真正执行由主线程 `Host.OnFrame` /
   headless `pump` 触发；headless 必须保留足够帧数才能观察到异步结果。
4. **shim 覆盖时机**：shim 注入 `<head>` 最前；若未来页面在 `<head>` 之前执行
   脚本（当前产物不会），需要在装配前用其他方式覆盖。
5. **后端端口**：桌面壳默认 9098，测试用 9097/9099；默认 9090 属 Web 模式与
   用户运行中的实例，任何情况下都不占用。
