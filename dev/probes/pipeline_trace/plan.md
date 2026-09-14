# Pipeline Trace Tool — 逐组件全链路追踪

> **状态：✅ 已实现**（`dev/probes/pipeline_trace/main.go`）。实际形态与下面的立项设想
> 有差异，以下三条为准：
>
> - **输入是诊断日志，不是「加载 Vue bundle」**：解析 `desktop_diag.log` 里的渲染树 dump
>   ——`go run ./dev/probes/pipeline_trace <desktop_diag.log>`，逐节点还原五个阶段的状态
>   （DOM 命中 / computed display 与背景色 / LayoutBox / RenderObject / 文本片段）。
> - **输出**：`pipeline_trace_report.json`（结构化，每个组件带 `anomalies` 列表）+ 同目录
>   的文本报告（零宽节点、越界 X/Y、颜色缺失等统计）。写在**日志文件所在目录**——惯例
>   是把日志放进 `dev/output/`。
> - **未完成的部分**：`fetch_html.py` 想从浏览器侧（`localhost:9090`）抓真实 DOM 做参照
>   （写 `full_browser_dom.json`），但脚本只实现了取 HTML 与列 `<script>`，DOM 导出是空的
>   （函数体只有 `pass`）。当前比对基准取自 Vue CSS 的 `expected` 字段，不依赖该脚本。

## 立项时的设想（历史，保留作设计依据）

构建一个 Go 诊断程序，在 wb-ui 内部对所有组件做全链路追踪：
1. 加载 Vue 应用 bundle（与 desktop.exe 相同）
2. 对每个 DOM 元素分配 trace ID
3. 在以下阶段记录状态：
   - DOM 阶段：tag/id/class/textContent
   - Style 阶段：computed display/width/height/color/bg/flex/grid
   - Layout 阶段：LayoutBox Rect (x,y,w,h)
   - Sync 阶段：RenderObject frame
   - Paint 阶段：绘制像素坐标
4. 输出结构化 JSON 报告
5. 运行异常检测：零宽、越界、颜色缺失、负坐标、重叠检测
