import os
# Check Array.of in globalobject.go
with open('jsc/globalobject.go', 'r', encoding='utf-8') as f:
    for i, line in enumerate(f, 1):
        if 'Array' in line and ('of' in line.lower() or 'from' in line.lower()):
            print(f'{i}: {line.rstrip()[:200]}')
