<!-- AI-PS 根组件：包住设计生成的 Main.vue，把「界面事件」接到「请求桩」。
     设计产物（vue/Main.vue）只声明交互意图（props.action="emit:<名字>"），
     由本组件监听并接上宿主链路：事件上报 → POST /api/events → 请求桩接口。 -->
<script setup>
import Main from '../vue/Main.vue'
import { postEvent, requestJSON } from './bridge.js'

function q(sel) {
  return document.querySelector(sel)
}

function setStatus(text) {
  var el = q('.d-ai-status')
  if (el) el.textContent = text
}

// 用桩返回的配色刷新三个结果缩略图（让「请求 → 桩 → 界面」闭环肉眼可见）
function paintResults(colors) {
  var wrap = q('.d-ai-results')
  if (!wrap) return
  var kids = wrap.children
  var palette = colors || []
  for (var i = 0; i < kids.length; i++) {
    kids[i].style.background = palette[i] ? palette[i] : '#3C3C3C'
  }
}

function promptText() {
  var el = q('.d-ai-prompt')
  return el ? String(el.value || '') : ''
}

async function onAiGenerate() {
  var prompt = promptText()
  await postEvent('ai.generate', { prompt: prompt })
  setStatus('生成中…（本地桩）')
  var data = await requestJSON('/api/ai/generate', { prompt: prompt })
  setStatus('[桩] ' + (data.message || '已受理') + ' · job ' + (data.jobId || '-') + ' · ' + (data.provider || 'local-stub'))
  paintResults(data.previewColors)
}

async function onAiVariant() {
  var prompt = promptText()
  await postEvent('ai.variant', { prompt: prompt })
  var data = await requestJSON('/api/ai/variant', { prompt: prompt })
  setStatus('[桩] ' + (data.message || '已生成变体') + ' · 变体 ' + (data.variants ? data.variants.length : 0) + ' 个')
  paintResults(data.previewColors)
}

async function onFileExport() {
  await postEvent('file.export', { format: 'png' })
  var data = await requestJSON('/api/export', { format: 'png', width: 1920, height: 1080 })
  setStatus('[桩] ' + (data.message || '导出已受理') + ' → ' + (data.path || '-'))
}
</script>

<template>
  <Main @ai-generate="onAiGenerate" @ai-variant="onAiVariant" @file-export="onFileExport" />
</template>
