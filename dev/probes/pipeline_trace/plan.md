# Pipeline Trace Tool — 逐组件全链路追踪

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
