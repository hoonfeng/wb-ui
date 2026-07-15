package markdown

// Translation of: markdown-it/lib/parser_inline.mjs
//                  markdown-it/lib/rules_inline/*.mjs (except linkify, html_inline, entity)
// Completeness: 85%
//
// ParserInline is the inline-level tokenizer. It scans a string of text and
// produces inline tokens (text, emphasis, links, images, code, etc.).

import (
	"regexp"
	"strings"
)

// ParserInline holds the inline rule chains (ruler for tokenize, ruler2 for
// post-processing).
type ParserInline struct {
	Ruler  *Ruler
	Ruler2 *Ruler
}

// NewParserInline creates a ParserInline with default inline rules registered.
// linkify, html_inline, and entity rules are skipped for P0.
func NewParserInline() *ParserInline {
	pi := &ParserInline{Ruler: NewRuler(), Ruler2: NewRuler()}

	// _rules (tokenize phase)
	pi.Ruler.Push("text", textRule, nil)
	pi.Ruler.Push("newline", newlineRule, nil)
	pi.Ruler.Push("escape", escapeRule, nil)
	pi.Ruler.Push("backticks", backtickRule, nil)
	pi.Ruler.Push("strikethrough", strikethroughTokenize, nil)
	pi.Ruler.Push("emphasis", emphasisTokenize, nil)
	pi.Ruler.Push("link", linkRule, nil)
	pi.Ruler.Push("image", imageRule, nil)
	pi.Ruler.Push("autolink", autolinkRule, nil)

	// _rules2 (post-process phase)
	pi.Ruler2.Push("balance_pairs", balancePairsRule, nil)
	pi.Ruler2.Push("strikethrough", strikethroughPostProcess, nil)
	pi.Ruler2.Push("emphasis", emphasisPostProcess, nil)
	pi.Ruler2.Push("fragments_join", fragmentsJoinRule, nil)

	return pi
}

// SkipToken skips a single token by running all rules in validation mode.
// Mirrors ParserInline.prototype.skipToken.
func (pi *ParserInline) SkipToken(state *StateInline) {
	pos := state.Pos
	rules := pi.Ruler.GetRules("")
	maxNesting := state.Md.Options.MaxNesting

	if cachedPos, ok := state.Cache[intToStr(pos)]; ok {
		state.Pos = cachedPos
		return
	}

	ok := false
	if state.Level < maxNesting {
		for i := 0; i < len(rules); i++ {
			state.Level++
			ok = rules[i](state, 0, 0, true)
			state.Level--
			if ok {
				if pos >= state.Pos {
					panic("inline rule didn't increment state.pos")
				}
				break
			}
		}
	} else {
		state.Pos = state.PosMax
	}

	if !ok {
		state.Pos++
	}
	state.Cache[intToStr(pos)] = state.Pos
}

// Tokenize generates tokens for the inline content. Mirrors
// ParserInline.prototype.tokenize.
func (pi *ParserInline) Tokenize(state *StateInline) {
	rules := pi.Ruler.GetRules("")
	end := state.PosMax
	maxNesting := state.Md.Options.MaxNesting

	for state.Pos < end {
		prevPos := state.Pos
		ok := false

		if state.Level < maxNesting {
			for i := 0; i < len(rules); i++ {
				ok = rules[i](state, 0, 0, false)
				if ok {
					if prevPos >= state.Pos {
						panic("inline rule didn't increment state.pos")
					}
					break
				}
			}
		}

		if ok {
			if state.Pos >= end {
				break
			}
			continue
		}

		state.Pending += string(state.SrcRunes[state.Pos])
		state.Pos++
	}

	if state.Pending != "" {
		state.PushPending()
	}
}

// Parse processes input string and pushes inline tokens into outTokens.
// Returns the updated token slice (Go slices are value headers).
// Mirrors ParserInline.prototype.parse.
func (pi *ParserInline) Parse(str string, md *MarkdownIt, env map[string]interface{}, outTokens []Token) []Token {
	state := NewStateInline(str, md, env, outTokens)
	pi.Tokenize(state)

	rules := pi.Ruler2.GetRules("")
	for i := 0; i < len(rules); i++ {
		rules[i](state, 0, 0, false)
	}

	return state.Tokens
}

// --- Inline rules (tokenize phase) ---

