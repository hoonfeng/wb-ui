# Tasks

## Phase 0：仓库基础设施

* [x] Task 0.1: 初始化 Go module `wb-ui`（go.mod / go.sum）

  * [x] SubTask 0.1.1: 创建 `go.mod`，模块名 `wb-ui`，Go 版本 1.22+

  * [x] SubTask 0.1.2: 创建顶层 README（仅一行说明，避免文件膨胀）

  * [x] SubTask 0.1.3: 创建 `.editorconfig` / `.gitignore` / `Makefile`（build/test/vet 目标）

* [x] Task 0.2: 建立顶层目录骨架

  * [x] SubTask 0.2.1: 创建 `wtf/ bmalloc/ dom/ html/ css/ style/ layout/ rendering/ editing/ page/ platform/ bindings/ jsc/ webkit/ gpu/ examples/ cmd/ internal/` 目录，每个目录放一个 `doc.go`

* [x] Task 0.3: 翻译进度跟踪脚本

  * [x] SubTask 0.3.1: `cmd/translation-progress/main.go`：扫描所有 .go 文件头，输出每个文件对应的 WebKit 源路径与完成度

## Phase 1：WTF 基础容器与抽象

* [x] Task 1.1: 翻译 `Source/WTF/wtf/` 核心容器

  * [x] SubTask 1.1.1: `wtf/vector.go` ← `Vector.h`（基于 Go slice 封装，保留 grow/shrink 语义）

  * [x] SubTask 1.1.2: `wtf/hashmap.go` ← `HashMap.h`（基于 Go map，封装 key hash trait）

  * [x] SubTask 1.1.3: `wtf/hashset.go` ← `HashSet.h`

  * [x] SubTask 1.1.4: `wtf/option.go` ← `Optional<T>` → Go 内建 `*T` 或 `Option[T]` 泛型

* [x] Task 1.2: 智能指针翻译

  * [x] SubTask 1.2.1: `wtf/ref.go` ← `Ref<T>` → Go 强引用（直接指针 + finalizer）

  * [x] SubTask 1.2.2: `wtf/refptr.go` ← `RefPtr<T>` → Go 普通指针

  * [x] SubTask 1.2.3: `wtf/weakptr.go` ← `WeakPtr<T>` → 基于 `runtime.SetFinalizer` + 弱引用 map

* [x] Task 1.3: 字符串抽象

  * [x] SubTask 1.3.1: `wtf/string.go` ← `WTFString` → 基于 Go string + UTF-16 视图

  * [x] SubTask 1.3.2: `wtf/atomstring.go` ← `AtomString` → intern table

* [x] Task 1.4: 编译器/平台抽象

  * [x] SubTask 1.4.1: `wtf/compiler.go` ← `Compiler.h`（属性宏映射到 Go build tags）

  * [x] SubTask 1.4.2: `wtf/platform.go` ← `Platform.h`（GOOS/GOARCH 判断）

## Phase 2：bmalloc 与内存

* [x] Task 2.1: `bmalloc/isoheap.go` ← `IsoHeap` 的 Go 等价物

  * [x] SubTask 2.1.1: 按 type 分桶的 slab allocator（Go 端可用 `sync.Pool` + 类型断言）

  * [x] SubTask 2.1.2: 提供 `bmalloc.Alloc[T any]() *T` / `Free[T any](ptr)` API

## Phase 3：DOM

* [x] Task 3.1: 节点基类与继承结构

  * [x] SubTask 3.1.1: `dom/node.go` ← `Node.h`（Go 接口 + 组合替代 C++ 多继承）

  * [x] SubTask 3.1.2: `dom/element.go` ← `Element.h`

  * [x] SubTask 3.1.3: `dom/document.go` ← `Document.h`

  * [x] SubTask 3.1.4: `dom/text.go` ← `Text.h` / `dom/comment.go` ← `Comment.h`

* [x] Task 3.2: 事件系统

  * [x] SubTask 3.2.1: `dom/event.go` ← `Event.h`

  * [x] SubTask 3.2.2: `dom/eventtarget.go` ← `EventTarget.h`

  * [x] SubTask 3.2.3: `dom/mouseevent.go` / `keyboardevent.go` / `wheelevent.go` / `focusevent.go`

  * [x] SubTask 3.2.4: 事件捕获/冒泡/默认行为分发器

