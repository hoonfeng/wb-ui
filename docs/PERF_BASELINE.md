# wb-ui 性能基线（PERF_BASELINE）

> 本文件 + 同目录 `PERF_BASELINE.json` 是**受控副本**：原始产物在 `out/`（被 `.gitignore` 忽略，不入库），
> 因此把基线与结论纳入版本控制以便干净 checkout 后可追溯、可复核。
> 复现：见文末「复现命令」。
>
> 生成轮次：第 8 次监督（对照实验轮）

## 1｜真实产物交互（panelSwitch / sessionListUpdate）

- **验收口径（唯一）**：**50ms 浏览器「长任务」阈值**。理由：panelSwitch 是**一次性离散交互**，超 50ms 即可感知卡顿；
  16.7ms 是 60fps 帧预算，适用于滚动/动画等**连续帧类**指标，不适用于本项。
- **结果：54.933ms → 29.68ms（−46%）→ 达标**
- 证据：`out/_before_panel.log`（before，用 `55a16d0` 版 webview.go 单编 exe 复跑）、`out/_panel_after.log`

## 2｜真实编辑器（gouide companion + CodeMirror 6）—— 本轮核心

目标编辑器为**真实 Vue 应用**（companion 前端即 Vue 3 应用，`__vue_app__` 已验）上的 `cm-scroller`。

| 项 | 数值 | 来源 |
|---|---|---|
| 滚动（n=30 均值） | **3558.567 ms/op** | `out/_editor_real.log` |
| 输入 | **104.1 ms/op**（61 行）/ 54.467 ms/op（37 行） | 同上 / `out/_editor_split.log` |
| 分步：assignOnly | 594 ms | `out/_editor_split.log` |
| 分步：assignReadLayout | 588 ms | 同上 |
| 分步：**readLayoutOnly** | **0 ms**（读布局免费） | 同上 |

### A/B 对照实验（根因判定，n=12）

| 组 | 首次 | 均值 | **稳态（后 11 次）** | 样本 |
|---|---|---|---|---|
| **G1** 现状（正常派发 scroll） | 582 ms | 3668.25 ms | **3948.818 ms** | 582, 3512, 3798, 4592, 4405, 4312, 3789, 4306, 3842, 3750, 3760, 3371 |
| **G2** 阻断 CM6（capture + `stopImmediatePropagation`） | 754 ms | 2638.33 ms | **2809.636 ms** | 754, 3681, 2474, 2776, 3047, 2628, 2603, 2543, 2717, 2780, 2875, 2782 |

**判定：已确认（对照实验）** —— 阻断 CM6 的 scroll 监听器后稳态只降 **28.8%**：

- **CM6 滚动处理 ≈ 29%**
- **其余 ≈ 71%（2809.636 ms）在 `scrollTop` 赋值路径本身**（引擎侧：setter / 事件派发骨架 / 失效标记）
- 结合 `readLayoutOnly = 0ms`（读布局免费）→ 引擎在**写入** `scrollTop` 时就产生主要开销

> **更正**：早前 verdict 称「成本几乎全部在 CM6 滚动处理」属**推断**，已由本对照实验**推翻并改写**。

## 3｜React / Vue 引擎级对比（夹具页合成负载）

| 指标 | React | Vue | 比值 |
|---|---|---|---|
| domCount | 39763 | 39764 | 1.000 |
| reflow perOpUs | 367400 | 393800 | 1.07 |
| styleRecalc perOpUs | 1022000 | 1035600 | 1.01 |
| editorInput perOpUs | 987600 | 978800 | 0.99 |
| update perBatchMs | 322 | 1673.33 | 5.20（语义不同，不作框架结论） |
| scroll perOpUs | 5200 | 5200 | **口径作废**（夹具页 `.code` 不可滚动，`acc=0` 空转） |

**结论**：引擎级指标（reflow / styleRecalc / editorInput）两框架差异 **≤7%** → **引擎成本与框架无关**，引擎侧优化同时惠及 React 与 Vue。
**口径说明**：本表为**夹具页合成负载**，与第 2 节的**真实 Vue 应用端到端**互不替代。

## 4｜Vue 覆盖情况

- **真实 Vue 应用端到端：已覆盖** —— companion 前端即 Vue 3 应用，第 2 节全部滚动/输入数据均在其上测得。
- **仍未覆盖**：Vue 应用特有路径（`vue-router` 导航、Pinia store 交互、组件级列表更新）→ 下一步。
- 不得用第 3 节夹具页数字替代第 2 节真实结论。

## 5｜当前性能结论（对用户目标）

- ✅ 真实产物交互（面板切换/会话更新）**达标**（29.68ms < 50ms）。
- ❌ **真实编辑器滚动 3.56s/op（稳态 ~2.8–3.9s）与输入 104ms/op 明确不满足**（远超 50ms 长任务阈值）；
  根因已由对照实验定位：**~71% 在引擎 `scrollTop` 写入路径**，~29% 在 CM6 滚动处理 —— 缓解尚未实施。

## 复现命令（CGO_ENABLED=1）

```bash
# 真实后端（gouide companion，避开 9090）
cd ../gou-ide && WEB_PORT=9191 ./companion.exe &

# 真实编辑器路径 + A/B 对照（G1/G2）+ 分步拆解
cd ../wb-ui && ./out/gouide_real_e2e.exe -url http://127.0.0.1:9191/ \
  -rounds 80 -hold 4 -interaction editorRealOpen -out out/gouide-editor-ab.json

# 夹具页双框架矩阵
./out/framework_matrix.exe -skip-js -reflow 5 -style 5 -scroll 5 -update 3 \
  -out out/framework-matrix-after4.json
```
