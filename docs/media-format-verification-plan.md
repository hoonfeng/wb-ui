# 媒体格式真实可用性验证项目（方案 v3 · 七项决策已确认：其中决策 6 经 2026-10 修订、决策 7 为 2026-10 新增）

> 状态：**设计定稿（七项关键决策已由用户选定，见 §0.1；其中决策 6 经 2026-10 修订、决策 7（项目归属）为 2026-10 新增，见 §0.2）**。
> ★ **实现侧已开始落地**：A0（宿主注入元数据）、**A1（视频出画面 · 宿主注入帧流）** 已实装，
> 逐项证据见 [docs/implementation-path.md](implementation-path.md) §0.1；本文的 G5 表格已回写实测结论。
> 本文所有「实测」结论均在本机复现过，证据文件与复现命令见 §3、§6。
> 落地顺序与验收方式见 §8，决策记录、归属划分与已采纳建议见 §9。
> ★ **实现路径已收敛（2026-10-06）**：本方案的**实现**部分（视频/音频真实播放 = **主线 A**）的分期、依赖与验收
> 已并入 [docs/implementation-path.md](implementation-path.md) §2；该总纲同时收录 **CDP 调试协议（主线 B）** 与
> **JS 引擎对标（主线 C）**，三线共用同一分期表（总纲 §6）。本文继续作为**验证侧设计稿**（L0–L4 分级、判定标准）。
> ★ **2026-10 复测已执行（四配置 × 96 格）**：结果与缺口清单见 §3.4，完整证据见
> `dev/media/out/report.md` 与四配置 `matrix-*.png`，回归基线由 `dev/media/baseline.json` 承载。
> 本轮修复 U2（`<img>` load/error 契约）、U5（SVG file/rel 渲染）、D4（WebP/BMP/ICO 固有尺寸），
> 并修复一处**引入即发现并修掉**的回归 R1（SVG 探测在 paint 线程同步联网）。

---

## 0. 一句话结论

本引擎的媒体能力边界是：

1. **光栅图像解码完整**：PNG / JPEG / GIF / WebP（有损+无损）/ BMP / ICO 都能解码并绘制；
2. **SVG 走自有矢量路径**，不入 Skia 解码器；
3. **动画只取首帧**：GIF / WebP 动画无帧推进机制，视觉上恒为静态首帧；
   ★ **2026-10 复测更新**：GIF 动画已能**帧推进**（3 个动画样本 × 3 来源均达 L4）；
   WebP 动画仍恒为静态首帧（缺口 D11）。
4. **视频、音频没有内置解码与输出能力**（无解码器、无音频后端）——
   ★ **2026-10-07 更新**：宿主注入元数据（A0）与帧（A1）之后，`<video>` 已能完成资源加载（L1：`readyState=4`、
   `duration`、`videoWidth/Height`）并**画出画面**（L2：无 poster 时画当前帧、有 poster 时先画 poster）；
   音频输出（L4-S）仍无。
5. ★ **在 UI 库模式（ModeToolkit，即 AI-PS 所用配置）下，光栅图片一律不渲染——连自包含的 `data:` URI 都不渲染**，接宿主 resolver 也无效（A/B 两组渲染结果字节完全相同）。
   ★ **2026-10-07 更新**：这一条已**修复**——`ImageResourceLoader` 的契约本就写着「data: URL 不走
   `AllowsExternal` 判定」，但实现把模式门禁放在了 `decodeDataURI` 之前。门禁顺序改正后 `data:` 无条件
   放行（`<video poster="data:…">` 在 ModeToolkit 下已能绘制，见 G5 的 TC-M-502 实测）；`file://` 与
   `http(s)` 的拒绝语义不变。
6. ★ **2026-10 复测新增（契约侧）**：`<img>` 的 `load` / `error` 事件契约已补齐（U2）——
   此前「画得出但脚本测不到」（§3.3 D1）的两侧现在都可用：`complete` / `naturalWidth` 正确，
   加载成功派发 `load`、失败派发 `error`。**SVG 仍是例外**（无固有尺寸、无 `load`/`complete`，
   见缺口 D8）。

第 5 条是本次盘点最重要的发现，直接决定「AI-PS 界面里图片能不能用」。

### 0.1 已确认决策（用户选择）

下表为方案 v1 遗留的六个开放问题，用户已逐项选定；本文各章节已按此回写。

| # | 决策项 | **用户选择** | 直接后果 |
|---|---|---|---|
| 1 | U1 门禁修法 | **(c) 新增模式开关，由宿主声明是否允许外部/本地资源** | 需设计新 API 与默认值；`data:` 无条件放行（API 草案见 §8.2 阶段 1） |
| 2 | 验证范围 | **纳入 Edge 双端对照**（复用 `dev/suites/consistency`） | 判定以「引擎 vs Edge」为准；无 Edge 环境时标 `SKIP(no-edge)`，不得默认通过 |
| 3 | U4 动图路线 | **goskia 暴露 `SkCodec`**（通用，含 WebP 动画） | 需改 goskia 并推 main → wb-ui 更新 `go.mod` → 跨仓库版本联动（§8.2 阶段 2） |
| 4 | 视频/音频目标 | **要真实播放**（宿主注入帧/音频流 或 内置 ffmpeg，单独立项） | U6/U7 从「明确不支持」升级为**阶段 3 实施项**；判定标准须覆盖 L4 |
| 5 | 样本与产物入库 | **不提交**：样本本地生成，报告仅作本地产物，仓库不动 | 入库物只有**生成脚本与探针代码**；样本/报告目录进 `.gitignore` |
| 6 | 验证触发方式 | **不入 CI 门禁**（CI 属 git 自动 CI 范畴）——本方案交付**可复现的按需执行工装**（§6.4） | ⚠️ **2026-10 修订**：原选「纳入 CI 自动门禁」，改判为不接 CI；不写接线脚本，本机按需执行 |

> ✅ **原「决策 5 × 决策 6」冲突已消解**：决策 6 修订为「不入 CI 门禁」后，「CI 无样本可跑」的前提不再成立。
> 决策 5 保持不变：入库物只有生成脚本与探针代码，样本/报告目录进 `.gitignore`，验证在本机按需执行（§6.4）。

### 0.2 项目归属与范围划分（2026-10 新增）

**用户决定：格式能力等级（L0–L4）验证不在 AI-PS 项目里做，另开项目验证。**

| 事项 | 归属 |
|---|---|
| 格式能力等级验证（L0–L4 分级、四配置渲染、Edge 双端对照、回归基线） | **独立媒体验证项目（另开）**——本文即其设计稿 |
| AI-PS 侧 | 仅保留**本机可用性盘点**（粗粒度：界面里的图片/媒体能否加载、能否显示），**不含**等级判定、Edge 双端对照、回归基线 |
| 引擎缺陷修复（U1–U9） | 仍属 wb-ui / goskia 的修复范畴，与本划分不冲突 |

⇒ 本文所有「验证」「判定」「基线」相关要求均为**该独立项目的交付物**，不构成 AI-PS 项目的验收项。

---

## 1. 目标与范围

### 1.1 目标

1. 用**可复现的实测**回答：哪些音视频/图像格式在真实页面里「真实可用」——能加载、能量尺寸、能绘制、脚本可观察、动画能动、声音能出。
2. 给出**证据化判定**（像素 / 几何 / 事件 / 日志），不接受「看起来能行」。
3. 对不可用项产出**分阶段、可审核**的落地计划。

### 1.2 范围

| 维度 | 取值 |
|---|---|
| 静态图像 | PNG、JPEG、GIF(静态)、WebP(有损/无损)、BMP、ICO、AVIF、TIFF、SVG |
| 动图 | GIF 动画、WebP 动画 |
| 视频 | MP4(H.264)、WebM(VP9) |
| 音频 | WAV、MP3、OGG(Vorbis)、M4A(AAC) |
| 资源来源 | `data:` URI、`file://` 绝对路径、文档相对路径 |
| 运行模式 | ModeBrowser（浏览器模式）、ModeToolkit（UI 库模式，±宿主 resolver） |
| 资源策略（决策 1 新增） | `DenyExternal`（默认；`data:` 无条件放行）/ `AllowHostResolved`（仅放行宿主 resolver 明确解析出的资源）/ `AllowAll`（等价 ModeBrowser） |
| 承载方式 | `<img>`、CSS `background-image`、`<video poster>`、`<video src>`、`<audio src>` |
| 判定基准（决策 2） | **Edge 双端对照**：同一样本页在 Edge 与引擎各跑一次，逐项比对，输出「引擎 / Edge」双列报告 |
| 项目归属（2026-10 新增） | **独立媒体验证项目**（另开立项；本文为其设计稿）；L0–L4 等级验证**不在 AI-PS 项目内进行**——AI-PS 仅做本机可用性盘点 |

### 1.3 非范围（本轮不做）

- 网络流媒体（HLS/DASH/MSE）、DRM、`<source>` 多源协商
- `getUserMedia` / WebRTC / `MediaStream`
- 视频编解码器移植本身（属落地计划，不属验证）
- ★ 说明（决策 4）：视频/音频的**真实播放**已确认要做，但实施属**阶段 3 单独立项**；本轮验证负责「判定等级 + 建立基线 + 定义验收标准」。
- ★ 项目归属（2026-10 新增）：**格式能力等级（L0–L4）验证不在 AI-PS 里做**，作为**独立项目另开验证**（§0.2）；AI-PS 侧只保留「本机可用性盘点」，不含等级判定、Edge 双端对照与回归基线。
- GPU/硬件解码路径（当前引擎只有 CPU 光栅）

---

## 2. 「真实可用」判定标准（L0–L4 分级）

**为什么必须分级**：本次盘点已实测到「画得出来但脚本测不到」的分离状态（§3.3）。只判「能不能看见」会把契约缺陷漏掉。

| 级别 | 名称 | 判据（须全部满足） | 证据形式 |
|---|---|---|---|
| **L0** | 不支持 | 解码器返回空 或 无绘制路径 | 解码探针 nil / 绘制日志缺失 |
| **L1** | 元数据可用 | 布局能取得固有尺寸（`<img>` 不给尺寸时盒宽非 0），且 `naturalWidth/naturalHeight` 正确 | 几何报告 + IDL 探针 |
| **L2** | 静态渲染 | 视口像素出现预期图案（采样点与源图比对，容差内） | 截图 + 像素比对 + 人眼复核 |
| **L3** | 语义完整 | L2 且 `complete=true`、`onload` 派发、失败时才 `onerror` | 事件日志 |
| **L4** | 动态播放 | L3 且内容随时间变化：动图换帧（`SkCodec` 路线）/ 视频换帧（宿主注入帧流）/ 音频输出波形（需音频后端） | 连续帧差异 + 音频采样 |

