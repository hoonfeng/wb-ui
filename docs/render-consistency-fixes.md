# 渲染一致性排查与修复（2026-10-10）

起因：实际界面（wb-ui 引擎渲染）与浏览器（Edge）观感差异明显。本文记录**同视口双端像素
对照**的排查方法、已修的视觉一致性缺陷（D1–D6、**D10**，含根因与证据）、剩余缺陷清单
（D7–D9、D11，契约/accent-color 类），以及载体页的收敛指标与差异归因（§0）。

载体页面：`cmd/mediastudio/web/index.html`（MediaStudio 录屏剪辑台，1280×860，深色三栏 +
网格舞台 + 时间轴），它是当前的渲染回归载体。

---

## 0. 结论摘要

载体页 `cmd/mediastudio/web/index.html`（1280×860、同视口同状态、动画冻结、tol=12）：

| 阶段 | 整页差异 | 平均通道差 | 左栏 | 舞台 | 右栏 |
|---|---|---|---|---|---|
| 修复前（D0） | 100086 px（**9.09%**） | 6.14 | 19.44% | 5.90% | 15.74% |
| D1/D2 后（渐变长度色标 + 平铺） | 70213 px（**6.38%**） | 5.55 | 19.44% | 0.51% | 15.74% |
| D1–D6 后 | 58390 px（5.30%） | 4.03 | 13.80% | 0.51% | 14.53% |
| **D1–D6 + D10 后（本次实测）** | **50765 px（4.61%）** | **2.51** | **9.29%** | 0.51% | 14.53% |

舞台网格线像素（色 `#2A2A2A`，Edge=29924）：修复前 **16**（≈完全没有）→ 修复后 **29941**（误差 0.06%）。

- **左栏 19.44% → 13.80% → 9.29%（累计 -10.15 pp，像素 32855 → 15690，-52%）**：
  前一段主因 **D5**（flex 内滚动容器漏末子 `margin-bottom` → 误预留 15px 滚动条、子元素宽
  189 vs 204 的**整片错位**）+ **D6**（按钮高 38→36）；后一段（13.80% → 9.29%，-4.51 pp /
  **-7625 px**）全部来自 **D10**（`input[type=range]` 的白色实底被实画），见 §0.2 与 §3 D10。
- **左栏纯白（三通道 > 250）像素：8868 → 209**（Edge 179；验收线 < 300）。整页纯白 8868 → 209（Edge 197）。
- **右栏 15.74% → 14.53%（-1.21 pp）**：降幅小是因为右栏差异的 **94.5%** 来自下面的「视频元素
  占位渲染」，与布局无关；D5/D6/D10 只影响其文字与控件边缘。
- **区域口径**（**双端一致**，`getBoundingClientRect()` 自上报核对）：左栏 `x[0,236) y[52,768)`、
  舞台 `x[237,1011) y[52,768)`、右栏 `x[1012,1280) y[52,768)`、顶栏 `y[0,52)`、时间轴 `y[768,860)`。
  该口径下重测**旧图**得整页 9.09% / 左栏 19.44% / 右栏 15.74%，与本文历史值逐位吻合 →
  **新旧数字直接可比**（另外：新截 Edge 图与历史 Edge 图逐像素 0 差异）。

### 0.1 「4.61% 里到底还剩什么」（三类对照实验，逐项闭合）

| 成分 | 像素 | 占整页差异 | 判据（两侧**同样**遮蔽后重测，见 §1.4） |
|---|---|---|---|
| 视频元素占位渲染（空 `<video controls>`，`x[1012,1280) y[91,238)`） | 26365 | **51.9%** | 隐藏该元素 → 右栏 **14.53% → 0.00%** |
| 文字光栅化（抗锯齿，非缺陷，见 §4） | 13819 | **27.2%** | 文字透明 → 整页 **4.61% → 3.36%** |
| 表单控件**残留**（D10 修完白底/勾色后的余项，见 D11） | 5959 | **11.7%** | 隐藏控件 → 整页 **4.61% → 4.07%**（左栏 9.29% → 5.76%） |
| 其余（含遮蔽手段自身副作用，作上界） | 4618 | 9.1% | 三者全隐藏 → 整页残余 **0.42%** |

闭合校验：26365 + 13819 + 5959 + 4618 = **50761 ≈ 50765**（四舍五入）。

> **读法**：与文字、视频占位、控件**无关**的残余只有 **0.42%**（舞台 0.07%、右栏 0.00%、
> 时间轴 0.07%、顶栏 0.31%、左栏 2.34%），集中在左栏按钮/分隔线的 1px 边缘与圆角。
> 控件项（5959 px）**不再是「未实现的 UA 外观」**——D10 修掉白底后它从 13584 px 降到此处，
> 余项是 range 已填充段的 `accent-color` 色差与轨道/圆角抗锯齿边缘（见 D11）。

