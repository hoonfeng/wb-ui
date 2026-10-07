# wb-ui 遗留问题与已知边界

> 定位：只记录**当前有效**的信息——未实现项的取舍结论、有意保留的边界。
>
> 已闭环项的**诊断与方案**已随实现落地移除（原 P0~P3 五项 + 2026-09 批次能力缺口）：
> 诊断过程在 git 历史与总览表列出的提交里，实现要点在代码注释与对应测试里
> （`ElementBox.CanSkipLayout` 的 B 剪枝五条件、`RenderView.ApplyTextChange` 的 C1 增量
> 链路、`engine/dom/element.go` 的 ShadowRoot、`engine/rendering/mask.go` 的遮罩语义），
> 本文档不再维护第二份。

## 总览（原五项遗留：全部闭环）

| # | 原遗留项 | 类型 | 结果 |
|---|---------|------|------|
| 1 | positioned `top/left` 不解析 `calc()` | 真实 bug | ✅ `f2f0a17` |
| 2 | `min()` / `max()` / `clamp()` 未实现 | 功能缺失 | ✅ `3ccdcd5` |
| 3 | 布局三次全树遍历（无增量） | 性能 | 🔍 阶段 A 收益 <1% 不投入；**B** `1ab27fe`、**C1** `d9084a8` 已实现；C2 见下节 |
| 4 | Shadow DOM selector（`:host` / `::slotted` / `::part`） | 功能缺失 | ✅ `11c7fbb`→`4b5aef4` |
| 5 | `mask-image` 仅存属性不绘制 | 功能缺失 | ✅ 子树遮罩 + `size` / `repeat` / `position` / `mode` + SVG `<mask>` + 同文档 `url(#id)` |
| — | WebSocket | ~~非问题~~ | 有意 stub（宿主注入事件），见「附」 |

2026-09 批次另闭环了一批「注释里记着的未实现项」：Fullscreen API、`<dialog>` 状态算法重写与
`::backdrop`、`ToggleEvent.oldState/newState/source`、表单约束校验伪类与 IDL（`:valid` /
`:invalid` / `:in-range` / `:out-of-range` / `:user-valid` / `:user-invalid`）、`:target` 语义、
伪元素名称表一致性、worker 脚本加载接线等——提交范围 `8cbc020`…`f00d306`。这些能力的清单以
**代码与测试**为准（`engine/css/selectorchecker.go`、`engine/js/bindings/`、`engine/html5/`、
`engine/style/validity.go`，测试名即清单），不在文档里维护第二份。

## 未实现项的取舍结论（2026-09 复核）

对文档与代码里残留的未实现项逐条复核。**除「保留为已知边界」者外，其余判定为不需要**
（依据：无业务驱动、无实测瓶颈证据、收益过低或非本仓库职责），条目已从文档移除：

| 项 | 结论与依据 |
|----|-----------|
| 布局增量 **C2**（IFC 行级增量重排） | **不做**。C1（`d9084a8`）已覆盖打字主热路径（text 变更 → 只重排 dirty block）；C2 只再省「block 内一次 IFC 重排」，收益更小，而风险高（等价于移植 WebKit LineLayout 增量）。当前**没有 profile 数据显示「整 block 重排」是瓶颈**——出现该数据再立项。 |
| `::part` 多 part-name 匹配改哈希集合 | **不做**。原调研结论为收益 <1%，却要新增一套索引与失效维护。 |
| canvas 补丁脚本预编译（`RunJS` → `Compile` + `RunProgram`） | **不做**。`applyCanvas2DPatch` 每次只跑一段 2KB 脚本且脚本内有 `__canvasPatched` 守卫；未采样证明其占比，不值得为它动 `EvalJS` 路径。 |
| canvas `getContext('2d')` 包装改 Go 原生回调 | **不做**。同上，且要改 `wrapDocument` 的 tag 分派，影响面大于收益。 |
| goja 引擎热点（`structuredClone` 原生化、`formatConsoleArg` 跳过对象序列化） | **不做**（保留方法论）。goja 慢是纯解释器的架构性代价；真实热点必须先 `goja.StartProfile` 采样定位（`dev/probes/gobench` 可复现大 bundle 编译耗时），凭猜测改热点没有依据。 |
| host-selector 前缀跨 shadow boundary 前向匹配 | **已实现**，无需立项。`selectorchecker.go` 里 compound 结尾是 `::part` / `::slotted` 时，前缀按 CSS Scoping L1 走宿主组合祖先（该文件注释有说明）。 |
| 多层 `background-image` 叠加 | **不做**。引擎只绘制第一层（`engine/rendering/backgroundimage.go` 注释已载明）；「第一层的解析必须正确」已修复并锁测试（`backgroundurl_layers_test.go`）。 |
| `<input>` / `<textarea>` 引入 dirty value flag | **不立项**。影响面大（牵动渲染取值、表单提交、配置面板三条取值路径），收益只在「无 `min` 的控件 + 用户输入 + step 校验」组合下显形，而规范推荐的写法（带 `min`）不受影响。 |
| bundle 缩小（code splitting / manualChunks / 依赖裁剪） | **非本仓库职责**。bundle 由宿主前端（gou-ide 的 web-ui 构建）产出。编译缓存（`0316ef4`）已消掉「重复导航」的 ~600ms 编译开销，首次加载成本归前端工程。 |

**保留为已知边界**（有意不做，逐项见下文各节）：Worker 的 module / SharedWorker / Blob URL /
真实网络、popover 的 top layer / close watcher / 键盘激活 / `CommandEvent`、`:autofill` /
view-transition 伪元素、模式不可热切换、`ui` 包不做声明式响应式等。

## 附：WebSocket —— 已完善（非遗留问题）

`engine/js/bindings/dom.go` 的 WebSocket 构造器（`wsCtor`，`g.Set("WebSocket", …)`）已是
**完整 stub**：提供 readyState 常量、
`onopen/onmessage/onerror/onclose`、`send/close/addEventListener/removeEventListener`，
并通过 `globalThis.__desktopWS.dispatchMessage/dispatchStatus` 供宿主注入事件。这是
桌面端「无真实网络」场景的**有意设计**（不建连接、不崩溃、事件由宿主推入），
不是 bug。有真实传输需求时在宿主注入层覆盖 `window.WebSocket` 即可，无需改引擎。

> ★ **与 CDP 的关系（2026-10-06 补注）**：CDP 调试协议的**服务端在 Go 侧**（`net/http` + 最小 WS 实现），
> **不依赖**引擎内的 `WebSocket` 是否具备真实网络；两者互不影响。规划见
> [docs/implementation-path.md](implementation-path.md) §3「主线 B」。

---

## 已知差异与有意保留的边界

### Worker 的已知差异（有意保留）

