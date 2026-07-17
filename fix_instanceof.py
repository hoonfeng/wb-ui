import sys
with open('jsc/parser.go', 'r', encoding='utf-8') as f:
    c = f.read()
old = '\t\t}\n\t\tprec := BinaryPrecedence(op)'
new = '\t\t}\n\t// Convert instanceof keyword to plain token kind\n\tif op.IsKeyword() && op.KeywordOf() == KeywordInstanceof {\n\t\top = TokenInstanceOf\n\t}\n\tprec := BinaryPrecedence(op)'
if old in c:
    c = c.replace(old, new)
    with open('jsc/parser.go', 'w', encoding='utf-8') as f:
        f.write(c)
    print('OK')
else:
    print('NOT FOUND')
    sys.exit(1)
