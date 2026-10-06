# AI-PS · AI 驱动图像编辑器 UI —— 前端代码（由 tool-design 生成）

> 本目录由 `design_codegen` 生成，**请勿手改**：真相源是同目录上一级的
> 设计工程（`design.tokens.json` 等）。改设计后用同样命令重新生成，生成幂等。

## 目录

```
web/ai-ps/
├── tokens.css      # 设计令牌 → CSS 自定义属性（所有 target 共用，务必全局引入）
├── README.md
├── vue/            # Vue 3 单文件组件（<Screen>.vue，style scoped）
```

## 屏幕

- `main`（AI-PS 主界面）· 1440×900 · 组件名 `Main`

## 集成

- **Vue**：把 `vue/*.vue` 放进 `src/components/`，在入口引入 `tokens.css`：
  ```js
  import './design-export/tokens.css'
  ```

## 约定

- class 语义化：`.d-<设计节点 id>`（节点 id 在设计工程里保证唯一）+ 屏幕容器 `.screen-<屏幕 id>`。
- 颜色 / 间距 / 字号 / 字重 / 行高 / 圆角 / 阴影 **一律引用令牌变量**（`var(--color-accent)`），
  改 `design.tokens.json` 后重新生成即全站同步；代码里不出现散落色值。
- 布局为 flex 流式（与设计器的布局引擎同一套规则），尺寸默认自适应，不写死画布像素。
