<!-- AI-PS 主界面（main）—— 由 tool-design 从设计工程生成，勿手改；
     真相源：design.project.json + design.tokens.json（改设计后重新生成，生成幂等）。
     依赖：项目入口需引入 tokens.css（设计令牌 → CSS 自定义属性）-->
<script setup>
// 交互：设计工程里 props.action="emit:<名字>" 的节点 → 屏幕作为根组件把事件交给父级监听
const emit = defineEmits(['fileExport', 'aiGenerate', 'aiVariant'])
</script>

<template>
  <main class="screen-main" data-screen="main">
    <div class="d-main_app">
      <div class="d-main_titlebar">
        <div class="d-tb-brand">
          <span class="d-n2" role="img" aria-label="star"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 4.5l2.4 5 5.1.7-3.7 3.6.9 5.1-4.7-2.5-4.7 2.5.9-5.1L4.5 10.2l5.1-.7z"/></svg></span>
          <p class="d-n3">
            AI-PS
          </p>
          <span class="d-n4">
            本地桩
          </span>
        </div>
        <div class="d-tb-menus">
          <p class="d-n6">
            文件
          </p>
          <p class="d-n7">
            编辑
          </p>
          <p class="d-n8">
            图像
          </p>
          <p class="d-n9">
            图层
          </p>
          <p class="d-n10">
            选择
          </p>
          <p class="d-n11">
            滤镜
          </p>
          <p class="d-n12">
            AI
          </p>
          <p class="d-n13">
            视图
          </p>
          <p class="d-n14">
            帮助
          </p>
        </div>
        <div class="d-tb-right">
          <p class="d-n16">
            未连接服务 · 本地桩
          </p>
          <button class="d-n17" @click="emit('fileExport')">
            导出
          </button>
          <span class="d-n18" role="img" aria-label="user"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 12a4 4 0 100-8 4 4 0 000 8M4.5 20c1.5-3.5 4.2-5 7.5-5s6 1.5 7.5 5"/></svg></span>
        </div>
      </div>
      <hr class="d-main_title-divider" />
      <div class="d-main_body">
        <div class="d-main_toolstrip">
          <span class="d-n19" role="img" aria-label="search"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M10.5 17a6.5 6.5 0 100-13 6.5 6.5 0 000 13M15.5 15.5 20.5 20.5"/></svg></span>
          <span class="d-n20" role="img" aria-label="edit"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4.5 19.5h4l10-10-4-4-10 10v4M14.5 5.5l4 4"/></svg></span>
          <span class="d-n21" role="img" aria-label="close"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18"/></svg></span>
          <span class="d-n22" role="img" aria-label="grid"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4.5 4.5h6v6h-6zM13.5 4.5h6v6h-6zM4.5 13.5h6v6h-6zM13.5 13.5h6v6h-6z"/></svg></span>
          <span class="d-n23" role="img" aria-label="file"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 3.5h7l5 5v12H6zM13 3.5v5h5"/></svg></span>
          <span class="d-n24" role="img" aria-label="folder"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 6.5h5.5l2 2.5H20v9.5H4z"/></svg></span>
          <span class="d-n25" role="img" aria-label="download"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 4v11M7.5 11 12 15.5 16.5 11M5 19.5h14"/></svg></span>
          <span class="d-n26" role="img" aria-label="upload"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 15.5V4.5M7.5 9 12 4.5 16.5 9M5 19.5h14"/></svg></span>
          <span class="d-n27" role="img" aria-label="copy"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M8.5 8.5h11V20h-11zM4.5 4.5h11v4M4.5 4.5V16h4"/></svg></span>
          <span class="d-n28" role="img" aria-label="eye"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M2.5 12S6 6.5 12 6.5 21.5 12 21.5 12 18 17.5 12 17.5 2.5 12 2.5 12zM12 14.5a2.5 2.5 0 100-5 2.5 2.5 0 000 5"/></svg></span>
          <span class="d-n29" role="img" aria-label="refresh"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M19 12a7 7 0 11-2.4-5.3M19 4.5V9h-4.5"/></svg></span>
          <span class="d-n30" role="img" aria-label="star"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 4.5l2.4 5 5.1.7-3.7 3.6.9 5.1-4.7-2.5-4.7 2.5.9-5.1L4.5 10.2l5.1-.7z"/></svg></span>
          <hr class="d-n31" />
          <span class="d-n32" role="img" aria-label="settings"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 7.5h16M4 16.5h16M9.5 5v5M15.5 14v5"/></svg></span>
        </div>
        <div class="d-main_center">
          <div class="d-docbar">
            <p class="d-n34">
              未命名-1.psd
            </p>
            <span class="d-n35" role="img" aria-label="close"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18"/></svg></span>
            <p class="d-n36">
              logo.ai
            </p>
            <p class="d-n37">
              banner.png
            </p>
          </div>
          <div class="d-canvas-stage">
            <div class="d-artboard">
              <div class="d-ab-photo"></div>
              <div class="d-ab-meta">
                <p class="d-n42">
                  AI 生成占位（桩）
                </p>
                <p class="d-n43">
                  1920 × 1080 px · RGB/8
                </p>
              </div>
            </div>
          </div>
          <div class="d-statusbar">
            <p class="d-n45">
              缩放 100%
            </p>
            <p class="d-n46">
              1920 × 1080 px
            </p>
            <p class="d-n47">
              RGB/8 · sRGB
            </p>
            <p class="d-n48">
              桩模式 · 不走网络
            </p>
          </div>
        </div>
        <div class="d-main_right">
          <div class="d-ai-panel">
            <div class="d-ai-head">
              <span class="d-n51" role="img" aria-label="star"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 4.5l2.4 5 5.1.7-3.7 3.6.9 5.1-4.7-2.5-4.7 2.5.9-5.1L4.5 10.2l5.1-.7z"/></svg></span>
              <p class="d-n52">
                AI 生成
              </p>
              <span class="d-n53">
                本地桩
              </span>
            </div>
            <p class="d-ai-hint">
              描述画面，本地桩返回占位结果
            </p>
            <input class="d-ai-prompt" type="text" placeholder="例如：赛博朋克城市夜景，霓虹招牌" />
            <div class="d-ai-actions">
              <button class="d-n57" @click="emit('aiGenerate')">
                生成图像
              </button>
              <button class="d-n58" @click="emit('aiVariant')">
                生成变体
              </button>
            </div>
            <p class="d-ai-status">
              未连接模型 · Go 桩记录本次请求
            </p>
            <div class="d-ai-results">
              <div class="d-n61"></div>
              <div class="d-n62"></div>
              <div class="d-n63"></div>
            </div>
          </div>
          <div class="d-props-panel">
            <p class="d-n65">
              属性
            </p>
            <hr class="d-n66" />
            <div class="d-pr-1">
              <p class="d-n68">
                图层
              </p>
              <p class="d-n69">
                AI 生成层
              </p>
            </div>
            <div class="d-pr-2">
              <p class="d-n71">
                不透明度
              </p>
              <p class="d-n72">
                80%
              </p>
            </div>
            <div class="d-pr-3">
              <p class="d-n74">
                混合模式
              </p>
              <p class="d-n75">
                正常
              </p>
            </div>
            <div class="d-pr-4">
              <p class="d-n77">
                尺寸
              </p>
              <p class="d-n78">
                1920 × 1080 px
              </p>
            </div>
          </div>
          <div class="d-layers-panel">
            <p class="d-n80">
              图层
            </p>
            <hr class="d-n81" />
            <div class="d-layers-list">
              <div class="d-n83">
                <div class="d-n84">
                  <span class="d-n85" role="img" aria-label="eye"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M2.5 12S6 6.5 12 6.5 21.5 12 21.5 12 18 17.5 12 17.5 2.5 12 2.5 12zM12 14.5a2.5 2.5 0 100-5 2.5 2.5 0 000 5"/></svg></span>
                  <p class="d-n86">
                    AI 生成层
                  </p>
                </div>
                <p class="d-n87">
                  80%
                </p>
              </div>
              <div class="d-n88">
                <div class="d-n89">
                  <span class="d-n90" role="img" aria-label="eye"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M2.5 12S6 6.5 12 6.5 21.5 12 21.5 12 18 17.5 12 17.5 2.5 12 2.5 12zM12 14.5a2.5 2.5 0 100-5 2.5 2.5 0 000 5"/></svg></span>
                  <p class="d-n91">
                    文字层
                  </p>
                </div>
                <p class="d-n92">
                  100%
                </p>
              </div>
              <div class="d-n93">
                <div class="d-n94">
                  <span class="d-n95" role="img" aria-label="eye"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M2.5 12S6 6.5 12 6.5 21.5 12 21.5 12 18 17.5 12 17.5 2.5 12 2.5 12zM12 14.5a2.5 2.5 0 100-5 2.5 2.5 0 000 5"/></svg></span>
                  <p class="d-n96">
                    背景
                  </p>
                </div>
                <p class="d-n97">
                  100%
                </p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </main>
