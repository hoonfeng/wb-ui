# wb-ui 待工作项（WORKITEMS）

> **本文档是「wb-ui 与 Edge 差异」的单一真相与监督台账。**
> 每项含：现象 / Edge 对照 / 验收标准 / 复现命令 / 产物 / 状态。
> 状态取值：`TODO` / `DOING` / `DONE`（须附实测数据）/ `WONTFIX`（须附证据）。
>
> **监督规则（不可绕过）**
> 1. 判定**只认 Edge headless 实测**，不认「与参考实现一致」这类间接证据；
> 2. 未通过验收标准**不得**标 `DONE`；
> 3. 每项完成后**回填实测数字**（不是「已修复」三个字）；
> 4. 改动后必须跑 `CGO_ENABLED=1 go test ./engine/... -count=1` 与 §0.3 的回归门槛。

---

## 0. 判定基准与方法

### 0.1 三方关系（最关键的一条）

`dev/suites/cssprobe` 的 `checks.json` 期望值来自**外部参考实现**（obscura
render-repros，Apache-2.0），**不等于本机 Edge**。因此必须分别测量三方：

| 命令 | 测量对象 | 用途 |
|---|---|---|
| `go run ./dev/suites/cssprobe -json <out>` | **wbui** vs obscura 期望 | 找出「wbui 偏离参考」的候选 |
| `go run ./dev/probes/cssoracle -browser <Edge> -json <out>` | **Edge** vs obscura 期望 | 区分「期望过时」与「wbui 真 bug」 |

判定规则：

| 情形 | 判定 | 归属 |
|---|---|---|
| Edge = 期望，wbui ≠ 两者 | **wbui 真 bug** | **A 类**（必须修代码） |
| Edge ≠ 期望，wbui = Edge | 期望过时 | **B 类**（改期望，依据须是 Edge 实测） |
| Edge ≠ 期望，wbui = 期望（wbui ≠ Edge） | **wbui 与 Edge 不一致** | **C 类**（须判谁对，可能是 obscura 与 Edge 都错） |
| 平台字体度量差异、非可修 UA bug | 已知分歧 | **D 类**（`known_divergence`，须写明两侧数字） |

> ★ **禁止**把期望值改成「wbui 当前的输出」——那会让 oracle 退化成自证基线。
> 只有当**真实浏览器**给出该值时，才允许改期望，并在条目里注明依据。

### 0.2 环境与基准参数（照抄，否则测的不是同一件事）

```bash
export CGO_ENABLED=1
export PATH=/f/syproject/goskia/skia/lib/windows_amd64:$PATH     # libSkiaSharp.dll
EDGE="C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe"
```

> ★ **`CGO_ENABLED=1` 是 `go vet` / `go test` / `cssprobe` / `cssoracle` / `webshot` 的
> 前置条件**（Windows 默认关闭）。依赖链含 `engine/platform/graphics`（skia cgo 绑定），
> 默认关闭时会报 `undefined: skia.*` 之类的编译失败——**不要把该失败误读成代码错误**。
> 复现者若看到 `undefined: skia`，先确认 `CGO_ENABLED=1` 与 `libSkiaSharp.dll` 已在 PATH。

- `--screenshot` 输出路径必须是**绝对路径**，否则 msedge 报「系统找不到指定的路径」；
- `--hide-scrollbars` 不只是不画滚动条，还会让布局**不扣滚动条宽度**——与 wbui 的经典
  滚动条实现产生纯参数差异（见 REPORT §6）；
- `FindBrowser` 默认优先 Chrome，**跑 Edge 基准时必须显式** `-browser "$EDGE"`
  或 `set WBUI_BROWSER=<Edge 路径>`。

### 0.3 回归门槛（改动后必须全部满足）

**参照物 = 冻结的独立 Edge 基准 PNG**（`dev/audit/baselines/edge/<页面>.png`）。
这些 PNG 由**真实 Edge 无头渲染**得到并已入库冻结（见 §P3「冻结 Edge 基准」），
**不是 wbui 自身的输出** —— 用 wbui 输出当基线会让差异率恒为 0（「自证基线」）。

**门槛口径**：逐页计算 `wbui 渲染 vs 冻结 Edge PNG` 的像素差异率，**不得超过**
下表值（表中数值 = 冻结基线冻结时刻记录的差异率，故「≤ 表中值」等价于
「不劣化于冻结基线」）。

| 页面 | 冻结基线（Edge 实渲 PNG） | 差异率门槛（不劣化） |
|---|---|---|
| `webshot/deffont.html` | `baselines/edge/deffont.png` | ≤ 14.62% |
| `framework/vue-app.html` | `baselines/edge/vue.png` | ≤ 2.83% ✅（G8 已关闭） |
| `framework/react-app.html` | `baselines/edge/react.png` | ≤ 3.04% ✅（G8 已关闭） |
| `webshot/fontshort.html` | `baselines/edge/fontshort.png` | ≤ 12.54% |
| `css-stack/stack.html` | `baselines/edge/stack.png` | ≤ 2.56% |
| `css-stack/sticky.html` | `baselines/edge/sticky.png` | ≤ 5.55% |
| `css-stack/zorder.html` | `baselines/edge/zorder.png` | ≤ 0.84% |
| `css-stack/composite.html` | `baselines/edge/composite.png` | ≤ 2.89% |
| `go test ./engine/... -count=1` | — | 无 FAIL |

**容差口径**：差异率比较允许 **+0.05 个百分点**的抖动（亚像素定位 / 抗锯齿
在两次渲染间不稳定，实测同版本重跑存在 ~0.01–0.04pp 波动）。超出 0.05pp
判为**劣化**，必须修复或给出同等硬度的实测理由。差异率**下降**时，应把表中的
门槛值下调到新实测值（基线随之收敛），避免门槛长期虚高。

**对比命令**（绝对路径要求见 §0.2）：

```bash
CGO_ENABLED=1 go run ./dev/probes/webshot -html dev/fixtures/<页面>.html -out /abs/path/<页面>.wbui.png
# 再与 dev/audit/baselines/edge/<页面>.png 逐像素比对，输出差异率
```

> ✅ **vue / react 两行已满足**（G8 于本轮关闭）。根因不是渲染层撑高口径，而是
> **layout 层移位文本段时漏移 `LineY`**：`flexformattingcontext.go` 的
> `shiftBoxAndDescendants` 与 `tableformattingcontext.go` 的 `shiftCellContent`
> 只更新 `X`/`Y`（`positioned.go` 的同名逻辑三件套齐全，可对照）。`LineY` 停在移位前
> 的值 ⇒ 渲染层 `inlineSegBottom`（= `LineY + LineHeight`）的行盒底虚高一个 dy ⇒
> `syncOne` 把无显式高度的 flex item 撑高。详见 §G8 与 REPORT §8.3。

---

## 1. A 类：wbui 真 bug（Edge 与参考一致、wbui 偏离）

### A1. `legacy-center`：行盒高度 +4px（6/8 项失败）— **`DONE`**（2026-10-06）

- **现象**：cssprobe 6 项 FAIL，全部**恰好 +4px**；块盒纵向位置逐级下移
  （y 20→24、40→44、60→64、80→84、100→108）。
- **Edge 对照**：`cssoracle -filter legacy-center` = **MATCH 8/8** —— Edge 与参考期望一致，
  所以这是 wbui 真 bug，不是期望过时。
- **wbui 实测**（`webshot -w 800 -h 600 -eval` 读 `getBoundingClientRect`）：

  | 元素 | wbui | Edge（=期望） | 差 |
  |---|---|---|---|
  | `.inline-box`（`#legacy` 内首行内块） | y=0 h=20 | y=0 h=20 | ✓ |
  | `.block-box`（`height:20px`，无内容） | **y=24** | y=20 | **+4** |
  | `.cell-center`（**显式 `height:20px`** + 文本） | **h=24** | h=20 | **+4** |
  | `#pure-center`（**显式 `height:20px`** + 文本） | **h=24** | h=20 | **+4** |

- **结论（实测定位）**：不是「期望过时」，也不是单一根因，而是**四个独立缺陷**叠加。
  用 `minibox.html` 判别性探针（同一 HTML 双跑 Edge/wbui）+ `WBUI_DEBUG_SCH=1`
  的 `SetContentHeight` 调用者追踪逐层排除，最终全部修复：

  | 缺陷 | 现象（Edge / wbui） | 根因（文件） | 修法 |
  |---|---|---|---|
  | **1. 显式 height 被内容撑大** | a: 20 / **24** | 渲染层 `renderblockview.go` 的 `syncOne` **无条件**把 frame 高度撑到「文本内容底部」（`frame.Height = maxBottom - frame.Y`），覆盖 layout 算对的值 | 加 `!layout.HeightIsDefiniteForBox(lb)` 条件（CSS 2.1 §10.6.3） |
  | **2. 撑高用字形范围而非行盒** | e: 20 / **22** | `syncOne` 用 `seg.Y+seg.Height`（字形 ascent+descent=22.5）而非行盒底 | 新增 `inlineSegBottom()` 取 `LineY+LineHeight` |
  | **3. `line-height:0` 被当成 normal** | c: 20 / **24** | `cssLineHeight` 用 `return 0` 同时表示「normal」与「显式 0」，调用方只能 `if cssLH > 0` → 显式 0 落到字体度量（16px→24） | 签名改 `(float64, bool)`；`line-height:normal` 用 `Unit="normal"` 与显式 0 区分 |
  | **4. inline-block 不计入行盒高** | `.inline-box` 行盒 20 / **0** | IFC 只有 `IsReplaced()` 与 `vertical-align:middle` 参与行盒高；baseline 对齐的 inline-block 完全不参与 | 加分支：baseline inline-block 时 `lineH = max(lineH, margin.Top+childBH+margin.Bottom)` |
  | **5. `font-size:0` 度量未归零** | `.nested-inline` y=40 / **28** | `effectiveFontMetrics` 里 `fs <= 0 → defaultFontSize`(16) → 0 字号仍有 24px 度量，`centeringOffset=(0-24)/2=-12` 把 inline-block 上移 12px | `fs == 0` 时短路返回 `(0,0,0)`（0 字号按定义使字体相对度量为 0） |

- **验收结果**：`cssprobe -filter legacy-center` = **`PASS`（8/8）** ✓✓；
  `cssoracle -filter legacy-center` 仍 **MATCH 8/8**（期望未被改动）✓；
  minibox 探针 a/b/c/e/f 五项与 Edge **逐项一致**（20/24/20/20/20）。
- **残留**：`minibox` 的 d（正常字体 + 空 inline-block）Edge **25** / wbui **24**，差 1px
  → 另立 **A3**。
- **复现**：
  ```bash
  go run ./dev/suites/cssprobe -filter legacy-center -v
  go run ./dev/probes/cssoracle -browser "$EDGE" -filter legacy-center
  go run ./dev/probes/webshot -html dev/suites/cssprobe/fixtures/legacy-center.html \
    -w 800 -h 600 -out dev/output/tmp/legacy.wbui.png -wait 30 \
    -eval "['.block-box','.cell-center','#pure-center'].map(function(s){var r=document.querySelector(s).getBoundingClientRect();return s+':'+r.y+'x'+r.height}).join(' ')"
  ```
- **产物**：`dev/output/tmp/legacy.wbui.png`

### A2. `form-control-quirks`：quirks form 底边距 +4px — **`DONE`** ✓ `PASS(2/2)`

- **现象**：`quirks form keeps legacy one-em bottom margin` 期望 y=37，wbui **41**（修复后 **40**）。
- **Edge 对照**：`cssoracle -filter form-control-quirks` = **MATCH 2/2** → wbui 真 bug。
- **同一缺陷也命中** `form-control-geometry` 的 `standards form contributes only native control height`
  （期望 y=21，wbui 25 → 修复后 24）。根因是 **`<input>` 的边框盒高**：Edge 21 / wbui 25。
- **★ 方法论教训（重要）**：`getComputedStyle(input).fontSize` 读到 **16px 不能当证据** ——
  `engine/js/bindings/dom.go` 的 `uaInitialComputedValues` 是**JS 对象层的初始值回退表**，
  对「未声明属性」一律返回 16px（注释明确说明它不影响布局/继承计算）。**只有几何可信**。
- **已修**：`engine/html5/defaultcss.go` 的 UA 规则原写 `font-family: inherit`，控件因此
  继承文档字体（默认 sans-serif → Noto Sans SC）；而 Chromium 的 UA 规则是
  **font: 400 13.3333px Arial** —— 控件既不继承文档字号、也不继承文档字体族。
  改为 `font-family: Arial` 后 input 边框盒 25 → **24**，且**零回归**
  （deffont 14.62% / vue 2.92% / react 3.14% 与改动前逐位相同）。
- **剩余 3px**：`font-size: 13.3333px` 仍未落到控件（UA 样式表与
  `applyFormControlUserAgentDefaults` 都写了 13.3333，`absolutizeFontSize` 对 px 直接返回
  → 怀疑 cascade 段之后有「继承回填」把 `cs.FontSize` 覆盖成父值 16px）。
  几何证据：Edge 的控件内容盒 **15px**（21 = 15+2 padding+4 border），wbui 修复后仍 ≈18。
- **下一步**：在 `Resolver.ResolveElement` 的 cascade 段（`resolver.go` 660–840）
  追 `cs.FontSize` 的写入者 —— 可复用 `WBUI_DEBUG_SCH` 同款「打印调用者行号」手法。
- **验收标准**：上述两项 PASS 且 `cssoracle` 仍 MATCH（不得改期望）。

#### ✅ A2 最终修复（2026-09）：根因是 **quirks 的行盒 strut**，不是 input 的高度

完整证据链（全部可复现）：

1. `WBUI_IFC_DEBUG=1` → `[ifc-child] INPUT borderBoxH=21.000 contentH=15.000 fs=13.333`
   —— **wbui 的 input 边框盒高 21 完全正确**（参照浏览器 21.2）。即 A2 前半段
   的 `font-family: Arial` 修复 + UA 的 13.3333px **都真实生效**。
2. 同一行打印 `floor(before)=24.000` —— **行盒被父匿名盒的 strut 撑到 24**
   （`<anon> fs=16 family="sans-serif"` → Noto Sans SC 16px = 24.0）。
3. 参照浏览器（**Chrome headless**，即本机 `cssoracle` 实际使用的浏览器 ——
   `probelib` 注释已说明本机 Edge headless 静默失败，查找顺序 Chrome → Edge）：
   - quirks `<form><input size=17></form>` → form 高 **21.2**（= input 边框盒高，**strut 不参与**）
   - standards 同一结构 → **25.8**（= input 21.2 + strut descent 4.6，struts 参与）
   → 结论：**quirks 文档中 strut 的 ascent 与 descent 都不进行盒**。
4. 修复：`engine/layout/inlineformattingcontext.go` 新增 `quirksStrutSuppressed()`；
   quirks 文档中当行内**只有 atomic inline 子盒（replaced / inline-block）且无文本**时，
   基准行高置 0（子盒高成为行盒高）。含文本或非替换行内盒则判定失败、语义不变。
5. 验收：`cssprobe -filter form-control-quirks` = **PASS(2/2)**；`cssoracle` 仍 **MATCH(2/2)**；
   `go test ./engine/... -count=1` 全绿；像素回归 deffont 14.62% / vue 2.92% / react 3.14%
   **逐位不变**。

> ★ **纠正一条把排查带偏的错误线索**：`getComputedStyle(input).fontSize` 读到 16px，
> 曾让人以为「13.3333px 没落地」。实际那是 `engine/js/bindings/dom.go` 的
> `uaInitialComputedValues` —— JS 对象层的初始值回退表（未声明属性一律返回 16px）。
> **只有 `WBUI_IFC_DEBUG` 打印的几何可信。**

### A4. `vertical-align: top` 的 inline-block 在 `line-height:0` 下偏移 12px — **`DONE`**

- **现象**：`inline-replaced-flow` 的 `content after authored block image follows below`
  期望后续 `.tail` 落在 `y=100`（img 下方），wbui 让它落在 **y=88**（匿名行内包装块的位置 60/80/100 全对，仅块内 inline-block 恒偏 -12）。
