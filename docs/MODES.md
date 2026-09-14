# 运行模式与 UI 构建方式（wb-ui 的两种形态）

> 本文档说明 wb-ui 的**运行模式**（嵌入浏览器 / UI 库）与 **UI 构建方式**
> （基础方式 = Go 构建 / web 方式 = HTML 片段），以及两者的接线点。
>
> 代码入口：`webkit/mode.go`（模式）、`bindings/browserglobals.go`（能力裁剪）、
> `page/fetcher.go`（fetch 策略）、`ui/`（UI 构建层）。

---

## 1. 为什么要模式

wb-ui 的渲染/布局/DOM/CSS/事件管线是同一套，但**装配阶段接入哪些「外部输入」
通道**决定了它是「浏览器」还是「UI 库」：

- 当它是**嵌入浏览器**时，页面要能加载外部样式/脚本、导航、跑 iframe、发网络
  请求、起 Worker —— 这些能力缺一不可。
- 当它是**UI 库**时（宿主用 Go 构建界面、或只把 HTML 当声明式界面描述），上面
  这些通道恰恰是**不确定性来源**：隐式的外部网络、非确定性的资源加载、
  UI 线程上的同步 HTTP（引擎的 `fetch` 是同步实现）、页面自己起线程。

两种形态共存而不是二选一：同一个进程里可以有多个 WebView，各持自己的模式
（引擎的绑定分派本来就是 per-WebView 的）。

## 2. 模式语义表

| 能力 | `ModeBrowser`（默认） | `ModeToolkit` |
|------|----------------------|---------------|
| HTML/CSS 解析、布局、绘制、滚动 | ✅ | ✅ |
| DOM/JS bindings、事件、`matchMedia` | ✅ | ✅ |
| `eval`/页面 `<script>`、宿主 `EvalJS`/`CallFunction` | ✅ | ✅ |
| 宿主桥路由（`bridge.Register` → `fetch('/api/x')`） | ✅ | ✅ |
| `fetch` 未命中桥路由时的真实网络请求 | ✅ | ❌ reject（`fetch(url): 网络请求在 UI 库模式下被禁用`） |
| `XMLHttpRequest` | ✅ | ❌ 不注册（全局不存在） |
| `Worker` / `WebSocket` | ✅ | ❌ 全局置 undefined |
| `<link rel=stylesheet href>` / `<script src>` 的 http(s)/file 加载 | ✅ | ❌ 只有 `data:` URL 与宿主 `ResourceResolver` |
| 同上的**相对引用**（`href="a.css"` / `src="/js/x.js"`），以文档 URL 为基准 | ✅ | ❌（同上一格） |
| `<img src>` / `background-image` / `mask-image` / SVG `<image href>` 的图片加载（含相对引用） | ✅ | ❌ 只有 `data:` URL 与宿主 `ResourceResolver` |
| CSS `@import`（相对引用以**样式表 URL** 为基准，含嵌套链） | ✅ | ❌（同外部样式表格） |
| `<iframe src>` 子文档装配 | ✅ | ❌ 不装配（`<iframe>` 元素仍参与布局/绘制） |
| `LoadURL()` 导航 | ✅ | ❌ `ErrModeNotSupported` |
| `<link>`/`@import`/`<script src>` 的 `data:` URL | ✅ | ✅ |
| 宿主 `ResourceResolver` | ✅（优先） | ✅（唯一外部通道） |

模式**在首次 `LoadHTML` 装配时锁定**：装配要按模式决定 5 处注入（fetch 版本、
XHR、浏览器全局、子框架、外部资源通道），中途切换会让「页面脚本已经
feature-detect 过旧能力」的状态不一致。装配后 `SetMode` 只接受同值调用，
否则返回 `ErrModeLocked`（要另一模式请新建 WebView）。

## 3. 用法

### 3.1 嵌入浏览器（与历史行为逐字节一致）

```go
wv := webkit.NewWebView()          // == NewWebViewWithMode(webkit.ModeBrowser)
wv.Resize(1024, 768)
wv.LoadURL("http://localhost:9090/")
```

`LoadURL` 取到内容后会把该 URL 写进文档（`document.URL` / `location.href`），
并作为**文档基地址**：页面里所有相对引用都按浏览器语义解析——

