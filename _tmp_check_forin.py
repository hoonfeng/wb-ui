with open('jsc/interpreter.go', 'r', encoding='utf-8') as f:
    for i, line in enumerate(f, 1):
        if 'BeginForIn' in line or 'ForInNext' in line or 'EndForIn' in line:
            print(f'{i}: {line.rstrip()[:200]}')
