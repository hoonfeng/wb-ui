# 渲染一致性排查与修复（2026-10-10）

起因：实际界面（wb-ui 引擎渲染）与浏览器（Edge）观感差异明显。本文记录**同视口双端像素
对照**的排查方法、已修的视觉一致性缺陷（D1–D6、**D10–D15**，含根因与证据）、剩余缺陷清单
（D7–D9 契约序列化类；载体页 checkbox 的 `vertical-align:-2px` 定位差 2px），以及载体页的
收敛指标与差异归因（§0）。

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
| D1–D6 + D10 后 | 50765 px（4.61%） | 2.51 | 9.29% | 0.51% | 14.53% |
| + D12（空 `<video controls>` 的 UA 占位面板） | 24420 px（2.22%） | 1.94 | 9.29% | 0.51% | **0.80%** |
| **D1–D12 全修后（本次实测）** | **21945 px（1.99%）** | **1.88** | **7.82%** | 0.51% | **0.80%** |
| + D13/D14（checkbox 描边 / range 几何，绘制层） | 21968 px（2.00%） | 1.78 | 7.83% | 0.51% | 0.80% |
| **+ D15（half-leading 按 Chromium 取整分配，布局层）** | **18236 px（1.66%）** | **1.39** | **4.75%** | 0.52% | 2.22% |

> **口径说明**：D13/D14 行沿用上表口径；**D15 行（2026-10-11 复测）**改用更严的口径——
> 冻结 `#orb` 无限动画（否则 wb-ui 与 Edge 相位不可比，混入 ~1% 噪声）、统一
> `--hide-scrollbars`（否则 Edge 内容区窄 26px，凭空多出整列差异）、并用**同一张 Edge 参照
> 图**对比「旧版代码 / 新版代码」两次渲染（旧版经临时 `git checkout` 渲染后立即恢复）。
> 该口径下旧版代码是 **2.25%（左栏 7.95%）**，故 D15 的前后对比（2.25% → 1.66%、
> 左栏 7.95% → 4.75%）为**同口径内部对比**，不可与 1.99% 那行直接相减。

舞台网格线像素（色 `#2A2A2A`，Edge=29924）：修复前 **16**（≈完全没有）→ 修复后 **29941**（误差 0.06%）。

- **左栏 19.44% → 13.80% → 9.29% → 7.82%（累计 -11.62 pp，像素 32855 → 13215，-60%）**：
  第一段主因 **D5**（flex 内滚动容器漏末子 `margin-bottom` → 误预留 15px 滚动条、子元素宽
  189 vs 204 的**整片错位**）+ **D6**（按钮高 38→36）；第二段（13.80% → 9.29%，-4.51 pp /
  **-7625 px**）来自 **D10**（`input[type=range]` 的白色实底被实画）；第三段
  （9.29% → 7.82%，-1.47 pp / **-2475 px**）来自 **D11**（`accent-color` 取值接入控件绘制）。
  见 §0.2 与 §3 D10/D11。
- **左栏纯白（三通道 > 250）像素：8868 → 209**（Edge 179；验收线 < 300）。整页纯白 8868 → 209（Edge 197）。
- **右栏 15.74% → 14.53% → 0.80%（-14.94 pp / -26345 px）**：前段的 94.5% 是空
  `<video controls>` 的 UA 占位面板；**D12** 一次实现该面板后，面板区
  `x[1012,1280) y[91,239)`（268×148）的差异 **26365 → 20 px**（39664 px 区域内），右栏只余 1541 px，
  且这 1541 px **全部在视频面板之外**（`y52..91` 标题区与 `y239..331` 产物列表的蓝色
  图标/文字像素，属文字类非缺陷；面板区本身仅 20 px）。
- **区域口径**（**双端一致**，`getBoundingClientRect()` 自上报核对）：左栏 `x[0,236) y[52,768)`、
  舞台 `x[237,1011) y[52,768)`、右栏 `x[1012,1280) y[52,768)`、顶栏 `y[0,52)`、时间轴 `y[768,860)`。
  该口径下重测**旧图**得整页 9.09% / 左栏 19.44% / 右栏 15.74%，与本文历史值逐位吻合 →
  **新旧数字直接可比**（另外：新截 Edge 图与历史 Edge 图逐像素 0 差异）。

### 0.1 「1.99% 里到底还剩什么」（三类对照实验，逐项闭合）

下表为 **D11/D12 之后**的实测（整页 21945 px = 1.99%）：

| 成分 | 像素 | 占整页差异 | 判据（两侧**同样**遮蔽后重测，见 §1.4） |
|---|---|---|---|
| 文字光栅化（抗锯齿，非缺陷，见 §4.1） | 13819 | **63.0%** | 文字透明 → 整页 **1.99% → 0.74%**（21945 → 8126 px） |
| 表单控件**残留**（D11 接入 `accent-color` 后的余项） | 3484 | **15.9%** | 隐藏控件 → 整页 **1.99% → 1.68%**（21945 → 18461 px）；左栏 7.82% → 5.76% |
| 空 `<video controls>` 面板**残余**（**D12 已实现**） | **20** | 0.1% | 面板区 `x[1012,1280) y[91,239)`（268×148=39664 px）逐像素差 **26365 → 20 px**，右栏整体 27886 → **1541 px** |
| 其余（含遮蔽手段自身副作用，作上界） | 4618 | 21.1% | 三者全隐藏（文字+控件+视频）→ 整页残余 **0.42%** |