判定写法：`PNG @ ModeBrowser @ file:// = L3`；`WebP @ ModeBrowser = L2（缺 L1 固有尺寸、缺 L3 契约）`。

**双端基准（决策 2）**：同一用例在 Edge 与引擎上各跑一次，报告记录 `引擎等级 / Edge 等级` 两列；`引擎 ≥ Edge` 判通过，低则记为缺陷并附双端截图。无 Edge 环境时该项标 `SKIP(no-edge)`，**不得**默认判通过。

---

## 3. 能力盘点结果（全部实测）

### 3.1 解码层：格式 × 解码器

数据来源 `_temp/mediacheck/out2.txt`（复现：`go run ./_temp/mediacheck/main.go`）。

| 格式 | Skia `DecodeImage`（**绘制**用的） | Go `image.DecodeConfig`（**布局量尺寸**用的） | 综合 |
|---|---|---|---|
| PNG | ✅ 120×80 | ✅ `png` | 双通 |
| JPEG | ✅ 120×80 | ✅ `jpeg` | 双通 |
| GIF 静态 | ✅ 120×80 | ✅ `gif` | 双通 |
| GIF 动画(3 帧) | ✅ 尺寸对（**仅首帧**） | ✅ `gif` | 首帧可用 |
| WebP 有损 | ✅ 120×80 | ❌ unknown format | **几何缺失** |
| WebP 无损 | ✅ 120×80 | ❌ | **几何缺失** |
| WebP 动画 | ✅（仅首帧） | ❌ | 首帧 + 几何缺失 |
| BMP (24bit) | ✅ 120×80 | ❌ | **几何缺失** |
| ICO（内嵌 PNG） | ✅ 120×80 | ❌ | **几何缺失** |
| SVG | ❌ 不入 Skia | ❌ | 靠引擎自有矢量路径 |
| AVIF | ❌ | ❌ | 不支持 |
| TIFF | ❌ | ❌ | 不支持 |
| MP4 / WebM | ❌ | ❌ | 不支持 |
| WAV / MP3 / OGG / M4A | ❌ | ❌ | 不支持 |
| 0 字节 / 垃圾数据 | ❌ | ❌ | 正确失败 |

★ **两套 codec 集合不一致**是本项目的核心结构性缺陷：
- 绘制走 Skia（`engine/platform/graphics/skia.go:29-42` → `sk_image_new_from_encoded`），支持 7 种；
- 布局固有尺寸走 Go `image.DecodeConfig`（`engine/layout/replaced.go:408`，仅注册了 `image/png|jpeg|gif`，见 `:21-23`），支持 3 种；
- 结果：WebP / BMP / ICO **画得出来，但量不出固有尺寸**——`<img>` 不给 CSS 尺寸时盒子可能塌成 0。

### 3.2 端到端渲染：真实页面 × 宿主配置

数据来源 `_temp/mediacheck/render/`（复现：`go run ./_temp/mediacheck/render/main.go`）。
同一份 HTML（17 种格式 × 3 种来源 + 固有尺寸例 + video/poster 例），三种配置各渲染一张 PNG。

| 组 | 配置 | data: 光栅图 | file:// 绝对 | 相对路径 | svg `data:` | 视频/音频 |
|---|---|---|---|---|---|---|
| **A** | ModeToolkit，无 resolver | ❌ 不画 | ❌ | ❌ | ✅ 画出 | ❌ 无画面 |
| **B** | ModeToolkit **+ resolver**（= AI-PS 配置） | ❌ | ❌ | ❌ | ✅ | ❌ |
| **C** | ModeBrowser（对照） | ✅ 全部画出 | ✅ 全部画出 | ✅ 全部画出 | ✅ | ❌ |

- A 与 B 的 PNG **字节数完全相同（15 956 B）**，视觉均为「几乎全空白表格，仅 `svg|data` 一个红块」→ 证明**宿主 resolver 对光栅图片零作用**。
- C 组 PNG 视觉可辨：png/jpeg/gif/webp/bmp/ico 三列全部显示出彩色渐变图；avif/tiff 空白；gif/webp 动画显示为**静态首帧**；`svg|file`、`svg|rel` 空白（仅 `svg|data` 渲染）；video/audio 单元格空白。

**代码级根因（已读源码确认）**：

```
engine/rendering/backgroundimage.go:349-358   loadBackgroundImageWith()
    if loader != nil && url != "" {
        if !loader.AllowsExternal() { return nil }   ← 门禁先于 data: 解析
        if abs := loader.ResolveURL(url); abs != "" { url = abs }
    }
webkit/image_resource.go:88-93                AllowsExternal()
    return l.wv.mode.allowsExternalURLs()
webkit/mode.go:66                             allowsExternalURLs()
    return m != ModeToolkit                       ← UI 库模式恒 false
```

⇒ **UI 库模式下任何 `<img src>` / `background-image` 都被拦，`data:` 自包含资源也被误伤**（`data:` 不需要任何外部通道，却因门禁位于解析之前而被拒）。唯一漏网的是 `data:image/svg+xml`——它走的是另一条分支（`loadBackgroundSVG`，`painter.go:2248`），不经图片门禁。

### 3.3 契约层缺陷（画得出，但脚本测不到）

| # | 现象 | 证据 | 影响 | 状态（2026-10 复测，见 §3.4） |
|---|---|---|---|---|
| D1 | ModeBrowser 下 file:// 图片**真实绘制**，但 `img.complete=false`、`naturalWidth=0` | C 组 PNG 可见图案 vs `render-out.txt` 报告 | 脚本无法判断加载完成；懒加载/骨架屏/占位逻辑失效 | ✅ **已修复**（U2：契约列 0 → 66 ✅，`complete=true`） |
| D2 | ModeToolkit 下 `data:` 自包含图被拒 | A/B 组全灰 | UI 库模式无法使用内联图标/贴图 | ✅ **已修复**（门禁移到 `decodeDataURI` 之后，`data:` 无条件放行） |
| D3 | `svg` 仅 `data:` 可渲染，`file://`/相对路径空白 | C 组第三列 svg 行为 | 文件引用的 SVG 图标不显示 | ✅ **已修复**（U5：file/rel 的 SVG 已绘制，实测 D=✅） |
| D4 | WebP/BMP/ICO 无固有尺寸 | §3.1 Go 列 ❌ | `<img>` 未给尺寸时 0×0 塌陷 | ✅ **已修复**（Skia `DecodeSize` 兜底 → 三者均 L3） |
| D5 | `<video>`/`<audio>` `readyState` 恒 0、`duration=NaN`、`currentTime` 不推进 | `render-out.txt` 中 `"rs":0` 全部 | 播放器类库走「未就绪」分支，永不 ready | 🟡 **部分修复**（A0：视频已派发 `loadedmetadata`，契约 C=✅） |
| D6 | `MediaMetadataResolver` 宿主从未注入 | `grep` 全仓仅定义(`media_element.go:41`)+单测赋值 | 即便本地 mp4 也拿不到时长 | ✅ **已修复**（A0：`app.InstallMediaMetadataResolver` 已在探针与 psai 装配） |
| D7 | 动图无帧推进 API | `goskia/skia` 无 codec/frame API（仅 `DecodeImage`/`NewImageFromPixels`） | GIF/WebP 动画恒为静态 | 🟡 **部分修复**（GIF 动图已达 L4；**WebP 动图仍未推进**，见 D11） |

### 3.4 本轮复测结果（2026-10）：四配置 × 96 格

工装：`python dev/media/gen_samples.py`（32 文件 + 1 内联条目 = 33 项样本）→ `cmd/psai -media`
（四配置矩阵页 + L0–L4 判定 + Markdown 报告 + 基线比对；`-media-update-baseline` 更新期望表）。
样本与矩阵页不入库（决策 5），**报告与四配置截图入库**（§6.3 证据纪律）。

| 配置 | L0 | L1 | L2 | L3 | L4 |
|---|---|---|---|---|---|
| Browser | 35 | 0 | **0** | **49** | **12** |
| Toolkit+AllowAll | 35 | 0 | 0 | 49 | 12 |
| Toolkit+AllowHostResolved | 35 | 0 | 0 | 49 | 12 |
| Toolkit+DenyExternal（默认） | 75 | 0 | 0 | 17 | 4 |

> 本表是**第二批修复**（D8/D9 闭环 + 探针动图判定稳定化，§9.4）后的复测值。
> 与第一批（U2/U5/D4，§9.3）对照：**L2 由 9/6 全部归零**（SVG 的固有尺寸与契约补齐）、
> Browser L3 42→49、L4 9→12、L0 36→35（内联 SVG 由 L0 升 L3）；
> `DenyExternal` 的 L0 73→75 是**策略正确化**——原先被缺陷放行的 `file://` SVG 回到 L0（§9.4）。

- **光栅静态全绿**：PNG / JPEG / GIF / WebP（有损+无损）/ BMP / ICO 在 data / file / rel 三种来源下
  全部 **L3**（含 `huge-4096.png`、含空格与中文文件名）。
- **GIF 动图已 L4**：3 个动画样本 × 3 来源 = 9 格达到「内容随时间变化」（A=✅）。
- **Toolkit+DenyExternal 保持安全默认**：file/rel 一律不加载（L0），只有 `data:` 无条件放行（L3/L4）。
- ★ **三个非拒配置的等级分布现已完全一致（35/0/0/49/12）**：第一批复测里 Browser 与
  `Toolkit+AllowAll` 曾出现 L3 42/39、L4 9/12 的差异，根因是探针的动图判定依赖
  「采样间隔 mod 动图循环周期」——3 帧 ×100ms 的 GIF 周期正是 300ms，而「再等 900ms
  拍一张」恰为其 3 倍，两拍落在**同一帧**、像素完全相同 → 判成「未推进」。
  实测同一 HEAD 连跑两次，下降项每次不同（`anim-3frames-rgb` 一次 L4 一次 L3，
  `anim-2frames.webp` 亦然），属**判定抖动**而非渲染差异；判据已改为等间隔连拍 6 帧 +
  「任意两帧不同」（§9.4 探针缺陷第 4 条），修后连续两次全量跑均「与基线一致」。

