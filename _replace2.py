# -*- coding: utf-8 -*-
import io

with io.open(r'F:\syproject\wb-ui\layout\blockformattingcontext.go', 'r', encoding='utf-8', newline='') as f:
    content = f.read()

# Fix the !NOTEB args - add missing i
old1 = '''"  [!NOTEB i=%d child=%T]\\r\\n", child)'''
new1 = '''"  [!NOTEB i=%d child=%T]\\r\\n", i, child)'''
content = content.replace(old1, new1, 1)

# Replace the -- child diagnostic with style pointer and i
old2 = '''\t\tfmt.Fprintf(diagFloats, "  -- child=%s float=%q width=%v display=%d\\r\\n",\r\n\t\t\telementName(childEb), childCs.Float, childCs.Width, childCs.Display)'''
new2 = '''\t\tfmt.Fprintf(diagFloats, "  [i=%d] child=%s float=%q width=%v display=%d style=%p\\r\\n",\r\n\t\t\ti, elementName(childEb), childCs.Float, childCs.Width, childCs.Display, childCs)'''
content = content.replace(old2, new2, 1)

with io.open(r'F:\syproject\wb-ui\layout\blockformattingcontext.go', 'w', encoding='utf-8', newline='') as f:
    f.write(content)

print('Done')
