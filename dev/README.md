# dev —— 诊断与验证套件

这里放的是渲染引擎的**诊断工具与验证套件**，不是库代码。所有命令都在**仓库根目录**执行
（工具用相对路径引用夹具），环境与 `cgo_env.bat` 一致：`CGO_ENABLED=1` + `libSkiaSharp.dll`
所在目录在 `PATH`。

## 目录约定

| 目录 | 内容 |
|---|---|
| `suites/` | 成规模的验证套件：自带夹具集 + 断言/参照，可当回归用 |
| `probes/` | 单文件诊断工具：一个目录一个 `main` 包 |
| `fixtures/` | 被多个工具共享的 HTML 夹具（无 Go 代码） |
| `lib/` | 探针共用的 Go 库（`probelib`） |
| `output/` | **所有产物的统一出口**（PNG / JSON / 日志），不纳入版本控制 |

约定：

- 产物一律写 `dev/output/`，不要写回套件目录（该目录已在 `.gitignore` 中，工具会自动创建）。
- 同一个目录里需要多个 `main` 时（如 `suites/static_probe`），除主 `main.go` 外的文件加
  `//go:build ignore`，这样 `go build ./...` 只编译主入口。
- 带 `//go:build ignore` 的探针不能按目录运行，需指定文件：
  `go run dev/probes/<名字>/main.go`（下表已标注）。

## suites —— 成规模套件

| 套件 | 用途 | 运行 |
|---|---|---|
| `consistency` | 与 Edge headless 的**像素级参照对比**（逐 case 截图比对；本机无 Edge 时自动 skip）。报告写 `dev/suites/consistency/report/` | `go test ./dev/suites/consistency`<br>`go run ./dev/suites/consistency -case layout_basic` |
| `cssprobe` | 确定性 CSS 夹具 + 几何断言：夹具在 `fixtures/`，期望值在 `fixtures/checks.json`（借自 obscura 参考实现）。`-v` 列明细、`-dump` 出 PNG、`-json` 出机读结果 | `go run ./dev/suites/cssprobe -v -filter table` |
| `pixel_probe` | 整页渲染并读像素（PNG + 布局树）。子目录 `clicktest/` `domdump/` `hitdbg/` `postclick/` `simclick/` `sim_events/` 为交互与命中测试类探针 | `go run ./dev/suites/pixel_probe --html FILE --out dev/output/out.png`<br>`go run ./dev/suites/pixel_probe/clicktest` |
| `static_probe` | 真实整页渲染（IDE 页面 / grid / desc）→ PNG（写 `dev/output/`）+ 几何 dump；同目录还有 `xseg_dump.go`、`vcenter_dump.go`、`gift_align_dump.go` 等夹具几何诊断（按文件运行） | `go run ./dev/suites/static_probe`<br>`go run dev/suites/static_probe/xseg_dump.go` |
| `static_test` | wb-ui 与浏览器的坐标/像素对比（`compare_test.go` + playwright 抽取脚本，脚本产物写 `dev/output/`） | `go test ./dev/suites/static_test`<br>`python dev/suites/static_test/pixel_compare.py` |

## probes —— 单文件诊断工具

