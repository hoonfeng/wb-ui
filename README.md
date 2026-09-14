# wb-ui

纯 Go 的 Web 渲染引擎：把 WebCore/WebKit 的「解析 → 样式 → 布局 → 绘制」管线按
1:1 对应关系移植到 Go，光栅化走 Skia（经 [goskia](https://github.com/hoonfeng/goskia)
的 CGO 绑定），JS 执行走内嵌的 goja 引擎。同一套引擎支持两种形态：

- **嵌入式浏览器**：加载真实网页（HTTP / 本地文件、外部样式与脚本、导航、Worker）
- **UI 工具包**：宿主用 Go 组件或 HTML 片段声明界面（裁剪浏览器专属能力）

两种形态的语义差异与接线点见 [docs/MODES.md](docs/MODES.md)。

## 包结构

对外 API（宿主直接使用，路径不含 `engine/`）：

| 目录 | 职责 |
|---|---|
| `webkit/` | WebView：引擎装配入口（模式、导航、置脏重绘、交互） |
| `app/` | 应用宿主：GLFW 窗口 + Skia GPU 表面 + WebView 渲染/事件循环 |
| `ui/` | UI 工具包构建层：用 Go 组件 / HTML 片段声明界面 |
| `bridge/` | JS ↔ Go 桥（前端用标准 `fetch` 调用 Go 注册的端点，无需改前端代码） |

引擎实现（`engine/`，按 WebKit `Source/` 的分层对应移植）：

| 目录 | 职责 | 对应 WebKit 源码 |
|---|---|---|
| `engine/wtf/` `engine/bmalloc/` | 基础库：原子串/HashMap/RefPtr；bmalloc + IsoHeap | `Source/WTF`、`Source/bmalloc` |
| `engine/platform/` | 平台层：graphics（Skia 画布/字体）、window（GLFW/光标/DPI）、ime、event、thread、time | `Source/WebCore/platform` + `PAL` |
| `engine/dom/` | DOM 树、事件、Node/Element API | `Source/WebCore/dom` |
| `engine/css/` `engine/style/` | CSS 解析（含 calc、媒体查询）与样式解析/层叠 | `Source/WebCore/css`、`style` |
| `engine/html/` `engine/html5/` | HTML 词法与树构建；HTML5 元素实现 + UA 样式表 | `Source/WebCore/html` |
| `engine/editing/` | 编辑子系统（WebKit editing 子集） | `Source/WebCore/editing` |
| `engine/popover/` | Popover API 算法（HTML Standard §6.12） | `Source/WebCore/html`（popover） |
| `engine/layout/` | 布局引擎：盒模型、行内格式化（IFC）、浮动、定位、flex、grid、表格 | `Source/WebCore/layout` |
| `engine/rendering/` | 渲染树与绘制管线（渐变、filter、文本描边、合成） | `Source/WebCore/rendering` |
| `engine/page/` | 文档生命周期、资源加载、导航、管线日志（`WB_VERBOSE` → `[PIPELINE:Stage]`） | `Source/WebCore/page` |
| `engine/js/goja/` | 内嵌 ECMAScript 引擎（vendored，源自 dop251/goja） | `Source/JavaScriptCore` |
| `engine/js/jsc/` | goja 适配层与浏览器全局兼容层 | 同上（绑定侧） |
| `engine/js/bindings/` | DOM/Web API 的 JS 绑定，以及按运行模式的能力裁剪 | `Source/WebCore/bindings` |
| `engine/js/worker/` | Web Worker 并发脚本执行 | `Source/WebCore/workers` |
| `engine/debugenv/` | 进程启动时的调试开关快照（`WB_*` / `WBUI_*`） | — |
| `engine/gpu/` | GPU 进程 / Skia 后端桥（**占位，未实现**） | `Source/WebKit/GPUProcess` |

工具、示例与资源：

| 目录 | 职责 |
|---|---|
| `cmd/browser` | 最小浏览器（导航/前进后退/刷新，`-verify` 为无头能力验证） |
| `cmd/pipeline_diag` | 解析 `[PIPELINE:Stage]` 日志并检测管线异常 |
| `cmd/translation-progress` | 统计各文件的翻译元数据头（Translation of / Completeness） |
| `examples/` | 示例程序（minibrowser / mixed / uitoolkit / bridge_demo / vue_load_test） |
| `dev/` | 诊断与验证套件（见 [dev/README.md](dev/README.md)） |
| `docs/` | 设计文档 |
| `resources/fonts/` | 内置字体（DejaVu / Liberation / Noto Color Emoji / Roboto / kochi） |

> 目录划分即依赖方向：`engine/` 内部不反向依赖 `webkit/`、`app/`、`ui/`、`bridge/`；
> 空包（`engine/gpu/`、`engine/platform/fonts/`）是**预留的架构槽位**，
> 其 `doc.go` 写明计划实现的层与将来的抽取点。

## 构建

前置条件：

- Go 1.25+
- CGO 工具链（Windows 用 MinGW-w64 GCC，如 MSYS2 mingw64 的 gcc）
- Skia 原生库（`libSkiaSharp.dll` / `.so` / `.dylib`），随 goskia 模块一起下载，
  位于 `$(go env GOMODCACHE)/github.com/hoonfeng/goskia@<ver>/skia/lib/<goos>_<goarch>/`，
  构建时其所在目录需在 `PATH` 中

```bat
:: Windows：脚本自动定位 goskia 原生库并设好 CGO_ENABLED
cgo_env.bat build

:: 或手动（首次先 go mod download github.com/hoonfeng/goskia）
set CGO_ENABLED=1
set PATH=<goskia>\skia\lib\windows_amd64;%PATH%
go build ./...
```

```bash
# Linux / macOS
CGO_ENABLED=1 PATH=<goskia>/skia/lib/$(go env GOOS)_$(go env GOARCH):$PATH go build ./...
```

`make build` 同样自动定位原生库；要指定别处的原生库，用
`make build SKIA_DLL_DIR=<dir>`（`cgo_env.bat` 认环境变量 `SKIA_DLL_DIR`）。

## 测试

```bash
go test ./...                                         # 全部（无 Edge 时 dev/suites/consistency 自动 skip）
go test $(go list ./... | grep -v dev/suites/consistency)    # 跳过像素对比套件，更快
```

## 诊断套件

`dev/` 按四类组织：`suites/`（成规模套件）、`probes/`（单文件诊断工具）、
`fixtures/`（共享夹具）、`output/`（产物统一出口，不纳入版本控制）。完整索引见
[dev/README.md](dev/README.md)。常用命令：

```bash
go run ./dev/suites/cssprobe        # CSS 夹具几何断言（-v 看明细，-dump 出 PNG）
go run ./dev/suites/consistency     # 与 Edge 的像素级参照对比（需 Edge）
go run ./dev/probes/calib -fixture dev/suites/cssprobe/fixtures/tables.html
go run ./dev/probes/tddiag -file dev/suites/cssprobe/fixtures/fixed-table-layout.html
```

> 少数探针的默认路径指向开发机上的兄弟项目（goskia 源码、gou-ide 前端 bundle、
> 主项目夹具）：`gobench`、`jssyntax`、`l2djscheck`、`vue_load_test` 等需按本机
> 情况改用 `-flag` 或改常量，`calib` 可直接用 `-obscura <path>` 指定。

## 依赖

- `github.com/hoonfeng/goskia` — Skia CGO 绑定 + `libSkiaSharp.dll`
- `goja` — ECMAScript 引擎（已并入本仓库 `engine/js/goja/`，非独立模块）
- 其余见 `go.mod`（glfw、regexp2、go-yaml、pprof、sourcemap、x/text、semver）

与 goskia 源码联动开发时，父目录的 `go.work` 会把 `goskia` 指向本地目录；
要验证「克隆后能否构建」，请用 `GOWORK=off go build ./...`（只按 `go.mod` 解析依赖）。

## 文档

| 文档 | 内容 |
|---|---|
| [docs/MODES.md](docs/MODES.md) | 运行模式（浏览器 / UI 工具包）与 UI 构建方式 |
| [docs/CALIB.md](docs/CALIB.md) | 渲染校准与诊断工具（calib / tddiag / cssprobe / scriptsdiag） |
| [docs/TECH_DEBT.md](docs/TECH_DEBT.md) | 遗留项取舍结论与有意保留的边界 |

> 计划类文档（原 `MASK_P3_PLAN.md`、`INCREMENTAL_DESIGN.md`、`PERF_PLAN.md`）已随实现落地
> 删除——诊断与方案在 git 历史里，实现要点在代码注释与测试里；未实现项的逐条取舍结论
> 留在 `docs/TECH_DEBT.md`。

## 第三方素材

- `engine/js/goja/`：ECMAScript 引擎（源自 dop251/goja，MIT License）
- `dev/suites/cssprobe/fixtures/`：CSS 渲染夹具与期望值借用自 obscura 的
  render-repros（Apache-2.0）
- `resources/fonts/`：DejaVu、Liberation、Roboto（各自的自由许可）、
  Noto Color Emoji（SIL OFL）、kochi 日文字体（公有领域）

## 许可

[MIT](LICENSE)