闭合校验：13819 + 3484 + 20 + 4618 = **21941 ≈ 21945**（差 4 px 属遮蔽项之间的交叠与四舍五入）。

> **读法**：与文字、控件、视频面板**无关**的残余仍只有 **0.42%**（舞台 0.07%、右栏 0.00%、
> 时间轴 0.07%、顶栏 0.31%、左栏 2.34%），集中在左栏按钮/分隔线的 1px 边缘与圆角。
> 控件项已从 D10 时的 5959 px 降到 **3484 px**（D11 消除其中 2475 px 的 `accent-color` 色差），
> 余项是 range 滑块/轨道与 checkbox 的**抗锯齿边缘**；视频面板项从 26365 px 降到 **20 px**
>（D12 实现后，面板**主体**渐变像素逐行偏差 ≤6、落在 tol=12 内；残余 20 px 全在面板边界抗锯齿处）。
>
> **右栏剩余 1541 px 不在视频面板内**：分布在 `y52..91`（产物面板标题区，Edge 有蓝色
> 图标/标题像素而 wb-ui 无）与 `y239..331`（`#assets` 列表项的蓝色文字/图标），属文字/图标类
> 差异（与 §4.1 文字光栅化同族），面板区本身仅 20 px。

### 0.2 D10 的因果证据：左栏改善 7625 px == 控件贡献减幅 7625 px

| 量 | D10 前 | D10 后 | 差 |
|---|---|---|---|
| 左栏差异（基准） | 23315 px（13.80%） | **15690 px（9.29%）** | **-7625** |
| 左栏差异（遮蔽 `range`+`checkbox` 后 = 剔除控件贡献） | 9731 px（5.76%） | 9731 px（5.76%） | **0** |
| ⇒ 控件贡献 | 13584 px | **5959 px** | **-7625** |

两个独立测量（左栏总降幅、控件贡献降幅）**逐像素相等**，且「遮蔽控件后」在 D10 前后同为
5.76% —— 说明 D10 只改了控件的绘制、没碰任何其它像素（与 §5 的 `go test`、consistency
13/13 一致）。

### 0.3 D11 / D12 的因果证据（同样逐像素闭合）

**D11（`accent-color` 取值接入控件绘制）**——三个独立测量均为 **-2475 px**：

| 量 | D11 前 | D11 后 | 差 |
|---|---|---|---|
| 整页差异 | 24420 px（2.22%） | **21945 px（1.99%）** | **-2475** |
| 左栏差异 | 15690 px（9.29%） | **13215 px（7.82%）** | **-2475** |
| 控件贡献（遮蔽 `range`+`checkbox` 口径） | 5959 px | **3484 px** | **-2475** |

整页与左栏的降幅完全相同 ⇒ D11 的影响**只落在左栏控件上**，其余区域（舞台/右栏/顶栏/
时间轴）像素零变化（实测四区域差异像素数与 D11 前逐位相同）。

**D12（空 `<video controls>` 的 UA 占位面板）**——面板区差异几乎清零：

| 量 | D12 前 | D12 后 | 差 |
|---|---|---|---|
| 右栏差异 | 27886 px（14.53%） | **1541 px（0.80%）** | **-26345** |
| video 面板区（`x[1012,1280) y[91,239)`，268×148=39664 px）差异 | **26365 px**（66.47%） | **20 px**（0.05%） | **-26345** |
| 其余四区域 | — | 与 D12 前**逐位相同** | 0 |

**右栏降幅 26345 px 与面板区降幅 26345 px 逐位相等** → 因果闭合：右栏的改善**全部**来自面板区，
其余四区域（左栏/舞台/顶栏/时间轴）**逐像素零变化**，证明 D12 只改了视频面板的绘制。

**面板残余 20 px（0.05%）的位置已定位**：全部落在面板**边界**上 —— 左右边缘 `x1029..1033` /
`x1258..1262`、底边界行 `y234..238`、顶部 `y91` 一像素（最大通道偏差 33）——即我们实现的
sRGB 线性渐变在**圆角/边界抗锯齿**处与 Edge 的差异；面板**主体**（`#333333` 平台 + 渐变段）
逐行偏差 ≤6，完全落在 tol=12 内。

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

> 「视觉一致性」项（D3–D6、**D10–D12**）已**全部收口**；**D11**（`accent-color` 取值接入
> 控件**绘制**）同属视觉项，也**已修**（见下表 D11 行）。仍在「待修」的只有 **D7–D9 的
> computed 序列化**（`getComputedStyle` 契约完备性，**非视觉**，按需再做）。
> 载体页残留差异的归因（哪些属 UA 元素渲染、哪些属文字光栅化）见 §0.1 与 §4。