- **消息是 JSON 克隆，不是结构化克隆**：`Date` 变 ISO 字符串、`Map`/`Set`/`RegExp`/`TypedArray` 变 `{}`、值为 `undefined` 的成员被丢弃、`NaN`/`Infinity` 变 `null`。函数、Symbol 与循环引用抛 `DataCloneError`（这点与浏览器一致；算法即 `JSON.stringify`/`JSON.parse`）。
- **无 transferable / SharedArrayBuffer**：`postMessage(msg, [buf])` 的第二个参数被忽略——既不做所有权转移，也不报错。
- **无 BroadcastChannel**；`MessageEvent.ports` 恒为空数组。
- **MessageChannel / MessagePort 已实现**（2026-09，`engine/js/bindings/message_port.go`）：
  `new MessageChannel()` 返回互联的 `port1`/`port2`；`postMessage` 经事件循环的 immediate
  队列**异步**投递（宏任务语义，先于 `setTimeout(cb,0)`）；端口支持 `onmessage` /
  `addEventListener('message')` / `removeEventListener` / `start()` / `close()`，
  未启动端口收到的消息先排队、启动（或挂上 handler）后补发。**仍未实现**：`transferable`
  列表、向对端派发 `close` 事件、`MessageEvent.ports` 携带端口对象。
- **无 module worker**：`new Worker(url, {type:"module"})` 按 classic 处理（`{name}` 生效）。
- **无 SharedWorker / ServiceWorker / worklet**。
- **无 Blob URL 脚本**：引擎没有 `Blob` 与 `URL.createObjectURL`，因此不支持 `blob:` worker。
- **worker 全局刻意保持最小**：只有 `self`/`name`/`console`/定时器/`postMessage`/`onmessage`/`importScripts`/`close`。没有 `location`、`navigator`、`fetch`、`XMLHttpRequest`（引擎的 fetch/XHR 在 bindings 的主线程层）。
- **无同源与 CSP 检查**：脚本 URL 完全交给宿主 Fetcher；引擎自身不联网（只认识 `data:` URL），未注入 Fetcher 时非 `data:` URL 派发 error 事件（`Failed to ... no fetcher registered`）。
- **脚本加载/编译失败派发 `error` 事件**（ErrorEvent 形状的 `message`/`filename`/`lineno`/`colno`/`error`），worker 仍然存活但不处理消息（与浏览器行为一致）。
- **宿主需要一行接线才能加载非 `data:` 脚本**：`bindings.SetWorkerScriptFetcher(func(url string) (string, error))`；引擎不替宿主决定联网策略（与 WebSocket 采用「宿主注入传输」同一取舍）。

### popover 的已知差异（有意保留）

- **没有真正的 top layer**：显示中的 popover 用 `position:fixed` + `z-index:1100`
  近似（高于普通内容与 `dialog[open]` 的 1000）。因此「后显示的 popover 一定在
  其它所有内容之上」只在 z-index 层面近似成立，popover 之间的先后顺序靠文档顺序
  / 作者样式。UA 的居中定位也不支持 `width:fit-content`，改用
  `top/left:50% + translate(-50%,-50%)`（与 `dialog[open]` 同一近似）。
- **没有 close watcher**：Esc（close request）由宿主直接调用
  `popover.CloseRequest`，只作用在最上层的 auto/hint popover（manual 不响应，与
  规范一致）。`<dialog>` 的 Esc 关闭仍未实现。
- **没有 implicit anchor element / CSS anchor positioning**：invoker 与 popover
  的锚点关联只记录到 `popover trigger`（供 `ToggleEvent.source` 用），不参与定位。
- **removal steps 未实现**：popover 元素从文档移除时不清理栈，改为在构建列表时
  过滤已断开的元素（与 `<dialog>` 的模态状态同一取舍）。
- **`command` 事件（CommandEvent）未派发**：本引擎还没有 CommandEvent 接口，派发
  一个无 `command`/`source` 字段的同名事件比不派发更容易误导作者。
- **键盘激活未实现**：本引擎没有「按钮上 Space/Enter 派发 click」的路径，因此
  invoker 的键盘激活不生效（鼠标点击路径完整）。
- **属性 setter 一贯宽松**：`popoverTargetElement = <无 id 的元素>` / `= <非元素>`
  在浏览器抛 InvalidStateError / TypeError，本端口静默忽略（与其它反射 ID 的
  属性一致）。

### 仍未建模（有意保留）
- `:autofill` / `:picture-in-picture`：本引擎没有表单自动填充或画中画模型，
  无判定依据，故作永不匹配。
- view-transition 伪元素：已解析但永不匹配（无 view-transition 机制）。
- **`<input>` / `<textarea>` 的 value 用内容属性建模**（本端口的简化）：规范里
  value 还带一层「dirty value flag」内部状态，属性只在初始值/反射时参与。这一差异
  在本轮新增的 step 校验里立刻显形——step base 的第 2 步取「value 内容属性」，而本
  端口的用户输入直接改写该属性，于是**没有 min 的控件在用户输入后 base 跟着漂移**，
  再也不可能出现 step mismatch（浏览器里用户输入不改属性，base 保持稳定）。有 min
  的元素不受影响（base 取 min），而那正是规范推荐的写法。彻底对齐需要引入 dirty
  value flag，会牵动渲染取值、表单提交与配置面板的取值路径，未立项。

---

### vendored goja：动态对象描述符语义放宽（strFlags）（2026-09）

**上游原文（被放宽的行为）**——`engine/js/goja/object_dynamic.go` 的
`checkDynamicObjectPropertyDescr` 对 dynamic object（DOM 元素包装器）一律拒绝受限描述符：

    Dynamic object field %q cannot be made non-enumerable
    Dynamic object field %q cannot be made read-only
    Dynamic object field %q cannot be made non-configurable
    Dynamic objects do not support accessor properties

**放宽内容**：仅对「host handler 不拥有」的键（`o.d.Has(name) == false`）允许完整**数据**
描述符（non-enumerable / read-only / non-configurable）；标志位由新增的 `strFlags` 记录，
**值仍由 host 的 expando 存储**（唯一副本，见 `bindings.lazyElemProps.expando`），避免两份值。
host 拥有的键（id / class / style / value …）保持原有约束（全 true，且仍拒绝 accessor）；
expando 路径同样拒绝 accessor（Vue 不需要，保持显式报错）。

**为什么必须放宽**：Vue 3.5 在 `mountElement` 里对**每个元素**执行
`def(el, '__vnode', vnode, false)`（`def()` = `Object.defineProperty` + `enumerable: false`）。
上游语义下抛 TypeError，而该异常被宿主异步吞掉 → **控制台零报错**、`createApp().mount()`
静默失败、页面空白（实测 `#app` innerHTML=0、DOM 仅 9 个元素）。放宽后 Vue 3.5.39 完整挂载；
gouide 真实产物 7 个区域槽位全部渲染（证据：`out/gouide-real-e2e.json`）。

**影响面**：

- 只影响「host 不拥有的键 + `Object.defineProperty`」这一条路径；host 字段语义不变。
- 非枚举 expando 不进 `for-in` / `Object.keys`（`dynamicObjectPropIter`、`stringKeys` 已过滤）；
  `Object.getOwnPropertyDescriptor` 返回真实标志位（`getOwnPropStr` 包装成 valueProperty）。