### 0.2 D10 的因果证据：左栏改善 7625 px == 控件贡献减幅 7625 px

| 量 | D10 前 | D10 后 | 差 |
|---|---|---|---|
| 左栏差异（基准） | 23315 px（13.80%） | **15690 px（9.29%）** | **-7625** |
| 左栏差异（遮蔽 `range`+`checkbox` 后 = 剔除控件贡献） | 9731 px（5.76%） | 9731 px（5.76%） | **0** |
| ⇒ 控件贡献 | 13584 px | **5959 px** | **-7625** |

两个独立测量（左栏总降幅、控件贡献降幅）**逐像素相等**，且「遮蔽控件后」在 D10 前后同为
5.76% —— 说明 D10 只改了控件的绘制、没碰任何其它像素（与 §5 的 `go test`、consistency
13/13 一致）。

**复现命令**（引擎侧 / 浏览器侧 / 分区域统计）见 §1.2；差异归因实验见 §1.4；
本次全量数字见 §5「载体页收敛指标」。

---

## 1. 排查方法（可复现）

### 1.1 ★ 截图口径校准（最容易踩的坑，先做）

Edge headless 的两种模式**视口口径不同**，混用会产生大面积**假差异**：

| 模式 | 视口与 `--window-size` 的关系 | 验证方式 |
|---|---|---|
| 只跑 `--screenshot` | **视口 = --window-size**，截图像素尺寸 = window-size | 标尺页 `100vw×100vh` 红块铺满整图 |
| 只跑 `--dump-dom` | 视口 = window-size **− 26×140**（客户区偏移） | 页面内 `innerWidth/innerHeight` 自报 |
| 同一次调用同时传两者 | 两者口径**不一致**（dump 报 1254×720，截图仍是 1280×860） | — |

实测（`--window-size=1280,860`）：截图尺寸 1280×860、`100vw×100vh` 红块 bbox=(0,0)-(1279,859)，
而同一命令行的 `--dump-dom` 页面自称 `inner=1254x720`。

**结论**：
- 取**截图**：`--window-size=W,H` → 与 wb-ui 的 `webshot -w W -h H` 同口径，可直接逐像素比。
- 取**页面几何**：`--window-size=W+26,H+140` + `--dump-dom`（视口才等于 W,H）。
- ⚠️ 不要「用大窗口截图再裁成目标视口」——内容按大视口布局，裁剪后元素位置全错，
  会凭空造出「时间轴缺失」「stage 宽 16px」之类的假差异。

### 1.2 采集

```bash
# 引擎侧（无头全管线：解析→样式→布局→JS→绘制）
CGO_ENABLED=1 GOWORK=off PATH="/f/syproject/goskia/bin:$PATH" \
  go run ./dev/probes/webshot -html dev/output/tmp/studio.cmp.html -w 1280 -h 860 -frames 1 -out dev/output/tmp/studio.cmp.wbui.png

# 浏览器侧（视口 = 窗口尺寸）
"/c/Program Files (x86)/Microsoft/Edge/Application/msedge.exe" --headless --disable-gpu --no-sandbox \
  --hide-scrollbars --user-data-dir=<abs-profile> --window-size=1280,860 --force-device-scale-factor=1 \
  --virtual-time-budget=2500 --screenshot=<abs-out.png> "file:///<abs>.html"

# 差异统计（红=wb-ui 更亮 / 绿=Edge 更亮）
python dev/tools/pixdiff.py dev/output/tmp/studio.cmp.wbui.png dev/output/tmp/studio.cmp.edge.png \
  --tol 12 --top 12 --out dev/output/tmp/studio.cmp.diff.png

# 分区域统计（口径与 pixdiff.py **完全一致**；区域边界由双端几何自上报确定，见 §0）
python dev/tools/pixdiff_region.py dev/output/tmp/studio.final.wbui.png dev/output/tmp/studio.final.edge.png --tol 12 \
  --region 整页:0,0,1280,860 --region 左栏:0,52,236,768 --region 舞台:237,52,1011,768 \
  --region 右栏:1012,52,1280,768 --region 顶栏:0,0,1280,52 --region 时间轴:0,768,1280,860
```

**消除噪声**：动画相位与环境相关文本先冻结，否则差分会混入无关变化。对比副本注入
`#orb{animation:none!important}`（动画相位）、`#stamp/#log{visibility:hidden!important}`
（计时器文字、宿主 bridge 报错串——引擎与浏览器文案必然不同）。

### 1.3 几何 + 计算样式双端自上报（定位「哪一层丢的」）

