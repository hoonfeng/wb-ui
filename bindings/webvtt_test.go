package bindings

import (
	"net/url"
	"strings"
	"testing"

	"wb-ui/dom"
)

// vttSrcWithRegion 构造带 STYLE / REGION / NOTE / 标记 cue 的 WebVTT 文本。
func vttSrcWithRegion() string {
	return strings.Join([]string{
		"WEBVTT",
		"",
		"STYLE",
		"::cue { color: lime }",
		"",
		"REGION",
		"id:r1",
		"width:40%",
		"lines:3",
		"regionanchor:0%,100%",
		"viewportanchor:10%,90%",
		"scroll:up",
		"",
		"NOTE 这是 --> 注释块（不是 cue）",
		"第二行注释",
		"",
		"00:00:01.000 --> 00:00:02.000 region:r1 align:start",
		"hello <b>world</b> &amp; <c.foo.bar>cls</c> <v Bob>speaker</v>",
	}, "\n")
}

// TestWebVTTDocumentBlocks 覆盖块分派：STYLE 块进 Styles、REGION 块进 Regions、
// NOTE 块（含 "-->"）被跳过、cue 正常解析。
func TestWebVTTDocumentBlocks(t *testing.T) {
	doc := parseWebVTTDocument(vttSrcWithRegion())

	if len(doc.Cues) != 1 {
		t.Fatalf("cue 数 = %d，want 1（NOTE 块含 --> 不应被当成 cue）", len(doc.Cues))
	}
	if len(doc.Regions) != 1 {
		t.Fatalf("region 数 = %d，want 1", len(doc.Regions))
	}
	if len(doc.Styles) != 1 || !strings.Contains(doc.Styles[0], "color: lime") {
		t.Fatalf("style 块 = %v", doc.Styles)
	}
	r := doc.Regions[0]
	if r.ID != "r1" || r.Width != 40 || r.Lines != 3 {
		t.Fatalf("region = %+v", r)
	}
	if r.RegionAnchorX != 0 || r.RegionAnchorY != 100 || r.ViewportAnchorX != 10 || r.ViewportAnchorY != 90 {
		t.Fatalf("region 锚点 = %+v", r)
	}
	if r.Scroll != "up" {
		t.Fatalf("region.scroll = %q", r.Scroll)
	}
	cue := doc.Cues[0]
	if cue.Settings["region"] != "r1" || cue.Settings["align"] != "start" {
		t.Fatalf("cue settings = %v", cue.Settings)
	}
	// cue.text 按规范保留原始标记文本
	if !strings.Contains(cue.Text, "<b>world</b>") {
		t.Fatalf("cue.text 应保留原始标记: %q", cue.Text)
	}
	// 旧 API 仍可用（只取 cue）
	if cues := parseWebVTT(vttSrcWithRegion()); len(cues) != 1 {
		t.Fatalf("parseWebVTT 兼容层返回 %d 条 cue", len(cues))
	}
}

// TestVTTCueMarkupParsing 覆盖 cue 正文标记解析：标签映射、时间戳标签丢弃、
// 错嵌套忽略、实体解码、文本保留。
func TestVTTCueMarkupParsing(t *testing.T) {
	nodes := parseVTTCueMarkup("a<b>B</b>c&lt;d&amp;e")
	if len(nodes) != 3 {
		t.Fatalf("节点数 = %d，want 3: %+v", len(nodes), nodes)
	}
	if nodes[0].Name != "" || nodes[0].Text != "a" {
		t.Fatalf("nodes[0] = %+v", nodes[0])
	}
	if nodes[1].Name != "b" || nodes[1].Tag != "b" || len(nodes[1].Children) != 1 ||
		nodes[1].Children[0].Text != "B" {
		t.Fatalf("nodes[1] = %+v", nodes[1])
	}
	if nodes[2].Text != "c<d&e" {
		t.Fatalf("实体解码失败: %q", nodes[2].Text)
	}
	// 时间戳标签丢弃（不产生节点）
	if got := parseVTTCueMarkup("a<00:00:01.000>b"); len(got) != 1 || got[0].Text != "ab" {
		t.Fatalf("时间戳标签处理 = %+v", got)
	}
	// 未识别的标签（乱码）丢弃标签、保留内容
	if got := parseVTTCueMarkup("x<foo>y</foo>z"); len(got) != 1 || got[0].Text != "xyz" {
		t.Fatalf("未识别标签处理 = %+v", got)
	}
	// 未闭合标签：内容保留在标签节点内
	if got := parseVTTCueMarkup("<b>tail"); len(got) != 1 || got[0].Name != "b" ||
		got[0].Children[0].Text != "tail" {
		t.Fatalf("未闭合标签处理 = %+v", got)
	}
	// 找不到匹配开始标签的结束标签：忽略
	if got := parseVTTCueMarkup("a</b>b"); len(got) != 1 || got[0].Text != "ab" {
		t.Fatalf("孤立结束标签处理 = %+v", got)
	}
}