| # | 缺陷 | 证据（Edge vs wb-ui） | 影响面 | 状态 |
|---|---|---|---|---|
| D3 | 渐变「颜色 → `transparent`」的插值应在**预乘空间**（CSS/Chromium 语义） | 夹具 `linear-gradient(#ff0000,transparent)`：Edge 8800 红 / wb-ui 修复前 4800（中间色偏暗、衰减更快）→ **修复后 8800**，逐行 R 与预乘模型误差 ≤1.2 | 所有遮罩/淡出/skeleton 渐变 | **已修**：`engine/rendering/gradient.go` 的 `premulStopsForShader` / `interpolateColor`（2026-10） |
| D4 | 背景定位与绘制区域应为 **padding-box**（`background-origin` 初始值），现用 border-box | 带 1px 边框的盒子上 `linear-gradient(#ff0000 1px,…)`：Edge 顶部 1px 红线可见（200 px），wb-ui 被边框盖住（0） | 带边框元素的渐定位整体偏 1px | **已修**（`bbc0dee`） |
| D5 | **flex 内的滚动容器 auto 高度未含末子元素 `margin-bottom`** | `.ctrl-body{overflow:auto}`：Edge client 236×534 / scroll 236×534（**不溢出**）；wb-ui client 236×504 / scroll **236×513.5**（溢出）→ 预留 15px 滚动条 → 子元素 `.field` 宽 189 vs Edge 204 | 所有 flex 里的滚动容器：容器矮 ~10px、**误触发滚动条**、内容宽 -15px（左栏 19.44% 差异的主因） | **已修**（`72ea2b9`） |
| D6 | 表单控件 UA 样式缺 `line-height:normal` | `#btn-record` 高度 Edge 36 / wb-ui 38；lineHeight Edge `normal` / wb-ui `21px`（继承了正文 1.5） | 所有 button/input/textarea 的高度与文字位置 | **已修**（`dev/audit/WORKITEMS.md` §H2） |
| D7 | `getComputedStyle('line-height')` 序列化用错字号 | `h2`（font-size 12px）：Edge `18px`（=12×1.5）/ wb-ui `21px`（=14×1.5）。布局实测行高是 18px（`.field .row` 两侧都 18）→ **仅序列化错**，但会误导网页自检脚本 | 依赖 computed style 做适配的页面脚本 | 待修（契约，非视觉） |
| D8 | `appearance` / `accent-color` 的 **computed 值**未实现（**绘制**部分见 D10/D11） | `input[type=range]`：Edge `auto` / `rgb(20,115,230)`；wb-ui 空串（`getComputedStyle(...).accentColor` 实测返回 `""`） | 依赖 computed 做适配的页面脚本 | **绘制部分已修**（D10 白底不画/勾点恒白、**D11** `accent-color` 取值接入）；仅剩 computed **序列化**未实现（契约，非视觉） |
| D9 | `getComputedStyle` 未返回**使用值**、序列化格式不一致 | `width:100%` → Edge `204px` / wb-ui `"100%"`；颜色 Edge `rgb(215,55,63)` / wb-ui `#D7373F`；长度 Edge `0px` / wb-ui `0` | 同上（契约一致性，非视觉） | 待修（契约，非视觉） |
| **D10** | ★ `input[type=range]` 的 UA `background-color: #ffffff` 被**实画**（Chromium 只算、不画）；同族：checked 控件勾/点色应用**白色**而非元素文本色 | Edge（`#1e1e1e` 页、129×16 range，`x[2,131) y[2,18)`）：盒内只有 8 行滑轨像素，其余整行 `(30,30,30)`＝页面背景；wb-ui 画 `(255,255,255)` → 左栏纯白(>250) **8868** px（Edge **179**）。checked checkbox：Edge 勾 `(255,255,255)` / wb-ui 元素文本色（套件页=黑，载体页 `#E1E1E1`） | 深色主题页面的**所有** range（白块）+ 所有 checked 控件的勾/点色；左栏差异 **-4.51 pp / -7625 px** | **已修**（2026-10）：`engine/rendering/painter.go` 的 `nativeAppearanceRange` + `renderformcontrol.go` 的勾/点色 |
| D11 | range 已填充段 / 滑块 / checked 控件底色应取 **元素声明的 `accent-color`**（此前恒用 UA 常量） | 载体页 `accent-color:var(--accent)`=`#1473E6`：Edge 填充段 `(20,115,230)` / wb-ui 修复前 `FormControlColors.SliderFill` `#0075FF` `(0,117,255)`（B 差 25 > tol 12）；checkbox 底 wb-ui `(26,115,232)` vs Edge `(20,115,230)` | 设了 `accent-color` 的页面；左栏 **-1.47 pp / -2475 px**（整页 -2475 px，见 §0.3） | **已修**（2026-10）：`renderformcontrol.go` 新增 `accentColorFor`，接入 `paintCheckbox`/`paintRadio`/`paintRangeSlider`；未声明/`auto`/非法值时回退 UA 默认色（故 consistency `form_controls` 不受影响：其夹具未声明 `accent-color`） |
| **D12** | ★ 空 `<video controls>`（`controls` 属性 + **无有效媒体源**）的 **UA 占位面板**此前未实现（现**已修**）：Edge 画「#333333 平台 + 底锚定 105px 竖向渐变到 #000000」，wb-ui 完全不画 | 载体页 `#preview`（`x[1012,1280) y[91,239)`，268×148=39664 px）：Edge 面板 y91..133 为 `(51,51,51)` 平台、y134..238 由 50 降到 0；wb-ui 只为作者背景 `(20,20,20)` → 该区差异 **26365 px（66.47%）**，占右栏差异 94.5% | 所有「带 controls 但无源」的视频元素；右栏 **-14.94 pp / -26345 px**、整页 4.61% → **2.22%** | **已修**（2026-10）：`painter.go` 新增 `nativeAppearanceVideoPanel` / `videoHasUsableMediaSource` / `paintNativeVideoPanel`，在作者背景**之上**绘制；无 `controls` 不画、有源（src/子 `<source src>`/宿主帧状态）不画 |

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
> - ~~**载体页剩余差异的两大项不是缺陷（2026-10 实测，数字见 §0.1）**：空 `<video controls>` 的
>   UA 占位渲染（占右栏差异 **94.5%**）与 `input[type=range]` 轨道 UA 外观（占左栏差异 **58%**），
>   均属 **D8 同类**（UA 元素外观 / `appearance`、`accent-color` 未实现），本轮按范围不动。~~
>   ★ **该条为历史判断，其结论已被逐项推翻（勿再援引）**：range 轨道 UA 外观是**纯视觉缺陷**，
>   已按 **D10** 修复（见下一条归因更正）；空 `<video controls>` 的 UA 占位面板同样是**视觉缺陷**，
>   已按 **D12** 实现（2026-10，见 §4.2）。两项现均为**已修**。
> - ★ **归因更正（2026-10，第 5 次监督轮）**：上一条把 `input[type=range]` 的白色实底归为
>   「D8 同类 / 契约非视觉 / 本轮不动」——**该判断是错的**。实测（Edge 深色页 + 分控件遮蔽）：
>   range 的 UA `background-color` **算作白**（computed 与 Edge 一致，`defaultcss.go` 那条注释
>   本身没错），但 Chromium **不把它画出来**；wb-ui 却实画 → 左栏纯白(>250) 8868 px（Edge 179）、
>   左栏差异 **-4.51 pp**。这是**纯视觉缺陷**，已按 **D10** 修复。
>   **教训**：computed（契约）与 paint（视觉）是两条路径，「UA 元素外观」不等于「不可修」——
>   判定缺陷归属时必须两条路径各查一遍，不能由 computed 相符就推断绘制相符。
> - 空 `<video controls>` 的占位底（占右栏差异 **94.5%**）**已按 D12 实现**（2026-10，
>   见 §4.2）。历史判断「仍属未实现 / 已单独评估并记为『**已知未实现**』，不再写作『非缺陷』」
>   **已被 D12 推翻**：第 6 次监督轮裁图确认无源时 Chromium **不渲染任何控件 UI**、只画该渐变
>   面板，故实现面板即**完全保真**，既非「未实现」也非「非缺陷」。