在页面 `</body>` 前注入自上报脚本，把 `getBoundingClientRect()` 与 `getComputedStyle()`
结果写进 `document.title`（浏览器与 wb-ui 都能读回），再逐字段 diff：

- 浏览器侧：`--dump-dom` → 正则抠 `<title>`（title 里不能有换行，否则序列化提前终止）
- 引擎侧：`go run ./dev/tools/jsread -html <页> -js "document.title" -w 1280 -h 860 -out <txt>`

这一步把「观感不同」拆成了**可判定的字段差异**（宽度、行高、计算值格式…），
是后续所有定位的起点。

### 1.4 差异归因：同名元素遮蔽对照（「差多少」之外回答「差在哪」）

单看差异率说不清剩余差异**是什么**。做法：把**同一个副本**在**两侧同时**遮蔽某一类元素后重测，
差值即该类元素的贡献（本页实测见 §0.1）：

```bash
# 三类遮蔽各自注入 index.html 的 </head> 之前（副本落 dev/output/tmp/），再各跑一遍采集 + pixdiff_region：
#   1) 文字：body *{color:transparent!important}                              → studio.notext.{wbui,edge}.png
#   2) 控件：input[type=range],input[type=checkbox]{visibility:hidden!important} → studio.norange.{wbui,edge}.png
#   3) 视频：video{visibility:hidden!important}（与 2 合并）                    → studio.noall.{wbui,edge}.png
# 基准副本共用冻结注入：#orb{animation:none!important}#stamp,#log{visibility:hidden!important}
```

⚠️ `color:transparent` 会连带改变依赖 `currentColor` 的绘制（边框/背景若用 `currentColor`），
故「全遮蔽」残余值（0.42%）只作**上界**用，不能当纯缺陷量读。

---

## 2. 已修复缺陷

### D1 色标长度位置被丢弃 → 整条渐变消失 ★★

**症状**：`linear-gradient(#2A2A2A 1px, transparent 1px)` 这类**硬边条纹/网格**背景在 wb-ui
完全不画（Edge 有 29924 个网格线像素，wb-ui 16）。`.grid` 元素的 computed
`background-image` 两边**字符串相同**——所以问题不在解析结果可见性，而在解析实现。

**根因**（`engine/rendering/gradient.go` 的 `parseColorStop`）：位置解析**只认 `%`**。
遇到 `#2A2A2A 1px` 时没有 `%` 可找 → 整串 `"#2A2A2A 1px"` 被当作**颜色**传给
`parseColorSimple` → 失败 → 该色标返回 nil；两个色标都带长度位置时 `stops < 2`
→ `parseGradient` 返回 nil → **整层渐变被丢弃**（不是画错，是根本没画）。

**修复**：
- 新增 `parseStopPositionToken`：位置 token 支持 `<length-percentage>`（`%` 与 `px`）。
- 重写切分：只在**括号深度 0** 处取最后一个空格分隔颜色与位置（颜色本身可能含空格，
  如 `rgb(42, 42, 42) 1px`）。
- `ColorStop` 增加 `PositionPx`：长度位置要按**渐变线长度**换算，而线长要到绘制期才知道 →
  解析期只保留长度、位置置 -1；新增 `hasPixelStop`（有长度色标时**跳过**解析期的位置补全，
  否则会被当成「未指定」填上错误锚点）。
- 新增 `resolveGradientStops(stops, length)`：绘制期把 `PositionPx/length` 换算成比例并
  clamp 到 [0,1]，再补全未指定位置。线性渐变传 `gradLen`，径向传半径（椭圆取较大半径）。
  无长度色标时结果与旧路径逐字段相同（**零回归**）。

### D2 渐变背景层没有平铺（`background-repeat`）★★

**症状**：`background-size:32px 32px` + 默认 repeat 的网格背景整片消失——只画出一个 32×32 方块。

**因子隔离夹具**（`dev/output/tmp/gradfact.html` 的做法）给出清晰切分：

| 变体 | Edge 红/蓝像素 | wb-ui（修复前） |
|---|---|---|
| 实体色渐变（无 size） | 7800/7800 | 7800/7800（**逐像素一致**） |
| 含 `1px` 长度位置（无 size） | 200/19800 | 0/20000（→ D1） |
| 多层实体色（无 size） | 20000/0 | 20000/0（一致） |
| **+ `background-size`** | 8300/7425 | **341/372（只画一块）** |

**根因**：`painter.go` 的渐变层走 `paintLinearGradient(gdx,gdy,gdw,gdh,…)` **只画一个 tile**。
`url()` 背景图早有 `paintBackgroundImageTiled`（含 repeat 语义），渐变层漏了这一环。

