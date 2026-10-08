# 媒体格式真实可用性报告（2026-10-08 11:36 5879442）

## 环境

配置：Browser / Toolkit+DenyExternal / Toolkit+AllowHostResolved / Toolkit+AllowAll ｜ 引擎：5879442 ｜ 样本：本地脚本生成、不入库（决策 5）｜ 模型：L0–L4（文档 §2）

复现：`python dev/media/gen_samples.py` → `cmd/psai -media`（本机按需，不入 CI 门禁——决策 6）

## 总表

> **列说明**：`加载`/`几何` 对音频行指「元数据可加载 / 时长可用」；**`绘制` 列对音频行不适用（N/A，报告显示 `—`）**——音频无视觉内容，其等级由音频输出判据决定（PCM 交付 + 频谱主峰（判据 A）+ 播放时钟（TC-M-602），见「音频」小节）。
> **L0 归因**：L0 = 未加载。`deny-external` 下的 `file`/`rel` 引用是被资源策略拒绝——**预期行为，非缺陷**（文档 §9.9）；元素 `error.code` 仅作实测佐证：引擎对「策略拒绝」与「源不可达」共用 `MEDIA_ERR_SRC_NOT_SUPPORTED`（4）。

| 配置 | 格式 | 样本 | 来源 | 加载 | 几何 | 绘制 | 契约 | 动画 | 等级 | 备注 |
|---|---|---|---|---|---|---|---|---|---|---|
| Browser | webp | anim-2frames.webp | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | webp | anim-2frames.webp | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | webp | anim-2frames.webp | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | webp | anim-2frames.webp | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | webp | anim-2frames.webp | file | ❌ | ❌ | ✅ | ❌ | ✅ | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | webp | anim-2frames.webp | file | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | webp | anim-2frames.webp | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | webp | anim-2frames.webp | file | ✅ | ✅ | ✅ | ❌ | ✅ | **L2** | 画得出但契约不完整（complete/onload） |
| Browser | webp | anim-2frames.webp | rel | ❌ | ❌ | ✅ | ❌ | ✅ | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | webp | anim-2frames.webp | rel | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | webp | anim-2frames.webp | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | webp | anim-2frames.webp | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-3frames-rgb.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-3frames-rgb.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | gif | anim-3frames-rgb.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-3frames-rgb.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-3frames-rgb.gif | file | ❌ | ❌ | ✅ | ❌ | ✅ | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | gif | anim-3frames-rgb.gif | file | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | gif | anim-3frames-rgb.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-3frames-rgb.gif | file | ✅ | ✅ | ✅ | ❌ | ✅ | **L2** | 画得出但契约不完整（complete/onload） |
| Browser | gif | anim-3frames-rgb.gif | rel | ❌ | ❌ | ✅ | ❌ | ✅ | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | gif | anim-3frames-rgb.gif | rel | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | gif | anim-3frames-rgb.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-3frames-rgb.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-noloop.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-noloop.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | gif | anim-noloop.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-noloop.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-noloop.gif | file | ❌ | ❌ | ✅ | ❌ | ✅ | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | gif | anim-noloop.gif | file | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | gif | anim-noloop.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-noloop.gif | file | ❌ | ❌ | ✅ | ❌ | ✅ | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Browser | gif | anim-noloop.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-noloop.gif | rel | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | gif | anim-noloop.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-noloop.gif | rel | ❌ | ❌ | ✅ | ❌ | ✅ | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Browser | gif | anim-uneven-delay.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-uneven-delay.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | gif | anim-uneven-delay.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-uneven-delay.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-uneven-delay.gif | file | ❌ | ❌ | ✅ | ❌ | ✅ | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | gif | anim-uneven-delay.gif | file | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | gif | anim-uneven-delay.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-uneven-delay.gif | file | ❌ | ❌ | ✅ | ❌ | ✅ | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Browser | gif | anim-uneven-delay.gif | rel | ❌ | ❌ | ✅ | ❌ | ✅ | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | gif | anim-uneven-delay.gif | rel | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | gif | anim-uneven-delay.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-uneven-delay.gif | rel | ❌ | ❌ | ✅ | ❌ | ✅ | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Browser | png | corrupt.png | data | ✅ | ✅ | ❌ | ✅ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Toolkit+DenyExternal | png | corrupt.png | data | ✅ | ✅ | ❌ | ✅ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Toolkit+AllowHostResolved | png | corrupt.png | data | ✅ | ✅ | ❌ | ✅ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Toolkit+AllowAll | png | corrupt.png | data | ✅ | ✅ | ❌ | ✅ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Browser | png | corrupt.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Toolkit+DenyExternal | png | corrupt.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | png | corrupt.png | file | ✅ | ✅ | ❌ | ✅ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Toolkit+AllowAll | png | corrupt.png | file | ✅ | ✅ | ❌ | ✅ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Browser | png | corrupt.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Toolkit+DenyExternal | png | corrupt.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | png | corrupt.png | rel | ✅ | ✅ | ❌ | ✅ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Toolkit+AllowAll | png | corrupt.png | rel | ✅ | ✅ | ❌ | ✅ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Browser | png | gradient.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | gradient.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | png | gradient.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | gradient.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | gradient.png | file | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | png | gradient.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | png | gradient.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | gradient.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | gradient.png | rel | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | png | gradient.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | png | gradient.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | gradient.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | huge-4096.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | huge-4096.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | png | huge-4096.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | huge-4096.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | huge-4096.png | file | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | png | huge-4096.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | png | huge-4096.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | huge-4096.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | huge-4096.png | rel | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | png | huge-4096.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | png | huge-4096.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | huge-4096.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | svg | icon-24.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | svg | icon-24.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | svg | icon-24.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | svg | icon-24.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | svg | icon-24.svg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | svg | icon-24.svg | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | svg | icon-24.svg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | svg | icon-24.svg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | svg | icon-24.svg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | svg | icon-24.svg | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | svg | icon-24.svg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | svg | icon-24.svg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | svg | inline-svg-data-uri | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | svg | inline-svg-data-uri | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | svg | inline-svg-data-uri | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | svg | inline-svg-data-uri | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | jpeg | mislabeled.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | jpeg | mislabeled.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | jpeg | mislabeled.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | jpeg | mislabeled.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | jpeg | mislabeled.png | file | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | jpeg | mislabeled.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | jpeg | mislabeled.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | jpeg | mislabeled.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | jpeg | mislabeled.png | rel | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | jpeg | mislabeled.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | jpeg | mislabeled.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | jpeg | mislabeled.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | ico | quad-64.ico | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | ico | quad-64.ico | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | ico | quad-64.ico | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | ico | quad-64.ico | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | ico | quad-64.ico | file | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | ico | quad-64.ico | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | ico | quad-64.ico | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | ico | quad-64.ico | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | ico | quad-64.ico | rel | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | ico | quad-64.ico | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | ico | quad-64.ico | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | ico | quad-64.ico | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | webp | quad-lossless.webp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | webp | quad-lossless.webp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | webp | quad-lossless.webp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | webp | quad-lossless.webp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | webp | quad-lossless.webp | file | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | webp | quad-lossless.webp | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | webp | quad-lossless.webp | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | webp | quad-lossless.webp | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | webp | quad-lossless.webp | rel | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | webp | quad-lossless.webp | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | webp | quad-lossless.webp | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | webp | quad-lossless.webp | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | webp | quad-lossy.webp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | webp | quad-lossy.webp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | webp | quad-lossy.webp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | webp | quad-lossy.webp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | webp | quad-lossy.webp | file | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | webp | quad-lossy.webp | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | webp | quad-lossy.webp | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | webp | quad-lossy.webp | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | webp | quad-lossy.webp | rel | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | webp | quad-lossy.webp | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | webp | quad-lossy.webp | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | webp | quad-lossy.webp | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | avif | quad.avif | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | avif | quad.avif | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | avif | quad.avif | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | avif | quad.avif | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | avif | quad.avif | file | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | avif | quad.avif | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | avif | quad.avif | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | avif | quad.avif | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | avif | quad.avif | rel | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | avif | quad.avif | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | avif | quad.avif | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | avif | quad.avif | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | bmp | quad.bmp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | bmp | quad.bmp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | bmp | quad.bmp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | bmp | quad.bmp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | bmp | quad.bmp | file | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | bmp | quad.bmp | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | bmp | quad.bmp | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | bmp | quad.bmp | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | bmp | quad.bmp | rel | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | bmp | quad.bmp | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | bmp | quad.bmp | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | bmp | quad.bmp | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | gif | quad.gif | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | gif | quad.gif | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | gif | quad.gif | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | gif | quad.gif | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | gif | quad.gif | file | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | gif | quad.gif | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | gif | quad.gif | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | gif | quad.gif | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | gif | quad.gif | rel | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | gif | quad.gif | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | gif | quad.gif | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | gif | quad.gif | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | jpeg | quad.jpg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | jpeg | quad.jpg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | jpeg | quad.jpg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | jpeg | quad.jpg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | jpeg | quad.jpg | file | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | jpeg | quad.jpg | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | jpeg | quad.jpg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | jpeg | quad.jpg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | jpeg | quad.jpg | rel | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | jpeg | quad.jpg | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | jpeg | quad.jpg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | jpeg | quad.jpg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | quad.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | quad.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | png | quad.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | quad.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | quad.png | file | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | png | quad.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | png | quad.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | quad.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | quad.png | rel | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | png | quad.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | png | quad.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | quad.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | tiff | quad.tiff | data | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Toolkit+DenyExternal | tiff | quad.tiff | data | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Toolkit+AllowHostResolved | tiff | quad.tiff | data | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Toolkit+AllowAll | tiff | quad.tiff | data | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Browser | tiff | quad.tiff | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Toolkit+DenyExternal | tiff | quad.tiff | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Toolkit+AllowHostResolved | tiff | quad.tiff | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Toolkit+AllowAll | tiff | quad.tiff | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Browser | tiff | quad.tiff | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Toolkit+DenyExternal | tiff | quad.tiff | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Toolkit+AllowHostResolved | tiff | quad.tiff | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Toolkit+AllowAll | tiff | quad.tiff | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Browser | svg | ratio-only.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | svg | ratio-only.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | svg | ratio-only.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | svg | ratio-only.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | svg | ratio-only.svg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | svg | ratio-only.svg | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | svg | ratio-only.svg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | svg | ratio-only.svg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | svg | ratio-only.svg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | svg | ratio-only.svg | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | svg | ratio-only.svg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | svg | ratio-only.svg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | svg | rect-120x80.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | svg | rect-120x80.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | svg | rect-120x80.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | svg | rect-120x80.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | svg | rect-120x80.svg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | svg | rect-120x80.svg | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | svg | rect-120x80.svg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | svg | rect-120x80.svg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | svg | rect-120x80.svg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | svg | rect-120x80.svg | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | svg | rect-120x80.svg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | svg | rect-120x80.svg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | m4a | sine-440-1s.m4a | data | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 49041 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.42s、结束时 1.00s（TC-M-602） |
| Toolkit+DenyExternal | m4a | sine-440-1s.m4a | data | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 49041 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowHostResolved | m4a | sine-440-1s.m4a | data | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 49041 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.41s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowAll | m4a | sine-440-1s.m4a | data | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 49041 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.41s、结束时 1.00s（TC-M-602） |
| Browser | m4a | sine-440-1s.m4a | file | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 49041 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.29s、结束时 1.00s（TC-M-602） |
| Toolkit+DenyExternal | m4a | sine-440-1s.m4a | file | ❌ | ❌ | — | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+AllowHostResolved | m4a | sine-440-1s.m4a | file | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 49041 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.23s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowAll | m4a | sine-440-1s.m4a | file | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 49041 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.24s、结束时 1.00s（TC-M-602） |
| Browser | m4a | sine-440-1s.m4a | rel | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 49041 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.00s、结束时 1.00s（TC-M-602） |
| Toolkit+DenyExternal | m4a | sine-440-1s.m4a | rel | ❌ | ❌ | — | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+AllowHostResolved | m4a | sine-440-1s.m4a | rel | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 49041 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.00s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowAll | m4a | sine-440-1s.m4a | rel | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 49041 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.00s、结束时 1.00s（TC-M-602） |
| Browser | mp3 | sine-440-1s.mp3 | data | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Toolkit+DenyExternal | mp3 | sine-440-1s.mp3 | data | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowHostResolved | mp3 | sine-440-1s.mp3 | data | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowAll | mp3 | sine-440-1s.mp3 | data | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Browser | mp3 | sine-440-1s.mp3 | file | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Toolkit+DenyExternal | mp3 | sine-440-1s.mp3 | file | ❌ | ❌ | — | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+AllowHostResolved | mp3 | sine-440-1s.mp3 | file | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowAll | mp3 | sine-440-1s.mp3 | file | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Browser | mp3 | sine-440-1s.mp3 | rel | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Toolkit+DenyExternal | mp3 | sine-440-1s.mp3 | rel | ❌ | ❌ | — | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+AllowHostResolved | mp3 | sine-440-1s.mp3 | rel | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowAll | mp3 | sine-440-1s.mp3 | rel | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Browser | ogg | sine-440-1s.ogg | data | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 47861 帧、PCM 主峰 439.87Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.89s、结束时 1.00s（TC-M-602） |
| Toolkit+DenyExternal | ogg | sine-440-1s.ogg | data | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 47861 帧、PCM 主峰 439.87Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowHostResolved | ogg | sine-440-1s.ogg | data | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 47861 帧、PCM 主峰 439.87Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.93s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowAll | ogg | sine-440-1s.ogg | data | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 47861 帧、PCM 主峰 439.87Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.86s、结束时 1.00s（TC-M-602） |
| Browser | ogg | sine-440-1s.ogg | file | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 47861 帧、PCM 主峰 439.87Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.73s、结束时 1.00s（TC-M-602） |
| Toolkit+DenyExternal | ogg | sine-440-1s.ogg | file | ❌ | ❌ | — | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+AllowHostResolved | ogg | sine-440-1s.ogg | file | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 47861 帧、PCM 主峰 439.87Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.73s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowAll | ogg | sine-440-1s.ogg | file | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 47861 帧、PCM 主峰 439.87Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.71s、结束时 1.00s（TC-M-602） |
| Browser | ogg | sine-440-1s.ogg | rel | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 47861 帧、PCM 主峰 439.87Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.58s、结束时 1.00s（TC-M-602） |
| Toolkit+DenyExternal | ogg | sine-440-1s.ogg | rel | ❌ | ❌ | — | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+AllowHostResolved | ogg | sine-440-1s.ogg | rel | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 47861 帧、PCM 主峰 439.87Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.59s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowAll | ogg | sine-440-1s.ogg | rel | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 47861 帧、PCM 主峰 439.87Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.55s、结束时 1.00s（TC-M-602） |
| Browser | wav | sine-440-1s.wav | data | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Toolkit+DenyExternal | wav | sine-440-1s.wav | data | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowHostResolved | wav | sine-440-1s.wav | data | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowAll | wav | sine-440-1s.wav | data | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Browser | wav | sine-440-1s.wav | file | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Toolkit+DenyExternal | wav | sine-440-1s.wav | file | ❌ | ❌ | — | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+AllowHostResolved | wav | sine-440-1s.wav | file | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowAll | wav | sine-440-1s.wav | file | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Browser | wav | sine-440-1s.wav | rel | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Toolkit+DenyExternal | wav | sine-440-1s.wav | rel | ❌ | ❌ | — | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+AllowHostResolved | wav | sine-440-1s.wav | rel | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowAll | wav | sine-440-1s.wav | rel | ✅ | ✅ | — | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=1.00s、结束时 1.00s（TC-M-602） |
| Browser | mp4 | solid-red-1s.mp4 | data | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足）；所有帧采样点相同（最大差异 0，共 11 帧） |
| Toolkit+DenyExternal | mp4 | solid-red-1s.mp4 | data | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足）；所有帧采样点相同（最大差异 0，共 11 帧） |
| Toolkit+AllowHostResolved | mp4 | solid-red-1s.mp4 | data | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足）；所有帧采样点相同（最大差异 0，共 11 帧） |
| Toolkit+AllowAll | mp4 | solid-red-1s.mp4 | data | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足）；所有帧采样点相同（最大差异 0，共 11 帧） |
| Browser | mp4 | solid-red-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足）；所有帧采样点相同（最大差异 0，共 11 帧） |
| Toolkit+DenyExternal | mp4 | solid-red-1s.mp4 | file | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+AllowHostResolved | mp4 | solid-red-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足）；所有帧采样点相同（最大差异 0，共 11 帧） |
| Toolkit+AllowAll | mp4 | solid-red-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足）；所有帧采样点相同（最大差异 0，共 11 帧） |
| Browser | mp4 | solid-red-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足）；所有帧采样点相同（最大差异 0，共 11 帧） |
| Toolkit+DenyExternal | mp4 | solid-red-1s.mp4 | rel | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+AllowHostResolved | mp4 | solid-red-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足）；所有帧采样点相同（最大差异 0，共 11 帧） |
| Toolkit+AllowAll | mp4 | solid-red-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足）；所有帧采样点相同（最大差异 0，共 11 帧） |
| Browser | mp4 | testsrc-1s.mp4 | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | mp4 | testsrc-1s.mp4 | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | mp4 | testsrc-1s.mp4 | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | mp4 | testsrc-1s.mp4 | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | mp4 | testsrc-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | mp4 | testsrc-1s.mp4 | file | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+AllowHostResolved | mp4 | testsrc-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | mp4 | testsrc-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | mp4 | testsrc-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | mp4 | testsrc-1s.mp4 | rel | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+AllowHostResolved | mp4 | testsrc-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | mp4 | testsrc-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | webm | testsrc-1s.webm | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | webm | testsrc-1s.webm | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | webm | testsrc-1s.webm | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | webm | testsrc-1s.webm | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | webm | testsrc-1s.webm | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | webm | testsrc-1s.webm | file | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+AllowHostResolved | webm | testsrc-1s.webm | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | webm | testsrc-1s.webm | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | webm | testsrc-1s.webm | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | webm | testsrc-1s.webm | rel | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+AllowHostResolved | webm | testsrc-1s.webm | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | webm | testsrc-1s.webm | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | mp4 | twophase-1s.mp4 | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | mp4 | twophase-1s.mp4 | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | mp4 | twophase-1s.mp4 | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | mp4 | twophase-1s.mp4 | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | mp4 | twophase-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | mp4 | twophase-1s.mp4 | file | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+AllowHostResolved | mp4 | twophase-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | mp4 | twophase-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | mp4 | twophase-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | mp4 | twophase-1s.mp4 | rel | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+AllowHostResolved | mp4 | twophase-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | mp4 | twophase-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | png | with space.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | with space.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | png | with space.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | with space.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | with space.png | file | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | png | with space.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | png | with space.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | with space.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | with space.png | rel | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | png | with space.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | png | with space.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | with space.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | zero-bytes.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 失败路径：未绘制 + onerror 已派发（契约正确） |
| Toolkit+DenyExternal | png | zero-bytes.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | png | zero-bytes.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 失败路径：未绘制 + onerror 已派发（契约正确） |
| Toolkit+AllowAll | png | zero-bytes.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 失败路径：未绘制 + onerror 已派发（契约正确） |
| Browser | png | zero-bytes.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 失败路径：未绘制 + onerror 已派发（契约正确） |
| Toolkit+DenyExternal | png | zero-bytes.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | png | zero-bytes.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 失败路径：未绘制 + onerror 已派发（契约正确） |
| Toolkit+AllowAll | png | zero-bytes.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 失败路径：未绘制 + onerror 已派发（契约正确） |
| Browser | png | 中文名-方块.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | 中文名-方块.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | png | 中文名-方块.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | 中文名-方块.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | 中文名-方块.png | file | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | png | 中文名-方块.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | png | 中文名-方块.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | 中文名-方块.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | 中文名-方块.png | rel | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | png | 中文名-方块.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | png | 中文名-方块.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | 中文名-方块.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | 图标-方块.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | 图标-方块.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | png | 图标-方块.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | 图标-方块.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | 图标-方块.png | file | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | png | 图标-方块.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | png | 图标-方块.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | 图标-方块.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | 图标-方块.png | rel | ❌ | ❌ | ✅ | ❌ | — | **L2** | 画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷） |
| Toolkit+DenyExternal | png | 图标-方块.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+AllowHostResolved | png | 图标-方块.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | 图标-方块.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |

