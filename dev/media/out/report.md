# 媒体格式真实可用性报告（2026-10-07 16:30 0d44787）

## 环境

配置：Browser / Toolkit+DenyExternal / Toolkit+AllowHostResolved / Toolkit+AllowAll ｜ 引擎：0d44787 ｜ 样本：本地脚本生成、不入库（决策 5）｜ 模型：L0–L4（文档 §2）

复现：`python dev/media/gen_samples.py` → `cmd/psai -media`（本机按需，不入 CI 门禁——决策 6）

## 总表

| 配置 | 格式 | 样本 | 来源 | 加载 | 几何 | 绘制 | 契约 | 动画 | 等级 | 备注 |
|---|---|---|---|---|---|---|---|---|---|---|
| Browser | webp | anim-2frames.webp | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | webp | anim-2frames.webp | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | webp | anim-2frames.webp | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | webp | anim-2frames.webp | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | webp | anim-2frames.webp | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | webp | anim-2frames.webp | file | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** |  |
| Toolkit+AllowHostResolved | webp | anim-2frames.webp | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | webp | anim-2frames.webp | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | webp | anim-2frames.webp | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | webp | anim-2frames.webp | rel | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** |  |
| Toolkit+AllowHostResolved | webp | anim-2frames.webp | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | webp | anim-2frames.webp | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-3frames-rgb.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-3frames-rgb.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | gif | anim-3frames-rgb.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-3frames-rgb.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-3frames-rgb.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-3frames-rgb.gif | file | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** |  |
| Toolkit+AllowHostResolved | gif | anim-3frames-rgb.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-3frames-rgb.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-3frames-rgb.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-3frames-rgb.gif | rel | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** |  |
| Toolkit+AllowHostResolved | gif | anim-3frames-rgb.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-3frames-rgb.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-noloop.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-noloop.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | gif | anim-noloop.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-noloop.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-noloop.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-noloop.gif | file | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** |  |
| Toolkit+AllowHostResolved | gif | anim-noloop.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-noloop.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-noloop.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-noloop.gif | rel | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** |  |
| Toolkit+AllowHostResolved | gif | anim-noloop.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-noloop.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-uneven-delay.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-uneven-delay.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | gif | anim-uneven-delay.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-uneven-delay.gif | data | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-uneven-delay.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-uneven-delay.gif | file | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** |  |
| Toolkit+AllowHostResolved | gif | anim-uneven-delay.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | gif | anim-uneven-delay.gif | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | gif | anim-uneven-delay.gif | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | gif | anim-uneven-delay.gif | rel | ❌ | ❌ | ❌ | ❌ | ❌ | **L0** |  |
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
| Browser | m4a | sine-440-1s.m4a | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+DenyExternal | m4a | sine-440-1s.m4a | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowHostResolved | m4a | sine-440-1s.m4a | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowAll | m4a | sine-440-1s.m4a | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Browser | m4a | sine-440-1s.m4a | file | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+DenyExternal | m4a | sine-440-1s.m4a | file | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowHostResolved | m4a | sine-440-1s.m4a | file | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowAll | m4a | sine-440-1s.m4a | file | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Browser | m4a | sine-440-1s.m4a | rel | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+DenyExternal | m4a | sine-440-1s.m4a | rel | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowHostResolved | m4a | sine-440-1s.m4a | rel | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowAll | m4a | sine-440-1s.m4a | rel | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Browser | mp3 | sine-440-1s.mp3 | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+DenyExternal | mp3 | sine-440-1s.mp3 | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowHostResolved | mp3 | sine-440-1s.mp3 | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowAll | mp3 | sine-440-1s.mp3 | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Browser | mp3 | sine-440-1s.mp3 | file | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+DenyExternal | mp3 | sine-440-1s.mp3 | file | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowHostResolved | mp3 | sine-440-1s.mp3 | file | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowAll | mp3 | sine-440-1s.mp3 | file | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Browser | mp3 | sine-440-1s.mp3 | rel | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+DenyExternal | mp3 | sine-440-1s.mp3 | rel | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowHostResolved | mp3 | sine-440-1s.mp3 | rel | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowAll | mp3 | sine-440-1s.mp3 | rel | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Browser | ogg | sine-440-1s.ogg | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+DenyExternal | ogg | sine-440-1s.ogg | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowHostResolved | ogg | sine-440-1s.ogg | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowAll | ogg | sine-440-1s.ogg | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Browser | ogg | sine-440-1s.ogg | file | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+DenyExternal | ogg | sine-440-1s.ogg | file | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowHostResolved | ogg | sine-440-1s.ogg | file | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowAll | ogg | sine-440-1s.ogg | file | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Browser | ogg | sine-440-1s.ogg | rel | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+DenyExternal | ogg | sine-440-1s.ogg | rel | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowHostResolved | ogg | sine-440-1s.ogg | rel | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowAll | ogg | sine-440-1s.ogg | rel | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Browser | wav | sine-440-1s.wav | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+DenyExternal | wav | sine-440-1s.wav | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowHostResolved | wav | sine-440-1s.wav | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowAll | wav | sine-440-1s.wav | data | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Browser | wav | sine-440-1s.wav | file | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+DenyExternal | wav | sine-440-1s.wav | file | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowHostResolved | wav | sine-440-1s.wav | file | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowAll | wav | sine-440-1s.wav | file | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Browser | wav | sine-440-1s.wav | rel | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+DenyExternal | wav | sine-440-1s.wav | rel | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowHostResolved | wav | sine-440-1s.wav | rel | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Toolkit+AllowAll | wav | sine-440-1s.wav | rel | ✅ | ✅ | ❌ | ✅ | — | **L1** | 音频可加载（元数据可用）；音频输出（L4-S）需音频后端，当前无 |
| Browser | mp4 | solid-red-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期） |
| Toolkit+DenyExternal | mp4 | solid-red-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期） |
| Toolkit+AllowHostResolved | mp4 | solid-red-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期） |
| Toolkit+AllowAll | mp4 | solid-red-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期） |
| Browser | mp4 | solid-red-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足） |
| Toolkit+DenyExternal | mp4 | solid-red-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足） |
| Toolkit+AllowHostResolved | mp4 | solid-red-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足） |
| Toolkit+AllowAll | mp4 | solid-red-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足） |
| Browser | mp4 | solid-red-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足） |
| Toolkit+DenyExternal | mp4 | solid-red-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足） |
| Toolkit+AllowHostResolved | mp4 | solid-red-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足） |
| Toolkit+AllowAll | mp4 | solid-red-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ❌ | **L3** | 单色视频：帧色恒定，动画判据不适用（画面正确即足） |
| Browser | mp4 | testsrc-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期） |
| Toolkit+DenyExternal | mp4 | testsrc-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期） |
| Toolkit+AllowHostResolved | mp4 | testsrc-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期） |
| Toolkit+AllowAll | mp4 | testsrc-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期） |
| Browser | mp4 | testsrc-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | mp4 | testsrc-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | mp4 | testsrc-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | mp4 | testsrc-1s.mp4 | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | mp4 | testsrc-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | mp4 | testsrc-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | mp4 | testsrc-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | mp4 | testsrc-1s.mp4 | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | webm | testsrc-1s.webm | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期） |
| Toolkit+DenyExternal | webm | testsrc-1s.webm | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期） |
| Toolkit+AllowHostResolved | webm | testsrc-1s.webm | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期） |
| Toolkit+AllowAll | webm | testsrc-1s.webm | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期） |
| Browser | webm | testsrc-1s.webm | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | webm | testsrc-1s.webm | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | webm | testsrc-1s.webm | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | webm | testsrc-1s.webm | file | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | webm | testsrc-1s.webm | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+DenyExternal | webm | testsrc-1s.webm | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowHostResolved | webm | testsrc-1s.webm | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Toolkit+AllowAll | webm | testsrc-1s.webm | rel | ✅ | ✅ | ✅ | ✅ | ✅ | **L4** |  |
| Browser | mp4 | twophase-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期） |
| Toolkit+DenyExternal | mp4 | twophase-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期） |
| Toolkit+AllowHostResolved | mp4 | twophase-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期） |
| Toolkit+AllowAll | mp4 | twophase-1s.mp4 | data | ✅ | ✅ | ❌ | ✅ | ❌ | **L1** | data: 媒体无本地路径，宿主帧源无法抽帧 → 无画面（预期） |
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

Browser vs Toolkit+AllowAll 首屏截图差异像素比例：3.9683% —— ❌ 存在差异（策略开关引入了渲染差异）

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
