import sys
with open('jsc/parser.go', 'r', encoding='utf-8') as f:
    c = f.read()
# Replace the entire template substitution handling block
old = '''		exprSrc := raw[:endIdx]
		if endIdx+3 > len(raw) {
			p.errorf("template truncated at end")
			break
		}
		raw = raw[endIdx+3:]
		// Parse the substitution expression by re-entering the parser.'''
new = '''		exprSrc := raw[:endIdx]
		if endIdx+3 >= len(raw) {
			// Expression may be empty or at end; skip gracefully.
			raw = ""
			continue
		}
		raw = raw[endIdx+3:]
		// Parse the substitution expression by re-entering the parser.'''
if old in c:
    c = c.replace(old, new)
    with open('jsc/parser.go', 'w', encoding='utf-8') as f:
        f.write(c)
    print('OK')
else:
    print('NOT FOUND')
    sys.exit(1)
