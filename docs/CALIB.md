# 渲染校准（wb-ui vs obscura）

cssprobe 夹具的期望值来自参考实现（obscura）的渲染结果。当某个期望看起来与标准
CSS 矛盾、或想量化「通过检查」与「视觉一致」的差距时，用本页的工具直接对照两个
引擎的实际像素，而不是盯着单个检查项的 `want` 猜。

## 工具

| 工具 | 用途 |
|------|------|
| `dev/calib` | 同一夹具分别用 wb-ui 与 obscura 渲染，输出差异像素占比、差异区域排名、可选差异图（绿=仅 wb-ui、红=仅 obscura） |
| `dev/tddiag` | 并排打印元素的 computed display 与布局几何——定位「谁把宽度/高度写坏了」 |
| `dev/cssprobe` | 夹具 + 期望检查（`-v` 明细、`-tree` 渲染树、`-dump` PNG） |

用法：

```bash
go run ./dev/calib -fixture dev/cssprobe/fixtures/tables.html -top 6 -out /tmp/diff.png
go run ./dev/tddiag -file dev/cssprobe/fixtures/fixed-table-layout.html -depth 7
go run ./dev/cssprobe -v -filter 'table-row-geometry'
```

## obscura 构建状态

- Rust 工具链就绪（`cargo 1.98.1` / `rustc 1.98.1`）。
- `cargo build --release`（整仓）在 **v8 crate 失败**：预编译静态库
  `target/release/gn_out/obj/rusty_v8.lib` 归档损坏
  （`invalid archive member at offset 169853782 with size 306867 exceeds archive size`）。
  修复需重新获取 rusty_v8 预编译产物（网络），与渲染算法无关。
- **渲染侧不依赖 v8**，离线绘制入口可直接使用（已构建）：

  ```bash
  ref/obscura/target/release/paint_file.exe <in.html> <out.png> [width] [height] [base_url]
  ```

  `dev/calib` 默认调用该二进制（`-obscura` 可换路径）。

## 一致性基线（900x1000 视口，差异像素占全图比例）

| 夹具 | 差异 | 差异形态 |
|------|------|----------|
| logical-borders | **0.006%**（58px） | 仅方块1 右下角 (190,74)-(199,79)：**相邻边框拐角的处理**——wb-ui 按 CSS 标准做 45° 斜切，obscura 各边独立矩形。夹具那两项期望（右边框 10x79、底边框 198x6）就是「不斜切」的结果 |
| forced-line-breaks | 0.222% | (600,10)-(699,39)：flex column 里 `<br>` 产生的空行高度（wb-ui 给 0，参照按继承行高给一行） |
| right-float-navigation | 0.267% | (64,0)-(143,29)：右侧 float 之后的第 2 个 inline-block 没有留在同一行 |
| table-row-geometry | 0.255% | 行高/单元格内容的细节差（文本位置为主） |
| tables | 3.956% | **主要是文本位置差 1-2px**（字体度量差异）叠加 T3/T5 的色块尺寸差；该夹具 cssprobe 检查**全部通过** |
| flex-post-ratio-cross-size | 0.316%（2840px） | 集中在 y=296..472 的整行边缘：`#row` 高度 302.3 的**亚像素高度**（aspect-ratio 推得）与参照取整不同，且 `#visual` 底部 6px 色带的边界随之下移半像素；cssprobe 检查 4/4 通过 |
| video-poster | 0.038%（340px） | 全部落在 `#positioned`（20,150)-(119,249)：`border-radius:20px` + `opacity:.5` 的圆角/半透明合成差；poster 的本体、object-fit:cover、object-position:right 与其余两项检查一致 |
| inline-block-flex-items | 0.012%（105px） | 单像素级边缘（`#brand` 右缘 382、`#links` 左缘 525 的 1px 边界）；该项的 flex/table 结构检查 3/3 通过 |
| flex-whitespace-items | 0.000%（0px） | 与参照逐像素一致（direct flex sidebar 225x180@(0,0)、body 675x200@(225,0)） |

## 结论

1. 夹具期望可用 obscura 原样复现 → 期望值可信；但 `logical-borders` 的拐角期望
   与标准 CSS 相左（参照不斜切）。以参照为准会牺牲标准化能力，**暂不改绘制**，
   该夹具固定为 4/6。
2. `tables` 的 3.96% 差异说明：**cssprobe 的色块检查通过 ≠ 视觉一致**（检查只覆盖
   若干色块位置/尺寸）。后续可把 `dev/calib` 的差异占比纳入回归基线，作为整体
   一致性的补充指标。
