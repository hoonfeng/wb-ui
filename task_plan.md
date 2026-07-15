# 任务计划：wb-ui 代码编辑器 + Markdown 组件实现

> 生成时间：2026-07-09
> 目标：在 wb-ui（WebKit 1:1 Go 翻译）中实现两个组件 —— 代码编辑框（参考 CodeMirror 6 / Monaco）、Markdown 渲染器（参考 markdown-it），并附带翻译 WebKit editing 子集作为底层支撑。
> 详细调研见 [findings.md](./findings.md)。

## 目标声明

实现两个 wb-ui 顶层组件包：
1. **`editor/`** — 代码编辑器组件（CM6 不可变状态模型 + Monaco 接口形态）
2. **`markdown/`** — Markdown 解析与渲染组件（markdown-it 三链 + 直接生成 wb-ui DOM 树）

附带翻译 WebKit `editing/` 子集（EditCommand/TypingCommand/VisibleSelection）作为编辑器底层。

完成后可在 `examples/comprehensive_test/main.go` 中演示：
- 一个 Go 语法高亮的代码编辑框（支持光标、选区、字符输入、撤销重做、行号）
- 一个 Markdown 渲染区域（支持标题/段落/列表/代码块/强调/链接/图片/表格，代码块共享编辑器 Tokenizer 着色）

## 决策原则

- **复用优先**：能复用 wb-ui 已有 dom/rendering/layout/editing 模块的不重写
- **翻译优先**：直接翻译 WebKit C++ 源到 Go，保持 1:1 文件映射（spec 要求）
- **增量交付**：每个 Phase 结束都有可编译/可测试的产物
- **不引入新依赖**：纯 Go + 现有 Skia cgo

---

## Phase 0：编辑器底层基础设施（翻译 WebKit editing 子集）

**目标**：为代码编辑器提供撤销/重做骨架、粒度选区。**这是 WebKit 翻译的直接迁移**，不引入新概念。

### Task 0.1: 翻译 EditCommand 体系
- [ ] 0.1.1 `editing/editcommand.go` ← `Source/WebCore/editing/EditCommand.h/.cpp`
  - `EditCommand` interface：`DoApply() error` / `DoUnapply() error` / `DoReapply() error` / `EditingAction() EditAction` / `StartingSelection() *VisibleSelection` / `EndingSelection() *VisibleSelection`
  - `SimpleEditCommand` interface：增加 `GetNodesInCommand(NodeSet)`
- [ ] 0.1.2 `editing/compositeeditcommand.go` ← `CompositeEditCommand.h/.cpp`
  - `CompositeEditCommand` struct：持有子命令列表，apply/unapply 顺序管理
  - `applyCommand(cmd)` / `append(cmd)` / `removeCommand(idx)`
- [ ] 0.1.3 `editing/editcommandcomposition.go` ← `EditCommandComposition.h/.cpp`
  - `EditCommandComposition`：CompositeEditCommand 的快照，实现 `UndoStep` 接口
- [ ] 0.1.4 `editing/editaction.go` ← `EditAction.h`
  - `EditAction` 枚举（InsertText/DeleteKey/ForwardDeleteKey/InsertParagraphSeparator 等）

### Task 0.2: 翻译 TypingCommand
- [ ] 0.2.1 `editing/typingcommand.go` ← `TypingCommand.h/.cpp`
  - `TypingCommand` struct：实现 `CompositeEditCommand`
  - `TypingCommand.Type` 枚举：DeleteSelection/DeleteKey/ForwardDeleteKey/InsertText/InsertLineBreak/InsertParagraphSeparator
  - `Option` 位掩码：SelectInsertedText/AddsToKillRing/RetainAutocorrectionIndicator/...
  - **关键方法**：`closeTyping()` / `isOpenForMoreTyping()` / `lastTypingCommandIfStillOpenForTyping()`（连续输入合并为一次撤销点）
  - 静态入口：`TypingCommand::InsertText(doc, text, options)` / `TypingCommand::DeleteSelection(doc, options)`

### Task 0.3: 翻译 VisibleSelection / VisiblePosition
- [ ] 0.3.1 `editing/visibleposition.go` ← `VisiblePosition.h`
  - `Position` struct（Container Node + Offset + PositionType）
  - `Affinity` 枚举（Upstream/Downstream）
  - `VisiblePosition` struct（Position + Affinity）
- [ ] 0.3.2 `editing/visibleselection.go` ← `VisibleSelection.h/.cpp`
  - `VisibleSelection` struct：base/extent（用户原始）/start/end（规范化）/affinity/isDirectional
  - `SelectionType` 枚举（None/Caret/Range）
  - `SelectionDirection` 枚举（Forward/Backward/Right/Left）
  - `TextGranularity` 枚举（Character/Word/Line/Paragraph/Document）
  - `validate()` 规范化流程
  - `selectionFromContentsOfNode(Node)` 工厂

