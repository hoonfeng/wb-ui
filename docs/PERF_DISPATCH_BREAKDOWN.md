# 滚动「同步派发」成本拆解（第10轮 必做2 → 监督者第1次指令 A 分支定稿）

**最终结论（一句话）**：滚动 1.8s 成本的 **89.62% 是引擎侧的 `RenderView.BoxContentSize`
重复全树递归**（CM6 的 scroll handler 每轮读 `scrollHeight/clientHeight` → 几何桥
`getElementScrollMetrics` → `forceLayout` + 无缓存的全子树遍历）。**不是 goja 纯 JS 执行量**。
加缓存后同端口径 **N=1：2006.8ms → 33.2ms（↓60×）**，**指标1 达成**（阈值 ≤1300ms）。

> ## ⚠️ 对本文档前一版结论的纠正（必读）
>
> 前一版结论为「1.65s ≈ 1100 万次 JS 操作 = CM6 handler 在 goja 上的执行量，引擎侧无对应
> 可削减点」。**该结论错误，已被本轮实测推翻**，原因有二：
> 1. **API 级插桩覆盖不到几何桥**：名单插桩只覆盖 `JSObject.Set` / `SetAccessor` 注册的
>    DOM API；而 `scrollHeight/clientHeight/scrollWidth/clientWidth` 走的是 webkit 的
>    `wvBridge` Go 闭包（`getElementScrollMetrics`），**不在注册表里** → 摘要为 `apiCallN=16 /
>    apiSum≈1ms`，是**假阴性**。
> 2. **差额推断不是实测**：把 `listenerTime − apiSum` 当作 "goja JS" 属于估算跳跃。
>    按监督者要求改用 **CPU profile 实测滚动窗口**（见 §3），真因即现形。
> 监督者的判断（"更像是某种随 DOM 规模退化的重复计算，而非与滚动处理量成正比的纯 JS"）
> **成立**。

---

## 1. 口径与工具

- **端到端口径**（监督者指定）：一次 `sc.scrollTop = v` 赋值 **+ 其触发的帧边界 flush 中那 1 次
  `DispatchEvent(scroll)`**（含 CM6 handler 全程）。`dev/probes/gouide_real_e2e -interaction editorE2E`，
  Go 侧打点「赋值 → `wv.Render()`」，同一次运行内先测「仅 `Render()`」基线。
- **插桩**（`WB_PERF_DISPATCH=1` 开启；**默认关闭零开销**）：
  - `engine/dom/eventtarget.go`：派发分段计时 + listener 回调计时 + DOM 变更计数；并提供
    `PerfAPIScope{Reset,Enter,Exit,Dump}` 钩子（dom 不依赖 jsc，用函数变量注入）。
  - `engine/js/jsc/perfapi.go`（新增）：**API 级计数/计时**，包裹点是 binding 层的**两个统一注册
    入口** `JSObject.Set`（方法）与 `JSObject.SetAccessor`（属性读写）——**不逐调用点改插桩**；
    嵌套时只有最外层计入耗时（避免重复计同一段时间）。
  - `dev/probes/gouide_real_e2e -interaction scrollProf`（新增）：**pprof 采样窗口只覆盖 3 轮
    N=1 滚动**（打开编辑器之后再 `StartCPUProfile`、3 轮结束即 `StopCPUProfile`），使 top 帧
    直接回答「listener 里那 ~1.8s 花在什么代码上」。
  - `-interaction jsbenchEditor` / `editorE2E` 内的 `runEditorDOMBench`（新增）：在**编辑器激活后
    的 DOM** 上重跑基准（首页 DOM 数字不代表编辑器场景，见 §5）。

## 2. 一级分解：名单内 DOM API 在 listener 内的调用（6 轮一致）

| 轮次 | listenerTime | API 调用数 | API 累计 | 占比 |
|---|---|---|---|---|
| 1 | 1.9068s | 16 | 1.0541ms | 0.06% |
| 2 | 1.7492s | 16 | 1.029ms | 0.06% |
| 3 | 1.8204s | 16 | 515.5µs | 0.03% |
| 4 | 1.8009s | 16 | 511.4µs | 0.03% |
| 5 | 1.6440s | 16 | 1.5358ms | 0.09% |
| 6 | 1.8301s | 16 | 1.053ms | 0.06% |

top2 仅 `getComputedStyle`（n=15）、`getSelection`（n=1）；`getBoundingClientRect` /
`offsetHeight` / `querySelector` / `style` / `elementFromPoint` 在 handler 内**零调用**。

