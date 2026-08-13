package bindings

// 复现 Vue insertStaticContent 的 SVG 分支：
// templateContainer.innerHTML='<svg>...</svg>' → content 移动子节点到
// fragment → while (wrapper.firstChild) template.appendChild(...) 循环。
// 若 appendChild 没有真正把子节点从 wrapper 移走，循环永不结束
// （desktop 全空白 = mount 挂死于此）。
import (
	"strings"
	"testing"

	"wb-ui/jsc"
)

func TestTemplateContentSVGMoveLoop(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	tpl := doc.CreateElement("template")
	doc.AppendChild(tpl)

	iv, err := rt.RunJS(`(function(){
		var t = document.querySelector('template');
		t.innerHTML = '<svg><circle/><path/><rect/></svg>';
		var svg = t.firstChild;
		var before = 'svgp=' + (svg.parentNode ? svg.parentNode.nodeName : 'nil')
			+ ' tfc=' + (t.firstChild ? t.firstChild.nodeName : 'null')
			+ ' tcc=' + t.childNodes.length;
		var frag2 = document.createDocumentFragment();
		frag2.appendChild(svg);
		var pre = 'tfc=' + (t.firstChild ? t.firstChild.nodeName : 'null')
			+ ' ff=' + (frag2.firstChild ? frag2.firstChild.nodeName : 'null')
			+ ' svgp=' + (svg.parentNode ? svg.parentNode.nodeName : 'nil');
		var wrapper = frag2.firstChild;
		var n = 0;
		var log = [];
		while (wrapper.firstChild) {
			var c = wrapper.firstChild;
			frag2.appendChild(c);
			var after = ' c=' + c.nodeName
				+ ' wp=' + (c.parentNode ? c.parentNode.nodeName : 'nil')
				+ ' wc=' + wrapper.childNodes.length
				+ ' fc=' + frag2.childNodes.length
				+ ' sameLast=' + (frag2.lastChild === c)
				+ ' sameFirst=' + (wrapper.firstChild === c);
			log.push(after);
			if (++n > 4) return 'LOOP:' + n + ' before=' + before + ' pre=' + pre + ' |' + log.join(' ||');
		}
		return 'OK moved=' + n + ' before=' + before + ' pre=' + pre + ' |' + log.join(' ||');
	})()`)
	if err != nil {
		t.Fatalf("RunJS: %v", err)
	}
	t.Logf("result: %s", iv.ToString())
	if !strings.HasPrefix(iv.ToString(), "OK moved=3 ") {
		t.Fatalf("unexpected result: %s", iv.ToString())
	}
	// 关键断言：三个子节点全部从 wrapper 移出（wc 递减到 0，循环终止）。
	if !strings.Contains(iv.ToString(), "c=RECT") || !strings.Contains(iv.ToString(), "wc=0") {
		t.Fatalf("children not fully moved: %s", iv.ToString())
	}
	// content 再次访问：template 已被搬空，应返回空 fragment
	iv2, err := rt.RunJS(`(function(){
		var t = document.querySelector('template');
		return t.content.childNodes.length;
	})()`)
	if err != nil {
		t.Fatalf("RunJS2: %v", err)
	}
	if iv2.ToString() != "0" {
		t.Fatalf("second content access children=%s, want 0", iv2.ToString())
	}
}

// TestTemplateInnerHTMLRoundTrip innerHTML 设置后 content 必须能反复使用
// （Vue 多次 insertStaticContent 复用同一 templateContainer）。
func TestTemplateInnerHTMLRoundTrip(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	tpl := doc.CreateElement("template")
	doc.AppendChild(tpl)
	for i := 0; i < 3; i++ {
		iv, err := rt.RunJS(`(function(){
			var t = document.querySelector('template');
			t.innerHTML = '<div class="s"><span>a</span></div>';
			var frag = t.content;
			return frag.childNodes.length + ':' + frag.firstChild.nodeName;
		})()`)
		if err != nil {
			t.Fatalf("round %d: %v", i, err)
		}
		if iv.ToString() != "1:DIV" {
			t.Fatalf("round %d: got %s, want 1:DIV", i, iv.ToString())
		}
		// 模拟 Vue：把 fragment 插入文档（子节点转移）
		_, _ = rt.RunJS(`(function(){
			var t = document.querySelector('template');
			var frag = t.content;
			var host = document.createElement('div');
			document.body.appendChild(host);
			host.insertBefore(frag, null);
			return host.childNodes.length;
		})()`)
	}
	_ = jsc.Undefined // keep import
}

// 确保 content 移动语义下 template 自身仍保留（Vue 复用同一容器）。
func TestTemplateContentKeepsTemplate(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	tpl := doc.CreateElement("template")
	doc.AppendChild(tpl)
	_, _ = rt.RunJS(`(function(){
		var t = document.querySelector('template');
		t.innerHTML = '<i>x</i>';
		var f = t.content;
	})()`)
	if tpl.FirstChild() != nil {
		t.Fatalf("template still has children after content access")
	}
	if tpl.ParentNode() == nil {
		t.Fatal("template element should remain in document")
	}
}
