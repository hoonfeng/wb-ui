# 滚动「同步派发」成本拆解（第10轮 必做2）

**结论（一句话）**：每次滚动约 **1.65s** 的成本 **100% 出现在宿主框架（CodeMirror 6）的
scroll 监听器回调内部**；引擎侧的派发骨架（事件路径构建 / 捕获 / 冒泡 / 默认行为）是 **零成本**，
同步全量布局、渲染树重建、DOM 变更均为 **0 次** —— **引擎侧没有可削减的派发骨架开销**。

---

## 1. 口径与工具

- **端到端口径**（监督者指定）：一次 `sc.scrollTop = v` 赋值 **+ 其触发的帧边界 flush 中那 1 次
  `DispatchEvent(scroll)`**（含 CM6 handler 全程）都计入窗口。
- **测量**：`dev/probes/gouide_real_e2e -interaction editorE2E`（Go 侧打点：赋值 → `wv.Render()`，
  并在同一次运行内先测「仅 `Render()`」基线）。
- **插桩**（`WB_PERF_DISPATCH=1` 开启，默认关闭、零开销）：
  - `engine/dom/eventtarget.go`：`DispatchEvent` 分段计时（buildEventPath / capture / target /
    bubble / default）+ listener 回调计时 + DOM 变更计数。
  - `engine/page/frameview.go`：`FrameView.Layout()` 次数与累计耗时、渲染树重建计数。
- **JS/桥基准**：`dev/probes/gouide_real_e2e -interaction jsbench`。

## 2. 分段占比表（端到端段，连续 6 次一致）

| 段 | 耗时 | 占比 |
|---|---|---|
| `buildEventPath`（17 层路径） | 0s | **0%** |
| capture 阶段 | 0s | **0%** |
| **target 阶段** | **1.65 ~ 1.84s** | **100%** |
| bubble 阶段 | 0s | **0%** |
| `defaultEventHandler` | 0s | **0%** |
| **合计** | **1.65 ~ 1.84s** | 100% |

target 阶段细分（`listeners=1`，该唯一监听器即 CM6 的 scroll handler）：

| 项 | 值 | 占比 |
|---|---|---|
| listener 回调合计（`listenerTime`） | 1.65 ~ 1.84s | **100%** |
| └ 同步全量布局 `FrameView.Layout()` | **0 次 / 0s** | 0% |
| └ 渲染树重建 | **0 次** | 0% |
| └ DOM 变更（createElement / createTextNode / appendChild / insertBefore / replaceChild / removeChild / setAttribute / textContent 写） | **0 次** | 0% |

## 3. 关键否定证据（逐项排除）

1. **不是派发骨架**：`build=0s cap=0s bub=0s def=0s`（17 层路径遍历零成本）。
2. **不是 layout thrashing**：`layouts=0 layoutTime=0s` —— 此前「每次滚动触发 ~44 次全量布局」
   的假设被数据**否决**。
3. **不是渲染树重建**：`treeRebuilds=0`。
4. **不是 DOM 变更风暴**：`domOps=0`。
5. **不是 Go↔JS 桥开销**：jsbench 实测（首页 DOM）：

   | 基准 | 耗时 | 单位成本 |
   |---|---|---|
   | 空循环 1e6 | 150.4ms | 150ns / 迭代 |
   | 空循环 1e5 | 12.2ms | 122ns / 迭代 |
   | offsetHeight 读 1e4 | 15.6ms | 1.56µs / 次 |
   | scrollTop 读 1e4 | 4.8ms | 0.48µs / 次 |
   | getBoundingClientRect 1e3 | 4.4ms | 4.4µs / 次 |
   | querySelector 1e3 | 145.2ms | 145µs / 次 |

   → DOM 属性读、几何读都是 **µs 级**，**桥不是瓶颈**。

## 4. 归因

按 goja 基本操作成本（150ns/次）折算，1.65s ≈ **1100 万次 JS 基本操作**。
即余额 **100% 是 CM6 的 scroll handler 在 goja 解释器上的执行量**
（含字符串 / 对象 / 正则密集路径，其相对 V8 的倍率远高于纯数值循环）。
**引擎侧无对应可削减点**。

## 5. 已做 / 可做的事

- **已做（本轮）**：滚动事件改为**帧边界合并派发** → 同一帧内 N 次滚动只跑 **1 次** handler
  （实测 N=12 与 N=1 相差 <30ms，合并生效）。真实滚轮是每帧 1 次赋值，故**单次成本不因此改变**。
- **已做（本轮）**：写 `scrollTop` 不再触发无条件 `forceLayout()`（与浏览器一致；
  实测对端到端无改善 −0.4%，已如实记录）。
- **引擎侧进一步**：**无** —— 骨架已零成本。真正的杠杆在 **JS 引擎速度**（超出「不换 V8」约束）
  或**宿主框架的滚动处理量**（前端侧，如 CM6 的 viewport 更新策略），不在本引擎的
  派发 / 布局 / DOM 路径上。

## 6. 指标1 判定

**未达成**。端到端 **N=1 = 1801 ~ 2122ms**（阈值 ≤1300ms，超出 0.39~0.63 倍）；
N=12 = 1847 ~ 2150ms。同口径 before（异步合并前，`a327149^`）见 `docs/PERF_BASELINE.json`
的 `scroll_ab` 段。

> 说明：本轮此前曾以「赋值口径 3948.818ms → 0.0ms」宣称达成，**该口径不成立** ——
> 纯 JS 循环窗口内不会触发 Go 侧 `Render()`/`flushScrollEvents()`，派发被移出窗口而非消失。
> 已在 baseline 中更正为「未达成」，并以本文件的分段占比表为准。
