# 调研发现：wb-ui 对比 WebKit 组件实现差距

> 生成时间：2026-07-09
> 目的：评估 wb-ui 当前已实现 / 未实现的 WebKit 组件清单，为代码编辑器 + Markdown 组件的实施提供依据。

---

## 一、wb-ui 已实现的 WebKit 模块（按目录映射）

wb-ui 是 WebKit 的 1:1 Go 翻译，目录结构与 `Source/` 一一对应。下表列出当前完成度（参考 `.trae/specs/translate-webkit-go-ui/tasks.md`、源码实际情况）：

| wb-ui 包 | WebKit 对应源 | 翻译完成度 | 关键能力 |
|---|---|---|---|
| `wtf/` | `Source/WTF/wtf` | ~95% | Vector/HashMap/HashSet/Option/Ref/RefPtr/WeakPtr/String/AtomString |
| `bmalloc/` | `Source/bmalloc` | ~80% | IsoHeap（sync.Pool 等价物） |
| `dom/` | `Source/WebCore/dom` | ~70% | Node/Element/Document/Text/Comment/Event/EventTarget/MouseEvent/KeyboardEvent/WheelEvent/FocusEvent/CompositionEvent/Range/TreeWalker/NodeIterator/DocumentFragment |
| `html/` | `Source/WebCore/html` | ~60% | HTML5 Tokenizer + TreeBuilder（adoption agency / foster parenting）+ Fragment 解析 |
| `css/` | `Source/WebCore/css` | ~75% | Tokenizer + Parser + Selector + Specificity + SelectorChecker + Cascade |
| `style/` | `Source/WebCore/style` | ~70% | Resolver + ComputedStyle |
| `layout/` | `Source/WebCore/layout` | ~85% | Block/Inline/Flex/Grid/Table/Float/Positioned formatting contexts |
| `rendering/` | `Source/WebCore/rendering` | ~80% | RenderObject/RenderBox/RenderBlockFlow/RenderInline/RenderText/RenderLayer/RenderLayerCompositor/RenderPipeline/Painter/Selection/HitTest/RenderTreeBuilder |
| `editing/` | `Source/WebCore/editing` | ~15% | **仅 IME composition 子集**：CompositionUnderline/EditorClient/Editor（HasComposition/SetComposition/ConfirmComposition/CancelComposition） |
| `page/` | `Source/WebCore/page` | ~80% | Page/Frame/FrameView/Settings |
| `platform/graphics/` | `Source/WebCore/platform/graphics` | ~85% | Skia 桥接：Canvas/Surface/Paint/Font/FontMgr/MeasureText/FontAscent |
| `platform/ime/` | `Source/WebCore/platform/ime` | ~90% | Windows Imm32 IME handler（subclass HWND、GCS_RESULTSTR 去重） |
| `platform/window/` | — | ~85% | GLFW + OpenGL + Skia GPUSurface（stencilBits=0 修复） |
| `bindings/` | `Source/WebCore/bindings` | ~70% | Go↔JS 桥（RegisterGoFunction/Call/ToJSValue/FromJSValue）+ DOM 暴露给 JS（getElementById/createElement/setAttribute/addEventListener/onclick） |
| `jsc/` | `Source/JavaScriptCore` | ~50% | Lexer/Parser/AST/Bytecode/Interpreter/GlobalObject（console.log/Math/JSON/Array 基本全局） |
| `webkit/` | `Source/WebKit` | ~70% | WebView/WebFrame/Process（UI/WebContent/Network 三进程 goroutine 抽象） |
| `gpu/` | `Source/WebCore/platform/graphics/gpu` | ~10% | 仅占位 |

---

## 二、HTML 元素 / 表单控件实现差距

