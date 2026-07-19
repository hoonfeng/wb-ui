with open('jsc/interpreter.go', 'r', encoding='utf-8') as f:
    for i, line in enumerate(f, 1):
        if 'String' in line and ('Global' in line or 'global' in line):
            print(f'{i}: {line.rstrip()[:200]}')
