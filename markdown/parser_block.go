package markdown

// Translation of: markdown-it/lib/parser_block.mjs
//                  markdown-it/lib/rules_block/*.mjs (except table, html_block)
// Completeness: 85%
//
// ParserBlock is the block-level tokenizer. It splits the source into block
// tokens (paragraphs, headings, code blocks, lists, blockquotes, etc.).

import (
	"strings"
)

// ParserBlock holds the block rule chain.
type ParserBlock struct {
	Ruler *Ruler
}

// NewParserBlock creates a ParserBlock with the default block rules registered.
// Table and html_block rules are skipped for P0.
func NewParserBlock() *ParserBlock {
	pb := &ParserBlock{Ruler: NewRuler()}

	// Rules are registered in the same order as markdown-it. The alt array
	// lists chains that this rule can terminate (e.g. a heading can terminate
	// a paragraph).
	pb.Ruler.Push("code", codeRule, nil)
	pb.Ruler.Push("fence", fenceRule, []string{"paragraph", "reference", "blockquote", "list"})
	pb.Ruler.Push("blockquote", blockquoteRule, []string{"paragraph", "reference", "blockquote", "list"})
	pb.Ruler.Push("hr", hrRule, []string{"paragraph", "reference", "blockquote", "list"})
	pb.Ruler.Push("list", listRule, []string{"paragraph", "reference", "blockquote"})
	pb.Ruler.Push("reference", referenceRule, nil)
	pb.Ruler.Push("heading", headingRule, []string{"paragraph", "reference", "blockquote"})
	pb.Ruler.Push("lheading", lheadingRule, nil)
	pb.Ruler.Push("paragraph", paragraphRule, nil)

	return pb
}

// Tokenize generates tokens for the input range [startLine, endLine).
// Mirrors ParserBlock.prototype.tokenize.
func (pb *ParserBlock) Tokenize(state *StateBlock, startLine, endLine int) {
	rules := pb.Ruler.GetRules("")
	maxNesting := state.Md.Options.MaxNesting
	line := startLine
	hasEmptyLines := false

	for line < endLine {
		state.Line = line
		line = state.SkipEmptyLines(line)
		if line >= endLine {
			break
		}
		// Termination condition for nested calls (blockquotes & lists)
		if state.SCount[line] < state.BlkIndent {
			break
		}
		if state.Level >= maxNesting {
			state.Line = endLine
			break
		}

		prevLine := state.Line
		ok := false
		for i := 0; i < len(rules); i++ {
			ok = rules[i](state, line, endLine, false)
			if ok {
				if prevLine >= state.Line {
					panic("block rule didn't increment state.line")
				}
				break
			}
		}
		if !ok {
			panic("none of the block rules matched")
		}

		// set state.tight if we had an empty line before current tag
		state.Tight = !hasEmptyLines

		// paragraph might "eat" one newline after it in nested lists
		if state.IsEmpty(state.Line - 1) {
			hasEmptyLines = true
		}

		line = state.Line

		if line < endLine && state.IsEmpty(line) {
			hasEmptyLines = true
			line++
			state.Line = line
		}
	}
}

// Parse processes input string and pushes block tokens into outTokens.
// Returns the updated token slice (Go slices are value headers).
func (pb *ParserBlock) Parse(src string, md *MarkdownIt, env map[string]interface{}, outTokens []Token) []Token {
	if src == "" {
		return outTokens
	}
	state := NewStateBlock(src, md, env, outTokens)
	pb.Tokenize(state, state.Line, state.LineMax)
	return state.Tokens
}

// --- Block rules ---

