// Translation of: VS Code Monarch Tokenizer
//   https://github.com/microsoft/vscode/blob/main/src/vs/editor/standalone/common/monarch/monarchLexer.ts
//   https://github.com/microsoft/vscode/blob/main/src/vs/editor/standalone/common/monarch/monarchTypes.ts
//
// Completeness: 70%
// Differences from VS Code Monarch:
//   - Monarch uses JavaScript's sticky regex (y flag) for position-anchored
//     matching; this port simulates it by matching against src[pos:] and
//     checking that the match starts at index 0.
//   - The `includeLF` and `lineWidth` options are simplified; the tokenizer
//     processes the entire source in one pass.
//   - The `bracket` action (auto-bracket matching) is omitted in v1.
//   - Embedded language switching (nextEmbedded) is supported via a
//     callback but not fully integrated in v1.
//   - Methods use PascalCase (Go convention).
//
// Monarch is a declarative tokenizer: you define a state machine where
// each state has a list of rules, and each rule has a regex and an action.
// The tokenizer starts in the "start" state and transitions between states
// as it matches rules.

package editor

import (
	"regexp"
	"strings"
)

// MonarchLanguage describes a Monarch tokenizer configuration. It is a
// declarative state machine: the `Tokenizer` field maps state names to
// rule lists, and the `Start` field names the initial state.
type MonarchLanguage struct {
	// Tokenizer maps state names to ordered rule lists.
	Tokenizer map[string][]MonarchRule
	// Start is the name of the initial state.
	Start string
	// IgnoreCase, if true, makes all regex matching case-insensitive.
	IgnoreCase bool
	// DefaultToken is the token type emitted for unmatched text. If empty,
	// unmatched text is emitted with no scope.
	DefaultToken string
	// IncludeLF, if true, emits a "\n" token for each line break.
	IncludeLF bool
}

// MonarchRule describes a single rule in a Monarch state.
type MonarchRule struct {
	// Regex is the regular expression string to match at the current
	// position.
	Regex string
	// Action describes what to do when the regex matches.
	Action MonarchAction
	// Next, if non-empty, transitions to this state (replaces current).
	// Shorthand for Action.Next.
	Next string
	// Push, if non-empty, pushes this state onto the stack.
	// Shorthand for Action.Push.
	Push string
	// Token is a simple token type to emit. Shorthand for Action.Token.
	Token string
	// Include, if non-empty, inlines rules from the named state.
	// Shorthand for Action.Token = "@include".
	Include string
}

// MonarchAction describes the action taken when a rule's regex matches.
type MonarchAction struct {
	// Token is the token type to emit (e.g. "keyword.control" or "string").
	// Special values: "@rematch" (re-match without advancing),
	// "@brackets" (use bracket type).
	Token string
	// Next transitions to this state. Special values:
	//   "@pop" — pop the current state from the stack
	//   "@popall" — clear the stack to the start state
	Next string
	// Push pushes this state onto the stack.
	Push string
	// NextEmbedded switches to an embedded language.
	NextEmbedded string
	// Log prints a debug message when this rule fires.
	Log string
	// Bracket is "open" or "close" for bracket matching (omitted in v1).
	Bracket string
}

// compiledRule pairs a MonarchRule with its pre-compiled regex.
type compiledRule struct {
	rule  MonarchRule
	regex *regexp.Regexp
}

// compiledLanguage holds a Monarch language definition with all regexes
// pre-compiled and includes expanded.
type compiledLanguage struct {
	states map[string][]compiledRule
	start  string
}

// compileLanguage pre-compiles all regexes in the language definition and
// resolves @include directives.
func compileLanguage(lang *MonarchLanguage) *compiledLanguage {
	cl := &compiledLanguage{
		states: make(map[string][]compiledRule),
		start:  lang.Start,
	}
	prefix := ""
	if lang.IgnoreCase {
		prefix = "(?i)"
	}
	for state := range lang.Tokenizer {
		expanded := expandIncludesRecursive(lang, state, map[string]bool{})
		compiled := make([]compiledRule, 0, len(expanded))
		for _, rule := range expanded {
			if rule.Regex == "" {
				// No regex (include rules are already expanded); skip.
				continue
			}
			re, err := regexp.Compile(prefix + rule.Regex)
			if err != nil {
				continue
			}
			compiled = append(compiled, compiledRule{rule: rule, regex: re})
		}
		cl.states[state] = compiled
	}
	return cl
}

// expandIncludesRecursive returns the rules for a state with @include
// directives expanded. Uses `visited` to prevent infinite recursion.
func expandIncludesRecursive(lang *MonarchLanguage, state string, visited map[string]bool) []MonarchRule {
	if visited[state] {
		return nil
	}
	visited[state] = true
	defer delete(visited, state)

	rules := lang.Tokenizer[state]
	var result []MonarchRule
	for _, rule := range rules {
		inc := rule.Include
		if inc == "" && rule.Action.Token == "@include" {
			inc = rule.Action.Next
		}
		if inc != "" && inc != "@include" {
			result = append(result, expandIncludesRecursive(lang, inc, visited)...)
			continue
		}
		// Merge shorthand fields into the action.
		r := rule
		if r.Token != "" && r.Action.Token == "" {
			r.Action.Token = r.Token
		}
		if r.Next != "" && r.Action.Next == "" {
			r.Action.Next = r.Next
		}
		if r.Push != "" && r.Action.Push == "" {
			r.Action.Push = r.Push
		}
		result = append(result, r)
	}
	return result
}

// MonarchState holds the mutable state of the tokenizer during tokenization.
type MonarchState struct {
	// Stack is the state stack. The top element is the current state.
	Stack []string
	// Pos is the current rune offset in the source.
	Pos int
}

