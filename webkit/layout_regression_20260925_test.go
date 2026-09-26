package webkit

import (
	"strconv"
	"strings"
	"testing"
)

// layoutProbe 加载 HTML、跑若干帧布局+渲染，返回 js 的 eval 结果。
func layoutProbe(t *testing.T, w, h int, html, js string) string {
	t.Helper()
	wv := NewWebView()
	defer wv.Destroy()
	wv.Resize(w, h)
	if err := wv.LoadHTML(html); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	for i := 0; i < 10; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	return dynEval(t, wv, js)
}

// TestMarginShorthandThreeValues 钉死 `margin: 4px 8px 8px`（3 值简写 =
// top 4 / left+right 8 / bottom 8）的解析。
//
// gou-ide 左栏底部胶囊 .conv-footer-pill 用该简写：wb-ui 实测 x=49（= 侧栏左缘），
// 浏览器 57（= 48 + 1 border + 8 margin）——margin 全丢，胶囊左右两侧顶死容器。
func TestMarginShorthandThreeValues(t *testing.T) {
	html := `<!DOCTYPE html><html><body style="margin:0">
<div id="p" style="width:200px;height:100px;background:#eeeeee">
<div id="c" style="margin:4px 8px 8px;width:50px;height:20px;background:#333333"></div>
</div></body></html>`
	got := layoutProbe(t, 400, 200, html, `(function(){var b=document.getElementById("c").getBoundingClientRect();return Math.round(b.x)+","+Math.round(b.y)+" "+Math.round(b.width)+"x"+Math.round(b.height);})()`)
	want := "8,4 50x20"
	if got != want {
		t.Fatalf("margin 3 值简写解析错误：\n  got  %s\n  want %s", got, want)
	}
}

// TestMarginShorthandTwoValues 钉死 `margin: 10px 20px`（2 值简写 = 上下 10、左右 20）。
func TestMarginShorthandTwoValues(t *testing.T) {
	html := `<!DOCTYPE html><html><body style="margin:0">
<div id="p" style="width:200px;height:100px;background:#eeeeee">
<div id="c" style="margin:10px 20px;width:50px;height:20px;background:#333333"></div>
</div></body></html>`
	got := layoutProbe(t, 400, 200, html, `(function(){var b=document.getElementById("c").getBoundingClientRect();return Math.round(b.x)+","+Math.round(b.y);})()`)
	want := "20,10"
	if got != want {
		t.Fatalf("margin 2 值简写解析错误：\n  got  %s\n  want %s", got, want)
	}
}

// TestSelectIntrinsicWidthFitsOption 钉死 select 的内在宽度：
// 无显式 width 的 select 应按最长 option 文本宽度收缩（shrink-to-fit），
// 而不是塌成几像素。gou-ide 输入区模型选择器 .sp-select 在 wb-ui 里宽 38px
// （文本全截断成 "…"），浏览器 240px。
func TestSelectIntrinsicWidthFitsOption(t *testing.T) {
	html := `<!DOCTYPE html><html><body style="margin:0">
<select id="s" style="font-size:12px"><option>暂无可用 AI 配置：请先在设置里添加</option></select>
</body></html>`
	got := layoutProbe(t, 800, 200, html, `(function(){var b=document.getElementById("s").getBoundingClientRect();return String(Math.round(b.width));})()`)
	w, err := strconv.Atoi(got)
	if err != nil {
		t.Fatalf("宽度解析失败：%q", got)
	}
	// 18 个 CJK 字符 × 12px ≈ 216px，加内边距/箭头留白；宽 38 说明只算了几个字符。
	if w < 150 {
		t.Fatalf("select 内在宽度未包含 option 文本：got width=%d, want >=150（浏览器同 HTML ≈240）", w)
	}
}

// TestFlexAutoMarginTakesAllRemainingSpace 钉死：单个 margin-left:auto 项必须
// 吃掉**全部**剩余空间（CSS-FLEXBOX §9.5），而不是一半。
//
// gou-ide 输入卡行 3（.input-bottom-bar：.ibb-btns + .ibb-enter-hint(ml:auto) +
// .send-btn）实测 hint x=622，浏览器 727——剩余空间 209px 只被吃掉 105px（≈一半），
// 导致 "Enter 发送 · Shift+Enter 换行" 悬在输入框中间。
func TestFlexAutoMarginTakesAllRemainingSpace(t *testing.T) {
	html := `<!DOCTYPE html><html><body style="margin:0">
<div id="bar" style="display:flex;width:600px;height:32px;padding:0 10px;box-sizing:border-box;gap:8px">
<div id="a" style="flex:0 0 auto;width:158px;height:32px;background:#222222"></div>
<span id="h" style="margin-left:auto;flex:0 0 auto;width:147px;height:20px;background:#444444"></span>
<button id="b" style="flex:0 0 auto;width:72px;height:32px">send</button>
</div></body></html>`
	got := layoutProbe(t, 800, 100, html, `(function(){var h=document.getElementById("h").getBoundingClientRect();var b=document.getElementById("b").getBoundingClientRect();return "h="+Math.round(h.x)+" b="+Math.round(b.x);})()`)
	// 内容区 10..590：a 占 10..168，gap 8；b 贴右 518..590；hint 应在 b 左侧留 gap：
	// h.x = 518 - 8 - 147 = 363。
	want := "h=363 b=518"
	if got != want {
		t.Fatalf("margin-left:auto 未吃掉全部剩余空间：\n  got  %s\n  want %s", got, want)
	}
}