- 同族前序放宽：`symValues`（Vue 的 `el[Symbol('_vei')] = {}`，上游原先直接拒绝 symbol 赋值）。
- 测试契约同步更新：`object_dynamic_test.go` 的 `TestDynamicObject` 由「必须抛 TypeError」改为
  「定义成功 + 标志位正确 + 只读拒绝赋值 + 非枚举不出现在 for-in」。
- **升级上游 goja 时必须保留本补丁**：`strFlags` 字段、`defineExpandoStr`、`getOwnPropStr` /
  `getOwnPropIdx` 的 valueProperty 包装、`setOwnStr` 的只读检查、`_delete` 的不可配置检查，
  以及 `dynamicObjectPropIter.next` / `stringKeys` 两处枚举过滤。

### color-mix()（CSS Color 5）支持范围（2026-09）

**为什么需要**：PairCode 前端用 `color-mix()` 定义**全部主要面板底色**与大量边框色
（`plugins-src/ui-app/index.html` 的 `--bg-primary` / `--sidebar-bg` / `--panel-bg` /
`--activity-bar-bg`，以及 RightPanel.vue / PluginPanel.vue / StatusBar.vue 的 border/background）。
引擎不支持该函数时这些变量整体失效 → 视为透明 → 界面只剩文字，大面积面板背景与边框不绘制。

**已支持**：`color-mix( [in <color-space>]? , <color> [<percentage>]? , <color> [<percentage>]? )`
—— 两个颜色分量 + 可选百分比（缺省各 50%，按合计归一），插值按默认 **premultiplied sRGB**
（与 Chromium 实测一致；每条期望值取自 Chromium `getComputedStyle`，见
`engine/style/color_mix_test.go`）。实现入口 `parseColorMix`（`engine/style/resolver.go`）。

**已知边界**：

- 只作为**计算颜色**解析（并入 `parseColor` 路径），不做延迟求值/动画插值；
- 非法输入**不伪装成有效颜色**（回退「不绘制」而非画成黑色，否则界面会留下错误色块），
  由 `TestColorMixInvalid` 钉死；
- 既有颜色语法不受影响（`TestColorMixDoesNotBreakExisting`）。

## 运行模式与 UI 库层（2026-09 批次）

`webkit.Mode`（嵌入浏览器 / UI 库）与 `ui` 包（Go 构建界面 + web 片段混入）：
**模式语义与用法见 `docs/MODES.md`**。这里只保留该批次**有意保留的边界**——「顺带修复」
「真实 HTTP 路径修复」「图片与 `@import` 接通」三节的实现记录已移除（代码与测试即事实：
`webkit/browser_http_test.go`、`webkit/browser_http_media_test.go`、`engine/dom/url_test.go`、
`dev/probes/browser_http_probe`）。

### 有意保留的边界

- **模式不可热切换**：装配阶段要按模式决定 5 处注入（fetch 版本 / XHR / 浏览器
  全局 / 子框架 / 外部资源通道），中途切换会留下「页面脚本已 feature-detect 过
  旧能力」的不一致状态 → 装配后 `SetMode` 只接受同值，否则 `ErrModeLocked`
  （要另一模式请新建 WebView；多形态共存靠「每个 WebView 一个模式」）。
- **浏览器全局的裁剪是「真删除」**：`jsc.JSObject.Delete` 已接出 goja 的属性
  删除，UI 库模式下 `"Worker" in window` 与 `typeof Worker` 同时为假——靠 `in`
  做 feature detect 的库不再误判（属性不可配置时才退回「置 undefined」）。
- **html/body 背景传播到画布**（CSS-BACKGROUNDS-3 §2.11.2）：html 没有背景而
  body 有时，body 的背景被提升为画布背景——整屏铺满、随视口固定（不随页面
  滚动）。iframe 子文档按自身 viewport 铺满，不溢出到父文档画布。
  `ui.TestBodyBackgroundPropagatesToCanvas` 正向锁定该行为。
- **UI 库模式不提供安全策略**：它不加载外部资源（这是它最大的安全收益），但引擎
  本身没有 CSP / 同源检查层，模式切换不改变这一点。
- **外部资源带内存缓存，范围是「每 WebView」**：同一 URL 的外部资源只取一次
  （`webkit/resource_cache.go`：上限 128 条 / 4 MB，超出按插入顺序淘汰最旧；
  响应带 `Cache-Control: no-store` / `no-cache` 时不缓存；模式门禁在缓存查询
  **之前**，否则别的 WebView 的缓存会穿透模式承诺）。缓存不跨 WebView 共享
  ——宿主 `ResourceResolver` 的内容随宿主状态而变，换 resolver 时整体清空
  （`WebView.ClearResourceCache`）。图片的**解码结果**另有一层进程级缓存
  （见下条），因此重复引用同一图片常常连字节都不再取。
- **图片是异步取回的（当帧不画）**：`<img>` 的字节在后台 goroutine 取回并解码，
  命中缓存后的**下一帧**才绘制（渲染线程不被网络阻塞）。宿主按帧渲染即可；
  `data:` URL 与宿主 `ResourceResolver` 提供的内容同步命中，无此延迟。
- **图片缓存是进程级全局的**：`rendering.backgroundImageCache` 按**规范化后的
  绝对 URL** 索引（多 WebView 共享已解码图片，省内存但内容也共享）。模式门禁
  优先于缓存判定，因此 UI 库模式不会显示外部图片；但同一模式下的多个 WebView
  之间仍会共享同名资源的字节。
- **`LoadHTML` 默认没有文档基准**：相对路径按宿主进程工作目录读取（与
  `<link>` 的既有行为一致）。需要浏览器语义时三条路：`LoadURL`（引擎取内容）、
  `LoadHTMLWithBaseURL(src, base)`（**只给基准、不取内容、不联网**，装配前就经
  `page.Frame.SetPendingDocumentURL` 把 URL 写进文档——否则 `<link>`/`<script>`
  在装配中途加载时还没有基准），或让宿主 `ResourceResolver` 提供内容。
- **`page.CachedResourceLoader` 仍无调用方**：它带着 `documentURL` 与相对 URL
  解析能力，但整条链路（`LoadStylesheet` / `RequestResource` 的异步回调）没有
  接到渲染/样式管线——当前图片与样式都走 `Frame` 的同步 loader 通道。
  `webkit/resource_cache.go` 现在按 WebView 提供「取一次 + 去重」的内存缓存，
  但那是 webkit 层的资源字节缓存，与 `CachedResourceLoader` 的异步加载链路仍
  是两套——收敛属后续架构题。
- **MIME：图片不看，`<link>`/`<script src>` 按 nosniff 看**：取内容层
  （`fetchHTTP`）现在带回 `Content-Type` / `X-Content-Type-Options` /
  `Cache-Control`，按**用途在消费端**判定（这正是浏览器的位置）：图片解码成功
  即采用（解码失败才算加载失败）；样式表要求 `text/css`、脚本要求 JS MIME
  类型，且**仅在响应带 `X-Content-Type-Options: nosniff` 时**严格拒绝，无
  nosniff 时宽松接受（与浏览器一致，仅控制台提示）。