## 缺陷清单（按影响排序）

| 配置 | 样本 | 来源 | 现象 |
|---|---|---|---|
| Browser | quad.tiff | data | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Browser | quad.tiff | file | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Browser | quad.tiff | rel | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Toolkit+DenyExternal | quad.png | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | quad.png | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | quad.jpg | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | quad.jpg | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | quad.gif | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | quad.gif | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | quad-lossy.webp | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | quad-lossy.webp | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | quad-lossless.webp | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | quad-lossless.webp | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | quad.bmp | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | quad.bmp | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | quad-64.ico | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | quad-64.ico | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | quad.tiff | data | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Toolkit+DenyExternal | quad.tiff | file | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Toolkit+DenyExternal | quad.tiff | rel | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Toolkit+DenyExternal | quad.avif | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | quad.avif | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | gradient.png | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | gradient.png | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | anim-3frames-rgb.gif | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | anim-3frames-rgb.gif | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | anim-uneven-delay.gif | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | anim-uneven-delay.gif | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | anim-noloop.gif | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | anim-noloop.gif | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | anim-2frames.webp | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | anim-2frames.webp | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | mislabeled.png | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | mislabeled.png | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | huge-4096.png | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | huge-4096.png | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | 中文名-方块.png | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | 中文名-方块.png | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | with space.png | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | with space.png | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | 图标-方块.png | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | 图标-方块.png | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | rect-120x80.svg | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | rect-120x80.svg | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | ratio-only.svg | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | ratio-only.svg | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | icon-24.svg | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | icon-24.svg | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷） |
| Toolkit+DenyExternal | testsrc-1s.mp4 | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+DenyExternal | testsrc-1s.mp4 | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+DenyExternal | solid-red-1s.mp4 | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+DenyExternal | solid-red-1s.mp4 | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+DenyExternal | twophase-1s.mp4 | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+DenyExternal | twophase-1s.mp4 | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+DenyExternal | testsrc-1s.webm | file | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+DenyExternal | testsrc-1s.webm | rel | 资源策略 deny-external 拒绝该引用（预期，非缺陷）；实测元素 error.code=4（MEDIA_ERR_SRC_NOT_SUPPORTED） |
| Toolkit+AllowHostResolved | quad.tiff | data | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Toolkit+AllowHostResolved | quad.tiff | file | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Toolkit+AllowHostResolved | quad.tiff | rel | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Toolkit+AllowAll | quad.tiff | data | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Toolkit+AllowAll | quad.tiff | file | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |
| Toolkit+AllowAll | quad.tiff | rel | 预期不支持（浏览器同样不支持 TIFF，L0 即对齐） |

