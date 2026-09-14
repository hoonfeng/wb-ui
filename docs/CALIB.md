# 渲染校准（wb-ui vs obscura）

cssprobe 夹具的期望值来自参考实现（obscura）的渲染结果。当某个期望看起来与标准
CSS 矛盾、或想量化「通过检查」与「视觉一致」的差距时，用本页的工具直接对照两个
引擎的实际像素，而不是盯着单个检查项的 `want` 猜。

## 工具

| 工具 | 用途 |
|------|------|
| `dev/probes/calib` | 同一夹具分别用 wb-ui 与 obscura 渲染，输出差异像素占比、差异区域排名、可选差异图（绿=仅 wb-ui、红=仅 obscura） |
| `dev/probes/tddiag` | 并排打印元素的 computed display 与布局几何——定位「谁把宽度/高度写坏了」 |
| `dev/suites/cssprobe` | 夹具 + 期望检查（`-v` 明细、`-tree` 渲染树、`-dump` PNG） |

用法：

```bash
go run ./dev/probes/calib -fixture dev/suites/cssprobe/fixtures/tables.html -obscura <path/to/paint_file> -top 6 -out /tmp/diff.png
go run ./dev/probes/tddiag -file dev/suites/cssprobe/fixtures/fixed-table-layout.html -depth 7
go run ./dev/suites/cssprobe -v -filter 'table-row-geometry'
```

## obscura 构建状态

- Rust 工具链就绪（`cargo 1.98.1` / `rustc 1.98.1`）。
- `cargo build --release`（整仓）在 **v8 crate 失败**：预编译静态库
  `target/release/gn_out/obj/rusty_v8.lib` 归档损坏
  （`invalid archive member at offset 169853782 with size 306867 exceeds archive size`）。
  修复需重新获取 rusty_v8 预编译产物（网络），与渲染算法无关。
- **渲染侧不依赖 v8**，离线绘制入口可直接使用（已构建）：

  ```bash
  <obscura>/target/release/paint_file.exe <in.html> <out.png> [width] [height] [base_url]
  ```

  obscura 是**外部参考实现**（Rust），不属于本仓库；`dev/probes/calib` 用
  `-obscura <该二进制路径>` 指定它（必填参数）。

## 一致性基线（900x1000 视口，差异像素占全图比例）

| 夹具 | 差异 | 差异形态 |
|------|------|----------|
| logical-borders | **0.000%**（0px） | 逐像素一致。此前为 0.006%（58px），全部落在方块1 右下角 (190,74)-(199,79)：**相邻边框拐角的颜色归属方向写反了**——bottom 侧的两个角把「自家外边缘所在的三角」填到了对面，右边框色块因此被底边框整行切断。已按 Edge 实测基准修复，见下文「边框拐角的方向」 |
| forced-line-breaks | 0.222% | (600,10)-(699,39)：flex column 里 `<br>` 产生的空行高度（wb-ui 给 0，参照按继承行高给一行） |
| right-float-navigation | 0.267% | (64,0)-(143,29)：右侧 float 之后的第 2 个 inline-block 没有留在同一行 |
| table-row-geometry | 0.255% | 行高/单元格内容的细节差（文本位置为主） |
| tables | 3.956% | **主要是文本位置差 1-2px**（字体度量差异）叠加 T3/T5 的色块尺寸差；该夹具 cssprobe 检查**全部通过** |
| flex-post-ratio-cross-size | 0.316%（2840px） | 集中在 y=296..472 的整行边缘：`#row` 高度 302.3 的**亚像素高度**（aspect-ratio 推得）与参照取整不同，且 `#visual` 底部 6px 色带的边界随之下移半像素；cssprobe 检查 4/4 通过 |
| video-poster | 0.038%（340px） | 全部落在 `#positioned`（20,150)-(119,249)：`border-radius:20px` + `opacity:.5` 的圆角/半透明合成差；poster 的本体、object-fit:cover、object-position:right 与其余两项检查一致 |
| inline-block-flex-items | 0.012%（105px） | 单像素级边缘（`#brand` 右缘 382、`#links` 左缘 525 的 1px 边界）；该项的 flex/table 结构检查 3/3 通过 |
| flex-whitespace-items | 0.000%（0px） | 与参照逐像素一致（direct flex sidebar 225x180@(0,0)、body 675x200@(225,0)） |

