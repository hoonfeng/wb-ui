package app

import (
	"strconv"
	"strings"
	"testing"

	"wb-ui/engine/dom"
	"wb-ui/engine/layout"
	"wb-ui/engine/platform/graphics"
	"wb-ui/engine/rendering"
	"wb-ui/webkit"
)

// ── 单元级：mouseleave/mouseenter 的最近共同祖先（LCA）语义 ──────────────

// TestDispatchHoverEventsCommonAncestor 钉死 UI Events 的 LCA 语义：
// 指针从 a 到 b 时，只有 LCA 之下的元素收到 mouseleave/mouseenter。
// 修复前实现无条件给 oldEl 派 mouseleave、只给 newEl 派 mouseenter，导致
// 「鼠标在同一组件内移动（祖先 → 后代）」也被判定为离开组件。
func TestDispatchHoverEventsCommonAncestor(t *testing.T) {
	newTree := func() (panel, pad, item, outside *dom.Element) {
		d := dom.NewDocument()
		root := d.CreateElement("div")
		_ = d.AppendChild(root)
		panel = d.CreateElement("div")
		_ = root.AppendChild(panel)
		pad = d.CreateElement("div")
		_ = panel.AppendChild(pad)
		item = d.CreateElement("div")
		_ = panel.AppendChild(item)
		outside = d.CreateElement("div")
		_ = d.AppendChild(outside)
		return
	}
	cases := []struct {
		name      string
		old, new  string
		wantLeave []string
		wantEnter []string
		why       string
	}{
		{
			name: "祖先→后代（面板 padding → 菜单项）", old: "panel", new: "item",
			wantLeave: nil, wantEnter: []string{"item"},
			why: "指针从未离开面板：面板不得收到 mouseleave（误派即菜单自关闭）",
		},
		{
			name: "后代→祖先（菜单项 → 面板 padding）", old: "item", new: "panel",
			wantLeave: []string{"item"}, wantEnter: nil,
			why: "离开的只有菜单项；面板仍是 hover 目标，不得收到 mouseenter",
		},
		{
			name: "兄弟（面板 padding → 菜单项，不同子节点）", old: "pad", new: "item",
			wantLeave: []string{"pad"}, wantEnter: []string{"item"},
			why: "LCA=面板：仅两个子节点互换，面板本身不参与",
		},
		{
			name: "进入窗口（nil → 菜单项）", old: "", new: "item",
			wantLeave: nil, wantEnter: []string{"panel", "item"},
			why: "自窗口外进入：进入链自外向内逐级 mouseenter",
		},
		{
			name: "离开窗口（菜单项 → nil）", old: "item", new: "",
			wantLeave: []string{"item", "panel"}, wantEnter: nil,
			why: "移出窗口：离开链自内向外逐级 mouseleave",
		},
		{
			name: "跨子树（菜单项 → 树外元素）", old: "item", new: "outside",
			wantLeave: []string{"item", "panel"}, wantEnter: []string{"outside"},
			why: "LCA=document：面板下整条链都要 leave，outside 单独 enter",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &Host{}
			panel, pad, item, outside := newTree()
			names := map[*dom.Element]string{panel: "panel", pad: "pad", item: "item", outside: "outside"}
			var leave, enter []string
			for _, el := range []*dom.Element{panel, pad, item, outside} {
				e, nm := el, names[el]
				for _, typ := range []string{dom.EventMouseLeave, dom.EventMouseEnter} {
					e.AddEventListener(typ, dom.EventListenerFunc(func(ev dom.Event) {
						if ev.Type() == dom.EventMouseLeave {
							leave = append(leave, nm)
						} else {
							enter = append(enter, nm)
						}
					}), false)
				}
			}
			pick := func(which string) *dom.Element {
				switch which {
				case "panel":
					return panel
				case "pad":
					return pad
				case "item":
					return item
				case "outside":
					return outside
				}
				return nil
			}
			h.dispatchHoverEvents(pick(tc.old), pick(tc.new), 10, 20)
			if strings.Join(leave, ",") != strings.Join(tc.wantLeave, ",") {
				t.Errorf("%s\n  mouseleave 链 = %v, want %v\n  (%s)", tc.name, leave, tc.wantLeave, tc.why)
			}
			if strings.Join(enter, ",") != strings.Join(tc.wantEnter, ",") {
				t.Errorf("%s\n  mouseenter 链 = %v, want %v\n  (%s)", tc.name, enter, tc.wantEnter, tc.why)
			}
		})
	}
}

