package webkit

// 探针（保留为正式回归）：configwin 左侧树「展开/折叠」交互——点击
// tree-head（含子元素 arrow/gname/gcount 覆盖）→ toggleGroup 执行、
// className 翻转、渲染树更新（tree-body display）、箭头图标旋转
// （.tree-group.open .tree-head .arrow transform:rotate）。
// 覆盖用户反馈：点击延迟生效/不生效 + 展开图标不变。

import (
	"strings"
	"testing"

	"wb-ui/engine/dom"
)

func treeProbeSpans(doc *dom.Document, cls string) []*dom.Element {
	var out []*dom.Element
	for _, e := range doc.GetElementsByTagName("span") {
		if strings.Contains(e.GetAttribute("class"), cls) {
			out = append(out, e)
		}
	}
	return out
}

func treeProbeHTML() string {
	return `<style>
.tree-group{margin:4px 6px;border:1px solid #232f4a;border-radius:8px;overflow:hidden;background:#1a2030}
.tree-head{display:flex;align-items:center;gap:6px;padding:10px 12px;font-size:15px;cursor:pointer;position:relative;color:#c8d4f0}
.tree-head:hover{background:#243050}
.tree-head .arrow{width:14px;height:14px;display:inline-flex;align-items:center;justify-content:center;color:#8fa3cc}
.tree-group.open .tree-head .arrow{transform:rotate(90deg)}
.tree-head .gname{flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.tree-head .gcount{font-size:15px;color:#5a6a8c}
.tree-body{display:none;padding:2px 0}
.tree-group.open .tree-body{display:block}
.tree-item{display:flex;align-items:center;gap:6px;padding:6px 10px 6px 26px;font-size:14px;color:#a9b8d8;cursor:pointer;position:relative}
</style>
<div class="tree-group open" id="grp-current">
<div class="tree-head" onclick="toggleGroup('grp-current')"><span class="arrow">&#9654;</span><span class="gname">当前配置</span><span class="gcount">2</span></div>
<div class="tree-body"><div class="tree-item" onclick="selectWidget('a')"><span class="iname">时钟</span></div></div>
</div>
<script>
function toggleGroup(gid){ var g = document.getElementById(gid); if (!g) return;
  g.className = g.className.indexOf('open')>=0 ? g.className.replace(' open','') : g.className+' open'; }
function selectWidget(x){ window.__sel = x; }
</script>`
}

func clickAt(t *testing.T, wv *WebView, cx, cy float64) {
	t.Helper()
	wv.HandleMouseButton(cx, cy, 0, 0)
	wv.HandleMouseButton(cx, cy, 0, 1)
}

func renderOnce(t *testing.T, wv *WebView) {
	t.Helper()
	if _, err := wv.Render(); err != nil {
		t.Fatalf("render: %v", err)
	}
}

// TestTreeProbeClickGname 点击 gname（子元素覆盖）→ toggleGroup 执行。
func TestTreeProbeClickGname(t *testing.T) {
	wv := interactTestWebView(t, treeProbeHTML())
	doc := wv.Document()
	gname := treeProbeSpans(doc, "gname")
	if len(gname) == 0 {
		t.Fatal("gname not found")
	}
	box := findBox(wv, gname[0])
	if box == nil {
		t.Fatal("gname render box not found")
	}
	cx, cy := box.Center()
	clickAt(t, wv, cx, cy)
	cls, _ := wv.EvalJS(`document.getElementById('grp-current').className`)
	t.Logf("点击 gname 后 className = %q", cls.ToString())
	if strings.Contains(cls.ToString(), "open") {
		t.Fatalf("点击 gname 未折叠（open 仍存在）：%q", cls.ToString())
	}
	renderOnce(t, wv)
	body := collectByClass(doc, "tree-body")
	if len(body) == 0 {
		t.Fatal("tree-body not found")
	}
	rv := wv.RenderView()
	b2 := rv.FindRenderBoxForNode(body[0])
	if b2 != nil && b2.Width() > 0 {
		t.Fatalf("折叠后 tree-body 仍可见（渲染树未更新）")
	}
	t.Log("折叠：click 即时生效 + tree-body 不可见 ✓")
	// 再点一次 → 展开
	clickAt(t, wv, cx, cy)
	cls2, _ := wv.EvalJS(`document.getElementById('grp-current').className`)
	t.Logf("再次点击后 className = %q", cls2.ToString())
	if !strings.Contains(cls2.ToString(), "open") {
		t.Fatalf("再次点击未展开：%q", cls2.ToString())
	}
	renderOnce(t, wv)
	rv2 := wv.RenderView()
	b3 := rv2.FindRenderBoxForNode(body[0])
	if b3 == nil || b3.Width() == 0 {
		t.Fatalf("展开后 tree-body 不可见")
	}
	t.Log("展开：tree-body 可见 ✓")
}