**本轮修复的引擎缺陷**

| 编号 | 缺陷 | 修法与落点 | 证据 |
|---|---|---|---|
| **U2** | `<img>` 的 `load`/`error` 从不派发（`onload`/`onerror` 失效） | 渲染层通知带 `ok` + 就绪查询；`webkit` 侧 goroutine 排队 → 主线程 `flushImageEvents` 派发；补 `rendering.RequestImageLoad`（浏览器「src 生效即加载」，不再依赖绘制） | `webkit/img_event_dispatch_test.go` 4 用例；探针失败路径备注「onerror 已派发（契约正确）」 |
| **U5 / D3** | SVG 仅 `data:` 可渲染 | `loadBackgroundSVGWith` 让 file:// 与相对路径走与栅格图同一条 loader 链 | 探针中 `rect-120x80.svg` / `icon-24.svg` / `ratio-only.svg` 的 file 与 rel 均 D=✅ |
| **D4** | WebP/BMP/ICO 无固有尺寸 | `layout.rasterIntrinsic` 在 Go `DecodeConfig` 失败时用 Skia `DecodeSize` 兜底 | 三者均达 L3（G=✅） |
| **R1** | *（本轮引入并修复的回归）* SVG 探测在 paint 线程同步 `loader.Load`：对栅格图/远端引用同步发请求，`Render()` 被扣住整个 HTTP 超时（实测 30 s）并重复发请求 | 同步通道收紧为「本地 + `.svg`/`.svgz` 扩展名」；栅格图与远端引用一律走异步通道 | `TestAsyncImageLoadMarksFrameDirty`：HEAD PASS(0.12 s) → 引入后 FAIL(60 s) → 修复后 PASS(0.12 s) |

**探针自身缺陷（测量伪影，会把引擎能力判低）**

1. `settleReal` 只推进事件循环、**从不渲染**——而本引擎的图片加载与事件派发由绘制驱动 →
   契约列恒 ❌；
2. 内联事件属性用**双引号**字面量作参数 → `onload="mvNote("c0",'load')"` 属性在第二个双引号处
   提前闭合 → 页面侧 `window.__mediaEvents` 恒为空；
3. 像素采样只支持 `quad`/`solid` 两种模式 → 动图（帧色）与渐变样本一律判「未绘制」。

修 1–3 后：契约列 **0 → 66 ✅**，Browser 下 L3/L4 由 **0 → 51** 格（`L1` 归零）。

**第二批已闭环的缺口（§9.4）**

| 编号 | 缺口 | 修复与结果 | 证据 |
|---|---|---|---|
| D8 | `<img src="*.svg">` 无固有尺寸、无 `complete`/`load` 契约 | ✅ 渲染层新增 `SVGReferenceIntrinsicSize` / `IsSVGReferenceReady`（width/height → viewBox → CSS 默认 300×150），bindings 增加「固有尺寸 hook」（`imgNaturalDim`），`flushImageEvents` 与错误派发按「能渲染即加载成功」纠正 SVG；SVG 九格 **L2 → L3**（`data:`/`file://`/相对路径三来源） | `webkit/img_svg_contract_test.go`、`TestSVGReferenceIntrinsicSize`；探针 `*.svg` 行四列全 ✅ |
| D9 | `data:image/svg+xml`（非 base64）内联 SVG 不绘制 | ✅ **探针伪影**（不是引擎缺陷）：矩阵页里 data URI 的双引号被按 JS 字面量写成 `\"`，HTML 属性在第一个 `"` 处提前闭合、`src` 只剩 `data:image/svg+xml,<svg xmlns=`；改按 RFC 2397 percent 编码 + 属性走 HTML 实体转义后 L0 → **L3** | 修复前后矩阵页 HTML 片段对照；探针 `inline-svg-data-uri` 行 |
| — | **（新发现）SVG 解析缓存绕过资源策略门禁** | ✅ 缓存查询移到策略门禁**之后**。缺陷表现：宽松配置（Browser）先解析过的 SVG 被包级缓存留存，严格配置（`Toolkit+DenyExternal`）随后命中缓存拿到文档 → 出现「加载/几何/契约 ✅ 而绘制 ❌」的自相矛盾 | 探针 `DenyExternal × *.svg × file` 由 L2 → **L0**（安全默认恢复）；两次全量跑结论一致 |
| D11 | 原判「WebP 动图帧推进未生效」 | ✅ **重新定性为探针判定抖动**：`anim-2frames.webp` 与 GIF 同源（Skia 多帧动图源）本就在推进，旧判据按「采样间隔 mod 周期」偶发同帧 → 误判 A=❌。判据稳定化后 `anim-*.gif/webp` 全部 **L4**（Browser L4 9 → 12） | 两次全量跑下降项每次不同（抖动实证）；修后连续两次「与基线一致」 |

**第三批已闭环的缺口（§9.5）**

| 编号 | 缺口 | 修复与结果 | 证据 |
|---|---|---|---|
| D10 | `<video>` **画面未绘制**（12 格 L0：帧流在推进、`loadedmetadata` 已派发，但静止态采样无画面） | ✅ **根因是「媒体资源选择算法的启动时机」**：本引擎的媒体状态是「首次访问媒体属性时惰性创建」（`mediaStateFor`），而规范的时机是「`src` 属性已设置且元素在文档中」（HTML §4.8.8）。于是用 HTML 属性写好的 `<video src>` 在脚本读属性之前**完全不加载**——静止态 `readyState=0`、`duration=NaN`、无首帧，`play()` 之后才补到 `rs=4`（同一格实测：静止 0 / 播放 4）。修法：新增 `bindings.StartMediaElementLoads`，`webkit` 在页面脚本执行后对文档里的 `<video>/<audio>` 补齐启动（幂等，动态创建的元素不受影响）。**video 由 L0 → L4**（testsrc mp4/webm、twophase），单色样本 L3 | `TestStartMediaElementLoadsStartsResourceSelection`（启动计数/幂等/无 src 不启动/`loadedmetadata` 派发）；探针 `*.mp4`/`*.webm` 行绘制列 ✅；`matrix-Browser.png` 经 `read_image` 复核（file/rel 三列有真实视频画面：testsrc 彩条、solid-red 纯红、twophase 红首段） |
| — | **（新发现·待决）媒体宿主注入通道未受资源策略约束** | ⚠️ **未闭环，上报待决**：`DenyExternal` 下 `<video src="file://…">` 仍取得元数据与画面（L3/L4），与 `<img>` 在同一策略下被严格拒绝（L0）不一致——媒体元数据探测与抽帧由宿主（`app.MediaProbe` → `ffmpeg` 按本地路径读文件）直接完成，未过资源策略门禁。该行为在 D10 修复前**就已存在**（旧报告的「动画 ✅」即帧流在跑的证据），只是当时静止态无画面而未被注意 | 探针 `DenyExternal × *.mp4 × file/rel` = L3/L4（同一策略下 `*.png` 为 L0） |

**仍存在的缺口（转入阶段 3）**

| 编号 | 缺口 | 实测表现 | Browser 下格数 |
|---|---|---|---|
| D12 | 音频**输出**后端（L4-S） | 元数据链路**已通**（第三批实测 12 格 **L1**：`rs=4`、`duration≈1s`、`loadedmetadata` 派发）；缺解码/输出后端，且 `AudioContext`/`decodeAudioData` 不存在 → 待阶段 3 立项（§9.6） | 12（L1） |
| — | AVIF / TIFF | L0（预期不支持，已入基线，不投入） | 6 |

---

## 4. 测试矩阵（格式 × 来源 × 模式 × 维度）

维度代号：**L** 加载 / **G** 几何与固有尺寸 / **P** 绘制(像素) / **E** 事件与 IDL 契约 / **A** 动画推进 / **S** 音频输出。

**执行配置（决策 1/2）**：每条用例在**四种配置**下跑 —— `Browser`、`Toolkit+DenyExternal`、`Toolkit+AllowHostResolved`、`Toolkit+AllowAll`；其中 `Browser` 与 `Toolkit+AllowAll` 必须逐像素一致。每个格式再与 **Edge 对照**（决策 2），报告输出「引擎 / Edge」双列等级。

| 格式 | data: | file:// | 相对 | 模式 | 覆盖维度 | 目标等级 |
|---|---|---|---|---|---|---|
| PNG | ✔ | ✔ | ✔ | Browser/Toolkit | L G P E | L3 |
| JPEG | ✔ | ✔ | ✔ | Browser/Toolkit | L G P E | L3 |
| GIF 静态 | ✔ | ✔ | ✔ | 同上 | L G P E | L3 |
| GIF 动画 | ✔ | ✔ | ✔ | 同上 | L G P E **A** | L4 |
| WebP(有损/无损) | ✔ | ✔ | ✔ | 同上 | L G P E | L3 |
| WebP 动画 | ✔ | ✔ | ✔ | 同上 | L G P E **A** | L4 |
| BMP / ICO | ✔ | ✔ | ✔ | 同上 | L G P E | L3 |
| SVG | ✔ | ✔ | ✔ | 同上 | L G P E | L3 |
| AVIF / TIFF | ✔ | ✔ | ✔ | 同上 | L P E（**失败路径**） | L0 且优雅降级 |
| MP4 / WebM | ✔ | ✔ | — | 同上 | L G P E **A** | L4 |
| WAV/MP3/OGG/M4A | ✔ | ✔ | — | 同上 | L E **S** | L4 |
| 损坏/0 字节 | ✔ | ✔ | ✔ | 同上 | 失败路径 + `onerror` | 契约正确 |
| 超大图(4096²) | — | ✔ | — | Browser | P（性能/内存） | L2 |

---

## 5. 测试用例清单

命名 `TC-M-<组><序号>`。每条判定等级见 §2；「证据」列是必须落盘的文件/日志。

