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
> 阶段 B/C（增量布局/脏子树）或 BFC/FFC 内部算法，均属高风险大工程，需先跑 `dev/suites/consistency`
> 像素护栏再动手。
>
> **最终决策（2026-08-13）**：阶段 A 收益 <1%，**不投入**；阶段 B/C 属高风险大工程，仅在
> 业务出现可感知布局卡顿（超大文档滚动/频繁重排）时立项，立项前必跑 `dev/suites/consistency` 像素护栏。

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
高。布局顺序/几何缓存一致性极易出回归，务必先跑 `dev/suites/consistency`（Edge 像素对比）
与全量 `go test ./...` 作为护栏。

### 验证
- `WB_LAYOUT_PROFILE=1 go run ./dev/suites/static_probe/main.go` 看各 FC 耗时分布。
- 对比改动前后 `dev/suites/consistency` 像素测试无差异。

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

---

## 已闭环的 DOM/渲染能力缺口（2026-09 批次）

这些是「注释里记着的未实现项」而不是文档里的 P0-P3 大项，逐项闭环后留档，
避免后续再被当成遗留重复调研：

| 能力 | 落地内容 | 提交 |
|------|----------|------|
| CSS 伪类缺口 | `:fullscreen` / `:open` / `:closed` / `:modal` 的枚举、名称解析与匹配（`:modal` 判定与渲染层共用 `dom.Element.IsModalDialog`） | `667c7ae`、`fb39d5f` |
| Fullscreen API | `Element.requestFullscreen()` / `document.exitFullscreen()` / `fullscreenElement` / `fullscreenEnabled`；事件异步派发（元素先于 document）、重复请求不派发、未连接元素派发 `fullscreenerror` 并 reject；UA 表加 `:fullscreen:not(:root)` 铺满规则；宿主钩子 `OnFullscreenChanged` | `4fd6d1b` |
| `<dialog>` JS 接口 | `open` 属性反射（`<details>` 共用）、`show()` / `showModal()` / `close()`、`returnValue`；已打开时按规范抛错（用 TypeError 兜底），`toggle` / `close` 异步派发；状态迁移走样式失效链 | `fb39d5f` |
| `::backdrop` 遮罩 | 解析（css 层已有）+ 生成（layout/rendering 在模态 dialog 之前插入伪元素盒/对象）+ 绘制（UA 规则 `position:fixed; inset:0; rgba(0,0,0,.1)`）；`ResolvePseudoElement` 不再要求 `::backdrop` 声明 `content` | `3583e6b` |
| `<dialog>` 状态算法重写 | 按 HTML §4.11.6 对齐：`open` 纯反射（移除属性**不**退出模态状态）；`show()`/`showModal()` 目标态相同时静默返回、不同时抛错；打开/关闭先同步派发**可取消**的 `beforetoggle`，再排队 `toggle`；关闭时 `toggle` 与 `close` 在同一批任务里按序派发（分两次排队实测顺序会颠倒）；`<details>` 的 open 切换派发 `toggle`（§4.11.4）；UA 表补 `dialog:not([open]){display:none}` | `a93a1d0` |
| CSS 表单状态伪类 | `:default`（表单首个 submit/reset 默认按钮、已勾选 checkbox/radio、已选中 option）与 `:indeterminate`（checkbox 的 IDL 状态、radio 组无勾选、`<progress>` 无 value）；`input.indeterminate` IDL 属性（不写内容属性） | `9a1794f` |
| 伪元素名称表一致性 | `PseudoElementName` 漏了 `-webkit-scrollbar{,-thumb,-track}`（查询表有、名称表无 → 序列化出裸 `::`）；新增「枚举 ↔ 名称表 ↔ 查询表」三向往返测试与序列化断言锁死这类漂移 | `9a1794f` |
| flex/grid 容器里的 `::backdrop` | 模态 `<dialog>` 作为 flex/grid item 时也生成遮罩：`buildFlexChildren` 两侧（layout + rendering）与块级路径同位置插入（fixed 定位子项不占 flex item 槽位） | `e8d2108` |
| `:target` 语义 | 此前只判「URL 非空 且有 id」→ 任意带 id 的元素在任意非空 URL 下都匹配；改为 URL fragment（百分号解码）与元素 id 相等，`#top` 无 `id="top"` 元素时回退根元素 | `f307666` |
| `ToggleEvent.oldState/newState` | dom 新增 `ToggleEvent`（`beforetoggle` 可取消）；`eventToJS` 暴露两个字段；`<dialog>` 与 `<details>` 的 `toggle`/`beforetoggle` 都带上迁移方向 | `db8d9b8` |
| 约束校验伪类 | `:valid` / `:invalid` / `:in-range` / `:out-of-range` 从硬编码 false 改为查「注入的约束校验状态」：`css/validity.go` 定义纯数据 `FormValidity` + 注入点（css 不能 import html5——html5 为 UA 表已 import css），`html5` 在 init 里注入实现。barred 元素（disabled/readonly/datalist 后代/type=hidden,reset,button 等）两者都不匹配；`:in-range` 只匹配「有范围限制」（min/max 存在且能按类型解析）的元素；日期类类型的 min/max 也进入 `ValidityState`（此前只有 number/range） | `46963a2` |
| 约束校验 IDL | 元素级 `validity`（11 个只读布尔属性）/ `validationMessage` / `willValidate` / `checkValidity()` / `reportValidity()` / `setCustomValidity()`；表单级 `checkValidity()` / `reportValidity()` / `noValidate`。判定逻辑复用 html5 的 ValidityState；`setCustomValidity` 走样式失效链。顺带修 `<input>` 的 `Validity()` 漏检 customError（`setCustomValidity` 后 `checkValidity()` 仍返回 true） | `83377cc` |
| 提交时交互校验 | 新增 `html5/interactive.go`：对每个无效控件派发 `invalid` 事件（不冒泡）、返回列表供宿主聚焦第一个；`requestSubmit`/提交按钮点击在未声明 `novalidate`/`formnovalidate` 时先校验，失败即中止（连 `submit` 事件都不派发）。顺带修 `form.submit()` 语义：规范里它**不**校验、也**不**派发 submit 事件（此前等价于 requestSubmit） | `83377cc` |
| `:user-valid` / `:user-invalid` | dom 新增 user validity 状态；css 新增枚举 + 名称 + 解析（含枚举三向往返测试）；匹配要求「candidate 且 user validity 为 true」。引擎在 change 事件路径（失焦提交/点击 checkbox-radio/选择 option/拖动 range）与控制点（交互校验=提交尝试）置位，并经注入钩子重算样式；脚本 `dispatchEvent(new Event('change'))` 不置位（与浏览器一致） | `54d2888` |
| 约束校验的像素级夹具 | `dev/suites/cssprobe` 新增 `constraint-validation` 夹具：6 个色块断言 `:valid`/`:invalid`/`:in-range`/`:out-of-range`/barred/`setCustomValidity` 的真实渲染，1 个断言「未交互的无效控件不匹配 `:user-invalid`」。反向验证：注释掉注入后 6 项检查中的 5 项立刻失败，确认夹具盯着注入链路 | `15f8b5a`、`54d2888` |
| `:user-valid` / `:user-invalid` 的焦点会话规则 | 补上 MDN 列出的第 3 条：值在控件获得焦点时无效、而用户在焦点仍在控件内时把它改成了有效（`:user-invalid` 是镜像方向）→ 立即获得 user validity。dom 增加焦点会话记忆（`SetFocusValidity`/`FocusValidity`，失焦清除）与 `OnElementFocused` 注入钩子（dom 不依赖 html5）；`html5.NoteFocusGained` / `NoteUserInput` 做判定（未翻转不置位、无焦点会话不置位）；三条真实值写入路径全部接线——`app.Host.setFocusedElementValue`（IME/组合输入）、`webkit.FormFocus.applyValue`（键入/退格/粘贴）、`app.Host.setRangeValueFromX`（range 点击与拖动），漏一条这条规则就静默失效 | `93c3032` |
| `ToggleEvent.source` | 把 IDL 里的 `source`（`Element?`）落地：dom 增加字段 + `ToggleEventInit.Source` + 构造参数 + `Source()`；bindings 暴露为 `null` 或对应元素对象。非 null 只出现在 popover 的 invoker 场景（已随 Popover API 落地，见下表），其余路径按规范恒为 `null`：dialog 的 `close()`/`requestClose()` 传 null、`form method=dialog` 提交传 null、close watcher 读的 request close source element 槽只被 `requestClose()` 写过（写的也是 null）、details 的 toggle 任务只初始化 oldState/newState。字段存在的意义是让 `e.source === null` 与 MDN 的 `event.source === undefined` 特性检测得到和浏览器一样的结果 | `8487f72` |
| 日期类输入的 step mismatch | 新增 `html5/step.go`：date 以天、month 以月（月长不等，不能换算成毫秒）、week 以周、time 与 datetime-local 以秒换算，default step 分别为 1 天/1 月/1 周/60 秒/60 秒；step base 按 min → value 内容属性 → default step base（只有 week 定义了它：−259,200,000 ms）→ 0；step 缺失/解析失败/≤0 时退回 default step 而不是跳过（只有 `step="any"` 跳过）；整数轴（毫秒/月，< 2^53）用整数取模精确判定，number/range 的容差路径顺带修掉 `0.3`（step=0.1）被浮点误差误判成 mismatch 的问题 | `0673d69` |
| `<input type=week>` 的 ISO 周解析 | 修 `parseWeek`：Go 的时间布局里 `2006-W02` 的 `02` 是「月中的第几天」，早期实现用它解析周值，于是 `1970-W03` 被读成 1970-01-03、`1970-W01…W04` 全部落进同一周（week 的 min/max 与 step 相位因此失真）。改为自行解析「四位以上年 + `-W` + 两位周号」、按 ISO 规则（含 1 月 4 日的那一周是 W01）求周一，并校验第 53 周只在该年真的存在时才合法 | `0673d69` |
| Popover API | HTML §6.12 整套落地：dom 的 popover visibility state + top layer 近似栈（`dom/popoverstate.go`）；新 `popover` 包实现 show/hide/toggle 三个算法、check popover validity、auto/hint 互斥与 topmost popover ancestor、light dismiss 的 pointerdown/pointerup 两阶段、close request（Esc）、invoker 的激活行为（`popovertarget*` 与 `commandfor`/`command`）；JS 层 `HTMLElement.popover`（枚举属性反射）+ 三个方法 + 四个 invoker 属性 + `ToggleEvent` 构造器；`:popover-open` 伪类与 UA 样式（未显示不生成盒、显示中的 fixed + `z-index:1100` 近似 top layer、`::backdrop` 默认透明不吃指针事件，backdrop 盒的判定泛化为 `dom.Element.NeedsBackdrop`）；宿主两条路径（`webkit.Interaction`、`app.Host`）都接 light dismiss / Esc / invoker。已知差异见下 | `634f4b0`、`3422a1a`、`3d56853`、`1b2aa8c` |
| Popover API 的像素夹具 | `dev/suites/cssprobe` 新增 `popover`（7 条：hidePopover 后回到不生成盒、`:popover-open` 上色、后打开的 auto 关掉先前的、嵌套 auto 保留 popover 祖先、manual 与 auto 互不影响、UA 定位居中）与 `popover-backdrop`（2 条：作者上色的 backdrop 铺满视口、popover 画在自己 backdrop 之上）。反向验证：`:popover-open` 恒 false → 5 条失败；去掉 UA 的 `display:none` → 2 条失败；backdrop 改成不透明 → 3 条失败 | `1b2aa8c` |
| Worker（并发脚本执行） | 新增 `worker` 包（每个 Worker 一个**独立 goja 运行时 + 独立 goroutine + 独立事件循环**，两个运行时之间只传 JSON 文本）与 `bindings/worker.go`（`Worker` 的 postMessage/onmessage/onerror/addEventListener/terminate + `MessageEvent`；worker → 主线程的消息由主事件循环的宏任务派发，主运行时的 JS 只在主线程 tick 里执行）。脚本来源：`data:` URL 或宿主注入的 `bindings.SetWorkerScriptFetcher`；worker 全局提供 self/name/close/importScripts/setTimeout 家族/console。单测 `worker/worker_test.go`（6 项）+ `bindings/worker_test.go`（12 项，含「worker 忙等 400ms 时主线程定时器照常推进」的并发本质断言） | `5a4a75c`、`ff965d5` |

