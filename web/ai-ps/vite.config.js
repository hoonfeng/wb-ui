import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// AI-PS 前端构建配置
// 产物必须能被 wb-ui 的 goja 引擎直接执行（无 HTTP、无模块加载器）：
//   - base './'        → 产物内引用全部相对路径，直接 file:// / 宿主 ResourceResolver 都能取
//   - target es2018    → 不用 goja 可能不支持的更新语法
//   - 单一 JS + 单一 CSS（vite 默认按入口合并）→ 宿主只需拦截少量资源
export default defineConfig({
  base: './',
  plugins: [vue()],
  build: {
    outDir: 'dist',
    assetsDir: 'assets',
    emptyOutDir: true,
    target: 'es2018',
    minify: 'esbuild',
    sourcemap: false,
    cssCodeSplit: false,
    modulePreload: false,
    reportCompressedSize: false,
  },
})
