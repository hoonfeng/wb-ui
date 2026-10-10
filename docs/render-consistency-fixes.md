# 渲染一致性排查与修复（2026-10-10）

起因：实际界面（wb-ui 引擎渲染）与浏览器（Edge）观感差异明显。本文记录**同视口双端像素
对照**的排查方法、两个已修缺陷（含根因与证据）、五个已定位待修缺陷，以及回归数字。

载体页面：`cmd/mediastudio/web/index.html`（MediaStudio 录屏剪辑台，1280×860，深色三栏 +
网格舞台 + 时间轴），它是当前的渲染回归载体。

---

## 0. 结论摘要

| 项 | 修复前 | 修复后 |
|---|---|---|
| 整页像素差异（vs Edge，同视口同状态，tol=12） | 100086 px（**9.09%**） | 70213 px（**6.38%**） |
| 平均通道差 | 6.14 | 5.55 |
| 舞台网格线像素数（色 `#2A2A2A`，Edge=29924） | **16**（≈完全没有） | **29941**（误差 0.06%） |
| 舞台区域差异率 | — | 0.47% |
| 剩余差异集中区 | — | 左栏 19.44% / 右栏 15.74%（见 §3 待修项） |

已修：**渐变长度色标被丢弃**、**渐变背景层不平铺**（两个都是「写了 CSS 却整片不画」级别）。

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

> 「视觉一致性」项（D3–D6）已全部收口；D7–D9 属**契约一致性**（非视觉），按需再做。

| # | 缺陷 | 证据（Edge vs wb-ui） | 影响面 | 状态 |
|---|---|---|---|---|
| D3 | 渐变「颜色 → `transparent`」的插值应在**预乘空间**（CSS/Chromium 语义） | 夹具 `linear-gradient(#ff0000,transparent)`：Edge 8800 红 / wb-ui 修复前 4800（中间色偏暗、衰减更快）→ **修复后 8800**，逐行 R 与预乘模型误差 ≤1.2 | 所有遮罩/淡出/skeleton 渐变 | **已修**：`engine/rendering/gradient.go` 的 `premulStopsForShader` / `interpolateColor`（2026-10） |
| D4 | 背景定位与绘制区域应为 **padding-box**（`background-origin` 初始值），现用 border-box | 带 1px 边框的盒子上 `linear-gradient(#ff0000 1px,…)`：Edge 顶部 1px 红线可见（200 px），wb-ui 被边框盖住（0） | 带边框元素的渐定位整体偏 1px | **已修**（`bbc0dee`） |
| D5 | **flex 内的滚动容器 auto 高度未含末子元素 `margin-bottom`** | `.ctrl-body{overflow:auto}`：Edge client 236×534 / scroll 236×534（**不溢出**）；wb-ui client 236×504 / scroll **236×513.5**（溢出）→ 预留 15px 滚动条 → 子元素 `.field` 宽 189 vs Edge 204 | 所有 flex 里的滚动容器：容器矮 ~10px、**误触发滚动条**、内容宽 -15px（左栏 19.44% 差异的主因） | **已修**（`72ea2b9`） |
| D6 | 表单控件 UA 样式缺 `line-height:normal` | `#btn-record` 高度 Edge 36 / wb-ui 38；lineHeight Edge `normal` / wb-ui `21px`（继承了正文 1.5） | 所有 button/input/textarea 的高度与文字位置 | **已修**（`dev/audit/WORKITEMS.md` §H2） |
| D7 | `getComputedStyle('line-height')` 序列化用错字号 | `h2`（font-size 12px）：Edge `18px`（=12×1.5）/ wb-ui `21px`（=14×1.5）。布局实测行高是 18px（`.field .row` 两侧都 18）→ **仅序列化错**，但会误导网页自检脚本 | 依赖 computed style 做适配的页面脚本 | 待修（契约，非视觉） |
| D8 | `appearance` / `accent-color` 未实现 | `input[type=range]`：Edge `auto` / `rgb(20,115,230)`；wb-ui `undefined` | 表单控件外观与配色 | 待修（契约，非视觉） |
| D9 | `getComputedStyle` 未返回**使用值**、序列化格式不一致 | `width:100%` → Edge `204px` / wb-ui `"100%"`；颜色 Edge `rgb(215,55,63)` / wb-ui `#D7373F`；长度 Edge `0px` / wb-ui `0` | 同上（契约一致性，非视觉） | 待修（契约，非视觉） |

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

---

## 4. 允许的已知差异（非缺陷）

- **文字光栅化**：DirectWrite 与 wb-ui 自有光栅化的抗锯齿不同，文字边缘必然有细碎差异
  （顶栏 3.40%、时间轴 1.76% 基本来自这里）。`dev/suites/consistency/pixels.go` 的像素用例
  历来避开文字，正是这个原因。
- **动画相位**：对比时必须冻结（见 §1.2），否则同一元素的相位差会被算成差异。
- **宿主环境文案**：宿主 bridge 错误串（`fetch: request failed…` vs `TypeError: Failed to fetch`）
  天然不同。

---

## 5. 回归记录（本次）

| 检查 | 结果 |
|---|---|
| `go build ./...` / `go vet`（rendering、layout） | 通过 |
| `go test ./engine/... -count=1` | 全通过（仅列出无测试文件的包） |
| `go test ./engine/rendering/ ./engine/layout/ ./engine/style/` | ok |
| `go run ./dev/suites/consistency` | **13/13 passed**（21.3s） |
| MediaStudio 双端像素对比 | 9.09% → **6.38%**；网格线像素 16 → **29941**（Edge 29924） |

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

## 6. 下一步建议（按收益排序）

1. ~~**D5**~~（**已完成**，`72ea2b9`）：flex 滚动容器高度含末子 margin；已跑全量
   consistency + cssprobe 回归。
2. ~~**D3**~~（**已完成**，2026-10）：最终落点既不是「扩 goskia（暴露
   `SkGradientShader::Interpolation`）」也不是「逐像素」——而是**等价色标重算**：
   `premulStopsForShader` 把预乘语义的色标序列改写成 Skia 非预乘插值下等价的序列。
   渐隐到 `transparent`（最常见：遮罩/淡出/skeleton）只需改写透明端的 RGB，
   色标数仍是 2 → 零额外开销；只有「两端都半透明且 alpha 不同」才细分（+60%）。
   故 **goskia 无需任何变更**。
3. ~~**D4 / D6**~~（**已完成**）**/ D7**：单点小改，收益中等。
4. **D8 / D9**：契约完备性，配合网页自检脚本需求再做。

> D3 的细分上限 `maxPremulSubdivs`（`engine/rendering/gradient.go`）为 24，是按
> 「每 ~8px 一个采样」的经验值；若后续遇到大量「两端半透明」渐变导致性能吃紧，
> 可下调该常量（精度与开销的旋钮），常见场景不受影响。