- **导航与历史已接通**（`webkit/navigation.go`）：`location.assign` / `replace` /
  `reload` 与 `location.href` 赋值真的换文档（相对引用按文档基准解析），
  `history.back/forward/go` 能跨文档遍历（遍历**不追加**条目）；装配期由页面
  脚本发起的导航**排队到装配结束后执行**，不在装配中途重入换文档；宿主
  `LoadURL` 与 location 导航都进历史栈（`history.length` 反映文档数）。UI 库
  模式拒绝导航，宿主可用 `SetOnNavigationBlocked` 感知。
- **样式表内 `url()` 的基准 = 样式表自身 URL（已实现）**：CSS Values 3 §4.4
  规定样式表里的 `url()` 在**解析时**即相对样式表自身解析（与 `@import` 同一
  规则）——`/css/theme.css` 里的 `background-image:url(bg.png)` 请求
  `/css/bg.png`；而内联 `<style>` 与元素 `style` 属性里的相对 `url()` 相对
  **文档** URL（内联样式的 base 就是文档的 base）；绝对引用 / 协议相对
  （`//cdn/x.png`）/ `data:` / 宿主逻辑名（`app://…`）一律原样保留。
  实现：`collectedDecl.sheetBase` 随声明带上来源样式表（收集链路的最后一参：
  `collectSheetDeclarations` / `collectFromStyleRule` / `collectDeclarations` /
  `collectScopedFromRules` / `collectPseudoDeclarations{,FromRule}` /
  `collectScrollbarFromRule`，由 `sheetBaseURL(sheet)` 提供，内联表为 ""），
  `ResolveElement` / `ResolvePseudoElement` 在级联排序前调
  `absolutizeCollectedURLs` 绝对化。★ 绝对化在 **token 层**重写 `url()`
  （`absolutizeDeclURLs`），因此 `background-image` / `mask-image` /
  `list-style-image` / `content` / `border-image-source` / 简写 `background` /
  自定义属性里的 `url()` 全部通道一次覆盖，将来新增的读 URL 属性也自动受益。
  `@keyframes` 的声明不在级联链路上，由 `addKeyframesFromSheet` 按来源表就地
  绝对化（幂等）。`@import` 继续用 `resolveURLAgainst`（同一函数的通用形式）。
  回归资产：`engine/style/url_base_test.go`（6 项：外部表逐属性 / 简写与多层 /
  内联保持文档基准 / 绝对引用原样 / `@keyframes` / `@import` 链），
  `webkit/browser_http_media_test.go`（`TestBrowserModeStylesheetURLBaseForCSSURLs`、
  `TestBrowserModeDocumentBaseForInlineURLs`——文档与样式表刻意放在**不同深度**
  的目录，否则两种基准会算出同一个 URL 而抓不到回归，这是反向验证暴露的
  fixture 陷阱），`dev/probes/browser_http_probe` 的「样式表内 url() 的基准」4 条断言。
  反向验证：把 `sheetBaseURL` 改成恒返回 "" → `webkit` 两条测试立即失败
  （请求落到文档同级、`#bg` 像素由红变蓝），恢复后通过。
- **WebKit 前缀属性与标准属性同义（已实现）**：`prefixedPropertyAliases` +
  `unprefixPropertyName` 在 `applyDeclaration` 入口归一前缀名。此前
  `-webkit-mask-image` / `-webkit-mask-size` / `-webkit-transform` /
  `-webkit-animation` / `-webkit-background-clip` 等只是被存进 `Properties` 的
  陌生键，没有任何消费点读它们——**mask 图片通道对前缀写法完全失效**（挂件
  模板与老页面大量使用前缀）。刻意不收录语义不同的前缀（`-webkit-box-orient`
  / `-webkit-line-clamp` / `-webkit-appearance` / `-webkit-gradient(…)` 旧渐变
  语法 / 本引擎已有专门实现的 `-webkit-text-stroke*`）——机械映射会产生
  「看起来生效但语义不同」的错误结果，比不支持更糟。回归：
  `engine/style/url_base_test.go:TestPrefixedPropertyAliases`（反向验证：让归一恒
  原样返回 → 5 条断言失败）。
- **多层 `background-image` 取第一层（已实现）**：`parseBackgroundURL` 用
  `strings.IndexByte(inner, ')')` 而不是 `LastIndex`——`url(a.png), url(b.png)`
  是常见写法，`LastIndex` 会得到 `a.png), url(b.png` 这种垃圾 URL，连第一层都
  加载不出来（多层叠加未实现，但第一层必须正确）。带引号形式按引号配对截断，
  避免引号内的 `)` 提前结束。回归：
  `engine/rendering/backgroundurl_layers_test.go`（反向验证：换回 `LastIndex` → 2 条
  断言失败）。
- **`ui` 包不做声明式/响应式**：没有虚拟 DOM、没有 diff、没有响应式绑定——它是
  「Go 操作引擎 DOM 的便利 API + 双源（native/web）组件注册表」。需要声明式
  响应式时走 web 方式（Vue 等在页面脚本里做）。

## CDP 调试服务端的实现边界（2026-10-07）

内置 CDP 服务端已实装（主线 B0/B1，使用说明见 [docs/CDP.md](CDP.md)）。以下是
**有意保留**的边界，不是待修的 bug：

- **控制台级别已保真（2026-10-07 补做 S2）**：`jsc.BufferLogger` 增 `Entries`
  （Level + Text），`console.log/info/warn/error/debug` 各自落账 → `Log.entryAdded` 与
  `Runtime.consoleAPICalled` 按级别上报。**剩余边界**：事件时间戳是宿主事件泵
  （150ms 轮询）取走的时刻，不是页面里 `console.error` 的真实时刻；引擎不带堆栈
  （`Runtime.consoleAPICalled` 没有 `stackTrace`）；`console.table`/`group` 等仍是
  普通文本。
- **`objectId` 句柄已实装，但存在页面侧（2026-10-07 补做 S2）**：句柄表是页面里的
  `window.__cdpHandles` 数组（`app/cdp.go`），`objectId` 形如 `cdp:3`——好处是句柄与
  JS 值同生命周期、宿主不必跨 goroutine 持 goja 值。**剩余边界**：① 句柄随页面导航
  失效（表在页面里）；② `Runtime.getProperties` 只回**自身**属性（无原型链、
  `internalProperties` 恒空）；③ 句柄没有「对象组」语义（`releaseObjectGroup` 是 no-op）；
  ④ 事件/异常里不会自动带 objectId（`Runtime.exceptionThrown` 未实装）。
- **`deviceScaleFactor` 只接受 1**：引擎没有 DSF / 移动端仿真，传其他值**明确
  报错**而不是静默忽略——静默忽略会让客户端以为缩放生效，量出来的几何全是错的。
