# WebKit-Go UI 库（wb-ui）翻译实现规范

## Why

需要一个用 Go 实现的、能"直接翻译 WebKit"的 UI 库，使 HTML/CSS/JS 与 Go 代码可以在同一运行时内混合使用。已有 `GWui` 项目翻译了 ~95% 的 WebKit 渲染层（~720K Go 代码），但缺乏完整的 HTML5 解析、CSS3 全属性、JS 引擎与 Go 互操作能力。`wb-ui` 将作为重写版本：从架构上对齐 WebKit 的源码组织，使每一份 Go 文件都能与 WebKit 中对应的 C++ 文件 1:1 映射，便于持续从上游同步修复与新特性。

## What Changes

- **新增模块**：`wtf`（基础容器/智能指针/编译器抽象），对齐 WebKit `Source/WTF/wtf`
- **新增模块**：`bmalloc`（隔离堆内存分配器），对齐 WebKit `Source/bmalloc`
- **新增模块**：`dom`（DOM 节点/事件/Range/TreeWalker），对齐 `Source/WebCore/dom`
- **新增模块**：`html`（HTML5 tokenzier + tree builder），对齐 `Source/WebCore/html`
- **新增模块**：`css`（CSS3 tokenizer + parser + selector + cascade），对齐 `Source/WebCore/css` + `Source/WebCore/style`
- **新增模块**：`layout`（block/inline/flex/grid/table/float/positioning），对齐 `Source/WebCore/layout`
- **新增模块**：`rendering`（RenderObject 树/Layer/Paint/LayoutState），对齐 `Source/WebCore/rendering`
- **新增模块**：`editing`（选区/编辑命令/输入），对齐 `Source/WebCore/editing`
- **新增模块**：`page`（Page/Frame/FrameView/Settings），对齐 `Source/WebCore/page`
- **新增模块**：`platform`（平台抽象：图形/字体/事件/线程/时间），对齐 `Source/WebCore/platform` + `Source/WebCore/PAL`
- **新增模块**：`bindings`（Go↔JS 桥接、Go↔DOM 桥接），对齐 `Source/WebCore/bindings`
- **新增模块**：`jsc`（JavaScriptCore 翻译：Lexer/Parser/Bytecode/Interpreter），对齐 `Source/JavaScriptCore`
- **新增模块**：`webkit`（WebView/WKView/多进程抽象），对齐 `Source/WebKit`
- **新增模块**：`gpu`（GPU 进程/Skia 对齐），对齐 `Source/WebCore/platform/graphics/gpu` + `Source/WebKit/GPUProcess`
- **BREAKING**：完全独立于 `GWui` 旧项目；不导入旧包，但允许复用其翻译经验与命名约定
- **BREAKING**：API 全部使用 Go idiom（首字母大写导出、接口而非多继承），但文件名与 WebKit C++ 文件保持 1:1 映射（如 `RenderBlockFlow.h/.cpp` → `renderblockflow.go`）

## Impact

- Affected specs：无（这是 `wb-ui` 项目首个 spec）
- Affected code：从空仓库开始，创建以下顶层目录结构
  - `wtf/` `bmalloc/` `dom/` `html/` `css/` `style/` `layout/` `rendering/` `editing/` `page/` `platform/` `bindings/` `jsc/` `webkit/` `gpu/` `examples/` `cmd/` `internal/`
- 受影响的参考代码：`f:\syproject\ref\WebKit\Source` 中各子目录
- 受影响的已有项目（仅作参考，不修改）：`f:\syproject\GWui`、`f:\syproject\goui`

## ADDED Requirements

### Requirement: 模块结构与 WebKit 1:1 映射
系统 SHALL 在仓库根目录下按 WebKit `Source/` 子目录建立对应的 Go 包，且每个 Go 文件名 SHALL 与对应的 WebKit C++ 头文件或实现文件同名（小写化）。

#### Scenario: 文件命名一致性
- **WHEN** 开发者查看 `rendering/renderblockflow.go`
- **THEN** 该文件 SHOULD 在 `f:\syproject\ref\WebKit\Source\WebCore\rendering\RenderBlockFlow.h` 与 `RenderBlockFlow.cpp` 找到对应源
- **AND** 文件头部 SHALL 包含 `// Translation of: Source/WebCore/rendering/RenderBlockFlow.{h,cpp}` 注释

#### Scenario: 包导入边界
- **WHEN** 模块 A 需要使用模块 B 的能力
- **THEN** A SHALL 只导入 `wb-ui/<module>` 形式的包
- **AND** 不允许出现循环导入（由 `go build` 自然保证）

### Requirement: 完整 HTML5 解析
系统 SHALL 实现 HTML5 规范定义的 tokenizer + tree constructor，支持所有 node types（element/text/comment/doctype/cdata/fragment）。

#### Scenario: 解析完整 HTML 文档
- **WHEN** 调用 `html.Parse("<!DOCTYPE html><html><head>...</head><body>...</body></html>")`
- **THEN** 返回的 `*Document` SHALL 包含完整的 DOM 树
- **AND** 解析过程 SHALL 遵循 HTML5 spec 的 adoption agency algorithm、foster parenting 等错误恢复规则