// ── 集成级：真实渲染 + 真实 HitTest 的菜单 hover 保持打开 ────────────────

// menuHoverHTML 复刻 PairCode IDE 顶栏「帮助」菜单的真实结构与样式：
// .menu-dropdown 为 position:fixed; z-index:9999，且位于 .titlebar(z-index:100)
// 之内（绘制上被提升到根层叠上下文），面板自带 4px padding —— 鼠标从面板
// 边缘进入时先命中面板自身，再移入 .menu-item。这正是用户实测的「移到菜单项
// 上菜单自消失」路径。菜单只在收到 mouseleave 时启动 200ms 关闭定时器，
// 故「未派 mouseleave」等价于「菜单保持打开」。
const menuHoverHTML = `<!DOCTYPE html><html><head><style>
html,body{margin:0;padding:0;font-family:sans-serif;font-size:13px}
#titlebar{position:relative;z-index:100;height:40px;background:#111;color:#ddd}
#menubar{display:flex;height:40px;align-items:center}
#help-btn{height:24px;padding:0 8px}
#menu{position:fixed;left:8px;top:28px;width:220px;background:#fff;color:#000;padding:4px;z-index:9999}
.menu-item{display:block;padding:6px 10px;font-size:12px}
#content{height:600px;background:#eee}
#under{height:600px;background:#ddd}
</style></head><body>
<div id="titlebar"><div id="menubar"><button id="help-btn">帮助</button>
  <div id="menu"><div class="menu-item" id="mi1">常见问题</div><div class="menu-item" id="mi2">快速开始</div></div>
</div></div>
<div id="content"><div id="under">under</div></div>
<script>
window.__log = [];
var menu = document.getElementById('menu');
menu.addEventListener('mouseleave', function(){ window.__log.push('menu-leave'); });
menu.addEventListener('mouseenter', function(){ window.__log.push('menu-enter'); });
document.getElementById('mi1').addEventListener('click', function(){ window.__log.push('click-mi1'); });
</script>
</body></html>`