- **`Page.captureScreenshot` 只支持 `format=png`**：宿主侧只有 PNG 编码路径
  （`Render()` 给像素缓冲）。请求 jpeg 时明确报错，避免「声称 jpeg 实为 png」。
- **中键点击明确报错**：引擎鼠标管线只处理左键点击与右键 contextmenu。
- **`Target.attachToTarget` 只支持 `flatten:true`**：非 flatten 需要为每个会话开
  独立隧道；CDP 客户端普遍用 flatten，不做无谓复杂度。
- **Windows 上不设 `SO_REUSEADDR`**：该选项在 Windows 的语义会让**其他进程**也能
  抢绑同一端口（与 Unix 不同），对「执行任意 JS」的调试端口不可接受。代价是端口
  处于 `TIME_WAIT` 时无法立即重绑——宿主错误信息会提示换端口。
- **不做 permessage-deflate、不做子协议协商**：`ws` 子包只实现 CDP 需要的那一小块
  （掩码文本帧 + ping/pong/close + 分片），换取零新依赖。
- **CSS 只读到「计算值 / 内联 / 匹配规则」三样（2026-10-07 补做 S2）**：
  `getMatchedStylesForNode` 是在页面上遍历 `document.styleSheets` + `querySelectorAll`
  判定命中得来的（引擎没暴露「元素 → 匹配规则」的内部接口，那要动样式解析层）。
  **剩余边界**：没有 specificity、没有规则的源码行号（`SourceURL` 只有样式表 href）、
  不支持 `CSS.setStyleSheetText` / `CSS.getStyleSheetText` / `CSS.setPropertyText`
  ——Elements 面板的**样式编辑**因此不可用（属性编辑走 `DOM.setAttributeValue` 可用）。
- **元素高亮（`Overlay.highlightNode`）未实现**：DevTools 的悬停高亮要在引擎里画一层
  覆盖——引擎有 hit-test 与几何，但没有「调试高亮」这层绘制（S2 面板仍能选节点、看几何与样式）。
- **`DOM.getNodeForLocation` / `DOM.setOuterHTML` / `DOM.performSearch` 未实现**：
  前者需要「坐标 → 节点」的稳定映射（`elementFromPoint` 可用，但要把元素转成 backendID
  路径，属可加项）；后两者属 Elements 面板的编辑/搜索路径——当前明确回 -32601，
  不静默成功（客户端能区分「不支持」与「执行了没效果」）。
- **引擎 `getAttribute` 在属性缺失时回空串（不是 `null`）**：浏览器语义是 `null`，本引擎回 `""`。
  CDP 判据 10 因此只断言「删除后不再读到 42」而不是「读到 null」。这是**引擎的 DOM 语义差异**，
  不是 CDP 层的（记在此处，避免下次又当成 CDP 的 bug 去查）。

## 媒体帧通道的实现边界（2026-10-07 · 主线 A1/A2）

视频出画面已实装（宿主注入帧流，见 [docs/implementation-path.md](implementation-path.md) §0.1 与 §2）。
以下是**有意保留**的边界与代价：

- **取帧分两条路，只有静止态会阻塞渲染线程**（A2 起）：播放推进中（`VideoElementState.Playing`）
  painter **不同步**等解码——绑定层每次 tick 预取下一时刻（`PrefetchVideoFrame`），宿主 worker
  池异步交付（`app/mediaframepump.go`），未交付时先显示上一次成功显示的帧。判据 A2-4 实测
  「播放全程同步抽帧 0 次」。静止态（首帧 / seek / 暂停）仍走同步：那次绘制必须在本次画出结果
  （宿主 `FrameAt` 自带缓存与负缓存，稳态下不重复跑 ffmpeg）。
- **预取窗口已按帧率给，但「判定粒度」仍是时钟步长**（2026-10-07 补做 A2-②）：宿主探测到 fps 后
  随元数据交给引擎（`MediaMetadata.FPS`），绑定层按帧预取 `mediaPrefetchFrames`(3) 帧（10fps =
  300ms 窗口），帧率未知时退回一个时钟步长。★ 仍未做到「每帧都提前备好」：呈现与预取的**判定**
  挂在 250ms 的时钟步长上，10fps 素材每秒只推进 4 次时钟——真正的逐帧播放（30fps 也每帧一次）
  要等播放时钟与帧率对齐（见本文末「A2 剩余的剩余项」）。
- **帧到达后的重绘通知已接线**（2026-10-07 补做 A2-③）：`rendering.AddVideoFrameReadyListener`
  → `webkit.onAsyncVideoFrame` → `MarkRenderTreeDirty + MarkAllDirty`（**不**请求重排：画面不参与
  布局）。按需渲染的宿主因此不会再停在上一帧；反向验证见 `webkit/videoframe_repaint_test.go`。
  代价：交付发生在 worker goroutine 上，标记与绘制之间隔着宿主的一帧（可接受）。
- **同一时刻可能被抽两次**：渲染层的去重只覆盖「它自己提交的请求」（同步路径与异步预取各算一路），
  宿主的 `FrameAt` 缓存也不做并发合并。实测影响可忽略（多跑一次 ffmpeg，结果一致）；若要收紧，
  应在宿主缓存上做 in-flight 合并（A2 剩余）。
- **seek 已精确到帧，但取样点由帧率推导**（2026-10-07 补做 A2-①）：实测 `-ss t` 是 **ceil 语义**
  （取「时间戳 ≥ t 的第一帧」，见 implementation-path 实现事实 18），因此宿主把请求值左移到目标帧
  区间的取样点 `(k-0.5)/fps`（`app/mediaprobe.go` 的 `alignFrameTime`）；判据 A2-5 用按帧号的
  `-vf select=eq(n,k)` 逐帧比对通过（20 个采样点）。**代价与剩余风险**：需要帧率（未知时不对齐，
  偏差 ≤ 一帧）；`-ss` 仍在 `-i` 之前，靠「取样点落在目标帧区间内」保证落点，样本的编码时间基若
  与 `k/fps` 偏离较大（异常容器）时可能再次偏一帧。
- **帧缓存是 64 条 FIFO**（渲染层已解码帧 + 宿主 PNG 字节各一层）：播放是前向推进，FIFO 淘汰掉的
  恰好是已播过的旧时刻（预取只提交未来时刻），当前实现够用；但**回退显示**（`last`）指向的帧若被
  淘汰，那一帧就会短暂空窗。长播放 + 大帧（1080p）需要按播放前进淘汰 + 有界预取窗口（A2 剩余）。
- **没有原生播放控件、没有自动播放**：`controls` 属性不绘制任何 UI；`autoplay` 不自动起播
  （宿主/脚本调 `play()`）。这两项都要等 A2/A3 一起定。
- **不校验帧尺寸与 `videoWidth/Height` 的一致性**：帧按 `object-fit` 拉伸进内容盒，宿主给的帧
  若与元数据尺寸不符不会告警（A2 可加一致性检查）。
- **音频仍无输出**（A3）：`<audio>` 不参与帧通道（`<video>` 之外的元素一律不取帧），
  `decodeAudioData`/`AudioContext` 不存在。