// codeRule handles indented code blocks (4+ spaces). Mirrors rules_block/code.mjs.
func codeRule(stateI interface{}, startLine, endLine int, _ bool) bool {
	state := stateI.(*StateBlock)
	if state.SCount[startLine]-state.BlkIndent < 4 {
		return false
	}
	nextLine := startLine + 1
	last := nextLine
	for nextLine < endLine {
		if state.IsEmpty(nextLine) {
			nextLine++
			continue
		}
		if state.SCount[nextLine]-state.BlkIndent >= 4 {
			nextLine++
			last = nextLine
			continue
		}
		break
	}
	state.Line = last
	token := state.Push("code_block", "code", 0)
	token.Content = state.GetLines(startLine, last, 4+state.BlkIndent, false) + "\n"
	token.Map = &[2]int{startLine, state.Line}
	return true
}

// fenceRule handles fenced code blocks (``` or ~~~). Mirrors rules_block/fence.mjs.
func fenceRule(stateI interface{}, startLine, endLine int, silent bool) bool {
	state := stateI.(*StateBlock)
	pos := state.BMarks[startLine] + state.TShift[startLine]
	max := state.EMarks[startLine]
	if state.SCount[startLine]-state.BlkIndent >= 4 {
		return false
	}
	if pos+3 > max {
		return false
	}
	marker := rune(state.Src[pos])
	if marker != 0x7E /* ~ */ && marker != 0x60 /* ` */ {
		return false
	}
	mem := pos
	pos = state.SkipChars(pos, marker)
	length := pos - mem
	if length < 3 {
		return false
	}
	markup := state.Src[mem:pos]
	params := state.Src[pos:max]
	if marker == 0x60 /* ` */ {
		if strings.ContainsRune(params, marker) {
			return false
		}
	}
	if silent {
		return true
	}

	nextLine := startLine
	haveEndMarker := false
	for {
		nextLine++
		if nextLine >= endLine {
			break
		}
		pos = state.BMarks[nextLine] + state.TShift[nextLine]
		mem = pos
		max = state.EMarks[nextLine]
		if pos < max && state.SCount[nextLine] < state.BlkIndent {
			break
		}
		if rune(state.Src[pos]) != marker {
			continue
		}
		if state.SCount[nextLine]-state.BlkIndent >= 4 {
			continue
		}
		pos = state.SkipChars(pos, marker)
		if pos-mem < length {
			continue
		}
		pos = state.SkipSpaces(pos)
		if pos < max {
			continue
		}
		haveEndMarker = true
		break
	}

	// If a fence has heading spaces, they should be removed from its inner block
	length = state.SCount[startLine]
	state.Line = nextLine
	if haveEndMarker {
		state.Line++
	}
	token := state.Push("fence", "code", 0)
	token.Info = params
	token.Content = state.GetLines(startLine+1, nextLine, length, true)
	token.Markup = markup
	token.Map = &[2]int{startLine, state.Line}
	return true
}