> 标注「资源策略 … 拒绝该引用（预期，非缺陷）」的行是配置策略的**正确行为**（文档 §9.9 门禁收口），不是实现缺陷；真正的缺陷行不含此标注。

## 策略一致性（决策 1 · TC-M-905）

- 全图差异像素比例：**0.0000%**（其中 12 个动画样本格未计入下方判定）
- 排除动画样本格后：**0.0000%** —— ✅ 非动画区域逐像素一致

## 音频（A3-1：宿主 ffmpeg 解码 → 引擎 PCM 通道 → 输出后端）

### 平台支持矩阵

| 平台 | PCM 解码（ffmpeg） | 播放时钟 | 内置输出后端 | 降级行为 |
|---|---|---|---|---|
| Windows | ✅ | ✅ 设备位置（waveOutGetPosition） | ✅ waveOut | — |
| Linux/macOS | ✅ | ✅ 挂钟 + 已交付帧 | ❌（未实现） | PCM → 宿主 tap（宿主自备 ALSA/CoreAudio 输出） |

本次运行平台：**windows**；本机内置输出设备：**可用**。

### 判据 A：宿主输出回调的 PCM 频谱（L4-S 主判据）

复查命令：`cmd/psai -media`（探针在 `app.PCMTap` 上取「即将写进输出设备的同一份 PCM」做 FFT）