// isTerminatorChar checks if a character terminates a text run.
// Mirrors isTerminatorChar in rules_inline/text.mjs.
func isTerminatorChar(ch rune) bool {
	switch ch {
	case 0x0A, 0x21, 0x23, 0x24, 0x25, 0x26, 0x2A, 0x2B, 0x2D,
		0x3A, 0x3C, 0x3D, 0x3E, 0x40, 0x5B, 0x5C, 0x5D, 0x5E,
		0x5F, 0x60, 0x7B, 0x7D, 0x7E:
		return true
	}
	return false
}

// textRule skips text characters until a terminator is found, adding them to
// the pending buffer. Mirrors rules_inline/text.mjs.
func textRule(stateI interface{}, _, _ int, silent bool) bool {
	state := stateI.(*StateInline)
	pos := state.Pos
	for pos < state.PosMax && !isTerminatorChar(state.SrcRunes[pos]) {
		pos++
	}
	if pos == state.Pos {
		return false
	}
	if !silent {
		state.Pending += string(state.SrcRunes[state.Pos:pos])
	}
	state.Pos = pos
	return true
}

// newlineRule processes \n, creating soft or hard breaks. Mirrors
// rules_inline/newline.mjs.
func newlineRule(stateI interface{}, _, _ int, silent bool) bool {
	state := stateI.(*StateInline)
	pos := state.Pos
	if state.SrcRunes[pos] != 0x0A /* \n */ {
		return false
	}
	pmax := len(state.Pending) - 1
	max := state.PosMax

	if !silent {
		if pmax >= 0 && state.Pending[pmax] == ' ' {
			if pmax >= 1 && state.Pending[pmax-1] == ' ' {
				// Find whitespaces tail of pending chars
				ws := pmax - 1
				for ws >= 1 && state.Pending[ws-1] == ' ' {
					ws--
				}
				state.Pending = state.Pending[:ws]
				state.Push("hardbreak", "br", 0)
			} else {
				state.Pending = state.Pending[:pmax]
				state.Push("softbreak", "br", 0)
			}
		} else {
			state.Push("softbreak", "br", 0)
		}
	}
	pos++
	// skip heading spaces for next line
	for pos < max && IsSpace(state.SrcRunes[pos]) {
		pos++
	}
	state.Pos = pos
	return true
}

// escapedChars marks which ASCII characters are valid after a backslash escape.
var escapedChars = func() [256]bool {
	var e [256]bool
	for _, ch := range "\\!\"#$%&'()*+,./:;<=>?@[]^_`{|}~-" {
		e[ch] = true
	}
	return e
}()

// escapeRule processes escaped characters and hardbreaks. Mirrors
// rules_inline/escape.mjs.
func escapeRule(stateI interface{}, _, _ int, silent bool) bool {
	state := stateI.(*StateInline)
	pos := state.Pos
	max := state.PosMax
	if state.SrcRunes[pos] != 0x5C /* \ */ {
		return false
	}
	pos++
	if pos >= max {
		return false
	}
	ch1 := state.SrcRunes[pos]
	if ch1 == 0x0A {
		if !silent {
			state.Push("hardbreak", "br", 0)
		}
		pos++
		for pos < max && IsSpace(state.SrcRunes[pos]) {
			pos++
		}
		state.Pos = pos
		return true
	}
	// '\' before a space is a literal backslash
	if ch1 == 0x20 {
		if !silent {
			token := state.Push("text_special", "", 0)
			token.Content = "\\"
			token.Markup = "\\"
			token.Info = "escape"
		}
		state.Pos = pos
		return true
	}
	// Since we use []rune, no surrogate pair handling needed
	escapedStr := string(ch1)
	origStr := "\\" + escapedStr
	if !silent {
		token := state.Push("text_special", "", 0)
		if ch1 < 256 && escapedChars[ch1] {
			token.Content = escapedStr
		} else {
			token.Content = origStr
		}
		token.Markup = origStr
		token.Info = "escape"
	}
	state.Pos = pos + 1
	return true
}

