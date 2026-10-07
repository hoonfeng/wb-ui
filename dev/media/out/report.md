# 媒体格式真实可用性报告（2026-10-07 17:23 0f4cd10）

## 环境

配置：Browser / Toolkit+DenyExternal / Toolkit+AllowHostResolved / Toolkit+AllowAll ｜ 引擎：0f4cd10 ｜ 样本：本地脚本生成、不入库（决策 5）｜ 模型：L0–L4（文档 §2）

复现：`python dev/media/gen_samples.py` → `cmd/psai -media`（本机按需，不入 CI 门禁——决策 6）

## 总表

| 配置 | 格式 | 样本 | 来源 | 加载 | 几何 | 绘制 | 契约 | 动画 | 等级 | 备注 |
|---|---|---|---|---|---|---|---|---|---|---|
| Browser | webp | anim-2frames.webp | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | webp | anim-2frames.webp | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | webp | anim-2frames.webp | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | webp | anim-2frames.webp | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | webp | anim-2frames.webp | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | webp | anim-2frames.webp | file | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+AllowHostResolved | webp | anim-2frames.webp | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | webp | anim-2frames.webp | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | webp | anim-2frames.webp | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | webp | anim-2frames.webp | rel | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+AllowHostResolved | webp | anim-2frames.webp | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | webp | anim-2frames.webp | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-3frames-rgb.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-3frames-rgb.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | gif | anim-3frames-rgb.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-3frames-rgb.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-3frames-rgb.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-3frames-rgb.gif | file | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+AllowHostResolved | gif | anim-3frames-rgb.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-3frames-rgb.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-3frames-rgb.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-3frames-rgb.gif | rel | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+AllowHostResolved | gif | anim-3frames-rgb.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-3frames-rgb.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-noloop.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-noloop.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | gif | anim-noloop.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-noloop.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-noloop.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-noloop.gif | file | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+AllowHostResolved | gif | anim-noloop.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-noloop.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-noloop.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-noloop.gif | rel | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+AllowHostResolved | gif | anim-noloop.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-noloop.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-uneven-delay.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-uneven-delay.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | gif | anim-uneven-delay.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-uneven-delay.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-uneven-delay.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-uneven-delay.gif | file | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+AllowHostResolved | gif | anim-uneven-delay.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-uneven-delay.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-uneven-delay.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-uneven-delay.gif | rel | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** | 所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+AllowHostResolved | gif | anim-uneven-delay.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-uneven-delay.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | png | corrupt.png | data | ✅ | ✅ | ❌ | ✅ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Toolkit+DenyExternal | png | corrupt.png | data | ✅ | ✅ | ❌ | ✅ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Toolkit+AllowHostResolved | png | corrupt.png | data | ✅ | ✅ | ❌ | ✅ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Toolkit+AllowAll | png | corrupt.png | data | ✅ | ✅ | ❌ | ✅ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Browser | png | corrupt.png | file | ✅ | ✅ | ❌ | ✅ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Toolkit+DenyExternal | png | corrupt.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Toolkit+AllowHostResolved | png | corrupt.png | file | ✅ | ✅ | ❌ | ✅ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Toolkit+AllowAll | png | corrupt.png | file | ✅ | ✅ | ❌ | ✅ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Browser | png | corrupt.png | rel | ✅ | ✅ | ❌ | ✅ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Toolkit+DenyExternal | png | corrupt.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Toolkit+AllowHostResolved | png | corrupt.png | rel | ✅ | ✅ | ❌ | ✅ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Toolkit+AllowAll | png | corrupt.png | rel | ✅ | ✅ | ❌ | ✅ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Browser | png | gradient.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | gradient.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | png | gradient.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | gradient.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | gradient.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | gradient.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | png | gradient.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | gradient.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | gradient.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | gradient.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | png | gradient.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | gradient.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | huge-4096.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | huge-4096.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | png | huge-4096.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | huge-4096.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | huge-4096.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | huge-4096.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | png | huge-4096.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | huge-4096.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | huge-4096.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | huge-4096.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | png | huge-4096.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | huge-4096.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | svg | icon-24.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | svg | icon-24.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | svg | icon-24.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | svg | icon-24.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | svg | icon-24.svg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | svg | icon-24.svg | file | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | svg | icon-24.svg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | svg | icon-24.svg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | svg | icon-24.svg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | svg | icon-24.svg | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
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
| Browser | jpeg | mislabeled.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | jpeg | mislabeled.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | jpeg | mislabeled.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | jpeg | mislabeled.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | jpeg | mislabeled.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | jpeg | mislabeled.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | jpeg | mislabeled.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | jpeg | mislabeled.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | ico | quad-64.ico | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | ico | quad-64.ico | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | ico | quad-64.ico | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | ico | quad-64.ico | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | ico | quad-64.ico | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | ico | quad-64.ico | file | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | ico | quad-64.ico | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | ico | quad-64.ico | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | ico | quad-64.ico | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | ico | quad-64.ico | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | ico | quad-64.ico | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | ico | quad-64.ico | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | webp | quad-lossless.webp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | webp | quad-lossless.webp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | webp | quad-lossless.webp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | webp | quad-lossless.webp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | webp | quad-lossless.webp | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | webp | quad-lossless.webp | file | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | webp | quad-lossless.webp | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | webp | quad-lossless.webp | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | webp | quad-lossless.webp | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | webp | quad-lossless.webp | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | webp | quad-lossless.webp | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | webp | quad-lossless.webp | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | webp | quad-lossy.webp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | webp | quad-lossy.webp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | webp | quad-lossy.webp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | webp | quad-lossy.webp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | webp | quad-lossy.webp | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | webp | quad-lossy.webp | file | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | webp | quad-lossy.webp | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | webp | quad-lossy.webp | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | webp | quad-lossy.webp | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | webp | quad-lossy.webp | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | webp | quad-lossy.webp | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | webp | quad-lossy.webp | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | avif | quad.avif | data | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Toolkit+DenyExternal | avif | quad.avif | data | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowHostResolved | avif | quad.avif | data | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowAll | avif | quad.avif | data | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Browser | avif | quad.avif | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Toolkit+DenyExternal | avif | quad.avif | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowHostResolved | avif | quad.avif | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowAll | avif | quad.avif | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Browser | avif | quad.avif | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Toolkit+DenyExternal | avif | quad.avif | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowHostResolved | avif | quad.avif | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowAll | avif | quad.avif | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Browser | bmp | quad.bmp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | bmp | quad.bmp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | bmp | quad.bmp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | bmp | quad.bmp | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | bmp | quad.bmp | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | bmp | quad.bmp | file | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | bmp | quad.bmp | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | bmp | quad.bmp | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | bmp | quad.bmp | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | bmp | quad.bmp | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | bmp | quad.bmp | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | bmp | quad.bmp | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | gif | quad.gif | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | gif | quad.gif | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | gif | quad.gif | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | gif | quad.gif | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | gif | quad.gif | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | gif | quad.gif | file | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | gif | quad.gif | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | gif | quad.gif | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | gif | quad.gif | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | gif | quad.gif | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | gif | quad.gif | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | gif | quad.gif | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | jpeg | quad.jpg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | jpeg | quad.jpg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | jpeg | quad.jpg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | jpeg | quad.jpg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | jpeg | quad.jpg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | jpeg | quad.jpg | file | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | jpeg | quad.jpg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | jpeg | quad.jpg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | jpeg | quad.jpg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | jpeg | quad.jpg | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | jpeg | quad.jpg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | jpeg | quad.jpg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | quad.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | quad.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | png | quad.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | quad.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | quad.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | quad.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | png | quad.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | quad.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | quad.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | quad.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | png | quad.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | quad.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | tiff | quad.tiff | data | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Toolkit+DenyExternal | tiff | quad.tiff | data | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowHostResolved | tiff | quad.tiff | data | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowAll | tiff | quad.tiff | data | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Browser | tiff | quad.tiff | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Toolkit+DenyExternal | tiff | quad.tiff | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowHostResolved | tiff | quad.tiff | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowAll | tiff | quad.tiff | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Browser | tiff | quad.tiff | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Toolkit+DenyExternal | tiff | quad.tiff | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowHostResolved | tiff | quad.tiff | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowAll | tiff | quad.tiff | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 预期不支持（L0，写入基线，不投入） |
| Browser | svg | ratio-only.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | svg | ratio-only.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | svg | ratio-only.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | svg | ratio-only.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | svg | ratio-only.svg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | svg | ratio-only.svg | file | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | svg | ratio-only.svg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | svg | ratio-only.svg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | svg | ratio-only.svg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | svg | ratio-only.svg | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | svg | ratio-only.svg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | svg | ratio-only.svg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | svg | rect-120x80.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | svg | rect-120x80.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | svg | rect-120x80.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | svg | rect-120x80.svg | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | svg | rect-120x80.svg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | svg | rect-120x80.svg | file | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | svg | rect-120x80.svg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | svg | rect-120x80.svg | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | svg | rect-120x80.svg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | svg | rect-120x80.svg | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | svg | rect-120x80.svg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | svg | rect-120x80.svg | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | m4a | sine-440-1s.m4a | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；未采到 PCM（无音轨 / 无输出后端） |
| Toolkit+DenyExternal | m4a | sine-440-1s.m4a | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；未采到 PCM（无音轨 / 无输出后端） |
| Toolkit+AllowHostResolved | m4a | sine-440-1s.m4a | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；未采到 PCM（无音轨 / 无输出后端） |
| Toolkit+AllowAll | m4a | sine-440-1s.m4a | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；未采到 PCM（无音轨 / 无输出后端） |
| Browser | m4a | sine-440-1s.m4a | file | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 49041 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.20s、结束时 1.00s（TC-M-602） |
| Toolkit+DenyExternal | m4a | sine-440-1s.m4a | file | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 49041 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.00s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowHostResolved | m4a | sine-440-1s.m4a | file | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 49041 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.20s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowAll | m4a | sine-440-1s.m4a | file | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 49041 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.20s、结束时 1.00s（TC-M-602） |
| Browser | m4a | sine-440-1s.m4a | rel | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 49041 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.00s、结束时 1.00s（TC-M-602） |
| Toolkit+DenyExternal | m4a | sine-440-1s.m4a | rel | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 49041 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.00s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowHostResolved | m4a | sine-440-1s.m4a | rel | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 49041 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.00s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowAll | m4a | sine-440-1s.m4a | rel | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 49041 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.00s、结束时 1.00s（TC-M-602） |
| Browser | mp3 | sine-440-1s.mp3 | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；未采到 PCM（无音轨 / 无输出后端） |
| Toolkit+DenyExternal | mp3 | sine-440-1s.mp3 | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；未采到 PCM（无音轨 / 无输出后端） |
| Toolkit+AllowHostResolved | mp3 | sine-440-1s.mp3 | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；未采到 PCM（无音轨 / 无输出后端） |
| Toolkit+AllowAll | mp3 | sine-440-1s.mp3 | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；未采到 PCM（无音轨 / 无输出后端） |
| Browser | mp3 | sine-440-1s.mp3 | file | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.52s、结束时 1.00s（TC-M-602） |
| Toolkit+DenyExternal | mp3 | sine-440-1s.mp3 | file | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.51s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowHostResolved | mp3 | sine-440-1s.mp3 | file | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.53s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowAll | mp3 | sine-440-1s.mp3 | file | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.53s、结束时 1.00s（TC-M-602） |
| Browser | mp3 | sine-440-1s.mp3 | rel | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.44s、结束时 1.00s（TC-M-602） |
| Toolkit+DenyExternal | mp3 | sine-440-1s.mp3 | rel | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.43s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowHostResolved | mp3 | sine-440-1s.mp3 | rel | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.45s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowAll | mp3 | sine-440-1s.mp3 | rel | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.44s、结束时 1.00s（TC-M-602） |
| Browser | ogg | sine-440-1s.ogg | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；未采到 PCM（无音轨 / 无输出后端） |
| Toolkit+DenyExternal | ogg | sine-440-1s.ogg | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；未采到 PCM（无音轨 / 无输出后端） |
| Toolkit+AllowHostResolved | ogg | sine-440-1s.ogg | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；未采到 PCM（无音轨 / 无输出后端） |
| Toolkit+AllowAll | ogg | sine-440-1s.ogg | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；未采到 PCM（无音轨 / 无输出后端） |
| Browser | ogg | sine-440-1s.ogg | file | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 47861 帧、PCM 主峰 439.87Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.36s、结束时 1.00s（TC-M-602） |
| Toolkit+DenyExternal | ogg | sine-440-1s.ogg | file | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 47861 帧、PCM 主峰 439.87Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.31s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowHostResolved | ogg | sine-440-1s.ogg | file | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 47861 帧、PCM 主峰 439.87Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.37s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowAll | ogg | sine-440-1s.ogg | file | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 47861 帧、PCM 主峰 439.87Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.37s、结束时 1.00s（TC-M-602） |
| Browser | ogg | sine-440-1s.ogg | rel | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 47861 帧、PCM 主峰 439.87Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.26s、结束时 1.00s（TC-M-602） |
| Toolkit+DenyExternal | ogg | sine-440-1s.ogg | rel | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 47861 帧、PCM 主峰 439.87Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.26s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowHostResolved | ogg | sine-440-1s.ogg | rel | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 47861 帧、PCM 主峰 439.87Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.26s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowAll | ogg | sine-440-1s.ogg | rel | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 47861 帧、PCM 主峰 439.87Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.26s、结束时 1.00s（TC-M-602） |
| Browser | wav | sine-440-1s.wav | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；未采到 PCM（无音轨 / 无输出后端） |
| Toolkit+DenyExternal | wav | sine-440-1s.wav | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；未采到 PCM（无音轨 / 无输出后端） |
| Toolkit+AllowHostResolved | wav | sine-440-1s.wav | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；未采到 PCM（无音轨 / 无输出后端） |
| Toolkit+AllowAll | wav | sine-440-1s.wav | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；未采到 PCM（无音轨 / 无输出后端） |
| Browser | wav | sine-440-1s.wav | file | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.70s、结束时 1.00s（TC-M-602） |
| Toolkit+DenyExternal | wav | sine-440-1s.wav | file | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.66s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowHostResolved | wav | sine-440-1s.wav | file | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.68s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowAll | wav | sine-440-1s.wav | file | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.70s、结束时 1.00s（TC-M-602） |
| Browser | wav | sine-440-1s.wav | rel | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.60s、结束时 1.00s（TC-M-602） |
| Toolkit+DenyExternal | wav | sine-440-1s.wav | rel | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.58s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowHostResolved | wav | sine-440-1s.wav | rel | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.60s、结束时 1.00s（TC-M-602） |
| Toolkit+AllowAll | wav | sine-440-1s.wav | rel | ✅ | ✅ | ❌ | ✅ | — | **L4** | 音频输出：交付 48000 帧、PCM 主峰 439.88Hz（期望 440Hz，判据 A）；play() 后 500ms currentTime=0.61s、结束时 1.00s（TC-M-602） |
| Browser | mp4 | solid-red-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+DenyExternal | mp4 | solid-red-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+AllowHostResolved | mp4 | solid-red-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+AllowAll | mp4 | solid-red-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Browser | mp4 | solid-red-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+DenyExternal | mp4 | solid-red-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+AllowHostResolved | mp4 | solid-red-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+AllowAll | mp4 | solid-red-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Browser | mp4 | solid-red-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+DenyExternal | mp4 | solid-red-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+AllowHostResolved | mp4 | solid-red-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+AllowAll | mp4 | solid-red-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Browser | mp4 | testsrc-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+DenyExternal | mp4 | testsrc-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+AllowHostResolved | mp4 | testsrc-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+AllowAll | mp4 | testsrc-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Browser | mp4 | testsrc-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | mp4 | testsrc-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | mp4 | testsrc-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | mp4 | testsrc-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | mp4 | testsrc-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | mp4 | testsrc-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | mp4 | testsrc-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | mp4 | testsrc-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | webm | testsrc-1s.webm | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+DenyExternal | webm | testsrc-1s.webm | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+AllowHostResolved | webm | testsrc-1s.webm | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+AllowAll | webm | testsrc-1s.webm | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Browser | webm | testsrc-1s.webm | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | webm | testsrc-1s.webm | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | webm | testsrc-1s.webm | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | webm | testsrc-1s.webm | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | webm | testsrc-1s.webm | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | webm | testsrc-1s.webm | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | webm | testsrc-1s.webm | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | webm | testsrc-1s.webm | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | mp4 | twophase-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+DenyExternal | mp4 | twophase-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+AllowHostResolved | mp4 | twophase-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Toolkit+AllowAll | mp4 | twophase-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期）；所有帧采样点相同（最大差异 0，共 14 帧） |
| Browser | mp4 | twophase-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | mp4 | twophase-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | mp4 | twophase-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | mp4 | twophase-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | mp4 | twophase-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | mp4 | twophase-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | mp4 | twophase-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | mp4 | twophase-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | png | with space.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | with space.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | png | with space.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | with space.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | with space.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | with space.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | png | with space.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | with space.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | with space.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | with space.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | png | with space.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | with space.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | zero-bytes.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 失败路径：未绘制 + onerror 已派发（契约正确） |
| Toolkit+DenyExternal | png | zero-bytes.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Toolkit+AllowHostResolved | png | zero-bytes.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 失败路径：未绘制 + onerror 已派发（契约正确） |
| Toolkit+AllowAll | png | zero-bytes.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** | 失败路径：未绘制 + onerror 已派发（契约正确） |
| Browser | png | zero-bytes.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 失败路径：未绘制 + onerror 已派发（契约正确） |
| Toolkit+DenyExternal | png | zero-bytes.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 失败路径：未绘制，但 onerror 未派发（契约缺陷） |
| Toolkit+AllowHostResolved | png | zero-bytes.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 失败路径：未绘制 + onerror 已派发（契约正确） |
| Toolkit+AllowAll | png | zero-bytes.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** | 失败路径：未绘制 + onerror 已派发（契约正确） |
| Browser | png | 中文名-方块.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | 中文名-方块.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | png | 中文名-方块.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | 中文名-方块.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | 中文名-方块.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | 中文名-方块.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | png | 中文名-方块.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | 中文名-方块.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | 中文名-方块.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | 中文名-方块.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | png | 中文名-方块.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | 中文名-方块.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | 图标-方块.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | 图标-方块.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowHostResolved | png | 图标-方块.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | 图标-方块.png | data | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | 图标-方块.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | 图标-方块.png | file | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | png | 图标-方块.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | 图标-方块.png | file | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Browser | png | 图标-方块.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+DenyExternal | png | 图标-方块.png | rel | ❌ | ❌ | ❌ | ❌ | — | **L0** |  |
| Toolkit+AllowHostResolved | png | 图标-方块.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |
| Toolkit+AllowAll | png | 图标-方块.png | rel | ✅ | ✅ | ✅ | ✅ | — | **L3** |  |

