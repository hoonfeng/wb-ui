with open('jsc/bytecode.go', 'r', encoding='utf-8') as f:
    for i, line in enumerate(f, 1):
        if 'of' in line.lower() and ('"of"' in line.lower() or "'of'" in line.lower()):
            print(f'{i}: {line.rstrip()[:200]}')