**修复**：新增 `paintBackgroundGradientTiled`（`engine/rendering/backgroundimage.go`），
按 `background-repeat` 在背景区域内平铺 tile；tile 覆盖整个区域（未设 size 的常见情形）时
走单次绘制快路径，绘制次数与结果不变。`painter.go` 的 linear/radial 两条路径统一接线。

**顺带修正一个编码了错误行为的单测**：`TestPaintGradientSized` 原先**没有**声明
`background-repeat` 却断言「tile 之外透明」——等同于依赖「渐变永不重复」的旧行为，
与 CSS 初始值 `repeat` 冲突。已显式改为 `no-repeat` 并新增 `TestPaintGradientRepeats`
覆盖默认平铺。

**回归单测**（`engine/rendering/`）：`TestParseColorStopLengthPosition`、
`TestParseGradientHardStopWithLength`、`TestResolveGradientStopsLengthConversion`、
`TestPaintGradientRepeats`。

---

## 3. 缺陷清单（附最小复现 / 硬证据；状态见末列）

> 「视觉一致性」项（D3–D6、**D10**）已全部收口；D7–D9 属**契约一致性**（非视觉），
> D11 属 `accent-color` 取值（D8 家族），按需再做。
> 载体页残留差异的归因（哪些属未实现的 UA 元素渲染、哪些属文字光栅化）见 §0.1 与 §4。

| # | 缺陷 | 证据（Edge vs wb-ui） | 影响面 | 状态 |
|---|---|---|---|---|
| D3 | 渐变「颜色 → `transparent`」的插值应在**预乘空间**（CSS/Chromium 语义） | 夹具 `linear-gradient(#ff0000,transparent)`：Edge 8800 红 / wb-ui 修复前 4800（中间色偏暗、衰减更快）→ **修复后 8800**，逐行 R 与预乘模型误差 ≤1.2 | 所有遮罩/淡出/skeleton 渐变 | **已修**：`engine/rendering/gradient.go` 的 `premulStopsForShader` / `interpolateColor`（2026-10） |
| D4 | 背景定位与绘制区域应为 **padding-box**（`background-origin` 初始值），现用 border-box | 带 1px 边框的盒子上 `linear-gradient(#ff0000 1px,…)`：Edge 顶部 1px 红线可见（200 px），wb-ui 被边框盖住（0） | 带边框元素的渐定位整体偏 1px | **已修**（`bbc0dee`） |
| D5 | **flex 内的滚动容器 auto 高度未含末子元素 `margin-bottom`** | `.ctrl-body{overflow:auto}`：Edge client 236×534 / scroll 236×534（**不溢出**）；wb-ui client 236×504 / scroll **236×513.5**（溢出）→ 预留 15px 滚动条 → 子元素 `.field` 宽 189 vs Edge 204 | 所有 flex 里的滚动容器：容器矮 ~10px、**误触发滚动条**、内容宽 -15px（左栏 19.44% 差异的主因） | **已修**（`72ea2b9`） |
| D6 | 表单控件 UA 样式缺 `line-height:normal` | `#btn-record` 高度 Edge 36 / wb-ui 38；lineHeight Edge `normal` / wb-ui `21px`（继承了正文 1.5） | 所有 button/input/textarea 的高度与文字位置 | **已修**（`dev/audit/WORKITEMS.md` §H2） |
| D7 | `getComputedStyle('line-height')` 序列化用错字号 | `h2`（font-size 12px）：Edge `18px`（=12×1.5）/ wb-ui `21px`（=14×1.5）。布局实测行高是 18px（`.field .row` 两侧都 18）→ **仅序列化错**，但会误导网页自检脚本 | 依赖 computed style 做适配的页面脚本 | 待修（契约，非视觉） |
| D8 | `appearance` / `accent-color` 的 **computed 值**未实现（**绘制**部分见 D10/D11） | `input[type=range]`：Edge `auto` / `rgb(20,115,230)`；wb-ui `undefined` | 表单控件外观与配色 | **绘制部分已修**（D10：白底不画、勾/点恒白）；`accent-color` **取值**仍缺（D11） |
| D9 | `getComputedStyle` 未返回**使用值**、序列化格式不一致 | `width:100%` → Edge `204px` / wb-ui `"100%"`；颜色 Edge `rgb(215,55,63)` / wb-ui `#D7373F`；长度 Edge `0px` / wb-ui `0` | 同上（契约一致性，非视觉） | 待修（契约，非视觉） |
| **D10** | ★ `input[type=range]` 的 UA `background-color: #ffffff` 被**实画**（Chromium 只算、不画）；同族：checked 控件勾/点色应用**白色**而非元素文本色 | Edge（`#1e1e1e` 页、129×16 range，`x[2,131) y[2,18)`）：盒内只有 8 行滑轨像素，其余整行 `(30,30,30)`＝页面背景；wb-ui 画 `(255,255,255)` → 左栏纯白(>250) **8868** px（Edge **179**）。checked checkbox：Edge 勾 `(255,255,255)` / wb-ui 元素文本色（套件页=黑，载体页 `#E1E1E1`） | 深色主题页面的**所有** range（白块）+ 所有 checked 控件的勾/点色；左栏差异 **-4.51 pp / -7625 px** | **已修**（2026-10）：`engine/rendering/painter.go` 的 `nativeAppearanceRange` + `renderformcontrol.go` 的勾/点色 |
| D11 | range 已填充段 / checked 控件底色应取 `accent-color`（作者声明时） | 载体页 `accent-color:#1473E6`：Edge 填充段 `(20,115,230)` / wb-ui `FormControlColors.SliderFill` `#0075FF` `(0,117,255)`（B 差 25 > tol 12）；checkbox 底 wb-ui `(26,115,232)` vs Edge `(20,115,230)`（差 6，在容差内） | 设了 `accent-color` 的页面；D11 是控件残留 5959 px 的主要成分之一 | 待修（属 D8 家族：`accent-color` 取值未接；未声明时 Chromium 默认 `#0075FF` = wb-ui 现值，故套件不受影响） |