// blockquoteRule handles blockquotes (>). Mirrors rules_block/blockquote.mjs.
func blockquoteRule(stateI interface{}, startLine, endLine int, silent bool) bool {
	state := stateI.(*StateBlock)
	pos := state.BMarks[startLine] + state.TShift[startLine]
	max := state.EMarks[startLine]
	oldLineMax := state.LineMax
	if state.SCount[startLine]-state.BlkIndent >= 4 {
		return false
	}
	if rune(state.Src[pos]) != 0x3E /* > */ {
		return false
	}
	if silent {
		return true
	}

	var oldBMarks, oldBSCount, oldSCount, oldTShift []int
	terminatorRules := state.Md.Block.Ruler.GetRules("blockquote")
	oldParentType := state.ParentType
	state.ParentType = "blockquote"
	lastLineEmpty := false
	nextLine := startLine

	for ; nextLine < endLine; nextLine++ {
		isOutdented := state.SCount[nextLine] < state.BlkIndent
		pos = state.BMarks[nextLine] + state.TShift[nextLine]
		max = state.EMarks[nextLine]
		if pos >= max {
			break
		}
		if rune(state.Src[pos]) != 0x3E /* > */ || isOutdented {
			// Case 2: line is not inside the blockquote, and the last line was empty.
			if lastLineEmpty {
				break
			}
			// Case 3: another tag found.
			terminate := false
			for i := 0; i < len(terminatorRules); i++ {
				if terminatorRules[i](state, nextLine, endLine, true) {
					terminate = true
					break
				}
			}
			if terminate {
				state.LineMax = nextLine
				if state.BlkIndent != 0 {
					oldBMarks = append(oldBMarks, state.BMarks[nextLine])
					oldBSCount = append(oldBSCount, state.BsCount[nextLine])
					oldTShift = append(oldTShift, state.TShift[nextLine])
					oldSCount = append(oldSCount, state.SCount[nextLine])
					state.SCount[nextLine] -= state.BlkIndent
				}
				break
			}
			oldBMarks = append(oldBMarks, state.BMarks[nextLine])
			oldBSCount = append(oldBSCount, state.BsCount[nextLine])
			oldTShift = append(oldTShift, state.TShift[nextLine])
			oldSCount = append(oldSCount, state.SCount[nextLine])
			state.SCount[nextLine] = -1
			continue
		}

		// This line is inside the blockquote.
		pos++
		initial := state.SCount[nextLine] + 1
		var spaceAfterMarker bool
		adjustTab := false

		// skip one optional space after '>'
		if pos < max && rune(state.Src[pos]) == 0x20 /* space */ {
			pos++
			initial++
			adjustTab = false
			spaceAfterMarker = true
		} else if pos < max && rune(state.Src[pos]) == 0x09 /* tab */ {
			spaceAfterMarker = true
			if (state.BsCount[nextLine]+initial)%4 == 3 {
				pos++
				initial++
				adjustTab = false
			} else {
				adjustTab = true
			}
		} else {
			spaceAfterMarker = false
		}

		offset := initial
		oldBMarks = append(oldBMarks, state.BMarks[nextLine])
		state.BMarks[nextLine] = pos
		for pos < max {
			ch := rune(state.Src[pos])
			if !IsSpace(ch) {
				break
			}
			if ch == 0x09 {
				offset += 4 - (offset+state.BsCount[nextLine])%4
				if adjustTab {
					offset++
				}
			} else {
				offset++
			}
			pos++
		}
		lastLineEmpty = pos >= max
		oldBSCount = append(oldBSCount, state.BsCount[nextLine])
		state.BsCount[nextLine] = state.SCount[nextLine] + 1
		if spaceAfterMarker {
			state.BsCount[nextLine]++
		}
		oldSCount = append(oldSCount, state.SCount[nextLine])
		state.SCount[nextLine] = offset - initial
		oldTShift = append(oldTShift, state.TShift[nextLine])
		state.TShift[nextLine] = pos - state.BMarks[nextLine]
	}

	oldIndent := state.BlkIndent
	state.BlkIndent = 0
	tokenO := state.Push("blockquote_open", "blockquote", 1)
	tokenO.Markup = ">"
	lines := [2]int{startLine, 0}
	tokenO.Map = &lines
	state.Md.Block.Tokenize(state, startLine, nextLine)
	tokenC := state.Push("blockquote_close", "blockquote", -1)
	tokenC.Markup = ">"
	state.LineMax = oldLineMax
	state.ParentType = oldParentType
	lines[1] = state.Line

	for i := 0; i < len(oldTShift); i++ {
		state.BMarks[i+startLine] = oldBMarks[i]
		state.TShift[i+startLine] = oldTShift[i]
		state.SCount[i+startLine] = oldSCount[i]
		state.BsCount[i+startLine] = oldBSCount[i]
	}
	state.BlkIndent = oldIndent
	return true
}