func TestMenuDropdownHoverStaysOpen(t *testing.T) {
	if graphics.GetFontManager() == nil {
		_ = graphics.InitFontManager("")
		if mgr := graphics.GetFontManager(); mgr != nil {
			mgr.LoadSystemFonts()
		}
	}
	layout.MeasureTextFunc = func(family string, size float64, weight int, style2, text string) float64 {
		return graphics.MeasureText(graphics.Font{Family: family, Size: size, Weight: weight, Style: style2}, text)
	}
	wv := webkit.NewWebView()
	defer wv.Destroy()
	wv.Resize(1400, 900)
	if err := wv.LoadHTML(menuHoverHTML); err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	settle := func() {
		for i := 0; i < 6; i++ {
			wv.EnsureLayout()
			if _, err := wv.Render(); err != nil {
				t.Fatalf("Render: %v", err)
			}
		}
	}
	settle()
	rv := wv.RenderView()
	if rv == nil {
		t.Fatal("RenderView = nil")
	}
	h := NewHostForTest(wv, 1400, 900)

	// 指针移动 = 真实 hover 路径（app/host.go processEvents 同构）：
	// HitTest 解析目标 → SetHovered → dispatchHoverEvents 派发。
	move := func(x, y float64) *dom.Element {
		newEl := rendering.HitTest(rv, x, y, "")
		if newEl != h.hoveredEl {
			old := h.hoveredEl
			if old != nil {
				old.SetHovered(false)
			}
			if newEl != nil {
				newEl.SetHovered(true)
			}
			h.hoveredEl = newEl
			h.dispatchHoverEvents(old, newEl, x, y)
		}
		return newEl
	}
	rectOf := func(id string) (x, y, w, h float64) {
		v, err := wv.EvalJS(`(function(){var r=document.getElementById("` + id + `").getBoundingClientRect();return r.left+","+r.top+","+r.width+","+r.height;})()`)
		if err != nil {
			t.Fatalf("EvalJS rect(%s): %v", id, err)
		}
		parts := strings.Split(v.ToString(), ",")
		if len(parts) != 4 {
			t.Fatalf("rect(%s) = %q", id, v.ToString())
		}
		f := make([]float64, 4)
		for i, p := range parts {
			f[i], _ = strconv.ParseFloat(p, 64)
		}
		return f[0], f[1], f[2], f[3]
	}
	log := func() string {
		v, err := wv.EvalJS(`window.__log.join(",")`)
		if err != nil {
			t.Fatalf("EvalJS log: %v", err)
		}
		return v.ToString()
	}
	hitID := func(el *dom.Element) string {
		if el == nil {
			return "<nil>"
		}
		if id := el.GetAttribute("id"); id != "" {
			return id
		}
		return el.LocalName() + "." + el.ClassName()
	}

	mx, my, mw, mh := rectOf("menu")
	if mw <= 8 || mh <= 8 {
		t.Fatalf("菜单几何异常: %.1f,%.1f %.1fx%.1f", mx, my, mw, mh)
	}
	i1x, i1y, i1w, i1h := rectOf("mi1")
	bx, by, _, _ := rectOf("help-btn")
	t.Logf("geometry: menu=(%.1f,%.1f %.1fx%.1f) mi1=(%.1f,%.1f %.1fx%.1f) btn=(%.1f,%.1f)",
		mx, my, mw, mh, i1x, i1y, i1w, i1h, bx, by)

	// ① 命中测试与绘制层叠一致：面板 padding 命中面板自身、菜单项区域命中菜单项。
	padEl := move(mx+2, my+2)
	t.Logf("hit panel-padding(%.1f,%.1f) = %s", mx+2, my+2, hitID(padEl))
	if got := hitID(padEl); got != "menu" {
		t.Errorf("面板 padding 命中 = %q, want \"menu\"（fixed+z-index 层必须优先命中）", got)
	}
	itemEl := move(i1x+i1w/2, i1y+i1h/2)
	t.Logf("hit menu-item(%.1f,%.1f) = %s", i1x+i1w/2, i1y+i1h/2, hitID(itemEl))
	if got := hitID(itemEl); got != "mi1" {
		t.Errorf("菜单项区域命中 = %q, want \"mi1\"", got)
	}
	// ② 进入面板与进入菜单项都不得给面板派 mouseleave（菜单保持打开）。
	l := log()
	t.Logf("hover 面板 padding → 菜单项后 log = %q", l)
	if !strings.Contains(l, "menu-enter") {
		t.Errorf("面板未收到 mouseenter（进入链未沿祖先派发）：log=%q", l)
	}
	if strings.Contains(l, "menu-leave") {
		t.Errorf("鼠标仍在菜单内却派发了 mouseleave（菜单会 200ms 后自关闭）：log=%q", l)
	}
	// ③ 指针真正移出菜单（到内容区）时才必须派 mouseleave。
	if el := move(700, 500); hitID(el) != "under" {
		t.Errorf("菜单外命中 = %q, want \"under\"", hitID(el))
	}
	l = log()
	t.Logf("移出菜单后 log = %q", l)
	if !strings.Contains(l, "menu-leave") {
		t.Errorf("指针离开菜单未派 mouseleave：log=%q", l)
	}
	// ④ 真实点击菜单项仍落到菜单项（不穿透）。
	wv.HandleMouseButton(i1x+i1w/2, i1y+i1h/2, 0, 0)
	wv.HandleMouseButton(i1x+i1w/2, i1y+i1h/2, 0, 1)
	settle()
	t.Logf("点击菜单项后 hovered=%s log=%q", hitID(h.hoveredEl), log())
}