### Worker 的已知差异（有意保留）

- **消息是 JSON 克隆，不是结构化克隆**：`Date` 变 ISO 字符串、`Map`/`Set`/`RegExp`/`TypedArray` 变 `{}`、值为 `undefined` 的成员被丢弃、`NaN`/`Infinity` 变 `null`。函数、Symbol 与循环引用抛 `DataCloneError`（这点与浏览器一致；算法即 `JSON.stringify`/`JSON.parse`）。
- **无 transferable / SharedArrayBuffer**：`postMessage(msg, [buf])` 的第二个参数被忽略——既不做所有权转移，也不报错。
- **无 MessageChannel / MessagePort / BroadcastChannel**：`MessageEvent.ports` 恒为空数组。
- **无 module worker**：`new Worker(url, {type:"module"})` 按 classic 处理（`{name}` 生效）。
- **无 SharedWorker / ServiceWorker / worklet**。
- **无 Blob URL 脚本**：引擎没有 `Blob` 与 `URL.createObjectURL`，因此不支持 `blob:` worker。
- **worker 全局刻意保持最小**：只有 `self`/`name`/`console`/定时器/`postMessage`/`onmessage`/`importScripts`/`close`。没有 `location`、`navigator`、`fetch`、`XMLHttpRequest`（引擎的 fetch/XHR 在 bindings 的主线程层）。
- **无同源与 CSP 检查**：脚本 URL 完全交给宿主 Fetcher；引擎自身不联网（只认识 `data:` URL），未注入 Fetcher 时非 `data:` URL 派发 error 事件（`Failed to ... no fetcher registered`）。
- **脚本加载/编译失败派发 `error` 事件**（ErrorEvent 形状的 `message`/`filename`/`lineno`/`colno`/`error`），worker 仍然存活但不处理消息（与浏览器行为一致）。
- **宿主需要一行接线才能加载非 `data:` 脚本**：`bindings.SetWorkerScriptFetcher(func(url string) (string, error))`；引擎不替宿主决定联网策略（与 WebSocket 采用「宿主注入传输」同一取舍）。

