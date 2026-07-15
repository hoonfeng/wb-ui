// GFM (GitHub Flavored Markdown) extensions.
//
// This file adds the three most commonly used GFM extensions:
//   1. Tables (| col1 | col2 | with a separator row)
//   2. Task lists (- [ ] todo / - [x] done)
//   3. Autolink of bare URLs (https://example.com without <>)
//
// Strikethrough (~~text~~) is already implemented in parser_inline.go
// and is always on (no opt-in needed).
//
// The table and task-list rules are registered in the block chain;
// bare-URL autolinking is registered in the inline chain. A MarkdownIt
// instance created with NewMarkdownItWithGFM() has all three enabled.
// NewMarkdownIt() (plain CommonMark) does not include them, matching
// markdown-it's opt-in plugin model.

package markdown

import (
	"regexp"
	"strings"
)

// EnableGFM registers the GFM extensions (table, task_list, autolink)
// on an existing MarkdownIt instance. This mirrors the effect of
// markdown-it's markdownIt().use(markdownItTaskLists).use(...) plugin
// chain, but without external dependencies.
//
// Call this on a freshly-created MarkdownIt before parsing.
func (md *MarkdownIt) EnableGFM() *MarkdownIt {
	// Bare-URL autolinking requires the Linkify option.
	md.Options.Linkify = true

	// Table rule goes before heading/paragraph so it can match first.
	// It terminates paragraph, reference, and blockquote chains.
	md.Block.Ruler.Before("heading", "table", tableRule,
		[]string{"paragraph", "reference", "blockquote"})

	// Bare-URL autolink goes after the existing autolink rule (which only
	// handles <url>).
	md.Inline.Ruler.After("autolink", "autolink_bare", autolinkBareRule, nil)

	// Task list detection is done at render time (no new block rule).
	// The list_item_open token's Info field is checked by the renderer
	// to emit a checkbox.
	return md
}

// NewMarkdownItWithGFM creates a MarkdownIt with GFM extensions enabled.
func NewMarkdownItWithGFM() *MarkdownIt {
	md := NewMarkdownIt()
	return md.EnableGFM()
}

// --- Table rule (block) ---

// tableRule handles GFM pipe tables. Mirrors markdown-it-multimd-table
// and the built-in markdown-it table rule (from the plugin ecosystem).
//
// A GFM table looks like:
//
//	| Header 1 | Header 2 |
//	| -------- | -------- |
//	| cell 1   | cell 2   |
//
// The header and separator rows are required; body rows are optional.
// The separator row cells may contain only `-`, `:`, and spaces, and
// may use leading/trailing `:` for alignment.
func tableRule(stateI interface{}, startLine, endLine int, silent bool) bool {
	state := stateI.(*StateBlock)

	// Tables can't be deeply indented (must be at base indent or less).
	if startLine == endLine || startLine >= endLine {
		return false
	}
	if state.SCount[startLine]-state.BlkIndent >= 4 {
		return false
	}

	// A table must have at least a header row and a next row.
	nextLine := startLine + 1
	if nextLine >= endLine {
		return false
	}

	// Check the header row has at least one pipe.
	headerText := state.GetLines(startLine, startLine+1, state.BlkIndent, false)
	if !strings.Contains(headerText, "|") {
		return false
	}
	// Header row must have non-whitespace content.
	if strings.TrimSpace(headerText) == "" {
		return false
	}

	// Check the separator row.
	if state.IsEmpty(nextLine) {
		return false
	}
	sepText := state.GetLines(nextLine, nextLine+1, state.BlkIndent, false)
	if !isValidTableSeparator(sepText) {
		return false
	}

	if silent {
		return true
	}

	// Parse header cells.
	headerCells := splitTableCells(headerText)

	// Parse alignment from separator.
	aligns := parseTableAlignments(sepText)

	// Collect body rows.
	nextLine++ // move past separator
	bodyStart := nextLine
	for nextLine < endLine {
		if state.IsEmpty(nextLine) {
			break
		}
		// A line without a pipe ends the table.
		lineText := state.GetLines(nextLine, nextLine+1, state.BlkIndent, false)
		if !strings.Contains(lineText, "|") {
			break
		}
		nextLine++
	}
	bodyEnd := nextLine

	// Emit table_open token.
	tokenO := state.Push("table_open", "table", 1)
	tokenO.Map = &[2]int{startLine, bodyEnd}

	// Header.
	tokenH := state.Push("thead_open", "thead", 1)
	tokenH.Map = &[2]int{startLine, startLine + 1}

	tokenHR := state.Push("tr_open", "tr", 1)
	tokenHR.Map = &[2]int{startLine, startLine + 1}

	for i, cell := range headerCells {
		tokenTH := state.Push("th_open", "th", 1)
		if i < len(aligns) && aligns[i] != "" {
			tokenTH.Attrs = [][2]string{{"style", "text-align:" + aligns[i]}}
		}
		tokenI := state.Push("inline", "", 0)
		tokenI.Content = strings.TrimSpace(cell)
		tokenI.Children = []Token{}
		tokenI.Map = &[2]int{startLine, startLine + 1}
		state.Push("th_close", "th", -1)
	}

	state.Push("tr_close", "tr", -1)
	state.Push("thead_close", "thead", -1)

	// Body (optional).
	if bodyStart < bodyEnd {
		tokenB := state.Push("tbody_open", "tbody", 1)
		tokenB.Map = &[2]int{bodyStart, bodyEnd}
		for line := bodyStart; line < bodyEnd; line++ {
			lineText := state.GetLines(line, line+1, state.BlkIndent, false)
			cells := splitTableCells(lineText)
			tokenTR := state.Push("tr_open", "tr", 1)
			tokenTR.Map = &[2]int{line, line + 1}
			for i := 0; i < len(headerCells); i++ {
				cell := ""
				if i < len(cells) {
					cell = cells[i]
				}
				tokenTD := state.Push("td_open", "td", 1)
				if i < len(aligns) && aligns[i] != "" {
					tokenTD.Attrs = [][2]string{{"style", "text-align:" + aligns[i]}}
				}
				tokenI := state.Push("inline", "", 0)
				tokenI.Content = strings.TrimSpace(cell)
				tokenI.Children = []Token{}
				tokenI.Map = &[2]int{line, line + 1}
				state.Push("td_close", "td", -1)
			}
			state.Push("tr_close", "tr", -1)
		}
		state.Push("tbody_close", "tbody", -1)
	}

	state.Push("table_close", "table", -1)
	state.Line = bodyEnd
	return true
}

