# Browser Consistency Verification Suite

验证 wb-ui 渲染与真实浏览器（Edge/Chrome headless）的一致性。

## 用法

```bash
# 运行全部用例
go run ./dev/consistency

# 运行单个用例
go run ./dev/consistency -case layout_block

# 指定浏览器路径（默认 Edge）
go run ./dev/consistency -edge "C:\path\to\chrome.exe"
```

报告输出到 `dev/consistency/report/<case>.txt`（gitignored）。

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
3. **表单控件精确尺寸**：progress/range 的默认宽度与浏览器仍有差异
4. **字体度量**：行高取决于系统字体（非引擎 bug）

## 关键经验

- Edge headless 的 `--window-size` ≠ innerWidth（标题栏吃掉 ~26px 宽），
  必须从页面内 `window.innerWidth` 读实际视口
- `--dump-dom` 的 title 序列化遇到换行会提前终止，需定位 `</title>` 边界
- 页面内 JS 用 document.title 传数据最可靠（dump-dom 必然序列化 title）
- 内联样式由 resolver 原生处理，无需手动转 sheet