* [x] Task 3.3: 选区与遍历

  * [x] SubTask 3.3.1: `dom/range.go` ← `Range.h`

  * [x] SubTask 3.3.2: `dom/treewalker.go` ← `TreeWalker.h`

  * [x] SubTask 3.3.3: `dom/nodeiterator.go` ← `NodeIterator.h`

## Phase 4：HTML5 解析器

* [ ] Task 4.1: Tokenizer

  * [ ] SubTask 4.1.1: `html/tokenizer.go` ← `HTMLTokenizer.cpp`（state machine: data/tag-open/tag-name/attr/etc.）

* [ ] Task 4.2: Tree Builder

  * [ ] SubTask 4.2.1: `html/treebuilder.go` ← `HTMLTreeBuilder.cpp`（insertion modes）

  * [ ] SubTask 4.2.2: 实现 adoption agency algorithm

  * [ ] SubTask 4.2.3: 实现 foster parenting

* [ ] Task 4.3: Fragment 解析与 DocumentParser

  * [ ] SubTask 4.3.1: `html/parser.go` 顶层入口 `Parse(string) *Document`

  * [ ] SubTask 4.3.2: `html/fragment.go` ← `HTMLDocumentParser::parseFragment`

## Phase 5：CSS 引擎

* [ ] Task 5.1: Tokenizer

  * [ ] SubTask 5.1.1: `css/tokenizer.go` ← `CSSTokenizer.cpp`

* [ ] Task 5.2: Parser

  * [ ] SubTask 5.2.1: `css/parser.go` ← `CSSParser.cpp`（stylesheet/rule/declaration/value）

  * [ ] SubTask 5.2.2: `css/stylesheet.go` ← `StyleSheet.h` + `CSSStyleSheet.h`

  * [ ] SubTask 5.2.3: `css/rule.go` ← `StyleRule.h`

* [ ] Task 5.3: Selector

  * [ ] SubTask 5.3.1: `css/selector.go` ← `CSSSelector.h`（type/id/class/attr/pseudo/class combinator）

  * [ ] SubTask 5.3.2: `css/selectorchecker.go` ← `SelectorChecker.cpp`

  * [ ] SubTask 5.3.3: specificity 计算

* [ ] Task 5.4: Cascade + Computed Style

  * [ ] SubTask 5.4.1: `style/resolver.go` ← `StyleResolver.cpp`

  * [ ] SubTask 5.4.2: `style/computedstyle.go` ← `ComputedStyle`

## Phase 6：Layout 引擎

* [x] Task 6.1: 基础 Layout 接口

  * [x] SubTask 6.1.1: `layout/formattingcontext.go` ← `FormattingContext.h`

  * [x] SubTask 6.1.2: `layout/layoutstate.go` ← `LayoutState.h`

* [x] Task 6.2: Block Layout

  * [x] SubTask 6.2.1: `layout/blockformattingcontext.go` ← `BlockFormattingContext.cpp`

  * [x] SubTask 6.2.2: margin collapse

* [x] Task 6.3: Inline Layout

  * [x] SubTask 6.3.1: `layout/inlineformattingcontext.go` ← `InlineFormattingContext.cpp`

  * [x] SubTask 6.3.2: line box + bidi

* [x] Task 6.4: Flex Layout

  * [x] SubTask 6.4.1: `layout/flexformattingcontext.go` ← `RenderFlexibleBox.cpp` 的 layoutBlock 部分

* [x] Task 6.5: Grid Layout

  * [x] SubTask 6.5.1: `layout/gridformattingcontext.go` ← `RenderGrid.cpp` + `GridTracksizingAlgorithm.cpp`

* [x] Task 6.6: Table + Float + Positioned

  * [x] SubTask 6.6.1: `layout/tablelayout.go`

  * [x] SubTask 6.6.2: `layout/float.go`

  * [x] SubTask 6.6.3: `layout/positioned.go`

## Phase 7：Rendering 树

* [x] Task 7.1: RenderObject 基础

  * [x] SubTask 7.1.1: `rendering/renderobject.go` ← `RenderObject.h/.cpp`

  * [x] SubTask 7.1.2: `rendering/renderbox.go` ← `RenderBox`

  * [x] SubTask 7.1.3: `rendering/renderblock.go` / `renderblockflow.go`

  * [x] SubTask 7.1.4: `rendering/renderinline.go` / `rendertext.go`