**关键发现**：wb-ui 中所有 HTML 元素都使用通用 `dom.Element` 类（见 [dom/element.go](file:///f:/syproject/wb-ui/dom/element.go)），**完全没有** WebKit 中 150+ 个专用 HTML 元素 C++ 子类。

### 2.1 wb-ui 当前 HTML 元素支持情况

| 类别 | 状态 | 说明 |
|---|---|---|
| 文档结构（html/head/body/title/meta/link/base/style/script） | 通用 Element | 解析与渲染可走通，但无 `HTMLScriptElement` 异步加载、无 `HTMLLinkElement` stylesheet 加载等 |
| 文本语义（div/span/p/h1-h6/br/hr/pre/blockquote/code/em/strong 等） | 通用 Element | 渲染依赖 CSS 默认样式表 |
| 列表（ul/ol/li/dl/dt/dd） | 通用 Element | 列表项标记由 layout 处理 |
| 表格（table/thead/tbody/tr/td/th/caption/col/colgroup） | 通用 Element | layout 有 `tableformattingcontext.go` 但无 `HTMLTableElement` API |
| **表单控件（form/input/button/textarea/select/option/optgroup/label/fieldset/legend/datalist/output/progress/meter）** | **完全缺失** | 没有专用类，没有 value 状态、selection、validation、submit/reset、label[for] 关联、disabled 级联 |
| 媒体（img/video/audio/source/picture/track/map/area/canvas/svg） | **完全缺失** | 无 `HTMLImageElement`（图片不渲染）、无 Canvas 2D API、无媒体元素 |
| 链接/嵌入（a/iframe/embed/object/param） | **完全缺失** | 无 `HTMLAnchorElement`（点击无导航）、无 iframe |
| 交互（details/summary/dialog/menu） | **完全缺失** | |
| 框架（frameset/frame） | **完全缺失** | |

### 2.2 当前"输入框"是 div 模拟

参考 [examples/comprehensive_test/test.html](file:///f:/syproject/wb-ui/examples/comprehensive_test/test.html#L199)：
```html
<div id="imeInput" style="...">点击此处输入文字...</div>
```
- 用 `<div>` + CSS 模拟 input 外观
- 文本通过 `el.SetTextContent()` 在 Go 端写入
- IME 组合直接挂在 focused element 上（不区分 input/textarea）
- 没有 `value` 属性、没有 `selectionStart/End`、没有 `placeholder` 真实行为、没有 `input`/`change` 事件

### 2.3 InputType 体系完全缺失

WebKit 中 22 种 InputType 子类（text/password/number/email/url/search/tel/date/time/datetime-local/month/week/color/range/file/checkbox/radio/button/submit/reset/image/hidden）在 wb-ui 中**全部未实现**。

---

## 三、editing 包实现差距

wb-ui 当前 `editing/` 包**只翻译了 IME composition 子集**：

| WebKit C++ 类 | wb-ui 状态 |
|---|---|
| `Editor.h/cpp` | 部分翻译（HasComposition/SetComposition/ConfirmComposition/CancelComposition/CompositionText/CompositionRange） |
| `EditorClient.h` | 接口翻译（SetInputMethodState/HandleKeyboardEvent/DiscardedComposition 等） |
| `CompositionUnderline.h` | 翻译 |
| `CompositionHighlight.h` | 未翻译 |
| **`EditCommand.h/cpp`** | **未翻译**（撤销/重做骨架缺失） |
| **`SimpleEditCommand.h`** | **未翻译** |
| **`CompositeEditCommand.h`** | **未翻译** |
| **`EditCommandComposition.h`** | **未翻译**（UndoStep） |
| **`TypingCommand.h/cpp`** | **未翻译**（连续输入合并） |
| **`VisibleSelection.h/cpp`** | **未翻译**（粒度选区） |
| **`VisiblePosition.h`** | **未翻译**（Affinity） |
| **`FrameSelection.h/cpp`** | **未翻译** |
| **`markup.{cpp,h}`** | **未翻译**（剪贴板序列化） |
| `EditAction.h` | 未翻译 |
| `ReplaceSelectionCommand.h` | 未翻译 |
| `InsertTextCommand.h` | 未翻译 |

---

## 四、代码编辑器与 Markdown 调研结论（详见子报告）

### 4.1 代码编辑器选型

**结论：以 CodeMirror 6 概念模型为蓝本移植到 Go + Skia，对外 API 借鉴 Monaco 接口形态。**

理由：
1. CM6 的 `EditorState`/`Transaction`/`StateField` 是纯数据结构，无 DOM 耦合，适合翻译到 Go
2. CM6 的 `Text` B+ 树模型对应 Go slice 切片
3. CM6 的 `Decoration` 声明式描述可直接被 Skia paint 阶段消费
4. Lezer 增量解析模型对应 wb-ui 已有的 `html/parser.go`
5. CM6 位置用单一整数偏移，比 Monaco 的 `(lineNumber, column)` 更易实现

**对外 API 借鉴 Monaco**：`deltaDecorations` / `pushEditOperations` / `IModelDecorationOptions` 等接口形态，便于熟悉 VS Code 扩展的开发者上手。

### 4.2 WebKit 编辑基础设施复用

**优先级 P0**（代码编辑器必需依赖）：
- `EditCommand` / `SimpleEditCommand` / `CompositeEditCommand` / `EditCommandComposition` → `editing/editcommand.go`
- `TypingCommand`（含 `closeTyping` / `isOpenForMoreTyping`）→ `editing/typingcommand.go`
- `VisibleSelection` + `VisiblePosition` → `editing/visibleselection.go`

**优先级 P1**（与现有 IME 集成）：
- `Editor` 命令分发子集 → 扩展 `editing/editor.go`
- `EditAction` 枚举 → `editing/editaction.go`（对应 DOM InputEvent.inputType）

**优先级 P2**（剪贴板）：
- `markup.cpp` 选区→HTML 序列化 → `editing/markup.go`

### 4.3 Markdown 渲染器选型

**结论：移植 markdown-it 三链（core/block/inline）到 `markdown/` 包，Renderer 直接生成 wb-ui DOM 树（不经 HTML 字符串中转）。**

理由：
1. wb-ui 已有完整的 `dom/Document` / `dom.Element` / `html/parser.go`
2. markdown-it 的 Token 是结构化数据，可直接 `AppendChild` 到 DOM 树
3. 避免"Markdown → HTML 字符串 → html.Parse"双重解析开销
4. 保留 Markdown 语义（heading 层级、list 类型、fence 语言）便于 TOC、跳转等扩展

**与代码编辑器共享**：Markdown 的 fenced code block（` ```go `）调用代码编辑器的 Tokenizer，共享一份"tag → Skia 颜色"映射表，保证编辑器内代码与预览代码视觉一致。

### 4.4 代码编辑器核心架构（CM6 风格 Go 翻译）

| 概念 | Go 类型 | 文件位置 |
|---|---|---|
| 不可变文档 | `Text` interface + `TextLeaf` / `TextNode` | `editor/text.go` |
| 位置 | `int`（字符偏移） | — |
| 选区 | `EditorSelection` struct（多 `Range` + `main`） | `editor/selection.go` |
| 不可变状态 | `EditorState` struct（doc + selection + fields） | `editor/state.go` |
| 状态变更 | `Transaction` struct（changes + selection + effects） | `editor/transaction.go` |
| 变更集 | `ChangeSet` struct（`ChangeDesc[]`） | `editor/changeset.go` |
| 增量解析树 | `Tree` + `TreeFragment` | `editor/tree.go`（参考 Lezer） |
| 装饰 | `Decoration`（4 类：mark/widget/replace/line） + `DecorationSet` | `editor/decoration.go` |
| 命令 | `Command = func(*EditorView) bool` | `editor/command.go` |
| 扩展 | `Extension` interface + `Facet[T]` 泛型 | `editor/extension.go` |
| 视图 | `EditorView` struct（state + decorations + viewport） | `editor/view.go` |
| 虚拟滚动 | `Viewport` struct（可见 [from,to]） | `editor/viewport.go` |

### 4.5 Markdown 包结构

| 文件 | 职责 | 对应 markdown-it 源 |
|---|---|---|
| `markdown/token.go` | `Token` struct（type/tag/attrs/map/children/content/markup/info） | `lib/token.mjs` |
| `markdown/ruler.go` | `Ruler` 有序规则集（before/after/at） | `lib/ruler.mjs` |
| `markdown/state_core.go` | `StateCore`（src/md/env/tokens） | `lib/rules_core/state_core.mjs` |
| `markdown/state_block.go` | `StateBlock`（line scanning） | `lib/parser_block.mjs` |
| `markdown/state_inline.go` | `StateInline`（char scanning） | `lib/parser_inline.mjs` |
| `markdown/parser_core.go` | Core 链：normalize/block/inline/text_join | `lib/parser_core.mjs` |
| `markdown/parser_block.go` | Block 规则：heading/paragraph/blockquote/hr/list/fence/code_block/table/reference/html_block | `lib/parser_block.mjs` |
| `markdown/parser_inline.go` | Inline 规则：text/escape/backticks/emphasis/link/image/autolink/html_inline/entity | `lib/parser_inline.mjs` |
| `markdown/renderer_dom.go` | 自定义 Renderer：直接生成 `dom.Element` | `lib/renderer.mjs` |
| `markdown/markdown.go` | `MarkdownIt` struct + `Render(src) *dom.DocumentFragment` 入口 | `lib/index.mjs` |

---

## 五、对外 API 暴露计划

### 5.1 Go API（直接调用）

```go
// 代码编辑器
import "wb-ui/editor"

ed := editor.New(&editor.Config{
    Language: "go",
    Theme:    editor.ThemeDarkPlus,
})
ed.SetDoc("package main\n\nfunc main() {\n    println(\"hi\")\n}\n")
ed.OnChange(func(tr *editor.Transaction) { ... })
host.AttachEditor(ed, x, y, w, h)

// Markdown
import "wb-ui/markdown"

md := markdown.New(&markdown.Config{HTML: false, Linkify: true})
frag, _ := md.Parse("# Title\n\nHello **world**\n\n```go\npackage main\n```")
doc.Root().AppendChild(frag)
```

### 5.2 JS API（通过 bindings）

```js
// 暴露给 JS 的 Editor
const ed = new wb.Editor({ language: "go" });
ed.setValue("package main\n...");
ed.on("change", (tr) => console.log(tr.changes));

// 暴露给 JS 的 Markdown
const md = new wb.Markdown();
const frag = md.parse("# Title\n...");
document.body.appendChild(frag);
```

### 5.3 HTML 标签（浏览器风格）

```html
<wb-editor language="go" theme="dark-plus">
package main
func main() {}
</wb-editor>

<wb-markdown>
# 标题

**正文**
</wb-markdown>
```

通过 `html/treebuilder.go` 的自定义元素机制（HTML5 custom elements）注册 `wb-editor` / `wb-markdown` 标签 → 实例化对应 Go 组件。

---

## 六、迁移路径与风险

### 6.1 推荐迁移路径

**阶段 1（基础设施）** → **阶段 2（代码编辑器核心）** → **阶段 3（Markdown 渲染）** → **阶段 4（HTML 表单控件）** → **阶段 5（集成与示例）**

详见 `task_plan.md`。

### 6.2 主要风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| Go 泛型 `Facet[T]` 实现复杂 | CM6 的 Facet 是其扩展系统的核心 | 第一版用 `interface{}` + 类型断言简化，后期可改泛型 |
| Skia 长文本渲染性能 | 10K+ 行代码编辑器卡顿 | 必须实现 Viewport 虚拟滚动（CM6 模型） |
| WebKit `EditCommand` 翻译量大 | C++ 多继承在 Go 中需拆为 interface 组合 | 仅翻译 TypingCommand 子集，跳过富文本命令 |
| Markdown CommonMark 完整性 | 测试套件 600+ 用例 | 第一版只覆盖 80% 高频用例（paragraph/heading/list/code/emphasis/link/image/table） |
| HTML 表单控件迁移量大 | 22 种 InputType + 15 种表单元素类 | 仅迁移 `HTMLInputElement`(text 子集) / `HTMLTextAreaElement` / `HTMLButtonElement` 三个核心类，其余后续 |
| 与现有 IME 集成冲突 | 已有 composition 逻辑要重写 | `TypingCommand` 设计已预留 IME 接口（`InsertText` 类型） |
| 自定义元素 `<wb-editor>` 需要扩展 HTML parser | treebuilder 不识别自定义标签 | 用 attribute data-wb-component 替代，或扩展 `treebuilder.go` 的 custom element 注册表 |

---

## 七、参考实现与源码索引

### 7.1 成熟实现源码

| 项目 | 用途 | 仓库 |
|---|---|---|
| CodeMirror 6 | 编辑器核心模型蓝本 | https://github.com/codemirror/codemirror.next |
| Monaco Editor | 对外 API 形态参考 | https://github.com/microsoft/vscode/tree/main/src/vs/editor |
| Lezer | 增量解析器生成器 | https://github.com/lezer-parser/lr |
| markdown-it | Markdown 解析三链蓝本 | https://github.com/markdown-it/markdown-it |
| markdown-it-py | 跨语言移植先例 | https://github.com/markdown-it/markdown-it-py |

### 7.2 WebKit 参考源（本地 `f:\syproject\ref\WebKit\Source`）

- `WebCore/html/HTMLTagNames.in` — 完整 HTML 元素清单
- `WebCore/html/InputType.cpp` — 22 种 InputType 工厂
- `WebCore/html/HTMLFormControlElement.h` — 表单控件基类
- `WebCore/html/HTMLTextFormControlElement.h` — 文本控件共用基类
- `WebCore/editing/EditCommand.h` — 编辑命令基类
- `WebCore/editing/CompositeEditCommand.h` — 复合命令
- `WebCore/editing/TypingCommand.h` — 输入命令
- `WebCore/editing/VisibleSelection.h` — 可见选区
- `WebCore/editing/FrameSelection.cpp` — Frame 级选区
- `WebCore/editing/Editor.h` — 编辑器总入口
- `WebCore/editing/markup.{cpp,h}` — 序列化/反序列化

### 7.3 wb-ui 已有可复用基础设施

- [dom/element.go](file:///f:/syproject/wb-ui/dom/element.go) — Element 已有 attrs map、AppendChild、GetElementById
- [dom/range.go](file:///f:/syproject/wb-ui/dom/range.go) — Range API（可直接被 VisibleSelection 包裹）
- [html/parser.go](file:///f:/syproject/wb-ui/html/parser.go) — HTMLDocumentParser（Markdown renderer 可复用）
- [layout/inlineformattingcontext.go](file:///f:/syproject/wb-ui/layout/inlineformattingcontext.go) — 已支持多行文本、white-space
- [rendering/rendertext.go](file:///f:/syproject/wb-ui/rendering/rendertext.go) — RenderText 已有
- [rendering/selection.go](file:///f:/syproject/wb-ui/rendering/selection.go) — 选区绘制已有
- [editing/editor.go](file:///f:/syproject/wb-ui/editing/editor.go) — Editor 已有 IME composition（需扩展）
- [platform/graphics/canvas.go](file:///f:/syproject/wb-ui/platform/graphics/canvas.go) — Skia Canvas（DrawText/MeasureText/FontAscent 可复用）
- [app/host.go](file:///f:/syproject/wb-ui/app/host.go) — Host 已有 FocusElement/Unfocus/SetIMECompositionPos（编辑器集成入口）

---

## 八、统计：组件覆盖率

### 8.1 模块级覆盖率（17 个模块）

| 状态 | 模块数 | 占比 |
|---|---|---|
| 已实现（≥70%） | 13 | 76% |
| 部分实现（30-70%） | 2 | 12%（jsc、dom） |
| 仅占位（<30%） | 2 | 12%（editing、gpu） |

### 8.2 HTML 元素组件级覆盖率

WebKit 共有约 150 个 HTML 元素，其中：
- **专用 C++ 子类的元素**：约 60 个
- **使用通用 HTMLElement 的元素**：约 90 个

wb-ui 当前：
- **专用 Go 子类的元素**：0 个
- **全部使用通用 dom.Element**：150 个（仅作为通用节点解析与渲染，无语义行为）

**专用元素类覆盖率：0 / 60 = 0%**

### 8.3 表单控件覆盖率

| 类别 | WebKit 类数 | wb-ui 实现 |
|---|---|---|
| 表单元素类（form/input/button/textarea/select/option/optgroup/label/fieldset/legend/datalist/output/progress/meter/selectedcontent） | 15 | 0 |
| InputType 子类 | 22 | 0 |
| 表单控件基类（HTMLFormControlElement/HTMLFormControlElementWithState/HTMLTextFormControlElement/FormAssociatedElement/FormListedElement/ValidatedFormListedElement） | 6 | 0 |
| **合计** | **43** | **0** |

**表单控件覆盖率：0 / 43 = 0%**

### 8.4 editing 包翻译覆盖率

| 类别 | WebKit 关键文件数 | wb-ui 实现 |
|---|---|---|
| Composition 相关 | 4 | 3 |
| EditCommand 体系 | 5 | 0 |
| Selection 体系 | 3 | 0 |
| Typing/Insert | 4 | 0 |
| 替换/删除命令 | 6 | 0 |
| markup/clipboard | 2 | 0 |
| Editor 总入口 | 1 | 1（仅 IME 子集） |
| **合计** | **25** | **4** |

**editing 翻译覆盖率：4 / 25 = 16%**

### 8.5 代码编辑器/Markdown 组件

| 组件 | wb-ui 现状 |
|---|---|
| 代码编辑器 | 完全缺失（0%） |
| Markdown 渲染器 | 完全缺失（0%） |
| 语法高亮 Tokenizer | 完全缺失（0%） |
| 不可变文档模型 | 完全缺失（0%） |
| 装饰系统 | 完全缺失（0%） |
| 虚拟滚动 | 完全缺失（0%） |

---

## 九、本次实施范围（用户需求）

用户明确要求实现：
1. **代码编辑框组件** — 参考 CodeMirror 6 / Monaco
2. **Markdown 组件** — 参考 markdown-it

**不在本次范围**（后续阶段）：
- 完整 HTML 表单控件迁移（HTMLInputElement/HTMLButtonElement/HTMLTextAreaElement/HTMLSelectElement）
- 媒体元素（img/video/audio/canvas）
- SVG
- 完整 execCommand API

但代码编辑器/Markdown 实现会**附带**翻译以下 WebKit editing 子集（作为编辑器底层支撑）：
- `EditCommand` / `SimpleEditCommand` / `CompositeEditCommand` / `EditCommandComposition`
- `TypingCommand`
- `VisibleSelection` / `VisiblePosition`

详见 `task_plan.md`。