---

## 4. 非缺陷差异 vs 曾判「不可修」项

> 本章此前把「可修的视觉缺陷」（range 白底、空 `<video controls>` 占位面板）与「不可修项」
> （文字光栅化）并列成「非缺陷」，属误导。现分两节：**4.1 非缺陷**（算法/环境固有，改不动）
> 与 **4.2 当时记为「已知未实现」的项**（判定可修、但当时范围超主线）—— **该两项现均已随
> D10 / D12 实现**，4.2 的标题保留原分节名以便追溯，**不存在「未实现」项**。

### 4.1 非缺陷差异

- **文字光栅化（非缺陷）**：Edge 用 **LCD 子像素**抗锯齿、wb-ui 用**灰度 AA**（`dev/audit/WORKITEMS.md`
  的 E1：`glyphcmp` 实测 deffont **灰阶**差异 14.62%，而**同一字形二值轮廓差异仅 0.95%** →
  墨迹形状一致，差异全在抗锯齿灰阶层）。载体页同口径量化（遮蔽文字后重测，**D10 后**口径）：
  整页 **4.61% → 3.36%**（50765 → 36946 px）、顶栏 3.47% → 0.31%、舞台 0.51% → 0.07%、
  时间轴 1.76% → 0.07%，
  即**顶栏 91% / 舞台 86% / 时间轴 96% 的差异来自文字**。
  ⚠️ §0.3 八页门槛里的 **deffont 14.62% / fontshort 12.54%** 同属此类（指标是灰阶差异；二值轮廓
  0.95% 达标），**不计缺陷**。`dev/suites/consistency/pixels.go` 的像素用例历来避开文字，正是这个原因。
- ~~**视频元素占位渲染**~~ / ~~**表单控件 UA 外观**~~：**两条均已移出本章并已完成** ——
  `input[type=range]` 的白色实底经实测是**视觉缺陷**（**D10**，已修）；空 `<video controls>` 的
  media-controls 占位面板同样是**视觉缺陷**（**D12**，已实现，见 §3 与 §4.2）。
  原文把两者一并称作「属 D8、本轮不动」是**误判**（见 §3 末更正）：computed 与 paint 是
  两条路径，「UA 元素外观未实现」不等于「不可修」，「只画底=半成品」也不成立（无源时
  Chromium **本来就不渲染任何控件 UI**，见 §4.2）。
- **动画相位**：对比时必须冻结（见 §1.2），否则同一元素的相位差会被算成差异。
- **宿主环境文案**：宿主 bridge 错误串（`fetch: request failed…` vs `TypeError: Failed to fetch`）
  天然不同。

### 4.2 空 `<video controls>` 的 UA 占位面板 —— **已实现（D12，2026-10）**

> 本节原为「已知未实现」，并列了「不实现的三条理由」。**第 6 次监督轮裁图证实：Edge 该面板
> 没有任何按钮/图标/进度条/文字**（无源时 Chromium **不渲染任何控件 UI**），因此实现这条渐变
> 即**完全保真** ——「只画底＝像但不可用的半成品」这一理由不成立。已按 **D12** 实现并验收。

**Edge 实测行为（二次探针 `probe_video2/3/4/5`，2026-10）**：