| 配置 | 样本 | 来源 | 交付帧数 | 主峰 Hz | 期望 Hz | 幅度 | 判定 |
|---|---|---|---|---|---|---|---|
| Browser | sine-440-1s.wav | data | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Browser | sine-440-1s.wav | file | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Browser | sine-440-1s.wav | rel | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Browser | sine-440-1s.mp3 | data | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Browser | sine-440-1s.mp3 | file | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Browser | sine-440-1s.mp3 | rel | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Browser | sine-440-1s.ogg | data | 47861 | 439.87 | 440 | 1.000 | ✅ |
| Browser | sine-440-1s.ogg | file | 47861 | 439.87 | 440 | 1.000 | ✅ |
| Browser | sine-440-1s.ogg | rel | 47861 | 439.87 | 440 | 1.000 | ✅ |
| Browser | sine-440-1s.m4a | data | 49041 | 439.88 | 440 | 1.000 | ✅ |
| Browser | sine-440-1s.m4a | file | 49041 | 439.88 | 440 | 1.000 | ✅ |
| Browser | sine-440-1s.m4a | rel | 49041 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.wav | data | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.mp3 | data | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.ogg | data | 47861 | 439.87 | 440 | 1.000 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.m4a | data | 49041 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.wav | data | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.wav | file | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.wav | rel | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.mp3 | data | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.mp3 | file | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.mp3 | rel | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.ogg | data | 47861 | 439.87 | 440 | 1.000 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.ogg | file | 47861 | 439.87 | 440 | 1.000 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.ogg | rel | 47861 | 439.87 | 440 | 1.000 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.m4a | data | 49041 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.m4a | file | 49041 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.m4a | rel | 49041 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowAll | sine-440-1s.wav | data | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowAll | sine-440-1s.wav | file | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowAll | sine-440-1s.wav | rel | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowAll | sine-440-1s.mp3 | data | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowAll | sine-440-1s.mp3 | file | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowAll | sine-440-1s.mp3 | rel | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowAll | sine-440-1s.ogg | data | 47861 | 439.87 | 440 | 1.000 | ✅ |
| Toolkit+AllowAll | sine-440-1s.ogg | file | 47861 | 439.87 | 440 | 1.000 | ✅ |
| Toolkit+AllowAll | sine-440-1s.ogg | rel | 47861 | 439.87 | 440 | 1.000 | ✅ |
| Toolkit+AllowAll | sine-440-1s.m4a | data | 49041 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowAll | sine-440-1s.m4a | file | 49041 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowAll | sine-440-1s.m4a | rel | 49041 | 439.88 | 440 | 1.000 | ✅ |