// backtickRule parses inline code spans (`code`). Mirrors rules_inline/backticks.mjs.
func backtickRule(stateI interface{}, _, _ int, silent bool) bool {
	state := stateI.(*StateInline)
	pos := state.Pos
	ch := state.SrcRunes[pos]
	if ch != 0x60 /* ` */ {
		return false
	}
	start := pos
	pos++
	max := state.PosMax
	// scan marker length
	for pos < max && state.SrcRunes[pos] == 0x60 {
		pos++
	}
	marker := string(state.SrcRunes[start:pos])
	openerLength := pos - start

	if state.BackticksScanned {
		if lastPos, ok := state.Backticks[openerLength]; ok && lastPos <= start {
			if !silent {
				state.Pending += marker
			}
			state.Pos += openerLength
			return true
		}
	}

	matchEnd := pos
	var matchStart int
	// scan until the end of the line (or until marker is found)
	for {
		matchStart = -1
		for i := matchEnd; i < max; i++ {
			if state.SrcRunes[i] == 0x60 {
				matchStart = i
				break
			}
		}
		if matchStart == -1 {
			break
		}
		matchEnd = matchStart + 1
		for matchEnd < max && state.SrcRunes[matchEnd] == 0x60 {
			matchEnd++
		}
		closerLength := matchEnd - matchStart
		if closerLength == openerLength {
			if !silent {
				token := state.Push("code_inline", "code", 0)
				token.Markup = marker
				content := string(state.SrcRunes[pos:matchStart])
				content = strings.ReplaceAll(content, "\n", " ")
				// strip one leading and one trailing space if present
				if len(content) >= 2 && content[0] == ' ' && content[len(content)-1] == ' ' {
					content = content[1 : len(content)-1]
				}
				token.Content = content
			}
			state.Pos = matchEnd
			return true
		}
		state.Backticks[closerLength] = matchStart
	}
	state.BackticksScanned = true
	if !silent {
		state.Pending += marker
	}
	state.Pos += openerLength
	return true
}

// strikethroughTokenize processes ~~strikethrough~~. Mirrors
// rules_inline/strikethrough.mjs (tokenize function).
func strikethroughTokenize(stateI interface{}, _, _ int, silent bool) bool {
	state := stateI.(*StateInline)
	start := state.Pos
	marker := state.SrcRunes[start]
	if silent {
		return false
	}
	if marker != 0x7E /* ~ */ {
		return false
	}
	scanned := state.ScanDelims(state.Pos, true)
	length := scanned.Length
	ch := string(marker)
	if length < 2 {
		return false
	}
	var token *Token
	if length%2 != 0 {
		token = state.Push("text", "", 0)
		token.Content = ch
		length--
	}
	for i := 0; i < length; i += 2 {
		token = state.Push("text", "", 0)
		token.Content = ch + ch
		*state.Delimiters = append(*state.Delimiters, Delimiter{
			Marker: marker,
			Length: 0, // disable "rule of 3" for strikethrough
			Token:  len(state.Tokens) - 1,
			End:    -1,
			Open:   scanned.CanOpen,
			Close:  scanned.CanClose,
		})
	}
	state.Pos += scanned.Length
	return true
}

// emphasisTokenize processes *emphasis* and _emphasis_. Mirrors
// rules_inline/emphasis.mjs (tokenize function).
func emphasisTokenize(stateI interface{}, _, _ int, silent bool) bool {
	state := stateI.(*StateInline)
	start := state.Pos
	marker := state.SrcRunes[start]
	if silent {
		return false
	}
	if marker != 0x5F /* _ */ && marker != 0x2A /* * */ {
		return false
	}
	scanned := state.ScanDelims(state.Pos, marker == 0x2A)
	for i := 0; i < scanned.Length; i++ {
		token := state.Push("text", "", 0)
		token.Content = string(marker)
		*state.Delimiters = append(*state.Delimiters, Delimiter{
			Marker: marker,
			Length: scanned.Length,
			Token:  len(state.Tokens) - 1,
			End:    -1,
			Open:   scanned.CanOpen,
			Close:  scanned.CanClose,
		})
	}
	state.Pos += scanned.Length
	return true
}