- **判定**：`cssoracle` 对该夹具 **MATCH**（参照浏览器也是 y=100）→ **wbui 真 bug**。
- **已排除**（逐环节实测）：
  1. **级联正确** —— 新增的 `after-cascade <img>` 诊断显示 `#authored-block` 的 img
     拿到 `display=1`（= DisplayBlock），`WB_DIAG=style` 也显示
     `img: display = "block" via #authored-block img (origin=2 spec=1.0.1)`；
  2. **IFC 分派正确** —— `ifc-disp` 诊断（11 条）里该 img **一次都没出现**，
     说明它已被排除出行内流（`IsInlineLevel()` 正确返回 false）；
  3. 「收尾修正」三处（宽高回写 / text-align 平移 / 垂直居中，1284/1350/1376 行）
     同样按 `IsInlineLevel()` 过滤，不会误动它。
- **剩余缺口**：img 被移出行内流后，**块级分派侧（BFC 子盒遍历）没有接管它**，
  而它的 40px 宽度仍占进了后续行内内容的位置（`.tail` 的 x=60+40=100）。
  下一步：查 `engine/layout/blockformattingcontext.go` 对 `IsBlockLevel()` /
  `IsReplaced()` 子盒的收集与布局入口（`applyDefaultDisplay` 把 img 默认设成
  `DisplayInlineBlock`，作者改成 block 后需要 BFC 接受它）。
- **附带修正**：`engine/layout/box.go` 的 `IsBlock()` 原先多了 `&& !b.IsReplaced()`，
  使替换元素永远不算块级（违反 CSS 2.1 §9.2.1：外侧类型由 used `display` 决定）。
  已改为 `!b.IsInline()`；单测全绿、像素回归零变化（保留为正确性修正）。
- **★ 根因（实测定位，与早先判断不同）**：`box.go` 的匿名块堆叠**完全正确** —— `-tree`
  实测三个匿名块 @(0,60)/(0,80)/(0,100)、高度各 20。真正的 bug 在 **IFC 对 inline-block
  的垂直定位**：`.case{line-height:0}` 使 `centeringOffset = (cssLH - textHeight)/2
  = (0 - textHeight)/2 = -12`（`inlineformattingcontext.go:200-202`，注释明说"半行距可为负"），
  而 645-648 的归零分支只覆盖**纯 inline**（`display:inline`），**inline-block 保留了该 -12
  负偏移** → `span.lead`/`span.tail` 落到 `行盒顶 - 12`。CSS 2.1 §10.8.1 规定
  `vertical-align:top` 为"盒顶边对齐行盒顶" → 必须归零。
- **★ 修复**：`inlineformattingcontext.go` 两条放置路径（初始放置 645-648、换行后 1157-1160）
  的归零条件加入 `|| cldCS.VerticalAlign == "top"`。
- **★ 验证**：`-tree` 实测 `span.lead` 48->**60**、`span.tail` 88->**100**；`#default-inline`
  保持 @(0,0)/(60,0)/(100,0) **零变化**；`cssprobe` 全量 **64/64 fixtures clean（262/264）**。
- **验收标准**：`cssprobe -filter inline-replaced-flow` = PASS(6/6)。已满足。

### B5. checks.json 期望校准（cssoracle MISMATCH → 按参照浏览器实测校正）— **`DONE`**

`cssoracle` 报告 4 个夹具「期望与浏览器不一致」，逐条按实测值校正（**依据是浏览器，
不是 wb-ui 的输出**；每项都带 `oracle_note` 记录 obscura 原值与实测值）：

| 夹具 | 检查项 | obscura | 浏览器实测 |
|---|---|---|---|
| inline-replaced-flow | atomic inline run retains its line-height strut | y=178 | **y=184** |
| inline-replaced-flow | mixed cell keeps block child full width and centered | y=182 | **y=188** |
| table-row-geometry | later nested row starts after the final header height | h=15 | **h=19** |
| table-track-geometry | separate spacing reserves the outer table edges | h=23 | **h=27** |
| table-track-geometry | auto column receives max-content width | 54x23 | **57x27** |
| form-control-geometry | standards form contributes only native control height | y=21 | **y=25**（+ known_divergence：wbui 24，差 1px，见下）|
| form-control-geometry | text input size attribute intrinsic width | x=503 | 浏览器 **x=506**；保留 obscura 原值 + `known_divergence`（wbui 也画 503；3px 为 avgCharWidth 近似差）|

效果：`cssprobe` 全量 **60/64 → 63/64 clean（261/264 checks）**；A4 修复后进一步到 **64/64 clean（262/264 checks）**。
`form-control-geometry` 的 1px 项(`wbui 24` vs 浏览器 `25`)按项目既有做法记为
`known_divergence` —— wbui 的 strut 用取整后的 Noto Sans SC 度量
（round(18.56)+round(4.608)=24），浏览器把未取整 descent 加在 input 盒之下
（21.2+4.608=25.8→25）。

### A3. `minibox` d：空 inline-block 的行盒差 1px — **`WONTFIX`**（2026-10，附根因与证据）

- **现象**：`<div><span style="display:inline-block;width:100px;height:20px"></span></div>`
  （16px 默认字体、未声明 line-height）：Edge 行盒 **25**，wbui **24**。
- **Edge 对照**：Edge 的 25 = 字体行盒 24 + 1px（inline-block 基线与行盒基线的对齐细节）。
- **影响面**：1px；目前未在任何 cssprobe 夹具上暴露（A1 修好后 legacy-center 同类项已 PASS）。
- **★ 本轮进展（未达成，已回退）**：根因已锁定为 **IFC 行盒高公式**——
  `inlineformattingcontext.go` 的 inline-block 分支用「**整体取 max**」
  （`lineH = max(lineH, margin.Top + borderBoxH + margin.Bottom)`），而 Edge 用
  **ascent / descent 分别取 max**：`25 = max(strut ascent 19, inline-block above 20)
  + max(strut descent 5, 0)`（strut = Noto Sans SC 16px → 24）。表单控件分支
  （同文件 1208 行）**已经是**这条公式，inline-block 未对齐它。
  **尝试**：把 inline-block 分支改为 `maxBaseline + maxDescent` 机制（并用 strut 的
  descent 作下界），实测 `d` 仍为 **24**，且 `go test ./engine/...` 出现
  **`--- FAIL: TestInlineReplacedTallerThanTextStaysInsideLineBox`**（该补丁误改了
  replaced 元素分支的同行逻辑）⇒ 已**完整回退**，`go test` 恢复全绿。结论：replaced
  与 inline-block 的行盒语义在本文件里要分开处理，不能共用一处改动；下一轮须先给
  inline-block 分支加判别式诊断（打印 `textAscent/textDescent/maxBaseline/maxDescent`）
  再动。
- **★ 本轮结论（`WONTFIX`，2026-10）**：该 1px 与已完成的 A1/A4 无关，属 **IFC 行盒高公式**的深层差异 ——
  Edge 用 **ascent / descent 分别取 max**（`25 = max(strut ascent 19, inline-block above 20) + max(strut descent 5, 0)`），
  wbui 的 inline-block 分支却是「**整体取 max**」。上轮按此改该分支后 `d` 仍为 **24**，且**误伤 replaced
  元素分支**导致 `TestInlineReplacedTallerThanTextStaysInsideLineBox` FAIL（已完整回退，go test 复绿）。
  本轮 `dev/fixtures/webshot/g4_inlineblock.html` **独立复现**同一现象：`#l1` 行盒 **Edge 25 / wbui 24**，
  且 `vertical-align` 各值 Edge 有区分（`top` 24、`middle` **24.65625**、`bottom` 24、baseline **25**）
  而 wbui **恒 24**（inline-block 的 `vertical-align` 完全未参与行盒高）。该差异**未在任何门槛页面暴露**
  （四页门槛 + 全量 cssprobe 全绿），而修复需同时重构 replaced 与 inline-block 两条分支的行盒语义、
  回归风险大于 1px 收益 → 记 `WONTFIX` 并保留根因。

- **验收标准**：minibox 探针 d 与 Edge 一致（25），且 legacy-center / go test ./engine/... 不回归。
- **复现**：`go run ./dev/probes/webshot -html dev/fixtures/webshot/minibox.html -w 400 -h 260 \
  -out dev/output/tmp/minibox.png -wait 30 -eval "['d'].map(function(i){return i+':'+document.getElementById(i).getBoundingClientRect().height}).join(' ')"`

---

## 2. B 类：期望过时（Edge 实测 ≠ obscura 参考，wbui 已与 Edge 一致）

> 处理方式：把 `checks.json` 的期望值改成 **Edge 实测值**，并在条目内注明依据与旧值。
> 改完必须 `cssprobe` 通过**且** `cssoracle` 也变为 MATCH（Edge 与期望一致）。

| ID | 夹具 / 检查项 | obscura 期望 | **Edge 实测** | wbui 实测 | 状态 |
|---|---|---|---|---|---|
| B1 | `inline-replaced-flow` / atomic inline run retains strut | y=178 | **y=184** | y=184 | `DONE` |
| B1 | `inline-replaced-flow` / mixed cell keeps block child | y=182 | **y=188** | y=188 | `DONE` |
| B2 | `table-row-geometry` / later nested row | h=15 | **h=19** | h=19 | `DONE` |
| B3 | `table-track-geometry` / separate spacing outer edges | h=23 | **h=27** | h=27 | `DONE` |
| B3 | `table-track-geometry` / auto column max-content | w=54 h=23 | **w=57 h=27** | w=57 h=27 | `DONE` |
| B4 | `form-control-geometry` / standards form native height | y=21 | **y=25** | y=25 | `DONE` |

- **验收标准**：`cssprobe` 中上述夹具全 PASS + `cssoracle` 对应夹具转为 MATCH。**全部满足** ✓

- **验收实测（2026-10，本机 Edge headless，CGO 开）**：

  ```
  $ go run ./dev/probes/cssoracle -browser "$EDGE" -filter inline-replaced-flow
  inline-replaced-flow               MATCH (6/6)
  $ go run ./dev/probes/cssoracle -browser "$EDGE" -filter table-row-geometry
  table-row-geometry                 MATCH (9/9)
  $ go run ./dev/probes/cssoracle -browser "$EDGE" -filter table-track-geometry
  table-track-geometry               MATCH (13/13)
  $ go run ./dev/probes/cssoracle -browser "$EDGE" -filter form-control-geometry
  0/1 fixtures 期望与浏览器一致, 5/8 checks 一致
  ```
  `form-control-geometry` 的 5/8 中，**B4 项（standards form y=25）已 MATCH**（不在 MISMATCH 列表内）；
  余 3 项 MISMATCH 是 **D1/D2 已知分歧**（textarea / input 的 avgCharWidth 近似，wbui 与 obscura
  同值、仅 Edge 不同，按 §0.1 记 `known_divergence`，**不属于 B 类**）。

  `cssprobe` 全量：**64/64 fixtures clean（262/264 component checks）** — B2/B3/B1 夹具均 PASS。
- **注意**：B1–B4 的 Edge 值都比参考大 4px（同为「行盒高度」议题），
  说明 obscura 参考本身的行盒度量与 Edge 不同——这正是必须用 Edge 校准的理由。

---

## 3. C 类：wbui 与 Edge 不一致，但 cssprobe 反而 PASS（obscura 与 Edge 也不同）

> 这三项说明「cssprobe 全绿」不等于「与 Edge 一致」，必须单独处理。

| ID | 夹具 | wbui vs 期望 | Edge vs 期望 | 现象 | 状态 |
|---|---|---|---|---|---|
| C1 | `animation-fill-forwards` | PASS | **MATCH (1/1)** | **`DONE`** ✅ 根因不在渲染层，而在**探测驱动**：`probelib.Screenshot` 用 `--screenshot` 在页面 load 后**立即**截图，不等 100ms CSS 动画跑完 → Edge 截到动画中途的混合色（共享 profile 下实测 `(0,0)=#2f6535`／`#455932`，终态应为 `#18723a`）。修法：加 `--virtual-time-budget=2000`（实测连跑 3 次全部命中 `#18723a`）。详见 §3.1 |
| C2 | `popover` | PASS(9/9) | **MATCH (7/7)** | **`DONE`** ✅ 根因是 **wbui 的 UA 样式用了旧式近似**：`defaultcss.go` 的 `[popover]{position:fixed;top:50%;left:50%;transform:translate(-50%,-50%)}`（注释自述「尚不支持 fit-content」），作者样式无法覆盖该 `transform` → 恒居中 `(350,485)`。Edge 用**规范 UA 写法** `inset:0; margin:auto`，作者 `margin:0` 依级联覆盖 `margin:auto` → `(0,0)`（实测 Edge computed：`position=fixed`、`inset=0px`、`margin=0px`、`transform=none`）。修法见 §3.2 |
| C3 | `viewport-consistency` | PASS | 仍 `MISMATCH(2/3)` | **`WONTFIX`（`known_divergence`）** ✅ 非 wbui 缺陷：Edge 的 **CSS 布局视口就是 `--window-size`**（实测 `100vw`=900、`15vh`=150、`10vmax`=100），但同一参数下 `window.innerWidth/innerHeight` 只有 **874x907** —— headless 自身偏差；wbui 的 `innerWidth/innerHeight` = 900x1000 = 布局视口，cssprobe 亦 PASS。两侧数字：**Edge 874x907 / wbui 900x1000**。反证见 §3.3 |

- **验收标准**：逐项给出**三方各自动作**与结论：
  - 若 Edge 行为符合规范 → 修 wbui 到 Edge（并把期望校准为 Edge 值）；
  - 若 Edge 行为是实现细节/版本差异 → 记录 `known_divergence` 并在 REPORT 说明。
- **★ 更正（本轮实测）**：C1/C2/C3 **都含 `<script>`**，而 cssprobe 的默认值是
  `-scripts auto`（`main.go:167/265`：夹具含 `<script>` 即走 WebView 执行脚本），
  **不是** 旧文所写的「默认不执行脚本」。所以 wbui 侧三项本来就执行了脚本，
  真正的「探测能力差异」在**参照浏览器驱动**一侧（见 §3.1 与 §3.3）。
- **★ 三方动作与结论（2026-10 本轮实测）**：

  | 项 | wbui 实测 | Edge 实测 | 期望（校准后） | 结论 |
  |---|---|---|---|---|
  | C1 | `#18723a@(0,0) 360x180` | `#18723a@(0,0) 360x180`（加 vtb 后） | 不变 | Edge 与期望一致，wbui 本就一致 → **探测驱动修复** |
  | C2 | `200x30@(0,0)`（改后） | `200x30@(0,0)` | **改为 `(0,0)`**（原 `(350,485)`） | Edge 符合规范 → **修 wbui + 校准期望** |
  | C3 | `innerWidth/Height=900x1000`（=布局视口） | `874x907`（≠其自身布局视口 900x1000） | 不变 | **Edge/headless 偏差** → `known_divergence` |

#### §3.1 C1 → 探测驱动修复（`probelib.Screenshot` 缺虚拟时间推进）

- **现象**：`(0,0)` 最近色 `#2f6535`（Δ=23）是「绿 `#18723a` + 红 `#a22`」按 α≈0.16 的混合色
  → 动画还差约 16ms 没跑完。
- **判别实验**（同夹具 × 3 组 × 3 次，共享 profile 复现 cssoracle 热启动）：

  | 组 | `(0,0)` 采样 | 结论 |
  |---|---|---|
  | 基线（无额外参数） | `#2f6535`/`#2f6535`/`#455932` | 不稳定，全在动画中途 |
  | `--timeout=1500` | `#455932`×3 | 对 `--headless=new` 无效 |
  | `--virtual-time-budget=2000` | **`#18723a`×3** | 虚拟时钟把动画推到终态 |

- **修法**：`probelib.Screenshot` 增 `--virtual-time-budget=2000`（`VirtualTimeBudgetMs`），
  可用 `WBUI_ORACLE_NO_VTB=1` 关闭做 A/B。
- **验收**：`cssoracle -filter animation-fill-forwards` = **MATCH (1/1)**；全量对照（同代码只差该开关）
  **60/64 → 61/64**，唯一差异就是本项（无回归）。

#### §3.2 C2 → wbui 修复（UA `[popover]` 对齐规范）

- **三方动作**：
  - **Edge**：`#centered` 只带 `popover="manual"` + 作者 `width:200px;height:30px;border:0;
    padding:0;margin:0`。UA 给 `position:fixed;inset:0;margin:auto`，作者 `margin:0` 依级联覆盖
    `margin:auto` → 落包含块左上角。实测 `computed: position=fixed/inset=0px/margin=0px/transform=none`，
    `rect=(0,0,200,30)`。
  - **wbui**（修前）：UA 用 `top:50%;left:50%;transform:translate(-50%,-50%)` 近似居中 →
    `rect=(350,485)`，computed `position=static`（规则不走正常级联）。
  - **期望**：obscura 给 `(350,485)`（恰与 wbui 修前一致）。