| 页面里的写法 | 解析结果 |
|---|---|
| `<link href="/style.css">` | `http://host/style.css`（根相对） |
| `<script src="app.js">`（页面 `/dir/page.html`） | `http://host/dir/app.js`（文档相对） |
| `<img src="logo.png">` / `background-image:url(bg.png)` | 同上规则；图片**异步**取回，下一帧绘制出来 |
| `<link href="/css/a.css">` 里的 `@import "b.css"` | `/css/b.css`——**以样式表 URL 为基准**（不是文档 URL） |
| `fetch("/api/users")` / `xhr.open("GET", "items.json")` | 同上规则；桥路由仍先按**原样** URL 匹配（宿主按 `"/api/users"` 注册的路由不受影响） |
| `<iframe src="child.html">` | 同文档相对规则（`resolveIframeSrc`） |

解析器是 [dom.ResolveURL](dom/url.go)（`document.baseURI` 语义）：绝对引用与宿主
自定义 scheme（`app://…`、`data:`、`file:`）原样返回，协议相对（`//cdn/x.css`）
补上文档 scheme。导航到另一个 URL 后基准立即跟随（不再残留旧文档的基准）。

`LoadHTML` 相反：内容**没有来源 URL**（等价 `about:blank`），没有基准可解析——
需要真实网页语义时用 `LoadURL`，或由宿主自己把引用规范化成绝对 URL。

### 3.2 UI 库（宿主不写 HTML）