## 结论

1. 夹具期望可用 obscura 原样复现 → 期望值可信。`logical-borders` 的拐角期望
   曾被误判为「参照不斜切、与标准 CSS 相左」，实际是 **wb-ui 的底部两个角把
   三角填到了对面**：用 Edge headless 实测同一页面（四角矩阵见「边框拐角的方向」），
   Edge 与 obscura 的方向一致，wb-ui 相反。修复绘制后该夹具 6/6、与参照
   **0.000%**（逐像素一致）。教训：参照与「标准」冲突时先查第三方实现（Edge），
   不要凭对标准的记忆下结论。
2. `tables` 的 3.96% 差异说明：**cssprobe 的色块检查通过 ≠ 视觉一致**（检查只覆盖
   若干色块位置/尺寸）。后续可把 `dev/probes/calib` 的差异占比纳入回归基线，作为整体
   一致性的补充指标。
3. 文本位置差 1-2px 属字体度量范畴（hinting/行高取整），非结构性布局错误。
4. cssprobe 现状：**64/64 夹具、264/264 检查**（默认 `-scripts auto`，见下文
   「探针的脚本执行模式」）；`-scripts off`（纯 CSS 管线）为 54/64、248/264，
   保留为对照基线——两者之差就是「必须执行脚本才能满足契约」的夹具集合。
   **没有剩余失败夹具**：此前的 3 项（`logical-borders` 拐角方向、
   `media-text-track`、`modern-streams`）已分别通过修正边框绘制、补
   TextTrack/WebVTT、补 Streams 实现解决。

   复核记录（2026-09 批次）：新增 Fullscreen API、`:modal` / `:open` /
   `:closed` 伪类、`<dialog>` JS 接口与 `::backdrop` 遮罩后，cssprobe 复测仍为
   **61/61 夹具、248/248 检查**，无回归。这批改动的验收落在单元测试上：
   `engine/js/bindings/fullscreen_test.go`、`engine/js/bindings/dialog_test.go`、
   `engine/css/fullscreen_selector_test.go`、`engine/css/modal_selector_test.go`、
   `engine/css/openstate_selector_test.go`、`engine/html5/fullscreen_ua_test.go`、
   `engine/rendering/dialog_backdrop_test.go`（结构中无 DOM 节点的 `::backdrop` 盒 +
   像素级的 10% 黑遮罩压暗）。
   复核记录（2026-09 批次·续）：随后的 `<dialog>` 状态算法重写（`open` 纯反射、
   `beforetoggle`、`toggle`/`close` 同批派发）、`PseudoElementName` 补
   `-webkit-scrollbar{,-thumb,-track}`、`:default` / `:indeterminate` 表单状态
   伪类、flex/grid 容器里的 `::backdrop` 三批改动，cssprobe 复测同样为
   **61/61 夹具、248/248 检查**。对应单测：`engine/js/bindings/dialog_test.go`（`open`
   纯反射 + 事件顺序 + `<details>` toggle）、`engine/css/selector_test.go`（枚举↔名称表
   ↔查询表三向往返、`:modal`/`::backdrop` 解析分流）、
   `engine/css/formstate_selector_test.go`、`engine/js/bindings/formstate_test.go`、
   `engine/layout/box_backdrop_test.go`（flex/inline-flex/grid/inline-grid 四种容器）、
   `engine/rendering/dialog_backdrop_test.go`（flex 场景 + `LayoutBox()` 链接断言 + 像素）。
   复核记录（2026-09 批次·续 2）：再补 `:target` 语义修正与 `ToggleEvent`
   （`toggle`/`beforetoggle` 带 `oldState`/`newState`）两批后，cssprobe 仍为
   **61/61 夹具、248/248 检查**（`:target` 此前会误匹配所有带 id 的元素，UA 表
   没有 `:target` 规则，故不影响夹具）。单测：`engine/css/selector_test.go` 的
   `TestSelector_TargetPseudoClass`、`engine/js/bindings/toggleevent_test.go`。

   复核记录（2026-09 批次·续 3，约束校验）：新增夹具 `constraint-validation`
   后为 **62/62 夹具、255/255 检查**。该夹具是本轮唯一新增的像素资产：

   - 6 个色块断言，每块对应一条约束校验判定（`:valid` / `:invalid` /
     `:in-range` / `:out-of-range` / readonly 被排除在约束校验之外 /
     `setCustomValidity` 后重新匹配），位置尺寸按 ±1px 断言、颜色精确匹配。
   - 第 7 块断言「未交互的无效控件不匹配 `:user-invalid`」——它用特化规则
     （`#id:invalid` 保持默认底色、`#id:user-invalid` 才变粉红）把「`:invalid`
     与 `:user-invalid` 的差别」变成可断言的像素差。
   - **反向验证**：把这批新代码里 html5 的 `css.SetFormValidityResolver` 注入
     临时注释掉再跑，6 项检查中 5 项立即失败（唯一仍通过的「readonly 保持默认
     底色」本就不依赖伪类命中），确认夹具真正盯着注入链路，而不是颜色巧合。
     恢复后重新通过。

   ⚠️ 本机 `msedge.exe --headless --dump-dom/--screenshot` 在本轮全程无输出
   （进程 exit 0 但 stdout/stderr/截图全空，`--user-data-dir` 与 `--headless=new`
   都不行），因此这批约束校验的行为依据是规范与 MDN（HTML §4.10.21、
   MDN `:in-range` / `:user-valid` / `readonly` / `willValidate`），**没有**做
   Edge 对照。两处需要留意的判断：空值不越界但匹配 `:in-range`（规范 `:in-range`
   定义只要求「有范围限制且不 underflow/overflow」）、barred 清单（disabled、
   input/textarea 的 readonly（仅支持 readonly 的类型）、input type=hidden/
   reset/button、button type=reset/button、datalist 后代）。待 Edge 恢复后应补
   真值表对照。

   复核记录（2026-09 批次·续 4，step / user validity 焦点会话 / ToggleEvent.source）：
   **夹具与检查数不变**（62/62、255/255）——这三项都不产生新的像素资产：
   `:user-valid` / `:user-invalid` 的焦点会话规则、`ToggleEvent.source` 与日期类的
   step mismatch 都不改变静态夹具的初始渲染（step 只在值不满足相位时才翻转
   `:valid`/`:invalid`，而夹具里的值都满足相位；`source` 只存在于事件对象上）。
   回归靠单测覆盖：html5 的 step / week / user validity 用例矩阵、bindings 的
   `ToggleEvent.source` 端到端（区分 null 与 undefined）、dom 的焦点钩子契约、
   webkit 与 app 的真实输入路径。

   浏览器对照情况：`msedge.exe --headless` 仍然无输出，因此 step / week 的行为依据
   换成了三件更强的证据——① WHATWG HTML 原文（§4.10.5.3.8 的 allowed value step 与
   step base 算法逐条比对，各 type 状态的 default step / step scale factor 也按原文
   取值：date 1 天、month 1 月、week 1 周（default step base −259,200,000 ms）、
   time 与 datetime-local 60 秒）；② WPT
   `html/semantics/forms/constraints/form-validation-validity-stepMismatch.html`
   的用例（date `1970-01-03` vs `1970-01-02`、month `1970-03` vs `1970-04`、
   week `1970-W03` vs `1970-W04`、time/datetime-local `…:02` vs `…:03`、
   number `3` 在 step=2 下 mismatch）——它们与 WPT validator 的 `ctl.value = …`
   （IDL 赋值 ⇒ 元素没有 value 内容属性 ⇒ step base 回退到 default）一起解释了
   「为什么这些用例的 base 不是 value」；③ Blink 的 `InputType::FindStepBase`
   （min → value 内容属性 → default），确认「value 内容属性可作为 step base」不只是
   纸上规定。

   week 的解析修复另有一条可直接复现的证据：Go 的布局 `2006-W02` 里的 `02` 是
   「月中的第几天」，`time.Parse("2006-W02", "1970-W03")` 返回 1970-01-03——用标准库
   就能演示早期实现为何把 `1970-W01…W04` 读成同一周。真值表见
   `engine/html5/week_test.go`。

   复核记录（2026-09 批次·续 5，popover）：新增夹具 `popover` / `popover-backdrop`
   后为 **64/64 夹具、264/264 检查**（`852081a`）——这是本页记录链上
   「62 → 64」的那一步。两个夹具与 Fullscreen / `<dialog>` 的 `::backdrop` 同属
   「顶层 UI 遮罩」路径：普通页面零新增节点，只在 popover / 模态 dialog 存在时
   多一个渲染对象与布局盒。两者都**依赖脚本**调用 `showPopover()`，因此在
   `-scripts off` 下失败，属「必须执行脚本才能满足契约」的夹具集合
   （这也是 `-scripts off` 的夹具分母从 61 变为 64 的构成）。检查数 +9 的构成 =
   `popover` 7 条 + `popover-backdrop` 2 条（明细见 `docs/TECH_DEBT.md` 的
   「Popover API 的像素夹具」）。