**这张表本身不足以支持「引擎侧无削减空间」** —— 它是**假阴性**（§0 说明：几何桥不在名单注册表里）。
它的正确用途是：证明「常规 DOM API 面」不是成本来源，从而把注意力推向几何桥。

## 3. 二级分解：CPU profile 实测（**决定性证据**）

`-interaction scrollProf`，采样窗口 = 仅 3 轮 N=1 滚动，`Duration 5.85s / Total samples 5.78s`：

| 帧 | flat | cum | 说明 |
|---|---|---|---|
| **`rendering.(*RenderView).BoxContentSize.func1`** | **76.82%** | 89.62% | 全子树递归 walk |
| `rendering.(*renderObjectBase).FirstChild` / `NextSibling` | 3.63% / 3.29% | — | 递归遍历本身 |
| `rendering.asRenderBox` / `overflowClipsContentStyle` | 1.90% / 1.90% | — | 每盒类型/样式判定 |
| `webkit.(*WebView).injectRenderTreeBridge.func7` | 0.01s | **89.62%** | **= `getElementScrollMetrics`（webview.go:1814）** |
| `main.evalStr`（JS 求值） | — | **1.73%** | goja 执行仅 1.73% |
| GC 合计 | ~5% | — | — |

- `func7` 的 cum **5.18s ≈ 三轮 listenerTime 合计 5.4s** —— 时间归属闭合。
- `getElementScrollMetrics` 内部：`forceLayout()`（≈6%）+ `rv.BoxContentSize(box)`（≈83%）+
  `Vertical/HorizontalScrollbarMetrics`。

## 4. 根因

`RenderView.BoxContentSize`（`engine/rendering/renderview.go:638`）：

1. **无缓存**：每次调用都对 box 的**整棵子树**做递归 walk（遍历 `FirstChild/NextSibling`、
   逐盒读 `frame`/`Style`/`Segments`）。
2. **重复递归**：对每个 `overflow` 子盒还会**再次调用自身**（原 679 行
   `if iw, ih := v.BoxContentSize(cb); ...`），同一子树被反复遍历 → 最坏 O(n·depth) 级退化。
3. **触发面**：CM6 的 scroll handler 每轮滚动会**连续读** `scrollHeight` / `clientHeight` /
   `scrollWidth` / `clientWidth`；每次读都经 webkit 几何桥 → `forceLayout()` + 一次全量递归。
   4 次读 × 每次全树 = 1.8s（94 行 CM6 DOM + 整棵渲染树，1774 个节点）。
   → 这正是「随 DOM 规模退化的重复计算」，与滚动处理量（N=1 与 N=12 耗时几乎相同）无关。

## 5. 优化与同端口径对照

**改动**（`engine/rendering/renderview.go`）：新增 `boxContentSizeCache map[*RenderBox][2]float64`
与 `InvalidateContentSizeCache()`；`BoxContentSize` 变为「缓存命中 O(1) / 未命中才递归」的包装，
原实现体改名 `boxContentSizeUncached`（内部对 overflow 子盒的递归调用仍走缓存版，一次遍历即把
沿途所有子盒结果填入缓存）。**失效点**：`syncGeometry()`（每次布局后的几何刷新）与
`ApplyTextChange()`（文本段原地更新）；渲染树重建会整体换新 `RenderView`，缓存随旧对象丢弃，
不存在跨重建的陈旧项。滚动偏移变化不影响内容尺寸，故不失效。

### 5.1 端到端对照（`-interaction editorE2E`，同一二进制口径）

| 口径 | 优化前 | 优化后（第 1 次） | 优化后（第 2 次独立复现） |
|---|---|---|---|
| **N=1 端到端** | **2006.8ms**（净 1975.8） | **33.2ms**（净 0.0） | **34.3ms** |
| **N=12 端到端** | **2021.4ms**（净 1990.4） | **33.7ms**（净 0.6） | **35.6ms** |
| `[DISP]` listenerTime（6 轮） | 1.64 ~ 1.91s | 1.01 ~ 3.52ms | — |
| `layouts` / `treeRebuilds` / `domOps` | 0 / 0 / 0 | 0 / 0 / 0 | 0 / 0 / 0 |

**指标1：达成**（阈值 ≤1300ms → 实测 33.2ms，余量 ~39×）。

### 5.2 语义等价证据（缓存未返回陈旧值）