// hrRule handles horizontal rules (---, ***, ___). Mirrors rules_block/hr.mjs.
func hrRule(stateI interface{}, startLine, endLine int, silent bool) bool {
	state := stateI.(*StateBlock)
	max := state.EMarks[startLine]
	if state.SCount[startLine]-state.BlkIndent >= 4 {
		return false
	}
	pos := state.BMarks[startLine] + state.TShift[startLine]
	marker := rune(state.Src[pos])
	pos++
	if marker != 0x2A /* * */ && marker != 0x2D /* - */ && marker != 0x5F /* _ */ {
		return false
	}
	cnt := 1
	for pos < max {
		ch := rune(state.Src[pos])
		pos++
		if ch != marker && !IsSpace(ch) {
			return false
		}
		if ch == marker {
			cnt++
		}
	}
	if cnt < 3 {
		return false
	}
	if silent {
		return true
	}
	state.Line = startLine + 1
	token := state.Push("hr", "hr", 0)
	token.Map = &[2]int{startLine, state.Line}
	token.Markup = strings.Repeat(string(marker), cnt)
	return true
}

// listRule handles both ordered and unordered lists. Mirrors rules_block/list.mjs.
func listRule(stateI interface{}, startLine, endLine int, silent bool) bool {
	state := stateI.(*StateBlock)
	var pos, start int
	var token *Token
	nextLine := startLine
	tight := true

	if state.SCount[nextLine]-state.BlkIndent >= 4 {
		return false
	}
	if state.ListIndent >= 0 &&
		state.SCount[nextLine]-state.ListIndent >= 4 &&
		state.SCount[nextLine] < state.BlkIndent {
		return false
	}

	isTerminatingParagraph := false
	if silent && state.ParentType == "paragraph" {
		if state.SCount[nextLine] >= state.BlkIndent {
			isTerminatingParagraph = true
		}
	}

	var isOrdered bool
	var markerValue int
	var posAfterMarker int

	if posAfterMarker = skipOrderedListMarker(state, nextLine); posAfterMarker >= 0 {
		isOrdered = true
		start = state.BMarks[nextLine] + state.TShift[nextLine]
		// parse the number
		numStr := state.Src[start : posAfterMarker-1]
		markerValue = 0
		for _, c := range []byte(numStr) {
			markerValue = markerValue*10 + int(c-'0')
		}
		if isTerminatingParagraph && markerValue != 1 {
			return false
		}
	} else if posAfterMarker = skipBulletListMarker(state, nextLine); posAfterMarker >= 0 {
		isOrdered = false
	} else {
		return false
	}

	if isTerminatingParagraph {
		if state.SkipSpaces(posAfterMarker) >= state.EMarks[nextLine] {
			return false
		}
	}
	if silent {
		return true
	}

	markerCharCode := rune(state.Src[posAfterMarker-1])
	listTokIdx := len(state.Tokens)

	if isOrdered {
		token = state.Push("ordered_list_open", "ol", 1)
		if markerValue != 1 {
			token.Attrs = [][2]string{{"start", string(rune('0'+markerValue))}}
		}
	} else {
		token = state.Push("bullet_list_open", "ul", 1)
	}
	listLines := [2]int{nextLine, 0}
	token.Map = &listLines
	token.Markup = string(markerCharCode)

	prevEmptyEnd := false
	terminatorRules := state.Md.Block.Ruler.GetRules("list")
	oldParentType := state.ParentType
	state.ParentType = "list"

	for nextLine < endLine {
		pos = posAfterMarker
		max := state.EMarks[nextLine]
		initial := state.SCount[nextLine] + posAfterMarker - (state.BMarks[nextLine] + state.TShift[nextLine])
		offset := initial
		for pos < max {
			ch := rune(state.Src[pos])
			if ch == 0x09 {
				offset += 4 - (offset+state.BsCount[nextLine])%4
			} else if ch == 0x20 {
				offset++
			} else {
				break
			}
			pos++
		}
		contentStart := pos
		var indentAfterMarker int
		if contentStart >= max {
			indentAfterMarker = 1
		} else {
			indentAfterMarker = offset - initial
		}
		if indentAfterMarker > 4 {
			indentAfterMarker = 1
		}
		indent := initial + indentAfterMarker

		token = state.Push("list_item_open", "li", 1)
		token.Markup = string(markerCharCode)
		itemLines := [2]int{nextLine, 0}
		token.Map = &itemLines
		if isOrdered {
			token.Info = state.Src[start : posAfterMarker-1]
		}

		oldTight := state.Tight
		oldTShift := state.TShift[nextLine]
		oldSCount := state.SCount[nextLine]
		oldListIndent := state.ListIndent
		state.ListIndent = state.BlkIndent
		state.BlkIndent = indent
		state.Tight = true
		state.TShift[nextLine] = contentStart - state.BMarks[nextLine]
		state.SCount[nextLine] = offset

		if contentStart >= max && state.IsEmpty(nextLine+1) {
			if nextLine+2 < endLine {
				state.Line = nextLine + 2
			} else {
				state.Line = endLine
			}
		} else {
			state.Md.Block.Tokenize(state, nextLine, endLine)
		}

		if !state.Tight || prevEmptyEnd {
			tight = false
		}
		prevEmptyEnd = (state.Line-nextLine) > 1 && state.IsEmpty(state.Line-1)

		state.BlkIndent = state.ListIndent
		state.ListIndent = oldListIndent
		state.TShift[nextLine] = oldTShift
		state.SCount[nextLine] = oldSCount
		state.Tight = oldTight

		token = state.Push("list_item_close", "li", -1)
		token.Markup = string(markerCharCode)
		nextLine = state.Line
		itemLines[1] = nextLine

		if nextLine >= endLine {
			break
		}
		if state.SCount[nextLine] < state.BlkIndent {
			break
		}
		if state.SCount[nextLine]-state.BlkIndent >= 4 {
			break
		}
		terminate := false
		for i := 0; i < len(terminatorRules); i++ {
			if terminatorRules[i](state, nextLine, endLine, true) {
				terminate = true
				break
			}
		}
		if terminate {
			break
		}
		if isOrdered {
			posAfterMarker = skipOrderedListMarker(state, nextLine)
			if posAfterMarker < 0 {
				break
			}
			start = state.BMarks[nextLine] + state.TShift[nextLine]
		} else {
			posAfterMarker = skipBulletListMarker(state, nextLine)
			if posAfterMarker < 0 {
				break
			}
		}
		if markerCharCode != rune(state.Src[posAfterMarker-1]) {
			break
		}
	}

	if isOrdered {
		token = state.Push("ordered_list_close", "ol", -1)
	} else {
		token = state.Push("bullet_list_close", "ul", -1)
	}
	token.Markup = string(markerCharCode)
	listLines[1] = nextLine
	state.Line = nextLine
	state.ParentType = oldParentType

	if tight {
		markTightParagraphs(state, listTokIdx)
	}
	return true
}

