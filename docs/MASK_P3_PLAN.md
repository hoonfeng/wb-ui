# mask-image P3 实施计划：前景遮罩 + SVG mask + 属性补全

> 状态：✅ 已实现（2026-08-13）。P3.1 / P3.2 / P3.3 / P3.4 全部落地。
> 目标：让 CSS `mask-image` 达到 CSS Masking Level 1 的可用语义，并补齐 SVG `<mask>`。

## 一、现状（以代码为准）

### 1.1 已实现：仅遮 background + border
- 位置：`rendering/renderpipeline.go` 的 `paintObjectBackground`（约 1553 行）。
- 流程：`SaveLayerForMask(rect)` → `PaintBackground` + `PaintBorder` → `ApplyImageMask(img, rect)` + `Restore`。
- 结论：mask 只包裹**背景和边框**，立即 restore；前景文字（Foreground phase）与子元素（递归遍历）完全不受遮罩 → 视觉上「文字/子元素穿透遮罩」。

### 1.2 skia 原语已就绪
- `platform/graphics/canvas.go`：`SaveLayerForMask(rect)`（离屏层）+ `ApplyImageMask(img, rect)`（`BlendModeDstIn`，取图片 alpha，忽略 RGB）+ `Restore`。
- 图片加载：`rendering/image.go` 的 `DecodedImage.SkiaImage()` + `loadBackgroundImage`。
- 这些原语可直接复用，无需新 skia 绑定。

### 1.3 缺失项
| 缺失 | 现状 | 规范语义 |
|------|------|---------|
| 前景遮罩 | 文字/子元素穿透 | mask 作用于整个元素（含内容、后代） |
| `mask-size` | 拉伸填充整个 rect | auto/contain/cover/length/% |
| `mask-repeat` | 无（固定填充） | repeat/no-repeat/…（默认 repeat） |
| `mask-position` | 无 | 默认 0 0 |
| `mask-mode` | 固定 alpha | alpha \| luminance \| match-source |
| SVG `<mask>` | 完全未实现（搜 `<mask`/maskUnits/maskContentUnits 零命中） | SVG mask 元素（luminance/alpha、maskUnits/maskContentUnits） |

## 二、分解任务与方案

### P3.2 mask-size / repeat / position 解析（难度中，收益直接，先做）
- **根因**：`ApplyImageMask` 用「src 整图 → dst 整 rect」缩放填充，无 size/repeat/position。
- **方案**：复用 background 的 `background-size`/`background-position`/`background-repeat` 解析（若 `rendering` 已有 background 铺排逻辑则直接挂接；否则新增 mask 专用解析，语义同 background）。
- **涉及文件**：`rendering/renderpipeline.go`（mask apply 处）、可能新增 `rendering/mask.go`。
- **验证**：`mask-size: cover`/`contain`/`50%`、`mask-repeat: no-repeat` 各写一个 `rendering/mask_test.go` 用例，对比半透明像素区域几何。

### P3.3 mask-mode（alpha / luminance，难度低）
- **根因**：`ApplyImageMask` 固定 `BlendModeDstIn`（alpha 语义）。
- **方案**：`mask-mode: luminance` 时先把图片 RGB 亮度（0.2126R+0.7152G+0.0722B）预乘成 alpha 得到等价图，再走 `DstIn`；或新增 skia `BlendModeLuminosity` 变体（若 goskia 支持）。默认 `match-source`（image → alpha）。
- **涉及文件**：`rendering/renderpipeline.go`、`rendering/image.go`。
- **验证**：luminance 模式下灰阶图片（黑=全遮、白=保留）的像素断言。

