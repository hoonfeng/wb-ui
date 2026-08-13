# 增量布局架构改进方案（设计缺陷诊断 + 分阶段修改）

> 日期：2026-08-13
> 结论先行：**「布局增量风险过大」确实是设计不足导致，但设计的「蓝图」已经画好了（RenderTreeUpdater 已对标 WebKit 翻译 45%），只是「未完成 + 未接入」。「修改设计」的正确路径是补齐骨架，而非硬啃高风险增量重建，也非从零设计。**

## 一、设计缺陷清单（已用代码验证）

### 缺陷 1：DOM 变更通知 `onTreeChange` 无参数（核心）
- 位置：`dom/document.go:36` `onTreeChange func()`
- `notifyTreeChange()` 在 `b *nodeBase` 上调用（`document.go:52`），**节点就在手边却丢弃了**。
- 6 个调用点：`node.go:346/396/444/475` + `text.go:125`，text 变更（`SetNodeValue→SetData→notifyTreeChange`）也走这里。
- 宿主 `app/host.go:2818` 只能 `MarkRenderTreeDirty()`（全量重建），无法定位「哪个节点变了」。
- **修复**：`onTreeChange func(node Node)`，`notifyTreeChange()` 传 `b`。签名变更是编译期可发现的，宿主侧仍可「置标志全量重建」，零行为风险。

### 缺陷 2：缺「node → render object」反向映射
- 位置：`rendering/renderobject.go:119` `renderObjectBase.node` 是「render object → node」**单向**。
- `rendertreeupdater.go:199` `findRenderObject` 靠 `NextInPreOrder()` **线性全树遍历（O(n)）**，注释直言「In this simplified port the search is a linear walk from the view root」。
- WebKit 用 `Node::renderer()` 指针 + `NodeRenderingContext` 维护 O(1) 映射，wb-ui 退化为线性。
- **修复**：`RenderView` 加 `nodeMap map[dom.Node]RenderObject`，构建时填充、销毁时清除、insert/remove 时增量维护。

### 缺陷 3：text 双份存储
- `rendering/rendertext.go:52` `RenderText.text`（`NewRenderText` 里 `rt.text = t.Data()` 拷贝）。
- `layout/box.go:313` `InlineTextBox.text`（`BuildLayoutTree` 里 `&InlineTextBox{text: data}` 再拷贝）。
- text 变更需「三处同步」（DOM data / RenderText.text / InlineTextBox.text），且 `linkLayoutBoxes` 对 text 靠「位置配对」脆弱。
- **修复**：RenderText 不存副本，`text()` 直接返回 `node.(*dom.Text).Data()`（WebKit 做法）。

### 缺陷 4：RenderTreeUpdater 存在但未启用（45% 完成）
- 位置：`rendering/rendertreeupdater.go`，已实现 insert/remove/style-change 三种 ChangeKind。
- **仅测试引用**（`rendertreebuilder_test.go:203/247`），生产代码（app.Host / page / webkit）未接入。
- 缺 `ChangeText`（text 变更不支持）——而打字场景的核心正是 text 变更。
- `applyInsert` 里 `buildChildren(child, v)` 是「全量重建受影响子树」而非 surgical diff（注释自述）。

### 缺陷 5：渲染树/布局树靠「内容匹配」关联
- `rendertreebuilder.go:405` `linkLayoutBoxes`：element 靠 DOM 指针身份，text/匿名 wrapper 靠「位置配对（sibling index）」，O(children²)。
- 每次全量重建都要重新匹配（attachLayout 20-30% 的一部分）。

## 二、关键洞察

1. **RenderTreeUpdater 骨架已存在**——它是对标 `WebCore::RenderTreeUpdater.cpp` 的翻译，说明「增量渲染树更新」的架构蓝图早已画好，只是停在 45% 未完成、未接入宿主。
2. **缺陷有清晰的依赖链**：缺身份映射（缺陷 2）→ 无法 O(1) 定位 → 通知必须带节点（缺陷 1）→ text 才能单一源（缺陷 3）→ RenderTreeUpdater 才能启用（缺陷 4）→ 布局树才能增量更新（缺陷 5）。
3. **每一步都可「低风险 + 编译期可验证」地推进**，不需要一步到位。

## 三、分阶段修改方案（低风险优先）

| 阶段 | 改动 | 收益 | 风险 | 验证 |
|------|------|------|------|------|
| 1 | `onTreeChange` 带节点（缺陷 1） | 为增量铺路，零性能影响 | 低 | go build + 全量测试 |
| 2 | node→render 映射（缺陷 2） | findRenderObject O(n)→O(1) | 低-中 | updater 测试 + 全量测试 |
| 3 | text 单一源（缺陷 3） | text 变更只改一处 | 中 | 一致性像素测试 |
| 4 | 启用 RenderTreeUpdater + ChangeText + 布局树增量（缺陷 4/5） | 打字 text 变更走局部 re-layout，省全量重建 ~12ms | 高 | cm6 编辑 probe + 全量测试 |

**阶段 1-3 是「低风险铺路」，阶段 4 才是「最终目标」**。铺路完成后，阶段 4 的复杂度会大幅下降（因为有了身份映射 + text 单一源 + 精确通知）。

## 四、与之前结论的关系

之前「增量重建收益 -11%、风险高、不建议投入」的结论，是在「未发现 RenderTreeUpdater 骨架 + 未诊断设计缺陷」的前提下做的。现在确认：

- **收益不变**（打字 -11%），但**风险可被「分阶段铺路」大幅摊薄**。
- **正确的下一步不是「硬啃阶段 4」**，而是**先做阶段 1-3 的低风险设计修复**，把「增量更新」的基础打好。

## 五、建议执行顺序

1. **阶段 1（mutation 通知带节点）**：立即可做，几十行改动，纯铺路。
2. **阶段 2（node 映射）**：紧随其后，让 RenderTreeUpdater 从「死代码」变「可用」。
3. 阶段 1-2 完成后，重新评估打字场景的「text 变更」能否低成本接入增量路径。
4. 阶段 3-4 视业务需要（CM6 编辑器是否还有打字卡顿）再决定。