3. 文本位置差 1-2px 属字体度量范畴（hinting/行高取整），非结构性布局错误。
4. cssprobe 现状：**58/61 夹具、244/248 检查**（默认 `-scripts auto`，见下文
   「探针的脚本执行模式」）；`-scripts off`（纯 CSS 管线）为 53/61、239/248，
   保留为对照基线。剩余 3 个夹具 = 1 个参照差异（`logical-borders`：参照
   「不斜切」与标准 CSS 相左，固定 4/6，58px）+ 2 个 Web 平台子系统缺失
   （`media-text-track`、`modern-streams`，见「夹具限制」表）。
5. 上一轮（aspect-ratio / flex 外盒尺寸 / 空 inline-block / `<video poster>`）
   修复 4 项，本轮脚本模式再修复 5 项（落点见「脚本模式下修复的夹具」）；
   一致性数字见上表 4 行。
   - `aspect-ratio`：flex item 交叉轴由主轴推出（`resolveCrossSizes` +
     `ratioCrossSizeInRowFlex` 供容器 auto 高度估算）、absolute 盒 `height:auto`
     由宽度推高（`layoutAbsolute`）、BFC auto 高度分支同样按比例兜底。
   - flex 主轴写入统一按「外盒尺寸 → content」扣 padding+border：
     `finalMainSize` 与非 border-box 项的 `paddingMain`（inline-block flex item
     外盒 414 → 382）。
   - 无文本的 inline-block 用 `intrinsicContentWidth` 作 shrink-to-fit 宽
     （`#chips` 30 → 90，三个 inline-block li 由重叠变横排）。
   - `<video poster>`：poster 走 `PaintImage`，SVG 资源矢量路径也遵循
     object-fit / object-position 并裁剪到内容盒，absolute + width:auto 的替换
     元素取资源固有宽。

## 夹具限制（探针能力边界，非引擎缺口）

判据：期望色是否由**页面脚本或媒体 API** 产生，且该能力是否落在 CSS 渲染引擎
（探针与 wb-ui 的定位）之外。下列 3 项在本探针内不可达，失败原因是探针边界，
**不记为渲染能力缺口**。判断「是探针边界还是真缺口」的方法：期望色是否由页面
脚本/媒体 API 产生（`closest` 报出的实际像素恰好是夹具里另一条静态规则的
颜色 ⇒ 脚本没跑），以及缺失的是否是 CSS 引擎职责（`dev/scriptsdiag` 逐项列出
脚本可用的平台 API）。

## 探针的脚本执行模式（-scripts）

`dev/cssprobe` 默认对含 `<script>` 的夹具走 **WebView 路径**（`webkit.NewWebView`
→ `Resize` → `LoadHTML` → `Render`），其余夹具走纯 CSS 管线；`-scripts off` 强制
全部走纯 CSS 管线（历史基线），`-scripts on` 强制全部走 WebView。

- 为什么要两条路径：夹具期望值来自参照实现（obscura/Chromium），其中一部分夹具的
  行为**只存在于脚本执行之后**（`classList` 触发动画、`CSS.supports` 报语法支持、
  页面读 `innerWidth`/`visualViewport`、React 19 的属性清空契约）。用纯 CSS 管线
  跑这类夹具，检验的是初始 HTML 而不是夹具的契约。
- 判定标准两条路径相同（精确同色 + 连通块 + ±1px），只是驱动不同：WebView 路径
  接上了 JS 运行时、DOM 绑定与媒体查询视口。
- 合成底色：WebView 从全透明表面开始（`Canvas.Clear(zero)`），参照实现合成到不透明
  白底，故探针把读回像素**预乘合成到白底**（`out = c + (255-a)`；`goskia
  Image.ReadPixels` 返回预乘 RGBA）。
- 动画时钟：有限动画夹具断言的是**动画结束后的终态**（`fill:forwards`），参照截图
  同样在页面稳定后拍摄。探针没有帧循环，因此在取图前把 `rendering.AnimationTime`
  推到 10s 并调用一次 `rendering.ApplyAnimations`（`WebView.Render` 自身不应用
  动画——时钟由宿主 `app.Host` 每帧推进）。
- ★ 执行顺序：含 `<script>` 的夹具**排在最后**。WebView 构造时会初始化字体管理器
  （`webkit.ensureFonts`），装载系统字体后 serif/mono 的 fallback 度量随之变化，
  而参照实现的文本度量恰好等同「未加载系统字体」的 wb-ui（`dev/calib` 实测
  `right-float-navigation` 差异 **0.000%**，逐像素相同）。脚本夹具若先跑，后续纯
  CSS 夹具的几何会被改写（实测 `font-metric-line-height` 行盒偏 40px、
  `right-float-navigation` 偏 4px、`table-row-geometry`/`table-track-geometry`
  偏 1-3px）。字体管理器没有回滚 API，故用排序把这种跨夹具状态污染限制在尾部。

