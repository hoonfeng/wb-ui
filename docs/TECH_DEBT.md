# wb-ui 遗留问题处理指南

> 本文档汇总 wb-ui 渲染引擎的一批已知遗留缺口，按「优先级 / 改动规模 / 风险」排序。
> **状态（2026-08-13）**：全部 5 项已处理完毕——P0 calc 定位（`0a042d3`）、P1 min/max/clamp
> （`a9c69d3`）、P2 Shadow DOM（`1dff19a`→`2bcf5cc`）、P3 mask-image 均已实现；P1 布局增量
> 经调研确认阶段 A 收益 <1% 暂不投入（B/C 高风险待业务驱动）。各条目正文保留历史分析，
> 顶部已追加「已实现 / 最终决策」块标记真实状态。调研基准 commit：`5cfff1c`。

## 总览

| # | 遗留项 | 类型 | 优先级 | 状态（2026-08-13） |
|---|--------|------|--------|--------------------|
| 1 | positioned `top/left` 不解析 `calc()` | 真实 bug | P0 | ✅ 已修复（`0a042d3`） |
| 2 | `min()/max()/clamp()` 未实现 | 功能缺失 | P1 | ✅ 已实现（`a9c69d3`） |
| 3 | 布局三次全树遍历（无增量） | 性能 | P1 | 🔍 已调研：阶段 A 收益 <1%，暂不投入 |
| 4 | Shadow DOM selector（`:host`/`::slotted`/`::part`） | 功能缺失 | P2 | ✅ 已完整实现（`1dff19a`→`2bcf5cc`） |
| 5 | `mask-image` 仅存属性不绘制 | 功能缺失 | P3 | ✅ 已实现（背景/边框 alpha 遮罩） |
| — | WebSocket | ~~非问题~~ | — | 有意 stub（宿主注入事件），非遗留 |

---

## 1. positioned `top/left/right/bottom` 不解析 `calc()`（P0 · 真实 bug）

> **已实现（2026-08-13，提交 `0a042d3`）**：`layout/layoututil.go` 的 `parseCSSLength`
> 开头新增 `mathFuncInfo` 检测——识别 `calc`/`min`/`max`/`clamp` 前缀并返回平衡的完整
> 函数表达式；含相对单位（%、em、rem、vw、vh 等）时延迟求值（返回 `Unit:"calc"`，由
> `resolveLength` 的 `calc` case 带真实 context 求值），纯绝对单位立即 `EvalCalcString`
> 求值为 px。至此 `inset`（top/left/right/bottom）与 `width/height` 两条解析路径对齐。
> 测试：`layout/calc_resolve_test.go` 覆盖 `top:calc(50% - 20px)` / `left:calc(...)` 等
> positioned 场景，全量通过。

### 现状定位
- 宽度/高度路径已支持 calc：`style/resolver.go:1582` `parseLength()` 对 `calc()` 做了
  相对单位检测，返回 `Length{Unit:"calc", CalcExpr:"..."}`（上一轮 `5cfff1c` 成果）。
- 定位路径**仍缺失**：`layout/positioned.go:109/110/159/160` 通过
  `asLength(cs.Properties["left"])` → `layout/layoututil.go:123 parseCSSLength()` 解析
  `top/left/right/bottom`。
- `parseCSSLength` 只做「数字 + 单位」切分：遇到 `calc(100% - 40px)` 时 `i==0`
  （首字符 `c` 非数字）→ 直接返回 `style.Length{Unit:"auto"}`。

### 根因
`width/height` 走 `style.parseLength`（已补 calc），而 `inset`（top/left/…）走的是
layout 包自己的 `parseCSSLength`，两条解析路径不一致，calc 支持只补了一半。

### 推荐方案
在 `layout/layoututil.go` 的 `parseCSSLength` 开头加 calc 分支（复用 `css` 包工具，
layout 已 import `wb-ui/css`）：

```go
func parseCSSLength(s string) style.Length {
    s = strings.TrimSpace(s)
    if s == "" || s == "auto" {
        return style.Length{Unit: "auto"}
    }
    // ★ 与 style.parseLength 对齐：calc() 含相对单位 → 延迟求值
    if strings.HasPrefix(s, "calc(") {
        arg := s[len("calc("):]
        if strings.HasSuffix(arg, ")") {
            arg = strings.TrimSuffix(arg, ")")
        }
        if css.CalcHasRelativeUnit(arg) {
            return style.Length{Unit: "calc", CalcExpr: strings.TrimSpace(arg)}
        }
        if v, err := css.EvalCalcString(arg, css.CalcContext{}); err == nil {
            return style.Length{Value: v, Unit: "px"}
        }
    }
    // …原有数字+单位切分…
}
```

