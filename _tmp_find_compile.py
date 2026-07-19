import os
# Find compileFunction definition
with open('jsc/bytecode.go', 'r', encoding='utf-8') as f:
    lines = f.readlines()
    
# Search for compileFunction definition (not usage)
found = False
for i, line in enumerate(lines, 1):
    if 'func compileFunction' in line:
        print(f'### compileFunction at line {i}')
        # Print next 20 lines
        for j in range(i, min(i+40, len(lines)+1)):
            print(f'{j}: {lines[j-1].rstrip()[:200]}')
        found = True
        break

if not found:
    print("compileFunction not found in bytecode.go!")