### popover 的已知差异（有意保留）

- **没有真正的 top layer**：显示中的 popover 用 `position:fixed` + `z-index:1100`
  近似（高于普通内容与 `dialog[open]` 的 1000）。因此「后显示的 popover 一定在
  其它所有内容之上」只在 z-index 层面近似成立，popover 之间的先后顺序靠文档顺序
  / 作者样式。UA 的居中定位也不支持 `width:fit-content`，改用
  `top/left:50% + translate(-50%,-50%)`（与 `dialog[open]` 同一近似）。
- **没有 close watcher**：Esc（close request）由宿主直接调用
  `popover.CloseRequest`，只作用在最上层的 auto/hint popover（manual 不响应，与
  规范一致）。`<dialog>` 的 Esc 关闭仍未实现。
- **没有 implicit anchor element / CSS anchor positioning**：invoker 与 popover
  的锚点关联只记录到 `popover trigger`（供 `ToggleEvent.source` 用），不参与定位。
- **removal steps 未实现**：popover 元素从文档移除时不清理栈，改为在构建列表时
  过滤已断开的元素（与 `<dialog>` 的模态状态同一取舍）。
- **`command` 事件（CommandEvent）未派发**：本引擎还没有 CommandEvent 接口，派发
  一个无 `command`/`source` 字段的同名事件比不派发更容易误导作者。