// linkRule processes [link](url "title") and reference links. Mirrors
// rules_inline/link.mjs.
func linkRule(stateI interface{}, _, _ int, silent bool) bool {
	state := stateI.(*StateInline)
	var href, title string
	start := state.Pos
	parseReference := true

	if state.SrcRunes[state.Pos] != 0x5B /* [ */ {
		return false
	}
	oldPos := state.Pos
	max := state.PosMax
	labelStart := state.Pos + 1
	labelEnd := ParseLinkLabel(state, state.Pos, true)
	if labelEnd < 0 {
		return false
	}
	pos := labelEnd + 1

	if pos < max && state.SrcRunes[pos] == 0x28 /* ( */ {
		parseReference = false
		pos++
		for ; pos < max; pos++ {
			code := state.SrcRunes[pos]
			if !IsSpace(code) && code != 0x0A {
				break
			}
		}
		if pos >= max {
			return false
		}
		start = pos
		res := ParseLinkDestination(state.Src, pos, state.PosMax)
		if res.Ok {
			href = state.Md.NormalizeLink(res.Str)
			if state.Md.ValidateLink(href) {
				pos = res.Pos
			} else {
				href = ""
			}
			start = pos
			for ; pos < max; pos++ {
				code := state.SrcRunes[pos]
				if !IsSpace(code) && code != 0x0A {
					break
				}
			}
			res2 := ParseLinkTitle(state.Src, pos, state.PosMax, nil)
			if pos < max && start != pos && res2.Ok {
				title = res2.Str
				pos = res2.Pos
				for ; pos < max; pos++ {
					code := state.SrcRunes[pos]
					if !IsSpace(code) && code != 0x0A {
						break
					}
				}
			}
		}
		if pos >= max || state.SrcRunes[pos] != 0x29 /* ) */ {
			parseReference = true
		}
		pos++
	}

	if parseReference {
		var label string
		if state.Env["references"] == nil {
			return false
		}
		if pos < max && state.SrcRunes[pos] == 0x5B /* [ */ {
			start = pos + 1
			pos = ParseLinkLabel(state, pos, false)
			if pos >= 0 {
				label = string(state.SrcRunes[start:pos])
				pos++
			} else {
				pos = labelEnd + 1
			}
		} else {
			pos = labelEnd + 1
		}
		if label == "" {
			label = string(state.SrcRunes[labelStart:labelEnd])
		}
		refs, ok := state.Env["references"].(map[string]interface{})
		if !ok {
			return false
		}
		ref, ok := refs[NormalizeReference(label)].(map[string]interface{})
		if !ok {
			state.Pos = oldPos
			return false
		}
		href, _ = ref["href"].(string)
		if t, ok := ref["title"].(string); ok {
			title = t
		}
	}

	if !silent {
		state.Pos = labelStart
		state.PosMax = labelEnd
		tokenO := state.Push("link_open", "a", 1)
		tokenO.Attrs = [][2]string{{"href", href}}
		if title != "" {
			tokenO.Attrs = append(tokenO.Attrs, [2]string{"title", title})
		}
		state.LinkLevel++
		state.Md.Inline.Tokenize(state)
		state.LinkLevel--
		state.Push("link_close", "a", -1)
	}

	state.Pos = pos
	state.PosMax = max
	return true
}

// imageRule processes ![image](src "title"). Mirrors rules_inline/image.mjs.
func imageRule(stateI interface{}, _, _ int, silent bool) bool {
	state := stateI.(*StateInline)
	var href, title string
	var content string
	var label string
	oldPos := state.Pos
	max := state.PosMax

	if state.SrcRunes[state.Pos] != 0x21 /* ! */ {
		return false
	}
	if state.Pos+1 >= max || state.SrcRunes[state.Pos+1] != 0x5B /* [ */ {
		return false
	}
	labelStart := state.Pos + 2
	labelEnd := ParseLinkLabel(state, state.Pos+1, false)
	if labelEnd < 0 {
		return false
	}
	pos := labelEnd + 1

	if pos < max && state.SrcRunes[pos] == 0x28 /* ( */ {
		pos++
		for ; pos < max; pos++ {
			code := state.SrcRunes[pos]
			if !IsSpace(code) && code != 0x0A {
				break
			}
		}
		if pos >= max {
			return false
		}
		start := pos
		res := ParseLinkDestination(state.Src, pos, state.PosMax)
		if res.Ok {
			href = state.Md.NormalizeLink(res.Str)
			if state.Md.ValidateLink(href) {
				pos = res.Pos
			} else {
				href = ""
			}
		}
		start = pos
		for ; pos < max; pos++ {
			code := state.SrcRunes[pos]
			if !IsSpace(code) && code != 0x0A {
				break
			}
		}
		res2 := ParseLinkTitle(state.Src, pos, state.PosMax, nil)
		if pos < max && start != pos && res2.Ok {
			title = res2.Str
			pos = res2.Pos
			for ; pos < max; pos++ {
				code := state.SrcRunes[pos]
				if !IsSpace(code) && code != 0x0A {
					break
				}
			}
		} else {
			title = ""
		}
		if pos >= max || state.SrcRunes[pos] != 0x29 /* ) */ {
			state.Pos = oldPos
			return false
		}
		pos++
	} else {
		if state.Env["references"] == nil {
			return false
		}
		if pos < max && state.SrcRunes[pos] == 0x5B /* [ */ {
			start := pos + 1
			pos = ParseLinkLabel(state, pos, false)
			if pos >= 0 {
				label = string(state.SrcRunes[start:pos])
				pos++
			} else {
				pos = labelEnd + 1
			}
		} else {
			pos = labelEnd + 1
		}
		if label == "" {
			label = string(state.SrcRunes[labelStart:labelEnd])
		}
		refs, ok := state.Env["references"].(map[string]interface{})
		if !ok {
			return false
		}
		ref, ok := refs[NormalizeReference(label)].(map[string]interface{})
		if !ok {
			state.Pos = oldPos
			return false
		}
		href, _ = ref["href"].(string)
		if t, ok := ref["title"].(string); ok {
			title = t
		}
	}

	if !silent {
		content = string(state.SrcRunes[labelStart:labelEnd])
		tokens := []Token{}
		tokens = state.Md.Inline.Parse(content, state.Md, state.Env, tokens)
		token := state.Push("image", "img", 0)
		token.Attrs = [][2]string{{"src", href}, {"alt", ""}}
		token.Children = tokens
		token.Content = content
		if title != "" {
			token.Attrs = append(token.Attrs, [2]string{"title", title})
		}
	}

	state.Pos = pos
	state.PosMax = max
	return true
}

