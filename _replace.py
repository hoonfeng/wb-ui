# -*- coding: utf-8 -*-
import io

with io.open(r'F:\syproject\wb-ui\layout\blockformattingcontext.go', 'r', encoding='utf-8', newline='') as f:
    content = f.read()

# Replace the for loop line - use exact CRLF
content = content.replace(
    'for _, child := range box.Children() {\r\n',
    'for i, child := range box.Children() {\r\n',
    1
)

# Replace the !invisible format string
content = content.replace(
    '  !invisible child=%s\\r\\n',
    '  [!INVIS i=%d child=%s type=%T]\\r\\n',
    1
)

# Replace the !invisible args - add i and type
content = content.replace(
    'elementNameOf(child))',
    'i, elementNameOf(child), child)',
    1
)

# Replace the !notEb format string  
content = content.replace(
    '  !notEb child=%T\\r\\n',
    '  [!NOTEB i=%d child=%T]\\r\\n',
    1
)

# Replace the !notEb args - add i
content = content.replace(
    'fmt.Fprintf(diagFloats, "  [!NOTEB i=%d child=%T]\\r\\n", child)',
    'fmt.Fprintf(diagFloats, "  [!NOTEB i=%d child=%T]\\r\\n", i, child)',
    1
)

# Replace the -- child diagnostic with style pointer
content = content.replace(
    '\t\tfmt.Fprintf(diagFloats, "  -- child=%s float=%q width=%v display=%d\\r\\n",\r\n\t\t\telementName(childEb), childCs.Float, childCs.Width, childCs.Display)',
    '\t\tfmt.Fprintf(diagFloats, "  [i=%d] child=%s float=%q width=%v display=%d style=%p\\r\\n",\r\n\t\t\ti, elementName(childEb), childCs.Float, childCs.Width, childCs.Display, childCs)',
    1
)

with io.open(r'F:\syproject\wb-ui\layout\blockformattingcontext.go', 'w', encoding='utf-8', newline='') as f:
    f.write(content)

print('Done')