### P3.1 mask 提升到「整棵子树」层级（核心正确性，难度高，风险最大）
- **根因**：mask 在 `paintObjectBackground` 内只包 background/border；绘制分三个 phase（Background / Foreground / Outline）各走一遍 `walkSubtreeExcluded`，mask 需要**跨 phase 包裹整棵子树**，现有单 phase 的 Save/Restore 做不到。
- **候选方案**：
  - **方案 A（推荐，改动最小）**：在 `walkSubtreeExcluded` 中检测「mask 元素」，进入其子树前 `SaveLayerForMask`，遍历完子树后 `ApplyImageMask`+`Restore`。需要引入「跨 phase 的 mask 层栈」——因为 Background phase 的 `Save` 要到 Outline phase 结束才 `Restore`。可维护一个 `*PaintInfo` 内的 mask 层列表，按 pre-order 进出配对，跨三个 phase 存活。
  - **方案 B（合成器，架构重）**：把 mask 元素整体离屏渲染（三个 phase 合并）后 mask 合成。最接近浏览器真实行为，但需引入合成层机制，成本高。
  - **方案 C（单元素、不遮后代，不完整）**：只遮元素自身 background+border+text，不遮子元素。偏离规范，仅作兜底。
- **风险**：与 overflow clip / `filter` / `mix-blend-mode` / `opacity<1` 的 Save/Restore 嵌套交互复杂；mask 元素强制离屏有性能开销（`walkSubtreeExcluded` 的 dirty-rect 早退需对 mask 元素保守关闭）。
- **涉及文件**：`rendering/renderpipeline.go`（`walkSubtreeExcluded` + `paintObjectBackground`）、`rendering/painter.go`。
- **验证**：构造「父元素 mask + 子元素/文字」，断言子元素和文字区域被遮罩；再验证 mask 元素自身无 overflow/filter 时路径不变。

### P3.4 SVG `<mask>` 元素（难度高，独立立项，最后做）
- **根因**：SVG 子系统无 `<mask>` 元素解析与渲染。
- **方案**：新建 SVG mask 元素解析（`maskUnits`/`maskContentUnits`/`x`/`y`/`width`/`height`），渲染时把 mask 内容（子图形）栅格成 alpha/luminance 图，作为 `ApplyImageMask` 的输入复用现有遮罩管线。
- **依赖**：SVG 图形元素（rect/circle/path 等）的独立栅格化能力是否齐备。
- **风险**：这是「SVG 特性补全」而非「CSS mask 补全」，工作量和测试面都大，建议单独任务、单独验收。

## 三、建议执行顺序与依赖

```
P3.2 (size/repeat/position) ──┐
                              ├──► P3.1 (子树遮罩) ──► P3.4 (SVG mask)
P3.3 (mask-mode) ─────────────┘
```

- **先 P3.2 + P3.3**：成本低、语义独立、可立即用 `mask_test.go` 验证，不碰跨 phase 架构。
- **再 P3.1**：核心正确性，但需先想清楚「跨 phase mask 层栈」与现有 Save/Restore 的交互，建议单独立项 + 独立评审。
- **最后 P3.4**：最重、最独立，可并行调研 SVG 栅格化能力，不阻塞前三项。

## 四、风险与回退

- P3.1 若「跨 phase 层栈」与现有 clip/filter/blend 交互失控，回退到方案 C（只遮自身 background+border+text），仍优于当前「仅遮 background/border」。
- 性能：mask 元素数量少（装饰性遮罩），离屏开销可接受；`walkSubtreeExcluded` 的 dirty-rect 早退需对 mask 元素保守（类似 filter/opacity 的处理）。

## 五、验证清单（全部完成后）

1. `mask-image` 遮罩文字与子元素（P3.1）。
2. `mask-size`/`mask-repeat`/`mask-position` 各值（P3.2）。
3. `mask-mode: luminance`（P3.3）。
4. SVG `<mask>` 元素 + `maskUnits`（P3.4）。
5. 全量 `go test ./...` 无回归，`rendering/mask_test.go` 扩充为参数化用例。

## 六、实现记录（2026-08-13，全部完成）