| 形态 | Edge 绘制 |
|---|---|
| `<video>`（无 `controls`） | **完全不绘制**（整块透出页面背景 `(30,30,30)`） |
| `<video controls>`（无 src） | **#333333 平台 + 底锚定 105px 竖向渐变到 #000000** |
| `<video controls src="missing.mp4">`（加载失败） | **同样画面板**（加载失败＝没有有效源） |
| `<video controls>` + 作者 `background` / 作者渐变 | 面板**不透明覆盖**作者背景（b/h 盒实测：绿 `#00ff00` 与作者渐变**完全看不见**） |
| `<video controls style="appearance:none">` | **与不设完全相同**（`<video>` 的 computed `appearance` 本来就是 `none`，probe_video4 的 `getComputedStyle().appearance` 实测；与 `<input type=range>` 不同，**没有**能关掉面板的作者开关） |
| 盒高 100 / 148 / 160 / 200 / 300 | 渐变**恒 105px、底锚定、与盒高解耦**：同一色值出现在距盒底**完全相同**的距离（`#2f2f2f` 恒在距底 90..92、`#1f1f1f` 恒在距底 63..64），高处填 `#333333` 平台 |
| 盒高 60（< 105px 跨度） | 渐变**被裁切而非压扁**：顶部像素 **28**，恰等于盒高 160 样本「距底 59px」处的值（盒高 100 同理：顶部 50 == 160 盒「距底 99」处的 50） |

**参数**：跨度 **105px**（不是 100px —— 逐行剖面显示渐变起点在距底 105 行处）、起点/平台色
`#333333`(51,51,51)、终点 `#000000`；曲线在 sRGB 空间近似线性（与线性模型逐行偏差 ≤6，
落在 tol=12 内，故实现直接用 Skia 两色线性渐变）。

> **那条 4px 灰条（距底 20..23px、宽 208px、`#4f4f4f`）不实现**：同一页面、同一命令重采三次
> （`studio.probeA/B.edge.png` 与基线 `studio.final.edge.png`），只有一次出现它 —— 它是
> Chromium 媒体控件的**竞态产物**（异步创建控件），本仓全部验收基线都没有；实现它反而与基线
> 引入差异。

**实现**（`engine/rendering/painter.go`）：

- `nativeAppearanceVideoPanel`：判定「`<video>` + `controls` 属性 + **无有效媒体源**」；
- `videoHasUsableMediaSource`：`src` 属性 / 子 `<source src>` / 媒体绑定层声明的宿主帧状态
  （`ElementVideoState`）三者任一成立即视为「有源」，不画占位面板；
- `paintNativeVideoPanel`：填 `#333333` 平台 + 底部 105px 竖向渐变（圆角与作者背景同口径，
  绘制裁到面板矩形内 —— 盒高不足时只裁切，不让渐变溢出到相邻元素）；
- 调用点在 `PaintBackground` 的 **作者背景之后**（面板不透明，必须盖住作者 `background`）。

单测 `engine/rendering/videopanel_test.go`（6 项）：无 controls 不画、有 src/子 source/宿主帧
状态不画、平台与底锚定渐变、盒高解耦（148 与 300 同距离同色）、盒高不足裁切而非压扁、
不透明覆盖作者背景。

**验收**：面板区 `x[1012,1280) y[91,239)`（268×148=39664 px）差异 **26365 → 20 px**；右栏
14.53% → **0.80%**；整页 4.61% → **2.22%**；`go test ./engine/... -count=1` 无 FAIL、
`dev/suites/consistency` **13/13**。

**范围**：只做**静态面板底**；控件 UI 与交互（播放/进度/时间/音量/全屏、命中测试、悬停态）
不做 —— 「带有效源的视频控件交互」仍是范围外的主线。

**状态**：**已实现**（D12）。

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

### 载体页收敛指标（D1–D12 全修后，2026-10 实测）

| 检查 | 结果 |
|---|---|
| 整页双端对比（`cmd/mediastudio/web/index.html`，1280×860、冻结、tol=12） | **9.09% → 6.38% → 5.30% → 4.61% → 2.22% → 1.99%**（100086 → 70213 → 58390 → 50765 → 24420 → **21945 px**）；平均通道差 6.14 → 5.55 → 4.03 → 2.51 → 1.94 → **1.88** |
| **左栏纯白（三通道 > 250）像素**（验收线 < 300） | **8868 → 209**（Edge **179**；D11/D12 不再变化）；整页纯白 209（Edge 197） |
| 分区域（同口径，`dev/tools/pixdiff_region.py`） | 左栏 **19.44% → 13.80% → 9.29% → 7.82%**（32855 → 23315 → 15690 → **13215 px**）；右栏 15.74% → 14.53% → **0.80%**（30210 → 27886 → **1541 px**）；舞台 **0.51%**（2805 px）；顶栏 3.47%（2308 px）；时间轴 1.76%（2076 px）——后三者 D11/D12 **全程零变化** |
| 差异归因（同名元素遮蔽对照，§1.4，**D11/D12 后**） | 文字光栅化 13819 px（**63.0%**）；控件残留 3484 px（**15.9%**）；视频面板残余 **20 px**（0.1%）；**三者全隐藏后残余 0.42%**（右栏 0.00%） |
| 口径可信度 | 新截 Edge 图 vs 历史 Edge 图 `--tol 0` **逐像素 0 差异**；按同一区域口径重测旧图得整页 9.09% / 左栏 19.44% / 右栏 15.74%，与历史值**逐位吻合** |
| 复现命令 | §0.1（结论表）/ §1.2（采集 + 分区域统计）/ §1.4（归因实验） |

