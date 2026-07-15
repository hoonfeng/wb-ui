# Checklist

## 仓库基础设施

- [ ] `go.mod` 存在，模块名 `wb-ui`，Go 版本 ≥ 1.22
- [ ] 顶层目录结构与 spec 中 "What Changes" 列表一致
- [ ] 每个目录有 `doc.go` 包含包注释
- [ ] `Makefile` 包含 `build`、`test`、`vet` 目标
- [ ] `cmd/translation-progress/main.go` 能运行并输出每个文件的翻译元信息

## 模块文件 ↔ WebKit 源 1:1 映射

- [ ] 每个 `rendering/*.go` 文件头包含 `// Translation of: Source/WebCore/rendering/XXX.{h,cpp}` 注释
- [x] 每个 `dom/*.go` 文件头包含对应 WebKit 路径
- [ ] 每个 `css/*.go` / `style/*.go` 文件头包含对应 WebKit 路径
- [ ] 每个 `layout/*.go` 文件头包含对应 WebKit 路径
- [ ] `jsc/*.go` 文件头包含 `Source/JavaScriptCore/...` 路径

## HTML5 解析

- [ ] `html.Parse` 能解析包含 DOCTYPE / html / head / body 的完整文档
- [ ] Tokenizer 覆盖 HTML5 spec 80+ 状态
- [ ] Tree builder 实现 adoption agency algorithm（验证用例：`<p><b>...</p></b>` 等）
- [ ] Tree builder 实现 foster parenting（table 内的 form 等场景）
- [ ] `html.ParseFragment` 能返回可挂载节点

## CSS 引擎

- [ ] Tokenizer 处理 identifier/number/string/url/function/at-keyword 等所有 token
- [ ] Parser 能解析 @media / @keyframes / @font-face / @import
- [ ] Selector 支持 `:not()` / `:nth-child()` / `:nth-of-type()` / `::before` / `::after` / `:hover` / `:focus`
- [ ] SelectorChecker 正确处理组合子（descendant / child / adjacent sibling / general sibling）
- [ ] Specificity 元组 (a,b,c) 计算正确
- [ ] Cascade 按 origin/importance/specificity/order 排序
- [ ] ComputedStyle 包含 CSS 变量 var() 解析后的最终值

## 布局引擎

- [ ] Block formatting context 正确处理 margin collapse
- [ ] Inline formatting context 处理 line box + 多种 white-space
- [ ] Flex 完整 6 阶段算法（resolveFlexibleLengths / relayoutCrossAxis 等）
- [ ] Flex `align-items: stretch` 脏标记传播到所有后代
- [ ] Grid auto-placement 支持 row-dense / column-dense
- [ ] Grid track sizing 包含 fr 分布 + minmax() + auto-fill / auto-fit
- [ ] Table 实现折叠边框模型
- [ ] Float + clearance 处理 left/right/both
- [ ] position:absolute / fixed / sticky / relative 全部生效
- [ ] Containing block 计算覆盖所有 spec 规定场景

## Rendering 树

- [ ] RenderObject 接口提供 `Layout()` / `Paint()` / `LocalPoint()` 等核心方法
- [ ] RenderTreeBuilder 接受 DOM 变更产生增量 RenderTree 更新
- [ ] RenderLayer 树正确反映 z-index / opacity / transform 创建的层叠上下文
- [ ] RenderLayerCompositor 决策合成层提升（video / canvas / transform / opacity 等 reasons）
- [ ] HitTest 穿透 transform 与 opacity 层

## Painting 管线

- [ ] PaintInfo 包含 phase（block background / float / foreground / outline）
- [ ] BorderPainter 8 种 border-style 全部实现（solid/dashed/dotted/double/groove/ridge/inset/outset）
- [ ] BackgroundPainter 支持 background-image / background-size / repeat / position
- [ ] TextPainter 处理 text-decoration / text-transform / text-overflow:ellipsis
- [ ] Canvas 抽象不绑定具体后端（Skia 可替换为软件光栅化）
- [ ] Skia 桥接正确加载并绘制文本/矩形/路径/图片

## Page / Frame

- [ ] Page 持有 Settings / Frame 列表
- [ ] Frame 关联 DOMWindow / Document / RenderView
- [ ] FrameView 触发 layout 与 paint 调度

## JavaScriptCore

- [x] Lexer 能正确 tokenize ES2020 示例代码（含 async/await / arrow / template literal）
- [x] Parser 生成 AST（Program / FunctionDeclaration / ExpressionStatement 等）
- [x] BytecodeGenerator 产生可被 Interpreter 执行的字节码
- [x] Interpreter 执行简单的 `let x = 1+2; console.log(x);` 并输出 `3`
- [x] GlobalObject 暴露 `console.log` / `Math` / `JSON` / `Array` 等基本全局
- [x] Go 函数注册后可在 JS 中调用：`go.XXX(...)`
- [x] JS 函数可在 Go 中调用：`jsRuntime.Call("myFunc", ...)`

## bindings（Go↔JS↔DOM）

- [x] Go 调用 `document.GetElementByID("foo").SetInnerHTML("<b>hi</b>")` 能更新 DOM
- [ ] DOM 变更触发 RenderTree dirty 标记，下次 paint 时正确反映（依赖 Phase 7 渲染树，跨阶段）
- [x] JS 调用 `document.getElementById("foo").innerHTML` 能取到最新值
- [x] 事件触发后，Go 端 `AddEventListener` 注册的回调能被调用
- [x] JS 端 `addEventListener("click", fn)` 注册的回调能被调用

## webkit 入口

- [ ] `webkit.NewWebView()` 返回可用的 WebView 实例
- [ ] `webview.LoadHTML(string)` 能加载并渲染 HTML
- [ ] `webview.LoadURL(url)` 能从本地文件或网络加载
- [ ] `webview.OnClick(cb)` 等事件回调能正常路由

## 进程模型

- [ ] UI / WebContent / Network 三进程抽象在 Go 中通过 goroutine + channel 表达
- [ ] WebContent 进程崩溃后 UI 进程能感知并展示错误页

## 示例

- [ ] `examples/minibrowser` 能打开一个 URL 并渲染
- [ ] `examples/mixed` 同时演示 Go 调用 JS、JS 调用 Go、HTML+CSS+JS 混合

## 测试对照

- [ ] 至少 10 个 WPT HTML 解析测试用例通过
- [ ] 至少 10 个 WPT CSS selector 测试用例通过
- [ ] 与 GWui 已有 PaintSim 期望输出对比，至少 30 个布局/绘制测试一致

## 翻译可追溯性

- [ ] 任意 Go 文件头包含 `// Translation of: ...` 注释
- [ ] 任意 Go 文件头包含 `// Completeness: NN%` 进度
- [ ] 简化点列在 `// Simplifications:` 下
- [ ] `cmd/translation-progress` 输出的总完成度统计能反映当前真实进度