- **键盘激活未实现**：本引擎没有「按钮上 Space/Enter 派发 click」的路径，因此
  invoker 的键盘激活不生效（鼠标点击路径完整）。
- **属性 setter 一贯宽松**：`popoverTargetElement = <无 id 的元素>` / `= <非元素>`
  在浏览器抛 InvalidStateError / TypeError，本端口静默忽略（与其它反射 ID 的
  属性一致）。

### 仍未建模（有意保留）
- `:autofill` / `:picture-in-picture`：本引擎没有表单自动填充或画中画模型，
  无判定依据，故作永不匹配。
- view-transition 伪元素：已解析但永不匹配（无 view-transition 机制）。
- **`<input>` / `<textarea>` 的 value 用内容属性建模**（本端口的简化）：规范里
  value 还带一层「dirty value flag」内部状态，属性只在初始值/反射时参与。这一差异
  在本轮新增的 step 校验里立刻显形——step base 的第 2 步取「value 内容属性」，而本
  端口的用户输入直接改写该属性，于是**没有 min 的控件在用户输入后 base 跟着漂移**，
  再也不可能出现 step mismatch（浏览器里用户输入不改属性，base 保持稳定）。有 min
  的元素不受影响（base 取 min），而那正是规范推荐的写法。彻底对齐需要引入 dirty
  value flag，会牵动渲染取值、表单提交与配置面板的取值路径，未立项。

---

## 运行模式与 UI 库层（2026-09 批次）

`webkit.Mode`（嵌入浏览器 / UI 库）与 `ui` 包（Go 构建界面 + web 片段混入）落地
后留档以下内容。模式语义表与用法见 `docs/MODES.md`。

### 顺带修复

