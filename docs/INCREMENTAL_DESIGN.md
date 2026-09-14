# 增量布局架构改进方案（设计缺陷诊断 + 分阶段修改）

> 日期：2026-08-13（更新）
> 结论先行：**「布局增量风险过大」是设计不足导致，但设计蓝图已存在（RenderTreeUpdater 45% + 增量布局 B 剪枝）。
> 通过「分阶段铺路」，已完成低风险的 A/B 阶段与中风险的 C1（粗粒度 text 增量更新），打字不再全量重建。**

## 一、设计缺陷清单（已用代码验证）

### 缺陷 1：DOM 变更通知 `onTreeChange` 无参数（已修复）
- `engine/dom/document.go:36` `onTreeChange func()` → `func(Node)`，`notifyTreeChange()` 传 `b`。
- 宿主能精确定位「哪个节点变了」。

### 缺陷 2：node→render 映射填充时机晚（已修复）
- `nodeRenderMap` 早已存在（`renderview.go:56`），`FindRenderObjectForNode` 已用 map O(1)。
- 真正缺陷：① 映射只在布局后 `syncGeometry` 填充（重建后→布局前窗口为空）；② `RenderTreeUpdater.findRenderObject` 没复用映射、自己线性遍历。
- 修复：`rebuildNodeMap()` 在 `Build()` 末尾（层树构建前）遍历渲染树填充；`findRenderObject` 改 O(1)。

### 缺陷 3：text 双份存储（修正：不是「单一源」问题）
- `RenderText.text`（rendertext.go）+ `layout.InlineTextBox.text`（box.go）各拷贝一份。
- **修正**：此前误判为「应做 text 单一源（不存副本）」。实际上 **WebKit 的 RenderText 也存副本（`m_text`）**，text 变更走 `RenderText::setText` 同步副本——「单一源」偏离 WebKit 设计且收益有限。
- 正确方案：text 变更通过 `RenderText.SetText`（已有）+ 同步布局树 `InlineTextBox.text`（阶段 C1 已做）。

### 缺陷 4：RenderTreeUpdater 存在但未启用（已补 ChangeText）
- 已实现 insert/remove/style-change，缺 `ChangeText`。
- 阶段 A 补齐 `MarkTextChange`/`applyTextChange`（`5fa3cfd`）。

### 缺陷 5：渲染树/布局树 text 靠「内容匹配」关联（已修复）
- `linkLayoutBoxes` 对 text 用 `tb.Text() == rt.OriginalText()` 值匹配，同文本多 span 会错配，且 text-transform 时值匹配失败。
- 阶段 B 改为 `tb.Node() == rt.Node()` 指针匹配（`0d541af`），并给 `layout.InlineTextBox` 加 `node` 引用。

## 二、关键洞察

1. **RenderTreeUpdater 骨架 + 增量布局 B 剪枝都已存在**——增量更新的「架构蓝图」早已画好，缺的是「接线」。
2. **`RenderObject.SetLayoutBox`（renderobject.go:185）已建立 render→layout 持久引用**（syncGeometry 时填充），C1 无需新增映射。
3. **`Frame.RebuildStyleForElement`（frame.go:321）是成熟的增量模式**——C1 完全模仿它：改样式/文本 → 同步布局树 → `f.view.SetNeedsLayout(true)` → 下帧剪枝重排。

## 三、分阶段方案与进度

| 阶段 | 改动 | 风险 | 状态 |
|------|------|------|------|
| 1 | onTreeChange 带节点 | 低 | ✅ c73728f |
| 2 | nodeRenderMap 提前填充 + findRenderObject O(1) | 低-中 | ✅ bfe4645 |
| A | RenderTreeUpdater 补 ChangeText | 低 | ✅ 5fa3cfd |
| B | InlineTextBox node 引用 + node 匹配 | 低-中 | ✅ 0d541af |
| C1 | text 变更粗粒度增量（打字热路径） | 中 | ✅ ae5d64f |
| C2 | IFC 行级增量重排 | 高 | ⏳ 后置 |

## 四、阶段 C1 的实现（text 变更粗粒度增量）

打字链路：JS 改 Text data → `notifyTreeChange(textNode)` → `onTreeChange` 识别 `*dom.Text` →
`Frame.ApplyTextChange` → `RenderView.ApplyTextChange`：

1. `rt.SetText(newData)`（清 segments + dirty）
2. `containingBlockForText` 向上定位所在 `RenderBlockFlow`，取其 `LayoutBox()`
3. `syncInlineTextBoxText` 递归同步布局树里 node 匹配的 `InlineTextBox.text`
4. `blockLB.MarkDirty()`（沿祖先链传播）
5. `f.view.SetNeedsLayout(true)`

下帧 `EnsureLayout` → `FrameView.Layout` → 增量布局 B 剪枝：仅 dirty block 重排（IFC 全量重排该 block），clean 兄弟子树跳过。**省掉了全量 `RebuildRenderTree`（渲染树+布局树+层树重建）。**

失败回退：无 RenderText / 未布局（`LayoutBox()` nil）时返回 false，`onTreeChange` 回退 `MarkRenderTreeDirty` 全量重建。

## 五、验证

- 编译 + 单测：rendering/page/app/webkit 全绿。
- `TestFrameApplyTextChange`（page）：LoadHTML → Layout → SetData → ApplyTextChange → NeedsLayout → Layout → `rt.Text()=="world"` + segments 非空（**不触发全量重建**，精确验证 C1 增量链路）。
- `TestUpdaterTextChange` / `TestApplyTextChange`（rendering）。
- 渲染引擎无回归：当时以 `cmd/render_test`（69 用例）核对，该工具未保留在仓库；现状等价
  验证为 `go test ./engine/page/... ./engine/rendering/...` 与 `dev/suites/cssprobe`。
- `ime_editor_probe`：IME 组合 + 普通字符打字，CM6 state 同步正确。

## 六、剩余工作（C2，后置）

IFC 行级增量重排（只重排 text 变更所在行，而非整 block）。收益进一步缩小，风险高（WebKit LineLayout 增量是独立大工程）。当前 C1 已覆盖打字主热路径，C2 视「整 block 重排」是否仍是瓶颈再决定。