// isValidTableSeparator checks if a line is a valid table separator row.
// A valid separator contains only `-`, `:`, `|`, and spaces, and has at
// least one `-` per cell.
func isValidTableSeparator(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	for _, ch := range trimmed {
		if ch != '-' && ch != ':' && ch != '|' && ch != ' ' {
			return false
		}
	}
	// Must contain at least one dash.
	if !strings.ContainsRune(trimmed, '-') {
		return false
	}
	return true
}

// splitTableCells splits a table row into cell contents, handling leading/
// trailing pipes and escaped pipes (\|). Mirrors the splitCells function
// in markdown-it's table rule.
func splitTableCells(line string) []string {
	// Strip leading/trailing whitespace.
	line = strings.TrimSpace(line)
	// Strip leading and trailing pipes (optional in GFM).
	if strings.HasPrefix(line, "|") {
		line = line[1:]
	}
	if strings.HasSuffix(line, "|") {
		line = line[:len(line)-1]
	}
	if line == "" {
		return nil
	}
	// Split on |, respecting escaped \| .
	var cells []string
	var current strings.Builder
	i := 0
	for i < len(line) {
		ch := line[i]
		if ch == '\\' && i+1 < len(line) && line[i+1] == '|' {
			current.WriteByte('|')
			i += 2
			continue
		}
		if ch == '|' {
			cells = append(cells, current.String())
			current.Reset()
			i++
			continue
		}
		current.WriteByte(ch)
		i++
	}
	cells = append(cells, current.String())
	return cells
}

// parseTableAlignments extracts per-column alignment from a separator row.
// Returns a slice of "" / "left" / "right" / "center".
func parseTableAlignments(sep string) []string {
	cells := splitTableCells(sep)
	aligns := make([]string, len(cells))
	for i, cell := range cells {
		cell = strings.TrimSpace(cell)
		left := strings.HasPrefix(cell, ":")
		right := strings.HasSuffix(cell, ":")
		switch {
		case left && right:
			aligns[i] = "center"
		case left:
			aligns[i] = "left"
		case right:
			aligns[i] = "right"
		default:
			aligns[i] = ""
		}
	}
	return aligns
}

// --- Bare-URL autolink (inline) ---

// bareURLRe matches a bare URL (http/https) not wrapped in <>.
// The URL ends at whitespace or certain punctuation.
var bareURLRe = regexp.MustCompile(`^https?://[^\s<>"{}|\\^` + "`" + `[\]]+`)