5. 修复脉络：`aspect-ratio`/flex 外盒/空 inline-block/`<video poster>`（4 项）→
   探针脚本执行模式 + 5 项 DOM/API 缺口 → 本轮 3 项（顶角修复、媒体轨道、流）。
   落点见「脚本模式下修复的夹具」与下文各节。
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

## 曾记为「探针边界」、现已补齐的 3 项

判别「探针边界 vs 真缺口」的方法（仍然有效）：看失败的实际像素是不是夹具里
**另一条静态规则**的颜色（`closest` 会直接指出）⇒ 脚本没跑到那一步；再用
`dev/probes/scriptsdiag` 逐项列出脚本可见的平台 API，确认缺的是哪一类能力。

复核后修正了一个判断错误：**缺失的 Web API 会让整段脚本抛 `ReferenceError`
中断**，而不是只让那一行失效——因此「媒体/Streams 子系统的缺失」同样会表现为
渲染失败，不能简单归为「不属 CSS 引擎范围」。这 3 项现已在引擎内实现，夹具
全部通过：

## 探针的脚本执行模式（-scripts）

`dev/suites/cssprobe` 默认对含 `<script>` 的夹具走 **WebView 路径**（`webkit.NewWebView`
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
  而参照实现的文本度量恰好等同「未加载系统字体」的 wb-ui（`dev/probes/calib` 实测
  `right-float-navigation` 差异 **0.000%**，逐像素相同）。脚本夹具若先跑，后续纯
  CSS 夹具的几何会被改写（实测 `font-metric-line-height` 行盒偏 40px、
  `right-float-navigation` 偏 4px、`table-row-geometry`/`table-track-geometry`
  偏 1-3px）。字体管理器没有回滚 API，故用排序把这种跨夹具状态污染限制在尾部。

