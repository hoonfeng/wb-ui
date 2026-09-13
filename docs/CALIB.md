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