// TestVTTCueMarkupTagMapping 覆盖 c/v/lang/ruby 的映射（class/title/lang）。
func TestVTTCueMarkupTagMapping(t *testing.T) {
	nodes := parseVTTCueMarkup(`<c.yellow.bg_black>c</c><v Ann>v</v><lang en-US>en</lang><ruby>漢<rt>kan</rt></ruby>`)
	if len(nodes) != 4 {
		t.Fatalf("节点数 = %d，want 4", len(nodes))
	}
	if nodes[0].Tag != "span" || nodes[0].Attrs["class"] != "yellow bg_black" {
		t.Fatalf("c 映射 = %+v", nodes[0])
	}
	if nodes[1].Tag != "span" || nodes[1].Attrs["title"] != "Ann" {
		t.Fatalf("v 映射 = %+v", nodes[1])
	}
	if nodes[2].Tag != "span" || nodes[2].Attrs["lang"] != "en-US" {
		t.Fatalf("lang 映射 = %+v", nodes[2])
	}
	if nodes[3].Tag != "ruby" || len(nodes[3].Children) != 2 ||
		nodes[3].Children[1].Tag != "rt" || nodes[3].Children[1].Children[0].Text != "kan" {
		t.Fatalf("ruby 映射 = %+v", nodes[3])
	}
}

// TestCueFragmentStructure 覆盖 getCueAsHTML 的 DOM 结构（Go 侧直接检查）：
// 文本节点 + 元素节点（span 带 class/title）、多行保留为文本里的换行。
func TestCueFragmentStructure(t *testing.T) {
	_, doc, _ := newRuntimeWithDoc(t)

	frag := buildCueFragment(doc, parseVTTCueMarkup("line1\nhello <b>world</b> <c.cls>c</c>"))
	kids := frag.ChildNodes()
	if len(kids) != 4 {
		t.Fatalf("fragment 子节点数 = %d，want 4", len(kids))
	}
	if kids[0].TextContent() != "line1\nhello " {
		t.Fatalf("kids[0] = %q", kids[0].TextContent())
	}
	b, ok := kids[1].(*dom.Element)
	if !ok || b.LocalName() != "b" || b.TextContent() != "world" {
		t.Fatalf("kids[1] = %v", kids[1])
	}
	span, ok := kids[3].(*dom.Element)
	if !ok || span.LocalName() != "span" || span.GetAttribute("class") != "cls" {
		t.Fatalf("kids[3] = %v", kids[3])
	}
	if span.TextContent() != "c" {
		t.Fatalf("span 内容 = %q", span.TextContent())
	}
}

// TestVTTCueGetCueAsHTML 覆盖 JS 侧 getCueAsHTML()/toString()：返回
// DocumentFragment（nodeType=11），cue.text 与 toString 保留原始标记。
func TestVTTCueGetCueAsHTML(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	video := doc.CreateElement("video")
	track := doc.CreateElement("track")
	track.SetId("captions")
	track.SetAttribute("src", "data:text/vtt,"+url.PathEscape(vttSrcWithRegion()))
	doc.AppendChild(video)
	video.AppendChild(track)

	mustRun(t, rt, `
		{
		const element = document.getElementById("captions");
		void element.track;
		const cue = element.track.cues[0];
		const frag = cue.getCueAsHTML();
		globalThis.__html = {
			nodeType: frag.nodeType,
			nodeName: frag.nodeName,
			childCount: frag.childNodes.length,
			firstIsText: frag.childNodes[0].nodeType === 3,
			hasB: frag.childNodes[1] && frag.childNodes[1].tagName === "B",
			cueText: cue.text,
			asString: String(cue),
			instanceofFragment: frag instanceof DocumentFragment
		};
		}
	`)
	mustRun(t, rt, `
		{
		const h = globalThis.__html;
		if (h.nodeType !== 11) throw new Error("getCueAsHTML 应返回 DocumentFragment，nodeType=" + h.nodeType);
		if (!h.instanceofFragment) throw new Error("不是 DocumentFragment 实例");
		if (h.childCount !== 6) throw new Error("fragment 子节点数 = " + h.childCount);
		if (!h.firstIsText) throw new Error("第一个子节点应是文本");
		if (!h.hasB) throw new Error("第二个子节点应是 <b>（tagName=B）");
		if (h.cueText.indexOf("<b>world</b>") < 0) throw new Error("cue.text 应保留原始标记: " + h.cueText);
		if (h.asString !== h.cueText) throw new Error("toString 应返回 cue.text");
		}
	`)
}

