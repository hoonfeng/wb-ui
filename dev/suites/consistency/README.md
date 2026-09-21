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

- 几何 x/y/w：±2px
- 高度 h：±12px（字体度量差异：wb-ui 用系统字体，浏览器用内置字体）
- inline 元素跳过几何比较（无盒几何，用文本段近似）

## 已知差异（引擎待办）

1. **vertical-align**：表单控件行内对齐（wb-ui 顶部对齐，浏览器基线对齐）
2. **heading margin 折叠**：h1-h6 在 body 首子的 margin 折叠
3. **字体度量**：行高取决于系统字体（非引擎 bug）

## 已修复（2026-09-22）

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

## 关键经验

- Edge headless 的 `--window-size` ≠ innerWidth（标题栏吃掉 ~26px 宽），
  必须从页面内 `window.innerWidth` 读实际视口
- `--dump-dom` 的 title 序列化遇到换行会提前终止，需定位 `</title>` 边界
- 页面内 JS 用 document.title 传数据最可靠（dump-dom 必然序列化 title）
- 内联样式由 resolver 原生处理，无需手动转 sheet