// autolinkBareRule matches bare http(s) URLs and converts them to link
// tokens. Mirrors the linkify rule in markdown-it (but simpler — only
// matches http/https, no email).
//
// Because ':' is a terminator character (isTerminatorChar), the text rule
// stops at the ':' in "https://..." and the scheme "https" ends up in
// state.Pending. This rule is therefore triggered at the ':' position
// and must look backward into pending to find the scheme, then forward
// in the source to match the full URL.
func autolinkBareRule(stateI interface{}, _, _ int, silent bool) bool {
	state := stateI.(*StateInline)
	if !state.Md.Options.Linkify {
		return false
	}
	pos := state.Pos
	if pos >= state.PosMax {
		return false
	}

	// We expect to be positioned at ':' (the terminator that stopped textRule).
	if state.SrcRunes[pos] != ':' {
		return false
	}

	// Check if pending ends with "https" or "http" (the URL scheme that
	// textRule already consumed into pending).
	schemes := []string{"https", "http"}
	var scheme string
	for _, s := range schemes {
		if strings.HasSuffix(state.Pending, s) {
			scheme = s
			break
		}
	}
	if scheme == "" {
		return false
	}

	// The scheme occupies source positions [urlStart, pos).
	urlStart := pos - len(scheme)

	// Match the full URL from urlStart. Use SrcRunes because Pos/PosMax
	// are rune indices, not byte offsets into Src.
	match := bareURLRe.FindString(string(state.SrcRunes[urlStart:state.PosMax]))
	if match == "" {
		return false
	}

	if silent {
		return true
	}

	url := trimTrailingPunct(match)
	href := state.Md.NormalizeLink(url)
	if !state.Md.ValidateLink(href) {
		return false
	}

	// Remove the scheme from pending (textRule added it).
	state.Pending = state.Pending[:len(state.Pending)-len(scheme)]

	tokenO := state.Push("link_open", "a", 1)
	tokenO.Attrs = [][2]string{{"href", href}}
	tokenT := state.Push("text", "", 0)
	tokenT.Content = state.Md.NormalizeLinkText(url)
	state.Push("link_close", "a", -1)

	state.Pos = urlStart + len(url)
	return true
}

// isAlnum reports whether ch is an ASCII letter or digit.
func isAlnum(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
}

// trimTrailingPunct removes trailing punctuation that is commonly not
// part of a URL (e.g. trailing . , ; : ! ? ). This avoids including
// sentence-ending punctuation in the link.
func trimTrailingPunct(url string) string {
	for len(url) > 0 {
		last := url[len(url)-1]
		switch last {
		case '.', ',', ';', ':', '!', '?', ')', ']', '}', '"', '\'':
			url = url[:len(url)-1]
		default:
			return url
		}
	}
	return url
}

// --- Task list detection (render-time) ---
//
// Task list items are detected at render time by checking the first
// child of a list_item_open's inline content for the pattern
// "[ ] " or "[x] " / "[X] ". If found, a checkbox <input> element is
// prepended to the <li> and the marker text is removed.
//
// This is implemented in the DOMRenderer via the "inline" case in
// renderToken — see renderer_dom.go's task list handling.

// tryTaskList inspects an inline token's first text child for a GFM
// task list marker ([ ] or [x]). If found, it returns:
//   - checked: true if the marker was [x]/[X], false if [ ]
//   - stripped: a copy of tok.Children with the marker removed from
//     the first text token's content
//   - ok: true if a marker was found
//
// The original token stream is not mutated; stripped is a new slice
// with a copied first text token.
func tryTaskList(tok *Token) (checked bool, stripped []Token, ok bool) {
	if tok == nil || len(tok.Children) == 0 {
		return false, nil, false
	}
	// Find the first text token.
	idx := -1
	for i := range tok.Children {
		if tok.Children[i].Type == "text" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false, nil, false
	}
	first := &tok.Children[idx]
	isChecked, found, newContent := detectTaskMarker(first.Content)
	if !found {
		return false, nil, false
	}
	// Make a shallow copy of children and replace the first text token
	// with one whose content has the marker stripped.
	stripped = make([]Token, len(tok.Children))
	copy(stripped, tok.Children)
	stripped[idx].Content = newContent
	return isChecked, stripped, true
}

// detectTaskMarker checks if an inline token's content starts with a
// GFM task list marker ([ ] or [x]). Returns:
//   - checked: true if [x], false if [ ]
//   - ok: true if a marker was found
//   - content: the content with the marker stripped
func detectTaskMarker(content string) (checked, ok bool, stripped string) {
	if len(content) < 5 {
		return false, false, content
	}
	if content[0] != '[' || content[2] != ']' || content[3] != ' ' {
		return false, false, content
	}
	switch content[1] {
	case ' ':
		return false, true, content[4:]
	case 'x', 'X':
		return true, true, content[4:]
	}
	return false, false, content
}
