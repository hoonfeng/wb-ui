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
| 1 | positioned `top/left` 不解析 `calc()` | 真实 bug | ✅ `0a042d3` |
| 2 | `min()` / `max()` / `clamp()` 未实现 | 功能缺失 | ✅ `a9c69d3` |
| 3 | 布局三次全树遍历（无增量） | 性能 | 🔍 阶段 A 收益 <1% 不投入；**B** `421f7f5`、**C1** `ae5d64f` 已实现；C2 见下节 |
| 4 | Shadow DOM selector（`:host` / `::slotted` / `::part`） | 功能缺失 | ✅ `1dff19a`→`2bcf5cc` |
| 5 | `mask-image` 仅存属性不绘制 | 功能缺失 | ✅ 子树遮罩 + `size` / `repeat` / `position` / `mode` + SVG `<mask>` + 同文档 `url(#id)` |
| — | WebSocket | ~~非问题~~ | 有意 stub（宿主注入事件），见「附」 |

2026-09 批次另闭环了一批「注释里记着的未实现项」：Fullscreen API、`<dialog>` 状态算法重写与
`::backdrop`、`ToggleEvent.oldState/newState/source`、表单约束校验伪类与 IDL（`:valid` /
`:invalid` / `:in-range` / `:out-of-range` / `:user-valid` / `:user-invalid`）、`:target` 语义、
伪元素名称表一致性、worker 脚本加载接线等——提交范围 `667c7ae`…`54d2888`。这些能力的清单以
**代码与测试**为准（`engine/css/selectorchecker.go`、`engine/js/bindings/`、`engine/html5/`、
`engine/style/validity.go`，测试名即清单），不在文档里维护第二份。

## 未实现项的取舍结论（2026-09 复核）

对文档与代码里残留的未实现项逐条复核。**除「保留为已知边界」者外，其余判定为不需要**
（依据：无业务驱动、无实测瓶颈证据、收益过低或非本仓库职责），条目已从文档移除：

| 项 | 结论与依据 |
|----|-----------|
| 布局增量 **C2**（IFC 行级增量重排） | **不做**。C1（`ae5d64f`）已覆盖打字主热路径（text 变更 → 只重排 dirty block）；C2 只再省「block 内一次 IFC 重排」，收益更小，而风险高（等价于移植 WebKit LineLayout 增量）。当前**没有 profile 数据显示「整 block 重排」是瓶颈**——出现该数据再立项。 |
| `::part` 多 part-name 匹配改哈希集合 | **不做**。原调研结论为收益 <1%，却要新增一套索引与失效维护。 |
| canvas 补丁脚本预编译（`RunJS` → `Compile` + `RunProgram`） | **不做**。`applyCanvas2DPatch` 每次只跑一段 2KB 脚本且脚本内有 `__canvasPatched` 守卫；未采样证明其占比，不值得为它动 `EvalJS` 路径。 |
| canvas `getContext('2d')` 包装改 Go 原生回调 | **不做**。同上，且要改 `wrapDocument` 的 tag 分派，影响面大于收益。 |
| goja 引擎热点（`structuredClone` 原生化、`formatConsoleArg` 跳过对象序列化） | **不做**（保留方法论）。goja 慢是纯解释器的架构性代价；真实热点必须先 `goja.StartProfile` 采样定位（`dev/probes/gobench` 可复现大 bundle 编译耗时），凭猜测改热点没有依据。 |
| host-selector 前缀跨 shadow boundary 前向匹配 | **已实现**，无需立项。`selectorchecker.go` 里 compound 结尾是 `::part` / `::slotted` 时，前缀按 CSS Scoping L1 走宿主组合祖先（该文件注释有说明）。 |
| 多层 `background-image` 叠加 | **不做**。引擎只绘制第一层（`engine/rendering/backgroundimage.go` 注释已载明）；「第一层的解析必须正确」已修复并锁测试（`backgroundurl_layers_test.go`）。 |
| `<input>` / `<textarea>` 引入 dirty value flag | **不立项**。影响面大（牵动渲染取值、表单提交、配置面板三条取值路径），收益只在「无 `min` 的控件 + 用户输入 + step 校验」组合下显形，而规范推荐的写法（带 `min`）不受影响。 |
| bundle 缩小（code splitting / manualChunks / 依赖裁剪） | **非本仓库职责**。bundle 由宿主前端（gou-ide 的 web-ui 构建）产出。编译缓存（`307da7b`）已消掉「重复导航」的 ~600ms 编译开销，首次加载成本归前端工程。 |

**保留为已知边界**（有意不做，逐项见下文各节）：Worker 的 module / SharedWorker / Blob URL /
真实网络、popover 的 top layer / close watcher / 键盘激活 / `CommandEvent`、`:autofill` /
view-transition 伪元素、模式不可热切换、`ui` 包不做声明式响应式等。

## 附：WebSocket —— 已完善（非遗留问题）

`engine/js/bindings/dom.go:1065-1190` 的 WebSocket 已是**完整 stub**：提供 readyState 常量、
`onopen/onmessage/onerror/onclose`、`send/close/addEventListener/removeEventListener`，
并通过 `globalThis.__desktopWS.dispatchMessage/dispatchStatus` 供宿主注入事件。这是
桌面端「无真实网络」场景的**有意设计**（不建连接、不崩溃、事件由宿主推入），
不是 bug。有真实传输需求时在宿主注入层覆盖 `window.WebSocket` 即可，无需改引擎。

---

## 已知差异与有意保留的边界

### Worker 的已知差异（有意保留）

- **消息是 JSON 克隆，不是结构化克隆**：`Date` 变 ISO 字符串、`Map`/`Set`/`RegExp`/`TypedArray` 变 `{}`、值为 `undefined` 的成员被丢弃、`NaN`/`Infinity` 变 `null`。函数、Symbol 与循环引用抛 `DataCloneError`（这点与浏览器一致；算法即 `JSON.stringify`/`JSON.parse`）。
- **无 transferable / SharedArrayBuffer**：`postMessage(msg, [buf])` 的第二个参数被忽略——既不做所有权转移，也不报错。
- **无 MessageChannel / MessagePort / BroadcastChannel**：`MessageEvent.ports` 恒为空数组。
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