</template>

<style scoped>
/* 布局：flex 流式；颜色/间距/字号/圆角全部走令牌变量（var(--color-fg) 等） */
*, *::before, *::after { box-sizing: border-box; }

.screen-main {
  display: flex;
  flex-direction: column;
  width: 100%;
  max-width: 1440px;
  min-height: 900px;
  margin: 0 auto;
  background: var(--color-bg);
}
.d-main_app {
  background: var(--color-bg);
  display: flex;
  flex-direction: column;
}
.d-main_titlebar {
  background: var(--color-surface);
  display: flex;
  flex-direction: row;
  gap: var(--space-md);
  padding: var(--space-sm);
  width: 100%;
  height: 44px;
}
.d-tb-brand {
  display: flex;
  flex-direction: row;
  gap: var(--space-sm);
  width: fit-content;
  align-self: flex-start;
}
.d-n2 {
  display: inline-flex;
  color: var(--color-accent-text);
  flex: none;
}
.d-n3 {
  margin: 0;
  font-size: var(--font-size-md);
  font-weight: var(--font-weight-semibold);
  line-height: var(--line-height-normal);
  color: var(--color-fg);
  overflow-wrap: break-word;
}
.d-n4 {
  display: inline-flex;
  align-items: center;
  padding: var(--space-xs) var(--space-sm);
  font-size: var(--font-size-xs);
  font-weight: 500;
  line-height: 1.4;
  white-space: nowrap;
  background: var(--color-surface-2);
  color: var(--color-fg);
  border-radius: var(--radius-full);
}
.d-tb-menus {
  display: flex;
  flex-direction: row;
  gap: var(--space-md);
  width: fit-content;
  align-self: flex-start;
}
.d-n6 {
  margin: 0;
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-n7 {
  margin: 0;
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-n8 {
  margin: 0;
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-n9 {
  margin: 0;
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-n10 {
  margin: 0;
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-n11 {
  margin: 0;
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-n12 {
  margin: 0;
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-semibold);
  line-height: var(--line-height-normal);
  color: var(--color-accent-text);
  overflow-wrap: break-word;
}
.d-n13 {
  margin: 0;
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-n14 {
  margin: 0;
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-tb-right {
  display: flex;
  flex-direction: row;
  gap: var(--space-md);
  width: fit-content;
  align-self: flex-start;
}
.d-n16 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-n17 {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-sm) var(--space-lg);
  font-size: var(--font-size-md);
  font-weight: 600;
  line-height: 1.5;
  background: var(--color-surface);
  color: var(--color-fg);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  cursor: pointer;
}
.d-n18 {
  display: inline-flex;
  color: var(--color-muted);
  flex: none;
}
.d-main_title-divider {
  border: none;
  height: 1px;
  background: var(--color-border);
  margin: 0;
  width: 100%;
}
.d-main_body {
  display: flex;
  flex-direction: row;
  width: 100%;
  height: 855px;
}
.d-main_toolstrip {
  background: var(--color-surface);
  display: flex;
  flex-direction: column;
  gap: var(--space-xs);
  padding: var(--space-xs);
  width: 48px;
  flex: 1 1 auto;
  min-height: 0;
}
.d-n19 {
  display: inline-flex;
  color: var(--color-accent-text);
  flex: none;
  align-self: flex-start;
}
.d-n20 {
  display: inline-flex;
  color: var(--color-muted);
  flex: none;
  align-self: flex-start;
}
.d-n21 {
  display: inline-flex;
  color: var(--color-muted);
  flex: none;
  align-self: flex-start;
}
.d-n22 {
  display: inline-flex;
  color: var(--color-muted);
  flex: none;
  align-self: flex-start;
}
.d-n23 {
  display: inline-flex;
  color: var(--color-muted);
  flex: none;
  align-self: flex-start;
}
.d-n24 {
  display: inline-flex;
  color: var(--color-muted);
  flex: none;
  align-self: flex-start;
}
.d-n25 {
  display: inline-flex;
  color: var(--color-muted);
  flex: none;
  align-self: flex-start;
}
.d-n26 {
  display: inline-flex;
  color: var(--color-muted);
  flex: none;
  align-self: flex-start;
}
.d-n27 {
  display: inline-flex;
  color: var(--color-muted);
  flex: none;
  align-self: flex-start;
}
.d-n28 {
  display: inline-flex;
  color: var(--color-muted);
  flex: none;
  align-self: flex-start;
}
.d-n29 {
  display: inline-flex;
  color: var(--color-muted);
  flex: none;
  align-self: flex-start;
}
.d-n30 {
  display: inline-flex;
  color: var(--color-accent-text);
  flex: none;
  align-self: flex-start;
}
.d-n31 {
  border: none;
  height: 1px;
  background: var(--color-border);
  margin: 0;
  width: 100%;
}
.d-n32 {
  display: inline-flex;
  color: var(--color-muted);
  flex: none;
  align-self: flex-start;
}
.d-main_center {
  display: flex;
  flex-direction: column;
  width: 1072px;
  flex: 1 1 auto;
  min-height: 0;
}
.d-docbar {
  background: var(--color-bg);
  display: flex;
  flex-direction: row;
  gap: var(--space-md);
  padding: var(--space-sm);
  width: 100%;
  height: 36px;
}
.d-n34 {
  margin: 0;
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-fg);
  overflow-wrap: break-word;
}
.d-n35 {
  display: inline-flex;
  color: var(--color-muted);
  flex: none;
}
.d-n36 {
  margin: 0;
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-n37 {
  margin: 0;
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-canvas-stage {
  background: var(--color-canvas);
  display: flex;
  flex-direction: column;
  padding: var(--space-xl);
  width: 100%;
  height: 791px;
}
.d-artboard {
  background: #FFFFFF;
  border-radius: var(--radius-sm);
  box-shadow: var(--shadow-md);
  display: flex;
  flex-direction: column;
  width: 720px;
  height: 480px;
}
.d-ab-photo {
  background: var(--color-canvas-checker);
  display: flex;
  flex-direction: column;
  gap: var(--space-sm);
  width: 100%;
  height: 400px;
}
.d-ab-meta {
  display: flex;
  flex-direction: row;
  gap: var(--space-md);
  padding: var(--space-sm);
  width: 100%;
}
.d-n42 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted-on-light);
  overflow-wrap: break-word;
}
.d-n43 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted-on-light);
  overflow-wrap: break-word;
}
.d-statusbar {
  background: var(--color-surface);
  display: flex;
  flex-direction: row;
  gap: var(--space-lg);
  padding: var(--space-sm);
  width: 100%;
  height: 28px;
}
.d-n45 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-n46 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-n47 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-n48 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-accent-text);
  overflow-wrap: break-word;
}
.d-main_right {
  background: var(--color-surface);
  display: flex;
  flex-direction: column;
  gap: var(--space-sm);
  padding: var(--space-sm);
  width: 320px;
  flex: 1 1 auto;
  min-height: 0;
}
.d-ai-panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-sm);
  padding: var(--space-md);
  width: 100%;
}
.d-ai-head {
  display: flex;
  flex-direction: row;
  gap: var(--space-sm);
  width: fit-content;
  align-self: flex-start;
}
.d-n51 {
  display: inline-flex;
  color: var(--color-accent-text);
  flex: none;
}
.d-n52 {
  margin: 0;
  font-size: var(--font-size-md);
  font-weight: var(--font-weight-semibold);
  line-height: var(--line-height-normal);
  color: var(--color-fg);
  overflow-wrap: break-word;
}
.d-n53 {
  display: inline-flex;
  align-items: center;
  padding: var(--space-xs) var(--space-sm);
  font-size: var(--font-size-xs);
  font-weight: 500;
  line-height: 1.4;
  white-space: nowrap;
  background: var(--color-surface-2);
  color: var(--color-fg);
  border-radius: var(--radius-full);
}
.d-ai-hint {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-ai-prompt {
  height: 36px;
  padding: 0 10px;
  font-size: var(--font-size-md);
  color: var(--color-fg);
  background: var(--color-bg);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  width: 100%;
}
.d-ai-actions {
  display: flex;
  flex-direction: row;
  gap: var(--space-sm);
  width: fit-content;
  align-self: flex-start;
}
.d-n57 {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-sm) var(--space-lg);
  font-size: var(--font-size-md);
  font-weight: 600;
  line-height: 1.5;
  background: var(--color-accent);
  color: var(--color-accent-fg);
  border: 1px solid var(--color-accent);
  border-radius: var(--radius-md);
  cursor: pointer;
}
.d-n58 {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-sm) var(--space-lg);
  font-size: var(--font-size-md);
  font-weight: 600;
  line-height: 1.5;
  background: var(--color-surface);
  color: var(--color-fg);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  cursor: pointer;
}
.d-ai-status {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-ai-results {
  display: flex;
  flex-direction: row;
  gap: var(--space-sm);
  width: fit-content;
  align-self: flex-start;
}
.d-n61 {
  background: var(--color-surface-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  display: flex;
  flex-direction: column;
  gap: var(--space-sm);
  width: 80px;
  height: 80px;
}
.d-n62 {
  background: var(--color-surface-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  display: flex;
  flex-direction: column;
  gap: var(--space-sm);
  width: 80px;
  height: 80px;
}
.d-n63 {
  background: var(--color-surface-2);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  display: flex;
  flex-direction: column;
  gap: var(--space-sm);
  width: 80px;
  height: 80px;
}
.d-props-panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-sm);
  padding: var(--space-md);
  width: 100%;
}
.d-n65 {
  margin: 0;
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-semibold);
  line-height: var(--line-height-normal);
  color: var(--color-fg);
  overflow-wrap: break-word;
}
.d-n66 {
  border: none;
  height: 1px;
  background: var(--color-border);
  margin: 0;
  width: 100%;
}
.d-pr-1 {
  display: flex;
  flex-direction: row;
  gap: var(--space-sm);
  width: fit-content;
  align-self: flex-start;
}
.d-n68 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-n69 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-fg);
  overflow-wrap: break-word;
}
.d-pr-2 {
  display: flex;
  flex-direction: row;
  gap: var(--space-sm);
  width: fit-content;
  align-self: flex-start;
}
.d-n71 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-n72 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-fg);
  overflow-wrap: break-word;
}
.d-pr-3 {
  display: flex;
  flex-direction: row;
  gap: var(--space-sm);
  width: fit-content;
  align-self: flex-start;
}
.d-n74 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-n75 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-fg);
  overflow-wrap: break-word;
}
.d-pr-4 {
  display: flex;
  flex-direction: row;
  gap: var(--space-sm);
  width: fit-content;
  align-self: flex-start;
}
.d-n77 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-n78 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-fg);
  overflow-wrap: break-word;
}
.d-layers-panel {
  display: flex;
  flex-direction: column;
  gap: var(--space-sm);
  padding: var(--space-md);
  width: 100%;
}
.d-n80 {
  margin: 0;
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-semibold);
  line-height: var(--line-height-normal);
  color: var(--color-fg);
  overflow-wrap: break-word;
}
.d-n81 {
  border: none;
  height: 1px;
  background: var(--color-border);
  margin: 0;
  width: 100%;
}
.d-layers-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-xs);
  padding: var(--space-sm);
  width: 100%;
}
.d-n83 {
  display: flex;
  flex-direction: row;
  gap: var(--space-sm);
  width: fit-content;
  align-self: flex-start;
}
.d-n84 {
  display: flex;
  flex-direction: row;
  gap: var(--space-sm);
  width: fit-content;
  align-self: flex-start;
}
.d-n85 {
  display: inline-flex;
  color: var(--color-muted);
  flex: none;
}
.d-n86 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-fg);
  overflow-wrap: break-word;
}
.d-n87 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-n88 {
  display: flex;
  flex-direction: row;
  gap: var(--space-sm);
  width: fit-content;
  align-self: flex-start;
}
.d-n89 {
  display: flex;
  flex-direction: row;
  gap: var(--space-sm);
  width: fit-content;
  align-self: flex-start;
}
.d-n90 {
  display: inline-flex;
  color: var(--color-muted);
  flex: none;
}
.d-n91 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-fg);
  overflow-wrap: break-word;
}
.d-n92 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
.d-n93 {
  display: flex;
  flex-direction: row;
  gap: var(--space-sm);
  width: fit-content;
  align-self: flex-start;
}
.d-n94 {
  display: flex;
  flex-direction: row;
  gap: var(--space-sm);
  width: fit-content;
  align-self: flex-start;
}
.d-n95 {
  display: inline-flex;
  color: var(--color-muted);
  flex: none;
}
.d-n96 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-fg);
  overflow-wrap: break-word;
}
.d-n97 {
  margin: 0;
  font-size: var(--font-size-xs);
  font-weight: var(--font-weight-normal);
  line-height: var(--line-height-normal);
  color: var(--color-muted);
  overflow-wrap: break-word;
}
</style>