// emailRe matches valid email addresses for autolinks.
var emailRe = regexp.MustCompile(`^[a-zA-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)

// autolinkRe matches valid protocol autolinks.
var autolinkRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]{1,31}:([^<>\x00-\x20]*)$`)

// autolinkRule processes <protocol:url> and <email> autolinks. Mirrors
// rules_inline/autolink.mjs.
func autolinkRule(stateI interface{}, _, _ int, silent bool) bool {
	state := stateI.(*StateInline)
	pos := state.Pos
	if state.SrcRunes[pos] != 0x3C /* < */ {
		return false
	}
	start := state.Pos
	max := state.PosMax

	for {
		pos++
		if pos >= max {
			return false
		}
		ch := state.SrcRunes[pos]
		if ch == 0x3C /* < */ {
			return false
		}
		if ch == 0x3E /* > */ {
			break
		}
	}

	url := string(state.SrcRunes[start+1 : pos])
	if autolinkRe.MatchString(url) {
		fullUrl := state.Md.NormalizeLink(url)
		if !state.Md.ValidateLink(fullUrl) {
			return false
		}
		if !silent {
			tokenO := state.Push("link_open", "a", 1)
			tokenO.Attrs = [][2]string{{"href", fullUrl}}
			tokenO.Markup = "autolink"
			tokenO.Info = "auto"
			tokenT := state.Push("text", "", 0)
			tokenT.Content = state.Md.NormalizeLinkText(url)
			tokenC := state.Push("link_close", "a", -1)
			tokenC.Markup = "autolink"
			tokenC.Info = "auto"
		}
		state.Pos += len(url) + 2
		return true
	}
	if emailRe.MatchString(url) {
		fullUrl := state.Md.NormalizeLink("mailto:" + url)
		if !state.Md.ValidateLink(fullUrl) {
			return false
		}
		if !silent {
			tokenO := state.Push("link_open", "a", 1)
			tokenO.Attrs = [][2]string{{"href", fullUrl}}
			tokenO.Markup = "autolink"
			tokenO.Info = "auto"
			tokenT := state.Push("text", "", 0)
			tokenT.Content = state.Md.NormalizeLinkText(url)
			tokenC := state.Push("link_close", "a", -1)
			tokenC.Markup = "autolink"
			tokenC.Info = "auto"
		}
		state.Pos += len(url) + 2
		return true
	}
	return false
}

// --- Inline rules2 (post-process phase) ---