### 判据 B：环回录音（可选，外部视角）

命令：`ffmpeg -hide_banner -v error -f dshow -i "audio=Voicemeeter Out B3 (VB-Audio Voicemeeter VAIO)" -t 1.0 -f s16le -ar 48000 -ac 2 -`

- 设备：Voicemeeter Out B3 (VB-Audio Voicemeeter VAIO)
- 结果：**mismatch**
- 录回音频主峰：4634.8 Hz（幅度 0.032）
- 说明：录回主峰 4634.8Hz 与期望 440Hz 不符（幅度 0.032）
- 幅度对照：判据 A 的同一份待输出 PCM 幅度 **1.000**；录回幅度仅为其 **1/32** ⇒ 基本只录到底噪，该采集设备未被系统输出喂到。

**判定条件与语义**（本节口径）：判据 B 是**可选的外部复核**（端到端视角：
解码 → 输出设备 → 采集设备），四种取值都**如实记录、都不伪装成通过**：
- `ok`：录回主峰落在期望 ±容差 ⇒ 端到端成立；
- `SKIP(no-loopback)`：无 ffmpeg，或无环回/虚拟声卡候选 ⇒ 跳过；
- `SKIP(loopback-failed)`：有候选但打开失败（被独占）⇒ 跳过；
- `mismatch`：录到数据但主峰不符 ⇒ **不视为通过，也不阻塞验收**。
L4-S 的**主线判据**是 A（同一份待输出 PCM 的 FFT 主峰/幅度）——它证明的是
「要写进输出设备的字节正确」；B 只补「设备真的出声且被采回」。因此 B 不可用
时验收仍成立，只是端到端那一环缺外部证据（本机即此情形）。