* [x] Task 7.2: RenderTreeBuilder + Updater

  * [x] SubTask 7.2.1: `rendering/rendertreebuilder.go` ← `RenderTreeBuilder.cpp`

  * [x] SubTask 7.2.2: `rendering/rendertreeupdater.go` ← `RenderTreeUpdater.cpp`

* [x] Task 7.3: Layer 系统

  * [x] SubTask 7.3.1: `rendering/renderlayer.go` ← `RenderLayer.cpp`

  * [x] SubTask 7.3.2: `rendering/renderlayercompositor.go` ← `RenderLayerCompositor.cpp`

  * [x] SubTask 7.3.3: `rendering/renderlayerbacking.go` ← `RenderLayerBacking.cpp`

## Phase 8：Painting 管线

* [x] Task 8.1: PaintInfo + Painter

  * [x] SubTask 8.1.1: `rendering/paintinfo.go` ← `PaintInfo.h`

  * [x] SubTask 8.1.2: `rendering/painter.go` ← `BorderPainter` / `BackgroundPainter` / `OutlinePainter` / `TextPainter` / `TextBoxPainter`

* [x] Task 8.2: Canvas 抽象

  * [x] SubTask 8.2.1: `platform/graphics/canvas.go` ← `GraphicsContext`

  * [x] SubTask 8.2.2: `platform/graphics/skia.go` ← Skia 桥接（cgo 调用 Skia）

* [x] Task 8.3: RenderPipeline

  * [x] SubTask 8.3.1: `rendering/renderpipeline.go` ← `RenderView::paint` + `FrameView::paint`

## Phase 9：Page / Frame / Settings

* [x] Task 9.1: Page 与 Frame

  * [x] SubTask 9.1.1: `page/page.go` ← `Page.h`

  * [x] SubTask 9.1.2: `page/frame.go` ← `Frame.h`

  * [x] SubTask 9.1.3: `page/frameview.go` ← `FrameView.h`

* [x] Task 9.2: Settings

  * [x] SubTask 9.2.1: `page/settings.go` ← `Settings.h`

## Phase 10：JavaScriptCore（jsc）

* [x] Task 10.1: Lexer

  * [x] SubTask 10.1.1: `jsc/lexer.go` ← `Source/JavaScriptCore/parser/Lexer.cpp`

* [x] Task 10.2: Parser

  * [x] SubTask 10.2.1: `jsc/parser.go` ← `Parser.cpp`（AST 生成）

* [x] Task 10.3: Bytecode + Interpreter

  * [x] SubTask 10.3.1: `jsc/bytecode.go` ← `BytecodeGenerator.cpp`

  * [x] SubTask 10.3.2: `jsc/interpreter.go` ← `Interpreter.cpp`（暂不实现 JIT）

* [x] Task 10.4: Runtime 对象模型

  * [x] SubTask 10.4.1: `jsc/object.go` ← `JSObject` / `JSValue`

  * [x] SubTask 10.4.2: `jsc/globalobject.go` ← `JSGlobalObject`

## Phase 11：bindings（Go↔JS↔DOM）

* [x] Task 11.1: Go→JS 桥

  * [x] SubTask 11.1.1: `bindings/go2js.go`：Go 函数注册为 JS 全局对象方法（原名 go\_to\_js.go，因 GOOS=js 隐式构建约束改名）

  * [x] SubTask 11.1.2: Go 值→JSValue 转换器（ToJSValue）

* [x] Task 11.2: JS→Go 桥

  * [x] SubTask 11.2.1: `bindings/js_to_go.go`：JSValue→Go 值转换器（FromJSValue）

* [x] Task 11.3: DOM bindings

  * [x] SubTask 11.3.1: `bindings/dom.go` + `bindings/dom_events.go`：把 DOM API 暴露给 JS（document.getElementById / createElement / addEventListener / dispatchEvent 等）

## Phase 12：webkit（WebView 入口）

* [x] Task 12.1: WebView

  * [x] SubTask 12.1.1: `webkit/webview.go` ← `WebView.h` 顶层 API

  * [x] SubTask 12.1.2: `webkit/webframe.go` ← `WebFrame`

* [x] Task 12.2: 进程模型

  * [x] SubTask 12.2.1: `webkit/process.go`：UI/WebContent/Network 三进程抽象（基于 Go goroutine + channel）

## Phase 13：集成与示例