| 项 | 内容 |
|----|------|
| 每次 LoadHTML 后样式重复全量重扫 | `page.Frame.SetDocument` 提取样式后未同步 `styleFP` → 紧随其后的首次 `RebuildRenderTree` 判定「指纹变化」→ 再全量重扫一次：`<style>` 重复解析、`<link>` 重复加载（宿主 `StyleSheetLoader` / `ResourceResolver` 被重复调用一次）。修复后整条加载路径上 resolver 只被请求 1 次（`ui.TestToolkitModeResourceResolver` 断言 `calls == 1`） |
| `file://` 标准 URL 形式读不到文件 | 旧的 `strings.TrimPrefix(href, "file://")` 对 `file:///C:/dir/f.css` 留下前导斜杠 → `os.ReadFile` 必然失败（只有 `file://C:/dir/f.css` 这种非标准写法能读）。新增 `webkit.fileURLPath` 走 `net/url` 解析：盘符路径去前导斜杠、支持 `file:///home/u/f.css` 与 UNC `file://host/share/f` |

### 真实 HTTP 路径修复（用 `dev/probes/browser_http_probe` 发现）

「嵌入浏览器」此前只用 `data:` URL 与内存替换验证过，没人用**真实服务器**跑过——
补上真起 HTTP 服务器（`httptest`）的端到端探针后，一次暴露 4 个缺口（均为
「能联网」这件事的必要条件）：

| 项 | 根因 | 修复 |
|----|------|------|
| 相对 `<link>` / `<script src>` 加载失败 | `WebView.loadExternalResource` 把引用原样交给文件/网络分支——`<link href="/style.css">` 落到「相对当前工作目录读文件」 | 新增 `dom.ResolveURL(base, ref)`（`document.baseURI` 语义）+ 文档基地址接线：引用先按原样问 `ResourceResolver`，再按文档 URL 绝对化后问一次，仍未命中才走网络/文件 |
| `fetch("/api.json")` 失败（`unsupported protocol scheme ""`） | fetch/XHR 把 URL 原样交给 `http.NewRequest`，没有文档基地址概念 | `page` 新增 per-interpreter 文档 URL 登记表（`SetDocumentBaseProvider` / `DocumentBase`，提供器而非快照→跟随导航）；fetch/XHR 请求前用 `dom.ResolveURL` 补全。**桥路由匹配顺序保持「原样 URL 优先」**，宿主按 `"/api/x"` 注册的路由不受影响，另补一次绝对 URL 匹配 |
| 外部 CSS/JS 即便解析正确也不生效 | `fetchHTTP` 对非 HTML 兼容的 Content-Type（`text/css`、`application/javascript`）**返回 error 与空内容**——注释写的是 "warn but still return the content"，实现却返回了空串 | 降级为 `page.Logf` 提示、内容照常返回（取内容层不知道调用方用途；真实服务器对 .css/.js 不会回 `text/html`） |
| `document.URL` / `location.href` 与文档不符 | `document.URL` 是注册时求值的静态快照（恒为空串）；`location` 是写死 `about:blank`/`file:` 的静态桩；`LoadURL` 也从未 `doc.SetURL` | `LoadURL` 把 URL 写进 `Document`（`LoadHTML` 视为 `about:blank`，内部拆出 `loadHTMLFrom(src, docURL)`）；`document.URL` 与 `location.href/protocol/host/hostname/port/pathname/search/hash/origin` 改为**动态 accessor**；`history.pushState/replaceState` 改为更新文档 URL（同源路径相对当前文档解析），不再直接写 location 字段 |
| 重定向后的文档基地址是「请求 URL」而不是「最终 URL」 | `fetchHTTP` 只返回响应正文，`LoadURL` 只能拿入参 URL 当基准——`http://host` 被 301 到 `https://host/` 后，页面里的 `style.css` 会解析回 `http://host/style.css` | 新增 `fetchURLWithFinalURL`（内部用 `resp.Request.URL`）→ `LoadURL` 用**最终 URL** 作为文档基地址；`fetchURL` 保持原签名，其余调用点不受影响 |

回归资产：`webkit/browser_http_test.go`（4 项，真起 `httptest`：相对引用/文档 URL/导航后基准跟随/重定向后基准/UI 库模式零网络）、`dom/url_test.go`（12 例解析表）、`dev/probes/browser_http_probe`（诊断脚本，47 条断言全 PASS）。
反向验证：隐去相对解析 → 3 条断言失败（含 `unsupported protocol scheme ""`）；恢复 Content-Type 返回 error → 「服务器收到了 /style.css 但样式不生效」；把 fetch 基地址去掉 → 相对 fetch 失败；把重定向基准改回请求 URL → 服务器日志显示 `/style.css`（而非 `/b/style.css`）。