## 缺陷清单（按影响排序）

| 配置 | 样本 | 来源 | 现象 |
|---|---|---|---|
| Browser | quad.tiff | data | 预期不支持（L0，写入基线，不投入） |
| Browser | quad.tiff | file | 预期不支持（L0，写入基线，不投入） |
| Browser | quad.tiff | rel | 预期不支持（L0，写入基线，不投入） |
| Browser | quad.avif | data | 预期不支持（L0，写入基线，不投入） |
| Browser | quad.avif | file | 预期不支持（L0，写入基线，不投入） |
| Browser | quad.avif | rel | 预期不支持（L0，写入基线，不投入） |
| Toolkit+DenyExternal | quad.png | file | 无绘制、无几何 |
| Toolkit+DenyExternal | quad.png | rel | 无绘制、无几何 |
| Toolkit+DenyExternal | quad.jpg | file | 无绘制、无几何 |
| Toolkit+DenyExternal | quad.jpg | rel | 无绘制、无几何 |
| Toolkit+DenyExternal | quad.gif | file | 无绘制、无几何 |
| Toolkit+DenyExternal | quad.gif | rel | 无绘制、无几何 |
| Toolkit+DenyExternal | quad-lossy.webp | file | 无绘制、无几何 |
| Toolkit+DenyExternal | quad-lossy.webp | rel | 无绘制、无几何 |
| Toolkit+DenyExternal | quad-lossless.webp | file | 无绘制、无几何 |
| Toolkit+DenyExternal | quad-lossless.webp | rel | 无绘制、无几何 |
| Toolkit+DenyExternal | quad.bmp | file | 无绘制、无几何 |
| Toolkit+DenyExternal | quad.bmp | rel | 无绘制、无几何 |
| Toolkit+DenyExternal | quad-64.ico | file | 无绘制、无几何 |
| Toolkit+DenyExternal | quad-64.ico | rel | 无绘制、无几何 |
| Toolkit+DenyExternal | quad.tiff | data | 预期不支持（L0，写入基线，不投入） |
| Toolkit+DenyExternal | quad.tiff | file | 预期不支持（L0，写入基线，不投入） |
| Toolkit+DenyExternal | quad.tiff | rel | 预期不支持（L0，写入基线，不投入） |
| Toolkit+DenyExternal | quad.avif | data | 预期不支持（L0，写入基线，不投入） |
| Toolkit+DenyExternal | quad.avif | file | 预期不支持（L0，写入基线，不投入） |
| Toolkit+DenyExternal | quad.avif | rel | 预期不支持（L0，写入基线，不投入） |
| Toolkit+DenyExternal | gradient.png | file | 无绘制、无几何 |
| Toolkit+DenyExternal | gradient.png | rel | 无绘制、无几何 |
| Toolkit+DenyExternal | anim-3frames-rgb.gif | file | 无绘制、无几何 |
| Toolkit+DenyExternal | anim-3frames-rgb.gif | rel | 无绘制、无几何 |
| Toolkit+DenyExternal | anim-uneven-delay.gif | file | 无绘制、无几何 |
| Toolkit+DenyExternal | anim-uneven-delay.gif | rel | 无绘制、无几何 |
| Toolkit+DenyExternal | anim-noloop.gif | file | 无绘制、无几何 |
| Toolkit+DenyExternal | anim-noloop.gif | rel | 无绘制、无几何 |
| Toolkit+DenyExternal | anim-2frames.webp | file | 无绘制、无几何 |
| Toolkit+DenyExternal | anim-2frames.webp | rel | 无绘制、无几何 |
| Toolkit+DenyExternal | mislabeled.png | file | 无绘制、无几何 |
| Toolkit+DenyExternal | mislabeled.png | rel | 无绘制、无几何 |
| Toolkit+DenyExternal | huge-4096.png | file | 无绘制、无几何 |
| Toolkit+DenyExternal | huge-4096.png | rel | 无绘制、无几何 |
| Toolkit+DenyExternal | 中文名-方块.png | file | 无绘制、无几何 |
| Toolkit+DenyExternal | 中文名-方块.png | rel | 无绘制、无几何 |
| Toolkit+DenyExternal | with space.png | file | 无绘制、无几何 |
| Toolkit+DenyExternal | with space.png | rel | 无绘制、无几何 |
| Toolkit+DenyExternal | 图标-方块.png | file | 无绘制、无几何 |
| Toolkit+DenyExternal | 图标-方块.png | rel | 无绘制、无几何 |
| Toolkit+DenyExternal | rect-120x80.svg | file | 无绘制、无几何 |
| Toolkit+DenyExternal | rect-120x80.svg | rel | 无绘制、无几何 |
| Toolkit+DenyExternal | ratio-only.svg | file | 无绘制、无几何 |
| Toolkit+DenyExternal | ratio-only.svg | rel | 无绘制、无几何 |
| Toolkit+DenyExternal | icon-24.svg | file | 无绘制、无几何 |
| Toolkit+DenyExternal | icon-24.svg | rel | 无绘制、无几何 |
| Toolkit+AllowHostResolved | quad.tiff | data | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowHostResolved | quad.tiff | file | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowHostResolved | quad.tiff | rel | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowHostResolved | quad.avif | data | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowHostResolved | quad.avif | file | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowHostResolved | quad.avif | rel | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowAll | quad.tiff | data | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowAll | quad.tiff | file | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowAll | quad.tiff | rel | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowAll | quad.avif | data | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowAll | quad.avif | file | 预期不支持（L0，写入基线，不投入） |
| Toolkit+AllowAll | quad.avif | rel | 预期不支持（L0，写入基线，不投入） |