// processDelimiters matches opening and closing emphasis-like delimiters.
// Mirrors processDelimiters in rules_inline/balance_pairs.mjs.
func processDelimiters(delimiters []Delimiter) {
	openersBottom := make(map[rune][6]int)
	max := len(delimiters)
	if max == 0 {
		return
	}

	headerIdx := 0
	lastTokenIdx := -2
	jumps := make([]int, max)

	for closerIdx := 0; closerIdx < max; closerIdx++ {
		closer := &delimiters[closerIdx]
		jumps[closerIdx] = 0

		if delimiters[headerIdx].Marker != closer.Marker || lastTokenIdx != closer.Token-1 {
			headerIdx = closerIdx
		}
		lastTokenIdx = closer.Token

		if closer.Length == 0 {
			// already 0, nothing to do (strikethrough sets 0)
		}
		if !closer.Close {
			continue
		}

		bottom, ok := openersBottom[closer.Marker]
		if !ok {
			bottom = [6]int{-1, -1, -1, -1, -1, -1}
		}
		modIdx := 0
		if closer.Open {
			modIdx = 3
		}
		modIdx += closer.Length % 3
		minOpenerIdx := bottom[modIdx]

		openerIdx := headerIdx - jumps[headerIdx] - 1
		newMinOpenerIdx := openerIdx

		for ; openerIdx > minOpenerIdx; openerIdx -= jumps[openerIdx] + 1 {
			opener := &delimiters[openerIdx]
			if opener.Marker != closer.Marker {
				continue
			}
			if opener.Open && opener.End < 0 {
				isOddMatch := false
				if opener.Close || closer.Open {
					if (opener.Length+closer.Length)%3 == 0 {
						if opener.Length%3 != 0 || closer.Length%3 != 0 {
							isOddMatch = true
						}
					}
				}
				if !isOddMatch {
					lastJump := 0
					if openerIdx > 0 && !delimiters[openerIdx-1].Open {
						lastJump = jumps[openerIdx-1] + 1
					}
					jumps[closerIdx] = closerIdx - openerIdx + lastJump
					jumps[openerIdx] = lastJump
					closer.Open = false
					opener.End = closerIdx
					opener.Close = false
					newMinOpenerIdx = -1
					lastTokenIdx = -2
					break
				}
			}
		}

		if newMinOpenerIdx != -1 {
			bottom[modIdx] = newMinOpenerIdx
			openersBottom[closer.Marker] = bottom
		}
	}
}

// balancePairsRule matches opening and closing delimiters for each scope.
// Mirrors rules_inline/balance_pairs.mjs.
func balancePairsRule(stateI interface{}, _, _ int, _ bool) bool {
	state := stateI.(*StateInline)
	if state.Delimiters != nil {
		processDelimiters(*state.Delimiters)
	}
	for i := 0; i < len(state.TokensMeta); i++ {
		if state.TokensMeta[i] != nil && state.TokensMeta[i].Delimiters != nil {
			processDelimiters(*state.TokensMeta[i].Delimiters)
		}
	}
	return true
}

// emphasisPostProcessInner walks the delimiter list and converts matched
// delimiter pairs into em/strong tokens. Mirrors postProcess in emphasis.mjs.
func emphasisPostProcessInner(state *StateInline, delimiters []Delimiter) {
	for i := len(delimiters) - 1; i >= 0; i-- {
		startDelim := &delimiters[i]
		if startDelim.Marker != 0x5F /* _ */ && startDelim.Marker != 0x2A /* * */ {
			continue
		}
		if startDelim.End == -1 {
			continue
		}
		endDelim := &delimiters[startDelim.End]

		isStrong := i > 0 &&
			delimiters[i-1].End == startDelim.End+1 &&
			delimiters[i-1].Marker == startDelim.Marker &&
			delimiters[i-1].Token == startDelim.Token-1 &&
			delimiters[startDelim.End+1].Token == endDelim.Token+1

		ch := string(startDelim.Marker)
		tokenO := &state.Tokens[startDelim.Token]
		if isStrong {
			tokenO.Type = "strong_open"
			tokenO.Tag = "strong"
			tokenO.Markup = ch + ch
		} else {
			tokenO.Type = "em_open"
			tokenO.Tag = "em"
			tokenO.Markup = ch
		}
		tokenO.Nesting = 1
		tokenO.Content = ""

		tokenC := &state.Tokens[endDelim.Token]
		if isStrong {
			tokenC.Type = "strong_close"
			tokenC.Tag = "strong"
			tokenC.Markup = ch + ch
		} else {
			tokenC.Type = "em_close"
			tokenC.Tag = "em"
			tokenC.Markup = ch
		}
		tokenC.Nesting = -1
		tokenC.Content = ""

		if isStrong {
			state.Tokens[delimiters[i-1].Token].Content = ""
			state.Tokens[delimiters[startDelim.End+1].Token].Content = ""
			// Skip the outer opener on the next iteration. Without this,
			// the loop would convert the outer pair to em_open/em_close,
			// producing <em><strong>...</strong></em> instead of just
			// <strong>...</strong>. Mirrors markdown-it's `i--`.
			i--
		}
	}
}