D5 的定位过程可复现：`dev/output/tmp/studio.diag3.html` 注入脚本读
`clientWidth/scrollWidth/scrollHeight/offsetHeight`，双端对照即见 513.5 vs 504。

> **口径校准与状态更新（2026-10）**
> - 上表 D6 的「wb-ui **38**」是**该缺陷修复前**的历史值。D6 已修复（UA 表单控件
>   `line-height: normal`，见 `dev/audit/WORKITEMS.md` §H2），此后同一元素
>   `#btn-record` 的「修复前」高度实测为 **204×34** —— 即后来 font-family inherit
>   缺陷（WORKITEMS §24）的基线；该缺陷修复后为 **204×37**（Edge 204×36）。
>   **同一元素同一状态只有一个权威值**：34 是现行值，38 已作废，勿混用。
> - D4 已修复：提交 `bbc0dee`（背景按 `background-origin` 定位区域，初始值 padding-box）；
>   D5 已修复：提交 `72ea2b9`（flex column 的 basis:auto 项改用预布局实测高度）。
>   两项均按 §0.3 门槛回归（八页像素不劣化）后入库。
> - **D3 已修复（2026-10）**：`engine/rendering/gradient.go` 的 `premulStopsForShader`
>   把「CSS 预乘语义的色标序列」转成「Skia 非预乘插值下**等价**的序列」（仍是一次
>   Skia 渐变绘制，**不是**逐像素；无 alpha 变化的序列原样返回）。夹具
>   `linear-gradient(#ff0000,transparent)` 红像素（R>150）4800 → **8800**（= Edge），
>   逐行 R 与预乘模型 `(1-t)·255+16t` 误差 ≤1.2；八页门槛前后逐像素 **0 差异**。
>   单测见 `engine/rendering/gradient_premul_test.go`，回归数据见 §5。
> - **载体页剩余差异的两大项不是缺陷（2026-10 实测，数字见 §0.1）**：空 `<video controls>` 的
>   UA 占位渲染（占右栏差异 **94.5%**）与 `input[type=range]` 轨道 UA 外观（占左栏差异 **58%**），
>   均属 **D8 同类**（UA 元素外观 / `appearance`、`accent-color` 未实现），本轮按范围不动。
> - ★ **归因更正（2026-10，第 5 次监督轮）**：上一条把 `input[type=range]` 的白色实底归为
>   「D8 同类 / 契约非视觉 / 本轮不动」——**该判断是错的**。实测（Edge 深色页 + 分控件遮蔽）：
>   range 的 UA `background-color` **算作白**（computed 与 Edge 一致，`defaultcss.go` 那条注释
>   本身没错），但 Chromium **不把它画出来**；wb-ui 却实画 → 左栏纯白(>250) 8868 px（Edge 179）、
>   左栏差异 **-4.51 pp**。这是**纯视觉缺陷**，已按 **D10** 修复。
>   **教训**：computed（契约）与 paint（视觉）是两条路径，「UA 元素外观」不等于「不可修」——
>   判定缺陷归属时必须两条路径各查一遍，不能由 computed 相符就推断绘制相符。
> - 空 `<video controls>` 的占位底（占右栏差异 **94.5%**）仍属未实现：已单独评估并记为
>   「**已知未实现**」（§4.2，含实测参数、成本与收益），不再写作「非缺陷」。