## 策略一致性（决策 1 · TC-M-905）

- 全图差异像素比例：**5.9524%**（其中 12 个动画样本格未计入下方判定）
- 排除动画样本格后：**0.0000%** —— ✅ 非动画区域逐像素一致

> **归因**：全图差异 **100% 落在动画样本格内**（`anim-noloop.gif`、`anim-2frames.webp` 等）——
> 两个配置各是一次独立加载与截图，动画停在不同帧（**帧相位差**，与策略开关无关）；
> 非动画区域（静态图 / SVG / 视频 / 音频格）**零差异**。详见 `media-format-verification-plan.md` §9.7。

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
| Browser | sine-440-1s.wav | file | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Browser | sine-440-1s.wav | rel | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Browser | sine-440-1s.mp3 | file | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Browser | sine-440-1s.mp3 | rel | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Browser | sine-440-1s.ogg | file | 47861 | 439.87 | 440 | 1.000 | ✅ |
| Browser | sine-440-1s.ogg | rel | 47861 | 439.87 | 440 | 1.000 | ✅ |
| Browser | sine-440-1s.m4a | file | 49041 | 439.88 | 440 | 1.000 | ✅ |
| Browser | sine-440-1s.m4a | rel | 49041 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.wav | file | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.wav | rel | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.mp3 | file | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.mp3 | rel | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.ogg | file | 47861 | 439.87 | 440 | 1.000 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.ogg | rel | 47861 | 439.87 | 440 | 1.000 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.m4a | file | 49041 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.m4a | rel | 49041 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.wav | file | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.wav | rel | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.mp3 | file | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.mp3 | rel | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.ogg | file | 47861 | 439.87 | 440 | 1.000 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.ogg | rel | 47861 | 439.87 | 440 | 1.000 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.m4a | file | 49041 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.m4a | rel | 49041 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowAll | sine-440-1s.wav | file | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowAll | sine-440-1s.wav | rel | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowAll | sine-440-1s.mp3 | file | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowAll | sine-440-1s.mp3 | rel | 48000 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowAll | sine-440-1s.ogg | file | 47861 | 439.87 | 440 | 1.000 | ✅ |
| Toolkit+AllowAll | sine-440-1s.ogg | rel | 47861 | 439.87 | 440 | 1.000 | ✅ |
| Toolkit+AllowAll | sine-440-1s.m4a | file | 49041 | 439.88 | 440 | 1.000 | ✅ |
| Toolkit+AllowAll | sine-440-1s.m4a | rel | 49041 | 439.88 | 440 | 1.000 | ✅ |