```go
wv := webkit.NewWebViewWithMode(webkit.ModeToolkit)
wv.Resize(400, 300)
view, err := ui.New(wv)            // 内部载入最小空文档
view.Style(`#panel{padding:12px;background:#1e2228;color:#eee}`)
view.Div().ID("panel").Append(
    view.Text("你好"),
    view.Button("点我", func(dom.Event) { clicks++ }),
)
pixels, _ := wv.Render()           // RGBA8888，宽*高*4 字节
```

`ui.New` / `ui.NewWithHTML` / `ui.Attach` 三个入口对应「空文档起步」「给定 HTML
起步」「挂到已加载文档」；`ui.View` 的所有写操作都会标记渲染树重建，宿主下一次
`Render()` 即生效。

### 3.3 用宿主资源替代外部资源（UI 库模式的关键接线）

```go
wv.SetResourceResolver(func(ref string) (string, bool) {
    switch ref {
    case "app://theme.css":
        return embeddedThemeCSS, true   // 内嵌/内存资源（基础方式）
    }
    return "", false                    // 交回引擎按模式策略处理
})
```

页面照旧写 `<link rel="stylesheet" href="app://theme.css">`（web 方式），内容来自
Go（基础方式）——**这就是「某些作为基础、某些以 web 方式提供」**。

## 4. UI 构建的两种来源

`ui` 包把两种构建方式放在同一棵文档树里，共用 CSS/布局/渲染/事件管线：

| | 基础方式（`SourceNative`） | web 方式（`SourceWeb`） |
|---|---|---|
| 写法 | `view.El("span").Class("badge").Text(...)` | `view.Web(\`<span class="badge">…</span>\`)` |
| 组件注册 | `reg.RegisterNative(name, func(v, props) *Node)` | `reg.RegisterWeb(name, func(props) string)` |
| 解析 | 无 HTML 解析（直接建节点） | 引擎的片段解析器（`el.innerHTML = …` 同一路径） |
| 适合 | 由 Go 驱动的界面、类型安全的组合 | 复用现成 HTML/CSS 资产、Markdown 渲染结果、页面脚本产出 |
| 混用 | 两边产出的都是引擎文档树里的节点，可互相查询、互相嵌套 | 同左 |

```go
reg := view.Registry()
reg.RegisterNative("badge", func(v *ui.View, p ui.Props) *ui.Node {
    return v.El("span").Class("badge").Text(p.String("text"))
})
reg.RegisterWeb("chip", func(p ui.Props) string {
    return `<span class="chip">` + p.String("text") + `</span>`
})
view.Mount("badge", ui.Props{"text": "native"})   // → 组件实例（span）
view.Mount("chip", ui.Props{"text": "web"})       // → 片段承载容器（div）
```

事件回调写在 Go 里（`Node.On("click", func(dom.Event))`），走的是引擎的命中测试
→ 事件分发路径，与页面 JS 的 `addEventListener` 完全同一条链路（同一个元素上
两种监听器可以并存）。同一事件重复 `On` 会**替换**旧处理器（不累积）。

## 5. 可运行的例子

```sh
# UI 库模式：Go 构建界面 + web 片段 + 双源组件 + 渲染成 PNG
go run ./examples/uitoolkit

# 嵌入浏览器模式（能力面差异直接打印出来）
go run ./examples/uitoolkit -mode browser -out out.png

# 真实 HTTP 端到端诊断（真起 httptest 服务器）：相对 CSS/脚本/图片、fetch
# 相对 URL、document.URL/location、导航与重定向后的基准跟随、CSS @import
# （含「以样式表 URL 为基准」的子目录场景）；再用 UI 库模式复验同一组动作 0 网络
go run ./dev/browser_http_probe
```

输出会打印组件来源、点击回调次数、`typeof fetch/XMLHttpRequest/Worker/WebSocket`
（两种模式的差异一目了然）以及 PNG 路径。

## 6. 已知边界

- **模式不可热切换**（见 §2）。多形态共存靠「每个 WebView 一个模式」。
- **图片是异步取回的**：`<img>` 的字节在后台 goroutine 取回并解码，**当帧不画**
  （不阻塞渲染线程）——下一帧命中缓存才出现。宿主按帧渲染（`WebView.Render`）
  即可；点击/布局不受影响。本地 `data:` 与宿主已缓存的内容则同步命中。
- **图片缓存是进程级的**（`rendering.backgroundImageCache`，按**规范化后的
  绝对 URL** 索引）：多 WebView 共享同一份已解码图片（省内存，但内容也共享）。
  模式门禁优先于缓存判定，因此 UI 库模式绝不会显示外部图片——即使浏览器
  模式的另一个 WebView 已经把它取回。
- **`LoadHTML` 下图片引用没有文档基准**：相对路径按宿主进程工作目录读取
  （与 `<link>` 的既有行为一致）；需要浏览器语义时用 `LoadURL`。
- **HTTP 响应不按 Content-Type 拒绝**：`fetchHTTP` 只记录提示。真实服务器给
  `text/css` / `application/javascript` / `application/json` 都是常态，而引擎的
  取内容层不知道调用方用途（同一个响应可能是 `<link>`、`<script src>` 或
  `fetch()`）；浏览器把 MIME 检查放在各用途的消费端，引擎尚未实现那一层。
- **`"Worker" in window` 在 UI 库模式下仍为 `true`**：裁剪实现是把全局置为
  `undefined`（jsc 层未暴露属性删除），`typeof` 判定正确、`in` 判定不严谨。
  feature detect 请用 `typeof`。
- **`<body>` 背景不传播到画布根**（引擎既有差异，与模式无关）：给界面铺底色
  用尺寸铺满的容器元素；`ui` 测试 `TestBodyBackgroundPropagationKnownGap` 锁定
  「两种模式行为一致」。
- **UI 库模式不提供安全策略**：它不加载外部资源，但也不实现 CSP/同源检查
  （引擎本身没有这些层）。
- **`ui` 包不是框架**：没有虚拟 DOM、没有 diff、没有响应式绑定——它是「Go 直接
  操作引擎 DOM 的一层便利 API + 双源组件注册表」。需要声明式响应式时，走 web
  方式（Vue 等）在页面脚本里做。
- **外部资源在 UI 库模式下每次样式重解析都会重新请求 resolver**（引擎按
  `<link>` 的 href 指纹决定是否重扫样式）：resolver 内部应自行缓存，避免重复
  读取大文件。

## 7. 相关提交

| 主题 | 内容 |
|------|------|
| 模式接线 | `webkit/mode.go`（枚举/装配策略/资源解析器）、`webkit/webview.go` 的 4 处注入分派、`page.RegisterFetchWithPolicy`、`bindings.HideBrowserThreadGlobals` |
| UI 构建层 | `ui/ui.go`（View/Node）、`ui/registry.go`（双源组件） |
| 顺带修复 | `page/frame.go` `SetDocument` 未同步 `styleFP` → 每次 LoadHTML 后首次重建会重复全量重扫样式（`<link>` 重复加载）；`file://` 的 URL 规范形式 `file:///C:/x` 之前读不到（前导斜杠） |
