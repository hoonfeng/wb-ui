import sys
with open('jsc/parser.go', 'r', encoding='utf-8') as f:
    c = f.read()
# Add bounds check before raw[endIdx+3:]
old = 'exprSrc := raw[:endIdx]\n\t\traw = raw[endIdx+3:]'
new = 'exprSrc := raw[:endIdx]\n\t\tif endIdx+3 > len(raw) {\n\t\t\tp.errorf("template truncated at end")\n\t\t\tbreak\n\t\t}\n\t\traw = raw[endIdx+3:]'
if old in c:
    c = c.replace(old, new)
    with open('jsc/parser.go', 'w', encoding='utf-8') as f:
        f.write(c)
    print('OK')
else:
    print('NOT FOUND')
    sys.exit(1)