### 判据 B：环回录音（可选，外部视角）

命令：`ffmpeg -hide_banner -v error -f dshow -i "audio=Voicemeeter Out B3 (VB-Audio Voicemeeter VAIO)" -t 1.0 -f s16le -ar 48000 -ac 2 -`

- 设备：Voicemeeter Out B3 (VB-Audio Voicemeeter VAIO)
- 结果：**mismatch**
- 录回音频主峰：4634.8 Hz（幅度 0.032）
- 说明：主峰 4634.8Hz 偏离期望 440Hz（该设备的输入未路由到系统输出）

### TC-M-602 定点：播放时钟（音频为主时钟）

判据：①终态 `currentTime` 到达 duration；②采样点 `currentTime` **不超前**于「该会话首块 PCM 交付以来经过的时间」（设备位置只可能 ≤ 已交付且已播出的时间，超前即说明时钟不是输出位置驱动的），滞后容许 0.45s（设备启动延迟与缓冲深度）。

引擎侧的**严格**断言另见 `engine/js/bindings/mediaaudio_test.go`（`TestAudioSessionDrivesCurrentTime`：currentTime 逐值精确等于会话 Position）。

| 配置 | 样本 | 来源 | 采样点（首块后 s） | currentTime | 期望 | 结束 currentTime | 判定 |
|---|---|---|---|---|---|---|---|
| Browser | sine-440-1s.wav | data | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Browser | sine-440-1s.wav | file | 0.74 | 0.70 | 0.74 | 1.00 | ✅ |
| Browser | sine-440-1s.wav | rel | 0.65 | 0.60 | 0.65 | 1.00 | ✅ |
| Browser | sine-440-1s.mp3 | data | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Browser | sine-440-1s.mp3 | file | 0.57 | 0.52 | 0.57 | 1.00 | ✅ |
| Browser | sine-440-1s.mp3 | rel | 0.49 | 0.44 | 0.49 | 1.00 | ✅ |
| Browser | sine-440-1s.ogg | data | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Browser | sine-440-1s.ogg | file | 0.41 | 0.36 | 0.41 | 1.00 | ✅ |
| Browser | sine-440-1s.ogg | rel | 0.33 | 0.26 | 0.33 | 1.00 | ✅ |
| Browser | sine-440-1s.m4a | data | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Browser | sine-440-1s.m4a | file | 0.25 | 0.20 | 0.25 | 1.00 | ✅ |
| Browser | sine-440-1s.m4a | rel | 0.16 | 0.00 | 0.16 | 1.00 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.wav | data | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Toolkit+DenyExternal | sine-440-1s.wav | file | 0.71 | 0.66 | 0.71 | 1.00 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.wav | rel | 0.62 | 0.58 | 0.62 | 1.00 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.mp3 | data | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Toolkit+DenyExternal | sine-440-1s.mp3 | file | 0.55 | 0.51 | 0.55 | 1.00 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.mp3 | rel | 0.47 | 0.43 | 0.47 | 1.00 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.ogg | data | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Toolkit+DenyExternal | sine-440-1s.ogg | file | 0.39 | 0.31 | 0.39 | 1.00 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.ogg | rel | 0.31 | 0.26 | 0.31 | 1.00 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.m4a | data | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Toolkit+DenyExternal | sine-440-1s.m4a | file | 0.23 | 0.00 | 0.23 | 1.00 | ✅ |
| Toolkit+DenyExternal | sine-440-1s.m4a | rel | 0.15 | 0.00 | 0.15 | 1.00 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.wav | data | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Toolkit+AllowHostResolved | sine-440-1s.wav | file | 0.73 | 0.68 | 0.73 | 1.00 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.wav | rel | 0.65 | 0.60 | 0.65 | 1.00 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.mp3 | data | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Toolkit+AllowHostResolved | sine-440-1s.mp3 | file | 0.57 | 0.53 | 0.57 | 1.00 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.mp3 | rel | 0.49 | 0.45 | 0.49 | 1.00 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.ogg | data | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Toolkit+AllowHostResolved | sine-440-1s.ogg | file | 0.42 | 0.37 | 0.42 | 1.00 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.ogg | rel | 0.34 | 0.26 | 0.34 | 1.00 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.m4a | data | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Toolkit+AllowHostResolved | sine-440-1s.m4a | file | 0.24 | 0.20 | 0.24 | 1.00 | ✅ |
| Toolkit+AllowHostResolved | sine-440-1s.m4a | rel | 0.15 | 0.00 | 0.15 | 1.00 | ✅ |
| Toolkit+AllowAll | sine-440-1s.wav | data | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Toolkit+AllowAll | sine-440-1s.wav | file | 0.74 | 0.70 | 0.74 | 1.00 | ✅ |
| Toolkit+AllowAll | sine-440-1s.wav | rel | 0.65 | 0.61 | 0.65 | 1.00 | ✅ |
| Toolkit+AllowAll | sine-440-1s.mp3 | data | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Toolkit+AllowAll | sine-440-1s.mp3 | file | 0.57 | 0.53 | 0.57 | 1.00 | ✅ |
| Toolkit+AllowAll | sine-440-1s.mp3 | rel | 0.49 | 0.44 | 0.49 | 1.00 | ✅ |
| Toolkit+AllowAll | sine-440-1s.ogg | data | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Toolkit+AllowAll | sine-440-1s.ogg | file | 0.41 | 0.37 | 0.41 | 1.00 | ✅ |
| Toolkit+AllowAll | sine-440-1s.ogg | rel | 0.32 | 0.26 | 0.32 | 1.00 | ✅ |
| Toolkit+AllowAll | sine-440-1s.m4a | data | 0.00 | 0.00 | 0.00 | 0.00 | — |
| Toolkit+AllowAll | sine-440-1s.m4a | file | 0.24 | 0.20 | 0.24 | 1.00 | ✅ |
| Toolkit+AllowAll | sine-440-1s.m4a | rel | 0.16 | 0.00 | 0.16 | 1.00 | ✅ |