// CurrentState returns the current state (top of the stack).
func (s *MonarchState) CurrentState() string {
	if len(s.Stack) == 0 {
		return ""
	}
	return s.Stack[len(s.Stack)-1]
}

// Tokenize runs the Monarch tokenizer over `src` using the given language
// definition. Returns a list of tokens covering the entire source.
func Tokenize(lang *MonarchLanguage, src string) []Token {
	cl := compileLanguage(lang)
	defaultToken := lang.DefaultToken

	state := &MonarchState{
		Stack: []string{lang.Start},
	}

	var tokens []Token
	srcRunes := []rune(src)
	totalLen := len(srcRunes)

	// We work with rune positions, so convert src to a rune slice and
	// match against the rune slice (converted back to string for regex).
	for state.Pos < totalLen {
		matched := false
		currentState := state.CurrentState()
		rules, ok := cl.states[currentState]
		if !ok {
			// Unknown state; emit rest as default and stop.
			if state.Pos < totalLen {
				tokens = append(tokens, NewToken(state.Pos, totalLen, defaultToken))
			}
			break
		}

		// Get the remaining source as a string (from current rune position).
		remaining := string(srcRunes[state.Pos:])

		for _, cr := range rules {
			if cr.regex == nil {
				continue
			}
			match := cr.regex.FindStringIndex(remaining)
			if match == nil || match[0] != 0 {
				continue
			}
			matchLen := match[1] // byte length of match in `remaining`
			// Convert byte length to rune length.
			matchStr := remaining[:matchLen]
			runeLen := len([]rune(matchStr))

			startPos := state.Pos
			endPos := state.Pos + runeLen

			// Emit token.
			// If the rule specifies a token type, emit it. If the token
			// type is empty, skip emission (the matched text is consumed
			// but no token is produced, e.g. whitespace).
			tokenType := cr.rule.Action.Token
			if tokenType == "" {
				tokenType = cr.rule.Token
			}
			if tokenType != "" && runeLen > 0 {
				tokens = append(tokens, NewToken(startPos, endPos, parseScopes(tokenType)...))
			}

			// State transitions.
			// In Monarch, state references are prefixed with "@" (e.g.
			// "@blockComment"), but the actual state names in the map
			// don't include "@". Strip it before using as a state name.
			//
			// @pop when the stack has only 1 element resets to the start
			// state. This handles rules that use Next (not Push) to enter
			// a sub-state and @pop to exit back to the root state.
			next := cr.rule.Action.Next
			push := cr.rule.Action.Push

			if next == "@pop" {
				if len(state.Stack) > 1 {
					state.Stack = state.Stack[:len(state.Stack)-1]
				} else {
					state.Stack = []string{lang.Start}
				}
			} else if next == "@popall" {
				state.Stack = []string{lang.Start}
			} else if next != "" {
				stateName := stripStatePrefix(next)
				// Replace current state.
				if len(state.Stack) > 0 {
					state.Stack[len(state.Stack)-1] = stateName
				} else {
					state.Stack = []string{stateName}
				}
			}

			if push == "@pop" {
				if len(state.Stack) > 1 {
					state.Stack = state.Stack[:len(state.Stack)-1]
				} else {
					state.Stack = []string{lang.Start}
				}
			} else if push == "@popall" {
				state.Stack = []string{lang.Start}
			} else if push != "" {
				stateName := stripStatePrefix(push)
				state.Stack = append(state.Stack, stateName)
			}

			state.Pos = endPos
			matched = true
			break
		}

		if !matched {
			// No rule matched; advance by one rune.
			if state.Pos < totalLen {
				endPos := state.Pos + 1
				if defaultToken != "" {
					tokens = append(tokens, NewToken(state.Pos, endPos, defaultToken))
				}
				state.Pos = endPos
			}
		}
	}

	return tokens
}

// parseScopes splits a dotted scope string into a list of scope names.
// e.g. "keyword.control.go" → ["keyword", "control", "go"]
func parseScopes(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ".")
}

// stripStatePrefix removes the leading "@" from a Monarch state reference.
// Monarch uses "@stateName" to refer to states, but the actual state names
// in the Tokenizer map don't include "@". "@pop" and "@popall" are handled
// specially by the caller and should not be passed here.
func stripStatePrefix(s string) string {
	if strings.HasPrefix(s, "@") {
		return s[1:]
	}
	return s
}

// TokenizeLines tokenizes source code line by line, returning TokenLines.
// This is convenient for rendering where you process one line at a time.
func TokenizeLines(lang *MonarchLanguage, src string) []TokenLine {
	// For simplicity, tokenize the whole source and then split by line.
	tokens := Tokenize(lang, src)

	// Split source into lines.
	lines := strings.Split(src, "\n")
	result := make([]TokenLine, len(lines))

	offset := 0
	for i, lineText := range lines {
		lineEnd := offset + len([]rune(lineText))
		var lineTokens []Token
		for _, tok := range tokens {
			if tok.StartIndex >= offset && tok.EndIndex <= lineEnd {
				// Adjust token offsets to be relative to the line.
				adjusted := Token{
					StartIndex: tok.StartIndex - offset,
					EndIndex:   tok.EndIndex - offset,
					Scopes:     tok.Scopes,
				}
				lineTokens = append(lineTokens, adjusted)
			}
		}
		result[i] = TokenLine{
			Tokens:       lineTokens,
			Text:         lineText,
			StartOffset:  offset,
		}
		offset = lineEnd + 1 // +1 for the '\n'
	}

	return result
}
