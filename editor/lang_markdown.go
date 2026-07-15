// Markdown language definition for the Monarch tokenizer.
// Based on @codemirror/lang-markdown and VS Code's markdown.tmLanguage
// simplified to common Markdown elements.

package editor

// LangMarkdown returns a Monarch language definition for Markdown source.
// It recognizes headings, emphasis, code spans, code blocks, links, images,
// lists, blockquotes, and horizontal rules.
func LangMarkdown() *MonarchLanguage {
	return &MonarchLanguage{
		Start:        "root",
		DefaultToken: "text.md",
		Tokenizer: map[string][]MonarchRule{
			"root": {
				// Fenced code block (``` or ~~~).
				{Regex: "```[a-zA-Z0-9+-]*", Action: MonarchAction{Token: "string.other.begin.code.fence.md", Next: "@codeFence"}},
				{Regex: "~~~[a-zA-Z0-9+-]*", Action: MonarchAction{Token: "string.other.begin.code.fence.md", Next: "@codeFence"}},
				// Heading (ATX: # ## ### ...).
				{Regex: `^#{1,6}[ \t]+`, Action: MonarchAction{Token: "markup.heading.atx.md", Next: "@headingContent"}},
				// Heading (Setext: === or --- under text, handled at line start).
				{Regex: `^={3,}[ \t]*$`, Action: MonarchAction{Token: "markup.heading.setext.md"}},
				{Regex: `^-{3,}[ \t]*$`, Action: MonarchAction{Token: "markup.heading.setext.md"}},
				// Blockquote.
				{Regex: `^>[ \t]?`, Action: MonarchAction{Token: "markup.quote.md"}},
				// Horizontal rule.
				{Regex: `^[\*\-_]{3,}[ \t]*$`, Action: MonarchAction{Token: "meta.separator.md"}},
				// Unordered list.
				{Regex: `^[\*\-+][ \t]+`, Action: MonarchAction{Token: "markup.list.unordered.md"}},
				// Ordered list.
				{Regex: `^[0-9]+\.[ \t]+`, Action: MonarchAction{Token: "markup.list.ordered.md"}},
				// Task list item.
				{Regex: `^\[[ xX]\][ \t]+`, Action: MonarchAction{Token: "markup.list.task.md"}},
				// Bold + italic: ***text*** or ___text___
				{Regex: `\*\*\*[^*]+\*\*\*`, Action: MonarchAction{Token: "markup.bold.italic.md"}},
				{Regex: `___[^_]+___`, Action: MonarchAction{Token: "markup.bold.italic.md"}},
				// Bold: **text** or __text__
				{Regex: `\*\*[^*]+\*\*`, Action: MonarchAction{Token: "markup.bold.md"}},
				{Regex: `__[^_]+__`, Action: MonarchAction{Token: "markup.bold.md"}},
				// Italic: *text* or _text_
				{Regex: `\*[^*]+\*`, Action: MonarchAction{Token: "markup.italic.md"}},
				{Regex: `_[^_]+_`, Action: MonarchAction{Token: "markup.italic.md"}},
				// Strikethrough: ~~text~~
				{Regex: `~~[^~]+~~`, Action: MonarchAction{Token: "markup.strikethrough.md"}},
				// Inline code span: `code`
				{Regex: "`[^`]+`", Action: MonarchAction{Token: "string.code.md"}},
				// Image: ![alt](url)
				{Regex: `!\[[^\]]*\]\([^\)]+\)`, Action: MonarchAction{Token: "string.image.md"}},
				// Link: [text](url)
				{Regex: `\[[^\]]*\]\([^\)]+\)`, Action: MonarchAction{Token: "string.link.md"}},
				// URL autolink: <url>
				{Regex: `<https?://[^>]+>`, Action: MonarchAction{Token: "string.link.url.md"}},
				// Newline.
				{Regex: `\n`, Action: MonarchAction{Token: ""}},
				// Any text.
				{Regex: `[^\n*` + "`" + `_~\[\]!]+`, Action: MonarchAction{Token: "text.md"}},
				// Single character fallback.
				{Regex: `.`, Action: MonarchAction{Token: "text.md"}},
			},
			"headingContent": {
				{Regex: `\n`, Action: MonarchAction{Token: "markup.heading.atx.md", Next: "@pop"}},
				{Regex: `[^\n]+`, Action: MonarchAction{Token: "markup.heading.atx.content.md"}},
			},
			"codeFence": {
				{Regex: "```", Action: MonarchAction{Token: "string.other.end.code.fence.md", Next: "@pop"}},
				{Regex: "~~~", Action: MonarchAction{Token: "string.other.end.code.fence.md", Next: "@pop"}},
				{Regex: `[^\n` + "`" + `]+`, Action: MonarchAction{Token: "string.code.fence.md"}},
				{Regex: `\n`, Action: MonarchAction{Token: "string.code.fence.md"}},
			},
		},
	}
}
