# -*- coding: utf-8 -*-
import io

with io.open(r'F:\syproject\wb-ui\layout\blockformattingcontext.go', 'r', encoding='utf-8', newline='') as f:
    content = f.read()

# Fix 1: [!NOTEB] args - add i before child
# Current: fmt.Fprintf(diagFloats, "  [!NOTEB i=%d child=%T]\n", child)
content = content.replace(
    '  [!NOTEB i=%d child=%T]\\n", child)',
    '  [!NOTEB i=%d child=%T]\\n", i, child)',
    1
)

# Fix 2: Replace -- child diagnostic with [i=%d] + style=%p
old2 = (
    '\t\tfmt.Fprintf(diagFloats, "  -- child=%s float=%q width=%v display=%d\\n",\r\n'
    '\t\t\telementName(childEb), childCs.Float, childCs.Width, childCs.Display)'
)
new2 = (
    '\t\tfmt.Fprintf(diagFloats, "  [i=%d] child=%s float=%q width=%v display=%d style=%p\\n",\r\n'
    '\t\t\ti, elementName(childEb), childCs.Float, childCs.Width, childCs.Display, childCs)'
)
content = content.replace(old2, new2, 1)

with io.open(r'F:\syproject\wb-ui\layout\blockformattingcontext.go', 'w', encoding='utf-8', newline='') as f:
    f.write(content)

print('Done')
