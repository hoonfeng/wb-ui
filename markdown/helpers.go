package markdown

// Translation of: markdown-it/lib/helpers/parse_link_label.mjs
//                  markdown-it/lib/helpers/parse_link_destination.mjs
//                  markdown-it/lib/helpers/parse_link_title.mjs
// Completeness: 100%
//
// These three helpers parse the three components of a Markdown link/reference:
// the label (text inside [...]), the destination (URL), and the title. They are
// used by the inline `link`, `image`, and block `reference` rules.

// ParseLinkLabel scans for the matching `]` of a `[` at position `start`.
// It assumes the first character at `start` is `[`. Returns the index of the
// `]` (inclusive) or -1 if no match.
//
// The `disableNested` flag, when true, causes the function to bail out if a
// nested `[` is encountered (used by image rules to prevent `![a[b]](url)`
// from being parsed as a nested image).
//
// Mirrors markdown-it/lib/helpers/parse_link_label.mjs.
//
// Note: this function calls state.md.inline.SkipToken(state) to skip over
// inline tokens (links, images, code spans, etc.) that appear inside the
// label. A `[` that is part of a token (e.g., inside a code span) does not
// increase the nesting level because skipToken advances pos past it.
func ParseLinkLabel(state *StateInline, start int, disableNested bool) int {
	level := 0
	found := false
	var prevPos int

	max := state.PosMax
	oldPos := state.Pos
	state.Pos = start + 1
	level = 1

	for state.Pos < max {
		marker := state.SrcRunes[state.Pos]
		if marker == 0x5D /* ] */ {
			level--
			if level == 0 {
				found = true
				break
			}
		}
		prevPos = state.Pos
		// skipToken advances state.Pos past one inline token. If a `[` is
		// consumed as part of a token (e.g. inside a code span), prevPos will
		// differ from state.Pos-1 after the call.
		if state.Md.Inline != nil {
			state.Md.Inline.SkipToken(state)
		} else {
			state.Pos++
		}
		if marker == 0x5B /* [ */ {
			if prevPos == state.Pos-1 {
				// `[` was not part of any token — increase nesting level
				level++
			} else if disableNested {
				state.Pos = oldPos
				return -1
			}
		}
	}

	labelEnd := -1
	if found {
		labelEnd = state.Pos
	}
	// restore old state
	state.Pos = oldPos
	return labelEnd
}

// LinkDestinationResult is the return value of ParseLinkDestination.
type LinkDestinationResult struct {
	Ok  bool
	Pos int
	Str string
}

// ParseLinkDestination parses a link destination (URL) starting at position
// `start` in `str`, up to position `max`. Two forms are supported:
//
//   - `<...>` — angle-bracket delimited; terminated by `>`. Newlines and `<`
//     inside are invalid.
//   - bare — terminated by whitespace or a control character; parentheses
//     may be balanced (up to 32 levels).
//
// Mirrors markdown-it/lib/helpers/parse_link_destination.mjs.
func ParseLinkDestination(str string, start, max int) LinkDestinationResult {
	var code rune
	pos := start
	result := LinkDestinationResult{}

	runes := []rune(str)
	if pos < len(runes) && runes[pos] == 0x3C /* < */ {
		pos++
		for pos < max {
			code = runes[pos]
			if code == 0x0A /* \n */ {
				return result
			}
			if code == 0x3C /* < */ {
				return result
			}
			if code == 0x3E /* > */ {
				result.Pos = pos + 1
				result.Str = UnescapeAll(string(runes[start+1 : pos]))
				result.Ok = true
				return result
			}
			if code == 0x5C /* \ */ && pos+1 < max {
				pos += 2
				continue
			}
			pos++
		}
		// no closing '>'
		return result
	}

	// bare destination
	level := 0
	for pos < max {
		code = runes[pos]
		if code == 0x20 /* space */ {
			break
		}
		// ascii control characters
		if code < 0x20 || code == 0x7F {
			break
		}
		if code == 0x5C /* \ */ && pos+1 < max {
			if runes[pos+1] == 0x20 {
				pos++
				continue
			}
			pos += 2
			continue
		}
		if code == 0x28 /* ( */ {
			level++
			if level > 32 {
				return result
			}
		}
		if code == 0x29 /* ) */ {
			if level == 0 {
				break
			}
			level--
		}
		pos++
	}

	if start == pos {
		return result
	}
	if level != 0 {
		return result
	}

	result.Str = UnescapeAll(string(runes[start:pos]))
	result.Pos = pos
	result.Ok = true
	return result
}

// LinkTitleResult is the return value of ParseLinkTitle.
type LinkTitleResult struct {
	Ok          bool
	CanContinue bool
	Pos         int
	Str         string
	Marker      rune
}

// ParseLinkTitle parses a link title starting at position `start` in `str`, up
// to `max`. A title is delimited by `"`, `'`, or matching `(`/`)`.
//
// If prevState is non-nil, the parse continues from a previous line (used for
// multi-line reference definitions). In that case the marker is inherited
// and the title string is appended to.
//
// Mirrors markdown-it/lib/helpers/parse_link_title.mjs.
func ParseLinkTitle(str string, start, max int, prevState *LinkTitleResult) LinkTitleResult {
	var code rune
	pos := start
	state := LinkTitleResult{}

	if prevState != nil {
		// continuation of a previous parseLinkTitle call on the next line
		state.Str = prevState.Str
		state.Marker = prevState.Marker
	} else {
		if pos >= max {
			return state
		}
		runes := []rune(str)
		marker := runes[pos]
		if marker != 0x22 /* " */ && marker != 0x27 /* ' */ && marker != 0x28 /* ( */ {
			return state
		}
		start++
		pos++
		// if opening marker is "(", switch to closing marker ")"
		if marker == 0x28 {
			marker = 0x29
		}
		state.Marker = marker
	}

	runes := []rune(str)
	for pos < max {
		code = runes[pos]
		if code == state.Marker {
			state.Pos = pos + 1
			state.Str += UnescapeAll(string(runes[start:pos]))
			state.Ok = true
			return state
		} else if code == 0x28 /* ( */ && state.Marker == 0x29 /* ) */ {
			return state
		} else if code == 0x5C /* \ */ && pos+1 < max {
			pos++
		}
		pos++
	}

	// no closing marker found, but this link title may continue on the next line
	state.CanContinue = true
	state.Str += UnescapeAll(string(runes[start:pos]))
	return state
}