// skipBulletListMarker searches for [-+*][\n ], returns next pos after marker
// or -1 on fail. Mirrors the function in rules_block/list.mjs.
func skipBulletListMarker(state *StateBlock, startLine int) int {
	max := state.EMarks[startLine]
	pos := state.BMarks[startLine] + state.TShift[startLine]
	marker := rune(state.Src[pos])
	pos++
	if marker != 0x2A /* * */ && marker != 0x2D /* - */ && marker != 0x2B /* + */ {
		return -1
	}
	if pos < max {
		ch := rune(state.Src[pos])
		if !IsSpace(ch) {
			return -1
		}
	}
	return pos
}

// skipOrderedListMarker searches for \d+[.)][\n ], returns next pos after marker
// or -1 on fail. Mirrors the function in rules_block/list.mjs.
func skipOrderedListMarker(state *StateBlock, startLine int) int {
	start := state.BMarks[startLine] + state.TShift[startLine]
	max := state.EMarks[startLine]
	pos := start
	if pos+1 >= max {
		return -1
	}
	ch := rune(state.Src[pos])
	pos++
	if ch < 0x30 /* 0 */ || ch > 0x39 /* 9 */ {
		return -1
	}
	for {
		if pos >= max {
			return -1
		}
		ch = rune(state.Src[pos])
		pos++
		if ch >= 0x30 && ch <= 0x39 {
			if pos-start >= 10 {
				return -1
			}
			continue
		}
		if ch == 0x29 /* ) */ || ch == 0x2E /* . */ {
			break
		}
		return -1
	}
	if pos < max {
		ch = rune(state.Src[pos])
		if !IsSpace(ch) {
			return -1
		}
	}
	return pos
}