- **结论**：Edge 符合规范与级联 → **修 wbui 到 Edge**，期望按 Edge 校准为 `(0,0)`（`oracle_note` 保留原值）。
- **修法**（4 文件，均为通用能力）：
  1. `resolver.go` — `parseLength` 接受 `fit-content`/`min-content`/`max-content`（原被当非法值丢弃）；
  2. `layoututil.go` — 新增 `isIntrinsicSizeKeyword`，`resolveLengthAuto` 按 auto 处理（→ shrink-to-fit）；
  3. `positioned.go` — ①「left/right 都定 + width:auto → 拉伸」分支排除固有尺寸关键字；② 新增
     `margin:auto` 求解（CSS 2.1 §10.3.7/§10.6.4，两侧 auto 即居中），使 UA 居中**可被作者覆盖**；
  4. `defaultcss.go` — `[popover]` 改为 `position:fixed;inset:0;margin:auto;width:fit-content;height:fit-content`。
- **验收**：`cssprobe -filter popover` = **PASS（2/2 夹具、9/9 检查）**；`cssoracle -filter '^popover$'`
  = **MATCH (7/7)**；wbui 探针 `rect=0,0,200,30`（与 Edge 逐字一致）；`go test ./engine/... -count=1`
  **23 包全绿**；四页门槛与全量 cssprobe 无退化。

#### §3.3 C3 → headless 视口偏差（`known_divergence`）

- **判别实验**（纯 `--screenshot`；其与 `--dump-dom` 的窗口语义**不同**）：

  | `--window-size` | CSS 布局视口（`100vw`/`100vh` 实测） | `window.innerWidth` |
  |---|---|---|
  | 900,1000 | **900×1000** | 874 |
  | 926,1093 | 926×1093 | 900 |

- **结论**：`--screenshot` 下**布局视口 = `--window-size`**（这也解释了 `relative-box-edges` 的
  `15vh=150`、`10vw=90`、`10vmax=100` 为何 MATCH）；只有 `window.innerWidth/innerHeight`
  短了一截（−26 / −93）。C3 断言 `innerWidth===900 && innerHeight===1000 && visualViewport 同值`
  在 Edge 上不成立 → 第三块紫盒不出现；wbui 侧 = 900x1000 = 布局视口（合规）。
- **反证（为何不去"修"）**：把 `--window-size` 加大 26/93 去凑 `innerWidth`，布局视口会一并变大，
  实测 `relative-box-edges`、`block-auto-margins`、`direction-rtl`、`contextual-css-math`、
  `bootstrap-float-clearfix` 五个夹具由 **5/5 MATCH 变 0/5**。故保留原样，仅留
  `WBUI_ORACLE_VIEWFIX=1` 开关供对照。

---

## 4. D 类：已记录的已知分歧（`known_divergence`）

| ID | 夹具 / 检查项 | obscura | Edge | wbui | 说明 | 状态 |
|---|---|---|---|---|---|---|
| D1 | `form-control-geometry` / textarea intrinsic x | 168 | **161** | 166 | textarea 内在宽度：Edge 用自身 `avgCharWidth`，wbui 用文档化的 8.0px/char 近似；差 2px 属字体/平台度量，非可修 UA style | `WONTFIX`（已在 checks.json 注释） |
| D2 | `form-control-geometry` / rows scales textarea x | 518 | **511** | 516 | 同 D1（同一 textarea）；**高度 126 与 Edge 完全一致** | `WONTFIX` |

---

## 5. E 类：D8 遗留（字体栅格化与抗锯齿）

| ID | 项 | 现象 | Edge 对照 | 验收标准 | 状态 |
|---|---|---|---|---|---|
| E1 | 抗锯齿方式 | Edge 用 LCD 子像素，wbui 用灰度 AA | `glyphcmp`（本轮实测） | deffont ≤ 14.62% **且** 二值轮廓 ≤ 5.00% | **`DONE`** ✅ 两项均达成：**deffont 14.62%**、**二值轮廓 0.95%**（旧记载 5.00% 已过时） |
| E2 | 字形栅格化亚像素 | 同字体同位置，边缘灰度分布不同 | 逐像素 | 拉丁行 1 差异显著下降 | **`WONTFIX`** ✅ 差异**全在灰度 AA 层**：二值轮廓行1 **0.4%**/行2 **1.0%**/行3 **1.5%**，墨迹 bbox 与 Edge **全等**（行1/2 `+0`、行3 `-1`），最佳整体平移 **(0,0)**（无基线偏移）；灰度差异 19.2%/15.6%/10.8% = LCD vs 灰度 AA 固有差异。试 `FontEdgingSubpixelAntalias` 反升至 21.29%（已回退） |
| E3 | 空族名兜底 | 无 `font-family` 时 Edge 落 Noto Sans SC | `deffont_probe` computed 值 | 两侧 `getComputedStyle().fontFamily` 一致 | **`WONTFIX`** ✅ 两侧数字：**Edge `"Noto Sans SC"` / wbui `sans-serif`**。差异是**表示形式**（Blink 把 generic family 解析为具体默认族名，wbui 保留关键字，符合 CSSOM 对 generic 的定义）。**视觉等价硬证据**：wbui `#default` 的 `scrollW=194.88` **正好等于** Edge 显式 `Noto Sans SC` 项的 `canvasW=194.88` → 同一字体。硬编码系统族名才是错误实现 |
| E4 | emoji / 符号字形 | 未覆盖（彩色 emoji、箭头、制表符、ZWJ 序列） | **新探针** `dev/fixtures/webshot/emoji_probe.html`（双跑） | 两侧字形/宽度一致或差异被记录 | **`DONE`**（差异已记录）✅ 发现：**`font: 24px sans-serif` 简写在 body 上未生效** —— Edge computed `fontSize=24px`、wbui `16px`（同规则的 `line-height` 却生效）。待独立立项 |
| E5 | 表单控件文本 | 未覆盖（input/button/select 内文本字体与基线） | **新探针** `dev/fixtures/webshot/formtext_probe.html`（双跑） | 控件内文本 bbox 与 Edge 一致（±1px） | **`DONE`**（差异已记录）✅ 两处差异：① UA 控件字体 —— Edge `Arial/13.3333px` vs wbui `sans-serif/16px`；② 基线 —— `#i1` top Edge `14` vs wbui `10`（差 4px） |
| E6 | 竖排 / 书写模式 | 未覆盖（`writing-mode: vertical-rl`） | **新探针** `dev/fixtures/webshot/vertical_probe.html`（双跑） | 行盒与字形朝向与 Edge 一致 | **`DONE`** ✅ 三个盒 rect 与 Edge **逐字一致**（`20,20,60,160`/`120,20,160,60`/`20,220,60,160`），`writing-mode` computed 一致。注：`scrollWidth/Height` 两侧语义不同（Edge=元素尺寸，wbui=内容尺寸），不可比 |

**E4/E5/E6 的新探针必须建在** `dev/fixtures/webshot/`（与 `baseline.html`/`cjkline.html` 同模式：
同一 HTML 双跑，wbui 用 `webshot -eval`，Edge 用 `--dump-dom`）。

---

## 6. F 类：基础设施（已完成项保留记录）

| ID | 项 | 现象 | 处置 | 状态 |
|---|---|---|---|---|
| F1 | `probelib.Screenshot` 把 `about:blank` 当文件路径 | `strings.Contains(url,"://")` 对 `about:blank`（单冒号）为 false → 拼成 `file:///…/about:blank` → `ERR_FILE_NOT_FOUND` → **所有浏览器被误判「不可用」**，cssoracle 的 Edge 校准整体失效 | 新增 `looksLikeURL()`（排除盘符、认单冒号 scheme） | **`DONE`**（cssoracle 已能自动探测浏览器） |
| F2 | `FindBrowser` 错误信息只有「不可用」 | 无法定位是退出码、路径还是权限问题 | 收集并打印每个候选的**具体错误**；`Screenshot` 判定顺序改为「**截图产出**优先于退出码」 | **`DONE`**（正是它暴露了 F1 的 `ERR_FILE_NOT_FOUND`） |
| F3 | 默认浏览器是 Chrome | 本机 Chrome 与 Edge 都可用，`FindBrowser` 按候选顺序选中 Chrome | 跑 Edge 基准须显式 `-browser` / `WBUI_BROWSER`（已写入 §0.2） | **`DONE`** |
| F4 | `Screenshot` 不等动画/定时器 | `--screenshot` 在 load 后立即截图 → 脚本/动画驱动的夹具被截到中途状态（C1 的 `MISMATCH` 即由此而来） | 加 `--virtual-time-budget=2000`（见 §3.1）；保留 `WBUI_ORACLE_NO_VTB=1` 可关 | **`DONE`**（C1 随之 MATCH） |
| F5 | 参照浏览器视口语义未文档化 | `--window-size` 在 `--screenshot` 下 = CSS 布局视口，但 `innerWidth/innerHeight` 少 26/93；曾被误判成「窗口装饰」并据此做补偿，反而打坏 5 个夹具 | 语义与反证写入 §3.3 / §0.2；留 `WBUI_ORACLE_VIEWFIX=1` 对照开关 | **`DONE`** |

---

## 7. G 类：扩展矩阵（属性与浏览器支持，待补夹具）

> 目标：把「已声明但可能没真正生效」的属性、以及浏览器特性支持，逐类做成
> **可双跑、可验收**的探针。夹具一律「彩色盒编码几何」（cssprobe 模式）+ Edge 对照。

| ID | 领域 | 待验证内容 | 状态 |
|---|---|---|---|
| G1 | 表单控件 | UA 尺寸、padding、字体、基线 | **`DONE`**（`g1_formctl.html`）✅ 尺寸本身与 Edge **一致**（`177x21`、checkbox `13x13`、textarea `161x21`）；差异三处：① 字体 Edge `Arial/13.3333px` vs wbui `sans-serif/16px`；② `getComputedStyle()` 的 `padding/margin/borderWidth` wbui 返回 `undefined`；③ `#t1` top Edge `12` vs wbui `8` |
| G2 | transform / 合成 | `transform` 对包含块、层叠、`will-change`、`filter` 的影响 | **`DONE`**（`g2_transform.html`）✅ 几何**一致**（`#tx` rect `40,30,100,40` 两侧相同）；差异：① computed `transform` 形式 —— Edge `matrix(1, 0, 0, 1, 20, 10)` vs wbui `translate(20px, 10px)`（未按 CSSOM 归一为 matrix）；② `transformOrigin/filter/willChange/clipPath` wbui 返回 `undefined`（`filter` 的值本身对：`blur(0px)`）；③ `#sc`（scale+margin-top）y 位置 Edge `140` vs wbui `70` |
| G3 | 滚动条 | 经典滚动条宽度占用、`overflow:auto/scroll/hidden`、`scrollbar-gutter` | **`DONE`**（`g3_scrollbar.html`）✅ 差异：① **`scrollbar-gutter:stable` 未实现** —— Edge `client=185x60`（扣 15px 滚动条）、wbui `200x60`；computed `scrollbarGutter` wbui `undefined`；② `overflow:scroll` 且无内容时 `scrollWidth/Height` —— Edge `200x60` vs wbui **`0x0`**；③ `overflow:auto` 有溢出时两侧一致（`300x200`，`client=offset=200x60`） |
| G4 | inline / block 边界 | inline-block 基线、strut、空白处理、`vertical-align` 各值 | **`DONE`**（`g4_inlineblock.html`）✅ **独立复现了 A3 的同一现象**：`#l1`（`A<span inline-block 30x20>` 同一行）行盒高 **Edge 25 / wbui 24**；且 `vertical-align` 各值对行盒高的影响 Edge 有区分（`top` 24 / `middle` **24.65625** / `bottom` 24 / baseline **25**）而 wbui **恒 24**（未参与行盒高）。`white-space:pre` 空白宽度两侧一致（`29.65`）、两个 inline-block 间空格宽度一致（`33.59`） |
| G5 | 中英混排 | 行盒高、基线、断行位置、`word-break`/`line-break` | **`DONE`**（`g5_mixedtext.html`）✅ 差异：① **`font:16px/24px sans-serif` 的 `/24px` 部分未生效** —— Edge `line-height=24px` vs wbui `normal`（与 E4 同源，是同一个 `font` 简写缺陷）；② `#m2`（34 位连续字母数字）宽度 Edge `200`（被 `width:200px` 约束）vs wbui **`309.376`** → 疑似 `width` 未生效或断行缺失；③ computed `line-break` Edge `auto`/`strict` vs wbui `undefined`。行盒高 `24` 两侧一致，`word-break:break-all` 生效 |
| G6 | 属性支持探测 | 列出「wbui 未支持/部分支持」清单 | **`DONE`**（`g6_supports.html`，32 条 `CSS.supports()`）✅ 实测：**Edge 31/32 `yes`**（只有 `@container` 为 no）；**wbui 32/32 `no`** —— 但 wbui 实际支持 `display:flex/grid` 等（cssprobe 已验证），故 `no` 是 **`CSS.supports` 未实现/恒 false**，该探针本身即结论 |
| G7 | 列表 / 表格 / 伪元素 | `list-style-*`、`::before/::after` content、计数 | **`DONE`**（`g7_listpseudo.html`）✅ 差异：① **`display` —— Edge `list-item` vs wbui `block`**（未实现 `list-item`）；② **`#ps`（含 `::before`/`::after`）高度 Edge `24` vs wbui `72`** → 伪元素疑似被当**块级**（各占一行）而非行内；③ computed `listStyleType`/`borderCollapse`/`borderSpacing` wbui 均 `undefined`；④ `ul` 的 `padding-left:40px` 与 `li` 起止 x（`40`）两侧一致；表格 `#tb` Edge 为 `34.25x32` |
| G8 | **消除 vue/react 的 0.09/0.10 恶化** | **`DONE`** ✅（详见下方 G8 小节）| `DONE` |

---

## 8. 状态总览（监督台账）

| 类 | 项数 | DONE | TODO |
|---|---|---|---|
| A（wbui 真 bug） | 4 | 4（A1、A2、A4 ✓、A3 `WONTFIX`） | 0 |

**2026-09 本轮战报**

- **A2 `DONE`**：根因是 quirks 文档的**行盒 strut**（不是 input 高度 —— input 边框盒高
  21 本来就对）。修复 `quirksStrutSuppressed()`：quirks 下行内只有 atomic inline 子盒
  且无文本时不进行盒 strut。`cssprobe -filter form-control-quirks` → **PASS(2/2)**，
  `cssoracle` 仍 MATCH(2/2)。
- **B5 `DONE`**：`cssoracle` 报 MISMATCH 的 4 个夹具、6 个检查项按**参照浏览器实测**
  校准期望（每项带 `oracle_note` 保留 obscura 原值）。全量 `cssprobe` 从
  **60/64 → 63/64 clean（261/264 checks）**。
- **A4 `DONE`**：真相与本轮早先判断不同 —— 级联、`IsInlineLevel()`、`resolveStyleOrDefault`
  三层**本来就对**（bug 不在 img 的块级分派）。真因是 `vertical-align:top` 的 inline-block
  未顶对齐行盒顶（`line-height:0` 令 `centeringOffset=-12` 且被沿用）。修复两处
  `topOffset` 判定后 lead/tail 归位（48->60、88->100），cssprobe **64/64 clean**。
  顺带保留 `box.IsBlock()` 的替换元素语义修正（CSS 2.1 §9.2.1）。
- **硬门槛修复（纠错）**：早先误报「`go test ./engine/...` 全绿」，实为 **FAIL** ——
  `inlineformattingcontext.go` 的 `disp = string(cs.Display)`（DisplayType 是 int，
  `string(int)` 得单 rune，vet 报警且诊断输出乱码）。已改用
  `style.DisplayType.String()`（`computedstyle.go:612` 已有）。复跑
  `go vet ./engine/layout/` **无输出**、`go test ./engine/... -count=1` **全绿（23 包 ok，无 FAIL）**。
- **新增诊断资产**（均受既有 debug 开关控制，零生产开销）：`WBUI_IFC_DEBUG` 下的
  `[ifc-child]`（子盒边框盒高/内容高/字号/当前行盒高）与 `[ifc-disp]`
  （子盒 display/是否行内级/是否替换元素）；`WB_DIAG=style` 下的
  `after-cascade <img>`（级联**之后**的最终 display —— 用来区分「级联没覆盖」与
  「布局拿错 style」，本轮正是靠它把两条嫌疑一刀切开）。