* [x] Task 13.1: 最小浏览器示例

  * [x] SubTask 13.1.1: `examples/minibrowser/main.go`：加载 URL → 渲染 → 响应点击

* [x] Task 13.2: Go+HTML+CSS+JS 混合示例

  * [x] SubTask 13.2.1: `examples/mixed/main.go` + `examples/mixed/index.html` + `examples/mixed/app.js` + `examples/mixed/style.css`

  * [x] SubTask 13.2.2: 在 HTML 中调用 `go.XXX()`，在 Go 中 `document.GetElementByID(...)`

## Phase 14：测试与对照

* [x] Task 14.1: 单元测试覆盖

  * [x] SubTask 14.1.1: `html/parser_test.go` 跑 WPT HTML 解析子集

  * [x] SubTask 14.1.2: `css/selector_test.go` 跑 WPT CSS selector 子集

* [ ] Task 14.2: 与 WebKit 行为对照

  * [ ] SubTask 14.2.1: `cmd/compare-webkit/main.go`：渲染同一 HTML，对照 GWui 已有 PaintSim 期望输出

# Task Dependencies

* Phase 1 (WTF) 阻塞所有后续

* Phase 2 (bmalloc) 仅被 Phase 7+ 使用

* Phase 3 (DOM) 依赖 Phase 1；被 Phase 4/5/11 依赖

* Phase 4 (HTML) 依赖 Phase 3

* Phase 5 (CSS) 依赖 Phase 3

* Phase 6 (Layout) 依赖 Phase 5

* Phase 7 (Rendering) 依赖 Phase 6

* Phase 8 (Painting) 依赖 Phase 7 + Phase 9 (platform)

* Phase 9 (Page) 依赖 Phase 7

* Phase 10 (jsc) 仅依赖 Phase 1；可并行

* Phase 11 (bindings) 依赖 Phase 3 + Phase 10

* Phase 12 (webkit) 依赖 Phase 9 + Phase 11

* Phase 13 (Examples) 依赖 Phase 12

* Phase 14 (Testing) 与各 Phase 同步推进

## Phase 15：Skia 绘制后端集成

* [x] Task 15.1: 接入 goskia（Skia cgo 绑定）

  * [x] SubTask 15.1.1: `go.mod` 添加 `github.com/hoonfeng/goskia` 依赖，通过 `go.work` 工作区解析

  * [x] SubTask 15.1.2: `platform/graphics/canvas.go` 重写为 Skia 后端（Surface/Canvas/Paint/Font）

  * [x] SubTask 15.1.3: 像素缓存机制（`invalidatePixels` / `ensurePixels` via `ReadPixels`）

  * [x] SubTask 15.1.4: 字体缓存（`fontKey` → `*skia.Font`），真实字体渲染

  * [x] SubTask 15.1.5: `FontAscent` 方法用于将文本框顶部转换为 Skia baseline 坐标

* [x] Task 15.2: 测试适配 Skia 行为

  * [x] SubTask 15.2.1: `platform/graphics/canvas_test.go` 适配 AA / baseline / 像素快照语义（10 测试通过）

  * [x] SubTask 15.2.2: `rendering/painter_test.go` 文本测试改为扫描区域查找非透明像素

  * [x] SubTask 15.2.3: `rendering/renderpipeline_test.go` 文本测试改为扫描区域查找非透明像素

* [x] Task 15.3: 全项目验证

  * [x] SubTask 15.3.1: `go build ./...` 全项目编译通过

  * [x] SubTask 15.3.2: `go test ./...` 全部包测试通过（bindings/bmalloc/css/dom/html/jsc/layout/page/platform/graphics/rendering/style/webkit/wtf）

### Skia 集成注意事项

* 运行时需将 `f:\syproject\goskia\skia\lib\windows_amd64` 加入 PATH（`libSkiaSharp.dll`）

* `CGO_ENABLED=1` 必须开启

* Skia `DrawText` 的 y 坐标是 baseline 而非 top-left，`PaintText` 通过 `FontAscent` 转换

* 像素读取通过 `Surface.Snapshot().ReadPixels()` 获取 RGBA 数据并缓存

## Phase 16：窗口系统与 IME 输入法支持

