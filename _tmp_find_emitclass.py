with open('jsc/bytecode.go', 'r', encoding='utf-8') as f:
    lines = f.readlines()
    
for i, line in enumerate(lines, 1):
    if 'emitClass' in line:
        print(f'{i}: {line.rstrip()[:200]}')