> **结论**：D4/D5/D6 使左栏 19.44% → 13.80%（整页 9.09% → 5.30%）；**D10**（range 白色实底
> 不再绘制 + 勾/点恒白）左栏 → **9.29%**（整页 4.61%）；**D12**（空 `<video controls>` 的
> UA 面板）右栏 14.53% → **0.80%**、整页 → **2.22%**；**D11**（`accent-color` 接入控件绘制）
> 左栏 → **7.82%**、整页 → **1.99%**。剩余 1.99% 的构成：**文字光栅化 63.0%**（非缺陷，§4.1）、
> **控件残留 15.9%**（抗锯齿边缘）、**视频面板残余 0.1%**（面板区 20 px，全在边界抗锯齿处）、
> **其余 21.1%**（上界，含遮蔽手段副作用）——与文字/控件/视频无关的残余只有 **0.42%**。

### D11 / D12 收口回归（2026-10）

| 检查 | 结果 |
|---|---|
| 探针二次实测（`probe_video2/3/4/5`） | 底锚定、跨度恒 **105px**、平台 `#333333`；盒高 60 顶部 **28** == 盒高 160「距底 59」处的值（裁切非压扁）；`appearance:none` 与默认**完全一致**（video 的 computed appearance 本就是 none）；无 `controls` 不画；`src=missing.mp4` 照画；作者背景/渐变被**不透明覆盖** |
| 面板区差异（`x[1012,1280) y[91,239)`，268×148=39664 px） | **26365 px（66.47%）→ 20 px（0.05%）**（残余全在面板左右边缘 `x1029..1033`/`x1258..1262` 与底边界行 `y234..238`、顶部 `y91`，最大通道偏差 33） |
| 右栏差异 | 27886（14.53%）→ **1541（0.80%）**，-26345 px（与面板区降幅吻合，§0.3） |
| 左栏差异（D11） | 15690（9.29%）→ **13215（7.82%）**，-2475 px；**整页同降 2475 px** → 因果闭合（§0.3） |
| 整页差异 | 50765（4.61%）→ **21945（1.99%）**；mean_ch 2.51 → **1.88** |
| `go test ./engine/... -count=1` | 全部 ok，无 FAIL（新增 `videopanel_test.go` 6 项、`formcontrol_accent_test.go` 2 项） |
| `go run ./dev/suites/consistency` | **13/13 passed**（18.5s；`form_controls` 不回归） |
| computed 契约（未受影响） | D11 只改**绘制取值**（未声明 `accent-color` 时回退 UA 默认色，套件夹具未声明 → 行为逐像素不变）；D12 不改任何 computed（面板纯绘制） |
| 视觉核验 | 右栏面板裁剪并排放大对照（`dev/output/tmp/video_panel_cmp.png`）：两侧同为「上浅下深竖向渐变」，无分界线 / 按钮 / 图标 / 文字差异；逐行剖面平台 51 完全一致、渐变段偏差 ≤6 |

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

### D13 / D14 收口回归（2026-10，用户实测反馈两处差异后）

缘由：用户实测 wb-ui 里「`input[type=range]` 滑动条样式与浏览器不同」「`input[type=checkbox]`
比浏览器多出描边/边框」。两处都在**绘制层**，逐像素取证后修复。

#### D13 checkbox：描边外溢 + checked 多画灰边

| 项 | Edge 实测 | wb-ui（修复前） | 修复 |
|---|---|---|---|
| 15×15 元素盒的非背景像素 | **15×15**（x4..18 / y109..123） | **17×16**（x3..19 / y108..124） | 描边改内缩 |
| checked 的描边 | 无（纯 accent 底 + 白勾，边缘只有抗锯齿） | 恒画 1px 灰描边 | checked 不再画描边 |
| unchecked 边框 | `#767676`，1px 且在盒内 | `#787878` 且向外溢出半像素 | `#767676` + 盒内 |

根因：`paintCheckbox` 对两种状态都调用 `StrokeRect(cx,cy,size,size,1)`，而
`Canvas.StrokeRect`（`canvas.go`）以矩形边界为**中心线**画边（top/bottom/left/right 四条
`FillRect` 各向外扩半像素）→ 15×15 的盒画出 17×16 的外轮廓，即用户看到的「多出一圈描边」。
修复：unchecked 改 `StrokeRoundRect`（实现自带 `strokeWidth/2` 内缩，描边完整落在盒内）、
方块加 2px 圆角；checked 只画 accent 圆角填充 + 白勾。

#### D14 range：滑块尺寸/行程 + 轨道结构（四项）

