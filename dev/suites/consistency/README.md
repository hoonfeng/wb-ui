# Browser Consistency Verification Suite

验证 wb-ui 渲染与真实浏览器（Edge/Chrome headless）的一致性。

## 用法

```bash
# 运行全部用例
go run ./dev/suites/consistency

# 运行单个用例
go run ./dev/suites/consistency -case layout_block

# 指定浏览器路径（默认 Edge）
go run ./dev/suites/consistency -edge "C:\path\to\chrome.exe"

# 并排打印双方全部元素快照（排查某个字段差异来自哪个元素/哪个宽度，
# 报告只列「不一致的字段」，不足以看出「本元素的 X 是被谁顶偏的」）
go run ./dev/suites/consistency -case form_controls -dump

# 像素级用例（比几何用例慢：每个用例 spawn 一次 Edge，约 2~16s）
go test ./dev/suites/consistency/ -run TestPx -timeout 900s
```

报告输出到 `dev/suites/consistency/report/<case>.txt`（gitignored）。

## 架构

```
HTML ──┬──▶ Edge headless（参照物）
       │      └ 注入 JS collector → document.title = VP:WxH;GEO:元素数据
       └──▶ wb-ui 全管线（被测）
              └ html.Parse → style.Resolver → RenderTreeBuilder → RenderView.Layout
              └ 提取相同字段
                    │
                    ▼
              compare.go：按 tag#id 对齐 → 几何/样式/内容 diff → 报告
```

- `edge.go`：Edge headless + collector 注入 + 结果解析
- `wbui.go`：wb-ui 渲染管线 + 快照提取
- `cases.go`：11 个用例（布局/样式/动画/组件/交互）
- `compare.go`：对齐与容差比较

## 覆盖维度

| 类别 | 用例 | 验证内容 |
|------|------|---------|
| 布局 | layout_block/flex/position/float | 块流/弹性/定位/浮动几何 |
| 样式 | style_cascade/style_box | 特异性/继承/!important/边框/背景 |
| 动画 | anim_opacity/anim_transform | opacity/transform 关键帧 |
| 组件 | form_controls/component_list | 表单控件/列表/标题几何与状态 |
| 交互 | interact_click | 点击事件处理器触发 |

## 容差

报告头（`report/*.txt`）会**按字段打印这套真实容差**——此前只印
`Tolerance: ±2 px (geometry)`，而比较实际用的是下面这张 per-field 表，
属报告误导（按报告头去读会误判哪些偏差算通过）。

| 字段 | 默认 | 表单控件 | 文本块 |
|---|---|---|---|
| x | ±2 | ±14（控件 UA 私有 padding/border 计入宽度） | ±6（缩进累积） |
| y | ±2 | **±6**（行内基线对齐已实现，实测偏差 ≤3px；**曾是 30**，那是掩盖 vertical-align 未实现） | ±40（行高漂移） |
| w | ±2 | ±10（默认宽度依赖 UA 字符宽常量，见残差量化） | ±2 |
| h | ±12（块容器：div/table/tr/form… 高度由子盒几何决定） | ±12 | ±40（**仅多行文本块** p/li/h*/td/pre… 的逐行行高漂移） |
| display / color / bg / font-size / text / value / checked | 精确比较 | — | — |

- 文本块的 y 还会按「双方高度差」进一步放大（多行文本逐行累积的行高漂移）
- inline 元素跳过几何比较（无盒几何，用文本段近似）

## 已知差异（引擎待办；5–8 已修，见各条末标注）

1. **普通 inline-block / replaced 的 vertical-align**：只有表单控件实现了真正的
   行内基线对齐（同行共享一条基线、行盒高 = maxAscent + maxDescent）；普通
   inline-block（span/div）与 `<img>` 仍走 `vertical-align:middle` 的「自高中
   居中」近似，同行存在更高元素时会整体偏高。
2. **heading margin 折叠**：h1-h6 在 body 首子的 margin 折叠
3. **字体度量**：行高取决于系统字体（非引擎 bug）
4. **表单控件默认宽度残差**：textarea 168 vs Edge 161（差 7px，量化见下）

5. **flex 内滚动容器的 auto 高度未含末子元素 margin-bottom**：`overflow:auto` 容器的
   clientHeight 少算最后一个子元素的 margin-bottom（实测 `.ctrl-body` client 504 /
   scroll **513.5**），于是被误判成「内容溢出」→ 预留 15px 滚动条 → 子元素可用宽度 -15px
   （`.field` 189 vs Edge 204）。Edge 侧同一容器不溢出（534/534，无滚动条）。
   这是「整栏元素宽度系统性少 15px」的根因（MediaStudio 左栏像素差异 19.44%）。
   （**已修** `72ea2b9`：flex column 的 basis:auto 项改用预布局实测高度）