### G1 静态光栅（每格式 × 每来源）
| 用例 | 输入 | 步骤 | 预期 | 证据 |
|---|---|---|---|---|
| TC-M-101 | `120x80` 渐变 PNG（`data:`） | 无 CSS 尺寸的 `<img>`，加载后读几何+像素 | 盒 120×82、`naturalWidth=120`、图案与源图一致 | `geom.json` + 截图 |
| TC-M-102 | 同上，`file://` | 同上 | 同上 | 同上 |
| TC-M-103 | 同上，相对路径 | 同上 | 同上 | 同上 |
| TC-M-110..112 | JPEG 三来源 | 同上 | 结构同 101（有损图用采样点+容差比对） | 同上 |
| TC-M-120..122 | GIF 静态三来源 | 同上 | 同上 | 同上 |
| TC-M-130..132 | WebP 有损三来源 | 同上 | **绘制 ✅ 但固有尺寸可能缺失（D4）** | 同上 + `naturalWidth` |
| TC-M-135..137 | WebP 无损三来源 | 同上 | 同上 | 同上 |
| TC-M-140..142 | BMP 三来源 | 同上 | 同 D4 风险 | 同上 |
| TC-M-150..152 | ICO 三来源 | 同上 | 同 D4 风险 | 同上 |
| TC-M-160 | CSS `background-image`（PNG/WebP） | 元素 100×100 | 与 `<img>` 同结论 | 截图 |

**每条额外跑三种 Toolkit 策略配置**（决策 1）：
- `Toolkit+DenyExternal`（默认）：修复前预期全 ❌；修复后**仅 `data:` 转 ✅**（此即 U1 的修复边界），其余仍 ❌ → 用于回归对比；
- `Toolkit+AllowHostResolved`：预期 `file://`/相对路径在 resolver 解析成功时 ✅，`http(s)` 仍 ❌；
- `Toolkit+AllowAll`：预期与 `Browser` **逐像素一致**（证明策略开关不引入渲染差异）。

### G2 SVG
| 用例 | 输入 | 预期 | 备注 |
|---|---|---|---|
| TC-M-201 | `data:image/svg+xml`（纯色 rect） | ✅ 渲染（L3） | 引擎自有矢量路径 |
| TC-M-202 | `file://` SVG | 当前 ❌（D3） | 与 Edge 对照应有图 |
| TC-M-203 | 相对路径 SVG | 当前 ❌（D3） | 同上 |
| TC-M-204 | 带 `width/height` 的 SVG 固有尺寸 | 盒尺寸 = 声明尺寸 | — |
| TC-M-205 | `<img src=svg>` + `object-fit:cover`（`video-poster` 既有回归场景） | 裁剪正确 | 已有参考用例 |

### G3 动图（帧推进）
| 用例 | 输入 | 步骤 | 预期 |
|---|---|---|---|
| TC-M-301 | 3 帧 GIF（红/绿/蓝，delay 100ms） | 加载后连拍 5 帧（间隔 120ms），比对像素 | **基线**：恒红（首帧）→ L2；**目标**（决策 3，`SkCodec` 落地后）按 100ms 换帧 → **L4** |
| TC-M-302 | 2 帧 WebP 动画 | 同上 | 同上——**WebP 动画必须走 SkCodec 通用路线**，Go `image/gif` 逐帧方案覆盖不到 |
| TC-M-303 | 动图 + `image-rendering: pixelated` | 同上 | 换帧正确 + 最近邻缩放正确 |
| TC-M-304 | 动图在 `<canvas>` 上 `drawImage` | 同上 | canvas 取到的应是**当前帧**（随播放推进变化） |
| TC-M-305 | 3 帧 GIF 且 delay 不均匀（50/200/100ms） | 连续 6 次截图，比对换帧时刻 | 时间轴按各帧 delay 推进（需 `SkCodec` 提供 `frameInfo.duration`） |
| TC-M-306 | 动图 + `visibility:hidden` 后恢复 | 隐藏 300ms → 恢复 → 比对像素 | 记录隐藏期间是否继续推进（规范：`<img>` 动图不因不可见而暂停） |
| TC-M-307 | 循环 GIF（loop=0）与不循环（loop=1） | 播放过末尾后读像素 | 循环正确；不循环停在末帧不重头 |

### G4 不支持格式的失败语义
| 用例 | 输入 | 预期 |
|---|---|---|
| TC-M-401 | AVIF/TIFF × 3 来源 | 不渲染、无异常；`onerror` 是否派发须记录（当前 `complete=false`） |
| TC-M-402 | 0 字节文件 | 同上 |
| TC-M-403 | 损坏 PNG（截断 50%） | 同上 |
| TC-M-404 | 扩展名与内容不符（`.png` 实为 JPEG） | 按内容解码（引擎不看扩展名）→ 应渲染成功 |
| TC-M-405 | 图片路径不存在 | 不渲染 + 无控制台报错刷屏 |

### G5 视频
| 用例 | 输入 | 步骤 | 预期 |
|---|---|---|---|
| TC-M-501 | `<video src=mp4>`（无 poster） | 加载 → 读 `readyState/networkState/duration/error` | **基线**：`rs=0`、`duration=NaN` → L0；**目标**（阶段 3）：`rs=4`、`duration≈1s` → L1 —— ✅ **实测达成（2026-10-07，A0）**：`rs=4`、`duration≈1`、`videoWidth×Height=120x80`、事件序列 `loadstart→durationchange→loadedmetadata→loadeddata→canplay` |
| TC-M-502 | `<video poster=png src=mp4>` | 同上 + 截图 | poster 是否绘制（基线灰块待确证；阶段 3 应正确绘制）—— ✅ **实测达成（2026-10-07，A1）**：有 poster 未播放时截图中心像素 = poster 色（纯红）；**无 poster** 时截图中心像素 = 宿主注入的当前帧（值取自独立 ffmpeg 抽帧，非手写期望值）→ **L2 达成**；判据 A1-1/2/3 见 `_temp/psai.exe -verify` |
| TC-M-503 | `video.play()` | 读 Promise 结果 + 事件序列 | **基线**记录实际序列；**目标**：`loadstart→loadedmetadata→loadeddata→canplay→playing` + `timeupdate` 推进 —— ✅ **实测（2026-10-07，A2 起步）**：事件序列 `play→playing→timeupdate×4→ended`，`currentTime` 0→1.00s 递增（A2-3 判据） |
| TC-M-504 | `video.currentTime = 0.5` | 读 `seeking/seeked` 与几何 | **基线**不退进；**目标**画面跳到 0.5s 帧 —— ✅ **画面部分实测达成（2026-10-07，A1-3）**：`currentTime=0.5` 后截图中心像素 = 0.5s 参照帧（同一纯色样本，参照值由独立 ffmpeg 抽帧得到）；`seeking/seeked` 事件与连续换帧（L4）仍属 A2 |
| TC-M-505 | `http(s)` 源 | 读 `error.code` | 保留：`MEDIA_ERR_SRC_NOT_SUPPORTED`（`media_element.go:304-309`） |
| TC-M-506 | `<video>` + `controls` 属性 | 截图 | 引擎是否绘制原生控件（基线预期否；阶段 3 再定） |
| TC-M-507 | 连续 1s 内取 5 帧截图 | 像素差异 | **目标**：画面随时间变化 → **L4** —— ◐ **部分达成（2026-10-07，A2 起步）**：播放中画面确实随时间变化（两段式样本：播放前首帧 = 前段色、播放到结束 = 后段色，各自与独立抽帧的参照一致）；「1s 内连续 5 帧」的密集采样判据尚未落地 |

### G6 音频
| 用例 | 输入 | 步骤 | 预期 |
|---|---|---|---|
| TC-M-601 | `<audio src=wav/mp3/ogg/m4a>` | 读 `readyState/duration/paused` | **基线**全 0/NaN → L0；**目标**（阶段 3）：`rs=4`、`duration≈1s` → L1 |
| TC-M-602 | `audio.play()` 后 500ms | 读 `currentTime` | **基线**不推进（duration=NaN 时空转，`media_element.go:420-423`）；**目标**推进 ≈0.5 |
| TC-M-603 | `AudioContext` / `decodeAudioData` | 探测 API 存在性 | **基线**不存在（无音频后端）；**目标**（阶段 3 若含 WebAudio）存在 |
| TC-M-604 | 系统输出设备录音比对（需环回设备） | 播放 1s 正弦，采回波形 | **L4-S 判据**：频谱主峰 ≈ 440 Hz |

> **决策 4 落点**：G5/G6 基线全部为 L0；因已确认「要真实播放」，本节即阶段 3 的**验收清单**——验收标准为「目标」列全部达成。

### G7 尺寸与响应式
| 用例 | 输入 | 预期 |
|---|---|---|
| TC-M-701 | WebP/BMP/ICO 不给尺寸的 `<img>` | 记录盒尺寸（验证 D4 是否塌陷） |
| TC-M-702 | `width:100%` + `object-fit: contain/cover/fill/none` 四值 × PNG | 与 Edge 对照裁剪一致 |
| TC-M-703 | `srcset` + `sizes`（PNG 1x/2x） | 选源与折算正确（`replaced.go:89-124` 已实现折算） |
| TC-M-704 | 中文文件名 + 中文目录 | 能加载（Windows 路径编码） |
| TC-M-705 | 路径含空格 / `..` 穿越 | 能加载 / 被 resolver 拒绝（安全） |

### G8 契约（事件与 IDL）
| 用例 | 输入 | 预期 |
|---|---|---|
| TC-M-801 | `<img>` 成功加载后读 `complete`/`naturalWidth` | **应 true/>0**（当前 ModeBrowser file:// 为 false/0 → D1） |
| TC-M-802 | `<img onload>` 计数 | 应派发 1 次 |
| TC-M-803 | `<img onerror>` + 不存在的文件 | 应派发 `error`（当前是否派发待测） |
| TC-M-804 | `new Image(); img.src=...` 动态创建 | 同 801 |
| TC-M-805 | 同一 URL 两个 `<img>` | 两者状态都正确（当前 §3.2 报告显示同 URL 元素状态不一致，须复测） |