// emphasisPostProcess walks all delimiter scopes (current + tokens_meta) and
// applies emphasis post-processing. Mirrors emphasis.postProcess.
func emphasisPostProcess(stateI interface{}, _, _ int, _ bool) bool {
	state := stateI.(*StateInline)
	if state.Delimiters != nil {
		emphasisPostProcessInner(state, *state.Delimiters)
	}
	for i := 0; i < len(state.TokensMeta); i++ {
		if state.TokensMeta[i] != nil && state.TokensMeta[i].Delimiters != nil {
			emphasisPostProcessInner(state, *state.TokensMeta[i].Delimiters)
		}
	}
	return true
}

// strikethroughPostProcessInner walks the delimiter list and converts matched
// delimiter pairs into s_open/s_close tokens. Mirrors postProcess in strikethrough.mjs.
func strikethroughPostProcessInner(state *StateInline, delimiters []Delimiter) {
	var loneMarkers []int

	for i := 0; i < len(delimiters); i++ {
		startDelim := &delimiters[i]
		if startDelim.Marker != 0x7E /* ~ */ {
			continue
		}
		if startDelim.End == -1 {
			continue
		}
		endDelim := &delimiters[startDelim.End]

		token := &state.Tokens[startDelim.Token]
		token.Type = "s_open"
		token.Tag = "s"
		token.Nesting = 1
		token.Markup = "~~"
		token.Content = ""

		token = &state.Tokens[endDelim.Token]
		token.Type = "s_close"
		token.Tag = "s"
		token.Nesting = -1
		token.Markup = "~~"
		token.Content = ""

		if endDelim.Token-1 >= 0 &&
			state.Tokens[endDelim.Token-1].Type == "text" &&
			state.Tokens[endDelim.Token-1].Content == "~" {
			loneMarkers = append(loneMarkers, endDelim.Token-1)
		}
	}

	// Move lone markers after subsequent s_close tags
	for len(loneMarkers) > 0 {
		i := loneMarkers[len(loneMarkers)-1]
		loneMarkers = loneMarkers[:len(loneMarkers)-1]
		j := i + 1
		for j < len(state.Tokens) && state.Tokens[j].Type == "s_close" {
			j++
		}
		j--
		if i != j {
			state.Tokens[i], state.Tokens[j] = state.Tokens[j], state.Tokens[i]
		}
	}
}

// strikethroughPostProcess walks all delimiter scopes and applies strikethrough
// post-processing. Mirrors strikethrough.postProcess.
func strikethroughPostProcess(stateI interface{}, _, _ int, _ bool) bool {
	state := stateI.(*StateInline)
	if state.Delimiters != nil {
		strikethroughPostProcessInner(state, *state.Delimiters)
	}
	for i := 0; i < len(state.TokensMeta); i++ {
		if state.TokensMeta[i] != nil && state.TokensMeta[i].Delimiters != nil {
			strikethroughPostProcessInner(state, *state.TokensMeta[i].Delimiters)
		}
	}
	return true
}

// fragmentsJoinRule merges adjacent text tokens and recalculates levels after
// emphasis/strikethrough post-processing. Mirrors rules_inline/fragments_join.mjs.
func fragmentsJoinRule(stateI interface{}, _, _ int, _ bool) bool {
	state := stateI.(*StateInline)
	level := 0
	tokens := state.Tokens
	max := len(tokens)
	last := 0

	for curr := 0; curr < max; curr++ {
		if tokens[curr].Nesting < 0 {
			level--
		}
		tokens[curr].Level = level
		if tokens[curr].Nesting > 0 {
			level++
		}
		if tokens[curr].Type == "text" &&
			curr+1 < max &&
			tokens[curr+1].Type == "text" {
			tokens[curr+1].Content = tokens[curr].Content + tokens[curr+1].Content
		} else {
			if curr != last {
				tokens[last] = tokens[curr]
			}
			last++
		}
	}
	if max != last {
		state.Tokens = tokens[:last]
	}
	return true
}