6. ~~**渐变「颜色 → transparent」的插值空间**~~（**已修**，2026-10）：Chromium 在
   **预乘空间**插值，wb-ui 曾按非预乘 → 中间色偏暗、衰减更快（夹具
   `linear-gradient(#ff0000,transparent)`：Edge 8800 红像素 / wb-ui **4800 → 8800**）。
   修复落点：`engine/rendering/gradient.go` 的 `premulStopsForShader`
   （等价色标重算，**非**逐像素；无 alpha 变化的渐变逐字段不变）。
7. **背景定位与绘制区域**：应为 padding-box（`background-origin` 初始值），当前用
   border-box → 带 1px 边框元素上的硬边渐变首行被边框盖住（Edge 可见 200px / wb-ui 0）。
   （**已修** `bbc0dee`）
8. **表单控件 UA `line-height: normal` 缺失**：button/input/textarea 继承了正文
   `line-height:1.5` → 按钮高 38 vs Edge 36。
   （**已修**：`dev/audit/WORKITEMS.md` §H2）
9. **getComputedStyle 细节**：`line-height` 序列化按**错误字号**折算（12px 字号的 h2
   报 21px = 1.5×14，Edge 18px；布局实测行高正确为 18px，故属序列化缺陷）、
   `width:100%` 未解析为使用值（报 `"100%"` 而非 `204px`）、颜色与长度序列化格式不一致
   （`#D7373F` vs `rgb(215,55,63)`、`0` vs `0px`）；`appearance` / `accent-color` 未实现。

> 5–9 的完整证据、最小复现与修复方向见 `docs/render-consistency-fixes.md`。

## 已修复（2026-10-10）

### ① 渐变色标的**长度位置**（`#2a2a2a 1px`）被丢弃 → 整条渐变消失

`parseColorStop` 此前只认 `%` 位置：`"#2a2a2a 1px"` 整串被当作颜色 → 解析失败 → 色标丢弃；
两个色标都这样时 `stops<2` → `parseGradient` 返回 nil。于是
`linear-gradient(#2a2a2a 1px, transparent 1px)` 这类**硬边条纹/网格**背景整片不画
（实测 MediaStudio `#stage .grid`：Edge 29924 网格线像素 / wb-ui **16**）。

修复：支持 `<length-percentage>`（`%` 与 `px`）；只在括号深度 0 处用最后一个空格切分颜色
与位置；`ColorStop` 增加 `PositionPx`，长度位置延后到绘制期由
`resolveGradientStops(stops, length)` 按渐变线长度换算（无长度色标时零回归）。

### ② 渐变背景层**未实现 background-repeat 平铺**

渐变层只画**一个** tile，`url()` 背景图早有平铺路径。于是 `background-size:32px 32px` +
默认 repeat 的网格/条纹渐变只出现一个方块（因子夹具：Edge 8300 红像素 / wb-ui 341）。

修复：新增 `paintBackgroundGradientTiled`，linear/radial 两条绘制路径统一接线；tile 覆盖
整个区域时走单次绘制快路径。顺带修正 `TestPaintGradientSized`（原先未声明
`background-repeat` 却断言「tile 外透明」，与 CSS 初始值 repeat 冲突），新增
`TestPaintGradientRepeats`。

**回归**：MediaStudio 整页像素差异 9.09% → **6.38%**，网格线像素 16 → **29941**
（Edge 29924，误差 0.06%）；`go test ./engine/...` 全通过；本套件 13/13 passed。

## 已修复（2026-09-22）

### ① 表单控件行内基线对齐（vertical-align:baseline）

`-dump` 实测 9 个控件的 y 偏差：**20/20/20/12/21/21/5px → 全部 ≤3px**
（btn 3 / chk 2 / rad 2 / range 3 / txt 2 / sel 2 / progress 1 / ta 0 / form 0）。

根因：`vertical-align:middle` 分支只按**自身**高度居中
（`lineH = max(lineHeight, childH)`，childH 就是自己的高度），完全不含同行更高的
元素，于是每个控件都贴行盒顶；而 `vertical-align:baseline` 根本没有实现。
Edge 第一行基线一致落在 y=44——textarea（高 36、基线取盒底边）的偏移最大，
其余控件按 `maxBaseline - 自身偏移` 下移。

修复（`engine/layout/formcontrol.go` + `engine/layout/inlineformattingcontext.go`）：

- `formControlBaselineFromBorderTop`：控件 border-box top → 基线的偏移
  （checkbox/radio/range/progress/textarea = border-box 高；input[text]/button/select
  = border-top + padding-top + font ascent）。
- IFC 行盒新增 `maxBaseline` / `maxDescent` / `baselineBoxes`：控件按
  `border-box top = 行盒顶 + maxBaseline - 自身偏移` 定位；后续出现更大的
  maxBaseline 时，把本行已放置的控件与已生成的文本段整体下移（行盒顶不变、
  基线随最高者下移）——这就是浏览器「同行共享一条基线」的语义。