### G9 资源策略开关（决策 1 新增）
| 用例 | 输入 | 预期 |
|---|---|---|
| TC-M-901 | `Toolkit` + `DenyExternal` + `data:` 图片 | ✅ 渲染（`data:` **无条件放行**是本项修复的核心）—— ✅ **已修复并实测（2026-10-07，A1）**：ModeToolkit 下 `<video poster="data:image/png;base64,…">` 已正确绘制（A1-2 判据的中心像素 = 纯红）；修复点是门禁顺序（`data:` 判断前移到 `AllowsExternal` 之前） |
| TC-M-902 | `Toolkit` + `DenyExternal` + `file://` / 相对路径 / `http(s)` | ❌ 一律不渲染（安全默认不变） |
| TC-M-903 | `Toolkit` + `AllowHostResolved` + resolver 能解析的 `file://` | ✅ 渲染；resolver 返回空的 URL 仍 ❌ |
| TC-M-904 | `Toolkit` + `AllowHostResolved` + `http(s)` | ❌（策略只放行宿主**明确提供**的资源，不代宿主联网） |
| TC-M-905 | `Toolkit` + `AllowAll` 与 `Browser` 同页对照 | 两图**逐像素一致** |
| TC-M-906 | 默认值回归：不设置开关时的 `ModeToolkit` | 与改动前一致，**唯一差异**是 `data:` 由 ❌ 转 ✅（属 bug 修复，非行为放宽） |
| TC-M-907 | 策略在 WebView 构造后切换 | 切换即生效（新资源按新策略；已缓存资源不重算） |

---

## 6. 工具链设计

### 6.1 已有资产（本次盘点产出，可直接复用）

| 资产 | 路径 | 用途 | 复现命令 |
|---|---|---|---|
| 解码探针 | `_temp/mediacheck/main.go` | 格式 × Skia/Go 解码矩阵 | `go run ./_temp/mediacheck/main.go` |
| 端到端探针 | `_temp/mediacheck/render/main.go` | 三配置真实渲染 + 出 PNG + IDL 报告（落地时扩为 **A/B/C/D 四配置**） | `go run ./_temp/mediacheck/render/main.go` |
| 样本库 | `_temp/mediacheck/samples/`（21 个文件） | 全格式样本 | 见 6.2；**不入库**（决策 5） |
| 证据 | `out.txt` / `out2.txt` / `render-out.txt` / `render/*.png` | 判定依据 | — |
| 宿主工装 | `cmd/psai`（`-audit` / `-verify` / `-png` / `-w` / `-h`） | 无头加载 + EvalJS + `wv.Render()` 出图 + 交互模拟 | 见 `.pair/project.md` |
| 视频帧自检 | `cmd/psai` 的 `mediaFrameSelfCheck`（新） | 判据 A1-1/2/3：无 poster 出画面 / poster 优先 / seek 后让位；期望像素由**独立 ffmpeg 抽帧**给出（不手写），取样点做 `elementFromPoint` 遮挡校验 | `./_temp/psai.exe -verify -png ""` |

环境前提：`CGO_ENABLED=1` + `PATH=F:\syproject\goskia\bin;%PATH%`；样本生成需 **PIL 11.3.0** 与 **ffmpeg**（本机均已具备）。

### 6.2 待固化的工具（落地计划的一部分）

1. **样本生成脚本（入库）** `dev/media/gen_samples.py`：PIL 生成 png/jpeg/gif(静态+动画)/webp/bmp/ico/tiff/avif/svg，ffmpeg 生成 mp4/webm/wav/mp3/ogg/m4a（各 1 秒 testsrc/sine）。
   ★ **决策 5：脚本入库、样本不入库**——样本生成到 `dev/media/samples/`（进 `.gitignore`），报告产物到 `dev/media/out/`（同样忽略）。理由：样本随仓库走会让二进制长期堆积且难以更新；固定生成参数 + 入库脚本即可保证「同一份样本、可重复判定」。
2. **媒体验证子命令** `cmd/psai -media`（或新 `cmd/mediaprobe`）：
   - **四配置**（Browser / Toolkit+DenyExternal / Toolkit+AllowHostResolved / Toolkit+AllowAll）各跑一遍矩阵页，并与 **Edge 对照**（决策 2），产出「引擎 / Edge」双列等级；
   - 每格式产出：几何 JSON、`complete/naturalWidth`、事件日志、截图 PNG；
   - 自动判定 L0–L4 并输出 Markdown 报告；
   - **回归比对（本机按需，非 CI 门禁）**：把 §3 的实测结论写成期望表（baseline），任何格式的等级升降都让 `-media` 非零退出，供开发者手动比对。
3. **像素比对器**：从 `wv.Render()` 读回预乘 RGBA（照 `cmd/psai/main.go:330` 的 `renderPNG` 反预乘），对每格做「命中色比例」判定（避免依赖人眼），并保留 PNG 供 `read_image` 复核。
4. **视觉验证**：`read_image` 看截图（本次已用它确证 A/B 全灰、C 组三列有色）。
5. **事件日志**：页面侧挂 `onload/onerror` 计数 + `MutationObserver`，结果回传 Go 侧写 JSON。

### 6.3 证据纪律

- 每条用例必须有：`几何 JSON` + `截图 PNG` + `事件计数`，缺一不算通过；
- 判定 L4（动态）必须**两张以上时间点截图**并给出像素差异（不接受「应该有动画」）；
- 对照基准：能安装 Edge 的场景用 `dev/suites/consistency` 既有双端对照机制（`edgeCollect` / `wbuiCollect`）。
- 决策 2：媒体验证同样复用上述机制，产出「引擎 / Edge」双列报告（见 §6.4）。

### 6.4 触发与执行方式（决策 6 修订：不入 CI 门禁）

**决策 6（2026-10 修订）**：媒体验证**不纳入 CI 自动门禁**——CI 属 git 自动 CI 范畴，本方案不写 CI 接线脚本、不把验证接进 `go test`/CI 门禁。交付物是**可复现的按需执行工装**：由开发者手动触发。

**执行方式**：

1. **入口（手动/按需）**
   - `cmd/psai -media`：四配置真实渲染 + 像素比对 + Edge 对照 + 出 Markdown 报告；
   - 或 `go test -tags=media -run TestMediaE2E`（等价入口，便于本地调试）；
   - 两者均**不进默认 `go test ./...`**（需 CGO + 样本 + 耗时），**也不挂 CI**。
2. **样本准备（决策 5）**
   - 入库：`dev/media/gen_samples.py`、`-media` 探针、基线期望表（纯文本 JSON，几 KB）；
   - 不入库：`dev/media/samples/`、`dev/media/out/`（写入 `.gitignore`）；
   - 执行前手动运行 `python dev/media/gen_samples.py --out dev/media/samples`（需 Pillow + ffmpeg）；缺 ffmpeg 时视音频用例标 `SKIP(no-ffmpeg)` 而非失败。
3. **轻量纯逻辑单测（可选）**：策略开关、契约（`complete`/`naturalWidth`/事件）等纯逻辑单测不需真实渲染，本身即普通单元测试；**是否随 git 自动 CI 执行属 CI 范畴，本方案不做接线、不做承诺**。
4. **Edge 对照降级**：无 Edge 环境时用 `-media -edge=false` 只跑引擎侧并标 `SKIP(no-edge)`；基线期望表记录「引擎等级」与「Edge 等级」两列，供人工比对。
5. **报告去向**：报告为**本地产物**（不入库、不上传），供人工复核；需长期留存时由开发者自行归档。

---

## 7. 报告模板

```markdown
# 媒体格式真实可用性报告（<日期> <commit>）

## 环境
模式：Browser / Toolkit+DenyExternal / Toolkit+AllowHostResolved / Toolkit+AllowAll ｜ 视口：1200×1250 ｜ 引擎：<hash> ｜ Edge：<版本 | SKIP(no-edge)> ｜ 样本：本地脚本生成、不入库（决策 5）

## 总表
| 格式 | 来源 | 模式 | 加载 | 几何 | 绘制 | 契约 | 动画 | 音频 | 引擎等级 | Edge 等级 | 备注 |
|---|---|---|---|---|---|---|---|---|---|---|---|
| PNG | data: | Browser | ✅ | ✅ | ✅ | ✅ | — | — | L3 | L3 | |
| WebP | file:// | Browser | ✅ | ❌ | ✅ | ❌ | — | — | L2 | L3 | D4 固有尺寸缺失 |
| PNG | data: | Toolkit+DenyExternal | ✅ | ✅ | ✅ | ✅ | — | — | L3 | L3 | 修复前为 L0（D2 门禁） |
| PNG | file:// | Toolkit+AllowHostResolved | ✅ | ✅ | ✅ | ✅ | — | — | L3 | L3 | 需宿主显式声明 |
| PNG | file:// | Toolkit+DenyExternal | ❌ | ❌ | ❌ | ❌ | — | — | L0 | L3 | 安全默认：预期行为 |
| GIF 动画 | file:// | Browser | ✅ | ✅ | ✅(首帧) | ❌ | ❌ | — | L2 | L4 | D7 无帧推进（决策 3 待修） |
| MP4 | file:// | Browser | ❌ | — | ❌ | — | ❌ | — | L0 | L4 | 无解码器（阶段 3 待修） |

## 缺陷清单（按影响排序）
D1 … （现象 / 证据文件 / 影响面 / 建议）

## 与基线的差异
<格式>: L2 → L3（本次提升/回退），对应 commit …

## 原始证据索引
- 解码矩阵：<文件>
- 端到端报告：<文件>
- 截图：<文件列表>
```

---

## 8. 不可用项清单 + 新一轮落地计划（已按七项决策定型）

### 8.1 不可用项（按优先级）

