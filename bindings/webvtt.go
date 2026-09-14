// Package bindings — WebVTT 文本轨道解析（最小可用子集）与 data: URL 解码。
//
// 覆盖范围（HTML §4.8.11 + WebVTT 标准）：
//   - 头部行 WEBVTT（可带说明文字）
//   - cue 块：可选标识行 + 时间行（start --> end settings）+ 正文（可多行）
//   - 时间戳：HH:MM:SS.mmm / MM:SS.mmm（毫秒分隔符 "." 或 ","，与浏览器一致）
//   - settings 原样保留在 map 里（line/size/align/position/vertical/region）
//
// 未实现（在本引擎场景中不被依赖）：STYLE/REGION 块、NOTE 之外的注释语义、
// 时间戳简写（"1:02.500" 之外的形式）、cue 内嵌标记（<b>/<c> 等）的语义解析
// ——正文按纯文本保留，标签原样出现在文本里。
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

// parseWebVTT 解析 WebVTT 文本，返回其中的 cue 列表（按出现顺序）。
// 非法输入（缺 WEBVTT 头、时间行无法解析）的块被跳过，不影响其余 cue。
func parseWebVTT(src string) []vttCue {
	// 行尾归一化（解析器给出的属性值是原文，可能含 \r\n）+ BOM 清理。
	src = strings.TrimPrefix(src, "\ufeff")
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")

	lines := strings.Split(src, "\n")
	if len(lines) == 0 || !strings.HasPrefix(strings.TrimSpace(lines[0]), "WEBVTT") {
		return nil
	}

	var cues []vttCue
	var block []string
	flush := func() {
		if len(block) == 0 {
			return
		}
		if cue, ok := parseVTTBlock(block); ok {
			cues = append(cues, cue)
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
	return cues
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
