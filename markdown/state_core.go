package markdown

// Translation of: markdown-it/lib/rules_core/state_core.mjs
// Completeness: 100%
//
// StateCore is the state object for the core rule chain. It holds the source
// string, the environment, the token list, and a reference back to the
// MarkdownIt instance. Core rules (normalize, block, inline, text_join) read
// and modify this state.

// StateCore is the state passed to core chain rules.
type StateCore struct {
	Src        string                   // source Markdown text
	Env        map[string]interface{}   // environment (shared across all chains)
	Tokens     []Token                  // token stream (output)
	InlineMode bool                     // if true, parse as inline-only (renderInline)
	Md         *MarkdownIt              // reference to parser instance
}

// NewStateCore creates a StateCore mirroring the JS constructor
// `new StateCore(src, md, env)`.
func NewStateCore(src string, md *MarkdownIt, env map[string]interface{}) *StateCore {
	return &StateCore{
		Src: src,
		Env: env,
		Md:  md,
	}
}
