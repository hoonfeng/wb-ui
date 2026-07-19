with open('jsc/interpreter.go', 'r', encoding='utf-8') as f:
    lines = f.readlines()

# Find proxyGet
for i, line in enumerate(lines, 1):
    if 'proxyGet' in line or 'proxyApply' in line:
        print(f'{i}: {line.rstrip()[:200]}')