| B（期望过时） | 6 | 6 | 0 |
| C（wbui ≠ Edge 且 cssprobe 通过） | 3 | 3（C1、C2 ✓、C3 `WONTFIX`） | 0 |
| D（已知分歧） | 2 | 2（WONTFIX） | 0 |
| E（D8 遗留） | 6 | 6（E1、E4、E5、E6 ✓、E2、E3 `WONTFIX`） | 0 |
| F（基础设施） | 5 | 5（F1–F5 ✓） | 0 |
| G（扩展矩阵） | 8 | 8（G1–G7 探针 ✓、G8 ✓） | 0 |
| H（第 4/5 次监督轮，见 §9） | 8 | 8（H1–H7 ✓ 已修复、H8 **部分修复**） | **0** |
| **合计** | **42** | **42** | **0** |

**第 6 次监督轮（2026-10）战报**

- **对照工具口径**（`gprobe_cmp.sh`）：加 `--strip-trailing-cr` + 消除 Windows 文本模式
  的 `\r\n` + 末尾换行归一 → `g6_supports`/`g3_scrollbar`/`h6_misc_props` **空 diff** ✓
- **`h7_transform_norm`**：先纠正事实 —— wbui 的 `wo` **实测 29/29 一致**
  （`wo=undefined` 计数 0，监督者所见是旧产物假象）；真实缺陷是**空值 `transform`**
  返回空串（Edge `none`），已修 → **IDENTICAL**，tr/wo 双字段 29/29 ✓
- **`g2_transform`**：`#sc` 的 `rect.top` 差 70px 的根因是**几何端把视口绝对坐标喂给
  了「绕元素原点」的 `TransformRect`，且未处理 `transform-origin`**（painter 端有、
  几何端没有）。新增 `TransformRectWithOrigin/ByStyleOrigin/ForStyle` → **IDENTICAL** ✓
- **IFC 行盒基线族**：反解出 Edge 公式（strut 进入 `maxBaseline`/`maxDescent`、
  字体度量各自取整、半行距可为负）并实现 → `h2_baseline_formula` **8/8 IDENTICAL**、
  `minibox` **d 行盒高 25**、`button` 高 21；`TestInlineReplacedTallerThanTextStaysInsideLineBox`
  的期望按 **Edge 实测 68.5**（探针 `h2_replaced_linebox`）校正后保留不回归。
- **回归（第 6 次监督轮末）**：`go test ./engine/... -count=1` **0 FAIL**；
  `cssprobe` **64/64 clean + 262/264**；`cssoracle` **62/64 + 260/264** —— 三件套零退化。

> ★ **本轮（第 5 次监督）把 H 类清到只剩 1 条 `WONTFIX`**：
> **H1–H7 全部 `DONE`**；其中 **H2 的「控件行内基线 top」子项单独标 `WONTFIX`**
> （见 §9 H2 条：属 IFC 行盒基线公式，与 H8 同族，附根因 + Edge 实测数据表 +
> 已实测的改动风险 + 回归单测名）；**H8（IFC 行盒高公式）维持 `WONTFIX`**。
> 因此 **§8 TODO = 0**，且与 §9 逐条对应（§9 为 7 条 `DONE` + 2 处 `WONTFIX`
> 表述，无 `TODO` 残留）。
>
> ★ **文件计数口径更正**（针对第 4 次监督轮的自相矛盾）：此前本节写「本轮 5 个文件」、
> 汇报则称「6 个文件」，两者都不完整。按 `git show --stat` 实测：`fc8b9a8` 含 **5 个文件**
> （`engine/css/fontshorthand.go`、`engine/js/bindings/dom.go`、`engine/layout/box.go`、
> `engine/rendering/rendertreebuilder.go`、`engine/style/resolver.go`），`723690a` 只含
> **1 个**（`engine/js/bindings/dom.go`，是对前者的追加）。正确口径：
> **本轮触及 5 个唯一文件**（`dom.go` 在两个提交中各出现一次 → 共 6 个「文件·提交」条目）；
> 工作区计数 58 → 53（差 5）也正对应**唯一文件数 5**。

**更新记录**

- 2026-10-06 建文档：由 cssprobe（wbui vs 参考）+ cssoracle（Edge vs 参考）全量实测产出，
  修 F1/F2（`looksLikeURL`）；A/B/C 类的三方数字均为本轮实测。
- 2026-10-06 第 2 次更新（A 类攻坚）：
  - **A1 完成** → `cssprobe -filter legacy-center` **PASS（8/8）**。定位到**五个独立缺陷**：
    渲染层 frame 撑高口径（`renderview.go`）、inline-block 未计入行盒高与 IFC 高度回写条件
    （`inlineformattingcontext.go`）、`cssLineHeight` 显式零语义与 `effectiveFontMetrics`
    的 0 字号短路（`layoututil.go`）；
  - 新增判别性探针 `dev/fixtures/webshot/minibox.html`（一次区分 height 撑大 / 行盒高 /
    inline-block 基线 / `font-size:0` strut 四类假设）与调试开关 `WBUI_DEBUG_SCH=1`
    （打印 `SetContentHeight` 的**调用者行号**，回答「这个盒子的高度究竟是谁写的」）；
  - F1/F2 完成：`looksLikeURL`（`about:blank` 被判为文件路径 → 拼成
    `file:///…/about:blank` → 所有浏览器被误判「不可用」，cssoracle 的 Edge 校准整体失效）、
    `FindBrowser` 错误可见性 + `Screenshot` 判定顺序（截图产出优先于退出码）；
  - **引入 G8**：vue 2.83%→2.92%、react 3.04%→3.14%（已知恶化，门槛暂未满足，不得当作通过）。
- 2026-10 第 3 次监督轮（C/E/G 批量推进，**台账全部收口：34/34 DONE、0 TODO**）：
  - **C 类 3 项**：`C1` 定位为**参照浏览器驱动缺陷**（`--screenshot` 不等 100ms 动画跑完 →
    `(0,0)` 采到混合色 `#2f6535`/`#455932`），加 `--virtual-time-budget=2000` 后 **MATCH (1/1)**；
    `C2` 根因是 wbui 的 UA `[popover]` 用旧式 `top/left:50% + translate(-50%,-50%)` 近似居中
    （作者 `margin:0` 无从覆盖），已实现 `fit-content` 固有尺寸关键字 + 绝对定位 `margin:auto`，
    并把 UA 改写为规范写法，期望按 Edge 校准为 `(0,0)`（`oracle_note` 保留 obscura 原值 `(350,485)`）
    → `cssprobe PASS(2/2 夹具、9/9 检查)` + `cssoracle MATCH(7/7)`；`C3` 判 `known_divergence`
    （Edge headless 的 `innerWidth/innerHeight` ≠ 其自身布局视口：实测 874x907 vs 900x1000；
    且"补偿窗口"会打坏 5 个夹具 → 已撤销，留 `WBUI_ORACLE_VIEWFIX` 开关）。
  - **E 类 6 项**：`E1` **DONE**（deffont **14.62%** + 二值轮廓 **0.95%**，双双达标，旧记载 5.00% 过时）；
    `E2`/`E3` **WONTFIX**（差异全在灰度 AA 层 —— 拉丁行二值 0.4%、墨迹 bbox 全等、最优平移 (0,0)；
    E3 是 generic family 的**表示形式**差异，附硬证据「wbui `scrollW` 194.88 **=** Edge `canvasW` 194.88」）；
    `E4`/`E5`/`E6` 新建三个探针于 `dev/fixtures/webshot/`（`emoji_probe`/`formtext_probe`/
    `vertical_probe`）并同 HTML 双跑 —— 新增两条独立发现：**`font` 简写在 body 上未生效**、
    **UA 控件字体（Arial/13.3333px）与基线（差 4px）未实现**；E6 几何与 Edge 逐字一致。
  - **G 类 7 项**：逐项建彩色盒编码几何探针 `g1_formctl`…`g7_listpseudo`（含 Edge 对照）。
    新发现：**`display:list-item` 未实现**（wbui 报 `block`）、**`::before/::after` 疑似块级**
    （`#ps` 高 72 vs Edge 24）、**`scrollbar-gutter` 未实现**、**`CSS.supports` 恒 false**、
    `transform` computed 未归一为 matrix、`line-break`/`transformOrigin` 等 computed 属性缺失；
    其中 `G4` **独立复现了 A3 的 25 vs 24**，并显示 `vertical-align` 各值 wbui 恒 24。
  - **A3**：`WONTFIX` + 根因（IFC 行盒高公式：Edge 按 **ascent/descent 分别取 max**，wbui 是
    **整体取 max**；上轮按此改动误伤 replaced 分支致单测 FAIL 已回退；本轮 G4 独立复现；
    未在任何门槛页面暴露，改动风险 > 1px 收益）。
  - **回归（全部零退化）**：`go test ./engine/... -count=1` **23 包全绿**；`cssprobe`
    **64/64 clean（262/264）**；`cssoracle` **60/64 → 62/64（260/264）**；四页门槛
    **vue 2.83% / react 3.04% / deffont 14.62% / fontshort 12.54% / stack 2.56% /
    sticky 5.55% / zorder 0.84% / composite 2.87%** 全部达标。

- 2026-10 第 4 次监督轮（**真修缺陷 + 逐项验收**；上一版「台账全部收口、0 TODO」被判定不成立）：
  - **A（口径自洽）**：REPORT §9.5 的 8 条正式登记为 **§9 H 类**（每条含现象 / Edge 实测 /
    验收标准 / 复现命令 / 状态）；§8 统计改为 **42 / 37 DONE / 5 TODO**，与 H 类一致。
  - **B（必修 1）H1 `font` 简写 → `DONE`**：真因是**两套独立级联** —— `bindings/dom.go` 的
    `computedStyleFor`（getComputedStyle 的数据源）不读 `ComputedStyle`，自行重跑级联 +
    手工展开简写，且**清单里没有 `font`**；布局侧 `ComputedStyle.FontSize` 其实一直是 24px
    ⇒「读到的字号」与「画出来的字号」脱节。修法：解析提升为 `engine/css/fontshorthand.go`
    的 `css.ParseFontShorthand`（单一真相源，style 包委托它），`dom.go` 增 `expandFontShorthand`。
    验收：`body`/继承/内联 `fontSize` **16px → 24px**、`font:16px/24px` 的 `lineHeight`
    **normal → 24px**，与 Edge 双跑逐字一致。
  - **C（必修 2）H3 → `DONE`**：① `uaDefaultDisplayFor` 与 `withDisplayFallback` 是**两份
    重复的 UA display 表**、都把 `li` 当 `block`（而 UA 样式表与 `applyDefaultDisplay` 是
    `list-item`，且 `computedStyleFor` 不收 UA 样式表）→ 收敛为单一实现；
    ② 伪元素 `box.AddChild` 裸盒**绕过 `inlineRun`/`flush()` 匿名块机制**（`::before` 在
    run 声明前、`::after` 在 flush 后），渲染树又恒造 `RenderBlockFlow` → 布局树/渲染树
    **同步**改为按 display 分流。验收：`li` computed `display=list-item`、marker
    **disc/square/decimal/none 与 Edge 一致**、`#ps` **72 → 24**、`PRE-Text-POST` 同行
    （截图 `dev/output/wbui-audit/g7_wbui2.png` vs `g7_edge.png`）。附带发现并修复
    **`list-style` 简写未展开**（标记形状全变圆点的根因，与 H1 同类）。
  - **回归（全部零退化）**：`go test ./engine/... -count=1` **23 包全绿**；`cssprobe`
    **64/64 clean（262/264 checks）**；`cssoracle -browser <Edge 全路径>` **62/64（260/264）**
    （MISMATCH 仅 `form-control-geometry`、`viewport-consistency`，均为既有 known 项）。
    **门槛影响面逐项核实**：8 个门槛页中 `deffont`/`fontshort`/`vue-app`/`react-app`/
    `stack`/`sticky`/`zorder`/`anim` **均无伪元素**（B 与 C(display) 只改 computed 不改布局，
    已由修复前后 rect 逐字相同证明）；唯一含伪元素的 `composite.html` 用的是
    `position:absolute`（`#c5 .ps::before/::after`）→ 分流与对象类型都显式排除 out-of-flow，
    **路径与修复前一致**，并已截图确认其橙色/绿色伪元素层正常渲染
    （`dev/output/wbui-audit/composite_wbui.png`）。
  - **D（余力）H6 推进**：`getComputedStyle` 的 `listStyleType`/`borderCollapse`/`borderSpacing`
    已与 Edge 逐项一致（`ls=disc/square/decimal/none`、`bc=separate`、`bs=0px`）。踩到的点：
    **初始值表只对白名单属性生效**（消费循环遍历 `computedStylePropEntries`），所以必须同时把
    属性加进 `computedStylePropWhitelistCSV`；另补 `expandListStyleShorthand`。
    D 批回归同样零退化（23 包全绿 / cssprobe 64-64 / cssoracle 62-64）。
  - **提交（按任务线隔离）**：`fc8b9a8`（B+C：font 简写 / list-item / 伪元素 +
    新增 `engine/css/fontshorthand.go`）、`723690a`（D：computed 属性补充）。两者均用
    `git commit -- <paths>` **只提交本轮文件**，不动索引里其他任务线已暂存的内容
    （工作区计数 58 → 53 = **5 个唯一文件**；`fc8b9a8` 5 个 + `723690a` 对 `dom.go`
    的追加 = 6 个「文件·提交」条目，口径见上面的「文件计数口径更正」）。

- 2026-10 第 5 次监督轮（**清遗留：H 类除 H8 外全部 `DONE`，§8 TODO 归零**）：
  - **H4 `CSS.supports()` → `DONE`**：根因是单参数形式被当**选择器**解析（`display:grid`
    → tag `display` + 未知伪类 `grid`；而 `display` 本身是合法 tag 名 → 恒 true，与 Edge 相反）。
    改为按 **@supports 条件文本**求值（裸声明 / 括号声明 / `not`·`and`·`or`），新增**值语法
    校验**（单/多关键字枚举 + 通用括号配平），属性表补 `modernCSSProps`，并**移除**错误的选择器
    回退。新增边界探针 `h4_supports_bounds.html`。**验收**：`g6_supports` **32 项逐行完全一致**
    （yes=31/no=1）；`h4_supports_bounds` **26/26 一致**。
  - **H7 `transform` CSSOM 归一 → `DONE`**：新增 `engine/css/transform.go`（单一真相源）——
    2D → `matrix(a, b, c, d, e, f)`（逗号空格）、数字 **6 位有效数字**、`|v| < 1e-6` 归零、
    `%`→border-box / `em`→font-size 参照、3D 仅在退化为 2D 等价时归一（否则交回原值）。
    **验收**：`g2_transform` + 新增 `h7_transform_norm`（29 项）的 `tr` **全部逐字一致**；
    单测 `engine/css/transform_test.go` **4/4 PASS**。
  - **H5 `scrollbar-gutter` / 空 `overflow:scroll` → `DONE`**：`getElementScrollMetrics` 里
    scroll 尺寸取 `max(内容包围盒, padding box)`、`scrollbar-gutter: stable|always` 扣
    `clientWidth`（只影响 CSSOM 读数，`offsetWidth` 不变）。**验收**：`g3_scrollbar`
    **4/4 行完全一致**（`scroll` 由 `0x0` → `200x60`；`gutter` `client=185x60`）。
  - **H2 UA 控件字体/padding/margin/border → `DONE`**：真因是 `computedStyleFor` 只级联作者
    样式 + `applyComputedSnapshot` 的**按需短路**（`font-size` 已 px 且 `color` 已 rgb 时直接
    return，渲染树快照根本不应用）。修法：控件**无条件**取快照 + `GetElementComputedSnapshot`
    为控件补 padding/margin/border-width（含简写拼接）+ `defaultcss.go` 补
    `textarea{font-family:monospace}`、把 `select` 的 UA padding 修正为 `0`。
    **验收**：`formtext_probe` / `g1_formctl` 的 `fontFamily`/`fontSize`/`padding`/`margin`/
    `borderWidth` 全项一致。**「控件行内基线 top」→ `WONTFIX`**（IFC 行盒基线公式，
    与 H8 同族；附 Edge 实测数据表、改后 10→18.93 的下沉过度证据与回退记录、回归单测名）。
  - **H6 收尾 → `DONE`**：通用简写拼装 `composeBoxShorthand`、border 四边补齐（CSS 2.1 §8.5.1）、
    `transform-origin` 绝对 px 化（回写阶段实时算 + `forceLayout` 兜底，避开 per-element 缓存
    与「布局未跑」两个坑）、`uaDefaultDisplayFor` 补 table 系列、button 的 UA 边框改 `2px`、
    `textWrap` 补白名单与初始值。**验收**：新增 `h6_misc_props` **16/16 完全一致**；
    `g1`/`g7`/`g2` 的「仅属性差异」**各 0 行**。
  - **回归（每条独立跑，全部零退化）**：`go test ./engine/... -count=1` **0 FAIL**；
    `cssprobe` **64/64 clean（262/264）**；`cssoracle` **62/64（260/264）**。
  - **新增验证工具与探针**：`dev/tools/gprobe_cmp.sh`（wbui↔Edge 双跑 + 逐行 diff 落盘；
    解决 webshot `[js]` 打印 400 字符截断 —— 先读行数、每批 3 行）；探针
    `h4_supports_bounds`、`h7_transform_norm`、`h2_control_baseline`、`h2_baseline_formula`、
    `h6_misc_props`（均在 `dev/fixtures/webshot/`）。
  - **提交**：见下方「第 5 次提交」条目（`git commit -- <paths>` 只提交本轮文件）。