| 夹具 | 曾经的性质 | 实现落点 |
|------|------------|----------|
| `media-text-track` | `<track>`/TextTrack 模型缺失 | `engine/js/bindings/media.go`（`HTMLTrackElement.track` 同元素同实例、`video.textTracks` live 列表、`TextTrack`/`TextTrackCueList`/`TextTrackList`/`VTTCue`、`load`/`error` 事件）+ `engine/js/bindings/webvtt.go`（WebVTT 解析、`data:` URL 解码）；单测 `engine/js/bindings/media_test.go`（含「track 元素与 video.textTracks[i] 是同一对象」的标识契约） |
| `modern-streams` | ReadableStream/TransformStream/TextEncoderStream 缺失 | `engine/js/jsc/streams.go`（ReadableStream + controller、`getReader().read()` 返回 Promise、`pipeThrough`/`pipeTo`、TransformStream + TextEncoderStream/TextDecoderStream）；单测 `engine/js/jsc/streams_test.go`（形状即夹具脚本：分块 + flush + await 读回） |
| `logical-borders` | 拐角颜色归属方向 | `engine/rendering/painter.go:paintBorderCorners`（bottom 侧两角改填「含该边外边缘」的三角）；单测 `engine/rendering/border_corner_direction_test.go`（四角方向 + 回归点）；基准见下节 |