---

## 4. 非缺陷差异 vs「已知未实现」

> 本章此前把「可修的视觉缺陷」（range 白底）与「不可修项」（文字光栅化）并列成「非缺陷」，
> 属误导。现分两节：**4.1 非缺陷**（算法/环境固有，改不动）与 **4.2 已知未实现**（可修，
> 但成本/范围超出当前主线，**不是**「非缺陷」）。

### 4.1 非缺陷差异

- **文字光栅化（非缺陷）**：Edge 用 **LCD 子像素**抗锯齿、wb-ui 用**灰度 AA**（`dev/audit/WORKITEMS.md`
  的 E1：`glyphcmp` 实测 deffont **灰阶**差异 14.62%，而**同一字形二值轮廓差异仅 0.95%** →
  墨迹形状一致，差异全在抗锯齿灰阶层）。载体页同口径量化（遮蔽文字后重测，**D10 后**口径）：
  整页 **4.61% → 3.36%**（50765 → 36946 px）、顶栏 3.47% → 0.31%、舞台 0.51% → 0.07%、
  时间轴 1.76% → 0.07%，
  即**顶栏 91% / 舞台 86% / 时间轴 96% 的差异来自文字**。
  ⚠️ §0.3 八页门槛里的 **deffont 14.62% / fontshort 12.54%** 同属此类（指标是灰阶差异；二值轮廓
  0.95% 达标），**不计缺陷**。`dev/suites/consistency/pixels.go` 的像素用例历来避开文字，正是这个原因。
- ~~**视频元素占位渲染**~~ / ~~**表单控件 UA 外观**~~：**两条均已移出本章** ——
  `input[type=range]` 的白色实底经实测是**视觉缺陷**（**D10，已修**，见 §3）；
  空 `<video controls>` 的 media-controls 占位底是**已知未实现**（可修、但成本与范围超出本轮），
  实测参数与代价见 **§4.2**。原文把两者一并称作「属 D8、本轮不动」是**误判**（见 §3 末更正）。
- **动画相位**：对比时必须冻结（见 §1.2），否则同一元素的相位差会被算成差异。
- **宿主环境文案**：宿主 bridge 错误串（`fetch: request failed…` vs `TypeError: Failed to fetch`）
  天然不同。

### 4.2 已知未实现（**不是**「非缺陷」）

#### 空 `<video controls>` 的 media-controls 占位底 —— 评估结论：**本轮不实现**

**Edge 实测行为**（探针页，2026-10）：

| 形态 | Edge 绘制 | computed `background-color` |
|---|---|---|
| `<video>`（无 src、无 `controls`） | **完全不绘制**（200×100 区域 100% 透出页面背景 `(30,30,30)`） | `rgba(0,0,0,0)` |
| `<video controls>`（无 src） | **底锚定竖向渐变**：底部 `#000000`，向上约 **100px** 到 `#323232`(50,50,50)，高出 100px 的部分填 `#333333`(51,51,51) | `rgba(0,0,0,0)` |
| `<video style="background:#ff0000">` | 作者背景**正常绘制**（全红 20000 px） | `rgb(255,0,0)` |

渐变参数（4 个样本一致，含载体页）：盒内容高 100 → 整高渐变、无平台；高 148 → 顶部 **43 行**
`(51,51,51)` 平台 + **105 行**渐变；载体页 `#preview`（234×148 内容区）→ `y=91..133` 平台 43 行、
`y=134..232` 从 50 线性降到 0 —— 即渐变**恒约 100px、锚定盒底**，与盒高解耦。

**与 range 的对比（关键）**：作者 `background` 在 `<video>` 上是**正常绘制**的 —— 因此载体页
`#preview{background:var(--canvas)}` 的 `(20,20,20)` 是 wb-ui **画对了**的那一层，Edge 只是在
其上叠了 media-controls 的面板渐变（wb-ui 缺的是这一层叠加，不是画错了背景）。

**收益**：右栏 26365 px（占整页差异 51.9%）几乎全部由「Edge 渐变 vs wb-ui 恒定 `(20,20,20)`」构成
（`|v-20| > tol 12` 的行：渐变中 `v>32` 的前 ~84 行与 `v<8` 的后 ~31 行）；实现后可让整页从
**4.61% 降到约 2.2%**（估）。

**成本**：约 20–40 行 —— 识别「`<video>` 带 `controls` 且无帧无 poster」后，复用现成的
`paintLinearGradient` 叠一条底锚定渐变（`#323232` → `#000000`，恒定 100px，其余填 `#333333`），
加一个像素单测。技术上是 D10 同一量级的改动。