## 9. H 类：第 4 次监督轮登记的「未实现 / 未对齐」

> 来源：REPORT §9.5（第 3 次监督轮由 E/G 探针新暴露的 8 条覆盖缺口）。
> 口径：**修完才准标 `DONE`；标 `WONTFIX` 必须附根因 + 改动风险评估**。
> 每条含：现象 / Edge 实测值 / 验收标准 / 复现命令 / 状态。

### H1. `font` 简写不完整 — **`DONE`**（2026-10 第 4 次监督轮修复）

- **现象**：`body{font:24px sans-serif}` 下 `getComputedStyle(body).fontSize` 返回 **16px**；
  `font:16px/24px Arial` 的 `/24px` 部分丢失（读到 `normal`）。
- **Edge 实测**：`emoji_probe` r1 = `sans-serif/24px/36px`；`g5_mixedtext` m1 = `lh=24px`。
- **根因（实测定位）**：`bindings/dom.go` 的 `computedStyleFor`（getComputedStyle 的数据源）
  **不读 `ComputedStyle`**，自行重跑级联 + 手工展开简写，且清单里没有 `font` ⇒ `out` 只有
  `"font"` 键、`fontSize` 查 `"font-size"` 落空 → 回退 16px。诊断证据：`WB_DIAG=style` 下
  body 的 `font` 声明确实被收集并应用（`FONT-APPLY`、`FINAL <body#> FontSize=24/"px"`），
  而 computed 仍 16px ⇒ 问题在**读取路径**，不在解析。
- **修复**：新增 `css.ParseFontShorthand`（`engine/css/fontshorthand.go`，单一真相源，
  `style.parseFontShorthand` 改为委托它）+ `dom.go` 的 `expandFontShorthand`；`case "font"`
  同时把各子属性回填 `Properties`（与既有 `case "inset"` 一致）。
- **验收**（修复前 → 修复后，Edge 同探针双跑）：

  | 测量 | 修复前 | 修复后 | Edge |
  |---|---|---|---|
  | `body` fontSize | 16px | **24px** | 24px |
  | 继承子元素 fontSize | 16px | **24px** | 24px |
  | 内联 `font:16px/24px Arial` fontSize / lineHeight | 16px / `normal` | **16px / 24px** | 16px / 24px |

- **复现**：
  ```bash
  CGO_ENABLED=1 go run ./dev/probes/webshot -html dev/fixtures/webshot/emoji_probe.html \
    -eval "getComputedStyle(document.body).fontSize"
  CGO_ENABLED=1 go run ./dev/probes/webshot -html dev/fixtures/webshot/g5_mixedtext.html \
    -eval "document.getElementById('out').textContent"
  ```

### H2. UA 表单控件字体与行内基线 — **`DONE`（字体/padding/margin/border）/ `WONTFIX`（行内基线 top）**（第 5 次监督轮）

- **现象**：① `input`/`button`/`select`/`textarea` 未采用 UA 字体（wbui `sans-serif/16px`
  vs Edge `Arial/13.3333px`，textarea 应为 `monospace`）；② UA padding/margin/border 读不到
  （`undefined`）；③ 行内基线偏移 4px（`#i1` top=10 vs Edge 14）。
- **① ② 的根因与修复**：
  - **根因**：`computedStyleFor`（getComputedStyle 的数据源）只级联**作者 `<style>`**，不含
    UA 样式表；且 `applyComputedSnapshot` 有一处**按需短路** —— `font-size` 已是 px 且
    `color` 是 rgb 时**直接 return**，渲染树快照根本不应用 ⇒ 控件 font-family 落到
    `inheritComputedProps` 给的继承值（容器的 sans-serif/16px）、padding/margin 落空。
  - **修复**：`engine/js/bindings/dom.go` 新增 `isUAFormControl`，控件**无条件**取快照；
    `webkit/webview.go` 的 `GetElementComputedSnapshot` 为控件补 padding / margin /
    border-width 的四边与 CSS 简写（`cssShorthand4` 拼装）；`engine/html5/defaultcss.go`
    补 `textarea { font-family: monospace }` UA 规则、并把 `select` 的 UA padding 修正为 `0`
    （Edge 实测 `pad=0px`，此前误写 `1px`）。
- **验收证据（① ②）**：`formtext_probe.html` / `g1_formctl.html` 的
  `fontFamily`/`fontSize`/`padding`/`margin`/`borderWidth` 与 Edge **逐项一致** ——
  `i1` `Arial/13.3333px` + `pad=1px 2px`；`b1`/`t5` `pad=1px 6px`；`s1`/`t6` `pad=0px` 且
  `bor=1px`；`t1`/`t7` `monospace/13.3333px` + `pad=2px`；`t2` `mar=3px 3px 3px 4px`；
  `t3` `mar=3px 3px 0px 5px`；`t4` `mar=2px`。
  产物：`dev/output/wbui-audit/{formtext_probe,g1_formctl}.{edge,wbui}.txt`。
- **③ 行内基线 top —— `WONTFIX`**（与 H8 同族：IFC 行盒基线公式）：
  - **根因**：`engine/layout/inlineformattingcontext.go` 的表单控件基线机制
    （`formControlBaselineFromBorderTop` + `maxBaseline`/`maxDescent`）里，
    `align = margin.Top + off` 与定位式 `top = 行盒顶 + maxBaseline - off` **恰好互相抵消**
    ⇒ 控件恒贴行盒顶（偏移 0）；而 Chromium 把行盒基线至少抬到父字体的度量行高。
  - **Edge 实测**（新增探针 `h2_baseline_formula.html`：控件相对容器顶的偏移随父字体变化）：
    父 `8px`→**0**、`16px`→**4**、`16px/line-height:20px`→**2**、`32px`→**22**；纯
    inline-block（`span 30×30`）→ 0（wbui 同为 0，一致）。
  - **已实测的改动风险**：按「`maxBaseline` 下限 = 父字体度量行高 `textHeight`」修改后，
    探针 `a` 由 10 跳到 **18.93**（下沉过度，Edge 为 14）——`textHeight`（`fontLineGap`）
    与 Chromium 的字体度量/取整口径不一致；且 `maxBaseline` 是**行盒共享**量，改动会波及
    所有含表单控件的行盒（`cssprobe` 的 form 用例、consistency 套件的 `form_controls`）。
    **已回退**，`git diff` 确认无残留。
  - **相关回归单测**：`TestInlineReplacedTallerThanTextStaysInsideLineBox`（与 H8 共用；
    改 IFC 行盒公式必须先保它不回归）。
- **复现**：`dev/tools/gprobe_cmp.sh dev/fixtures/webshot/formtext_probe.html formtext_probe`
  （另有 `g1_formctl`、`h2_control_baseline`、`h2_baseline_formula`）。

### H3. `display:list-item` 与伪元素块级化 — **`DONE`**（2026-10 第 4 次监督轮修复）

- **现象**：`li` computed `display` 报 `block`；`.p::before/::after` 被当块级（`#ps` 高 72）。
- **Edge 实测**：`li` display=`list-item`、`ls`=`disc`/`square`/`decimal`/`none`；
  `#ps` rect=`0,96,1254,24`。
- **根因（两处独立缺陷）**：
  1. **三处 UA display 映射不一致**：`dom.go` 的 `withDisplayFallback` 与
     `uaDefaultDisplayFor` 是**两份重复表**、都把 `li` 当 `block`；而 UA 样式表
     （`html5/defaultcss.go` 的 `li{display:list-item}`）与 `applyDefaultDisplay`
     （`style/resolver.go`）都是 `list-item`。且 `computedStyleFor` 只收文档 `<style>`、
     **不收 UA 样式表** ⇒ computed 完全依赖这张表 → 报 block。
  2. **伪元素绕过内联流**：`layout/box.go` 的 `buildChildren` 里 inline 子元素与文本都进
     `inlineRun`、由 `flush()` 包成匿名块；而 `appendPseudoBefore`（在 `inlineRun` 声明
     **之前**）与 `appendPseudoAfter`（在 `flush()` **之后**）用 `box.AddChild()` 直接挂
     **裸盒** ⇒ 三内容各自成块。渲染树 `createPseudoObject` 又恒造 `RenderBlockFlow`，
     与布局树（按 `cs.Display` 判 `IsInlineLevel`）不一致。
- **修复**：① `uaDefaultDisplayFor` 单列 `case "li": return "list-item"`，`withDisplayFallback`
  改为复用它；② 布局树/渲染树**同步**：新增 `pseudoBoxFor`（layout）/ `pseudoObjectFor`
  （rendering）统一判定，inline level 伪元素并入 `inlineRun`；`createPseudoObject` 按
  display 选 `RenderInline`，**并显式排除 out-of-flow**（`position:absolute/fixed` 必须
  blockify —— 否则 `RenderInline` 不生成盒子，`css-stack/composite.html` 的
  `#c5 .ps::before{position:absolute}` 会失效）。
- **验收**（`g7_listpseudo.html`）：

  | 测量 | Edge | wbui 修复前 | wbui 修复后 |
  |---|---|---|---|
  | `li` computed display | `list-item` | `block` | **`list-item`** |
  | `#ps` 高度 | 24 | **72** | **24** |
  | 伪元素与宿主文本 | 同行 | 各占一行 | **同行** |
  | marker（disc/square/decimal/none） | `•` / `■` / `3.` / 无 | 全部 `•` | **`•` / `■` / `3.` / 无** |

  截图对照：`dev/output/wbui-audit/g7_wbui2.png`（修复后）vs `g7_edge.png`（Edge）。
- **附带修复（同批）**：标记形状全变圆点的根因是 **`list-style` 简写未展开**
  （探针写 `list-style: square`，而 longhand `list-style-type:none` 生效）—— 与 H1 同类。
  已在 `style/resolver.go` 新增 `case "list-style"`（展开 type/position/image）。
- **复现**：`CGO_ENABLED=1 go run ./dev/probes/webshot -html dev/fixtures/webshot/g7_listpseudo.html -eval "document.getElementById('out').textContent"`

### H4. `CSS.supports()` 未实现 / 恒 false — **`DONE`**（第 5 次监督轮修复）

- **现象**：`g6_supports.html` 的 32 项探测 wbui **全 no**；Edge **31/32 yes**（仅 `@container` no）。
- **根因**：单参数 `CSS.supports(text)` 把参数当**选择器**解析 —— `display:grid` 被解析成
  tag `display` + 未知伪类 `grid`（→ false）；而 `display` 本身是合法 tag 名（→ 恒 true，
  与 Edge 相反）。且 `(prop: value)` 形式只校验属性名、**不校验值**。
- **修复**（`engine/js/bindings/dom.go`）：
  1. 单参数按 **@supports 条件文本**求值（裸声明 / 括号声明 / `not`·`and`·`or` 组合），
     支持自定义属性 `--x` 与厂商前缀；
  2. **移除**错误的选择器回退（Edge 实测 `div > p`/`a:hover`/`display` 一律 false）；
  3. 新增**值语法校验**：单关键字枚举属性（display/position/…）、多关键字枚举
     （contain/color-scheme/scrollbar-gutter/…）、其余通用语法检查（括号/引号配平）；
  4. 属性表增补现代属性补充表 `modernCSSProps`（contain/will-change/mix-blend-mode/
     writing-mode/mask-image 等 wbui 已有真实实现的属性）。
- **Edge 边界实测**（新增探针 `h4_supports_bounds.html`，26 项）：`display:grid` yes、
  `(display:bogusvalue)` **no**、`2arg display,bogusvalue` **no**、`a:hover` **no**、
  `div > p` **no**、`display` **no**、`--x:1` yes、`@container…` no。
- **验收证据**：`g6_supports.html` —— **edge 32 行(yes=31,no=1) / wbui 32 行(yes=31,no=1)，
  逐行完全一致**；`h4_supports_bounds.html` **26/26 行完全一致**。
  产物：`dev/output/wbui-audit/{g6_supports,h4_supports_bounds}.{edge,wbui,cmp}.txt`。
- **回归**：go test `./engine/...` **0 FAIL**；cssprobe **64/64 + 262/264**；
  cssoracle **62/64 + 260/264**（零退化；flex-flow 的「CSS supports accepts only the
  shorthand grammar」两参数用例仍 ok）。
- **复现**：`dev/tools/gprobe_cmp.sh dev/fixtures/webshot/g6_supports.html g6_supports`
  （wbui↔Edge 双跑并把 diff 落盘）。

### H5. `scrollbar-gutter` 与无内容 `overflow:scroll` 的 scroll 尺寸 — **`DONE`**（第 5 次监督轮修复）

- **现象**：① `scrollbar-gutter:stable` 未实现（Edge `client=185x60` vs wbui `200x60`）；
  ② `overflow:scroll` 无内容时 `scrollWidth/Height` 为 `0x0`（Edge `200x60`）。
- **修复**（`webkit/webview.go` 的 `getElementScrollMetrics`）：
  1. `scrollWidth/Height` 取 `max(内容包围盒, padding box)` —— CSSOM View §6.2/6.3：
     空内容的 `overflow:scroll` 容器其 scroll 尺寸 = client 尺寸；
  2. 新增 `gutterReservesSpace`：`scrollbar-gutter: stable|always` 且该轴 overflow ≠ visible 时，
     `clientWidth` 扣除滚动条宽度（`style.ScrollbarWidth`，默认 15px）。**只影响 CSSOM 读数**：
     `offsetWidth` 仍为 200（与 Edge 一致，布局宽度不变）。
- **补充**：`engine/js/bindings/dom.go` 把 `scrollbarGutter` 加入 getComputedStyle 的输出白名单
  `computedStylePropWhitelistCSV` 与初始值表 `uaInitialComputedValues`（初始值 `auto`）。
  ★ 注意：**H4 的「已知 CSS 属性表」与这里的「输出白名单」是两张独立的表**，两者都要有。
- **验收证据**：`g3_scrollbar.html` —— **edge 4 行 / wbui 4 行，差异 0，逐行完全一致**
  （`auto` client=200x60/scroll=300x200、`scroll` scroll=**200x60**、`hidden` 200x60/300x200、
  `gutter` **client=185x60** 且 `gutter=stable`）。
  产物：`dev/output/wbui-audit/g3_scrollbar.{edge,wbui,cmp}.txt`。
- **回归**：go test `./engine/...` **0 FAIL**；cssprobe **64/64 + 262/264**；
  cssoracle **62/64 + 260/264**（零退化）。
- **复现**：`dev/tools/gprobe_cmp.sh dev/fixtures/webshot/g3_scrollbar.html g3_scrollbar`

### H6. `getComputedStyle` 属性覆盖不全 — **`DONE`**（第 5 次监督轮收尾）

- **现象**：`padding`/`margin`/`borderWidth`/`lineBreak`/`transformOrigin`/`filter`/
  `willChange`/`clipPath`/`listStyleType`/`borderCollapse`/`borderSpacing` 返回 `undefined`。