### 图片（`<img src>`）与 CSS `@import` 接通外部资源通道

上面那批修完之后，探针把 `<img src>` 与 `@import` 作为「事实项」打印出来——
两条都**没有 URL 加载通道**：

| 项 | 根因 | 修复 |
|----|------|------|
| `<img src>`（及 `background-image`/`mask-image`/SVG `<image href>`）不走外部资源通道 | 图片由渲染层自己取字节（`rendering.backgroundimage.go` 内置 `httpGet`）：**绕开宿主 `ResourceResolver`、不看运行模式**（UI 库模式下 `<img src="http://…">` 会真的发请求）；相对引用被当成宿主进程工作目录里的文件（真实页面的 `<img src="logo.png">` 必然失败） | 新增 `rendering.ImageResourceLoader` 接线（`ResolveURL` 文档基准解析 / `AllowsExternal` 模式门禁 / `Load` 取字节）。载体链：`Frame.ImageLoader` → `RenderTreeBuilder` → `RenderView.imageLoader` → `Paint` 入口提升为「当前 loader」（save/restore，嵌套 Paint 即 iframe 子文档绘制安全）→ `loadBackgroundImageWith`。取字节统一复用 `webkit.loadExternalResource`，因此 `<img>` 与 `<link>`/`<script>` 的拒绝面完全一致；iframe 子文档的图片按**子文档 URL** 解析 |
| CSS `@import` 全部静默跳过 | `style.Resolver.resolveImports` 依赖 `Resolver.StyleSheetLoader`，而该字段**从未被设置**（`page.Frame.StyleSheetLoader` 是另一个字段，两者没打通）→ 第一行就 return。真实站点普遍用 `@import` 拆分样式 | `page.Frame` 在 `SetDocument` 时把 resolver 接到 `Frame.StyleSheetLoader`（与 `<link>` 同通道）；相对 @import 按 CSS 规范以**样式表 URL**（`sheet.BaseURL`）为基准解析（`resolveImportURL`：绝对 URL 走 URL 语义、相对 base 走目录拼接、带 scheme/协议相对原样）；递归加深度上限 8 挡循环导入；导入表带解析后的 URL（嵌套逐级正确）。@import 规则的源序仍在本表之前（CSS-CASCADE-5 §3） |

顺带修掉一处**模式门禁旁路**：图片缓存（`rendering.backgroundImageCache`）是
进程级全局的，只看缓存会让另一个（浏览器模式）WebView 取回的同名 URL 在 UI 库
模式里照样显示出来。为此 `AllowsExternal` 在**缓存查询之前**判定——探针实测到
过这个现象，`TestToolkitModeRejectsImageURLsEvenWhenCached` 锁定它。

回归资产：`webkit/browser_http_media_test.go`（5 项，真起 `httptest`：图片
请求+固有尺寸+**绘制像素**、background-image 同通道、`@import` 生效与
「样式表 URL 基准」（文档 `/imp/page/`、样式表 `/imp/css/`，误按文档 URL 会拿到
故意放在同级的 999px 那份）、@import 源序、UI 库模式拒绝（含缓存旁路））。
反向验证 4 项：`ResolveURL` 返回空 → `/y/page.html` 显示 `/x` 的红图（跨文档
串味，证明规范化缓存键的必要性）；`AllowsExternal` 恒 true → UI 库模式显示
浏览器模式缓存里的图；断开 `@import` 接线 → 两个导入表都不加载；`resolveImportURL`
原样返回 → 服务器收到 `/imp/page/theme.css`（#box = 999px）而不是 `/imp/css/theme.css`。
**注意第一项的价值**：它证明「只测最终像素」不够——`loadExternalResource` 取字节时
也会解析相对 URL，所以禁掉图片侧的解析后「图片能加载」的断言照样通过；真正被
约束的是**缓存键必须是规范化后的绝对 URL**，为此补了跨文档串味测试。

### 有意保留的边界

- **模式不可热切换**：装配阶段要按模式决定 5 处注入（fetch 版本 / XHR / 浏览器
  全局 / 子框架 / 外部资源通道），中途切换会留下「页面脚本已 feature-detect 过
  旧能力」的不一致状态 → 装配后 `SetMode` 只接受同值，否则 `ErrModeLocked`
  （要另一模式请新建 WebView；多形态共存靠「每个 WebView 一个模式」）。