| 夹具 | 探针内不可达的原因 | 引擎侧覆盖方式 |
|------|--------------------|----------------|
| `media-text-track` | 需要 `<track>` → `HTMLTrackElement.track`（TextTrack + WebVTT 解析）与 `video.textTracks`，且 cues 就绪依赖 `data:` URL 的异步加载与 `load` 事件时序；探针没有媒体元素模型。`dev/scriptsdiag` 实测 `trackElement.track`、`video.textTracks` 均为 `undefined` | 未实现（媒体子系统，不属 CSS 引擎范围）。wb-ui 的 `<video>`/camera 挂件走帧注入（`vcam`），不经 DOM 媒体模型 |
| `modern-streams` | 需要 `ReadableStream` + `TransformStream` + `TextEncoderStream`（`pipeThrough` / `getReader()` / Promise 微任务队列，Web Streams 标准）；实测三者均为 `undefined`（`TextDecoder` 已有：`jsc/webapi.go:RegisterWebAPIs`） | 未实现（Web Streams 子系统） |
| `logical-borders` | 4/6：参照（obscura）对相邻边框拐角**不斜切**，wb-ui 按标准做 45° 斜接（差 58px @(190,74)-(199,79)）。曾试「右下角改填 bottom 色」与 FillPath 三角，bbox 更差 | 保持标准化不牺牲，固定 4/6（结论 1） |

### 脚本模式下修复的夹具（本轮）

| 夹具 | 缺口 | 落点 |
|------|------|------|
| `eventtarget-lifecycle` | 仅缺脚本执行（`class LifecycleTarget extends EventTarget` + `new Event` + 自定义属性） | 引擎侧 `EventTarget`/`Event`/`dispatchEvent` 早已实现：`dom/event_test.go:TestDispatchEventTargetPhase`、`bindings/dom_test.go:TestDOMAddEventListenerAsMethodCall` 等 |
| `flex-flow` | 仅缺第 11 项「CSS supports accepts only the shorthand grammar」——由脚本查询 `CSS.supports()` | `bindings` 的 `CSS.supports` 已有实现；布局 10 项此前已通过（`css/values_test.go:TestParseFlexFlow` 覆盖语法） |
| `animation-fill-forwards` | 探针不推进动画时钟（`WebView.Render` 不应用动画） | 见上文「动画时钟」；`rendering/animation_test.go:TestAnimateVisibilityFillForwards`（`fill:forwards` 保持终帧 + visibility 离散插值） |
| `viewport-consistency` | 页面脚本读 `visualViewport.width/height` 抛 `ReferenceError`，**整段脚本中断**（不是只有那一行失效） | 新增 `window.visualViewport`（CSSOM View §4.2）：宽高与 `innerWidth/innerHeight` 同源（按解释器分派）、`scale=1`、offset/page=0、事件方法 no-op（`bindings/dom.go`）。另：前两项媒体查询由 `RenderView.SetViewportSize`/`Frame.syncMediaQueryViewport` 修复，`rendering/mediaquery_viewport_sync_test.go:TestMediaQueryViewportSync` 覆盖 |
| `modern-hydration-contracts` | React 19 水合契约：`document.currentScript`、`el.attributes instanceof NamedNodeMap` + **live 集合**、`removeAttributeNode`、`hasAttributes`、`scrollTo({left,top,behavior})`/`scrollBy` | `bindings/dom.go`（`NamedNodeMap` 构造器 + 同元素同实例的 live 集合缓存 `namedNodeMapFor`、`hasAttributes`/`removeAttributeNode`、`Element.prototype.scrollTo`/`scrollBy`）、`page/frame.go`（脚本执行期间设置 `bindings.CurrentScriptElement`，结束恢复）、`rendering/scrollbargeom.go` + `webkit`（新增 `ScrollRange`：可滚动性判定不再用滚动条几何——10×10 的 `overflow:scroll` 容器此前被静默丢弃 `scrollTop` 赋值） |

### 尺寸媒体查询的视口同步（本轮）

`@media` 的 width/height 之前按 **0×0** 求值——`style.Resolver.mediaQueryCtx`
只在被显式设置时才有值，而真实链路（`page/frameview.go` → `RenderView`）从未
设置它。于是 `@media (min-width: 600px)` 恒不匹配、`@media (max-width: 950px)`
恒匹配（0 ≤ 950），所有尺寸媒体查询都落在错误分支上。

修复：`Resolver.SetViewportSize` 变化时清 ComputedStyle 缓存并报告变化；
`RenderView.SetViewportSize` 与 `Frame.syncMediaQueryViewport`（渲染树 Build
之前调用）把它与真实视口同步；探针侧在 Build 之前显式设置。

已知取舍：视口尺寸变化**不会**主动标记渲染树重建（尺寸抖动会让 iframe 子文档
每帧重建，滚动偏移与跨 frame 选区失效）。因此 resize 后的媒体查询分支要等
下一次渲染树重建（DOM 变更/样式变更触发）才切换，而首帧加载与重建路径已正确。