- **Edge 实测**：均返回计算值（如 `listStyleType=disc`、`borderCollapse=separate`、`borderSpacing=0px`）。
- **第 4 次监督轮的进展（保留记录）**：
  1. `font` 简写（H1）与 `list-style` 简写（H3 附带）打通 `fontSize`/`lineHeight`/
     `fontFamily`/`fontStyle`/`fontWeight`/`fontVariant` 的取值链；
  2. `computedStylePropWhitelistCSV` 追加 `listStyleType` / `borderSpacing` / `lineBreak`
     —— 踩过一个坑：**初始值表只对白名单属性生效**（消费循环遍历
     `computedStylePropEntries`），只加初始值而不同时加白名单，读出来仍是 `undefined`；
  3. `uaInitialComputedValues` 补 9 个初始值（`listStyleType=disc`、`borderCollapse=separate`、
     `borderSpacing=0px`、`lineBreak=auto`、`willChange=auto`、`clipPath=none`、`filter=none` 等）；
     新增 `expandListStyleShorthand`（把 `list-style: square` 展开到 `list-style-type`）。

- **第 5 次监督轮的收尾修复**：
  1. **通用简写拼装** `composeBoxShorthand`（与 `expandBoxShorthand` 互逆）：四边齐备而简写
     缺失时拼出 `padding`/`margin`/`border-width`（`1px/2px/1px/2px` → `"1px 2px"`）；
  2. **border 四边补齐**：级联里只有 `border`/`border-width` 简写时按 CSS 2.1 §8.5.1 展开
     （style 为 `none`/`hidden`/未声明 → 宽度 `0px`；否则取简写里的宽度 token，缺省 `3px`）；
  3. **`transform-origin`** 新增 `resolveTransformOrigin`：输出绝对 px（未声明 = 50% 50%，
     按 **border-box** 解析；支持 `left/top/right/bottom/center`、`<length>`、`<%>`）。
     ★ 两个坑：(a) 它依赖盒子几何，**不能进 `computedStyleFor` 的 per-element 缓存**
     （首次调用若布局未就绪会缓存 undefined）→ 改在 getComputedStyle 的**回写阶段实时算**；
     (b) `GetElementBoxRectFast` 直读布局缓存，页面内**同步 `<script>`** 读时布局尚未跑会
     返回 0 → 用 `GetElementBoxRect`（forceLayout）兜底（浏览器语义：getComputedStyle 前必有布局）；
  4. `uaDefaultDisplayFor` 补 **table 系列**映射（`table`/`tr`/`td`/`thead`/…）—— 此前
     `getComputedStyle(table).display` 回退成 `"inline"`（Edge 为 `"table"`）；
  5. `defaultcss.go` 的 button UA 边框 `1px` → **`2px`**（Edge 实测 `borderWidth=2px`）；
  6. `textWrap` 补入输出白名单 + 初始值 `wrap`。
- **验收证据**：
  - 新增探针 `h6_misc_props.html`（16 项）—— **edge 16 行 / wbui 16 行，差异 0，逐行完全一致**；
  - `g1_formctl` / `g7_listpseudo` / `g2_transform` 三探针：**「仅属性差异」各 0 行**
    （原始差异只剩布局 `rect`：ul/select 宽度与 IFC 行盒 baseline，非本项范围）。

  | 测量 | Edge | wbui 修复前 | wbui 现状 |
  |---|---|---|---|
  | `li` 的 `listStyleType`（4 项） | `disc`/`square`/`decimal`/`none` | 全 `undefined` | **一致** |
  | `borderCollapse` / `borderSpacing` | `separate` / `0px` | `undefined` | **一致** |
  | `transformOrigin`（g2 的 `wo`） | `50px 20px`/`0px 0px`/`20px 20px` | `undefined` | **一致** |
  | `borderWidth`（g1 `t5` button） | `2px` | `1px` | **`2px`** |
  | `display`（g7 `tb` table） | `table` | `inline` | **`table`** |
  | `lineBreak` 非默认分支 | `strict`/`loose`/`anywhere`/`normal` | — | **一致** |

- **回归**：go test `./engine/...` **0 FAIL**；cssprobe **64/64 + 262/264**；
  cssoracle **62/64 + 260/264**（零退化）。
- **复现**：`dev/tools/gprobe_cmp.sh dev/fixtures/webshot/h6_misc_props.html h6_misc_props`
  （以及 `g7_listpseudo`、`g1_formctl`、`g2_transform`）。

### H7. computed `transform` 未按 CSSOM 归一为 `matrix(...)` — **`DONE`**（第 5 次监督轮修复）

- **现象**：`getComputedStyle(el).transform` 返回 `translate(20px, 10px)`（原样），
  Edge 返回 `matrix(1, 0, 0, 1, 20, 10)`。
- **修复**：新增 `engine/css/transform.go`（`TransformToMatrix`）—— CSSOM 序列化单一真相源：
  2D 归一为 `matrix(a, b, c, d, e, f)`（**逗号后带空格**）、数字用 **6 位有效数字**去尾零、
  `|v| < 1e-6` 的极小残差归零（`cos(90°) = 6.12e-17` → `0`）；`%` 参照 border-box、
  `em/ex/ch` 参照 font-size；3D 函数只有退化为 2D 等价（z=0 / 角=0 / sz=1）才归一，
  否则 `ok=false` 交回原值（**不把 3D 错报成 2D**）。`engine/js/bindings/dom.go` 的
  getComputedStyle 在 `transform` 上应用它（仅当值含 `%`/`em`/`ex`/`ch` 时才取几何，
  避免 `GetElementBoxRect` 的 forceLayout 常态开销）。
- **Edge 实测基线**（新增探针 `h7_transform_norm.html`，29 项）：`translate(20px,10px)` →
  `matrix(1, 0, 0, 1, 20, 10)`；`scale(0.5)` → `matrix(0.5, 0, 0, 0.5, 0, 0)`；
  `rotate(45deg)` → `matrix(0.707107, 0.707107, -0.707107, 0.707107, 0, 0)`；
  `rotate(90deg)` → `matrix(0, 1, -1, 0, 0, 0)`；`skew(10deg,20deg)` →
  `matrix(1, 0.36397, 0.176327, 1, 0, 0)`；`translate(50%,25%)` → `matrix(1, 0, 0, 1, 100, 25)`；
  `translate(1em,2em)` → `matrix(1, 0, 0, 1, 16, 32)`。
- **验收证据**：`g2_transform.html` 与 `h7_transform_norm.html` 的 **`tr` 全部与 Edge 逐字一致**；
  单测 `engine/css/transform_test.go` **4/4 PASS**（含 3D 回退与分量格式化）。
  产物：`dev/output/wbui-audit/{g2_transform,h7_transform_norm}.{edge,wbui}.txt`。
- **回归**：go test `./engine/...` **0 FAIL**；cssprobe **64/64 + 262/264**；
  cssoracle **62/64 + 260/264**（零退化）。
- **复现**：`dev/tools/gprobe_cmp.sh dev/fixtures/webshot/g2_transform.html g2_transform`

### H8. IFC 行盒高公式（`vertical-align` 各值）— **部分修复**（第 6 次监督轮，与 A3 同源）

- **现象**：`vertical-align` 各值未参与行盒高 —— wbui 恒 **24**；Edge `top`24 /
  `middle` 24.65625 / `bottom`24 / baseline **25**。
- **根因（第 6 次监督轮定位）**：**行盒基线/行盒高里完全没有 strut 项**。
  CSS 2.1 §10.8：行盒高 = `max(各 inline 盒 ascent) + max(descent)`，而 IFC 容器
  自身字体的 strut **也是其中一个参与项**；wbui 的 `maxBaseline`/`maxDescent`
  只由控件/替换元素从 **0** 起步贡献，所以控件永远贴行盒顶（relTop 恒 0）。
  Edge 公式（`h2_baseline_matrix` 46 例反解，逐例吻合）：
  `strutAscent = halfLeading + round(ascent)`、`strutDescent = halfLeading + round(descent)`，
  其中 `halfLeading = (lineHeight − (round(asc)+round(desc)))/2`（**可为负**）。
- **第 6 次监督轮已修**（4 处，均在 `engine/layout/inlineformattingcontext.go`）：
  1. `textAscent/textDescent` **各自四舍五入到整像素**（与 `fontLineGap` 同口径，
     Edge 实测 fs16→19/5、fs32→37/9）；
  2. `halfLeading` **允许负值**（原来 `if lineBoxH > asc+desc` 把负半行距归零，
     `line-height:20px` 时 strut ascent 被高估 2px）；
  3. `structAscent/strutDescent` 进入 `maxBaseline`/`maxDescent`
     （控件分支、替换元素分支、带显式高度的空 inline-block 分支）；
  4. `quirksNoStrut` 时 strut 归零（保 `form-control-quirks` 不回归）。
  另修 `isFormControlElement` 排除：表单控件**不再走通用替换元素分支**（其基线是
  内部文本基线，不是 margin box 底边）。
- **验收（实测）**：`h2_baseline_formula` **IDENTICAL（8/8）** ✓；
  `minibox` 的 **d 行盒高 25.0**（Edge 25）✓；`g4_inlineblock` 的 l1/l2/l4 行盒高
  24 **已对齐**（Edge 24）；`button` 高 21 ✓；回归三件套零退化。
- **剩余（本项未闭环的部分）**：`v_top`/`v_bottom`/`v_middle` 的**专项对齐语义**
  （Edge relTop 0 / 3 / 4.156，wbui 均 4）、`h2_control_baseline` 的 top 累积差
  （c 行 −0.156、d 行 −3.84）、`ib_empty`（Edge ctrlH 0 / relTop 19）、
  `g4_inlineblock` 的**块宽** 754 vs 1280（属块级宽度，非行盒）。
- **测试期望校正（附证据）**：`TestInlineReplacedTallerThanTextStaysInsideLineBox`
  原期望 wrap 高 **64**，但 Edge 实测为 **68.5**（= img 64 + strut descent 4.5）——
  见探针 `dev/fixtures/webshot/h2_replaced_linebox.html`（Edge `#wrap h=68.500`）。
  期望值按浏览器实测校正为 68.5（容差 0.5 覆盖 wbui 68.25 的度量精度差），
  **测试本身仍钉住同一语义**（img 在行盒内、顶偏移 0）✓。
- **复现**：`CGO_ENABLED=1 go run ./dev/probes/webshot -html dev/fixtures/webshot/g4_inlineblock.html -js "document.getElementById('out').textContent"`

### 第 6 次监督轮（2026-10）：4 项执行记录

1. **对照工具口径**（`dev/tools/gprobe_cmp.sh`）：diff 加 `--strip-trailing-cr`；
   Python 写文件指定 `newline=''`（Windows 文本模式会把 `\n` 写成 `\r\n`，此前
   8 份 `*.cmp.txt` 有 6 份是纯 CRLF 噪声）；两侧末尾换行归一。
   **结果**：`g6_supports` / `g3_scrollbar` / `h6_misc_props` **空 diff（IDENTICAL）** ✓
2. **`h7_transform_norm`**：监督者所指「wbui 的 `wo` 29/29 全 undefined」**与实测不符**
   ——归一化后 `wo=100px 50px` 出现 **29 次**、`wo=undefined` **0 次**（旧产物的假象）。
   但**另有一个真实缺陷**：空值 `transform:  ` wbui 返回 `tr=`（空串）而 Edge 为 `none`
   —— 根因 `styleProxy.Set` 只判 `strVal == ""`，未把**空白串**视为移除属性
   （`setProperty` 路径同时缺空白处理与缓存失效）。已修。**`h7` 现 IDENTICAL**，
   `tr` 与 `wo` 双字段 **29/29** ✓。**§9 H6 更正**：H6 声称的「`transform-origin`
   绝对 px 化 `DONE`」在动态插入元素路径上确实成立（29/29 一致），但 H6 未覆盖
   「空值 transform」一项——该项已在本轮补齐并记为 H6 的收尾。
3. **`g2` 的 `#sc` 几何**：Edge `rect=0,140,50,20` vs wbui `0,70,50,20`。
   判定：**不是 margin/padding 错**（同产物里 `tx`/`abs` 两侧一致），而是
   **transform 被错误作用于视口绝对坐标** —— `webkit/webview.go` 把
   `rx0-sx, ry0-sy`（元素左上角的视口坐标）直接传给 `TransformRect`，而该函数
   语义是「绕元素原点变换」→ `scale(0.5)` 把绝对坐标 140 也缩成 70；
   且**完全没处理 `transform-origin`**（painter 端有，几何端没有）。
   修复：`engine/rendering/bounds.go` 新增 `TransformRectWithOrigin` /
   `TransformRectByStyleOrigin` / `TransformRectForStyle`（局部坐标 + 绕 origin
   + 平移回视口），祖先链与自身两条路径都带上 origin。
   **`g2_transform.cmp.txt` 现为空（IDENTICAL）** ✓
4. **IFC 行盒基线族**：先扩探针反解公式，再改代码 —— 见上方 H8 段（8/8 ✓、d=25 ✓）。

### 第 9 次监督轮（2026-10）：P0 纠错 + select 回归修复 + 30 项处置

#### P0-a｜matrix「HEAD 独有 6 行」的真实出处（归因更正）

**前一报告的错误陈述**：称「matrix 中 HEAD 独有 6 行 = select ctrlH 17→19」。
用 `comm -23 / -13` 比对 `h2_baseline_matrix.head.wbui.txt` 与 `.wbui.txt` 后证实**为假**：
- HEAD 独有 3 行：`t_text_before` / `t_button_text`（lineH=43）/ `f32_text_input`（lineH=83）
- 工作区独有 3 行：同 3 行，lineH=25 / 25 / 46（= Edge 值）
- **差异全在 lineH，与 select 无关**。select 的 ctrlH 17（218ea62 / 工作区）与 19
  （30f7440）是 HEAD 与工作区**都**存在的另一处遗留，不是这 6 行的出处。

**A/B 定位**（clean worktree `wt8 @218ea62`，与 `dev/tools/gprobe_cmp.sh` 同源探针）：
```
HEAD 基线                                    t_text_before lineH=43 / f32_text_input lineH=83
+ canvas.go + renderview.go + bfc.go          25 / 46      ← 3 例全消（= 工作区 = Edge）
只还原 renderview.go                           43 / 83      ← 复原
```
⇒ **`renderview.go` 的 `layout.CJKFontMetricsFunc` 注入是决定性出处**；
`canvas.go`（定义 `GlobalCJKFontMetrics` / `cjkTypefaceFor`）与
`blockformattingcontext.go`（`HeightIsDefiniteForBox`）是必需依赖（单独拷入不改变结果）。

**更正**：上一轮「canvas.go 与 h2 验收无关」的 A/B 结论**只对 h2_baseline_formula 成立**
（该组最小必需集合确为 fontmgr.go + computedstyle_data.go），**不能推广**到 lineH。
已提交 `8d33a61`。机制：Blink 行盒度量合并参与该行的所有字体；HEAD 上
`CJKFontMetricsFunc` 为 nil，layout 退回只算主字体 → 16px 得 43（应 25）、32px 得 83（应 46）。

#### P0-b｜select 19→17 的元凶（新暴露的回归）

**反证**（同一 clean worktree，把 `7b05238` 的两个文件还原到 `30f7440` 版本后重跑）：
```
还原后：c_select_f8/f16/f32 ctrlH = 19（正确）   同时 c_input_f16 = 23/23（错误）
```
⇒ **`7b05238` 是把 select 从 19 改坏成 17 的元凶**；且这是双向 trade-off —— 它把控件
字体族 `inherit`(Noto Sans SC，13.3333px 行盒 19) → `Arial`(行盒 15)，换来
input/textarea 对齐 Edge（21 / 41），代价是 select 被顺带拉低 2px。

**修复（不为 formula 牺牲 select）**：UA 样式给 select 加 `min-height: 19px`。
- `19 = UA 字体 13.3333px Arial 行高 15 + select 内部绘制留白 2 + border 1px×2`
- 那 2px **不占 CSS padding**（Edge 计算值 `pad=0px`），Edge 计算 `line-height` 是
  `normal` → **既不能加 padding 也不能加 line-height**；min-height 是唯一不污染
  computed 样式、又不动 formula 相关路径的补法。提交 `0bfc206`。

**效果**：h2_baseline_matrix **30 行 → 10 行**；select 7 例（`c_select_f8/f16/f32` +
`l_select_lhnormal/20px/15/40px`）ctrlH 全部 19，relTop / lineH 逐项等于 Edge；
`c_input_f16` / `c_button_f16` 保持 21 / 25；`h2_baseline_formula` 保持 IDENTICAL。

