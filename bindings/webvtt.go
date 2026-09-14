// Package bindings — WebVTT 文本轨道解析与 data: URL 解码。
//
// 覆盖范围（HTML §4.8.11 + WebVTT 标准）：
//   - 头部行 WEBVTT（可带说明文字）
//   - cue 块：可选标识行 + 时间行（start --> end settings）+ 正文（可多行）
//   - 时间戳：HH:MM:SS.mmm / MM:SS.mmm（毫秒分隔符 "." 或 ","，与浏览器一致）
//   - settings 原样保留在 map 里（line/size/align/position/vertical/region）
//   - NOTE 注释块（跳过）
//   - STYLE 块（CSS 文本收集到 vttDocument.Styles；规范要求忽略其渲染影响）
//   - REGION 块（id/width/lines/regionanchor/viewportanchor/scroll）
//   - cue 正文的内嵌标记：<b>/<i>/<u>/<ruby>/<rt>、<c.class1.class2>、
//     <v Speaker>、<lang en>（映射为带 class/title/lang 的 span）、时间戳标签、
//     实体（&amp; &lt; &gt; &lrm; &rlm; &nbsp; 与数字实体）
//
// 标记解析在 getCueAsHTML() 时按需进行；cue.text 按规范保留**原始**标记文本。
package bindings

import (
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"
)

// vttCue 是一条解析后的提示。
type vttCue struct {
	ID       string
	Start    float64
	End      float64
	Settings map[string]string
	Text     string
}

// vttRegion 是一条 REGION 块（WebVTT §4.4）；零值即规范默认值。
type vttRegion struct {
	ID              string
	Width           float64
	Lines           float64
	RegionAnchorX   float64
	RegionAnchorY   float64
	ViewportAnchorX float64
	ViewportAnchorY float64
	Scroll          string
}

// vttDocument 是一次解析的完整结果（cue + 区域 + 样式块）。
type vttDocument struct {
	Cues    []vttCue
	Regions []vttRegion
	Styles  []string
}

// parseWebVTT 解析 WebVTT 文本，返回其中的 cue 列表（按出现顺序）。
// 非法输入（缺 WEBVTT 头、时间行无法解析）的块被跳过，不影响其余 cue。
func parseWebVTT(src string) []vttCue {
	return parseWebVTTDocument(src).Cues
}

// parseWebVTTDocument 解析 WebVTT 文本为完整文档（cue / 区域 / 样式）。
// 非法输入（缺 WEBVTT 头、时间行无法解析）的块被跳过，不影响其余块。
func parseWebVTTDocument(src string) vttDocument {
	// 行尾归一化（解析器给出的属性值是原文，可能含 \r\n）+ BOM 清理。
	src = strings.TrimPrefix(src, "\ufeff")
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")

	lines := strings.Split(src, "\n")
	if len(lines) == 0 || !strings.HasPrefix(strings.TrimSpace(lines[0]), "WEBVTT") {
		return vttDocument{}
	}

	doc := vttDocument{}
	var block []string
	flush := func() {
		if len(block) == 0 {
			return
		}
		// 块分派（WebVTT §6.2）：STYLE / REGION / NOTE / cue。
		head := strings.TrimSpace(block[0])
		switch {
		case head == "STYLE":
			if len(block) > 1 {
				doc.Styles = append(doc.Styles, strings.Join(block[1:], "\n"))
			}
		case head == "REGION":
			if region, ok := parseVTTRegionBlock(block); ok {
				doc.Regions = append(doc.Regions, region)
			}
		case strings.HasPrefix(head, "NOTE"):
			// 注释块（含 NOTE 后的 "-->" 之类的自由文本）：跳过。
		default:
			if cue, ok := parseVTTBlock(block); ok {
				doc.Cues = append(doc.Cues, cue)
			}
		}
		block = block[:0]
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		block = append(block, line)
	}
	flush()
	return doc
}

