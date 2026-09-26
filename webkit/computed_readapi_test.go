package webkit

import "testing"

func loadAndSettle(t *testing.T, html string) *WebView {
	t.Helper()
	wv := NewWebView()
	t.Cleanup(func() { wv.Destroy() })
	wv.Resize(420, 320)
	if err := wv.LoadHTML(html); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	for i := 0; i < 8; i++ {
		wv.EnsureLayout()
		if _, err := wv.Render(); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	return wv
}

// TestComputedShorthandExpansion 钉死「简写必须展开为长写」：浏览器
// getComputedStyle 对 `overflow:auto` 必给 overflowX/overflowY === "auto"，
// 对 `border:2px solid #123` 必给 borderTopStyle === "solid"。
// 此前引擎只展开 border-width，读 overflowX / borderTopStyle 得 undefined →
// 依赖它们做滚动/边框判断的 JS 走错分支。
func TestComputedShorthandExpansion(t *testing.T) {
	wv := loadAndSettle(t, `<!DOCTYPE html><html><body style="margin:0">
<div id="box" style="width:120px;height:40px;overflow:auto;border:2px dashed #123456;
     border-left-width:5px">x</div>
</body></html>`)
	got := dynEval(t, wv, `(function(){
  var cs=getComputedStyle(document.getElementById("box"));
  function r(p){ var c=p.replace(/-([a-z])/g,function(_,x){return x.toUpperCase();});
    return p+"="+cs[c]+"(gpv:"+cs.getPropertyValue(p)+")"; }
  return r("overflow-x")+" "+r("overflow-y")+" "+r("border-top-style")+" "+
         r("border-right-style")+" "+r("border-left-width")+" "+r("border-top-width");
})()`)
	t.Logf("简写展开实测 = %s", got)
	for _, want := range []string{
		"overflow-x=auto(gpv:auto)", "overflow-y=auto(gpv:auto)",
		"border-top-style=dashed(gpv:dashed)", "border-right-style=dashed(gpv:dashed)",
		"border-left-width=5px(gpv:5px)", "border-top-width=2px(gpv:2px)",
	} {
		if !contains(got, want) {
			t.Fatalf("长写展开缺失/错误：期望含 %q，实测 %s", want, got)
		}
	}
}

// TestComputedInitialValueFallback 钉死「computed style 对每个属性恒有值」
// （未声明 = CSS 初始值）。浏览器对未声明属性返回 16px/normal/400/auto/auto/none，
// 此前引擎返回 undefined → `parseFloat(getComputedStyle(el).fontSize)` 等得到 NaN。
func TestComputedInitialValueFallback(t *testing.T) {
	wv := loadAndSettle(t, `<!DOCTYPE html><html><body style="margin:0">
<div id="plain" style="width:80px;height:20px">plain</div>
</body></html>`)
	got := dynEval(t, wv, `(function(){
  var cs=getComputedStyle(document.getElementById("plain"));
  var props=["font-size","line-height","font-weight","pointer-events","min-width","max-width","text-align","box-sizing"];
  var miss=[], out=[];
  for (var i=0;i<props.length;i++){
    var p=props[i]; var c=p.replace(/-([a-z])/g,function(_,x){return x.toUpperCase();});
    var v=cs[c];
    var g=(typeof cs.getPropertyValue==="function")?cs.getPropertyValue(p):null;
    if ((v===undefined||v==="") || (g===null||g===undefined||g==="")) miss.push(p);
    else out.push(p+"["+v+"/"+g+"]");
  }
  return "MISSING["+miss.length+"]="+miss.join(",")+" || "+out.join(" ");
})()`)
	t.Logf("初始值回退实测 = %s", got)
	if !contains(got, "MISSING[0]=") {
		t.Fatalf("仍有属性读不到 computed 值（浏览器恒有值）：%s", got)
	}
	// 属性访问（camelCase）与 getPropertyValue（kebab）必须**双路**都有值：
	// 浏览器 getPropertyValue 对未声明属性同样返回初始值，仅补属性访问会漏一半。
	for _, want := range []string{"font-size[16px/16px]", "line-height[normal/normal]", "font-weight[400/400]",
		"pointer-events[auto/auto]", "min-width[auto/auto]", "max-width[none/none]", "box-sizing[content-box/content-box]"} {
		if !contains(got, want) {
			t.Fatalf("初始值不符：期望含 %q，实测 %s", want, got)
		}
	}
}
