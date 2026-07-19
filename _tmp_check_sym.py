with open('jsc/globalobject.go', 'r', encoding='utf-8') as f:
    for i, line in enumerate(f, 1):
        if 'iterator' in line.lower() or '[Symbol' in line or 'Symbol' in line:
            print(f'{i}: {line.rstrip()[:200]}')