- **浏览器全局的裁剪是「真删除」**：`jsc.JSObject.Delete` 已接出 goja 的属性
  删除，UI 库模式下 `"Worker" in window` 与 `typeof Worker` 同时为假——靠 `in`
  做 feature detect 的库不再误判（属性不可配置时才退回「置 undefined」）。
- **html/body 背景传播到画布**（CSS-BACKGROUNDS-3 §2.11.2）：html 没有背景而
  body 有时，body 的背景被提升为画布背景——整屏铺满、随视口固定（不随页面
  滚动）。iframe 子文档按自身 viewport 铺满，不溢出到父文档画布。
  `ui.TestBodyBackgroundPropagatesToCanvas` 正向锁定该行为。
- **UI 库模式不提供安全策略**：它不加载外部资源（这是它最大的安全收益），但引擎
  本身没有 CSP / 同源检查层，模式切换不改变这一点。
- **外部资源带内存缓存，范围是「每 WebView」**：同一 URL 的外部资源只取一次
  （`webkit/resource_cache.go`：上限 128 条 / 4 MB，超出按插入顺序淘汰最旧；
  响应带 `Cache-Control: no-store` / `no-cache` 时不缓存；模式门禁在缓存查询
  **之前**，否则别的 WebView 的缓存会穿透模式承诺）。缓存不跨 WebView 共享
  ——宿主 `ResourceResolver` 的内容随宿主状态而变，换 resolver 时整体清空
  （`WebView.ClearResourceCache`）。图片的**解码结果**另有一层进程级缓存
  （见下条），因此重复引用同一图片常常连字节都不再取。
- **图片是异步取回的（当帧不画）**：`<img>` 的字节在后台 goroutine 取回并解码，
  命中缓存后的**下一帧**才绘制（渲染线程不被网络阻塞）。宿主按帧渲染即可；
  `data:` URL 与宿主 `ResourceResolver` 提供的内容同步命中，无此延迟。
- **图片缓存是进程级全局的**：`rendering.backgroundImageCache` 按**规范化后的
  绝对 URL** 索引（多 WebView 共享已解码图片，省内存但内容也共享）。模式门禁
  优先于缓存判定，因此 UI 库模式不会显示外部图片；但同一模式下的多个 WebView
  之间仍会共享同名资源的字节。
- **`LoadHTML` 默认没有文档基准**：相对路径按宿主进程工作目录读取（与
  `<link>` 的既有行为一致）。需要浏览器语义时三条路：`LoadURL`（引擎取内容）、
  `LoadHTMLWithBaseURL(src, base)`（**只给基准、不取内容、不联网**，装配前就经
  `page.Frame.SetPendingDocumentURL` 把 URL 写进文档——否则 `<link>`/`<script>`
  在装配中途加载时还没有基准），或让宿主 `ResourceResolver` 提供内容。
- **`page.CachedResourceLoader` 仍无调用方**：它带着 `documentURL` 与相对 URL
  解析能力，但整条链路（`LoadStylesheet` / `RequestResource` 的异步回调）没有
  接到渲染/样式管线——当前图片与样式都走 `Frame` 的同步 loader 通道。
  `webkit/resource_cache.go` 现在按 WebView 提供「取一次 + 去重」的内存缓存，
  但那是 webkit 层的资源字节缓存，与 `CachedResourceLoader` 的异步加载链路仍
  是两套——收敛属后续架构题。
- **MIME：图片不看，`<link>`/`<script src>` 按 nosniff 看**：取内容层
  （`fetchHTTP`）现在带回 `Content-Type` / `X-Content-Type-Options` /
  `Cache-Control`，按**用途在消费端**判定（这正是浏览器的位置）：图片解码成功
  即采用（解码失败才算加载失败）；样式表要求 `text/css`、脚本要求 JS MIME
  类型，且**仅在响应带 `X-Content-Type-Options: nosniff` 时**严格拒绝，无
  nosniff 时宽松接受（与浏览器一致，仅控制台提示）。
- **导航与历史已接通**（`webkit/navigation.go`）：`location.assign` / `replace` /
  `reload` 与 `location.href` 赋值真的换文档（相对引用按文档基准解析），
  `history.back/forward/go` 能跨文档遍历（遍历**不追加**条目）；装配期由页面
  脚本发起的导航**排队到装配结束后执行**，不在装配中途重入换文档；宿主
  `LoadURL` 与 location 导航都进历史栈（`history.length` 反映文档数）。UI 库
  模式拒绝导航，宿主可用 `SetOnNavigationBlocked` 感知。