### 脚本模式下修复的夹具

| 夹具 | 缺口 | 落点 |
|------|------|------|
| `eventtarget-lifecycle` | 仅缺脚本执行（`class LifecycleTarget extends EventTarget` + `new Event` + 自定义属性） | 引擎侧 `EventTarget`/`Event`/`dispatchEvent` 早已实现：`engine/dom/event_test.go:TestDispatchEventTargetPhase`、`engine/js/bindings/dom_test.go:TestDOMAddEventListenerAsMethodCall` 等 |
| `flex-flow` | 仅缺第 11 项「CSS supports accepts only the shorthand grammar」——由脚本查询 `CSS.supports()` | `bindings` 的 `CSS.supports` 已有实现；布局 10 项此前已通过（`engine/css/values_test.go:TestParseFlexFlow` 覆盖语法） |
| `animation-fill-forwards` | 探针不推进动画时钟（`WebView.Render` 不应用动画） | 见上文「动画时钟」；`engine/rendering/animation_test.go:TestAnimateVisibilityFillForwards`（`fill:forwards` 保持终帧 + visibility 离散插值） |
| `viewport-consistency` | 页面脚本读 `visualViewport.width/height` 抛 `ReferenceError`，**整段脚本中断**（不是只有那一行失效） | 新增 `window.visualViewport`（CSSOM View §4.2）：宽高与 `innerWidth/innerHeight` 同源（按解释器分派）、`scale=1`、offset/page=0、事件方法 no-op（`engine/js/bindings/dom.go`）。另：前两项媒体查询由 `RenderView.SetViewportSize`/`Frame.syncMediaQueryViewport` 修复，`engine/rendering/mediaquery_viewport_sync_test.go:TestMediaQueryViewportSync` 覆盖 |
| `modern-hydration-contracts` | React 19 水合契约：`document.currentScript`、`el.attributes instanceof NamedNodeMap` + **live 集合**、`removeAttributeNode`、`hasAttributes`、`scrollTo({left,top,behavior})`/`scrollBy` | `engine/js/bindings/dom.go`（`NamedNodeMap` 构造器 + 同元素同实例的 live 集合缓存 `namedNodeMapFor`、`hasAttributes`/`removeAttributeNode`、`Element.prototype.scrollTo`/`scrollBy`）、`engine/page/frame.go`（脚本执行期间设置 `bindings.CurrentScriptElement`，结束恢复）、`engine/rendering/scrollbargeom.go` + `webkit`（新增 `ScrollRange`：可滚动性判定不再用滚动条几何——10×10 的 `overflow:scroll` 容器此前被静默丢弃 `scrollTop` 赋值） |

### 同轮补齐的平台能力夹具（媒体轨道与流）

| 夹具 | 缺口 | 落点 |
|------|------|------|
| `media-text-track` | `<track>` → `HTMLTrackElement.track`（WebVTT + `data:` URL 异步加载 + `load` 事件）与 `video.textTracks` 缺失，脚本读 `element.track` 得 `undefined` → 抛错中断 | `engine/js/bindings/media.go`（同一 `<track>` 元素同一 TextTrack 实例；`video.textTracks` 为 live 列表，`textTracks[i]` 与 `<track>.track` 是**同一对象**）+ `engine/js/bindings/webvtt.go`（WEBVTT 头 / cue 块 / 时间戳 / settings、percent 与 base64 的 `data:` 解码）。单测 `engine/js/bindings/media_test.go`（6 项，含 live 列表、`load`/`error` 时序、VTTCue 构造） |
| `modern-streams` | `ReadableStream` / `TransformStream` / `TextEncoderStream` 均为 `undefined` | `engine/js/jsc/streams.go`：构造器 + 原型、`enqueue/close/error` 控制器、`getReader().read()` 返回 Promise（队列空时挂起）、`pipeThrough`/`pipeTo` 直接接线（无背压）、`TextEncoderStream`/`TextDecoderStream` 复用 Go 侧 `TextEncoder`/`TextDecoder`。单测 `engine/js/jsc/streams_test.go`（3 项） |

