// esbuild 构建脚本（用 JS API 而非 CLI：Windows 下 shell 会吃掉 --define 的引号，
// 导致 process.env.NODE_ENV 被替换成非法字面量，React 会走 development 分支）。
import { build } from 'esbuild';

const common = {
  bundle: true,
  minify: true,
  format: 'iife',
  target: 'es2019',
  loader: { '.jsx': 'jsx' },
  define: { 'process.env.NODE_ENV': '"production"' },
  legalComments: 'none',
};

// 1) IDE 形态压测页
await build({ ...common, entryPoints: ['src/main.jsx'], outfile: 'dist/bundle.js' });
// 2) 最小 React 复现（隔离「引擎能否跑 React 19」本身）
await build({ ...common, entryPoints: ['src/minimal.jsx'], outfile: 'dist/minimal.js' });
// 3) 最小 React 复现·未压缩版（诊断用：让引擎抛出的堆栈显示真实函数名与源码位置）
await build({
  ...common,
  minify: false,
  sourcemap: 'inline',
  entryPoints: ['src/minimal.jsx'],
  outfile: 'dist/minimal.dev.js',
});

console.log('build ok -> dist/bundle.js, dist/minimal.js, dist/minimal.dev.js');