- **样式表内 `url()` 的基准 = 样式表自身 URL（已实现）**：CSS Values 3 §4.4
  规定样式表里的 `url()` 在**解析时**即相对样式表自身解析（与 `@import` 同一
  规则）——`/css/theme.css` 里的 `background-image:url(bg.png)` 请求
  `/css/bg.png`；而内联 `<style>` 与元素 `style` 属性里的相对 `url()` 相对
  **文档** URL（内联样式的 base 就是文档的 base）；绝对引用 / 协议相对
  （`//cdn/x.png`）/ `data:` / 宿主逻辑名（`app://…`）一律原样保留。
  实现：`collectedDecl.sheetBase` 随声明带上来源样式表（收集链路的最后一参：
  `collectSheetDeclarations` / `collectFromStyleRule` / `collectDeclarations` /
  `collectScopedFromRules` / `collectPseudoDeclarations{,FromRule}` /
  `collectScrollbarFromRule`，由 `sheetBaseURL(sheet)` 提供，内联表为 ""），
  `ResolveElement` / `ResolvePseudoElement` 在级联排序前调
  `absolutizeCollectedURLs` 绝对化。★ 绝对化在 **token 层**重写 `url()`
  （`absolutizeDeclURLs`），因此 `background-image` / `mask-image` /
  `list-style-image` / `content` / `border-image-source` / 简写 `background` /
  自定义属性里的 `url()` 全部通道一次覆盖，将来新增的读 URL 属性也自动受益。
  `@keyframes` 的声明不在级联链路上，由 `addKeyframesFromSheet` 按来源表就地
  绝对化（幂等）。`@import` 继续用 `resolveURLAgainst`（同一函数的通用形式）。
  回归资产：`style/url_base_test.go`（6 项：外部表逐属性 / 简写与多层 /
  内联保持文档基准 / 绝对引用原样 / `@keyframes` / `@import` 链），
  `webkit/browser_http_media_test.go`（`TestBrowserModeStylesheetURLBaseForCSSURLs`、
  `TestBrowserModeDocumentBaseForInlineURLs`——文档与样式表刻意放在**不同深度**
  的目录，否则两种基准会算出同一个 URL 而抓不到回归，这是反向验证暴露的
  fixture 陷阱），`dev/probes/browser_http_probe` 的「样式表内 url() 的基准」4 条断言。
  反向验证：把 `sheetBaseURL` 改成恒返回 "" → `webkit` 两条测试立即失败
  （请求落到文档同级、`#bg` 像素由红变蓝），恢复后通过。
- **WebKit 前缀属性与标准属性同义（已实现）**：`prefixedPropertyAliases` +
  `unprefixPropertyName` 在 `applyDeclaration` 入口归一前缀名。此前
  `-webkit-mask-image` / `-webkit-mask-size` / `-webkit-transform` /
  `-webkit-animation` / `-webkit-background-clip` 等只是被存进 `Properties` 的
  陌生键，没有任何消费点读它们——**mask 图片通道对前缀写法完全失效**（挂件
  模板与老页面大量使用前缀）。刻意不收录语义不同的前缀（`-webkit-box-orient`
  / `-webkit-line-clamp` / `-webkit-appearance` / `-webkit-gradient(…)` 旧渐变
  语法 / 本引擎已有专门实现的 `-webkit-text-stroke*`）——机械映射会产生
  「看起来生效但语义不同」的错误结果，比不支持更糟。回归：
  `style/url_base_test.go:TestPrefixedPropertyAliases`（反向验证：让归一恒
  原样返回 → 5 条断言失败）。
- **多层 `background-image` 取第一层（已实现）**：`parseBackgroundURL` 用
  `strings.IndexByte(inner, ')')` 而不是 `LastIndex`——`url(a.png), url(b.png)`
  是常见写法，`LastIndex` 会得到 `a.png), url(b.png` 这种垃圾 URL，连第一层都
  加载不出来（多层叠加未实现，但第一层必须正确）。带引号形式按引号配对截断，
  避免引号内的 `)` 提前结束。回归：
  `rendering/backgroundurl_layers_test.go`（反向验证：换回 `LastIndex` → 2 条
  断言失败）。
- **`ui` 包不做声明式/响应式**：没有虚拟 DOM、没有 diff、没有响应式绑定——它是
  「Go 操作引擎 DOM 的便利 API + 双源（native/web）组件注册表」。需要声明式
  响应式时走 web 方式（Vue 等在页面脚本里做）。