// TestTreeProbeClickArrow 点击 arrow（子元素）→ toggle + 图标旋转。
func TestTreeProbeClickArrow(t *testing.T) {
	wv := interactTestWebView(t, treeProbeHTML())
	doc := wv.Document()
	arrow := treeProbeSpans(doc, "arrow")
	if len(arrow) == 0 {
		t.Fatal("arrow not found")
	}
	box := findBox(wv, arrow[0])
	if box == nil {
		t.Fatal("arrow render box not found")
	}
	cx, cy := box.Center()
	clickAt(t, wv, cx, cy) // 折叠
	cls, _ := wv.EvalJS(`document.getElementById('grp-current').className`)
	t.Logf("点击 arrow 后 className = %q", cls.ToString())
	if strings.Contains(cls.ToString(), "open") {
		t.Fatalf("点击 arrow 未折叠：%q", cls.ToString())
	}
	// 展开（图标旋转态）
	clickAt(t, wv, cx, cy)
	renderOnce(t, wv)
	rv := wv.RenderView()
	b := rv.FindRenderBoxForNode(arrow[0])
	if b == nil {
		t.Fatal("arrow render box nil")
	}
	tr := b.Style().Transform
	t.Logf("展开态 arrow computed transform = %q", tr)
	if !strings.Contains(tr, "rotate") {
		t.Fatalf("展开态 arrow 无旋转 transform：%q", tr)
	}
	// 折叠（旋转回 0）
	clickAt(t, wv, cx, cy)
	renderOnce(t, wv)
	rv2 := wv.RenderView()
	b2 := rv2.FindRenderBoxForNode(arrow[0])
	if b2 == nil {
		t.Fatal("arrow render box nil (fold)")
	}
	tr2 := b2.Style().Transform
	t.Logf("折叠态 arrow computed transform = %q", tr2)
	if strings.Contains(tr2, "rotate") {
		t.Fatalf("折叠态 arrow 仍带旋转：%q", tr2)
	}
}

// TestTreeProbeClickGcount 点击 gcount（自身无 onclick，同组当前配置）→ 冒泡到 head。
func TestTreeProbeClickGcount(t *testing.T) {
	wv := interactTestWebView(t, treeProbeHTML())
	doc := wv.Document()
	gc := treeProbeSpans(doc, "gcount")
	if len(gc) == 0 {
		t.Fatal("gcount not found")
	}
	box := findBox(wv, gc[0])
	if box == nil {
		t.Fatal("gcount render box not found")
	}
	cx, cy := box.Center()
	clickAt(t, wv, cx, cy)
	cls, _ := wv.EvalJS(`document.getElementById('grp-current').className`)
	t.Logf("点击 gcount 后 className = %q", cls.ToString())
	if strings.Contains(cls.ToString(), "open") {
		t.Fatalf("点击 gcount 未折叠（未冒泡到 head）：%q", cls.ToString())
	}
}

// TestTreeProbeClickItem 树条目（tree-item 内含 iname 子元素）→ selectWidget
// 冒泡执行（与树头同样的容器 onclick 冒泡路径）。
func TestTreeProbeClickItem(t *testing.T) {
	wv := interactTestWebView(t, treeProbeHTML())
	doc := wv.Document()
	items := collectByClass(doc, "tree-item")
	if len(items) == 0 {
		t.Fatal("tree-item not found")
	}
	// 点 item 文字（iname span）
	names := treeProbeSpans(doc, "iname")
	if len(names) == 0 {
		t.Fatal("iname not found")
	}
	box := findBox(wv, names[0])
	if box == nil {
		t.Fatal("iname render box not found")
	}
	cx, cy := box.Center()
	clickAt(t, wv, cx, cy)
	if v, err := wv.EvalJS(`window.__sel||''`); err == nil {
		t.Logf("点击 iname 后 __sel = %q", v.ToString())
		if v.ToString() != "a" {
			t.Fatalf("点击 iname 未触发 selectWidget：%q", v.ToString())
		}
	} else {
		t.Fatalf("EvalJS: %v", err)
	}
}
