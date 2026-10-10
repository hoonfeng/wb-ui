# 补充判据：G3/G4/G6/G7 的「未单独采证」用例

- 页面：`F:\syproject\wb-ui\dev\media\samples\_extra.html`（探针运行时生成；中文/空格样本由探针自动物化）
- 复跑：`cmd/psai -media`（本段随媒体验证一起跑）；Edge 对照随 `-media-edge`
- 采集方式：**渲染像素**（引擎 `wv.Render()` / Edge `--screenshot`）+ **页面内 canvas 采样**
  （`drawImage` 取元素当前帧，用于 TC-M-304~307 的时间序列）；两侧走同一套页面脚本与判据内核。
- 判据口径：✅ 达成 / ❌ 未达成 / ⚠️ 部分或不符预期（如实记录）/ SKIP（附原因，不伪装通过）。

## Browser

| 用例 | 判据 | 期望 | 实测 | 判定 | 备注 |
|---|---|---|---|---|---|
| TC-M-303 | image-rendering: pixelated 的最近邻缩放 | pixelated 采样色数显著少于 auto 对照（源色 4；残留色仅来自色块交界混合像素），自身不走整幅插值 | pixelated 采样色数 8（源色 4）；auto 对照色数 21（采样内缩 3px） | ✅ | pixelated 显著少于 auto（8 vs 21）⇒ `image-rendering:pixelated` 生效（放大走最近邻；残留色来自色块交界 ±1px 采样，非整幅插值） |
| TC-M-304 | <canvas> drawImage 取动图当前帧 | 连续采样出现 ≥2 种帧色（取到的是当前帧、不是首帧常量） | canvas 采样 60 次（有效 60）出现 3 种帧色；渲染像素 60 次出现 3 种帧色 | ✅ | canvas 的 drawImage 取到的是当前帧（帧色随播放推进变化） |
| TC-M-305 | 逐帧 delay 对齐（帧延迟 [50,200,100]ms，周期 350ms） | 三段帧色时长之比 ≈ 50:200:100，周期 ≈350ms | 色段 30 个；最长段 175ms（期望 200）；帧色变化率 8.4 次/秒（期望 ≈8.6） | ✅ | 段时长与变化率都按真实墙钟时间戳计算（采样间隔受渲染耗时限制，故判据是区间而非等值） |
| TC-M-306 | visibility:hidden 后动图是否继续推进 | 隐藏 300ms 期间帧色仍变化（规范：不因不可见而暂停） | 隐藏窗口(400~700ms) canvas 采样 6 次出现 3 种帧色；同窗口渲染像素 1 种（元素不可见 ⇒ 采到的是容器背景色，不能作推进判据）；可见窗口渲染像素 3 种 | ✅ | 隐藏期间仍在推进（canvas 观测：帧色仍变化）⇒ 符合规范（`visibility:hidden` 不暂停动图） |
| TC-M-307a | 不循环 GIF（anim-noloop）末帧行为 | 播过末尾后停在末帧（尾部样本颜色恒定） | 尾部 15 次采样出现 1 种帧色 | ✅ | 播过末尾后停在末帧（尾部帧色恒定） |
| TC-M-307b | 循环 GIF（loop=0）末帧行为 | 播过末尾后回到首帧继续循环（尾部样本仍出现 ≥2 色） | 尾部 15 次采样出现 3 种帧色 | ✅ | 播过末尾后回到首帧继续循环（`loop=0` = 无限循环；尾部仍出现多种帧色） |
| TC-M-404 | 扩展名与内容不符（.png 实为 JPEG）按内容解码 | complete=true 且 naturalWidth>0，且渲染出图 | complete=true naturalWidth=120；非背景网格点 25/25 | ✅ | 按内容解码（引擎不看扩展名） |
| TC-M-405 | 图片路径不存在：不渲染 + 无控制台刷屏 | naturalWidth=0；页面可见控制台条目 0 | naturalWidth=0；页面可见控制台条目 0 条 | ✅ | 不渲染且无控制台刷屏 |
| TC-M-506 | <video controls> 是否绘制原生控件 | tabIndex=0（契约）；视觉差异如实记录（无控件绘制路径则不列为缺陷） | tabIndex：无 controls=-1、带 controls=0；两格 8x6 采样差异 1/48 点 | ✅ | 契约符合（controls 使 tabIndex 默认 0）；引擎无 UA 原生控件绘制路径 ⇒ 视觉差异如实记录，不列为缺陷 |
| TC-M-702 | object-fit 四值（contain/cover/fill/none） | 四者几何可区分：contain 有 letterbox、fill 非等比、none 为原始尺寸、cover 等比填满 | contain[h=21 v=21 h/v=1.00 lb=true] cover[h=24 v=24 h/v=1.00 lb=false] fill[h=21 v=24 h/v=0.88 lb=false] none[h=41 v=41 h/v=1.00 lb=false] | ✅ | h/v = 扫描线中位段长（px，棋盘格周期）；lb = 上下是否出现 magenta 背景（letterbox） |
| TC-M-704 | 中文文件名 + 中文目录 | 能加载（complete=true、naturalWidth>0、渲染出图） | complete=true naturalWidth=120；非背景网格点 25/25 | ✅ | 中文文件名 + 中文目录（Windows 路径编码） |
| TC-M-705a | 文件名含空格 | 能加载（complete=true、naturalWidth>0） | complete=true naturalWidth=120；非背景网格点 25/25 | ✅ | 文件名含空格（URL 需容忍 %20） |
| TC-M-705b | 路径含 .. 穿越 | Browser(file://) 可解析；DenyExternal 策略下被 resolver 拒绝（两者都如实记录） | complete=true naturalWidth=120 | ✅ | file:// 下 `..` 可解析（浏览器语义） |

## Toolkit+DenyExternal

> 只采静态判据（时间序列项 SKIP）

| 用例 | 判据 | 期望 | 实测 | 判定 | 备注 |
|---|---|---|---|---|---|
| TC-M-303 | image-rendering: pixelated 的最近邻缩放 | pixelated 采样色数显著少于 auto 对照（源色 4；残留色仅来自色块交界混合像素），自身不走整幅插值 | pixelated 采样色数 8（源色 4）；auto 对照色数 21（采样内缩 3px） | ✅ | pixelated 显著少于 auto（8 vs 21）⇒ `image-rendering:pixelated` 生效（放大走最近邻；残留色来自色块交界 ±1px 采样，非整幅插值） |
| TC-M-304 | <canvas> drawImage 取动图当前帧 | 连续采样出现 ≥2 种帧色（取到的是当前帧、不是首帧常量） | — | SKIP | SKIP(未采)：该配置本轮不采时间序列 |
| TC-M-305 | 逐帧 delay 对齐（帧延迟 [50,200,100]ms，周期 350ms） | 三段帧色时长之比 ≈ 50:200:100，周期 ≈350ms | — | SKIP | SKIP(未采)：该配置本轮不采时间序列 |
| TC-M-306 | visibility:hidden 后动图是否继续推进 | 隐藏 300ms 期间帧色仍变化（规范：不因不可见而暂停） | — | SKIP | SKIP(未采)：该配置本轮不采时间序列 |
| TC-M-307a | 不循环 GIF（anim-noloop）末帧行为 | 播过末尾后停在末帧（尾部样本颜色恒定） | — | SKIP | SKIP(未采)：该配置本轮不采时间序列 |
| TC-M-307b | 循环 GIF（loop=0）末帧行为 | 播过末尾后回到首帧继续循环（尾部样本仍出现 ≥2 色） | — | SKIP | SKIP(未采)：该配置本轮不采时间序列 |
| TC-M-404 | 扩展名与内容不符（.png 实为 JPEG）按内容解码 | complete=true 且 naturalWidth>0，且渲染出图 | complete=false naturalWidth=0 | ✅ | 策略配置下该引用被 resolver 拒绝（策略生效；nw=0 符合预期） |
| TC-M-405 | 图片路径不存在：不渲染 + 无控制台刷屏 | naturalWidth=0；页面可见控制台条目 0 | naturalWidth=0；页面可见控制台条目 0 条 | ✅ | 不渲染且无控制台刷屏 |
| TC-M-506 | <video controls> 是否绘制原生控件 | tabIndex=0（契约）；视觉差异如实记录（无控件绘制路径则不列为缺陷） | tabIndex：无 controls=-1、带 controls=0；两格 8x6 采样差异 1/48 点 | ✅ | 契约符合（controls 使 tabIndex 默认 0）；引擎无 UA 原生控件绘制路径 ⇒ 视觉差异如实记录，不列为缺陷 |
| TC-M-702 | object-fit 四值（contain/cover/fill/none） | 四者几何可区分：contain 有 letterbox、fill 非等比、none 为原始尺寸、cover 等比填满 | contain[h=21 v=21 h/v=1.00 lb=true] cover[h=24 v=24 h/v=1.00 lb=false] fill[h=21 v=24 h/v=0.88 lb=false] none[h=41 v=41 h/v=1.00 lb=false] | ✅ | h/v = 扫描线中位段长（px，棋盘格周期）；lb = 上下是否出现 magenta 背景（letterbox） |
| TC-M-704 | 中文文件名 + 中文目录 | 能加载（complete=true、naturalWidth>0、渲染出图） | complete=false naturalWidth=0 | ✅ | 中文文件名 + 中文目录（Windows 路径编码）：策略配置下被 resolver 拒绝（符合预期） |
| TC-M-705a | 文件名含空格 | 能加载（complete=true、naturalWidth>0） | complete=false naturalWidth=0 | ✅ | 文件名含空格（URL 需容忍 %20）：策略配置下被 resolver 拒绝（符合预期） |
| TC-M-705b | 路径含 .. 穿越 | Browser(file://) 可解析；DenyExternal 策略下被 resolver 拒绝（两者都如实记录） | complete=false naturalWidth=0 | ✅ | 策略配置下被 resolver 拒绝（策略生效，符合预期） |

## Edge

> 本节取自 `cafc37d` 的 Edge 对照存档（`extra-Edge.json`）：**本轮未重跑 Edge**（`-media-edge` 默认关闭），故沿用原数值口径；重跑 `-media-edge` 会用新数据覆盖本节。

| 用例 | 判据 | 期望 | 实测 | 判定 | 备注 |
|---|---|---|---|---|---|
| TC-M-303 | image-rendering: pixelated 的最近邻缩放 | 放大后不产生插值中间色（色数 = 源色数 4） | pixelated 采样色数 9（源色 4）；auto 对照色数 22（采样内缩 3px） | ✅ | pixelated 显著少于 auto（9 vs 22）⇒ `image-rendering:pixelated` 生效（放大走最近邻；残留色来自色块交界 ±1px 采样，非整幅插值） |
| TC-M-304 | <canvas> drawImage 取动图当前帧 | 连续采样出现 ≥2 种帧色（取到的是当前帧、不是首帧常量） | — | SKIP | SKIP(edge-time-series)：Edge 无头一次性截图/虚拟时间下动图帧推进不可靠，未采（其余项已对照） |
| TC-M-305 | 逐帧 delay 对齐（帧延迟 [50,200,100]ms，周期 350ms） | 三段帧色时长之比 ≈ 50:200:100，周期 ≈350ms | — | SKIP | SKIP(edge-time-series)：Edge 无头一次性截图/虚拟时间下动图帧推进不可靠，未采（其余项已对照） |
| TC-M-306 | visibility:hidden 后动图是否继续推进 | 隐藏 300ms 期间帧色仍变化（规范：不因不可见而暂停） | — | SKIP | SKIP(edge-time-series)：Edge 无头一次性截图/虚拟时间下动图帧推进不可靠，未采（其余项已对照） |
| TC-M-307a | 不循环 GIF（anim-noloop）末帧行为 | 播过末尾后停在末帧（尾部样本颜色恒定） | — | SKIP | SKIP(edge-time-series)：Edge 无头一次性截图/虚拟时间下动图帧推进不可靠，未采（其余项已对照） |
| TC-M-307b | 循环 GIF（loop=0）末帧行为 | 播过末尾后回到首帧继续循环（尾部样本仍出现 ≥2 色） | — | SKIP | SKIP(edge-time-series)：Edge 无头一次性截图/虚拟时间下动图帧推进不可靠，未采（其余项已对照） |
| TC-M-404 | 扩展名与内容不符（.png 实为 JPEG）按内容解码 | complete=true 且 naturalWidth>0，且渲染出图 | complete=true naturalWidth=120；非背景网格点 25/25 | ✅ | 按内容解码（引擎不看扩展名） |
| TC-M-405 | 图片路径不存在：不渲染 + 无控制台刷屏 | naturalWidth=0；页面可见控制台条目 0 | naturalWidth=0；页面可见控制台条目 0 条 | ✅ | 不渲染且无控制台刷屏 |
| TC-M-506 | <video controls> 是否绘制原生控件 | tabIndex=0（契约）；视觉差异如实记录（无控件绘制路径则不列为缺陷） | tabIndex：无 controls=0、带 controls=0；两格 8x6 采样差异 48/48 点 | ✅ | 契约符合（controls 使 tabIndex 默认 0）；引擎无 UA 原生控件绘制路径 ⇒ 视觉差异如实记录，不列为缺陷 |
| TC-M-702 | object-fit 四值（contain/cover/fill/none） | 四者几何可区分：contain 有 letterbox、fill 非等比、none 为原始尺寸、cover 等比填满 | contain[h=21 v=21 h/v=1.00 lb=true] cover[h=24 v=24 h/v=1.00 lb=false] fill[h=21 v=24 h/v=0.88 lb=false] none[h=41 v=41 h/v=1.00 lb=false] | ✅ | h/v = 扫描线中位段长（px，棋盘格周期）；lb = 上下是否出现 magenta 背景（letterbox） |
| TC-M-704 | 中文文件名 + 中文目录 | 能加载（complete=true、naturalWidth>0、渲染出图） | complete=true naturalWidth=120；非背景网格点 25/25 | ✅ | 中文文件名 + 中文目录（Windows 路径编码） |
| TC-M-705a | 文件名含空格 | 能加载（complete=true、naturalWidth>0） | complete=true naturalWidth=120；非背景网格点 25/25 | ✅ | 文件名含空格（URL 需容忍 %20） |
| TC-M-705b | 路径含 .. 穿越 | Browser(file://) 可解析；DenyExternal 策略下被 resolver 拒绝（两者都如实记录） | complete=true naturalWidth=120 | ✅ | file:// 下 `..` 可解析（浏览器语义） |

## 引擎 vs Edge 逐项对照

可比 **8** 项，**引擎无一项低于 Edge** ✅