- **播放结束时的最后一帧靠「越界收敛」拿到**（2026-10-07 补）：`currentTime == duration`
  这个精确时刻没有帧（1s/10fps 的末帧在 0.9s），宿主按已探测时长把请求收敛到
  `duration - 0.1s`（`MediaProbe.clampFrameTime`）+ 退一步重试。副作用是**seek 到时长末尾**
  拿到的是末帧之前的帧（视觉上就是最后一帧）。★ 本机 ffmpeg 的 `-sseof -0.05` 在这些样本上
  产出 0 字节（`-ss 0.9` 才有帧），所以末尾兜底不用它。
- **`requestVideoFrameCallback` 的已知差异**（2026-10-07 实装 A2-③）：本引擎没有合成器，「新帧
  呈现」定义为**帧号变化**（`frameNo = floor(currentTime*fps)`，帧率未知退化为毫秒）：
  ① 回调**粒度 = 播放时钟步长**（10fps/1s 的素材回调 4 次而不是 10 次，判据 A2-7 就此断言）；
  ② 回调里的 `this` 传 `undefined`（规范是元素本身）；③ `presentationTime` / `expectedDisplayTime`
  是宿主 wall clock 的近似值（单调、可用于量帧间隔），`processingDuration` 恒 0；
  ④ 回调经宏任务派发（与其它媒体事件一致），比规范的「合成器提交时同步调用」晚一个任务。
- **A2 剩余的剩余项**：① `videoWidth` 与帧尺寸的一致性检查（帧按 `object-fit` 拉伸进内容盒，
  尺寸不符时不告警）；② 宿主抽帧缓存的 in-flight 合并（渲染层与宿主各有一路去重）；
  ③ 播放时钟与帧率对齐（现在的 tick 固定 250ms，逐帧播放需要「每帧一个 tick」或按帧率推进）；
  ④ 帧缓存淘汰策略（64 条 FIFO，回退显示指向的帧被淘汰时短暂空窗）。

## JS 引擎后端抽象的现状与边界（2026-10-07 · C-P2）

`engine/js/jsc` 已完成**后端接口抽象的第一步**（见 [docs/implementation-path.md](implementation-path.md) §4.5 的 C-P2 行）：

- **已达成**：goja 的类型与构造器全部收敛到 `engine/js/jsc/backend_goja.go`（全包唯一 import goja
  的文件），其余六个文件只用 `be*` 名；`backend.go` 给出 `Backend` / `RuntimeHandle` 契约与后端注册表；
  `bindings` 零改动（它只用 jsc 的公开 API）。
- **明说的边界**：这**不是**「第二个后端的实现」——`Backend` 契约目前只覆盖「建运行时 / 执行脚本 /
  取全局对象」（任何后端都必须有的最小面），且 jsc 的实现仍与 goja 的**对象模型同构**（`beValue` 是
  `goja.Value` 的别名，零成本）。真正的 V8 后端（`backend_v8.go` + MSYS2 SDK + v8go fork）是 C-P3，
  属「必须用户确认」的体积/许可/构建复杂度门槛（§6）。
- **契约为什么不预先铺开**：按 C-P2 的原则「等真后端接进来时按**实际编译错误**补齐，而不是凭想象
  设计一套没人用的抽象」——把接口方法写满会在没有第二个实现时无法验证，反而变成猜测。
- **换后端时的已知难点**（给 C-P3 的提醒）：`JSValue` 里带 `interp *Interpreter` 与 `nativeFn`，
  且绑定层大量依赖 `ToObject/ToValue/Call` 的隐式转换；V8 侧需要句柄表（Persistent + Scope）与
  「Go 回调 → v8 函数」的适配，跨界成本还会上升 7.4–240×（§1.3 实测）。因此 C-P3 之前应先做
  C-P4 的跨界优化，否则「换后端提速」会被跨界开销吃掉。

## 动图（A4）的实现边界（2026-10-07）

GIF/WebP 动画已实装（宿主用 goskia 的 SkCodec 解多帧，引擎按帧时长推进，见
[docs/implementation-path.md](implementation-path.md) §0.1 的 A4 行）。**有意保留**的边界：

- **帧推进精度依赖宿主给的时长**：容器没给时长（部分 GIF/WebP）时按 100ms/帧兜底；
  `loops == 0`（只播一次）停在末帧，`loops < 0` 无限循环，`loops > 0` **近似**为循环
  （Skia 的 repetitionCount 语义是「再播 n 次」，没有精确映射成「共播 n+1 次」）。
- **重绘驱动要宿主接线**：按需渲染的宿主必须把 `rendering.HasAnimatedImages()` 纳入「本帧是否
  需要 Paint」的判断（`app/host.go` 已接；不接的话动图会停在某一帧——这个坑在自检里以
  「6 次采样只出现 1 种颜色」暴露过）。
- **帧位图常驻内存**：每帧一张 `DecodedImage`，且解码策略是**一次性全解**（Skia 的增量帧依赖
  `fPriorFrame`，随机访问要重解前置帧，逐帧按需解反而更慢）。长动画 × 大图会明显吃内存，
  没有「按需解帧 / 释放远帧」机制。
- **动画只在「有图在动」时推进**：不跟踪元素可见性（滚出视口也继续推进），代价是每帧一次
  选帧（可忽略）与宿主持续重绘。
- **入口覆盖**：`<img>` 与 `background-image` 都走同一通道（动图不进单帧缓存）；`canvas` 的
  `drawImage(gif)` 仍只拿首帧（canvas 侧没有帧推进，属未落地）。
- **格式支持取决于 Skia**：goskia 的 codec 绑定与格式无关，能解什么由 libSkiaSharp 决定
  （本机实测 GIF 通过；**WebP 动画未验证**——要验证需补样本）。
- **`<img>` 的 alt 回退与动图并存**：解码失败的动图仍走 alt 文本路径（既有行为不变）。

## C-P4 跨界优化（2026-10-08）：已完成的范围与仍然存在的边界

详细数据与复现命令见 **[docs/CROSS_BOUNDARY_PERF.md](CROSS_BOUNDARY_PERF.md)**。要点：

- **已达成（实测）**：`styleProxy` 写路径同值快速路径 + 写路径副本 ⇒ `SetPropertyDotted`
  686 531 → **106 393 ns/op**（分配 6 628 → 928）、同值 `SetPropertyNoop` 386 938 → **79 123 ns/op**；
  读 `GetPropertyValue` 609 588 → 415 324 ns/op。
- **事件派发聚合（T4）**：Move 路径复用 hover 追踪的命中结果（原先每次移动两次 `HitTest`）；
  宿主事件批里**连续**移动合并为段内最后一条（`app/input_coalesce.go` 的 `coalesceCursorMoves`，
  UI Events coalesced events 语义）；裸 WebView 宿主可用 `HandleMouseMoveBatched` / `FlushMouseMoves`。