| # | 项 | Edge 实测 | wb-ui（修复前） | 修复 |
|---|---|---|---|---|
| 1 | 滑块直径 | **16px**（y41..56） | 14px | `thumbR` 7→8 |
| 2 | 滑块行程 | 圆心 ∈ [盒左+R, 盒右−R]（值 min/50%/max → 10 / 66.5 / 123，行程 113 = 129−2×8） | `x + frac*w`（行程 = 全宽，端点值有一半在盒外） | `rangeThumbCenterX`（内缩 R） |
| 3 | 轨道高度 | **8px**：上下各 1px 边缘色 = (主体色+#757575)/2（未填充 `#b2b2b2` / 已填充 `#4574ae`） | 6px 纯主体色 | `trackH` 6→8、`SliderEdgeMix`、`mixColors` |
| 4 | 轨道水平 | 相对元素盒**左右各内缩 1px**（盒 x2..131 → 轨道像素 x3..129；载体页盒 x16..220 → x17..218） | 按整盒宽绘制（每边宽 1px） | `trackInset = 1` |

另：填充段右端 = 滑块右边缘（圆心+R），而不是 `frac × 轨道宽`（探针 frac=0.5 时两侧 fill
都蓝到 x74、x75 起才是灰 track；载体页 rec-seconds Edge 蓝到 x156.5 == 圆心 148.5 + 8）。

> **取证设计教训**：首版探针只测 value=min/50%/max 三点，而「内缩线性」与「全宽后夹紧」
> 两条候选公式**恰在中点重合**，三点无法区分。后经载体页 frac=1/6、5/6、2/3 三处（Edge
> 圆心 55 / 180.5 / 148.5）才判定为内缩线性（偏差 ≤0.5px）。测公式要选**非同构点**。

| 检查 | 结果 |
|---|---|
| 探针页（3 条 range + 2 个 checkbox，同口径双端） | **4.80%（1536 px）→ 1.075%（344 px）** |
| 载体页逐控件（`x[0,236)`，tol=12） | sec_range 502→**87**、checkbox 1145→**1071**；fps 542→807、clip-in 1096→1212、clip-out 1099→1230（见下「新发现」，属布局漂移被放大的表现） |
| 载体页整页 / 分栏 | 21945(1.99%) → **21968(2.00%)**，mean_ch 1.88 → **1.78**；左栏 13215(7.82%) → 13238(7.83%)；舞台 0.51%、右栏 0.80% **零变化** |
| `go test ./engine/... -count=1` | 25 个包全 ok，无 FAIL |
| `go test ./dev/suites/consistency/ -run TestPx` | **20/20 PASS**（`px_checkbox` 0.276%、`px_button` 0.393%） |
| `go run ./dev/suites/consistency` | **13/13 passed** |
| 新增回归用例 | `dev/suites/consistency/formctl_probe_test.go::TestPxFormControl`（预算 1.5%；同时把行跨度表 + 5× 放大并排图落盘到 `dev/output/` 供目视与排查） |
| Edge 侧稳定性 | 本轮新截 Edge 图与历史 Edge 图 `--tol 0` **逐像素 0 差异**（差异全部来自 wb-ui 侧） |

#### D15（**已修**，2026-10，布局层）：range 垂直错位 = half-leading 未按 Chromium 取整分配

**现象**：载体页 `input[type=range]`（fps / clip-in / clip-out）比 Edge 偏高 0.6~1.6px
（同期 D14 把轨道由 6px 修正为 Edge 真实的 8px 后，参与错位的行变多，可见度更高）。

**定位（载体页逐元素亚像素几何对照）**：`_temp/probe_pair.py` 把同一 collector 注入
`cmd/mediastudio/web/index.html`，Edge 侧 `--dump-dom` 读 `document.title`、wb-ui 侧
`dev/tools/jsread`（一次读回、不截断）双侧采集 109 个元素（`toFixed(2)` 亚像素）：

| 元素 | Edge | wb-ui（修复前） | 差 |
|---|---|---|---|
| `.field` 时长（div#5） | y90.00 **h46.00** | y90.00 **h45.50** | **−0.50** |
| `.field` 帧率（div#7） | y148.00 h46.00 | y147.50 h45.50 | −0.50 |
| `input#clip-in` | y381.50 | y380.50 | −1.00 |
| `input#clip-out` | y439.50 | y438.00 | −1.50 |
| `button#btn-export-clip` | y475.50 | y473.50 | −2.00 |

即**每个 `.field` 矮 0.5px** 并逐 field 累积。`.field` = `.row`(18px) + margin 4px + range
匿名行盒 ⇒ 0.5px 精确落在 **range 所在匿名行盒的高度**（Edge 24.00 / wb-ui 23.50）。

**根因（探针逐点扫描）**：`engine/layout/inlineformattingcontext.go` 的 strut half-leading
用**精确对半** `halfLeading = (lineBoxH − (textAscent + textDescent)) / 2`，而 Chromium 是
**上侧 `floor(leading/2)`、下侧 `ceil(leading/2)`**：

| `probe_halfleading`（Arial 14px，取整 asc13/desc3，leading = lh−16） | lh16 | lh17 | lh18 | lh19 | lh21 | lh23 |
|---|---|---|---|---|---|---|
| Edge：含 18px inline-block 的匿名行盒 | 21.00 | 22.00 | 22.00 | 23.00 | **24.00** | 25.00 |
| wb-ui（修复前） | 21.00 | 21.50 | 22.00 | 22.50 | **23.50** | 24.50 |
| 恒等式 `18 + desc + ceil(leading/2)` | 3+0 | 3+1 | 3+1 | 3+2 | 3+3 | 3+4 |

基线同一规律（`probe_baseline2`：8px baseline 对齐 inline-block 的底边即基线）——
Edge 基线 = `round(asc) + floor(leading/2)`：lh20→15、lh21→**15**、lh22→16、lh23→**16**、
YaHeiUI lh21→15、fs20 lh30→22；wb-ui 修复前在 leading 为奇数时低 0.5px。

载体页命中条件：`body{font:14px system-ui;line-height:1.5}` ⇒ 行盒 21px；YaHei UI 14px
取整 asc/desc = 14/4 ⇒ leading = **3（奇数）** ⇒ 精确半值 1.5 使行盒底部少 0.5px。

**修复**（纯布局层；绘制层常量一律未动）：

```go
leading := lineBoxH - (textAscent + textDescent)
leading = math.Round(leading*64) / 64 // 归到 Chromium LayoutUnit(1/64)，防浮点噪声
halfLeadingUp = math.Floor(leading / 2)   // 行盒上侧
halfLeadingDown = math.Ceil(leading / 2)  // 行盒下侧
strutAscent = halfLeadingUp + textAscent
strutDescent = halfLeadingDown + textDescent
```

`floor + ceil = leading` ⇒ **纯文本行盒高仍 = line-height（零回归面）**；只有含 inline-block /
替换元素（range 等控件）的行盒底部与基线按 Chromium 取整。

| 检查 | 结果 |
|---|---|
| `probe_halfleading` 行盒高（lh16..23 + YaHeiUI/Segoe + 40px ib） | 修复后**两侧逐一相同**：21/22/22/23/23/24/24/25、YaHeiUI-lh21 24、Segoe-lh21 23、ib40 45/46 |
| 载体页 form 控件 y（亚像素，容差 ±2） | **range 4/4 全 0**（rec-seconds / rec-fps / clip-in / clip-out）；`.field` #5/#7/#11/#13 均为 **h46.00 == Edge**；3 个按钮 dy=0 |
| 载体页整页像素（冻结 `#orb` 动画 + `--hide-scrollbars` + 同一 Edge 参照） | **2.25% → 1.66%**（mean_ch 2.04 → 1.39）；clip-in 带 y362-373 **1762→1653**、y379-403 **2291→1891** |
| 载体页残余 | `input#rec-audio` dy=−2、`label#3` rect h 15 vs 20 —— checkbox 的 `vertical-align:-2px` 分支，**另一独立问题**（不属 D15） |

> **口径教训（整页像素验收的坑）**：载体页 `#orb{animation:orb-move 2.4s linear infinite
> alternate}` 是**无限动画**，wb-ui（`webshot -frames N -step M` 走真实时间）与 Edge
> （`--virtual-time-budget`）的相位不可能一致 ⇒ 直接对比会混入 ~1% 的相位噪声（本轮首次
> 测得「2.69%」即此，同时我漏了 `--hide-scrollbars` 又叠加 26px 内容宽差 → 2.69% 全属口径
> 错误）。**做整页像素验收必须：冻结动画、统一 `--hide-scrollbars`、并用同一张 Edge 参照图
> 对比「修复前 / 修复后」两版代码**（临时 `git checkout` 旧版渲染一次再恢复）。

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
4. ~~**D11**~~（**已完成**，2026-10）：`accent-color` 取值接到控件绘制（range 已填充段 / 滑块 /
   checked 底色），未声明时回退 UA 默认色 —— 左栏 **9.29% → 7.82%**（-2475 px）、整页 4.61% →
   **2.22%**，见 §5「D11 / D12 收口回归」。**剩余**：`accent-color` / `appearance` 的 computed
   **序列化**（**D8** 的契约侧，非视觉）—— 与 D7/D9 同属「契约完备性」，配合网页自检脚本需求再做。
5. ~~**空 `<video controls>` 的占位底**~~（**已完成**，2026-10，**D12**，§4.2）：实测跨度 **105px**
   底锚定渐变 + `#333333` 平台；右栏 14.53% → **0.80%**、整页 4.61% → **2.22%**（面板区 26365 →
   **20 px**）。**剩余**：media-controls 的**控件 UI 与交互**（播放/进度/时间/音量/全屏、命中测试、
   悬停态）—— 仅在「有有效源的视频」上出现，仍是范围外主线。
6. **D7 / D9 / D8(computed 侧)**：契约完备性（序列化格式与使用值），配合网页自检脚本需求再做。
7. ~~**D15**~~（**已完成**，2026-10-11，**布局层**）：载体页 range 垂直漂移 = strut
   half-leading 未按 Chromium 的 `floor(上侧) / ceil(下侧)` 分配 —— 当
   `line-height − (取整 ascent + 取整 descent)` 为**奇数**时，精确对半会让行盒底部少
   0.5px、基线低 0.5px，逐 `.field` 累积成 range 错位 2px。修复后载体页 form 控件 y 偏差
   **全 0**（range 4/4）、整页 **2.25% → 1.66%**、左栏 **7.95% → 4.75%**（舞台/右栏零变化），
   见 §5「D15」（含探针逐点扫描与口径教训）。同时按当时的建议把 GEO 的
   `FormControlYTolerance` 由 6.0 **收紧到 2.0**（`dev/suites/consistency/compare.go`）——
   收紧后 `form_controls` 仍 **13/13** 通过（探针页控件 y 偏差本就 ≤2px，真正的漂移只在
   载体页累积）。**残余**：载体页 checkbox `input#rec-audio` 比 Edge 高 2px、`label#3` 的
   inline rect 15 vs 20 —— 属 `vertical-align:-2px` 分支，另立项。

> D3 的细分上限 `maxPremulSubdivs`（`engine/rendering/gradient.go`）为 24，是按
> 「每 ~8px 一个采样」的经验值；若后续遇到大量「两端半透明」渐变导致性能吃紧，
> 可下调该常量（精度与开销的旋钮），常见场景不受影响。