| 级别 | 编号 | 问题 | 现状 | **修法/路线（已确认）** | 影响面 | **复测状态（2026-10，§3.4）** |
|---|---|---|---|---|---|---|
| **P0** | U1 | ModeToolkit 下光栅图片一律不渲染（含 `data:`） | A/B 组实测全灰 | **决策 1：(c) 新增资源策略开关**，`data:` 无条件放行、默认值不变（§8.2 阶段 1） | AI-PS 及一切 UI 库模式宿主无法显示任何位图 | ✅ **已闭环**（`data:` 在 Toolkit 下 L3/L4；`AllowHostResolved`/`AllowAll` 开关可用） |
| **P0** | U2 | 图片契约失效：`complete`/`naturalWidth` 恒 false/0、`onload` 疑似不派发 | C 组报告 vs 截图矛盾 | 阶段 1：按「解码缓存就绪」修 IDL 反射与事件派发 | 所有依赖图片加载事件的业务逻辑失效 | ✅ **已闭环**（契约列 0 → 66 ✅；`load`/`error` 均派发） |
| **P1** | U3 | WebP/BMP/ICO 无固有尺寸（两套 codec 集不一致） | 实测 Go DecodeConfig ❌ | 阶段 2：Skia 优先 + Go 兜底（或注册 `x/image/webp`+`bmp`） | 未给尺寸的 `<img>` 塌陷 | ✅ **已闭环**（Skia `DecodeSize` 兜底，三者 L3；SVG 固有尺寸由 **D8** 补齐，§9.4） |
| **P1** | U4 | 动图（GIF/WebP）不推进帧 | 无 codec/frame API | **决策 3：goskia 暴露 `SkCodec`**（跨仓库联动，§8.2 阶段 2） | 动图退化为静态图 | ✅ **已闭环**（GIF 与 WebP 动画均达 **L4**；原 D11「WebP 未推进」经两次对照实证为**探针判定抖动**，判据已稳定化，§9.4） |
| **P1** | U5 | `svg` 仅 `data:` 可渲染 | C 组第三列空白 | 阶段 2：`loadBackgroundSVG` 补 `file://`/相对路径分支 | 文件引用 SVG 图标不显示 | ✅ **已闭环**（file/rel 已绘制；**D8** 补齐固有尺寸与 `load`/`complete` 契约 → SVG 九格 **L2 → L3**，§9.4） |
| **P2** | U6 | 视频：无解码器、无画面（除 poster）、`duration=NaN` | 实测 rs=0 | **决策 4：要真实播放** → 阶段 3 单独立项（宿主注入 vs 内置 ffmpeg） | 任何 `<video>` 场景不可用 | 🟡 **部分闭环**（A0/A1：元数据与帧流在跑；探针采样点仍无画面 → D10） |
| **P2** | U7 | 音频：无解码、无输出后端 | 实测 rs=0，goskia 无 audio | **决策 4：要真实播放** → 阶段 3 单独立项（音频繁重最高） | 任何 `<audio>` 场景不可用 | ❌ **未闭环**（12 格 L0，需音频后端） |
| **P2** | U8 | `MediaMetadataResolver` 宿主未注入 | 仅定义+单测 | 阶段 3 前置：宿主接 `ffmpeg -i` 探测时长（一次赋值） | 即使本地媒体也拿不到时长 | ✅ **已闭环**（A0：探针与 psai 均装配该 resolver） |
| **P3** | U9 | AVIF/TIFF 不支持 | 实测 ❌ | 维持不支持（写入基线，不投入） | 新格式资源不可用（可接受） | ✅ **维持**（6 格 L0，已入基线） |

### 8.2 落地计划（分阶段，已按七项决策定型）

**阶段 0：验证工装落地（决策 5/6；低风险）**
- **入库**：`dev/media/gen_samples.py`（样本生成脚本）、`-media` 四配置探针、基线期望表（纯文本 JSON）；
- **不入库**：`dev/media/samples/`、`dev/media/out/`（写入 `.gitignore`）；
- **执行方式**：本机按需（`python dev/media/gen_samples.py` → `cmd/psai -media`），**不接 CI 门禁**（决策 6 修订，详见 §6.4）；
- **归属**：本工装与等级验证属**独立媒体验证项目**，不在 AI-PS 项目内（§0.2）；
- **产出**：全格式基线报告（A/B/C 三组 PNG + 新增 **D 组 = Toolkit+AllowHostResolved**）；
- **验收**：`-media` 在本机跑通并产出报告（无 Edge/ffmpeg 的项标 SKIP 而非失败）；工装合入后默认 `go test ./...` 不受影响。
- 风险：低（只加验证代码，不动引擎）。

**阶段 1：P0 修复 — 资源策略开关与图片契约（决策 1）**

*U1（修法已定：(c) 新增模式开关）*

现状：`AllowsExternal()` → `mode.allowsExternalURLs()` → `m != ModeToolkit`，且门禁**位于 `data:` 解析之前**（`backgroundimage.go:350`）。

API 草案：

```go
// webkit/resource_policy.go（新增）
type ResourcePolicy int
const (
    DenyExternal    ResourcePolicy = iota // 默认：拒 http(s)/file/相对路径；data: 无条件放行
    AllowHostResolved                     // 仅放行宿主 resolver 明确解析出的资源
    AllowAll                              // 等价 ModeBrowser
)
func (wv *WebView) SetResourcePolicy(p ResourcePolicy)
```

三条兼容原则：
1. **`data:` 与开关解耦**——`data:` 自包含、不经任何外部通道，**无条件放行**（属 bug 修复，不属行为放宽）；
2. **默认值保持现状**——`ModeBrowser → AllowAll`；`ModeToolkit → DenyExternal`（与今天完全一致，唯一差异是 `data:` 由 ❌ 转 ✅）；
3. 需要读盘的宿主必须**显式**声明 `AllowHostResolved`，安全默认不被静默放宽。