// TestVTTCueRegionBinding 覆盖 cue.region：settings 里的 region:<id> 绑定到
// REGION 块声明的 VTTRegion 对象（属性齐全），未声明的 id 保持 null。
func TestVTTCueRegionBinding(t *testing.T) {
	rt, doc, _ := newRuntimeWithDoc(t)
	video := doc.CreateElement("video")
	track := doc.CreateElement("track")
	track.SetId("captions")
	track.SetAttribute("src", "data:text/vtt,"+url.PathEscape(vttSrcWithRegion()))
	doc.AppendChild(video)
	video.AppendChild(track)

	mustRun(t, rt, `
		{
		const element = document.getElementById("captions");
		void element.track;
		const cue = element.track.cues[0];
		const r = cue.region;
		globalThis.__region = {
			isRegion: r instanceof VTTRegion,
			id: r.id,
			width: r.width,
			lines: r.lines,
			viewportAnchorX: r.viewportAnchorX,
			viewportAnchorY: r.viewportAnchorY,
			scroll: r.scroll,
			align: cue.align
		};
		const fresh = new VTTRegion();
		globalThis.__regionDefaults = [fresh.width, fresh.lines, fresh.scroll, fresh.id];
		}
	`)
	mustRun(t, rt, `
		{
		const r = globalThis.__region;
		if (!r.isRegion) throw new Error("cue.region 不是 VTTRegion");
		if (r.id !== "r1") throw new Error("region.id = " + r.id);
		if (r.width !== 40 || r.lines !== 3) throw new Error("region 尺寸 = " + r.width + "x" + r.lines);
		if (r.viewportAnchorX !== 10 || r.viewportAnchorY !== 90) throw new Error("viewportAnchor 未生效");
		if (r.scroll !== "up") throw new Error("scroll = " + r.scroll);
		if (r.align !== "start") throw new Error("cue.align = " + r.align);
		const d = globalThis.__regionDefaults;
		if (d[0] !== 100 || d[1] !== 3 || d[2] !== "" || d[3] !== "") {
			throw new Error("new VTTRegion() 默认值不对: " + JSON.stringify(d));
		}
		}
	`)
}

// TestVTTUnknownRegionSetting 覆盖未声明 region id：规范要求 cue.region 为 null
// （不给一个指向不存在区域的假对象）。
func TestVTTUnknownRegionSetting(t *testing.T) {
	src := "WEBVTT\n\n00:00:01.000 --> 00:00:02.000 region:nope\nhi"
	rt, doc, _ := newRuntimeWithDoc(t)
	video := doc.CreateElement("video")
	track := doc.CreateElement("track")
	track.SetId("captions")
	track.SetAttribute("src", "data:text/vtt,"+url.PathEscape(src))
	doc.AppendChild(video)
	video.AppendChild(track)

	mustRun(t, rt, `
		{
		const element = document.getElementById("captions");
		void element.track;
		const cue = element.track.cues[0];
		if (cue.region !== null) throw new Error("未声明 region 应为 null，实际 " + cue.region);
		if (element.track.cues.length !== 1) throw new Error("cue 数 = " + element.track.cues.length);
		}
	`)
}

// TestDecodeVTTEntities 覆盖字符引用：具名实体、十进制/十六进制数字实体、
// 未知实体原样保留、未闭合的 "&" 原样保留。
func TestDecodeVTTEntities(t *testing.T) {
	cases := map[string]string{
		"a&amp;b":     "a&b",
		"a&lt;b&gt;c": "a<b>c",
		"x&nbsp;y":    "x\u00a0y",
		"r&lrm;l":     "r\u200el",
		"r&rlm;l":     "r\u200fl",
		"&#65;&#x42;": "AB",
		"&unknown;":   "&unknown;",
		"a & b":       "a & b",
		"&amp":        "&amp",
		"&nbsp;&amp;": "\u00a0&",
	}
	for in, want := range cases {
		if got := decodeVTTEntities(in); got != want {
			t.Errorf("decodeVTTEntities(%q) = %q，want %q", in, got, want)
		}
	}
}
