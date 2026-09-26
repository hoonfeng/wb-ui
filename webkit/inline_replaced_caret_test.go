package webkit

import "testing"

// caretFixtureHTML 复刻 gou-ide 顶栏「快速执行」按钮的结构（ui-quick-exec 插件
// 注入 titlebar-right 槽位的真实 DOM，2026-09-26 定位）：
//
//	<button id=btn>              display:inline-flex; gap:4px; padding:0 8px; border:1px
//	  <svg id=bolt width=11>     bolt 图标（flex item，直接子项）
//	  <span>快速执行</span>      文本（flex item）
//	  <span id=caret>            caret 包装（flex item，margin-left:2px）
//	    <svg id=caretsvg w=8>    下拉小三角（replaced，在 span 的 IFC 内按基线摆放）
//
// 该结构曾同时暴露两个布局缺陷（引擎与浏览器在 1280x800 同视口下的差异）：
//
//	① 横向：按钮 border-box 宽被算成 77（浏览器 90.6）—— computeInlineContentWidth
//	   只统计文本段，漏掉末尾替换元素（caret svg）的占位宽，而 flex 内部按内容自然宽
//	   73 摆放子项 → caret 右缘越过按钮 padding box 右边界 14px，视觉上「小三角跑到
//	   边框外」。
//	② 垂直：caret svg 贴 span 顶（y=12.0）而浏览器在 16.0 —— replaced 子项只按
//	   centeringOffset 偏移，缺 ascent 项；且该 span 未声明 line-height（cssLH=0）
//	   时偏移恒为 0，暴露得最彻底。
func caretFixtureHTML() string {
	return `<!DOCTYPE html><html><body style="margin:0">
<button id="btn" style="display:inline-flex;align-items:center;gap:4px;height:22px;padding:0 8px;font-size:11px;font-family:Arial,sans-serif;border:1px solid #30363d;border-radius:4px;box-sizing:border-box;background:#21262d">
<svg id="bolt" width="11" height="11" viewBox="0 0 16 16" fill="currentColor"><path d="M8.5 1 3.2 9h4l-.7 6L12 7H8l.5-6z"/></svg>
<span>快速执行</span>
<span id="caret" style="margin-left:2px"><svg id="caretsvg" width="8" height="8" viewBox="0 0 16 16" fill="currentColor"><path d="M4 6l4 4 4-4H4z"/></svg></span>
</button></body></html>`
}

// loadCaretFixture 加载夹具并驱动若干轮布局/渲染（与其它 webkit 用例一致）。
func loadCaretFixture(t *testing.T) *WebView {
	t.Helper()
	wv := NewWebView()
	t.Cleanup(func() { wv.Destroy() })
	wv.Resize(800, 200)
	if err := wv.LoadHTML(caretFixtureHTML()); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	for i := 0; i < 10; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	return wv
}

// TestInlineFlexButtonWidthIncludesTrailingReplacedChild 钉死缺陷①：
// inline-flex 按钮的固有宽度必须把末尾的替换元素（caret svg）算进去 ——
// 小三角的右缘要精确贴合按钮 padding box 的右边界，且按钮内部不得溢出。
func TestInlineFlexButtonWidthIncludesTrailingReplacedChild(t *testing.T) {
	wv := loadCaretFixture(t)
	got := dynEval(t, wv, `(function(){
		var btn=document.getElementById('btn'), b=btn.getBoundingClientRect();
		var cs=getComputedStyle(btn);
		var padRight=b.x+b.width-(parseFloat(cs.borderRightWidth)||0)-(parseFloat(cs.paddingRight)||0);
		var c=document.getElementById('caretsvg').getBoundingClientRect();
		var caretRight=c.x+c.width;
		if(caretRight>padRight+0.5){
			return 'OUT:caretRight='+caretRight.toFixed(1)+'>padRight='+padRight.toFixed(1);
		}
		if(Math.abs(caretRight-padRight)>1.0){
			return 'NOT_FLUSH:caretRight='+caretRight.toFixed(1)+' padRight='+padRight.toFixed(1);
		}
		if(btn.scrollWidth>btn.clientWidth){
			return 'OVERFLOW:scrollWidth='+btn.scrollWidth+'>clientWidth='+btn.clientWidth;
		}
		return 'ok';
	})()`)
	if got != "ok" {
		t.Fatalf("inline-flex 按钮宽度漏算末尾替换元素（caret svg 越出边框）：%s\n"+
			"  期望 caret 右缘 == 按钮 padding box 右边界（浏览器 1280 视口下 90.6 宽按钮的 caret 右缘 1263.2）", got)
	}
}

// TestInlineReplacedChildBaselineAlignment 钉死缺陷②：
// 行内替换元素（svg）必须坐在父行盒的基线上（CSS 2.1 §10.8.1），而不是贴行盒顶。
// 对外可观测的等价断言：svg 的垂直中心与所在 inline 包装（span）的中心相差 <1px
// （浏览器实测 0.4px；修复前为 3.5px，因为 svg 顶 = span 顶）。
func TestInlineReplacedChildBaselineAlignment(t *testing.T) {
	wv := loadCaretFixture(t)
	got := dynEval(t, wv, `(function(){
		var s=document.getElementById('caret').getBoundingClientRect();
		var c=document.getElementById('caretsvg').getBoundingClientRect();
		if(c.y<s.y+1.0){
			return 'TOP_GLUED:svgY='+c.y.toFixed(1)+' spanY='+s.y.toFixed(1);
		}
		var sc=s.y+s.height/2, cc=c.y+c.height/2;
		if(Math.abs(sc-cc)>1.0){
			return 'NOT_CENTERED:svgCenter='+cc.toFixed(1)+' spanCenter='+sc.toFixed(1);
		}
		return 'ok';
	})()`)
	if got != "ok" {
		t.Fatalf("行内替换元素未按行盒基线对齐（小三角垂直位置错）：%s\n"+
			"  浏览器：svg 底边坐在行盒基线上（该夹具下 svg 顶 15.9/16.0、span 中心 19.6）", got)
	}
}