### TC-M-602 定点：播放时钟（音频为主时钟）

判据：①终态 `currentTime` 到达 duration；②采样点 `currentTime` **不超前**于「该会话首块 PCM 交付以来经过的时间」（设备位置只可能 ≤ 已交付且已播出的时间，超前即说明时钟不是输出位置驱动的），滞后容许 0.45s（设备启动延迟与缓冲深度）。

引擎侧的**严格**断言另见 `engine/js/bindings/mediaaudio_test.go`（`TestAudioSessionDrivesCurrentTime`：currentTime 逐值精确等于会话 Position）。

| 配置 | 样本 | 来源 | 采样点（首块后 s） | currentTime | 期望 | 结束 currentTime | 判定 |
|---|---|---|---|---|---|---|---|
| Browser | sine-440-1s.wav | data | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Browser | sine-440-1s.wav | file | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Browser | sine-440-1s.wav | rel | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Browser | sine-440-1s.mp3 | data | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Browser | sine-440-1s.mp3 | file | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Browser | sine-440-1s.mp3 | rel | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Browser | sine-440-1s.ogg | data | 0.95 | 0.89 | 0.95 | 1.00 | ✅ |
| Browser | sine-440-1s.ogg | file | 0.78 | 0.73 | 0.78 | 1.00 | ✅ |
| Browser | sine-440-1s.ogg | rel | 0.64 | 0.58 | 0.64 | 1.00 | ✅ |
| Browser | sine-440-1s.m4a | data | 0.48 | 0.42 | 0.48 | 1.00 | ✅ |
| Browser | sine-440-1s.m4a | file | 0.33 | 0.29 | 0.33 | 1.00 | ✅ |
| Browser | sine-440-1s.m4a | rel | 0.15 | 0.00 | 0.15 | 1.00 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.wav | data | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.wav | file | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Toolkit+DenyExternal | sine-440-1s.wav | rel | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Toolkit+DenyExternal | sine-440-1s.mp3 | data | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.mp3 | file | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Toolkit+DenyExternal | sine-440-1s.mp3 | rel | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Toolkit+DenyExternal | sine-440-1s.ogg | data | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.ogg | file | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Toolkit+DenyExternal | sine-440-1s.ogg | rel | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Toolkit+DenyExternal | sine-440-1s.m4a | data | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.m4a | file | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Toolkit+DenyExternal | sine-440-1s.m4a | rel | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Toolkit+AllowHostResolved | sine-440-1s.wav | data | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.wav | file | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.wav | rel | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.mp3 | data | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.mp3 | file | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.mp3 | rel | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.ogg | data | 0.98 | 0.93 | 0.98 | 1.00 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.ogg | file | 0.80 | 0.73 | 0.80 | 1.00 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.ogg | rel | 0.65 | 0.59 | 0.65 | 1.00 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.m4a | data | 0.47 | 0.41 | 0.47 | 1.00 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.m4a | file | 0.31 | 0.23 | 0.31 | 1.00 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.m4a | rel | 0.15 | 0.00 | 0.15 | 1.00 | ✅ |
| Toolkit+AllowAll | sine-440-1s.wav | data | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Toolkit+AllowAll | sine-440-1s.wav | file | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Toolkit+AllowAll | sine-440-1s.wav | rel | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Toolkit+AllowAll | sine-440-1s.mp3 | data | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Toolkit+AllowAll | sine-440-1s.mp3 | file | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Toolkit+AllowAll | sine-440-1s.mp3 | rel | 1.00 | 1.00 | 1.00 | 1.00 | ✅ |
| Toolkit+AllowAll | sine-440-1s.ogg | data | 0.92 | 0.86 | 0.92 | 1.00 | ✅ |
| Toolkit+AllowAll | sine-440-1s.ogg | file | 0.78 | 0.71 | 0.78 | 1.00 | ✅ |
| Toolkit+AllowAll | sine-440-1s.ogg | rel | 0.62 | 0.55 | 0.62 | 1.00 | ✅ |
| Toolkit+AllowAll | sine-440-1s.m4a | data | 0.47 | 0.41 | 0.47 | 1.00 | ✅ |
| Toolkit+AllowAll | sine-440-1s.m4a | file | 0.32 | 0.24 | 0.32 | 1.00 | ✅ |
| Toolkit+AllowAll | sine-440-1s.m4a | rel | 0.15 | 0.00 | 0.15 | 1.00 | ✅ |

