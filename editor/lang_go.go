// Go language definition for the Monarch tokenizer.
// Based on VS Code's vscode-go Monarch definition
// (https://github.com/golang/vscode-go/blob/master/syntaxes/go.tmLanguage.json)
// translated to Go's MonarchLanguage struct.

package editor

// LangGo returns a Monarch language definition for Go source code.
// It recognizes keywords, built-in types and functions, strings (including
// raw strings), comments (line and block), numbers, and operators.
func LangGo() *MonarchLanguage {
	return &MonarchLanguage{
		Start:        "root",
		DefaultToken: "source.go",
		Tokenizer: map[string][]MonarchRule{
			"root": {
				// Whitespace.
				{Regex: `[ \t\r\n]+`, Action: MonarchAction{Token: ""}},
				// Line comment.
				{Regex: `//[^\n]*`, Action: MonarchAction{Token: "comment.line.double-slash.go"}},
				// Block comment.
				{Regex: `/\*`, Action: MonarchAction{Token: "comment.block.go", Next: "@blockComment"}},
				// Double-quoted string.
				{Regex: `"`, Action: MonarchAction{Token: "string.quoted.double.go", Next: "@dQString"}},
				// Single-quoted rune literal.
				{Regex: `'`, Action: MonarchAction{Token: "string.quoted.single.go", Next: "@sQString"}},
				// Raw string (backtick).
				{Regex: "`", Action: MonarchAction{Token: "string.quoted.raw.go", Next: "@rawString"}},
				// Keywords.
				{Regex: `\b(break|case|chan|const|continue|default|defer|else|fallthrough|for|func|go|goto|if|import|interface|map|package|range|return|select|struct|switch|type|var)\b`,
					Action: MonarchAction{Token: "keyword.control.go"}},
				// Constants.
				{Regex: `\b(true|false|nil|iota)\b`,
					Action: MonarchAction{Token: "constant.language.go"}},
				// Built-in types.
				{Regex: `\b(bool|byte|complex64|complex128|float32|float64|int|int8|int16|int32|int64|rune|string|uint|uint8|uint16|uint32|uint64|uintptr|error|any)\b`,
					Action: MonarchAction{Token: "support.type.builtin.go"}},
				// Built-in functions.
				{Regex: `\b(append|cap|close|complex|copy|delete|imag|len|make|new|panic|print|println|real|recover)\b`,
					Action: MonarchAction{Token: "support.function.builtin.go"}},
				// Hex number.
				{Regex: `\b0[xX][0-9a-fA-F]+\b`,
					Action: MonarchAction{Token: "constant.numeric.hex.go"}},
				// Octal number.
				{Regex: `\b0[oO][0-7]+\b`,
					Action: MonarchAction{Token: "constant.numeric.octal.go"}},
				// Binary number.
				{Regex: `\b0[bB][01]+\b`,
					Action: MonarchAction{Token: "constant.numeric.binary.go"}},
				// Float (with exponent).
				{Regex: `\b[0-9]+\.[0-9]+([eE][+-]?[0-9]+)?\b`,
					Action: MonarchAction{Token: "constant.numeric.float.go"}},
				{Regex: `\b[0-9]+[eE][+-]?[0-9]+\b`,
					Action: MonarchAction{Token: "constant.numeric.float.go"}},
				// Integer.
				{Regex: `\b[0-9]+\b`,
					Action: MonarchAction{Token: "constant.numeric.integer.go"}},
				// Punctuation.
				{Regex: `[{}()\[\],;.:]`,
					Action: MonarchAction{Token: "punctuation.go"}},
				// Operators.
				{Regex: `[+\-*/%=<>!&|^~]`,
					Action: MonarchAction{Token: "keyword.operator.go"}},
				{Regex: `:=|==|!=|<=|>=|&&|\|\||<<|>>|\+\+|--`,
					Action: MonarchAction{Token: "keyword.operator.go"}},
				// Identifier (including function/type names after matching keywords).
				{Regex: `[A-Za-z_][A-Za-z0-9_]*`,
					Action: MonarchAction{Token: "identifier.go"}},
			},
			"blockComment": {
				{Regex: `\*/`, Action: MonarchAction{Token: "comment.block.go", Next: "@pop"}},
				{Regex: `[^\*]+`, Action: MonarchAction{Token: "comment.block.go"}},
				{Regex: `\*`, Action: MonarchAction{Token: "comment.block.go"}},
			},
			"dQString": {
				{Regex: `\\(u[0-9a-fA-F]{4}|U[0-9a-fA-F]{8}|[abfnrtv\\"'x[0-9a-fA-F]{2}])`,
					Action: MonarchAction{Token: "string.escape.go"}},
				{Regex: `"`, Action: MonarchAction{Token: "string.quoted.double.go", Next: "@pop"}},
				{Regex: `[^"\\]+`, Action: MonarchAction{Token: "string.quoted.double.go"}},
				{Regex: `\\.`, Action: MonarchAction{Token: "string.escape.go"}},
			},
			"sQString": {
				{Regex: `\\(u[0-9a-fA-F]{4}|U[0-9a-fA-F]{8}|[abfnrtv\\"'x[0-9a-fA-F]{2}])`,
					Action: MonarchAction{Token: "string.escape.go"}},
				{Regex: `'`, Action: MonarchAction{Token: "string.quoted.single.go", Next: "@pop"}},
				{Regex: `[^'\\]+`, Action: MonarchAction{Token: "string.quoted.single.go"}},
				{Regex: `\\.`, Action: MonarchAction{Token: "string.escape.go"}},
			},
			"rawString": {
				{Regex: "`", Action: MonarchAction{Token: "string.quoted.raw.go", Next: "@pop"}},
				{Regex: `[^` + "`" + `]+`, Action: MonarchAction{Token: "string.quoted.raw.go"}},
			},
		},
	}
}