改动点：`webkit/mode.go:66` 拆为 `AllowsDataURI()`（恒 true）与 `AllowsExternalURLs()`（按策略）；`engine/rendering/backgroundimage.go:349-358` 把 `data:` 判定提到门禁之前；`webkit/image_resource.go:88-93` 转发策略。
验收：轻量单测（TC-M-901..906）+ 端到端 A/B/C/**D** 四组 PNG 对照（防「修 A 破 B」）。

*U2（契约）*：修 `complete`/`naturalWidth`/`onload` 的 IDL 反射与事件派发，使其以「解码缓存就绪」为准（`DecodedImage.Loaded()` 已可判定）；须同时覆盖「同一 URL 多个 `<img>`」场景（TC-M-805）。

**阶段 2：P1 修复 — 格式一致性、动图、SVG（决策 3）**
- **U3**：布局侧固有尺寸探查改为「Skia 优先、Go 兜底」（或注册 `golang.org/x/image/webp` + `bmp`），保证「能画的就能量尺寸」；
- **U4（路线已定：goskia 暴露 `SkCodec`）**，四步链路：
  1. goskia 侧新增 codec API（`frameCount` / 逐帧解码 / `frameInfo.duration` / `loopCount`），提交并 `git push origin main`；
  2. wb-ui 侧 `GOWORK=off go get github.com/hoonfeng/goskia@<短hash>`；
  3. `GOWORK=off go build ./...` 与测试（依赖联动细节见 `.pair/project.md`）；
  4. 引擎侧接帧推进（复用已实现的 RAF/动画帧队列，`engine/js/jsc/eventloop.go:207`）；
- **U5**：`loadBackgroundSVG` 解析分支补 `file://` 与相对路径（当前仅 `data:` 可渲染）；
- 交付物：每项单测 + 报告等级提升（U4：GIF/WebP 动画 L2→**L4**；U3：WebP/BMP/ICO L2→L3）。

> **状态（2026-10）：阶段 2 已完成。** U3/U5 见 §9.3，U4 + D8/D9 + 「SVG 解析缓存绕过
> 策略门禁」见 §9.4。交付物达成：SVG 九格 **L2 → L3**、GIF/WebP 动画 **L4**、
> 全 384 格 **L2 归零**（详见 §3.4 总表）。

> **状态（2026-10 续）：视频（A0/A1/A2）已在验证矩阵侧闭环。** D10 见 §9.5：
> `<video>` 的本地两来源（file/rel）由 **L0 → L4/L3**，宿主帧源注入通道端到端
> 可用；音频（A3）仍是阶段 3 的主体（12 格 L0，需音频后端）。

**阶段 3：P2 大工程 — 视频/音频真实播放（决策 4，单独立项）**

> ★ 本条的分期（A0–A4）、依赖（goskia/ffmpeg/音频后端）与验收判据已并入
> [docs/implementation-path.md](implementation-path.md) §2「主线 A」；本节保留**选型细节**（a/b 两条路线）。

- 候选路线：
  - **(a) 宿主注入帧/音频流**——沿用 `media_element.go` 注释中的 vcam/ffmpeg 帧注入思路，引擎只做状态机 + 画布，最小侵入、不背编解码负担；
  - **(b) 引擎内置 ffmpeg**——能力最全，但体积/许可/跨平台成本高。
- **U8（前置，成本极低）**：宿主用 `ffmpeg -i` 探测时长/尺寸并注入 `MediaMetadataResolver`，先让 `duration` 与 `loadedmetadata` 正确（TC-M-501/601 的 L1 目标）；
- 交付物：选型文档 + 原型 + 按 §5 的 G5/G6「目标」列逐条判定（video 先争 L1→L2→L4；音频 L1→L4-S）；
- 风险：高（体积、许可、平台差异、音频后端需新建）。**必须单独审核后再动。**

### 8.3 每阶段统一的验收方式

1. `cmd/psai -media` 在**四配置**（Browser / Toolkit+DenyExternal / Toolkit+AllowHostResolved / Toolkit+AllowAll）下全量跑，并与 **Edge 双端对照**（决策 2；无 Edge 时标 `SKIP(no-edge)`）；
2. 报告等级与 §8.1「修法/路线」的目标一致，且**不低于 Edge 等级**（如 U1 修复后：PNG@Toolkit+DenyExternal@data: ≥ L3，且 = Edge 等级）；
3. 截图经 `read_image` 人眼复核；
4. 默认 `go test ./...` 保持全绿（当前 23 包，作为回归底线，不受工装影响）；四配置渲染为**本机按需**执行（决策 6 修订：不入 CI 门禁）。

---

## 9. 决策记录与遗留事项

### 9.1 已确认决策（用户选定）

| # | 决策项 | 结论 |
|---|---|---|
| 1 | U1 门禁修法 | (c) 新增资源策略开关，由宿主声明；`data:` 无条件放行、默认值不变 |
| 2 | 验证范围 | 纳入 Edge 双端对照（复用 `dev/suites/consistency`）；无 Edge 时标 `SKIP(no-edge)` |
| 3 | U4 动图路线 | goskia 暴露 `SkCodec`（通用，含 WebP 动画） |
| 4 | 视频/音频目标 | 要真实播放（阶段 3 单独立项） |
| 5 | 样本/产物入库 | 不提交样本与报告；只入库生成脚本与探针 |
| 6 | 验证触发方式 | **不入 CI 门禁**（CI 属 git 自动 CI 范畴）——按需本机执行（2026-10 修订，原为「纳入 CI 自动门禁」） |
| 7 | 项目归属（2026-10 新增） | 格式能力等级（L0–L4）验证**不在 AI-PS 项目内**进行，另开**独立媒体验证项目**；AI-PS 仅做本机可用性盘点（§0.2） |

> 上述七项已定型（其中第 6 项经 2026-10 修订、第 7 项为 2026-10 新增），不再是开放问题；如需变更请显式提出。回写位置见 §0.1、§0.2。

### 9.2 原遗留 3 项 —— 已采纳（2026-10 用户确认）

用户已确认**同意以上三项建议**，该组不再是开放问题：

1. **U1 开关默认值** ✅ 采纳：`ModeToolkit → DenyExternal`（向后兼容）+ `data:` 无条件放行。AI-PS 若要开箱即可读本地图片，由 **AI-PS 宿主显式设 `AllowHostResolved`**（改动 1 行），不放宽全局默认——其他 UI 库宿主仍受安全默认保护。
2. **阶段 3 实现路线** ✅ 采纳「立项时定」：宿主注入帧/音频流（最小侵入，需宿主自带解码）vs 引擎内置 ffmpeg（体积/许可成本高），在阶段 3 立项时按当时约束选定。
3. **CI 环境能力** ✅ 随决策 6 修订作废：不入 CI 门禁 → 不再需要 runner 具备 Edge/ffmpeg；Edge 对照与视音频用例改为**本机按需**执行（无 Edge 时标 `SKIP(no-edge)`）。

---

### 9.3 本轮（2026-10）验证与修复记录

| 事项 | 结论 | 证据 |
|---|---|---|
| 四配置 × 96 格复测 | Browser：**L3 42 / L4 9** / L2 9 / L0 36；`Toolkit+DenyExternal` 保持安全默认（file/rel 全拒，仅 `data:` 放行） | `dev/media/out/report.md`、`matrix-*.png` |
| 回归基线 | 384 条（配置 × 样本 × 来源）等级期望表，等级下降即非零退出 | `dev/media/baseline.json` |
| **U2** `<img>` load/error 契约 | ✅ 已闭环：成功派发 `load`、失败派发 `error`、`complete`/`naturalWidth` 正确 | `webkit/img_event_dispatch_test.go`（4 用例）+ 探针失败路径备注 |
| **U5 / D3** SVG `file://` 与相对路径 | ✅ 已闭环：走与栅格图同一条 loader 链（9 格由 L0 → L2） | 探针 `*.svg` 行的 file/rel 列 D=✅ |
| **D4** WebP/BMP/ICO 固有尺寸 | ✅ 已闭环：Skia `DecodeSize` 兜底（三格式 × 三来源 = 9 格 L3） | 探针 `quad.bmp` / `quad-*.webp` / `quad-64.ico` |
| **R1**（引入即发现并修掉的回归） | SVG 探测在 paint 线程同步 `loader.Load` → 栅格图/远端引用被同步取字节，`Render()` 扣住整个 HTTP 超时（实测 30 s）且重复发请求；已收紧为「本地 + `.svg` 扩展名」 | `TestAsyncImageLoadMarksFrameDirty`：HEAD PASS(0.12 s) → 引入后 FAIL(60 s) → 修复后 PASS(0.12 s) |
| 探针 3 处测量伪影 | 已修：①`settleReal` 不渲染即采集；②内联事件属性用双引号字面量作参数导致属性提前闭合；③采样只支持 quad/solid（动图与渐变样本误判未绘制） | 契约列 **0 → 66 ✅**；Browser 下 L3/L4 由 **0 → 51** 格 |
| 全量测试 | `GOWORK=off go test ./... -count=1`：**仅 webkit 3 个 pre-existing 失败**（已用 HEAD 版本实证同样失败，非本轮引入） | `TestButtonTextVerticalCenter` / `TestCM6RangeMeasurementMatchesSkia` / `TestCheckedStateInvalidatesStyle` |
| 视觉与像素复核（§8.3 第 3 条） | `Browser` 与 `Toolkit+DenyExternal` 截图经 `read_image` **人眼复核**：图案只出现在预期列（后者仅 `data:` 列有内容，file:// 与相对路径全为灰底）；另对 Browser 的 95 格做**自动化像素核验**（报告的绘制判定 vs 截图中心像素）→ **0 处矛盾** | `matrix-Browser.png`、`matrix-Toolkit-DenyExternal.png` |

仍存缺口见 §3.4 末表（D10/D12 与 AVIF/TIFF），转入 §8.2 阶段 3。

### 9.4 第二批修复（2026-10，接续 §9.3）

| 事项 | 结论 | 证据 |
|---|---|---|
| **D8** SVG 的固有尺寸与契约 | ✅ 已闭环。渲染层新增 `SVGReferenceIntrinsicSize` / `IsSVGReferenceReady`（解析顺序：`width/height` → `viewBox` → CSS 默认尺寸 300×150）；`bindings` 增加「资源固有尺寸 hook」（`imgNaturalDim`：位图优先、SVG 兜底），`naturalWidth`/`naturalHeight`/`complete` 全部改走它；`webkit` 侧接线 hook + `flushImageEvents` 的 SVG 分支 + 失败通知按「能渲染即加载成功」纠正（此前 SVG 会被 Skia 按位图解码判失败而派发 `error`）。**SVG 九格 L2 → L3** | `webkit/img_svg_contract_test.go`（complete/naturalWidth=24/viewBox=40×20/load=1/err=0）、`TestSVGReferenceIntrinsicSize`；探针 `*.svg` 行「加载/几何/绘制/契约」四列全 ✅ |
| **D9** 内联 SVG（非 base64）不绘制 | ✅ 已闭环，且**根因是探针伪影而非引擎缺陷**：矩阵页把 data URI 里的 `"` 写成 JS 风格 `\"`，HTML 解析器在第一个 `"` 处结束属性值（`src` 只剩 `data:image/svg+xml,<svg xmlns=`）。修法：data URI 按 **RFC 2397 percent 编码**（`url.PathEscape`）+ 属性值改 `htmlAttrValue`（HTML 实体）。L0 → **L3** | 修复前后矩阵页 HTML 片段对照；探针 `inline-svg-data-uri` 行 |
| 引擎：data URI 载荷的 `+` 语义 | ✅ RFC 2397 里 data URI 载荷是 percent-编码文本，`+` **保持字面**（form-encoding 才把 `+` 当空格）。原先用 `QueryUnescape`，SVG 内容里字面的 `+`（`transform="translate(+1,2)"` 等）会被吃成空格。改为 `PathUnescape` 优先、解析不出 SVG 时才回退 `QueryUnescape`（兼容按 `QueryEscape` 生成的既有 URI） | `TestDecodeDataURITextKeepsLiteralPlus`、`TestLoadBackgroundSVGPercentEncoded`（既有 `TestLoadBackgroundSVGURLEncoded` 走回退路径仍绿） |
| 引擎：**SVG 解析缓存绕过资源策略门禁**（D8 修复过程中暴露） | ✅ 解析结果缓存是包级全局、跨 WebView/配置共享，而缓存查询原先在门禁**之前**：Browser 先跑并解析过的 SVG 会被后续 `Toolkit+DenyExternal` 命中缓存拿到，于是同一格出现「加载/几何/契约 ✅ 而绘制 ❌」。缓存查询移到门禁之后（`data:` 与策略无关，保留在自身分支）。`DenyExternal × *.svg × file` 由 L2 → **L0**（安全默认恢复） | 探针 `DenyExternal × *.svg` 三来源全 L0；两次全量跑结论一致 |
| 探针：动图判定稳定化（第 4 处测量伪影） | ✅ 旧判据取「静止/播放/再等 900ms」三个**单点**像素，是否判「推进」取决于「采样间隔 mod 动图循环周期」——3 帧×100ms 的 GIF 周期 300ms、900ms 恰为其 3 倍 → 两拍同帧 → 误判未推进。**抖动实证**：同一 HEAD 连跑两次，下降项每次不同（`anim-3frames-rgb` L4/L3 摇摆、`anim-2frames.webp` 亦然）。修法：等间隔连拍 6 帧（130ms，不与常见帧时长成整数倍）+ 采样点由 1 个增到 5 个 + 判据放宽为「任意两帧不同」。修后**连续两次全量跑均「与基线一致」**，且三个非拒配置等级分布完全一致（35/0/0/49/12） | `runMediaConfig` ③、`framesDiffer`/`samplePointsOf`/`framesMaxDiff`；两次跑输出对照 |
| 基线维护 | `DenyExternal × *.svg × file` 三条期望值原为缺陷放行产生的 L2，已随正确行为更新为 L0（其余 381 条不变） | `dev/media/baseline.json` diff |
| 四配置复测（第二批后） | Browser / AllowAll / AllowHostResolved 完全一致：**L0 35 / L2 0 / L3 49 / L4 12**；`DenyExternal`：L0 75 / L3 17 / L4 4。**L2 全部归零** | `dev/media/out/report.md`（384 行） |

### 9.5 第三批修复（2026-10，接续 §9.4）：`<video>` 画面（D10）

| 事项 | 结论 | 证据 |
|---|---|---|
| **D10** `<video>` 画面未绘制 | ✅ 已闭环。根因**不是**「帧流没推进」，而是「媒体资源选择算法的启动时机」：本引擎把规范的「`src` 已设置且元素在文档中即启动加载」（HTML §4.8.8）推迟到「首次访问媒体属性」（`mediaStateFor` 惰性创建）——用 HTML 属性写好的 `<video src>` 在脚本读属性之前根本不加载，静止态 `readyState=0`、`duration=NaN`、无首帧；`play()` 走 `media_element.go` 的补触发路径才到 `rs=4`（同一格实测：静止 0 / 播放 4，盒尺寸 120×80 正确）。修法：新增 `bindings.StartMediaElementLoads(in, doc)`，`webkit.loadHTMLFrom` 在页面脚本执行后对文档中已连接的 `<video>/<audio>` 启动资源选择（幂等；无 src 不启动；动态创建的元素仍走各自属性路径） | `TestStartMediaElementLoadsStartsResourceSelection`；探针 `*.mp4`/`*.webm` 行：加载/几何/契约 ✅、绘制 ✅、动画 ✅；`matrix-Browser.png` 经 `read_image` 复核 |
| 探针：视频判定补齐（第 5 处测量伪影） | ✅ 两处判据缺口：①manifest 的 `phases` 字段（分段期望色）**从未被解析**（`sampleSpec` 缺字段），twophase 这类样本因此没有期望色；②无期望色的视频样本（testsrc 这类自然序列）落到 `sampleCell` 末尾 `return false` → 「绘制」恒 ❌。修法：新增 `mediaPhase` 类型解析 `phases`（取**首段**色作静止态期望）；其余视频按「非灰块」判定（中心点非 `.cell` 底色、非页面白），与 §5 TC-M-502「截图有画面（非灰块）」的达成标准一致 | 修前 12 格「绘制 ❌」→ 修后 file/rel 全 ✅ |
| 备注语义（避免把预期当缺陷报） | ✅ 报告为两条「非缺陷」情形加注：①`data:` 来源的媒体没有本地路径，宿主帧源（ffmpeg 按路径抽帧）给不出画面 → L1 属预期；②单色样本（solid-red）帧色恒定，动画判据（帧间差异）不适用 → L3 | 报告「备注」列 |
| 探针：音频状态采集对象修正（第 6 处测量伪影） | ✅ 矩阵页把采集用的 `c.ID` 给了包裹 div、音频元素挂在 `c.ID+"_a"` 上，`collectPage` 因此读到的是 div——`readyState`/`duration` 一律读不到，「加载/几何」两列恒 ❌、音频恒判 L0，把「元数据其实可加载」（TC-M-601 的 L1）压成了 L0。修法：采集 id 交给音频元素本身，包裹 div 改挂 `_box` 后缀。音频 12 格 **L0 → L1**——这正是 A3 的真实起点（元数据通、输出缺） | 探针 `sine-440-1s.*` 行：加载/几何/契约 ✅ → **L1**；备注「音频可加载（元数据可用）；音频输出（L4-S）需音频后端」 |
| 基线维护（第三批） | 两次更新：① video 条目 **50 行**（含 `generated_at`）；② 音频采集修正再 **49 行**（48 条 `L0 → L1` + `generated_at`） | `git diff --numstat dev/media/baseline.json` |
| 四配置复测（第三批后） | Browser / AllowHostResolved / AllowAll 三者完全一致：**L0 11 / L1 16 / L3 51 / L4 18**；`DenyExternal`：L0 51 / L1 16 / L3 19 / L4 10。连续两次全量跑均「与基线一致」。余下 11 格 L0 = `data:` 来源的 video 4 格（无本地路径）+ AVIF/TIFF 6 格（预期不支持）+ 失败路径 1 格 | `dev/media/out/report.md`（384 行） |
| **（新发现·待决）媒体宿主注入通道未受资源策略约束** | ⚠️ 见 §3.4 缺口表：`DenyExternal` 下 `<video src="file://…">` 仍取得画面（L3/L4），与 `<img>` 在同一策略下 L0 的行为不一致；根因是媒体元数据/帧由宿主按本地路径直接读取，未过策略门禁。**上报待决**——策略语义变更不在本轮范围内擅自处理 | 探针 `DenyExternal × *.mp4 × file/rel` = L3/L4；同策略下 `*.png` 为 L0 |

---

### 9.6 阶段 3 立项材料（A3 音频后端）

**A3 音频后端立项评估：见 [`audio-backend-proposal.md`](audio-backend-proposal.md)**（2026-10）。

要点摘录：

- **现状**：引擎侧状态机与播放时钟**已可用**（TC-M-601/602 实测达成：`rs=4`、`duration≈1s`、
  `play()` 后 `currentTime` 0 → 1 播完）；缺的只是「PCM 解码 → 输出设备」这一段——
  `goskia` 当前**完全没有音频能力**（无 audio/sound/wave/mixer 任何 API）。
- **推荐路线**：(a) 宿主注入 PCM（与 A1/A2 的视频帧注入同构，引擎不背解码器），
  输出后端分阶段交付（先 Windows）。
- **判据**：TC-M-601/602 已达成；TC-M-603（WebAudio）与 TC-M-604（L4-S）待实施。
  TC-M-604 建议「宿主输出回调的 PCM 做 FFT（主峰 440 Hz）」为主线判据，
  环回设备录音作为可选外部复核（无设备标 `SKIP(no-loopback)`）。
- **待确认**（实施前需用户选定）：路线 (a)/(b)、平台范围（仅 Windows / 三平台）、
  WebAudio 是否需要、L4-S 判据口径、以及是否与「媒体宿主注入通道未受资源策略约束」
  的修复（§3.4 待决项）合并立项——两者都动「宿主注入媒体数据」这条链路。

---

### 9.7 TC-M-905 策略一致性差异（3.9683%）的归因与处置（2026-10）

**结论（一句话）**：**已知 / 可接受**——该 ❌ 不是「策略开关引入的渲染差异」，而是
**动画样本的帧相位差**；判据口径已修正为「排除动画样本格后」判定，本次实测
**0.0000%**（非动画区域逐像素一致）。引擎与资源策略语义**未做任何改动**。

| 项 | 内容 |
|---|---|
| 现象 | `dev/media/out/report.md` 的 TC-M-905 段长期显示「Browser vs Toolkit+AllowAll 首屏截图差异 3.9683% ❌」——判据原文为 `adDiff > 0` 即失败（零容差），且把遍历时仍在播放的动画格一并计入 |
| 既有性（取证） | `report.md` **全部四个入库版本**（`c5fe57d` 起至 `81b2e1d`）数值均为 **3.9683%**，与每版提交主题无关 → 自报告首次入库即存在，**非本轮（D10/探针修复）引入** |
| 差异归因（取证） | 逐像素反查（脚本按 `geom-Browser.json` 的格子矩形归因）：57600 个差异像素 **100% 落在 6 个动画样本格内**——`anim-noloop.gif`（file/rel/data）与 `anim-2frames.webp`（file/rel/data），每格差异恰为整格 120×80 = 9600px；**其余 90 格逐像素一致**。差异幅度直方图显示全部 ≥64（整帧色不同），无 1–15 级噪声 → 是**抓到了不同帧**，不是抗锯齿/字体度量 |
| 归因解释 | Browser 与 Toolkit+AllowAll 是**两次独立加载与截图**，动画各自的相位不同（中途抓拍），两配置在动画格上自然停在不同帧；「帧相位差」与策略开关无关。对照组：非动画区域（静态图 / SVG / 视频 / 音频格）**零差异**，说明策略开关**确实没有**引入渲染差异——原判据的**结论意图成立**，只是测量口径把动画格算了进去 |
| 处置（本轮） | **修正测量口径**（不改引擎）：探针新增 `animatedCellRects`（按 `Kind=="animated"` + 几何矩形）+ `comparePNGFilesExcluding`；报告同时给出**全图差异**（如实保留）与**排除动画格后的差异**（判定依据）。实测：全图 **3.9683%**（12 个动画格排除在外）→ 排除后 **0.0000% ✅**，并输出归因说明 |
| 附带发现（记录，未改） | 本轮两次跑之间出现**一处动画判据抖动**：`Toolkit+DenyExternal / anim-uneven-delay.gif / data` 在 **L4 ↔ L3** 间摇摆（第一次跑 L4→L3 报缺陷、第二次跑与基线一致）；同 HEAD 两次结果不同即抖动特征。系「动画推进」判据对**不均匀帧延迟**（样本名即 `uneven-delay`）的灵敏度问题，属 §9.4 第 4 处伪影（动图判定稳定化）的**同类残余**，非本轮改动引入（本轮改动不触及等级判定）。**处置建议**：扩充采样窗口/点数或对该类样本按帧延迟序列预测采样时刻；是否落地待定（可能影响全部动画格判定，需多次跑回归）。 |
| 顺手修复（既有缺陷） | `cmd/psai/mediaprobe.go:196` `sampleDot` 的 `X, Y int \`json:"x"\`` 两字段共用同一 json tag——`go vet` 明确报错（`struct field Y repeats json tag "x"`），且 Go 对同层同 tag 字段会**双双忽略**（将来一旦序列化即静默丢 x/y）。取证确认该结构当前**未进入任何入库产物**（`writeGeomJSON` 用 `cellProbe`、`writeBaseline` 用简化 `baselineItem`），故仅表现为 vet 警告。已拆为 `X int \`json:"x"\`` / `Y int \`json:"y"\``，`go vet ./cmd/psai/` 恢复干净 |

## 附：本次盘点产出的证据索引

| 文件 | 内容 |
|---|---|
| `_temp/mediacheck/out.txt` | 首轮解码探针（含一次 BMP 生成 bug） |
| `_temp/mediacheck/out2.txt` | **解码矩阵定稿**（21 样本 × Skia/Go） |
| `_temp/mediacheck/render-out.txt` | A/B/C 三组逐元素 IDL 报告（102 个 img + 57 个 media） |
| `_temp/mediacheck/render/A-toolkit-noresolver.png` | ModeToolkit 无 resolver：仅 `svg\|data` 出图 |
| `_temp/mediacheck/render/B-toolkit-resolver.png` | ModeToolkit + resolver：与 A **逐字节相同** |
| `_temp/mediacheck/render/C-browser.png` | ModeBrowser 对照：三列光栅图全部画出 |
| `_temp/mediacheck/samples/` | 21 个格式样本（PIL/ffmpeg 生成） |
| **`dev/media/gen_samples.py`** | **媒体验证样本生成脚本**（32 文件 + 1 内联条目；入库，决策 5） |
| **`dev/media/baseline.json`** | **等级回归基线**：384 条（四配置 × 样本 × 来源），由 `-media-update-baseline` 生成 |
| **`dev/media/out/report.md`** | **四配置 L0–L4 报告**：总表 / 缺陷清单 / 与基线差异 / A-D 一致性（Browser vs Toolkit+AllowAll） |
| **`dev/media/out/matrix-*.png`**、`geom-*.json` | 四配置静止态与播放态截图 + 逐格几何/IDL/事件采集（证据） |
| **`cmd/psai/mediaprobe.go`** | `-media` 探针实现：矩阵页构建、判定器、报告、基线比对（入库） |

> 注（决策 5/6）：以上路径全部位于 `_temp/`（已被 git 忽略），**不随仓库提交**。落地时改为「脚本入库 + 样本本机生成、报告仅本地产物」（§6.2、§6.4）；本表仅记录设计阶段的证据出处。
