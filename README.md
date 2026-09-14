# wb-ui

纯 Go 的 Web 渲染引擎：把 WebCore/WebKit 的「解析 → 样式 → 布局 → 绘制」管线按
1:1 对应关系移植到 Go，光栅化走 Skia（经 [goskia](https://github.com/hoonfeng/goskia)
的 CGO 绑定），JS 执行走内嵌的 goja 引擎。同一套引擎支持两种形态：

- **嵌入式浏览器**：加载真实网页（HTTP / 本地文件、外部样式与脚本、导航、Worker）
- **UI 工具包**：宿主用 Go 组件或 HTML 片段声明界面（裁剪浏览器专属能力）

两种形态的语义差异与接线点见 [docs/MODES.md](docs/MODES.md)。

## 包结构

| 目录 | 职责 |
|---|---|
| `wtf/` `bmalloc/` | WebKit 基础库移植（原子串/字符串、bmalloc + IsoHeap） |
| `html/` `html5/` | HTML 词法与树构建、UA 样式表 |
| `css/` `style/` | CSS 解析（含 calc、媒体查询）与样式解析/层叠 |
| `dom/` | DOM 树、事件、Node/Element API |
| `layout/` | 布局引擎：盒模型、行内格式化（IFC）、浮动、定位、flex、grid、表格 |
| `rendering/` | 渲染树与绘制管线（渐变、filter、文本描边、合成） |
| `editing/` | 编辑子系统（WebKit editing 子集） |
| `page/` | 文档生命周期、资源加载、导航、管线日志（`WB_VERBOSE` → `[PIPELINE:Stage]`） |
| `platform/graphics` `platform/window` `platform/ime` | 平台层：Skia 画布/字体、窗口与光标、输入法 |
| `jsc/` `jsenv/` `goja/` | JS 运行环境（goja 为引擎，jsc 为浏览器全局兼容层） |
| `bindings/` | DOM/Web API 的 JS 绑定，以及按运行模式的能力裁剪 |
| `bridge/` | JS ↔ Go 桥（前端用标准 `fetch` 调用 Go 注册的端点，无需改前端代码） |
| `webkit/` | WebView：引擎装配入口（模式、导航、置脏重绘） |
| `app/` | 应用宿主：GLFW 窗口 + Skia GPU 表面 + WebView 渲染/事件循环 |
| `ui/` `popover/` `worker/` `gpu/` | UI 组件注册表、Popover API、Web Worker、GPU 桥（占位） |
| `cmd/browser` | 最小浏览器（导航/前进后退/刷新，`-verify` 为无头能力验证） |
| `cmd/pipeline_diag` | 解析 `[PIPELINE:Stage]` 日志并检测管线异常 |
| `cmd/translation-progress` | 统计各文件的翻译元数据头（Translation of / Completeness） |
| `examples/` | 示例程序（minibrowser / mixed / uitoolkit / bridge_demo / vue_load_test） |
| `dev/` | 诊断与验证套件（见 [dev/README.md](dev/README.md)） |
| `docs/` | 设计文档 |
| `resources/fonts/` | 内置字体（DejaVu / Liberation / Noto Color Emoji / Roboto / kochi） |

## 构建

前置条件：

- Go 1.25+
- CGO 工具链（Windows 用 MinGW-w64 GCC，如 MSYS2 mingw64 的 gcc）
- Skia 原生库 `libSkiaSharp.dll`（由 goskia 提供），且其所在目录在 `PATH` 中

```bat
:: Windows：脚本内部已设 CGO_ENABLED=1 与 DLL 目录
cgo_env.bat build

:: 或手动
set CGO_ENABLED=1
set PATH=<goskia>\bin;%PATH%
go build ./...
```

```bash
# Linux / macOS
CGO_ENABLED=1 PATH=<goskia>/bin:$PATH go build ./...
```

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

## 依赖

- `github.com/hoonfeng/goskia` — Skia CGO 绑定 + `libSkiaSharp.dll`
- `goja` — ECMAScript 引擎（已并入本仓库 `goja/`，非独立模块）
- 其余见 `go.mod`（glfw、regexp2、go-yaml、pprof、sourcemap、x/text、semver）

本机开发时父目录的 `go.work` 会把 `goskia` 指向本地目录；要验证「克隆后能否构建」，
请用 `GOWORK=off go build ./...`（只按 `go.mod` 解析依赖）。

## 文档

| 文档 | 内容 |
|---|---|
| [docs/MODES.md](docs/MODES.md) | 运行模式（浏览器 / UI 工具包）与 UI 构建方式 |
| [docs/CALIB.md](docs/CALIB.md) | 渲染校准与诊断工具（calib / tddiag / cssprobe / scriptsdiag） |
| [docs/INCREMENTAL_DESIGN.md](docs/INCREMENTAL_DESIGN.md) | 增量布局架构改进方案 |
| [docs/PERF_PLAN.md](docs/PERF_PLAN.md) | 性能优化调研与方案 |
| [docs/MASK_P3_PLAN.md](docs/MASK_P3_PLAN.md) | mask-image P3 实施计划 |
| [docs/TECH_DEBT.md](docs/TECH_DEBT.md) | 遗留问题处理指南 |

## 第三方素材

- `goja/`：ECMAScript 引擎（源自 dop251/goja，MIT License）
- `dev/suites/cssprobe/fixtures/`：CSS 渲染夹具与期望值借用自 obscura 的
  render-repros（Apache-2.0）
- `resources/fonts/`：DejaVu、Liberation、Roboto（各自的自由许可）、
  Noto Color Emoji（SIL OFL）、kochi 日文字体（公有领域）

## 许可

[MIT](LICENSE)