> 容差 **±0.2s**（采样开销 + 设备启动延迟）。`data:` 来源没有本地路径（宿主解不了码），采样时无 PCM、时钟未起步——不在判据 A 的覆盖范围，保持 L1。

### TC-M-603：WebAudio（`AudioContext` / `decodeAudioData` + 音频图）

> 范围：按 `audio-backend-proposal.md` 的 **A3-3**——**最小子集**（`AudioContext` / `decodeAudioData` **真解码** → `AudioBuffer`）之上已扩展为**完整子集**：AudioNode 图（8 类节点）、`AudioParam` 全套自动化、`OfflineAudioContext`、实时输出。
> **不做**（调用即抛错，不返回假节点）：`AudioWorklet` / `ScriptProcessorNode` / 多声道路由（`ChannelMerger`·`ChannelSplitter`）/ 3D `PannerNode` / `ConvolverNode` / `DynamicsCompressorNode`；已知限制（立体声内核、多实时上下文不混音等）见该文档文首「实施状态」。

| 项 | 实测 |
|---|---|
| 特性检测 `typeof AudioContext === "function"` | ✅ |
| 别名 `webkitAudioContext` | ✅ |
| `AudioContext.sampleRate` / `state` | 48000 / `running` |
| 音频图工厂（9 个 `create*`） | ✅ |
| `OfflineAudioContext` 可用 | ✅ |
| 未实现的多声道路由**调用即抛错** | ✅ |
| 离线渲染（`ConstantSource(1) → Gain(0.25)`，256 帧 / 1 声道） | 帧数 256、声道 1、样本 min=0.25 / max=0.25（期望**恒为** 0.25）、`getChannelData` 视图 ✅ |
| 输入字节（`sine-440-1s.wav`） | 88278 字节 |
| `decodeAudioData` → AudioBuffer | ✅ sampleRate=44100、声道=1、length=44100 帧、duration=1.000s |
| `getChannelData(0) instanceof Float32Array` | ✅ |
| 样本峰值 | 0.1250（样本自身幅度，非满幅） |
| 频谱主峰（Go 侧 FFT，判据 A 同一套工具） | 439.95 Hz（幅度 0.108） |