// TestFlexItemMarginInsideFlexColumn 钉死：flex column 容器内**子项**的 margin
// 简写必须生效（上面的用例验证的是块容器内的 margin）。
//
// gou-ide 左栏底部胶囊 .conv-footer-pill（display:flex 容器 .conv-sidebar 的
// flex item，自身 margin:4px 8px 8px）在 wb-ui 里 x=49（= 容器左缘）、y=170；
// 浏览器 x=57（+8 margin）、y=169 —— margin-left/top 在 flex item 位置上整体丢失，
// 但同一条规则里的 height/padding/gap 都生效（胶囊宽 247 一致）。
func TestFlexItemMarginInsideFlexColumn(t *testing.T) {
	html := `<!DOCTYPE html><html><body style="margin:0">
<div id="col" style="display:flex;flex-direction:column;width:264px;height:130px;background:#111111">
<div id="p" style="margin:4px 8px 8px;height:32px;background:#333333"></div>
</div></body></html>`
	got := layoutProbe(t, 400, 200, html, `(function(){var b=document.getElementById("p").getBoundingClientRect();return Math.round(b.x)+","+Math.round(b.y)+" "+Math.round(b.width)+"x"+Math.round(b.height);})()`)
	// 容器 264 宽：左右各 8 margin → 宽 248；margin-top 4 → y=4。
	want := "8,4 248x32"
	if got != want {
		t.Fatalf("flex column 内子项 margin 未生效：\n  got  %s\n  want %s", got, want)
	}
}

// TestSelectWidthWithDynamicOptions 钉死：**运行时创建**的 option 必须参与 select
// 的内在宽度（shrink-to-fit）计算。
//
// gou-ide 输入卡模型选择器（SheetPicker 的 .sp-select，Vue 渲染 7 个 option）在
// wb-ui 里宽 38px、scrollWidth=0（选项文本全截断），浏览器 240px。
// 静态 option 的用例（TestSelectIntrinsicWidthFitsOption）已通过 → 差异在动态创建。
func TestSelectWidthWithDynamicOptions(t *testing.T) {
	html := `<!DOCTYPE html><html><body style="margin:0">
<select id="s" style="appearance:none;-webkit-appearance:none;font-size:12px;padding:0 20px 0 10px;border:none;background:none"></select>
<script>
var s=document.getElementById("s");
["选择工具集…","计划讨论 · 9 个插件","全栈开发 · 14 个插件","办公 · 9 个插件","调试 · 9 个插件","基础 · 默认生效 · 7 个插件","全功能 · 16 个插件"].forEach(function(t){
  var o=document.createElement("option"); o.textContent=t; s.appendChild(o);
});
</script>
</body></html>`
	got := layoutProbe(t, 800, 200, html, `(function(){var b=document.getElementById("s").getBoundingClientRect();return String(Math.round(b.width))+" sw="+document.getElementById("s").scrollWidth;})()`)
	head := got
	if i := strings.IndexByte(got, ' '); i >= 0 {
		head = got[:i]
	}
	w, err := strconv.Atoi(head)
	if err != nil {
		t.Fatalf("宽度解析失败：%q", got)
	}
	// 最长 option「基础 · 默认生效 · 7 个插件」在 12px 下 ≈ 150px；+ padding 30 → ≈180。
	// 宽 38 说明运行时 option 的文本宽度完全未参与（当前缺陷）。
	if w < 140 {
		t.Fatalf("动态 option 的 select 内在宽度塌陷：got %s（浏览器同 DOM ≈240）", got)
	}
}