// markTightParagraphs hides paragraph_open/close tokens for tight lists.
// Mirrors the function in rules_block/list.mjs.
func markTightParagraphs(state *StateBlock, idx int) {
	level := state.Level + 2
	for i := idx + 2; i < len(state.Tokens)-2; i++ {
		if state.Tokens[i].Level == level && state.Tokens[i].Type == "paragraph_open" {
			state.Tokens[i+2].Hidden = true
			state.Tokens[i].Hidden = true
			i += 2
		}
	}
}

// referenceRule handles reference link definitions ([label]: url "title").
// Mirrors rules_block/reference.mjs.
func referenceRule(stateI interface{}, startLine, _ int, silent bool) bool {
	state := stateI.(*StateBlock)
	pos := state.BMarks[startLine] + state.TShift[startLine]
	max := state.EMarks[startLine]
	nextLine := startLine + 1

	if state.SCount[startLine]-state.BlkIndent >= 4 {
		return false
	}
	if rune(state.Src[pos]) != 0x5B /* [ */ {
		return false
	}

	// getNextLine fetches the next line content (including newline) for multi-line references
	getNextLine := func(nextLine int) string {
		endLine := state.LineMax
		if nextLine >= endLine || state.IsEmpty(nextLine) {
			return ""
		}
		isContinuation := false
		if state.SCount[nextLine]-state.BlkIndent > 3 {
			isContinuation = true
		}
		if state.SCount[nextLine] < 0 {
			isContinuation = true
		}
		if !isContinuation {
			terminatorRules := state.Md.Block.Ruler.GetRules("reference")
			oldParentType := state.ParentType
			state.ParentType = "reference"
			terminate := false
			for i := 0; i < len(terminatorRules); i++ {
				if terminatorRules[i](state, nextLine, endLine, true) {
					terminate = true
					break
				}
			}
			state.ParentType = oldParentType
			if terminate {
				return ""
			}
		}
		pos := state.BMarks[nextLine] + state.TShift[nextLine]
		max := state.EMarks[nextLine]
		return state.Src[pos : max+1]
	}

	str := state.Src[pos : max+1]
	maxLen := len(str)
	labelEnd := -1

	pos = 1
	for pos < maxLen {
		ch := rune(str[pos])
		if ch == 0x5B /* [ */ {
			return false
		} else if ch == 0x5D /* ] */ {
			labelEnd = pos
			break
		} else if ch == 0x0A /* \n */ {
			lineContent := getNextLine(nextLine)
			if lineContent != "" {
				str += lineContent
				maxLen = len(str)
				nextLine++
			}
		} else if ch == 0x5C /* \ */ {
			pos++
			if pos < maxLen && str[pos] == 0x0A {
				lineContent := getNextLine(nextLine)
				if lineContent != "" {
					str += lineContent
					maxLen = len(str)
					nextLine++
				}
			}
		}
		pos++
	}

	if labelEnd < 0 || rune(str[labelEnd+1]) != 0x3A /* : */ {
		return false
	}

	// [label]:   destination   'title'
	//         ^^^ skip optional whitespace
	for pos = labelEnd + 2; pos < maxLen; pos++ {
		ch := rune(str[pos])
		if ch == 0x0A {
			lineContent := getNextLine(nextLine)
			if lineContent != "" {
				str += lineContent
				maxLen = len(str)
				nextLine++
			}
		} else if IsSpace(ch) {
			// skip
		} else {
			break
		}
	}

	destRes := ParseLinkDestination(str, pos, maxLen)
	if !destRes.Ok {
		return false
	}
	href := state.Md.NormalizeLink(destRes.Str)
	if !state.Md.ValidateLink(href) {
		return false
	}
	pos = destRes.Pos
	destEndPos := pos
	destEndLineNo := nextLine

	// skip spaces after destination
	start := pos
	for ; pos < maxLen; pos++ {
		ch := rune(str[pos])
		if ch == 0x0A {
			lineContent := getNextLine(nextLine)
			if lineContent != "" {
				str += lineContent
				maxLen = len(str)
				nextLine++
			}
		} else if IsSpace(ch) {
			// skip
		} else {
			break
		}
	}

	// parse title
	titleRes := ParseLinkTitle(str, pos, maxLen, nil)
	for titleRes.CanContinue {
		lineContent := getNextLine(nextLine)
		if lineContent == "" {
			break
		}
		str += lineContent
		pos = maxLen
		maxLen = len(str)
		nextLine++
		titleRes = ParseLinkTitle(str, pos, maxLen, &titleRes)
	}

	var title string
	if pos < maxLen && start != pos && titleRes.Ok {
		title = titleRes.Str
		pos = titleRes.Pos
	} else {
		title = ""
		pos = destEndPos
		nextLine = destEndLineNo
	}

	// skip trailing spaces
	for pos < maxLen {
		ch := rune(str[pos])
		if !IsSpace(ch) {
			break
		}
		pos++
	}
	if pos < maxLen && rune(str[pos]) != 0x0A {
		if title != "" {
			title = ""
			pos = destEndPos
			nextLine = destEndLineNo
			for pos < maxLen {
				ch := rune(str[pos])
				if !IsSpace(ch) {
					break
				}
				pos++
			}
		}
	}
	if pos < maxLen && rune(str[pos]) != 0x0A {
		return false
	}

	label := NormalizeReference(str[1:labelEnd])
	if label == "" {
		return false
	}
	if silent {
		return true
	}
	if state.Env["references"] == nil {
		state.Env["references"] = make(map[string]interface{})
	}
	refs := state.Env["references"].(map[string]interface{})
	if refs[label] == nil {
		refs[label] = map[string]interface{}{
			"title": title,
			"href":  href,
		}
	}
	state.Line = nextLine
	return true
}

