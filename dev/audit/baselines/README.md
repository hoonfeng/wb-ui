# 独立 Edge 基准（§0.3 八页像素回归门槛的对照物）

## 为什么需要这一份
§0.3 的八页门槛此前取自**当轮实测**（deffont/vue/react/fontshort/stack/sticky/zorder
7 项恰好等于门槛值）—— 属**自证基线**：只能证明「相对设阈时无恶化」，无法发现
渐进退化。本目录把独立抓取的 Edge 基准 PNG 冻结入库，使门槛可改为
「与冻结基线对比且不劣化」。

## 内容
`edge/<name>.png` —— Edge（`--headless --disable-gpu --no-sandbox --hide-scrollbars`）
对同名夹具的截图，抓取批次：2026-10-06 12:49–12:50（与本轮 §0.3 复跑同批）。

| 夹具 | 视口 | 当轮 wbui 侧 diff |
|---|---|---|
| deffont   | 400x100 | 14.62% |
| vue       | 460x320 | 2.83% |
| react     | 460x320 | 3.04% |
| fontshort | 400x230 | 12.54% |
| stack     | 740x400 | 2.56% |
| sticky    | 460x220 | 5.55% |
| zorder    | 700x420 | 0.84% |
| composite | 860x320 | 2.87% |

## 复跑方式
夹具与截图工具均在仓库内（`dev/fixtures/webshot/` 与 `dev/probes/webshot`），
像素对比用 `dev/tools/pixdiff.py`（两图像素差异统计）。

## 注意
- 基准 PNG 由**当时本机 Edge 版本**生成；Edge 升级后应重新冻结并记录版本。
- 抓取依赖字体环境（本机 60 个系统字体，含 Noto Sans SC）——与 CI 环境不同时
  基准不可直接复用，须在目标环境重新冻结。