**判定：✅ 达成**——页面脚本在真实宿主环境里拿到 `AudioContext`，把一段真音频字节解成 `AudioBuffer`（样本量纲经 FFT 复核为 440Hz 正弦），并且**音频图逐样本算对**（离线渲染的每个样本都精确等于设定的增益 0.25）。

## 与基线的差异

- ⚠️ Browser / anim-2frames.webp / file：L4 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / anim-2frames.webp / rel：L4 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / anim-3frames-rgb.gif / file：L4 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / anim-3frames-rgb.gif / rel：L4 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / anim-noloop.gif / file：L4 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / anim-uneven-delay.gif / file：L4 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / anim-uneven-delay.gif / rel：L4 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / gradient.png / file：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / gradient.png / rel：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / huge-4096.png / file：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / huge-4096.png / rel：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / mislabeled.png / file：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / mislabeled.png / rel：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / quad-64.ico / file：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / quad-64.ico / rel：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / quad-lossless.webp / file：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / quad-lossless.webp / rel：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / quad-lossy.webp / file：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / quad-lossy.webp / rel：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / quad.avif / file：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / quad.avif / rel：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / quad.bmp / file：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / quad.bmp / rel：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / quad.gif / file：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / quad.gif / rel：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / quad.jpg / file：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / quad.jpg / rel：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / quad.png / file：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / quad.png / rel：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / with space.png / file：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / with space.png / rel：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / 中文名-方块.png / file：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / 中文名-方块.png / rel：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / 图标-方块.png / file：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Browser / 图标-方块.png / rel：L3 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Toolkit+AllowAll / anim-2frames.webp / file：L4 → L2（画得出但契约不完整（complete/onload））
- ⚠️ Toolkit+AllowAll / anim-3frames-rgb.gif / file：L4 → L2（画得出但契约不完整（complete/onload））
- ⚠️ Toolkit+AllowAll / anim-noloop.gif / file：L4 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Toolkit+AllowAll / anim-noloop.gif / rel：L4 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Toolkit+AllowAll / anim-uneven-delay.gif / file：L4 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））
- ⚠️ Toolkit+AllowAll / anim-uneven-delay.gif / rel：L4 → L2（画得出但无固有尺寸（D4：<img> 未给尺寸时盒子塌陷））

## Edge 双端对照（决策 2、§8.3 第 2 条）

判据：引擎等级不低于 Edge 等级。

| 项 | 值 |
|---|---|
| 结论 | SKIP(未启用 -media-edge) —— **未执行，不算通过** |

## 原始证据索引

- Browser：截图 `dev/media/out/matrix-Browser.png`、播放态 `dev/media/out/matrix-Browser-playing.png`、几何/事件 `dev/media/out/geom-Browser.json`
- Toolkit+DenyExternal：截图 `dev/media/out/matrix-Toolkit-DenyExternal.png`、播放态 `dev/media/out/matrix-Toolkit-DenyExternal-playing.png`、几何/事件 `dev/media/out/geom-Toolkit-DenyExternal.json`
- Toolkit+AllowHostResolved：截图 `dev/media/out/matrix-Toolkit-AllowHostResolved.png`、播放态 `dev/media/out/matrix-Toolkit-AllowHostResolved-playing.png`、几何/事件 `dev/media/out/geom-Toolkit-AllowHostResolved.json`
- Toolkit+AllowAll：截图 `dev/media/out/matrix-Toolkit-AllowAll.png`、播放态 `dev/media/out/matrix-Toolkit-AllowAll-playing.png`、几何/事件 `dev/media/out/geom-Toolkit-AllowAll.json`
- 矩阵页：`dev/media/samples/_matrix.html`（与样本同级，文档相对路径来源依赖此位置）
- 样本清单：`dev\media\samples\manifest.json`

> 截图为预乘 RGBA 反预乘后的 sRGB；判定基于像素采样（非人眼），截图供 `read_image` 复核。