> 容差 **±0.2s**（采样开销 + 设备启动延迟）。`data:` 来源没有本地路径（宿主解不了码），采样时无 PCM、时钟未起步——不在判据 A 的覆盖范围，保持 L1。

## 与基线的差异

无等级下降（与基线一致）。

## 原始证据索引

- Browser：截图 `dev/media/out/matrix-Browser.png`、播放态 `dev/media/out/matrix-Browser-playing.png`、几何/事件 `dev/media/out/geom-Browser.json`
- Toolkit+DenyExternal：截图 `dev/media/out/matrix-Toolkit-DenyExternal.png`、播放态 `dev/media/out/matrix-Toolkit-DenyExternal-playing.png`、几何/事件 `dev/media/out/geom-Toolkit-DenyExternal.json`
- Toolkit+AllowHostResolved：截图 `dev/media/out/matrix-Toolkit-AllowHostResolved.png`、播放态 `dev/media/out/matrix-Toolkit-AllowHostResolved-playing.png`、几何/事件 `dev/media/out/geom-Toolkit-AllowHostResolved.json`
- Toolkit+AllowAll：截图 `dev/media/out/matrix-Toolkit-AllowAll.png`、播放态 `dev/media/out/matrix-Toolkit-AllowAll-playing.png`、几何/事件 `dev/media/out/geom-Toolkit-AllowAll.json`
- 矩阵页：`dev/media/samples/_matrix.html`（与样本同级，文档相对路径来源依赖此位置）
- 样本清单：`dev\media\samples\manifest.json`

> 截图为预乘 RGBA 反预乘后的 sRGB；判定基于像素采样（非人眼），截图供 `read_image` 复核。