// headingRule handles ATX headings (#, ##, ...). Mirrors rules_block/heading.mjs.
func headingRule(stateI interface{}, startLine, endLine int, silent bool) bool {
	state := stateI.(*StateBlock)
	pos := state.BMarks[startLine] + state.TShift[startLine]
	max := state.EMarks[startLine]
	if state.SCount[startLine]-state.BlkIndent >= 4 {
		return false
	}
	ch := rune(state.Src[pos])
	if ch != 0x23 /* # */ || pos >= max {
		return false
	}
	level := 1
	pos++
	ch = rune(state.Src[pos])
	for ch == 0x23 /* # */ && pos < max && level <= 6 {
		level++
		pos++
		ch = rune(state.Src[pos])
	}
	if level > 6 || (pos < max && !IsSpace(ch)) {
		return false
	}
	if silent {
		return true
	}
	max = state.SkipSpacesBack(max, pos)
	tmp := state.SkipCharsBack(max, 0x23, pos) // #
	if tmp > pos && IsSpace(rune(state.Src[tmp-1])) {
		max = tmp
	}
	state.Line = startLine + 1
	tokenO := state.Push("heading_open", "h"+intToStr(level), 1)
	tokenO.Markup = strings.Repeat("#", level)
	tokenO.Map = &[2]int{startLine, state.Line}
	tokenI := state.Push("inline", "", 0)
	tokenI.Content = AsciiTrim(state.Src[pos:max])
	tokenI.Map = &[2]int{startLine, state.Line}
	tokenI.Children = []Token{}
	tokenC := state.Push("heading_close", "h"+intToStr(level), -1)
	tokenC.Markup = strings.Repeat("#", level)
	return true
}

