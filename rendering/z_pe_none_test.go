package rendering

// pointer-events:none 命中过滤测试：覆盖层（拖拽幽灵/提示 toast）声明
// pointer-events:none 后 HitTest 必须跳过它及其子树，命中落到下层元素
// —— 修复前 hittest.go 完全不处理该属性，幽灵在鼠标下方会被命中
// （主项目 dragGhost 用注释+坐标规避，现下沉为引擎语义）。

import (
	"testing"

	"wb-ui/css"
	"wb-ui/dom"
	"wb-ui/html"
	"wb-ui/style"
)

func peNoneHTML() string {
	return `<html><head><style>
body { margin: 0; }
.btn{position:absolute;left:100px;top:100px;width:160px;height:48px;background:#2a3a5f;border-radius:6px}
.wide{position:absolute;left:50px;top:80px;width:400px;height:200px;background:rgba(59,111,212,.12)}
/* 拖拽幽灵：fixed 悬浮在上层，pointer-events:none */
#ghost{position:fixed;left:120px;top:110px;width:120px;height:40px;background:rgba(59,111,212,.5);z-index:9999;pointer-events:none}
/* 提示 toast：同样 none */
#toast{position:fixed;left:90px;top:90px;width:200px;height:80px;z-index:9998;pointer-events:none;
       background:rgba(26,32,48,.95)}
</style></head><body>
<div class="wide" id="wide"></div>
<div class="btn" id="btn" onclick="window.__hit='btn'"></div>
<div id="toast"></div>
<div id="ghost" onclick="window.__hit='ghost'"></div>
</body></html>`
}

func TestHitTestPointerEventsNone(t *testing.T) {
	doc, err := html.ParseDocument(peNoneHTML())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	resolver := style.NewResolver()
	sheet := css.NewCSSStyleSheet()
	css.NewParser(peNoneHTML()).ParseStyleSheetInto(sheet)
	resolver.AddStyleSheet(sheet)
	rv := NewRenderTreeBuilder(resolver).Build(doc)
	rv.SetViewportSize(1280, 800)
	rv.Layout(nil)

	btn := doc.GetElementById("btn")
	cx, cy := btnC(t, rv, btn)

	// 1) 点按钮中心：ghost/toast 覆盖其上但 pointer-events:none →
	//    跳过 → 命中按钮（不得命中 ghost/toast/穿透到 wide）。
	got := HitTest(rv, cx, cy, "onclick")
	if got == nil {
		t.Fatalf("HitTest 返回 nil（应命中 btn）")
	}
	if got.GetAttribute("id") != "btn" {
		t.Fatalf("pointer-events:none 覆盖层抢走命中：实得 id=%s onclick=%q（want btn）",
			got.GetAttribute("id"), got.GetAttribute("onclick"))
	}
	// 2) 无 attr 过滤时同样跳过 none 层（hover 追踪同源）。
	got2 := HitTest(rv, cx, cy, "")
	if got2 == nil {
		t.Fatalf("HitTest(无过滤) 返回 nil")
	}
	if got2.GetAttribute("id") != "btn" {
		t.Fatalf("pointer-events:none 覆盖层抢走 hover 目标：实得 %s (want btn)",
			got2.GetAttribute("id"))
	}
	// 3) 无覆盖区域（right side beyond ghost/wide）：命中按钮自身区域外
	//    的 pan? 此处验证 ghost 自身不可命中：点 ghost 左上（仍落在
	//    btn 内也可）——改为点 btn 角落（被 toast 覆盖但非 widget）：
	//    CSS 上 toast 也 none → 命中继续落到 btn。已由 1/2 覆盖；补验
	//    pointer-events:none 元素自身不作为候选（点 ghost 唯一区域左上
	//    角 (125,115)——在 btn 范围内 → 命中 btn 而非 ghost）。
	got3 := HitTest(rv, 125, 115, "")
	if got3 == nil || got3.GetAttribute("id") != "btn" {
		t.Fatalf("none 层自身不可命中：实得 %v", got3)
	}
	_ = dom.NewDocument // keep import
}