// LangJS returns a Monarch language definition for JavaScript source code.
// Based on VS Code's monaco-languages/typescript Monarch definition.
func LangJS() *MonarchLanguage {
	return &MonarchLanguage{
		Start:        "root",
		DefaultToken: "source.js",
		Tokenizer: map[string][]MonarchRule{
			"root": {
				{Regex: `[ \t\r\n]+`, Action: MonarchAction{Token: ""}},
				{Regex: `//[^\n]*`, Action: MonarchAction{Token: "comment.line.double-slash.js"}},
				{Regex: `/\*`, Action: MonarchAction{Token: "comment.block.js", Next: "@blockComment"}},
				{Regex: `"(?:\\.|[^"\\])*"`, Action: MonarchAction{Token: "string.quoted.double.js"}},
				{Regex: `'(?:\\.|[^'\\])*'`, Action: MonarchAction{Token: "string.quoted.single.js"}},
				{Regex: "`(?:\\.|[^`\\\\])*`", Action: MonarchAction{Token: "string.quoted.template.js"}},
				// Keywords.
				{Regex: `\b(break|case|catch|class|const|continue|debugger|default|delete|do|else|enum|export|extends|finally|for|function|if|import|in|instanceof|let|new|return|super|switch|this|throw|try|typeof|var|void|while|with|yield|async|await|of)\b`,
					Action: MonarchAction{Token: "keyword.control.js"}},
				// Literals.
				{Regex: `\b(true|false|null|undefined|NaN|Infinity)\b`,
					Action: MonarchAction{Token: "constant.language.js"}},
				// Numbers.
				{Regex: `\b0[xX][0-9a-fA-F]+\b`,
					Action: MonarchAction{Token: "constant.numeric.hex.js"}},
				{Regex: `\b[0-9]+\.[0-9]+([eE][+-]?[0-9]+)?\b`,
					Action: MonarchAction{Token: "constant.numeric.float.js"}},
				{Regex: `\b[0-9]+\b`,
					Action: MonarchAction{Token: "constant.numeric.integer.js"}},
				// Operators.
				{Regex: `=>|===|!==|==|!=|<=|>=|&&|\|\||<<|>>|\+\+|--`,
					Action: MonarchAction{Token: "keyword.operator.js"}},
				{Regex: `[+\-*/%=<>!&|^~?]`,
					Action: MonarchAction{Token: "keyword.operator.js"}},
				// Punctuation.
				{Regex: `[{}()\[\];,.:]`,
					Action: MonarchAction{Token: "punctuation.js"}},
				// Identifiers.
				{Regex: `[A-Za-z_$][A-Za-z0-9_$]*`,
					Action: MonarchAction{Token: "identifier.js"}},
			},
			"blockComment": {
				{Regex: `\*/`, Action: MonarchAction{Token: "comment.block.js", Next: "@pop"}},
				{Regex: `[^*]+`, Action: MonarchAction{Token: "comment.block.js"}},
				{Regex: `\*`, Action: MonarchAction{Token: "comment.block.js"}},
			},
		},
	}
}