**为什么不实现（本轮）**：
1. `controls` 是一个**交互控件子系统**（播放/进度/时间/音量/全屏按钮 + 命中测试 + 悬停态），
   只画面板底会得到「像但不可用」的半成品，反而制造新的「看起来一致」的假象；
2. 引擎 UA 样式表当前**没有 media-controls 概念**，需要新的接入口与元素识别路径，与将来完整
   实现重复；
3. 本轮主线是「表单控件实底白块」（用户肉眼即见），且监督轮明确要求「video 只评估、不实现」。

**状态**：**已知未实现**（可低成本起步，但与完整 media controls 应一起做）。

---

## 5. 回归记录（本次）

| 检查 | 结果 |
|---|---|
| `go build ./...` / `go vet`（rendering、layout） | 通过 |
| `go test ./engine/... -count=1` | 全通过（仅列出无测试文件的包） |
| `go test ./engine/rendering/ ./engine/layout/ ./engine/style/` | ok |
| `go run ./dev/suites/consistency` | **13/13 passed**（21.3s） |
| MediaStudio 双端像素对比（**D1/D2 阶段值**；D3–D6 全修后见下节） | 9.09% → **6.38%**；网格线像素 16 → **29941**（Edge 29924） |

### D3 收口回归（2026-10）

| 检查 | 结果 |
|---|---|
| 夹具 `linear-gradient(#ff0000,transparent)` 红像素（R>150，200×100 盒） | 4800 → **8800**（Edge 8800） |
| 逐行 R 通道 vs 两条模型 | 修复后吻合预乘模型 `(1-t)·255+16t`（误差 ≤1.2）；修复前吻合非预乘模型 `255(1-t)²+16t`（误差 ≤1.3） |
| §0.3 八页门槛（vs 冻结 Edge 基线，门槛值见 `dev/audit/baselines/README.md`） | deffont 14.62 / vue 2.83 / react 3.04 / fontshort 12.54 / stack 2.56 / sticky 2.81（门槛 5.55） / zorder 0.84 / composite 2.87 % —— **全部 ≤ 门槛** |
| 八页「修复前 vs 修复后」（wb-ui 自身对照） | 逐页 **0.00%**（差异像素 0）——八页无 alpha 变化渐变，实证「零行为变化」 |
| `go test ./engine/... -count=1` | 全部 ok，无 FAIL |
| `go run ./dev/suites/consistency` | **13/13 passed**（20.0s） |
| `go run ./dev/suites/cssprobe` | 62/64；失败夹具 `img-density-and-alt` / `ratio-only-inline-images` —— **既有失败，逐字不变** |
| 性能（200×100 盒单次绘制 + 光栅化落地，中位） | 渐隐到 `transparent` 42.7µs（修复前等价路径 43.9µs，色标数不变 → 无回退）；硬边 `1px,transparent 1px` 58.1µs；两端半透明且 alpha 不同（细分为 25 色标）92.5µs vs raw 2 色标 57.9µs（+60%） |

### 载体页收敛指标（D1–D6 + D10 全修后，2026-10 实测）

| 检查 | 结果 |
|---|---|
| 整页双端对比（`cmd/mediastudio/web/index.html`，1280×860、冻结、tol=12） | **9.09% → 6.38% → 5.30% → 4.61%**（100086 → 70213 → 58390 → **50765 px**）；平均通道差 6.14 → 5.55 → 4.03 → **2.51** |
| **左栏纯白（三通道 > 250）像素**（验收线 < 300） | **8868 → 209**（Edge **179**）；整页纯白 8868 → 209（Edge 197） |
| 分区域（同口径，`dev/tools/pixdiff_region.py`） | 左栏 **19.44% → 13.80% → 9.29%**（32855 → 23315 → **15690 px**）；右栏 15.74% → **14.53%**（30210 → 27886 px，D10 不涉及，其 94.5% 是视频占位）；舞台 0.51%（2811 → 2805 px）；顶栏 3.40% → 3.47%；时间轴 1.76% |
| 差异归因（同名元素遮蔽对照，§1.4，**D10 后**） | 空 `<video controls>` 占位 26365 px（51.9%）；文字光栅化 13819 px（27.2%）；控件残留 5959 px（11.7%）；**三者全隐藏后残余 0.42%**（右栏 0.00%） |
| 口径可信度 | 新截 Edge 图 vs 历史 Edge 图 `--tol 0` **逐像素 0 差异**；按同一区域口径重测旧图得整页 9.09% / 左栏 19.44% / 右栏 15.74%，与历史值**逐位吻合** |
| 复现命令 | §0.1（结论表）/ §1.2（采集 + 分区域统计）/ §1.4（归因实验） |