**链路已验证可打通**：`resolveOffset`（`positioned.go:287`）→ `resolveLengthAuto` →
`resolveLength`（`layoututil.go:33`）已有 `case "calc"`，用真实 `cbSize`/fontSize/
viewport 求值。只需 `parseCSSLength` 正确吐出 `Unit:"calc"` 即可，无需改动 positioned 侧。

### 涉及文件
- `layout/layoututil.go`（唯一改动点）

### 风险
低。`style.Length` 已含 `CalcExpr` 字段，`resolveLength` 已处理 calc，纯增量补丁。
需注意 `inset` 简写是否走同一条 `cs.Properties` 路径（见下方验证）。

### 验证
- 新增 `layout` 测试：`position:absolute; top:calc(50% - 20px); left:calc(50% - 30px)`
  的 box 几何 = 包含块中心偏移后的坐标。
- `go test ./layout/...`（需 CGO + goskia PATH）。

---

## 2. `min()/max()/clamp()` 未实现（P1 · 功能缺失）

> **已实现（2026-08-13，提交 `a9c69d3`）**：`css/calc.go` 的 `parsePrimary` 支持
> `min`/`max`/`clamp` 多参比较函数与嵌套 calc；`extractCalcInner` 修正 TokenFunction
> 隐含开括号导致的嵌套函数右括号误匹配；新增 `parseMinMaxClamp`（逗号分隔参数列表，
> `clamp` 三参校验、`min`/`max` 至少一参）。`style/resolver.go` 与 `layout/layoututil.go`
> 的 `parseLength`/`parseCSSLength` 识别 `min(`/`max(`/`clamp(` 前缀（含相对单位延迟、
> 纯绝对立即求值）。
> 测试：`css/calc_string_test.go` / `style/calc_resolve_test.go` / `layout/calc_resolve_test.go`
> 覆盖 `min(600px, 100%)` / `clamp(16px, 4vw, 40px)` / `max(10px, 5em)` 等，全量通过。

### 现状定位
- `css/calc.go:9` 注释明确 `no min() / max() / clamp() support`。
- `parsePrimary()`（calc.go 末尾）：`TokenFunction` 分支仅识别 `calc`（嵌套也直接报
  `nested calc() not supported`），其它函数名统一报
  `unexpected function %s() in expression`。

### 根因
现代 CSS 里 `width: min(100%, 600px)`、`clamp(16px, 4vw, 40px)` 越来越常见（响应式
侧边栏、编辑器字号），但 calc 求值器只实现了二元算术，缺「多参比较函数」。

### 推荐方案
分两步：
1. **求值层**：在 `calcParser` 增加 `parseMinMaxClamp`——解析逗号分隔的参数列表，
   每个参数递归 `parseExpr()`（参数本身可以是 calc 或嵌套 min/max），`min/max` 取
   最值、`clamp(min, val, max)` 夹取。相对单位同样依赖 `CalcContext`。
2. **style 层**：`parseLength` / `isCalcValueS` / `extractCalcArgS` 目前只认 `calc(`。
   需扩展识别 `min(`/`max(`/`clamp(` 前缀，统一走「含相对单位 → 延迟、纯绝对 → 立即」
   的同一套分支。

### 涉及文件
- `css/calc.go`（求值核心）
- `style/resolver.go`（`parseLength` 前缀识别 + `extractCalcArgS`）
- `css/calc_string_test.go` / `style/calc_resolve_test.go`（补测试）

### 风险
中。难点在参数列表的分隔（逗号在嵌套 calc/函数内不参与切分）与 `clamp` 三参校验；
`min/max` 的语义与 CSS 规范（不接受空参、至少一参）需对齐。

### 验证
- 单测：`min(600px, 100%)`（context 1000px → 600px）、`clamp(16px, 4vw, 40px)`
  （vw 800 → 32px）、`max(10px, 5em)`（fontSize 16 → 80px）。
- 全量 `go test ./...`。

---

## 3. 布局三次全树遍历（P1 · 性能）