// lheadingRule handles Setext headings (===, ---). Mirrors rules_block/lheading.mjs.
func lheadingRule(stateI interface{}, startLine, endLine int, _ bool) bool {
	state := stateI.(*StateBlock)
	terminatorRules := state.Md.Block.Ruler.GetRules("paragraph")
	if state.SCount[startLine]-state.BlkIndent >= 4 {
		return false
	}
	oldParentType := state.ParentType
	state.ParentType = "paragraph"

	level := 0
	var marker rune
	nextLine := startLine + 1

	for ; nextLine < endLine && !state.IsEmpty(nextLine); nextLine++ {
		if state.SCount[nextLine]-state.BlkIndent > 3 {
			continue
		}
		if state.SCount[nextLine] >= state.BlkIndent {
			pos := state.BMarks[nextLine] + state.TShift[nextLine]
			max := state.EMarks[nextLine]
			if pos < max {
				marker = rune(state.Src[pos])
				if marker == 0x2D /* - */ || marker == 0x3D /* = */ {
					pos = state.SkipChars(pos, marker)
					pos = state.SkipSpaces(pos)
					if pos >= max {
						if marker == 0x3D /* = */ {
							level = 1
						} else {
							level = 2
						}
						break
					}
				}
			}
		}
		if state.SCount[nextLine] < 0 {
			continue
		}
		terminate := false
		for i := 0; i < len(terminatorRules); i++ {
			if terminatorRules[i](state, nextLine, endLine, true) {
				terminate = true
				break
			}
		}
		if terminate {
			break
		}
	}

	if level == 0 {
		state.ParentType = oldParentType
		return false
	}

	content := AsciiTrim(state.GetLines(startLine, nextLine, state.BlkIndent, false))
	state.Line = nextLine + 1
	tokenO := state.Push("heading_open", "h"+intToStr(level), 1)
	tokenO.Markup = string(marker)
	tokenO.Map = &[2]int{startLine, state.Line}
	tokenI := state.Push("inline", "", 0)
	tokenI.Content = content
	tokenI.Map = &[2]int{startLine, state.Line - 1}
	tokenI.Children = []Token{}
	tokenC := state.Push("heading_close", "h"+intToStr(level), -1)
	tokenC.Markup = string(marker)
	state.ParentType = oldParentType
	return true
}

// paragraphRule handles paragraphs. Mirrors rules_block/paragraph.mjs.
func paragraphRule(stateI interface{}, startLine, endLine int, _ bool) bool {
	state := stateI.(*StateBlock)
	terminatorRules := state.Md.Block.Ruler.GetRules("paragraph")
	oldParentType := state.ParentType
	nextLine := startLine + 1
	state.ParentType = "paragraph"

	for ; nextLine < endLine && !state.IsEmpty(nextLine); nextLine++ {
		if state.SCount[nextLine]-state.BlkIndent > 3 {
			continue
		}
		if state.SCount[nextLine] < 0 {
			continue
		}
		terminate := false
		for i := 0; i < len(terminatorRules); i++ {
			if terminatorRules[i](state, nextLine, endLine, true) {
				terminate = true
				break
			}
		}
		if terminate {
			break
		}
	}

	content := AsciiTrim(state.GetLines(startLine, nextLine, state.BlkIndent, false))
	state.Line = nextLine
	tokenO := state.Push("paragraph_open", "p", 1)
	tokenO.Map = &[2]int{startLine, state.Line}
	tokenI := state.Push("inline", "", 0)
	tokenI.Content = content
	tokenI.Map = &[2]int{startLine, state.Line}
	tokenI.Children = []Token{}
	state.Push("paragraph_close", "p", -1)
	state.ParentType = oldParentType
	return true
}

// intToStr converts an int to its decimal string representation.
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + intToStr(-n)
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