// parseVTTRegionBlock 解析 REGION 块：
//
//	REGION
//	id:fred
//	width:40%
//	lines:3
//	regionanchor:0%,100%
//	viewportanchor:10%,90%
//	scroll:up
func parseVTTRegionBlock(block []string) (vttRegion, bool) {
	r := vttRegion{
		Width:           100,
		Lines:           3,
		RegionAnchorY:   100,
		ViewportAnchorY: 100,
	}
	hasID := false
	for _, line := range block[1:] {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		switch key {
		case "id":
			r.ID = val
			hasID = val != ""
		case "width":
			if f, ok := parseVTTPercent(val); ok {
				r.Width = f
			}
		case "lines":
			if n, err := strconv.Atoi(val); err == nil && n >= 0 {
				r.Lines = float64(n)
			}
		case "regionanchor":
			if x, y, ok := parseVTTPercentPair(val); ok {
				r.RegionAnchorX, r.RegionAnchorY = x, y
			}
		case "viewportanchor":
			if x, y, ok := parseVTTPercentPair(val); ok {
				r.ViewportAnchorX, r.ViewportAnchorY = x, y
			}
		case "scroll":
			r.Scroll = val
		}
	}
	if !hasID {
		return vttRegion{}, false
	}
	return r, true
}

// parseVTTPercent 解析 "40%" / "40" 形式的百分比数值。
func parseVTTPercent(s string) (float64, bool) {
	s = strings.TrimSuffix(strings.TrimSpace(s), "%")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// parseVTTPercentPair 解析 "10%,90%" 形式的锚点对。
func parseVTTPercentPair(s string) (float64, float64, bool) {
	x, y, ok := strings.Cut(s, ",")
	if !ok {
		return 0, 0, false
	}
	fx, ok1 := parseVTTPercent(x)
	fy, ok2 := parseVTTPercent(y)
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return fx, fy, true
}

// parseVTTBlock 解析单个 cue 块。块形如：
//
//	[cue identifier]
//	00:00:01.000 --> 00:00:03.000 [settings]
//	text line 1
//	text line 2
func parseVTTBlock(block []string) (vttCue, bool) {
	idx := 0
	cue := vttCue{}
	// 时间行可能直接是首行；否则首行是标识行。
	if !strings.Contains(block[0], "-->") {
		cue.ID = strings.TrimSpace(block[0])
		idx = 1
	}
	if idx >= len(block) {
		return vttCue{}, false
	}
	startStr, endStr, settings, ok := splitVTTTiming(block[idx])
	if !ok {
		return vttCue{}, false
	}
	start, ok1 := parseVTTTimestamp(startStr)
	end, ok2 := parseVTTTimestamp(endStr)
	if !ok1 || !ok2 {
		return vttCue{}, false
	}
	cue.Start, cue.End = start, end
	cue.Settings = settings
	// 正文：时间行之后的所有行（保留内嵌换行）。
	if idx+1 < len(block) {
		cue.Text = strings.Join(block[idx+1:], "\n")
	}
	return cue, true
}

// splitVTTTiming 拆分时间行 "start --> end [key:value ...]"。
func splitVTTTiming(line string) (string, string, map[string]string, bool) {
	parts := strings.SplitN(line, "-->", 2)
	if len(parts) != 2 {
		return "", "", nil, false
	}
	start := strings.TrimSpace(parts[0])
	rest := strings.TrimSpace(parts[1])
	end := rest
	settings := map[string]string{}
	if i := strings.IndexAny(rest, " \t"); i >= 0 {
		end = rest[:i]
		for _, field := range strings.Fields(rest[i:]) {
			k, v, found := strings.Cut(field, ":")
			if !found {
				continue
			}
			settings[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return start, end, settings, true
}

// parseVTTTimestamp 解析 "HH:MM:SS.mmm" / "MM:SS.mmm"（毫秒分隔符 "." 或 ","）。
func parseVTTTimestamp(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	s = strings.Replace(s, ",", ".", 1)
	// 毫秒（小数部分）与 hh:mm:ss 部分分开处理。
	frac := 0.0
	if i := strings.LastIndex(s, "."); i >= 0 {
		ms, err := strconv.ParseFloat("0"+s[i:], 64)
		if err != nil {
			return 0, false
		}
		frac = ms
		s = s[:i]
	}
	fields := strings.Split(s, ":")
	if len(fields) == 0 || len(fields) > 3 {
		return 0, false
	}
	total := 0.0
	for _, f := range fields {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || n < 0 {
			return 0, false
		}
		total = total*60 + float64(n)
	}
	return total + frac, true
}

// ─── cue 正文的内嵌标记（getCueAsHTML 用）────────────────

// vttNode 是 cue 正文解析后的一棵节点树：Name 为空表示文本节点；否则是标签
// 节点（Name 是原始标签名，Tag 是映射到的 DOM 标签，Attrs 是 class/title/lang）。
type vttNode struct {
	Name     string
	Tag      string
	Attrs    map[string]string
	Children []*vttNode
	Text     string
}

// vttEntities 是 WebVTT 允许的具名实体（其余实体原样保留）。
var vttEntities = map[string]string{
	"amp":  "&",
	"lt":   "<",
	"gt":   ">",
	"lrm":  "\u200e",
	"rlm":  "\u200f",
	"nbsp": "\u00a0",
}

// parseVTTCueMarkup 把 cue 正文解析成节点树（WebVTT §6.4 的 cue text 到 DOM
// 的映射）：
//
//	<b>/<i>/<u>/<ruby>/<rt> → 同名元素
//	<c.foo.bar>             → <span class="foo bar">
//	<v Speaker>             → <span title="Speaker">
//	<lang en>               → <span lang="en">
//	时间戳标签与未识别标签     → 丢弃标签本身（内容保留）
//	& 实体                  → 解码（&amp; &lt; &gt; &lrm; &rlm; &nbsp; + 数字实体）
//
// 结束标签关闭最近的同名开始标签；跨层错嵌套按「找不到就忽略」处理（规范行为）。
func parseVTTCueMarkup(text string) []*vttNode {
	root := &vttNode{}
	stack := []*vttNode{root}
	pushText := func(s string) {
		if s == "" {
			return
		}
		top := stack[len(stack)-1]
		decoded := decodeVTTEntities(s)
		// 与被忽略的标签相邻的文本合并成一个文本节点（规范：无法识别的标签
		// 不产生元素、也不切断文本累积）。
		if n := len(top.Children); n > 0 && top.Children[n-1].Name == "" {
			top.Children[n-1].Text += decoded
			return
		}
		top.Children = append(top.Children, &vttNode{Text: decoded})
	}
	for i := 0; i < len(text); {
		if text[i] != '<' {
			next := strings.IndexByte(text[i:], '<')
			if next < 0 {
				pushText(text[i:])
				break
			}
			pushText(text[i : i+next])
			i += next
			continue
		}
		// '<' 开始：找匹配的 '>'（找不到则按字面文本保留）。
		close := strings.IndexByte(text[i:], '>')
		if close < 0 {
			pushText(text[i:])
			break
		}
		raw := text[i+1 : i+close]
		i += close + 1
		if strings.HasPrefix(raw, "/") {
			name := strings.ToLower(strings.TrimSpace(raw[1:]))
			for k := len(stack) - 1; k >= 1; k-- {
				if stack[k].Name == name || stack[k].Tag == name {
					stack = stack[:k]
					break
				}
			}
			continue
		}
		node, ok := parseVTTTag(raw)
		if !ok {
			// 时间戳标签（<00:00:01.000>）等：丢弃标签本身，保留其后的内容。
			continue
		}
		top := stack[len(stack)-1]
		top.Children = append(top.Children, node)
		stack = append(stack, node)
	}
	return root.Children
}

// parseVTTTag 解析一个开始标签（不含尖括号）。返回 ok=false 表示不是合法标签
// （时间戳、乱码）——调用方按「丢弃标签」处理。
func parseVTTTag(raw string) (*vttNode, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, false
	}
	switch trimmed {
	case "b", "i", "u", "ruby", "rt":
		return &vttNode{Name: trimmed, Tag: trimmed}, true
	case "c":
		return &vttNode{Name: "c", Tag: "span"}, true
	case "v":
		return &vttNode{Name: "v", Tag: "span"}, true
	}
	if strings.HasPrefix(trimmed, "c.") {
		var classes []string
		for _, c := range strings.Split(trimmed[2:], ".") {
			if c = strings.TrimSpace(c); c != "" {
				classes = append(classes, c)
			}
		}
		n := &vttNode{Name: "c", Tag: "span"}
		if len(classes) > 0 {
			n.Attrs = map[string]string{"class": strings.Join(classes, " ")}
		}
		return n, true
	}
	if strings.HasPrefix(trimmed, "v ") {
		title := strings.TrimSpace(trimmed[2:])
		n := &vttNode{Name: "v", Tag: "span"}
		if title != "" {
			n.Attrs = map[string]string{"title": title}
		}
		return n, true
	}
	if trimmed == "lang" || strings.HasPrefix(trimmed, "lang ") {
		lang := strings.TrimSpace(strings.TrimPrefix(trimmed, "lang"))
		n := &vttNode{Name: "lang", Tag: "span"}
		if lang != "" {
			n.Attrs = map[string]string{"lang": lang}
		}
		return n, true
	}
	return nil, false
}

// decodeVTTEntities 解码 WebVTT 文本里的字符引用（具名 + 十进制/十六进制）。
// 未知具名实体原样保留（规范行为）。
func decodeVTTEntities(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '&' {
			b.WriteByte(s[i])
			i++
			continue
		}
		end := strings.IndexByte(s[i:], ';')
		if end < 0 || end > 32 {
			b.WriteByte('&')
			i++
			continue
		}
		body := s[i+1 : i+end]
		if strings.HasPrefix(body, "#") {
			numStr := body[1:]
			base := 10
			if len(numStr) > 0 && (numStr[0] == 'x' || numStr[0] == 'X') {
				base = 16
				numStr = numStr[1:]
			}
			if n, err := strconv.ParseInt(numStr, base, 32); err == nil && n > 0 {
				b.WriteRune(rune(n))
				i += end + 1
				continue
			}
			b.WriteByte('&')
			i++
			continue
		}
		if rep, ok := vttEntities[body]; ok {
			b.WriteString(rep)
			i += end + 1
			continue
		}
		b.WriteByte('&')
		i++
	}
	return b.String()
}

// decodeDataURL 解码 data: URL，返回 MIME 类型与内容文本。
// 支持 percent 编码与 ";base64" 两种载荷形式（HTML 解析器给出的属性值是
// 原始文本，百分号编码尚未解码）。
func decodeDataURL(raw string) (mime string, body string, ok bool) {
	if !strings.HasPrefix(raw, "data:") {
		return "", "", false
	}
	rest := raw[len("data:"):]
	comma := strings.Index(rest, ",")
	if comma < 0 {
		return "", "", false
	}
	meta := rest[:comma]
	payload := rest[comma+1:]
	if i := strings.Index(meta, ";"); i >= 0 {
		mime = meta[:i]
	} else {
		mime = meta
	}
	if strings.HasSuffix(meta, ";base64") {
		// data URL 的 base64 载荷可能含 URL 编码的 "+"（%2B）与换行。
		payload = strings.NewReplacer("%2B", "+", "%2b", "+").Replace(payload)
		payload = strings.TrimSpace(payload)
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(payload)
			if err != nil {
				return mime, "", false
			}
		}
		return mime, string(decoded), true
	}
	// percent 解码：PathUnescape 保留 "+" 原义（与 data URL 语义一致，
	// 表单式 QueryUnescape 会把 "+" 变成空格）。
	if decoded, err := url.PathUnescape(payload); err == nil {
		return mime, decoded, true
	}
	return mime, payload, true
}