### P3.1 子树级遮罩（核心）
- `rendering/renderlayer.go`：`RequiresLayer` 增加 mask-image 判断——mask 元素必须有
  自己的 RenderLayer，才能像 opacity 一样在 layer 层包裹整棵子树。
- `rendering/renderpipeline.go`：新增 `paintLayerWithEffects`（mask 离屏层在最外、
  opacity 离屏层在内，跨 Background/Foreground/Outline 三个 phase 包裹整棵子树），
  fixed 分支和普通分支统一走它；移除 `paintObjectBackground` 里只遮 background/border
  的旧 mask 代码。
- 效果：mask 现在遮罩文字 + 子元素，不再「文字/子元素穿透遮罩」。

### P3.2 mask-size/repeat/position
- `rendering/mask.go`：`applyMaskLayer` 复用 `computeBackgroundDest` 解析 size/position；
  repeat 语义用「clip 到有效区域 + 图片 shader(TileModeRepeat) + `ClearRect` 清除区域外」，
  no-repeat 时 tile 之外被遮掉（与 background-repeat 的空区域语义不同）。

### P3.3 mask-mode luminance
- `platform/graphics/canvas.go`：`ApplyImageMaskMode` / `ApplyImageMaskTiled` 增加
  luminance 参数，用 goskia `NewColorMatrixFilter` 把 RGB 亮度（0.2126R+0.7152G+0.0722B）
  编码到 alpha；`mask-mode: luminance | alpha | match-source` 全支持。

### P3.4 SVG `<mask>`
- `rendering/svg.go`：`svgMask` 结构 + `<defs>` 解析 `<mask>`（maskUnits/maskContentUnits/
  mask-type/x/y/width/height，默认 -10%/-10%/120%/120%）；`renderSVGMask` 把 mask 内容
  栅格化成 alpha/luminance 图（maskUnits 解析目标区域，maskContentUnits 处理坐标系）。
- `rendering/mask.go`：`mask-image: url(file.svg#maskId)` / `url(data:...#id)` 引用 SVG
  mask；`mask-type` 与 CSS `mask-mode` 交互（match-source 时 SVG 默认 luminance）。

### 测试
- `rendering/mask_test.go`：TestMaskImageAlpha / TestMaskImageMasksDescendants /
  TestMaskSizeNoRepeat / TestMaskModeLuminance / TestMaskPropertyNoCrash。
- `rendering/svg_mask_test.go`：TestSVGMaskParseAndRender / TestMaskImageSVGMask /
  TestMaskImageSVGMaskAlpha。
- 全量 `go test ./...` 通过，无回归。

### 备注
- goskia `Image.MakeShader` 的 sampling 参数必须传非 nil（传 nil 会崩溃），已传
  `&skia.SamplingLinear`。
- `TileModeDecal`（Skia raster 后端）在 no-repeat 场景行为异常，改用 clip+ClearRect
  实现 no-repeat 的「tile 外透明」语义。

### P3.5 同文档 url(#id) 引用（2026-08-13 追加，已实现）

- `rendering/svg.go`：把 `buildSVGDocument` 的 `case "mask"` 解析逻辑提取为独立
  `parseMaskElement(defEl *dom.Element) *svgMask`，供同文档引用复用。
- `rendering/mask.go`：`applyMaskLayer` 增加 `ownerEl *dom.Element` 参数——当
  `mask-image: url(#id)` 的 URL 无文件部分（`fileURL == ""`）时，通过
  `ownerEl.OwnerDocument().GetElementById(id)` 找到同文档内联 `<mask>` 元素，用
  `parseMaskElement` + `renderSVGMask` 栅格化遮罩。
- `rendering/renderpipeline.go`：`paintLayerWithEffects` 从 `layer.Owner().Node()`
  取出 owner 元素传给 `applyMaskLayer`。
- 测试：`svg_mask_test.go` 的 `TestMaskImageSameDocumentReference`（内联
  `<svg><mask id="m">` + `mask-image: url(#m)` 右半遮罩）。