> **调研结论（2026-08-13）**：`WB_LAYOUT_PROFILE=1` 实测 ide_static.html（真实 IDE 页面）
> 布局总耗时 620ms，其中 **BFC 292ms（1043 次）+ FFC 238ms（400 次）占 85%**，IFC 44ms（577
> 次）、grid 38ms、table 7ms。`roundTree` 与 `updateContentSize` 是两次额外 O(n) walk（约
> 2000 节点 × 2），估算 <5ms（占比 <1%）——**阶段 A（合并这两次 walk）收益微乎其微，不建议
> 投入**。真正的瓶颈是 BFC/FFC 布局算法本身（单次 FFC 0.6ms、BFC 0.28ms），优化方向应聚焦
> 阶段 B/C（增量布局/脏子树）或 BFC/FFC 内部算法，均属高风险大工程，需先跑 `dev/consistency`
> 像素护栏再动手。
>
> **最终决策（2026-08-13）**：阶段 A 收益 <1%，**不投入**；阶段 B/C 属高风险大工程，仅在
> 业务出现可感知布局卡顿（超大文档滚动/频繁重排）时立项，立项前必跑 `dev/consistency` 像素护栏。

### 现状定位
一次完整布局存在**三次全树遍历**：
1. `layout/layout.go:17 Layout()` → `LayoutRoot`（每个 box 的 FormattingContext 布局）
   + `roundTree`（`layout.go:44` 全树 `Geometry.Round()`）。
2. `page/frameview.go:235 Layout()` → `rv.Layout(nil)` → `v.updateContentSize(rv)`
   （`frameview.go:259` 全树 walk 求最大 extent）。

### 根因
无 dirty-subtree 增量：`SetNeedsLayout(true)` 即整树重跑。`layout/layoutstate.go` 已
有 `GeometryForBox` 缓存，`layout/layoutprofile.go` 提供 `WB_LAYOUT_PROFILE=1` 分 FC
耗时统计（可先跑一次定位大头），但缺「只重排受影响子树」的脏标记传播。

### 推荐方案（分阶段，勿一步到位）
- **阶段 A（低风险）**：合并 `roundTree` 到布局主循环末尾（在 `ctx.Layout` 逐 box
  完成后就地 Round，省一次独立遍历），并让 `updateContentSize` 复用布局时已算出的
  box 几何而非重新 walk——前提是记录每层 `AbsoluteX/Y + Width/Height` 的边界增量。
- **阶段 B（中风险）**：`SetNeedsLayout` 携带「脏子树根」而非全局布尔，布局入口
  从该子树根往下重排，兄弟/祖先几何稳定时短路。
- **阶段 C（高收益·高复杂）**：为 FormattingContext 引入「尺寸依赖图」，仅当
  包含块尺寸/font-size 变化时才重排后代（对齐 WebKit 的 `LayoutState` dirty 传播）。

### 涉及文件
- `layout/layout.go`、`page/frameview.go`（阶段 A）
- `layout/layoutstate.go`、`page/frame.go`、`app/host.go`（阶段 B/C）

### 风险
高。布局顺序/几何缓存一致性极易出回归，务必先跑 `dev/consistency`（Edge 像素对比）
与全量 `go test ./...` 作为护栏。

### 验证
- `WB_LAYOUT_PROFILE=1 go run ./dev/static_probe/main.go` 看各 FC 耗时分布。
- 对比改动前后 `dev/consistency` 像素测试无差异。

---

## 4. Shadow DOM selector（P2 · 超大工程）