- **`el.style` 句柄缓存（T5）**：现在满足浏览器语义 `el.style === el.style`（按元素 + 解释器缓存，
  随文档切换清理）。**但实测取句柄本身不是热点**（15.9 → 16.4 µs/1000 次，持平）——缓存是为语义与
  后续访问稳定性，不是为这一项的吞吐，别把它当性能主因。
- **仍然存在的边界**：`getPropertyValue` 每次仍构造返回串与补白名单属性（读路径 ~4.8k allocs/1000 次，
  未优化）；`cssText` 读必须每次序列化（持平，符合预期）。
- **换后端的杠杆仍然成立**：单次跨界 108–197 ns（JS→Go 回调 / Go→JS 调用 / `obj.Get` 12 ns），
  本轮把**跨境次数**与**同值写**的天花板压低后，C-P3（V8）的收益结构才看得清（见下节）。

## A3 音频后端：现状与开工前置（2026-10-08 结论）

**现状（未开工，属 P3「必须用户确认」项）**：

- `<audio>` 不参与帧通道——引擎的媒体帧通道只服务 `<video>`（见本文「媒体」节），因此音频**无输出**；
- `decodeAudioData` / `AudioContext` / `OscillatorNode` 等 Web Audio 面**不存在**；
- 编码侧（`MediaRecorder` 等）同样未落地。

**开工需要的三件事**（缺一不可，因此本轮只落结论、不动代码）：

1. **平台音频 API**：Windows 侧（WASAPI/`winmm`）与 goskia/GLFW 侧的音频输出接口都要扩
   —— 当前 `goskia` 只有 GL/Skia 面，没有音频面，这不在 wb-ui 单侧能完成的范围内；
2. **后端选型**：自带解码（复用已集成的 ffmpeg/宿主 ffmpeg 管线，与 A2 视频同源）vs 只做输出
   （宿主给 PCM）——前者更完整、后者更轻；选型未定；
3. **用户拍板**：涉及分发体积与平台 API 门槛，与 C-P3（+60MB）同属需确认项。

**建议路线（若获准开工）**：沿用 A1 已验证的「宿主注入」思路 —— 视频侧当初就是**把解码器留在宿主、
引擎只接帧流**（因此不受分发体积约束）。音频同样可先做「宿主注入 PCM 流 + 引擎侧 `<audio>` 状态机
（`currentTime`/`paused`/`ended`）」，把平台音频 API 关在宿主里；Web Audio 图（`AudioContext`）
另立一期，不与之捆绑。

## C-P3 V8 后端探测结论（2026-10-08 汇总，未开工）

**探测已做且结论明确**（数据源：`docs/implementation-path.md` §4.5、`scripts/v8/README.md`）：

- **可行性已实测通过**：MSYS2 `mingw-w64-x86_64-v8 11.9` + 本地 fork 的 v8go（补丁 6 行）在 Windows 上
  **编译 / 链接 / 运行 / 基准全通过** ⇒ 「V8 路线在 Windows 上跑得通」不再是假设；
- **收益（纯 JS 计算）**：fib 6.9×、array 12.1×、string concat **232×**、object prop **87×**、
  closure 45×；但 `try/catch` 反而 **goja 快 3.1×**（V8 侧不擅长的路径）；
- **代价（跨界）**：V8 的 JS↔Go 每次调用比 goja **慢 7.4×–240×**（`obj.Get` 240×、`obj.Set` 67×、
  JS→Go 回调 7.4×）⇒ 换后端只在「代码在 JS 里跑」的负载上赚，在边界负载上亏；
- **未决门槛**：分发体积 **+≈60 MB**、许可与构建复杂度（MSYS2 SDK + 维护 v8go fork）、
  跨平台版本不一致（Windows 11.9 / 其他平台待定）⇒ 属 P3「必须用户确认」；
- **净结论**：**C-P4 是本项的前置**（本轮已完成，见上节）——把跨界次数与同值写的天花板压低之后，
  「换 V8 值不值」才是在真实负载上可判定的事；在后端未接入前，`jsc` 的后端抽象只铺到
  「建运行时 / 执行脚本 / 取全局对象」（C-P2 结论：按实际编译错误补齐，不预先想象契约）。

## webkit 测试基线：3 个 pre-existing 失败（**已全部修复 2026-10-07**）与 @media 归属缺陷（2026-10-07）

### 一、三个 pre-existing 失败（有 HEAD 对比证据，非本轮引入）

对比方法（**两边同一条命令**，排除环境差异）：

```bat
git worktree add ../wb-ui-head HEAD          REM HEAD = 45da5dd
set CGO_ENABLED=1 && set GOWORK=off
go test ./webkit/... -count=1                 REM 分别在 ../wb-ui-head 与工作区执行
```

为什么两边都用 `GOWORK=off`：父目录 `F:\syproject\go.work` 里 `use ./GWui` 指向的目录只剩
`.Pair`（无 `go.mod`）⇒ 整个 workspace 不可用（`go` 直接报
`cannot load module ..\GWui listed in go.work file`）；`GOWORK=off` 让两边都按 `go.mod` 解析，
且解析到的 goskia 与本地 `goskia` 目录 HEAD 是**同一 commit**（工作区 `go.mod` 已升到
`v0.0.0-20261006194810-5015494aa077` = `5015494aa077`），因此两次运行的差异只剩 wb-ui 自身代码。

| 运行 | goskia 版本 | 失败用例 |
|---|---|---|
| HEAD worktree | `v0.0.0-20261006060753-395100befa2a` | `TestButtonTextVerticalCenter`、`TestCM6RangeMeasurementMatchesSkia`、`TestCheckedStateInvalidatesStyle` |
| 工作区（本轮改动） | `v0.0.0-20261006194810-5015494aa077` | 同上 3 个（用例名逐字相同） |

失败摘要（HEAD 输出原文）：

- `TestButtonTextVerticalCenter`：`button_center_test.go:82: glyph vertical center 19.0 too far from button center 21 (range 19.5..22.5)`；
- `TestCM6RangeMeasurementMatchesSkia`：`cm6_measure_skia_test.go:99: getClientRects height 14.8281 != Skia ascent+descent 13.0000`；
- `TestCheckedStateInvalidatesStyle`：`formstate_invalidation_test.go:75/91: SetChecked(true) 后 #c color="rgb(1, 2, 3)"，want "rgb(9, 9, 9)"`。

**判定 pre-existing**：三项在 HEAD 与工作区的失败用例名、失败文件与行号、断言值完全一致。
> ★ **2026-10-07 后续：三项已全部修复，单测基线首次全绿**（`ok 29 包 + 67 无测试 + FAIL 0`，EXIT=0）。
> 其中最关键的订正是上表第 3 行——原根因假设（`:checked` 读 attribute 而非 IDL 状态）经实测被**推翻**，
> 真正的缺陷在 `getComputedStyle` 的渲染树快照覆盖语义；前两项则是**断言口径**写错（非引擎缺陷）。
（另有 4 个 webkit 级失败只在 HEAD 侧的 goskia 版本上出现/消失的项，本表只列上述 3 个共同项。）