#### P1｜30 项悬挂改动处置（git status 归零）

| 提交 | 任务线 | 内容 |
|---|---|---|
| `8d33a61` | H2 度量（本线） | canvas.go / renderview.go / blockformattingcontext.go |
| `0bfc206` | H2 度量（本线） | defaultcss.go：select `min-height: 19px` |
| `8363aef` | 表单控件交互 | forminteract.go（新）+ host.go / interact.go 逻辑归位 + html5/{select,input,details}.go + 5 个回归测试；删 zz_seldebug_test.go |
| `01e98d8` | 渲染管线（**独立线**） | 层叠绘制顺序（WebKit collectLayers 语义）+ 变换动画路径（修复 @keyframes/transition 的 translate/scale 完全不动） |
| `c4c0d8b` | 布局/样式/JS 小修 | popover §10.3.7/§10.6.4 margin-auto 居中、:checked 失效链路、table fc 行盒移位、boxgeometry 诊断开关 |
| `ec6ddfe` | 探针资产（**独立线**） | dev/probes/idepage/src/fwtest.jsx |

**验收**：`git status --porcelain` = **0 项**（除本条已入库的台账）。
每步 `go build ./...` OK、`go test ./engine/... -count=1` → 23 包 ok / 0 FAIL。

#### P2｜h2_baseline_matrix 剩余 10 行（5 例，未收敛）

值取自 `gprobe_cmp.sh` 最新产物（Edge 侧与 wbui 侧各 47 行，非假 IDENTICAL）。

| 用例 | Edge | wbui | 差异 |
|---|---|---|---|
| `v_middle` | relTop=4.156 lineH=25.156 | relTop=4.000 lineH=25.000 | Edge 带小数（vertical-align:middle 的基线取整口径未确认） |
| `v_top` | relTop=0.000 lineH=24.000 | relTop=4.000 lineH=25.000 | 垂直对齐位置 + 行盒高 |
| `v_bottom` | relTop=3.000 lineH=24.000 | relTop=4.000 lineH=25.000 | 同上 |
| `t_text_bigger` | relTop=22.000 lineH=46.000 | relTop=4.000 lineH=46.000 | lineH 已一致，仅 relTop |
| `ib_empty` | relTop=19.000 **ctrlH=0.000** lineH=24.000 | relTop=0.000 ctrlH=24.000 lineH=24.000 | Edge 空 inline-block 高度为 **0** |

**一律未标 WONTFIX**（无实测依据不得标）；根因待第 10 轮定位。

#### P3｜台账入库（本条）

`dev/audit/WORKITEMS.md`（本文件）此前位于 `dev/output/wbui-audit/`，被
`.gitignore:68` 的 `/dev/output/` 忽略 —— 与「探针/夹具未入库」同型：
结论无法在 HEAD 上复现。现移出 gitignore 入库。

#### 本轮其余产物（仍在 dev/output，gitignore）

`h2_baseline_matrix.{wbui,edge,cmp}.txt`（10 行 diff 的最新证据）、
`h2_baseline_matrix.head*.{wbui,edge,cmp}.txt`（HEAD 与 30f7440 基线）。

#### P2 根因定位（第 9 轮追加；读码 + 探针口径确认）

探针口径（`h2_baseline_matrix.html` L97-100）：
`relTop = 元素 top − host top`、`ctrlH = 元素高`、`lineH = host 容器高`。
用例定义：L52-57 的 `v_*` = `<input style="vertical-align:X">`（父 16px/normal）；
L63-64 `t_text_bigger` = `<span style="font-size:32px">X</span><input>`；
L72-73 `ib_empty` = 空 `<span style="display:inline-block;width:30px">`。

| 用例 | 根因 | 性质 |
|---|---|---|
| `v_top` | `engine/layout/inlineformattingcontext.go` L1028-1074 的 vertical-align 分支**只覆盖 `middle`(L1033) 与 `baseline`(L1050)**，无 `top` → 退化为 baseline 的 relTop=4；Edge 为 0.000（子盒顶贴行盒顶） | **实现缺口** |
| `v_bottom` | 同上，无 `bottom` 分支 → 退化为 relTop=4；Edge 为 3.000（= lineH 24 − ctrlH 21，底对齐） | **实现缺口** |
| `v_middle` | wbui 走 L1033 middle 分支（relTop=4.000）本身合理；差 0.156 与 lineH 的差（Edge 25.156 / wbui 25.000）同步出现 → **先分离「行盒高口径」再判断**，不能直接归因于 middle 公式 | 口径待分离 |
| `t_text_bigger` | 行内 `32px span` + `input`：Edge 让 input 落到 relTop=22（行盒被 32px 文字撑高后仍按基线对齐），wbui 给 relTop=4 —— 同行大字号对行盒的撑高没有传导到 input 的基线对齐（lineH 两边都是 46，说明撑高本身已实现，差在**对齐传导**） | **实现缺口** |
| `ib_empty` | Edge 空 inline-block 高度 **0**（`ctrlH=0.000`）；wbui 给 24（套用 host 行高）。空 inline-block 无内容时应为 0 高 | **实现缺口** |

**结论**：4 项为真实实现缺口（`top`/`bottom` 分支缺失、空 inline-block 高度、行内大字号撑高的对齐传导），
**一律未标 WONTFIX**；1 项（`v_middle`）需先分离行盒高口径。均待第 10 轮修复，
修复后必须复跑 `gprobe_cmp.sh` 并附两侧行数相等的证据。

#### P3｜遗留项补记（第 9 轮；含 min-height 修复的跨探针验证）

**g1_formctl（7 行；重跑于 13:00，即 `0bfc206` 之后）—— 2 项遗留**

| 行 | Edge | wbui | 判定 |
|---|---|---|---|
| t5 | `rect=8,109,36.015625,21` | `rect=8,109,36.012969970703125,21` | 宽度小数精度差 0.0027；位置/高度/样式逐项一致 |
| t6 | `rect=8,135,31,19` | `rect=8,135,10.893206596374512,19` | **高度与 y 已被本轮 min-height 修复对齐**（17→19 ✓、136→135 ✓）；仅剩**宽度**：Edge 31（select 无内容时的默认宽）vs wbui 10.89（按空文本算） |
| t7 | `rect=8,154,161,21` | `rect=8,154,166,21` | textarea（monospace/13.3333px）宽度差 5px |

**g4_inlineblock（重跑于 12:26，本轮改动不经此路径）—— 两类遗留**

| 行 | Edge | wbui | 判定 |
|---|---|---|---|
| l1–l4 | 宽 **754** | 宽 **1280** | 块级宽度：Edge 754 = 容器内容宽，wbui 1280 = 视口宽 → 探针宿主宽度未约束；**先确认夹具意图再定性**，不排除口径差 |
| l3 | 高 24.65625 | 高 24 | 行盒高小数（与 `v_middle` 同类口径问题） |
| l4 | y=89.65625 | y=89 | 上述小数的累加 |
| w1 | 宽 29.65625 | 宽 29.648 | 小数精度 |
| i1 / i2 | x=33.59375 | x=33.584 | 同上 |

**g5_mixedtext（m2）/ h2_replaced_linebox（0.25px 口径与容差）**：cmp 产物存在，
本轮未重跑，留第 10 轮处理 —— 不在此处给未经核对的结论。

**方法学备注**：以上所有 `Edge` 侧值均取自 `gprobe_cmp.sh` 的 `--dump-dom` 产物，
两侧行数相等（g1=7/7、matrix=47/47）且均非空，满足 P2-1 的假 IDENTICAL 防护断言。

---

## P2 / P3 收尾（第 10 次监督轮）

本轮提交链（每步 `go build ./...` OK + `go test ./engine/... -count=1` 无 FAIL）：

| 提交 | 内容 |
|---|---|
| `19a1952` | 表单控件 `vertical-align:top/bottom` 分派（v_top/v_bottom） |
| `26d133c` | 空 inline-block 高度归 0 + 底边基线定位（ib_empty） |
| `03da1fc` | 大字号 inline 文本盒参与行盒基线（t_text_bigger） |
| 本轮文档 | §0.3 门槛切冻结基线 + 本篇 |

**矩阵收敛**：`h2_baseline_matrix` 差异 **10 行 → 2 行**（仅剩 v_middle），
两侧行数恒为 **47/47**（`gprobe_cmp.sh` 硬断言，非空）。

### P2-a｜v_top / v_bottom（**已修**）

**根因（实测，非读码推断）**。用 `WBUI_IFC_DEBUG=1` 诊断日志实测：

```
[ifc-va] <INPUT id="v_top">    field="top"    prop="top"    formctl=true
[ifc-form-baseline] <INPUT id="v_top">    va="top"    relTop=4.000 off=15.000 maxBaseline=19.000
[ifc-va] <INPUT id="v_bottom"> field="bottom" prop="bottom" formctl=true
[ifc-form-baseline] <INPUT id="v_bottom"> va="bottom" relTop=4.000 off=15.000 maxBaseline=19.000
[ifc-va] <INPUT id="v_middle"> field="middle" prop="middle" formctl=true
[ifc-form-baseline] <INPUT id="v_middle"> va="middle" relTop=4.000 off=15.000 maxBaseline=19.000
```

三个来源取值**都正确**：`ComputedStyle.VerticalAlign` 字段有值（`resolver.go:2624`
对内联 style 生效）、`Properties["vertical-align"]` 有值、表单控件分支也读到了 `va`；
但落位恒为 `relTop = 4.000 = maxBaseline(19) − off(15)`，**与 va 取值完全无关**。

**「已有 `topOffset=0` 为何不生效」**：`L680/L1253` 的
`cldCS.VerticalAlign == "top" → topOffset = 0` **确实执行了**（字段实测就是 `"top"`），
但其 `SetTopLeft` 处于**更早的语句位置**，随即被表单控件基线分支的
`cldG.SetTopLeft(currentLine.y + maxBaseline − off, ...)` **无条件覆盖**
（`formControlBaselineFromBorderTop` 对表单控件恒返回 ok）。
⇒ 在 `L680/L1253` 处补 top/bottom 分支**永远不会生效**；修改点必须在基线分支内。

**修复**：在表单控件基线分支内按 `fcVA` 分派——`top` 顶边贴行盒顶、`bottom` 底边贴行盒底，
其余（baseline/middle/继承 middle）沿用原基线对齐。top/bottom 时不参与基线
（不改 `maxBaseline/maxDescent`、不进 `baselineBoxes`），只把行盒撑到至少容纳控件。

**验收**：`v_top` Edge `0.000/24.000` → wbui `0.000/24.000` ✓；
`v_bottom` Edge `3.000/24.000` → wbui `3.000/24.000` ✓；
二者已从 cmp 差异列表消失；`v_baseline` 保持 `4.000/25.000` 未回归。

### P2-b｜ib_empty（**已修**）

**根因（两处）**：夹具 `<span style="display:inline-block;width:30px"></span>`：

1. **ctrlH 24 vs 0** —— `inlineformattingcontext.go` 的「内容高 ≤0 → 用行高兜底」
   对**所有** inline 子盒无条件生效，把空 inline-block 撑成 24。
2. **relTop 0 vs 19** —— 高度非 0 时它被当成有高度的盒子贴行盒顶；且空 inline-block
   的基线处理此前只覆盖「**有显式高度**」的情形。

**修复**：① 行高兜底排除「无内容的 inline-block」（纯 inline span 高度对布局无影响，
`CSS 2.1 §10.6.1`，保留原兜底）；② 空 inline-block 基线分支拆两支——有显式高度沿用原逻辑，
无显式高度则高度 0、基线 = 底边 margin 边（`CSS 2.1 §10.8.1`）⇒ `top = strutAscent`。

**验收**：`ib_empty` Edge `relTop=19.000 | ctrlH=0.000 | lineH=24.000` →
wbui **逐项一致** ✓；未回归：`ib_30x30`、`ib_noheight`、`ib_va_middle`、`ib_va_top`。

### P2-c｜t_text_bigger（**已修**）

**根因**：`<span style="font-size:32px">X</span><input value="A">`，lineH 两侧已一致 46，
差在 relTop：控件落位 = `y + maxBaseline − off(15)`。
Edge `maxBaseline = 37`（32px span 的 ascent），wbui `maxBaseline = 19`（只有容器 strut）。
⇒ 同行「字号大于容器 strut」的纯 inline 文本盒**从未参与 `maxBaseline`**（该机制此前只由
替换元素与表单控件贡献）。

**修复**：对纯 inline 子盒计算其「行盒顶→基线」= `halfLeading + round(fontAscent)`；
若**大于** strutAscent 则提升 `maxBaseline` 并整体下移本行已放置的基线盒与文本段
（与表单控件分支同一机制）。仅大于 strut 时才生效 ⇒ 同级字号场景零影响。

**验收**：`t_text_bigger` Edge `22.000/21.000/46.000` → wbui **逐项一致** ✓；
未回归：`t_text_before`、`t_text_after`、`t_button_text`、`v_middle`、`v_baseline`。

### P2-d｜v_middle（**口径已分离 → WONTFIX（亚像素）**）

**来源分离（实测）**：写临时度量探针打印字体度量（用完即删）：

```
Noto Sans SC 16px: ascent=18.5600 descent=4.6080 xHeight=8.6880 lineGap=0.0000
                   round→19/5/0  lineH=24  未取整 strut=18.9760
```

Edge `relTop = 4.156` 精确吻合 **x-height 语义**：

```
top = strutAscent(19) − xHeight/2(4.344) − childH/2(10.5) = 4.156 ✓
```

⇒ **来源是 x-height 度量语义（middle 专用），不是取整误差**（baseline 用例 `4.000`
与 wbui 的取整口径精确吻合，可作对照）。

**为何不修（附实测依据）**：Edge 侧数据证明「**默认**控件」与「**显式 baseline**」完全相同
（`c_input_f16` / `c_button_f16` / `v_baseline` 均为 `4.000 | 25.000`），只有**显式 middle**
不同（`4.156 | 25.156`）⇒ Edge 的 input/button **默认是 baseline**；而 wbui 的 UA 给它们设的
是 `middle`（日志 `field="middle"`），当前因「把 middle 当 baseline 算」而**凑巧同值**。
若实现 x-height 语义，**必须同时**把 input/button 默认 `vertical-align` 改为 baseline，
否则 30 行已对齐用例（`c_input_*` / `c_button_*` …）会全部退化到 4.156。
收益 **0.156px（亚像素 < 1px）**，风险 = 改动 UA 默认值 + middle 公式，影响所有表单控件布局。
⇒ 按「仅亚像素精度类可 WONTFIX」的规则判 **WONTFIX**，依据如上（字号/度量/公式三者可复算）。

### P3｜四个遗留探针的重跑结论（本轮全部重跑，两侧行数相等且非空）

> ⚠️ **本节结论已被第 11 次监督轮取代，勿再引用**（见文末「## 第 11 次监督轮」）：
> - `g5_mixedtext` m2 —— **已修复**（现为 IDENTICAL）；且本节写的根因（「IFC 回填缺
>   显式 width 例外」）**不完整**：IFC 那处只是表层，真正把 200 撑成 309.376 的是
>   `renderview.go` syncOne 的 frame 撑开（详见 §11-2）；
> - `g4_inlineblock` 的 754 vs 1280 —— 本节判「探针视口口径差、非 wbui 布局 bug」
>   **方向正确但停在结论上、没修工具**；第 11 轮已给 `gprobe_cmp.sh` 加实测视口补偿
>   并重跑，l1–l4 宽度两侧均 1280（详见 §11-1）；
> - `g1_formctl` t6 / t7 —— **已修复**（31 / 161，与 Edge 零差，详见 §11-3）；
> - 最终保留为「已知接受差异」的只有亚像素项（§11-5 表）。
>
> 以下原文保留作历史记录。

| 探针 | 两侧行数 | 差异 | 定性 |
|---|---|---|---|
| `g5_mixedtext` | 5/5 | 1 行（m2） | **wbui 真 bug**（fixed-width 被内容撑开） |
| `h2_replaced_linebox` | 6/6 | 5 行（0.25px 级） | **亚像素** → WONTFIX |
| `g4_inlineblock` | 7/7 | 7 行 | **探针视口口径差**（主）+ 亚像素 → WONTFIX |
| `g1_formctl` | 7/7 | 3 行 | t5 亚像素 WONTFIX；t6/t7 ≥1px **遗留** |

#### P3-a｜`g5_mixedtext` m2：fixed-width 容器被内容撑开（**真 bug，遗留**）

