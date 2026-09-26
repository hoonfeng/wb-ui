package webkit

import "testing"

// gridItemHeightHTML 与 gou-ide tmp/grid-probe.html 完全一致（供浏览器对照）。
const gridItemHeightHTML = `<!DOCTYPE html><html><head><style>
html,body{margin:0;padding:0}
.app-root{display:grid;grid-template-columns:48px 1fr;grid-template-rows:30px 1fr 22px;width:400px;height:300px}
.tb{grid-column:1/-1;grid-row:1;background:#222222}
.side{grid-column:1;grid-row:2;background:#444444}
.app-statusbar-host{grid-column:1/-1;grid-row:3;height:28px;background:#666666}
</style></head><body>
<div class="app-root">
<div class="tb"></div>
<div class="side"></div>
<div class="app-statusbar-host" id="sb"></div>
</div></body></html>`

// TestGridItemExplicitHeightIsApplied 钉死 grid item 的**显式 height** 必须生效。
//
// 复现 gou-ide 的 `.app-statusbar-host`（ShellApp.vue:471 声明
// `grid-column:1/-1; grid-row:3; height:28px`）：已知现象是该 grid item 在 wb-ui
// 的 rect **高 = 视口高**（1000px），与声明的 28px 完全不符——若为普遍行为则影响
// 所有「grid 布局 + item 显式高度」的场景。
//
// 浏览器行为（同一 HTML）：grid-row 3 的轨道高 22px，但 item 声明 height:28px
// → 显式高度胜出，rect 高 = 28（CSS Grid §6.6：item 尺寸由自身尺寸属性决定，
// align-self:stretch 只作用于「未显式指定尺寸」的轴）。
func TestGridItemExplicitHeightIsApplied(t *testing.T) {
	got := layoutProbe(t, 800, 400, gridItemHeightHTML, `(function(){var b=document.getElementById("sb").getBoundingClientRect();return Math.round(b.x)+","+Math.round(b.y)+" "+Math.round(b.width)+"x"+Math.round(b.height);})()`)
	// 容器 400x300：rows = 30px / 1fr(248px) / 22px → row3 起点 y = 278。
	// 期望：显式 height 28 生效 → "0,278 400x28"。
	want := "0,278 400x28"
	if got != want {
		t.Fatalf("grid item 显式 height 未生效：\n  got  %s\n  want %s（浏览器行为）", got, want)
	}
}

// percentHeightGridHTML 与 gou-ide tmp/grid-probe.html 的第二个容器一致：
// grid item 只声明 `height:100%`（gou-ide 的 .plugin-slot-host 就是这样），
// 由内部的 .statusbar-host 承担 grid 占位。
const percentHeightGridHTML = `<!DOCTYPE html><html><head><style>
html,body{margin:0;padding:0}
.r2{display:grid;grid-template-columns:1fr;grid-template-rows:40px 1fr 28px;width:400px;height:300px}
.plugin-slot-host{height:100%;overflow:hidden}
.statusbar-host{grid-column:1/-1;grid-row:3;background:#666666}
</style></head><body>
<div class="r2">
<div style="grid-row:1;background:#222222"></div>
<div style="grid-row:2;background:#444444"></div>
<div class="plugin-slot-host statusbar-host" id="sb2"></div>
</div></body></html>`

// TestGridItemPercentHeightUsesGridArea 钉死：grid item 的 `height:100%` 的
// **包含块是 grid area（轨道尺寸）**，不是 grid 容器自身高度（CSS Grid §6.6 /
// CSS 2.1 §10.5：百分比高度相对包含块高度解析）。
//
// 这是 gou-ide 实测到的真实缺陷：`.app-root` 是 grid（rows: 40px 1fr 28px），
// 五个 `.plugin-slot-host` item 都声明 `height:100%` → wb-ui 里全部量成
// **容器高 800**（titlebar slot 应 40、sidebar/activitybar/right-rail 应 732、
// statusbar slot 应 28），只有非 slot 的 `.main-area` 正确（732）；
// 内层 `.status-bar` 因自身 28px 生效而视觉正常，掩盖了该缺陷。
func TestGridItemPercentHeightUsesGridArea(t *testing.T) {
	got := layoutProbe(t, 800, 400, percentHeightGridHTML, `(function(){var b=document.getElementById("sb2").getBoundingClientRect();return Math.round(b.x)+","+Math.round(b.y)+" "+Math.round(b.width)+"x"+Math.round(b.height);})()`)
	// 容器 400x300：rows = 40px / 1fr(232px) / 28px → row3 起点 y = 272；
	// item 的 height:100% 应解析为轨道高 28。
	want := "0,272 400x28"
	if got != want {
		t.Fatalf("grid item 的 height:100%% 未按 grid area 解析：\n  got  %s\n  want %s（浏览器行为：相对轨道 28px）", got, want)
	}
}