- 行盒高改为 **maxAscent + maxDescent**：基线对齐后元素底边可以超过最高的盒子
  （textarea 高 38 但基线在其底边，而 input 的底边更低），沿用「max(border-box 高)」
  会让下一行起点偏高（progress 差 5px 的根因）。
- 文本段 Y 改为基线感知（`lineTextOffset()`）：本行被控件顶高时文本随基线下移，
  无控件参与时与原行为逐像素等价（零回归）。
- 容差由 30px **收紧到 6px** 后重跑：`=== 13/13 passed in 21.495s ===`。

### ② 表单控件默认宽度与背景色

`form_controls` 由 **5/9 差异 → 0/9（13/13 用例全通过）**。并排 dump 显示 5 处差异
其实只有 2 个根因：

| 字段 | Edge | 修复前 wb-ui | 根因 |
|---|---|---|---|
| `textarea#ta` w | 161 | 36 | textarea 的子文本让 inline-block shrink-to-fit 把内容宽写进 ContentWidth，后面 `formControlContentSize` 的 cols 路径被 `ContentWidth()<=0` 守卫跳过 → 缺 cols→宽度 |
| `input#range` x | 506 | 381 | **连带**：差 125px = textarea 宽度差 |
| `progress#prog` x,y | 8,51 | 516,19 | **连带**：第一行变短，progress 没换行到第二行 |
| `input#chk/#rad` bg | 透明 | 白 | UA 样式 `input{background:#fff}` 未对 appearance 型控件复位 |
| `input#range` bg | 白 | 透明 | UA 样式把 range 写成 transparent（与 Edge 相反） |

修复：
- `engine/layout/inlineformattingcontext.go`：inline-block shrink-to-fit 跳过
  「宽度由属性决定的表单控件」（`formControlContentSize` 判定），textarea 回归
  UA 固有宽度 cols×charWidth。
- `engine/html5/defaultcss.go`：checkbox/radio 显式 `background-color: transparent`，
  range 改 `#ffffff`（两侧都以 Edge 实测计算值为准，不再按推测写）。

**残差（可量化、且在设计容差内）**：textarea 宽 168 vs Edge 161（差 7px）——
wb-ui 用常量 `formControlAvgCharWidth=8.0px`（该值让 `input size=20` 的 177px
与 Edge **逐像素一致**），而 Edge 的 textarea 实测字符步进为 153/20 = **7.65px/char**；
两个控件在 Chromium 里走不同的度量来源，同一常量无法同时精确命中。7px 落在
width 容差 10px 内，故判 PASS；range 的 x 随之为 513 vs 506（差 7 ≤ x 容差 14）。

### ③ 容器高度回归（基线对齐的副作用，本轮修复）

| 版本 | form 容器高（Edge=60） | 判定 |
|---|---|---|
| 父提交 0b8e912 | 76（差 16） | hTol=40 判 OK |
| daaa7b7（基线对齐后） | **88（差 28）** | hTol=40 仍判 OK ← **被容差掩盖的真实回归** |
| 本轮修复后 | **62（差 2）** | hTol=12 判 OK（不再依赖大容差） |

根因：`inlineformattingcontext.go` 的
`totalHeight = (lastLine.y - contentY) + lineHeight` 用的是**跨行累积的全局
lineHeight**，而基线对齐把某一行的行盒顶到 44px 后这个全局值也变成 44，
于是最后一行（progress，实际约 16 高）被当成 44 高——容器底部多出 ~28px 死空间，
且其后的兄弟元素会被整体下推。

修复：行盒新增 `lineInfo.lineH`（**每行自身的行盒高度**，从基准行高
`baseLineHeight` 起步，只被本行更高的子盒/基线顶高）；换行推进
（5 处 `newY := currentLine.y + …`）、容器总高（`lastLine.lineH`）、
文本段的 `LineHeight`、`vertical-align:middle` 分支的行盒高全部改用该行自身值。
验证：form 高差 28 → **2px**，且 9 个控件 y 偏差仍全部 ≤3px（progress 仍在 52，
Edge 51）——推进量没跑偏。

同时把**块容器**（div/table/tbody/tr/form/fieldset/ul…）从「文本块 40px 容差」
里分出来（新增 `isMultiLineTextBlock`），一律按 12px 判——这类容器高度由子盒
几何决定，与字体无关，正是它让 form 的 28px 误差长期判 OK。

## 关键经验

- Edge headless 的 `--window-size` ≠ innerWidth（标题栏吃掉 ~26px 宽），
  必须从页面内 `window.innerWidth` 读实际视口
- `--dump-dom` 的 title 序列化遇到换行会提前终止，需定位 `</title>` 边界
- 页面内 JS 用 document.title 传数据最可靠（dump-dom 必然序列化 title）
- 内联样式由 resolver 原生处理，无需手动转 sheet