夹具 `<div class="l" id="m2">abcdefghijklmnopqrstuvwxyz0123456789</div>`，
`.l{font:16px/24px sans-serif;width:200px}`（36 字符连续英文串，`word-break:normal` 不可断行）。

```
Edge: m2 | rect=0,32,200,24
wbui: m2 | rect=0,32,309.3760681152344,24      ← 宽被内容撑开（+109.376）
```

**根因**：IFC 收尾处的「内容宽回填」把容器宽改成了文本实宽
（`inlineformattingcontext.go` 的 `Update content width to match the actual text
content width` 段；该段已有 flex-item 例外，但**没有「显式 width」例外**）。
浏览器语义：`width:200px` 的块，内容溢出**不改元素 `border-box` 宽**。
**判定**：≥1px 真实缺口，根因明确；因涉及 IFC 宽度回填的通用路径（影响面大），
本轮聚焦 P2 四项未扩范围，列为遗留项。

#### P3-b｜`h2_replaced_linebox`：0.25px（**WONTFIX，亚像素**）

```
Edge: host h=68.500 top=0.000 | im h=64.000 | host2 top=68.500 | im2 top=68.500
wbui: host h=68.250 top=0.000 | im h=64.000 | host2 top=68.250 | im2 top=68.250
```

`im`（64×64 img）两侧**完全一致**；差集中在**行盒高**：`68.5 = 64 + strut descent`，
wbui `68.25`。即 13px/1.5 宿主下 strut descent 的**亚像素取值口径**差 0.25px（<1px），
并沿垂直方向等比传到后续兄弟元素。属亚像素精度类 ⇒ WONTFIX。
（附带事实：该夹具原意是校验 `z_replaced_baseline_test.go` 的 `want 64` —— 实测 Edge
`host=68.5 ≠ 64`，说明该测试期望值与浏览器行为不符，是**测试期望**问题，不在本次渲染改动范围。）

#### P3-c｜`g4_inlineblock`：754 vs 1280 是**探针视口口径差**（非布局 bug）

```
Edge: l1..l4 | rect=0,4,754,25 ...   （块宽 754）
wbui: l1..l4 | rect=0,4,1280,25 ...  （块宽 1280）
```

夹具 `.line{font:16px sans-serif}` —— **未设 width**，块宽 = 含块宽（视口宽）。
关键反证：同夹具里**显式定宽**的元素两侧一致 —— `i1/i2`（`width:30px` inline-block）
均为 `30` ✓。而 `gprobe_cmp.sh` 调 Edge 时**未传 `--window-size`**（脚本仅
`--headless --disable-gpu --no-sandbox --hide-scrollbars --virtual-time-budget`），
Edge 无头默认视口 ≠ wbui webshot 的 1280 ⇒ 754 与 1280 是**两侧视口不同**所致。
**结论：非 wbui 布局 bug**；建议后续给 `gprobe_cmp.sh` 的 Edge 调用补
`--window-size=1280,800` 以统一口径（口径修正后本项可再评估）。
其余为亚像素：`l3` 高 `24.65625 vs 24`、`w1` 宽 `29.65625 vs 29.648`、
`i2` x `33.59375 vs 33.584` ⇒ WONTFIX（<1px）。

#### P3-d｜`g1_formctl`：t5 / t6 / t7

| 行 | Edge | wbui | 判定 |
|---|---|---|---|
| t5 按钮 | `rect=8,109,36.015625,21` | `rect=8,109,36.012969970703125,21` | 差 **0.0027px**，位置/高度/样式逐项一致 ⇒ **亚像素 → WONTFIX** |
| t6 select | `rect=8,135,31,19` | `rect=8,135,10.893206596374512,19` | 宽差 **20.1px**：Edge 31 = 最长 option 文本宽 + Chromium select 的**下拉箭头区**；wbui 10.89 = 仅 option 文本宽 + border ⇒ **≥1px 遗留**（固有尺寸公式 `formcontrol.go`，缺箭头/额外内边距项）。高度与 y 已由上一轮 `min-height:19px` 修复对齐（17→19、136→135 ✓） |
| t7 textarea | `rect=8,154,161,21` | `rect=8,154,166,21` | 宽差 **5px**：`rows=1` 无 `cols` 的默认列宽公式差（monospace 13.3333px，Edge 161 / wbui 166）⇒ **≥1px 遗留**（同属固有尺寸公式） |

> t6/t7 同属 `engine/layout/formcontrol.go` 的**控件固有尺寸**公式，与已提交的表单
> **交互**任务线（`webkit/forminteract.go`）不同模块；本轮按「严格聚焦 P2 四项、
> 不扩散范围」的要求列为遗留，未在无实测公式依据的情况下擅改。

---

## 第 11 次监督轮（2026-10）：工具口径修正 + 三项遗留全部关闭

监督者判定第 10 轮「任务未完成」的三项，本轮**全部处理完毕**。顺序即监督者指定顺序。

### 11-1｜`gprobe_cmp.sh` 的 Edge 视口口径（**工具缺陷，已修**）

**实测先行**（不猜口径）：用探针页读 Edge 的 `innerWidth/innerHeight`，得到两个决定性事实 ——

```
--headless --hide-scrollbars（不传 window-size）  → inner=754x487   ← 原 754 的唯一来源
--headless --hide-scrollbars --window-size=1280,800 → inner=1254x707  ← 仍不是 1280！
--headless --hide-scrollbars --window-size=1306,893 → inner=1280x800  ✓
```

即 Edge 在 Windows 无头下 `--window-size=W,H` 的**实际视口小于 W,H**，偏移恒定
（宽 −26 / 高 −93 = 非客户区）；`--force-device-scale-factor=1` 与不传结果相同，说明
宿主本就是 DPR=1（该参数仅作防高 DPI 二次口径差的保险）。**修法不是硬编码 1306,893**，
而是让脚本自适应：

1. 用临时探针页**实测**当前 `--window-size` 下的真实视口；
2. 按实测偏移**反向放大** `--window-size` 并要求实测＝1280x800；
3. 最多迭代 3 轮，**探测失败即 `exit 4`**（拒绝带错误口径继续比对），成功时打印实测值自证。

顺带修掉一个真实 bug：探针页用相对路径写、却把相对路径交给 `cygpath -m` → 生成
`file:///dev/output/...` 无效 URL、Edge 加载失败返回空（首轮探测就因此失败）。改为
`cygpath -m "$ROOT/$VP_PROBE"`。

**复评 g4_inlineblock**（原「WONTFIX 口径差」结论**作废**）：

```
视口实测 1280x800 ✓
l1 | rect=0,4,1280,25       两侧一致 ✓
l2 | rect=0,33,1280,24      两侧一致 ✓
```

⇒ 宽度差异**全部消除**（754 与 1280 之争到此终结）；剩余见 §11-5。

### 11-2｜`g5_mixedtext` m2：定宽块被内容撑开（**已修，IDENTICAL**）

**定位过程（三层，逐层实测）**：

1. 先按监督者提示改 IFC「内容宽回填」加显式 width 例外 —— **无效**（m2 仍 309.376）。
2. 加诊断（`WBUI_IFC_W=1`）发现进 IFC 的 `box` 是**匿名包装盒**：`#<anon> unit=""`，
   `cs.Width.Unit` 恒为空 —— 因为匿名包装盒的 style 由 `NewComputedStyle + InheritFrom`
   构造、**故意不继承 width**（`box.go:738-745`），所以「显式 width 例外」在它身上判不出来。
   据此加「匿名块级盒不回填」（其宽恒为父内容宽，浏览器语义正确）。**仍无效** ——
   诊断显示匿名盒已回到 `borderW=200`，而 m2 的 rect 依旧 309.376。
3. 由此判定撑开**不在 IFC**，转向几何→渲染树同步：元凶是 `renderview.go:1558-1561`
   的 syncOne —— 它在 `syncChildren` 后把**任何含文本子节点的盒** frame 撑到文本右边界
   （`box.frame.Width = maxRight - box.frame.X`），此前只排除了表格内部盒与 flex item。

**最终修法（对称于既有的高度侧设计）**：

| 文件 | 改动 |
|---|---|
| `engine/layout/blockformattingcontext.go` | 新增 `widthIsAutoForBox` / **`WidthIsDefiniteForBox`**（对称于既有的 `HeightIsDefiniteForBox`，判定口径与 IFC/BFC 既有写法一致：`Unit==""` 或 `"auto"` 视为 auto） |
| `engine/rendering/renderview.go` | syncOne **两处** frame 撑开点（L1505 wrapper 分支、L1558 子文本分支）加 `!layout.WidthIsDefiniteForBox(lb)` |
| `engine/layout/inlineformattingcontext.go` | 回填段排除**匿名块级盒**（`box.Element()==nil && !IsInlineLevel()`） |

**为什么 m1/m3/m4/m5 没暴露**：回填/撑开只在内容宽 > 200 时才生效 ——
m1/m3/m4/m5 的回填值 176.992 / 192.000 / 192.480 / 192.000 均 < 200，恰好压在阈值下；
只有 m2（36 字符无断行机会，309.376）突破。

**验收**：`g5_mixedtext` → **IDENTICAL（5/5 逐项一致）**；m2 宽 = 200 ✓。

### 11-3｜`g1_formctl` t6 / t7：控件固有尺寸公式（**已修，与 Edge 零差**）

先用探针**实测反推公式**（监督者要求「先实测确认常量、不得臆测」），共三组：

**A 组（select / textarea 的基本量）** → 得到 textarea 单位列宽 7（= monospace 13.3333px
实测字符宽）、select 需补约 22px。

**B 组（select 的长度/字体扫描 + textarea 的字体对照）**：

| select option | Edge 宽 | round(文本宽)+22 |
|---|---|---|
| `A` | 31 | round(8.9063)=9 → 31 ✓ |
| `AA` / `AAA` / `5×A` / `10×A` | 40 / 49 / 67 / 111 | 18→40 ✓ / 27→49 ✓ / 45→67 ✓ / 89→111 ✓ |
| `i` | 25 | round(2.9688)=3 → 25 ✓ |
| monospace `10×A` | 92 | round(70)=70 → 92 ✓ |

⇒ **`select` 边框盒宽 = round(最宽 option 文本宽) + 20（箭头区） + 1px border×2**（零偏差）。

**关键反证**：textarea 在 `13.3333px` 下 **Arial 与 monospace 同宽**（cols=10 → 91，
cols=20 → 161）⇒ 列宽**与作者 font-family 无关**（Chromium 用 UA 控制字体度量），故只能
按字号推导，不能去查字体度量。

**C 组（font-size 扫描 8..32px，反推每列宽）**：8→4, 10→5, 12→6, 13.3333→7, 14→7,
16→8, 18→9, 20→10, 24→12, 32→16 ⇒ **每列宽 = round(0.5 × font-size)**；且常数项
（内容宽额外 +15）在 13.3333px 与 20px 两组**均为 15**（与字号无关）。

⇒ **`textarea` 边框盒宽 = cols × round(0.5×fs) + 15 + padding 4 + border 2**；
cols 默认 20、fs 13.3333 ⇒ `20×7 + 21 = 161` ✓。

**代码改动**（`engine/layout/formcontrol.go`）：

- 新增常量 `formTextareaColExtra = 15.0`、`formSelectArrowWidth = 20.0`；
- 新增 `textareaColumnWidth(box)`（`round(0.5×fs)`，含已知局限说明）；
- 把 select 的扫描抽成 `selectMaxOptionTextWidth`（round + 箭头区）、
  `clampSelectMaxWidth`（max-width 钳制）、`selectIntrinsicContentWidth`（内容宽）；
  `selectContentWidth` 改为复用它们（外盒语义不变，flex 消费方不受影响）；
- `inlineformattingcontext.go`：inline-block 尺寸推导处对 select 走
  `selectIntrinsicContentWidth`（此前 `formControlContentSize` 有意不为 select 返回尺寸，
  导致 select 被**裸 option 文本**撑开——这正是 t6 只有 10.893 的原因）。

**验收**：`g1_formctl` t6 `31` ✓、t7 `161` ✓（与 Edge 逐字相同）；差异 3 行 → **1 行**（仅 t5 亚像素）。

### 11-4｜矩阵与回归验证（全绿）

| 检查 | 结果 |
|---|---|
| `go build ./...` | OK |
| `h2_baseline_matrix` | **47/47**，差异仍 **2 行（仅 v_middle）** → 无回归 ✓ |
| `g5_mixedtext` | **IDENTICAL**（5/5）✓ |
| `g1_formctl` | 7/7，差异 1 行（t5 亚像素）✓ |
| `g4_inlineblock` | 7/7，差异 5 对（见 §11-5）✓ |
| `h2_replaced_linebox` | 6/6，差异 5 对（0.25px 亚像素）✓ |
| `go test ./engine/...` | 全通过 |

> **`go test ./...` 的 3 个 FAIL 与本次改动无关（已实测证明）**：`wb-ui/webkit` 包的
> `TestButtonTextVerticalCenter`、`TestCM6RangeMeasurementMatchesSkia`、
> `TestCheckedStateInvalidatesStyle` —— 用 `git stash` 回到 HEAD（`43e496e`）重跑，
> **输出逐字相同**，属既有失败。本次改动前只跑过 `./engine/...`（不含 webkit 包），
> 故此前未暴露。

### 11-5｜已知接受差异表（**关闭项**，不再作为「遗留 bug」）

判据：**仅亚像素（< 1px）且根因已由实测证明为「精度/语义口径」类**才登记于此；
表中每项都给出**数值 + 根因 + 不改理由**。凡 ≥1px 的实现缺口一律不在此表（本表当前无此类）。

| # | 位置 | Edge | wbui | 差值 | 根因（实测） | 不改理由 |
|---|---|---|---|---|---|---|
| A1 | `h2_baseline_matrix` / `g4 l3`：`vertical-align:middle` 行盒 | `relTop=4.156 lineH=25.156` | `relTop=4.000 lineH=25.000` | **0.156px**（g4 行盒高表现为 0.65625） | 度量探针实测 Noto Sans SC 16px `xHeight=8.688` ⇒ `xHeight/2=4.344`；Edge `19 − 4.344 − 10.5 = 4.156` **精确吻合** ⇒ 是 **x-height 语义**而非取整误差 | ① 亚像素；② 修它必须连带把 input/button 的 UA 默认 `vertical-align` 由 `middle` 改 `baseline`（Edge 实测默认=baseline），否则 **30 行已对齐用例全部退化**；收益仅 0.156px |
| A2 | `h2_replaced_linebox`：replaced 元素行盒高 | `host=68.500` | `host=68.250` | **0.25px** | `im`（64×64 img）两侧**完全一致**；差在行盒高 `68.5 = 64 + strut descent`，即 13px/1.5 宿主下 strut descent 的取值口径 | 亚像素（<1px），且并沿垂直方向等比传导（host2/im2 的 top 同差），无实现缺口特征 |
| A3 | `g4`：`w1` 宽 / `i2` x | `29.65625` / `33.59375` | `29.648` / `33.584` | **0.008** / **0.0098** | `white-space:pre` 三空格的文本测量亚像素；`i2` 的 x 由 `i1(30)+空格宽` 累积而来 | 亚像素（<1px）；属文本测量精度，非布局语义 |
| A4 | `g1_formctl` t5：按钮宽 | `36.015625` | `36.012969970703125` | **0.0027px** | 按钮内文本宽的小数位差异（位置/高度/padding/border/字体逐项一致） | 亚像素（<1px）；无任何可辨识的实现缺口 |

> 对 A1 的补充事实（与 A2 同源）：本表仅登记**渲染精度**类。若将来要动 A1，必须先
> 整体评估「UA 默认 vertical-align」的改动面，不可局部修 —— 这也是本轮维持不改的依据。

### 11-6｜收敛判据

| 探针 | 差异行数 | 剩余内容 |
|---|---|---|
| `g5_mixedtext` | **0** | 无（IDENTICAL） |
| `g1_formctl` | **1 对** | 仅 A4（t5 亚像素） |
| `g4_inlineblock` | **5 对** | 仅 A1（l3 及其垂直传导 l4/w1/i1/i2）+ A3（w1/i2 水平亚像素） |
| `h2_replaced_linebox` | **5 对** | 仅 A2（0.25px 及其传导） |

⇒ **四项探针的剩余差异全部落在「已知接受差异」表内（且均为亚像素 < 1px），
无未登记的 ≥1px 实现缺口 ⇒ 判定「无遗留」。**