| 检查 | 结果 |
|---|---|
| 编辑器 DOM bench 返回值（优化前 → 优化后） | `offsetHeight(.cm-scroller)` 5790000 → **5790000**；`scrollTop` 11398000 → **11398000**；`clientHeight` 5790000 → **5790000**；`offsetWidth(.cm-line)` 6480000 → **6480000**（逐项一致） |
| `clientHeight 1e4` 基准 | 1041.4ms → **6.5ms**（↓160×，正是缓存命中路径） |
| 精准回归 | `engine/rendering` `engine/page` `engine/style` `engine/dom` `webkit` **全 ok**（含 `content_size_perf_test`、`scrollbar_resident_test`、`scroll_chaining_test` 等钉死用例） |

## 6. 编辑器 DOM 上的基准（**替代此前首页 DOM 数字**）

`-interaction jsbenchEditor` / `editorE2E` 内置，DOM 取证：`allNodes=1774, cmLines=94, cmScroller=1`
（首页 DOM 仅 405 节点、`.cm-scroller` 不存在 —— 再次说明首页数字不能代表编辑器场景）。

| 基准 | 编辑器 DOM（优化前） | 编辑器 DOM（优化后） | 单位成本 |
|---|---|---|---|
| 空循环 1e6（goja 基线） | 149 ~ 153ms | 129 ~ 134ms | ~140ns/迭代 |
| `offsetHeight` 1e4 (.cm-scroller) | 33.3ms | 21.3ms | 2.1µs |
| `offsetHeight` 1e4 (.cm-line) | 19.9ms | 18.8ms | 1.9µs |
| `scrollTop` 读 1e4 | 5.7ms | 6.5ms | 0.65µs |
| **`clientHeight` 1e4 (.cm-scroller)** | **1041.4ms** | **6.5ms** | 0.65µs（**本次修复直接受益**） |
| `getBoundingClientRect` 1e3 | 4.6 ~ 5.3ms | 4.4 ~ 4.9ms | ~5µs |
| `getClientRects` 1e3 | 5.8 ~ 15.7ms | 6.0ms | ~6µs |
| `getComputedStyle` 1e3 | 51.4ms | 46.9ms | ~47µs |
| **`querySelector` 1e4** | 1981 ~ 2257ms | 2024ms | **~200µs** |
| **`querySelectorAll` 1e3** | 383 ~ 436ms | 380ms | **~380µs** |
| **`elementFromPoint` 1e3** | 166.8 ~ 203.1ms | 169.0 ~ 164.4ms | **~165µs** |

**独立发现（不在指标1路径上，列为后续项）**：编辑器 DOM 上 `querySelector`（~200µs/次）、
`querySelectorAll`（~380µs/次）、`elementFromPoint`（~165µs/次）、`getComputedStyle`（~47µs/次）
相对浏览器仍有 10~100× 差距（疑为每次调用重建选择器匹配/无索引）。CM6 的 scroll handler
**不调用**这些 API（§2：handler 内相关调用为 0），故它们与本轮指标1 无关，另行收口。

### 6.1 上述四个退化 API 的收口（commit `5c66363`）

| API | 根因 | 改动 | 证据 |
|---|---|---|---|
| `querySelector` | `GetAttribute/HasAttribute` 每次 `strings.ToLower(属性名)` + map 查找；子孙遍历重复取 `nodeBaseOf` | 属性名已小写时**零分配直查**；css 属性选择器与 js bindings 查询统一走该入口；遍历复用 `nodeBaseOf` | 端到端 1e4：2978.2 → **1765.3ms（-40.7%）**；Go 紧邻对照（3000x×3 中位数）68.0 → **29.6µs（-56.5%）** |
| `querySelectorAll` | 与 `querySelector` 同一匹配入口 | 同上 | 端到端 1e3：500.4 → **447.7ms（-10.5%）**；Go 对照 144.0 → **68.3µs（-52.6%）** |
| `getComputedStyle` | 每次调用在函数内构造 **117 元素白名单切片**并对每项跑 `camelToKebab`（含分配）；`border-width` longhand 回写还每次构造 4 项 map 字面量（分配 + 顺序随机） | 白名单与 kebab 键改**包级预计算一次**（成员/顺序/三处重复项逐字保留）；回写改有序切片表 | 基准（`-benchtime=200000x`）：**14,475 ns/op / 224 allocs / 4920 B → 41 ns/op / 0 allocs**；对照实测 ≈47µs/次 → 每次省 ≈14.4µs |
| `elementFromPoint` | `HitTest` 的 Pass1（`hitTestFixedFirst`）**无条件全树 DFS**，却只对 fixed 子树内的盒子感兴趣——树里没有 `position:fixed` 时对结果零贡献（cpuprofile：占 HitTest 纯工作量的 **61%**）；另有每节点一次 `PresentedBoxScrollOffset` 的接口 key（`dom.Node`）map 查找 | **命中测试剪枝**（commit `adae53e`）：`RenderView.MayHaveFixedDescendant` 惰性探测 + 缓存，无 fixed 时跳过整趟 Pass1；滚动偏移空即短路查表；失效协议 = 树重建换新 RenderView / 就地变更调 `InvalidateHitTestProbes`（钩子已审计挂全） | 同窗口 Go 基准（编辑器形树 1200+ 节点）：**56,974 → 6,197 ns/op（-89.1%）**；miss 路径 43,347 → **566 ns/op（-98.7%）**；正确性见 `hittest_prune_test.go` |

