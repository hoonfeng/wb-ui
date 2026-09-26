package webkit

import (
	"strconv"
	"strings"
	"testing"
)

// sidebarScrollProbeHTML 复刻 gou-ide 侧栏「插件面板」的容器链：
// .sidebar(flex column, h100%) > .sidebar-header(32px) + .sidebar-content(flex:1, overflow:auto)
//   > .plugin-panel(height:100%, flex column, overflow:hidden)
//     > .pp-header(flex-shrink:0) + .pp-list(flex:1, overflow:auto)
//
// 用户可见现象：插件面板出现**两层竖直滚动条**
// （.sidebar-content 一个 + .pp-list 一个），Chromium 只有一层。
const sidebarScrollProbeHTML = `<!DOCTYPE html><html><head><style>
html,body{margin:0;padding:0;height:100%}
.sidebar{height:100%;display:flex;flex-direction:column;overflow:hidden;width:263px;background:#111}
.sidebar-header{height:32px;flex-shrink:0;background:#222;color:#fff}
.sidebar-content{flex:1;overflow:auto}
.plugin-panel{height:100%;display:flex;flex-direction:column;overflow:hidden;font-size:13px}
.pp-header{flex-shrink:0;padding:8px 10px;background:#333;color:#fff}
.pp-list{flex:1;overflow:auto;padding:4px 0}
.item{height:39px;border-bottom:1px solid #000}
</style></head><body>
<div class="sidebar">
  <div class="sidebar-header">s</div>
  <div class="sidebar-content"><div class="plugin-panel">
    <div class="pp-header">p</div>
    <div class="pp-list" id="list"><div id="filler" style="height:1300px"></div></div>
  </div></div>
</div></body></html>`

const sidebarProbeJS = `(function(){
  var c=document.querySelector('.sidebar-content'),
      p=document.querySelector('.plugin-panel'),
      l=document.querySelector('.pp-list');
  return c.clientHeight+'|'+c.scrollHeight+'|'+Math.round(p.getBoundingClientRect().height)
    +'|'+l.clientHeight+'|'+l.scrollHeight;
})()`

// TestSidebarPanelSingleScrollLayer 钉死：column flex item 被分配确定主尺寸后，
// 子元素的 `height:100%` 必须解析（Chromium 行为），插件面板只应有**一层**滚动。
//
// 根因（2026-09-26，gou-ide 用户报「插件面板两个滚动条」）：
// `heightIsAutoForBox` 只看 CSS `height` 属性，忽略外层布局算法**分配**的确定高度。
// `.sidebar-content{flex:1;overflow:auto}` 的 CSS height 是 auto → 被判为「高度
// auto」→ 子元素 `.plugin-panel{height:100%}` 也按 auto 处理（CSS 2.1 §10.5 的
// 误用），高度由内容撑成 1363（父内容区只有 768），`.pp-list{flex:1}` 同步失控
// （1332 而非 745）→ `.sidebar-content` 与 `.pp-list` 各自出滚动条。
//
// Chromium 行为（同一 HTML，1400x800 视口）：sidebar 800 / header 32 /
// content 768（**不溢出，无滚动条**）/ plugin-panel 768（height:100% 解析）/
// pp-list 745（768 - 23 头部）且内容 1304 → **只有 .pp-list 一层滚动条**。
func TestSidebarPanelSingleScrollLayer(t *testing.T) {
	got := layoutProbe(t, 1400, 800, sidebarScrollProbeHTML, sidebarProbeJS)
	f := strings.Split(got, "|")
	if len(f) != 5 {
		t.Fatalf("探针返回格式异常: %q", got)
	}
	// 1) .sidebar-content 不得溢出（否则它自己出滚动条 = 第二层）。
	if f[0] != f[1] {
		t.Errorf(".sidebar-content 溢出（会多出一层滚动条）：client=%s scroll=%s\n"+
			"  → flex:1 分配的确定高度 768 未被当作包含块高度（完整读数 %s）", f[0], f[1], got)
	}
	// 2) .plugin-panel 的 height:100% 必须解析为父内容高 768（而非内容高 1363）。
	if f[2] != "768" {
		t.Errorf(".plugin-panel{height:100%%} 未解析为父内容高：got %s want 768（浏览器行为）\n"+
			"  完整读数 %s", f[2], got)
	}
	// 3) .pp-list 必须是唯一溢出的滚动层。
	lClient, err1 := strconv.Atoi(f[3])
	lScroll, err2 := strconv.Atoi(f[4])
	if err1 != nil || err2 != nil {
		t.Fatalf("探针数值解析失败: %q", got)
	}
	if lScroll <= lClient {
		t.Errorf(".pp-list 未出滚动条：client=%d scroll=%d（1300px 内容应溢出 745 的 flex 槽）", lClient, lScroll)
	}
}