> **已实现基础增量（2026-08-13，提交 `1dff19a` + slot 投影）**：
> 1. **DOM 层**：`dom.ShadowRoot` 类型 + `Element.AttachShadow(mode)` + `ShadowRoot()`
>    访问器（closed 返回 nil）+ `FirstComposedChild`（有 shadow root 时返回 shadow
>    tree 子节点，light DOM 被隐藏）+ `AssignedNodes`（slot 匹配 host light-DOM）。
> 2. **渲染/布局层**：`rendering/rendertreebuilder.go` 与 `layout/box.go` 的
>    `buildChildren`/`buildFlexChildren` 统一走 `FirstComposedChild`；`<slot>` 元素
>    不生成 render object，展开渲染 assigned 节点（slot 投影）。
> 3. **bindings**：`el.attachShadow({mode})` / `el.shadowRoot` + `wrapShadowRoot`。
> 4. **测试**：`TestAttachShadow*` / `TestFirstComposedChild` / `TestAssignedNodes*`
>    / `TestShadowRootRendersInsteadOfLightDOM` / `TestSlotProjection` 全绿，全量
>    `go test ./...` 通过。
>
> **已实现样式隔离（2026-08-13，本轮）**：
> 1. **继承链**：`style/resolver.go parentElement` 对「父是 ShadowRoot」的情况返回
>    shadow host，使 shadow tree 顶层元素从 host 继承（CSS Scoping 继承跨边界）。
> 2. **作用域隔离**：`collectSheetDeclarations` 开头按 scoping root 过滤——UA sheet
>    全局作用；author sheet 只在相同 shadow root 内作用（文档级 sheet 不穿透 shadow，
>    shadow 内 sheet 不泄漏到文档/其它 shadow）。
> 3. **样式提取**：`page/frame.go` 的 `extractAndAddStyles` + `styleFingerprint` 改用
>    `dom.WalkComposedTree`（新增）进入 shadow tree 收集 `<style>`（此前
>    `GetElementsByTagName` 只遍历 light DOM，漏掉 shadow 内 `<style>`）。
> 4. **测试**：`TestResolver_ShadowInheritanceFromHost` / `_ShadowStyleScopedInside` /
>    `_ShadowStyleDoesNotLeak` / `_DocumentStyleDoesNotPenetrate` 全绿，全量通过。
>
> **已实现级联来源（2026-08-13，本轮）**：
> 1. **DOM 辅助**：`HasShadowRoot`（closed 也返回 true）/ `AssignedSlot`（slot 分配反向
>    查询）/ `TreeScopeDepth`（shadow 嵌套深度）/ `PartNames`。
> 2. **解析器**：`:host()`/`:host-context()`/`::slotted()` 参数解析为 SelectorList，
>    `::part()` 参数解析为 part-name 列表。
> 3. **SelectorChecker**：`:host`/`:host(sel)`/`:host-context(sel)`/`::slotted(sel)`/
>    `::part(name)` 匹配逻辑（对标 CSS Scoping Level 1）。
> 4. **级联 scope 维度**：`collectedDecl.scope` = 样式表 tree-scope 深度（0=document，
>    N=N 层 shadow）；排序在 origin/importance 之后按 scope 比较——normal 声明内层
>    scope 优先、!important 反转（外层优先）。
> 5. **跨边界路由**：`collectShadowHostDeclarations`（:host 规则→host）、
>    `collectSlottedDeclarations`（::slotted 规则→assigned light-DOM 节点）、
>    `collectPartDeclarations`（::part 规则→shadow 内 part 元素）。
> 6. **测试**：`TestResolver_Host*` / `TestResolver_SlottedStyle` / `TestResolver_Part*`
>    + `TestSelector_Match*` 全绿，全量通过。
>
> **已实现 host-selector 前缀前向匹配（2026-08-13，本轮）**：
> `x-widget::part(btn)` / `x-widget::slotted(span)` 中 `::part`/`::slotted` 之前的
> simple selector 现在匹配 shadow host（CSS Scoping Level 1 forward matching），不再
> 误匹配候选元素本身。`SelectorChecker.matchCompound` 检测 compound 末尾的
> `::part`/`::slotted`，将前缀路由到 `shadowHostFor`（`::part`→`ContainingShadowRoot
> (el).Host()`，`::slotted`→`AssignedSlot` 所在 shadow root 的 host）。
> 测试：`TestSelector_MatchPartWithHostPrefix` / `_MatchSlottedWithHostPrefix` /
> `TestResolver_PartWithHostPrefix` / `_SlottedWithHostPrefix` 全绿，全量通过。
>
> **已实现跨 combinator forward matching + composedPath + fallback（2026-08-13，本轮）**：
> ① 跨 combinator 前向匹配：`matchComplex` 检测 compound 以 `::part`/`::slotted` 结尾时，
> 往左 combinator 从 shadow host 出发（新增 `dom.ComposedParent` 穿透 shadow boundary），
> 使 `.outer x-widget::part(btn)` 中 `.outer` 匹配 host 的祖先而非 part 元素的祖先。
> ② 事件 composedPath：`DispatchEvent` 用 `buildEventPath`（ComposedParent 穿透 shadow）
> 构建路径并存到事件；`ComposedPath()` 返回完整路径；composed 事件穿透 shadow boundary、
> 非 composed 事件在 shadow root 处截断；JS 绑定层 `dom_events.go` 同步穿透。
> ③ slot fallback content：`AssignedNodes` 无 assigned 节点时返回 `<slot>` 自身子节点。
> 测试：`TestSelector_MatchPart/SlottedAcrossCombinator` / `TestEvent_ComposedPathShadow` /
> `TestAssignedNodesFallback*` 全绿，全量通过。
>
> **仍缺（后续增量）**：① 事件 retargeting（listener 内 `event.target` 在 shadow
> boundary 处应 retarget 到 host）；② slot 分配的 composed 事件路径未插入 slot 节点
> 本身；③ `exportparts`（跨层 part 转发）与 `::part` 的多个 part-name 匹配优化。
>
> **已实现 retargeting + slot 路径 + exportparts（2026-08-13，本轮收尾）**：
> ① 事件 retargeting（DOM §2.8）：`DispatchEvent` 对每个 currentTarget 调用
> `retargetedTarget`——当真实 target 位于某 shadow tree 内且 listener 位于该 host 之上
> （composed 树中）时，target 逐层 retarget 到最内层 shadow host；dispatch 结束后恢复
> 真实 target。② slot 节点插入 composed 路径：新增 `eventPathParent`（assigned 节点的
> 下一跳是分配它的 `<slot>`，而非其 light-DOM 父节点），`buildEventPath` 改用之；非
> composed 事件仍走裸 parent 链并在 shadow root 截断。③ exportparts 跨层转发
> （CSS Scoping L1 §4.5）：`matchPart` 沿 `exportparts` 属性逐层重命名 part 名，支持
> 多层嵌套 shadow 的 `inner→mid→outer` 转发链；`parseExportparts` 返回 `map[string][]string`
> 支持一对多导出（`exportparts="x: a, x: b"` 同时导出 a、b）。
> 测试：`TestEvent_Retargeting` / `TestEvent_ComposedPathSlot` /
> `TestSelector_MatchPartExportparts*` 全绿，全量通过。
>
> **已实现 relatedTarget retargeting + hover 派发（2026-08-13）**：`DispatchEvent` 内
> relatedTarget 与 target 同步 retarget（DOM §2.8 末段，dispatch 结束后恢复）；新增
> `EventMouseOver/Out/Enter/Leave` 类型 + `dispatchHoverEvents`（mouseover/out 冒泡带
> relatedTarget，mouseenter/leave 不冒泡），两个 hover 切换点接入。测试：
> `TestEvent_RelatedTargetRetargeting` / `TestDispatchHoverEvents` 全绿。
>
> **仍缺（后续增量）**：① `::part` 的多个 part-name 匹配优化（当前 O(names) 线性扫描，
> 可改为哈希集合，收益 <1%）。