| 探针 | 用途 | 运行 |
|---|---|---|
| `browser_http_probe` | 「嵌入式浏览器」的真实网络端到端：起 HTTP 服务 → `LoadURL` 加载真实页面（HTML / 外部样式 / 脚本） | `go run ./dev/probes/browser_http_probe` |
| `calib` | 同一夹具分别用 wb-ui 与 obscura 渲染，量化像素差异占比与差异区域 | `go run ./dev/probes/calib -fixture dev/suites/cssprobe/fixtures/tables.html -top 6 -out dev/output/diff.png` |
| `canvas_probe` | WebView 级 canvas 2D 端到端冒烟（JS 绘制 → 像素断言） | `go run ./dev/probes/canvas_probe` |
| `cssoracle` | 用**真实浏览器**渲染 cssprobe 的同一批夹具，把「期望错了」与「wb-ui 渲染错了」分开 | `go run ./dev/probes/cssoracle -filter legacy -v` |
| `flexdbg` | flex 布局调试打印（盒子树 + flex 参数） | `go run ./dev/probes/flexdbg` |
| `fontmetric` | 打印注入布局引擎的字体度量（ascent/descent/lineGap）与其行高，与浏览器 grid-fit 对照 | `go run ./dev/probes/fontmetric Arial 12` |
| `fontprobe` | 逐条字体解析路径的 CJK 渲染验证（OS 字型 vs 注册的原始数据字型） | `go run ./dev/probes/fontprobe` |
| `fxprobe` | dump 渲染树 + 绘制顺序输入（Display / Float / IsFloated），回答「这个盒子为什么后画」 | `go run ./dev/probes/fxprobe <file.html>` |
| `gobench` | goja 编译大 bundle 的耗时剖析（`GC_PERCENT` 可调 GC 频率） | `go run ./dev/probes/gobench` |
| `jssyntax` | 用主项目（直播挂件助手）的模板 HTML 做 goja 语法检查（`//go:build ignore`） | `go run dev/probes/jssyntax/main.go` |
| `l2djscheck` | Live2D 模板脚本的语法检查（读主项目生成的脚本；`//go:build ignore`） | `go run dev/probes/l2djscheck/main.go` |
| `leakprobe` | WebView 创建 → LoadHTML → Render → Destroy 多轮，验证 DOM/渲染树/JS 解释器整棵树可回收（挂件重建泄漏回归；`//go:build ignore`） | `go run dev/probes/leakprobe/main.go` |
| `mem_probe` | 循环 LoadHTML / Render 测堆增长，定位泄漏与高频分配 | `go run ./dev/probes/mem_probe` |
| `pipeline_trace` | 采集渲染管线日志（配合 `cmd/pipeline_diag` 分析；JSON 写 `dev/output/`） | `go run ./dev/probes/pipeline_trace <log>` |
| `quicktest` | 最小冒烟（Skia 画布可用性 + 字体度量） | `go run ./dev/probes/quicktest` |
| `scriptsdiag` | 把夹具跑进 WebView 环境，报告脚本输出、脚本可见的平台 API、夹具自评结论 | `go run ./dev/probes/scriptsdiag -file dev/suites/cssprobe/fixtures/modern-streams.html` |
| `skia_check` | Skia 冒烟：`Init` → 建光栅表面 → `Clear` → 退出码（原根目录 `check_skia_build.go`） | `go run ./dev/probes/skia_check` |
| `tddiag` | 并排打印 computed display 与布局几何，定位「盒子宽度/高度被谁写坏」 | `go run ./dev/probes/tddiag -file dev/suites/cssprobe/fixtures/fixed-table-layout.html` |
| `window_test` | 在真实窗口里打开 HTML 做交互测试 | `go run ./dev/probes/window_test --html FILE` |
| `xheight_probe` | 字体 x-height（`ex` 单位的解析基础）度量，对比未初始化/已初始化的字体管理器（`//go:build ignore`） | `go run dev/probes/xheight_probe/main.go` |

## fixtures —— 共享夹具

| 夹具 | 说明 |
|---|---|
| `xseg/` | flex 项内文本对齐/垂直居中诊断页（`xseg.html`、`xseg_real.html`），由 `suites/static_probe` 的 `xseg_dump.go` / `vcenter_dump.go` 读取 |
| `vcenter/` | `.txt` 的 `align-items:center` CJK 垂直居中夹具（当前无探针引用，保留作参考） |

## lib —— 共用库

`lib/probelib`（import 路径 `wb-ui/dev/lib/probelib`）提供 cssprobe 夹具的公共原语
（颜色/组件扫描），被 `probes/cssoracle` 使用。