### 尺寸媒体查询的视口同步（本轮）

`@media` 的 width/height 之前按 **0×0** 求值——`style.Resolver.mediaQueryCtx`
只在被显式设置时才有值，而真实链路（`engine/page/frameview.go` → `RenderView`）从未
设置它。于是 `@media (min-width: 600px)` 恒不匹配、`@media (max-width: 950px)`
恒匹配（0 ≤ 950），所有尺寸媒体查询都落在错误分支上。

修复：`Resolver.SetViewportSize` 变化时清 ComputedStyle 缓存并报告变化；
`RenderView.SetViewportSize` 与 `Frame.syncMediaQueryViewport`（渲染树 Build
之前调用）把它与真实视口同步；探针侧在 Build 之前显式设置。

已知取舍：视口尺寸变化**不会**主动标记渲染树重建（尺寸抖动会让 iframe 子文档
每帧重建，滚动偏移与跨 frame 选区失效）。因此 resize 后的媒体查询分支要等
下一次渲染树重建（DOM 变更/样式变更触发）才切换，而首帧加载与重建路径已正确。

## 边框拐角的方向（Edge 实测基准）

相邻两边颜色不同时，拐角矩形被「外角 → 内角」的对角线分成两半，**每条边的
颜色占据含该边外边缘的那一半**。用 Edge headless 渲染一个四角隔离页（每个盒子
只给两条相邻边框上不同颜色）实测得到（对角线上的像素是抗锯齿混合色，归属任意）：

| 角 | 水平边（top/bottom）颜色所在 | 垂直边（left/right）颜色所在 |
|----|------------------------------|------------------------------|
| top-left | 右上三角 | 左下三角 |
| top-right | 左上三角 | 右下三角 |
| bottom-left | 右下三角 | 左上三角 |
| bottom-right | 左下三角 | 右上三角 |

wb-ui 此前 top 侧两角正确、**bottom 侧两角填到了对面**：`fillBelow`/`fillBelowRev`
用在 bottom 角时把「含 left/right 外边缘」的三角判成了另一半，于是相邻边框颜色
互换。修复后四角方向与 Edge 一致（残差只剩对角线 1px 阶梯的抗锯齿偏置），
`logical-borders` 夹具 6/6、与 obscura **0.000%**（此前 0.006%，58px 集中在
方块1 右下角）。

复现与锁定：`dev/suites/cssprobe -filter logical-borders`（夹具期望的右边框 10x79、
底边框 198x6 正是该方向的结果）；`engine/rendering/border_corner_direction_test.go`
锁定四角方向与 6 个回归采样点（把填充改回旧写法即失败）。

## 探针的帧循环（事件循环 + 重建 cooldown）

脚本夹具断言的是**异步工作收敛后**的状态，而探针没有宿主帧循环，因此
`renderFixtureWithScripts` 在取图前显式推进两件事：

1. **JS 事件循环**（`eventLoopTurns`，默认 25 轮）：`EventLoop.ProcessTasks(0)` +
   `Interpreter.RunJobs()`，直到 `PendingTasks()==0`。覆盖 Promise/await 水合链、
   `setTimeout` 延迟写入、资源事件（`<track>` 的 `load`/`error`）。
2. **帧轮次**（`frameTurns`，默认 8）：`WebView.EnsureLayout()` 直到
   `NeedsLayout()` 与 `NeedsRenderTreeRebuild()` 同时为假。**这一步是必需的**：
   DOM 变更触发的渲染树重建被 `Frame.rebuildCooldown`（逐帧递减的变更风暴降频
   计数）推迟，而 `FrameView.Layout()` 在「重建仍挂起」时**直接 return、不做
   布局**——只调一次 `EnsureLayout` 会停在「树未布局」状态（`html`/`body`/`div`
   全是 0x0，画布全白）。宿主 `app.Host` 每帧都走一遍，所以线上不显现；探针必须
   自己补上。

两者都有轮数上限，避免「每帧变更 DOM」的页面或自续期定时器把探针挂住。