* [x] Task 16.1: GLFW 窗口系统（`platform/window/`）

  * [x] SubTask 16.1.1: `window.go` 创建 GLFW 窗口 + OpenGL 上下文 + Skia GPU Surface（`NewGPUSurfaceFromFBO`）

  * [x] SubTask 16.1.2: 事件收集（鼠标/键盘/resize/close 回调）

  * [x] SubTask 16.1.3: `Display(srcImg)` 将 CPU Canvas 的 Snapshot blit 到 GPU Surface + SwapBuffers

  * [x] SubTask 16.1.4: 修复 `stencilBits` 参数（8→0，与 GWui 一致，解决窗口空白问题）

* [x] Task 16.2: 翻译 WebKit IME 架构（`editing/` + `dom/`）

  * [x] SubTask 16.2.1: `editing/compositionunderline.go` ← `CompositionUnderline.h` + `CompositionHighlight.h`

  * [x] SubTask 16.2.2: `editing/editorclient.go` ← `EditorClient.h`（IME 相关子集：SetInputMethodState / HandleKeyboardEvent / HandleInputMethodKeydown / DiscardedComposition / CanceledComposition / DidUpdateComposition）

  * [x] SubTask 16.2.3: `editing/editor.go` ← `Editor.h/cpp`（composition 状态管理：HasComposition / SetComposition / ConfirmComposition / CancelComposition / CompositionText / CompositionRange）

  * [x] SubTask 16.2.4: `dom/compositionevent.go` ← `CompositionEvent.h/cpp`（compositionstart / compositionupdate / compositionend DOM 事件）

* [x] Task 16.3: 平台 IME 处理器（`platform/ime/`）

  * [x] SubTask 16.3.1: `ime.go` 跨平台 Handler 接口（Init / PopEvents / SetCompositionPos / SetEnabled / IsComposing）+ Event 类型（CompositionUpdate / CharInput / CompositionEnd）

  * [x] SubTask 16.3.2: `ime_windows.go` Windows Imm32 实现：子类化 HWND（SetWindowLongW GWL\_WNDPROC）拦截 WM\_IME\_SETCONTEXT / WM\_IME\_STARTCOMPOSITION / WM\_IME\_COMPOSITION / WM\_IME\_ENDCOMPOSITION / WM\_CHAR；ImmGetContext / ImmGetCompositionStringW 读取组合/结果字符串；ImmSetCompositionWindow / ImmSetCandidateWindow 设置位置；清除 ISC\_SHOWUICOMPOSITIONWINDOW 阻止 IME 自绘组合窗口

  * [x] SubTask 16.3.3: `ime_other.go` 非 Windows 平台桩实现

* [x] Task 16.4: 窗口集成 IME

  * [x] SubTask 16.4.1: `window/ime_windows.go` 从 GLFW 获取 HWND（`GetWin32Window`）传入 IME handler

  * [x] SubTask 16.4.2: `window/ime_other.go` 非 Windows 平台返回 hwnd=0

  * [x] SubTask 16.4.3: `window.go` 添加 `IME()` / `PollIMEEvents()` / `SetIMECompositionPos(cssX, cssY)` / `SetIMEEnabled(bool)` 方法

* [x] Task 16.5: 综合测试示例

  * [x] SubTask 16.5.1: `examples/comprehensive_test/main.go` 添加 IME 输入框测试区域

  * [x] SubTask 16.5.2: 主循环处理 IME 事件（compositionupdate → 拼音预览更新、charinput → 字符输入、compositionend → 组合结束）

  * [x] SubTask 16.5.3: 焦点追踪（点击输入框 → 启用 IME + 设置组合窗口位置，点击其他 → 禁用 IME）

* [x] Task 16.6: 全项目验证

  * [x] SubTask 16.6.1: `go build ./...` 编译通过

  * [x] SubTask 16.6.2: `go test ./...` 全部包测试通过

### IME 集成注意事项

* Windows IME 通过子类化 GLFW 窗口的 Win32 HWND 实现（GLFW 不暴露 WM\_IME\_\* 消息）

* `stencilBits` 必须为 0（非 8），否则 GPU Surface 创建异常导致窗口空白

* IME 事件通过 Win32 回调线程缓冲，主循环通过 `PollIMEEvents()` 取出处理

* `GCS_RESULTSTR` 确认后会跟发重复 `WM_CHAR`，通过 `skipChars` 计数器跳过

* `ISC_SHOWUICOMPOSITIONWINDOW` 被清除以阻止 IME 自绘组合窗口（由渲染管线自绘）

