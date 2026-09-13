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

## 结论

1. 夹具期望可用 obscura 原样复现 → 期望值可信；但 `logical-borders` 的拐角期望
   与标准 CSS 相左（参照不斜切）。以参照为准会牺牲标准化能力，**暂不改绘制**，
   该夹具固定为 4/6。
2. `tables` 的 3.96% 差异说明：**cssprobe 的色块检查通过 ≠ 视觉一致**（检查只覆盖
   若干色块位置/尺寸）。后续可把 `dev/calib` 的差异占比纳入回归基线，作为整体
   一致性的补充指标。
3. 文本位置差 1-2px 属字体度量范畴（hinting/行高取整），非结构性布局错误。

## 夹具限制（探针能力边界，非引擎缺口）

`dev/cssprobe` 只做 HTML 解析 + CSS 计算 + 布局 + 绘制，**不执行 `<script>`**
（也没有媒体加载 / DOM 事件运行时）。下列夹具的期望因此在本探针内不可达：
失败原因是探针边界，**不记为渲染能力缺口**，对应能力改用 Go 单测或宿主集成
测试覆盖。判断「是探针边界还是真缺口」的方法：期望色是否由页面脚本/媒体 API
产生（`closest` 报出的实际像素恰好是夹具里另一条静态规则的颜色 ⇒ 脚本没跑）。

| 夹具 | 探针内不可达的原因 | 引擎侧覆盖方式 |
|------|--------------------|----------------|
| `animation-fill-forwards` | `.dismissed` 由内联 `<script>` 的 `classList.add` 添加，脚本不执行 ⇒ 动画从未绑定，`#overlay` 保持静态可见（红），而期望是终态隐藏后露出的 `#content` 绿 | **已实现**：有限动画结束时 `fill:forwards` 保持终帧（`opacity:0` + `visibility:hidden`），`rendering/animation_test.go:TestAnimateVisibilityFillForwards`；visibility 的离散插值（CSS-ANIM：区间任一端点 `visible` ⇒ 区间内 `visible`）本轮补齐，另见 `TestAnimateVisibilityBothHidden` |
| `eventtarget-lifecycle` | 需要 `addEventListener` / `dispatchEvent` 时序 | DOM 事件层单测（非渲染管线） |
| `modern-hydration-contracts`、`modern-streams` | 需要 JS 运行时 API | 同上 |
| `media-text-track` | 需要 `<track>` 媒体加载 | 未实现（媒体能力） |
| `flex-flow` | 11 项里 10 项通过；仅剩「CSS supports accepts only the shorthand grammar」需要 `CSS.supports()`（JS API） | `css/values_test.go:TestParseFlexFlow` 覆盖语法，布局行为已由其余 10 项像素验证 |
| `viewport-consistency` | 3 项里仅第 3 项「page JavaScript sees the screenshot viewport」需要 `window.innerHeight`（JS） | ⚠️ 第 2 项「height media query uses the screenshot viewport」**不是** JS 依赖，属真缺口（`@media (height: …)` 未按视口高匹配），见下 |

仍待处理（真缺口，非探针边界）：

- `viewport-consistency` 的 height 媒体查询项：`@media (height: 1000px)` 未命中，
  元素按不匹配分支落成 `#087f5b`。
