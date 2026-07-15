package markdown

// Translation of: markdown-it/lib/parser_core.mjs
//                  markdown-it/lib/rules_core/normalize.mjs
//                  markdown-it/lib/rules_core/block.mjs
//                  markdown-it/lib/rules_core/inline.mjs
//                  markdown-it/lib/rules_core/text_join.mjs
// Completeness: 90%
//
// ParserCore is the top-level rule executor. It runs the core chain which
// normalizes input, runs the block parser, runs the inline parser on each
// inline token's content, and joins adjacent text tokens.

import (
	"strings"
)

// ParserCore holds the core rule chain.
type ParserCore struct {
	Ruler *Ruler
}

// NewParserCore creates a ParserCore with the default core rules registered.
// Mirrors the Core constructor.
func NewParserCore() *ParserCore {
	pc := &ParserCore{Ruler: NewRuler()}

	// Core rules: normalize, block, inline, text_join.
	// (linkify, replacements, smartquotes are skipped for P0.)
	pc.Ruler.Push("normalize", normalizeRule, nil)
	pc.Ruler.Push("block", blockRule, nil)
	pc.Ruler.Push("inline", inlineRule, nil)
	pc.Ruler.Push("text_join", textJoinRule, nil)

	return pc
}

// Process executes all core chain rules on the given state. Mirrors
// Core.prototype.process.
func (pc *ParserCore) Process(state *StateCore) {
	rules := pc.Ruler.GetRules("")
	for _, rule := range rules {
		rule(state, 0, 0, false)
	}
}

// --- Core rules ---

// normalizeRule normalizes line endings (\r\n, \r → \n), replaces NULL
// characters with U+FFFD, and ensures the source ends with a trailing newline.
// Mirrors rules_core/normalize.mjs. The trailing newline is required because
// block rules like referenceRule use Src[pos:EMarks+1] to include the line's
// newline character — without it, the last line would cause an out-of-bounds
// slice. (In JS, String.prototype.slice clamps silently; Go panics.)
func normalizeRule(stateI interface{}, _, _ int, _ bool) bool {
	state := stateI.(*StateCore)
	// Normalize newlines: \r\n and \r → \n
	str := strings.ReplaceAll(state.Src, "\r\n", "\n")
	str = strings.ReplaceAll(str, "\r", "\n")
	// Replace NULL characters with U+FFFD
	str = strings.ReplaceAll(str, "\x00", "\uFFFD")
	// Ensure trailing newline so that Src[pos:EMarks+1] is always safe
	if len(str) > 0 && str[len(str)-1] != '\n' {
		str += "\n"
	}
	state.Src = str
	return true
}

// blockRule runs the block parser on the source. In inline mode, it creates
// a single inline token instead. Mirrors rules_core/block.mjs.
func blockRule(stateI interface{}, _, _ int, _ bool) bool {
	state := stateI.(*StateCore)
	if state.InlineMode {
		token := NewToken("inline", "", 0)
		token.Content = state.Src
		token.Map = &[2]int{0, 1}
		token.Children = []Token{}
		state.Tokens = append(state.Tokens, *token)
	} else {
		state.Tokens = state.Md.Block.Parse(state.Src, state.Md, state.Env, state.Tokens)
	}
	return true
}

// inlineRule runs the inline parser on each 'inline' token's content, populating
// its Children. Mirrors rules_core/inline.mjs.
func inlineRule(stateI interface{}, _, _ int, _ bool) bool {
	state := stateI.(*StateCore)
	tokens := state.Tokens
	for i := range tokens {
		if tokens[i].Type != "inline" {
			continue
		}
		children := state.Md.Inline.Parse(tokens[i].Content, state.Md, state.Env, tokens[i].Children)
		tokens[i].Children = children
	}
	return true
}

// textJoinRule merges adjacent 'text' tokens in inline token children, and
// converts 'text_special' tokens to 'text'. Mirrors rules_core/text_join.mjs.
func textJoinRule(stateI interface{}, _, _ int, _ bool) bool {
	state := stateI.(*StateCore)
	blockTokens := state.Tokens
	for j := range blockTokens {
		if blockTokens[j].Type != "inline" {
			continue
		}
		tokens := blockTokens[j].Children
		max := len(tokens)

		// Convert text_special → text
		for curr := 0; curr < max; curr++ {
			if tokens[curr].Type == "text_special" {
				tokens[curr].Type = "text"
			}
		}

		// Collapse adjacent text tokens
		last := 0
		for curr := 0; curr < max; curr++ {
			if tokens[curr].Type == "text" &&
				curr+1 < max &&
				tokens[curr+1].Type == "text" {
				// collapse two adjacent text nodes
				tokens[curr+1].Content = tokens[curr].Content + tokens[curr+1].Content
			} else {
				if curr != last {
					tokens[last] = tokens[curr]
				}
				last++
			}
		}
		if max != last {
			tokens = tokens[:last]
			blockTokens[j].Children = tokens
		}
	}
	return true
}