// TestSelectWidthWithMaxWidthAndPadding 钉死带 `max-width` + 大 padding 的 select
// 内在宽度（gou-ide .sp-select 的真实声明组合：appearance:none + max-width:240px +
// padding:5px 26px 5px 10px + border + font-size:12px）。
// 静态/动态 option 两个用例都已通过 → 继续逼近真实声明，定位 38px 塌陷的触发条件。
func TestSelectWidthWithMaxWidthAndPadding(t *testing.T) {
	html := `<!DOCTYPE html><html><body style="margin:0">
<select id="s" style="appearance:none;-webkit-appearance:none;max-width:240px;padding:5px 26px 5px 10px;border:1px solid #444444;font-size:12px;line-height:1.4">
<option>选择工具集…</option><option>基础 · 默认生效 · 7 个插件</option><option>全功能 · 16 个插件</option>
</select></body></html>`
	got := layoutProbe(t, 800, 200, html, `(function(){var s=document.getElementById("s");var b=s.getBoundingClientRect();return String(Math.round(b.width))+" sw="+s.scrollWidth;})()`)
	head := got
	if i := strings.IndexByte(got, ' '); i >= 0 {
		head = got[:i]
	}
	w, err := strconv.Atoi(head)
	if err != nil {
		t.Fatalf("宽度解析失败：%q", got)
	}
	// 最长 option「基础 · 默认生效 · 7 个插件」≈150px + padding 36 + border 2 → ≈188（未触 max-width）。
	// 若这里塌成 38 则说明 max-width/padding 组合打断了内在宽度测量。
	if w < 140 {
		t.Fatalf("带 max-width/padding 的 select 内在宽度塌陷：got %s（浏览器同 DOM ≈188）", got)
	}
}

// TestSelectWidthWithOptgroup 钉死：位于 <optgroup> 内的 option 同样参与 select
// 的内在宽度（shrink-to-fit）。
//
// gou-ide 模型选择器的 option 由 SheetPicker 的 groupedItems 渲染：**带 group 的项
// 包在 <optgroup> 里**（SheetPicker.vue 模板的 optgroup 分支）。前三个用例
// （静态/动态 option、max-width+padding）都通过 → optgroup 分支是剩下的可疑触发条件。
func TestSelectWidthWithOptgroup(t *testing.T) {
	html := `<!DOCTYPE html><html><body style="margin:0">
<select id="s" style="appearance:none;-webkit-appearance:none;max-width:240px;padding:5px 26px 5px 10px;border:1px solid #444444;font-size:12px">
<option value="" disabled>选择模型…</option>
<optgroup label="本地"><option>基础 · 默认生效 · 7 个插件</option></optgroup>
<optgroup label="云端"><option>全功能 · 16 个插件</option></optgroup>
</select></body></html>`
	got := layoutProbe(t, 800, 200, html, `(function(){var s=document.getElementById("s");var b=s.getBoundingClientRect();return String(Math.round(b.width))+" sw="+s.scrollWidth;})()`)
	head := got
	if i := strings.IndexByte(got, ' '); i >= 0 {
		head = got[:i]
	}
	w, err := strconv.Atoi(head)
	if err != nil {
		t.Fatalf("宽度解析失败：%q", got)
	}
	if w < 140 {
		t.Fatalf("optgroup 内 option 未参与 select 内在宽度：got %s（浏览器同 DOM ≈188）", got)
	}
}

// TestScopedAttributeSelectorApplies 钉死：`.class[attr]` 组合选择器必须命中元素。
//
// Vue SFC 的 `<style scoped>` 编译产物就是这种形式（`.sp-select[data-v-58d34149]`）。
// gou-ide 输入卡模型选择器在 wb-ui 里 width=38px、computed 读不到 padding/max-width/
// font-size —— 说明该规则整体未应用，select 退回 UA 默认尺寸（文本全截断）。
func TestScopedAttributeSelectorApplies(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
.sp-select[data-v-test]{max-width:240px;padding:5px 26px 5px 10px;border:none;font-size:12px}
</style></head><body style="margin:0">
<select id="s" class="sp-select" data-v-test=""><option>基础 · 默认生效 · 7 个插件</option></select>
</body></html>`
	got := layoutProbe(t, 800, 200, html, `(function(){var s=document.getElementById("s");var b=s.getBoundingClientRect();var cs=getComputedStyle(s);return String(Math.round(b.width))+" padL="+cs.paddingLeft+" maxW="+cs.maxWidth;})()`)
	head := got
	if i := strings.IndexByte(got, ' '); i >= 0 {
		head = got[:i]
	}
	w, err := strconv.Atoi(head)
	if err != nil {
		t.Fatalf("宽度解析失败：%q", got)
	}
	// padding 36 + 文本 ≈150 → ≈186（未触 max-width 240）。
	if w < 140 {
		t.Fatalf(".class[attr] 组合选择器未命中（Vue scoped CSS）：got %s，应 ≈186", got)
	}
}

// TestClassAttrSelectorWithDescendant 钉死 `.parent[attr] .child` 后代组合
// （Vue scoped + :deep() 的编译产物形态，如 `.ir-pill-picker[data-v-x] .sp-select`）。
func TestClassAttrSelectorWithDescendant(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
.wrap[data-v-test] .slct{width:123px;height:20px}
</style></head><body style="margin:0">
<div class="wrap" data-v-test=""><select id="s" class="slct"><option>x</option></select></div>
</body></html>`
	got := layoutProbe(t, 800, 200, html, `(function(){var b=document.getElementById("s").getBoundingClientRect();return Math.round(b.width)+"x"+Math.round(b.height);})()`)
	want := "123x20"
	if got != want {
		t.Fatalf(".parent[attr] .child 后代选择器未命中：got %s, want %s", got, want)
	}
}