**语义等价证据**：`engine/js/bindings/computed_style_props_test.go` 断言预计算表与优化前字面量
**逐项一致**（117 项，含 `backgroundRepeat` / `backgroundPosition` / `backgroundSize` 三处重复项），
并固化新旧固定开销对照基准；`engine/js/bindings/query_perf_test.go` 固化 `querySelector` 热路径基准。
**回归**：`go test -count=1 ./engine/{dom,js/bindings,css,style,rendering,layout}` 六包全绿
（`out/_test_after_round2.log`）。

★ **口径提醒**：探针端到端数字波动 ±11~30%，上表端到端项仅作量级参考，**以同窗口 Go 微基准
紧邻对照为准**。

## 7. 指标1 判定与建议

- **判定：达成**。端到端 N=1：**2006.8ms → 33.2/34.3ms**（同端口径、两次独立复现）；阈值 ≤1300ms。
- **此前"不换 V8 不可达"的说法作废**：该结论建立在前一版的假阴性插桩与差额估算之上；
  本轮实测表明成本在**引擎侧的重复计算**，属可修范围，且已修复。
- **§6 四个退化 API 的处置（全部收口）**：
  - `getComputedStyle`：白名单/回写表预计算（§6.1，commit `5c66363`）；
  - `elementFromPoint`：**命中测试剪枝**（commit `adae53e`）——无 `position:fixed` 的树
    跳过 Pass1 全树遍历（-89.1%）；
  - `querySelector` / `querySelectorAll`：**选择器匹配索引化**（commit `adae53e`）——
    选择器解析缓存（纯函数映射，无需失效）+ 文档级 tag/class/id **按需索引**
    （按 DOM 变更序号失效）：`querySelector` 27,090 → **48.6 ns/op（≈557×）**、
    `querySelectorAll` 74,176 → **8,465 ns/op（≈8.8×）**。
- **失效协议（索引化的前提，本轮建立）**：`dom.DOMChangeSeq()`（`engine/dom/change_seq.go`，
  atomic、只增不减），挂点 = 4 个结构 mutation 方法 + `notifyAttributeChanged`
  （`SetAttribute`/`RemoveAttribute` 的无条件钩子）。索引只依赖 tag/class/id，
  因此失效面 = 结构变更 + 属性变更，已全覆盖；**只增不减 ⇒ 宁可多失效，
  绝不返回陈旧结果**。
- **剩余可选方向（非阻塞）**：索引目前只覆盖「单 compound + 单 simple」形态
  （`.cls` / `#id` / `tag` / `*`）；组合器 / 多 compound 的查询仍走全树遍历。
  若后续在此出现热点，可扩展为「取最右 compound 的候选集 + 逐候选验证」。

## 8. 复现命令

```bash
# 端到端（含编辑器 DOM bench + [DISP]/[DISP-API] 分解表）
WB_PERF_DISPATCH=1 ./out/_api_probe3.exe -url http://127.0.0.1:9191/ -interaction editorE2E
# 仅滚动的 CPU profile（采样窗口只有 3 轮 N=1）
WB_PERF_DISPATCH=1 ./out/_api_probe3.exe -url http://127.0.0.1:9191/ -interaction scrollProf
go tool pprof -top -nodecount=25 out/_api_probe3.exe out/_scroll_only.prof
go tool pprof -list='injectRenderTreeBridge' out/_api_probe3.exe out/_scroll_only.prof
# 编辑器 DOM 基准（独立入口）
WB_PERF_DISPATCH=1 ./out/_api_probe3.exe -url http://127.0.0.1:9191/ -interaction jsbenchEditor
```

产物：`out/_api_e2e.log`（优化前）、`out/_api_e2e_after.log` / `out/_api_e2e_after2.log`
（优化后两次复现）、`out/_scroll_only.prof`（滚动窗口 profile）、`out/_api_prof2.log`。
