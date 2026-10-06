# C-P4 跨界优化实测报告（JS ↔ Go 边界 + 事件派发聚合）

> 生成：2026-10-08（C-P4 收口轮）。原始基准日志在 `_temp/`（不入库），本文件把**结论与数据**纳入版本控制，
> 保证干净 checkout 后可追溯、可复核。复现命令见文末。
>
> 定位：`docs/implementation-path.md` §4.5 的 C-P4 行；TECH_DEBT 的 C-P2 节末留有提醒
> 「C-P3 之前应先做 C-P4 的跨界优化，否则换后端的提速会被跨界开销吃掉」——本轮的实测与改造即回应它。

## 1｜为什么先做跨界优化（问题陈述）

换 JS 引擎（V8）能把**纯计算**提速 7×–232×，但 **V8 的跨界反而比 goja 慢 7.4×–240×**
（`docs/implementation-path.md` §4.5：`obj.Set` 67×、`obj.Get` 240×、JS→Go 回调 7.4×）。

⇒ 页面代码真正跑得快、还是把时间花在「JS 与宿主来回过界」，决定了换后端的收益。
因此 C-P4 的目标不是「让 goja 变成 V8」，而是**把跨界次数与单次跨境成本降下来**，
让两条引擎路线都受益。

## 2｜基线：单次跨界成本（T2）

`engine/js/bindings/jsboundary_bench_test.go`（同机、`-benchmem`）：

| 基准 | 数值 | 说明 |
|---|---|---|
| JS→Go 空回调 | 108.6 ns/op | 一次 `el.addEventListener` 回调入 Go 的最小成本 |
| JS→Go 读参 + 返回 | 141.5 ns/op | 带 2 个参数与返回值 |
| Go→JS 调用 | 196.6 ns/op | 宿主回调进 JS 函数 |
| Go `obj.Get` | 12.1 ns/op | 单属性读 |

**读法**：跨界单次是 **百纳秒级**。按 1 万次/帧估算即 1–2 ms/帧 —— 够用但经不起「每个 DOM 操作
过界 N 次」的写法；这正是 styleProxy 路径（Vue patchStyle、编辑器逐属性读写）要解决的问题。

## 3｜styleProxy 批量属性读写（T3/T5/T6）

### 3.1 做了什么

1. **同值快速路径**：`setProperty` 写入与现值相同的声明时不再走「解析 → 改切片 → 重新序列化属性文本」
   全流程（Vue 的 `patchStyle` 每帧重复写同一批值，绝大部分是同值写）。
2. **写路径副本**：`setStyleDecl` 命中同名声明时原位改共享的解析结果切片 —— 写路径必须先克隆，
   否则解析缓存被写脏（属性文本没变、缓存内容已变）。
3. **句柄缓存（T5）**：`el.style` 现在返回**稳定实例**（浏览器语义 `el.style === el.style`），
   按元素 + 解释器缓存（`styleObjectCache`，跨解释器不复用 goja 对象，避免
   `Illegal runtime transition`；随文档切换 `clearNodeCache` 一并清理）。

### 3.2 实测对比（`engine/js/bindings/styleproxy_bench_test.go`，before → after）

| 基准 | before ns/op | after ns/op | 变化 | allocs before → after |
|---|---|---|---|---|
| SetPropertyDotted | 686 531 | **106 393** | **−84.5%** | 6 628 → 928 |
| SetPropertyNoop（同值写） | 386 938 | **79 123** | **−79.6%** | 4 428 → 628 |
| GetPropertyValue | 609 588 | **415 324** | **−31.9%** | 7 628 → 4 828 |
| SetPropertyMethod | 283 799 | **176 584** | **−37.8%** | 3 628 → 2 228 |
| CssTextRead | 194 364 | 199 688 | 持平（±3%） | 2 628 → 2 628 |
| HandleAccess（`el.style` 取句柄） | 15 938 | 16 399 | 持平（±3%） | 1 832 / 28 → 不变 |

**诚实说明**：

- 前四项是真实收益（最热的是同值写与点号写，正是 Vue patchStyle 的形状），分配次数降 5–7× 是
  收益的主因（少了声明切片重建与属性文本序列化）。
- **`HandleAccess` 持平**：取句柄本身不是瓶颈 —— 单次 ~160 ns，开销在「属性访问分派 + goja 动态对象
  属性读」而非「创建 CSSStyleDeclaration 对象」。句柄缓存仍然要做（浏览器语义要求稳定实例，
  且它把后续 `style.width` 这类访问的目标对象固定下来），但它**不是**性能主因，不夸大。
- `CssTextRead` 持平符合预期：读 `cssText` 必须每次序列化声明列表。

### 3.3 语义回归保护

`engine/js/bindings/styleproxy_cache_test.go`（3 条）：

- `el.style === el.style` 且两个句柄共享状态（浏览器语义）；
- 读 → 写 → 再读必须看到新值、`setAttribute('style')` 文本正确、同值写不改动任何东西、
  `removeProperty` / `delete` 后 `length` 与文本同步；
- 同值写与改值写都**保序**（`cssText` 顺序不变）。

## 4｜事件派发聚合（T4）

### 4.1 两个独立开销点

1. **每次移动做两次命中测试**：`Interaction.MouseMove` 先 `HitTest` 做 hover 追踪，
   随后 `dispatchMouse` 又 `HitTest` 一次同一坐标 —— 鼠标移动是最高频交互事件，命中测试要遍历整棵
   渲染树。
2. **事件批内逐条处理移动**：系统会重发 `WM_MOUSEMOVE`，拖拽时一轮 `PollEvents` 常收到一串移动；
   逐条走完整管线（命中测试 + hover 样式重算 + 冒泡派发 + 渲染脏标记）而中间坐标页面根本观察不到。