### 现状定位
- `css/selectorchecker.go`：`:host`/`:host-context`/`::slotted`/`::part` 匹配已实现，
  但 host-selector 前缀跨 shadow boundary 前向匹配未实现。
- `dom/node.go:13`、`dom/element.go:8`：整个 dom 包注释 `shadow tree / custom elements
  / mutation observers / style recalc / rendering hooks are omitted`。
- `bindings/dom.go:3694` `getRootNode` 已按标准返回根，但注释点明「无 shadow DOM」。

### 根因
这是**跨三层**的系统性缺口，非单点：
1. **DOM 层**：无 `ShadowRoot` 节点、无 `attachShadow()`、无 slot 分配；
2. **Selector 层**：`::slotted`/`::part`/`:host` 无匹配逻辑；
3. **样式层**：shadow 边界不隔离样式，无 `:host` 级联来源。

### 推荐方案
WebKit 架构参考（`ref/WebKit` 已在本工作区）：
1. DOM 层：`dom.ShadowRoot` + `Element.attachShadow({mode})` + slot/flattened-tree 遍历
   （参考 `dom/node.go` 现有树遍历 + `editing/visibleposition.go` 已标注的「shadow
   roots 遍历」TODO）。
2. Selector 层：`selectorchecker.go` 增加 `::slotted`（匹配 slot 分配的 light-DOM 节点）、
   `:host`（匹配 shadow host 的自身 + `:host()` 参数）。
3. 样式层：shadow 内样式优先于 light DOM 继承（CSS Scoping），`:host` 规则来源单独处理。