> **结论**：D5（flex 滚动容器漏末子 margin → 左栏整片错位）+ D6（按钮高 38→36）+ D4 使
> 左栏 19.44% → 13.80%；**D10**（range 白色实底不再绘制）再降到 **9.29%**（累计 -10.15 pp，
> 像素 32855 → 15690）。整页 9.09% → **4.61%**。剩余 4.61% 的构成：**视频占位 51.9%**
> （已知未实现，§4.2）、**文字光栅化 27.2%**（非缺陷，§4.1）、**控件残留 11.7%**
> （D11 的 `accent-color` 色差 + 抗锯齿边缘）、**其余 9.1%**（上界，含遮蔽手段副作用）
> ——与文字/视频/控件无关的残余只有 **0.42%**。

### D10 收口回归（2026-10）

| 检查 | 结果 |
|---|---|
| 探针页 range 盒内逐行（深色 `#1e1e1e` 页、`<input type=range>` 129×16 @2,2） | Edge：`y=2..5` / `y=14..17` **整行** `(30,30,30)`（页面背景），只有 `y=6..13` 有滑轨像素；wb-ui 修复前同行为 `(255,255,255)`（纯白实底）→ 修复后为 `(30,30,30)` ✓ |
| 载体页左栏纯白（>250）像素 | **8868 → 209**（Edge 179；验收线 < 300 达标） |
| 左栏差异 | 23315（13.80%）→ **15690（9.29%）**，-7625 px；同口径「遮蔽 range+checkbox」左栏在 D10 前后**同为 5.76%**（9731 px）→ 因果闭合（§0.2） |
| 整页差异 | 58390（5.30%）→ **50765（4.61%）**；mean_ch 4.03 → **2.51** |
| `go test ./engine/... -count=1` | 全部 ok，无 FAIL |
| `go run ./dev/suites/consistency` | **13/13 passed**（17.7s；`form_controls` 不回归） |
| computed 契约（未受影响） | range `background-color` 仍为白（与 Edge 一致）；checkbox/radio 的 computed 本就透明（`defaultcss.go` 已显式 `transparent`）——套件的 `bg` 字段比较走 computed，故与本修复正交 |
| Edge 参考图稳定性 | 修复前后两次 Edge 截图 `--tol 0` **逐像素 0 差异**（bbox=None），差异全部来自 wb-ui 侧 |

## 6. 下一步建议（按收益排序）

1. ~~**D5**~~（**已完成**，`72ea2b9`）：flex 滚动容器高度含末子 margin；已跑全量
   consistency + cssprobe 回归。
2. ~~**D3**~~（**已完成**，2026-10）：最终落点既不是「扩 goskia（暴露
   `SkGradientShader::Interpolation`）」也不是「逐像素」——而是**等价色标重算**：
   `premulStopsForShader` 把预乘语义的色标序列改写成 Skia 非预乘插值下等价的序列。
   渐隐到 `transparent`（最常见：遮罩/淡出/skeleton）只需改写透明端的 RGB，
   色标数仍是 2 → 零额外开销；只有「两端都半透明且 alpha 不同」才细分（+60%）。
   故 **goskia 无需任何变更**。
3. ~~**D4 / D6**~~（**已完成**）；~~**D10**~~（**已完成**，2026-10）：`input[type=range]` 的白色
   实底不再绘制（computed 保持白）+ checked 控件勾/点色恒白 —— 左栏 **13.80% → 9.29%**、
   左栏纯白 8868 → **209**，见 §5「D10 收口回归」。
4. **D11**（`accent-color` **取值**接到控件绘制：range 已填充段 / checked 底色）：载体页场景已
   量化为「控件残留 5959 px」的主要成分之一（Edge 按 `accent-color:#1473E6` 画，wb-ui 用 UA 默认
   `#0075FF`）。与 **D8** 的 computed 侧（`appearance` / `accent-color` 值）**同源**，建议一起做：
   一次接入即可同时满足契约与视觉。
5. **空 `<video controls>` 的占位底**（独立主线，§4.2）：单做「底锚定渐变」可把整页从 4.61% 降到
   约 **2.2%**（估，收益最大），但它属 media-controls 交互子系统 —— 建议与播放/进度/时间/音量
   控件一起做，避免「像但不可用」的半成品。
6. **D7 / D9**：契约完备性（序列化格式与使用值），配合网页自检脚本需求再做。

> D3 的细分上限 `maxPremulSubdivs`（`engine/rendering/gradient.go`）为 24，是按
> 「每 ~8px 一个采样」的经验值；若后续遇到大量「两端半透明」渐变导致性能吃紧，
> 可下调该常量（精度与开销的旋钮），常见场景不受影响。