#### Scenario: Fragment 解析
- **WHEN** 调用 `html.ParseFragment("<div>hi</div>", parentElement)`
- **THEN** SHALL 返回可挂载到 `parentElement` 的节点列表

### Requirement: 完整 CSS3 引擎
系统 SHALL 实现 CSS Syntax Module Level 3 的 tokenizer + parser，CSS Values Level 3，CSS Selectors Level 4，CSS Cascade Level 4。

#### Scenario: 选择器匹配
- **WHEN** 给定选择器 `div.foo > span:not(.bar):nth-child(2n+1)`
- **THEN** selector checker SHALL 正确匹配 DOM 中所有满足条件的元素
- **AND** 计算出的 specificity 元组 SHALL 用于级联排序

#### Scenario: 级联与!important
- **WHEN** 同一元素被多条规则匹配
- **THEN** cascade SHALL 按 origin/importance/specificity/order 排序
- **AND** 计算出的 computed style SHALL 反映最终胜出的声明

### Requirement: 完整布局引擎
系统 SHALL 实现以下 layout formatting contexts：
- Block formatting context（BFC）
- Inline formatting context
- Flex formatting context（一维 flexbox，完整 6 阶段算法）
- Grid formatting context（二维，含 auto-placement + track sizing）
- Table formatting context（含 collapsed borders + auto/fixed layout）
- Float + clearance
- Positioned layout（absolute/fixed/sticky，含 containing block 计算）

#### Scenario: Flex Layout
- **WHEN** 给定 `<div style="display:flex;justify-content:space-between"><div>f1</div><div>f2</div></div>`
- **THEN** flex 容器 SHALL 计算 main-axis 起始位置使两子项均匀分布在两端
- **AND** cross-axis 默认 stretch 行为 SHALL 传播到所有后代

#### Scenario: Grid Auto-Placement
- **WHEN** 给定 `display:grid;grid-template-columns:repeat(3,1fr)` 与若干未指定 `grid-column` 的子项
- **THEN** auto-placement 算法 SHALL 按 row-dense（默认）策略填充网格

### Requirement: 完整渲染管线
系统 SHALL 实现 WebKit 风格的 RenderObject 树、RenderLayer 树、GraphicsLayer 树，并支持：
- Paint phase 分离（block background、float、foreground、outline、child outline）
- 合成层提升决策（CompositingReasons）
- 滚动区域（ScrollableArea）与滚动条
- 命中测试（HitTest）穿透合成层与 transform

#### Scenario: 命中测试
- **WHEN** 用户点击屏幕坐标 (x, y)
- **THEN** HitTest SHALL 自顶向下遍历 RenderLayer 树
- **AND** 返回最深层的 RenderObject 及其 HitTestResult

### Requirement: JavaScript 运行时（jsc）
系统 SHALL 提供一个 JavaScript 运行时，支持 ES2020+ 标准，并暴露 DOM API 与 Go 互操作 API。

#### Scenario: 脚本执行
- **WHEN** HTML 解析遇到 `<script>console.log('hi')</script>`
- **THEN** jsc SHALL 在全局执行上下文中执行该脚本
- **AND** `console.log` SHALL 路由到 Go 端注册的 logger

#### Scenario: Go↔JS 互操作
- **WHEN** Go 代码调用 `jsRuntime.Call("myFunc", 42, "str")`
- **THEN** SHALL 在 JS 全局对象上调用 `myFunc(42, "str")`
- **AND** 返回值 SHALL 转换回 Go 类型
- **WHEN** JS 代码调用 `go.ToUpper("hello")`
- **THEN** SHALL 路由到 Go 端注册的函数并返回 `"HELLO"`

### Requirement: Go 混合 API
系统 SHALL 提供 Go API 让开发者：
- 用 Go 创建/操作 DOM 节点
- 用 Go 注册事件监听器
- 用 Go 注册 JS 全局对象方法
- 用 Go 提供 `<template>` 数据绑定源

#### Scenario: Go 操作 DOM
- **WHEN** Go 代码调用 `document.GetElementByID("foo").SetInnerHTML("<b>hi</b>")`
- **THEN** DOM 树 SHALL 立即更新
- **AND** 标记 dirty 触发样式重算与布局

#### Scenario: Go 注册事件
- **WHEN** Go 代码调用 `el.AddEventListener("click", func(e *Event) {...})`
- **THEN** 该回调 SHALL 在事件分发到该元素时被调用
- **AND** 回调中可同步读取 `e.Target()` 并修改 DOM

### Requirement: 翻译可追溯性
每个翻译文件 SHALL 在头部包含元信息，记录上游源文件路径、翻译完成度、简化点列表。

#### Scenario: 文件头格式
- **WHEN** 查看任意 `rendering/*.go` 文件
- **THEN** 文件头 SHALL 包含形如下方的注释：
  ```
  // Translation of: Source/WebCore/rendering/RenderBlockFlow.h
  //                  Source/WebCore/rendering/RenderBlockFlow.cpp
  // Completeness: 95%
  // Simplifications:
  //   - line-height shrink factor uses naive 1.2
  ```

## MODIFIED Requirements

无（首版 spec）。

## REMOVED Requirements

无。