### 涉及文件
- `dom/`（新增 ShadowRoot）、`css/selectorchecker.go`、`css/selector.go`、
  `style/resolver.go`、`bindings/dom.go`。

### 风险
高。影响面横跨 DOM 遍历、事件路径（`dom/event.go` 已标注 composedPath 无 shadow）、
样式级联，建议按「先 attachShadow + slot 基础 → 再 ::slotted/:host → 最后 ::part」拆
多个可验证增量，每步配独立测试。

### 验证
- 单测：`el.attachShadow({mode:'open'})` 后 `shadowRoot.innerHTML` 渲染隔离；
  `::slotted(span)` 命中 slot 分配节点。

---

## 5. `mask-image` 仅存属性不绘制（P3）

> **已实现（2026-08-13）**：图片-as-alpha 遮罩已落地。goskia 补 `Image.MakeShader` 绑定（C API
> `sk_image_make_shader` 早已存在，仅缺 Go 封装）；`graphics.Canvas` 新增 `SaveLayerForMask` /
> `ApplyImageMask`（SaveLayer + `BlendModeDstIn` 把图片缩放到元素尺寸作为 alpha 遮罩）；
> `paintObjectBackground` 在 mask-image 存在时用离屏 layer 遮罩 background/border。像素测试
> `TestMaskImageAlpha` 验证「mask alpha=0 丢弃、alpha=255 保留」。当前仅遮罩 background/border
> （未遮罩前景文字/SVG），且 mask-repeat/size/position 尚未解析（默认整图缩放到元素尺寸）。

### 现状定位
- `rendering/mask_test.go`：`mask-image` 仅被存为 property（`cs.GetProperty("mask-image")`
  非空），**无 image-as-alpha 绘制**。

### 根因
实现 mask 需要「把图片解码为 alpha 通道再对目标做 Skia mask」，当前 goskia 绑定未暴露
`SkMaskFilter` 或 image-as-alpha 合成路径。

### 推荐方案
- 短期：确认 goskia 是否有 `MaskFilter`/`SkImageFilter` 绑定（`goskia/skia/` 目录），
  有则走 `SaveLayer` + `BlendMode::kSrcIn` 的 mask 合成。
- 若绑定缺失：先补 goskia 侧 API，再在 `rendering` 层接入。

### 涉及文件
- `goskia/skia/`（若需补绑定）、`rendering/`（mask 合成）。

### 风险
中。依赖跨仓改动（goskia），需与主项目协调；先用 `mask_test.go` 守回归（保证不 crash）。

### 验证
- 像素测试：`mask-image: url(mask.png)` 下目标区域 alpha 与图片一致。

---

## 附：WebSocket —— 已完善（非遗留问题）

`bindings/dom.go:1065-1190` 的 WebSocket 已是**完整 stub**：提供 readyState 常量、
`onopen/onmessage/onerror/onclose`、`send/close/addEventListener/removeEventListener`，
并通过 `globalThis.__desktopWS.dispatchMessage/dispatchStatus` 供宿主注入事件。这是
桌面端「无真实网络」场景的**有意设计**（不建连接、不崩溃、事件由宿主推入），
不是 bug。有真实传输需求时在宿主注入层覆盖 `window.WebSocket` 即可，无需改引擎。

---

## 建议执行顺序（全部完成，仅供复盘）

> 截至 2026-08-13，5 项遗留已全部处理完毕。以下为当时拟定的推进顺序及最终结果：

1. **P0**：`parseCSSLength` 补 calc → ✅ 已实现（`0a042d3`）。
2. **P1-min/max/clamp** → ✅ 已实现（`a9c69d3`）。
3. **P1-布局增量** → 🔍 已调研，阶段 A 收益 <1% 暂不投入，B/C 待业务驱动。
4. **P2 Shadow DOM** → ✅ 已完整实现（`1dff19a` → `2bcf5cc` 提交链）。
5. **P3 mask-image** → ✅ 已完整实现（子树遮罩 + size/repeat/position/mode + SVG `<mask>`）。

### 剩余可优化项（非阻塞，按需）
- `::part` 多 part-name 线性扫描 → 哈希集合（CSS Scoping L1 性能优化，收益 <1%）。
- 布局增量（阶段 B/C）：脏子树/尺寸依赖图，高风险，业务驱动时再立项。