### 4.2 改造

- **引擎侧**（`webkit/interact.go`）：`dispatchMouseAt(..., target, known)` —— 已知命中结果时跳过
  重复 `HitTest`（Move 路径复用 hover 追踪刚算出的元素）；新增 `MouseMoveBatched` / `FlushMoves`
  与 WebView 层的 `HandleMouseMoveBatched` / `FlushMouseMoves`，供裸 WebView 宿主做批内合并。
- **宿主侧**（`app/input_coalesce.go` + `app/host.go`）：`coalesceCursorMoves` 把事件批里**连续**的
  移动合并为段内最后一条（即 UI Events 的 coalesced events 语义），按键/滚轮等事件不改顺序，
  输入切片不被改写（`PollEvents` 的返回值可能被窗口层复用）。

**语义边界**：只丢「下一条也是移动」的移动 ⇒ 段内最后一条必然保留、位置语义不变；
`HandleMouseMove`（单条路径）行为完全不变；未 `Flush` 时不派发（宿主必须成对使用）。

### 4.3 测试

- `app/input_coalesce_test.go`：6 组用例（空批/单条/连续三条只留最后/按键打断/非移动保序/两段各留一条）
  + 不改写入参。
- `webkit/interact_move_test.go`：批内 3 条移动只派发 1 次且坐标为批内最后一次（JS 侧与引擎侧都断言）、
  重复 `Flush` 不重复派发、单条路径逐条派发、hover 序列（`enter-a` → `leave-a` → `enter-b`）在命中
  复用后仍正确、空白处移动仍到 `document`。

## 5｜本轮未做/待办（诚实记录）

- **`HandleAccess` 未提升**（见 3.2）——不打算再优化：取句柄不是热点，继续压它属于优化错目标。
- **`GetPropertyValue` 仍有 4 828 allocs**：读路径每次会构造返回字符串与补白名单属性，
  下一步可做「按需补白名单」或复用小切片，属 C-P4 剩余（未做）。
- **webkit 包仍有 3 个基线失败测试（非本轮引入，已用干净 HEAD 判定）**：
  `TestButtonTextVerticalCenter`、`TestCM6RangeMeasurementMatchesSkia`、`TestCheckedStateInvalidatesStyle`
  在「隔离全部未跟踪文件 + 暂存全部已改文件」的 HEAD 基线上**同样失败** ⇒ 与本轮改动无关，
  本轮不掩盖、不顺手改（各自属于渲染度量与表单状态失效链，见 TECH_DEBT）。

## 5.1｜顺带修好的两处真实缺陷（多 WebView 媒体上下文）

收尾时 `webkit/devicescale_test.go`（`TestDeviceScaleFactor`，B3/S3 的 DSF 行为判据）暴露了两个
**与测试顺序无关也不该存在**的缺陷，均为跨 WebView/跨路径的上下文串台，已修并验证：

1. **`matchMedia` 读到别的 WebView 的像素比**（`engine/js/bindings/dom.go` + `webkit/webview.go`）：
   `MediaQueryContextProvider` 是包级单例，多 WebView 时只能指向其中之一（原实现甚至遍历
   `webviews` map 取**任意一个**，顺序不确定）。新增 `MediaQueryContextForInterpreter` 按解释器
   归属解析（与 `DevicePixelRatioForInterpreter` 同一套路，遵循「闭包不捕获 wv、按归属解析」的
   既有教训），matchMedia 优先用它、Provider 仅作兜底。
   实测：修复前两个 WebView 同存时，被测 WebView 的 DSF 变化完全不反映到 `matchMedia`
   （读到另一个 WebView 的 dpr），且与其自身 `getComputedStyle` 结论**自相矛盾**。
2. **`@media (min-resolution: …)` 在样式解析里永不匹配**（`engine/style/resolver.go` +
   `webkit/webview.go`）：`Resolver.mediaQueryCtx` 只由 `SetViewportSize`（宽高）与
   `SetMediaQueryContext` 更新，**DSF 从未进入** ⇒ `@media (min-resolution: 2dppx)` 恒不命中
   （`matchMedia` 已 true 而 computed color 停在基础规则）。新增
   `Resolver.SetDevicePixelRatio`（只改 dpr，不用整体替换，避免覆盖 frame 权威的视口尺寸），
   并把媒体查询上下文收敛成**单一来源** `WebView.mediaQueryContext()`：bridge（matchMedia）与
   resolver（样式解析）共用一份，加载时与 `SetDeviceScaleFactor` 时各同步一次。

验证：`go test ./webkit/ -run TestDeviceScaleFactor` 通过；全套 `go test ./...`（排除
`dev/suites/consistency`，它依赖本机 Edge 会挂起）后，webkit 包只剩上面 3 个基线失败。

## 6｜复现命令（CGO_ENABLED=1）

```bash
export CGO_ENABLED=1 PATH="/f/syproject/goskia/bin:$PATH" GOWORK=off
# T3/T5 基准（before 需回退 engine/js/bindings/{dom,lazyelement}.go 到 HEAD）
go test ./engine/js/bindings/ -run '^$' -bench 'BenchmarkStyle' -benchmem -benchtime=300ms
# 语义回归
go test ./engine/js/bindings/ -run 'TestStyle' -count=1
# T4
go test ./app/ -run 'TestCoalesceCursorMoves' -count=1
go test ./webkit/ -run 'TestInteractionMouseMove|TestInteractionHoverEventsWithHitReuse' -count=1
# 全包
go test ./engine/js/bindings/ ./app/ -count=1
```