### Task 0.4: 扩展现有 Editor 接口
- [ ] 0.4.1 修改 [editing/editor.go](file:///f:/syproject/wb-ui/editing/editor.go)
  - 增加 `UndoStack` / `RedoStack`
  - `ExecuteCommand(cmd EditCommand)` 入口
  - `InsertText(text string)` 委托给 TypingCommand
  - `DeleteSelection()` 委托给 TypingCommand
  - 保留现有 IME composition 方法（SetComposition/ConfirmComposition 等），内部改走 TypingCommand.InsertText

### 验证
- [ ] `go build ./editing/...` 编译通过
- [ ] 新增 `editing/editcommand_test.go`：测试撤销/重做链
- [ ] 新增 `editing/typingcommand_test.go`：测试连续字符合并为一次撤销

---

## Phase 1：代码编辑器核心模型（CM6 风格 Go 翻译）

**目标**：在 `editor/` 包实现不可变文档模型 + 选区 + 事务 + 变更集。**纯数据结构，不涉及渲染**。

### Task 1.1: 文档模型 Text
- [ ] 1.1.1 `editor/text.go` ← CM6 `src/text.ts`
  - `Text` interface：`Length() int` / `Lines() int` / `Replace(from, to, Text) Text` / `Append(Text) Text` / `Slice(from, to) Text` / `LineAt(pos) Line` / `Line(n) Line` / `String() string` / `Iter() TextIterator`
  - `TextLeaf` struct：保存 `[]string`（按行切分，每叶最多 32 行）
  - `TextNode` struct：内部节点，保存 `[]Text` 子节点
  - `Line` struct：`From/To/Length/Number/Text`
  - `TextOf(lines []string) Text` 工厂
  - **位置模型**：单一 `int` 偏移（与 CM6 一致）

### Task 1.2: 选区模型
- [ ] 1.2.1 `editor/selection.go` ← CM6 `state.ts`
  - `Range` struct：`From/To/Anchor/Head/Empty/Bound/Assoc`
  - `EditorSelection` struct：`Ranges []Range` + `Main int` + `asSingle()` + `addRange()`
  - `EditorSelection.Range(from, to)` 工厂
  - 支持多光标

### Task 1.3: 状态变更 Transaction
- [ ] 1.3.1 `editor/changeset.go` ← CM6 `changeSet.ts`
  - `ChangeDesc` struct：`FromA/ToA/InsertLength`
  - `ChangeSet` struct：`[]ChangeDesc`
  - `MapPos(pos, assoc) int` 位置映射
  - `ChangedRanges() []ChangedRange`
- [ ] 1.3.2 `editor/transaction.go` ← CM6 `transaction.ts`
  - `Transaction` struct：`Changes ChangeSet` / `Selection EditorSelection` / `Effects []StateEffect` / `Annotations` / `Reconfigured bool`
  - `StateUpdate` spec struct

### Task 1.4: 不可变状态 EditorState
- [ ] 1.4.1 `editor/state.go` ← CM6 `state.ts`
  - `EditorState` struct：`Doc Text` / `Selection EditorSelection` / `fields map[FieldID]interface{}`
  - `Update(spec StateUpdate) EditorState` 产生新状态（旧状态不变）
  - `Field(fieldID) interface{}` / `Facet(facetID) interface{}`

### Task 1.5: 扩展系统简化版
- [ ] 1.5.1 `editor/extension.go` ← CM6 `extension.ts`
  - `Extension` interface（任何值都可作为 Extension）
  - `Facet` struct（简化版：`Inputs []interface{}` + `Combine` 函数）
  - `StateField` struct：`Create(state) interface{}` / `Update(value, tr) interface{}` / `Provide(facet)`
  - 第一版**不用 Go 泛型**，用 `interface{}` + 类型断言（避免泛型复杂度）

### 验证
- [ ] `go build ./editor/...` 编译通过
- [ ] `editor/text_test.go`：测试 Text replace/append/slice/lineAt
- [ ] `editor/selection_test.go`：测试多选区
- [ ] `editor/transaction_test.go`：测试 transaction 应用 + 位置映射
- [ ] `editor/state_test.go`：测试不可变状态更新

---

## Phase 2：语法高亮 Tokenizer

**目标**：实现一个声明式词法分析器，可解析 Go/JS/Markdown 等语言。**借鉴 Monarch 声明式状态机模型**，比 Lezer 简单且适合 Go 翻译。

### Task 2.1: Tokenizer 核心
- [ ] 2.1.1 `editor/monarch.go` ← VS Code `monaco.languages.Monarch`
  - `MonarchLanguage` struct：`Tokenizer map[string][]MonarchRule` / `Start string` / `IgnoreCase bool` / `DefaultToken string`
  - `MonarchRule` struct：`Regex *regexp.Regexp` / `Action MonarchAction` / `Next string` / `Token string` / `Push string`
  - `MonarchAction` struct：`Token string` / `Next string` / `NextEmbedded string` / `Bracket string`
  - `Tokenize(state *MonarchState, src string) []Token`
- [ ] 2.1.2 `editor/monarch_state.go`
  - `MonarchState` struct：`Stack []string`（状态栈）/ `Pos int` / `Src string`
- [ ] 2.1.3 `editor/token.go`
  - `Token` struct：`StartIndex int` / `EndIndex int` / `Scopes []string`（如 `["keyword.control.go"]`）

### Task 2.2: 内置语言定义
- [ ] 2.2.1 `editor/lang_go.go` ← `vscode-go` 的 Monarch 描述（声明式 JSON 翻译到 Go）
  - 关键字、字符串、注释、数字、操作符
- [ ] 2.2.2 `editor/lang_js.go` ← VS Code `monaco-languages/typescript`
- [ ] 2.2.3 `editor/lang_markdown.go` ← `@codemirror/lang-markdown`

### Task 2.3: HighlightStyle（tag → 样式）
- [ ] 2.3.1 `editor/highlight.go` ← CM6 `@codemirror/language`
  - `HighlightStyle` struct：`map[Tag]SkiaColor`（tag → 颜色 + 字重 + 斜体）
  - 内置 `ThemeDarkPlus`（VS Code Dark+ 配色） / `ThemeLight`

### 验证
- [ ] `go build ./editor/...` 编译通过
- [ ] `editor/monarch_test.go`：用一段 Go 代码 tokenize 验证 token 范围正确
- [ ] `editor/highlight_test.go`：验证 tag → 颜色映射

---

## Phase 3：装饰系统 Decoration

**目标**：实现 CM6 Decoration 4 种类型 + DecorationSet，用于行号、当前行高亮、语法着色、折叠。

### Task 3.1: Decoration 数据结构
- [ ] 3.1.1 `editor/decoration.go` ← CM6 `view/decorations.ts`
  - `Decoration` interface：`From/To/Type`
  - `DecorationMark`：`Class string` / `Attributes map[string]string`（语法高亮 span）
  - `DecorationWidget`：`Widget WidgetType` / `Block bool`
  - `DecorationReplace`：`Widget WidgetType` / `Inclusive bool`（折叠占位）
  - `DecorationLine`：`Class string` / `Attributes map[string]string`（行号、当前行）
  - `DecorationSet` struct：`[]Decoration`（按 From 排序）
  - `Set(decos []Decoration, sort bool) DecorationSet`
  - `Map(changes ChangeSet) DecorationSet`（范围随编辑漂移）

### Task 3.2: Widget 抽象
- [ ] 3.2.1 `editor/widget.go`
  - `WidgetType` interface：`ToDOM(view *EditorView) dom.Node` / `Eq(other WidgetType) bool` / `IgnoreEvent(event) bool`
  - 第一版 widget 用 wb-ui dom.Node 而非 DOM

### 验证
- [ ] `go build ./editor/...` 编译通过
- [ ] `editor/decoration_test.go`：测试 DecorationSet 构建 + Map 漂移

---

## Phase 4：EditorView（视图层 + Skia 渲染）

**目标**：把 EditorState 同步到 Skia 画面，处理鼠标/键盘输入。**这是与 wb-ui rendering 集成的关键**。

### Task 4.1: EditorView 核心
- [ ] 4.1.1 `editor/view.go` ← CM6 `view/view.ts`
  - `EditorView` struct：`State EditorState` / `Decorations DecorationSet` / `Viewport Viewport` / `Dom *dom.Element`（在 wb-ui 中 EditorView 持有一个根 Element 用于挂载到文档）
  - `Dispatch(tr Transaction)` 应用变更
  - `ScrollToDOM()` 滚动
  - `VisibleRanges() []ViewRange`
- [ ] 4.1.2 `editor/viewport.go`
  - `Viewport` struct：`From int / To int`（可见文档区间）
  - `BlockInfo` struct：`Top/Height/Type`
  - **虚拟滚动**：只渲染 Viewport 内行 + 缓冲区

### Task 4.2: Skia 渲染管线
- [ ] 4.2.1 `editor/painter.go`
  - `PaintEditor(canvas *graphics.Canvas, view *EditorView, x, y, w, h float64)`
  - 调用 [platform/graphics/canvas.go](file:///f:/syproject/wb-ui/platform/graphics/canvas.go) 的 DrawText / FillRect / StrokeRoundRect
  - 应用 HighlightStyle 把 Token 转换为 Skia 颜色
  - 应用 Decoration 把行号/当前行高亮绘制到画面
  - 绘制光标（caret）：竖线 + 闪烁动画
  - 绘制选区：半透明蓝色矩形
- [ ] 4.2.2 `editor/layout.go`
  - 行高计算（基于 Skia FontMetrics）
  - 字符宽度测量（复用 [layout/inlineformattingcontext.go](file:///f:/syproject/wb-ui/layout/inlineformattingcontext.go) 的 MeasureTextFunc hook）
  - 行号区宽度计算（基于最大行号位数）

### Task 4.3: 输入处理
- [ ] 4.3.1 `editor/input.go`
  - `HandleClick(view, x, y) Position` — 鼠标点击转字符偏移
  - `HandleDrag(view, x, y) Position` — 拖拽选区
  - `HandleKey(view, key KeyboardEvent) Transaction` — 键盘输入生成 transaction
  - `HandleIME(view, event ime.Event) Transaction` — IME 组合事件
  - 字符偏移 → 像素坐标 转换
- [ ] 4.3.2 与 [app/host.go](file:///f:/syproject/wb-ui/app/host.go) 集成
  - `host.AttachEditor(view, x, y, w, h)` 把编辑器挂载到窗口
  - 路由鼠标/键盘/IME 事件到 EditorView
  - 编辑器内部光标绘制时调用 `host.SetIMECompositionPos()`

### Task 4.4: 命令系统
- [ ] 4.4.1 `editor/command.go` ← CM6 `commands.ts`
  - `Command = func(*EditorView) bool`
  - `KeyBinding` struct：`Key string` / `Run Command` / `PreventDefault bool` / `Shift Command`
  - `Keymap` 函数：注册多个 KeyBinding
  - 内置命令：`insertText` / `deleteCharBackward` / `deleteCharForward` / `moveCharLeft` / `moveCharRight` / `moveLineUp` / `moveLineDown` / `selectCharLeft` / ... / `undo` / `redo` / `selectAll`
  - 默认 keymap：Ctrl+C/V/X/Z/Y/A，方向键，Backspace/Delete，Enter

### Task 4.5: 撤销/重做集成
- [ ] 4.5.1 `editor/history.go` ← CM6 `@codemirror/commands` 的 `history()`
  - `History` struct：`UndoStack []TransactionGroup` / `RedoStack []TransactionGroup` / `NewGroupDelay time.Duration`
  - `TransactionGroup` struct：连续 transaction 合并（与 WebKit TypingCommand 的 openForMoreTyping 对齐）
  - 内部用 Phase 0 的 EditCommand 体系，但提供更高层 API：`History.Undo(view) bool` / `History.Redo(view) bool`

### 验证
- [ ] `go build ./editor/...` 编译通过
- [ ] `editor/view_test.go`：测试 viewport 计算
- [ ] 新增 `examples/editor_demo/main.go`：一个独立运行的代码编辑器 demo（加载一段 Go 代码，显示语法高亮、行号、光标、可输入字符、Ctrl+Z 撤销）

---

## Phase 5：Markdown 解析器（markdown-it 三链 Go 翻译）

**目标**：在 `markdown/` 包实现 markdown-it 三链（core/block/inline）+ Token 数据结构。**纯解析，不渲染**。

### Task 5.1: Token 与 Ruler
- [ ] 5.1.1 `markdown/token.go` ← markdown-it `lib/token.mjs`
  - `Token` struct：`Type string` / `Tag string` / `Attrs [][2]string` / `Map [2]int` / `Nesting int` / `Level int` / `Children []Token` / `Content string` / `Markup string` / `Info string` / `Meta interface{}` / `Block bool` / `Hidden bool`
  - `Open()` / `Close()` / `AddAttr(name, val)` / `AttrGet(name)` / `AttrSet(name, val)`
- [ ] 5.1.2 `markdown/ruler.go` ← markdown-it `lib/ruler.mjs`
  - `Ruler` struct：`rules []Rule` 有序规则集
  - `Rule` struct：`Name string` / `Fn RuleFn` / `Alt []string`
  - `Before(beforeName, ruleName, fn, alt)` / `After(afterName, ruleName, fn, alt)` / `At(atName, ruleName, fn, alt)` / `Push(ruleName, fn, alt)` / `Enable(names, ignoreInvalid)` / `Disable(names, ignoreInvalid)` / `GetRules(chainName) []RuleFn`
  - `RuleFn` 类型：`func(state interface{}, ...interface{}) bool`

### Task 5.2: StateCore / StateBlock / StateInline
- [ ] 5.2.1 `markdown/state_core.go` ← `lib/rules_core/state_core.mjs`
  - `StateCore` struct：`Src string` / `Env map[string]interface{}` / `Tokens []Token` / `Md *MarkdownIt`
- [ ] 5.2.2 `markdown/state_block.go` ← `lib/parser_block.mjs`
  - `StateBlock` struct：`Src string` / `Md *MarkdownIt` / `Env map` / `Tokens []Token` / `BMarks []int` / `EMarks []int` / `TShift []int` / `LineMax int` / `BlkIndent int` / `SCount []int` / `Indent int` / `TStart int` / `Line int` / `Level int` / `ParentType string` / `Pending []Token` / `PendingLevel int`
- [ ] 5.2.3 `markdown/state_inline.go` ← `lib/parser_inline.mjs`
  - `StateInline` struct：`Src string` / `Md *MarkdownIt` / `Env map` / `OutTokens []Token` / `Tokens []Token` / `PendingText string` / `PendingLevel int` / `Level int` / `Pos int` / `PosMax int` / `Pending []Token` / `Delimiter []Delimiter`

### Task 5.3: Core 链规则
- [ ] 5.3.1 `markdown/parser_core.go` ← `lib/parser_core.mjs`
  - `ParserCore` struct：`Ruler Ruler` / `Parse(state *StateCore)`
  - 规则：`normalize` / `block` / `inline` / `text_join` / `replacements` / `smartquotes` / `linkify` / `abbreviations`（前 4 个 P0，其余可选）

### Task 5.4: Block 链规则
- [ ] 5.4.1 `markdown/parser_block.go` ← `lib/parser_block.mjs`
  - `ParserBlock` struct：`Ruler Ruler` / `Parse(state *StateCore, startLine, endLine int) bool`
  - P0 规则：`table` / `code` / `fence` / `blockquote` / `hr` / `heading` / `lheading` / `bullet_list` / `ordered_list` / `paragraph` / `html_block` / `reference`
  - 列表项内部递归调用 block parse

### Task 5.5: Inline 链规则
- [ ] 5.5.1 `markdown/parser_inline.go` ← `lib/parser_inline.mjs`
  - `ParserInline` struct：`Ruler Ruler` / `SkipToken(state, startLine)` / `Parse(state, silent)`
  - P0 rules 链：`text` / `newline` / `escape` / `backticks` / `strikethrough` / `emphasis` / `link` / `image` / `autolink` / `html_inline` / `entity`
  - P0 rules2 链：`balance_pairs` / `emphasis_post` / `fragments_join` / `text_collapse`

### Task 5.6: 入口 MarkdownIt
- [ ] 5.6.1 `markdown/markdown.go` ← `lib/index.mjs`
  - `MarkdownIt` struct：`Config Config` / `Inline *ParserInline` / `Block *ParserBlock` / `Core *ParserCore` / `Renderer *Renderer`
  - `New(opts)` 构造（预设 `default` / `commonmark`）
  - `Parse(src, env) []Token`
  - `Render(src) string`（HTML 字符串，调试用）
  - `Use(plugin)` 插件注册

### 验证
- [ ] `go build ./markdown/...` 编译通过
- [ ] `markdown/parser_test.go`：跑 10+ 高频 CommonMark 用例（paragraph/heading/list/fence/blockquote/emphasis/link/image/horizontal_rule/lheading）

---

## Phase 6：Markdown Renderer（直接生成 wb-ui DOM 树）

**目标**：把 Token 流转换为 wb-ui `dom.Element` 子树，挂到现有 Document 上由 wb-ui 渲染管线绘制。**不经过 HTML 字符串中转**。

### Task 6.1: DOM Renderer 核心
- [ ] 6.1.1 `markdown/renderer_dom.go` ← markdown-it `lib/renderer.mjs`
  - `Renderer` struct：`Rules map[string]RenderRule` / `Render(tokens []Token, opts, env) *dom.DocumentFragment`
  - `RenderRule` 类型：`func(tokens []Token, idx int, opts Config, env, self *Renderer) dom.Node`
  - 默认规则实现：
    - `paragraph_open/close` → `<p>`
    - `heading_open/close` → `<h1>`-`<h6>`
    - `bullet_list_open/close` → `<ul>`
    - `ordered_list_open/close` → `<ol>`
    - `list_item_open/close` → `<li>`
    - `fence` → `<pre><code class="language-xxx">` + 调用 Tokenizer 着色
    - `code_block` → `<pre><code>`
    - `blockquote_open/close` → `<blockquote>`
    - `hr` → `<hr>`
    - `inline` → 递归调用 `renderInline`
    - `text` → text node
    - `em_open/close` → `<em>`
    - `strong_open/close` → `<strong>`
    - `code_inline` → `<code>`
    - `link_open/close` → `<a>`
    - `image` → `<img>`
    - `softbreak` → `\n` text
    - `hardbreak` → `<br>`
    - `html_inline` / `html_block` → 解析为 dom.Node（如果 Config.HTML=true，否则转义）

### Task 6.2: 与代码编辑器共享 Tokenizer
- [ ] 6.2.1 `markdown/fence_highlight.go`
  - `RenderFence(content, lang string, opts) []dom.Node`
  - 调用 `editor.Tokenize(content, lang)` 拿到 `[]Token`
  - 根据 `editor.HighlightStyle` 把 token 范围转换为带 `style` 属性的 `<span>`
  - 一份"tag → Skia 颜色"映射，编辑器和 Markdown 共享

### Task 6.3: 高级入口
- [ ] 6.3.1 `markdown/markdown.go` 增加 `ParseToDOM(src) (*dom.DocumentFragment, error)`
  - 内部调用 `Parse` 拿到 tokens，再调用 `Renderer.Render`
  - 输出的 DocumentFragment 可直接 `AppendChild` 到目标 Element

### Task 6.4: 内置 GFM 扩展
- [ ] 6.4.1 `markdown/gfm.go`
  - table 渲染（thead/tbody/tr/th/td）
  - strikethrough（`~~text~~`）
  - autolink
  - task_list（`- [ ]` / `- [x]`）

### 验证
- [ ] `go build ./markdown/...` 编译通过
- [ ] `markdown/renderer_test.go`：测试 paragraph→`<p>`、heading→`<h2>`、fence→`<pre><code class="language-go">`
- [ ] 新增 `examples/markdown_demo/main.go`：加载一个 markdown 文件，渲染到 wb-ui 窗口

---

## Phase 7：HTML 标签 / 自定义元素集成

**目标**：让 `<wb-editor>` 和 `<wb-markdown>` 标签在 HTML 中可用，自动实例化对应组件。

### Task 7.1: HTML 解析器扩展
- [ ] 7.1.1 修改 [html/treebuilder.go](file:///f:/syproject/wb-ui/html/treebuilder.go)
  - 增加 custom element 注册表 `var customElements = map[string]CustomElementConstructor{}`
  - `RegisterCustomElement(tag string, ctor CustomElementConstructor)`
  - `createElementForToken` 检测到注册的标签时调用对应 ctor（替代通用 `doc.CreateElement`）

### Task 7.2: 注册 wb-editor / wb-markdown
- [ ] 7.2.1 `editor/element.go`
  - `RegisterEditorElement()` 注册 `<wb-editor>`
  - 当解析到 `<wb-editor language="go" theme="dark-plus">...</wb-editor>` 时：
    - 创建 `EditorView` 实例
    - 把元素 textContent 作为初始文档
    - 把 EditorView 挂载到该 Element（替代渲染 Element 自身）
- [ ] 7.2.2 `markdown/element.go`
  - `RegisterMarkdownElement()` 注册 `<wb-markdown>`
  - 当解析到 `<wb-markdown>...</wb-markdown>` 时：
    - 把元素 textContent 作为 Markdown 源
    - 调用 `markdown.ParseToDOM(src)` 得到 DocumentFragment
    - 替换该 Element 的子节点为 fragment

### Task 7.3: JS 绑定
- [ ] 7.3.1 `editor/bindings.go`
  - 暴露给 JS：`new wb.Editor({...})` / `editor.setValue(...)` / `editor.getValue()` / `editor.onChange(cb)`
- [ ] 7.3.2 `markdown/bindings.go`
  - 暴露给 JS：`new wb.Markdown({...})` / `md.parse(src)` 返回 dom fragment / `md.render(src)` 返回 HTML 字符串

### 验证
- [ ] 修改 [examples/comprehensive_test/test.html](file:///f:/syproject/wb-ui/examples/comprehensive_test/test.html) 增加 `<wb-editor>` 和 `<wb-markdown>` 卡片
- [ ] 修改 [examples/comprehensive_test/main.go](file:///f:/syproject/wb-ui/examples/comprehensive_test/main.go) 注册自定义元素并启动

---

## Phase 8：集成与综合测试

### Task 8.1: 综合示例
- [ ] 8.1.1 `examples/comprehensive_test/test.html` 增加：
  - 代码编辑器卡片：`<wb-editor language="go" theme="dark-plus">package main\n...</wb-editor>`
  - Markdown 卡片：`<wb-markdown># 标题\n\n正文...</wb-markdown>`
- [ ] 8.1.2 修改 [examples/comprehensive_test/main.go](file:///f:/syproject/wb-ui/examples/comprehensive_test/main.go)
  - 调用 `editor.RegisterEditorElement()` + `markdown.RegisterMarkdownElement()`
  - 重新编译 `comprehensive_test.exe`

### Task 8.2: 文档
- [ ] 8.2.1 更新 `.trae/specs/translate-webkit-go-ui/tasks.md` 增加 Phase 17（编辑器+Markdown）
- [ ] 8.2.2 更新 `.trae/specs/translate-webkit-go-ui/checklist.md` 增加相应勾选项

### 验证
- [ ] `go build ./...` 全项目编译通过
- [ ] `go test ./...` 全部包测试通过
- [ ] 运行 `comprehensive_test.exe`：代码编辑器卡片可输入、显示语法高亮、行号、Ctrl+Z 撤销
- [ ] Markdown 卡片正确渲染标题/段落/列表/代码块（代码块带语法高亮）

---

## Phase 9：HTML 表单控件迁移（完整 15 元素 + 22 InputType）

**目标**：完整迁移 WebKit 表单控件体系到 wb-ui，让 `<input>` `<textarea>` `<button>` `<select>` 等真实可用，替换当前 div 模拟。**直接翻译 WebKit C++ 实现**。

### Task 9.1: 表单控件基类层次
- [ ] 9.1.1 `html/htmlelement.go` ← `Source/WebCore/html/HTMLElement.h/.cpp`
  - `HTMLElement` struct 嵌入 `dom.Element`，增加 `isContentEditable()` / `contentEditable()` / `setContentEditable()` / `ContentEditable` 枚举（True/False/PlaintextOnly/Inherit）
- [ ] 9.1.2 `html/formassociatedelement.go` ← `FormAssociatedElement.h`
  - `FormAssociated` interface：`Form() *HTMLFormElement` / `SetForm(*HTMLFormElement)` / `Name() string` / `Reset()` / `EnqueueChangeEvent()`
- [ ] 9.1.3 `html/formlistedelement.go` ← `FormListedElement.h`
  - `FormListed` interface：增加 `isLabelable()` / `isEnumeratable()`
- [ ] 9.1.4 `html/validatedformlistedelement.go` ← `ValidatedFormListedElement.h`
  - `ValidatedFormListed` interface：`Validity() *ValidityState` / `CheckValidity() bool` / `ReportValidity() bool` / `SetCustomValidity(msg)` / `WillValidate() bool` / `ValidationMessage() string`
- [ ] 9.1.5 `html/htmlformcontrolelement.go` ← `HTMLFormControlElement.h/.cpp`
  - `HTMLFormControlElement` struct：嵌入 `HTMLElement`，实现 `ValidatedFormListed`
  - 字段：`disabled` / `required` / `autofocus` / `autocomplete` / `wasChangedSinceLastFormControlChangeEvent`
  - 方法：`IsDisabled()` / `IsRequired()` / `IsSubmitButton()` / `IsActivatedSubmit()` / `DispatchFormControlChangeEvent()` / `DispatchFormControlInputEvent()`
- [ ] 9.1.6 `html/htmlformcontrolelementwithstate.go` ← `HTMLFormControlElementWithState.h`
  - 增加 `saveFormControlState()` / `restoreFormControlState()` / `shouldSaveAndRestoreFormControlState()`
- [ ] 9.1.7 `html/htmltextformcontrolelement.go` ← `HTMLTextFormControlElement.h/.cpp`
  - `HTMLTextFormControlElement` struct：嵌入 `HTMLFormControlElementWithState`
  - 方法：`SelectionStart() int` / `SelectionEnd() int` / `SelectionDirection() TextFieldSelectionDirection` / `SetSelectionRange(start, end, dir)` / `Select()`
  - `valueMatchesRenderer` 标志
  - 内部 innerText 子树管理（用作渲染源）

### Task 9.2: 15 个表单元素类
- [ ] 9.2.1 `html/htmlformelement.go` ← `HTMLFormElement.h/.cpp`
  - `Submit()` / `RequestSubmit()` / `Reset()` / `Elements() []FormListed` / `Length() int`
  - 表单数据收集：`appendFormData(DOMFormData)` 编排
  - 隐式提交：`submitIfPossible()`
- [ ] 9.2.2 `html/htmlinputelement.go` ← `HTMLInputElement.h/.cpp`
  - `HTMLInputElement` struct 嵌入 `HTMLTextFormControlElement`
  - 共享状态：`value` / `checked` / `defaultChecked` / `defaultValue` / `name` / `type` / `disabled` / `required` / `multiple` / `accept` / `alt` / `src` / `size` / `maxLength` / `minLength` / `min` / `max` / `step` / `pattern` / `placeholder` / `autocomplete` / `list`
  - 委托行为给 `InputType` 子类
- [ ] 9.2.3 `html/htmltextinputtype.go` ← `TextInputType.h` / `BaseTextInputType.h` / `TextFieldInputType.h`
  - 文本输入基类（被 EmailInputType/PasswordInputType/SearchInputType/TelephoneInputType/URLInputType 共享）
- [ ] 9.2.4 `html/inputtype.go` ← `InputType.h/.cpp`
  - `InputType` interface：`FormControlType() string` / `IsTextType() bool` / `IsTextField() bool` / `IsCheckable() bool` / `IsSteppable() bool` / `Value() string` / `SetValue(sanitized, eventBehavior)` / `SanitizeValue(val) string` / `TypeMismatch(val) bool` / `ValueMissing(val) bool` / `PatternMismatch(val) bool` / `HasBadInput(val) bool` / `IsValidValue(val) bool` / `StepUp(n)` / `StepDown(n)` / `AppendFormData(data)` / `CreateRenderer()` / `SaveFormControlState()` / `RestoreFormControlState()`
  - `InputType.Type` 枚举（22 值）
  - `createInputTypeFactoryMap()` 工厂表：`type 关键字 → 工厂函数`
- [ ] 9.2.5 `html/inputtypes.go` ← `ButtonInputType.h` / `CheckboxInputType.h` / `ColorInputType.h` / `DateInputType.h` / `DateTimeLocalInputType.h` / `EmailInputType.h` / `FileInputType.h` / `HiddenInputType.h` / `ImageInputType.h` / `MonthInputType.h` / `NumberInputType.h` / `PasswordInputType.h` / `RadioInputType.h` / `RangeInputType.h` / `ResetInputType.h` / `SearchInputType.h` / `SubmitInputType.h` / `TelephoneInputType.h` / `TimeInputType.h` / `URLInputType.h` / `WeekInputType.h`
  - 22 个 InputType 子类全部实现（可放在同一个文件，按 type 分文件太大）
- [ ] 9.2.6 `html/htmlbuttonelement.go` ← `HTMLButtonElement.h/.cpp`
  - `type` 属性（button/submit/reset/menu）/ `formAction` / `formEnctype` / `formMethod` / `formNoValidate` / `formTarget`
- [ ] 9.2.7 `html/htmltextareaelement.go` ← `HTMLTextAreaElement.h/.cpp`
  - `value` / `defaultValue` / `rows` / `cols` / `wrap` / `maxLength` / `minLength` / `placeholder` / `readOnly` / `required` / `disabled`
  - 实现 `HTMLTextFormControlElement`
- [ ] 9.2.8 `html/htmlselectelement.go` ← `HTMLSelectElement.h/.cpp`
  - `value` / `selectedIndex` / `selectedOptions` / `multiple` / `size` / `required` / `disabled` / `name` / `autocomplete`
  - `add(opt, before)` / `remove(idx)` / `item(idx)` / `namedItem(name)` / `Options()` `HTMLOptionsCollection`
- [ ] 9.2.9 `html/htmloptionelement.go` ← `HTMLOptionElement.h/.cpp`
  - `value` / `text` / `defaultSelected` / `selected` / `disabled` / `index` / `form` / `label`
- [ ] 9.2.10 `html/htmloptgroupelement.go` ← `HTMLOptGroupElement.h/.cpp`
  - `disabled` / `label`
- [ ] 9.2.11 `html/htmllabelelement.go` ← `HTMLLabelElement.h/.cpp`
  - `Control() HTMLElement` getter（for 属性解析或祖先 labelable 后代）
  - 点击 label 时触发关联控件合成 click
- [ ] 9.2.12 `html/htmlfieldsetelement.go` ← `HTMLFieldSetElement.h/.cpp`
  - `disabled` 级联禁用下属控件 / `Elements() HTMLFormControlsCollection` / `Form() *HTMLFormElement`
- [ ] 9.2.13 `html/htmllegendelement.go` ← `HTMLLegendElement.h/.cpp`
  - `Form()` / `Align()` (deprecated)
- [ ] 9.2.14 `html/htmldatalistelement.go` ← `HTMLDataListElement.h/.cpp`
  - `Options()` HTMLCollection / `settingsConditional=dataListElementEnabled`
- [ ] 9.2.15 `html/htmloutputelement.go` ← `HTMLOutputElement.h/.cpp`
  - `value` / `defaultValue` / `type="output"` / `htmlFor DOMTokenList`
- [ ] 9.2.16 `html/htmlprogresselement.go` ← `HTMLProgressElement.h/.cpp`
  - `value` / `max` / `position` / `labels`
- [ ] 9.2.17 `html/htmlmeterelement.go` ← `HTMLMeterElement.h/.cpp`
  - `value` / `min` / `max` / `low` / `high` / `optimum` / `labels`

### Task 9.3: ValidityState 与约束验证
- [ ] 9.3.1 `html/validitystate.go` ← `ValidityState.h`
  - `ValidityState` struct：`ValueMissing` / `TypeMismatch` / `PatternMismatch` / `TooLong` / `TooShort` / `RangeUnderflow` / `RangeOverflow` / `StepMismatch` / `BadInput` / `CustomError` / `Valid`
  - 脏标记机制（`wasChangedSinceLastFormControlChangeEvent`）
- [ ] 9.3.2 `html/formcontroller.go` ← `FormController.h`
  - 表单状态保存/恢复（bfcache）

### Task 9.4: DOMFormData
- [ ] 9.4.1 `html/domformdata.go` ← `DOMFormData.h/.cpp` / `DOMFormData.idl`
  - `Append(name, value)` / `Delete(name)` / `Get(name)` / `GetAll(name)` / `Has(name)` / `Set(name, value)` / `Entries()` / `Keys()` / `Values()`
  - 编码：`application/x-www-form-urlencoded` / `multipart/form-data` / `text/plain`

### Task 9.5: HTML 元素工厂
- [ ] 9.5.1 修改 [dom/document.go](file:///f:/syproject/wb-ui/dom/document.go) 的 `CreateElement(tagName)`
  - 引入 `html.ElementFactory`：`type 关键字 → 构造函数`
  - 注册 15 个表单元素 + HTMLElement（默认回退）
- [ ] 9.5.2 修改 [html/treebuilder.go](file:///f:/syproject/wb-ui/html/treebuilder.go) 的 `createElementForToken`
  - 调用 `html.ElementFactory.Create(tagName, doc)` 替代 `doc.CreateElement(tagName)`
  - `constructorNeedsCreatedByParser` 标记（input/script/style/link/audio/video/canvas 需要 parser 上下文）

### Task 9.6: 默认样式与渲染
- [ ] 9.6.1 `html/defaultcss.go` ← `Source/WebCore/css/html.css`
  - 表单控件默认样式（input/textarea/select/button/fieldset/legend/label/option/optgroup/progress/meter/datalist/output）
  - 替换当前 [style/resolver.go](file:///f:/syproject/wb-ui/style/resolver.go) 的 UA 样式表
- [ ] 9.6.2 `rendering/renderformcontrol.go` ← `Source/WebCore/rendering/RenderTextControl.{h,cpp}` / `RenderButton.h` / `RenderListBox.h` / `RenderMenuList.h`
  - 文本控件光标矩形计算（与代码编辑器的 caret 绘制复用）
  - 复选框/单选按钮绘制
  - select 下拉箭头绘制
  - range 滑块绘制
  - progress/meter 进度条绘制

### Task 9.7: 表单事件
- [ ] 9.7.1 `dom/inputevent.go` ← `Source/WebCore/dom/InputEvent.h`
  - `InputEvent` struct：`InputType string` / `Data string` / `IsComposing bool`
  - inputType 取值对应 `EditAction` 枚举（insertText/insertFromPaste/deleteContentBackward/...）
- [ ] 9.7.2 `dom/submitevent.go` ← `SubmitEvent.h`
  - `SubmitEvent` struct：`Submitter *Element` / `FormData DOMFormData`
- [ ] 9.7.3 `dom/formdataevent.go` ← `FormDataEvent.h`
- [ ] 9.7.4 修改 [editing/editor.go](file:///f:/syproject/wb-ui/editing/editor.go)
  - 输入触发 `beforeinput` / `input` 事件分发
  - 与 Phase 0 的 TypingCommand 集成

### Task 9.8: HTML 表单控件测试
- [ ] 9.8.1 `html/htmlinputelement_test.go`
  - value 状态、selectionStart/End、type 切换、checked/disabled 切换
- [ ] 9.8.2 `html/htmltextareaelement_test.go`
  - 多行 value、rows/cols、selection
- [ ] 9.8.3 `html/htmlselectelement_test.go`
  - selectedIndex、selectedOptions、add/remove
- [ ] 9.8.4 `html/htmlformelement_test.go`
  - 表单数据收集、submit/reset、约束验证
- [ ] 9.8.5 `html/validitystate_test.go`
  - 22 种 InputType 的校验矩阵测试
- [ ] 9.8.6 `html/inputtype_test.go`
  - 工厂表、type 关键字 → 类映射、UnknownInputType fallback

### 验证
- [ ] `go build ./html/...` 编译通过
- [ ] `go test ./html/...` 全部通过
- [ ] 修改 [examples/comprehensive_test/test.html](file:///f:/syproject/wb-ui/examples/comprehensive_test/test.html) 把 `#imeInput` div 替换为真实 `<input>`
- [ ] 验证输入框获得真实 `value` / `selectionStart` / `selectionEnd` API
- [ ] 验证 `<form>` submit 收集数据并触发 submit 事件

---

# 阶段依赖与执行顺序

```
Phase 0 (editing 翻译) ──┬──→ Phase 1 (编辑器核心模型) ──→ Phase 2 (Tokenizer) ──→ Phase 3 (Decoration)
                       │                          ↓
                       │                          Phase 4 (EditorView + Skia 渲染) ──→ Phase 7 (HTML 集成) ──┐
                       │                          ↓                                                     │
                       │                          Phase 8 (综合测试) ←──────────────────────────────────┘
                       │                          ↑
                       │                          Phase 6 (Markdown Renderer) ──┘
                       │                          ↑
                       │    Phase 5 (Markdown 解析，100% CommonMark) ──┘
                       │
                       └──→ Phase 9 (HTML 表单控件 15 元素 + 22 InputType)
                                  ↓
                                  Phase 8 (综合测试，与编辑器/Markdown 一起验证)
```

- **可并行**：Phase 5+6（Markdown）、Phase 9（表单控件）与 Phase 1-4（编辑器）相互独立，可同时推进
- **必须串行**：Phase 0 → {Phase 1, Phase 5, Phase 9} 三路分支
- **汇聚点**：Phase 8 综合测试，需要所有组件就位

---

# 遇到的错误

| 错误 | 尝试次数 | 解决方法 |
|------|----------|----------|
| 字段/方法同名冲突 `IsOpenForMoreTyping` | 1 | 将字段重命名为私有 `isOpenForMoreTyping`，保留公共方法 `IsOpenForMoreTyping()` |
| `Affinity_` 残留引用 | 2 | 统一改为 `Affinity` 字段名 |
| DeleteKey/ForwardDeleteKey 无法 undo | 1 | 增加 `deletedText` 字段记录被删文本，DoUnapply 重新插入 |
| 合并插入重复写入整个 TextToInsert | 1 | InsertTextStatic 合并路径改为只插入新文本，不调用 DoApply |
| EditCommandComposition 无法 undo TypingCommand | 1 | Wrap 保存原 composite 引用，DoUnapply/DoReapply 委托给原 composite |
| Go test 缺少 Skia DLL (0xc0000135) | 1 | 设置 `$env:PATH = "f:\syproject\goskia\skia\lib\windows_amd64;" + $env:PATH` |

---

# 关键决策记录

## 决策 1：编辑器模型选 CodeMirror 6 而非 Monaco
- **理由**：CM6 不可变状态/Transaction 纯数据结构，无 DOM 耦合，适合翻译到 Go + Skia
- **代价**：CM6 的 Facet 系统复杂，第一版用 `interface{}` + 类型断言简化

## 决策 2：Markdown 解析走 Token → DOM，不走 HTML 字符串
- **理由**：避免双重解析，保留 Markdown 语义，便于 TOC/跳转扩展
- **代价**：Renderer 需要自己实现 token → dom.Element 映射

## 决策 3：Tokenizer 用 Monarch 而非 Lezer
- **理由**：Lezer 需要解析生成器（.grammar → Go），复杂；Monarch 是声明式 JSON 状态机，直接翻译
- **代价**：没有增量解析树，但编辑器场景下每次重 token 整文档足够快

## 决策 4：自定义元素用 HTML parser 注册表实现
- **理由**：HTML5 custom elements 标准在 wb-ui 中尚未实现
- **方案**：扩展 [html/treebuilder.go](file:///f:/syproject/wb-ui/html/treebuilder.go) 的 `createElementForToken`，加入 Go 端注册表

## 决策 5：完整迁移 15 个 HTML 表单元素 + 22 种 InputType（用户 2026-07-09 确认）
- **理由**：用户选择完整对齐 WebKit 表单控件体系
- **范围**：Phase 9 完整迁移 HTMLInputElement/HTMLButtonElement/HTMLTextAreaElement/HTMLSelectElement/HTMLOptionElement/HTMLOptGroupElement/HTMLLabelElement/HTMLFieldSetElement/HTMLLegendElement/HTMLDataListElement/HTMLOutputElement/HTMLProgressElement/HTMLMeterElement/HTMLFormElement/HTMLSelectedContentElement 共 15 类 + 22 种 InputType + ValidityState + DOMFormData

## 决策 6：Markdown 100% CommonMark 兼容（用户 2026-07-09 确认）
- **理由**：用户要求通过全部 600+ CommonMark 测试用例
- **范围**：Phase 5 需覆盖所有 CommonMark 0.31 规范场景，包括嵌套强调、reference link、entity 解析、setext heading、lazy continuation、link reference definitions、entity 解析等 edge case
