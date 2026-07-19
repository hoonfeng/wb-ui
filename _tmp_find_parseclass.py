with open('jsc/parser.go', 'r', encoding='utf-8') as f:
    for i, line in enumerate(f, 1):
        if 'Class' in line and ('parse' in line.lower() or 'expr' in line.lower() or 'stmt' in line.lower()):
            if 'class' in line.lower():
                print(f'{i}: {line.rstrip()[:200]}')