// TestSelectWidthInsideInlineFlexWrapper 钉死真实嵌套结构下的 select 内在宽度：
// gou-ide 的 SheetPicker = .ibb-btns(flex, gap 8) > .sp-wrap(inline-flex) > select.sp-select。
// 平铺/optgroup/max-width 组合用例都已通过 → 差异只剩「嵌套 flex 祖先」这一层。
func TestSelectWidthInsideInlineFlexWrapper(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
.ibb-btns{display:flex;gap:8px;align-items:center;height:32px}
.ibb-btns > *{flex:0 0 auto}
.sp-wrap{position:relative;display:inline-flex;align-items:center}
.sp-select{appearance:none;max-width:240px;padding:5px 26px 5px 10px;border:1px solid #444444;font-size:12px;line-height:1.4;background:#222222;color:#dddddd}
</style></head><body style="margin:0">
<div class="ibb-btns"><div class="sp-wrap"><select id="s" class="sp-select">
<option>选择模型…</option><option>基础 · 默认生效 · 7 个插件</option><option>全功能 · 16 个插件</option>
</select></div><span id="x" style="width:32px;height:32px;background:#333333"></span></div>
</body></html>`
	got := layoutProbe(t, 800, 200, html, `(function(){var s=document.getElementById("s");var b=s.getBoundingClientRect();var w=s.parentElement.getBoundingClientRect();return Math.round(b.width)+" wrapW="+Math.round(w.width)+" sw="+s.scrollWidth;})()`)
	head := got
	if i := strings.IndexByte(got, ' '); i >= 0 {
		head = got[:i]
	}
	w, err := strconv.Atoi(head)
	if err != nil {
		t.Fatalf("宽度解析失败：%q", got)
	}
	if w < 140 {
		t.Fatalf("inline-flex 祖先内 select 内在宽度塌陷：got %s（浏览器同结构 ≈186）", got)
	}
}

// TestSelectWidthClampedByMaxWidth 钉死 max-width 对 select 内在宽度的钳制：
// gou-ide 的 .sp-select 声明 max-width:240px（box-sizing:border-box），而最宽
// option 需要 ≈259px 内容宽 → 浏览器给 240px 边框盒。
// 不钳制时 intrinsicContentWidth 会把 297px 的**外盒宽**上传给 inline-flex 容器
// .sp-wrap 的 max-content（容器自身没有 max-width，钳不住）→ .ibb-btns 超出输入卡
// 可用宽度，三个 32px 按钮被挤到第二行（真实页面实测 .ibb-btns[2]/[3] y=751）。
func TestSelectWidthClampedByMaxWidth(t *testing.T) {
	html := `<!DOCTYPE html><html><head><style>
.ibb-btns{display:flex;gap:8px;align-items:center;height:32px}
.ibb-btns > *{flex:0 0 auto}
.sp-wrap{position:relative;display:inline-flex;align-items:center}
.sp-select{appearance:none;box-sizing:border-box;max-width:240px;padding:5px 26px 5px 10px;border:1px solid #444444;font-size:12px;line-height:1.4}
</style></head><body style="margin:0">
<div class="ibb-btns"><div class="sp-wrap"><select id="s" class="sp-select">
<option>暂无可用 AI 配置：请先在「设置 → AI → AI 配置」添加服务商</option>
</select></div></div></body></html>`
	got := layoutProbe(t, 800, 200, html, `(function(){var s=document.getElementById("s");var b=s.getBoundingClientRect();var w=s.parentElement.getBoundingClientRect();return Math.round(b.width)+" wrapW="+Math.round(w.width);})()`)
	if !strings.HasPrefix(got, "240 ") {
		t.Fatalf("max-width 未钳制 select 宽度：got %s, want \"240 wrapW=240\"（该 option 文本需 ≈259px 内容宽）", got)
	}
}