#### 三条失败的逐条侦查与处置（2026-10-07：Q4-B 只读定位 → **Q4-C/D 已修**）

复跑入口（本轮实跑，已固化为脚本子命令）：

```bat
MSYS_NO_PATHCONV=1 cmd /c "cgo_env.bat test-all"
REM → 修复前：ok 28 包 + 1 FAIL(wb-ui/webkit，3 用例) + 67 无测试 = 96 包，EXIT=1（2026-10-07）
REM → 修复后：ok 29 包 + 0 FAIL + 67 无测试 = 96 包，EXIT=0（2026-10-07，Q4-C/D）
```

| # | 测试（文件:行） | 失败断言（本轮输出） | 根因假设 | 修复风险 | 建议选项 |
|---|---|---|---|---|---|
| 1 | `TestButtonTextVerticalCenter`<br>（`webkit/button_center_test.go`） | `glyph vertical center 19.0 too far from button center 21 (range 19.5..22.5)`；`glyph white pixels=6 y-range=[19,19]` | 「断言脆性」方向对，但更准确的说法是**断言本身错了**：`−`（U+2212）整条横画位于 **x-height 中部**而不是行盒中心，ink 中心**天然偏上**（软阈值 >100 也只给出 `[19,19]` ⇒ 并非抗锯齿边缘差异）。**布局居中本身没问题**：同批实测 `TestButtonTextCJK` 的 ink 中心 = 按钮中心（36.0 vs 36.0，**完全相等**） | **改测试断言**：扁字形改判它**能**判的两件事——① **水平居中**（实测 20.5 vs 21.0，差 0.5px）② ink **完整落在按钮内容框内**；垂直保留 ±3px 宽松中心检查。✅ PASS |
| 2 | `TestCM6RangeMeasurementMatchesSkia`<br>（`webkit/cm6_measure_skia_test.go`） | `getClientRects height 14.8281 != Skia ascent+descent 13.0000` | 原假设正确：**口径不一致**（非引擎缺陷）。`14.8281 / 13.0 = 1.1406` 正是该字体 `line-height: normal` 的行距系数 ⇒ 行盒高度 = ascent+descent+**lineGap**，而断言只取 `GlobalFontAscent + GlobalFontDescent`（**不含** lineGap）。引擎侧同一口径见 `engine/layout/layoututil.go` 的 `fontLineGap`（注释含 Chrome 实测对照） | **修断言口径**：改用 `graphics.GlobalFontMetrics` 的 `ascent+descent+lineGap`。✅ PASS（13.0000 + 1.8281 = 14.8281 精确吻合） |
| 3 | `TestCheckedStateInvalidatesStyle`<br>（`webkit/formstate_invalidation_test.go`） | `SetChecked(true) 后 #c color="rgb(1, 2, 3)"，want "rgb(9, 9, 9)"`；`JS el.checked=true 后` 同 | ★ **原根因假设被实测推翻**。原文写「`:checked` 匹配读 attribute 而非 IDL 状态」，但实测：`HasAttribute("checked")` 为 **true**（`SetChecked` **确实写了** attribute）、`querySelector('#c:checked')` = **MATCH**、同夹具里**兄弟** `#c:checked + #s` 的样式**正确更新**、用 `min-height` 写的同构规则（matchedDecls=2）**完全生效**。真实根因在 **`getComputedStyle` 的快照覆盖语义**（`engine/js/bindings/dom.go` 的 `applyComputedSnapshot`）：`isUAFormControl(el)` 对 `<input>` **无条件**置 `need = true` ⇒ 用渲染树快照**逐键全量覆盖**级联结果；而快照取自**尚未按最新文档状态重算**的渲染树（`ro.Style()`），于是 `color`/`font-size`/`font-family` 被陈旧值覆盖（`min-height` 不在快照键里 ⇒ 正常）。完整实测链见 §一 末尾 | **修引擎**：`applyComputedSnapshot` 改为「补缺 + 归一化 + 控件的 UA 属性」才覆盖，不再全量覆盖（新增 `snapshotNormalizes` / `uaControlSnapshotProp`）。✅ PASS |

- 三项均有上表（§一）的 HEAD worktree 对比证据 ⇒ **均为 pre-existing，不是本计划引入**；
- **2026-10-07 已全部修复**（Q4-C/D 落地）：前两项是**测试断言口径**问题（断言本身错），第三项是
  **引擎真缺陷**（`applyComputedSnapshot` 用**尚未重算**的渲染树快照覆盖级联结果，见上表第 3 行）；
- **验收**：`go test ./webkit/` 整包 **ok**；全量 `go test`（除 `dev/suites/consistency`）
  **EXIT=0、FAIL 0**（ok 29 包 + 67 无测试）—— 单测基线**首次全绿**（此前为 ok 28 + FAIL 1 包/3 用例）。

### 二、本轮修掉的缺陷：getComputedStyle 的 @media 判定丢失「元素归属」

**症状**：新增用例 `TestDeviceScaleFactor` 单独 `-run` 跑通过，与同包其他「建 WebView」的用例
**并跑**时失败 —— `window.devicePixelRatio === 2` 与
`matchMedia("(min-resolution: 2dppx)").matches === true` 都正确，但
`getComputedStyle(#a).color` 仍是基础规则的 `rgb(1, 2, 3)`（媒体查询里声明的 `rgb(4, 5, 6)` 未生效）。

**根因**：`computedStyleFor`（getComputedStyle 的实现）在 bindings 内自行走一遍级联，
其 `@media` 判定用 `mediaMatches` → **包级单例** `MediaQueryContextProvider`（无元素/解释器上下文，
只能遍历 `webviews` 取第一个）。多 WebView 同进程时（主窗口 + 挂件窗口，或同包测试并存多个
WebView）会读到**另一个页面**的像素比（1）⇒ `min-resolution: 2dppx` 不匹配。
`matchMedia` 早先用 `MediaQueryContextForInterpreter` 解决过同类问题，这条路径漏了。

**修复**：新增 `bindings.MediaQueryContextForElement func(dom.Node) *css.MediaQueryContext`
（webkit 侧按元素归属注入，走 `webViewForNode` → 该 WebView 的 `mediaQueryCtx`），
`mediaMatches(rule, el)` 优先按归属解析 → 退化到包级 Provider → 最后默认上下文，
与 `ForInterpreter` 同构；`@media` 与 `matchMedia` 从此共用同一份输入。

**验证**：修复前「单独跑 PASS / 与 `TestButtonTextVerticalCenter` 并跑 FAIL」；
修复后两种跑法均 PASS；`./webkit/...` 全量失败集合回到上述 3 个；
`./engine/js/bindings/... ./engine/style/... ./engine/css/...` 无回归。

### 三、环境提醒

父目录 `F:\syproject\go.work` 的 `use ./GWui` 目前指向一个没有 `go.mod` 的目录（只剩 `.Pair`），
该 workspace 整体不可用：在 wb-ui 下跑任何 `go` 命令都需 `GOWORK=off`（或修好那份 go.work）。

